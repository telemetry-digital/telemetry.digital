// Package alarm evaluates high/low limits on new observations, raises and clears alarms, writes the
// append-only alarm log and triggers notifications (URS C-01, C-05, C-07, C-10).
//
// PoC scope: types high/low with warning/action limits and hysteresis; delay is recorded but not yet enforced
// (a rule with delay > 0 is evaluated as delay = 0 and logged as such). Escalation ladders come in phase 1.
package alarm

import (
	"context"
	"fmt"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/metrics"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/escalate"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/ingest"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/notify"
)

type rule struct {
	ID         string
	Type       string
	Warning    *float64
	Action     *float64
	Hysteresis float64
	Delay      time.Duration
}

// LiveSink receives alarm changes for live streams (optional).
type LiveSink interface {
	Alarm(org, alarmID, datastreamID, state, severity string, value *float64)
}

// Engine implements ingest.Sink.
type Engine struct {
	pool  *pgxpool.Pool
	log   *slog.Logger
	notif *notify.Notifier
	Live  LiveSink
	// Escalate delivers through the rule's escalation policy; false = no policy, fall back to the global SMTP recipients.
	Escalate interface {
		OnAlarm(ctx context.Context, a escalate.Alarm) bool
	}

	mu     sync.Mutex
	rules  map[string][]rule // datastream id -> rules
	loaded map[string]time.Time
}

// New creates the engine.
func New(pool *pgxpool.Pool, log *slog.Logger, n *notify.Notifier) *Engine {
	return &Engine{pool: pool, log: log.With("component", "alarm"), notif: n, rules: map[string][]rule{}, loaded: map[string]time.Time{}}
}

// OnObservations evaluates every numeric observation against the active rules of its datastream.
func (e *Engine) OnObservations(ctx context.Context, obs []ingest.Observation) {
	for _, o := range obs {
		if o.Value == nil {
			continue
		}
		rules, err := e.rulesFor(ctx, o.DatastreamID)
		if err != nil {
			e.log.Error("rule lookup failed", "err", err)
			continue
		}
		for _, r := range rules {
			if err := e.evaluate(ctx, o, r); err != nil {
				e.log.Error("evaluation failed", "rule", r.ID, "err", err)
			}
		}
	}
}

func (e *Engine) rulesFor(ctx context.Context, dsID string) ([]rule, error) {
	e.mu.Lock()
	if rs, ok := e.rules[dsID]; ok && time.Since(e.loaded[dsID]) < 30*time.Second {
		e.mu.Unlock()
		return rs, nil
	}
	e.mu.Unlock()
	rows, err := e.pool.Query(ctx, `
		SELECT id::text, type, warning::float8, action::float8, hysteresis::float8, EXTRACT(EPOCH FROM delay)
		FROM core.alarm_rule
		WHERE datastream_id = $1::uuid AND enabled AND valid_to IS NULL AND type IN ('high','low')`, dsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rs []rule
	for rows.Next() {
		var r rule
		var delaySec float64
		if err := rows.Scan(&r.ID, &r.Type, &r.Warning, &r.Action, &r.Hysteresis, &delaySec); err != nil {
			return nil, err
		}
		r.Delay = time.Duration(delaySec * float64(time.Second))
		if r.Delay > 0 {
			e.log.Warn("alarm delay not yet enforced in PoC; evaluating immediately", "rule", r.ID, "delay", r.Delay)
		}
		rs = append(rs, r)
	}
	e.mu.Lock()
	e.rules[dsID] = rs
	e.loaded[dsID] = time.Now()
	e.mu.Unlock()
	return rs, rows.Err()
}

// severity returns "", "warning" or "action" for the value under the rule.
func severity(r rule, v float64) string {
	switch r.Type {
	case "high":
		if r.Action != nil && v >= *r.Action {
			return "action"
		}
		if r.Warning != nil && v >= *r.Warning {
			return "warning"
		}
	case "low":
		if r.Action != nil && v <= *r.Action {
			return "action"
		}
		if r.Warning != nil && v <= *r.Warning {
			return "warning"
		}
	}
	return ""
}

// backInBand reports whether v is inside the limits by at least the hysteresis (for clearing).
func backInBand(r rule, v float64) bool {
	limit := r.Warning
	if limit == nil {
		limit = r.Action
	}
	if limit == nil {
		return true
	}
	if r.Type == "high" {
		return v < *limit-r.Hysteresis
	}
	return v > *limit+r.Hysteresis
}

func (e *Engine) evaluate(ctx context.Context, o ingest.Observation, r rule) error {
	v := *o.Value
	sev := severity(r, v)

	var alarmID, state, curSev string
	err := e.pool.QueryRow(ctx, `SELECT id::text, state, severity FROM core.alarm WHERE rule_id = $1::uuid AND cleared_at IS NULL ORDER BY raised_at DESC LIMIT 1`, r.ID).
		Scan(&alarmID, &state, &curSev)
	hasActive := err == nil

	switch {
	case sev != "" && !hasActive:
		return e.raise(ctx, o, r, sev, v)
	case sev == "action" && hasActive && curSev == "warning":
		// Escalate severity in place: record the event; the alarm row keeps the highest severity.
		if _, err := e.pool.Exec(ctx, `UPDATE core.alarm SET severity = 'action', value = $2::numeric WHERE id = $1::uuid`, alarmID, fmtNum(v)); err != nil {
			return err
		}
		_, err := e.pool.Exec(ctx, `INSERT INTO core.alarm_event (alarm_id, event, detail) VALUES ($1::uuid, 'escalated', jsonb_build_object('severity','action','value',$2::numeric))`, alarmID, fmtNum(v))
		if e.Live != nil {
			e.Live.Alarm(o.OrgID, alarmID, o.DatastreamID, "active", "action", &v)
		}
		return err
	case sev == "" && hasActive && backInBand(r, v):
		if _, err := e.pool.Exec(ctx, `UPDATE core.alarm SET state = 'cleared', cleared_at = now() WHERE id = $1::uuid`, alarmID); err != nil {
			return err
		}
		if _, err := e.pool.Exec(ctx, `INSERT INTO core.alarm_event (alarm_id, event, detail) VALUES ($1::uuid, 'cleared', jsonb_build_object('value',$2::numeric))`, alarmID, fmtNum(v)); err != nil {
			return err
		}
		e.log.Info("alarm cleared", "alarm", alarmID, "datastream", o.Key, "value", v)
		if e.Escalate != nil {
			e.Escalate.OnAlarm(ctx, escalate.Alarm{OrgID: o.OrgID, ID: alarmID, RuleID: r.ID, DatastreamID: o.DatastreamID, Key: o.Key, Severity: curSev, Value: v, Event: "cleared", MeasuredAt: o.MeasuredAt})
		}
		if e.Live != nil {
			e.Live.Alarm(o.OrgID, alarmID, o.DatastreamID, "cleared", curSev, &v)
		}
	}
	return nil
}

func (e *Engine) raise(ctx context.Context, o ingest.Observation, r rule, sev string, v float64) error {
	var alarmID string
	err := e.pool.QueryRow(ctx, `
		INSERT INTO core.alarm (org_id, rule_id, datastream_id, severity, retrospective, value, detail)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6::numeric, jsonb_build_object('key', $7::text, 'measured_at', $8::timestamptz))
		RETURNING id::text`, o.OrgID, r.ID, o.DatastreamID, sev, o.Backfilled, fmtNum(v), o.Key, o.MeasuredAt).Scan(&alarmID)
	if err != nil {
		return err
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO core.alarm_event (alarm_id, event, detail) VALUES ($1::uuid, 'raised', jsonb_build_object('severity',$2::text,'value',$3::numeric))`, alarmID, sev, fmtNum(v)); err != nil {
		return err
	}
	if o.Backfilled {
		if _, err := e.pool.Exec(ctx, `INSERT INTO core.alarm_event (alarm_id, event, detail) VALUES ($1::uuid, 'retrospective', '{}'::jsonb)`, alarmID); err != nil {
			return err
		}
	}
	e.log.Warn("alarm raised", "alarm", alarmID, "severity", sev, "datastream", o.Key, "value", v, "retrospective", o.Backfilled)
	metrics.AlarmsRaised.Inc(sev)
	if !o.ReceivedAt.IsZero() && !o.Backfilled {
		metrics.AlarmLatency.Observe(time.Since(o.ReceivedAt).Seconds())
	}
	if e.Live != nil {
		e.Live.Alarm(o.OrgID, alarmID, o.DatastreamID, "active", sev, &v)
	}

	if e.Escalate != nil && e.Escalate.OnAlarm(ctx, escalate.Alarm{OrgID: o.OrgID, ID: alarmID, RuleID: r.ID, DatastreamID: o.DatastreamID, Key: o.Key, Severity: sev, Value: v, Event: "raised", MeasuredAt: o.MeasuredAt, Retrospective: o.Backfilled}) {
		return nil
	}
	subject := fmt.Sprintf("[ctrl32] %s alarm: %s = %s", sev, o.Key, fmtNum(v))
	body := fmt.Sprintf("Alarm %s\nRule: %s (%s)\nDatastream: %s\nValue: %s\nMeasured at: %s\nRetrospective (backfilled data): %v\n",
		alarmID, r.ID, r.Type, o.Key, fmtNum(v), o.MeasuredAt.Format(time.RFC3339), o.Backfilled)
	delivered, nerr := e.notif.Send(ctx, subject, body)
	event := "notified"
	detail := `{"channel":"email"}`
	if nerr != nil {
		event = "delivery_failed"
		detail = `{"channel":"email","error":` + strconv.Quote(nerr.Error()) + `}`
		e.log.Error("notification failed", "security", true, "alarm", alarmID, "err", nerr)
	} else if !delivered {
		detail = `{"channel":"email","skipped":"smtp not configured"}`
	}
	_, err = e.pool.Exec(ctx, `INSERT INTO core.alarm_event (alarm_id, event, detail) VALUES ($1::uuid, $2, $3::jsonb)`, alarmID, event, detail)
	return err
}

func fmtNum(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

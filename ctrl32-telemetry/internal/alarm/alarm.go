// Package alarm evaluates the alarm rules of datastreams on new observations and on a clock, raises and clears
// alarms, writes the append-only alarm log and triggers notifications (URS C-01, C-05, C-07, C-10).
//
// Rule types (all per datastream):
//   - high, low: the value at or above (high) / at or below (low) the warning and action limits; cleared when the
//     value is back inside the limit by the hysteresis.
//   - low_battery: the datastream carries a battery level (%, V, …); like low.
//   - power_loss: the datastream carries the supply state; a value of 0 (false) means the power is lost (action);
//     any other value clears it. No limits.
//   - door_open: the datastream carries a door contact; a value other than 0 (true) means open (warning); 0 clears
//     it. The delay is how long the door may stay open. No limits.
//   - comm_loss: no new value for longer than the limits in seconds (warning, action); without limits 1.5 × the
//     datastream's expected interval (action). Checked on a clock; a new value clears it.
//
// sensor_fault rules are refused when saved (devices do not report sensor faults); stored ones are not evaluated.
//
// Delay: the condition must hold for the rule's delay before the alarm is raised (in measurement time between
// observations; a clock raises a pending alarm when no newer value ended the condition). A value back in band during
// the delay cancels it. Pending delays are kept in memory: after a restart the delay starts again with the next value.
package alarm

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/escalate"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/ingest"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/metrics"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/notify"
)

// Evaluated lists the rule types the engine evaluates.
var Evaluated = []string{"high", "low", "low_battery", "power_loss", "door_open", "comm_loss"}

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

// pending is a condition waiting for its rule's delay.
type pending struct {
	since time.Time          // measurement time the condition started
	obs   ingest.Observation // the newest observation in the condition
	sev   string
	v     float64
	r     rule
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
	wait   map[string]*pending // rule id -> condition waiting for the delay

	locks [64]sync.Mutex // one evaluation per rule at a time (observations and the clock)
}

// New creates the engine.
func New(pool *pgxpool.Pool, log *slog.Logger, n *notify.Notifier) *Engine {
	return &Engine{pool: pool, log: log.With("component", "alarm"), notif: n, rules: map[string][]rule{}, loaded: map[string]time.Time{}, wait: map[string]*pending{}}
}

func (e *Engine) lock(ruleID string) *sync.Mutex {
	h := fnv.New32a()
	h.Write([]byte(ruleID))
	return &e.locks[h.Sum32()%uint32(len(e.locks))]
}

// OnObservations evaluates every observation against the active rules of its datastream.
func (e *Engine) OnObservations(ctx context.Context, obs []ingest.Observation) {
	for _, o := range obs {
		rules, err := e.rulesFor(ctx, o.DatastreamID)
		if err != nil {
			e.log.Error("rule lookup failed", "err", err)
			continue
		}
		for _, r := range rules {
			if r.Type != "comm_loss" && o.Value == nil {
				continue
			}
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
		SELECT id::text, type, warning::float8, action::float8, hysteresis::float8, EXTRACT(EPOCH FROM delay)::float8
		FROM core.alarm_rule
		WHERE datastream_id = $1::uuid AND enabled AND valid_to IS NULL AND type = ANY($2)`, dsID, Evaluated)
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
	case "comm_loss": // v is the silence in seconds
		if r.Action != nil && v >= *r.Action {
			return "action"
		}
		if r.Warning != nil && v >= *r.Warning {
			return "warning"
		}
	case "low", "low_battery":
		if r.Action != nil && v <= *r.Action {
			return "action"
		}
		if r.Warning != nil && v <= *r.Warning {
			return "warning"
		}
	case "power_loss":
		if v == 0 {
			return "action"
		}
	case "door_open":
		if v != 0 {
			return "warning"
		}
	}
	return ""
}

// backInBand reports whether v is inside the limits by at least the hysteresis (for clearing).
func backInBand(r rule, v float64) bool {
	switch r.Type {
	case "power_loss":
		return v != 0
	case "door_open":
		return v == 0
	}
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

// held reports whether a condition that started at since has held for the delay at time at.
func held(since, at time.Time, delay time.Duration) bool {
	return delay <= 0 || !at.Before(since.Add(delay))
}

func (e *Engine) evaluate(ctx context.Context, o ingest.Observation, r rule) error {
	if r.Type == "comm_loss" {
		// a value arrived: the communication is back
		return e.decide(ctx, o, r, "", 0, true)
	}
	v := *o.Value
	sev := severity(r, v)
	return e.decide(ctx, o, r, sev, v, sev == "" && backInBand(r, v))
}

// decide raises, escalates or clears the rule's alarm for a condition (sev "" = not in alarm), honouring the delay.
// at is the time of the condition: the observation's measurement time.
func (e *Engine) decide(ctx context.Context, o ingest.Observation, r rule, sev string, v float64, clear bool) error {
	l := e.lock(r.ID)
	l.Lock()
	defer l.Unlock()
	var alarmID, state, curSev string
	err := e.pool.QueryRow(ctx, `SELECT id::text, state, severity FROM core.alarm WHERE rule_id = $1::uuid AND cleared_at IS NULL ORDER BY raised_at DESC LIMIT 1`, r.ID).
		Scan(&alarmID, &state, &curSev)
	hasActive := err == nil

	if sev == "" {
		e.mu.Lock()
		delete(e.wait, r.ID)
		e.mu.Unlock()
	}
	switch {
	case sev != "" && !hasActive:
		if r.Delay > 0 {
			e.mu.Lock()
			p := e.wait[r.ID]
			if p == nil {
				p = &pending{since: o.MeasuredAt, r: r}
				e.wait[r.ID] = p
			}
			p.obs, p.sev, p.v, p.r = o, worse(p.sev, sev), v, r
			ok := held(p.since, o.MeasuredAt, r.Delay)
			if ok {
				sev = p.sev
				delete(e.wait, r.ID)
			}
			e.mu.Unlock()
			if !ok {
				return nil
			}
		}
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
	case sev == "" && hasActive && clear:
		if _, err := e.pool.Exec(ctx, `UPDATE core.alarm SET state = 'cleared', cleared_at = now() WHERE id = $1::uuid`, alarmID); err != nil {
			return err
		}
		var val *float64
		if o.Value != nil {
			val = o.Value
		}
		detail := `{}`
		if val != nil {
			detail = `{"value":` + fmtNum(*val) + `}`
		}
		if _, err := e.pool.Exec(ctx, `INSERT INTO core.alarm_event (alarm_id, event, detail) VALUES ($1::uuid, 'cleared', $2::jsonb)`, alarmID, detail); err != nil {
			return err
		}
		e.log.Info("alarm cleared", "alarm", alarmID, "datastream", o.Key, "type", r.Type)
		cv := 0.0
		if val != nil {
			cv = *val
		}
		if e.Escalate != nil {
			e.Escalate.OnAlarm(ctx, escalate.Alarm{OrgID: o.OrgID, ID: alarmID, RuleID: r.ID, DatastreamID: o.DatastreamID, Key: o.Key, Severity: curSev, Value: cv, Event: "cleared", MeasuredAt: o.MeasuredAt})
		}
		if e.Live != nil {
			e.Live.Alarm(o.OrgID, alarmID, o.DatastreamID, "cleared", curSev, val)
		}
	}
	return nil
}

func worse(a, b string) string {
	if a == "action" || b == "action" {
		return "action"
	}
	if a != "" {
		return a
	}
	return b
}

// Run checks on a clock until ctx ends: pending delays whose time has come, and comm_loss rules.
func (e *Engine) Run(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	var lastComm time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			e.raiseDue(ctx, now)
			if now.Sub(lastComm) >= 5*time.Second {
				lastComm = now
				if err := e.checkCommLoss(ctx, now); err != nil && ctx.Err() == nil {
					e.log.Error("comm_loss check failed", "err", err)
				}
			}
		}
	}
}

// raiseDue raises the alarms whose condition has held for the delay with no newer value ending it.
func (e *Engine) raiseDue(ctx context.Context, now time.Time) {
	e.mu.Lock()
	var due []*pending
	for _, p := range e.wait {
		if held(p.since, now, p.r.Delay) {
			due = append(due, p)
		}
	}
	e.mu.Unlock()
	for _, p := range due {
		// a rule that was changed or disabled in the meantime raises nothing
		var current bool
		if err := e.pool.QueryRow(ctx, `SELECT valid_to IS NULL AND enabled FROM core.alarm_rule WHERE id = $1::uuid`, p.r.ID).Scan(&current); err != nil || !current {
			e.mu.Lock()
			if e.wait[p.r.ID] == p {
				delete(e.wait, p.r.ID)
			}
			e.mu.Unlock()
			continue
		}
		o := p.obs
		if p.r.Type == "comm_loss" {
			o.MeasuredAt = now.UTC()
		}
		// decide again at the delay's end: the condition still holds (no newer value cancelled it)
		o2 := o
		o2.MeasuredAt = maxTime(o.MeasuredAt, p.since.Add(p.r.Delay))
		if err := e.decide(ctx, o2, p.r, p.sev, p.v, false); err != nil {
			e.log.Error("delayed alarm failed", "rule", p.r.ID, "err", err)
		}
	}
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// checkCommLoss evaluates comm_loss rules: the silence is the time since the newest value was received (or since the
// rule took effect, when the datastream has no value yet).
func (e *Engine) checkCommLoss(ctx context.Context, now time.Time) error {
	rows, err := e.pool.Query(ctx, `
		SELECT r.id::text, r.org_id::text, r.datastream_id::text, d.key, r.warning::float8, r.action::float8, EXTRACT(EPOCH FROM r.delay)::float8,
		       EXTRACT(EPOCH FROM d.expected_interval)::float8,
		       greatest(r.valid_from, (SELECT o.received_at FROM core.observation o WHERE o.datastream_id = d.id ORDER BY o.measured_at DESC LIMIT 1))
		FROM core.alarm_rule r JOIN core.datastream d ON d.id = r.datastream_id
		WHERE r.type = 'comm_loss' AND r.enabled AND r.valid_to IS NULL`)
	if err != nil {
		return err
	}
	type item struct {
		r    rule
		o    ingest.Observation
		last time.Time
	}
	var list []item
	for rows.Next() {
		var it item
		var delay float64
		var interval *float64
		if err := rows.Scan(&it.r.ID, &it.o.OrgID, &it.o.DatastreamID, &it.o.Key, &it.r.Warning, &it.r.Action, &delay, &interval, &it.last); err != nil {
			rows.Close()
			return err
		}
		it.r.Type = "comm_loss"
		it.r.Delay = time.Duration(delay * float64(time.Second))
		if it.r.Warning == nil && it.r.Action == nil {
			if interval == nil || *interval <= 0 {
				continue // nothing to compare with (refused when saved)
			}
			a := *interval * 1.5
			it.r.Action = &a
		}
		list = append(list, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, it := range list {
		silence := now.Sub(it.last).Seconds()
		sev := severity(it.r, silence)
		it.o.MeasuredAt = now.UTC()
		v := float64(int64(silence))
		// values clear the alarm as they arrive; the clock clears it too (a rule saved a moment ago is not yet in
		// the rule cache of the observations)
		if err := e.decide(ctx, it.o, it.r, sev, v, sev == ""); err != nil {
			e.log.Error("comm_loss evaluation failed", "rule", it.r.ID, "err", err)
		}
	}
	return nil
}

func (e *Engine) raise(ctx context.Context, o ingest.Observation, r rule, sev string, v float64) error {
	var alarmID string
	err := e.pool.QueryRow(ctx, `
		INSERT INTO core.alarm (org_id, rule_id, datastream_id, severity, retrospective, value, detail)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6::numeric, jsonb_build_object('key', $7::text, 'measured_at', $8::timestamptz, 'type', $9::text))
		RETURNING id::text`, o.OrgID, r.ID, o.DatastreamID, sev, o.Backfilled, fmtNum(v), o.Key, o.MeasuredAt, r.Type).Scan(&alarmID)
	if err != nil {
		return err
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO core.alarm_event (alarm_id, event, detail) VALUES ($1::uuid, 'raised', jsonb_build_object('severity',$2::text,'value',$3::numeric,'type',$4::text,'delay_s',$5::float8))`,
		alarmID, sev, fmtNum(v), r.Type, r.Delay.Seconds()); err != nil {
		return err
	}
	if o.Backfilled {
		if _, err := e.pool.Exec(ctx, `INSERT INTO core.alarm_event (alarm_id, event, detail) VALUES ($1::uuid, 'retrospective', '{}'::jsonb)`, alarmID); err != nil {
			return err
		}
	}
	e.log.Warn("alarm raised", "alarm", alarmID, "type", r.Type, "severity", sev, "datastream", o.Key, "value", v, "retrospective", o.Backfilled)
	metrics.AlarmsRaised.Inc(sev)
	if !o.ReceivedAt.IsZero() && !o.Backfilled && r.Delay == 0 {
		metrics.AlarmLatency.Observe(time.Since(o.ReceivedAt).Seconds())
	}
	if e.Live != nil {
		e.Live.Alarm(o.OrgID, alarmID, o.DatastreamID, "active", sev, &v)
	}

	if e.Escalate != nil && e.Escalate.OnAlarm(ctx, escalate.Alarm{OrgID: o.OrgID, ID: alarmID, RuleID: r.ID, DatastreamID: o.DatastreamID, Key: o.Key, Severity: sev, Value: v, Event: "raised", MeasuredAt: o.MeasuredAt, Retrospective: o.Backfilled}) {
		return nil
	}
	what := fmt.Sprintf("%s = %s", o.Key, fmtNum(v))
	if r.Type == "comm_loss" {
		what = fmt.Sprintf("%s: no data for %s s", o.Key, fmtNum(v))
	}
	subject := fmt.Sprintf("[ctrl32] %s alarm: %s", sev, what)
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

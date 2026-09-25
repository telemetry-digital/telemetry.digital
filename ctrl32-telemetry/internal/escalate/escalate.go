// Package escalate turns alarms into notifications through the escalation policy of the alarm rule
// (URS C-06/C-08): step 0 is delivered at once, later steps are scheduled and sent only while the alarm is still
// active and unacknowledged; clearing an alarm cancels the remaining steps and tells the step-0 channels.
// Every attempt is a row in core.notification and an alarm_event, so the delivery history is auditable.
package escalate

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/notify"
)

// Alarm is what the alarm engine reports.
type Alarm struct {
	OrgID, ID, RuleID, DatastreamID, Key string
	Severity                             string
	Value                                float64
	Event                                string // raised | cleared
	MeasuredAt                           time.Time
	Retrospective                        bool
	// PolicyID names the escalation policy directly (alarms raised by automation rules, which have no alarm rule).
	PolicyID string
}

// Step of an escalation policy.
type Step struct {
	AfterS    int    `json:"after_s"`
	ChannelID string `json:"channel_id"`
}

// IncidentSink receives alarms whose escalation step could not be delivered (the incident log, URS C-07).
type IncidentSink interface {
	AlarmUndelivered(ctx context.Context, org, alarmID string, step int, channel, channelKind, errText string, alarm map[string]any)
}

// Engine schedules and delivers notifications.
type Engine struct {
	pool      *pgxpool.Pool
	notif     *notify.Notifier
	log       *slog.Logger
	Tick      time.Duration
	Incidents IncidentSink // optional
}

func New(pool *pgxpool.Pool, notif *notify.Notifier, log *slog.Logger) *Engine {
	return &Engine{pool: pool, notif: notif, log: log.With("component", "escalate"), Tick: 5 * time.Second}
}

// Start runs the dispatcher for scheduled steps.
func (e *Engine) Start(ctx context.Context) {
	go func() {
		t := time.NewTicker(e.Tick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				e.tick(ctx)
			}
		}
	}()
}

func (e *Engine) policySteps(ctx context.Context, ruleID, direct string) (string, []Step) {
	var policyID *string
	if direct != "" {
		policyID = &direct
	} else if err := e.pool.QueryRow(ctx, `SELECT escalation_policy_id::text FROM core.alarm_rule WHERE id = $1::uuid`, ruleID).Scan(&policyID); err != nil || policyID == nil {
		return "", nil
	}
	var raw []byte
	if err := e.pool.QueryRow(ctx, `SELECT steps FROM core.escalation_policy WHERE id = $1::uuid AND revoked_at IS NULL`, *policyID).Scan(&raw); err != nil {
		return "", nil
	}
	var steps []Step
	if json.Unmarshal(raw, &steps) != nil || len(steps) == 0 {
		return "", nil
	}
	return *policyID, steps
}

func (e *Engine) message(a Alarm, event string) notify.Message {
	sev := a.Severity
	subject := fmt.Sprintf("[ctrl32] %s alarm: %s = %s", sev, a.Key, strconv.FormatFloat(a.Value, 'f', -1, 64))
	if event == "cleared" {
		subject = fmt.Sprintf("[ctrl32] cleared: %s = %s", a.Key, strconv.FormatFloat(a.Value, 'f', -1, 64))
	} else if event == "escalated" {
		subject = "[ctrl32] still active — " + subject[len("[ctrl32] "):]
	}
	text := fmt.Sprintf("Alarm %s\nEvent: %s\nRule: %s\nDatastream: %s\nSeverity: %s\nValue: %s\nMeasured at: %s\nRetrospective (backfilled data): %v\n",
		a.ID, event, a.RuleID, a.Key, sev, strconv.FormatFloat(a.Value, 'f', -1, 64), a.MeasuredAt.Format(time.RFC3339), a.Retrospective)
	return notify.Message{Event: event, Subject: subject, Text: text, Alarm: map[string]any{"id": a.ID, "rule_id": a.RuleID, "datastream_id": a.DatastreamID, "key": a.Key, "severity": sev, "value": a.Value, "measured_at": a.MeasuredAt, "retrospective": a.Retrospective}}
}

// OnAlarm schedules the policy steps for a raised alarm or cancels them for a cleared one. It returns false when
// the rule has no policy, so the caller can fall back to the global SMTP recipients.
func (e *Engine) OnAlarm(ctx context.Context, a Alarm) bool {
	policyID, steps := e.policySteps(ctx, a.RuleID, a.PolicyID)
	if policyID == "" {
		return false
	}
	switch a.Event {
	case "raised":
		msg := e.message(a, "raised")
		payload, _ := json.Marshal(msg)
		for i, st := range steps {
			event := "raised"
			if st.AfterS > 0 {
				event = "escalated"
				payload, _ = json.Marshal(e.message(a, "escalated"))
			}
			var nid string
			err := e.pool.QueryRow(ctx, `INSERT INTO core.notification (org_id, alarm_id, channel_id, policy_id, step, event, due_at, payload)
				VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, now() + make_interval(secs => $7), $8::jsonb) RETURNING id::text`,
				a.OrgID, a.ID, st.ChannelID, policyID, i, event, st.AfterS, string(payload)).Scan(&nid)
			if err != nil {
				e.log.Error("schedule notification", "alarm", a.ID, "step", i, "err", err)
				continue
			}
			if st.AfterS == 0 {
				e.send(ctx, nid)
			}
		}
	case "cleared":
		_, _ = e.pool.Exec(ctx, `UPDATE core.notification SET status = 'cancelled', error = 'alarm cleared' WHERE alarm_id = $1::uuid AND status = 'scheduled'`, a.ID)
		payload, _ := json.Marshal(e.message(a, "cleared"))
		for i, st := range steps {
			if st.AfterS != 0 {
				continue
			}
			var nid string
			if err := e.pool.QueryRow(ctx, `INSERT INTO core.notification (org_id, alarm_id, channel_id, policy_id, step, event, payload) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, 'cleared', $6::jsonb) RETURNING id::text`,
				a.OrgID, a.ID, st.ChannelID, policyID, i, string(payload)).Scan(&nid); err == nil {
				e.send(ctx, nid)
			}
		}
	}
	return true
}

// tick delivers due steps of alarms that are still active and unacknowledged; others are cancelled.
func (e *Engine) tick(ctx context.Context) {
	rows, err := e.pool.Query(ctx, `SELECT n.id::text, COALESCE(a.state, 'active'), a.cleared_at IS NOT NULL
		FROM core.notification n LEFT JOIN core.alarm a ON a.id = n.alarm_id
		WHERE n.status = 'scheduled' AND n.due_at <= now() ORDER BY n.due_at LIMIT 100`)
	if err != nil {
		return
	}
	type due struct {
		id      string
		state   string
		cleared bool
	}
	var list []due
	for rows.Next() {
		var d due
		if rows.Scan(&d.id, &d.state, &d.cleared) == nil {
			list = append(list, d)
		}
	}
	rows.Close()
	for _, d := range list {
		if d.cleared || d.state != "active" {
			_, _ = e.pool.Exec(ctx, `UPDATE core.notification SET status = 'cancelled', error = $2 WHERE id = $1::uuid AND status = 'scheduled'`, d.id, "alarm "+d.state)
			continue
		}
		e.send(ctx, d.id)
	}
}

// send delivers one notification row on its channel and records the outcome.
func (e *Engine) send(ctx context.Context, nid string) {
	var ch notify.Channel
	var raw []byte
	var payload []byte
	var alarmID *string
	var event string
	var step int
	err := e.pool.QueryRow(ctx, `SELECT c.id::text, c.org_id::text, c.name, c.kind, c.settings, n.payload, n.alarm_id::text, n.event, n.step
		FROM core.notification n JOIN core.notification_channel c ON c.id = n.channel_id
		WHERE n.id = $1::uuid AND n.status = 'scheduled' AND c.enabled AND c.revoked_at IS NULL`, nid).
		Scan(&ch.ID, &ch.OrgID, &ch.Name, &ch.Kind, &raw, &payload, &alarmID, &event, &step)
	if err != nil {
		_, _ = e.pool.Exec(ctx, `UPDATE core.notification SET status = 'cancelled', error = 'channel disabled or removed' WHERE id = $1::uuid AND status = 'scheduled'`, nid)
		return
	}
	_ = json.Unmarshal(raw, &ch.Settings)
	var msg notify.Message
	_ = json.Unmarshal(payload, &msg)
	msg.SentAt = time.Now().UTC()
	derr := e.notif.Deliver(ctx, ch, msg)
	status, errText := "sent", ""
	if derr != nil {
		status, errText = "failed", derr.Error()
		e.log.Error("notification failed", "security", true, "notification", nid, "channel", ch.Name, "err", derr)
	}
	_, _ = e.pool.Exec(ctx, `UPDATE core.notification SET status = $2, sent_at = now(), attempts = attempts + 1, error = NULLIF($3, '') WHERE id = $1::uuid`, nid, status, errText)
	if alarmID != nil {
		ev, detail := "notified", map[string]any{"channel": ch.Name, "kind": ch.Kind, "event": event}
		if derr != nil {
			ev, detail["error"] = "delivery_failed", derr.Error()
		}
		d, _ := json.Marshal(detail)
		_, _ = e.pool.Exec(ctx, `INSERT INTO core.alarm_event (alarm_id, event, detail) VALUES ($1::uuid, $2, $3::jsonb)`, *alarmID, ev, string(d))
		if derr != nil && e.Incidents != nil && event != "cleared" {
			// Every channel of this step failed (a step has one channel; nothing of this step was delivered) → incident.
			var sent int
			_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM core.notification WHERE alarm_id = $1::uuid AND step = $2 AND status = 'sent'`, *alarmID, step).Scan(&sent)
			if sent == 0 {
				e.Incidents.AlarmUndelivered(ctx, ch.OrgID, *alarmID, step, ch.Name, ch.Kind, derr.Error(), msg.Alarm)
			}
		}
	}
}

// Test delivers a test message on a channel and records it in the log.
func (e *Engine) Test(ctx context.Context, ch notify.Channel) error {
	msg := notify.Message{Event: "test", Subject: "[ctrl32] test notification: " + ch.Name, Text: "This is a test message from ctrl32 telemetry. Channel: " + ch.Name + " (" + ch.Kind + ")."}
	payload, _ := json.Marshal(msg)
	var nid string
	_ = e.pool.QueryRow(ctx, `INSERT INTO core.notification (org_id, channel_id, event, payload) VALUES ($1::uuid, $2::uuid, 'test', $3::jsonb) RETURNING id::text`, ch.OrgID, ch.ID, string(payload)).Scan(&nid)
	err := e.notif.Deliver(ctx, ch, msg)
	status, errText := "sent", ""
	if err != nil {
		status, errText = "failed", err.Error()
	}
	if nid != "" {
		_, _ = e.pool.Exec(ctx, `UPDATE core.notification SET status = $2, sent_at = now(), attempts = 1, error = NULLIF($3, '') WHERE id = $1::uuid`, nid, status, errText)
	}
	return err
}

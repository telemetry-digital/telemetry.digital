// Package alarmmgmt holds what alarm management after ISA-18.2 (spec 035, 0.89.0) adds to the alarm log: the alarm
// classes, the alarm state as people see it (the condition and the acknowledgement are two facts) and shelving. Both
// alarm sources use it: the limit alarms (internal/alarm) and the alarms of automation rules (internal/rules).
package alarmmgmt

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Class is an alarm class of an organization.
type Class struct {
	ID          string `json:"id,omitempty"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Priority    int    `json:"priority"` // 1 (highest) … 4
	Color       string `json:"color"`
	AckRequired bool   `json:"ack_required"`
	Sound       string `json:"sound"` // none, beep, siren
	Builtin     bool   `json:"builtin"`
}

// Builtin are the classes every organization has; an alarm without a class of its rule takes Alarm or Warning by
// its severity (CodeFor).
var Builtin = []Class{
	{Code: "alarm", Name: "Alarm", Priority: 1, Color: "#d63939", AckRequired: true, Sound: "siren", Builtin: true},
	{Code: "warning", Name: "Warning", Priority: 2, Color: "#f59f00", AckRequired: true, Sound: "beep", Builtin: true},
	{Code: "message", Name: "Message", Priority: 4, Color: "#4299e1", AckRequired: false, Sound: "none", Builtin: true},
}

// CodeFor is the built-in class of a severity.
func CodeFor(severity string) string {
	if severity == "warning" {
		return "warning"
	}
	return "alarm"
}

// ClassJoin joins the class of alarm a (rule r may be NULL): the class of its rule, else the built-in class of its
// severity. Needs "LEFT JOIN core.alarm_rule r ON r.id = a.rule_id" before it.
const ClassJoin = ` LEFT JOIN core.alarm_class c ON c.org_id = a.org_id AND (c.id = r.class_id OR (r.class_id IS NULL AND c.code = CASE WHEN a.severity = 'warning' THEN 'warning' ELSE 'alarm' END)) `

// NeedsAck is the SQL condition "alarm a still waits for its acknowledgement" (after ClassJoin).
const NeedsAck = `(a.acked_at IS NULL AND coalesce(c.ack_required, true))`

// ISA states as the API names them.
const (
	UnackActive = "unack_active"
	AckActive   = "ack_active"
	UnackRTN    = "unack_rtn" // returned to normal, not acknowledged
	Normal      = "normal"
	Shelved     = "shelved"
)

// State derives the ISA-18.2 state of an alarm.
func State(cleared, acked, ackRequired, shelved bool) string {
	needs := !acked && ackRequired
	switch {
	case shelved:
		return Shelved
	case !cleared && needs:
		return UnackActive
	case !cleared:
		return AckActive
	case needs:
		return UnackRTN
	}
	return Normal
}

var (
	ensured   = map[string]bool{}
	ensuredMu sync.Mutex
)

// EnsureClasses gives an organization its built-in classes (organizations created after the migration).
func EnsureClasses(ctx context.Context, pool *pgxpool.Pool, org string) error {
	if org == "" {
		return nil
	}
	ensuredMu.Lock()
	done := ensured[org]
	ensuredMu.Unlock()
	if done {
		return nil
	}
	for _, c := range Builtin {
		if _, err := pool.Exec(ctx, `INSERT INTO core.alarm_class (org_id, code, name, priority, color, ack_required, sound, builtin)
			VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, true) ON CONFLICT (org_id, code) DO NOTHING`, org, c.Code, c.Name, c.Priority, c.Color, c.AckRequired, c.Sound); err != nil {
			return err
		}
	}
	ensuredMu.Lock()
	ensured[org] = true
	ensuredMu.Unlock()
	return nil
}

// ShelvedUntil reports until when the alarms of a rule are shelved (nil: not shelved). One of the ids is empty.
func ShelvedUntil(ctx context.Context, pool *pgxpool.Pool, ruleID, automationRuleID string) *time.Time {
	var until time.Time
	err := pool.QueryRow(ctx, `SELECT until FROM core.alarm_shelf WHERE released_at IS NULL AND until > now()
		AND coalesce(rule_id, automation_rule_id) = $1::uuid`, ruleID+automationRuleID).Scan(&until)
	if err != nil {
		return nil
	}
	return &until
}

// Shelving limits.
const (
	MinShelf = 15 * time.Minute
	MaxShelf = 24 * time.Hour
)

// Errors of Shelve and Unshelve.
var (
	ErrNotFound       = errors.New("alarm not found")
	ErrAlreadyShelved = errors.New("the alarm is already shelved")
	ErrNotShelved     = errors.New("the alarm is not shelved")
	ErrDuration       = errors.New("an alarm is shelved for 15 minutes to 24 hours")
	ErrReason         = errors.New("reason required")
)

// Changed is an alarm whose state the shelf changed, for the live pages.
type Changed struct {
	OrgID, AlarmID, DatastreamID, State, Severity string
	Value                                         *float64
}

const changedCols = ` a.org_id::text, a.id::text, coalesce(a.datastream_id::text, ''), a.state, a.severity, a.value::float8 `

func scanChanged(rows pgx.Rows) ([]Changed, error) {
	defer rows.Close()
	var out []Changed
	for rows.Next() {
		var c Changed
		if err := rows.Scan(&c.OrgID, &c.AlarmID, &c.DatastreamID, &c.State, &c.Severity, &c.Value); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Shelve shelves the rule of an alarm: its alarms that have not returned to normal, and every new one until then,
// are recorded but not annunciated.
func Shelve(ctx context.Context, pool *pgxpool.Pool, org, alarmID, userID, userName, reason string, d time.Duration) (time.Time, []Changed, error) {
	reason = strings.TrimSpace(reason)
	if len(reason) < 3 {
		return time.Time{}, nil, ErrReason
	}
	if d < MinShelf || d > MaxShelf {
		return time.Time{}, nil, ErrDuration
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return time.Time{}, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var rule, auto *string
	if err := tx.QueryRow(ctx, `SELECT rule_id::text, automation_rule_id::text FROM core.alarm WHERE id = $1::uuid AND org_id = $2::uuid`, alarmID, org).Scan(&rule, &auto); err != nil {
		return time.Time{}, nil, ErrNotFound
	}
	// a shelf that ran out but was not released yet (the engine does it within seconds) must not block a new one
	if _, err := tx.Exec(ctx, `UPDATE core.alarm_shelf SET released_at = now(), released_by = 'system', release_reason = 'time ran out'
		WHERE released_at IS NULL AND until <= now() AND coalesce(rule_id, automation_rule_id) = coalesce($1::uuid, $2::uuid)`, rule, auto); err != nil {
		return time.Time{}, nil, err
	}
	var until time.Time
	err = tx.QueryRow(ctx, `INSERT INTO core.alarm_shelf (org_id, rule_id, automation_rule_id, until, by_id, by_name, reason)
		VALUES ($1::uuid, $2::uuid, $3::uuid, now() + make_interval(secs => $4), $5::uuid, $6, $7)
		ON CONFLICT DO NOTHING RETURNING until`, org, rule, auto, d.Seconds(), userID, userName, reason).Scan(&until)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil, ErrAlreadyShelved
	}
	if err != nil {
		return time.Time{}, nil, err
	}
	rows, err := tx.Query(ctx, `UPDATE core.alarm a SET state = 'suppressed', shelved_until = $3
		WHERE a.org_id = $4::uuid AND a.cleared_at IS NULL AND coalesce(a.rule_id, a.automation_rule_id) = coalesce($1::uuid, $2::uuid)
		RETURNING`+changedCols, rule, auto, until, org)
	if err != nil {
		return time.Time{}, nil, err
	}
	ch, err := scanChanged(rows)
	if err != nil {
		return time.Time{}, nil, err
	}
	for _, c := range ch {
		if _, err := tx.Exec(ctx, `INSERT INTO core.alarm_event (alarm_id, event, actor_id, actor_name, reason, detail)
			VALUES ($1::uuid, 'suppressed', $2::uuid, $3, $4, jsonb_build_object('until', $5::timestamptz))`, c.AlarmID, userID, userName, reason, until); err != nil {
			return time.Time{}, nil, err
		}
	}
	return until, ch, tx.Commit(ctx)
}

// release ends the open shelves matching where (args) and returns their alarms to the list.
func release(ctx context.Context, pool *pgxpool.Pool, where string, args []any, by, reason string) ([]Changed, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	n := len(args)
	rows, err := tx.Query(ctx, `UPDATE core.alarm_shelf s SET released_at = now(), released_by = $`+itoa(n+1)+`, release_reason = $`+itoa(n+2)+`
		WHERE s.released_at IS NULL AND `+where+` RETURNING coalesce(s.rule_id, s.automation_rule_id)::text`, append(args, by, reason)...)
	if err != nil {
		return nil, err
	}
	var rules []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		rules = append(rules, id)
	}
	rows.Close()
	if len(rules) == 0 {
		return nil, tx.Commit(ctx)
	}
	// alarms that returned to normal meanwhile only lose the mark; the others return to the list
	if _, err := tx.Exec(ctx, `UPDATE core.alarm SET shelved_until = NULL WHERE shelved_until IS NOT NULL AND cleared_at IS NOT NULL
		AND coalesce(rule_id, automation_rule_id) = ANY($1::uuid[])`, rules); err != nil {
		return nil, err
	}
	arows, err := tx.Query(ctx, `UPDATE core.alarm a SET state = CASE WHEN a.acked_at IS NULL THEN 'active' ELSE 'acknowledged' END, shelved_until = NULL
		WHERE a.cleared_at IS NULL AND a.state = 'suppressed' AND coalesce(a.rule_id, a.automation_rule_id) = ANY($1::uuid[]) RETURNING`+changedCols, rules)
	if err != nil {
		return nil, err
	}
	ch, err := scanChanged(arows)
	if err != nil {
		return nil, err
	}
	for _, c := range ch {
		if _, err := tx.Exec(ctx, `INSERT INTO core.alarm_event (alarm_id, event, actor_name, reason) VALUES ($1::uuid, 'unsuppressed', $2, $3)`, c.AlarmID, by, reason); err != nil {
			return nil, err
		}
	}
	return ch, tx.Commit(ctx)
}

// Unshelve ends the shelf of the rule of an alarm by hand.
func Unshelve(ctx context.Context, pool *pgxpool.Pool, org, alarmID, userName, reason string) ([]Changed, error) {
	reason = strings.TrimSpace(reason)
	if len(reason) < 3 {
		return nil, ErrReason
	}
	var rule string
	if err := pool.QueryRow(ctx, `SELECT coalesce(rule_id, automation_rule_id)::text FROM core.alarm WHERE id = $1::uuid AND org_id = $2::uuid`, alarmID, org).Scan(&rule); err != nil {
		return nil, ErrNotFound
	}
	var open bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM core.alarm_shelf WHERE released_at IS NULL AND coalesce(rule_id, automation_rule_id) = $1::uuid)`, rule).Scan(&open); err != nil {
		return nil, err
	}
	if !open {
		return nil, ErrNotShelved
	}
	return release(ctx, pool, `coalesce(s.rule_id, s.automation_rule_id) = $1::uuid`, []any{rule}, userName, reason)
}

// ReleaseExpired ends the shelves whose time ran out.
func ReleaseExpired(ctx context.Context, pool *pgxpool.Pool) ([]Changed, error) {
	return release(ctx, pool, `s.until <= now()`, nil, "system", "time ran out")
}

func itoa(n int) string { return string(rune('0' + n)) }

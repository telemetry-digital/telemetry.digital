// Package incident keeps the incident log (URS C-07, I-09): what went wrong operationally, its impact, cause and
// the corrective and preventive action (CAPA), with a small workflow. The system opens incidents itself for an
// undelivered alarm and for a broken audit chain; operators open the rest. Every change is appended as an event
// and recorded in the audit trail; administrators are told by e-mail (global recipients) and Web Push.
package incident

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/notify"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/push"
)

// Incident is one row of core.incident.
type Incident struct {
	ID         string         `json:"id"`
	OrgID      string         `json:"org_id"`
	Kind       string         `json:"kind"`
	Severity   string         `json:"severity"`
	Status     string         `json:"status"`
	Title      string         `json:"title"`
	Detail     map[string]any `json:"detail"`
	Impact     string         `json:"impact"`
	Cause      string         `json:"cause"`
	CAPA       string         `json:"capa"`
	DedupKey   string         `json:"-"`
	OpenedAt   time.Time      `json:"opened_at"`
	OpenedBy   string         `json:"opened_by"`
	UpdatedAt  time.Time      `json:"updated_at"`
	ResolvedAt *time.Time     `json:"resolved_at"`
	ClosedAt   *time.Time     `json:"closed_at"`
	ClosedBy   string         `json:"closed_by"`
	OrgName    string         `json:"org_name,omitempty"`
}

// Event is one append-only row of the incident's history.
type Event struct {
	ID     string         `json:"id"`
	At     time.Time      `json:"at"`
	Actor  string         `json:"actor"`
	Event  string         `json:"event"`
	Detail map[string]any `json:"detail"`
}

// Update carries the fields an operator may change; nil = untouched.
type Update struct {
	Status, Impact, Cause, CAPA, Note *string
}

var (
	Kinds      = []string{"alarm_undelivered", "audit_chain", "backup_failed", "manual"}
	Severities = []string{"minor", "major", "critical"}
	Statuses   = []string{"open", "investigating", "resolved", "closed"}
	// allowed transitions
	next = map[string][]string{
		"open":          {"investigating", "resolved", "closed"},
		"investigating": {"open", "resolved", "closed"},
		"resolved":      {"investigating", "closed"},
		"closed":        {"investigating"},
	}
)

// ErrInvalid marks a validation problem the API turns into 400.
var ErrInvalid = errors.New("invalid")

// Manager opens, lists and updates incidents.
type Manager struct {
	pool  *pgxpool.Pool
	log   *slog.Logger
	Email *notify.Notifier // global recipients (SMTP)
	Push  *push.Manager
}

func New(pool *pgxpool.Pool, log *slog.Logger) *Manager {
	return &Manager{pool: pool, log: log.With("component", "incident")}
}

func has(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Open creates the incident unless an open one with the same dedup key exists (then that one is returned and
// created is false). System-opened incidents notify administrators.
func (m *Manager) Open(ctx context.Context, in Incident) (*Incident, bool, error) {
	if m == nil {
		return nil, false, errors.New("incident manager not configured")
	}
	if !has(Kinds, in.Kind) || !has(Severities, in.Severity) || strings.TrimSpace(in.Title) == "" {
		return nil, false, fmt.Errorf("%w: kind, severity and title are required", ErrInvalid)
	}
	if in.OpenedBy == "" {
		in.OpenedBy = "system"
	}
	if in.DedupKey != "" {
		var id string
		err := m.pool.QueryRow(ctx, `SELECT id::text FROM core.incident WHERE dedup_key = $1 AND status IN ('open', 'investigating')`, in.DedupKey).Scan(&id)
		if err == nil {
			existing, _, gerr := m.Get(ctx, "", id, true)
			return existing, false, gerr
		}
	}
	detail, _ := json.Marshal(in.Detail)
	if in.Detail == nil {
		detail = []byte("{}")
	}
	var org *string
	if in.OrgID != "" {
		org = &in.OrgID
	}
	var dedup *string
	if in.DedupKey != "" {
		dedup = &in.DedupKey
	}
	err := m.pool.QueryRow(ctx, `INSERT INTO core.incident (org_id, kind, severity, title, detail, impact, cause, capa, dedup_key, opened_by)
		VALUES ($1::uuid, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10) RETURNING id::text, opened_at, updated_at`,
		org, in.Kind, in.Severity, strings.TrimSpace(in.Title), string(detail), in.Impact, in.Cause, in.CAPA, dedup, in.OpenedBy).
		Scan(&in.ID, &in.OpenedAt, &in.UpdatedAt)
	if err != nil {
		return nil, false, err
	}
	in.Status = "open"
	m.event(ctx, in.ID, in.OpenedBy, "opened", map[string]any{"kind": in.Kind, "severity": in.Severity})
	actorType := "user"
	if in.OpenedBy == "system" {
		actorType = "system"
	}
	_, _, _ = audit.Append(ctx, m.pool, audit.Entry{OrgID: in.OrgID, ActorType: actorType, ActorName: in.OpenedBy, Action: "incident.open", EntityType: "incident", EntityID: in.ID,
		New: map[string]any{"kind": in.Kind, "severity": in.Severity, "title": in.Title}, Reason: "incident opened"})
	m.log.Warn("incident opened", "security", true, "incident", in.ID, "kind", in.Kind, "severity", in.Severity, "title", in.Title, "org", in.OrgID)
	if in.Kind != "manual" {
		go m.notifyAdmins(context.WithoutCancel(ctx), in)
	}
	return &in, true, nil
}

// AlarmUndelivered opens an incident when every channel of an escalation step failed (URS C-07).
func (m *Manager) AlarmUndelivered(ctx context.Context, org, alarmID string, step int, channel, channelKind, errText string, alarm map[string]any) {
	if m == nil {
		return
	}
	key, _ := alarm["key"].(string)
	sev, _ := alarm["severity"].(string)
	severity := "major"
	if sev == "critical" {
		severity = "critical"
	}
	title := fmt.Sprintf("Alarm not delivered: %s (%s), step %d via %s", key, sev, step+1, channel)
	detail := map[string]any{"alarm_id": alarmID, "step": step, "channel": channel, "channel_kind": channelKind, "error": errText, "alarm": alarm}
	if _, _, err := m.Open(ctx, Incident{OrgID: org, Kind: "alarm_undelivered", Severity: severity, Title: title, Detail: detail,
		Impact: "The people on this escalation step were not told about an active alarm.", DedupKey: fmt.Sprintf("alarm:%s:step:%d", alarmID, step)}); err != nil {
		m.log.Error("could not open incident for undelivered alarm", "alarm", alarmID, "err", err)
	}
}

// AuditChainBroken opens an instance-level incident when the audit trail hash chain does not verify (URS I-02).
func (m *Manager) AuditChainBroken(ctx context.Context, firstBadSeq, checked int64) {
	if m == nil {
		return
	}
	title := fmt.Sprintf("Audit trail hash chain broken at seq %d", firstBadSeq)
	if _, _, err := m.Open(ctx, Incident{Kind: "audit_chain", Severity: "critical", Title: title,
		Detail: map[string]any{"first_bad_seq": firstBadSeq, "checked": checked},
		Impact: "Audit trail entries after this sequence number cannot be trusted until the cause is known.", DedupKey: fmt.Sprintf("audit_chain:%d", firstBadSeq)}); err != nil {
		m.log.Error("could not open incident for broken audit chain", "err", err)
	}
}

func (m *Manager) event(ctx context.Context, id, actor, event string, detail map[string]any) {
	d, _ := json.Marshal(detail)
	if detail == nil {
		d = []byte("{}")
	}
	if _, err := m.pool.Exec(ctx, `INSERT INTO core.incident_event (incident_id, actor, event, detail) VALUES ($1::uuid, $2, $3, $4::jsonb)`, id, actor, event, string(d)); err != nil {
		m.log.Error("incident event insert failed", "incident", id, "err", err)
	}
}

const cols = `i.id::text, COALESCE(i.org_id::text, ''), i.kind, i.severity, i.status, i.title, i.detail, i.impact, i.cause, i.capa, COALESCE(i.dedup_key, ''),
	i.opened_at, i.opened_by, i.updated_at, i.resolved_at, i.closed_at, COALESCE(i.closed_by, ''), COALESCE(o.name, '')`

func scan(row interface{ Scan(...any) error }) (*Incident, error) {
	var in Incident
	var detail []byte
	if err := row.Scan(&in.ID, &in.OrgID, &in.Kind, &in.Severity, &in.Status, &in.Title, &detail, &in.Impact, &in.Cause, &in.CAPA, &in.DedupKey,
		&in.OpenedAt, &in.OpenedBy, &in.UpdatedAt, &in.ResolvedAt, &in.ClosedAt, &in.ClosedBy, &in.OrgName); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(detail, &in.Detail)
	if in.Detail == nil {
		in.Detail = map[string]any{}
	}
	return &in, nil
}

// List returns incidents of one organization (org != "") or, for system administrators (org == ""), of every
// organization plus the instance-level ones. status: "open" (open + investigating), "all", or one status.
func (m *Manager) List(ctx context.Context, org, status string, limit int) ([]Incident, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := m.pool.Query(ctx, `SELECT `+cols+` FROM core.incident i LEFT JOIN core.organization o ON o.id = i.org_id
		WHERE ($1 = '' OR i.org_id = $1::uuid)
		  AND ($2 = 'all' OR ($2 = 'open' AND i.status IN ('open', 'investigating')) OR i.status = $2)
		ORDER BY (i.status IN ('open', 'investigating')) DESC, i.opened_at DESC LIMIT $3`, org, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Incident{}
	for rows.Next() {
		in, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *in)
	}
	return out, rows.Err()
}

// Get returns one incident with its events; org scoping as in List (instance-level rows only when instance is true).
func (m *Manager) Get(ctx context.Context, org, id string, instance bool) (*Incident, []Event, error) {
	in, err := scan(m.pool.QueryRow(ctx, `SELECT `+cols+` FROM core.incident i LEFT JOIN core.organization o ON o.id = i.org_id
		WHERE i.id = $1::uuid AND ($2 = '' OR i.org_id = $2::uuid) AND ($3 OR i.org_id IS NOT NULL)`, id, org, instance))
	if err != nil {
		return nil, nil, err
	}
	rows, err := m.pool.Query(ctx, `SELECT id::text, at, actor, event, detail FROM core.incident_event WHERE incident_id = $1::uuid ORDER BY at`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		var e Event
		var d []byte
		if err := rows.Scan(&e.ID, &e.At, &e.Actor, &e.Event, &d); err != nil {
			return nil, nil, err
		}
		_ = json.Unmarshal(d, &e.Detail)
		events = append(events, e)
	}
	return in, events, rows.Err()
}

// Apply changes fields and/or status. Closing needs a cause and a CAPA, resolving needs a cause. It returns what
// changed (for the audit trail) or ErrInvalid.
func (m *Manager) Apply(ctx context.Context, in *Incident, u Update, actor string) (map[string]any, error) {
	changed := map[string]any{}
	set := func(name string, dst *string, v *string) {
		if v != nil && strings.TrimSpace(*v) != *dst {
			*dst = strings.TrimSpace(*v)
			changed[name] = *dst
		}
	}
	set("impact", &in.Impact, u.Impact)
	set("cause", &in.Cause, u.Cause)
	set("capa", &in.CAPA, u.CAPA)
	for k, v := range changed {
		if len(v.(string)) > 4000 {
			return nil, fmt.Errorf("%w: %s is longer than 4000 characters", ErrInvalid, k)
		}
	}
	newStatus := in.Status
	if u.Status != nil && *u.Status != in.Status {
		if !has(next[in.Status], *u.Status) {
			return nil, fmt.Errorf("%w: status %s cannot change to %s", ErrInvalid, in.Status, *u.Status)
		}
		switch *u.Status {
		case "resolved":
			if in.Cause == "" {
				return nil, fmt.Errorf("%w: a cause is required to resolve an incident", ErrInvalid)
			}
		case "closed":
			if in.Cause == "" || in.CAPA == "" {
				return nil, fmt.Errorf("%w: a cause and a CAPA are required to close an incident", ErrInvalid)
			}
		}
		newStatus = *u.Status
	}
	if len(changed) == 0 && newStatus == in.Status && (u.Note == nil || strings.TrimSpace(*u.Note) == "") {
		return nil, fmt.Errorf("%w: nothing to change", ErrInvalid)
	}
	var resolvedAt, closedAt *time.Time
	resolvedAt, closedAt = in.ResolvedAt, in.ClosedAt
	closedBy := in.ClosedBy
	now := time.Now().UTC()
	if newStatus != in.Status {
		switch newStatus {
		case "resolved":
			resolvedAt = &now
		case "closed":
			closedAt, closedBy = &now, actor
			if resolvedAt == nil {
				resolvedAt = &now
			}
		case "open", "investigating":
			resolvedAt, closedAt, closedBy = nil, nil, ""
		}
	}
	if _, err := m.pool.Exec(ctx, `UPDATE core.incident SET status = $2, impact = $3, cause = $4, capa = $5, resolved_at = $6, closed_at = $7, closed_by = NULLIF($8, ''), updated_at = now() WHERE id = $1::uuid`,
		in.ID, newStatus, in.Impact, in.Cause, in.CAPA, resolvedAt, closedAt, closedBy); err != nil {
		return nil, err
	}
	if len(changed) > 0 {
		m.event(ctx, in.ID, actor, "fields", changed)
	}
	if newStatus != in.Status {
		m.event(ctx, in.ID, actor, "status", map[string]any{"from": in.Status, "to": newStatus})
		changed["status"] = newStatus
	}
	if u.Note != nil && strings.TrimSpace(*u.Note) != "" {
		m.event(ctx, in.ID, actor, "note", map[string]any{"text": strings.TrimSpace(*u.Note)})
		changed["note"] = strings.TrimSpace(*u.Note)
	}
	in.Status, in.ResolvedAt, in.ClosedAt, in.ClosedBy, in.UpdatedAt = newStatus, resolvedAt, closedAt, closedBy, now
	return changed, nil
}

// OpenCount returns open + investigating incidents (for /metrics and the navigation badge).
func (m *Manager) OpenCount(ctx context.Context, org string) int64 {
	var n int64
	_ = m.pool.QueryRow(ctx, `SELECT count(*) FROM core.incident WHERE status IN ('open', 'investigating') AND ($1 = '' OR org_id = $1::uuid)`, org).Scan(&n)
	return n
}

// notifyAdmins tells the administrators: e-mail to the server's global recipients and Web Push to users holding
// alarm.ack or system.admin (of the incident's organization, or of every organization for instance-level ones).
func (m *Manager) notifyAdmins(ctx context.Context, in Incident) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	subject := fmt.Sprintf("[ctrl32] incident (%s): %s", in.Severity, in.Title)
	detail, _ := json.MarshalIndent(in.Detail, "", "  ")
	text := fmt.Sprintf("Incident %s\nKind: %s\nSeverity: %s\nOpened: %s\nImpact: %s\n\n%s\n\nOpen the Incidents page to record the cause and the CAPA.\n",
		in.ID, in.Kind, in.Severity, in.OpenedAt.Format(time.RFC3339), in.Impact, detail)
	sent := map[string]any{}
	if m.Email != nil && m.Email.Configured() {
		if ok, err := m.Email.Send(ctx, subject, text); err != nil {
			sent["email_error"] = err.Error()
		} else if ok {
			sent["email"] = true
		}
	}
	if m.Push != nil {
		rows, err := m.pool.Query(ctx, `WITH cur AS (
			SELECT DISTINCT ON (user_id, role_id, scope_site_id) user_id, role_id, event FROM core.user_role_history ORDER BY user_id, role_id, scope_site_id, ts DESC)
			SELECT DISTINCT u.org_id::text, u.username FROM cur JOIN core.role r ON r.id = cur.role_id JOIN core.user_account u ON u.id = cur.user_id
			WHERE cur.event = 'granted' AND u.disabled_at IS NULL AND (r.is_admin OR 'alarm.ack' = ANY(r.permissions) OR 'system.admin' = ANY(r.permissions))
			  AND ($1 = '' OR u.org_id = $1::uuid)`, in.OrgID)
		if err == nil {
			byOrg := map[string][]string{}
			for rows.Next() {
				var org, name string
				if rows.Scan(&org, &name) == nil {
					byOrg[org] = append(byOrg[org], name)
				}
			}
			rows.Close()
			total := 0
			for org, names := range byOrg {
				n, _, _ := m.Push.Send(ctx, org, names, notify.Message{Event: "incident", Subject: subject, Text: text, SentAt: time.Now().UTC()})
				total += n
			}
			sent["push"] = total
		}
	}
	m.event(ctx, in.ID, "system", "notified", sent)
}

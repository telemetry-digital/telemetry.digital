// Package console implements the remote device console (protocol §3.5): a session opened by an operator, lines
// sent to d/{id}/console/in, output received on d/{id}/console/out, everything stored as append-only records.
// The device only listens while the shared attribute console.enabled is true; the server sets it on open and
// clears it on close or after 15 minutes without activity. The console is a channel into the firmware, never a
// shell into the server.
package console

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
)

const (
	IdleTimeout = 15 * time.Minute
	MaxLine     = 512
	ratePerSec  = 5
)

var (
	ErrNoSession = errors.New("no open console session")
	ErrBusy      = errors.New("a console session is already open on this device")
	ErrRate      = errors.New("rate limit: at most 5 lines per second")
)

// Session is an open or closed console session.
type Session struct {
	ID           string     `json:"id"`
	DeviceID     string     `json:"device_id"`
	OpenedBy     string     `json:"opened_by"`
	OpenedAt     time.Time  `json:"opened_at"`
	LastActivity time.Time  `json:"last_activity_at"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
	CloseReason  *string    `json:"close_reason,omitempty"`
}

// Line is one console record.
type Line struct {
	ID        string     `json:"id"`
	TS        time.Time  `json:"ts"`
	DeviceTS  *time.Time `json:"device_ts,omitempty"`
	Direction string     `json:"direction"`
	Line      string     `json:"line"`
	Actor     string     `json:"actor"`
}

// Manager owns sessions.
type Manager struct {
	pool *pgxpool.Pool
	br   *broker.Broker
	log  *slog.Logger
	mu   sync.Mutex
	rate map[string][]time.Time // session id -> recent send times
}

// New creates the manager.
func New(pool *pgxpool.Pool, br *broker.Broker, log *slog.Logger) *Manager {
	return &Manager{pool: pool, br: br, log: log.With("component", "console"), rate: map[string][]time.Time{}}
}

// Open starts a session (one open session per device). Reopening by the same user returns the existing session.
func (m *Manager) Open(ctx context.Context, orgID, deviceID, ext, userID, username string) (*Session, bool, error) {
	if s, err := m.openSession(ctx, deviceID); err == nil {
		if s.OpenedBy == username {
			return s, false, nil
		}
		return nil, false, ErrBusy
	}
	var s Session
	err := m.pool.QueryRow(ctx, `INSERT INTO core.console_session (org_id, device_id, opened_by, opened_by_name) VALUES ($1::uuid, $2::uuid, NULLIF($3,'')::uuid, $4)
		RETURNING id::text, device_id::text, opened_by_name, opened_at, last_activity_at`, orgID, deviceID, userID, username).
		Scan(&s.ID, &s.DeviceID, &s.OpenedBy, &s.OpenedAt, &s.LastActivity)
	if err != nil {
		return nil, false, err
	}
	if err := m.setEnabled(ctx, deviceID, ext, true); err != nil {
		m.log.Warn("console.enabled publish failed", "device", ext, "err", err)
	}
	return &s, true, nil
}

func (m *Manager) openSession(ctx context.Context, deviceID string) (*Session, error) {
	var s Session
	err := m.pool.QueryRow(ctx, `SELECT id::text, device_id::text, opened_by_name, opened_at, last_activity_at FROM core.console_session
		WHERE device_id = $1::uuid AND closed_at IS NULL ORDER BY opened_at DESC LIMIT 1`, deviceID).Scan(&s.ID, &s.DeviceID, &s.OpenedBy, &s.OpenedAt, &s.LastActivity)
	if err != nil {
		return nil, ErrNoSession
	}
	return &s, nil
}

// Get returns a session by id within the organization.
func (m *Manager) Get(ctx context.Context, orgID, sessionID string) (*Session, error) {
	var s Session
	err := m.pool.QueryRow(ctx, `SELECT id::text, device_id::text, opened_by_name, opened_at, last_activity_at, closed_at, close_reason FROM core.console_session
		WHERE id = $1::uuid AND org_id = $2::uuid`, sessionID, orgID).Scan(&s.ID, &s.DeviceID, &s.OpenedBy, &s.OpenedAt, &s.LastActivity, &s.ClosedAt, &s.CloseReason)
	if err != nil {
		return nil, ErrNoSession
	}
	return &s, nil
}

// Close ends a session and disables the console on the device.
func (m *Manager) Close(ctx context.Context, sessionID, ext, reason string) error {
	var deviceID string
	err := m.pool.QueryRow(ctx, `UPDATE core.console_session SET closed_at = now(), close_reason = $2 WHERE id = $1::uuid AND closed_at IS NULL RETURNING device_id::text`, sessionID, reason).Scan(&deviceID)
	if err != nil {
		return ErrNoSession
	}
	m.mu.Lock()
	delete(m.rate, sessionID)
	m.mu.Unlock()
	return m.setEnabled(ctx, deviceID, ext, false)
}

// Send publishes one line to the device and records it.
func (m *Manager) Send(ctx context.Context, s *Session, ext, line, actor string) (*Line, error) {
	if s.ClosedAt != nil {
		return nil, ErrNoSession
	}
	line = sanitize(line)
	if line == "" || len(line) > MaxLine {
		return nil, errors.New("line must be 1–512 bytes without control characters")
	}
	if !m.allow(s.ID) {
		return nil, ErrRate
	}
	msg, _ := json.Marshal(map[string]any{"s": s.ID, "l": line})
	if err := m.br.Publish("d/"+ext+"/console/in", msg, false); err != nil {
		return nil, err
	}
	var l Line
	err := m.pool.QueryRow(ctx, `INSERT INTO core.console_log (session_id, org_id, device_id, direction, line, actor_name)
		SELECT id, org_id, device_id, 'in', $2, $3 FROM core.console_session WHERE id = $1::uuid RETURNING id::text, ts, direction, line, actor_name`, s.ID, line, actor).
		Scan(&l.ID, &l.TS, &l.Direction, &l.Line, &l.Actor)
	if err != nil {
		return nil, err
	}
	_, _ = m.pool.Exec(ctx, `UPDATE core.console_session SET last_activity_at = now() WHERE id = $1::uuid`, s.ID)
	return &l, nil
}

// HandleOut stores a line published by the device on console/out: {"s": session, "t": unix, "l": line}.
func (m *Manager) HandleOut(ctx context.Context, dev *broker.Device, payload []byte) {
	var out struct {
		S string `json:"s"`
		T int64  `json:"t"`
		L string `json:"l"`
	}
	if err := json.Unmarshal(payload, &out); err != nil || out.S == "" {
		return
	}
	line := sanitize(out.L)
	if len(line) > MaxLine {
		line = line[:MaxLine]
	}
	var devTS *time.Time
	if out.T > 0 {
		t := time.Unix(out.T, 0).UTC()
		devTS = &t
	}
	tag, err := m.pool.Exec(ctx, `INSERT INTO core.console_log (session_id, org_id, device_id, device_ts, direction, line, actor_name)
		SELECT id, org_id, device_id, $3, 'out', $4, $5 FROM core.console_session WHERE id = $1::uuid AND device_id = $2::uuid AND closed_at IS NULL`,
		out.S, dev.ID, devTS, line, dev.ExternalID)
	if err != nil || tag.RowsAffected() == 0 {
		m.log.Warn("console output without open session dropped", "security", true, "device", dev.ExternalID, "session", out.S)
		return
	}
	_, _ = m.pool.Exec(ctx, `UPDATE core.console_session SET last_activity_at = now() WHERE id = $1::uuid`, out.S)
}

// Lines returns records of a session after the given id (uuidv7 order), oldest first.
func (m *Manager) Lines(ctx context.Context, sessionID, after string, limit int) ([]Line, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := m.pool.Query(ctx, `SELECT id::text, ts, device_ts, direction, line, actor_name FROM core.console_log WHERE session_id = $1::uuid AND ($2 = '' OR id > $2::uuid) ORDER BY id LIMIT $3`, sessionID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Line{}
	for rows.Next() {
		var l Line
		if err := rows.Scan(&l.ID, &l.TS, &l.DeviceTS, &l.Direction, &l.Line, &l.Actor); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Run closes idle sessions every minute.
func (m *Manager) Run(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rows, err := m.pool.Query(ctx, `SELECT s.id::text, d.external_id FROM core.console_session s JOIN core.device d ON d.id = s.device_id WHERE s.closed_at IS NULL AND s.last_activity_at < now() - $1::interval`, IdleTimeout.String())
			if err != nil {
				continue
			}
			type row struct{ id, ext string }
			var idle []row
			for rows.Next() {
				var r row
				if rows.Scan(&r.id, &r.ext) == nil {
					idle = append(idle, r)
				}
			}
			rows.Close()
			for _, r := range idle {
				if err := m.Close(ctx, r.id, r.ext, "idle timeout"); err == nil {
					m.log.Info("console session closed after idle timeout", "session", r.id, "device", r.ext)
				}
			}
		}
	}
}

func (m *Manager) setEnabled(ctx context.Context, deviceID, ext string, enabled bool) error {
	var raw []byte
	if err := m.pool.QueryRow(ctx, `UPDATE core.device SET shared_attributes = shared_attributes || jsonb_build_object('console.enabled', $2::boolean) WHERE id = $1::uuid RETURNING shared_attributes`, deviceID, enabled).Scan(&raw); err != nil {
		return err
	}
	return m.br.Publish("d/"+ext+"/attributes/shared", raw, true)
}

func (m *Manager) allow(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	recent := m.rate[sessionID][:0]
	for _, t := range m.rate[sessionID] {
		if now.Sub(t) < time.Second {
			recent = append(recent, t)
		}
	}
	if len(recent) >= ratePerSec {
		m.rate[sessionID] = recent
		return false
	}
	m.rate[sessionID] = append(recent, now)
	return true
}

// sanitize strips control characters except tab; output is shown in the UI as text only.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimRight(s, "\r\n"))
}

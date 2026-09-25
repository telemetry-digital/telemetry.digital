package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/console"
)

// Remote device console (protocol §3.5, URS D-*). Permission device.console; every line is a record.
//
//	GET    /api/v1/devices/{id}/console                    open session (if any) and recent sessions
//	POST   /api/v1/devices/{id}/console                    open (or resume own) session
//	DELETE /api/v1/devices/{id}/console/{sid}?reason=      close
//	POST   /api/v1/devices/{id}/console/{sid}/in           {line}
//	GET    /api/v1/devices/{id}/console/{sid}?after=<id>   lines after id (poll every second)

func (s *Server) consoleDevice(w http.ResponseWriter, r *http.Request, org string) (id, ext string, ok bool) {
	id = r.PathValue("id")
	if err := s.pool.QueryRow(r.Context(), `SELECT external_id FROM core.device WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, id, org).Scan(&ext); err != nil {
		writeErr(w, 404, "device not found")
		return "", "", false
	}
	return id, ext, true
}

func (s *Server) consoleStatus(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	rows, err := s.pool.Query(r.Context(), `SELECT id::text, opened_by_name, opened_at, last_activity_at, closed_at, close_reason,
		(SELECT count(*) FROM core.console_log l WHERE l.session_id = s.id) FROM core.console_session s WHERE device_id = $1::uuid AND org_id = $2::uuid ORDER BY opened_at DESC LIMIT 10`, id, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	var open map[string]any
	list := []map[string]any{}
	for rows.Next() {
		var sid, by string
		var opened, last time.Time
		var closed *time.Time
		var reason *string
		var n int
		if err := rows.Scan(&sid, &by, &opened, &last, &closed, &reason, &n); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		m := map[string]any{"id": sid, "opened_by": by, "opened_at": opened, "last_activity_at": last, "closed_at": closed, "close_reason": reason, "lines": n}
		if closed == nil && open == nil {
			open = m
		}
		list = append(list, m)
	}
	writeJSON(w, 200, map[string]any{"open": open, "sessions": list, "idle_timeout_minutes": int(console.IdleTimeout.Minutes())})
}

func (s *Server) consoleOpen(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	id, ext, ok := s.consoleDevice(w, r, org)
	if !ok {
		return
	}
	sess, created, err := s.con.Open(r.Context(), org, id, ext, p.UserID, p.Username)
	if err != nil {
		if errors.Is(err, console.ErrBusy) {
			writeErr(w, 409, err.Error())
			return
		}
		writeErr(w, 500, "could not open session")
		return
	}
	if created {
		s.audit(r, "console.open", "device", id, nil, map[string]any{"session_id": sess.ID, "external_id": ext}, "")
	}
	writeJSON(w, 201, sess)
}

func (s *Server) consoleClose(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id, ext, ok := s.consoleDevice(w, r, org)
	if !ok {
		return
	}
	sid := r.PathValue("sid")
	sess, err := s.con.Get(r.Context(), org, sid)
	if err != nil || sess.DeviceID != id {
		writeErr(w, 404, "session not found")
		return
	}
	reason := r.URL.Query().Get("reason")
	if reason == "" {
		reason = "closed by operator"
	}
	if err := s.con.Close(r.Context(), sid, ext, reason); err != nil {
		writeErr(w, 409, "session already closed")
		return
	}
	s.audit(r, "console.close", "device", id, nil, map[string]any{"session_id": sid}, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) consoleSend(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	id, ext, ok := s.consoleDevice(w, r, org)
	if !ok {
		return
	}
	sess, err := s.con.Get(r.Context(), org, r.PathValue("sid"))
	if err != nil || sess.DeviceID != id {
		writeErr(w, 404, "session not found")
		return
	}
	var in struct{ Line string }
	if err := decodeJSON(r, &in, 2048); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	l, err := s.con.Send(r.Context(), sess, ext, in.Line, p.Username)
	if err != nil {
		switch {
		case errors.Is(err, console.ErrRate):
			writeErr(w, 429, err.Error())
		case errors.Is(err, console.ErrNoSession):
			writeErr(w, 409, "session closed")
		default:
			writeErr(w, 400, err.Error())
		}
		return
	}
	writeJSON(w, 200, l)
}

func (s *Server) consoleLines(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	sess, err := s.con.Get(r.Context(), org, r.PathValue("sid"))
	if err != nil || sess.DeviceID != r.PathValue("id") {
		writeErr(w, 404, "session not found")
		return
	}
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	lines, err := s.con.Lines(r.Context(), sess.ID, r.URL.Query().Get("after"), limit)
	if err != nil {
		writeErr(w, 400, "invalid cursor")
		return
	}
	writeJSON(w, 200, map[string]any{"session": sess, "lines": lines})
}

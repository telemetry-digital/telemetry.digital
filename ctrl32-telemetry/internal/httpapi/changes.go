package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

// Four eyes for configuration changes (URS H-08): with the organization policy four_eyes_limits (alarm rules) or
// four_eyes_assignments (device ↔ datastream assignments) the wrapped call is not executed; it is stored as a
// change request (method, path, body, reason) and answered with 202. A different user who holds the permission
// of the call approves it, and the server replays the original request under the approver's identity, so the
// change is executed exactly as requested and audited with both names.
//
//	GET  /api/v1/change-requests?status=pending|all       (config.write or device.manage)
//	POST /api/v1/change-requests/{id}/approve {reason}    (permission of the underlying change, not the requester)
//	POST /api/v1/change-requests/{id}/reject  {reason}

type changeCtxKey struct{}

type changeInfo struct {
	ID, RequestedBy string
}

func changeFrom(ctx context.Context) *changeInfo {
	ci, _ := ctx.Value(changeCtxKey{}).(*changeInfo)
	return ci
}

// fourEyes wraps a handler: when the policy for kind is on, the call becomes a pending change request.
func (s *Server) fourEyes(kind string, fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if changeFrom(r.Context()) != nil {
			fn(w, r) // approved replay
			return
		}
		p := PrincipalFrom(r.Context())
		if p == nil || p.OrgID == "" || s.iam == nil {
			fn(w, r)
			return
		}
		pol := s.iam.Policy(r.Context(), p.OrgID)
		on := (kind == "limits" && pol.FourEyesLimits) || (kind == "assignments" && pol.FourEyesAssignments) ||
			(kind == "flows" && pol.FourEyesCommands && s.flowNeedsFourEyes(r, p.OrgID))
		if !on {
			fn(w, r)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
		if err != nil {
			writeErr(w, 400, "body too large")
			return
		}
		body = bytes.TrimSpace(body)
		reason := strings.TrimSpace(r.URL.Query().Get("reason"))
		if len(body) > 0 {
			if !json.Valid(body) {
				writeErr(w, 400, "invalid JSON body")
				return
			}
			var probe struct {
				Reason string `json:"reason"`
			}
			_ = json.Unmarshal(body, &probe)
			if probe.Reason != "" {
				reason = strings.TrimSpace(probe.Reason)
			}
		} else {
			body = []byte("{}")
		}
		if !reasonOf(w, reason) {
			return
		}
		summary := s.changeSummary(r.Context(), kind, r.Method, r.URL.Path, body)
		var id string
		var expires time.Time
		if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.change_request (org_id, kind, method, path, body, summary, reason, requested_by, requested_by_name)
			VALUES ($1::uuid, $2, $3, $4, $5::jsonb, $6, $7, NULLIF($8,'')::uuid, $9) RETURNING id::text, expires_at`,
			p.OrgID, kind, r.Method, r.URL.RequestURI(), string(body), summary, reason, p.UserID, p.Username).Scan(&id, &expires); err != nil {
			s.log.Error("change request insert", "err", err)
			writeErr(w, 500, "change request could not be recorded")
			return
		}
		s.audit(r, "change_request.create", "change_request", id, nil, map[string]any{"kind": kind, "method": r.Method, "path": r.URL.Path, "summary": summary}, reason)
		writeJSON(w, 202, map[string]any{"status": "pending", "change_request": map[string]any{"id": id, "kind": kind, "summary": summary, "expires_at": expires}})
	}
}

// changeSummary describes the change in words for the approver.
func (s *Server) changeSummary(ctx context.Context, kind, method, path string, body []byte) string {
	var in map[string]any
	_ = json.Unmarshal(body, &in)
	str := func(k string) string {
		v, _ := in[k].(string)
		return v
	}
	dsLabel := func(id string) string {
		var asset, key string
		if err := s.pool.QueryRow(ctx, `SELECT a.name, d.key FROM core.datastream d JOIN core.asset a ON a.id = d.asset_id WHERE d.id = $1::uuid`, id).Scan(&asset, &key); err == nil {
			return asset + " / " + key
		}
		return id
	}
	devLabel := func(id string) string {
		var ext, name string
		if err := s.pool.QueryRow(ctx, `SELECT external_id, name FROM core.device WHERE id = $1::uuid`, id).Scan(&ext, &name); err == nil {
			return name + " (" + ext + ")"
		}
		return id
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch {
	case kind == "limits" && method == http.MethodPost:
		lim := ""
		if v, ok := in["warning"].(float64); ok {
			lim += " warning " + jsonNum(v)
		}
		if v, ok := in["action"].(float64); ok {
			lim += " action " + jsonNum(v)
		}
		return "alarm rule " + str("type") + " on " + dsLabel(str("datastream_id")) + ":" + lim
	case kind == "limits" && method == http.MethodDelete && len(parts) >= 4:
		var typ, ds string
		if err := s.pool.QueryRow(ctx, `SELECT type, datastream_id::text FROM core.alarm_rule WHERE id = $1::uuid`, parts[3]).Scan(&typ, &ds); err == nil {
			return "disable alarm rule " + typ + " on " + dsLabel(ds)
		}
		return "disable alarm rule " + parts[3]
	case kind == "assignments" && method == http.MethodPost && len(parts) >= 4:
		return "assign " + dsLabel(str("datastream_id")) + " to device " + devLabel(parts[3])
	case kind == "assignments" && method == http.MethodDelete && len(parts) >= 6:
		return "remove " + dsLabel(parts[5]) + " from device " + devLabel(parts[3])
	case kind == "flows" && len(parts) >= 4:
		var name string
		var version int
		p := PrincipalFrom(ctx)
		if p != nil {
			_ = s.pool.QueryRow(ctx, `SELECT name, version FROM core.flow WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL`, p.OrgID, parts[3]).Scan(&name, &version)
		}
		return fmt.Sprintf("deploy flow %s version %d (it sends device commands)", name, version)
	}
	return method + " " + path
}

func jsonNum(v float64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (s *Server) canApprove(p *iam.Principal, kind string) bool {
	switch kind {
	case "limits":
		return p.Has("config.write")
	case "assignments":
		return p.Has("device.manage")
	case "flows":
		return p.Has("content.write") && p.Has("device.command")
	}
	return false
}

func (s *Server) listChangeRequests(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	if !p.Has("config.write") && !p.Has("device.manage") {
		writeErr(w, 403, "config.write or device.manage permission required")
		return
	}
	// lazy expiry
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.change_request SET status = 'expired', decided_at = now() WHERE org_id = $1::uuid AND status = 'pending' AND expires_at < now()`, org)
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}
	rows, err := s.pool.Query(r.Context(), `SELECT id::text, kind, method, path, body, summary, reason, requested_by_name, requested_at, expires_at, status, decided_by_name, decided_at, decision_reason, result
		FROM core.change_request WHERE org_id = $1::uuid AND ($2 = 'all' OR status = $2) ORDER BY requested_at DESC LIMIT 200`, org, status)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, kind, method, path, summary, reason, by, st string
		var body, result json.RawMessage
		var requested, expires time.Time
		var decidedBy, decisionReason *string
		var decided *time.Time
		if rows.Scan(&id, &kind, &method, &path, &body, &summary, &reason, &by, &requested, &expires, &st, &decidedBy, &decided, &decisionReason, &result) != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "kind": kind, "method": method, "path": path, "body": body, "summary": summary, "reason": reason, "requested_by": by, "requested_at": requested, "expires_at": expires,
			"status": st, "decided_by": decidedBy, "decided_at": decided, "decision_reason": decisionReason, "result": result, "can_approve": st == "pending" && s.canApprove(p, kind) && by != p.Username})
	}
	writeJSON(w, 200, out)
}

func (s *Server) decideChange(w http.ResponseWriter, r *http.Request, approve bool) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var in struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var kind, method, path, summary, status, byID, byName string
	var body []byte
	var expires time.Time
	if err := s.pool.QueryRow(r.Context(), `SELECT kind, method, path, body, summary, status, COALESCE(requested_by::text, ''), requested_by_name, expires_at FROM core.change_request WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).
		Scan(&kind, &method, &path, &body, &summary, &status, &byID, &byName, &expires); err != nil {
		writeErr(w, 404, "change request not found")
		return
	}
	if status != "pending" {
		writeErr(w, 409, "change request is not pending (status "+status+")")
		return
	}
	if expires.Before(time.Now()) {
		_, _ = s.pool.Exec(r.Context(), `UPDATE core.change_request SET status = 'expired', decided_at = now() WHERE id = $1::uuid AND status = 'pending'`, id)
		writeErr(w, 409, "change request expired")
		return
	}
	if !s.canApprove(p, kind) {
		writeErr(w, 403, "permission for this change is required")
		return
	}
	if !approve {
		_, _ = s.pool.Exec(r.Context(), `UPDATE core.change_request SET status = 'rejected', decided_by = NULLIF($2,'')::uuid, decided_by_name = $3, decided_at = now(), decision_reason = $4 WHERE id = $1::uuid`, id, p.UserID, p.Username, in.Reason)
		s.audit(r, "change_request.reject", "change_request", id, map[string]any{"status": "pending", "summary": summary, "requested_by": byName}, map[string]any{"status": "rejected", "rejected_by": p.Username}, in.Reason)
		writeJSON(w, 200, map[string]any{"id": id, "status": "rejected"})
		return
	}
	if (byID != "" && byID == p.UserID) || (byID == "" && byName == p.Username) {
		writeErr(w, 403, "four eyes: a different user must approve this change")
		return
	}
	// Replay the original call under the approver's identity, marked as an approved change.
	ctx := context.WithValue(r.Context(), changeCtxKey{}, &changeInfo{ID: id, RequestedBy: byName})
	var reqBody io.Reader
	if method != http.MethodGet && method != http.MethodDelete && len(body) > 0 {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, path, reqBody)
	if err != nil {
		writeErr(w, 500, "replay failed")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", p.CSRFSecret) // the approver's own session passes the CSRF check
	req.RemoteAddr = r.RemoteAddr
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	var result any
	if rec.Body.Len() > 0 && json.Valid(rec.Body.Bytes()) {
		result = json.RawMessage(rec.Body.Bytes())
	} else {
		result = rec.Body.String()
	}
	next := "approved"
	if rec.Code < 200 || rec.Code >= 300 {
		next = "failed"
	}
	res, _ := json.Marshal(map[string]any{"code": rec.Code, "body": result})
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.change_request SET status = $2, decided_by = NULLIF($3,'')::uuid, decided_by_name = $4, decided_at = now(), decision_reason = $5, result = $6::jsonb WHERE id = $1::uuid`, id, next, p.UserID, p.Username, in.Reason, string(res))
	s.audit(r, "change_request.approve", "change_request", id, map[string]any{"status": "pending", "summary": summary, "requested_by": byName}, map[string]any{"status": next, "approved_by": p.Username, "code": rec.Code}, in.Reason)
	writeJSON(w, 200, map[string]any{"id": id, "status": next, "code": rec.Code, "result": result})
}

func (s *Server) approveChange(w http.ResponseWriter, r *http.Request) { s.decideChange(w, r, true) }
func (s *Server) rejectChange(w http.ResponseWriter, r *http.Request)  { s.decideChange(w, r, false) }

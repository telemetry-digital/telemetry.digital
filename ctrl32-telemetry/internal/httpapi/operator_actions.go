package httpapi

// Operator action log (spec 039, 0.93.0, ADR-0026). Off by default; an organization turns it on with the policy
// log_operator_actions. Then every action of a person at the controls is kept in core.operator_action: commands (each
// press of a control widget or of an element of a process picture), recipe loads, PTZ moves, display commands, alarm
// acknowledgements and shelving, video playback and export, sign-in and sign-out, and — reported by the pages — the
// pictures and faceplates the person opened and the presses that were cancelled before anything was sent.
//
// Actions that already write the audit trail are taken from there (one place, nothing is forgotten when a new
// handler appears); the rest is logged where it happens.
//
//	GET  /api/v1/operator-actions?from=&to=&user=&kind=&q=&limit=&before=   newest first      (audit.read)
//	POST /api/v1/operator-actions {kind, target, detail}                    reported by pages (signed-in user)
//	GET  /api/v1/export/operator-actions?from=&to=&format=                  CSV / JSON Lines  (audit.read)

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// opAuditKinds maps the audit actions that are operator actions to the kind shown in the log.
var opAuditKinds = map[string]string{
	"device.command": "command", "command.approve": "command_approve", "command.reject": "command_reject",
	"device.attributes": "attributes",
	"alarm.acknowledge": "alarm_ack", "alarm.suppress": "alarm_shelve", "alarm.unsuppress": "alarm_unshelve",
	"camera.mark": "camera_mark", "video.playback": "video_playback", "video.export": "video_export",
	"video.hold": "video_hold", "video.hold_release": "video_hold_release", "video.evidence_export": "video_evidence", "video.unmask": "video_unmask",
	"display.command": "display", "kiosk.command": "kiosk",
	"flow.inject":     "flow_inject",
	"recipe.download": "recipe_load", "recipe.download_approve": "recipe_approve", "recipe.download_reject": "recipe_reject",
	"counting_occupancy.update": "counting_correction",
}

// opClientKinds are the kinds a page may report itself: they change nothing, so the server cannot see them.
var opClientKinds = map[string]bool{"navigate": true, "popup": true, "press": true, "cancel": true}

// OpKinds lists every kind of the log (for the filter of the page and the API reference).
func opKinds() []string {
	seen := map[string]bool{"ptz": true, "scene": true, "login": true, "logout": true}
	for _, k := range opAuditKinds {
		seen[k] = true
	}
	for k := range opClientKinds {
		seen[k] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

func sortStrings(a []string) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

type opLogState struct {
	on bool
	at time.Time
}

var opLogCache sync.Map // org id → opLogState; the policy is read at most every few seconds

// opLogOn reports whether the organization keeps the operator action log.
func (s *Server) opLogOn(ctx context.Context, org string) bool {
	if org == "" || s.iam == nil {
		return false
	}
	if v, ok := opLogCache.Load(org); ok && time.Since(v.(opLogState).at) < 5*time.Second {
		return v.(opLogState).on
	}
	on := s.iam.Policy(ctx, org).LogOperatorActions
	opLogCache.Store(org, opLogState{on: on, at: time.Now()})
	return on
}

// opScreen is the page an action came from: the pages send their path in X-Ctrl32-Screen.
func opScreen(r *http.Request) string {
	v := r.Header.Get("X-Ctrl32-Screen")
	if v == "" {
		return ""
	}
	if u, err := url.PathUnescape(v); err == nil {
		v = u
	}
	if !strings.HasPrefix(v, "/") || len(v) > 300 {
		return ""
	}
	return v
}

// opLog records one operator action of the signed-in user, when the organization keeps the log.
func (s *Server) opLog(r *http.Request, kind, targetType, targetID, target string, detail any, reason string) {
	p := PrincipalFrom(r.Context())
	if p == nil || p.OrgID == "" || !s.opLogOn(r.Context(), p.OrgID) {
		return
	}
	actor, name := "user", p.Username
	switch {
	case p.GrantID != "":
		name = p.Username + " (AI assistant " + p.ClientName + " via MCP)"
	case p.TokenID != "":
		actor, name = "api_token", p.Username+" (token "+p.TokenID[:8]+")"
	}
	s.opInsert(r.Context(), p.OrgID, p.UserID, name, actor, kind, targetType, targetID, target, opScreen(r), detail, reason, clientIP(r))
}

func (s *Server) opInsert(ctx context.Context, org, userID, userName, actor, kind, targetType, targetID, target, screen string, detail any, reason, station string) {
	dj := []byte(`{}`)
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil && len(b) <= 16<<10 && len(b) > 0 && b[0] == '{' {
			dj = b
		}
	}
	if !uuidRe.MatchString(strings.ToLower(targetID)) {
		targetID = ""
	}
	if !uuidRe.MatchString(strings.ToLower(userID)) {
		userID = ""
	}
	if len(target) > 200 {
		target = target[:200]
	}
	if len(reason) > 500 {
		reason = reason[:500]
	}
	// the log must never break the action it describes: a failed insert is reported, not returned
	if _, err := s.pool.Exec(context.WithoutCancel(ctx), `INSERT INTO core.operator_action (org_id, user_id, user_name, actor_type, kind, target_type, target_id, target, screen, detail, reason, station)
		VALUES ($1::uuid, NULLIF($2,'')::uuid, $3, $4, $5, $6, NULLIF($7,'')::uuid, $8, $9, $10::jsonb, $11, $12)`,
		org, userID, userName, actor, kind, targetType, targetID, target, screen, string(dj), reason, station); err != nil {
		s.log.Error("operator action not logged", "security", true, "kind", kind, "err", err)
	}
}

// opSession logs a sign-in or a sign-out of the user the session token belongs to.
func (s *Server) opSession(r *http.Request, token, kind string) {
	if s.iam == nil || token == "" {
		return
	}
	p, err := s.iam.Authenticate(r.Context(), token)
	if err != nil || p == nil || p.OrgID == "" || !s.opLogOn(r.Context(), p.OrgID) {
		return
	}
	s.opInsert(r.Context(), p.OrgID, p.UserID, p.Username, "user", kind, "user", p.UserID, p.Username, opScreen(r), nil, "", clientIP(r))
}

// opFromAudit copies an audit entry that is an operator action into the log.
func (s *Server) opFromAudit(r *http.Request, action, entityType, entityID string, detail any, reason string) {
	kind, ok := opAuditKinds[action]
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	if p == nil || p.OrgID == "" || !s.opLogOn(r.Context(), p.OrgID) {
		return
	}
	s.opLog(r, kind, entityType, entityID, s.opTargetName(r.Context(), p.OrgID, entityType, entityID, detail), detail, reason)
}

// opTargetName finds the name a person knows the target by.
func (s *Server) opTargetName(ctx context.Context, org, entityType, entityID string, detail any) string {
	if m, ok := detail.(map[string]any); ok {
		for _, k := range []string{"camera", "recipe", "display", "name"} {
			if v, ok := m[k].(string); ok && v != "" {
				return v
			}
		}
	}
	if !uuidRe.MatchString(strings.ToLower(entityID)) {
		return ""
	}
	var q string
	switch entityType {
	case "device":
		q = `SELECT COALESCE(NULLIF(name, ''), external_id) FROM core.device WHERE id = $1::uuid AND org_id = $2::uuid`
	case "display":
		q = `SELECT name FROM core.display WHERE id = $1::uuid AND org_id = $2::uuid`
	case "flow":
		q = `SELECT name FROM core.flow WHERE id = $1::uuid AND org_id = $2::uuid`
	case "alarm":
		q = `SELECT COALESCE(ast.name || ' · ', '') || COALESCE(a.detail->>'rule', a.detail->>'key', '') FROM core.alarm a
			LEFT JOIN core.datastream d ON d.id = a.datastream_id LEFT JOIN core.asset ast ON ast.id = d.asset_id WHERE a.id = $1::uuid AND a.org_id = $2::uuid`
	default:
		return ""
	}
	var name string
	_ = s.pool.QueryRow(ctx, q, entityID, org).Scan(&name)
	return name
}

// POST /api/v1/operator-actions — a page reports what the server cannot see: a picture or a faceplate was opened, a
// control was pressed and the press was cancelled. Answers whether the organization keeps the log, so a page stops
// reporting when it does not.
func (s *Server) reportOperatorAction(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	if p == nil || p.OrgID == "" || p.SessionID == "" {
		writeErr(w, 403, "a signed-in user is needed")
		return
	}
	var in struct {
		Kind   string         `json:"kind"`
		Target string         `json:"target"`
		Detail map[string]any `json:"detail"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !opClientKinds[in.Kind] {
		writeErr(w, 400, "kind: navigate, popup, press or cancel")
		return
	}
	in.Target = strings.TrimSpace(in.Target)
	if in.Target == "" || len(in.Target) > 200 {
		writeErr(w, 400, "target: 1 to 200 characters")
		return
	}
	on := s.opLogOn(r.Context(), p.OrgID)
	if on {
		s.opLog(r, in.Kind, "screen", "", in.Target, in.Detail, "")
	}
	writeJSON(w, 200, map[string]any{"logged": on})
}

const opCols = `a.id::text, a.ts, a.user_name, a.actor_type, a.operator, a.kind, a.target_type, COALESCE(a.target_id::text, ''), a.target, a.screen, a.detail, a.reason, a.station`

// GET /api/v1/operator-actions
func (s *Server) listOperatorActions(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	from, to, ok := timeRange(w, r, 24*time.Hour)
	if !ok {
		return
	}
	q := r.URL.Query()
	if q.Get("to") == "" {
		to = to.Add(time.Hour) // the clock of the database may be a little ahead: the newest rows must not fall out
	}
	limit := 200
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 && n <= 1000 {
		limit = n
	}
	if v := q.Get("before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			writeErr(w, 400, "before must be RFC3339")
			return
		}
		if t.Before(to) {
			to = t
		}
	}
	kinds := []string{}
	for _, k := range strings.Split(q.Get("kind"), ",") {
		if k = strings.TrimSpace(k); k != "" {
			kinds = append(kinds, k)
		}
	}
	text := strings.TrimSpace(q.Get("q"))
	if len(text) > 80 {
		writeErr(w, 400, "q: at most 80 characters")
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT `+opCols+` FROM core.operator_action a
		WHERE a.org_id = $1::uuid AND a.ts >= $2 AND a.ts < $3 AND ($4 = '' OR a.user_name = $4 OR a.operator = $4)
		  AND (cardinality($5::text[]) = 0 OR a.kind = ANY($5::text[]))
		  AND ($6 = '' OR a.target ILIKE '%' || $6 || '%' OR a.screen ILIKE '%' || $6 || '%' OR a.reason ILIKE '%' || $6 || '%' OR a.detail::text ILIKE '%' || $6 || '%')
		ORDER BY a.ts DESC, a.id DESC LIMIT $7`, org, from, to, strings.TrimSpace(q.Get("user")), kinds, strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text), limit)
	if err != nil {
		s.log.Error("operator actions", "err", err)
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, user, actor, operator, kind, ttype, tid, target, screen, reason, station string
		var ts time.Time
		var detail json.RawMessage
		if err := rows.Scan(&id, &ts, &user, &actor, &operator, &kind, &ttype, &tid, &target, &screen, &detail, &reason, &station); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, map[string]any{"id": id, "ts": ts, "user": user, "actor_type": actor, "operator": operator, "kind": kind, "target_type": ttype, "target_id": tid,
			"target": target, "screen": screen, "detail": detail, "reason": reason, "station": station})
	}
	var users []string
	urows, err := s.pool.Query(r.Context(), `SELECT DISTINCT user_name FROM core.operator_action WHERE org_id = $1::uuid AND ts >= $2 AND ts < $3 ORDER BY 1 LIMIT 200`, org, from, to)
	if err == nil {
		defer urows.Close()
		for urows.Next() {
			var u string
			if urows.Scan(&u) == nil {
				users = append(users, u)
			}
		}
	}
	if users == nil {
		users = []string{}
	}
	writeJSON(w, 200, map[string]any{"enabled": s.opLogOn(r.Context(), org), "actions": out, "users": users, "kinds": opKinds()})
}

// GET /api/v1/export/operator-actions
func (s *Server) exportOperatorActions(w http.ResponseWriter, r *http.Request) {
	from, to, ok := timeRange(w, r, 31*24*time.Hour)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	s.streamExport(w, r, exportSpec{
		name:    "operator_actions",
		columns: []string{"time", "user", "operator", "kind", "target", "screen", "reason", "detail", "station", "actor_type"},
		query: `SELECT a.ts, a.user_name, a.operator, a.kind, a.target, a.screen, a.reason, a.detail::text, a.station, a.actor_type
		        FROM core.operator_action a WHERE ($1 = '' OR a.org_id = $1::uuid) AND a.ts >= $2 AND a.ts < $3 ORDER BY a.ts LIMIT $4`,
		args:  []any{orgOf(p), from, to, exportMaxRows},
		kind:  "generic",
		title: "Operator action log",
		from:  from, to: to,
	})
}

package httpapi

// SCADA users (spec 041, 0.95.0, ADR-0026 step 3): a second level of identity that exists only at the controls.
//
// The station — a computer in the control room — is signed in with an account as before. On top of that session a
// person signs in inside a picture or a dashboard with a name and a PIN or a password (the administrator chooses per
// person), and is signed out after a time without activity. A SCADA user is not an account: it has no session, no
// permissions and no access to the API; it never gets more than the account of the station may do.
//
// Groups as in WinCC (Operator, Shift leader, Technologist, Maintenance by default). A control of a dashboard and an
// element of a process picture can name the groups that may use it. The command names the control it came from; the
// server reads the groups (and the mark "needs a second person") from the stored dashboard and checks them.
//
//	GET    /api/v1/scada/groups                       (data.read)
//	POST   /api/v1/scada/groups {key, name, description, approves, reason}        (user.admin)
//	PUT    /api/v1/scada/groups/{id} {name, description, approves, position, reason}
//	DELETE /api/v1/scada/groups/{id}?reason=
//	GET    /api/v1/scada/users                        (user.admin)
//	POST   /api/v1/scada/users {name, full_name, secret_kind, secret, groups, reason}
//	PUT    /api/v1/scada/users/{id} {full_name, enabled, groups, secret_kind, secret, unlock, reason}
//	DELETE /api/v1/scada/users/{id}?reason=
//	GET    /api/v1/scada/session                      who is signed in at this station (browser session)
//	POST   /api/v1/scada/session {name, secret}       sign in at the controls
//	POST   /api/v1/scada/session/touch                the person is still there
//	DELETE /api/v1/scada/session                      sign out

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

var scadaGroupKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// scadaSess is the person signed in at the controls of a station.
type scadaSess struct {
	ID, UserID, Name, FullName string
	Left                       int // seconds until the sign-out for inactivity
}

// scadaAt returns the person signed in at the controls of the caller's station. Only a browser session can carry one.
func (s *Server) scadaAt(ctx context.Context, p *iam.Principal) *scadaSess {
	if p == nil || p.OrgID == "" || p.SessionID == "" {
		return nil
	}
	var x scadaSess
	err := s.pool.QueryRow(ctx, `SELECT ss.id::text, ss.scada_user_id::text, ss.name, u.full_name,
			GREATEST(0, EXTRACT(EPOCH FROM (ss.last_seen_at + make_interval(mins => i.m) - now())))::int
		FROM core.scada_session ss JOIN core.scada_user u ON u.id = ss.scada_user_id AND u.enabled
		JOIN core.organization o ON o.id = ss.org_id,
		LATERAL (SELECT CASE WHEN (o.settings->'security'->>'scada_idle_minutes') ~ '^[0-9]{1,3}$' AND (o.settings->'security'->>'scada_idle_minutes')::int BETWEEN 1 AND 480
			THEN (o.settings->'security'->>'scada_idle_minutes')::int ELSE 10 END AS m) i
		WHERE ss.station = $1 AND ss.org_id = $2::uuid AND ss.ended_at IS NULL AND ss.last_seen_at + make_interval(mins => i.m) > now()
		ORDER BY ss.started_at DESC LIMIT 1`, p.SessionID, p.OrgID).Scan(&x.ID, &x.UserID, &x.Name, &x.FullName, &x.Left)
	if err != nil {
		return nil
	}
	return &x
}

// opName is the SCADA user an action is recorded with: the second person who approved on the spot with a PIN, or the
// person signed in at the controls of the station.
func (s *Server) opName(r *http.Request) string {
	if si := spotFrom(r.Context()); si != nil {
		return si.Operator
	}
	if x := s.scadaAt(r.Context(), PrincipalFrom(r.Context())); x != nil {
		return x.Name
	}
	return ""
}

func (s *Server) scadaGroupsOf(ctx context.Context, userID string) []string {
	out := []string{}
	rows, err := s.pool.Query(ctx, `SELECT g.key FROM core.scada_user_group ug JOIN core.scada_group g ON g.id = ug.group_id WHERE ug.user_id = $1::uuid ORDER BY g.position, g.key`, userID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		if rows.Scan(&k) == nil {
			out = append(out, k)
		}
	}
	return out
}

func (s *Server) scadaTouch(ctx context.Context, id string) {
	_, _ = s.pool.Exec(context.WithoutCancel(ctx), `UPDATE core.scada_session SET last_seen_at = now() WHERE id = $1::uuid AND ended_at IS NULL`, id)
}

// ---- the control a command came from --------------------------------------------------------------------------------

// controlRef names a control: a widget of a dashboard or an element of a process picture.
type controlRef struct {
	Dashboard string `json:"dashboard"`
	ID        string `json:"id"`
}

// control is what the stored dashboard says about it.
type control struct {
	Name   string
	Groups []string
	Second bool
}

var controlWidgets = map[string]bool{"setpoint": true, "switch": true, "button": true, "knob": true, "slider": true, "round_switch": true}

func strList(v any) []string {
	out := []string{}
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, x...)
	}
	return out
}

// keysIn collects every command key an element can send: its own and those of its events.
func keysIn(v any, out map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			if k == "key" {
				if s, ok := e.(string); ok && s != "" {
					out[s] = true
				}
				continue
			}
			keysIn(e, out)
		}
	case []any:
		for _, e := range x {
			keysIn(e, out)
		}
	}
}

// resolveControl finds the control a command names and checks that the command can come from it: the control exists in
// a dashboard of the organization, it is a control, and the key is the one it sends (for a faceplate type, whose keys
// are rebound to another prefix, the part after the prefix).
func (s *Server) resolveControl(ctx context.Context, org string, ref *controlRef, args json.RawMessage) (*control, error) {
	if ref == nil || (ref.Dashboard == "" && ref.ID == "") {
		return nil, nil
	}
	if !dashSlugRe.MatchString(ref.Dashboard) || !widgetIDRe.MatchString(ref.ID) {
		return nil, errors.New("control: dashboard must be a slug and id the id of a widget or an element")
	}
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT spec FROM core.dashboard WHERE org_id = $1::uuid AND valid_to IS NULL AND slug = $2`, org, ref.Dashboard).Scan(&raw); err != nil {
		return nil, errors.New("control: dashboard not found")
	}
	var spec struct {
		Faceplate *struct {
			Prefix string `json:"prefix"`
		} `json:"faceplate"`
		Widgets []struct {
			ID     string         `json:"id"`
			Type   string         `json:"type"`
			Params map[string]any `json:"params"`
		} `json:"widgets"`
		Elements []map[string]any `json:"elements"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, errors.New("control: dashboard not readable")
	}
	var a struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(args, &a)
	prefix := ""
	if spec.Faceplate != nil {
		prefix = spec.Faceplate.Prefix
	}
	keyOK := func(keys map[string]bool) bool {
		if a.Key == "" || len(keys) == 0 {
			return true // a method without a key (a button that calls a method), or a control without one
		}
		for k := range keys {
			if k == a.Key || (spec.Faceplate != nil && strings.HasSuffix(a.Key, strings.TrimPrefix(k, prefix))) {
				return true
			}
		}
		return false
	}
	for _, w := range spec.Widgets {
		if w.ID != ref.ID {
			continue
		}
		if !controlWidgets[w.Type] {
			return nil, errors.New("control: this widget sends no commands")
		}
		keys := map[string]bool{}
		if k, _ := w.Params["key"].(string); k != "" {
			keys[k] = true
		}
		if !keyOK(keys) {
			return nil, errors.New("control: the key is not the one this control sends")
		}
		c := &control{Groups: strList(w.Params["scada_groups"])}
		c.Name, _ = w.Params["title"].(string)
		c.Second, _ = w.Params["second_person"].(bool)
		return c, nil
	}
	for _, e := range spec.Elements {
		if id, _ := e["id"].(string); id != ref.ID {
			continue
		}
		keys := map[string]bool{}
		keysIn(e, keys)
		if !keyOK(keys) {
			return nil, errors.New("control: the key is not the one this control sends")
		}
		c := &control{Groups: strList(e["scada_groups"])}
		c.Name, _ = e["name"].(string)
		c.Second, _ = e["second_person"].(bool)
		return c, nil
	}
	return nil, errors.New("control: not found on the dashboard")
}

// scadaGate decides who is at the controls before something is sent to the plant, and answers 403 itself when the
// person may not. kind says what is sent: "command" (it can name its control), "recipe" or "scene".
//
//   - A control that names groups needs a SCADA user who is a member of one of them.
//   - With the policy scada_users_required, everything sent from a browser session needs a SCADA user. The exception is
//     an account with device.manage that names no control: an engineer on the device page works under their own name.
//     Everybody else can then send commands only from controls, so the groups of the controls cannot be walked around.
//   - API tokens and AI assistants are the system, not a person at the controls: they are not asked for a SCADA user,
//     and cannot use a control that names groups.
func (s *Server) scadaGate(w http.ResponseWriter, r *http.Request, org, kind string, c *control) (operator string, ok bool) {
	p := PrincipalFrom(r.Context())
	if p == nil || s.iam == nil {
		return "", true
	}
	groups := []string{}
	if c != nil {
		groups = c.Groups
	}
	deny := func(code, msg string) (string, bool) {
		writeJSON(w, 403, map[string]any{"error": msg, "code": code, "groups": s.scadaGroupNames(r.Context(), org, groups)})
		return "", false
	}
	if p.SessionID == "" {
		if len(groups) > 0 {
			return deny("scada_group_required", "this control is for people signed in at the controls")
		}
		return "", true
	}
	required := s.iam.Policy(r.Context(), org).ScadaUsersRequired
	x := s.scadaAt(r.Context(), p)
	// an account that may manage devices works under its own name outside the pictures (the device page, recipes)
	own := c == nil && p.Has("device.manage")
	if x == nil {
		if len(groups) > 0 || (required && !own) {
			return deny("scada_user_required", "sign in at the controls first")
		}
		return "", true
	}
	if len(groups) > 0 {
		member := false
		mine := s.scadaGroupsOf(r.Context(), x.UserID)
		for _, g := range groups {
			for _, m := range mine {
				member = member || g == m
			}
		}
		if !member {
			s.opLog(r, "scada_denied", "scada_user", x.UserID, x.Name, map[string]any{"control": c.Name, "groups": groups}, "")
			return deny("scada_group_required", "your group may not use this control")
		}
	}
	if required && kind == "command" && c == nil && !own {
		return deny("scada_control_required", "with SCADA users required, commands are sent from the controls of pictures and dashboards")
	}
	s.scadaTouch(r.Context(), x.ID)
	return x.Name, true
}

// scadaGroupNames turns group keys into the names people read.
func (s *Server) scadaGroupNames(ctx context.Context, org string, keys []string) []string {
	out := []string{}
	if len(keys) == 0 {
		return out
	}
	names := map[string]string{}
	rows, err := s.pool.Query(ctx, `SELECT key, name FROM core.scada_group WHERE org_id = $1::uuid AND key = ANY($2::text[])`, org, keys)
	if err == nil {
		for rows.Next() {
			var k, n string
			if rows.Scan(&k, &n) == nil {
				names[k] = n
			}
		}
		rows.Close()
	}
	for _, k := range keys {
		if n := names[k]; n != "" {
			out = append(out, n)
		} else {
			out = append(out, k)
		}
	}
	return out
}

// validGroupKeys normalizes the groups a control names (dashboard validation).
func validGroupKeys(v any) ([]string, error) {
	list := strList(v)
	if s, ok := v.(string); ok {
		for _, x := range strings.Split(s, ",") {
			if x = strings.TrimSpace(x); x != "" {
				list = append(list, x)
			}
		}
	}
	if len(list) > 16 {
		return nil, errors.New("at most 16 groups")
	}
	seen := map[string]bool{}
	out := []string{}
	for _, k := range list {
		if !scadaGroupKeyRe.MatchString(k) {
			return nil, errors.New("groups must be keys of SCADA groups (a-z, 0-9, _)")
		}
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out, nil
}

// ---- sign-in at the controls ----------------------------------------------------------------------------------------

func (s *Server) scadaState(ctx context.Context, p *iam.Principal) map[string]any {
	pol := s.iam.Policy(ctx, p.OrgID)
	out := map[string]any{"required": pol.ScadaUsersRequired, "idle_minutes": pol.ScadaIdle(), "user": nil}
	var n int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM core.scada_user WHERE org_id = $1::uuid AND enabled`, p.OrgID).Scan(&n)
	out["users"] = n
	if x := s.scadaAt(ctx, p); x != nil {
		out["user"] = map[string]any{"name": x.Name, "full_name": x.FullName, "groups": s.scadaGroupsOf(ctx, x.UserID), "seconds_left": x.Left}
	}
	return out
}

// GET /api/v1/scada/session
func (s *Server) scadaSession(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mustOrg(w, r); !ok {
		return
	}
	writeJSON(w, 200, s.scadaState(r.Context(), PrincipalFrom(r.Context())))
}

// POST /api/v1/scada/session {name, secret}
func (s *Server) scadaSignIn(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	if p.SessionID == "" {
		writeErr(w, 403, "a SCADA user signs in at a station: in a browser session, not with an API token")
		return
	}
	var in struct {
		Name   string `json:"name"`
		Secret string `json:"secret"`
	}
	if err := decodeJSON(r, &in, 2048); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Name) == "" || in.Secret == "" {
		writeErr(w, 400, "name and secret (the PIN or the password) are required")
		return
	}
	if !s.stepUpLimiter.peek("scada:" + p.SessionID) {
		writeErr(w, 429, "too many failed attempts; wait a minute")
		return
	}
	u, err := s.iam.ScadaVerify(r.Context(), org, in.Name, in.Secret, "scada_login", clientIP(r), r.UserAgent())
	if err != nil {
		s.stepUpLimiter.allow("scada:" + p.SessionID)
		s.log.Warn("SCADA sign-in refused", "security", true, "station_user", p.Username, "name", in.Name, "err", err, "remote", clientIP(r))
		s.opLog(r, "scada_failed", "scada_user", "", strings.ToLower(strings.TrimSpace(in.Name)), nil, "")
		msg := "wrong name, PIN or password"
		if errors.Is(err, iam.ErrLocked) {
			msg = "this SCADA user is locked; try again later or ask the administrator"
		}
		writeJSON(w, 403, map[string]string{"error": msg, "code": "scada_failed"})
		return
	}
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.scada_session SET ended_at = now(), ended_why = 'replaced' WHERE station = $1 AND org_id = $2::uuid AND ended_at IS NULL`, p.SessionID, org)
	if _, err := s.pool.Exec(r.Context(), `INSERT INTO core.scada_session (org_id, scada_user_id, name, station, station_user, station_ip) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)`,
		org, u.ID, u.Name, p.SessionID, p.Username, clientIP(r)); err != nil {
		s.log.Error("scada session", "err", err)
		writeErr(w, 500, "sign-in failed")
		return
	}
	s.opLog(r, "scada_login", "scada_user", u.ID, u.Name, map[string]any{"groups": u.Groups}, "")
	writeJSON(w, 200, s.scadaState(r.Context(), p))
}

// POST /api/v1/scada/session/touch — the page reports that the person is still working.
func (s *Server) scadaTouchH(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mustOrg(w, r); !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	if x := s.scadaAt(r.Context(), p); x != nil {
		s.scadaTouch(r.Context(), x.ID)
	}
	writeJSON(w, 200, s.scadaState(r.Context(), p))
}

// DELETE /api/v1/scada/session?why=idle
func (s *Server) scadaSignOut(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	why := "signed_out"
	if r.URL.Query().Get("why") == "idle" {
		why = "idle"
	}
	if x := s.scadaAt(r.Context(), p); x != nil {
		s.opLog(r, "scada_logout", "scada_user", x.UserID, x.Name, map[string]any{"why": why}, "")
	}
	if p.SessionID != "" {
		_, _ = s.pool.Exec(r.Context(), `UPDATE core.scada_session SET ended_at = now(), ended_why = $3 WHERE station = $1 AND org_id = $2::uuid AND ended_at IS NULL`, p.SessionID, org, why)
	}
	writeJSON(w, 200, s.scadaState(r.Context(), p))
}

// ---- groups ---------------------------------------------------------------------------------------------------------

// GET /api/v1/scada/groups
func (s *Server) listScadaGroups(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT g.id::text, g.key, g.name, g.description, g.approves, g.position,
		(SELECT count(*) FROM core.scada_user_group ug WHERE ug.group_id = g.id) FROM core.scada_group g WHERE g.org_id = $1::uuid ORDER BY g.position, g.name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, key, name, desc string
		var approves bool
		var pos, n int
		if err := rows.Scan(&id, &key, &name, &desc, &approves, &pos, &n); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, map[string]any{"id": id, "key": key, "name": name, "description": desc, "approves": approves, "position": pos, "members": n})
	}
	writeJSON(w, 200, out)
}

type scadaGroupIn struct {
	Key         string  `json:"key"`
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Approves    *bool   `json:"approves"`
	Position    *int    `json:"position"`
	Reason      string  `json:"reason"`
}

// POST /api/v1/scada/groups
func (s *Server) createScadaGroup(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in scadaGroupIn
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	name := ""
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	if name == "" || len(name) > 60 {
		writeErr(w, 400, "name: 1 to 60 characters")
		return
	}
	key := strings.TrimSpace(in.Key)
	if key == "" {
		key = groupKeyOf(name)
	}
	if !scadaGroupKeyRe.MatchString(key) {
		writeErr(w, 400, "key must match ^[a-z][a-z0-9_]{0,31}$")
		return
	}
	desc, approves, pos := "", false, 0
	if in.Description != nil {
		desc = strings.TrimSpace(*in.Description)
	}
	if in.Approves != nil {
		approves = *in.Approves
	}
	if in.Position != nil {
		pos = *in.Position
	} else {
		_ = s.pool.QueryRow(r.Context(), `SELECT COALESCE(max(position), 0) + 1 FROM core.scada_group WHERE org_id = $1::uuid`, org).Scan(&pos)
	}
	if len(desc) > 200 || pos < 0 || pos > 9999 {
		writeErr(w, 400, "description: at most 200 characters; position 0 to 9999")
		return
	}
	var n int
	_ = s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.scada_group WHERE org_id = $1::uuid`, org).Scan(&n)
	if n >= 32 {
		writeErr(w, 409, "at most 32 groups")
		return
	}
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.scada_group (org_id, key, name, description, approves, position) VALUES ($1::uuid, $2, $3, $4, $5, $6)
		ON CONFLICT (org_id, key) DO NOTHING RETURNING id::text`, org, key, name, desc, approves, pos).Scan(&id); err != nil {
		writeErr(w, 409, "a group with this key exists")
		return
	}
	g := map[string]any{"id": id, "key": key, "name": name, "description": desc, "approves": approves, "position": pos, "members": 0}
	s.audit(r, "scada_group.create", "scada_group", id, nil, g, in.Reason)
	writeJSON(w, 201, g)
}

// groupKeyOf makes a key from a name: "Shift leader" → shift_leader.
func groupKeyOf(name string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(name) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9' && b.Len() > 0:
			b.WriteRune(c)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "_"):
			b.WriteByte('_')
		}
	}
	k := strings.TrimRight(b.String(), "_")
	if len(k) > 32 {
		k = strings.TrimRight(k[:32], "_")
	}
	return k
}

// PUT /api/v1/scada/groups/{id}
func (s *Server) updateScadaGroup(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var in scadaGroupIn
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var key, name, desc string
	var approves bool
	var pos int
	if err := s.pool.QueryRow(r.Context(), `SELECT key, name, description, approves, position FROM core.scada_group WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).Scan(&key, &name, &desc, &approves, &pos); err != nil {
		writeErr(w, 404, "group not found")
		return
	}
	old := map[string]any{"key": key, "name": name, "description": desc, "approves": approves, "position": pos}
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		desc = strings.TrimSpace(*in.Description)
	}
	if in.Approves != nil {
		approves = *in.Approves
	}
	if in.Position != nil {
		pos = *in.Position
	}
	if name == "" || len(name) > 60 || len(desc) > 200 || pos < 0 || pos > 9999 {
		writeErr(w, 400, "name: 1 to 60 characters; description: at most 200; position 0 to 9999")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.scada_group SET name = $2, description = $3, approves = $4, position = $5 WHERE id = $1::uuid`, id, name, desc, approves, pos); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	g := map[string]any{"id": id, "key": key, "name": name, "description": desc, "approves": approves, "position": pos}
	s.audit(r, "scada_group.update", "scada_group", id, old, g, in.Reason)
	writeJSON(w, 200, g)
}

// DELETE /api/v1/scada/groups/{id}?reason= — controls that name the group stay closed to everyone until they are edited.
func (s *Server) deleteScadaGroup(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	var key, name string
	if err := s.pool.QueryRow(r.Context(), `DELETE FROM core.scada_group WHERE id = $1::uuid AND org_id = $2::uuid RETURNING key, name`, id, org).Scan(&key, &name); err != nil {
		writeErr(w, 404, "group not found")
		return
	}
	s.audit(r, "scada_group.delete", "scada_group", id, map[string]any{"key": key, "name": name}, nil, reason)
	writeJSON(w, 200, map[string]any{"id": id, "deleted": true})
}

// ---- users ----------------------------------------------------------------------------------------------------------

// GET /api/v1/scada/users
func (s *Server) listScadaUsers(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT u.id::text, u.name, u.full_name, u.secret_kind, u.enabled, u.locked_until, u.last_used_at, u.created_at,
		COALESCE((SELECT array_agg(g.key ORDER BY g.position, g.key) FROM core.scada_user_group ug JOIN core.scada_group g ON g.id = ug.group_id WHERE ug.user_id = u.id), '{}'),
		COALESCE((SELECT ss.station_user FROM core.scada_session ss WHERE ss.scada_user_id = u.id AND ss.ended_at IS NULL AND ss.last_seen_at > now() - interval '8 hours' ORDER BY ss.started_at DESC LIMIT 1), '')
		FROM core.scada_user u WHERE u.org_id = $1::uuid ORDER BY u.name`, org)
	if err != nil {
		s.log.Error("scada users", "err", err)
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, full, kind, at string
		var enabled bool
		var locked, used *time.Time
		var created time.Time
		var groups []string
		if err := rows.Scan(&id, &name, &full, &kind, &enabled, &locked, &used, &created, &groups, &at); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		if locked != nil && locked.Before(time.Now()) {
			locked = nil
		}
		out = append(out, map[string]any{"id": id, "name": name, "full_name": full, "secret_kind": kind, "enabled": enabled, "locked_until": locked, "last_used_at": used, "created_at": created, "groups": groups, "signed_in_at": at})
	}
	writeJSON(w, 200, out)
}

type scadaUserIn struct {
	Name       string    `json:"name"`
	FullName   *string   `json:"full_name"`
	SecretKind string    `json:"secret_kind"`
	Secret     string    `json:"secret"`
	Groups     *[]string `json:"groups"`
	Enabled    *bool     `json:"enabled"`
	Unlock     bool      `json:"unlock"`
	Reason     string    `json:"reason"`
}

// setScadaGroups replaces the memberships of a SCADA user; unknown keys are an error.
func (s *Server) setScadaGroups(ctx context.Context, org, userID string, keys []string) error {
	ks, err := validGroupKeys(keys)
	if err != nil {
		return err
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM core.scada_group WHERE org_id = $1::uuid AND key = ANY($2::text[])`, org, ks).Scan(&n); err != nil || n != len(ks) {
		return errors.New("groups: an unknown group")
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM core.scada_user_group WHERE user_id = $1::uuid`, userID); err != nil {
		return errors.New("update failed")
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO core.scada_user_group (user_id, group_id) SELECT $1::uuid, id FROM core.scada_group WHERE org_id = $2::uuid AND key = ANY($3::text[])`, userID, org, ks)
	return err
}

// POST /api/v1/scada/users
func (s *Server) createScadaUser(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in scadaUserIn
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	name := strings.ToLower(strings.TrimSpace(in.Name))
	if !iam.ScadaNameRe.MatchString(name) {
		writeErr(w, 400, "name: 1 to 32 characters — small letters, digits, dot, dash and underscore")
		return
	}
	full := ""
	if in.FullName != nil {
		full = strings.TrimSpace(*in.FullName)
	}
	if len(full) > 100 {
		writeErr(w, 400, "full_name: at most 100 characters")
		return
	}
	hash, err := s.iam.ScadaHash(in.SecretKind, in.Secret)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	groups := []string{}
	if in.Groups != nil {
		groups = *in.Groups
	}
	if _, err := validGroupKeys(groups); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var n int
	_ = s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.scada_user WHERE org_id = $1::uuid`, org).Scan(&n)
	if n >= 500 {
		writeErr(w, 409, "at most 500 SCADA users")
		return
	}
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.scada_user (org_id, name, full_name, secret_kind, secret_hash) VALUES ($1::uuid, $2, $3, $4, $5)
		ON CONFLICT (org_id, name) DO NOTHING RETURNING id::text`, org, name, full, in.SecretKind, hash).Scan(&id); err != nil {
		writeErr(w, 409, "a SCADA user with this name exists")
		return
	}
	if err := s.setScadaGroups(r.Context(), org, id, groups); err != nil {
		_, _ = s.pool.Exec(r.Context(), `DELETE FROM core.scada_user WHERE id = $1::uuid`, id)
		writeErr(w, 400, err.Error())
		return
	}
	u := map[string]any{"id": id, "name": name, "full_name": full, "secret_kind": in.SecretKind, "enabled": true, "groups": s.scadaGroupsOf(r.Context(), id)}
	s.audit(r, "scada_user.create", "scada_user", id, nil, u, in.Reason)
	writeJSON(w, 201, u)
}

// PUT /api/v1/scada/users/{id}
func (s *Server) updateScadaUser(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var in scadaUserIn
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var name, full, kind string
	var enabled bool
	if err := s.pool.QueryRow(r.Context(), `SELECT name, full_name, secret_kind, enabled FROM core.scada_user WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).Scan(&name, &full, &kind, &enabled); err != nil {
		writeErr(w, 404, "SCADA user not found")
		return
	}
	old := map[string]any{"name": name, "full_name": full, "secret_kind": kind, "enabled": enabled, "groups": s.scadaGroupsOf(r.Context(), id)}
	if in.FullName != nil {
		full = strings.TrimSpace(*in.FullName)
		if len(full) > 100 {
			writeErr(w, 400, "full_name: at most 100 characters")
			return
		}
	}
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	newSecret := false
	if in.Secret != "" || (in.SecretKind != "" && in.SecretKind != kind) {
		if in.SecretKind == "" {
			in.SecretKind = kind
		}
		hash, err := s.iam.ScadaHash(in.SecretKind, in.Secret)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if _, err := s.pool.Exec(r.Context(), `UPDATE core.scada_user SET secret_kind = $2, secret_hash = $3, failed_count = 0, locked_until = NULL WHERE id = $1::uuid`, id, in.SecretKind, hash); err != nil {
			writeErr(w, 500, "update failed")
			return
		}
		kind, newSecret = in.SecretKind, true
	}
	if in.Groups != nil {
		if err := s.setScadaGroups(r.Context(), org, id, *in.Groups); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.scada_user SET full_name = $2, enabled = $3, updated_at = now(),
		failed_count = CASE WHEN $4 THEN 0 ELSE failed_count END, locked_until = CASE WHEN $4 THEN NULL ELSE locked_until END WHERE id = $1::uuid`, id, full, enabled, in.Unlock); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	if !enabled || newSecret {
		// a disabled person, or one whose secret was replaced, is signed out everywhere
		_, _ = s.pool.Exec(r.Context(), `UPDATE core.scada_session SET ended_at = now(), ended_why = 'removed' WHERE scada_user_id = $1::uuid AND ended_at IS NULL`, id)
	}
	u := map[string]any{"id": id, "name": name, "full_name": full, "secret_kind": kind, "enabled": enabled, "groups": s.scadaGroupsOf(r.Context(), id)}
	nw := map[string]any{"name": name, "full_name": full, "secret_kind": kind, "enabled": enabled, "groups": u["groups"], "secret_changed": newSecret, "unlocked": in.Unlock}
	s.audit(r, "scada_user.update", "scada_user", id, old, nw, in.Reason)
	writeJSON(w, 200, u)
}

// DELETE /api/v1/scada/users/{id}?reason= — signs the person out everywhere; the records keep the name.
func (s *Server) deleteScadaUser(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	var name string
	if err := s.pool.QueryRow(r.Context(), `SELECT name FROM core.scada_user WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).Scan(&name); err != nil {
		writeErr(w, 404, "SCADA user not found")
		return
	}
	// the sign-ins of the person go with the row; commands, the audit trail and the operator log keep the name
	if _, err := s.pool.Exec(r.Context(), `DELETE FROM core.scada_user WHERE id = $1::uuid AND org_id = $2::uuid`, id, org); err != nil {
		s.log.Error("scada user delete", "err", err)
		writeErr(w, 500, "delete failed")
		return
	}
	s.audit(r, "scada_user.delete", "scada_user", id, map[string]any{"name": name}, nil, reason)
	writeJSON(w, 200, map[string]any{"id": id, "deleted": true})
}

// ---- retention of the operator log ----------------------------------------------------------------------------------

// purgeOperatorLog drops rows of the operator action log older than the retention each organization chose
// (policy operator_log_days; 0 keeps them for good). The database function refuses anything younger than 30 days.
func (s *Server) purgeOperatorLog(ctx context.Context) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, (settings->'security'->>'operator_log_days')::int FROM core.organization
		WHERE (settings->'security'->>'operator_log_days') ~ '^[0-9]{2,4}$' AND (settings->'security'->>'operator_log_days')::int >= 30`)
	if err != nil {
		return
	}
	type job struct {
		org  string
		days int
	}
	var jobs []job
	for rows.Next() {
		var j job
		if rows.Scan(&j.org, &j.days) == nil {
			jobs = append(jobs, j)
		}
	}
	rows.Close()
	for _, j := range jobs {
		var n int64
		if err := s.pool.QueryRow(ctx, `SELECT core.purge_operator_actions($1::uuid, $2)`, j.org, j.days).Scan(&n); err != nil {
			s.log.Error("operator log retention", "org", j.org, "err", err)
			continue
		}
		if n > 0 {
			s.log.Info("operator log retention", "org", j.org, "days", j.days, "deleted", n)
		}
	}
	// sign-ins nobody ended (a closed browser) are closed in the records
	_, _ = s.pool.Exec(ctx, `UPDATE core.scada_session SET ended_at = last_seen_at, ended_why = 'idle' WHERE ended_at IS NULL AND last_seen_at < now() - interval '1 day'`)
}

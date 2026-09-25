package httpapi

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/escalate"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/notify"
)

// Notification channels, escalation policies and the delivery log (URS C-06, 0.23.0), all config.write:
//
//	GET/POST /api/v1/notification-channels            list / create {name, kind, settings, reason}
//	PUT      /api/v1/notification-channels/{id}       {name?, settings?, enabled?, reason}
//	DELETE   /api/v1/notification-channels/{id}?reason=
//	POST     /api/v1/notification-channels/{id}/test  sends a test message, logged
//	GET/POST /api/v1/escalation-policies              list / create {name, steps:[{after_s, channel_id}], reason}
//	PUT      /api/v1/escalation-policies/{id}         {name?, steps, reason}  (new version in place)
//	DELETE   /api/v1/escalation-policies/{id}?reason= (refused while alarm rules use it)
//	GET      /api/v1/notifications?limit=             delivery log, newest first

func (s *Server) loadChannel(r *http.Request, org, id string) (*notify.Channel, string, error) {
	var ch notify.Channel
	var raw []byte
	var enabled bool
	err := s.pool.QueryRow(r.Context(), `SELECT id::text, org_id::text, name, kind, settings, enabled FROM core.notification_channel WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, id, org).
		Scan(&ch.ID, &ch.OrgID, &ch.Name, &ch.Kind, &raw, &enabled)
	if err != nil {
		return nil, "", err
	}
	_ = json.Unmarshal(raw, &ch.Settings)
	return &ch, string(raw), nil
}

func (s *Server) listChannels(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT id::text, name, kind, settings, enabled, created_at, revoked_at FROM core.notification_channel WHERE org_id = $1::uuid ORDER BY revoked_at NULLS FIRST, name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, kind string
		var raw []byte
		var enabled bool
		var created time.Time
		var revoked *time.Time
		if rows.Scan(&id, &name, &kind, &raw, &enabled, &created, &revoked) != nil {
			continue
		}
		var st notify.ChannelSettings
		_ = json.Unmarshal(raw, &st)
		out = append(out, map[string]any{"id": id, "name": name, "kind": kind, "settings": st.Redact(), "has_secret": st.Secret != "", "enabled": enabled, "created_at": created, "revoked_at": revoked})
	}
	writeJSON(w, 200, out)
}

func (s *Server) createChannel(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		Name     string          `json:"name"`
		Kind     string          `json:"kind"`
		Settings json.RawMessage `json:"settings"`
		Reason   string          `json:"reason"`
	}
	if err := decodeJSON(r, &in, 64<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 64 {
		writeErr(w, 400, "name required (max 64)")
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	st, err := notify.ValidateChannel(in.Kind, in.Settings)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	settings, _ := json.Marshal(st)
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.notification_channel (org_id, name, kind, settings) VALUES ($1::uuid, $2, $3, $4::jsonb) RETURNING id::text`, org, in.Name, in.Kind, string(settings)).Scan(&id); err != nil {
		writeErr(w, 409, "channel could not be created (duplicate name?)")
		return
	}
	s.audit(r, "notification_channel.create", "notification_channel", id, nil, map[string]any{"name": in.Name, "kind": in.Kind, "settings": st.Redact()}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id, "name": in.Name, "kind": in.Kind, "settings": st.Redact(), "enabled": true})
}

func (s *Server) updateChannel(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var in struct {
		Name     *string         `json:"name"`
		Settings json.RawMessage `json:"settings"`
		Enabled  *bool           `json:"enabled"`
		Reason   string          `json:"reason"`
	}
	if err := decodeJSON(r, &in, 64<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	ch, oldRaw, err := s.loadChannel(r, org, id)
	if err != nil {
		writeErr(w, 404, "channel not found")
		return
	}
	name := ch.Name
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		if name == "" || len(name) > 64 {
			writeErr(w, 400, "name required (max 64)")
			return
		}
	}
	st := ch.Settings
	if len(in.Settings) > 0 {
		next, err := notify.ValidateChannel(ch.Kind, in.Settings)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if next.Secret == "" {
			next.Secret = ch.Settings.Secret // blank keeps the stored secret
		}
		st = next
	}
	enabled := true
	_ = s.pool.QueryRow(r.Context(), `SELECT enabled FROM core.notification_channel WHERE id = $1::uuid`, id).Scan(&enabled)
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	settings, _ := json.Marshal(st)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.notification_channel SET name = $3, settings = $4::jsonb, enabled = $5 WHERE id = $1::uuid AND org_id = $2::uuid`, id, org, name, string(settings), enabled); err != nil {
		writeErr(w, 409, "update failed (duplicate name?)")
		return
	}
	var old notify.ChannelSettings
	_ = json.Unmarshal([]byte(oldRaw), &old)
	s.audit(r, "notification_channel.update", "notification_channel", id, map[string]any{"name": ch.Name, "settings": old.Redact()}, map[string]any{"name": name, "settings": st.Redact(), "enabled": enabled}, in.Reason)
	writeJSON(w, 200, map[string]any{"id": id, "name": name, "kind": ch.Kind, "settings": st.Redact(), "enabled": enabled})
}

func (s *Server) deleteChannel(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	tag, err := s.pool.Exec(r.Context(), `UPDATE core.notification_channel SET revoked_at = now(), enabled = false WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, id, org)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, 404, "channel not found")
		return
	}
	s.audit(r, "notification_channel.revoke", "notification_channel", id, nil, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) testChannel(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	ch, _, err := s.loadChannel(r, org, r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "channel not found")
		return
	}
	if s.esc == nil {
		writeErr(w, 503, "notifications not available")
		return
	}
	if err := s.esc.Test(r.Context(), *ch); err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	s.audit(r, "notification_channel.test", "notification_channel", ch.ID, nil, map[string]any{"name": ch.Name, "kind": ch.Kind}, "test message")
	writeJSON(w, 200, map[string]any{"ok": true, "channel": ch.Name, "kind": ch.Kind})
}

// ---- escalation policies --------------------------------------------------------------------------

func (s *Server) validateSteps(r *http.Request, org string, raw json.RawMessage) ([]escalate.Step, error) {
	var steps []escalate.Step
	if err := json.Unmarshal(raw, &steps); err != nil {
		return nil, errBad("steps must be an array of {after_s, channel_id}")
	}
	if len(steps) == 0 || len(steps) > 10 {
		return nil, errBad("1..10 steps required")
	}
	for i, st := range steps {
		if st.AfterS < 0 || st.AfterS > 86400*7 {
			return nil, errBad("step " + strconv.Itoa(i+1) + ": after_s must be 0..604800")
		}
		if !uuidRe.MatchString(st.ChannelID) {
			return nil, errBad("step " + strconv.Itoa(i+1) + ": channel_id required")
		}
		var exists bool
		if err := s.pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM core.notification_channel WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL)`, st.ChannelID, org).Scan(&exists); err != nil || !exists {
			return nil, errBad("step " + strconv.Itoa(i+1) + ": unknown channel")
		}
	}
	sort.SliceStable(steps, func(i, j int) bool { return steps[i].AfterS < steps[j].AfterS })
	return steps, nil
}

type badRequest string

func (b badRequest) Error() string { return string(b) }
func errBad(s string) error        { return badRequest(s) }

func (s *Server) listPolicies(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT p.id::text, p.name, p.steps, p.version, p.revoked_at,
		(SELECT count(*) FROM core.alarm_rule ar WHERE ar.escalation_policy_id = p.id AND ar.valid_to IS NULL AND ar.enabled)
		FROM core.escalation_policy p WHERE p.org_id = $1::uuid ORDER BY p.revoked_at NULLS FIRST, p.name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	names := map[string]string{}
	if crow, err := s.pool.Query(r.Context(), `SELECT id::text, name FROM core.notification_channel WHERE org_id = $1::uuid`, org); err == nil {
		for crow.Next() {
			var id, name string
			if crow.Scan(&id, &name) == nil {
				names[id] = name
			}
		}
		crow.Close()
	}
	out := []map[string]any{}
	for rows.Next() {
		var id, name string
		var raw []byte
		var version int
		var revoked *time.Time
		var used int64
		if rows.Scan(&id, &name, &raw, &version, &revoked, &used) != nil {
			continue
		}
		var steps []escalate.Step
		_ = json.Unmarshal(raw, &steps)
		list := make([]map[string]any, 0, len(steps))
		for _, st := range steps {
			list = append(list, map[string]any{"after_s": st.AfterS, "channel_id": st.ChannelID, "channel_name": names[st.ChannelID]})
		}
		out = append(out, map[string]any{"id": id, "name": name, "steps": list, "version": version, "revoked_at": revoked, "rules": used})
	}
	writeJSON(w, 200, out)
}

func (s *Server) createPolicy(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		Name   string          `json:"name"`
		Steps  json.RawMessage `json:"steps"`
		Reason string          `json:"reason"`
	}
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 64 {
		writeErr(w, 400, "name required (max 64)")
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	steps, err := s.validateSteps(r, org, in.Steps)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	b, _ := json.Marshal(steps)
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.escalation_policy (org_id, name, steps, reason) VALUES ($1::uuid, $2, $3::jsonb, $4) RETURNING id::text`, org, in.Name, string(b), in.Reason).Scan(&id); err != nil {
		writeErr(w, 409, "policy could not be created (duplicate name?)")
		return
	}
	s.audit(r, "escalation_policy.create", "escalation_policy", id, nil, map[string]any{"name": in.Name, "steps": steps}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id, "name": in.Name, "steps": steps, "version": 1})
}

func (s *Server) updatePolicy(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var in struct {
		Name   *string         `json:"name"`
		Steps  json.RawMessage `json:"steps"`
		Reason string          `json:"reason"`
	}
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var name string
	var oldRaw []byte
	var version int
	if err := s.pool.QueryRow(r.Context(), `SELECT name, steps, version FROM core.escalation_policy WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, id, org).Scan(&name, &oldRaw, &version); err != nil {
		writeErr(w, 404, "policy not found")
		return
	}
	oldName := name
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		if name == "" || len(name) > 64 {
			writeErr(w, 400, "name required (max 64)")
			return
		}
	}
	steps, err := s.validateSteps(r, org, in.Steps)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	b, _ := json.Marshal(steps)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.escalation_policy SET name = $3, steps = $4::jsonb, version = version + 1, reason = $5 WHERE id = $1::uuid AND org_id = $2::uuid`, id, org, name, string(b), in.Reason); err != nil {
		writeErr(w, 409, "update failed (duplicate name?)")
		return
	}
	s.audit(r, "escalation_policy.update", "escalation_policy", id, map[string]any{"name": oldName, "steps": json.RawMessage(oldRaw), "version": version}, map[string]any{"name": name, "steps": steps, "version": version + 1}, in.Reason)
	writeJSON(w, 200, map[string]any{"id": id, "name": name, "steps": steps, "version": version + 1})
}

func (s *Server) deletePolicy(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	var used int64
	_ = s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.alarm_rule WHERE escalation_policy_id = $1::uuid AND valid_to IS NULL AND enabled`, id).Scan(&used)
	if used > 0 {
		writeErr(w, 409, "policy is used by "+strconv.FormatInt(used, 10)+" active alarm rule(s)")
		return
	}
	tag, err := s.pool.Exec(r.Context(), `UPDATE core.escalation_policy SET revoked_at = now() WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, id, org)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, 404, "policy not found")
		return
	}
	s.audit(r, "escalation_policy.revoke", "escalation_policy", id, nil, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/v1/notifications?limit=
func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	rows, err := s.pool.Query(r.Context(), `SELECT n.id::text, n.alarm_id::text, c.name, c.kind, n.step, n.event, n.due_at, n.sent_at, n.status, n.attempts, n.error, n.payload->>'subject'
		FROM core.notification n LEFT JOIN core.notification_channel c ON c.id = n.channel_id
		WHERE n.org_id = $1::uuid ORDER BY n.due_at DESC LIMIT $2`, org, limit)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id string
		var alarmID, channel, kind, errText, subject *string
		var step, attempts int
		var event, status string
		var due time.Time
		var sent *time.Time
		if rows.Scan(&id, &alarmID, &channel, &kind, &step, &event, &due, &sent, &status, &attempts, &errText, &subject) != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "alarm_id": alarmID, "channel": channel, "kind": kind, "step": step, "event": event, "due_at": due, "sent_at": sent, "status": status, "attempts": attempts, "error": errText, "subject": subject})
	}
	writeJSON(w, 200, out)
}

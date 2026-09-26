package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/rules"
)

// Automation rules "condition → action" (URS C-09, 0.24.0):
//
//	GET    /api/v1/rules                     (config.write) list with spec, status, version and live state
//	POST   /api/v1/rules                     (config.write; device.command when the rule sends commands)
//	                                         {name, description, spec, reason} → 201 {id, status, version}
//	PUT    /api/v1/rules/{id}                {name?, description?, spec?, enabled?, reason} → new version in place
//	DELETE /api/v1/rules/{id}?reason=        revoke (alarms raised by the rule are cleared)
//	POST   /api/v1/rules/{id}/approve        (device.command, not the last editor) four eyes for command rules
//	POST   /api/v1/rules/{id}/reject         (device.command)
//	POST   /api/v1/rules/test                {spec, rule_id?} dry run against the current values, no actions
//	GET    /api/v1/rule-runs?rule_id=&limit= transitions with their action results, newest first
//
// With the organization policy four_eyes_commands a rule whose actions send device commands is stored as
// 'pending' and evaluated only after a different user with device.command approves it.

func (s *Server) rulesAvailable(w http.ResponseWriter) bool {
	if s.rules == nil {
		writeErr(w, 503, "automation rules are not available on this server")
		return false
	}
	return true
}

func (s *Server) listRules(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.rulesAvailable(w) {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT r.id::text, r.name, r.description, r.spec, r.enabled, r.status, r.version, r.created_by_name, r.updated_by_name, r.approved_by_name, r.approved_at, r.created_at, r.updated_at, r.revoked_at,
		(SELECT fired_at FROM core.rule_run rr WHERE rr.rule_id = r.id ORDER BY fired_at DESC LIMIT 1), (SELECT event FROM core.rule_run rr WHERE rr.rule_id = r.id ORDER BY fired_at DESC LIMIT 1)
		FROM core.rule r WHERE r.org_id = $1::uuid ORDER BY r.revoked_at NULLS FIRST, r.name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, desc, status, createdBy, updatedBy string
		var spec json.RawMessage
		var enabled bool
		var version int
		var approvedBy, lastEvent *string
		var approvedAt, revoked, lastRun *time.Time
		var created, updated time.Time
		if rows.Scan(&id, &name, &desc, &spec, &enabled, &status, &version, &createdBy, &updatedBy, &approvedBy, &approvedAt, &created, &updated, &revoked, &lastRun, &lastEvent) != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "name": name, "description": desc, "spec": spec, "enabled": enabled, "status": status, "version": version,
			"created_by": createdBy, "updated_by": updatedBy, "approved_by": approvedBy, "approved_at": approvedAt, "created_at": created, "updated_at": updated, "revoked_at": revoked,
			"last_run": lastRun, "last_event": lastEvent, "state": s.rules.StatusOf(id)})
	}
	writeJSON(w, 200, out)
}

// checkSpec validates a spec structurally and against the organization's data.
func (s *Server) checkSpec(w http.ResponseWriter, r *http.Request, org string, spec rules.Spec) bool {
	if _, err := rules.Compile(spec); err != nil {
		writeErr(w, 400, err.Error())
		return false
	}
	if err := s.rules.CheckRefs(r.Context(), org, spec); err != nil {
		writeErr(w, 400, err.Error())
		return false
	}
	if spec.HasCommands() && !PrincipalFrom(r.Context()).Has("device.command") {
		writeErr(w, 403, "device.command permission required for rules that send commands")
		return false
	}
	return true
}

func (s *Server) createRule(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.rulesAvailable(w) {
		return
	}
	p := PrincipalFrom(r.Context())
	var in struct {
		Name        string     `json:"name"`
		Description string     `json:"description"`
		Spec        rules.Spec `json:"spec"`
		Reason      string     `json:"reason"`
	}
	if err := decodeJSON(r, &in, 256<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 64 {
		writeErr(w, 400, "name required (max 64)")
		return
	}
	if len(in.Description) > 1000 {
		writeErr(w, 400, "description max 1000 characters")
		return
	}
	if !reasonOf(w, in.Reason) || !s.checkSpec(w, r, org, in.Spec) {
		return
	}
	status := "active"
	if in.Spec.HasCommands() && s.iam != nil && s.iam.Policy(r.Context(), org).FourEyesCommands {
		status = "pending"
	}
	spec, _ := json.Marshal(in.Spec)
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.rule (org_id, name, description, spec, status, created_by, created_by_name, updated_by, updated_by_name)
		VALUES ($1::uuid, $2, $3, $4::jsonb, $5, NULLIF($6,'')::uuid, $7, NULLIF($6,'')::uuid, $7) RETURNING id::text`, org, in.Name, in.Description, string(spec), status, p.UserID, p.Username).Scan(&id); err != nil {
		writeErr(w, 409, "rule could not be created (duplicate name?)")
		return
	}
	s.audit(r, "rule.create", "rule", id, nil, map[string]any{"name": in.Name, "status": status, "spec": in.Spec}, in.Reason)
	s.rules.Resync()
	code := 201
	if status == "pending" {
		code = 202
	}
	writeJSON(w, code, map[string]any{"id": id, "name": in.Name, "status": status, "version": 1})
}

func (s *Server) updateRule(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.rulesAvailable(w) {
		return
	}
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var in struct {
		Name        *string     `json:"name"`
		Description *string     `json:"description"`
		Spec        *rules.Spec `json:"spec"`
		Enabled     *bool       `json:"enabled"`
		Reason      string      `json:"reason"`
	}
	if err := decodeJSON(r, &in, 256<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var name, desc, status string
	var specRaw []byte
	var enabled bool
	var version int
	if err := s.pool.QueryRow(r.Context(), `SELECT name, description, spec, enabled, status, version FROM core.rule WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, id, org).Scan(&name, &desc, &specRaw, &enabled, &status, &version); err != nil {
		writeErr(w, 404, "rule not found")
		return
	}
	old := map[string]any{"name": name, "description": desc, "spec": json.RawMessage(specRaw), "enabled": enabled, "status": status, "version": version}
	var spec rules.Spec
	_ = json.Unmarshal(specRaw, &spec)
	specChanged := false
	if in.Spec != nil {
		if !s.checkSpec(w, r, org, *in.Spec) {
			return
		}
		nb, _ := json.Marshal(*in.Spec)
		ob, _ := json.Marshal(spec)
		specChanged = string(nb) != string(ob)
		spec = *in.Spec
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" || len(n) > 64 {
			writeErr(w, 400, "name required (max 64)")
			return
		}
		name = n
	}
	if in.Description != nil {
		if len(*in.Description) > 1000 {
			writeErr(w, 400, "description max 1000 characters")
			return
		}
		desc = *in.Description
	}
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	fourEyes := s.iam != nil && s.iam.Policy(r.Context(), org).FourEyesCommands
	if specChanged {
		version++
		if spec.HasCommands() && fourEyes {
			status = "pending"
		} else {
			status = "active"
		}
	} else if status == "rejected" && in.Enabled != nil && *in.Enabled {
		writeErr(w, 409, "a rejected rule must be edited before it can be enabled")
		return
	}
	specJSON, _ := json.Marshal(spec)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.rule SET name = $2, description = $3, spec = $4::jsonb, enabled = $5, status = $6, version = $7, updated_by = NULLIF($8,'')::uuid, updated_by_name = $9, updated_at = now(),
		approved_by = CASE WHEN $6 = 'pending' THEN NULL ELSE approved_by END, approved_by_name = CASE WHEN $6 = 'pending' THEN NULL ELSE approved_by_name END, approved_at = CASE WHEN $6 = 'pending' THEN NULL ELSE approved_at END
		WHERE id = $1::uuid`, id, name, desc, string(specJSON), enabled, status, version, p.UserID, p.Username); err != nil {
		writeErr(w, 409, "rule could not be updated (duplicate name?)")
		return
	}
	if !enabled || status != "active" || specChanged {
		s.rules.Retire(r.Context(), id)
	}
	s.rules.Resync()
	s.audit(r, "rule.update", "rule", id, old, map[string]any{"name": name, "description": desc, "spec": spec, "enabled": enabled, "status": status, "version": version}, in.Reason)
	writeJSON(w, 200, map[string]any{"id": id, "name": name, "status": status, "version": version, "enabled": enabled})
}

func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.rulesAvailable(w) {
		return
	}
	id := r.PathValue("id")
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	var name string
	if err := s.pool.QueryRow(r.Context(), `UPDATE core.rule SET revoked_at = now(), enabled = false WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL RETURNING name`, id, org).Scan(&name); err != nil {
		writeErr(w, 404, "rule not found")
		return
	}
	s.rules.Retire(r.Context(), id)
	s.rules.Resync()
	s.audit(r, "rule.revoke", "rule", id, map[string]any{"name": name}, nil, reason)
	writeJSON(w, 200, map[string]any{"id": id, "revoked": true})
}

func (s *Server) decideRule(w http.ResponseWriter, r *http.Request, approve bool) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.rulesAvailable(w) {
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
	var name, status, editorID, editorName string
	if err := s.pool.QueryRow(r.Context(), `SELECT name, status, COALESCE(updated_by::text, ''), updated_by_name FROM core.rule WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, id, org).Scan(&name, &status, &editorID, &editorName); err != nil {
		writeErr(w, 404, "rule not found")
		return
	}
	if status != "pending" {
		writeErr(w, 409, "rule is not pending approval (status "+status+")")
		return
	}
	if approve && ((editorID != "" && editorID == p.UserID) || (editorID == "" && editorName == p.Username)) {
		writeErr(w, 403, "four eyes: a different user must approve this rule")
		return
	}
	next, action := "active", "rule.approve"
	if !approve {
		next, action = "rejected", "rule.reject"
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.rule SET status = $2, approved_by = NULLIF($3,'')::uuid, approved_by_name = $4, approved_at = now() WHERE id = $1::uuid`, id, next, p.UserID, p.Username); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	s.rules.Resync()
	s.audit(r, action, "rule", id, map[string]any{"name": name, "status": "pending", "edited_by": editorName}, map[string]any{"status": next, "decided_by": p.Username}, in.Reason)
	writeJSON(w, 200, map[string]any{"id": id, "status": next})
}

func (s *Server) approveRule(w http.ResponseWriter, r *http.Request) { s.decideRule(w, r, true) }
func (s *Server) rejectRule(w http.ResponseWriter, r *http.Request)  { s.decideRule(w, r, false) }

func (s *Server) testRule(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.rulesAvailable(w) {
		return
	}
	var in struct {
		Spec   rules.Spec `json:"spec"`
		RuleID string     `json:"rule_id"`
	}
	if err := decodeJSON(r, &in, 256<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if _, err := rules.Compile(in.Spec); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.rules.CheckRefs(r.Context(), org, in.Spec); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.RuleID != "" {
		var n int
		if err := s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.rule WHERE id = $1::uuid AND org_id = $2::uuid`, in.RuleID, org).Scan(&n); err != nil || n == 0 {
			in.RuleID = ""
		}
	}
	ev, err := s.rules.Evaluate(r.Context(), in.Spec, in.RuleID)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, ev)
}

func (s *Server) listRuleRuns(w http.ResponseWriter, r *http.Request) {
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
	ruleID := r.URL.Query().Get("rule_id")
	rows, err := s.pool.Query(r.Context(), `SELECT rr.id::text, rr.rule_id::text, r.name, rr.rule_version, rr.fired_at, rr.event, rr.inputs, rr.actions, rr.ok
		FROM core.rule_run rr JOIN core.rule r ON r.id = rr.rule_id WHERE rr.org_id = $1::uuid AND ($2 = '' OR rr.rule_id = NULLIF($2,'')::uuid) ORDER BY rr.fired_at DESC LIMIT $3`, org, ruleID, limit)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, rid, name, event string
		var version int
		var fired time.Time
		var inputs, actions json.RawMessage
		var okRun bool
		if rows.Scan(&id, &rid, &name, &version, &fired, &event, &inputs, &actions, &okRun) != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "rule_id": rid, "rule": name, "version": version, "fired_at": fired, "event": event, "inputs": inputs, "actions": actions, "ok": okRun})
	}
	writeJSON(w, 200, out)
}

// RunCommand executes a device command on behalf of an automation rule (rules.Commander). The rule itself went
// through the four-eyes approval when the policy requires it, so the command is dispatched at once; it is recorded
// in core.device_command with the rule as issuer and in the audit trail as a system action.
func (s *Server) RunCommand(ctx context.Context, c rules.CommandRequest) (rules.CommandResult, error) {
	var ext, transport string
	var revoked *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT external_id, transport, revoked_at FROM core.device WHERE id = $1::uuid AND org_id = $2::uuid`, c.DeviceID, c.OrgID).Scan(&ext, &transport, &revoked); err != nil {
		return rules.CommandResult{}, errors.New("device not found")
	}
	if revoked != nil {
		return rules.CommandResult{}, errors.New("device is revoked")
	}
	if len(c.Args) == 0 {
		c.Args = json.RawMessage(`{}`)
	}
	mbOp, opcOp, _, err := s.prepareConnector(ctx, c.DeviceID, transport, c.Method, c.Args)
	if err != nil {
		return rules.CommandResult{}, err
	}
	exp := time.Now().Add(5 * time.Minute).UTC()
	issuer, reason := "rule:"+c.RuleName, "automation rule "+c.RuleName
	if c.Issuer != "" {
		issuer, reason = c.Issuer, c.Reason
	}
	var cid string
	if err := s.pool.QueryRow(ctx, `INSERT INTO core.device_command (org_id, device_id, method, args, expires_at, issued_by_name, reason, status, rule_id, flow_id)
		VALUES ($1::uuid, $2::uuid, $3, $4::jsonb, $5, $6, $7, 'queued', NULLIF($8, '')::uuid, NULLIF($9, '')::uuid) RETURNING id::text`, c.OrgID, c.DeviceID, c.Method, string(c.Args), exp, issuer, reason, c.RuleID, c.FlowID).Scan(&cid); err != nil {
		return rules.CommandResult{}, errors.New("command could not be recorded")
	}
	status, result, cmdErr := s.dispatchCommand(ctx, cmdExec{org: c.OrgID, deviceID: c.DeviceID, ext: ext, transport: transport, cid: cid, method: c.Method, args: c.Args, exp: exp, mbOp: mbOp, opcOp: opcOp})
	msg, _ := json.Marshal(map[string]any{"id": cid, "m": c.Method, "a": c.Args, "exp": exp.Unix()})
	_, _ = s.pool.Exec(ctx, `INSERT INTO core.raw_message (org_id, device_id, connector, direction, topic, payload, payload_json, decode_status) VALUES ($1::uuid, $2::uuid, 'mqtt', 'out', $3, $4, $4::jsonb, 'ok')`, c.OrgID, c.DeviceID, "d/"+ext+"/cmd", msg)
	e := audit.Entry{OrgID: c.OrgID, ActorType: "system", ActorName: issuer, Action: "device.command", EntityType: "device", EntityID: c.DeviceID,
		New: map[string]any{"command_id": cid, "method": c.Method, "args": c.Args, "status": status, "rule_id": c.RuleID, "flow_id": c.FlowID}, Reason: reason}
	if _, _, err := audit.Append(ctx, s.pool, e); err != nil {
		s.log.Error("audit append failed", "security", true, "action", "device.command", "err", err)
	}
	return rules.CommandResult{ID: cid, Status: status, Result: result, Error: cmdErr}, nil
}

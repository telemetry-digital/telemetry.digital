package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/modbus"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/opcua"
)

// Command execution and four-eyes approval (URS H-08, A-12, ADR-0008). A command is recorded first; with the
// organization policy four_eyes_commands it stays 'pending' until a different user with device.command approves
// it (then it is executed exactly as a direct command) or rejects it. Every step is in the audit trail.
//
//	POST /api/v1/devices/{id}/commands/{cid}/approve {reason}   (device.command, not the issuer)
//	POST /api/v1/devices/{id}/commands/{cid}/reject  {reason}   (device.command)
//	GET  /api/v1/settings/policy                                (config.write)  {four_eyes_commands}
//	PUT  /api/v1/settings/policy {four_eyes_commands, reason}   (user.admin)

type cmdExec struct {
	org, deviceID, ext, transport, cid, method string
	args                                       json.RawMessage
	exp                                        time.Time
	mbOp                                       *modbus.WriteOp
	opcOp                                      *opcua.WriteOp
}

// prepareConnector validates a command for connector devices (Modbus/OPC UA) before anything is recorded or sent.
func (s *Server) prepareConnector(ctx context.Context, deviceID, transport, method string, args json.RawMessage) (*modbus.WriteOp, *opcua.WriteOp, int, error) {
	switch transport {
	case "opcua":
		if s.opc == nil {
			return nil, nil, 503, errors.New("OPC UA is not available on this server")
		}
		op, err := s.opc.Prepare(ctx, deviceID, method, args)
		if err != nil {
			return nil, nil, 400, err
		}
		return nil, op, 0, nil
	case "modbus":
		if s.mb == nil {
			return nil, nil, 503, errors.New("Modbus is not available on this server")
		}
		op, err := s.mb.Prepare(ctx, deviceID, method, args)
		if err != nil {
			return nil, nil, 400, err
		}
		return op, nil, 0, nil
	}
	return nil, nil, 0, nil
}

// dispatchCommand sends or executes a recorded command and stores the outcome; MQTT devices ack later.
func (s *Server) dispatchCommand(ctx context.Context, e cmdExec) (status string, result any, cmdErr string) {
	status = "queued"
	if e.transport == "mqtt" && s.br != nil {
		msg, _ := json.Marshal(map[string]any{"id": e.cid, "m": e.method, "a": e.args, "exp": e.exp.Unix()})
		// ThingsBoard-style devices receive the same command as an RPC request (only the subscribed dialect arrives)
		if s.ing != nil {
			n := s.ing.RegisterRPC(e.deviceID, e.cid)
			rpc, _ := json.Marshal(map[string]any{"method": e.method, "params": e.args})
			_ = s.br.Publish("d/"+e.ext+"/tb/rpc/request/"+strconv.FormatInt(n, 10), rpc, false)
		}
		if err := s.br.Publish("d/"+e.ext+"/cmd", msg, false); err == nil {
			status = "sent"
			_, _ = s.pool.Exec(ctx, `UPDATE core.device_command SET status = 'sent', sent_at = now() WHERE id = $1::uuid`, e.cid)
		} else {
			s.log.Warn("command publish failed", "device", e.ext, "err", err)
		}
	}
	finish := func(res map[string]any, err error) {
		if err != nil {
			status, cmdErr = "failed", err.Error()
			_, _ = s.pool.Exec(ctx, `UPDATE core.device_command SET status = 'failed', sent_at = now(), acked_at = now(), error = $2 WHERE id = $1::uuid`, e.cid, cmdErr)
			return
		}
		status, result = "acked", res
		b, _ := json.Marshal(res)
		_, _ = s.pool.Exec(ctx, `UPDATE core.device_command SET status = 'acked', sent_at = now(), acked_at = now(), result = $2::jsonb WHERE id = $1::uuid`, e.cid, string(b))
	}
	if e.opcOp != nil {
		finish(s.opc.Do(ctx, e.opcOp))
	}
	if e.mbOp != nil {
		finish(s.mb.Do(ctx, e.mbOp))
	}
	if s.live != nil {
		s.live.Command(e.org, e.deviceID, e.cid, e.method, status, map[string]any{"result": result, "error": cmdErr})
	}
	return status, result, cmdErr
}

type pendingCmd struct {
	ext, transport, method, status, issuedBy, issuedName string
	args                                                 json.RawMessage
	exp                                                  *time.Time
}

func (s *Server) loadPending(w http.ResponseWriter, r *http.Request, org, deviceID, cid string) (*pendingCmd, bool) {
	var c pendingCmd
	err := s.pool.QueryRow(r.Context(), `SELECT d.external_id, d.transport, c.method, c.args, c.status, COALESCE(c.issued_by::text, ''), c.issued_by_name, c.expires_at
		FROM core.device_command c JOIN core.device d ON d.id = c.device_id
		WHERE c.id = $1::uuid AND c.device_id = $2::uuid AND c.org_id = $3::uuid`, cid, deviceID, org).
		Scan(&c.ext, &c.transport, &c.method, &c.args, &c.status, &c.issuedBy, &c.issuedName, &c.exp)
	if err != nil {
		writeErr(w, 404, "command not found")
		return nil, false
	}
	if c.status != "pending" {
		writeErr(w, 409, "command is not pending (status "+c.status+")")
		return nil, false
	}
	if c.exp != nil && c.exp.Before(time.Now()) {
		_, _ = s.pool.Exec(r.Context(), `UPDATE core.device_command SET status = 'expired' WHERE id = $1::uuid AND status = 'pending'`, cid)
		writeErr(w, 409, "command expired before approval")
		return nil, false
	}
	return &c, true
}

// POST /api/v1/devices/{id}/commands/{cid}/approve
func (s *Server) approveCommand(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	id, cid := r.PathValue("id"), r.PathValue("cid")
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
	c, ok := s.loadPending(w, r, org, id, cid)
	if !ok {
		return
	}
	if (c.issuedBy != "" && c.issuedBy == p.UserID) || (c.issuedBy == "" && c.issuedName == p.Username) {
		writeErr(w, 403, "four eyes: a different user must approve this command")
		return
	}
	mbOp, opcOp, code, err := s.prepareConnector(r.Context(), id, c.transport, c.method, c.args)
	if err != nil {
		writeErr(w, code, err.Error())
		return
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.device_command SET status = 'queued', approved_by = NULLIF($2,'')::uuid, approved_by_name = $3, approved_at = now() WHERE id = $1::uuid AND status = 'pending'`, cid, p.UserID, p.Username); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	exp := time.Now().Add(5 * time.Minute)
	if c.exp != nil {
		exp = *c.exp
	}
	status, result, cmdErr := s.dispatchCommand(r.Context(), cmdExec{org: org, deviceID: id, ext: c.ext, transport: c.transport, cid: cid, method: c.method, args: c.args, exp: exp, mbOp: mbOp, opcOp: opcOp})
	s.audit(r, "command.approve", "device", id, map[string]any{"command_id": cid, "status": "pending", "issued_by": c.issuedName}, map[string]any{"command_id": cid, "method": c.method, "args": c.args, "status": status, "approved_by": p.Username}, in.Reason)
	writeJSON(w, 200, map[string]any{"id": cid, "method": c.method, "status": status, "result": result, "error": cmdErr})
}

// POST /api/v1/devices/{id}/commands/{cid}/reject
func (s *Server) rejectCommand(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	id, cid := r.PathValue("id"), r.PathValue("cid")
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
	c, ok := s.loadPending(w, r, org, id, cid)
	if !ok {
		return
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.device_command SET status = 'rejected', error = $2, acked_at = now(), approved_by = NULLIF($3,'')::uuid, approved_by_name = $4, approved_at = now() WHERE id = $1::uuid AND status = 'pending'`, cid, "rejected: "+in.Reason, p.UserID, p.Username); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	if s.live != nil {
		s.live.Command(org, id, cid, c.method, "rejected", map[string]any{"error": "rejected: " + in.Reason})
	}
	s.audit(r, "command.reject", "device", id, map[string]any{"command_id": cid, "status": "pending", "issued_by": c.issuedName}, map[string]any{"command_id": cid, "method": c.method, "status": "rejected", "rejected_by": p.Username}, in.Reason)
	writeJSON(w, 200, map[string]any{"id": cid, "method": c.method, "status": "rejected"})
}

// GET /api/v1/settings/policy
func (s *Server) getPolicy(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	pol := s.iam.Policy(r.Context(), org)
	writeJSON(w, 200, map[string]any{"four_eyes_commands": pol.FourEyesCommands, "four_eyes_limits": pol.FourEyesLimits, "four_eyes_assignments": pol.FourEyesAssignments, "mfa_required_all": pol.MFARequiredAll, "mfa_required_roles": pol.MFARequiredRoles, "password_min_length": pol.PasswordMinLength})
}

// PUT /api/v1/settings/policy {four_eyes_commands?, four_eyes_limits?, four_eyes_assignments?, reason}
func (s *Server) putPolicy(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		FourEyes    *bool  `json:"four_eyes_commands"`
		Limits      *bool  `json:"four_eyes_limits"`
		Assignments *bool  `json:"four_eyes_assignments"`
		Reason      string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	set := map[string]any{}
	if in.FourEyes != nil {
		set["four_eyes_commands"] = *in.FourEyes
	}
	if in.Limits != nil {
		set["four_eyes_limits"] = *in.Limits
	}
	if in.Assignments != nil {
		set["four_eyes_assignments"] = *in.Assignments
	}
	if len(set) == 0 {
		writeErr(w, 400, "four_eyes_commands, four_eyes_limits or four_eyes_assignments required")
		return
	}
	old := s.iam.Policy(r.Context(), org)
	patch, _ := json.Marshal(set)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.organization SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), '{security}', COALESCE(settings->'security', '{}'::jsonb) || $2::jsonb) WHERE id = $1::uuid`, org, string(patch)); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	s.audit(r, "organization.policy", "organization", org, map[string]any{"four_eyes_commands": old.FourEyesCommands, "four_eyes_limits": old.FourEyesLimits, "four_eyes_assignments": old.FourEyesAssignments}, set, in.Reason)
	pol := s.iam.Policy(r.Context(), org)
	writeJSON(w, 200, map[string]any{"four_eyes_commands": pol.FourEyesCommands, "four_eyes_limits": pol.FourEyesLimits, "four_eyes_assignments": pol.FourEyesAssignments})
}

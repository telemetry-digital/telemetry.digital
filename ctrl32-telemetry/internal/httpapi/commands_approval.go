package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/modbus"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/opcua"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/s7"
)

// Command execution and four-eyes approval (URS H-08, A-12, ADR-0008). A command is recorded first; with the
// organization policy four_eyes_commands it stays 'pending' until a different user with device.command approves
// it (then it is executed exactly as a direct command) or rejects it. Every step is in the audit trail.
//
//	POST /api/v1/devices/{id}/commands/{cid}/approve {reason}   (device.command, not the issuer)
//	POST /api/v1/devices/{id}/commands/{cid}/reject  {reason}   (device.command)
//	GET  /api/v1/settings/policy                                (config.write)  {four_eyes_commands}
//	PUT  /api/v1/settings/policy {four_eyes_commands, …, remember_device_days, reason}   (user.admin)

type cmdExec struct {
	org, deviceID, ext, transport, cid, method string
	args                                       json.RawMessage
	exp                                        time.Time
	op                                         connOp
}

// connOp is a validated write for a connector device (Modbus, OPC UA or S7); at most one field is set.
type connOp struct {
	mb  *modbus.WriteOp
	opc *opcua.WriteOp
	s7  *s7.WriteOp
}

// prepareConnector validates a command for connector devices (Modbus/OPC UA/S7) before anything is recorded or sent.
func (s *Server) prepareConnector(ctx context.Context, deviceID, transport, method string, args json.RawMessage) (connOp, int, error) {
	switch transport {
	case "opcua":
		if s.opc == nil {
			return connOp{}, 503, errors.New("OPC UA is not available on this server")
		}
		op, err := s.opc.Prepare(ctx, deviceID, method, args)
		if err != nil {
			return connOp{}, 400, err
		}
		return connOp{opc: op}, 0, nil
	case "modbus":
		if s.mb == nil {
			return connOp{}, 503, errors.New("Modbus is not available on this server")
		}
		op, err := s.mb.Prepare(ctx, deviceID, method, args)
		if err != nil {
			return connOp{}, 400, err
		}
		return connOp{mb: op}, 0, nil
	case "s7":
		if s.s7 == nil {
			return connOp{}, 503, errors.New("S7 is not available on this server")
		}
		op, err := s.s7.Prepare(ctx, deviceID, method, args)
		if err != nil {
			return connOp{}, 400, err
		}
		return connOp{s7: op}, 0, nil
	}
	return connOp{}, 0, nil
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
	if e.transport == "mqtt_discovery" {
		if s.smart == nil {
			status, cmdErr = "failed", "smart home is not available"
		} else {
			var res any
			status, res, cmdErr = s.smart.Dispatch(ctx, e.deviceID, e.cid, e.method, e.args)
			result = res
		}
		b, _ := json.Marshal(result)
		switch status {
		case "sent":
			_, _ = s.pool.Exec(ctx, `UPDATE core.device_command SET status = 'sent', sent_at = now(), result = $2::jsonb WHERE id = $1::uuid AND status = 'queued'`, e.cid, string(b))
		case "acked":
			_, _ = s.pool.Exec(ctx, `UPDATE core.device_command SET status = 'acked', sent_at = now(), acked_at = now(), result = $2::jsonb WHERE id = $1::uuid`, e.cid, string(b))
		default:
			_, _ = s.pool.Exec(ctx, `UPDATE core.device_command SET status = 'failed', sent_at = now(), acked_at = now(), error = $2 WHERE id = $1::uuid`, e.cid, cmdErr)
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
	if e.op.opc != nil {
		finish(s.opc.Do(ctx, e.op.opc))
	}
	if e.op.mb != nil {
		finish(s.mb.Do(ctx, e.op.mb))
	}
	if e.op.s7 != nil {
		finish(s.s7.Do(ctx, e.op.s7))
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
	op, code, err := s.prepareConnector(r.Context(), id, c.transport, c.method, c.args)
	if err != nil {
		writeErr(w, code, err.Error())
		return
	}
	// The decision is taken by the database: only the request whose update changes the pending row goes on. Two people
	// deciding at the same moment — one approves, one rejects — can never both win, and a command that was rejected (or
	// approved by somebody else, or expired) in between is not sent.
	tag, err := s.pool.Exec(r.Context(), `UPDATE core.device_command SET status = 'queued', approved_by = NULLIF($2,'')::uuid, approved_by_name = $3, approved_at = now()
		WHERE id = $1::uuid AND status = 'pending' AND (expires_at IS NULL OR expires_at > now())`, cid, p.UserID, p.Username)
	if err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	if tag.RowsAffected() != 1 {
		writeErr(w, 409, "the command is not waiting for approval any more")
		return
	}
	exp := time.Now().Add(5 * time.Minute)
	if c.exp != nil {
		exp = *c.exp
	}
	status, result, cmdErr := s.dispatchCommand(r.Context(), cmdExec{org: org, deviceID: id, ext: c.ext, transport: c.transport, cid: cid, method: c.method, args: c.args, exp: exp, op: op})
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
	tag, err := s.pool.Exec(r.Context(), `UPDATE core.device_command SET status = 'rejected', error = $2, acked_at = now(), approved_by = NULLIF($3,'')::uuid, approved_by_name = $4, approved_at = now() WHERE id = $1::uuid AND status = 'pending'`, cid, "rejected: "+in.Reason, p.UserID, p.Username)
	if err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	if tag.RowsAffected() != 1 { // approved or rejected by somebody else in between: nothing is recorded as rejected
		writeErr(w, 409, "the command is not waiting for approval any more")
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
	writeJSON(w, 200, map[string]any{"four_eyes_commands": pol.FourEyesCommands, "four_eyes_limits": pol.FourEyesLimits, "four_eyes_assignments": pol.FourEyesAssignments, "four_eyes_vpn": pol.FourEyesVPN, "four_eyes_alarms": pol.FourEyesAlarms, "four_eyes_attributes": pol.FourEyesAttributes, "four_eyes_cameras": pol.FourEyesCameras, "four_eyes_displays": pol.FourEyesDisplays, "four_eyes_minutes": pol.SecondPersonMinutes(), "log_operator_actions": pol.LogOperatorActions, "operator_log_days": pol.OperatorLogDays, "scada_users_required": pol.ScadaUsersRequired, "scada_idle_minutes": pol.ScadaIdle(), "mfa_required_all": pol.MFARequiredAll, "mfa_required_roles": pol.MFARequiredRoles, "password_min_length": pol.PasswordMinLength,
		"session_idle_minutes": pol.SessionIdleMinutes, "session_absolute_hours": pol.SessionAbsoluteHours, "remember_device_days": pol.RememberDeviceDays})
}

// PUT /api/v1/settings/policy {four_eyes_commands?, four_eyes_limits?, four_eyes_assignments?, four_eyes_vpn?, remember_device_days?, reason}
func (s *Server) putPolicy(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		FourEyes    *bool  `json:"four_eyes_commands"`
		Limits      *bool  `json:"four_eyes_limits"`
		Assignments *bool  `json:"four_eyes_assignments"`
		VPN         *bool  `json:"four_eyes_vpn"`
		LogActions  *bool  `json:"log_operator_actions"`
		Alarms      *bool  `json:"four_eyes_alarms"`
		Attributes  *bool  `json:"four_eyes_attributes"`
		Cameras     *bool  `json:"four_eyes_cameras"`
		Displays    *bool  `json:"four_eyes_displays"`
		Minutes     *int   `json:"four_eyes_minutes"`
		Remember    *int   `json:"remember_device_days"`
		LogDays     *int   `json:"operator_log_days"`
		ScadaReq    *bool  `json:"scada_users_required"`
		ScadaIdle   *int   `json:"scada_idle_minutes"`
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
	if in.VPN != nil {
		set["four_eyes_vpn"] = *in.VPN
	}
	for k, v := range map[string]*bool{"four_eyes_alarms": in.Alarms, "four_eyes_attributes": in.Attributes, "four_eyes_cameras": in.Cameras, "four_eyes_displays": in.Displays} {
		if v != nil {
			set[k] = *v
		}
	}
	if in.Minutes != nil {
		if *in.Minutes < iam.MinFourEyesMinutes || *in.Minutes > iam.MaxFourEyesMinutes {
			writeErr(w, 400, fmt.Sprintf("four_eyes_minutes must be %d to %d", iam.MinFourEyesMinutes, iam.MaxFourEyesMinutes))
			return
		}
		set["four_eyes_minutes"] = *in.Minutes
	}
	if in.LogActions != nil {
		set["log_operator_actions"] = *in.LogActions
	}
	if in.Remember != nil {
		if *in.Remember < 0 || *in.Remember > iam.MaxRememberDeviceDays {
			writeErr(w, 400, fmt.Sprintf("remember_device_days must be 0 to %d", iam.MaxRememberDeviceDays))
			return
		}
		set["remember_device_days"] = *in.Remember
	}
	if in.LogDays != nil {
		if *in.LogDays != 0 && (*in.LogDays < iam.MinOperatorLogDays || *in.LogDays > iam.MaxOperatorLogDays) {
			writeErr(w, 400, fmt.Sprintf("operator_log_days must be 0 (keep for good) or %d to %d", iam.MinOperatorLogDays, iam.MaxOperatorLogDays))
			return
		}
		set["operator_log_days"] = *in.LogDays
	}
	if in.ScadaReq != nil {
		set["scada_users_required"] = *in.ScadaReq
	}
	if in.ScadaIdle != nil {
		if *in.ScadaIdle < iam.MinScadaIdleMinutes || *in.ScadaIdle > iam.MaxScadaIdleMinutes {
			writeErr(w, 400, fmt.Sprintf("scada_idle_minutes must be %d to %d", iam.MinScadaIdleMinutes, iam.MaxScadaIdleMinutes))
			return
		}
		set["scada_idle_minutes"] = *in.ScadaIdle
	}
	if len(set) == 0 {
		writeErr(w, 400, "a four_eyes_… switch, four_eyes_minutes, log_operator_actions, operator_log_days, scada_users_required, scada_idle_minutes or remember_device_days required")
		return
	}
	old := s.iam.Policy(r.Context(), org)
	patch, _ := json.Marshal(set)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.organization SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), '{security}', COALESCE(settings->'security', '{}'::jsonb) || $2::jsonb) WHERE id = $1::uuid`, org, string(patch)); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	s.audit(r, "organization.policy", "organization", org, map[string]any{"four_eyes_commands": old.FourEyesCommands, "four_eyes_limits": old.FourEyesLimits, "four_eyes_assignments": old.FourEyesAssignments, "four_eyes_vpn": old.FourEyesVPN, "four_eyes_alarms": old.FourEyesAlarms, "four_eyes_attributes": old.FourEyesAttributes, "four_eyes_cameras": old.FourEyesCameras, "four_eyes_displays": old.FourEyesDisplays, "four_eyes_minutes": old.SecondPersonMinutes(), "log_operator_actions": old.LogOperatorActions, "operator_log_days": old.OperatorLogDays, "scada_users_required": old.ScadaUsersRequired, "scada_idle_minutes": old.ScadaIdle(), "remember_device_days": old.RememberDeviceDays}, set, in.Reason)
	opLogCache.Delete(org)
	pol := s.iam.Policy(r.Context(), org)
	writeJSON(w, 200, map[string]any{"four_eyes_commands": pol.FourEyesCommands, "four_eyes_limits": pol.FourEyesLimits, "four_eyes_assignments": pol.FourEyesAssignments, "four_eyes_vpn": pol.FourEyesVPN, "four_eyes_alarms": pol.FourEyesAlarms, "four_eyes_attributes": pol.FourEyesAttributes, "four_eyes_cameras": pol.FourEyesCameras, "four_eyes_displays": pol.FourEyesDisplays, "four_eyes_minutes": pol.SecondPersonMinutes(), "log_operator_actions": pol.LogOperatorActions, "operator_log_days": pol.OperatorLogDays, "scada_users_required": pol.ScadaUsersRequired, "scada_idle_minutes": pol.ScadaIdle(), "remember_device_days": pol.RememberDeviceDays})
}

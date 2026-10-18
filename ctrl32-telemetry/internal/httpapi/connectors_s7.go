package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/s7"
)

// Siemens S7 connectors (0.83.0): the server reads tags of an S7-300/400/1200/1500, ET 200SP or LOGO! over S7comm
// and feeds virtual devices (transport s7). Writes are commands on those devices:
//
//	POST /api/v1/connectors            {kind:"s7", name, settings, reason}            (config.write)
//	PUT  /api/v1/connectors/{id}       {settings, reason}                             (config.write) replace settings, poller restarts
//	POST /api/v1/connectors/test       {kind:"s7", settings}                          (config.write) connect, identify the CPU, read once
//	POST /api/v1/devices/{id}/commands {method:"write", args:{key, value}, reason}    (device.command) validated against the
//	                                                                                 tag (writable, type, min, max), executed,
//	                                                                                 read back and acked; every step in the audit trail

func s7Summary(st s7.Settings) map[string]any {
	writable := 0
	for _, t := range st.Tags {
		if t.Writable {
			writable++
		}
	}
	return map[string]any{"host": st.Host, "port": st.Port, "plc": st.PLC, "rack": *st.Rack, "slot": *st.Slot, "conn_type": st.ConnType,
		"local_tsap": st.LocalTSAP, "remote_tsap": st.RemoteTSAP, "tags": len(st.Tags), "writable": writable, "interval_s": st.IntervalS}
}

func (s *Server) createS7Connector(w http.ResponseWriter, r *http.Request, org, name string, raw json.RawMessage, reason string) {
	st, err := s7.ValidateSettings(raw)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	settings, _ := json.Marshal(st)
	secret, err := iam.RandomToken(32) // the schema requires one; S7 connectors are outbound and never use it
	if err != nil {
		writeErr(w, 500, "secret generation failed")
		return
	}
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.connector (org_id, kind, name, secret_hash, settings) VALUES ($1::uuid, 's7', $2, $3, $4::jsonb) RETURNING id::text`, org, name, iam.HashToken(secret), string(settings)).Scan(&id); err != nil {
		writeErr(w, 409, "connector could not be created (duplicate name?)")
		return
	}
	sum := s7Summary(st)
	sum["kind"], sum["name"] = "s7", name
	s.audit(r, "connector.create", "connector", id, nil, sum, reason)
	if s.s7 != nil {
		s.s7.Resync()
	}
	writeJSON(w, 201, map[string]any{"id": id, "kind": "s7", "name": name, "settings": st})
}

func (s *Server) updateS7Connector(w http.ResponseWriter, r *http.Request, org, id, oldRaw string, raw json.RawMessage, reason string) {
	st, err := s7.ValidateSettings(raw)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	settings, _ := json.Marshal(st)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.connector SET settings = $3::jsonb WHERE id = $1::uuid AND org_id = $2::uuid`, id, org, string(settings)); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	var before map[string]any
	if old, err := s7.ValidateSettings(json.RawMessage(oldRaw)); err == nil {
		before = s7Summary(old)
	}
	s.audit(r, "connector.update", "connector", id, before, s7Summary(st), reason)
	if s.s7 != nil {
		s.s7.Resync()
	}
	writeJSON(w, 200, map[string]any{"id": id, "kind": "s7", "settings": st})
}

func (s *Server) testS7Connector(w http.ResponseWriter, r *http.Request, raw json.RawMessage) {
	st, err := s7.ValidateSettings(raw)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if s.s7 == nil {
		writeErr(w, 503, "S7 is not available on this server")
		return
	}
	res, err := s.s7.Test(r.Context(), st)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "values": res.Values, "bad_nodes": res.Bad, "pdu": res.PDU, "cpu": res.CPU, "addr": st.Host, "port": st.Port, "plc": st.PLC, "rack": *st.Rack, "slot": *st.Slot})
}

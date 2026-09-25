package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/modbus"
)

// Modbus TCP connectors (URS A-11/A-12, 0.20.0): the server polls holding/input registers, coils and discrete
// inputs of one unit and feeds a virtual device (transport modbus_tcp). Writes are commands on that device:
//
//	POST /api/v1/connectors            {kind:"modbus_tcp", name, settings, reason}   (config.write)
//	PUT  /api/v1/connectors/{id}       {settings, reason}                            (config.write) replace settings, poller restarts
//	POST /api/v1/connectors/test       {settings}                                    (config.write) connect + read once, nothing stored
//	POST /api/v1/devices/{id}/commands {method:"write", args:{key, value}, reason}   (device.command) validated against the
//	                                                                                register map (writable, min, max), executed,
//	                                                                                read back and acked; every step in the audit trail

func (s *Server) createModbusConnector(w http.ResponseWriter, r *http.Request, org, name string, raw json.RawMessage, reason string) {
	raw, err := s.resolveProfile(r.Context(), org, raw)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	st, err := modbus.ValidateSettings(raw)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	settings, _ := json.Marshal(st)
	secret, err := iam.RandomToken(32) // the schema requires one; Modbus connectors are outbound and never use it
	if err != nil {
		writeErr(w, 500, "secret generation failed")
		return
	}
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.connector (org_id, kind, name, secret_hash, settings) VALUES ($1::uuid, 'modbus_tcp', $2, $3, $4::jsonb) RETURNING id::text`, org, name, iam.HashToken(secret), string(settings)).Scan(&id); err != nil {
		writeErr(w, 409, "connector could not be created (duplicate name?)")
		return
	}
	writable := 0
	for _, reg := range st.Registers {
		if reg.Writable {
			writable++
		}
	}
	s.audit(r, "connector.create", "connector", id, nil, map[string]any{"kind": "modbus_tcp", "name": name, "host": st.Host, "port": st.Port, "unit_id": st.UnitID, "registers": len(st.Registers), "writable": writable, "interval_s": st.IntervalS, "profile": st.ProfileName, "profile_version": st.ProfileVersion}, reason)
	if s.mb != nil {
		s.mb.Resync()
	}
	writeJSON(w, 201, map[string]any{"id": id, "kind": "modbus_tcp", "name": name, "settings": st})
}

// updateModbusConnector replaces the settings of an existing modbus_tcp connector (called from updateConnector).
func (s *Server) updateModbusConnector(w http.ResponseWriter, r *http.Request, org, id, oldRaw string, raw json.RawMessage, reason string) {
	raw, err := s.resolveProfile(r.Context(), org, raw)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	st, err := modbus.ValidateSettings(raw)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var old modbus.Settings
	_ = json.Unmarshal([]byte(oldRaw), &old)
	settings, _ := json.Marshal(st)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.connector SET settings = $3::jsonb WHERE id = $1::uuid AND org_id = $2::uuid`, id, org, string(settings)); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	s.audit(r, "connector.update", "connector", id, map[string]any{"host": old.Host, "port": old.Port, "registers": len(old.Registers), "interval_s": old.IntervalS}, map[string]any{"host": st.Host, "port": st.Port, "registers": len(st.Registers), "interval_s": st.IntervalS}, reason)
	if s.mb != nil {
		s.mb.Resync()
	}
	writeJSON(w, 200, map[string]any{"id": id, "kind": "modbus_tcp", "settings": st})
}

// testModbusConnector connects once and reads every register (called from testConnector).
func (s *Server) testModbusConnector(w http.ResponseWriter, r *http.Request, raw json.RawMessage) {
	st, err := modbus.ValidateSettings(raw)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if s.mb == nil {
		writeErr(w, 503, "Modbus is not available on this server")
		return
	}
	values, bad, err := s.mb.Test(r.Context(), st)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "values": values, "bad_nodes": bad, "addr": st.Host, "port": st.Port, "unit_id": st.UnitID})
}

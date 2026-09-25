package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/opcua"
)

// OPC UA client connectors (URS E-*, 0.13.0): the server polls an OPC UA server and feeds a virtual device.
//
//	POST /api/v1/connectors                      {kind:"opcua", name, settings, reason}   (config.write)
//	PUT  /api/v1/connectors/{id}                 {settings, reason}                        (config.write) replace settings, poller restarts
//	POST /api/v1/connectors/test                 {settings}                                (config.write) connect + read once, nothing stored
//	GET  /api/v1/connectors/opcua/certificate    client certificate (PEM) to trust on servers that require Sign/SignAndEncrypt

func (s *Server) createOPCUAConnector(w http.ResponseWriter, r *http.Request, org, name string, raw json.RawMessage, reason string) {
	st, err := opcua.ValidateSettings(raw)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	settings, _ := json.Marshal(st)
	secret, err := iam.RandomToken(32) // the schema requires one; OPC UA connectors are outbound and never use it
	if err != nil {
		writeErr(w, 500, "secret generation failed")
		return
	}
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.connector (org_id, kind, name, secret_hash, settings) VALUES ($1::uuid, 'opcua', $2, $3, $4::jsonb) RETURNING id::text`, org, name, iam.HashToken(secret), string(settings)).Scan(&id); err != nil {
		writeErr(w, 409, "connector could not be created (duplicate name?)")
		return
	}
	s.audit(r, "connector.create", "connector", id, nil, map[string]any{"kind": "opcua", "name": name, "endpoint": st.Endpoint, "nodes": len(st.Nodes), "interval_s": st.IntervalS, "security": st.SecurityPolicy + "/" + st.SecurityMode}, reason)
	if s.opc != nil {
		s.opc.Resync()
	}
	writeJSON(w, 201, map[string]any{"id": id, "kind": "opcua", "name": name, "settings": redact(st)})
}

func redact(st opcua.Settings) opcua.Settings {
	if st.Password != "" {
		st.Password = "••••••"
	}
	return st
}

func (s *Server) updateConnector(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var in struct {
		Settings json.RawMessage `json:"settings"`
		Reason   string          `json:"reason"`
	}
	if err := decodeJSON(r, &in, 256<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var kind, oldRaw string
	if err := s.pool.QueryRow(r.Context(), `SELECT kind, settings::text FROM core.connector WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, id, org).Scan(&kind, &oldRaw); err != nil {
		writeErr(w, 404, "connector not found")
		return
	}
	if kind == "modbus_tcp" {
		s.updateModbusConnector(w, r, org, id, oldRaw, in.Settings, in.Reason)
		return
	}
	if kind != "opcua" {
		writeErr(w, 400, "only OPC UA and Modbus connectors have editable settings")
		return
	}
	st, err := opcua.ValidateSettings(in.Settings)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var old opcua.Settings
	_ = json.Unmarshal([]byte(oldRaw), &old)
	if st.Auth == "username" && st.Password == "" {
		st.Password = old.Password // blank keeps the stored password
	}
	settings, _ := json.Marshal(st)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.connector SET settings = $3::jsonb WHERE id = $1::uuid AND org_id = $2::uuid`, id, org, string(settings)); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	s.audit(r, "connector.update", "connector", id, map[string]any{"endpoint": old.Endpoint, "nodes": len(old.Nodes), "interval_s": old.IntervalS}, map[string]any{"endpoint": st.Endpoint, "nodes": len(st.Nodes), "interval_s": st.IntervalS, "security": st.SecurityPolicy + "/" + st.SecurityMode}, in.Reason)
	if s.opc != nil {
		s.opc.Resync()
	}
	writeJSON(w, 200, map[string]any{"id": id, "kind": kind, "settings": redact(st)})
}

func (s *Server) testConnector(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind     string          `json:"kind"`
		Settings json.RawMessage `json:"settings"`
		ID       string          `json:"id"` // optional: reuse the stored password when the form left it blank
	}
	if err := decodeJSON(r, &in, 256<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.Kind == "modbus_tcp" {
		s.testModbusConnector(w, r, in.Settings)
		return
	}
	st, err := opcua.ValidateSettings(in.Settings)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if st.Auth == "username" && st.Password == "" && in.ID != "" {
		org, _ := s.mustOrg(w, r)
		var oldRaw string
		if s.pool.QueryRow(r.Context(), `SELECT settings::text FROM core.connector WHERE id = $1::uuid AND org_id = $2::uuid`, in.ID, org).Scan(&oldRaw) == nil {
			var old opcua.Settings
			_ = json.Unmarshal([]byte(oldRaw), &old)
			st.Password = old.Password
		}
	}
	if s.opc == nil {
		writeErr(w, 503, "OPC UA is not available on this server")
		return
	}
	values, bad, err := s.opc.Test(r.Context(), st)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "values": values, "bad_nodes": bad, "endpoint": st.Endpoint, "security": st.SecurityPolicy + "/" + st.SecurityMode})
}

func (s *Server) opcuaCertificate(w http.ResponseWriter, r *http.Request) {
	if s.opc == nil {
		writeErr(w, 503, "OPC UA is not available on this server")
		return
	}
	pemText, err := s.opc.ClientCertificatePEM()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="ctrl32-opcua-client.crt"`)
	_, _ = w.Write([]byte(strings.TrimSpace(pemText) + "\n"))
}

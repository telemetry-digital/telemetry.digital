package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

// POST /api/v1/devices {external_id, name, transport, reason} -> device + one-time claim code (24 h)
func (s *Server) createDevice(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	var in struct {
		ExternalID string `json:"external_id"`
		Name       string `json:"name"`
		Transport  string `json:"transport"`
		Reason     string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.ExternalID = strings.TrimSpace(in.ExternalID)
	if in.ExternalID == "" || strings.ContainsAny(in.ExternalID, "/#+ ") || len(in.ExternalID) > 64 {
		writeErr(w, 400, "external_id required (no spaces or / # +, max 64)")
		return
	}
	if in.Transport == "" {
		in.Transport = "mqtt"
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason required")
		return
	}
	if in.Name == "" {
		in.Name = in.ExternalID
	}
	claim, err := iam.RandomToken(9) // 12 chars
	if err != nil {
		writeErr(w, 500, "claim generation failed")
		return
	}
	claim = strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(claim, "-", "A"), "_", "B"))
	var id string
	err = s.pool.QueryRow(r.Context(), `INSERT INTO core.device (org_id, external_id, name, transport) VALUES ($1::uuid, $2, $3, $4) RETURNING id::text`,
		p.OrgID, in.ExternalID, in.Name, in.Transport).Scan(&id)
	if err != nil {
		writeErr(w, 409, "device could not be created (duplicate external_id?)")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `INSERT INTO core.device_credential (device_id, kind, secret_hash, valid_to) VALUES ($1::uuid, 'claim', $2, now() + interval '24 hours')`, id, iam.HashToken(claim)); err != nil {
		writeErr(w, 500, "claim could not be stored")
		return
	}
	s.audit(r, "device.create", "device", id, nil, map[string]any{"external_id": in.ExternalID, "name": in.Name, "transport": in.Transport}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id, "external_id": in.ExternalID, "name": in.Name, "transport": in.Transport, "claim_code": claim, "claim_expires_in_hours": 24})
}

// POST /api/v1/devices/{id}/token {reason} -> new raw token (previous tokens revoked)
func (s *Server) rotateDeviceToken(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var in struct{ Reason string }
	if err := decodeJSON(r, &in, 1024); err != nil || strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason required")
		return
	}
	var ext string
	if err := s.pool.QueryRow(r.Context(), `SELECT external_id FROM core.device WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, id, p.OrgID).Scan(&ext); err != nil {
		writeErr(w, 404, "device not found")
		return
	}
	raw, err := issueDeviceToken(r, s, id)
	if err != nil {
		writeErr(w, 500, "token issue failed")
		return
	}
	s.audit(r, "device.token_rotate", "device", id, nil, map[string]any{"external_id": ext}, in.Reason)
	writeJSON(w, 200, map[string]any{"external_id": ext, "token": raw})
}

func issueDeviceToken(r *http.Request, s *Server, deviceID string) (string, error) {
	raw, err := iam.RandomToken(32)
	if err != nil {
		return "", err
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		return "", err
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), `UPDATE core.device_credential SET revoked_at = now() WHERE device_id = $1::uuid AND kind = 'token' AND revoked_at IS NULL`, deviceID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO core.device_credential (device_id, kind, secret_hash) VALUES ($1::uuid, 'token', $2)`, deviceID, broker.TokenHash(raw)); err != nil {
		return "", err
	}
	return raw, tx.Commit(r.Context())
}

// POST /api/v1/devices/{id}/revoke {reason}
func (s *Server) revokeDevice(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var in struct{ Reason string }
	if err := decodeJSON(r, &in, 1024); err != nil || strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason required")
		return
	}
	tag, err := s.pool.Exec(r.Context(), `UPDATE core.device SET revoked_at = now() WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, id, p.OrgID)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, 404, "device not found")
		return
	}
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.device_credential SET revoked_at = now() WHERE device_id = $1::uuid AND revoked_at IS NULL`, id)
	s.br.DisconnectDevice(id)
	s.audit(r, "device.revoke", "device", id, nil, nil, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/v1/devices/{id}/messages?limit=100 — message viewer v0 (raw messages, newest first)
func (s *Server) deviceMessages(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	orgFilter := ""
	if p != nil {
		orgFilter = p.OrgID
	}
	rows, err := s.pool.Query(r.Context(), `
		SELECT m.id::text, m.received_at, m.connector, m.direction, m.topic, encode(m.payload,'escape'), m.payload_json, m.meta, m.decode_status, m.decode_error
		FROM core.raw_message m JOIN core.device d ON d.id = m.device_id
		WHERE m.device_id = $1::uuid AND ($2 = '' OR d.org_id = $2::uuid)
		ORDER BY m.received_at DESC LIMIT $3`, id, orgFilter, limit)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var mid, connector, direction, status string
		var topic, payloadText, decodeErr *string
		var pj, meta json.RawMessage
		var at time.Time
		if err := rows.Scan(&mid, &at, &connector, &direction, &topic, &payloadText, &pj, &meta, &status, &decodeErr); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, map[string]any{"id": mid, "received_at": at, "connector": connector, "direction": direction, "topic": topic,
			"payload_text": payloadText, "payload_json": pj, "meta": meta, "decode_status": status, "decode_error": decodeErr})
	}
	writeJSON(w, 200, out)
}

// POST /api/v1/provision {claim, hw, fw} — unauthenticated, rate limited; exchanges a one-time claim code for a token.
func (s *Server) provision(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	body, ok := readProvisionBody(w, r)
	if !ok {
		return
	}
	if strings.Contains(string(body), "provisionDeviceKey") {
		s.provisionByKey(w, r, body) // limits failed attempts per address and key itself
		return
	}
	if !s.provisionLimiter.allow(ip) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts")
		return
	}
	var in struct{ Claim, HW, FW string }
	if err := json.Unmarshal(body, &in); err != nil {
		writeErr(w, 400, "request is not JSON")
		return
	}
	var deviceID, ext, credID string
	err := s.pool.QueryRow(r.Context(), `
		SELECT d.id::text, d.external_id, c.id::text FROM core.device_credential c JOIN core.device d ON d.id = c.device_id
		WHERE c.kind = 'claim' AND c.secret_hash = $1 AND c.used_at IS NULL AND c.revoked_at IS NULL AND (c.valid_to IS NULL OR c.valid_to > now()) AND d.revoked_at IS NULL`,
		iam.HashToken(strings.ToUpper(strings.TrimSpace(in.Claim)))).Scan(&deviceID, &ext, &credID)
	if err != nil {
		s.log.Warn("provisioning rejected", "security", true, "ip", ip)
		writeErr(w, http.StatusUnauthorized, "invalid claim code")
		return
	}
	raw, err := issueDeviceToken(r, s, deviceID)
	if err != nil {
		writeErr(w, 500, "token issue failed")
		return
	}
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.device_credential SET used_at = now() WHERE id = $1::uuid`, credID)
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.device SET status = status || jsonb_build_object('attributes', jsonb_build_object('hw', $2::text, 'fw', $3::text), 'provisioned_at', now()) WHERE id = $1::uuid`, deviceID, in.HW, in.FW)
	s.audit(r, "device.provision", "device", deviceID, nil, map[string]any{"external_id": ext, "hw": in.HW, "fw": in.FW, "ip": ip}, "")
	host := strings.TrimPrefix(strings.TrimPrefix(s.cfg.HTTP.BaseURL, "https://"), "http://")
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	mqtt := map[string]any{"host": host, "port": 8883, "tls": true}
	if s.cfg.MQTT.TLSListen == "" {
		mqtt = map[string]any{"host": host, "port": 1883, "tls": false}
	}
	writeJSON(w, 200, map[string]any{"device_id": ext, "token": raw, "mqtt": mqtt, "protocol": 1})
}

// POST /api/v1/alarms/{id}/ack {reason}

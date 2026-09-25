package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
)

// Device HTTP API (protocol §3.8, URS A-07): the same things a device does over MQTT, for devices that only have an
// HTTP client (trackers, PLC gateways, scripts). Authentication is the device token as `Authorization: Bearer …`;
// the token is the same one MQTT uses as password. Every message is stored as a raw message like the MQTT ones.

func (s *Server) deviceFromBearer(w http.ResponseWriter, r *http.Request) (*broker.Device, bool) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		w.Header().Set("WWW-Authenticate", `Bearer realm="ctrl32 device"`)
		writeErr(w, 401, "device token required: Authorization: Bearer <token>")
		return nil, false
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	var d broker.Device
	err := s.pool.QueryRow(r.Context(), `SELECT d.id::text, d.org_id::text, d.external_id, d.transport
		FROM core.device d JOIN core.organization o ON o.id = d.org_id JOIN core.device_credential c ON c.device_id = d.id
		WHERE d.revoked_at IS NULL AND o.disabled_at IS NULL AND c.kind = 'token' AND c.secret_hash = $1 AND c.revoked_at IS NULL
		  AND (c.valid_to IS NULL OR c.valid_to > now()) LIMIT 1`, broker.TokenHash(token)).Scan(&d.ID, &d.OrgID, &d.ExternalID, &d.Transport)
	if err != nil {
		s.log.Warn("device token rejected", "security", true, "remote", clientIP(r), "path", r.URL.Path)
		w.Header().Set("WWW-Authenticate", `Bearer realm="ctrl32 device"`)
		writeErr(w, 401, "invalid device token")
		return nil, false
	}
	return &d, true
}

func readDeviceBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		writeErr(w, 413, "body larger than "+strconv.FormatInt(limit, 10)+" bytes")
		return nil, false
	}
	return body, true
}

func (s *Server) deviceMeta(r *http.Request) string {
	m, _ := json.Marshal(map[string]any{"ip": clientIP(r), "ua": r.UserAgent()})
	return string(m)
}

// POST /api/v1/d/telemetry — one sample or an array, any accepted format (ctrl32, ThingsBoard, flat).
func (s *Server) devTelemetry(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.deviceFromBearer(w, r)
	if !ok {
		return
	}
	body, ok := readDeviceBody(w, r, 64<<10)
	if !ok {
		return
	}
	res := s.ing.IngestTelemetryResult(r.Context(), dev, "http", "http:/api/v1/d/telemetry", body, s.deviceMeta(r))
	out := map[string]any{"accepted": res.Observations, "status": res.Status}
	if len(res.UnknownKeys) > 0 {
		out["unknown_keys"] = res.UnknownKeys
	}
	if res.Detail != "" {
		out["error"] = res.Detail
	}
	code := 202
	if res.Status == "error" {
		code = 400
	}
	writeJSON(w, code, out)
}

// POST /api/v1/d/attributes — client attributes (fw, hw, model, ip …).
func (s *Server) devAttributes(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.deviceFromBearer(w, r)
	if !ok {
		return
	}
	body, ok := readDeviceBody(w, r, 16<<10)
	if !ok {
		return
	}
	if !json.Valid(body) || len(body) == 0 || body[0] != '{' {
		writeErr(w, 400, "attributes must be a JSON object")
		return
	}
	s.ing.HTTPAttributes(r.Context(), dev, body)
	writeJSON(w, 202, map[string]any{"ok": true})
}

// POST /api/v1/d/status — heartbeat.
func (s *Server) devStatus(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.deviceFromBearer(w, r)
	if !ok {
		return
	}
	body, ok := readDeviceBody(w, r, 4<<10)
	if !ok {
		return
	}
	if !json.Valid(body) || len(body) == 0 || body[0] != '{' {
		writeErr(w, 400, "status must be a JSON object")
		return
	}
	s.ing.HTTPStatus(r.Context(), dev, body)
	writeJSON(w, 202, map[string]any{"ok": true})
}

// GET /api/v1/d/attributes/shared — the device's configuration set by the server (the retained MQTT document).
func (s *Server) devSharedAttributes(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.deviceFromBearer(w, r)
	if !ok {
		return
	}
	var raw []byte
	if err := s.pool.QueryRow(r.Context(), `SELECT shared_attributes FROM core.device WHERE id = $1::uuid`, dev.ID).Scan(&raw); err != nil {
		writeErr(w, 500, "lookup failed")
		return
	}
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(raw)
}

// GET /api/v1/d/cmd?wait=N — queued commands for the device, marked sent when handed over; waits up to N seconds
// (max 60) for one to appear (long polling).
func (s *Server) devCommands(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.deviceFromBearer(w, r)
	if !ok {
		return
	}
	wait, _ := strconv.Atoi(r.URL.Query().Get("wait"))
	if wait < 0 {
		wait = 0
	}
	if wait > 60 {
		wait = 60
	}
	deadline := time.Now().Add(time.Duration(wait) * time.Second)
	type cmd struct {
		ID  string          `json:"id"`
		M   string          `json:"m"`
		A   json.RawMessage `json:"a"`
		Exp int64           `json:"exp"`
	}
	for {
		rows, err := s.pool.Query(r.Context(), `SELECT id::text, method, COALESCE(args, '{}'::jsonb), expires_at FROM core.device_command
			WHERE device_id = $1::uuid AND status = 'queued' AND (expires_at IS NULL OR expires_at > now()) ORDER BY issued_at LIMIT 20`, dev.ID)
		if err != nil {
			writeErr(w, 500, "query failed")
			return
		}
		list := []cmd{}
		ids := []string{}
		for rows.Next() {
			var c cmd
			var exp *time.Time
			if rows.Scan(&c.ID, &c.M, &c.A, &exp) == nil {
				if exp != nil {
					c.Exp = exp.Unix()
				}
				list = append(list, c)
				ids = append(ids, c.ID)
			}
		}
		rows.Close()
		if len(list) > 0 {
			_, _ = s.pool.Exec(r.Context(), `UPDATE core.device_command SET status = 'sent', sent_at = now() WHERE id = ANY($1::uuid[]) AND status = 'queued'`, ids)
			if s.live != nil {
				for _, c := range list {
					s.live.Command(dev.OrgID, dev.ID, c.ID, c.M, "sent", nil)
				}
			}
			writeJSON(w, 200, list)
			return
		}
		if time.Now().After(deadline) {
			writeJSON(w, 200, list)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// POST /api/v1/d/cmd/{id}/ack — {"ok": true, "r": {...}} or {"ok": false, "err": "..."}.
func (s *Server) devCommandAck(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.deviceFromBearer(w, r)
	if !ok {
		return
	}
	body, ok := readDeviceBody(w, r, 16<<10)
	if !ok {
		return
	}
	var in struct {
		OK  *bool           `json:"ok"`
		R   json.RawMessage `json:"r"`
		Err string          `json:"err"`
	}
	if err := json.Unmarshal(body, &in); err != nil || in.OK == nil {
		writeErr(w, 400, `ack must be {"ok": true|false, "r": {...}, "err": "..."}`)
		return
	}
	cid := r.PathValue("id")
	var exists bool
	_ = s.pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM core.device_command WHERE id = $1::uuid AND device_id = $2::uuid AND status IN ('queued', 'sent'))`, cid, dev.ID).Scan(&exists)
	if !exists {
		writeErr(w, 404, "no open command with this id for this device")
		return
	}
	ack := map[string]any{"id": cid, "ok": *in.OK}
	if len(in.R) > 0 && json.Valid(in.R) {
		ack["r"] = json.RawMessage(in.R)
	}
	if in.Err != "" {
		ack["err"] = in.Err
	}
	payload, _ := json.Marshal(ack)
	s.ing.HTTPCmdAck(r.Context(), dev, payload)
	status := "acked"
	if !*in.OK {
		status = "failed"
	}
	writeJSON(w, 200, map[string]any{"id": cid, "status": status})
}

// POST /api/v1/d/fota/status — {"fw_id","st","pct","ver","err"} as over MQTT.
func (s *Server) devFotaStatus(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.deviceFromBearer(w, r)
	if !ok {
		return
	}
	body, ok := readDeviceBody(w, r, 4<<10)
	if !ok {
		return
	}
	if !json.Valid(body) {
		writeErr(w, 400, "fota status must be JSON")
		return
	}
	s.ing.HTTPFotaStatus(r.Context(), dev, body)
	writeJSON(w, 202, map[string]any{"ok": true})
}

var _ = context.Background

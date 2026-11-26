package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/opcua"
)

// Connectors (URS E-*): inbound integrations authenticated by a per-connector secret. Phase 1 ships the ChirpStack
// HTTP integration: ChirpStack v4 posts every event to <base_url>/api/v1/ingest/chirpstack/{connector_id}?event=up
// with header "Authorization: Bearer <secret>". ChirpStack has already run the device-profile codec, so the
// decoded "object" is mapped to datastream keys exactly like MQTT telemetry; the raw event is kept as a record.
//
//	GET    /api/v1/connectors               (config.write)   list (no secrets)
//	POST   /api/v1/connectors               (config.write)   {kind, name, auto_create_devices, reason} -> secret shown once
//	DELETE /api/v1/connectors/{id}?reason=  (config.write)   revoke
//	POST   /api/v1/ingest/chirpstack/{id}?event=up|join|status|...   public, bearer secret, rate limited

func (s *Server) listConnectors(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT id::text, kind, name, settings, created_at, last_seen_at, messages, revoked_at FROM core.connector WHERE org_id = $1::uuid ORDER BY revoked_at NULLS FIRST, name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, kind, name string
		var settings json.RawMessage
		var created time.Time
		var seen, revoked *time.Time
		var n int64
		if err := rows.Scan(&id, &kind, &name, &settings, &created, &seen, &n, &revoked); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		row := map[string]any{"id": id, "kind": kind, "name": name, "settings": settings, "created_at": created, "last_seen_at": seen, "messages": n, "revoked_at": revoked,
			"url": s.cfg.HTTP.BaseURL + "/api/v1/ingest/chirpstack/" + id}
		if kind == "opcua" {
			row["url"] = ""
			var st opcua.Settings
			if json.Unmarshal(opcua.OpenStored(settings), &st) == nil {
				row["settings"] = redact(st)
			} else {
				row["settings"] = json.RawMessage(`{}`) // never the stored form
			}
			if s.opc != nil {
				row["status"] = s.opc.Status(id)
			}
		}
		if kind == "modbus_tcp" {
			row["url"] = ""
			if s.mb != nil {
				row["status"] = s.mb.Status(id)
			}
		}
		if kind == "s7" {
			row["url"] = ""
			if s.s7 != nil {
				row["status"] = s.s7.Status(id)
			}
		}
		out = append(out, row)
	}
	writeJSON(w, 200, out)
}

func (s *Server) createConnector(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		Kind              string          `json:"kind"`
		Name              string          `json:"name"`
		AutoCreateDevices *bool           `json:"auto_create_devices"`
		Settings          json.RawMessage `json:"settings"`
		Reason            string          `json:"reason"`
	}
	if err := decodeJSON(r, &in, 256<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.Kind == "" {
		in.Kind = "chirpstack_http"
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 64 {
		writeErr(w, 400, "name required (max 64)")
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if in.Kind == "opcua" {
		s.createOPCUAConnector(w, r, org, in.Name, in.Settings, in.Reason)
		return
	}
	if in.Kind == "modbus_tcp" {
		s.createModbusConnector(w, r, org, in.Name, in.Settings, in.Reason)
		return
	}
	if in.Kind == "s7" {
		s.createS7Connector(w, r, org, in.Name, in.Settings, in.Reason)
		return
	}
	if in.Kind != "chirpstack_http" {
		writeErr(w, 400, "kind must be chirpstack_http, opcua, modbus_tcp or s7")
		return
	}
	auto := true
	if in.AutoCreateDevices != nil {
		auto = *in.AutoCreateDevices
	}
	secret, err := iam.RandomToken(32)
	if err != nil {
		writeErr(w, 500, "secret generation failed")
		return
	}
	secret = "c32c_" + secret
	settings, _ := json.Marshal(map[string]any{"auto_create_devices": auto})
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.connector (org_id, kind, name, secret_hash, settings) VALUES ($1::uuid, $2, $3, $4, $5::jsonb) RETURNING id::text`, org, in.Kind, in.Name, iam.HashToken(secret), string(settings)).Scan(&id); err != nil {
		writeErr(w, 409, "connector could not be created (duplicate name?)")
		return
	}
	s.audit(r, "connector.create", "connector", id, nil, map[string]any{"kind": in.Kind, "name": in.Name, "auto_create_devices": auto}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id, "kind": in.Kind, "name": in.Name, "secret": secret, "url": s.cfg.HTTP.BaseURL + "/api/v1/ingest/chirpstack/" + id,
		"chirpstack": map[string]any{"integration": "HTTP", "event_endpoint_url": s.cfg.HTTP.BaseURL + "/api/v1/ingest/chirpstack/" + id, "headers": map[string]string{"Authorization": "Bearer " + secret}, "payload_encoding": "JSON"}})
}

func (s *Server) revokeConnector(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	tag, err := s.pool.Exec(r.Context(), `UPDATE core.connector SET revoked_at = now() WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, id, org)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, 404, "connector not found")
		return
	}
	s.audit(r, "connector.revoke", "connector", id, nil, nil, reason)
	if s.opc != nil {
		s.opc.Resync()
	}
	if s.mb != nil {
		s.mb.Resync()
	}
	if s.s7 != nil {
		s.s7.Resync()
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// chirpstackUplink is the subset of the ChirpStack v4 "up" event we use.
type chirpstackUplink struct {
	DeduplicationID string    `json:"deduplicationId"`
	Time            time.Time `json:"time"`
	DeviceInfo      struct {
		DevEUI     string `json:"devEui"`
		DeviceName string `json:"deviceName"`
	} `json:"deviceInfo"`
	FCnt   int64           `json:"fCnt"`
	FPort  int             `json:"fPort"`
	Data   string          `json:"data"`
	Object json.RawMessage `json:"object"`
	RxInfo []struct {
		GatewayID string  `json:"gatewayId"`
		RSSI      float64 `json:"rssi"`
		SNR       float64 `json:"snr"`
	} `json:"rxInfo"`
	BatteryLevel *float64 `json:"batteryLevel"`
	Margin       *float64 `json:"margin"`
}

// POST /api/v1/ingest/chirpstack/{id}?event=...
func (s *Server) chirpstackIngest(w http.ResponseWriter, r *http.Request) {
	if !s.provisionLimiter.allow(clientIP(r)) {
		writeErr(w, 429, "too many requests")
		return
	}
	id := r.PathValue("id")
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer c32c_") {
		writeErr(w, 401, "connector secret required")
		return
	}
	var org string
	var settings json.RawMessage
	err := s.pool.QueryRow(r.Context(), `SELECT org_id::text, settings FROM core.connector WHERE id = $1::uuid AND kind = 'chirpstack_http' AND secret_hash = $2 AND revoked_at IS NULL`, id, iam.HashToken(strings.TrimPrefix(auth, "Bearer "))).Scan(&org, &settings)
	if err != nil {
		s.log.Warn("connector auth rejected", "security", true, "connector", id, "remote", clientIP(r))
		writeErr(w, 401, "invalid connector or secret")
		return
	}
	var cfg struct {
		AutoCreate bool `json:"auto_create_devices"`
	}
	_ = json.Unmarshal(settings, &cfg)
	body := make([]byte, 0, 4096)
	buf := http.MaxBytesReader(w, r.Body, 256*1024)
	b, err := io.ReadAll(buf)
	if err != nil || !json.Valid(b) {
		writeErr(w, 400, "JSON body required (max 256 KiB)")
		return
	}
	body = b
	event := r.URL.Query().Get("event")
	var up chirpstackUplink
	if err := json.Unmarshal(body, &up); err != nil || up.DeviceInfo.DevEUI == "" {
		writeErr(w, 400, "deviceInfo.devEui required")
		return
	}
	devEUI := strings.ToLower(strings.TrimSpace(up.DeviceInfo.DevEUI))
	dev, err := s.lorawanDevice(r, org, devEUI, up.DeviceInfo.DeviceName, cfg.AutoCreate)
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.connector SET last_seen_at = now(), messages = messages + 1 WHERE id = $1::uuid`, id)
	meta := map[string]any{"event": event, "fcnt": up.FCnt, "fport": up.FPort, "dedup": up.DeduplicationID, "connector_id": id}
	if len(up.RxInfo) > 0 {
		best := up.RxInfo[0]
		for _, rx := range up.RxInfo[1:] {
			if rx.RSSI > best.RSSI {
				best = rx
			}
		}
		meta["rssi"], meta["snr"], meta["gateway"], meta["gateways"] = best.RSSI, best.SNR, best.GatewayID, len(up.RxInfo)
	}
	metaJSON, _ := json.Marshal(meta)
	switch event {
	case "up", "":
		// Build a protocol telemetry message from the decoded object; the ChirpStack time is the source time.
		values := map[string]any{}
		if len(up.Object) > 0 {
			_ = json.Unmarshal(up.Object, &values)
		}
		ts := up.Time
		if ts.IsZero() {
			ts = time.Now()
		}
		if len(values) == 0 {
			// nothing decoded (no codec in the device profile): keep the raw frame as a record
			s.ing.IngestTelemetry(r.Context(), dev, "lorawan.chirpstack", "up", body, string(metaJSON))
			writeJSON(w, 202, map[string]any{"accepted": true, "device": dev.ExternalID, "decoded_keys": 0, "note": "no decoded object: configure a codec in the ChirpStack device profile"})
			return
		}
		msg, _ := json.Marshal(map[string]any{"ts": ts.Unix(), "v": values})
		s.ing.IngestTelemetry(r.Context(), dev, "lorawan.chirpstack", "up", msg, string(metaJSON))
		status := map[string]any{"last_seen": time.Now().UTC(), "fcnt": up.FCnt}
		if v, ok := meta["rssi"]; ok {
			status["rssi"], status["snr"] = v, meta["snr"]
		}
		st, _ := json.Marshal(status)
		_, _ = s.pool.Exec(r.Context(), `UPDATE core.device SET status = status || $2::jsonb WHERE id = $1::uuid`, dev.ID, string(st))
		writeJSON(w, 202, map[string]any{"accepted": true, "device": dev.ExternalID, "decoded_keys": len(values)})
	case "status":
		st := map[string]any{"last_seen": time.Now().UTC()}
		if up.BatteryLevel != nil {
			st["battery"] = *up.BatteryLevel
		}
		if up.Margin != nil {
			st["margin"] = *up.Margin
		}
		stJSON, _ := json.Marshal(st)
		_, _ = s.pool.Exec(r.Context(), `UPDATE core.device SET status = status || $2::jsonb WHERE id = $1::uuid`, dev.ID, string(stJSON))
		s.storeConnectorRaw(r, org, dev.ID, event, body, string(metaJSON))
		writeJSON(w, 202, map[string]any{"accepted": true, "device": dev.ExternalID})
	default:
		s.storeConnectorRaw(r, org, dev.ID, event, body, string(metaJSON))
		writeJSON(w, 202, map[string]any{"accepted": true, "device": dev.ExternalID, "event": event})
	}
}

func (s *Server) storeConnectorRaw(r *http.Request, org, devID, topic string, body []byte, meta string) {
	_, _ = s.pool.Exec(r.Context(), `INSERT INTO core.raw_message (org_id, device_id, connector, direction, topic, payload, payload_json, meta, decode_status) VALUES ($1::uuid, $2::uuid, 'lorawan.chirpstack', 'in', $3, $4, $4::jsonb, $5::jsonb, 'ok')`, org, devID, topic, body, meta)
}

// lorawanDevice finds the device by DevEUI within the organization, creating it when the connector allows it.
func (s *Server) lorawanDevice(r *http.Request, org, devEUI, name string, autoCreate bool) (*broker.Device, error) {
	d := &broker.Device{OrgID: org, ExternalID: devEUI, Transport: "lorawan"}
	err := s.pool.QueryRow(r.Context(), `SELECT id::text FROM core.device WHERE org_id = $1::uuid AND external_id = $2 AND revoked_at IS NULL`, org, devEUI).Scan(&d.ID)
	if err == nil {
		return d, nil
	}
	if !autoCreate {
		return nil, fmt.Errorf("unknown device %s (auto-create is off for this connector)", devEUI)
	}
	if name == "" {
		name = "LoRaWAN " + devEUI
	}
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.device (org_id, external_id, name, transport) VALUES ($1::uuid, $2, $3, 'lorawan') RETURNING id::text`, org, devEUI, name).Scan(&d.ID); err != nil {
		return nil, fmt.Errorf("device could not be created")
	}
	s.audit(r, "device.create", "device", d.ID, nil, map[string]any{"external_id": devEUI, "name": name, "transport": "lorawan", "by": "connector auto-create"}, "connector auto-create")
	return d, nil
}

package ingest

import (
	"context"
	"encoding/json"

	"github.com/mochi-mqtt/server/v2/packets"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
)

// Command acknowledgements (protocol §3.4): d/{id}/cmd/ack {"id","ok","r"|"err"}.
func (in *Ingest) handleCmdAck(ctx context.Context, dev *broker.Device, _ string, pk packets.Packet) {
	var ack struct {
		ID  string          `json:"id"`
		OK  bool            `json:"ok"`
		R   json.RawMessage `json:"r"`
		Err string          `json:"err"`
	}
	if err := json.Unmarshal(pk.Payload, &ack); err != nil || ack.ID == "" || len(pk.Payload) > 16384 {
		in.storeRejected(ctx, dev, pk, "invalid ack json")
		return
	}
	status := "acked"
	if !ack.OK {
		status = "failed"
	}
	var result *string
	if len(ack.R) > 0 && json.Valid(ack.R) {
		r := string(ack.R)
		result = &r
	}
	var errText *string
	if ack.Err != "" {
		e := ack.Err
		errText = &e
	}
	tag, err := in.pool.Exec(ctx, `UPDATE core.device_command SET status = $3, acked_at = now(), result = $4::jsonb, error = $5
		WHERE id = $1::uuid AND device_id = $2::uuid AND status IN ('queued', 'sent')`, ack.ID, dev.ID, status, result, errText)
	if err != nil || tag.RowsAffected() == 0 {
		in.log.Warn("ack for unknown or finished command", "security", err != nil, "device", dev.ExternalID, "command", ack.ID, "err", err)
		in.storeRejected(ctx, dev, pk, "unknown command id")
		return
	}
	in.storeRaw(ctx, dev, pk, "ok", nil)
	in.log.Info("command acknowledged", "device", dev.ExternalID, "command", ack.ID, "status", status)
	if in.OnCommand != nil {
		var res json.RawMessage
		if result != nil {
			res = json.RawMessage(*result)
		}
		in.OnCommand(CommandEvent{OrgID: dev.OrgID, DeviceID: dev.ID, CommandID: ack.ID, Status: status, Result: res, Error: ack.Err})
	}
}

// Shared attribute requests (protocol §3.2): the device asks for its configuration after boot; the server answers on
// d/{id}/attributes/response with all shared attributes or the requested keys.
func (in *Ingest) handleAttrRequest(ctx context.Context, dev *broker.Device, _ string, pk packets.Packet) {
	var req struct {
		Keys []string `json:"keys"`
	}
	if len(pk.Payload) > 0 && json.Unmarshal(pk.Payload, &req) != nil {
		in.storeRejected(ctx, dev, pk, "invalid request json")
		return
	}
	var raw []byte
	if err := in.pool.QueryRow(ctx, `SELECT shared_attributes FROM core.device WHERE id = $1::uuid`, dev.ID).Scan(&raw); err != nil {
		in.log.Error("shared attributes lookup failed", "err", err)
		return
	}
	out := raw
	if len(req.Keys) > 0 {
		var all map[string]json.RawMessage
		_ = json.Unmarshal(raw, &all)
		sub := map[string]json.RawMessage{}
		for _, k := range req.Keys {
			if v, ok := all[k]; ok {
				sub[k] = v
			}
		}
		out, _ = json.Marshal(sub)
	}
	if err := in.br.Publish("d/"+dev.ExternalID+"/attributes/response", out, false); err != nil {
		in.log.Error("attributes response publish failed", "err", err)
	}
	in.storeRaw(ctx, dev, pk, "ok", nil)
}

// InvalidateDevice drops the cached key -> datastream mapping of a device (called when assignments change).
func (in *Ingest) InvalidateDevice(deviceID string) {
	in.dsMu.Lock()
	delete(in.dsMap, deviceID)
	delete(in.dsTime, deviceID)
	in.dsMu.Unlock()
}

package ingest

import (
	"context"
	"encoding/json"

	"github.com/mochi-mqtt/server/v2/packets"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
)

// ConsoleSink receives device console output (protocol §3.5); wired by main to the console manager.
type ConsoleSink interface {
	HandleOut(ctx context.Context, dev *broker.Device, payload []byte)
}

// SetConsole attaches the console output handler.
func (in *Ingest) SetConsole(c ConsoleSink) { in.console = c }

func (in *Ingest) handleConsoleOut(ctx context.Context, dev *broker.Device, _ string, pk packets.Packet) {
	if len(pk.Payload) > 2048 || !json.Valid(pk.Payload) {
		in.storeRejected(ctx, dev, pk, "invalid console output")
		return
	}
	if in.console != nil {
		in.console.HandleOut(ctx, dev, pk.Payload)
	}
	// console lines are records in console_log; the raw message is not duplicated here
}

// FOTA status (protocol §3.6): {"fw_id": "...", "st": "downloading|verifying|applying|updated|failed|rolled_back", "pct": 42, "ver": "1.5.0", "err": "..."}
func (in *Ingest) handleFotaStatus(ctx context.Context, dev *broker.Device, _ string, pk packets.Packet) {
	var st struct {
		FwID string `json:"fw_id"`
		St   string `json:"st"`
		Pct  *int   `json:"pct"`
		Ver  string `json:"ver"`
		Err  string `json:"err"`
	}
	if err := json.Unmarshal(pk.Payload, &st); err != nil || st.FwID == "" {
		in.storeRejected(ctx, dev, pk, "invalid fota status")
		return
	}
	switch st.St {
	case "downloading", "verifying", "applying", "updated", "failed", "rolled_back":
	default:
		in.storeRejected(ctx, dev, pk, "unknown fota state")
		return
	}
	var errText *string
	if st.Err != "" {
		e := st.Err
		errText = &e
	}
	var ver *string
	if st.Ver != "" {
		v := st.Ver
		ver = &v
	}
	tag, err := in.pool.Exec(ctx, `UPDATE core.fota_target t SET status = $3, pct = COALESCE($4, t.pct), version = COALESCE($5, t.version), error = $6, updated_at = now()
		FROM core.fota_campaign c WHERE t.campaign_id = c.id AND t.device_id = $1::uuid AND c.firmware_id = $2::uuid AND (c.closed_at IS NULL OR ($3 IN ('updated','failed','rolled_back') AND c.closed_at > now() - interval '24 hours'))`,
		dev.ID, st.FwID, st.St, st.Pct, ver, errText)
	if err != nil || tag.RowsAffected() == 0 {
		in.log.Warn("fota status for unknown campaign", "device", dev.ExternalID, "fw", st.FwID, "err", err)
		in.storeRejected(ctx, dev, pk, "no active campaign for this firmware")
		return
	}
	if st.St == "updated" && st.Ver != "" {
		_, _ = in.pool.Exec(ctx, `UPDATE core.device SET status = status || jsonb_build_object('fw', $2::text, 'last_seen', now()) WHERE id = $1::uuid`, dev.ID, st.Ver)
	}
	in.storeRaw(ctx, dev, pk, "ok", nil)
	in.log.Info("fota status", "device", dev.ExternalID, "fw", st.FwID, "state", st.St, "pct", st.Pct)
}

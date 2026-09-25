// Package ingest turns device messages into raw_message + observation rows (append-only), closes gaps
// and hands new observations to the alarm engine. Every message is stored, including rejected ones (URS F-01).
package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/metrics"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/protocol"
)

// Observation is the in-memory form handed to the alarm engine after insertion.
type Observation struct {
	ID           string
	OrgID        string
	DatastreamID string
	Key          string
	MeasuredAt   time.Time
	ReceivedAt   time.Time
	Value        *float64
	Quality      string
	Backfilled   bool

	valueJSON  *string // structured/string values as JSON text
	timeSource string  // device | device_relative | server
}

// Sink receives stored observations (alarm engine, live subscribers).
type Sink interface {
	OnObservations(ctx context.Context, obs []Observation)
}

// Ingest subscribes to device topics on the in-process broker.
type Ingest struct {
	pool    *pgxpool.Pool
	br      *broker.Broker
	log     *slog.Logger
	sinks   []Sink
	dsMu    sync.Mutex
	dsMap   map[string]map[string]string // device id -> key -> datastream id (cache)
	dsTime  map[string]time.Time
	console ConsoleSink
	// OnCommand is called when a device acknowledges a command (live data hook).
	OnCommand func(CommandEvent)
}

// CommandEvent describes an acknowledged command for live streams.
type CommandEvent struct {
	OrgID, DeviceID, CommandID, Status string
	Result                             json.RawMessage
	Error                              string
}

// New creates the ingest layer.
func New(pool *pgxpool.Pool, br *broker.Broker, log *slog.Logger, sinks ...Sink) *Ingest {
	return &Ingest{pool: pool, br: br, log: log.With("component", "ingest"), sinks: sinks,
		dsMap: map[string]map[string]string{}, dsTime: map[string]time.Time{}}
}

// AddSink registers another observation sink (the rules engine is created after ingest).
func (in *Ingest) AddSink(s Sink) { in.sinks = append(in.sinks, s) }

type derivedDepthKey struct{}

// StoreDerived stores a value computed by an automation rule as an observation of the target datastream
// (quality ok, server time, decoder rule-v1, no raw message) and hands it to the sinks like any other
// observation. A chain of rules deriving from each other is cut after 8 levels so a loop cannot run away.
func (in *Ingest) StoreDerived(ctx context.Context, org, datastreamID, key string, value float64, at time.Time) error {
	depth, _ := ctx.Value(derivedDepthKey{}).(int)
	if depth >= 8 {
		return errors.New("derived datastream chain too deep (rules deriving from each other?)")
	}
	ctx = context.WithValue(ctx, derivedDepthKey{}, depth+1)
	o := Observation{OrgID: org, DatastreamID: datastreamID, Key: key, MeasuredAt: at.UTC(), Quality: "ok", timeSource: "server"}
	o.Value = &value
	if err := in.pool.QueryRow(ctx, `
		INSERT INTO core.observation (org_id, datastream_id, measured_at, value, quality, time_source, decoder_version)
		VALUES ($1::uuid, $2::uuid, $3, $4::numeric, 'ok', 'server', 'rule-v1') RETURNING id::text, received_at`,
		org, datastreamID, o.MeasuredAt, strconv.FormatFloat(value, 'g', -1, 64)).Scan(&o.ID, &o.ReceivedAt); err != nil {
		return err
	}
	_, _ = in.pool.Exec(ctx, `UPDATE core.gap SET gap_to = $2, closed_at = now(), closed_by = 'resumed' WHERE datastream_id = $1::uuid AND closed_at IS NULL`, datastreamID, o.MeasuredAt)
	for _, s := range in.sinks {
		s.OnObservations(ctx, []Observation{o})
	}
	return nil
}

// Start registers the inline subscriptions. Must be called after the broker is created.
func (in *Ingest) Start() error {
	subs := []struct {
		filter string
		fn     func(ctx context.Context, dev *broker.Device, rest string, pk packets.Packet)
	}{
		{"d/+/telemetry", in.handleTelemetry},
		{"d/+/status", in.handleStatus},
		{"d/+/attributes", in.handleAttributes},
		{"d/+/cmd/ack", in.handleCmdAck},
		{"d/+/attributes/request", in.handleAttrRequest},
		{"d/+/console/out", in.handleConsoleOut},
		{"d/+/fota/status", in.handleFotaStatus},
		{"v1/devices/me/telemetry", in.handleTelemetry},
		{"v1/devices/me/attributes", in.handleAttributes},
	}
	for i, s := range subs {
		fn := s.fn
		err := in.br.Server.Subscribe(s.filter, 100+i, func(_ *mqtt.Client, sub packets.Subscription, pk packets.Packet) {
			// Inline subscriptions are delivered to the inline client; the publisher is identified by pk.Origin
			// (its client id). The ACL guarantees that d/{ext}/… is only writable by the owning device, so the
			// topic is a safe fallback when the origin is not an authenticated device (e.g. a retained replay).
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			dev, ok := in.br.DeviceOf(pk.Origin)
			rest := ""
			if strings.HasPrefix(pk.TopicName, "v1/devices/me/") {
				if !ok {
					in.log.Warn("alias topic from unauthenticated origin dropped", "security", true, "origin", pk.Origin, "topic", pk.TopicName)
					return
				}
				rest = strings.TrimPrefix(pk.TopicName, "v1/devices/me/")
			} else {
				ext, r, valid := broker.DeviceIDFromTopic(pk.TopicName)
				if !valid {
					return
				}
				rest = r
				if !ok || dev.ExternalID != ext {
					d, err := in.deviceByExternalID(ctx, ext)
					if err != nil {
						in.log.Warn("message for unknown device dropped", "security", true, "origin", pk.Origin, "topic", pk.TopicName, "err", err)
						return
					}
					dev = d
				}
			}
			fn(ctx, dev, rest, pk)
		})
		if err != nil {
			return fmt.Errorf("subscribe %s: %w", s.filter, err)
		}
	}
	return nil
}

// handleTelemetry: parse → resolve datastreams → single transaction: raw_message + observations → close gaps → sinks.
func (in *Ingest) handleTelemetry(ctx context.Context, dev *broker.Device, _ string, pk packets.Packet) {
	in.IngestTelemetry(ctx, dev, "mqtt", pk.TopicName, pk.Payload, `{"qos":`+strconv.Itoa(int(pk.FixedHeader.Qos))+`}`)
}

// IngestTelemetry stores a telemetry message from any connector (MQTT, HTTP, LoRaWAN) as raw_message + observations
// in one transaction, closes gaps and hands the observations to the sinks (alarm engine).
// TelemetryResult tells an HTTP caller what happened to a telemetry message.
type TelemetryResult struct {
	Status       string   `json:"status"` // ok | unknown_key | error
	Detail       string   `json:"detail,omitempty"`
	Observations int      `json:"observations"`
	UnknownKeys  []string `json:"unknown_keys,omitempty"`
}

// IngestTelemetry stores a telemetry message; see IngestTelemetryResult for the outcome.
func (in *Ingest) IngestTelemetry(ctx context.Context, dev *broker.Device, connector, topic string, payload []byte, metaJSON string) {
	in.IngestTelemetryResult(ctx, dev, connector, topic, payload, metaJSON)
}

// HTTP entry points (protocol §3.8): the MQTT handlers fed with a synthetic packet whose origin is "http".
func httpPacket(dev *broker.Device, suffix string, payload []byte) packets.Packet {
	return packets.Packet{TopicName: "d/" + dev.ExternalID + "/" + suffix, Payload: payload, Origin: "http"}
}

func (in *Ingest) HTTPStatus(ctx context.Context, dev *broker.Device, payload []byte) {
	in.handleStatus(ctx, dev, "status", httpPacket(dev, "status", payload))
}

func (in *Ingest) HTTPAttributes(ctx context.Context, dev *broker.Device, payload []byte) {
	in.handleAttributes(ctx, dev, "attributes", httpPacket(dev, "attributes", payload))
}

func (in *Ingest) HTTPCmdAck(ctx context.Context, dev *broker.Device, payload []byte) {
	in.handleCmdAck(ctx, dev, "cmd/ack", httpPacket(dev, "cmd/ack", payload))
}

func (in *Ingest) HTTPFotaStatus(ctx context.Context, dev *broker.Device, payload []byte) {
	in.handleFotaStatus(ctx, dev, "fota/status", httpPacket(dev, "fota/status", payload))
}

func (in *Ingest) IngestTelemetryResult(ctx context.Context, dev *broker.Device, connector, topic string, payload []byte, metaJSON string) TelemetryResult {
	samples, perr := protocol.ParseTelemetry(payload)

	var payloadJSON *string
	if json.Valid(payload) {
		s := string(payload)
		payloadJSON = &s
	}

	status := "ok"
	var decodeErr *string
	var rows []Observation
	unknown := map[string]struct{}{}

	if perr != nil {
		status = "error"
		msg := perr.Error()
		decodeErr = &msg
	} else {
		keys, err := in.datastreamsFor(ctx, dev)
		if err != nil {
			in.log.Error("datastream lookup failed", "device", dev.ExternalID, "err", err)
			return TelemetryResult{Status: "error", Detail: "internal error, see the server log"}
		}
		now := time.Now().UTC()
		in.locationFromSamples(ctx, dev, samples)
		for _, s := range samples {
			measuredAt := now
			timeSource := "server"
			switch {
			case s.MeasuredAt != nil:
				measuredAt = *s.MeasuredAt
				timeSource = "device"
			case s.Relative != nil:
				measuredAt = now.Add(time.Duration(*s.Relative) * time.Second)
				timeSource = "device_relative"
			}
			quality := "ok"
			if s.Backfilled {
				quality = "backfilled"
			}
			// Clock plausibility: a device timestamp more than 5 minutes in the future is suspect.
			if timeSource == "device" && measuredAt.After(now.Add(5*time.Minute)) {
				quality = "clock_suspect"
			}
			for k, v := range s.Values {
				dsID, ok := keys[k]
				if !ok {
					unknown[k] = struct{}{}
					continue
				}
				o := Observation{OrgID: dev.OrgID, DatastreamID: dsID, Key: k, MeasuredAt: measuredAt, Quality: quality, Backfilled: s.Backfilled}
				o.Value = v.Num
				if v.Bool != nil {
					f := 0.0
					if *v.Bool {
						f = 1
					}
					o.Value = &f
				}
				rows = append(rows, obsWithJSON(o, v, timeSource))
			}
		}
		if len(unknown) > 0 {
			status = "unknown_key"
			ks := make([]string, 0, len(unknown))
			for k := range unknown {
				ks = append(ks, k)
			}
			msg := "unknown keys: " + strings.Join(ks, ",")
			decodeErr = &msg
		}
	}

	tx, err := in.pool.Begin(ctx)
	if err != nil {
		in.log.Error("begin failed", "err", err)
		return TelemetryResult{Status: "error", Detail: "internal error, see the server log"}
	}
	defer tx.Rollback(ctx)

	metrics.IngestMessages.Inc(status)
	var rawID string
	var receivedAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO core.raw_message (org_id, device_id, connector, direction, topic, payload, payload_json, meta, decode_status, decode_error)
		VALUES ($1::uuid, $2::uuid, $9, 'in', $3, $4, $5::jsonb, $6::jsonb, $7, $8)
		RETURNING id::text, received_at`,
		dev.OrgID, dev.ID, topic, payload, payloadJSON, metaJSON, status, decodeErr, connector,
	).Scan(&rawID, &receivedAt)
	if err != nil {
		in.log.Error("raw_message insert failed", "err", err)
		return TelemetryResult{Status: "error", Detail: "internal error, see the server log"}
	}

	if len(rows) > 0 {
		batch := &pgx.Batch{}
		for i := range rows {
			r := &rows[i]
			r.ReceivedAt = receivedAt
			var valStr *string
			if r.Value != nil {
				s := strconv.FormatFloat(*r.Value, 'g', -1, 64)
				valStr = &s
			}
			batch.Queue(`
				INSERT INTO core.observation (org_id, datastream_id, measured_at, received_at, value, value_json, quality, time_source, decoder_version, raw_id)
				VALUES ($1::uuid, $2::uuid, $3, $4, $5::numeric, $6::jsonb, $7, $8, 'json-v1', $9::uuid)
				RETURNING id::text`,
				r.OrgID, r.DatastreamID, r.MeasuredAt, r.ReceivedAt, valStr, r.valueJSON, r.Quality, r.timeSource, rawID)
		}
		br := tx.SendBatch(ctx, batch)
		for i := range rows {
			if err := br.QueryRow().Scan(&rows[i].ID); err != nil {
				br.Close()
				in.log.Error("observation insert failed", "err", err, "key", rows[i].Key)
				return TelemetryResult{Status: "error", Detail: "internal error, see the server log"}
			}
		}
		if err := br.Close(); err != nil {
			in.log.Error("observation batch close failed", "err", err)
			return TelemetryResult{Status: "error", Detail: "internal error, see the server log"}
		}
		metrics.Observations.Add("", uint64(len(rows)))
		// Close open gaps for datastreams that just received data.
		for i := range rows {
			closedBy := "resumed"
			if rows[i].Backfilled {
				closedBy = "backfill"
			}
			if _, err := tx.Exec(ctx, `UPDATE core.gap SET gap_to = $2, closed_at = now(), closed_by = $3
				WHERE datastream_id = $1::uuid AND closed_at IS NULL`, rows[i].DatastreamID, rows[i].MeasuredAt, closedBy); err != nil {
				in.log.Error("gap close failed", "err", err)
				return TelemetryResult{Status: "error", Detail: "internal error, see the server log"}
			}
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE core.device SET status = status || jsonb_build_object('last_seen', now(), 'last_topic', $2::text) WHERE id = $1::uuid`, dev.ID, topic); err != nil {
		in.log.Error("device status update failed", "err", err)
		return TelemetryResult{Status: "error", Detail: "internal error, see the server log"}
	}
	if err := tx.Commit(ctx); err != nil {
		in.log.Error("commit failed", "err", err)
		return TelemetryResult{Status: "error", Detail: "internal error, see the server log"}
	}

	if status != "ok" {
		in.log.Warn("telemetry stored with decode issue", "device", dev.ExternalID, "status", status, "detail", derefStr(decodeErr))
	} else {
		in.log.Debug("telemetry stored", "device", dev.ExternalID, "observations", len(rows))
	}
	for _, s := range in.sinks {
		s.OnObservations(ctx, rows)
	}
	res := TelemetryResult{Status: status, Detail: derefStr(decodeErr), Observations: len(rows)}
	for k := range unknown {
		res.UnknownKeys = append(res.UnknownKeys, k)
	}
	sort.Strings(res.UnknownKeys)
	return res
}

func (in *Ingest) handleStatus(ctx context.Context, dev *broker.Device, _ string, pk packets.Packet) {
	if !json.Valid(pk.Payload) || len(pk.Payload) > 4096 {
		in.storeRejected(ctx, dev, pk, "invalid status json")
		return
	}
	_, err := in.pool.Exec(ctx, `UPDATE core.device SET status = status || $2::jsonb || jsonb_build_object('last_seen', now()) WHERE id = $1::uuid`, dev.ID, string(pk.Payload))
	in.locationFromJSON(ctx, dev, pk.Payload)
	if err != nil {
		in.log.Error("status update failed", "err", err)
	}
	in.storeRaw(ctx, dev, pk, "ok", nil)
}

func (in *Ingest) handleAttributes(ctx context.Context, dev *broker.Device, _ string, pk packets.Packet) {
	if !json.Valid(pk.Payload) || len(pk.Payload) > 16384 {
		in.storeRejected(ctx, dev, pk, "invalid attributes json")
		return
	}
	// PoC: client attributes live under device.status.attributes; a dedicated attribute table comes in phase 1.
	_, err := in.pool.Exec(ctx, `UPDATE core.device SET status = jsonb_set(status, '{attributes}', COALESCE(status->'attributes','{}'::jsonb) || $2::jsonb, true) WHERE id = $1::uuid`, dev.ID, string(pk.Payload))
	if err != nil {
		in.log.Error("attributes update failed", "err", err)
	}
	in.storeRaw(ctx, dev, pk, "ok", nil)
}

func (in *Ingest) storeRejected(ctx context.Context, dev *broker.Device, pk packets.Packet, reason string) {
	in.storeRaw(ctx, dev, pk, "error", &reason)
}

func (in *Ingest) storeRaw(ctx context.Context, dev *broker.Device, pk packets.Packet, status string, decodeErr *string) {
	metrics.IngestMessages.Inc(status)
	var payloadJSON *string
	if json.Valid(pk.Payload) {
		s := string(pk.Payload)
		payloadJSON = &s
	}
	if _, err := in.pool.Exec(ctx, `
		INSERT INTO core.raw_message (org_id, device_id, connector, direction, topic, payload, payload_json, decode_status, decode_error)
		VALUES ($1::uuid, $2::uuid, $8, 'in', $3, $4, $5::jsonb, $6, $7)`,
		dev.OrgID, dev.ID, pk.TopicName, pk.Payload, payloadJSON, status, decodeErr, connectorOf(pk)); err != nil {
		in.log.Error("raw_message insert failed", "err", err)
	}
}

// deviceByExternalID resolves an active device by its external id (used when the packet origin is not a live client).
func (in *Ingest) deviceByExternalID(ctx context.Context, ext string) (*broker.Device, error) {
	var d broker.Device
	err := in.pool.QueryRow(ctx, `SELECT d.id::text, d.org_id::text, d.external_id, d.transport FROM core.device d JOIN core.organization o ON o.id = d.org_id WHERE d.external_id = $1 AND d.revoked_at IS NULL AND o.disabled_at IS NULL`, ext).
		Scan(&d.ID, &d.OrgID, &d.ExternalID, &d.Transport)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// datastreamsFor returns key -> datastream id for a device via current device_datastream assignments (cached 30 s).
func (in *Ingest) datastreamsFor(ctx context.Context, dev *broker.Device) (map[string]string, error) {
	in.dsMu.Lock()
	if m, ok := in.dsMap[dev.ID]; ok && time.Since(in.dsTime[dev.ID]) < 30*time.Second {
		in.dsMu.Unlock()
		return m, nil
	}
	in.dsMu.Unlock()

	rows, err := in.pool.Query(ctx, `
		SELECT ds.key, ds.id::text
		FROM core.assignment a JOIN core.datastream ds ON ds.id = a.to_id
		WHERE a.kind = 'device_datastream' AND a.from_id = $1::uuid AND a.valid_to IS NULL`, dev.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var k, id string
		if err := rows.Scan(&k, &id); err != nil {
			return nil, err
		}
		m[k] = id
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	in.dsMu.Lock()
	in.dsMap[dev.ID] = m
	in.dsTime[dev.ID] = time.Now()
	in.dsMu.Unlock()
	return m, nil
}

// obsWithJSON attaches structured values and the time source (kept out of the exported struct).
func obsWithJSON(o Observation, v protocol.Value, timeSource string) Observation {
	o.timeSource = timeSource
	if v.Str != nil {
		b, _ := json.Marshal(*v.Str)
		s := string(b)
		o.valueJSON = &s
	} else if len(v.JSON) > 0 {
		s := string(v.JSON)
		o.valueJSON = &s
	}
	return o
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// connectorOf names the transport a synthetic packet came from ("http") — real MQTT packets carry a client id.
func connectorOf(pk packets.Packet) string {
	if pk.Origin == "http" {
		return "http"
	}
	return "mqtt"
}

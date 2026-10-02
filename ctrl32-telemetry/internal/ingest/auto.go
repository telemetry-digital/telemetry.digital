package ingest

// Automatic datastreams (spec 005 research R4): a device with auto_datastreams gets a datastream for every new
// telemetry key, under its own asset (created in the provisioning profile's site and room when missing). Keys that are
// not valid datastream keys are normalized; the original key is kept in attributes.source_key and resolved on ingest.

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/metrics"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/protocol"
)

// MaxAutoDatastreams per device.
const MaxAutoDatastreams = 64

var dsKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// NormalizeKey turns any telemetry key into a valid datastream key ("Temperature 1" → "temperature_1").
func NormalizeKey(k string) string {
	var b strings.Builder
	prevUnderscore := false
	for _, r := range strings.ToLower(k) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			prevUnderscore = false
		case !prevUnderscore:
			b.WriteByte('_')
			prevUnderscore = true
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		s = "k_" + s
	}
	if len(s) > 32 {
		s = s[:32]
	}
	return strings.TrimRight(s, "_")
}

type autoInfo struct {
	enabled bool
	at      time.Time
}

func (in *Ingest) autoEnabled(ctx context.Context, dev *broker.Device) bool {
	in.dsMu.Lock()
	if in.auto == nil {
		in.auto = map[string]autoInfo{}
	}
	a, ok := in.auto[dev.ID]
	in.dsMu.Unlock()
	if ok && time.Since(a.at) < 30*time.Second {
		return a.enabled
	}
	var enabled bool
	_ = in.pool.QueryRow(ctx, `SELECT auto_datastreams FROM core.device WHERE id = $1::uuid`, dev.ID).Scan(&enabled)
	in.dsMu.Lock()
	in.auto[dev.ID] = autoInfo{enabled, time.Now()}
	in.dsMu.Unlock()
	return enabled
}

// autoDatastreams creates datastreams for the unknown keys of the samples and returns the updated key map.
func (in *Ingest) autoDatastreams(ctx context.Context, dev *broker.Device, samples []protocol.Sample, keys map[string]string) map[string]string {
	kinds := map[string]string{}
	for _, s := range samples {
		for k, v := range s.Values {
			if _, ok := keys[k]; !ok {
				kinds[k] = v.Kind()
			}
		}
	}
	if len(kinds) == 0 || !in.autoEnabled(ctx, dev) {
		return keys
	}
	created, err := CreateDeviceDatastreams(ctx, in.pool, dev.OrgID, dev.ID, kinds)
	if err != nil {
		in.log.Warn("automatic datastreams not created", "device", dev.ExternalID, "err", err)
	}
	if len(created) == 0 {
		return keys
	}
	in.dsMu.Lock()
	delete(in.dsMap, dev.ID)
	in.dsMu.Unlock()
	out := make(map[string]string, len(keys)+len(created))
	for k, v := range keys {
		out[k] = v
	}
	for k, v := range created {
		out[k] = v
	}
	return out
}

// DatastreamSpec describes a datastream to create for a device.
type DatastreamSpec struct {
	Key      string // preferred datastream key ("" = from the source key)
	Kind     string // gauge | counter | state | event
	Quantity string
	Unit     string
	Attrs    map[string]any
}

// CreateDeviceDatastreams creates and assigns datastreams for telemetry keys (key → value kind num/bool/str/json).
func CreateDeviceDatastreams(ctx context.Context, pool *pgxpool.Pool, org, deviceID string, kinds map[string]string) (map[string]string, error) {
	specs := map[string]DatastreamSpec{}
	for k, kind := range kinds {
		dk := "state"
		if kind == "num" {
			dk = "gauge"
		}
		specs[k] = DatastreamSpec{Kind: dk, Quantity: NormalizeKey(k)}
	}
	return CreateDatastreams(ctx, pool, org, deviceID, specs, "automatic datastream from device telemetry")
}

// CreateDatastreams creates datastreams for a device (source key → spec) under the device's asset and assigns them.
// It returns source key → datastream id. At most MaxAutoDatastreams automatic datastreams per device.
func CreateDatastreams(ctx context.Context, pool *pgxpool.Pool, org, deviceID string, specs map[string]DatastreamSpec, reason string) (map[string]string, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var name, ext string
	var profileSite, profileRoom *string
	if err := tx.QueryRow(ctx, `SELECT coalesce(nullif(d.name, ''), d.external_id), d.external_id, pp.site_id::text, pp.room_id::text
		FROM core.device d LEFT JOIN core.provision_profile pp ON pp.id = d.provision_profile_id WHERE d.id = $1::uuid FOR UPDATE OF d`, deviceID).
		Scan(&name, &ext, &profileSite, &profileRoom); err != nil {
		return nil, err
	}
	var count int
	_ = tx.QueryRow(ctx, `SELECT count(*) FROM core.assignment a JOIN core.datastream ds ON ds.id = a.to_id
		WHERE a.kind = 'device_datastream' AND a.from_id = $1::uuid AND a.valid_to IS NULL AND ds.attributes->>'auto' = 'true'`, deviceID).Scan(&count)
	asset, err := DeviceAsset(ctx, tx, org, deviceID, name, profileSite, profileRoom)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	var added []map[string]any
	for src, sp := range specs {
		if count >= MaxAutoDatastreams {
			break
		}
		key := sp.Key
		if key == "" {
			key = src
		}
		if !dsKeyRe.MatchString(key) {
			key = NormalizeKey(key)
		}
		attrs := map[string]any{"auto": true, "source_key": src}
		for k, v := range sp.Attrs {
			attrs[k] = v
		}
		quantity := sp.Quantity
		if quantity == "" {
			quantity = key
		}
		// reuse the datastream of the same source key; another source with the same key gets key_2, key_3, …
		var id string
		for i := 1; i <= 20 && id == ""; i++ {
			k := key
			if i > 1 {
				suffix := fmt.Sprintf("_%d", i)
				k = strings.TrimRight(key[:min(len(key), 32-len(suffix))], "_") + suffix
			}
			err := tx.QueryRow(ctx, `INSERT INTO core.datastream (org_id, asset_id, key, quantity, unit, kind, attributes)
				VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7) ON CONFLICT (asset_id, key) DO NOTHING RETURNING id::text`,
				org, asset, k, quantity, sp.Unit, sp.Kind, attrs).Scan(&id)
			if err == pgx.ErrNoRows {
				var other, otherSrc string
				if err := tx.QueryRow(ctx, `SELECT id::text, coalesce(attributes->>'source_key', key) FROM core.datastream WHERE asset_id = $1::uuid AND key = $2`, asset, k).Scan(&other, &otherSrc); err != nil {
					return nil, err
				}
				if otherSrc == src {
					id = other
				}
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("datastream %s: %w", k, err)
			}
			key = k
		}
		if id == "" {
			return nil, fmt.Errorf("no free datastream key for %s", src)
		}
		var assigned int
		_ = tx.QueryRow(ctx, `SELECT count(*) FROM core.assignment WHERE kind = 'device_datastream' AND from_id = $1::uuid AND to_id = $2::uuid AND valid_to IS NULL`, deviceID, id).Scan(&assigned)
		if assigned == 0 {
			if _, err := tx.Exec(ctx, `INSERT INTO core.assignment (org_id, kind, from_id, to_id, reason) VALUES ($1::uuid, 'device_datastream', $2::uuid, $3::uuid, $4)`,
				org, deviceID, id, reason); err != nil {
				return nil, err
			}
		}
		out[src] = id
		added = append(added, map[string]any{"key": key, "source_key": src, "kind": sp.Kind, "unit": sp.Unit, "datastream_id": id})
		count++
	}
	if len(added) > 0 {
		if _, _, err := audit.Append(ctx, tx, audit.Entry{OrgID: org, ActorType: "device", ActorID: deviceID, ActorName: ext, Action: "datastream.auto_create",
			EntityType: "device", EntityID: deviceID, New: added, Reason: reason}); err != nil {
			return nil, err
		}
	}
	return out, tx.Commit(ctx)
}

// DeviceAsset returns the asset holding a device's datastreams, creating "<device name>" in the given site/room (or
// the organization's first site, or a new site "Devices") when the device has none yet.
func DeviceAsset(ctx context.Context, tx pgx.Tx, org, deviceID, name string, siteID, roomID *string) (string, error) {
	var asset string
	err := tx.QueryRow(ctx, `SELECT ds.asset_id::text FROM core.assignment a JOIN core.datastream ds ON ds.id = a.to_id
		WHERE a.kind = 'device_datastream' AND a.from_id = $1::uuid AND a.valid_to IS NULL ORDER BY a.valid_from LIMIT 1`, deviceID).Scan(&asset)
	if err == nil {
		return asset, nil
	}
	if err := tx.QueryRow(ctx, `SELECT id::text FROM core.asset WHERE org_id = $1::uuid AND attributes->>'device_id' = $2 ORDER BY created_at LIMIT 1`, org, deviceID).Scan(&asset); err == nil {
		return asset, nil
	}
	site := ""
	if roomID != nil && *roomID != "" {
		_ = tx.QueryRow(ctx, `SELECT site_id::text FROM core.asset WHERE id = $1::uuid AND org_id = $2::uuid`, *roomID, org).Scan(&site)
	}
	if site == "" && siteID != nil && *siteID != "" {
		site = *siteID
	}
	if site == "" {
		_ = tx.QueryRow(ctx, `SELECT id::text FROM core.site WHERE org_id = $1::uuid ORDER BY created_at LIMIT 1`, org).Scan(&site)
	}
	if site == "" {
		if err := tx.QueryRow(ctx, `INSERT INTO core.site (org_id, name) VALUES ($1::uuid, 'Devices') ON CONFLICT (org_id, name) DO UPDATE SET name = EXCLUDED.name RETURNING id::text`, org).Scan(&site); err != nil {
			return "", err
		}
	}
	var parent *string
	if roomID != nil && *roomID != "" {
		parent = roomID
	}
	for i := 0; i < 20; i++ {
		n := name
		if i > 0 {
			n = fmt.Sprintf("%s (%d)", name, i+1)
		}
		err = tx.QueryRow(ctx, `INSERT INTO core.asset (org_id, site_id, parent_id, type, name, attributes) VALUES ($1::uuid, $2::uuid, $3::uuid, 'device', $4, jsonb_build_object('device_id', $5::text))
			ON CONFLICT (site_id, name) DO NOTHING RETURNING id::text`, org, site, parent, n, deviceID).Scan(&asset)
		if err == nil {
			return asset, nil
		}
		if err != pgx.ErrNoRows {
			return "", err
		}
	}
	return "", fmt.Errorf("no free asset name for %s", name)
}

// StoreState stores a state of a smart-home entity as an observation of its datastream (a number, and/or a text as
// JSON) and hands it to the sinks (alarms, flows, live data) like telemetry.
func (in *Ingest) StoreState(ctx context.Context, org, datastreamID, key string, num *float64, text *string, at time.Time) error {
	o := Observation{OrgID: org, DatastreamID: datastreamID, Key: key, MeasuredAt: at.UTC(), Quality: "ok", timeSource: "server", Value: num}
	if text != nil {
		b, _ := json.Marshal(*text)
		s := string(b)
		o.valueJSON = &s
	}
	var numStr *string
	if num != nil {
		s := strconv.FormatFloat(*num, 'g', -1, 64)
		numStr = &s
	}
	in.ensureMonths(ctx, "observation", o.MeasuredAt)
	if err := in.pool.QueryRow(ctx, `
		INSERT INTO core.observation (org_id, datastream_id, measured_at, value, value_json, quality, time_source, decoder_version)
		VALUES ($1::uuid, $2::uuid, $3, $4::numeric, $5::jsonb, 'ok', 'server', 'ha-discovery-v1') RETURNING id::text, received_at`,
		org, datastreamID, o.MeasuredAt, numStr, o.valueJSON).Scan(&o.ID, &o.ReceivedAt); err != nil {
		return err
	}
	metrics.Observations.Add("", 1)
	_, _ = in.pool.Exec(ctx, `UPDATE core.gap SET gap_to = $2, closed_at = now(), closed_by = 'resumed' WHERE datastream_id = $1::uuid AND closed_at IS NULL`, datastreamID, o.MeasuredAt)
	for _, s := range in.sinks {
		s.OnObservations(ctx, []Observation{o})
	}
	return nil
}

// Package seed creates demo data for development and the public demo instance.
package seed

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
)

// Demo creates organization "demo" with one site, two assets, three datastreams, one MQTT device
// (external id "esp32-demo") with a fresh token, the device→datastream assignments and one high-limit alarm rule.
// Idempotent for the structural rows; a new token is issued on every run (older tokens are revoked).
func Demo(ctx context.Context, pool *pgxpool.Pool, out io.Writer) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var orgID string
	if err := tx.QueryRow(ctx, `INSERT INTO core.organization (slug, name, tz, locale, profile) VALUES ('demo', 'Demo organization', 'Europe/Bratislava', 'en', 'intranet')
		ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name RETURNING id::text`).Scan(&orgID); err != nil {
		return fmt.Errorf("organization: %w", err)
	}
	var siteID string
	if err := tx.QueryRow(ctx, `INSERT INTO core.site (org_id, name, tz) VALUES ($1::uuid, 'Demo site', 'Europe/Bratislava')
		ON CONFLICT (org_id, name) DO UPDATE SET tz = EXCLUDED.tz RETURNING id::text`, orgID).Scan(&siteID); err != nil {
		return fmt.Errorf("site: %w", err)
	}
	assets := []struct{ typ, name, attrs string }{
		{"fridge", "Fridge 1", `{"storage_class":"2-8C"}`},
		{"room", "Room 1", `{"storage_class":"15-25C"}`},
	}
	assetIDs := map[string]string{}
	for _, a := range assets {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO core.asset (org_id, site_id, type, name, attributes) VALUES ($1::uuid, $2::uuid, $3, $4, $5::jsonb)
			ON CONFLICT (site_id, name) DO UPDATE SET attributes = EXCLUDED.attributes RETURNING id::text`, orgID, siteID, a.typ, a.name, a.attrs).Scan(&id); err != nil {
			return fmt.Errorf("asset %s: %w", a.name, err)
		}
		assetIDs[a.name] = id
	}
	streams := []struct {
		asset, key, quantity, unit, kind, interval string
		accuracy                                   float64
	}{
		{"Fridge 1", "temp", "temperature", "°C", "gauge", "10 minutes", 0.5},
		{"Room 1", "temp", "temperature", "°C", "gauge", "10 minutes", 0.5},
		{"Room 1", "rh", "humidity", "%", "gauge", "10 minutes", 3},
	}
	dsIDs := map[string]string{}
	for _, s := range streams {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO core.datastream (org_id, asset_id, key, quantity, unit, kind, expected_interval, accuracy)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7::interval, $8)
			ON CONFLICT (asset_id, key) DO UPDATE SET unit = EXCLUDED.unit RETURNING id::text`,
			orgID, assetIDs[s.asset], s.key, s.quantity, s.unit, s.kind, s.interval, s.accuracy).Scan(&id); err != nil {
			return fmt.Errorf("datastream %s/%s: %w", s.asset, s.key, err)
		}
		dsIDs[s.asset+"/"+s.key] = id
	}

	var devID string
	if err := tx.QueryRow(ctx, `INSERT INTO core.device (org_id, external_id, name, transport) VALUES ($1::uuid, 'esp32-demo', 'ESP32 demo board', 'mqtt')
		ON CONFLICT (org_id, external_id) DO UPDATE SET name = EXCLUDED.name RETURNING id::text`, orgID).Scan(&devID); err != nil {
		return fmt.Errorf("device: %w", err)
	}
	// Issue a fresh token; older tokens are revoked (device_credential is mutable operational data).
	if _, err := tx.Exec(ctx, `UPDATE core.device_credential SET revoked_at = now() WHERE device_id = $1::uuid AND kind = 'token' AND revoked_at IS NULL`, devID); err != nil {
		return err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	token := hex.EncodeToString(raw)
	if _, err := tx.Exec(ctx, `INSERT INTO core.device_credential (device_id, kind, secret_hash) VALUES ($1::uuid, 'token', $2)`, devID, broker.TokenHash(token)); err != nil {
		return fmt.Errorf("credential: %w", err)
	}

	// Demo device publishes keys "temp" (fridge) and "room_temp"/"rh"? Keep it simple: the ESP32 reports the fridge
	// temperature as "temp" and the room humidity as "rh"; "room_temp" is left for a second device.
	for _, pair := range []struct{ ds string }{{"Fridge 1/temp"}, {"Room 1/rh"}} {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM core.assignment WHERE kind='device_datastream' AND from_id=$1::uuid AND to_id=$2::uuid AND valid_to IS NULL)`,
			devID, dsIDs[pair.ds]).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			if _, err := tx.Exec(ctx, `INSERT INTO core.assignment (org_id, kind, from_id, to_id, reason) VALUES ($1::uuid, 'device_datastream', $2::uuid, $3::uuid, 'demo seed')`,
				orgID, devID, dsIDs[pair.ds]); err != nil {
				return fmt.Errorf("assignment %s: %w", pair.ds, err)
			}
		}
	}

	var ruleExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM core.alarm_rule WHERE datastream_id = $1::uuid AND type = 'high' AND valid_to IS NULL)`, dsIDs["Fridge 1/temp"]).Scan(&ruleExists); err != nil {
		return err
	}
	if !ruleExists {
		if _, err := tx.Exec(ctx, `INSERT INTO core.alarm_rule (org_id, datastream_id, type, warning, action, hysteresis, reason)
			VALUES ($1::uuid, $2::uuid, 'high', 7.5, 8.0, 0.2, 'demo: fridge 2-8 °C, warning inside the band per NHS SPS guidance')`, orgID, dsIDs["Fridge 1/temp"]); err != nil {
			return fmt.Errorf("alarm rule: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	if _, _, err := audit.Append(ctx, pool, audit.Entry{OrgID: orgID, ActorType: "system", ActorName: "dev seed", Action: "device.create", EntityType: "device", EntityID: devID,
		New: map[string]any{"external_id": "esp32-demo", "token_rotated": true}}); err != nil {
		return err
	}

	fmt.Fprintf(out, "organization %s\nsite %s\ndevice esp32-demo (%s)\n", orgID, siteID, devID)
	fmt.Fprintf(out, "datastreams: %v\n", dsIDs)
	fmt.Fprintf(out, "\nMQTT username: esp32-demo\nMQTT password (token, shown once): %s\n", token)
	return nil
}

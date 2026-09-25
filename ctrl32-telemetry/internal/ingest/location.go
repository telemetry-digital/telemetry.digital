package ingest

import (
	"context"
	"encoding/json"
	"math"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/protocol"
)

// Device location (URS J-19): a device either has a fixed position set in the UI or reports one. Reported
// positions come from telemetry keys lat/lon (or latitude/longitude) and from the status heartbeat; they never
// overwrite a fixed position. The current position lives in core.device.location; the history stays in the
// observations when the keys are assigned to datastreams.

func validLatLon(lat, lon float64) bool {
	return !math.IsNaN(lat) && !math.IsNaN(lon) && lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180 && !(lat == 0 && lon == 0)
}

func (in *Ingest) setReportedLocation(ctx context.Context, dev *broker.Device, lat, lon float64) {
	if !validLatLon(lat, lon) {
		return
	}
	_, err := in.pool.Exec(ctx, `UPDATE core.device SET location = jsonb_build_object('lat', $2::float8, 'lon', $3::float8, 'source', 'reported', 'updated_at', now())
		WHERE id = $1::uuid AND (location IS NULL OR location->>'source' IS DISTINCT FROM 'fixed')`, dev.ID, lat, lon)
	if err != nil {
		in.log.Warn("device location update failed", "device", dev.ExternalID, "err", err)
	}
}

// locationFromSamples takes the last sample that carries both coordinates.
func (in *Ingest) locationFromSamples(ctx context.Context, dev *broker.Device, samples []protocol.Sample) {
	for i := len(samples) - 1; i >= 0; i-- {
		v := samples[i].Values
		lat, okLat := numOf(v, "lat", "latitude")
		lon, okLon := numOf(v, "lon", "lng", "longitude")
		if okLat && okLon {
			in.setReportedLocation(ctx, dev, lat, lon)
			return
		}
	}
}

func numOf(v map[string]protocol.Value, keys ...string) (float64, bool) {
	for _, k := range keys {
		if x, ok := v[k]; ok && x.Num != nil {
			return *x.Num, true
		}
	}
	return 0, false
}

// locationFromJSON reads {"lat":..,"lon":..} (or latitude/longitude) from a status or attributes payload.
func (in *Ingest) locationFromJSON(ctx context.Context, dev *broker.Device, payload []byte) {
	var m map[string]any
	if json.Unmarshal(payload, &m) != nil {
		return
	}
	f := func(keys ...string) (float64, bool) {
		for _, k := range keys {
			if x, ok := m[k].(float64); ok {
				return x, true
			}
		}
		return 0, false
	}
	lat, okLat := f("lat", "latitude")
	lon, okLon := f("lon", "lng", "longitude")
	if okLat && okLon {
		in.setReportedLocation(ctx, dev, lat, lon)
	}
}

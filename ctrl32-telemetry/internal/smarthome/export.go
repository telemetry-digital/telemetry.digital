package smarthome

// Publishing ctrl32 values to Home Assistant (spec 005 US6, research R11): a bridge can publish selected datastreams
// to its broker with Home Assistant discovery, so an existing Home Assistant shows them as sensors. Configs are
// retained; every new value goes to the datastream's state topic.

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/ingest"
)

type exportInfo struct {
	name, unit, deviceClass string
}

func (s *source) exports() map[string]bool {
	out := map[string]bool{}
	if list, ok := s.settings["publish_datastreams"].([]any); ok {
		for _, x := range list {
			if id, ok := x.(string); ok {
				out[id] = true
			}
		}
	}
	return out
}

func stateTopic(src *source, dsID string) string {
	return "ctrl32/" + strings.ReplaceAll(src.id[len(src.id)-12:], "-", "") + "/" + dsID + "/state"
}

// publishConfigs announces the exported datastreams of a bridge (called when the bridge connects).
func (m *Manager) publishConfigs(ctx context.Context, src *source) {
	ids := src.exports()
	if len(ids) == 0 || src.bridge == nil {
		return
	}
	for id := range ids {
		var name, unit, quantity, key string
		if err := m.pool.QueryRow(ctx, `SELECT a.name, ds.unit, ds.quantity, ds.key FROM core.datastream ds JOIN core.asset a ON a.id = ds.asset_id
			WHERE ds.id = $1::uuid AND ds.org_id = $2::uuid`, id, src.org).Scan(&name, &unit, &quantity, &key); err != nil {
			continue
		}
		cfg := map[string]any{"name": key, "unique_id": "ctrl32_" + strings.ReplaceAll(id, "-", ""), "state_topic": stateTopic(src, id),
			"device": map[string]any{"identifiers": []string{"ctrl32_" + src.org}, "name": "ctrl32 " + name, "manufacturer": "ctrl32 telemetry"},
			"origin": map[string]any{"name": "ctrl32 telemetry"}}
		if unit != "" {
			cfg["unit_of_measurement"] = unit
			cfg["state_class"] = "measurement"
		}
		if dc := haDeviceClass(quantity); dc != "" {
			cfg["device_class"] = dc
		}
		b, _ := json.Marshal(cfg)
		_ = src.bridge.publishRaw(ctx, src.prefix+"/sensor/ctrl32_"+strings.ReplaceAll(id, "-", "")+"/config", b, true)
	}
}

// unpublish removes the discovery config of a datastream that is no longer exported.
func (m *Manager) unpublish(ctx context.Context, src *source, dsID string) {
	if src.bridge != nil {
		_ = src.bridge.publishRaw(ctx, src.prefix+"/sensor/ctrl32_"+strings.ReplaceAll(dsID, "-", "")+"/config", nil, true)
	}
}

func haDeviceClass(q string) string {
	switch q {
	case "temperature", "humidity", "pressure", "power", "energy", "voltage", "current", "illuminance", "battery", "co2", "frequency", "speed", "weight":
		if q == "co2" {
			return "carbon_dioxide"
		}
		return q
	case "power_active":
		return "power"
	case "energy_active_import":
		return "energy"
	}
	return ""
}

// OnObservations publishes new values of exported datastreams (the manager is an ingest sink).
func (m *Manager) OnObservations(ctx context.Context, obs []ingest.Observation) {
	m.mu.RLock()
	var bridges []*source
	for _, s := range m.sources {
		if s.kind == "mqtt_bridge" && s.bridge != nil && len(s.exports()) > 0 {
			bridges = append(bridges, s)
		}
	}
	m.mu.RUnlock()
	for _, s := range bridges {
		ids := s.exports()
		for _, o := range obs {
			if o.OrgID != s.org || !ids[o.DatastreamID] || o.Value == nil || !s.bridge.connected.Load() {
				continue
			}
			_ = s.bridge.publishRaw(ctx, stateTopic(s, o.DatastreamID), []byte(strconv.FormatFloat(*o.Value, 'g', -1, 64)), true)
		}
	}
}

// Package smarthome integrates devices that announce themselves with Home Assistant MQTT discovery (spec 005):
// discovery configs are normalized (abbreviations, "~" base topic, device info, availability), states are parsed per
// component with a safe Jinja subset, and control actions become MQTT payloads per component.
package smarthome

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Components the engine understands (FR-007).
var Components = map[string]bool{
	"sensor": true, "binary_sensor": true, "switch": true, "light": true, "cover": true, "fan": true, "lock": true,
	"climate": true, "number": true, "select": true, "button": true, "text": true, "device_tracker": true, "event": true,
	"scene": true, "valve": true, "siren": true, "humidifier": true, "update": true,
}

// DeviceInfo is the device block of a discovery config.
type DeviceInfo struct {
	Identifiers   []string `json:"identifiers,omitempty"`
	Name          string   `json:"name,omitempty"`
	Manufacturer  string   `json:"manufacturer,omitempty"`
	Model         string   `json:"model,omitempty"`
	SWVersion     string   `json:"sw_version,omitempty"`
	HWVersion     string   `json:"hw_version,omitempty"`
	SuggestedArea string   `json:"suggested_area,omitempty"`
	ViaDevice     string   `json:"via_device,omitempty"`
	ConfigURL     string   `json:"configuration_url,omitempty"`
}

// Discovered is one entity announced by a discovery message.
type Discovered struct {
	Component string         `json:"component"`
	NodeID    string         `json:"node_id,omitempty"`
	ObjectID  string         `json:"object_id"`
	UniqueID  string         `json:"unique_id"`
	Name      string         `json:"name"`
	Config    map[string]any `json:"config"` // full key names, "~" expanded
	Device    DeviceInfo     `json:"device"`
	Topic     string         `json:"topic"` // the config topic
}

// DeviceKey identifies the device an entity belongs to.
func (d *Discovered) DeviceKey() string {
	if len(d.Device.Identifiers) > 0 && d.Device.Identifiers[0] != "" {
		return d.Device.Identifiers[0]
	}
	if d.NodeID != "" {
		return d.NodeID
	}
	return d.UniqueID
}

// DeviceName is the name to show for the device.
func (d *Discovered) DeviceName() string {
	if d.Device.Name != "" {
		return d.Device.Name
	}
	if d.NodeID != "" {
		return d.NodeID
	}
	return d.Name
}

// ErrNotDiscovery means the topic is not a discovery config topic.
var ErrNotDiscovery = errors.New("not a discovery topic")

var idPart = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ParseTopic splits "{prefix}/{component}/[{node_id}/]{object_id}/config".
func ParseTopic(prefix, topic string) (component, nodeID, objectID string, err error) {
	rest, ok := strings.CutPrefix(topic, prefix+"/")
	if !ok || !strings.HasSuffix(rest, "/config") {
		return "", "", "", ErrNotDiscovery
	}
	parts := strings.Split(strings.TrimSuffix(rest, "/config"), "/")
	switch len(parts) {
	case 2:
		component, objectID = parts[0], parts[1]
	case 3:
		component, nodeID, objectID = parts[0], parts[1], parts[2]
	default:
		return "", "", "", ErrNotDiscovery
	}
	for _, p := range []string{component, objectID} {
		if !idPart.MatchString(p) {
			return "", "", "", ErrNotDiscovery
		}
	}
	if nodeID != "" && !idPart.MatchString(nodeID) {
		return "", "", "", ErrNotDiscovery
	}
	return component, nodeID, objectID, nil
}

// ParseConfig parses a discovery message. An empty payload means removal: it returns no entities and removed=true.
// Device-based discovery ("{prefix}/device/{id}/config" with "components") returns one entity per component.
func ParseConfig(prefix, topic string, payload []byte) (list []Discovered, removed bool, err error) {
	component, nodeID, objectID, err := ParseTopic(prefix, topic)
	if err != nil {
		return nil, false, err
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return nil, true, nil
	}
	if len(payload) > 64<<10 {
		return nil, false, errors.New("discovery config larger than 64 KB")
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, false, fmt.Errorf("discovery config is not a JSON object: %w", err)
	}
	cfg := Expand(raw)
	if component == "device" {
		comps, _ := cfg["components"].(map[string]any)
		if len(comps) == 0 {
			return nil, false, errors.New("device discovery without components")
		}
		shared := map[string]any{}
		for k, v := range cfg {
			switch k {
			case "components", "device", "origin", "platform":
			default:
				shared[k] = v
			}
		}
		keys := make([]string, 0, len(comps))
		for k := range comps {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, id := range keys {
			c, ok := comps[id].(map[string]any)
			if !ok {
				continue
			}
			c = Expand(c)
			if len(c) == 0 || c["platform"] == nil {
				continue // a component being removed or without platform
			}
			for k, v := range shared {
				if _, ok := c[k]; !ok {
					c[k] = v
				}
			}
			if _, ok := c["device"]; !ok && cfg["device"] != nil {
				c["device"] = cfg["device"]
			}
			d, err := finish(fmt.Sprint(c["platform"]), objectID, id, topic, c)
			if err != nil {
				return nil, false, err
			}
			list = append(list, d)
		}
		return list, false, nil
	}
	d, err := finish(component, nodeID, objectID, topic, cfg)
	if err != nil {
		return nil, false, err
	}
	return []Discovered{d}, false, nil
}

func finish(component, nodeID, objectID, topic string, cfg map[string]any) (Discovered, error) {
	expandBase(cfg)
	d := Discovered{Component: component, NodeID: nodeID, ObjectID: objectID, Config: cfg, Topic: topic}
	d.UniqueID = str(cfg, "unique_id")
	if d.UniqueID == "" {
		d.UniqueID = component + "." + strings.Trim(nodeID+"."+objectID, ".")
	}
	if dev, ok := cfg["device"].(map[string]any); ok {
		d.Device = DeviceInfo{Name: str(dev, "name"), Manufacturer: str(dev, "manufacturer"), Model: str(dev, "model"),
			SWVersion: str(dev, "sw_version"), HWVersion: str(dev, "hw_version"), SuggestedArea: str(dev, "suggested_area"),
			ViaDevice: str(dev, "via_device"), ConfigURL: str(dev, "configuration_url")}
		switch ids := dev["identifiers"].(type) {
		case string:
			d.Device.Identifiers = []string{ids}
		case []any:
			for _, x := range ids {
				d.Device.Identifiers = append(d.Device.Identifiers, fmt.Sprint(x))
			}
		}
		if len(d.Device.Identifiers) == 0 {
			if cns, ok := dev["connections"].([]any); ok {
				for _, c := range cns {
					if pair, ok := c.([]any); ok && len(pair) == 2 {
						d.Device.Identifiers = append(d.Device.Identifiers, fmt.Sprint(pair[0])+":"+fmt.Sprint(pair[1]))
					}
				}
			}
		}
	}
	d.Name = str(cfg, "name")
	if _, has := cfg["name"]; has && cfg["name"] == nil {
		d.Name = "" // entity named after its device (HA "has_entity_name")
	}
	if d.Name == "" {
		d.Name = d.Device.Name
	}
	if d.Name == "" {
		d.Name = strings.ReplaceAll(objectID, "_", " ")
	}
	if !Components[component] {
		return d, fmt.Errorf("component %q is not supported", component)
	}
	return d, nil
}

// expandBase replaces "~" at the start or end of topic values with the base topic.
func expandBase(cfg map[string]any) {
	base, ok := cfg["~"].(string)
	if !ok || base == "" {
		return
	}
	fix := func(s string) string {
		if strings.HasPrefix(s, "~") {
			return base + s[1:]
		}
		if strings.HasSuffix(s, "~") {
			return s[:len(s)-1] + base
		}
		return s
	}
	for k, v := range cfg {
		if s, ok := v.(string); ok && (strings.HasSuffix(k, "_topic") || k == "topic") {
			cfg[k] = fix(s)
		}
	}
	if av, ok := cfg["availability"].([]any); ok {
		for _, a := range av {
			if m, ok := a.(map[string]any); ok {
				if s, ok := m["topic"].(string); ok {
					m["topic"] = fix(s)
				}
			}
		}
	}
}

// Expand replaces abbreviated keys with their full names (in the config, the device, the origin and availability).
func Expand(raw map[string]any) map[string]any {
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		if full, ok := abbreviations[k]; ok {
			k = full
		}
		out[k] = v
	}
	if dev, ok := out["device"].(map[string]any); ok {
		out["device"] = expandWith(dev, deviceAbbreviations)
	}
	if o, ok := out["origin"].(map[string]any); ok {
		out["origin"] = expandWith(o, originAbbreviations)
	}
	if av, ok := out["availability"].([]any); ok {
		for i, a := range av {
			if m, ok := a.(map[string]any); ok {
				av[i] = expandWith(m, abbreviations)
			}
		}
	}
	return out
}

func expandWith(m map[string]any, table map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if full, ok := table[k]; ok {
			k = full
		}
		out[k] = v
	}
	return out
}

func str(m map[string]any, k string) string {
	switch v := m[k].(type) {
	case string:
		return v
	case float64, bool:
		return ToString(v)
	}
	return ""
}

func parseJSON(b []byte) (any, bool) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || (b[0] != '{' && b[0] != '[' && b[0] != '"') {
		// numbers, true/false are left as plain values; value_json of "21.5" is still usable through value
		var f float64
		if json.Unmarshal(b, &f) == nil {
			return f, true
		}
		return nil, false
	}
	var v any
	if json.Unmarshal(b, &v) != nil {
		return nil, false
	}
	return v, true
}

package smarthome

// Native adapters (spec 006): devices that do not speak Home Assistant discovery out of the box are described by
// discovery configs synthesized here, so adoption, entities, commands, history and flows work exactly as for
// discovered devices.
//
//   - Shelly Gen2+/Gen3 (Plus, Pro, Mini, Gen3/Gen4): recognized by their RPC notifications (<prefix>/events/rpc) or
//     status topics (<prefix>/status/switch:0); ctrl32 asks the device for Shelly.GetDeviceInfo and
//     Shelly.GetStatus over MQTT RPC and creates entities for its components. Partial NotifyStatus updates are merged
//     and delivered as <prefix>/status/<component> to the entities; commands are RPC calls on <prefix>/rpc.
//   - Tasmota's native discovery (tasmota/discovery/<MAC>/config and /sensors, the default with SetOption19 0):
//     relays, lights (dimmer, colour temperature, RGB) and the sensors of tele/<topic>/SENSOR.
//
// The synthesized configs live under <discovery prefix>/<component>/ctrl32native_<device>/<object>/config; they are
// stored with the discovered device like published ones but never published.

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const nativeNode = "ctrl32native_"

type nativeState struct {
	reply  string                  // our RPC reply prefix (Shelly answers on <reply>/rpc)
	shelly map[string]*shellyDev   // device topic prefix -> device
	reqs   map[int]shellyReq       // RPC request id -> what it asked
	sent   map[string]string       // synthesized config topic -> payload (unchanged configs are not re-processed)
	tasCfg map[string]*tasmotaConf // MAC -> native Tasmota config
	tasSns map[string]map[string]any
	nextID int
}

type shellyReq struct{ prefix, method string }

type shellyDev struct {
	info   map[string]any
	status map[string]map[string]any
	asked  time.Time
	ready  bool // configs synthesized
}

var shellyComp = regexp.MustCompile(`^(.+)/status/((switch|cover|light|rgb|rgbw|input|temperature|humidity|devicepower|illuminance|voltmeter|pm1|em1):\d+)$`)

func (m *Manager) nat(src *source) *nativeState {
	if src.nat == nil {
		id := strings.ReplaceAll(src.id, "-", "")
		if len(id) > 12 {
			id = id[len(id)-12:]
		}
		src.nat = &nativeState{reply: "ctrl32-" + id, shelly: map[string]*shellyDev{}, reqs: map[int]shellyReq{}, sent: map[string]string{},
			tasCfg: map[string]*tasmotaConf{}, tasSns: map[string]map[string]any{}}
	}
	return src.nat
}

// nativeTopics lists what a bridge subscribes to for the native adapters.
func (s *source) nativeTopics() []string {
	id := strings.ReplaceAll(s.id, "-", "")
	if len(id) > 12 {
		id = id[len(id)-12:]
	}
	return []string{"+/events/rpc", "+/status/+", "ctrl32-" + id + "/rpc", "tasmota/discovery/#"}
}

// native handles the adapters' messages; it reports true when the message is consumed.
func (m *Manager) native(ctx context.Context, src *source, topic string, payload []byte) bool {
	n := m.nat(src)
	switch {
	case topic == n.reply+"/rpc":
		m.shellyReply(ctx, src, payload)
		return true
	case strings.HasSuffix(topic, "/events/rpc"):
		m.shellyEvent(ctx, src, strings.TrimSuffix(topic, "/events/rpc"), payload)
		return false
	case strings.HasPrefix(topic, "tasmota/discovery/"):
		m.tasmotaDiscovery(ctx, src, topic, payload)
		return true
	}
	if sm := shellyComp.FindStringSubmatch(topic); sm != nil {
		var st map[string]any
		if json.Unmarshal(payload, &st) == nil {
			d := m.shellyDevice(ctx, src, sm[1])
			d.status[sm[2]] = st
		}
	}
	return false
}

// ---- Shelly Gen2+ -------------------------------------------------------------------------------------------------

func (m *Manager) shellyDevice(ctx context.Context, src *source, prefix string) *shellyDev {
	n := m.nat(src)
	d := n.shelly[prefix]
	if d == nil {
		if len(n.shelly) >= 500 {
			return &shellyDev{status: map[string]map[string]any{}} // not tracked
		}
		d = &shellyDev{status: map[string]map[string]any{}}
		n.shelly[prefix] = d
	}
	if !d.ready && time.Since(d.asked) > 5*time.Minute {
		d.asked = time.Now()
		m.shellyCall(ctx, src, prefix, "Shelly.GetDeviceInfo")
		m.shellyCall(ctx, src, prefix, "Shelly.GetStatus")
	}
	return d
}

func (m *Manager) shellyCall(ctx context.Context, src *source, prefix, method string) {
	n := m.nat(src)
	n.nextID++
	id := n.nextID
	n.reqs[id] = shellyReq{prefix, method}
	if len(n.reqs) > 1000 {
		for k := range n.reqs {
			if k < id-500 {
				delete(n.reqs, k)
			}
		}
	}
	b, _ := json.Marshal(map[string]any{"id": id, "src": n.reply, "method": method})
	if err := m.PublishRaw(ctx, src.org, src.id, prefix+"/rpc", b, false); err != nil {
		m.log.Debug("shelly request not sent", "source", src.name, "device", prefix, "err", err)
	}
}

func (m *Manager) shellyEvent(ctx context.Context, src *source, prefix string, payload []byte) {
	var ev struct {
		Src    string         `json:"src"`
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if json.Unmarshal(payload, &ev) != nil || !strings.HasPrefix(strings.ToLower(ev.Src), "shelly") {
		return
	}
	d := m.shellyDevice(ctx, src, prefix)
	if ev.Method != "NotifyStatus" && ev.Method != "NotifyFullStatus" {
		return
	}
	for comp, v := range ev.Params {
		part, ok := v.(map[string]any)
		if !ok || !strings.Contains(comp, ":") {
			continue
		}
		cur := d.status[comp]
		if cur == nil || ev.Method == "NotifyFullStatus" {
			cur = map[string]any{}
		}
		mergeJSON(cur, part)
		d.status[comp] = cur
		if d.ready {
			b, _ := json.Marshal(cur)
			m.dispatch(ctx, src, prefix+"/status/"+comp, b)
		}
	}
}

func mergeJSON(dst, src map[string]any) {
	for k, v := range src {
		if sub, ok := v.(map[string]any); ok {
			if cur, ok := dst[k].(map[string]any); ok {
				mergeJSON(cur, sub)
				continue
			}
		}
		dst[k] = v
	}
}

func (m *Manager) shellyReply(ctx context.Context, src *source, payload []byte) {
	var r struct {
		ID     int            `json:"id"`
		Result map[string]any `json:"result"`
	}
	n := m.nat(src)
	if json.Unmarshal(payload, &r) != nil {
		return
	}
	req, ok := n.reqs[r.ID]
	if !ok {
		return
	}
	delete(n.reqs, r.ID)
	d := n.shelly[req.prefix]
	if d == nil || r.Result == nil {
		return
	}
	switch req.method {
	case "Shelly.GetDeviceInfo":
		d.info = r.Result
	case "Shelly.GetStatus":
		for comp, v := range r.Result {
			if st, ok := v.(map[string]any); ok && strings.Contains(comp, ":") {
				d.status[comp] = st
			}
		}
	}
	if d.info == nil || len(d.status) == 0 {
		return
	}
	cfgs := ShellyConfigs(req.prefix, n.reply, d.info, d.status)
	for _, c := range cfgs {
		m.synthesize(ctx, src, c)
	}
	d.ready = true
	comps := make([]string, 0, len(d.status))
	for comp := range d.status {
		comps = append(comps, comp)
	}
	sort.Strings(comps)
	for _, comp := range comps {
		b, _ := json.Marshal(d.status[comp])
		m.dispatch(ctx, src, req.prefix+"/status/"+comp, b)
	}
	b := []byte("true")
	m.dispatch(ctx, src, req.prefix+"/online", b) // it answered, so it is online
}

// NativeConfig is one synthesized discovery config.
type NativeConfig struct {
	Component, Device, Object string
	Config                    map[string]any
}

// synthesize processes a synthesized config like a published one; it reports false when it is unchanged.
func (m *Manager) synthesize(ctx context.Context, src *source, c NativeConfig) bool {
	topic := fmt.Sprintf("%s/%s/%s%s/%s/config", src.prefix, c.Component, nativeNode, extRe.ReplaceAllString(c.Device, "_"), extRe.ReplaceAllString(c.Object, "_"))
	b, _ := json.Marshal(c.Config)
	n := m.nat(src)
	if n.sent[topic] == string(b) {
		return false
	}
	n.sent[topic] = string(b)
	m.discover(ctx, src, topic, b)
	return true
}

type shellySensor struct {
	path            []string
	name, unit, cls string
	stateClass, cat string
}

var shellySensors = map[string][]shellySensor{
	"switch": {{path: []string{"apower"}, name: "Power", unit: "W", cls: "power", stateClass: "measurement"},
		{path: []string{"voltage"}, name: "Voltage", unit: "V", cls: "voltage", stateClass: "measurement"},
		{path: []string{"current"}, name: "Current", unit: "A", cls: "current", stateClass: "measurement"},
		{path: []string{"aenergy", "total"}, name: "Energy", unit: "Wh", cls: "energy", stateClass: "total_increasing"},
		{path: []string{"temperature", "tC"}, name: "Temperature", unit: "°C", cls: "temperature", stateClass: "measurement", cat: "diagnostic"}},
	"cover": {{path: []string{"apower"}, name: "Power", unit: "W", cls: "power", stateClass: "measurement"},
		{path: []string{"aenergy", "total"}, name: "Energy", unit: "Wh", cls: "energy", stateClass: "total_increasing"}},
	"light": {{path: []string{"apower"}, name: "Power", unit: "W", cls: "power", stateClass: "measurement"}},
	"rgb":   {{path: []string{"apower"}, name: "Power", unit: "W", cls: "power", stateClass: "measurement"}},
	"rgbw":  {{path: []string{"apower"}, name: "Power", unit: "W", cls: "power", stateClass: "measurement"}},
	"pm1": {{path: []string{"apower"}, name: "Power", unit: "W", cls: "power", stateClass: "measurement"},
		{path: []string{"voltage"}, name: "Voltage", unit: "V", cls: "voltage", stateClass: "measurement"},
		{path: []string{"current"}, name: "Current", unit: "A", cls: "current", stateClass: "measurement"},
		{path: []string{"aenergy", "total"}, name: "Energy", unit: "Wh", cls: "energy", stateClass: "total_increasing"}},
	"em1": {{path: []string{"act_power"}, name: "Power", unit: "W", cls: "power", stateClass: "measurement"},
		{path: []string{"voltage"}, name: "Voltage", unit: "V", cls: "voltage", stateClass: "measurement"},
		{path: []string{"current"}, name: "Current", unit: "A", cls: "current", stateClass: "measurement"}},
	"temperature": {{path: []string{"tC"}, name: "Temperature", unit: "°C", cls: "temperature", stateClass: "measurement"}},
	"humidity":    {{path: []string{"rh"}, name: "Humidity", unit: "%", cls: "humidity", stateClass: "measurement"}},
	"devicepower": {{path: []string{"battery", "percent"}, name: "Battery", unit: "%", cls: "battery", stateClass: "measurement", cat: "diagnostic"}},
	"illuminance": {{path: []string{"lux"}, name: "Illuminance", unit: "lx", cls: "illuminance", stateClass: "measurement"}},
	"voltmeter":   {{path: []string{"voltage"}, name: "Voltage", unit: "V", cls: "voltage", stateClass: "measurement"}},
}

func jsonPath(m map[string]any, path []string) (any, bool) {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = mm[p]; !ok {
			return nil, false
		}
	}
	return cur, cur != nil
}

func tplPath(path []string) string {
	var b strings.Builder
	b.WriteString("value_json")
	for _, p := range path {
		b.WriteString("['" + p + "']")
	}
	return b.String()
}

// ShellyConfigs synthesizes the discovery configs of a Shelly Gen2+ device from Shelly.GetDeviceInfo and
// Shelly.GetStatus. reply is the RPC reply prefix put into commands.
func ShellyConfigs(prefix, reply string, info map[string]any, status map[string]map[string]any) []NativeConfig {
	id := ToString(info["id"])
	if id == "" || id == "<undefined>" {
		id = prefix
	}
	name := strings.TrimSpace(ToString(info["name"]))
	if info["name"] == nil || name == "" {
		name = id
	}
	model := ToString(info["model"])
	if app := ToString(info["app"]); app != "" && info["app"] != nil {
		model = strings.TrimSpace(app + " " + model)
	}
	dev := map[string]any{"identifiers": []string{id}, "name": name, "manufacturer": "Shelly", "model": model}
	if v, ok := info["ver"].(string); ok {
		dev["sw_version"] = v
	}
	avail := map[string]any{"availability_topic": prefix + "/online", "payload_available": "true", "payload_not_available": "false"}
	base := func(n string, uniq string) map[string]any {
		c := map[string]any{"name": n, "unique_id": uniq, "device": dev}
		for k, v := range avail {
			c[k] = v
		}
		return c
	}
	rpc := func(method, params string) string {
		return `{"id":0,"src":"` + reply + `","method":"` + method + `","params":{` + params + `}}`
	}
	count := map[string]int{}
	comps := make([]string, 0, len(status))
	for comp := range status {
		comps = append(comps, comp)
		count[strings.SplitN(comp, ":", 2)[0]]++
	}
	sort.Strings(comps)
	var out []NativeConfig
	for _, comp := range comps {
		kind, idx, _ := strings.Cut(comp, ":")
		st := status[comp]
		stateTopic := prefix + "/status/" + comp
		label := map[string]string{"switch": "Switch", "cover": "Cover", "light": "Light", "rgb": "Light", "rgbw": "Light", "input": "Input",
			"pm1": "Meter", "em1": "Meter"}[kind]
		ename := name
		if kind == "input" {
			ename = label + " " + idx // shown with the device name
		} else if count[kind] > 1 {
			ename = fmt.Sprintf("%s %s %s", name, label, idx)
		}
		uniq := id + "_" + kind + "_" + idx
		switch kind {
		case "switch":
			c := base(ename, uniq)
			c["state_topic"] = stateTopic
			c["value_template"] = "{{ 'ON' if value_json.output else 'OFF' }}"
			c["command_topic"] = prefix + "/rpc"
			c["payload_on"], c["payload_off"] = "true", "false"
			c["state_on"], c["state_off"] = "ON", "OFF"
			c["command_template"] = rpc("Switch.Set", `"id":`+idx+`,"on":{{ value }}`)
			out = append(out, NativeConfig{"switch", id, kind + "_" + idx, c})
		case "cover":
			c := base(ename, uniq)
			c["device_class"] = "shutter"
			c["state_topic"] = stateTopic
			c["value_template"] = "{{ value_json.state }}"
			c["command_topic"] = prefix + "/rpc"
			c["payload_open"], c["payload_close"], c["payload_stop"] = "Open", "Close", "Stop"
			c["command_template"] = rpc("Cover.{{ value }}", `"id":`+idx)
			c["position_topic"] = stateTopic
			c["position_template"] = "{% if value_json.current_pos is number %}{{ value_json.current_pos }}{% endif %}"
			c["set_position_topic"] = prefix + "/rpc"
			c["set_position_template"] = rpc("Cover.GoToPosition", `"id":`+idx+`,"pos":{{ position | int }}`)
			out = append(out, NativeConfig{"cover", id, kind + "_" + idx, c})
		case "light", "rgb", "rgbw":
			method := map[string]string{"light": "Light.Set", "rgb": "RGB.Set", "rgbw": "RGBW.Set"}[kind]
			c := base(ename, uniq)
			c["schema"] = "template"
			c["command_topic"] = prefix + "/rpc"
			on := `"id":` + idx + `,"on":true{% if brightness is defined %},"brightness":{{ [ (brightness * 100 / 255) | round(0) | int, 1 ] | max }}{% endif %}`
			if kind != "light" {
				on += `{% if red is defined %},"rgb":[{{ red | int }},{{ green | int }},{{ blue | int }}]{% endif %}`
				c["red_template"] = "{{ value_json.rgb[0] }}"
				c["green_template"] = "{{ value_json.rgb[1] }}"
				c["blue_template"] = "{{ value_json.rgb[2] }}"
			}
			c["command_on_template"] = rpc(method, on)
			c["command_off_template"] = rpc(method, `"id":`+idx+`,"on":false`)
			c["state_topic"] = stateTopic
			c["state_template"] = "{{ 'on' if value_json.output else 'off' }}"
			c["brightness_template"] = "{{ (value_json.brightness * 255 / 100) | round(0) }}"
			out = append(out, NativeConfig{"light", id, kind + "_" + idx, c})
		case "input":
			if _, ok := st["state"].(bool); !ok {
				continue // a button input reports events, not a state
			}
			c := base(ename, uniq)
			c["state_topic"] = stateTopic
			c["value_template"] = "{{ 'ON' if value_json.state else 'OFF' }}"
			out = append(out, NativeConfig{"binary_sensor", id, kind + "_" + idx, c})
		}
		for _, s := range shellySensors[kind] {
			if _, ok := jsonPath(st, s.path); !ok {
				continue
			}
			sn := s.name
			if ename != name || count[kind] > 1 {
				sn = ename + " " + s.name
			}
			c := base(sn, uniq+"_"+strings.Join(s.path, "_"))
			c["state_topic"] = stateTopic
			c["value_template"] = "{{ " + tplPath(s.path) + " }}"
			c["unit_of_measurement"] = s.unit
			c["device_class"] = s.cls
			c["state_class"] = s.stateClass
			if s.cat != "" {
				c["entity_category"] = s.cat
			}
			out = append(out, NativeConfig{"sensor", id, kind + "_" + idx + "_" + strings.Join(s.path, "_"), c})
		}
	}
	return out
}

// ---- Tasmota native discovery ---------------------------------------------------------------------------------------

type tasmotaConf struct {
	IP    string         `json:"ip"`
	DN    string         `json:"dn"`
	FN    []*string      `json:"fn"`
	HN    string         `json:"hn"`
	MAC   string         `json:"mac"`
	MD    string         `json:"md"`
	Ofln  string         `json:"ofln"`
	Onln  string         `json:"onln"`
	State []string       `json:"state"`
	SW    string         `json:"sw"`
	T     string         `json:"t"`
	FT    string         `json:"ft"`
	TP    []string       `json:"tp"`
	RL    []int          `json:"rl"`
	SO    map[string]any `json:"so"`
	LtSt  int            `json:"lt_st"`
}

func (m *Manager) tasmotaDiscovery(ctx context.Context, src *source, topic string, payload []byte) {
	parts := strings.Split(topic, "/")
	if len(parts) != 4 {
		return
	}
	mac, kind := parts[2], parts[3]
	n := m.nat(src)
	switch kind {
	case "config":
		if len(payload) == 0 {
			delete(n.tasCfg, mac)
			m.removeNative(ctx, src, mac)
			return
		}
		var c tasmotaConf
		if json.Unmarshal(payload, &c) != nil || c.T == "" {
			return
		}
		n.tasCfg[mac] = &c
	case "sensors":
		var s struct {
			SN map[string]any `json:"sn"`
		}
		if json.Unmarshal(payload, &s) != nil {
			return
		}
		n.tasSns[mac] = s.SN
	default:
		return
	}
	c := n.tasCfg[mac]
	if c == nil {
		return
	}
	changed := false
	for _, x := range TasmotaConfigs(c, n.tasSns[mac]) {
		changed = m.synthesize(ctx, src, x) || changed
	}
	if changed {
		// ask for the current state; the answer arrives on stat/…/RESULT
		_ = m.PublishRaw(ctx, src.org, src.id, c.topic(0)+"STATE", nil, false)
	}
}

// removeNative removes the synthesized entities of a device (its config was cleared).
func (m *Manager) removeNative(ctx context.Context, src *source, device string) {
	rows, err := m.pool.Query(ctx, `SELECT DISTINCT config_topic FROM core.entity WHERE connector_id = $1::uuid AND config_topic LIKE $2 AND status <> 'removed'`,
		src.id, src.prefix+"/%/"+nativeNode+extRe.ReplaceAllString(device, "_")+"/%")
	if err != nil {
		return
	}
	var topics []string
	for rows.Next() {
		var t string
		if rows.Scan(&t) == nil {
			topics = append(topics, t)
		}
	}
	rows.Close()
	n := m.nat(src)
	for _, t := range topics {
		delete(n.sent, t)
		m.removeConfig(ctx, src, t)
	}
}

// topic returns the full topic prefix for cmnd (0), stat (1) or tele (2), ending with "/".
func (c *tasmotaConf) topic(i int) string {
	pfx := []string{"cmnd", "stat", "tele"}[i]
	if i < len(c.TP) && c.TP[i] != "" {
		pfx = c.TP[i]
	}
	ft := c.FT
	if ft == "" {
		ft = "%prefix%/%topic%/"
	}
	id := c.MAC
	if len(id) > 6 {
		id = id[len(id)-6:]
	}
	s := strings.NewReplacer("%prefix%", pfx, "%topic%", c.T, "%hostname%", c.HN, "%id%", id).Replace(ft)
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}
	return s
}

func (c *tasmotaConf) so(k string) bool {
	f, _ := toNum(c.SO[k])
	return f == 1
}

var tasmotaUnits = map[string][3]string{ // field -> unit, device class, state class
	"Temperature": {"°C", "temperature", "measurement"}, "DewPoint": {"°C", "temperature", "measurement"},
	"Humidity": {"%", "humidity", "measurement"}, "Pressure": {"hPa", "pressure", "measurement"},
	"SeaPressure": {"hPa", "pressure", "measurement"}, "Power": {"W", "power", "measurement"},
	"ApparentPower": {"VA", "apparent_power", "measurement"}, "ReactivePower": {"var", "reactive_power", "measurement"},
	"Factor": {"", "power_factor", "measurement"}, "Voltage": {"V", "voltage", "measurement"}, "Current": {"A", "current", "measurement"},
	"Total": {"kWh", "energy", "total_increasing"}, "Today": {"kWh", "energy", "total_increasing"}, "Yesterday": {"kWh", "energy", ""},
	"Illuminance": {"lx", "illuminance", "measurement"}, "CarbonDioxide": {"ppm", "carbon_dioxide", "measurement"},
	"eCO2": {"ppm", "carbon_dioxide", "measurement"}, "TVOC": {"ppb", "volatile_organic_compounds_parts", "measurement"},
	"PM2.5": {"µg/m³", "pm25", "measurement"}, "PM10": {"µg/m³", "pm10", "measurement"}, "PM1": {"µg/m³", "pm1", "measurement"},
	"Distance": {"cm", "distance", "measurement"}, "Frequency": {"Hz", "frequency", "measurement"},
}

// TasmotaConfigs synthesizes discovery configs from Tasmota's native discovery (config and sensors).
func TasmotaConfigs(c *tasmotaConf, sensors map[string]any) []NativeConfig {
	mac := strings.ToUpper(strings.ReplaceAll(c.MAC, ":", ""))
	name := c.DN
	if name == "" {
		name = c.T
	}
	dev := map[string]any{"identifiers": []string{mac}, "name": name, "manufacturer": "Tasmota", "model": c.MD, "sw_version": c.SW}
	if c.IP != "" {
		dev["configuration_url"] = "http://" + c.IP + "/"
	}
	onln, ofln := c.Onln, c.Ofln
	if onln == "" {
		onln, ofln = "Online", "Offline"
	}
	cmnd, stat, tele := c.topic(0), c.topic(1), c.topic(2)
	base := func(n, uniq string) map[string]any {
		return map[string]any{"name": n, "unique_id": uniq, "device": dev, "availability_topic": tele + "LWT",
			"payload_available": onln, "payload_not_available": ofln}
	}
	off, on := "OFF", "ON"
	if len(c.State) >= 2 {
		off, on = c.State[0], c.State[1]
	}
	relays := 0
	for _, r := range c.RL {
		if r == 1 || r == 2 {
			relays++
		}
	}
	var out []NativeConfig
	for i, r := range c.RL {
		if r != 1 && r != 2 {
			continue
		}
		key := "POWER" + strconv.Itoa(i+1)
		if relays == 1 {
			key = "POWER"
		}
		n := name
		if i < len(c.FN) && c.FN[i] != nil && strings.TrimSpace(*c.FN[i]) != "" {
			n = *c.FN[i]
		} else if relays > 1 {
			n = fmt.Sprintf("%s %d", name, i+1)
		}
		uniq := fmt.Sprintf("%s_RL_%d", mac, i+1)
		cfg := base(n, uniq)
		cfg["command_topic"] = cmnd + key
		cfg["payload_on"], cfg["payload_off"] = on, off
		state := "{{ value_json['" + key + "'] | default('') }}"
		if relays == 1 {
			state = "{{ value_json['POWER'] | default(value_json['POWER1'] | default('')) }}"
		}
		comp := "switch"
		if r == 2 || c.so("30") {
			comp = "light"
			cfg["state_topic"] = stat + "RESULT"
			cfg["state_value_template"] = state
			if r == 2 && c.LtSt >= 1 {
				cfg["brightness_command_topic"] = cmnd + "Dimmer"
				cfg["brightness_scale"] = 100
				cfg["brightness_state_topic"] = stat + "RESULT"
				cfg["brightness_value_template"] = "{{ value_json.Dimmer | default('') }}"
				cfg["on_command_type"] = "brightness"
			}
			if r == 2 && (c.LtSt == 2 || c.LtSt == 5) {
				cfg["color_temp_command_topic"] = cmnd + "CT"
				cfg["color_temp_state_topic"] = stat + "RESULT"
				cfg["color_temp_value_template"] = "{{ value_json.CT | default('') }}"
				cfg["min_mireds"], cfg["max_mireds"] = 153, 500
			}
			if r == 2 && c.LtSt >= 3 {
				cfg["rgb_command_topic"] = cmnd + "Color2"
				cfg["rgb_state_topic"] = stat + "RESULT"
				if c.so("17") { // colours as decimal "r,g,b"
					cfg["rgb_value_template"] = "{% if value_json.Color is defined %}{{ value_json.Color.split(',')[0:3] | join(',') }}{% endif %}"
				} else {
					cfg["rgb_value_template"] = "{% if value_json.Color is defined %}{{ value_json.Color[0:2] | int(0, 16) }},{{ value_json.Color[2:4] | int(0, 16) }},{{ value_json.Color[4:6] | int(0, 16) }}{% endif %}"
				}
			}
		} else {
			cfg["state_topic"] = stat + "RESULT"
			cfg["value_template"] = state
		}
		out = append(out, NativeConfig{comp, mac, fmt.Sprintf("RL_%d", i+1), cfg})
	}
	// sensors: {"AM2301":{"Temperature":21.5,"Humidity":40},"ENERGY":{"Power":5,…},"TempUnit":"C"}
	tempUnit := "°" + strings.TrimPrefix(ToString(sensors["TempUnit"]), "°")
	if tempUnit == "°" || tempUnit == "°<undefined>" {
		tempUnit = "°C"
	}
	sensorKeys := make([]string, 0, len(sensors))
	for k := range sensors {
		sensorKeys = append(sensorKeys, k)
	}
	sort.Strings(sensorKeys)
	for _, sk := range sensorKeys {
		obj, ok := sensors[sk].(map[string]any)
		if !ok {
			continue
		}
		fields := make([]string, 0, len(obj))
		for f := range obj {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		for _, f := range fields {
			if _, ok := toNum(obj[f]); !ok {
				continue
			}
			label := sk + " " + f // entity names are shown with their device, so they do not repeat it
			if sk == "ENERGY" {
				label = map[string]string{"Total": "Energy total", "Today": "Energy today", "Yesterday": "Energy yesterday"}[f]
				if label == "" {
					label = f
				}
			}
			cfg := base(label, fmt.Sprintf("%s_%s_%s", mac, sk, f))
			cfg["state_topic"] = tele + "SENSOR"
			cfg["value_template"] = "{{ value_json['" + sk + "']['" + f + "'] }}"
			if u, ok := tasmotaUnits[f]; ok {
				unit := u[0]
				if u[1] == "temperature" {
					unit = tempUnit
				}
				if f == "Pressure" || f == "SeaPressure" {
					if pu := ToString(sensors["PressureUnit"]); sensors["PressureUnit"] != nil && pu != "" {
						unit = pu
					}
				}
				if unit != "" {
					cfg["unit_of_measurement"] = unit
				}
				cfg["device_class"] = u[1]
				if u[2] != "" {
					cfg["state_class"] = u[2]
				}
			}
			out = append(out, NativeConfig{"sensor", mac, sk + "_" + f, cfg})
		}
	}
	return out
}

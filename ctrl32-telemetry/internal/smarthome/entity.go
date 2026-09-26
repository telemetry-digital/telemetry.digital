package smarthome

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
)

// Entity is a discovered entity with its compiled templates. Apply turns received messages into state updates;
// Command turns control actions into MQTT publishes.
type Entity struct {
	Component string
	Config    map[string]any

	mu   sync.Mutex
	tpls map[string]*Template
	errs map[string]error
}

// NewEntity prepares an entity from a normalized config.
func NewEntity(component string, cfg map[string]any) *Entity {
	return &Entity{Component: component, Config: cfg, tpls: map[string]*Template{}, errs: map[string]error{}}
}

func (e *Entity) s(k string) string { return str(e.Config, k) }

func (e *Entity) sDef(k, def string) string {
	if _, ok := e.Config[k]; ok {
		return e.s(k)
	}
	return def
}

func (e *Entity) num(k string, def float64) float64 {
	if f, ok := toNum(e.Config[k]); ok {
		return f
	}
	return def
}

func (e *Entity) boolean(k string, def bool) bool {
	if b, ok := e.Config[k].(bool); ok {
		return b
	}
	return def
}

// tpl returns the compiled template of a config key (nil when not set).
func (e *Entity) tpl(k string) (*Template, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if t, ok := e.tpls[k]; ok {
		return t, e.errs[k]
	}
	src := e.s(k)
	if src == "" {
		e.tpls[k] = nil
		return nil, nil
	}
	t, err := ParseTemplate(src)
	e.tpls[k], e.errs[k] = t, err
	return t, err
}

// Check compiles every template of the config and returns the first problem.
func (e *Entity) Check() error {
	for k := range e.Config {
		if strings.HasSuffix(k, "_template") {
			if _, err := e.tpl(k); err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
		}
	}
	return nil
}

// Topics returns the topics the entity reads (state, attributes, availability).
func (e *Entity) Topics() []string {
	seen := map[string]bool{}
	var out []string
	add := func(t string) {
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for k, v := range e.Config {
		if s, ok := v.(string); ok && (strings.HasSuffix(k, "_state_topic") || k == "state_topic" || k == "json_attributes_topic" ||
			k == "availability_topic" || k == "position_topic" || k == "current_temperature_topic" || k == "current_humidity_topic" ||
			k == "action_topic" || k == "tilt_status_topic" || k == "latest_version_topic") {
			add(s)
		}
	}
	for _, a := range e.availability() {
		add(a.topic)
	}
	return out
}

// Update is the effect of one message on an entity.
type Update struct {
	State     *string        // new state, if the message carries one
	Attrs     map[string]any // changed attributes
	Available *bool
}

type avail struct {
	topic, on, off string
	tpl            string
}

func (e *Entity) availability() []avail {
	var out []avail
	if list, ok := e.Config["availability"].([]any); ok {
		for _, x := range list {
			if m, ok := x.(map[string]any); ok {
				a := avail{topic: str(m, "topic"), on: "online", off: "offline", tpl: str(m, "value_template")}
				if v, ok := m["payload_available"]; ok {
					a.on = ToString(v)
				}
				if v, ok := m["payload_not_available"]; ok {
					a.off = ToString(v)
				}
				out = append(out, a)
			}
		}
	}
	if t := e.s("availability_topic"); t != "" {
		out = append(out, avail{topic: t, on: e.sDef("payload_available", "online"), off: e.sDef("payload_not_available", "offline"), tpl: e.s("availability_template")})
	}
	return out
}

// Stateless components have no state topic.
func (e *Entity) Stateless() bool {
	switch e.Component {
	case "button", "scene":
		return true
	}
	return false
}

// Optimistic entities do not report their state; commands are assumed to work.
func (e *Entity) Optimistic() bool {
	if e.Stateless() {
		return true
	}
	if b, ok := e.Config["optimistic"].(bool); ok {
		return b
	}
	return e.s("state_topic") == "" && e.s("position_topic") == "" && e.s("mode_state_topic") == "" && e.s("temperature_state_topic") == ""
}

// render applies a template key (or the identity when missing) to a payload.
func (e *Entity) render(key string, payload []byte, extra map[string]any) (string, error) {
	t, err := e.tpl(key)
	if err != nil {
		return "", err
	}
	if t == nil {
		return string(payload), nil
	}
	vars := PayloadVars(payload)
	for k, v := range extra {
		vars[k] = v
	}
	out, err := t.Render(vars)
	return strings.TrimSpace(out), err
}

// Apply parses a message received on topic. The returned update is empty when the topic is not the entity's.
func (e *Entity) Apply(topic string, payload []byte) (Update, error) {
	var u Update
	var firstErr error
	keep := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	attr := func(k string, v any) {
		if u.Attrs == nil {
			u.Attrs = map[string]any{}
		}
		u.Attrs[k] = v
	}
	set := func(s string) { u.State = &s }

	for _, a := range e.availability() {
		if a.topic != topic {
			continue
		}
		v := string(payload)
		if a.tpl != "" {
			t, err := ParseTemplate(a.tpl)
			if err == nil {
				v, err = t.RenderPayload(payload)
			}
			keep(err)
			v = strings.TrimSpace(v)
		}
		switch v {
		case a.on:
			b := true
			u.Available = &b
		case a.off:
			b := false
			u.Available = &b
		}
	}
	if topic == e.s("json_attributes_topic") {
		v, err := e.render("json_attributes_template", payload, nil)
		keep(err)
		var m map[string]any
		if json.Unmarshal([]byte(v), &m) == nil {
			for k, x := range m {
				attr(k, x)
			}
		}
	}

	isState := topic == e.s("state_topic")
	switch e.Component {
	case "sensor", "text", "select", "event", "update":
		if isState {
			if e.Component == "event" {
				v, err := e.render("value_template", payload, nil)
				keep(err)
				var m map[string]any
				if json.Unmarshal([]byte(v), &m) == nil {
					if t, ok := m["event_type"].(string); ok {
						set(t)
						for k, x := range m {
							attr(k, x)
						}
					}
				}
				break
			}
			if e.Component == "update" {
				v, err := e.render("value_template", payload, nil)
				keep(err)
				var m map[string]any
				if json.Unmarshal([]byte(v), &m) == nil {
					for k, x := range m {
						attr(k, x)
					}
					if iv, ok := m["installed_version"]; ok {
						set(ToString(iv))
					}
				} else {
					attr("installed_version", v)
					set(v)
				}
				break
			}
			v, err := e.render("value_template", payload, nil)
			keep(err)
			if err == nil {
				set(v)
			}
		}
		if e.Component == "update" && topic == e.s("latest_version_topic") {
			v, err := e.render("latest_version_template", payload, nil)
			keep(err)
			attr("latest_version", v)
		}
	case "number":
		if isState {
			v, err := e.render("value_template", payload, nil)
			keep(err)
			if _, ok := toNum(v); ok {
				set(normNum(v))
			}
		}
	case "binary_sensor":
		if isState {
			v, err := e.render("value_template", payload, nil)
			keep(err)
			switch v {
			case e.sDef("payload_on", "ON"):
				set("on")
			case e.sDef("payload_off", "OFF"):
				set("off")
			}
		}
	case "switch", "fan", "humidifier", "siren", "scene":
		if isState {
			v, err := e.render("state_value_template", payload, nil)
			if e.s("state_value_template") == "" {
				v, err = e.render("value_template", payload, nil)
			}
			keep(err)
			on, off := e.sDef("state_on", e.sDef("payload_on", "ON")), e.sDef("state_off", e.sDef("payload_off", "OFF"))
			switch v {
			case on:
				set("on")
			case off:
				set("off")
			}
		}
		if e.Component == "fan" && topic == e.s("percentage_state_topic") {
			v, err := e.render("percentage_value_template", payload, nil)
			keep(err)
			if f, ok := toNum(v); ok {
				lo, hi := e.num("speed_range_min", 1), e.num("speed_range_max", 100)
				pct := 0.0
				if f > 0 && hi > lo {
					pct = math.Round((f - lo + 1) / (hi - lo + 1) * 100)
				}
				attr("percentage", pct)
			}
		}
		if (e.Component == "fan" || e.Component == "humidifier") && topic == e.s("preset_mode_state_topic") {
			v, err := e.render("preset_mode_value_template", payload, nil)
			keep(err)
			attr("preset_mode", v)
		}
		if e.Component == "humidifier" && topic == e.s("target_humidity_state_topic") {
			v, err := e.render("target_humidity_state_template", payload, nil)
			keep(err)
			if f, ok := toNum(v); ok {
				attr("humidity", f)
			}
		}
	case "lock":
		if isState {
			v, err := e.render("value_template", payload, nil)
			keep(err)
			for st, def := range map[string]string{"locked": "LOCKED", "unlocked": "UNLOCKED", "locking": "LOCKING", "unlocking": "UNLOCKING", "jammed": "JAMMED", "open": "OPEN", "opening": "OPENING"} {
				if v == e.sDef("state_"+st, def) {
					set(st)
				}
			}
		}
	case "cover", "valve":
		if isState {
			v, err := e.render("value_template", payload, nil)
			keep(err)
			for st, def := range map[string]string{"open": "open", "opening": "opening", "closed": "closed", "closing": "closing", "stopped": "stopped"} {
				if v == e.sDef("state_"+st, def) {
					set(st)
				}
			}
			if v == e.sDef("payload_open", "OPEN") && u.State == nil {
				set("open")
			}
			if v == e.sDef("payload_close", "CLOSE") && u.State == nil {
				set("closed")
			}
			if e.Component == "valve" && e.boolean("reports_position", false) {
				if f, ok := toNum(v); ok {
					e.position(f, attr, set)
				}
			}
		}
		if topic == e.s("position_topic") {
			v, err := e.render("position_template", payload, nil)
			keep(err)
			if f, ok := toNum(v); ok {
				e.position(f, attr, set)
			}
		}
		if topic == e.s("tilt_status_topic") {
			v, err := e.render("tilt_status_template", payload, nil)
			keep(err)
			if f, ok := toNum(v); ok {
				attr("tilt_position", f)
			}
		}
	case "light":
		e.applyLight(topic, payload, set, attr, keep)
	case "climate":
		if topic == e.s("mode_state_topic") {
			v, err := e.render("mode_state_template", payload, nil)
			keep(err)
			set(v)
		}
		for _, k := range []struct{ topic, tpl, attr string }{
			{"temperature_state_topic", "temperature_state_template", "temperature"},
			{"current_temperature_topic", "current_temperature_template", "current_temperature"},
			{"current_humidity_topic", "current_humidity_template", "current_humidity"},
			{"target_humidity_state_topic", "target_humidity_state_template", "humidity"},
			{"temperature_low_state_topic", "temperature_low_state_template", "target_temp_low"},
			{"temperature_high_state_topic", "temperature_high_state_template", "target_temp_high"},
		} {
			if topic == e.s(k.topic) {
				v, err := e.render(k.tpl, payload, nil)
				keep(err)
				if f, ok := toNum(v); ok {
					attr(k.attr, f)
				}
			}
		}
		for _, k := range []struct{ topic, tpl, attr string }{
			{"action_topic", "action_template", "hvac_action"},
			{"fan_mode_state_topic", "fan_mode_state_template", "fan_mode"},
			{"swing_mode_state_topic", "swing_mode_state_template", "swing_mode"},
			{"preset_mode_state_topic", "preset_mode_value_template", "preset_mode"},
		} {
			if topic == e.s(k.topic) {
				v, err := e.render(k.tpl, payload, nil)
				keep(err)
				attr(k.attr, v)
			}
		}
		if e.s("mode_state_topic") == "" && isState {
			v, err := e.render("value_template", payload, nil)
			keep(err)
			set(v)
		}
	case "device_tracker":
		if isState {
			v, err := e.render("value_template", payload, nil)
			keep(err)
			switch v {
			case e.sDef("payload_home", "home"):
				set("home")
			case e.sDef("payload_not_home", "not_home"):
				set("not_home")
			default:
				set(v)
			}
		}
	}
	return u, firstErr
}

func (e *Entity) position(f float64, attr func(string, any), set func(string)) {
	open, closed := e.num("position_open", 100), e.num("position_closed", 0)
	pct := f
	if open != closed {
		pct = math.Round((f - closed) / (open - closed) * 100)
	}
	pct = math.Max(0, math.Min(100, pct))
	attr("position", pct)
	if e.s("state_topic") == "" {
		if pct > 0 {
			set("open")
		} else {
			set("closed")
		}
	}
}

func (e *Entity) applyLight(topic string, payload []byte, set func(string), attr func(string, any), keep func(error)) {
	scale := e.num("brightness_scale", 255)
	if e.s("schema") == "json" {
		if topic != e.s("state_topic") {
			return
		}
		var m map[string]any
		if err := json.Unmarshal(payload, &m); err != nil {
			keep(err)
			return
		}
		switch s := ToString(m["state"]); s {
		case "ON", "on":
			set("on")
		case "OFF", "off":
			set("off")
		}
		if b, ok := toNum(m["brightness"]); ok && scale > 0 {
			attr("brightness", math.Round(b/scale*255))
		}
		if ct, ok := toNum(m["color_temp"]); ok {
			attr("color_temp", e.mireds(ct))
		}
		if c, ok := m["color"].(map[string]any); ok {
			if r, ok := toNum(c["r"]); ok {
				g, _ := toNum(c["g"])
				b, _ := toNum(c["b"])
				attr("rgb_color", []any{r, g, b})
			}
			if x, ok := toNum(c["x"]); ok {
				y, _ := toNum(c["y"])
				attr("xy_color", []any{x, y})
			}
			if h, ok := toNum(c["h"]); ok {
				s, _ := toNum(c["s"])
				attr("hs_color", []any{h, s})
			}
		}
		if cm, ok := m["color_mode"].(string); ok {
			attr("color_mode", cm)
		} else if _, hasCT := m["color_temp"]; hasCT && m["color"] == nil {
			attr("color_mode", "color_temp") // devices without color_mode: the one colour field reported decides
		} else if m["color"] != nil && !hasCT {
			attr("color_mode", "rgb")
		}
		if fx, ok := m["effect"].(string); ok {
			attr("effect", fx)
		}
		return
	}
	if e.s("schema") == "template" {
		if topic == e.s("state_topic") {
			v, err := e.render("state_template", payload, nil)
			keep(err)
			if v == "on" || v == "off" {
				set(v)
			}
			if bt := e.s("brightness_template"); bt != "" {
				b, err := e.render("brightness_template", payload, nil)
				keep(err)
				if f, ok := toNum(b); ok {
					attr("brightness", f)
				}
			}
			if e.s("red_template") != "" && e.s("green_template") != "" && e.s("blue_template") != "" {
				rgb := make([]any, 0, 3)
				for _, k := range []string{"red_template", "green_template", "blue_template"} {
					v, err := e.render(k, payload, nil)
					keep(err)
					if f, ok := toNum(v); ok {
						rgb = append(rgb, f)
					}
				}
				if len(rgb) == 3 {
					attr("rgb_color", rgb)
				}
			}
			if e.s("color_temp_template") != "" {
				v, err := e.render("color_temp_template", payload, nil)
				keep(err)
				if f, ok := toNum(v); ok {
					attr("color_temp", e.mireds(f))
				}
			}
		}
		return
	}
	if topic == e.s("state_topic") {
		v, err := e.render("state_value_template", payload, nil)
		keep(err)
		switch v {
		case e.sDef("payload_on", "ON"):
			set("on")
		case e.sDef("payload_off", "OFF"):
			set("off")
		}
	}
	if topic == e.s("brightness_state_topic") {
		v, err := e.render("brightness_value_template", payload, nil)
		keep(err)
		if b, ok := toNum(v); ok && scale > 0 {
			attr("brightness", math.Round(b/scale*255))
		}
	}
	if topic == e.s("color_temp_state_topic") {
		v, err := e.render("color_temp_value_template", payload, nil)
		keep(err)
		if ct, ok := toNum(v); ok {
			attr("color_temp", e.mireds(ct))
		}
	}
	if topic == e.s("rgb_state_topic") {
		v, err := e.render("rgb_value_template", payload, nil)
		keep(err)
		if parts := strings.Split(v, ","); len(parts) == 3 {
			rgb := make([]any, 3)
			for i, p := range parts {
				f, _ := toNum(p)
				rgb[i] = f
			}
			attr("rgb_color", rgb)
		}
	}
	if topic == e.s("effect_state_topic") {
		v, err := e.render("effect_value_template", payload, nil)
		keep(err)
		attr("effect", v)
	}
}

// ---- commands ----------------------------------------------------------------------------------------------------

// Publish is one MQTT message a command produces.
type Publish struct {
	Topic   string
	Payload string
	Retain  bool
	QoS     byte
}

// Params are the arguments of a control action.
type Params struct {
	Value       any       `json:"value,omitempty"`       // number/select/text value
	Brightness  *float64  `json:"brightness,omitempty"`  // 0–100 %
	RGB         []float64 `json:"rgb,omitempty"`         // [r,g,b] 0–255
	ColorTemp   *float64  `json:"color_temp,omitempty"`  // mireds
	Position    *float64  `json:"position,omitempty"`    // 0–100 %
	Temperature *float64  `json:"temperature,omitempty"` // target
	Mode        string    `json:"mode,omitempty"`        // hvac mode, preset
	Percentage  *float64  `json:"percentage,omitempty"`  // fan
}

// Expectation is the state that confirms a command.
type Expectation struct {
	Attr  string // "" = the state
	Value string
}

// Services lists the control actions of the entity's component.
func (e *Entity) Services() []string {
	switch e.Component {
	case "switch", "siren", "humidifier":
		return []string{"turn_on", "turn_off", "toggle"}
	case "light":
		return []string{"turn_on", "turn_off", "toggle"}
	case "fan":
		return []string{"turn_on", "turn_off", "toggle", "set_percentage"}
	case "cover":
		return []string{"open", "close", "stop", "set_position"}
	case "valve":
		return []string{"open", "close", "stop", "set_position"}
	case "lock":
		return []string{"lock", "unlock", "open"}
	case "climate":
		return []string{"set_temperature", "set_hvac_mode", "turn_off", "turn_on"}
	case "number":
		return []string{"set_value"}
	case "select":
		return []string{"select_option"}
	case "text":
		return []string{"set_value"}
	case "button":
		return []string{"press"}
	case "scene":
		return []string{"turn_on"}
	}
	return nil
}

// ErrNotControllable is returned for services an entity does not offer.
var ErrNotControllable = errors.New("this entity cannot do that")

// Command builds the MQTT messages of a control action. state is the entity's current state (for toggle).
func (e *Entity) Command(service string, p Params, state string) ([]Publish, *Expectation, error) {
	ok := false
	for _, s := range e.Services() {
		ok = ok || s == service
	}
	if !ok {
		return nil, nil, fmt.Errorf("%w: %s on %s", ErrNotControllable, service, e.Component)
	}
	if service == "toggle" {
		service = "turn_on"
		if state == "on" {
			service = "turn_off"
		}
	}
	retain := e.boolean("retain", false)
	qos := byte(e.num("qos", 0))
	pub := func(topicKey, tplKey string, value any, extra map[string]any) (Publish, error) {
		topic := e.s(topicKey)
		if topic == "" {
			return Publish{}, fmt.Errorf("%w: no %s", ErrNotControllable, topicKey)
		}
		payload := ToString(value)
		if t, err := e.tpl(tplKey); err != nil {
			return Publish{}, err
		} else if t != nil {
			vars := map[string]any{"value": value}
			for k, v := range extra {
				vars[k] = v
			}
			if payload, err = t.Render(vars); err != nil {
				return Publish{}, err
			}
		}
		return Publish{Topic: topic, Payload: payload, Retain: retain, QoS: qos}, nil
	}
	one := func(topicKey, tplKey string, value any, exp *Expectation) ([]Publish, *Expectation, error) {
		m, err := pub(topicKey, tplKey, value, nil)
		if err != nil {
			return nil, nil, err
		}
		return []Publish{m}, exp, nil
	}
	num := func(v any, what string) (float64, error) {
		f, ok := toNum(v)
		if !ok {
			return 0, fmt.Errorf("%s must be a number", what)
		}
		return f, nil
	}

	switch e.Component {
	case "switch", "siren", "humidifier", "scene":
		if service == "turn_on" {
			if e.Component == "scene" {
				return one("command_topic", "command_template", e.sDef("payload_on", "ON"), nil)
			}
			return one("command_topic", "command_template", e.sDef("payload_on", "ON"), &Expectation{Value: "on"})
		}
		return one("command_topic", "command_template", e.sDef("payload_off", "OFF"), &Expectation{Value: "off"})
	case "button":
		return one("command_topic", "command_template", e.sDef("payload_press", "PRESS"), nil)
	case "fan":
		switch service {
		case "turn_on":
			return one("command_topic", "command_template", e.sDef("payload_on", "ON"), &Expectation{Value: "on"})
		case "turn_off":
			return one("command_topic", "command_template", e.sDef("payload_off", "OFF"), &Expectation{Value: "off"})
		default:
			if p.Percentage == nil {
				return nil, nil, errors.New("percentage is required")
			}
			lo, hi := e.num("speed_range_min", 1), e.num("speed_range_max", 100)
			v := math.Ceil(*p.Percentage / 100 * (hi - lo + 1))
			if *p.Percentage > 0 {
				v += lo - 1
			}
			return one("percentage_command_topic", "percentage_command_template", v, &Expectation{Attr: "percentage", Value: normNum(ToString(*p.Percentage))})
		}
	case "lock":
		m := map[string][3]string{"lock": {"payload_lock", "LOCK", "locked"}, "unlock": {"payload_unlock", "UNLOCK", "unlocked"}, "open": {"payload_open", "OPEN", "open"}}[service]
		return one("command_topic", "command_template", e.sDef(m[0], m[1]), &Expectation{Value: m[2]})
	case "cover", "valve":
		switch service {
		case "open":
			return one("command_topic", "command_template", e.sDef("payload_open", "OPEN"), &Expectation{Value: "open"})
		case "close":
			exp := "closed"
			return one("command_topic", "command_template", e.sDef("payload_close", "CLOSE"), &Expectation{Value: exp})
		case "stop":
			return one("command_topic", "command_template", e.sDef("payload_stop", "STOP"), nil)
		default:
			if p.Position == nil {
				return nil, nil, errors.New("position is required")
			}
			open, closed := e.num("position_open", 100), e.num("position_closed", 0)
			v := math.Round(closed + *p.Position/100*(open-closed))
			topic := "set_position_topic"
			if e.Component == "valve" {
				topic = "command_topic"
			}
			msgs, _, err := func() ([]Publish, *Expectation, error) {
				m, err := pub(topic, "set_position_template", v, map[string]any{"position": v})
				return []Publish{m}, nil, err
			}()
			return msgs, &Expectation{Attr: "position", Value: normNum(ToString(*p.Position))}, err
		}
	case "number", "text", "select":
		if p.Value == nil {
			return nil, nil, errors.New("value is required")
		}
		v := p.Value
		if e.Component == "number" {
			f, err := num(v, "value")
			if err != nil {
				return nil, nil, err
			}
			lo, hi := e.num("min", 1), e.num("max", 100)
			if f < lo || f > hi {
				return nil, nil, fmt.Errorf("value must be between %s and %s", ToString(lo), ToString(hi))
			}
			v = f
		}
		if e.Component == "select" {
			found := false
			if opts, ok := e.Config["options"].([]any); ok {
				for _, o := range opts {
					found = found || ToString(o) == ToString(v)
				}
			}
			if !found {
				return nil, nil, fmt.Errorf("%q is not one of the options", ToString(v))
			}
		}
		if e.Component == "text" {
			s := ToString(v)
			if max := int(e.num("max", 255)); len([]rune(s)) > max {
				return nil, nil, fmt.Errorf("text longer than %d characters", max)
			}
		}
		return one("command_topic", "command_template", v, &Expectation{Value: normNum(ToString(v))})
	case "climate":
		switch service {
		case "set_temperature":
			if p.Temperature == nil {
				return nil, nil, errors.New("temperature is required")
			}
			t := *p.Temperature
			if t < e.num("min_temp", 7) || t > e.num("max_temp", 35) {
				return nil, nil, fmt.Errorf("temperature must be between %s and %s", ToString(e.num("min_temp", 7)), ToString(e.num("max_temp", 35)))
			}
			return one("temperature_command_topic", "temperature_command_template", t, &Expectation{Attr: "temperature", Value: normNum(ToString(t))})
		case "set_hvac_mode", "turn_off", "turn_on":
			mode := p.Mode
			if service == "turn_off" {
				mode = "off"
			}
			if service == "turn_on" && mode == "" {
				mode = "heat"
				if modes, ok := e.Config["modes"].([]any); ok {
					for _, m := range modes {
						if ToString(m) != "off" {
							mode = ToString(m)
							break
						}
					}
				}
			}
			if e.s("mode_command_topic") == "" && e.s("power_command_topic") != "" && (mode == "off" || service == "turn_on") {
				pl := e.sDef("payload_on", "ON")
				if mode == "off" {
					pl = e.sDef("payload_off", "OFF")
				}
				return one("power_command_topic", "power_command_template", pl, nil)
			}
			return one("mode_command_topic", "mode_command_template", mode, &Expectation{Value: mode})
		}
	case "light":
		return e.lightCommand(service, p, pub)
	}
	return nil, nil, fmt.Errorf("%w: %s on %s", ErrNotControllable, service, e.Component)
}

func (e *Entity) lightCommand(service string, p Params, pub func(string, string, any, map[string]any) (Publish, error)) ([]Publish, *Expectation, error) {
	scale := e.num("brightness_scale", 255)
	onExp := &Expectation{Value: "on"}
	if e.s("schema") == "json" {
		m := map[string]any{}
		if service == "turn_off" {
			m["state"] = e.sDef("payload_off", "OFF")
		} else {
			m["state"] = e.sDef("payload_on", "ON")
			if p.Brightness != nil {
				m["brightness"] = math.Round(*p.Brightness / 100 * scale)
			}
			if len(p.RGB) == 3 {
				m["color"] = map[string]any{"r": p.RGB[0], "g": p.RGB[1], "b": p.RGB[2]}
			}
			if p.ColorTemp != nil {
				m["color_temp"] = e.deviceColorTemp(*p.ColorTemp)
			}
		}
		b, _ := json.Marshal(m)
		topic := e.s("command_topic")
		if topic == "" {
			return nil, nil, fmt.Errorf("%w: no command_topic", ErrNotControllable)
		}
		exp := onExp
		if service == "turn_off" {
			exp = &Expectation{Value: "off"}
		}
		return []Publish{{Topic: topic, Payload: string(b), Retain: e.boolean("retain", false), QoS: byte(e.num("qos", 0))}}, exp, nil
	}
	if e.s("schema") == "template" {
		key := "command_on_template"
		exp := onExp
		vars := map[string]any{"state": "on"}
		if service == "turn_off" {
			key, exp, vars = "command_off_template", &Expectation{Value: "off"}, map[string]any{"state": "off"}
		}
		if p.Brightness != nil {
			vars["brightness"] = math.Round(*p.Brightness / 100 * 255)
		}
		if len(p.RGB) == 3 {
			vars["red"], vars["green"], vars["blue"] = p.RGB[0], p.RGB[1], p.RGB[2]
		}
		if p.ColorTemp != nil {
			vars["color_temp"] = e.deviceColorTemp(*p.ColorTemp)
		}
		m, err := pub("command_topic", key, "", vars)
		return []Publish{m}, exp, err
	}
	// default schema
	if service == "turn_off" {
		m, err := pub("command_topic", "", e.sDef("payload_off", "OFF"), nil)
		return []Publish{m}, &Expectation{Value: "off"}, err
	}
	var out []Publish
	on, err := pub("command_topic", "", e.sDef("payload_on", "ON"), nil)
	if err != nil {
		return nil, nil, err
	}
	var extra []Publish
	if p.Brightness != nil && e.s("brightness_command_topic") != "" {
		m, err := pub("brightness_command_topic", "brightness_command_template", math.Round(*p.Brightness/100*scale), nil)
		if err != nil {
			return nil, nil, err
		}
		extra = append(extra, m)
	}
	if len(p.RGB) == 3 && e.s("rgb_command_topic") != "" {
		rgb := fmt.Sprintf("%s,%s,%s", ToString(p.RGB[0]), ToString(p.RGB[1]), ToString(p.RGB[2]))
		m, err := pub("rgb_command_topic", "rgb_command_template", rgb, map[string]any{"red": p.RGB[0], "green": p.RGB[1], "blue": p.RGB[2]})
		if err != nil {
			return nil, nil, err
		}
		extra = append(extra, m)
	}
	if p.ColorTemp != nil && e.s("color_temp_command_topic") != "" {
		m, err := pub("color_temp_command_topic", "color_temp_command_template", e.deviceColorTemp(*p.ColorTemp), nil)
		if err != nil {
			return nil, nil, err
		}
		extra = append(extra, m)
	}
	switch e.sDef("on_command_type", "last") {
	case "first":
		out = append([]Publish{on}, extra...)
	case "brightness":
		if len(extra) > 0 {
			out = extra
		} else {
			out = []Publish{on}
		}
	default:
		out = append(extra, on)
	}
	return out, onExp, nil
}

// kelvin reports whether the light's colour temperatures are in kelvin (color_temp_kelvin) rather than mireds.
func (e *Entity) kelvin() bool { return e.boolean("color_temp_kelvin", false) }

// mireds converts a colour temperature reported by the light to mireds (the unit of the color_temp attribute).
func (e *Entity) mireds(v float64) float64 {
	if e.kelvin() && v > 0 {
		return math.Round(1e6 / v)
	}
	return v
}

// deviceColorTemp converts mireds to the unit the light expects.
func (e *Entity) deviceColorTemp(m float64) float64 {
	if e.kelvin() && m > 0 {
		return math.Round(1e6 / m)
	}
	return math.Round(m)
}

// LightColor describes the colour controls of a light: RGB, colour temperature and its range in mireds.
type LightColor struct {
	RGB       bool    `json:"rgb,omitempty"`
	ColorTemp bool    `json:"color_temp,omitempty"`
	MinMireds float64 `json:"min_mireds,omitempty"`
	MaxMireds float64 `json:"max_mireds,omitempty"`
}

// Color reports the colour controls of a light entity (nil for other components or lights without colour).
func (e *Entity) Color() *LightColor {
	if e.Component != "light" {
		return nil
	}
	c := &LightColor{}
	switch e.s("schema") {
	case "json":
		if modes, ok := e.Config["supported_color_modes"].([]any); ok {
			for _, m := range modes {
				switch ToString(m) {
				case "rgb", "rgbw", "rgbww", "hs", "xy":
					c.RGB = true
				case "color_temp":
					c.ColorTemp = true
				}
			}
		}
		c.RGB = c.RGB || e.boolean("rgb", false) || e.boolean("hs", false) || e.boolean("xy", false)
		c.ColorTemp = c.ColorTemp || e.boolean("color_temp", false)
	case "template":
		on := e.s("command_on_template")
		c.RGB = e.s("red_template") != "" || strings.Contains(on, "red")
		c.ColorTemp = e.s("color_temp_template") != "" || strings.Contains(on, "color_temp")
	default:
		c.RGB = e.s("rgb_command_topic") != ""
		c.ColorTemp = e.s("color_temp_command_topic") != ""
	}
	if !c.RGB && !c.ColorTemp {
		return nil
	}
	if c.ColorTemp {
		c.MinMireds, c.MaxMireds = e.num("min_mireds", 153), e.num("max_mireds", 500)
		if e.kelvin() {
			if hi := e.num("max_kelvin", 6535); hi > 0 {
				c.MinMireds = math.Round(1e6 / hi)
			}
			if lo := e.num("min_kelvin", 2000); lo > 0 {
				c.MaxMireds = math.Round(1e6 / lo)
			}
		}
		if c.MinMireds >= c.MaxMireds {
			c.MinMireds, c.MaxMireds = 153, 500
		}
	}
	return c
}

// Matches reports whether an entity's state/attributes meet an expectation.
func (x *Expectation) Matches(state string, attrs map[string]any) bool {
	if x == nil {
		return true
	}
	if x.Attr == "" {
		if x.Value == "open" && state == "opening" || x.Value == "closed" && state == "closing" {
			return false
		}
		return normNum(state) == x.Value
	}
	return normNum(ToString(attrs[x.Attr])) == x.Value
}

func normNum(s string) string {
	if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
		return ToString(f)
	}
	return s
}

// NumericState reports whether a state is a number (recorded as a value) and returns it.
func NumericState(s string) (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f, err == nil && !math.IsInf(f, 0) && !math.IsNaN(f)
}

package smarthome

import (
	"strings"
	"testing"
)

func render(t *testing.T, tpl, payload string) string {
	t.Helper()
	p, err := ParseTemplate(tpl)
	if err != nil {
		t.Fatalf("parse %q: %v", tpl, err)
	}
	out, err := p.RenderPayload([]byte(payload))
	if err != nil {
		t.Fatalf("render %q: %v", tpl, err)
	}
	return out
}

func TestTemplates(t *testing.T) {
	cases := []struct{ tpl, payload, want string }{
		{"{{ value_json.temperature }}", `{"temperature":21.4,"humidity":55}`, "21.4"},
		{"{{ value_json['temperature'] }}", `{"temperature":21}`, "21"},
		{"{{ value_json.POWER }}", `{"POWER":"ON"}`, "ON"},
		{"{{value_json.ENERGY.Power}}", `{"ENERGY":{"Power":12.5}}`, "12.5"},
		{"{{ value_json.list[1] }}", `{"list":[1,2,3]}`, "2"},
		{"{{ value | float * 0.1 }}", `215`, "21.5"},
		{"{{ (value_json.t | float) | round(1) }}", `{"t":"21.456"}`, "21.5"},
		{"{{ value_json.x | default('none') }}", `{}`, "none"},
		{"{{ 'ON' if value_json.state == 'on' else 'OFF' }}", `{"state":"on"}`, "ON"},
		{"{{ 'ON' if value_json.state == 'on' else 'OFF' }}", `{"state":"off"}`, "OFF"},
		{"{% if value_json.contact %}OFF{% else %}ON{% endif %}", `{"contact":false}`, "ON"},
		{"{% if value_json.a > 5 %}big{% elif value_json.a > 2 %}mid{% else %}small{% endif %}", `{"a":3}`, "mid"},
		{"{{ value_json.brightness is defined }}", `{"brightness":10}`, "True"},
		{"{{ value_json.brightness is not defined }}", `{}`, "True"},
		{"{{ value_json.state | lower }}", `{"state":"ON"}`, "on"},
		{"{{ value }}", `online`, "online"},
		{"{{ value_json.a ~ '-' ~ value_json.b }}", `{"a":"x","b":2}`, "x-2"},
		{"{{ float(value) + 1 }}", `1.5`, "2.5"},
		{"{{ value_json.v | int }}", `{"v":"7.9"}`, "7"},
		{"{{ 'on' in value }}", `turn on`, "True"},
		{"{{ value_json.list | length }}", `{"list":[1,2]}`, "2"},
		{"{{- value_json.a -}}", `{"a":1}`, "1"},
		{`{"state": "{{ value }}"}`, `ON`, `{"state": "ON"}`},
		{"{{ value_json.power | round(1, 'floor') }}", `{"power":1.99}`, "1.9"},
		{"{{ value_json.data.temp if value_json.data is defined else 0 }}", `{"x":1}`, "0"},
	}
	for _, c := range cases {
		if got := render(t, c.tpl, c.payload); got != c.want {
			t.Errorf("%s with %s = %q, want %q", c.tpl, c.payload, got, c.want)
		}
	}
}

func TestTemplateErrors(t *testing.T) {
	for _, bad := range []string{"{{ value | nosuchfilter }}", "{% for x in y %}{% endfor %}", "{{ value", "{% if a %}x", "{{ ( }}"} {
		if _, err := ParseTemplate(bad); err == nil {
			t.Errorf("%q should not parse", bad)
		}
	}
	if _, err := ParseTemplate(strings.Repeat("x", MaxTemplateLen+1)); err == nil {
		t.Error("too long template accepted")
	}
	p, _ := ParseTemplate("{{ value_json.a / 0 }}")
	if _, err := p.RenderPayload([]byte(`{"a":1}`)); err == nil {
		t.Error("division by zero accepted")
	}
	// steps limit: a long chain of additions
	long := "{{ " + strings.Repeat("1 + ", 400) + "1 }}"
	if p, err := ParseTemplate(long); err == nil {
		if _, err := p.RenderPayload(nil); err != nil {
			t.Logf("long expression refused at render: %v", err)
		}
	}
}

// zigbee2mqtt announces entities like this (abbreviated keys are also possible).
const z2mTemp = `{"availability":[{"topic":"zigbee2mqtt/bridge/state","value_template":"{{ value_json.state }}"}],
 "device":{"identifiers":["zigbee2mqtt_0x00158d0001a2b3c4"],"manufacturer":"Aqara","model":"Temperature and humidity sensor","name":"Living room sensor","suggested_area":"Living room"},
 "device_class":"temperature","enabled_by_default":true,"name":"Temperature","object_id":"living_room_sensor_temperature",
 "state_class":"measurement","state_topic":"zigbee2mqtt/Living room sensor","unique_id":"0x00158d0001a2b3c4_temperature_zigbee2mqtt",
 "unit_of_measurement":"°C","value_template":"{{ value_json.temperature }}"}`

const z2mLight = `{"availability":[{"topic":"zigbee2mqtt/bridge/state","value_template":"{{ value_json.state }}"}],
 "brightness":true,"brightness_scale":254,"command_topic":"zigbee2mqtt/Hall light/set","device":{"identifiers":["zigbee2mqtt_0x0017880104e45517"],"name":"Hall light"},
 "name":null,"schema":"json","state_topic":"zigbee2mqtt/Hall light","supported_color_modes":["color_temp","xy"],"unique_id":"0x0017880104e45517_light_zigbee2mqtt"}`

// Tasmota with SetOption19 1 (abbreviated keys).
const tasmotaRelay = `{"name":"Garage relay","stat_t":"tele/tasmota_A1B2C3/STATE","avty_t":"tele/tasmota_A1B2C3/LWT","pl_avail":"Online","pl_not_avail":"Offline",
 "cmd_t":"cmnd/tasmota_A1B2C3/POWER","val_tpl":"{{value_json.POWER}}","pl_off":"OFF","pl_on":"ON","uniq_id":"A1B2C3_RL_1",
 "dev":{"ids":["A1B2C3"],"name":"Garage","mdl":"Sonoff Basic","sw":"14.2.0","mf":"Tasmota"}}`

// ESPHome with the "~" base topic.
const espCover = `{"~":"esphome/blinds","name":"Blinds","uniq_id":"esp_blinds","stat_t":"~/cover/blinds/state","cmd_t":"~/cover/blinds/command",
 "pos_t":"~/cover/blinds/position/state","set_pos_t":"~/cover/blinds/position/command","avty_t":"~/status",
 "dev":{"ids":"esp_aabbcc","name":"blinds","sa":"Bedroom","mf":"espressif"}}`

func TestDiscoveryZigbee2mqtt(t *testing.T) {
	list, removed, err := ParseConfig("homeassistant", "homeassistant/sensor/0x00158d0001a2b3c4/temperature/config", []byte(z2mTemp))
	if err != nil || removed || len(list) != 1 {
		t.Fatalf("parse: %v %v %d", err, removed, len(list))
	}
	d := list[0]
	if d.Component != "sensor" || d.UniqueID != "0x00158d0001a2b3c4_temperature_zigbee2mqtt" || d.DeviceKey() != "zigbee2mqtt_0x00158d0001a2b3c4" ||
		d.Device.SuggestedArea != "Living room" || d.Name != "Temperature" || d.DeviceName() != "Living room sensor" {
		t.Fatalf("unexpected %+v", d)
	}
	e := NewEntity(d.Component, d.Config)
	if err := e.Check(); err != nil {
		t.Fatal(err)
	}
	topics := strings.Join(e.Topics(), "|")
	if !strings.Contains(topics, "zigbee2mqtt/Living room sensor") || !strings.Contains(topics, "zigbee2mqtt/bridge/state") {
		t.Fatalf("topics %s", topics)
	}
	u, err := e.Apply("zigbee2mqtt/Living room sensor", []byte(`{"temperature":21.4,"humidity":55,"battery":97}`))
	if err != nil || u.State == nil || *u.State != "21.4" {
		t.Fatalf("state %v %v", u.State, err)
	}
	u, _ = e.Apply("zigbee2mqtt/bridge/state", []byte(`{"state":"offline"}`))
	if u.Available == nil || *u.Available {
		t.Fatalf("availability %+v", u)
	}
	// removal
	if _, removed, _ := ParseConfig("homeassistant", "homeassistant/sensor/0x00158d0001a2b3c4/temperature/config", nil); !removed {
		t.Fatal("empty config should remove")
	}
}

func TestLightJSONSchema(t *testing.T) {
	list, _, err := ParseConfig("homeassistant", "homeassistant/light/0x0017880104e45517/light/config", []byte(z2mLight))
	if err != nil {
		t.Fatal(err)
	}
	d := list[0]
	if d.Name != "Hall light" {
		t.Fatalf("name null should take the device name, got %q", d.Name)
	}
	e := NewEntity(d.Component, d.Config)
	u, _ := e.Apply("zigbee2mqtt/Hall light", []byte(`{"state":"ON","brightness":127,"color_temp":350}`))
	if *u.State != "on" || u.Attrs["brightness"] != 128.0 || u.Attrs["color_temp"] != 350.0 {
		t.Fatalf("light state %v %v", *u.State, u.Attrs)
	}
	b := 40.0
	msgs, exp, err := e.Command("turn_on", Params{Brightness: &b}, "off")
	if err != nil || len(msgs) != 1 {
		t.Fatal(err)
	}
	if msgs[0].Topic != "zigbee2mqtt/Hall light/set" || msgs[0].Payload != `{"brightness":102,"state":"ON"}` {
		t.Fatalf("payload %+v", msgs[0])
	}
	if !exp.Matches("on", nil) || exp.Matches("off", nil) {
		t.Fatal("expectation")
	}
	msgs, _, _ = e.Command("toggle", Params{}, "on")
	if msgs[0].Payload != `{"state":"OFF"}` {
		t.Fatalf("toggle %s", msgs[0].Payload)
	}
}

func TestTasmotaRelay(t *testing.T) {
	list, _, err := ParseConfig("homeassistant", "homeassistant/switch/A1B2C3_RL_1/config", []byte(tasmotaRelay))
	if err != nil {
		t.Fatal(err)
	}
	d := list[0]
	if d.UniqueID != "A1B2C3_RL_1" || d.DeviceKey() != "A1B2C3" || d.Device.Model != "Sonoff Basic" {
		t.Fatalf("%+v", d)
	}
	e := NewEntity(d.Component, d.Config)
	u, _ := e.Apply("tele/tasmota_A1B2C3/STATE", []byte(`{"Time":"2026-09-26T10:00:00","POWER":"ON","Wifi":{"RSSI":80}}`))
	if u.State == nil || *u.State != "on" {
		t.Fatalf("state %+v", u)
	}
	u, _ = e.Apply("tele/tasmota_A1B2C3/LWT", []byte(`Offline`))
	if u.Available == nil || *u.Available {
		t.Fatalf("lwt %+v", u)
	}
	msgs, exp, err := e.Command("turn_on", Params{}, "off")
	if err != nil || msgs[0].Topic != "cmnd/tasmota_A1B2C3/POWER" || msgs[0].Payload != "ON" || exp.Value != "on" {
		t.Fatalf("command %+v %v", msgs, err)
	}
	if _, _, err := e.Command("set_position", Params{}, ""); err == nil {
		t.Fatal("a switch has no position")
	}
}

func TestESPHomeCover(t *testing.T) {
	list, _, err := ParseConfig("homeassistant", "homeassistant/cover/blinds/config", []byte(espCover))
	if err != nil {
		t.Fatal(err)
	}
	d := list[0]
	if d.Config["state_topic"] != "esphome/blinds/cover/blinds/state" || d.DeviceKey() != "esp_aabbcc" || d.Device.SuggestedArea != "Bedroom" {
		t.Fatalf("%+v", d.Config)
	}
	e := NewEntity(d.Component, d.Config)
	u, _ := e.Apply("esphome/blinds/cover/blinds/position/state", []byte("40"))
	if u.Attrs["position"] != 40.0 {
		t.Fatalf("position %+v", u.Attrs)
	}
	u, _ = e.Apply("esphome/blinds/cover/blinds/state", []byte("open"))
	if *u.State != "open" {
		t.Fatal("state")
	}
	p := 75.0
	msgs, exp, err := e.Command("set_position", Params{Position: &p}, "open")
	if err != nil || msgs[0].Topic != "esphome/blinds/cover/blinds/position/command" || msgs[0].Payload != "75" || exp.Attr != "position" {
		t.Fatalf("%+v %v", msgs, err)
	}
	msgs, _, _ = e.Command("stop", Params{}, "")
	if msgs[0].Payload != "STOP" {
		t.Fatal("stop")
	}
}

func TestDeviceDiscoveryAndOthers(t *testing.T) {
	payload := `{"dev":{"ids":"ea334450945afc","name":"Kitchen","mf":"Bla"},"o":{"name":"bla2mqtt"},"stat_t":"sensorBedroom/state","qos":2,
	 "cmps":{"temp1":{"p":"sensor","dev_cla":"temperature","unit_of_measurement":"°C","val_tpl":"{{ value_json.temperature}}","unique_id":"temp01ae_t"},
	         "hum1":{"p":"sensor","dev_cla":"humidity","unit_of_measurement":"%","val_tpl":"{{ value_json.humidity}}","unique_id":"temp01ae_h"}}}`
	list, _, err := ParseConfig("homeassistant", "homeassistant/device/0AFFD2/config", []byte(payload))
	if err != nil || len(list) != 2 {
		t.Fatalf("%v %d", err, len(list))
	}
	if list[0].Config["state_topic"] != "sensorBedroom/state" || list[1].DeviceKey() != "ea334450945afc" {
		t.Fatalf("shared options %+v", list[0].Config)
	}
	e := NewEntity("sensor", list[0].Config)
	if u, _ := e.Apply("sensorBedroom/state", []byte(`{"temperature":23.2,"humidity":43.7}`)); *u.State != "43.7" {
		t.Fatal("device discovery state")
	}
	// climate
	cl := NewEntity("climate", map[string]any{"mode_command_topic": "hvac/mode/set", "mode_state_topic": "hvac/mode", "temperature_command_topic": "hvac/temp/set",
		"temperature_state_topic": "hvac/temp", "current_temperature_topic": "hvac/current", "modes": []any{"off", "heat"}, "min_temp": 10.0, "max_temp": 30.0})
	if u, _ := cl.Apply("hvac/current", []byte("20.5")); u.Attrs["current_temperature"] != 20.5 {
		t.Fatal("current temperature")
	}
	tt := 22.5
	if msgs, _, err := cl.Command("set_temperature", Params{Temperature: &tt}, "heat"); err != nil || msgs[0].Payload != "22.5" {
		t.Fatalf("%v %v", msgs, err)
	}
	tt = 40
	if _, _, err := cl.Command("set_temperature", Params{Temperature: &tt}, "heat"); err == nil {
		t.Fatal("out of range temperature accepted")
	}
	if msgs, _, _ := cl.Command("turn_on", Params{}, "off"); msgs[0].Payload != "heat" {
		t.Fatalf("turn on mode %v", msgs)
	}
	// number and select
	n := NewEntity("number", map[string]any{"command_topic": "n/set", "state_topic": "n", "min": 0.0, "max": 10.0, "command_template": `{"v": {{ value }}}`})
	if msgs, _, err := n.Command("set_value", Params{Value: 7.0}, ""); err != nil || msgs[0].Payload != `{"v": 7}` {
		t.Fatalf("%v %v", msgs, err)
	}
	if _, _, err := n.Command("set_value", Params{Value: 11.0}, ""); err == nil {
		t.Fatal("number out of range accepted")
	}
	s := NewEntity("select", map[string]any{"command_topic": "s/set", "options": []any{"eco", "comfort"}})
	if _, _, err := s.Command("select_option", Params{Value: "turbo"}, ""); err == nil {
		t.Fatal("unknown option accepted")
	}
	// binary sensor and lock
	b := NewEntity("binary_sensor", map[string]any{"state_topic": "door", "value_template": "{{ value_json.contact }}", "payload_on": "False", "payload_off": "True"})
	if u, _ := b.Apply("door", []byte(`{"contact":false}`)); *u.State != "on" {
		t.Fatalf("binary %v", *u.State)
	}
	l := NewEntity("lock", map[string]any{"state_topic": "lock", "command_topic": "lock/set"})
	if u, _ := l.Apply("lock", []byte("LOCKED")); *u.State != "locked" {
		t.Fatal("lock state")
	}
	if msgs, exp, _ := l.Command("unlock", Params{}, "locked"); msgs[0].Payload != "UNLOCK" || exp.Value != "unlocked" {
		t.Fatal("unlock")
	}
	// json attributes
	j := NewEntity("sensor", map[string]any{"state_topic": "x", "json_attributes_topic": "x"})
	if u, _ := j.Apply("x", []byte(`{"battery":90}`)); u.Attrs["battery"] != 90.0 {
		t.Fatalf("attrs %+v", u.Attrs)
	}
	// default-schema light with brightness topic, on_command_type last
	dl := NewEntity("light", map[string]any{"command_topic": "l/set", "brightness_command_topic": "l/bri/set", "brightness_scale": 100.0, "state_topic": "l"})
	b50 := 50.0
	msgs, _, _ := dl.Command("turn_on", Params{Brightness: &b50}, "off")
	if len(msgs) != 2 || msgs[0].Topic != "l/bri/set" || msgs[0].Payload != "50" || msgs[1].Payload != "ON" {
		t.Fatalf("default light %+v", msgs)
	}
	// unsupported component
	if _, _, err := ParseConfig("homeassistant", "homeassistant/camera/x/config", []byte(`{"topic":"a"}`)); err == nil {
		t.Fatal("camera should be unsupported")
	}
	// not a discovery topic
	if _, _, err := ParseConfig("homeassistant", "zigbee2mqtt/x", []byte(`{}`)); err != ErrNotDiscovery {
		t.Fatal("not a discovery topic")
	}
}

func TestTopicFilters(t *testing.T) {
	for _, c := range []struct {
		f, t string
		ok   bool
	}{{"a/b", "a/b", true}, {"a/+", "a/b", true}, {"a/+", "a/b/c", false}, {"a/#", "a/b/c", true}, {"#", "x", true}, {"a/+/c", "a/b/c", true}, {"a/b", "a/c", false}, {"a/#", "a", true}} {
		if Match(c.f, c.t) != c.ok {
			t.Errorf("Match(%q, %q) != %v", c.f, c.t, c.ok)
		}
	}
	for f, ok := range map[string]bool{"a/#": true, "a/#/b": false, "a+/b": false, "+/+": true, "$SYS/#": false, "": false} {
		if ValidFilter(f) != ok {
			t.Errorf("ValidFilter(%q) != %v", f, ok)
		}
	}
	if ValidTopic("a/+") || !ValidTopic("cmnd/x/POWER") {
		t.Error("ValidTopic")
	}
}

func TestLightColor(t *testing.T) {
	list, _, _ := ParseConfig("homeassistant", "homeassistant/light/0x0017880104e45517/light/config", []byte(z2mLight))
	e := NewEntity(list[0].Component, list[0].Config)
	c := e.Color()
	if c == nil || !c.ColorTemp {
		t.Fatalf("z2m light colour %+v", c)
	}
	ct := 250.0
	msgs, _, err := e.Command("turn_on", Params{RGB: []float64{255, 128, 0}, ColorTemp: &ct}, "off")
	if err != nil || !strings.Contains(msgs[0].Payload, `"color":{"b":0,"g":128,"r":255}`) || !strings.Contains(msgs[0].Payload, `"color_temp":250`) {
		t.Fatalf("colour command %v %v", msgs, err)
	}
	// default schema in kelvin
	k := NewEntity("light", map[string]any{"command_topic": "l/set", "state_topic": "l/state", "rgb_command_topic": "l/rgb", "rgb_state_topic": "l/rgb/state",
		"color_temp_command_topic": "l/ct", "color_temp_state_topic": "l/ct/state", "color_temp_kelvin": true, "min_kelvin": 2700.0, "max_kelvin": 6500.0})
	c = k.Color()
	if c == nil || !c.RGB || !c.ColorTemp || c.MinMireds != 154 || c.MaxMireds != 370 {
		t.Fatalf("kelvin light %+v", c)
	}
	msgs, _, _ = k.Command("turn_on", Params{RGB: []float64{1, 2, 3}, ColorTemp: &ct}, "off")
	got := map[string]string{}
	for _, m := range msgs {
		got[m.Topic] = m.Payload
	}
	if got["l/rgb"] != "1,2,3" || got["l/ct"] != "4000" || got["l/set"] != "ON" {
		t.Fatalf("kelvin commands %v", got)
	}
	u, _ := k.Apply("l/ct/state", []byte("2500"))
	if u.Attrs["color_temp"] != 400.0 {
		t.Fatalf("kelvin state %v", u.Attrs)
	}
	// template schema with RGB templates
	tp := NewEntity("light", map[string]any{"schema": "template", "command_topic": "t/rpc", "state_topic": "t/state",
		"command_on_template": `{"on":true{% if red is defined %},"rgb":[{{ red }},{{ green }},{{ blue }}]{% endif %}}`, "command_off_template": `{"on":false}`,
		"state_template": "{{ 'on' if value_json.on else 'off' }}", "red_template": "{{ value_json.rgb[0] }}", "green_template": "{{ value_json.rgb[1] }}", "blue_template": "{{ value_json.rgb[2] }}"})
	if c := tp.Color(); c == nil || !c.RGB || c.ColorTemp {
		t.Fatalf("template colour %+v", c)
	}
	u, err = tp.Apply("t/state", []byte(`{"on":true,"rgb":[10,20,30]}`))
	if err != nil || *u.State != "on" || ToString(u.Attrs["rgb_color"]) == "" {
		t.Fatalf("template state %v %v", u, err)
	}
	if rgb := u.Attrs["rgb_color"].([]any); rgb[2] != 30.0 {
		t.Fatalf("template rgb %v", rgb)
	}
	msgs, _, _ = tp.Command("turn_on", Params{RGB: []float64{9, 8, 7}}, "off")
	if msgs[0].Payload != `{"on":true,"rgb":[9,8,7]}` {
		t.Fatalf("template command %s", msgs[0].Payload)
	}
	if NewEntity("light", map[string]any{"command_topic": "x"}).Color() != nil || NewEntity("switch", nil).Color() != nil {
		t.Fatal("no colour expected")
	}
}

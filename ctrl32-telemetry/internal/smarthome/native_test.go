package smarthome

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// entities turns synthesized configs into entities the way discovery does (through ParseConfig).
func entities(t *testing.T, cfgs []NativeConfig) map[string]*Entity {
	t.Helper()
	out := map[string]*Entity{}
	for _, c := range cfgs {
		b, _ := json.Marshal(c.Config)
		list, _, err := ParseConfig("homeassistant", "homeassistant/"+c.Component+"/"+nativeNode+c.Device+"/"+c.Object+"/config", b)
		if err != nil || len(list) != 1 {
			t.Fatalf("%s: %v %v", c.Object, list, err)
		}
		out[c.Object] = NewEntity(list[0].Component, list[0].Config)
	}
	return out
}

func TestShellyConfigs(t *testing.T) {
	info := map[string]any{"id": "shellyplus2pm-a8032ab12345", "name": nil, "model": "SNSW-102P16EU", "app": "Plus2PM", "ver": "1.4.4", "gen": 2.0}
	var status map[string]map[string]any
	_ = json.Unmarshal([]byte(`{"switch:0":{"id":0,"output":false,"apower":0,"voltage":231.2,"current":0,"aenergy":{"total":1234.5},"temperature":{"tC":41.2}},
		"switch:1":{"id":1,"output":true,"apower":12.5,"voltage":231.2},
		"cover:0":{"id":0,"state":"stopped","current_pos":40,"apower":0},
		"input:0":{"id":0,"state":false},"input:1":{"id":1,"state":null},
		"rgbw:0":{"id":0,"output":true,"brightness":50,"rgb":[255,0,0],"white":0},
		"temperature:0":{"id":0,"tC":22.5},"devicepower:0":{"id":0,"battery":{"V":3.1,"percent":80}},
		"sys":{"uptime":5},"wifi":{"rssi":-60}}`), &status)
	cfgs := ShellyConfigs("shellyplus2pm-a8032ab12345", "ctrl32-abc", info, status)
	es := entities(t, cfgs)
	for _, k := range []string{"switch_0", "switch_1", "cover_0", "input_0", "rgbw_0", "switch_0_apower", "switch_0_aenergy_total", "switch_0_temperature_tC",
		"switch_1_apower", "temperature_0_tC", "devicepower_0_battery_percent"} {
		if es[k] == nil {
			t.Fatalf("missing %s in %v", k, keys(es))
		}
	}
	if es["input_1"] != nil || es["switch_1_current"] != nil {
		t.Fatal("button input or absent field synthesized")
	}
	// switch state and command
	sw := es["switch_0"]
	u, err := sw.Apply("shellyplus2pm-a8032ab12345/status/switch:0", []byte(`{"id":0,"output":true}`))
	if err != nil || *u.State != "on" {
		t.Fatalf("switch state %v %v", u, err)
	}
	msgs, exp, err := sw.Command("turn_off", Params{}, "on")
	if err != nil || msgs[0].Topic != "shellyplus2pm-a8032ab12345/rpc" || exp.Value != "off" {
		t.Fatalf("switch command %v %v", msgs, err)
	}
	var rpc map[string]any
	if err := json.Unmarshal([]byte(msgs[0].Payload), &rpc); err != nil || rpc["method"] != "Switch.Set" || rpc["src"] != "ctrl32-abc" ||
		rpc["params"].(map[string]any)["on"] != false || rpc["params"].(map[string]any)["id"] != 0.0 {
		t.Fatalf("switch rpc %s %v", msgs[0].Payload, err)
	}
	// energy sensor
	u, _ = es["switch_0_aenergy_total"].Apply("shellyplus2pm-a8032ab12345/status/switch:0", []byte(`{"aenergy":{"total":1300.25}}`))
	if *u.State != "1300.25" {
		t.Fatalf("energy %v", *u.State)
	}
	// cover
	cv := es["cover_0"]
	u, _ = cv.Apply("shellyplus2pm-a8032ab12345/status/cover:0", []byte(`{"state":"opening","current_pos":55}`))
	if *u.State != "opening" || u.Attrs["position"] != 55.0 {
		t.Fatalf("cover %v %v", *u.State, u.Attrs)
	}
	pos := 30.0
	msgs, _, _ = cv.Command("set_position", Params{Position: &pos}, "open")
	if !strings.Contains(msgs[0].Payload, `"method":"Cover.GoToPosition","params":{"id":0,"pos":30}`) {
		t.Fatalf("cover position %s", msgs[0].Payload)
	}
	msgs, _, _ = cv.Command("close", Params{}, "open")
	if !strings.Contains(msgs[0].Payload, `"method":"Cover.Close"`) {
		t.Fatalf("cover close %s", msgs[0].Payload)
	}
	// rgbw light with colour
	l := es["rgbw_0"]
	if c := l.Color(); c == nil || !c.RGB {
		t.Fatalf("rgbw colour %+v", c)
	}
	u, _ = l.Apply("shellyplus2pm-a8032ab12345/status/rgbw:0", []byte(`{"output":true,"brightness":50,"rgb":[0,128,255]}`))
	if *u.State != "on" || u.Attrs["brightness"] != 128.0 || fmt.Sprint(u.Attrs["rgb_color"]) != "[0 128 255]" {
		t.Fatalf("rgbw state %v %v", *u.State, u.Attrs)
	}
	b := 40.0
	msgs, _, err = l.Command("turn_on", Params{Brightness: &b, RGB: []float64{1, 2, 3}}, "off")
	if err != nil || !strings.Contains(msgs[0].Payload, `"method":"RGBW.Set","params":{"id":0,"on":true,"brightness":40,"rgb":[1,2,3]}`) {
		t.Fatalf("rgbw command %v %v", msgs, err)
	}
	// availability
	u, _ = sw.Apply("shellyplus2pm-a8032ab12345/online", []byte("false"))
	if u.Available == nil || *u.Available {
		t.Fatalf("availability %+v", u)
	}
}

func keys(m map[string]*Entity) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

const tasmotaNative = `{"ip":"192.168.1.50","dn":"Desk plug","fn":["Desk plug",null,null,null,null,null,null,null],"hn":"tasmota-ABCDEF-1234",
"mac":"A4CF12ABCDEF","md":"Sonoff POW R2","ty":0,"if":0,"ofln":"Offline","onln":"Online","state":["OFF","ON","TOGGLE","HOLD"],"sw":"13.4.0",
"t":"tasmota_ABCDEF","ft":"%prefix%/%topic%/","tp":["cmnd","stat","tele"],"rl":[1,0,0,0,0,0,0,0],"swc":[-1,-1],"swn":[null,null],"btn":[0,0],
"so":{"4":0,"11":0,"13":0,"17":0,"20":0,"30":0,"68":0,"73":0,"82":0,"114":0,"117":0},"lk":0,"lt_st":0,"sho":[],"sht":[],"ver":1}`

func TestTasmotaNative(t *testing.T) {
	var c tasmotaConf
	if err := json.Unmarshal([]byte(tasmotaNative), &c); err != nil {
		t.Fatal(err)
	}
	var sns struct {
		SN map[string]any `json:"sn"`
	}
	_ = json.Unmarshal([]byte(`{"sn":{"Time":"2026-09-26T10:00:00","ENERGY":{"TotalStartTime":"2024-01-01T00:00:00","Total":12.5,"Today":0.3,"Power":42,"Voltage":230,"Current":0.18},
		"AM2301":{"Temperature":21.5,"Humidity":40.1},"TempUnit":"C"},"ver":1}`), &sns)
	es := entities(t, TasmotaConfigs(&c, sns.SN))
	if es["RL_1"] == nil || es["ENERGY_Power"] == nil || es["AM2301_Temperature"] == nil || es["ENERGY_TotalStartTime"] != nil {
		t.Fatalf("configs %v", keys(es))
	}
	rl := es["RL_1"]
	if rl.Component != "switch" {
		t.Fatalf("component %s", rl.Component)
	}
	u, _ := rl.Apply("stat/tasmota_ABCDEF/RESULT", []byte(`{"POWER":"ON"}`))
	if u.State == nil || *u.State != "on" {
		t.Fatalf("relay state %+v", u)
	}
	u, _ = rl.Apply("stat/tasmota_ABCDEF/RESULT", []byte(`{"Dimmer":20}`))
	if u.State != nil {
		t.Fatalf("unrelated RESULT changed the state: %v", *u.State)
	}
	msgs, _, _ := rl.Command("turn_off", Params{}, "on")
	if msgs[0].Topic != "cmnd/tasmota_ABCDEF/POWER" || msgs[0].Payload != "OFF" {
		t.Fatalf("relay command %+v", msgs[0])
	}
	u, _ = es["AM2301_Temperature"].Apply("tele/tasmota_ABCDEF/SENSOR", []byte(`{"AM2301":{"Temperature":22.1,"Humidity":41}}`))
	if *u.State != "22.1" || es["AM2301_Temperature"].Config["unit_of_measurement"] != "°C" {
		t.Fatalf("sensor %v %v", *u.State, es["AM2301_Temperature"].Config)
	}
	if es["ENERGY_Total"].Config["state_class"] != "total_increasing" || es["ENERGY_Total"].Config["unit_of_measurement"] != "kWh" {
		t.Fatalf("energy total %v", es["ENERGY_Total"].Config)
	}
	u, _ = rl.Apply("tele/tasmota_ABCDEF/LWT", []byte("Offline"))
	if u.Available == nil || *u.Available {
		t.Fatal("LWT")
	}

	// RGBCCT light, two relays with the second as light
	c2 := c
	c2.RL = []int{1, 2, 0}
	c2.LtSt = 5
	c2.FN = nil
	es = entities(t, TasmotaConfigs(&c2, nil))
	if es["RL_1"].Component != "switch" || es["RL_2"].Component != "light" {
		t.Fatalf("components %s %s", es["RL_1"].Component, es["RL_2"].Component)
	}
	if es["RL_1"].Config["command_topic"] != "cmnd/tasmota_ABCDEF/POWER1" || es["RL_2"].Config["name"] != "Desk plug 2" {
		t.Fatalf("multi relay %v", es["RL_2"].Config)
	}
	l := es["RL_2"]
	if col := l.Color(); col == nil || !col.RGB || !col.ColorTemp {
		t.Fatalf("light colour %+v", col)
	}
	u, err := l.Apply("stat/tasmota_ABCDEF/RESULT", []byte(`{"POWER2":"ON","Dimmer":60,"Color":"FF8000","CT":300}`))
	if err != nil || *u.State != "on" || u.Attrs["brightness"] != 153.0 || fmt.Sprint(u.Attrs["rgb_color"]) != "[255 128 0]" || u.Attrs["color_temp"] != 300.0 {
		t.Fatalf("light state %v %v %v %q", *u.State, u.Attrs, err, ToString(u.Attrs["rgb_color"]))
	}
	b := 50.0
	msgs, _, _ = l.Command("turn_on", Params{Brightness: &b, RGB: []float64{0, 255, 0}}, "off")
	got := map[string]string{}
	for _, m := range msgs {
		got[m.Topic] = m.Payload
	}
	if got["cmnd/tasmota_ABCDEF/Dimmer"] != "50" || got["cmnd/tasmota_ABCDEF/Color2"] != "0,255,0" {
		t.Fatalf("light commands %v", got)
	}
	// SetOption17: decimal colours
	c2.SO = map[string]any{"17": 1.0}
	es = entities(t, TasmotaConfigs(&c2, nil))
	u, _ = es["RL_2"].Apply("stat/tasmota_ABCDEF/RESULT", []byte(`{"Color":"10,20,30,0,0"}`))
	if fmt.Sprint(u.Attrs["rgb_color"]) != "[10 20 30]" {
		t.Fatalf("decimal colour %v", u.Attrs)
	}
}

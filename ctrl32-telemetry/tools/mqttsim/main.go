// mqttsim simulates MQTT devices for tests and demos of spec 005 (provisioning and smart-home devices):
//
//	mqttsim tb      -addr 127.0.0.1:1883 -key pk_… -secret … -name sensor-1   ThingsBoard device: provisions itself,
//	                                                                         reconnects with the token, sends telemetry
//	                                                                         and answers RPC requests
//	mqttsim z2m     -addr … -user hs_… -pass … [-base zigbee2mqtt]         zigbee2mqtt bridge with a temperature
//	                                                                         sensor, a dimmable light and a plug
//	mqttsim tasmota -addr … -user hs_… -pass … -id A1B2C3 [-native]       Tasmota relay with Home Assistant discovery
//	                                                                         (-native: Tasmota's own discovery format)
//	mqttsim shelly  -addr … -user hs_… -pass … -id a8032ab12345           Shelly Plus 1PM (Gen2 RPC over MQTT)
//	mqttsim broker  -listen 127.0.0.1:1885 -user u -pass p                 a plain MQTT broker (a "foreign" broker)
//
// Every device prints one line per event on stdout ("EVENT <kind> <detail>") and runs for -wait seconds.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eclipse/paho.golang/paho"
	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: mqttsim tb|z2m|tasmota|shelly|broker [flags]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:1883", "broker host:port")
	user := fs.String("user", "", "MQTT user name")
	pass := fs.String("pass", "", "MQTT password")
	key := fs.String("key", "", "provisioning key (tb)")
	secret := fs.String("secret", "", "provisioning secret (tb)")
	name := fs.String("name", "sensor-1", "device name (tb)")
	token := fs.String("token", "", "access token; skips provisioning (tb)")
	base := fs.String("base", "zigbee2mqtt", "base topic (z2m)")
	prefix := fs.String("prefix", "homeassistant", "discovery prefix")
	id := fs.String("id", "A1B2C3", "device id (tasmota)")
	listen := fs.String("listen", "127.0.0.1:1885", "listen address (broker)")
	native := fs.Bool("native", false, "Tasmota native discovery instead of Home Assistant discovery (tasmota)")
	wait := fs.Int("wait", 10, "seconds to run")
	_ = fs.Parse(os.Args[2:])
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*wait)*time.Second)
	defer cancel()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	var err error
	switch os.Args[1] {
	case "tb":
		err = runTB(ctx, *addr, *key, *secret, *name, *token)
	case "z2m":
		err = runZ2M(ctx, *addr, *user, *pass, *base, *prefix)
	case "tasmota":
		if *native {
			err = runTasmotaNative(ctx, *addr, *user, *pass, *id)
		} else {
			err = runTasmota(ctx, *addr, *user, *pass, *id, *prefix)
		}
	case "shelly":
		err = runShelly(ctx, *addr, *user, *pass, *id)
	case "broker":
		err = runBroker(ctx, *listen, *user, *pass)
	default:
		err = fmt.Errorf("unknown mode %q", os.Args[1])
	}
	if err != nil {
		fmt.Println("EVENT error", err)
		os.Exit(1)
	}
}

var outMu sync.Mutex

func event(kind string, detail ...any) {
	outMu.Lock()
	defer outMu.Unlock()
	fmt.Println(append([]any{"EVENT", kind}, detail...)...)
}

type client struct {
	c        *paho.Client
	conn     net.Conn
	mu       sync.Mutex
	handlers []func(topic string, payload []byte)
}

func connect(ctx context.Context, addr, clientID, user, pass string, will *paho.WillMessage) (*client, error) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	cl := &client{conn: conn}
	cl.c = paho.NewClient(paho.ClientConfig{Conn: conn, ClientID: clientID, OnPublishReceived: []func(paho.PublishReceived) (bool, error){
		func(p paho.PublishReceived) (bool, error) {
			cl.mu.Lock()
			hs := append([]func(string, []byte){}, cl.handlers...)
			cl.mu.Unlock()
			for _, h := range hs {
				h(p.Packet.Topic, p.Packet.Payload)
			}
			return true, nil
		}}})
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ca, err := cl.c.Connect(cctx, &paho.Connect{ClientID: clientID, KeepAlive: 30, CleanStart: true, Username: user, UsernameFlag: user != "",
		Password: []byte(pass), PasswordFlag: pass != "", WillMessage: will})
	if err != nil {
		conn.Close()
		return nil, err
	}
	if ca.ReasonCode != 0 {
		conn.Close()
		return nil, fmt.Errorf("connection refused (%d)", ca.ReasonCode)
	}
	return cl, nil
}

func (cl *client) on(h func(topic string, payload []byte)) {
	cl.mu.Lock()
	cl.handlers = append(cl.handlers, h)
	cl.mu.Unlock()
}

func (cl *client) sub(ctx context.Context, filters ...string) error {
	opts := make([]paho.SubscribeOptions, len(filters))
	for i, f := range filters {
		opts[i] = paho.SubscribeOptions{Topic: f, QoS: 1}
	}
	sctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := cl.c.Subscribe(sctx, &paho.Subscribe{Subscriptions: opts})
	return err
}

func (cl *client) pub(ctx context.Context, topic string, payload any, retain bool) {
	var b []byte
	switch p := payload.(type) {
	case string:
		b = []byte(p)
	case []byte:
		b = p
	default:
		b, _ = json.Marshal(p)
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := cl.c.Publish(pctx, &paho.Publish{Topic: topic, Payload: b, Retain: retain, QoS: 1}); err != nil {
		event("publish_error", topic, err)
	}
}

func (cl *client) close() {
	_ = cl.c.Disconnect(&paho.Disconnect{})
	cl.conn.Close()
}

// ---- ThingsBoard device ----------------------------------------------------------------------------------------

func runTB(ctx context.Context, addr, key, secret, name, token string) error {
	if token == "" {
		p, err := connect(ctx, addr, "prov-"+name, "provision", "", nil)
		if err != nil {
			return fmt.Errorf("provisioning connection: %w", err)
		}
		got := make(chan []byte, 1)
		p.on(func(topic string, payload []byte) {
			if topic == "/provision/response" {
				got <- payload
			}
		})
		if err := p.sub(ctx, "/provision/response"); err != nil {
			return err
		}
		p.pub(ctx, "/provision/request", map[string]string{"deviceName": name, "provisionDeviceKey": key, "provisionDeviceSecret": secret}, false)
		var resp struct {
			Status           string `json:"status"`
			CredentialsValue string `json:"credentialsValue"`
			ErrorMsg         string `json:"errorMsg"`
		}
		select {
		case b := <-got:
			_ = json.Unmarshal(b, &resp)
		case <-time.After(10 * time.Second):
			return fmt.Errorf("no provisioning response")
		}
		p.close()
		if resp.Status != "SUCCESS" {
			event("provision_failed", resp.ErrorMsg)
			return nil
		}
		token = resp.CredentialsValue
		event("provisioned", token)
	}
	var cl *client
	var err error
	for i := 0; i < 20; i++ { // an approval may be pending: retry
		cl, err = connect(ctx, addr, "tb-"+name, token, "", nil)
		if err == nil {
			break
		}
		if i == 0 {
			event("connect_refused", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
	if err != nil {
		return err
	}
	defer cl.close()
	event("connected")
	cl.on(func(topic string, payload []byte) {
		switch {
		case strings.HasPrefix(topic, "v1/devices/me/rpc/request/"):
			n := strings.TrimPrefix(topic, "v1/devices/me/rpc/request/")
			event("rpc", n, string(payload))
			cl.pub(ctx, "v1/devices/me/rpc/response/"+n, map[string]any{"done": true}, false)
		case strings.HasPrefix(topic, "v1/devices/me/attributes/response/"):
			event("attributes_response", string(payload))
		case topic == "v1/devices/me/attributes":
			event("attributes", string(payload))
		}
	})
	if err := cl.sub(ctx, "v1/devices/me/rpc/request/+", "v1/devices/me/attributes", "v1/devices/me/attributes/response/+"); err != nil {
		return err
	}
	cl.pub(ctx, "v1/devices/me/attributes", map[string]any{"fw": "1.0.3", "model": "TB-sim"}, false)
	cl.pub(ctx, "v1/devices/me/attributes/request/1", map[string]string{"sharedKeys": "interval"}, false)
	t := 21.5
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		cl.pub(ctx, "v1/devices/me/telemetry", map[string]any{"temp": t, "door": t > 22, "mode": "eco", "Battery Level": 97}, false)
		t += 0.5
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

// ---- zigbee2mqtt ------------------------------------------------------------------------------------------------

func runZ2M(ctx context.Context, addr, user, pass, base, prefix string) error {
	cl, err := connect(ctx, addr, "z2m-"+strconv.FormatInt(time.Now().UnixNano()%1e6, 10), user, pass,
		&paho.WillMessage{Topic: base + "/bridge/state", Payload: []byte(`{"state":"offline"}`), Retain: true})
	if err != nil {
		return err
	}
	defer cl.close()
	event("connected")
	avail := []any{map[string]any{"topic": base + "/bridge/state", "value_template": "{{ value_json.state }}"}}
	dev := func(ieee, name, model, area string) map[string]any {
		return map[string]any{"identifiers": []string{"zigbee2mqtt_" + ieee}, "name": name, "model": model, "manufacturer": "Sim", "suggested_area": area, "via_device": "zigbee2mqtt_bridge"}
	}
	cl.pub(ctx, base+"/bridge/state", `{"state":"online"}`, true)
	sensor := dev("0x00158d0001a2b3c4", "Living room sensor", "WSDCGQ11LM", "Living room")
	cl.pub(ctx, prefix+"/sensor/0x00158d0001a2b3c4/temperature/config", map[string]any{"availability": avail, "device": sensor, "device_class": "temperature",
		"name": "Temperature", "state_class": "measurement", "state_topic": base + "/Living room sensor", "unique_id": "0x00158d0001a2b3c4_temperature_zigbee2mqtt",
		"unit_of_measurement": "°C", "value_template": "{{ value_json.temperature }}"}, true)
	cl.pub(ctx, prefix+"/sensor/0x00158d0001a2b3c4/humidity/config", map[string]any{"availability": avail, "device": sensor, "device_class": "humidity",
		"name": "Humidity", "state_topic": base + "/Living room sensor", "unique_id": "0x00158d0001a2b3c4_humidity_zigbee2mqtt",
		"unit_of_measurement": "%", "value_template": "{{ value_json.humidity }}"}, true)
	cl.pub(ctx, prefix+"/binary_sensor/0x00158d0001a2b3c5/contact/config", map[string]any{"availability": avail,
		"device": dev("0x00158d0001a2b3c5", "Front door", "MCCGQ11LM", "Hall"), "device_class": "door", "name": "Door",
		"state_topic": base + "/Front door", "unique_id": "0x00158d0001a2b3c5_contact_zigbee2mqtt", "payload_on": false, "payload_off": true,
		"value_template": "{{ value_json.contact }}"}, true)
	cl.pub(ctx, prefix+"/light/0x0017880104e45517/light/config", map[string]any{"availability": avail,
		"device": dev("0x0017880104e45517", "Hall light", "9290012573A", "Hall"), "name": nil, "schema": "json", "brightness": true, "brightness_scale": 254,
		"command_topic": base + "/Hall light/set", "state_topic": base + "/Hall light", "supported_color_modes": []string{"color_temp", "xy"}, "min_mireds": 153, "max_mireds": 454,
		"unique_id": "0x0017880104e45517_light_zigbee2mqtt"}, true)
	cl.pub(ctx, prefix+"/switch/0x0017880104e45518/switch/config", map[string]any{"availability": avail,
		"device": dev("0x0017880104e45518", "Kitchen plug", "SP 120", "Kitchen"), "name": nil, "command_topic": base + "/Kitchen plug/set",
		"state_topic": base + "/Kitchen plug", "payload_on": "ON", "payload_off": "OFF", "value_template": "{{ value_json.state }}",
		"unique_id": "0x0017880104e45518_switch_zigbee2mqtt"}, true)

	var mu sync.Mutex
	light := map[string]any{"state": "OFF", "brightness": 0.0, "color_temp": 300.0}
	plug := map[string]any{"state": "OFF", "power": 0.0}
	cl.on(func(topic string, payload []byte) {
		if !strings.HasSuffix(topic, "/set") {
			return
		}
		event("command", topic, string(payload))
		var m map[string]any
		_ = json.Unmarshal(payload, &m)
		mu.Lock()
		defer mu.Unlock()
		switch topic {
		case base + "/Hall light/set":
			for k, v := range m {
				light[k] = v
			}
			cl.pub(ctx, base+"/Hall light", light, false)
		case base + "/Kitchen plug/set":
			if s, ok := m["state"].(string); ok {
				plug["state"] = s
			} else {
				plug["state"] = strings.TrimSpace(string(payload))
			}
			cl.pub(ctx, base+"/Kitchen plug", plug, false)
		}
	})
	if err := cl.sub(ctx, base+"/+/set"); err != nil {
		return err
	}
	t := 21.0
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for i := 0; ; i++ {
		cl.pub(ctx, base+"/Living room sensor", map[string]any{"temperature": t, "humidity": 45.5, "battery": 97, "linkquality": 120}, false)
		cl.pub(ctx, base+"/Front door", map[string]any{"contact": i%4 != 3, "battery": 90}, false)
		if i == 0 {
			mu.Lock()
			cl.pub(ctx, base+"/Hall light", light, false)
			cl.pub(ctx, base+"/Kitchen plug", plug, false)
			mu.Unlock()
		}
		t += 0.1
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

// ---- Tasmota ------------------------------------------------------------------------------------------------------

func runTasmota(ctx context.Context, addr, user, pass, id, prefix string) error {
	topic := "tasmota_" + id
	cl, err := connect(ctx, addr, "DVES_"+id, user, pass, &paho.WillMessage{Topic: "tele/" + topic + "/LWT", Payload: []byte("Offline"), Retain: true})
	if err != nil {
		return err
	}
	defer cl.close()
	event("connected")
	cl.pub(ctx, "tele/"+topic+"/LWT", "Online", true)
	cl.pub(ctx, prefix+"/switch/"+id+"_RL_1/config", map[string]any{"name": "Relay", "stat_t": "tele/" + topic + "/STATE", "avty_t": "tele/" + topic + "/LWT",
		"pl_avail": "Online", "pl_not_avail": "Offline", "cmd_t": "cmnd/" + topic + "/POWER", "val_tpl": "{{value_json.POWER}}", "pl_off": "OFF", "pl_on": "ON",
		"uniq_id": id + "_RL_1", "dev": map[string]any{"ids": []string{id}, "name": "Garage", "mdl": "Sonoff Basic", "sw": "14.2.0", "mf": "Tasmota"}}, true)
	cl.pub(ctx, prefix+"/sensor/"+id+"_energy_power/config", map[string]any{"name": "Power", "stat_t": "tele/" + topic + "/SENSOR", "avty_t": "tele/" + topic + "/LWT",
		"pl_avail": "Online", "pl_not_avail": "Offline", "val_tpl": "{{value_json['ENERGY']['Power']}}", "unit_of_meas": "W", "dev_cla": "power",
		"uniq_id": id + "_energy_power", "dev": map[string]any{"ids": []string{id}}}, true)
	var mu sync.Mutex
	power := "OFF"
	state := func() {
		cl.pub(ctx, "tele/"+topic+"/STATE", map[string]any{"Time": time.Now().Format("2006-01-02T15:04:05"), "POWER": power}, false)
	}
	cl.on(func(t string, payload []byte) {
		if t != "cmnd/"+topic+"/POWER" {
			return
		}
		event("command", t, string(payload))
		mu.Lock()
		switch strings.ToUpper(string(payload)) {
		case "ON", "1":
			power = "ON"
		case "OFF", "0":
			power = "OFF"
		case "TOGGLE", "2":
			if power == "ON" {
				power = "OFF"
			} else {
				power = "ON"
			}
		}
		cl.pub(ctx, "stat/"+topic+"/RESULT", map[string]string{"POWER": power}, false)
		state()
		mu.Unlock()
	})
	if err := cl.sub(ctx, "cmnd/"+topic+"/#"); err != nil {
		return err
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		mu.Lock()
		state()
		w := 0.0
		if power == "ON" {
			w = 60.5
		}
		mu.Unlock()
		cl.pub(ctx, "tele/"+topic+"/SENSOR", map[string]any{"ENERGY": map[string]any{"Power": w, "Voltage": 231}}, false)
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

// runTasmotaNative is a Sonoff POW with an AM2301 sensor using Tasmota's native discovery (SetOption19 0).
func runTasmotaNative(ctx context.Context, addr, user, pass, id string) error {
	topic := "tasmota_" + id
	mac := "A4CF12" + strings.ToUpper(id)
	cl, err := connect(ctx, addr, "DVES_"+id, user, pass, &paho.WillMessage{Topic: "tele/" + topic + "/LWT", Payload: []byte("Offline"), Retain: true})
	if err != nil {
		return err
	}
	defer cl.close()
	event("connected")
	cl.pub(ctx, "tele/"+topic+"/LWT", "Online", true)
	cl.pub(ctx, "tasmota/discovery/"+mac+"/config", map[string]any{"ip": "192.168.1.50", "dn": "Workshop plug", "fn": []any{"Workshop plug", nil, nil, nil},
		"hn": "tasmota-" + id + "-1234", "mac": mac, "md": "Sonoff POW R2", "ty": 0, "if": 0, "ofln": "Offline", "onln": "Online",
		"state": []string{"OFF", "ON", "TOGGLE", "HOLD"}, "sw": "14.2.0", "t": topic, "ft": "%prefix%/%topic%/", "tp": []string{"cmnd", "stat", "tele"},
		"rl": []int{1, 0, 0, 0}, "swc": []int{-1, -1, -1, -1}, "swn": []any{nil, nil, nil, nil}, "btn": []int{0, 0, 0, 0},
		"so": map[string]int{"4": 0, "11": 0, "13": 0, "17": 0, "20": 0, "30": 0, "68": 0, "73": 0, "82": 0, "114": 0, "117": 0},
		"lk": 0, "lt_st": 0, "sho": []int{}, "sht": []int{}, "ver": 1}, true)
	var mu sync.Mutex
	power := "OFF"
	temp := 21.0
	sensor := func() map[string]any {
		w := 0.0
		if power == "ON" {
			w = 1200 + float64(time.Now().Second())
		}
		temp += 0.1
		if temp > 24 {
			temp = 21
		}
		return map[string]any{"Time": time.Now().Format("2006-01-02T15:04:05"), "ENERGY": map[string]any{"Total": 12.5, "Today": 0.4, "Power": w, "Voltage": 231, "Current": w / 231},
			"AM2301": map[string]any{"Temperature": math.Round(temp*10) / 10, "Humidity": 45.5}, "TempUnit": "C"}
	}
	cl.pub(ctx, "tasmota/discovery/"+mac+"/sensors", map[string]any{"sn": sensor(), "ver": 1}, true)
	cl.on(func(t string, payload []byte) {
		if !strings.HasPrefix(t, "cmnd/"+topic+"/") {
			return
		}
		cmd := strings.ToUpper(strings.TrimPrefix(t, "cmnd/"+topic+"/"))
		event("command", t, string(payload))
		mu.Lock()
		defer mu.Unlock()
		switch cmd {
		case "POWER", "POWER1":
			switch strings.ToUpper(string(payload)) {
			case "ON", "1":
				power = "ON"
			case "OFF", "0":
				power = "OFF"
			case "TOGGLE", "2":
				power = map[string]string{"ON": "OFF", "OFF": "ON"}[power]
			}
			cl.pub(ctx, "stat/"+topic+"/RESULT", map[string]string{"POWER": power}, false)
			cl.pub(ctx, "stat/"+topic+"/POWER", power, false)
		case "STATE":
			cl.pub(ctx, "stat/"+topic+"/RESULT", map[string]any{"Time": time.Now().Format("2006-01-02T15:04:05"), "Uptime": "0T00:01:00", "POWER": power}, false)
		}
	})
	if err := cl.sub(ctx, "cmnd/"+topic+"/#"); err != nil {
		return err
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		mu.Lock()
		s := sensor()
		mu.Unlock()
		cl.pub(ctx, "tele/"+topic+"/SENSOR", s, false)
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

// ---- Shelly Gen2 ----------------------------------------------------------------------------------------------------

// runShelly is a Shelly Plus 1PM: RPC over MQTT on <id>/rpc, notifications on <id>/events/rpc, <id>/online.
func runShelly(ctx context.Context, addr, user, pass, id string) error {
	dev := "shellyplus1pm-" + strings.ToLower(id)
	cl, err := connect(ctx, addr, dev, user, pass, &paho.WillMessage{Topic: dev + "/online", Payload: []byte("false"), Retain: true})
	if err != nil {
		return err
	}
	defer cl.close()
	event("connected")
	cl.pub(ctx, dev+"/online", "true", true)
	var mu sync.Mutex
	on, energy, input := false, 1234.5, false
	sw := func() map[string]any {
		p := 0.0
		if on {
			p = 35.2
		}
		return map[string]any{"id": 0, "source": "init", "output": on, "apower": p, "voltage": 230.8, "current": p / 230.8, "freq": 50.0,
			"aenergy":     map[string]any{"total": energy, "by_minute": []float64{0, 0, 0}, "minute_ts": time.Now().Unix()},
			"temperature": map[string]any{"tC": 44.1, "tF": 111.4}}
	}
	notify := func(params map[string]any) {
		params["ts"] = float64(time.Now().UnixMilli()) / 1000
		cl.pub(ctx, dev+"/events/rpc", map[string]any{"src": dev, "dst": dev + "/events", "method": "NotifyStatus", "params": params}, false)
	}
	cl.on(func(t string, payload []byte) {
		if t != dev+"/rpc" {
			return
		}
		var req struct {
			ID     any            `json:"id"`
			Src    string         `json:"src"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if json.Unmarshal(payload, &req) != nil || req.Src == "" {
			return
		}
		event("rpc", req.Method, string(payload))
		mu.Lock()
		defer mu.Unlock()
		var result any
		switch req.Method {
		case "Shelly.GetDeviceInfo":
			result = map[string]any{"name": "Boiler", "id": dev, "mac": strings.ToUpper(id), "slot": 1, "model": "SNSW-001P16EU", "gen": 2,
				"fw_id": "20241011-114455/1.4.4-g6d2a586", "ver": "1.4.4", "app": "Plus1PM", "auth_en": false}
		case "Shelly.GetStatus":
			result = map[string]any{"ble": map[string]any{}, "cloud": map[string]any{"connected": false}, "input:0": map[string]any{"id": 0, "state": input},
				"mqtt": map[string]any{"connected": true}, "switch:0": sw(), "sys": map[string]any{"uptime": 60}, "wifi": map[string]any{"rssi": -58}}
		case "Switch.Set", "Switch.Toggle":
			was := on
			if req.Method == "Switch.Toggle" {
				on = !on
			} else if v, ok := req.Params["on"].(bool); ok {
				on = v
			}
			result = map[string]any{"was_on": was}
			defer notify(map[string]any{"switch:0": map[string]any{"id": 0, "output": on, "source": "MQTT", "apower": sw()["apower"]}})
		default:
			cl.pub(ctx, req.Src+"/rpc", map[string]any{"id": req.ID, "src": dev, "dst": req.Src, "error": map[string]any{"code": 404, "message": "No handler for " + req.Method}}, false)
			return
		}
		cl.pub(ctx, req.Src+"/rpc", map[string]any{"id": req.ID, "src": dev, "dst": req.Src, "result": result}, false)
	})
	if err := cl.sub(ctx, dev+"/rpc"); err != nil {
		return err
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	n := 0
	for {
		mu.Lock()
		n++
		if on {
			energy += 0.01
		}
		if n%5 == 0 {
			input = !input
			notify(map[string]any{"input:0": map[string]any{"id": 0, "state": input}})
		}
		if on {
			notify(map[string]any{"switch:0": map[string]any{"id": 0, "apower": 35.2 + float64(n%3), "aenergy": map[string]any{"total": energy}}})
		}
		mu.Unlock()
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

// ---- a plain broker ---------------------------------------------------------------------------------------------

func runBroker(ctx context.Context, listen, user, pass string) error {
	srv := mqtt.New(&mqtt.Options{InlineClient: true})
	srv.Log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if user != "" {
		if err := srv.AddHook(new(auth.Hook), &auth.Options{Ledger: &auth.Ledger{
			Auth: auth.AuthRules{{Username: auth.RString(user), Password: auth.RString(pass), Allow: true}},
			ACL:  auth.ACLRules{{Username: auth.RString(user), Filters: auth.Filters{"#": auth.ReadWrite}}},
		}}); err != nil {
			return err
		}
	} else if err := srv.AddHook(new(auth.AllowHook), nil); err != nil {
		return err
	}
	if err := srv.AddListener(listeners.NewTCP(listeners.Config{ID: "t", Address: listen})); err != nil {
		return err
	}
	if err := srv.Serve(); err != nil {
		return err
	}
	event("listening", listen)
	<-ctx.Done()
	return srv.Close()
}

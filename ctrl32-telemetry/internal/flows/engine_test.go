package flows

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/ingest"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/live"
)

type fakeHost struct {
	mu       sync.Mutex
	commands []string
	notes    []string
	stored   []float64
	http     []string
	incident []string
	attrs    []string
	entCmds  []string
	mqttOut  []string
	mqttSubs map[string]func(string, []byte)
	scenes   []string
	marks    []string
}

func (h *fakeHost) CameraMark(_ context.Context, _, cam, label string, seconds int) (int64, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.marks = append(h.marks, fmt.Sprintf("%s | %s | %d", cam, label, seconds))
	return int64(len(h.marks)), nil
}

func (h *fakeHost) CameraName(context.Context, string, string) string { return "Gate" }

func (h *fakeHost) DatastreamName(_ context.Context, _, id string) (string, string) {
	return "Mixer / temp", "temp"
}
func (h *fakeHost) DeviceName(_ context.Context, _, id string) string { return "PLC-1" }
func (h *fakeHost) StoreDerived(_ context.Context, _, _, _ string, v float64, _ time.Time) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stored = append(h.stored, v)
	return nil
}
func (h *fakeHost) RunCommand(_ context.Context, _, dev, method string, args json.RawMessage, flowID, issuer, reason string) (string, string, string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.commands = append(h.commands, dev+" "+method+" "+string(args)+" | "+issuer+" | "+reason)
	return "cmd-1", "sent", "", nil
}
func (h *fakeHost) Notify(_ context.Context, _, ch, flowID, subject, text string, _ map[string]any) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.notes = append(h.notes, subject+" / "+text)
	return nil
}
func (h *fakeHost) SetAttribute(_ context.Context, _, dev, key string, v any, issuer, reason string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.attrs = append(h.attrs, key+"="+Str(v)+" by "+issuer)
	return nil
}
func (h *fakeHost) EntityCommand(_ context.Context, _, ent, service string, params map[string]any, _, issuer, reason string) (string, string, string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entCmds = append(h.entCmds, fmt.Sprintf("%s %s %v | %s | %s", ent, service, params, issuer, reason))
	return "cmd1", "sent", "", nil
}
func (h *fakeHost) MQTTSubscribe(_, src, filter string, fn func(string, []byte)) (func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.mqttSubs == nil {
		h.mqttSubs = map[string]func(string, []byte){}
	}
	h.mqttSubs[filter] = fn
	return func() { h.mu.Lock(); delete(h.mqttSubs, filter); h.mu.Unlock() }, nil
}
func (h *fakeHost) MQTTPublish(_ context.Context, _, src, topic string, payload []byte, retain bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mqttOut = append(h.mqttOut, fmt.Sprintf("%s %s %v", topic, payload, retain))
	return nil
}
func (h *fakeHost) EntityName(_ context.Context, _, id string) string { return "Hall door" }
func (h *fakeHost) SceneActivate(_ context.Context, _, scene, _, issuer, reason string) (int, int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.scenes = append(h.scenes, scene+" | "+issuer+" | "+reason)
	return 2, 1, nil
}
func (h *fakeHost) Incident(_ context.Context, _, action, sev, title, dedup string, _ map[string]any) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.incident = append(h.incident, action+" "+sev+" "+title+" "+dedup)
	return nil
}
func (h *fakeHost) HTTP(_ context.Context, _, method, url string, body []byte) (int, string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.http = append(h.http, method+" "+url+" "+string(body))
	return 200, "ok", nil
}
func (h *fakeHost) Location(context.Context, string) *time.Location { return time.UTC }

func (h *fakeHost) snapshot() ([]string, []string, []float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string{}, h.commands...), append([]string{}, h.notes...), append([]float64{}, h.stored...)
}

func testEngine(t *testing.T, js string) (*Engine, *flow, *fakeHost, context.CancelFunc) {
	t.Helper()
	var sp Spec
	if err := json.Unmarshal([]byte(js), &sp); err != nil {
		t.Fatal(err)
	}
	if p := Validate(&sp, fakeRefs{}); len(p) != 0 {
		t.Fatalf("invalid test flow: %v", p)
	}
	h := &fakeHost{}
	e := New(nil, h, "0123456789abcdef-secret", slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.LoadSubflow = testLoadSubflow
	ctx, cancel := context.WithCancel(context.Background())
	f, err := e.startTest(ctx, &sp, "Cooling")
	if err != nil {
		t.Fatal(err)
	}
	return e, f, h, cancel
}

func publish(e *Engine, v float64) {
	e.OnObservations(context.Background(), []ingest.Observation{{OrgID: "org", DatastreamID: dsA, Value: &v, MeasuredAt: time.Now(), Quality: "ok"}})
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout: %s", what)
}

func debugTexts(e *Engine, kind string) []string {
	var out []string
	for _, d := range e.Debug("org", "test", 0) {
		if kind == "" || d.Kind == kind {
			out = append(out, d.Text)
		}
	}
	return out
}

// US1: temperature above 80 for the hold time → one notification and one command with the flow as origin.
func TestFlowUS1(t *testing.T) {
	js := strings.Replace(exampleFlow, `"seconds":120`, `"seconds":0.2`, 1)
	e, _, h, cancel := testEngine(t, js)
	defer cancel()
	publish(e, 85)
	time.Sleep(50 * time.Millisecond)
	publish(e, 86)
	waitFor(t, "command and notification", func() bool { c, n, _ := h.snapshot(); return len(c) == 1 && len(n) == 1 })
	publish(e, 87) // still hot: nothing more
	publish(e, 70) // cools down: resets
	time.Sleep(300 * time.Millisecond)
	c, n, _ := h.snapshot()
	if len(c) != 1 || len(n) != 1 {
		t.Fatalf("actions once per trigger: %v %v", c, n)
	}
	if !strings.Contains(c[0], `"key":"cooling","value":true`) || !strings.Contains(c[0], "flow:Cooling") || !strings.Contains(c[0], "flow Cooling v3 node cool (Mixer / temp = 86)") {
		t.Errorf("command origin: %s", c[0])
	}
	if !strings.HasPrefix(n[0], "Too hot: 86.0") {
		t.Errorf("notification: %s", n[0])
	}
	st := e.Status("org", "test")
	if st.Nodes["t"].In != 4 || st.Nodes["hot"].Out != 2 {
		t.Errorf("counters %+v", st.Nodes)
	}
}

// US2: a dry-run injection shows what would happen and executes nothing.
func TestFlowDryRun(t *testing.T) {
	js := strings.Replace(exampleFlow, `"seconds":120`, `"seconds":0`, 1)
	e, _, h, cancel := testEngine(t, js)
	defer cancel()
	if err := e.Inject(context.Background(), "org", "test", "t", 90.0, "", true, nil, ""); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "dry actions", func() bool { return len(debugTexts(e, "dry_action")) == 2 })
	c, n, _ := h.snapshot()
	if len(c) != 0 || len(n) != 0 {
		t.Fatalf("dry run executed actions: %v %v", c, n)
	}
	d := strings.Join(debugTexts(e, "dry_action"), " | ")
	if !strings.Contains(d, "would send write") || !strings.Contains(d, "would notify: Too hot: 90.0") {
		t.Errorf("dry actions: %s", d)
	}
}

// Logic nodes with the acceptance scenario of the deadband (20.0, 20.3, 20.6 → 20.0, 20.6).
func TestFlowLogic(t *testing.T) {
	js := `{"nodes":[
	 {"id":"t","type":"ds_value","settings":{"datastream":"` + dsA + `"}},
	 {"id":"db","type":"deadband","settings":{"band":0.5}},
	 {"id":"d1","type":"debug","settings":{"expr":"'db ' + value"}},
	 {"id":"sw","type":"switch","settings":{"rules":["value < 20.5","value >= 20.5"]}},
	 {"id":"lo","type":"template","settings":{"text":"low {{ value }}"}},
	 {"id":"d2","type":"debug","settings":{"expr":"value"}},
	 {"id":"ch","type":"change","settings":{"fields":[{"op":"set","name":"msg.f","expr":"value * 2"},{"op":"rename","name":"msg.f","to":"topic"}]}},
	 {"id":"d3","type":"debug","settings":{"expr":"'topic ' + topic"}},
	 {"id":"cnt","type":"counter","settings":{}},
	 {"id":"d4","type":"debug","settings":{"expr":"'count ' + value"}},
	 {"id":"la","type":"latch","settings":{"set":"value > 20.5","reset":"value < 20.1"}},
	 {"id":"d5","type":"debug","settings":{"expr":"'latch ' + value"}},
	 {"id":"st","type":"derive","settings":{"datastream":"` + dsA + `","expr":"value / 2"}}
	 ],"wires":[{"from":"t","port":0,"to":"db"},{"from":"db","port":0,"to":"d1"},{"from":"db","port":0,"to":"sw"},
	 {"from":"sw","port":0,"to":"lo"},{"from":"lo","port":0,"to":"d2"},{"from":"sw","port":1,"to":"ch"},{"from":"ch","port":0,"to":"d3"},
	 {"from":"t","port":0,"to":"cnt"},{"from":"cnt","port":0,"to":"d4"},{"from":"t","port":0,"to":"la"},{"from":"la","port":0,"to":"d5"},{"from":"db","port":0,"to":"st"}]}`
	e, _, h, cancel := testEngine(t, js)
	defer cancel()
	for _, v := range []float64{20.0, 20.3, 20.6} {
		publish(e, v)
	}
	waitFor(t, "stored values", func() bool { _, _, s := h.snapshot(); return len(s) == 2 })
	got := strings.Join(debugTexts(e, "debug"), " | ")
	for _, want := range []string{"db 20", "db 20.6", "low 20", "topic 41.2", "count 1", "count 2", "count 3", "latch true"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "db 20.3") {
		t.Errorf("deadband passed 20.3: %s", got)
	}
	_, _, stored := h.snapshot()
	if stored[0] != 10 || stored[1] != 10.3 {
		t.Errorf("derived %v", stored)
	}
}

// Timing nodes: aggregate, rate limit, debounce, delay; and alarm/device triggers through the hub tap.
func TestFlowTimingAndEvents(t *testing.T) {
	js := `{"nodes":[
	 {"id":"t","type":"ds_value","settings":{"datastream":"` + dsA + `"}},
	 {"id":"ag","type":"aggregate","settings":{"window_s":1,"fn":"max"}},
	 {"id":"d1","type":"debug","settings":{"expr":"'max ' + value + ' of ' + msg.count"}},
	 {"id":"rl","type":"rate_limit","settings":{"count":2,"period_s":10}},
	 {"id":"d2","type":"debug","settings":{"expr":"'rl ' + value"}},
	 {"id":"de","type":"debounce","settings":{"seconds":0.2}},
	 {"id":"d3","type":"debug","settings":{"expr":"'deb ' + value"}},
	 {"id":"al","type":"alarm_event","settings":{"events":["raised","cleared"]}},
	 {"id":"dv","type":"device_state","settings":{}},
	 {"id":"d4","type":"debug","settings":{"expr":"msg.event + ' ' + topic"}}
	 ],"wires":[{"from":"t","port":0,"to":"ag"},{"from":"ag","port":0,"to":"d1"},{"from":"t","port":0,"to":"rl"},{"from":"rl","port":0,"to":"d2"},
	 {"from":"t","port":0,"to":"de"},{"from":"de","port":0,"to":"d3"},{"from":"al","port":0,"to":"d4"},{"from":"dv","port":0,"to":"d4"}]}`
	e, _, _, cancel := testEngine(t, js)
	defer cancel()
	for _, v := range []float64{1, 5, 3} {
		publish(e, v)
	}
	v := 9.5
	e.OnEvent(live.Event{Type: "alarm", Org: "org", State: "active", Severity: "action", DatastreamID: dsA, Value: &v})
	on := true
	e.OnEvent(live.Event{Type: "device", Org: "org", DeviceID: devA, Connected: &on})
	e.OnEvent(live.Event{Type: "alarm", Org: "other-org", State: "active"})
	waitFor(t, "aggregate", func() bool { return strings.Contains(strings.Join(debugTexts(e, "debug"), "|"), "max 5 of 3") })
	got := strings.Join(debugTexts(e, "debug"), " | ")
	for _, want := range []string{"rl 1", "rl 5", "deb 3", "raised Mixer / temp", "connected PLC-1"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "rl 3") || strings.Contains(got, "deb 1") || strings.Count(got, "raised") != 1 {
		t.Errorf("unexpected: %s", got)
	}
}

// Webhook tokens, HMAC and the queue limit.
func TestFlowWebhookAndQueue(t *testing.T) {
	js := `{"nodes":[{"id":"w","type":"webhook","settings":{"secret":"s3cret"}},{"id":"d","type":"debug","settings":{"expr":"topic + ' ' + value + ' ' + msg.batch"}}],
	 "wires":[{"from":"w","port":0,"to":"d"}]}`
	e, f, _, cancel := testEngine(t, js)
	tok := e.HookToken("org", "test", "w")
	if !strings.HasPrefix(tok, "fh_") || tok == e.HookToken("org", "test", "x") {
		t.Fatal("token")
	}
	if err := e.Webhook("fh_wrong", nil, nil); err != ErrUnknownHook {
		t.Fatalf("unknown token: %v", err)
	}
	if err := e.Webhook(tok, map[string]any{"value": 5.0}, func(s string) bool { return false }); err == nil {
		t.Fatal("bad signature accepted")
	}
	if err := e.Webhook(tok, map[string]any{"value": 5.0, "topic": "line1", "batch": "B-7"}, func(s string) bool { return s == "s3cret" }); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "webhook message", func() bool { return strings.Contains(strings.Join(debugTexts(e, "debug"), "|"), "line1 5 B-7") })
	cancel()
	f.stopWait()
	// a stopped flow does not drain its queue: the limit drops and counts
	for i := 0; i < QueueSize+10; i++ {
		f.enqueue(qitem{node: "d", msg: &Message{}})
	}
	if f.dropped.Load() != 10 {
		t.Errorf("dropped %d", f.dropped.Load())
	}
}

// The attribute node sets a shared attribute with the flow as origin.
func TestFlowAttribute(t *testing.T) {
	e, _, h, cancel := testEngine(t, `{"nodes":[{"id":"t","type":"ds_value","x":0,"y":0,"settings":{"datastream":"`+dsA+`"}},
		{"id":"a","type":"attribute","x":200,"y":0,"settings":{"device":"`+devA+`","key":"setpoint","value":"value * 2"}}],"wires":[{"from":"t","port":0,"to":"a"}]}`)
	defer cancel()
	publish(e, 21)
	waitFor(t, "attribute", func() bool { h.mu.Lock(); defer h.mu.Unlock(); return len(h.attrs) == 1 })
	if h.attrs[0] != "setpoint=42 by flow:Cooling" {
		t.Fatalf("attribute: %v", h.attrs)
	}
}

// SC-002: a message passes a 10-node flow in well under 50 ms and the engine sustains 1 000 messages per second.
func TestFlowThroughput(t *testing.T) {
	js := `{"nodes":[{"id":"t","type":"ds_value","x":0,"y":0,"settings":{"datastream":"` + dsA + `"}},
		{"id":"m1","type":"math","x":0,"y":0,"settings":{"expr":"value * 2","target":"value"}},
		{"id":"f1","type":"filter","x":0,"y":0,"settings":{"condition":"value >= 0"}},
		{"id":"c1","type":"change","x":0,"y":0,"settings":{"fields":[{"op":"set","name":"msg.raw","expr":"value"}]}},
		{"id":"m2","type":"math","x":0,"y":0,"settings":{"expr":"value / 2","target":"value"}},
		{"id":"s1","type":"switch","x":0,"y":0,"settings":{"rules":["value < 1e9"],"check":"first"}},
		{"id":"tp","type":"template","x":0,"y":0,"settings":{"text":"{{ fixed(value, 1) }}","target":"msg.text"}},
		{"id":"cn","type":"counter","x":0,"y":0,"settings":{"add":"1"}},
		{"id":"m3","type":"math","x":0,"y":0,"settings":{"expr":"msg.raw / 2","target":"value"}},
		{"id":"d","type":"derive","x":0,"y":0,"settings":{"datastream":"` + dsA + `","expr":"value"}}],
		"wires":[{"from":"t","port":0,"to":"m1"},{"from":"m1","port":0,"to":"f1"},{"from":"f1","port":0,"to":"c1"},{"from":"c1","port":0,"to":"m2"},
		{"from":"m2","port":0,"to":"s1"},{"from":"s1","port":0,"to":"tp"},{"from":"tp","port":0,"to":"cn"},{"from":"cn","port":0,"to":"m3"},{"from":"m3","port":0,"to":"d"}]}`
	e, _, h, cancel := testEngine(t, js)
	defer cancel()
	stored := func() int { h.mu.Lock(); defer h.mu.Unlock(); return len(h.stored) }
	// latency of a single message through all ten nodes
	start := time.Now()
	publish(e, 1)
	waitFor(t, "first message", func() bool { return stored() == 1 })
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("one message took %v through 10 nodes", d)
	}
	// throughput: 2 000 messages in batches that fit the queue
	const n = 2000
	start = time.Now()
	for i := 0; i < n; i += 250 {
		for j := 0; j < 250; j++ {
			publish(e, float64(i+j))
		}
		for stored() < 1+i+250 && time.Since(start) < 10*time.Second {
			time.Sleep(time.Millisecond)
		}
	}
	el := time.Since(start)
	if stored() != n+1 {
		t.Fatalf("stored %d of %d", stored(), n+1)
	}
	rate := float64(n) / el.Seconds()
	t.Logf("%d messages through 10 nodes in %v (%.0f msg/s)", n, el, rate)
	if rate < 1000 {
		t.Errorf("throughput %.0f msg/s, want ≥ 1 000", rate)
	}
}

// Entity trigger with from/to and an entity action: "door opened → hall light on at 60 %".
func TestFlowEntities(t *testing.T) {
	e, _, h, cancel := testEngine(t, `{"nodes":[{"id":"door","type":"entity","x":0,"y":0,"settings":{"entity":"`+entA+`","to":"on"}},
		{"id":"light","type":"entity_action","x":200,"y":0,"settings":{"entity":"`+entA+`","service":"turn_on","value":"60"}},
		{"id":"res","type":"command_result","x":0,"y":100,"settings":{"statuses":["failed"]}},
		{"id":"dbg","type":"debug","x":200,"y":100,"settings":{"active":true,"expr":"value + ' ' + msg.error"}}],
		"wires":[{"from":"door","port":0,"to":"light"},{"from":"res","port":0,"to":"dbg"}]}`)
	defer cancel()
	on := true
	e.OnEvent(live.Event{Type: "entity", Org: "org", EntityID: entA, State: "off", Connected: &on})
	e.OnEvent(live.Event{Type: "entity", Org: "org", EntityID: entA, State: "on", Connected: &on, Detail: map[string]any{"battery": 90.0}})
	e.OnEvent(live.Event{Type: "entity", Org: "org", EntityID: entA, State: "on", Connected: &on, Detail: map[string]any{"battery": 89.0}}) // same state: nothing
	waitFor(t, "entity action", func() bool { h.mu.Lock(); defer h.mu.Unlock(); return len(h.entCmds) == 1 })
	time.Sleep(100 * time.Millisecond)
	h.mu.Lock()
	got := append([]string(nil), h.entCmds...)
	h.mu.Unlock()
	if len(got) != 1 || !strings.Contains(got[0], "turn_on map[brightness:60]") || !strings.Contains(got[0], "flow:Cooling") || !strings.Contains(got[0], "Hall door off→on") {
		t.Fatalf("entity commands: %v", got)
	}
	e.OnEvent(live.Event{Type: "command", Org: "org", DeviceID: devA, CommandID: "c1", Method: "turn_on", Status: "acked"})
	e.OnEvent(live.Event{Type: "command", Org: "org", DeviceID: devA, CommandID: "c2", Method: "turn_on", Status: "failed", Detail: map[string]any{"error": "no answer"}})
	waitFor(t, "command result", func() bool { return len(debugTexts(e, "debug")) == 1 })
	if d := debugTexts(e, "debug")[0]; d != "failed no answer" {
		t.Fatalf("debug %q", d)
	}
}

// MQTT in → condition → MQTT out: a Tasmota sensor above 25 °C switches a relay.
func TestFlowMQTT(t *testing.T) {
	e, _, h, cancel := testEngine(t, `{"nodes":[{"id":"in","type":"mqtt_in","x":0,"y":0,"settings":{"source":"`+srcA+`","topic":"tele/+/SENSOR"}},
		{"id":"hot","type":"filter","x":200,"y":0,"settings":{"condition":"msg.temperature > 25"}},
		{"id":"out","type":"mqtt_out","x":400,"y":0,"settings":{"source":"`+srcA+`","topic":"cmnd/{{ replace(topic, '/SENSOR', '') }}/POWER","payload":"ON"}}],
		"wires":[{"from":"in","port":0,"to":"hot"},{"from":"hot","port":0,"to":"out"}]}`)
	defer cancel()
	e.register(e.flows[key("org", "test")])
	h.mu.Lock()
	fn := h.mqttSubs["tele/+/SENSOR"]
	h.mu.Unlock()
	if fn == nil {
		t.Fatal("not subscribed")
	}
	fn("tele/fan/SENSOR", []byte(`{"temperature":24}`))
	fn("tele/fan/SENSOR", []byte(`{"temperature":26.5}`))
	waitFor(t, "publish", func() bool { h.mu.Lock(); defer h.mu.Unlock(); return len(h.mqttOut) == 1 })
	if h.mqttOut[0] != "cmnd/tele/fan/POWER ON false" {
		t.Fatalf("published %v", h.mqttOut)
	}
	e.Stop("org", "test")
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.mqttSubs) != 0 {
		t.Fatal("subscription kept after stop")
	}
}

func TestMQTTMessageFields(t *testing.T) {
	m := mqttMessage("src", "tele/plug/SENSOR", []byte(`{"Time":"2026-09-26T10:00:00","ENERGY":{"Power":42.5,"Voltage":231,"Deep":{"A":{"B":1}}},"POWER":"ON"}`))
	if m.Fields["ENERGY_Power"] != 42.5 || m.Fields["ENERGY_Voltage"] != 231.0 || m.Fields["POWER"] != "ON" || m.Fields["Time"] == nil {
		t.Fatalf("fields %v", m.Fields)
	}
	if _, ok := m.Fields["ENERGY_Deep_A_B"]; ok {
		t.Fatalf("depth not limited: %v", m.Fields)
	}
	if m := mqttMessage("src", "stat/x/POWER", []byte("ON")); m.Value != "ON" {
		t.Fatalf("plain text %v", m.Value)
	}
	if m := mqttMessage("src", "x", []byte("21.5")); m.Value != 21.5 {
		t.Fatalf("number %v", m.Value)
	}
}

func TestFlowScene(t *testing.T) {
	e, _, h, cancel := testEngine(t, `{"nodes":[{"id":"i","type":"inject","x":0,"y":0,"settings":{"value":"1"}},
		{"id":"s","type":"scene","x":200,"y":0,"settings":{"scene":"`+srcA+`"}},
		{"id":"d","type":"debug","x":400,"y":0,"settings":{"expr":"msg.ok + '/' + msg.failed"}}],
		"wires":[{"from":"i","port":0,"to":"s"},{"from":"s","port":0,"to":"d"}]}`)
	defer cancel()
	if err := e.Inject(context.Background(), "org", "test", "i", 1.0, "", false, nil, ""); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "scene", func() bool { h.mu.Lock(); defer h.mu.Unlock(); return len(h.scenes) == 1 })
	if !strings.HasPrefix(h.scenes[0], srcA+" | flow:") {
		t.Fatalf("scene call %v", h.scenes)
	}
	waitFor(t, "debug", func() bool { d := debugTexts(e, "debug"); return len(d) == 1 && d[0] == "2/1" })
	if err := e.Inject(context.Background(), "org", "test", "i", 1.0, "", true, nil, ""); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "dry", func() bool { return len(debugTexts(e, "dry_action")) == 1 })
}

// Camera events start flows; the camera mark node records on a camera (spec 013).
func TestCameraEventAndMark(t *testing.T) {
	js := `{"nodes":[{"id":"c","type":"camera_event","settings":{"camera":"` + srcA + `","kinds":["motion"],"phases":["start","end"]}},
	 {"id":"d","type":"debug","settings":{"expr":"msg.kind + ' ' + msg.phase + ' ' + value + ' ' + topic"}},
	 {"id":"mk","type":"camera_mark","settings":{"camera":"` + srcA + `","label":"{{topic}} {{msg.kind}}","seconds":45}}],
	 "wires":[{"from":"c","port":0,"to":"d"},{"from":"c","port":0,"to":"mk"}]}`
	e, _, h, cancel := testEngine(t, js)
	defer cancel()
	ev := func(cam, kind, phase string) {
		e.OnEvent(live.Event{Type: "camera", Org: "org", State: phase, Detail: map[string]any{"camera_id": cam, "camera": "Gate", "kind": kind, "phase": phase, "event_id": int64(7)}})
	}
	ev(srcA, "tamper", "start")                                   // other kind
	ev("00000000-0000-4000-8000-000000000000", "motion", "start") // other camera
	ev(srcA, "motion", "start")
	ev(srcA, "motion", "end")
	waitFor(t, "camera events", func() bool { return len(debugTexts(e, "debug")) >= 2 })
	got := strings.Join(debugTexts(e, "debug"), " | ")
	if got != "motion start true Gate | motion end false Gate" {
		t.Fatalf("debug: %s", got)
	}
	waitFor(t, "marks", func() bool { h.mu.Lock(); defer h.mu.Unlock(); return len(h.marks) == 2 })
	if h.marks[0] != srcA+" | Gate motion | 45" {
		t.Fatalf("marks %v", h.marks)
	}
}

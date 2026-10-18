package opcua

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/opcua/opcuasim"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func startSim(t *testing.T, secure bool) (*opcuasim.Sim, string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	port := freePort(t)
	sim, err := opcuasim.Start(ctx, opcuasim.Config{Host: "127.0.0.1", Port: port, Secure: secure})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); sim.Close() })
	return sim, fmt.Sprintf("opc.tcp://127.0.0.1:%d", port)
}

func newManager(t *testing.T) *Manager {
	return New(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), t.TempDir())
}

func settings(t *testing.T, raw string) Settings {
	t.Helper()
	st, err := ValidateSettings(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("settings %s: %v", raw, err)
	}
	return st
}

func TestValidateSettings(t *testing.T) {
	bad := map[string]string{
		`{"endpoint":"opc.tcp://h:4840","mode":"stream","nodes":[{"node_id":"i=1","key":"a"}]}`:                                           "mode",
		`{"endpoint":"opc.tcp://h:4840","mode":"subscribe","publishing_ms":50,"nodes":[{"node_id":"i=1","key":"a"}]}`:                     "publishing",
		`{"endpoint":"opc.tcp://h:4840","server_thumbprint":"abc","nodes":[{"node_id":"i=1","key":"a"}]}`:                                 "thumbprint",
		`{"endpoint":"opc.tcp://h:4840","nodes":[{"node_id":"i=1","key":"a"},{"node_id":"i=2","key":"a"}]}`:                               "duplicate",
		`{"endpoint":"opc.tcp://h:4840","nodes":[{"node_id":"i=1","key":"a","device":"bad id/x"}]}`:                                       "device id",
		`{"endpoint":"opc.tcp://h:4840","nodes":[{"node_id":"i=1","key":"a","deadband":-1}]}`:                                             "deadband",
		`{"endpoint":"opc.tcp://h:4840","device_external_id":"x y","nodes":[{"node_id":"i=1","key":"a"}]}`:                                "virtual device",
		`{"endpoint":"opc.tcp://h:4840","security_policy":"Basic256Sha256","security_mode":"None","nodes":[{"node_id":"i=1","key":"a"}]}`: "policy None",
	}
	for raw, want := range bad {
		if _, err := ValidateSettings(json.RawMessage(raw)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want error containing %q", raw, err, want)
		}
	}
	// The same key on two devices is allowed; a node on the connector's own device id is normalized to "".
	st := settings(t, `{"endpoint":"opc.tcp://h:4840","device_external_id":"own","mode":"subscribe","server_thumbprint":"AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD","nodes":[{"node_id":"i=1","key":"a"},{"node_id":"i=2","key":"a","device":"line1"},{"node_id":"i=3","key":"b","device":"own"}]}`)
	if st.PublishingMs != 1000 || st.Mode != "subscribe" || st.Nodes[2].Device != "" || st.ServerThumbprint != "aabbccddeeff00112233445566778899aabbccdd" {
		t.Fatalf("normalized: %+v", st)
	}
	if st.ref(st.Nodes[1]) != "line1/a" || st.ref(st.Nodes[0]) != "a" {
		t.Fatal("ref")
	}
	if _, err := ValidateConnection(json.RawMessage(`{"endpoint":"opc.tcp://h:4840"}`)); err != nil {
		t.Fatalf("connection without nodes: %v", err)
	}
}

func TestReadBrowseEndpoints(t *testing.T) {
	sim, ep := startSim(t, false)
	m := newManager(t)
	ctx := context.Background()
	ns := sim.Namespace
	st := settings(t, fmt.Sprintf(`{"endpoint":%q,"nodes":[{"node_id":"ns=%d;s=Sinusoid","key":"sinus"},{"node_id":"ns=%d;s=Line1.Speed","key":"speed","device":"line1"},{"node_id":"ns=%d;s=Line2.Speed","key":"speed","device":"line2"},{"node_id":"ns=%d;s=Missing","key":"missing"}]}`, ep, ns, ns, ns, ns))
	values, bad, _, err := m.Test(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := values["sinus"].(float64); !ok {
		t.Errorf("sinus: %v", values)
	}
	if s1, s2 := values["line1/speed"], values["line2/speed"]; s1 == nil || s2 == nil {
		t.Errorf("per-device values: %v", values)
	}
	if len(bad) != 1 || !strings.HasPrefix(bad[0], "missing:") {
		t.Errorf("bad: %v", bad)
	}

	root, err := m.Browse(ctx, st, "")
	if err != nil {
		t.Fatal(err)
	}
	var sim0 *Item
	for i := range root.Items {
		if root.Items[i].DisplayName == "Simulation" || root.Items[i].BrowseName == "Simulation" {
			sim0 = &root.Items[i]
		}
	}
	if sim0 == nil {
		t.Fatalf("Objects has no Simulation folder: %+v", root.Items)
	}
	objs, err := m.Browse(ctx, st, sim0.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Item{}
	for _, it := range objs.Items {
		byName[it.DisplayName] = it
	}
	if p, ok := byName["Plant"]; !ok || p.NodeClass != "Object" || !p.HasChildren {
		t.Fatalf("Plant: %+v", objs.Items)
	}
	if s, ok := byName["Setpoint"]; !ok || !s.Writable || s.DataType != "Double" {
		t.Errorf("Setpoint: %+v", s)
	}
	if s, ok := byName["Sinusoid"]; !ok || s.Writable || !s.Readable || s.Value == nil {
		t.Errorf("Sinusoid: %+v", s)
	}
	if objs.Items[0].NodeClass == "Variable" {
		t.Error("objects should be listed before variables")
	}
	line, err := m.Browse(ctx, st, fmt.Sprintf("ns=%d;s=Line1", ns))
	if err != nil {
		t.Fatal(err)
	}
	if len(line.Items) != 4 {
		t.Errorf("Line1 children: %+v", line.Items)
	}
	if _, err := m.Browse(ctx, st, "nonsense"); err == nil {
		t.Error("invalid node id accepted")
	}

	eps, err := m.Endpoints(ctx, ep)
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) == 0 || eps[0].SecurityPolicy != "None" || !eps[0].Supported || len(eps[0].UserTokens) == 0 || eps[0].UserTokens[0] != "anonymous" {
		t.Errorf("endpoints: %+v", eps)
	}
}

func TestSecureAndPinnedThumbprint(t *testing.T) {
	sim, ep := startSim(t, true)
	m := newManager(t)
	ctx := context.Background()
	eps, err := m.Endpoints(ctx, ep)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range eps {
		if e.SecurityPolicy == "Basic256Sha256" && e.SecurityMode == "SignAndEncrypt" && e.Thumbprint == sim.Thumbprint {
			found = true
		}
	}
	if !found {
		t.Fatalf("no Basic256Sha256 endpoint with thumbprint %s: %+v", sim.Thumbprint, eps)
	}
	node := fmt.Sprintf(`[{"node_id":"ns=%d;s=Sinusoid","key":"sinus"}]`, sim.Namespace)
	ok := settings(t, fmt.Sprintf(`{"endpoint":%q,"security_policy":"Basic256Sha256","security_mode":"SignAndEncrypt","server_thumbprint":%q,"nodes":%s}`, ep, sim.Thumbprint, node))
	values, _, thumb, err := m.Test(ctx, ok)
	if err != nil {
		t.Fatalf("pinned secure read: %v", err)
	}
	if thumb != sim.Thumbprint || values["sinus"] == nil {
		t.Errorf("thumb %s values %v", thumb, values)
	}
	wrong := settings(t, fmt.Sprintf(`{"endpoint":%q,"security_policy":"Basic256Sha256","security_mode":"SignAndEncrypt","server_thumbprint":%q,"nodes":%s}`, ep, strings.Repeat("0", 40), node))
	if _, _, thumb, err := m.Test(ctx, wrong); err == nil || !strings.Contains(err.Error(), "does not match") || thumb != sim.Thumbprint {
		t.Errorf("wrong pin: thumb %s err %v", thumb, err)
	}
}

func TestSubscribeAndWrite(t *testing.T) {
	sim, ep := startSim(t, false)
	m := newManager(t)
	var mu sync.Mutex
	got := map[string]map[string]any{}
	notes := 0
	m.sink = func(dev string, vals map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		if got[dev] == nil {
			got[dev] = map[string]any{}
		}
		for k, v := range vals {
			got[dev][k] = v
		}
		notes++
	}
	ns := sim.Namespace
	st := settings(t, fmt.Sprintf(`{"endpoint":%q,"mode":"subscribe","publishing_ms":200,"nodes":[{"node_id":"ns=%d;s=Counter","key":"counter"},{"node_id":"ns=%d;s=Line1.Count","key":"count","device":"line1"},{"node_id":"ns=%d;s=Setpoint","key":"sp","writable":true,"min":0,"max":100},{"node_id":"ns=%d;s=Missing","key":"missing"}]}`, ep, ns, ns, ns, ns))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &worker{m: m, row: connRow{ID: "c1", Name: "sim", Settings: st}, st: &Status{}, cancel: cancel}
	m.mu.Lock()
	m.workers["c1"], m.status["c1"] = w, w.st
	m.mu.Unlock()
	go w.run(ctx)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n, c1, c2 := notes, got[""]["counter"], got["line1"]["count"]
		mu.Unlock()
		if n >= 3 && c1 != nil && c2 != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	mu.Lock()
	if notes < 3 || got[""]["counter"] == nil || got["line1"]["count"] == nil {
		t.Fatalf("notifications %d values %v", notes, got)
	}
	mu.Unlock()
	s := m.Status("c1")
	if s.Mode != "subscribe" || s.State != "connected" || s.Notifications < 2 || len(s.BadNodes) != 1 {
		t.Errorf("status: %+v", s)
	}
	if w.live() == nil {
		t.Fatal("no live client")
	}
	// A write goes over the live session and is read back.
	op := &WriteOp{ConnectorID: "c1", ConnectorName: "sim", Settings: st, Node: *st.node("", "sp"), Value: 42.5}
	res, err := m.Do(ctx, op)
	if err != nil {
		t.Fatal(err)
	}
	if res["readback"] != 42.5 {
		t.Errorf("readback: %v", res)
	}
	if m.Status("c1").Writes != 1 {
		t.Error("writes not counted")
	}
}

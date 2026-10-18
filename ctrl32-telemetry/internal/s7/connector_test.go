package s7_test

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

	"github.com/telemetry-digital/ctrl32-telemetry/internal/s7"
)

func TestValidateSettings(t *testing.T) {
	bad := map[string]string{
		`{"host":"","tags":[{"key":"a","address":"DB1.DBD0"}]}`:                           "host",
		`{"host":"plc","plc":"s5","tags":[{"key":"a","address":"DB1.DBD0"}]}`:             "plc must be",
		`{"host":"plc","tags":[]}`:                                                        "at least one tag",
		`{"host":"plc","tags":[{"key":"A","address":"DB1.DBD0"}]}`:                        "key must match",
		`{"host":"plc","tags":[{"key":"a","address":"DB1.DBQ0"}]}`:                        "tag 1 (a)",
		`{"host":"plc","tags":[{"key":"a","address":"DB1.DBW0","type":"real"}]}`:          "tag 1 (a)",
		`{"host":"plc","tags":[{"key":"a","address":"I0.0","writable":true}]}`:            "inputs cannot be written",
		`{"host":"plc","tags":[{"key":"a","address":"MW0"},{"key":"a","address":"MW2"}]}`: "duplicate key",
		`{"host":"plc","tags":[{"key":"a","address":"MW0","min":5,"max":1}]}`:             "min must not exceed max",
		`{"host":"plc","conn_type":"x","tags":[{"key":"a","address":"MW0"}]}`:             "connection type",
		`{"host":"plc","local_tsap":"zz","tags":[{"key":"a","address":"MW0"}]}`:           "local TSAP",
		`{"host":"plc","plc":"other","slot":40,"tags":[{"key":"a","address":"MW0"}]}`:     "slot",
		`{"host":"plc","tags":[{"key":"a","address":"MW0","device":"bad id"}]}`:           "device id",
		`{"host":"plc","interval_s":0.5,"tags":[{"key":"a","address":"MW0"}]}`:            "settings",
	}
	for raw, want := range bad {
		if _, err := s7.ValidateSettings(json.RawMessage(raw)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want %q", raw, err, want)
		}
	}
	st, err := s7.ValidateSettings(json.RawMessage(`{"host":"10.0.0.5","plc":"s7-300","tags":[{"key":"t","address":"db1.dbd0"},{"key":"t","address":"DB1,INT4","device":"line2"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if st.Port != 102 || *st.Rack != 0 || *st.Slot != 2 || st.ConnType != "pg" || st.IntervalS != 10 || st.Tags[0].Type != "real" || st.Tags[1].Type != "int" {
		t.Fatalf("defaults: %+v", st)
	}
	logo, _ := s7.ValidateSettings(json.RawMessage(`{"host":"logo","plc":"logo","tags":[{"key":"v","address":"VW10"}]}`))
	if logo.LocalTSAP != "01.00" || logo.RemoteTSAP != "02.00" {
		t.Fatalf("logo TSAPs: %+v", logo)
	}
	s1500, _ := s7.ValidateSettings(json.RawMessage(`{"host":"p","plc":"s7-1500","slot":0,"tags":[{"key":"v","address":"MW10"}]}`))
	if *s1500.Slot != 0 {
		t.Fatal("explicit slot 0 must win over the family default")
	}
}

func settingsFor(t *testing.T, port int, extra string) s7.Settings {
	t.Helper()
	st, err := s7.ValidateSettings(json.RawMessage(fmt.Sprintf(`{"host":"127.0.0.1","port":%d,"plc":"s7-1200","interval_s":1,"timeout_ms":2000,%s}`, port, extra)))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestConnectorPollAndWrite(t *testing.T) {
	sim, port := startSim(t, 240, false)
	st := settingsFor(t, port, `"tags":[
		{"key":"sinus","address":"DB1.DBD0"},
		{"key":"counter","address":"DB1.DBW4"},
		{"key":"run","address":"DB1.DBX6.0","writable":true},
		{"key":"sp","address":"DB1.DBD8","writable":true,"min":0,"max":100,"scale":10},
		{"key":"txt","address":"DB1.DBS20.20","writable":true},
		{"key":"mw","address":"MW10","device":"line2"},
		{"key":"gone","address":"DB99.DBW0"}]`)
	m := s7.NewManager(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	res, err := m.Test(context.Background(), st)
	if err != nil {
		t.Fatal(err)
	}
	if res.PDU != 240 || res.Values["counter"] == nil || res.Values["line2/mw"] != 1234.0 || res.Values["sp"] != 215.0 || res.Values["txt"] != "hello" {
		t.Errorf("test read: %+v", res)
	}
	if len(res.Bad) != 1 || !strings.HasPrefix(res.Bad[0], "gone (DB99.DBW0)") {
		t.Errorf("bad: %v", res.Bad)
	}

	var mu sync.Mutex
	got := map[string]map[string]any{}
	m.SetSink(func(dev string, vals map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		got[dev] = vals
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.StartWorker(ctx, "c1", st)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		ok := got[""] != nil && got["line2"] != nil
		mu.Unlock()
		if ok && m.LiveConn("c1") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	mu.Lock()
	if got["line2"]["mw"] != 1234.0 || got[""]["run"] == nil {
		t.Fatalf("ingested: %v", got)
	}
	mu.Unlock()
	if s := m.Status("c1"); s == nil || s.State != "connected" || s.PDU != 240 || len(s.BadNodes) != 1 {
		t.Fatalf("status: %+v", s)
	}

	// Scaled real: 55 (engineering) is written as 5.5 and read back as 55.
	out, err := m.Do(ctx, s7.WriteFor("c1", st, "", "sp", 55.0))
	if err != nil {
		t.Fatal(err)
	}
	if out["readback"] != 55.0 {
		t.Errorf("readback: %v", out)
	}
	if b := sim.DB(1, 8, 4); fmt.Sprintf("% x", b) != "40 b0 00 00" {
		t.Errorf("DB1.DBD8 = % x, want 5.5", b)
	}
	if out, err := m.Do(ctx, s7.WriteFor("c1", st, "", "txt", "ahoj")); err != nil || out["readback"] != "ahoj" {
		t.Errorf("string write: %v %v", out, err)
	}
	if out, err := m.Do(ctx, s7.WriteFor("c1", st, "", "run", true)); err != nil || out["readback"] != true {
		t.Errorf("bool write: %v %v", out, err)
	}
	if s := m.Status("c1"); s.Writes != 3 {
		t.Errorf("writes: %d", s.Writes)
	}
}

func TestConnectorPutGetOff(t *testing.T) {
	_, port := startSim(t, 480, true)
	st := settingsFor(t, port, `"tags":[{"key":"a","address":"DB1.DBD0"}]`)
	m := s7.NewManager(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := m.Test(context.Background(), st); err == nil || !strings.Contains(err.Error(), "PUT/GET") {
		t.Fatalf("want PUT/GET hint, got %v", err)
	}
	if _, err := m.Test(context.Background(), settingsFor(t, 1, `"tags":[{"key":"a","address":"DB1.DBD0"}]`)); err == nil {
		t.Fatal("dead port accepted")
	}
}

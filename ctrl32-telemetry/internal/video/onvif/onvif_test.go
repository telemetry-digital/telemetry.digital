package onvif

import (
	"context"
	"errors"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/video/camsim"
)

func sim(t *testing.T) (*camsim.ONVIF, string) {
	ov := &camsim.ONVIF{RTSPHost: "127.0.0.1:8554", User: "admin", Pass: "secret"}
	ts := httptest.NewServer(ov)
	t.Cleanup(ts.Close)
	ov.HTTPHost = strings.TrimPrefix(ts.URL, "http://")
	return ov, ts.URL
}

func TestProfilesAndAuth(t *testing.T) {
	_, base := sim(t)
	ctx := context.Background()
	c := &Client{Device: base + "/onvif/gate/device_service", Username: "admin", Password: "secret"}
	ps, err := c.Profiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 || ps[0].URI != "rtsp://127.0.0.1:8554/gate/h264/640x352" || ps[1].Width != 256 || !ps[0].PTZ || ps[0].Encoding != "H264" {
		t.Fatalf("profiles %+v", ps)
	}
	bad := &Client{Device: base + "/onvif/gate/device_service", Username: "admin", Password: "wrong"}
	if _, err := bad.Profiles(ctx); !errors.Is(err, ErrAuth) {
		t.Fatalf("wrong password: %v", err)
	}
}

func TestMotionEvents(t *testing.T) {
	ov, base := sim(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := &Client{Device: base + "/onvif/yard/device_service", Username: "admin", Password: "secret"}
	got := make(chan Event, 10)
	go func() { _ = c.Subscribe(ctx, func(e Event) { got <- e }) }()
	first := <-got
	if first.Kind != "motion" || first.Active || !first.Init {
		t.Fatalf("initial %+v", first)
	}
	ov.SetMotion("yard", true)
	e := <-got
	if e.Kind != "motion" || !e.Active || e.Init || e.Topic != "tns1:RuleEngine/CellMotionDetector/Motion" {
		t.Fatalf("motion %+v", e)
	}
	ov.SetMotion("yard", false)
	if e := <-got; e.Active {
		t.Fatalf("motion end %+v", e)
	}
}

func TestPTZ(t *testing.T) {
	ov, base := sim(t)
	ctx := context.Background()
	c := &Client{Device: base + "/onvif/cam/device_service", Username: "admin", Password: "secret"}
	if err := c.Move(ctx, "main", 1, 0, 0.5); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if err := c.Stop(ctx, "main"); err != nil {
		t.Fatal(err)
	}
	pan, _, zoom := ov.View("cam")
	if pan <= 0.05 || zoom <= 0.02 {
		t.Fatalf("pan %v zoom %v", pan, zoom)
	}
	pr, err := c.Presets(ctx, "main")
	if err != nil || len(pr) != 3 || pr[1].Name != "Gate" {
		t.Fatalf("presets %v %v", pr, err)
	}
	if err := c.GotoPreset(ctx, "main", "3"); err != nil {
		t.Fatal(err)
	}
	if pan, _, _ := ov.View("cam"); pan < 0.69 {
		t.Fatalf("preset pan %v", pan)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		topic  string
		data   map[string]string
		kind   string
		active bool
		pulse  bool
	}{
		{"tns1:RuleEngine/CellMotionDetector/Motion", map[string]string{"IsMotion": "true"}, "motion", true, false},
		{"tns1:VideoSource/MotionAlarm", map[string]string{"State": "false"}, "motion", false, false},
		{"tns1:RuleEngine/TamperDetector/Tamper", map[string]string{"IsTamper": "true"}, "tamper", true, false},
		{"tns1:RuleEngine/LineDetector/Crossed", map[string]string{"ObjectId": "7"}, "line", true, true},
		{"tns1:RuleEngine/FieldDetector/ObjectsInside", map[string]string{"IsInside": "1"}, "intrusion", true, false},
		{"tns1:Device/Trigger/DigitalInput", map[string]string{"LogicalState": "true"}, "input", true, false},
	}
	for _, c := range cases {
		k, a, p := Classify(c.topic, c.data)
		if k != c.kind || a != c.active || p != c.pulse {
			t.Errorf("%s: %s %v %v", c.topic, k, a, p)
		}
	}
}

func TestDiscovery(t *testing.T) {
	ov, _ := sim(t)
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	go ov.ServeDiscovery(conn, []string{"gate", "yard"})
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	devs, err := Discover(ctx, conn.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	mine := devs[:0]
	for _, d := range devs { // other simulators in the network may answer the multicast probe too
		if strings.Contains(d.Address, ov.HTTPHost) {
			mine = append(mine, d)
		}
	}
	devs = mine
	if len(devs) != 2 || devs[0].Name == "" || !strings.HasSuffix(devs[0].Address, "/device_service") || devs[0].Hardware != "camsim" {
		t.Fatalf("devices %+v", devs)
	}
}

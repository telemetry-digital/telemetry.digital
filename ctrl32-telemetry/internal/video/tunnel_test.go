package video

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/video/camsim"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/video/onvif"
)

// relayAPI mirrors the server's relay endpoints (internal/httpapi/video_relay.go) for the video package's tests.
func relayAPI(m *Manager) http.Handler {
	mux := http.NewServeMux()
	fail := func(w http.ResponseWriter, err error) {
		switch {
		case errors.Is(err, ErrRelayBlocked):
			w.WriteHeader(http.StatusTooManyRequests)
		case errors.Is(err, ErrRelayAuth):
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusConflict)
		}
	}
	mux.HandleFunc("GET /api/v1/video/relay/{id}/onvif/next", func(w http.ResponseWriter, r *http.Request) {
		wait := 25 * time.Second
		if r.URL.Query().Get("wait") == "0" {
			wait = 0
		}
		rq, err := m.RelayNext(r.Context(), r.PathValue("id"), r.Header.Get(relaySecretHeader), "127.0.0.1", wait)
		if err != nil {
			fail(w, err)
			return
		}
		if rq == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_ = json.NewEncoder(w).Encode(rq)
	})
	mux.HandleFunc("POST /api/v1/video/relay/{id}/onvif/{rid}", func(w http.ResponseWriter, r *http.Request) {
		if err := m.RelayAuth(r.PathValue("id"), r.Header.Get(relaySecretHeader), "127.0.0.1"); err != nil {
			fail(w, err)
			return
		}
		var in TunnelResponse
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&in); err != nil {
			w.WriteHeader(400)
			return
		}
		if !m.RelayAnswer(r.PathValue("id"), r.PathValue("rid"), in) {
			w.WriteHeader(http.StatusGone)
		}
	})
	return mux
}

// onvifSim serves the ONVIF simulator on a local port.
func onvifSim(t *testing.T) (*camsim.ONVIF, string) {
	sim := &camsim.ONVIF{User: "admin", Pass: "pw", Codec: "h264", RTSPHost: "127.0.0.1:554"}
	srv := httptest.NewServer(sim)
	t.Cleanup(srv.Close)
	sim.HTTPHost = strings.TrimPrefix(srv.URL, "http://")
	return sim, srv.URL
}

func TestTunnelRoundTripPTZ(t *testing.T) {
	sim, camURL := onvifSim(t)
	secret := NewPushSecret()
	m, _ := pushManager(secret)
	api := httptest.NewServer(relayAPI(m))
	defer api.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := RelayConfig{Server: "rtsp://127.0.0.1:8322", Web: api.URL, Cameras: []RelayCamera{{ID: pushCam, Secret: secret, Source: "rtsp://x/y", ONVIF: camURL}}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	go relayONVIF(ctx, cfg, cfg.Cameras[0], slog.New(slog.NewTextHandler(io.Discard, nil)))

	deadline := time.Now().Add(5 * time.Second)
	for !m.tunnelConnected(pushCam) {
		if time.Now().After(deadline) {
			t.Fatal("relay did not connect for camera control")
		}
		time.Sleep(20 * time.Millisecond)
	}
	hc, err := m.tunnelClient(pushCam, camURL+"/onvif/lot/device_service")
	if err != nil {
		t.Fatal(err)
	}
	cl := &onvif.Client{Device: camURL + "/onvif/lot/device_service", Username: "admin", Password: "pw", HTTP: hc}
	cctx, ccancel := context.WithTimeout(ctx, 10*time.Second)
	defer ccancel()
	ps, err := cl.Presets(cctx, "main")
	if err != nil || len(ps) != 3 {
		t.Fatalf("presets through the relay: %v %v", ps, err)
	}
	if err := cl.GotoPreset(cctx, "main", "2"); err != nil {
		t.Fatal(err)
	}
	if p, _, _ := sim.View("lot"); p > -0.5 {
		t.Fatalf("preset did not reach the camera: pan %v", p)
	}
	if err := cl.Move(cctx, "main", 0.5, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := cl.Stop(cctx, "main"); err != nil {
		t.Fatal(err)
	}
	// events: a pull-point subscription through the same channel
	got := make(chan onvif.Event, 4)
	ectx, ecancel := context.WithTimeout(ctx, 8*time.Second)
	defer ecancel()
	go func() { _ = cl.Subscribe(ectx, func(e onvif.Event) { got <- e }) }()
	select {
	case e := <-got:
		if e.Kind != "motion" || !e.Init {
			t.Fatalf("first event %+v", e)
		}
	case <-ectx.Done():
		t.Fatal("no event through the relay")
	}
	sim.SetMotion("lot", true)
	select {
	case e := <-got:
		if e.Kind != "motion" || !e.Active {
			t.Fatalf("motion %+v", e)
		}
	case <-ectx.Done():
		t.Fatal("motion not delivered through the relay")
	}
	// a wrong ONVIF password still reaches the camera and is refused by it
	bad := &onvif.Client{Device: camURL + "/onvif/lot/device_service", Username: "admin", Password: "wrong", HTTP: hc}
	if _, err := bad.Presets(cctx, "main"); !errors.Is(err, onvif.ErrAuth) {
		t.Fatalf("wrong password: %v", err)
	}
}

func TestTunnelNotConnectedAndTimeout(t *testing.T) {
	m, _ := pushManager(NewPushSecret())
	hc, _ := m.tunnelClient(pushCam, "http://192.168.0.21:2020/onvif/device_service")
	cl := &onvif.Client{Device: "http://192.168.0.21:2020/onvif/device_service", HTTP: hc}
	start := time.Now()
	if _, err := cl.Capabilities(context.Background()); !errors.Is(err, ErrRelayNotConnected) {
		t.Fatalf("without a relay: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("a missing relay must fail at once")
	}
	// a relay that polls but never answers: the request gives up after tunnelWait
	old := tunnelWait
	tunnelWait = 300 * time.Millisecond
	defer func() { tunnelWait = old }()
	tn := m.tunnelOf(pushCam, true)
	tn.mu.Lock()
	tn.seen = time.Now()
	tn.mu.Unlock()
	start = time.Now()
	_, err := hc.Post("http://192.168.0.21:2020/onvif/device_service", "application/soap+xml", strings.NewReader("<x/>"))
	if err == nil || !strings.Contains(err.Error(), "in time") {
		t.Fatalf("timeout: %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("timeout took %v", d)
	}
	// an answer to an expired request is refused
	if m.RelayAnswer(pushCam, "nope", TunnelResponse{Status: 200}) {
		t.Fatal("answer to an unknown request accepted")
	}
	// once the relay has not polled for a while it counts as disconnected
	tn.mu.Lock()
	tn.seen = time.Now().Add(-time.Minute)
	tn.mu.Unlock()
	if m.tunnelConnected(pushCam) {
		t.Fatal("stale relay still connected")
	}
}

func TestTunnelAuthBlocks(t *testing.T) {
	secret := NewPushSecret()
	m, _ := pushManager(secret)
	ctx := context.Background()
	if _, err := m.RelayNext(ctx, pushCam, secret, "10.0.0.1", 0); err != nil {
		t.Fatalf("right secret: %v", err)
	}
	for i := 0; i < m.Settings().PushMaxFails; i++ {
		if _, err := m.RelayNext(ctx, pushCam, "wrong-secret-0123456789", "10.0.0.2", 0); !errors.Is(err, ErrRelayAuth) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, err := m.RelayNext(ctx, pushCam, secret, "10.0.0.2", 0); !errors.Is(err, ErrRelayBlocked) {
		t.Fatalf("blocked address accepted: %v", err)
	}
	if err := m.RelayAuth("00000000-0000-0000-0000-000000000000", secret, "10.0.0.3"); !errors.Is(err, ErrRelayAuth) {
		t.Fatalf("unknown camera: %v", err)
	}
	if err := m.RelayAuth(pushCam, "", "10.0.0.3"); !errors.Is(err, ErrRelayAuth) {
		t.Fatalf("empty secret: %v", err)
	}
	// a pulled camera has no relay
	m.cams[pushCam].cfg.Push = false
	if err := m.RelayAuth(pushCam, secret, "10.0.0.4"); !errors.Is(err, ErrRelayAuth) {
		t.Fatalf("pulled camera: %v", err)
	}
	// the RTSP push port shares the blocking
	if !(&PushServer{m: m}).blocked("10.0.0.2") {
		t.Fatal("blocking not shared with the push listener")
	}
}

func TestTunnelTransportRefuses(t *testing.T) {
	m, _ := pushManager(NewPushSecret())
	tn := m.tunnelOf(pushCam, true)
	tn.mu.Lock()
	tn.seen = time.Now()
	tn.mu.Unlock()
	hc, _ := m.tunnelClient(pushCam, "http://192.168.0.21:2020/onvif/device_service")
	for _, u := range []string{"http://192.168.0.22:2020/onvif/device_service", "https://192.168.0.21:2020/onvif/x", "http://192.168.0.21/onvif/x", "http://192.168.0.21:2020/cgi-bin/x"} {
		if _, err := hc.Post(u, "text/xml", strings.NewReader("x")); err == nil || !strings.Contains(err.Error(), "refused") {
			t.Errorf("%s: %v", u, err)
		}
	}
	req, _ := http.NewRequest(http.MethodPut, "http://192.168.0.21:2020/onvif/x", nil)
	if _, err := hc.Do(req); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Errorf("PUT: %v", err)
	}
	if _, err := hc.Post("http://192.168.0.21:2020/onvif/x", "text/xml", bytes.NewReader(make([]byte, tunnelMaxBody+1))); err == nil || !strings.Contains(err.Error(), "1 MiB") {
		t.Errorf("large body: %v", err)
	}
	if len(tn.queue) != 0 {
		t.Fatal("a refused request reached the relay")
	}
}

func TestRelayAllowsOnlyTheCameraOrigin(t *testing.T) {
	const origin = "http://192.168.0.21:2020"
	good := []string{"/onvif/device_service", "/onvif/service", "/onvif/events/sub/12", "/onvif/x?a=1"}
	for _, p := range good {
		u, err := relayTarget(origin, &TunnelRequest{Method: "POST", Path: p})
		if err != nil || !strings.HasPrefix(u, origin+"/onvif/") {
			t.Errorf("%s: %q %v", p, u, err)
		}
	}
	bad := []string{"", "/", "/cgi-bin/x", "onvif/x", "//evil.example/onvif/x", "/onvif/../cgi-bin/x", "/onvif/%2e%2e/x", "/onvif/x/../../y",
		"/onvif/@evil/x", "/onvif/x#y", "/onvif/x\r\nHost: evil", "/onvif//x", "/onvif/a\\b", "http://evil/onvif/x"}
	for _, p := range bad {
		if u, err := relayTarget(origin, &TunnelRequest{Method: "POST", Path: p}); err == nil {
			t.Errorf("%q allowed as %q", p, u)
		}
	}
	for _, meth := range []string{"PUT", "DELETE", "CONNECT", ""} {
		if _, err := relayTarget(origin, &TunnelRequest{Method: meth, Path: "/onvif/x"}); err == nil {
			t.Errorf("method %q allowed", meth)
		}
	}
	if _, err := relayTarget(origin, &TunnelRequest{Method: "POST", Path: "/onvif/x", Body: make([]byte, tunnelMaxBody+1)}); err == nil {
		t.Error("large body allowed")
	}
	for raw, want := range map[string]string{"http://192.168.0.21:2020": origin, "http://192.168.0.21:2020/": origin,
		"http://192.168.0.21:2020/onvif/device_service": origin, "HTTP://Cam.Local": "http://cam.local"} {
		if o, err := relayOrigin(raw); err != nil || o != want {
			t.Errorf("origin %s: %q %v", raw, o, err)
		}
	}
	for _, raw := range []string{"rtsp://192.168.0.21", "http://admin:pw@192.168.0.21:2020", "http://192.168.0.21/cgi-bin/", "http://192.168.0.21/?x=1", "192.168.0.21:2020"} {
		if _, err := relayOrigin(raw); err == nil {
			t.Errorf("origin %s accepted", raw)
		}
	}
}

func TestRelayPerformLimitsAndRedirects(t *testing.T) {
	var got []string
	camSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/onvif/big":
			_, _ = w.Write(make([]byte, tunnelMaxBody+10))
		case "/onvif/redirect":
			http.Redirect(w, r, "http://169.254.169.254/latest", http.StatusFound)
		default:
			w.Header().Set("Content-Type", "application/soap+xml")
			w.Header().Set("Set-Cookie", "x=1")
			_, _ = w.Write([]byte("<ok/>"))
		}
	}))
	defer camSrv.Close()
	origin, _ := relayOrigin(camSrv.URL)
	hc := cameraClient()
	ctx := context.Background()
	r := relayPerform(ctx, hc, origin, &TunnelRequest{Method: "POST", Path: "/onvif/device_service", Header: map[string]string{"Content-Type": "application/soap+xml", "Cookie": "no"}, Body: []byte("<x/>")})
	if r.Status != 200 || string(r.Body) != "<ok/>" || r.Header["Content-Type"] == "" || len(r.Header) != 1 {
		t.Fatalf("answer %+v", r)
	}
	if r := relayPerform(ctx, hc, origin, &TunnelRequest{Method: "POST", Path: "/onvif/big"}); r.Error == "" || !strings.Contains(r.Error, "1 MiB") {
		t.Fatalf("large answer %+v", r.Error)
	}
	if r := relayPerform(ctx, hc, origin, &TunnelRequest{Method: "POST", Path: "/onvif/redirect"}); r.Status != http.StatusFound {
		t.Fatalf("redirect followed: %+v", r.Status)
	}
	if r := relayPerform(ctx, hc, origin, &TunnelRequest{Method: "POST", Path: "/admin"}); r.Error == "" {
		t.Fatal("path outside /onvif/ performed")
	}
	for _, g := range got {
		if !strings.HasPrefix(g, "POST /onvif/") {
			t.Fatalf("the camera saw %q", g)
		}
	}
	if len(got) != 3 {
		t.Fatalf("requests at the camera: %v", got)
	}
}

func TestRelayConfigONVIF(t *testing.T) {
	cam := RelayCamera{ID: pushCam, Secret: "0123456789abcdef", Source: "rtsp://a/b", ONVIF: "http://192.168.0.21:2020"}
	ok := RelayConfig{Server: "rtsps://telemetry.example.com:8322", Cameras: []RelayCamera{cam}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	if b, _ := ok.webBase(); b != "https://telemetry.example.com" {
		t.Fatalf("derived web %q", b)
	}
	bad := []RelayConfig{
		{Server: "rtsp://10.0.0.1:8322", Cameras: []RelayCamera{cam}},                  // no web without TLS
		{Server: "rtsps://x:8322", Web: "http://x", Cameras: []RelayCamera{cam}},       // secret in clear text
		{Server: "rtsps://x:8322", Web: "https://x/path", Cameras: []RelayCamera{cam}}, // not an origin
		{Server: "rtsps://x:8322", Cameras: []RelayCamera{{ID: pushCam, Secret: "0123456789abcdef", Source: "rtsp://a/b", ONVIF: "http://u:p@cam"}}},
	}
	for i, c := range bad {
		if c.Validate() == nil {
			t.Errorf("config %d accepted", i)
		}
	}
	lan := RelayConfig{Server: "rtsp://10.0.0.1:8322", Web: "http://10.0.0.1:8080", Cameras: []RelayCamera{cam}}
	if err := lan.Validate(); err != nil {
		t.Fatalf("intranet: %v", err)
	}
	if relayDevice("http://192.168.0.21:2020") != "http://192.168.0.21:2020/onvif/device_service" || relayDevice("http://c/onvif/dev") != "http://c/onvif/dev" {
		t.Fatal("device address")
	}
}

// TP-Link Tapo names its events subscription outside /onvif/ (/event-1025_1025): the SOAP calls to it go through the
// relay, and motion arrives (0.61.1).
func TestTunnelTapoEventsOutsideONVIF(t *testing.T) {
	sim, camURL := onvifSim(t)
	sim.TapoEvents = true
	secret := NewPushSecret()
	m, _ := pushManager(secret)
	api := httptest.NewServer(relayAPI(m))
	defer api.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := RelayConfig{Server: "rtsp://127.0.0.1:8322", Web: api.URL, Cameras: []RelayCamera{{ID: pushCam, Secret: secret, Source: "rtsp://x/y", ONVIF: camURL}}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	go relayONVIF(ctx, cfg, cfg.Cameras[0], slog.New(slog.NewTextHandler(io.Discard, nil)))
	for deadline := time.Now().Add(5 * time.Second); !m.tunnelConnected(pushCam); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("relay did not connect for camera control")
		}
	}
	hc, err := m.tunnelClient(pushCam, camURL+"/onvif/gate/device_service")
	if err != nil {
		t.Fatal(err)
	}
	cl := &onvif.Client{Device: camURL + "/onvif/gate/device_service", Username: "admin", Password: "pw", HTTP: hc}
	got := make(chan onvif.Event, 4)
	ectx, ecancel := context.WithTimeout(ctx, 8*time.Second)
	defer ecancel()
	go func() { _ = cl.Subscribe(ectx, func(e onvif.Event) { got <- e }) }()
	select {
	case e := <-got:
		if e.Kind != "motion" || !e.Init {
			t.Fatalf("first event %+v", e)
		}
	case <-ectx.Done():
		t.Fatal("no event from a Tapo-style subscription through the relay")
	}
	sim.SetMotion("gate", true)
	select {
	case e := <-got:
		if !e.Active {
			t.Fatalf("motion %+v", e)
		}
	case <-ectx.Done():
		t.Fatal("motion not delivered")
	}
}

// Outside /onvif/ only SOAP calls pass, on the server and on the relay.
func TestTunnelAllowedOutsideONVIF(t *testing.T) {
	env := []byte(`<?xml version="1.0" encoding="UTF-8"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body/></s:Envelope>`)
	soap := "application/soap+xml; charset=utf-8"
	for _, c := range []struct {
		method, path, ct string
		body             []byte
		ok               bool
	}{
		{"POST", "/onvif/device_service", soap, env, true},
		{"GET", "/onvif/snapshot", "", nil, true},
		{"POST", "/event-1025_1025", soap, env, true},
		{"POST", "/event-1025_1025", "text/xml", env, true},
		{"GET", "/event-1025_1025", "", nil, false},
		{"POST", "/cgi-bin/reboot", "application/x-www-form-urlencoded", []byte("a=1"), false},
		{"POST", "/cgi-bin/reboot", soap, []byte("reboot=1"), false},
		{"POST", "/", soap, env, false},
		{"PUT", "/onvif/x", soap, env, false},
	} {
		err := tunnelAllowed(c.method, c.path, c.ct, c.body)
		if (err == nil) != c.ok {
			t.Errorf("%s %s %q: %v", c.method, c.path, c.ct, err)
		}
		_, rerr := relayTarget("http://192.168.0.197:2020", &TunnelRequest{Method: c.method, Path: c.path, Header: map[string]string{"Content-Type": c.ct}, Body: c.body})
		if (rerr == nil) != c.ok {
			t.Errorf("relay %s %s %q: %v", c.method, c.path, c.ct, rerr)
		}
	}
	// the path checks stay the same outside /onvif/
	for _, p := range []string{"/event-1/../cgi-bin/x", "/event-1/%2e%2e/x", "//evil/x", "/a\b", "/x#y"} {
		if _, err := relayTarget("http://192.168.0.197:2020", &TunnelRequest{Method: "POST", Path: p, Header: map[string]string{"Content-Type": soap}, Body: env}); err == nil {
			t.Errorf("relay accepted %q", p)
		}
	}
}

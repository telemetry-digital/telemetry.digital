package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/agent"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

// The server's VPN settings need system.admin; peers, rules and grants are managed by the server administrator or
// by an organization's VPN manager (vpn.manage, limited to the own organization in the handlers; ADR-0019). Peer
// creation, rotation and access grants are not offered to AI assistants.
func TestVPNRoutesPermissions(t *testing.T) {
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`mux\.Handle\("([A-Z]+ /api/v1/(?:vpn|server/wireguard)[^"]*)", s\.require\("([a-z.|]*)"`)
	serverOnly := map[string]bool{"GET /api/v1/server/wireguard": true, "PUT /api/v1/server/wireguard": true, "POST /api/v1/vpn/apply": true}
	n := 0
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		n++
		want := "system.admin|vpn.manage"
		// the uplink of this server to a hub (phase 3) is server administration
		if serverOnly[m[1]] || strings.Contains(m[1], "/api/v1/vpn/uplink") {
			want = "system.admin"
		}
		if m[2] != want {
			t.Errorf("%s needs %q, want %q", m[1], m[2], want)
		}
	}
	if n != 18 {
		t.Fatalf("found %d VPN routes, want 18", n)
	}
	if !strings.Contains(string(src), `mux.HandleFunc("GET /api/v1/vpn/uplink/tunnel", s.uplinkTunnelWS)`) || vpn.UplinkTunnelPath != "/api/v1/vpn/uplink/tunnel" {
		t.Error("the HTTPS transport's endpoint moved")
	}
	for _, op := range []string{"POST /api/v1/vpn/peers", "POST /api/v1/vpn/peers/{id}/rotate", "POST /api/v1/vpn/peers/{id}/grant", "POST /api/v1/vpn/uplink"} {
		if mcpExclusions[op] == "" {
			t.Errorf("%s is offered to AI assistants", op)
		}
	}
	if strings.Contains(string(src), "/api/v1/server/wireguard/peers") {
		t.Error("the old peer operations bypass the database and the access rules")
	}
}

func TestVPNFailCodes(t *testing.T) {
	for _, c := range []struct {
		err  error
		code int
		key  string
	}{
		{&vpn.LicenceError{Msg: "Site peers … require a license."}, 402, "license_required"},
		{vpnBad("bad subnet"), 400, ""},
		{&vpnErr{code: 404, msg: "peer not found"}, 404, ""},
		{errors.New("database down"), 500, ""},
	} {
		w := httptest.NewRecorder()
		vpnFail(w, c.err)
		var body map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if w.Code != c.code || body["code"] != c.key || body["error"] == "" {
			t.Errorf("%v → %d %v", c.err, w.Code, body)
		}
	}
}

func TestParseExpiry(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	if e, err := parseExpiry("", now); e != nil || err != nil {
		t.Fatal("empty = no expiry")
	}
	if e, err := parseExpiry("2026-10-02T18:00:00+02:00", now); err != nil || e == nil || e.UTC().Hour() != 16 {
		t.Fatalf("future: %v %v", e, err)
	}
	for _, bad := range []string{"2026-10-02T12:00:30Z", "2026-10-01T00:00:00Z", "tomorrow", "2026-10-03"} {
		if _, err := parseExpiry(bad, now); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestVPNServices(t *testing.T) {
	keys := func(c *config.Config) string {
		s := &Server{cfg: c}
		var out []string
		for _, sv := range s.vpnServices(&agent.WireGuardState{}) {
			out = append(out, sv["key"].(string)+"="+sv["ports"].(string))
		}
		return strings.Join(out, " ")
	}
	c := &config.Config{}
	c.HTTP.Listen, c.HTTP.BaseURL = ":8080", "http://plant:8080"
	if got := keys(c); got != "mqtt=1883 web=8080 ssh=22" {
		t.Errorf("intranet: %s", got)
	}
	c.HTTP.Listen, c.HTTP.BaseURL, c.MQTT.TLSListen = "127.0.0.1:8080", "https://telemetry.example.com", ":8883"
	if got := keys(c); got != "mqtt=1883 web=443 mqtt_tls=8883 ssh=22" {
		t.Errorf("internet: %s", got)
	}
	c.HTTP.BaseURL, c.MQTT.TLSListen = "http://localhost:8080", ""
	if got := keys(c); got != "mqtt=1883 ssh=22" {
		t.Errorf("local only: %s", got)
	}
}

// Showing a configuration needs the password; after five failures a minute the user is refused before any check.
func TestStepUp(t *testing.T) {
	s := &Server{log: slog.New(slog.NewTextHandler(io.Discard, nil)), stepUpLimiter: newLimiter(5, 5)}
	p := &iam.Principal{UserID: "u1", Username: "admin"}
	req := func() (*httptest.ResponseRecorder, bool) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/v1/vpn/peers", nil)
		r = r.WithContext(context.WithValue(r.Context(), principalKey, p))
		return w, s.stepUp(w, r, "", "")
	}
	w, ok := req()
	if ok || w.Code != 403 || !strings.Contains(w.Body.String(), "step_up_required") {
		t.Fatalf("without password: %v %d %s", ok, w.Code, w.Body)
	}
	for i := 0; i < 5; i++ {
		s.stepUpLimiter.allow("u1") // five failed confirmations
	}
	if w, ok := req(); ok || w.Code != 429 {
		t.Fatalf("after five failures: %v %d", ok, w.Code)
	}
	if !s.stepUpLimiter.peek("someone-else") {
		t.Fatal("the limit is per user")
	}
}

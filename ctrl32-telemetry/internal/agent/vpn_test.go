package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

// fakeHost records the programs the agent runs and fails the ones a test names.
type fakeHost struct {
	cmds    []string
	fail    map[string]string // command prefix -> output of the failure
	fwd     []bool
	route   string            // output of `ip -4 -o route show dev wg0`
	outputs map[string]string // command prefix -> output (uplink tests)
	mu      sync.Mutex        // the uplink's endpoint watch runs commands from its own goroutine
}

func (f *fakeHost) exec(_ context.Context, _ time.Duration, name string, args ...string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := strings.TrimSpace(name + " " + strings.Join(args, " "))
	f.cmds = append(f.cmds, c)
	for p, out := range f.fail {
		if strings.HasPrefix(c, p) {
			return out, errors.New(name + ": " + out)
		}
	}
	for p, out := range f.outputs {
		if strings.HasPrefix(c, p) {
			return out, nil
		}
	}
	switch {
	case strings.HasPrefix(c, "wg-quick strip"):
		return readFile(wgConfFile), nil
	case strings.HasPrefix(c, "ip -4 -o route show"):
		return f.route, nil
	}
	return "", nil
}

func (f *fakeHost) ran(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, c := range f.cmds {
		if strings.HasPrefix(c, prefix) {
			return i
		}
	}
	return -1
}

func vpnAgent(t *testing.T, enabled bool) (*Server, *fakeHost) {
	t.Helper()
	dir := t.TempDir()
	etcDir, optDir = dir, dir
	wgConfFile = filepath.Join(dir, "wireguard", "wg0.conf")
	derivePaths()
	f := &fakeHost{fail: map[string]string{}, route: "10.66.0.0/24 proto kernel scope link src 10.66.0.1\n192.168.99.0/24 scope link\n"}
	a := NewServer(filepath.Join(dir, "a.sock"), filepath.Join(dir, "config.toml"), "test", "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	a.exec = f.exec
	a.localNets = func() []netip.Prefix { return []netip.Prefix{netip.MustParsePrefix("172.17.0.0/16")} }
	a.setForwarding = func(on bool) error { f.fwd = append(f.fwd, on); return nil }
	a.vpnHost = true
	a.tunnelWatchEvery = 0
	priv, pub, _ := vpn.GenerateKey()
	_ = os.MkdirAll(wgDir, 0o700)
	_ = os.WriteFile(wgKeyFile, []byte(priv+"\n"), 0o600)
	_ = saveJSON(wgStateFile, wgStored{Enabled: enabled, ListenPort: 51820, Address: "10.66.0.1/24", PublicKey: pub}, 0o600)
	return a, f
}

func desiredFor(t *testing.T) (vpn.Desired, string) {
	t.Helper()
	srv := netip.MustParsePrefix("10.66.0.1/24")
	_, p1, _ := vpn.GenerateKey()
	_, p2, _ := vpn.GenerateKey()
	psk, _ := vpn.GeneratePSK()
	peers := []vpn.Peer{
		{ID: "0192a000-0000-7000-8000-000000000001", Name: "technician", Kind: vpn.KindUser, Address: netip.MustParseAddr("10.66.0.2"), PublicKey: p1, Enabled: true},
		{ID: "0192a000-0000-7000-8000-000000000002", Name: "plant-1", Kind: vpn.KindSite, Address: netip.MustParseAddr("10.66.0.3"), PublicKey: p2, Enabled: true,
			Subnets: []netip.Prefix{netip.MustParsePrefix("192.168.10.0/24")}},
	}
	rules := []vpn.Rule{
		{ID: "r1", Sources: []vpn.Source{{Type: vpn.SrcKind, Kind: vpn.KindSite}}, DestType: vpn.DstServer, Protocol: "tcp", Ports: "1883", Enabled: true},
		{ID: "r2", Sources: []vpn.Source{{Type: vpn.SrcPeer, ID: peers[0].ID}}, DestType: vpn.DstSite, DestPeerID: peers[1].ID, Protocol: "tcp", Ports: "102", Enabled: true},
	}
	d := vpn.Compile(srv, peers, rules, time.Now())
	d.Peers[0].PresharedKey = psk
	return d, psk
}

func apply(a *Server, d vpn.Desired) *httptest.ResponseRecorder {
	b, _ := json.Marshal(d)
	w := httptest.NewRecorder()
	a.hApplyVPN(w, httptest.NewRequest("PUT", "/vpn", bytes.NewReader(b)))
	return w
}

func TestApplyVPNLive(t *testing.T) {
	a, f := vpnAgent(t, true)
	d, psk := desiredFor(t)
	w := apply(a, d)
	if w.Code != 200 {
		t.Fatalf("apply: %d %s", w.Code, w.Body)
	}
	order := []string{"nft -c -f", "wg show wg0", "nft -f", "wg-quick strip wg0", "wg syncconf wg0", "ip -4 -o route show dev wg0", "ip -4 route replace 192.168.10.0/24 dev wg0", "ip -4 route del 192.168.99.0/24 dev wg0"}
	last := -1
	for _, c := range order {
		i := f.ran(c)
		if i < 0 || i < last {
			t.Fatalf("%q not run in order: %v", c, f.cmds)
		}
		last = i
	}
	if len(f.fwd) != 1 || !f.fwd[0] {
		t.Fatalf("forwarding not switched on: %v", f.fwd)
	}
	nft, conf := readFile(wgNftFile), readFile(wgConfFile)
	if !strings.Contains(nft, `ip saddr 10.66.0.2/32 ip daddr 192.168.10.0/24 tcp dport 102 counter accept comment "rule r2"`) {
		t.Fatalf("nft:\n%s", nft)
	}
	if !strings.Contains(conf, "PresharedKey = "+psk) || !strings.Contains(conf, "AllowedIPs = 10.66.0.3/32, 192.168.10.0/24") || !strings.Contains(conf, "PreUp = nft -f "+wgNftFile) {
		t.Fatalf("wg0.conf:\n%s", conf)
	}
	// the preshared key is kept by the agent: the next state without it keeps the key
	d.Peers[0].PresharedKey = ""
	d.Rules = d.Rules[:1]
	d.Forward = false
	if w := apply(a, d); w.Code != 200 {
		t.Fatalf("second apply: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(readFile(wgConfFile), "PresharedKey = "+psk) {
		t.Fatal("preshared key lost")
	}
	if f.fwd[len(f.fwd)-1] {
		t.Fatal("forwarding still on without forward rules")
	}
	var st WireGuardState
	_ = json.Unmarshal(w.Body.Bytes(), &st)
	if !st.Supported || len(st.Peers) != 2 || st.Peers[1].Subnets[0] != "192.168.10.0/24" {
		t.Fatalf("state %+v", st)
	}
}

func TestApplyVPNRollback(t *testing.T) {
	a, f := vpnAgent(t, true)
	d, _ := desiredFor(t)
	if w := apply(a, d); w.Code != 200 {
		t.Fatalf("first apply: %d %s", w.Code, w.Body)
	}
	goodNft, goodConf, goodPeers := readFile(wgNftFile), readFile(wgConfFile), readFile(wgPeersFile)
	// remove the site, and make applying the peers fail
	d2 := d
	d2.Peers = d.Peers[:1]
	d2.Rules = nil
	d2.Forward = false
	f.cmds = nil
	f.fail["wg syncconf"] = "Unable to modify interface"
	w := apply(a, d2)
	if w.Code != 500 || !strings.Contains(w.Body.String(), "applying the peers failed") {
		t.Fatalf("expected failure: %d %s", w.Code, w.Body)
	}
	if readFile(wgNftFile) != goodNft || readFile(wgConfFile) != goodConf || readFile(wgPeersFile) != goodPeers {
		t.Fatal("files not restored after the failure")
	}
	// the old firewall was loaded again after the new one
	if n := strings.Count(strings.Join(f.cmds, "\n"), "nft -f "+wgNftFile); n != 2 {
		t.Fatalf("firewall not reloaded on rollback (%d): %v", n, f.cmds)
	}
	var vs vpnStored
	_ = loadJSON(wgVPNFile, &vs)
	if !strings.Contains(vs.Error, "applying the peers failed") || len(vs.Rules) != 2 {
		t.Fatalf("stored state after rollback: %+v", vs)
	}
	// a ruleset nft rejects is never loaded
	delete(f.fail, "wg syncconf")
	f.fail["nft -c"] = "Error: syntax error"
	f.cmds = nil
	if w := apply(a, d2); w.Code != 500 || !strings.Contains(w.Body.String(), "nftables rejected") {
		t.Fatalf("nft check: %d %s", w.Code, w.Body)
	}
	if f.ran("nft -f") >= 0 || f.ran("wg syncconf") >= 0 || readFile(wgNftFile) != goodNft {
		t.Fatalf("applied despite the failed check: %v", f.cmds)
	}
}

func TestApplyVPNValidation(t *testing.T) {
	a, f := vpnAgent(t, true)
	d, _ := desiredFor(t)
	d.Peers[1].Subnets = []string{"172.17.5.0/24"} // the host's Docker network
	w := apply(a, d)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "host network") {
		t.Fatalf("host network overlap: %d %s", w.Code, w.Body)
	}
	if len(f.cmds) != 0 {
		t.Fatalf("host touched on invalid input: %v", f.cmds)
	}
	// unknown fields are refused (the agent accepts only its own format)
	rec := httptest.NewRecorder()
	a.hApplyVPN(rec, httptest.NewRequest("PUT", "/vpn", strings.NewReader(`{"peers":[],"rules":[],"forward":false,"shell":"x"}`)))
	if rec.Code != 400 {
		t.Fatalf("unknown field: %d", rec.Code)
	}
}

func TestApplyVPNDisabledWritesFilesOnly(t *testing.T) {
	a, f := vpnAgent(t, false)
	d, _ := desiredFor(t)
	if w := apply(a, d); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for _, c := range f.cmds {
		if !strings.HasPrefix(c, "wg --version") && !strings.HasPrefix(c, "wg show wg0 dump") { // reading the state only
			t.Fatalf("host changed while the VPN is off: %v", f.cmds)
		}
	}
	if !strings.Contains(readFile(wgConfFile), "plant-1") || readFile(wgNftFile) == "" {
		t.Fatal("files not rendered")
	}
}

func TestVPNSimulation(t *testing.T) {
	dir := t.TempDir()
	a := NewServer(filepath.Join(dir, "a.sock"), filepath.Join(dir, "config.toml"), "test", "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	a.SetSimulation(dir)
	h := a.routes()
	put := func(path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("PUT", path, strings.NewReader(body)))
		return w
	}
	if w := put("/wireguard", `{"enabled":true,"listen_port":51820,"address":"10.66.0.1/24","endpoint":"vpn.example.com:51820","dns":""}`); w.Code != 200 {
		t.Fatalf("configure: %d %s", w.Code, w.Body)
	}
	d, _ := desiredFor(t)
	b, _ := json.Marshal(d)
	w := put("/vpn", string(b))
	if w.Code != 200 {
		t.Fatalf("apply: %d %s", w.Code, w.Body)
	}
	var st WireGuardState
	_ = json.Unmarshal(w.Body.Bytes(), &st)
	if !st.Simulated || !st.Running || !st.Firewall.Applied || st.Firewall.Rules != 2 || !strings.Contains(st.Firewall.Ruleset, "table inet ctrl32_vpn") {
		t.Fatalf("simulated state %+v", st.Firewall)
	}
	// host operations are refused (the VPN, the detector, updates and backups are simulated in dir)
	for _, p := range []string{"/caddy"} {
		if w := put(p, `{}`); w.Code != 501 {
			t.Errorf("%s in simulation: %d", p, w.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/restart", strings.NewReader(`{"unit":"app"}`)))
	if rec.Code != 501 {
		t.Errorf("restart in simulation: %d", rec.Code)
	}
	if !strings.Contains(readFile(filepath.Join(dir, "commands.log")), "wg --version") {
		t.Error("simulated commands not logged")
	}
}

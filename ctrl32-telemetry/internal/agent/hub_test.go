package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

// fakeHub stands in for the Windows userspace hub on any platform.
type fakeHub struct {
	running bool
	starts  int
	port    int
	applied []vpn.Desired
	fail    error
	live    []hubPeer
}

func (h *fakeHub) Start(key string, port int, addr netip.Prefix) error {
	if key == "" || !addr.IsValid() {
		return errors.New("bad start")
	}
	h.running, h.port = true, port
	h.starts++
	return nil
}
func (h *fakeHub) Apply(d vpn.Desired) error {
	if h.fail != nil {
		return h.fail
	}
	h.applied = append(h.applied, d)
	return nil
}
func (h *fakeHub) Stop()            { h.running = false }
func (h *fakeHub) Running() bool    { return h.running }
func (h *fakeHub) Peers() []hubPeer { return h.live }
func (h *fakeHub) Describe(addr netip.Prefix, d vpn.Desired) (string, error) {
	return "# userspace rules for " + addr.String() + " rules " + strings.Repeat("r", len(d.Rules)), nil
}

func hubAgent(t *testing.T, enabled bool) (*Server, *fakeHost, *fakeHub) {
	t.Helper()
	a, f := vpnAgent(t, enabled)
	a.vpnHost = false
	h := &fakeHub{}
	a.hub = h
	return a, f, h
}

func TestUserspaceApply(t *testing.T) {
	a, f, h := hubAgent(t, true)
	d, psk := desiredFor(t)
	w := apply(a, d)
	if w.Code != 200 {
		t.Fatalf("apply: %d %s", w.Code, w.Body)
	}
	if h.starts != 1 || len(h.applied) != 1 {
		t.Fatalf("hub not started/applied: %d %d", h.starts, len(h.applied))
	}
	if h.applied[0].Peers[0].PresharedKey != psk || len(h.applied[0].Rules) != 2 || !h.applied[0].Forward {
		t.Fatalf("applied state: %+v", h.applied[0])
	}
	for _, c := range []string{"nft", "wg ", "wg-quick", "ip ", "systemctl", "ufw"} {
		if f.ran(c) >= 0 {
			t.Fatalf("%q run in userspace mode: %v", c, f.cmds)
		}
	}
	if readFile(wgNftFile) != "" || readFile(wgConfFile) != "" {
		t.Fatal("kernel files written in userspace mode")
	}
	if !strings.Contains(readFile(wgRulesFile), "userspace rules for 10.66.0.1/24") {
		t.Fatalf("rules text: %q", readFile(wgRulesFile))
	}
	// the preshared key is carried over when the next state does not send it
	d.Peers[0].PresharedKey = ""
	if w := apply(a, d); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if h.starts != 1 || h.applied[1].Peers[0].PresharedKey != psk {
		t.Fatalf("second apply: starts %d psk %q", h.starts, h.applied[1].Peers[0].PresharedKey)
	}
	// state with live values
	h.live = []hubPeer{{PublicKey: d.Peers[0].PublicKey, Endpoint: "203.0.113.5:40000", Handshake: time.Now(), Rx: 10, Tx: 20}}
	st := a.wgState(t.Context())
	if st.Mode != ModeUserspace || !st.Running || !st.Installed || !st.Firewall.Applied || st.Firewall.Rules != 2 {
		t.Fatalf("state: %+v", st)
	}
	online := false
	for _, p := range st.Peers {
		if p.PublicKey == d.Peers[0].PublicKey && p.Online && p.RxBytes == 10 {
			online = true
		}
	}
	if !online {
		t.Fatalf("live values missing: %+v", st.Peers)
	}
	// a failure keeps the previous state and records the error
	h.fail = errors.New("ipc failed")
	if w := apply(a, d); w.Code != 500 {
		t.Fatalf("failure: %d", w.Code)
	}
	if st := a.wgState(t.Context()); st.Firewall.Error == "" || st.Firewall.Applied {
		t.Fatalf("error not reported: %+v", st.Firewall)
	}
}

func TestUserspaceDisabledApplyOnlyStores(t *testing.T) {
	a, _, h := hubAgent(t, false)
	d, _ := desiredFor(t)
	if w := apply(a, d); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if h.starts != 0 || len(h.applied) != 0 {
		t.Fatal("hub started while the VPN is disabled")
	}
	if len(readFile(wgPeersFile)) == 0 || readFile(wgRulesFile) == "" {
		t.Fatal("state not stored")
	}
}

func TestUserspaceSetWireGuard(t *testing.T) {
	a, f, h := hubAgent(t, false)
	put := func(in WireGuardRequest) *httptest.ResponseRecorder {
		b, _ := json.Marshal(in)
		w := httptest.NewRecorder()
		a.hSetWireGuard(w, httptest.NewRequest("PUT", "/wireguard", bytes.NewReader(b)))
		return w
	}
	w := put(WireGuardRequest{Enabled: true, ListenPort: 51900, Address: "10.66.0.1/24", Endpoint: "vpn.example.com:51900"})
	if w.Code != 200 {
		t.Fatalf("enable: %d %s", w.Code, w.Body)
	}
	if !h.running || h.port != 51900 {
		t.Fatalf("hub not running on the port: %+v", h)
	}
	if f.ran("netsh advfirewall firewall add rule name=ctrl32 WireGuard dir=in action=allow protocol=UDP localport=51900") < 0 {
		t.Fatalf("firewall rule not added: %v", f.cmds)
	}
	for _, c := range []string{"apt-get", "systemctl", "ufw", "wg"} {
		if f.ran(c) >= 0 {
			t.Fatalf("%q run in userspace mode: %v", c, f.cmds)
		}
	}
	var st WireGuardState
	_ = json.Unmarshal(w.Body.Bytes(), &st)
	if st.Mode != ModeUserspace || !st.Running || st.PublicKey == "" {
		t.Fatalf("state: %+v", st)
	}
	f.cmds = nil
	if w := put(WireGuardRequest{Enabled: false, ListenPort: 51900, Address: "10.66.0.1/24"}); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if h.running || f.ran("netsh advfirewall firewall delete rule name=ctrl32 WireGuard") < 0 || f.ran("netsh advfirewall firewall add") >= 0 {
		t.Fatalf("disable: running %v cmds %v", h.running, f.cmds)
	}
}

func TestUserspaceStartFromFiles(t *testing.T) {
	a, _, h := hubAgent(t, true)
	a.startHubFromFiles()
	if !h.running || len(h.applied) != 1 {
		t.Fatalf("hub not started from the stored state: %+v", h)
	}
	b, _, h2 := hubAgent(t, false)
	b.startHubFromFiles()
	if h2.running {
		t.Fatal("hub started while disabled")
	}
}

func TestVPNMode(t *testing.T) {
	a, _ := vpnAgent(t, false)
	if a.vpnMode() != ModeKernel {
		t.Fatal(a.vpnMode())
	}
	a.vpnHost, a.hub = false, nil
	if a.vpnMode() != "" || a.wgSupported() {
		t.Fatal("no mode expected")
	}
	a.hub = &fakeHub{}
	if a.vpnMode() != ModeUserspace {
		t.Fatal(a.vpnMode())
	}
	a.simulate = true
	if a.vpnMode() != ModeSimulation {
		t.Fatal(a.vpnMode())
	}
}

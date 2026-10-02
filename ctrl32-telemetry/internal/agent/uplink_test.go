package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

func uplinkToken(t *testing.T, vpnNet, addr, hub string, subnets ...string) (vpn.UplinkToken, string) {
	t.Helper()
	priv, _, _ := vpn.GenerateKey()
	_, hubPub, _ := vpn.GenerateKey()
	psk, _ := vpn.GeneratePSK()
	key, _, _ := vpn.GenerateTunnelKey()
	tok := vpn.UplinkToken{Name: "customer-a", Endpoint: "203.0.113.5:51820", HubPublicKey: hubPub, PresharedKey: psk, PrivateKey: priv,
		Address: addr, VPNNet: vpnNet, HubAddress: hub, Subnets: subnets, TunnelURL: "http://127.0.0.1:9" + vpn.UplinkTunnelPath, TunnelKey: key}
	return tok, vpn.EncodeUplinkToken(tok)
}

// uplinkAgent: a Linux agent with the fake host; eth0 leads to 192.168.1.0/24 with forwarding off.
func uplinkAgent(t *testing.T) (*Server, *fakeHost, map[string]bool) {
	t.Helper()
	a, f := vpnAgent(t, true) // own VPN 10.66.0.1/24 enabled
	f.outputs = map[string]string{"ip -4 -o route get 192.168.1.1": "192.168.1.1 dev eth0 src 192.168.1.10 uid 0"}
	fwd := map[string]bool{"all": false, "eth0": false}
	a.ifForwarding = func(i string) (bool, error) { return fwd[i], nil }
	a.setIfForwarding = func(i string, on bool) error { fwd[i] = on; return nil }
	return a, f, fwd
}

func call(a *Server, method, path string, body any) *httptest.ResponseRecorder {
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	switch {
	case method == "PUT" && path == "/uplink":
		a.hUplinkConnect(w, r)
	case path == "/uplink/rules":
		a.hUplinkRules(w, r)
	case path == "/uplink/transport":
		a.hUplinkTransport(w, r)
	case method == "DELETE":
		a.hUplinkDisconnect(w, r)
	}
	return w
}

var uplinkRules = []vpn.FwRule{
	{ID: "r1", Chain: vpn.ChainInput, Src: []string{"10.99.0.0/16"}, Dst: []string{"10.99.0.7/32"}, Proto: "tcp", Ports: []string{"443"}},
	{ID: "r2", Chain: vpn.ChainForward, Src: []string{"10.99.0.5/32"}, Dst: []string{"192.168.1.20/32"}, Proto: "tcp", Ports: []string{"502"}},
}

func TestUplinkConnectKernel(t *testing.T) {
	a, f, fwd := uplinkAgent(t)
	tok, s := uplinkToken(t, "10.99.0.0/16", "10.99.0.7", "10.99.0.1", "192.168.1.0/24")
	w := call(a, "PUT", "/uplink", UplinkRequest{Token: s, Transport: "udp", Rules: uplinkRules})
	if w.Code != 200 {
		t.Fatalf("connect: %d %s", w.Code, w.Body)
	}
	order := []string{"ip -4 -o route get 192.168.1.1", "nft -c -f", "systemctl enable wg-quick@wg1", "systemctl restart wg-quick@wg1"}
	last := -1
	for _, c := range order {
		i := f.ran(c)
		if i < 0 || i < last {
			t.Fatalf("%q not run in order: %v", c, f.cmds)
		}
		last = i
	}
	if !fwd["wg1"] || !fwd["eth0"] {
		t.Fatalf("forwarding: %v", fwd)
	}
	conf, nft := readFile(uplinkConfFile()), readFile(uplinkNftFile)
	for _, want := range []string{vpn.UplinkConfHeader, "Endpoint = 203.0.113.5:51820", "PostUp = sysctl -q -w net.ipv4.conf.eth0.forwarding=1", "PrivateKey = " + tok.PrivateKey} {
		if !strings.Contains(conf, want) {
			t.Fatalf("wg1.conf misses %q:\n%s", want, conf)
		}
	}
	if !strings.Contains(nft, `iifname "eth0" oifname != "wg1" counter drop`) || !strings.Contains(nft, `comment "rule r2"`) {
		t.Fatalf("table:\n%s", nft)
	}
	if fi, err := os.Stat(uplinkFile); err != nil || fi.Mode().Perm()&0o077 != 0 && !isWindows() {
		t.Fatalf("uplink.json: %v %v", fi, err)
	}
	var st WireGuardState
	_ = json.Unmarshal(w.Body.Bytes(), &st)
	u := st.Uplink
	if u == nil || !u.Configured || u.Mode != ModeKernel || u.Address != "10.99.0.7" || u.Transport != "udp" || u.Firewall.Rules != 2 || !u.Firewall.Forward || !u.HTTPSAvailable {
		t.Fatalf("state %+v", u)
	}
	if strings.Contains(w.Body.String(), tok.PrivateKey) || strings.Contains(w.Body.String(), tok.PresharedKey) || strings.Contains(w.Body.String(), tok.TunnelKey) {
		t.Fatal("secrets in the state")
	}

	// new rules without a LAN rule: no restart, the table is reloaded, forwarding goes back off where the agent turned it on
	f.cmds = nil
	f.outputs["wg show wg1"] = "interface: wg1"
	w = call(a, "PUT", "/uplink/rules", UplinkRulesRequest{Rules: uplinkRules[:1]})
	if w.Code != 200 {
		t.Fatalf("rules: %d %s", w.Code, w.Body)
	}
	if f.ran("systemctl restart") >= 0 || f.ran("nft -f "+uplinkNftFile) < 0 || f.ran("wg set wg1 peer "+tok.HubPublicKey+" endpoint 203.0.113.5:51820") < 0 {
		t.Fatalf("rules apply: %v", f.cmds)
	}
	if fwd["wg1"] || fwd["eth0"] {
		t.Fatalf("forwarding stays on: %v", fwd)
	}
	if strings.Contains(readFile(uplinkNftFile), "eth0") {
		t.Fatal("LAN guard without a LAN rule")
	}
	// rules outside the uplink's ranges are refused by the agent itself
	bad := []vpn.FwRule{{ID: "x", Chain: vpn.ChainForward, Src: []string{"10.99.0.0/16"}, Dst: []string{"192.168.0.0/16"}, Proto: "any"}}
	if w := call(a, "PUT", "/uplink/rules", UplinkRulesRequest{Rules: bad}); w.Code != 500 || !strings.Contains(w.Body.String(), "offered networks") {
		t.Fatalf("bad rule: %d %s", w.Code, w.Body)
	}

	// switching to the HTTPS transport points wg1 at the local end of the tunnel
	f.cmds = nil
	w = call(a, "PUT", "/uplink/transport", UplinkTransportRequest{Transport: "https"})
	if w.Code != 200 || a.tunnel == nil {
		t.Fatalf("transport: %d %s", w.Code, w.Body)
	}
	if f.ran("wg set wg1 peer "+tok.HubPublicKey+" endpoint 127.0.0.1:") < 0 || !strings.Contains(readFile(uplinkConfFile()), "Endpoint = 127.0.0.1:") {
		t.Fatalf("endpoint: %v", f.cmds)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &st)
	if st.Uplink.Transport != "https" || st.Uplink.Tunnel == nil || st.Uplink.TunnelHost != "127.0.0.1:9" {
		t.Fatalf("https state %+v", st.Uplink)
	}

	// the own VPN may not take the hub's range any more
	b, _ := json.Marshal(WireGuardRequest{Enabled: true, ListenPort: 51820, Address: "10.99.200.1/24"})
	rec := httptest.NewRecorder()
	a.hSetWireGuard(rec, httptest.NewRequest("PUT", "/wireguard", bytes.NewReader(b)))
	if rec.Code != 409 || !strings.Contains(rec.Body.String(), "uplink") {
		t.Fatalf("own range over the hub's: %d %s", rec.Code, rec.Body)
	}
	d, _ := desiredFor(t)
	d.Peers[1].Subnets, d.Rules = []string{"10.99.5.0/24"}, nil
	if w := apply(a, d); w.Code != 400 || !strings.Contains(w.Body.String(), "uplink") {
		t.Fatalf("site subnet in the hub's range: %d %s", w.Code, w.Body)
	}

	// disconnect removes everything, keys included
	f.cmds = nil
	w = call(a, "DELETE", "/uplink", nil)
	if w.Code != 200 || a.tunnel != nil {
		t.Fatalf("disconnect: %d %s", w.Code, w.Body)
	}
	for _, c := range []string{"systemctl disable --now wg-quick@wg1", "nft delete table inet ctrl32_uplink"} {
		if f.ran(c) < 0 {
			t.Fatalf("%q not run: %v", c, f.cmds)
		}
	}
	for _, p := range []string{uplinkFile, uplinkConfFile(), uplinkNftFile} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("%s left behind", p)
		}
	}
	_ = json.Unmarshal(w.Body.Bytes(), &st)
	if st.Uplink == nil || st.Uplink.Configured {
		t.Fatalf("state after disconnect %+v", st.Uplink)
	}
}

func TestUplinkConnectRefusals(t *testing.T) {
	a, f, _ := uplinkAgent(t)
	// the hub's range overlaps the own VPN 10.66.0.1/24, a host network 172.17.0.0/16
	for _, c := range []struct{ net, addr, hub string }{{"10.66.0.0/16", "10.66.9.7", "10.66.9.1"}, {"172.17.0.0/16", "172.17.9.7", "172.17.9.1"}} {
		_, s := uplinkToken(t, c.net, c.addr, c.hub)
		if w := call(a, "PUT", "/uplink", UplinkRequest{Token: s}); w.Code != 409 {
			t.Fatalf("%s accepted: %d %s", c.net, w.Code, w.Body)
		}
	}
	if w := call(a, "PUT", "/uplink", UplinkRequest{Token: "nonsense"}); w.Code != 400 {
		t.Fatalf("bad token: %d", w.Code)
	}
	tok, s := uplinkToken(t, "10.99.0.0/16", "10.99.0.7", "10.99.0.1")
	if w := call(a, "PUT", "/uplink", UplinkRequest{Token: s, Transport: "carrier-pigeon"}); w.Code != 400 {
		t.Fatalf("bad transport: %d", w.Code)
	}
	tok.TunnelURL, tok.TunnelKey = "", ""
	if w := call(a, "PUT", "/uplink", UplinkRequest{Token: vpn.EncodeUplinkToken(tok), Transport: "https"}); w.Code != 400 {
		t.Fatalf("https without a tunnel URL: %d", w.Code)
	}
	// a wg1.conf that is not ours is never overwritten
	_ = os.MkdirAll(wgDir, 0o700)
	_ = os.WriteFile(uplinkConfFile(), []byte("[Interface]\n# somebody else's\n"), 0o600)
	if w := call(a, "PUT", "/uplink", UplinkRequest{Token: s}); w.Code != 500 || !strings.Contains(w.Body.String(), "not written by this server") {
		t.Fatalf("foreign wg1.conf: %d %s", w.Code, w.Body)
	}
	if readFile(uplinkConfFile()) != "[Interface]\n# somebody else's\n" {
		t.Fatal("foreign wg1.conf changed")
	}
	_ = os.Remove(uplinkConfFile())
	// a failed start leaves nothing behind
	f.cmds = nil
	f.fail["systemctl restart wg-quick@wg1"] = "Job failed"
	if w := call(a, "PUT", "/uplink", UplinkRequest{Token: s, Rules: uplinkRules[:1]}); w.Code != 500 || !strings.Contains(w.Body.String(), "could not be started") {
		t.Fatalf("failed start: %d %s", w.Code, w.Body)
	}
	if f.ran("systemctl disable --now wg-quick@wg1") < 0 {
		t.Fatalf("not rolled back: %v", f.cmds)
	}
	if _, ok := a.uplinkLoad(); ok || readFile(uplinkConfFile()) != "" {
		t.Fatal("state left behind after a failed connect")
	}
	// rules and transport need a connected uplink
	if w := call(a, "PUT", "/uplink/rules", UplinkRulesRequest{}); w.Code != 409 {
		t.Fatalf("rules without uplink: %d", w.Code)
	}
}

func TestUplinkRouterHostNoGuard(t *testing.T) {
	a, _, fwd := uplinkAgent(t)
	fwd["all"], fwd["eth0"] = true, true // e.g. Docker turned on forwarding for the whole host
	_, s := uplinkToken(t, "10.99.0.0/16", "10.99.0.7", "10.99.0.1", "192.168.1.0/24")
	if w := call(a, "PUT", "/uplink", UplinkRequest{Token: s, Rules: uplinkRules}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if strings.Contains(readFile(uplinkNftFile), `iifname "eth0"`) || strings.Contains(readFile(uplinkConfFile()), "eth0") {
		t.Fatal("guard or sysctl on a host that forwards anyway")
	}
	u, _ := a.uplinkLoad()
	if len(u.LANForward) != 0 {
		t.Fatalf("lan forward %v", u.LANForward)
	}
	call(a, "DELETE", "/uplink", nil)
	if !fwd["eth0"] {
		t.Fatal("the host's own forwarding switched off")
	}
}

// fakeUplink stands in for the Windows userspace uplink.
type fakeUplink struct {
	rules    [][]vpn.FwRule
	endpoint string
	stopped  bool
}

func (u *fakeUplink) SetRules(r []vpn.FwRule) error { u.rules = append(u.rules, r); return nil }
func (u *fakeUplink) SetEndpoint(_, ep string) error {
	u.endpoint = ep
	return nil
}
func (u *fakeUplink) Stop()            { u.stopped = true }
func (u *fakeUplink) Describe() string { return "# userspace uplink rules" }
func (u *fakeUplink) Peers() []hubPeer {
	return []hubPeer{{Endpoint: u.endpoint, Handshake: time.Now(), Rx: 10, Tx: 20}}
}

func TestUplinkUserspace(t *testing.T) {
	a, f, _ := uplinkAgent(t)
	a.vpnHost, a.hub = false, &fakeHub{}
	var dev *fakeUplink
	var started []string
	a.startUplink = func(priv string, addr netip.Prefix, hubPub, psk, ep string, lan []netip.Prefix, _ *slog.Logger) (uplinkDevice, error) {
		if ep == "bad:1" {
			return nil, errors.New("no")
		}
		started = append(started, addr.String()+" "+ep+" "+lan[0].String())
		dev = &fakeUplink{endpoint: ep}
		return dev, nil
	}
	_, s := uplinkToken(t, "10.99.0.0/16", "10.99.0.7", "10.99.0.1", "192.168.1.0/24")
	w := call(a, "PUT", "/uplink", UplinkRequest{Token: s, Rules: uplinkRules})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if len(started) != 1 || started[0] != "10.99.0.7/16 203.0.113.5:51820 192.168.1.0/24" || len(dev.rules) != 1 || len(dev.rules[0]) != 2 {
		t.Fatalf("started %v rules %v", started, dev.rules)
	}
	for _, c := range []string{"nft", "wg", "systemctl", "ip ", "ufw"} {
		if f.ran(c) >= 0 {
			t.Fatalf("%q run in userspace mode: %v", c, f.cmds)
		}
	}
	var st WireGuardState
	_ = json.Unmarshal(w.Body.Bytes(), &st)
	if st.Uplink.Mode != ModeUserspace || !st.Uplink.Running || !st.Uplink.Online || st.Uplink.RxBytes != 10 || st.Uplink.Firewall.Ruleset != "# userspace uplink rules" {
		t.Fatalf("state %+v", st.Uplink)
	}
	if w := call(a, "PUT", "/uplink/transport", UplinkTransportRequest{Transport: "https"}); w.Code != 200 || !strings.HasPrefix(dev.endpoint, "127.0.0.1:") {
		t.Fatalf("transport: %d %s %q", w.Code, w.Body, dev.endpoint)
	}
	if w := call(a, "PUT", "/uplink/rules", UplinkRulesRequest{Rules: uplinkRules[:1]}); w.Code != 200 || len(dev.rules) != 3 {
		t.Fatalf("rules: %d %v", w.Code, len(dev.rules))
	}
	// the agent restarts: the device and the tunnel come back from the files
	a.upDev.Stop()
	a.upDev = nil
	a.stopTunnel()
	a.startUplinkFromFiles()
	if len(started) != 2 || !strings.Contains(started[1], " 127.0.0.1:") || a.tunnel == nil {
		t.Fatalf("not restarted from files: %v", started)
	}
	call(a, "DELETE", "/uplink", nil)
	if !dev.stopped || a.upDev != nil || a.tunnel != nil {
		t.Fatal("not stopped")
	}
}

func TestUplinkSimulation(t *testing.T) {
	dir := t.TempDir()
	a := NewServer(dir+"/a.sock", dir+"/config.toml", "test", "", slog.New(slog.NewTextHandler(os_discard{}, nil)))
	a.SetSimulation(dir)
	_, s := uplinkToken(t, "10.99.0.0/16", "10.99.0.7", "10.99.0.1", "192.168.1.0/24")
	w := call(a, "PUT", "/uplink", UplinkRequest{Token: s, Rules: uplinkRules})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var st WireGuardState
	_ = json.Unmarshal(w.Body.Bytes(), &st)
	if st.Uplink == nil || st.Uplink.Mode != ModeSimulation || !st.Uplink.Running || !strings.Contains(st.Uplink.Firewall.Ruleset, "table inet ctrl32_uplink") {
		t.Fatalf("%+v", st.Uplink)
	}
	if !strings.Contains(readFile(uplinkConfFile()), "AllowedIPs = 10.99.0.0/16") {
		t.Fatal(readFile(uplinkConfFile()))
	}
	log := readFile(dir + "/commands.log")
	if strings.Contains(log, "systemctl") || strings.Contains(log, "nft -f") {
		t.Fatalf("the simulation ran programs:\n%s", log)
	}
	call(a, "DELETE", "/uplink", nil)
}

type os_discard struct{}

func (os_discard) Write(p []byte) (int, error) { return len(p), nil }

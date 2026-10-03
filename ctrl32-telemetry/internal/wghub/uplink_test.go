package wghub

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/uptunnel"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

// technician is a WireGuard client of the hub that routes the hub's range and the customer's LAN into its tunnel.
func technician(t *testing.T, addr, hubPub string, hubPort int, allowed ...string) *client {
	t.Helper()
	priv, pub, _ := vpn.GenerateKey()
	psk, _ := vpn.GeneratePSK()
	tn, n, err := netstack.CreateNetTUN([]netip.Addr{netip.MustParseAddr(addr)}, nil, MTU)
	if err != nil {
		t.Fatal(err)
	}
	dev := device.NewDevice(tn, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))
	ph, _ := b64ToHex(priv)
	hh, _ := b64ToHex(hubPub)
	kh, _ := b64ToHex(psk)
	cfg := "private_key=" + ph + "\npublic_key=" + hh + "\npreshared_key=" + kh + "\nendpoint=127.0.0.1:" + strconv.Itoa(hubPort) + "\npersistent_keepalive_interval=1\n"
	for _, a := range allowed {
		cfg += "allowed_ip=" + a + "\n"
	}
	if err := dev.IpcSet(cfg); err != nil {
		t.Fatal(err)
	}
	if err := dev.Up(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dev.Close)
	return &client{dev: dev, net: n, pub: pub, psk: psk, addr: addr}
}

type uplinkLab struct {
	hub, cust   *Hub
	hubNet      netip.Prefix
	tech, tech2 *client
	custPub     string
	custPSK     string
	hubDesired  vpn.Desired
	web, plc    uint16 // services: the customer server's web (loopback) and a PLC in its LAN
	lanDials    chan netip.AddrPort
	hubPort     int
	custPriv    string
	techAllowed []string
	custAddress netip.Prefix
	custLAN     []netip.Prefix
}

func listenEcho(t *testing.T) (net.Listener, uint16) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	echoServer(t, l)
	return l, uint16(l.Addr().(*net.TCPAddr).Port)
}

// newUplinkLab: a hub (10.99.0.1/16), technicians 10.99.0.5 and 10.99.0.6, and the uplink peer 10.99.0.7 offering
// 192.168.50.0/24. The customer's web service and a PLC at 192.168.50.10:502 are echo servers on loopback.
func newUplinkLab(t *testing.T) *uplinkLab {
	t.Helper()
	L := &uplinkLab{hubNet: netip.MustParsePrefix("10.99.0.1/16"), lanDials: make(chan netip.AddrPort, 16)}
	_, L.web = listenEcho(t)
	_, L.plc = listenEcho(t)
	hubPriv, _, _ := vpn.GenerateKey()
	h, err := Start(Config{PrivateKey: hubPriv, ListenPort: 0, Address: L.hubNet})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Stop)
	L.hub, L.hubPort = h, h.ListenPort()
	L.techAllowed = []string{"10.99.0.0/16", "192.168.50.0/24"}
	L.tech = technician(t, "10.99.0.5", h.PublicKey(), L.hubPort, L.techAllowed...)
	L.tech2 = technician(t, "10.99.0.6", h.PublicKey(), L.hubPort, L.techAllowed...)
	L.custPriv, L.custPub, _ = vpn.GenerateKey()
	L.custPSK, _ = vpn.GeneratePSK()
	L.custAddress = netip.MustParsePrefix("10.99.0.7/16")
	L.custLAN = []netip.Prefix{netip.MustParsePrefix("192.168.50.0/24")}
	L.hubDesired = vpn.Desired{Peers: []vpn.DesiredPeer{
		{ID: "t1", Name: "tech", PublicKey: L.tech.pub, PresharedKey: L.tech.psk, Address: "10.99.0.5", Enabled: true},
		{ID: "t2", Name: "tech2", PublicKey: L.tech2.pub, PresharedKey: L.tech2.psk, Address: "10.99.0.6", Enabled: true},
		{ID: "up", Name: "customer", PublicKey: L.custPub, PresharedKey: L.custPSK, Address: "10.99.0.7", Subnets: []string{"192.168.50.0/24"}, Enabled: true},
	}, Rules: []vpn.FwRule{
		// hub side: technicians may reach the customer server (any port) and the customer's PLC network on 502
		{ID: "h1", Chain: vpn.ChainForward, Src: []string{"10.99.0.5/32", "10.99.0.6/32"}, Dst: []string{"10.99.0.7/32"}, Proto: "any"},
		{ID: "h2", Chain: vpn.ChainForward, Src: []string{"10.99.0.5/32", "10.99.0.6/32"}, Dst: []string{"192.168.50.0/24"}, Proto: "tcp", Ports: []string{strconv.Itoa(int(L.plc)), "502"}},
	}, Forward: true}
	if err := h.Apply(L.hubDesired); err != nil {
		t.Fatal(err)
	}
	return L
}

func (L *uplinkLab) startCustomer(t *testing.T, endpoint string) {
	t.Helper()
	plc := L.plc
	c, err := StartUplink(UplinkConfig{PrivateKey: L.custPriv, Address: L.custAddress, HubPublicKey: L.hub.PublicKey(), PresharedKey: L.custPSK,
		Endpoint: endpoint, LAN: L.custLAN,
		Dial: func(ctx context.Context, port uint16) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", "127.0.0.1:"+strconv.Itoa(int(port)))
		},
		DialLAN: func(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
			L.lanDials <- dst
			// the "LAN": only 192.168.50.10 exists, its port 502 is the PLC echo server
			if dst.Addr() != netip.MustParseAddr("192.168.50.10") || dst.Port() != 502 {
				return nil, &net.OpError{Op: "dial", Err: io.EOF}
			}
			var d net.Dialer
			return d.DialContext(ctx, "tcp", "127.0.0.1:"+strconv.Itoa(int(plc)))
		}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Stop)
	L.cust = c
}

func dialFrom(n *netstack.Net, dst string) func(context.Context) (net.Conn, error) {
	return func(ctx context.Context) (net.Conn, error) {
		return n.DialContextTCPAddrPort(ctx, netip.MustParseAddrPort(dst))
	}
}

func reach(t *testing.T, n *netstack.Net, dst string, d time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	got, err := roundTrip(ctx, dialFrom(n, dst))
	if err == nil && got != "echo hello\n" {
		t.Fatalf("%s answered %q", dst, got)
	}
	return err
}

func waitHandshake(t *testing.T, h *Hub) {
	t.Helper()
	for i := 0; i < 100; i++ {
		for _, p := range h.Peers() {
			if !p.LastHandshake.IsZero() {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("the uplink did not complete a handshake with the hub")
}

func TestUplinkEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	L := newUplinkLab(t)
	L.startCustomer(t, "127.0.0.1:"+strconv.Itoa(L.hubPort))
	waitHandshake(t, L.cust)
	web := "10.99.0.7:" + strconv.Itoa(int(L.web))

	// default deny on the customer side: the hub allows everything towards the uplink, the customer nothing yet
	if err := reach(t, L.tech.net, web, 2*time.Second); err == nil {
		t.Fatal("customer server reachable without an uplink rule")
	}
	if err := ping(L.tech.net, "10.99.0.7", 30*time.Second); err != nil {
		t.Fatalf("ping of the customer server through the hub: %v", err)
	}
	rules := []vpn.FwRule{
		{ID: "c1", Chain: vpn.ChainInput, Src: []string{"10.99.0.0/16"}, Dst: []string{"10.99.0.7/32"}, Proto: "tcp", Ports: []string{strconv.Itoa(int(L.web))}},
		{ID: "c2", Chain: vpn.ChainForward, Src: []string{"10.99.0.5/32"}, Dst: []string{"192.168.50.10/32"}, Proto: "tcp", Ports: []string{"502"}},
	}
	if err := L.cust.SetUplinkRules(rules); err != nil {
		t.Fatal(err)
	}
	if err := reach(t, L.tech.net, web, 10*time.Second); err != nil {
		t.Fatalf("customer web through hub and uplink: %v", err)
	}
	if err := reach(t, L.tech.net, "10.99.0.7:"+strconv.Itoa(int(L.plc)), 2*time.Second); err == nil {
		t.Fatal("a port without a customer rule reached")
	}
	if err := reach(t, L.tech.net, "192.168.50.10:502", 10*time.Second); err != nil {
		t.Fatalf("PLC in the customer's LAN: %v", err)
	}
	select {
	case d := <-L.lanDials:
		if d.String() != "192.168.50.10:502" {
			t.Fatalf("relayed to %s", d)
		}
	default:
		t.Fatal("no connection from the customer host to the LAN")
	}
	// another LAN host: the hub allows the subnet, the customer only .10
	if err := reach(t, L.tech.net, "192.168.50.11:502", 2*time.Second); err == nil {
		t.Fatal("LAN host without a customer rule reached")
	}
	// the second technician: allowed by the hub, not by the customer's rule (source 10.99.0.5 only)
	if err := reach(t, L.tech2.net, "192.168.50.10:502", 2*time.Second); err == nil {
		t.Fatal("technician without a customer rule reached the PLC")
	}
	if err := reach(t, L.tech2.net, web, 10*time.Second); err != nil {
		t.Fatalf("the whole hub range may reach the web: %v", err)
	}
	if len(L.lanDials) != 0 {
		t.Fatalf("denied connections reached the host's dialer: %d", len(L.lanDials))
	}
	// the hub's side too: without the hub rule the customer's rule alone does not help
	hubOnly := L.hubDesired
	hubOnly.Rules = hubOnly.Rules[:1]
	if err := L.hub.Apply(hubOnly); err != nil {
		t.Fatal(err)
	}
	if err := reach(t, L.tech.net, "192.168.50.10:502", 2*time.Second); err == nil {
		t.Fatal("PLC reached without the hub's rule")
	}
	if err := L.hub.Apply(L.hubDesired); err != nil {
		t.Fatal(err)
	}
	// taking the customer's rule away ends access at once
	if err := L.cust.SetUplinkRules(rules[:1]); err != nil {
		t.Fatal(err)
	}
	if err := reach(t, L.tech.net, "192.168.50.10:502", 2*time.Second); err == nil {
		t.Fatal("PLC reachable after the rule was removed")
	}
	st := L.cust.Peers()
	if len(st) != 1 || st[0].RxBytes == 0 || st[0].TxBytes == 0 || st[0].LastHandshake.IsZero() {
		t.Fatalf("uplink statistics %+v", st)
	}
	if !strings.Contains(L.cust.Describe(), "rule c1") || !strings.Contains(L.cust.Describe(), "192.168.50.0/24") {
		t.Fatal(L.cust.Describe())
	}
}

// TestUplinkHTTPS: the same uplink with the HTTPS transport (WireGuard datagrams in a WebSocket through the hub's web
// server), including a reconnect of the tunnel.
func TestUplinkHTTPS(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	L := newUplinkLab(t)
	key, hash, _ := vpn.GenerateTunnelKey()
	var refused int
	h := &uptunnel.Handler{Hash: vpn.HashTunnelKey, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Lookup: func(_ context.Context, kh string) (string, string, bool) {
			if kh == hash {
				return "up", "customer", true
			}
			refused++
			return "", "", false
		},
		Target: func(context.Context) (netip.AddrPort, error) {
			return netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(L.hubPort)), nil
		}}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	// a wrong key is refused and keeps retrying without reaching the hub
	bad, err := uptunnel.NewClient(srv.URL+vpn.UplinkTunnelPath, strings.Repeat("x", 43), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	if st := bad.Status(); st.Connected || !strings.Contains(st.Error, "401") {
		t.Fatalf("wrong key: %+v", st)
	}
	bad.Close()

	tc, err := uptunnel.NewClient(srv.URL+vpn.UplinkTunnelPath, key, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tc.Close)
	L.startCustomer(t, tc.Endpoint())
	waitHandshake(t, L.cust)
	if !tc.Status().Connected {
		t.Fatal("tunnel not connected")
	}
	if err := L.cust.SetUplinkRules([]vpn.FwRule{{ID: "c1", Chain: vpn.ChainInput, Src: []string{"10.99.0.0/16"}, Dst: []string{"10.99.0.7/32"}, Proto: "tcp", Ports: []string{strconv.Itoa(int(L.web))}}}); err != nil {
		t.Fatal(err)
	}
	web := "10.99.0.7:" + strconv.Itoa(int(L.web))
	if err := reach(t, L.tech.net, web, 10*time.Second); err != nil {
		t.Fatalf("customer web over the HTTPS transport: %v", err)
	}
	// the hub's peer sees the relay on loopback
	for _, p := range L.hub.Peers() {
		if p.PublicKey == L.custPub && !strings.HasPrefix(p.Endpoint, "127.0.0.1:") {
			t.Fatalf("uplink endpoint on the hub %q", p.Endpoint)
		}
	}
	// the hub drops the tunnel (e.g. a restart of the web service): the client reconnects and traffic flows again
	h.Close("up")
	time.Sleep(300 * time.Millisecond)
	ok := false
	for i := 0; i < 30 && !ok; i++ {
		ok = reach(t, L.tech.net, web, 2*time.Second) == nil
	}
	if !ok {
		t.Fatal("no traffic after the tunnel reconnected")
	}
	if refused == 0 {
		t.Fatal("the wrong key was not looked up")
	}
}

func TestUplinkFilterAndValidation(t *testing.T) {
	srv := netip.MustParsePrefix("10.99.0.7/16")
	lan := []netip.Prefix{netip.MustParsePrefix("192.168.50.0/24")}
	f := NewFilter(srv)
	if err := f.SetUplinkRules(srv, lan, []vpn.FwRule{
		{ID: "a", Chain: vpn.ChainForward, Src: []string{"10.99.0.5/32"}, Dst: []string{"192.168.50.10/32"}, Proto: "tcp", Ports: []string{"502"}},
		{ID: "b", Chain: vpn.ChainForward, Src: []string{"10.99.0.0/16"}, Dst: []string{"192.168.50.0/24"}, Proto: "udp"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		pkt  []byte
		want Verdict
	}{
		{pkt(protoTCP, "10.99.0.5", "192.168.50.10", 40000, 502, 0), Input},
		{pkt(protoTCP, "10.99.0.5", "192.168.50.10", 40000, 503, 0), Drop},
		{pkt(protoTCP, "10.99.0.6", "192.168.50.10", 40000, 502, 0), Drop},
		{pkt(protoUDP, "10.99.0.5", "192.168.50.10", 40000, 161, 0), Drop}, // UDP to the LAN is not relayed on Windows
		{pkt(protoTCP, "10.98.0.5", "192.168.50.10", 40000, 502, 0), Drop}, // not from the hub's range
		{pkt(protoTCP, "10.99.0.5", "10.99.0.8", 40000, 502, 0), Drop},     // another hub peer
	} {
		if got := f.Decide(c.pkt); got != c.want {
			p, _ := ParseIPv4(c.pkt)
			t.Errorf("%+v: %v, want %v", p, got, c.want)
		}
	}
	if !f.lanTarget(netip.MustParseAddr("192.168.50.99")) || f.lanTarget(netip.MustParseAddr("192.168.51.1")) {
		t.Fatal("lanTarget")
	}
	txt, err := DescribeUplinkRules(srv, lan, []vpn.FwRule{{ID: "b", Chain: vpn.ChainForward, Src: []string{"10.99.0.0/16"}, Dst: []string{"192.168.50.0/24"}, Proto: "udp"}})
	if err != nil || !strings.Contains(txt, "not available on Windows") || !strings.Contains(txt, "192.168.50.0/24") {
		t.Fatalf("%v\n%s", err, txt)
	}
	if _, err := StartUplink(UplinkConfig{PrivateKey: "x", Address: srv, HubPublicKey: "y", Endpoint: "hub:1"}); err == nil {
		t.Fatal("bad config accepted")
	}
	_, pub, _ := vpn.GenerateKey()
	priv, _, _ := vpn.GenerateKey()
	if _, err := StartUplink(UplinkConfig{PrivateKey: priv, Address: srv, HubPublicKey: pub, Endpoint: "hub.example.com:51820"}); err == nil {
		t.Fatal("hostname endpoint accepted (must be resolved first)")
	}
	if _, err := StartUplink(UplinkConfig{PrivateKey: priv, Address: srv, HubPublicKey: pub, Endpoint: "127.0.0.1:9", LAN: []netip.Prefix{netip.MustParsePrefix("10.99.5.0/24")}}); err == nil {
		t.Fatal("LAN inside the hub's range accepted")
	}
	if !isHostAddr(netip.MustParseAddr("127.0.0.1")) {
		t.Fatal("loopback is a host address")
	}
	if _, err := dialLANFromHost(context.Background(), netip.MustParseAddrPort("127.0.0.1:80")); err == nil {
		t.Fatal("relay to the host itself")
	}
}

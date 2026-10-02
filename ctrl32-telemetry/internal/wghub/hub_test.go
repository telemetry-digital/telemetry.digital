package wghub

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

// client is a WireGuard peer with its own userspace stack (wireguard-go netstack), connected to the hub over
// loopback UDP.
type client struct {
	dev  *device.Device
	net  *netstack.Net
	pub  string
	psk  string
	addr string
}

func newClient(t *testing.T, addr string, hubPub string, hubPort int) *client {
	t.Helper()
	priv, pub, err := vpn.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	psk, _ := vpn.GeneratePSK()
	tn, n, err := netstack.CreateNetTUN([]netip.Addr{netip.MustParseAddr(addr)}, nil, MTU)
	if err != nil {
		t.Fatal(err)
	}
	dev := device.NewDevice(tn, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))
	ph, _ := b64ToHex(priv)
	hh, _ := b64ToHex(hubPub)
	kh, _ := b64ToHex(psk)
	cfg := "private_key=" + ph + "\npublic_key=" + hh + "\npreshared_key=" + kh + "\nendpoint=127.0.0.1:" + strconv.Itoa(hubPort) +
		"\npersistent_keepalive_interval=1\nallowed_ip=10.66.0.0/24\n"
	if err := dev.IpcSet(cfg); err != nil {
		t.Fatal(err)
	}
	if err := dev.Up(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dev.Close)
	return &client{dev: dev, net: n, pub: pub, psk: psk, addr: addr}
}

func echoServer(t *testing.T, ln net.Listener) {
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				line, err := bufio.NewReader(c).ReadString('\n')
				if err == nil {
					_, _ = io.WriteString(c, "echo "+line)
				}
			}()
		}
	}()
}

func roundTrip(ctx context.Context, dial func(context.Context) (net.Conn, error)) (string, error) {
	c, err := dial(ctx)
	if err != nil {
		return "", err
	}
	defer c.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = c.SetDeadline(dl)
	}
	if _, err := io.WriteString(c, "hello\n"); err != nil {
		return "", err
	}
	return bufio.NewReader(c).ReadString('\n')
}

func ping(n *netstack.Net, dst string, timeout time.Duration) error {
	pc, err := n.DialPingAddr(netip.Addr{}, netip.MustParseAddr(dst))
	if err != nil {
		return err
	}
	defer pc.Close()
	req := make([]byte, 16)
	req[0] = 8 // echo request; the stack fills identifier and checksum
	binary.BigEndian.PutUint16(req[6:8], 1)
	copy(req[8:], "ctrl32hb")
	if _, err := pc.Write(req); err != nil {
		return err
	}
	_ = pc.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 1500)
	_, err = pc.Read(buf)
	return err
}

func TestHubEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	// a service of "the server" on loopback, reached through the hub's relay
	svc, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	echoServer(t, svc)
	svcPort := uint16(svc.Addr().(*net.TCPAddr).Port)

	priv, _, _ := vpn.GenerateKey()
	srv := netip.MustParsePrefix("10.66.0.1/24")
	h, err := Start(Config{PrivateKey: priv, ListenPort: 0, Address: srv})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Stop()
	port := h.ListenPort()
	if port == 0 {
		t.Fatal("no listen port")
	}
	a := newClient(t, "10.66.0.2", h.PublicKey(), port)
	b := newClient(t, "10.66.0.3", h.PublicKey(), port)

	// B offers a TCP service inside the VPN
	lb, err := b.net.ListenTCP(&net.TCPAddr{IP: net.ParseIP("10.66.0.3"), Port: 7000})
	if err != nil {
		t.Fatal(err)
	}
	defer lb.Close()
	echoServer(t, lb)

	d := vpn.Desired{
		Peers: []vpn.DesiredPeer{
			{ID: "a", Name: "a", PublicKey: a.pub, PresharedKey: a.psk, Address: "10.66.0.2", Enabled: true},
			{ID: "b", Name: "b", PublicKey: b.pub, PresharedKey: b.psk, Address: "10.66.0.3", Enabled: true},
		},
		Rules: []vpn.FwRule{
			{ID: "svc", Chain: vpn.ChainInput, Src: []string{"10.66.0.2/32"}, Dst: []string{"10.66.0.1/32"}, Proto: "tcp", Ports: []string{strconv.Itoa(int(svcPort))}},
		},
	}
	if err := h.Apply(d); err != nil {
		t.Fatal(err)
	}

	dialServer := func(n *netstack.Net, p uint16) func(context.Context) (net.Conn, error) {
		return func(ctx context.Context) (net.Conn, error) {
			return n.DialContextTCPAddrPort(ctx, netip.AddrPortFrom(srv.Addr(), p))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	got, err := roundTrip(ctx, dialServer(a.net, svcPort))
	cancel()
	if err != nil || got != "echo hello\n" {
		t.Fatalf("allowed service through the relay: %q %v", got, err)
	}

	// B has no rule towards the server service; a port without a rule is dropped for A too
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	if _, err := roundTrip(ctx, dialServer(b.net, svcPort)); err == nil {
		t.Fatal("peer without a rule reached the service")
	}
	cancel()
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	if _, err := roundTrip(ctx, dialServer(a.net, svcPort+1)); err == nil {
		t.Fatal("port without a rule reachable")
	}
	cancel()

	// ping of the server always works
	if err := ping(a.net, "10.66.0.1", 5*time.Second); err != nil {
		t.Fatalf("ping server: %v", err)
	}

	// forwarding A -> B only with a rule
	dialB := func(ctx context.Context) (net.Conn, error) {
		return a.net.DialContextTCPAddrPort(ctx, netip.MustParseAddrPort("10.66.0.3:7000"))
	}
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	if _, err := roundTrip(ctx, dialB); err == nil {
		t.Fatal("forwarding without a rule")
	}
	cancel()
	d.Rules = append(d.Rules, vpn.FwRule{ID: "ab", Chain: vpn.ChainForward, Src: []string{"10.66.0.2/32"}, Dst: []string{"10.66.0.3/32"}, Proto: "tcp", Ports: []string{"7000"}})
	d.Forward = true
	if err := h.Apply(d); err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	got, err = roundTrip(ctx, dialB)
	cancel()
	if err != nil || got != "echo hello\n" {
		t.Fatalf("forward with a rule (replies through connection tracking): %q %v", got, err)
	}
	// the reverse direction has no rule
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	if _, err := roundTrip(ctx, func(ctx context.Context) (net.Conn, error) {
		return b.net.DialContextTCPAddrPort(ctx, netip.MustParseAddrPort("10.66.0.2:7000"))
	}); err == nil {
		t.Fatal("reverse forwarding without a rule")
	}
	cancel()

	// live statistics and removing a peer
	stats := h.Peers()
	if len(stats) != 2 {
		t.Fatalf("peers %d", len(stats))
	}
	hs := false
	for _, s := range stats {
		if !s.LastHandshake.IsZero() && s.RxBytes > 0 {
			hs = true
		}
	}
	if !hs {
		t.Fatal("no handshake in the statistics")
	}
	d.Peers = d.Peers[:1]
	if err := h.Apply(d); err != nil {
		t.Fatal(err)
	}
	if len(h.Peers()) != 1 {
		t.Fatal("peer not removed")
	}
	if !strings.Contains(h.Describe(), "rule ab") {
		t.Fatal(h.Describe())
	}
	// a bad key is refused and the previous peers stay
	bad := d
	bad.Peers = append([]vpn.DesiredPeer{}, d.Peers...)
	bad.Peers = append(bad.Peers, vpn.DesiredPeer{ID: "x", Name: "x", PublicKey: "nope", Address: "10.66.0.9", Enabled: true})
	if err := h.Apply(bad); err == nil {
		t.Fatal("bad key accepted")
	}
	if len(h.Peers()) != 1 {
		t.Fatal("previous peers lost")
	}
	if dr, in, fw := h.Counters(); dr == 0 || in == 0 || fw == 0 {
		t.Fatalf("counters %d %d %d", dr, in, fw)
	}
}

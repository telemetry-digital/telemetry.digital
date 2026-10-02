// Package wghub is the userspace WireGuard hub of the VPN for servers without kernel WireGuard and nftables (Windows,
// ADR-0020). WireGuard runs inside the agent process (wireguard-go) on a virtual interface that is not an interface of
// the operating system: packets from peers pass the in-process filter (filter.go: the same compiled access rules as
// the nftables table on Linux, default deny), packets for the server's own services go into a private TCP/IP stack
// (gvisor) whose TCP forwarder relays accepted connections to 127.0.0.1:<port>, and packets for other peers or site
// subnets are sent back into WireGuard without touching the host. No driver (wintun), no DLL and no routing or
// firewall changes of the host are needed besides the inbound UDP port.
package wghub

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/icmp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/waiter"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

// MTU of the virtual interface (the same as the clients' wg-quick default).
const MTU = 1420

// maxConns limits concurrent relayed TCP connections to the server's services.
const maxConns = 1024

// vtun is the virtual interface between wireguard-go and the hub: Write receives decrypted packets from peers, Read
// hands packets to WireGuard for encryption (replies of the stack and forwarded packets).
type vtun struct {
	ep         *channel.Endpoint
	filter     *Filter
	out        chan []byte
	events     chan tun.Event
	done       chan struct{}
	once       sync.Once
	queueDrops atomic.Uint64
}

func (t *vtun) File() *os.File           { return nil }
func (t *vtun) MTU() (int, error)        { return MTU, nil }
func (t *vtun) Name() (string, error)    { return "ctrl32-vpn", nil }
func (t *vtun) Events() <-chan tun.Event { return t.events }
func (t *vtun) BatchSize() int           { return 1 }
func (t *vtun) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	select {
	case p := <-t.out:
		sizes[0] = copy(bufs[0][offset:], p)
		return 1, nil
	case <-t.done:
		return 0, os.ErrClosed
	}
}

func (t *vtun) Write(bufs [][]byte, offset int) (int, error) {
	for _, b := range bufs {
		pkt := b[offset:]
		if len(pkt) == 0 {
			continue
		}
		switch t.filter.Decide(pkt) {
		case Input:
			pkb := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(pkt)})
			t.ep.InjectInbound(header.IPv4ProtocolNumber, pkb)
			pkb.DecRef()
		case Forward:
			cp := append([]byte(nil), pkt...)
			select {
			case t.out <- cp:
			default:
				t.queueDrops.Add(1) // the queue towards WireGuard is full: drop like a congested link
			}
		}
	}
	return len(bufs), nil
}

// WriteNotify is called by the stack's link endpoint when it has a packet to send to a peer.
func (t *vtun) WriteNotify() {
	pkt := t.ep.Read()
	if pkt == nil {
		return
	}
	view := pkt.ToView()
	pkt.DecRef()
	b := append([]byte(nil), view.AsSlice()...)
	view.Release()
	select {
	case t.out <- b:
	case <-t.done:
	}
}

func (t *vtun) Close() error {
	t.once.Do(func() {
		close(t.done)
		close(t.events)
	})
	return nil
}

// Config starts a hub.
type Config struct {
	PrivateKey string       // base64, the server's key
	ListenPort int          // UDP; 0 picks a free port (tests)
	Address    netip.Prefix // the server's VPN address with the VPN prefix, e.g. 10.66.0.1/24
	// Dial connects an accepted connection to a service of the server; default 127.0.0.1:<port>.
	Dial func(ctx context.Context, port uint16) (net.Conn, error)
	Log  *slog.Logger
}

// PeerStats are live values of one peer.
type PeerStats struct {
	PublicKey     string // base64
	Endpoint      string
	LastHandshake time.Time
	RxBytes       int64
	TxBytes       int64
}

// Hub is a running userspace WireGuard hub.
type Hub struct {
	mu      sync.Mutex
	cfg     Config
	dev     *device.Device
	tun     *vtun
	stack   *stack.Stack
	filter  *Filter
	applied vpn.Desired
	peers   map[string]bool // hex public keys configured on the device
	conns   chan struct{}
	log     *slog.Logger
}

func b64ToHex(k string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(k))
	if err != nil || len(b) != 32 {
		return "", errors.New("invalid WireGuard key")
	}
	return hex.EncodeToString(b), nil
}

func hexToB64(k string) string {
	b, err := hex.DecodeString(k)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}

// Start creates the virtual interface, the stack and the WireGuard device and listens on the UDP port. No peer is
// configured and everything from the VPN is dropped until Apply.
func Start(cfg Config) (*Hub, error) {
	if !cfg.Address.Addr().Is4() {
		return nil, errors.New("the VPN address must be IPv4")
	}
	priv, err := b64ToHex(cfg.PrivateKey)
	if err != nil {
		return nil, err
	}
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if cfg.Dial == nil {
		cfg.Dial = func(ctx context.Context, port uint16) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))))
		}
	}
	h := &Hub{cfg: cfg, filter: NewFilter(cfg.Address), peers: map[string]bool{}, conns: make(chan struct{}, maxConns), log: cfg.Log}
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, icmp.NewProtocol4},
	})
	t := &vtun{ep: channel.New(1024, MTU, ""), filter: h.filter, out: make(chan []byte, 1024), events: make(chan tun.Event, 4), done: make(chan struct{})}
	t.ep.AddNotify(t)
	if e := s.CreateNIC(1, t.ep); e != nil {
		return nil, fmt.Errorf("virtual interface: %v", e)
	}
	if e := s.AddProtocolAddress(1, tcpip.ProtocolAddress{Protocol: ipv4.ProtocolNumber,
		AddressWithPrefix: tcpip.AddrFromSlice(cfg.Address.Addr().AsSlice()).WithPrefix()}, stack.AddressProperties{}); e != nil {
		return nil, fmt.Errorf("VPN address: %v", e)
	}
	s.AddRoute(tcpip.Route{Destination: header.IPv4EmptySubnet, NIC: 1})
	fwd := tcp.NewForwarder(s, 0, 256, h.relay)
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, fwd.HandlePacket)
	h.stack, h.tun = s, t
	t.events <- tun.EventUp

	h.dev = device.NewDevice(t, conn.NewDefaultBind(), &device.Logger{
		Verbosef: func(string, ...any) {},
		Errorf:   func(f string, a ...any) { cfg.Log.Warn("wireguard: " + fmt.Sprintf(f, a...)) },
	})
	if err := h.dev.IpcSet("private_key=" + priv + "\nlisten_port=" + strconv.Itoa(cfg.ListenPort) + "\n"); err != nil {
		h.Stop()
		return nil, fmt.Errorf("WireGuard: %w", err)
	}
	if err := h.dev.Up(); err != nil {
		h.Stop()
		return nil, fmt.Errorf("WireGuard could not listen on UDP port %d: %w", cfg.ListenPort, err)
	}
	return h, nil
}

// relay accepts a TCP connection that passed the filter and connects it to the service on the server.
func (h *Hub) relay(r *tcp.ForwarderRequest) {
	id := r.ID()
	select {
	case h.conns <- struct{}{}:
	default:
		r.Complete(true)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	up, err := h.cfg.Dial(ctx, id.LocalPort)
	cancel()
	if err != nil {
		<-h.conns
		r.Complete(true) // nothing listens there: reset, as a closed port would
		return
	}
	var wq waiter.Queue
	ep, e := r.CreateEndpoint(&wq)
	if e != nil {
		<-h.conns
		_ = up.Close()
		r.Complete(true)
		return
	}
	r.Complete(false)
	down := gonet.NewTCPConn(&wq, ep)
	go func() {
		defer func() { <-h.conns }()
		pipe(down, up)
	}()
}

func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		if c, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = c.CloseWrite()
		}
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done
	<-done
	_ = a.Close()
	_ = b.Close()
}

// ListenPort is the UDP port WireGuard listens on.
func (h *Hub) ListenPort() int {
	s, err := h.dev.IpcGet()
	if err != nil {
		return 0
	}
	for _, l := range strings.Split(s, "\n") {
		if v, ok := strings.CutPrefix(l, "listen_port="); ok {
			n, _ := strconv.Atoi(v)
			return n
		}
	}
	return 0
}

// PublicKey of the hub (base64).
func (h *Hub) PublicKey() string {
	s, err := h.dev.IpcGet()
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(s, "\n") {
		if v, ok := strings.CutPrefix(l, "private_key="); ok {
			b, _ := hex.DecodeString(v)
			if len(b) == 32 {
				if pub, err := vpn.PublicKey(base64.StdEncoding.EncodeToString(b)); err == nil {
					return pub
				}
			}
		}
	}
	return ""
}

// peerConfig renders the UAPI configuration that turns the configured peers into the wanted ones without
// recreating peers that stay (their sessions survive).
func peerConfig(d vpn.Desired, have map[string]bool) (string, map[string]bool, error) {
	var b strings.Builder
	want := map[string]bool{}
	for _, p := range d.Peers {
		if !p.Enabled {
			continue
		}
		pub, err := b64ToHex(p.PublicKey)
		if err != nil {
			return "", nil, fmt.Errorf("peer %s: %w", p.Name, err)
		}
		want[pub] = true
		b.WriteString("public_key=" + pub + "\n")
		if p.PresharedKey != "" {
			psk, err := b64ToHex(p.PresharedKey)
			if err != nil {
				return "", nil, fmt.Errorf("peer %s: preshared key: %w", p.Name, err)
			}
			b.WriteString("preshared_key=" + psk + "\n")
		}
		b.WriteString("replace_allowed_ips=true\n")
		for _, a := range p.AllowedIPs() {
			b.WriteString("allowed_ip=" + a + "\n")
		}
	}
	var rm strings.Builder
	for k := range have {
		if !want[k] {
			rm.WriteString("public_key=" + k + "\nremove=true\n")
		}
	}
	return rm.String() + b.String(), want, nil
}

// Apply makes the hub match a compiled VPN state: the filter rules and the peers. On failure the previous state is
// restored and the error returned.
func (h *Hub) Apply(d vpn.Desired) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	oldDesired := h.applied
	if err := h.filter.SetRules(h.cfg.Address, d); err != nil {
		return err
	}
	conf, want, err := peerConfig(d, h.peers)
	if err == nil {
		err = h.dev.IpcSet(conf)
	}
	if err != nil {
		_ = h.filter.SetRules(h.cfg.Address, oldDesired)
		if back, w, e := peerConfig(oldDesired, want); e == nil && h.dev.IpcSet(back) == nil {
			h.peers = w
		}
		return fmt.Errorf("applying the peers failed: %w", err)
	}
	h.peers, h.applied = want, d
	return nil
}

// Describe is the ruleset text enforced now.
func (h *Hub) Describe() string { return h.filter.Describe() }

// Counters: packets dropped, delivered to the server, forwarded.
func (h *Hub) Counters() (dropped, input, forwarded uint64) { return h.filter.Counters() }

// Peers returns the live values of the configured peers.
func (h *Hub) Peers() []PeerStats {
	s, err := h.dev.IpcGet()
	if err != nil {
		return nil
	}
	var out []PeerStats
	var cur *PeerStats
	var sec, nsec int64
	flush := func() {
		if cur != nil {
			if sec > 0 {
				cur.LastHandshake = time.Unix(sec, nsec)
			}
			out = append(out, *cur)
		}
	}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		k, v, _ := strings.Cut(sc.Text(), "=")
		switch k {
		case "public_key":
			flush()
			cur, sec, nsec = &PeerStats{PublicKey: hexToB64(v)}, 0, 0
		case "endpoint":
			if cur != nil {
				cur.Endpoint = v
			}
		case "last_handshake_time_sec":
			sec, _ = strconv.ParseInt(v, 10, 64)
		case "last_handshake_time_nsec":
			nsec, _ = strconv.ParseInt(v, 10, 64)
		case "rx_bytes":
			if cur != nil {
				cur.RxBytes, _ = strconv.ParseInt(v, 10, 64)
			}
		case "tx_bytes":
			if cur != nil {
				cur.TxBytes, _ = strconv.ParseInt(v, 10, 64)
			}
		}
	}
	flush()
	return out
}

// Stop closes WireGuard, the stack and the virtual interface.
func (h *Hub) Stop() {
	if h.dev != nil {
		h.dev.Close() // closes the virtual interface too
	} else if h.tun != nil {
		_ = h.tun.Close()
	}
	if h.stack != nil {
		h.stack.Close()
	}
}

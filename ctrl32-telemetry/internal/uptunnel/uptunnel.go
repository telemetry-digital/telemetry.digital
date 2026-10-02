// Package uptunnel is the HTTPS transport of the VPN uplink (ADR-0021): for customer networks that block outgoing UDP,
// the WireGuard datagrams of the uplink travel inside a WebSocket over the hub's HTTPS port (TCP 443, through the hub's
// reverse proxy) instead of UDP to the hub's WireGuard port.
//
//	customer server                                      hub
//	wg1 (kernel or userspace) ⇄ UDP 127.0.0.1:<port> ⇄ Client ══ wss://hub/api/v1/vpn/uplink/tunnel ══ Handler ⇄ UDP 127.0.0.1:<wg port>
//
// Nothing here is trusted for confidentiality or integrity: every frame is a WireGuard datagram, encrypted and
// authenticated by WireGuard end to end; the relay only moves bytes. The WebSocket is authenticated with a per-uplink
// bearer secret (the hub stores its SHA-256 only), so the hub's WireGuard port is not opened to the internet through
// the web server. Each binary message carries exactly one datagram (at most MaxFrame bytes); anything else closes the
// connection. WireGuard over TCP is slower under packet loss (TCP inside TCP); it is meant for maintenance access,
// not bulk transfers.
package uptunnel

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// MaxFrame is the largest datagram relayed (WireGuard packets at MTU 1420 are about 1500 bytes).
const MaxFrame = 4096

// idle closes a relay that carried nothing for this long (WireGuard's keepalive of 25 s keeps an uplink alive).
const idle = 2 * time.Minute

// maxConns limits concurrent relays on the hub.
const maxConns = 1000

var upgrader = websocket.Upgrader{ReadBufferSize: MaxFrame, WriteBufferSize: MaxFrame,
	// not a browser endpoint: no cookies are used, the bearer secret authenticates; refuse browser pages anyway
	CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == "" }}

// Handler is the hub's endpoint. Lookup resolves the SHA-256 (hex) of a presented key to an active uplink peer;
// Target is the hub's WireGuard UDP port on the loopback address.
type Handler struct {
	Lookup func(ctx context.Context, keyHash string) (peerID, name string, ok bool)
	Target func(ctx context.Context) (netip.AddrPort, error)
	Hash   func(key string) string
	Log    *slog.Logger

	mu     sync.Mutex
	active map[string]*websocket.Conn // one relay per uplink peer: a new connection replaces the old one
	count  atomic.Int32
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if k, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(k)
	}
	return ""
}

func (h *Handler) log() *slog.Logger {
	if h.Log == nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return h.Log
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := bearer(r)
	peerID, name, ok := "", "", false
	if len(key) >= 32 && len(key) <= 128 {
		peerID, name, ok = h.Lookup(r.Context(), h.Hash(key))
	}
	if !ok {
		h.log().Warn("vpn uplink tunnel: refused (unknown or disabled uplink key)", "security", true, "remote", r.RemoteAddr)
		time.Sleep(500 * time.Millisecond)
		http.Error(w, "unknown uplink", http.StatusUnauthorized)
		return
	}
	if h.count.Add(1) > maxConns {
		h.count.Add(-1)
		http.Error(w, "too many uplink tunnels", http.StatusServiceUnavailable)
		return
	}
	defer h.count.Add(-1)
	target, err := h.Target(r.Context())
	if err != nil {
		http.Error(w, "the VPN of this server is not running", http.StatusServiceUnavailable)
		return
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // the upgrader answered
	}
	udp, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(target))
	if err != nil {
		_ = ws.Close()
		return
	}
	h.mu.Lock()
	if h.active == nil {
		h.active = map[string]*websocket.Conn{}
	}
	if old := h.active[peerID]; old != nil {
		_ = old.Close()
	}
	h.active[peerID] = ws
	h.mu.Unlock()
	h.log().Info("vpn uplink tunnel connected", "uplink", name, "remote", r.RemoteAddr)
	relay(ws, udp, nil)
	h.mu.Lock()
	if h.active[peerID] == ws {
		delete(h.active, peerID)
	}
	h.mu.Unlock()
	h.log().Info("vpn uplink tunnel closed", "uplink", name)
}

// Close ends the relay of an uplink peer (removed, disabled or rotated).
func (h *Handler) Close(peerID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c := h.active[peerID]; c != nil {
		_ = c.Close()
		delete(h.active, peerID)
	}
}

// relay moves datagrams between a WebSocket and a connected UDP socket (hub) or, with wgAddr set, an unconnected local
// socket whose WireGuard address is learned from the datagrams it sends (customer; the socket stays open for the next
// connection). It returns when either side ends.
func relay(ws *websocket.Conn, udp *net.UDPConn, wgAddr *atomic.Pointer[net.UDPAddr]) {
	done := make(chan struct{}, 2)
	stop := make(chan struct{})
	var wmu sync.Mutex
	ws.SetReadLimit(MaxFrame)
	go func() { // websocket -> udp
		defer func() { done <- struct{}{} }()
		for {
			_ = ws.SetReadDeadline(time.Now().Add(idle))
			t, b, err := ws.ReadMessage()
			if err != nil || t != websocket.BinaryMessage {
				return
			}
			if wgAddr == nil {
				if _, err := udp.Write(b); err != nil {
					return
				}
			} else if a := wgAddr.Load(); a != nil {
				_, _ = udp.WriteToUDP(b, a)
			}
		}
	}()
	go func() { // udp -> websocket
		defer func() { done <- struct{}{} }()
		buf := make([]byte, MaxFrame)
		for {
			_ = udp.SetReadDeadline(time.Now().Add(time.Second))
			n, from, err := udp.ReadFromUDP(buf)
			if err != nil {
				var ne net.Error
				if errors.As(err, &ne) && ne.Timeout() {
					select {
					case <-stop:
						return
					default:
						continue
					}
				}
				return
			}
			if wgAddr != nil {
				// the socket listens on 127.0.0.1, so only this host can send to it; the source address is not
				// necessarily a loopback one (kernel WireGuard keeps the source address it used for the hub before)
				wgAddr.Store(from)
			}
			wmu.Lock()
			_ = ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err = ws.WriteMessage(websocket.BinaryMessage, buf[:n])
			wmu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	<-done
	close(stop)
	_ = ws.Close()
	if wgAddr == nil {
		_ = udp.Close()
	}
	<-done
}

// ---- customer side --------------------------------------------------------------------------------------------------

// Client is the customer's end: a UDP socket on 127.0.0.1 that the uplink's WireGuard uses as the hub's endpoint, and
// a WebSocket to the hub that is re-established with back-off while the client runs.
type Client struct {
	url, key string
	udp      *net.UDPConn
	log      *slog.Logger
	wgAddr   atomic.Pointer[net.UDPAddr]
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}

	mu        sync.Mutex
	connected bool
	since     time.Time
	lastErr   string
}

// Status of the client.
type Status struct {
	Connected bool      `json:"connected"`
	Since     time.Time `json:"since,omitzero"`
	Error     string    `json:"error,omitempty"`
}

// NewClient listens on 127.0.0.1:port (0 = any free port; a busy port falls back to a free one) and starts connecting.
func NewClient(url, key string, port int, log *slog.Logger) (*Client, error) {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil && port != 0 {
		udp, err = net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	}
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{url: url, key: key, udp: udp, log: log, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	go c.loop()
	return c, nil
}

// Port is the local UDP port (the uplink's WireGuard endpoint is 127.0.0.1:Port).
func (c *Client) Port() int { return c.udp.LocalAddr().(*net.UDPAddr).Port }

// Endpoint is the local endpoint for WireGuard.
func (c *Client) Endpoint() string { return "127.0.0.1:" + strconv.Itoa(c.Port()) }

// Status reports whether the WebSocket to the hub is up.
func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Status{Connected: c.connected, Since: c.since, Error: c.lastErr}
}

// Close stops the client and releases the port.
func (c *Client) Close() {
	c.cancel()
	_ = c.udp.Close()
	<-c.done
}

func (c *Client) set(connected bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if connected != c.connected {
		c.since = time.Now()
	}
	c.connected = connected
	if err != nil {
		c.lastErr = err.Error()
	} else if connected {
		c.lastErr = ""
	}
}

func (c *Client) loop() {
	defer close(c.done)
	backoff := time.Second
	d := websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: 15 * time.Second, ReadBufferSize: MaxFrame, WriteBufferSize: MaxFrame}
	for c.ctx.Err() == nil {
		hdr := http.Header{"Authorization": {"Bearer " + c.key}, "User-Agent": {"ctrl32-telemetry-uplink"}}
		ws, resp, err := d.DialContext(c.ctx, strings.Replace(strings.Replace(c.url, "https://", "wss://", 1), "http://", "ws://", 1), hdr)
		if err != nil {
			if resp != nil {
				err = errors.New("the hub refused the tunnel: " + resp.Status)
			}
			c.set(false, err)
			c.log.Warn("vpn uplink: HTTPS tunnel to the hub failed", "err", err, "retry_in", backoff)
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		backoff = time.Second
		c.set(true, nil)
		c.log.Info("vpn uplink: HTTPS tunnel to the hub connected")
		stop := context.AfterFunc(c.ctx, func() { _ = ws.Close() })
		relay(ws, c.udp, &c.wgAddr)
		stop()
		c.set(false, errors.New("the tunnel closed"))
	}
}

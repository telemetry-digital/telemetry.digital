package video

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ONVIF control of cameras that send their video through a relay (0.58.0). The server cannot reach a camera's ONVIF
// address in the remote network, so the relay carries the requests: it long-polls
//
//	GET  /api/v1/video/relay/{camera id}/onvif/next     → one pending request {rid, method, path, headers, body}
//	POST /api/v1/video/relay/{camera id}/onvif/{rid}    ← the camera's answer {status, headers, body}
//
// over the server's ordinary HTTPS port (no new port, works behind a reverse proxy), authenticated with the camera's
// push secret. The ONVIF client of such a camera gets an http.Client whose transport puts each request into the
// camera's queue and waits for the answer, so PTZ, presets and events work unchanged. The server hands out only a
// path at the camera's ONVIF origin; the relay builds the address from the origin in its own relay.toml, so the
// server can never use a relay to reach anything else in the remote network.

// tunnelWait is how long a request waits for the relay's answer (a variable for tests).
var tunnelWait = 10 * time.Second

const (
	tunnelMaxBody = 1 << 20          // request and answer bodies, each way
	tunnelPoll    = 25 * time.Second // longest long poll of the relay
	tunnelSeen    = 35 * time.Second // a relay polled within this time is connected
	tunnelQueue   = 32               // requests waiting for the relay
	tunnelPollers = 4                // long polls of one camera at a time
)

// ErrRelayNotConnected is returned for ONVIF requests of a push camera whose relay does not poll for them.
var ErrRelayNotConnected = errors.New("the relay of this camera is not connected for camera control (onvif = … in relay.toml, relay 0.58.0 or newer)")

// ErrRelayAuth and ErrRelayBlocked answer a relay with a wrong camera or secret, or from a blocked address.
var (
	ErrRelayAuth    = errors.New("wrong camera or secret")
	ErrRelayBlocked = errors.New("too many failed attempts from this address")
)

// TunnelRequest is one HTTP request for a camera's ONVIF service, handed to its relay.
type TunnelRequest struct {
	ID     string            `json:"rid"`
	Method string            `json:"method"`
	Path   string            `json:"path"` // path and query at the camera's ONVIF origin (tunnelAllowed)
	Header map[string]string `json:"headers,omitempty"`
	Body   []byte            `json:"body"` // base64 in JSON

	ctx   context.Context
	reply chan TunnelResponse
}

// TunnelResponse is the camera's answer, or the relay's reason why there is none.
type TunnelResponse struct {
	Status int               `json:"status"`
	Header map[string]string `json:"headers,omitempty"`
	Body   []byte            `json:"body"`
	Error  string            `json:"error,omitempty"`
}

// tunnelHeaders are the only headers carried in either direction.
var tunnelHeaders = []string{"Content-Type", "Soapaction", "Accept"}

func pickHeaders(h http.Header) map[string]string {
	out := map[string]string{}
	for _, k := range tunnelHeaders {
		if v := h.Get(k); v != "" && len(v) <= 512 {
			out[k] = v
		}
	}
	return out
}

// tunnelAllowed decides, on the server and again on the relay, what may reach a camera at its ONVIF origin. Paths
// under /onvif/ take POST and GET. A camera may also name another address at the same origin for a service it
// creates — TP-Link Tapo gives its events subscription as /event-1025_1025 (0.61.1) — so elsewhere only a SOAP call
// is carried: POST, a SOAP content type and an XML envelope in the body. The camera's web pages and GET-driven CGI
// stay out of reach; the origin (relay.toml) remains the boundary, and the server holds the camera's ONVIF account
// anyway.
func tunnelAllowed(method, path, contentType string, body []byte) error {
	if strings.HasPrefix(path, "/onvif/") {
		if method != http.MethodPost && method != http.MethodGet {
			return errors.New("only POST and GET are sent through the relay")
		}
		return nil
	}
	if !strings.HasPrefix(path, "/") || path == "/" {
		return errors.New("only /onvif/… or a SOAP service of the camera is reached through the relay")
	}
	if method != http.MethodPost {
		return errors.New("outside /onvif/ only SOAP calls (POST) are sent through the relay")
	}
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	if ct != "application/soap+xml" && ct != "text/xml" {
		return errors.New("outside /onvif/ only SOAP calls (application/soap+xml) are sent through the relay")
	}
	head := body[:min(len(body), 2048)]
	if !bytes.HasPrefix(bytes.TrimSpace(head), []byte("<")) || !(bytes.Contains(head, []byte(":Envelope")) || bytes.Contains(head, []byte("<Envelope"))) {
		return errors.New("outside /onvif/ only SOAP envelopes are sent through the relay")
	}
	return nil
}

// tunnel is the queue of one push camera.
type tunnel struct {
	queue chan *TunnelRequest

	mu      sync.Mutex
	pending map[string]*TunnelRequest
	seen    time.Time // the relay's last long poll
	polling int
}

func newTunnel() *tunnel {
	return &tunnel{queue: make(chan *TunnelRequest, tunnelQueue), pending: map[string]*TunnelRequest{}}
}

func (t *tunnel) connected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.polling > 0 || time.Since(t.seen) < tunnelSeen
}

// do queues a request and waits for the relay's answer.
func (t *tunnel) do(ctx context.Context, rq *TunnelRequest) (TunnelResponse, error) {
	if !t.connected() {
		return TunnelResponse{}, ErrRelayNotConnected
	}
	ctx, cancel := context.WithTimeout(ctx, tunnelWait)
	defer cancel()
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	rq.ID, rq.ctx, rq.reply = hex.EncodeToString(id), ctx, make(chan TunnelResponse, 1)
	t.mu.Lock()
	t.pending[rq.ID] = rq
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.pending, rq.ID)
		t.mu.Unlock()
	}()
	select {
	case t.queue <- rq:
	default:
		return TunnelResponse{}, errors.New("too many camera control requests are waiting for the relay")
	}
	select {
	case r := <-rq.reply:
		return r, nil
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return TunnelResponse{}, errors.New("the relay did not bring the camera's answer in time")
		}
		return TunnelResponse{}, ctx.Err()
	}
}

// next waits up to wait for a request; nil when none came. Requests whose caller gave up are skipped.
func (t *tunnel) next(ctx context.Context, wait time.Duration) (*TunnelRequest, error) {
	t.mu.Lock()
	if t.polling >= tunnelPollers {
		t.mu.Unlock()
		return nil, errors.New("too many polls for this camera")
	}
	t.polling++
	t.seen = time.Now()
	t.mu.Unlock()
	seen := time.Now() // a relay that finished its poll polls again at once
	defer func() {
		t.mu.Lock()
		t.polling--
		t.seen = seen
		t.mu.Unlock()
	}()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		select {
		case rq := <-t.queue:
			if rq.ctx.Err() != nil {
				continue
			}
			seen = time.Now()
			return rq, nil
		case <-timer.C:
			seen = time.Now()
			return nil, nil
		case <-ctx.Done():
			// the relay went away in the middle of its poll: unless it polls again within a few seconds, it is gone
			seen = time.Now().Add(3*time.Second - tunnelSeen)
			return nil, nil
		}
	}
}

// answer delivers the relay's answer; false for an unknown (or expired) request.
func (t *tunnel) answer(rid string, r TunnelResponse) bool {
	t.mu.Lock()
	rq := t.pending[rid]
	t.mu.Unlock()
	if rq == nil {
		return false
	}
	select {
	case rq.reply <- r:
		return true
	default:
		return false // answered twice
	}
}

func (m *Manager) tunnelOf(id string, create bool) *tunnel {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.tunnels[id]
	if t == nil && create {
		if m.tunnels == nil {
			m.tunnels = map[string]*tunnel{}
		}
		t = newTunnel()
		m.tunnels[id] = t
	}
	return t
}

func (m *Manager) dropTunnel(id string) {
	m.mu.Lock()
	delete(m.tunnels, id)
	m.mu.Unlock()
}

func (m *Manager) tunnelConnected(id string) bool {
	t := m.tunnelOf(id, false)
	return t != nil && t.connected()
}

// relayAuth checks a relay's camera id and secret (constant time); failures count towards blocking the address.
func (m *Manager) relayAuth(id, secret, ip string) error {
	if m.relayBlocked(ip) {
		return ErrRelayBlocked
	}
	m.mu.Lock()
	c := m.cams[id]
	var want string
	if c != nil && c.cfg.Push {
		want = c.cfg.PushSecret
	}
	m.mu.Unlock()
	if want == "" || secret == "" || subtle.ConstantTimeCompare([]byte(want), []byte(secret)) != 1 {
		m.relayFail(ip, "wrong camera or secret (camera control)")
		return ErrRelayAuth
	}
	return nil
}

// RelayNext is the relay's long poll for the camera's next ONVIF request: nil after wait without one. wait = 0 only
// tests the secret and never hands out a request (the relay's self-check).
func (m *Manager) RelayNext(ctx context.Context, id, secret, ip string, wait time.Duration) (*TunnelRequest, error) {
	if err := m.relayAuth(id, secret, ip); err != nil {
		return nil, err
	}
	if wait <= 0 {
		return nil, nil
	}
	return m.tunnelOf(id, true).next(ctx, min(wait, tunnelPoll))
}

// RelayAuth checks a relay's camera id and secret before its answer is read.
func (m *Manager) RelayAuth(id, secret, ip string) error { return m.relayAuth(id, secret, ip) }

// RelayAnswer takes the answer of a relay authenticated with RelayAuth; false for an unknown or expired request.
func (m *Manager) RelayAnswer(id, rid string, r TunnelResponse) bool {
	if len(r.Body) > tunnelMaxBody {
		r = TunnelResponse{Error: "the camera's answer is larger than 1 MiB"}
	}
	t := m.tunnelOf(id, false)
	return t != nil && t.answer(rid, r)
}

// onvifOrigin is scheme://host[:port] of an ONVIF address.
func onvifOrigin(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", errors.New("the ONVIF address must be http(s)://host[:port]/onvif/… without credentials")
	}
	return strings.ToLower(u.Scheme + "://" + u.Host), nil
}

// TunnelClient is an HTTP client that reaches a push camera's ONVIF origin through its relay (the camera form's
// "fill in from ONVIF" for a camera behind a relay).
func (m *Manager) TunnelClient(id, device string) (*http.Client, error) {
	return m.tunnelClient(id, device)
}

// tunnelClient is the HTTP client of a push camera's ONVIF client: requests go through the relay.
func (m *Manager) tunnelClient(id, device string) (*http.Client, error) {
	origin, err := onvifOrigin(device)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Transport:     &tunnelTransport{m: m, cam: id, origin: origin},
		Timeout:       tunnelWait + 2*time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, nil
}

// tunnelTransport is the http.RoundTripper that sends a request through the camera's relay.
type tunnelTransport struct {
	m      *Manager
	cam    string
	origin string
}

// RoundTrip implements http.RoundTripper. Only the camera's configured ONVIF origin is allowed, and there what
// tunnelAllowed permits (paths under /onvif/, elsewhere SOAP calls only).
func (tt *tunnelTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		defer req.Body.Close()
	}
	if o := strings.ToLower(req.URL.Scheme + "://" + req.URL.Host); o != tt.origin {
		return nil, fmt.Errorf("refused: %s is not the camera's ONVIF address", o)
	}
	path := req.URL.EscapedPath()
	if req.Method != http.MethodPost && req.Method != http.MethodGet {
		return nil, errors.New("refused: only POST and GET are sent through the relay")
	}
	if req.URL.RawQuery != "" {
		path += "?" + req.URL.RawQuery
	}
	var body []byte
	if req.Body != nil {
		b, err := io.ReadAll(io.LimitReader(req.Body, tunnelMaxBody+1))
		if err != nil {
			return nil, err
		}
		if len(b) > tunnelMaxBody {
			return nil, errors.New("refused: request larger than 1 MiB")
		}
		body = b
	}
	if err := tunnelAllowed(req.Method, req.URL.EscapedPath(), req.Header.Get("Content-Type"), body); err != nil {
		return nil, errors.New("refused: " + err.Error())
	}
	t := tt.m.tunnelOf(tt.cam, false)
	if t == nil {
		return nil, ErrRelayNotConnected
	}
	r, err := t.do(req.Context(), &TunnelRequest{Method: req.Method, Path: path, Header: pickHeaders(req.Header), Body: body})
	if err != nil {
		return nil, err
	}
	if r.Error != "" {
		return nil, errors.New("relay: " + r.Error)
	}
	if r.Status < 100 || r.Status > 599 {
		return nil, errors.New("relay: invalid answer")
	}
	h := http.Header{}
	for _, k := range tunnelHeaders {
		if v := r.Header[k]; v != "" {
			h.Set(k, v)
		}
	}
	return &http.Response{Status: fmt.Sprintf("%d %s", r.Status, http.StatusText(r.Status)), StatusCode: r.Status, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: h, Body: io.NopCloser(bytes.NewReader(r.Body)), ContentLength: int64(len(r.Body)), Request: req}, nil
}

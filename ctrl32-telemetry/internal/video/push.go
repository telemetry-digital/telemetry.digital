package video

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"log/slog"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/auth"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/liberrors"
)

// Cameras at remote sites send their video to the server (spec 011): a relay at the site (`ctrl32-telemetry
// camera-relay`) reads the local cameras and publishes each stream over RTSPS to rtsps://server:port/<camera id>
// (or …/<camera id>/sub), authenticated with the camera's own secret. The site needs no open port and no VPN; the
// connection is outgoing and encrypted. Without TLS (intranet only) the secret is checked with digest authentication,
// so it never crosses the network in clear text.

// pushConn is one publishing session handed to the camera's Source.
type pushConn struct {
	sess  *gortsplib.ServerSession
	desc  *description.Session
	reply chan error
	done  chan struct{}
}

// waitPush serves a push camera: it waits for a publisher and decodes its stream until the publisher leaves.
func (s *Source) waitPush(ctx context.Context) error {
	s.mu.Lock()
	if s.push == nil {
		s.push = make(chan *pushConn)
	}
	ch := s.push
	if s.status.State != "offline" {
		s.status = Status{State: "offline", Error: "waiting for the relay to connect", Since: time.Now()}
	}
	s.mu.Unlock()
	var pc *pushConn
	select {
	case <-ctx.Done():
		return nil
	case pc = <-ch:
	}
	medi, forma, onPacket, err := s.decoder(ctx, pc.desc, pc.sess.PacketPTS)
	if err == nil {
		// registered while OnRecord still waits for the reply, so before the session reads any packet
		pc.sess.OnPacketRTP(medi, forma, onPacket)
		if am, af, aon := s.withSound(ctx, pc.desc, pc.sess.PacketPTS); am != nil {
			pc.sess.OnPacketRTP(am, af, aon)
		}
	}
	pc.reply <- err
	if err != nil {
		return err
	}
	select {
	case <-pc.done:
		return errors.New("the camera (relay) disconnected")
	case <-ctx.Done():
		pc.sess.Close()
		return nil
	}
}

// attach hands a publisher to the source; the source must be running (push sources are always kept).
func (s *Source) attach(pc *pushConn) error {
	s.mu.Lock()
	if s.push == nil {
		s.push = make(chan *pushConn)
	}
	ch := s.push
	s.mu.Unlock()
	select {
	case ch <- pc:
	case <-time.After(5 * time.Second):
		return errors.New("camera not ready")
	}
	select {
	case err := <-pc.reply:
		return err
	case <-time.After(5 * time.Second):
		return errors.New("camera not ready")
	}
}

// NewPushSecret makes a random secret for a push camera.
func NewPushSecret() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// PushConfig is the listener for cameras that send their video.
type PushConfig struct {
	Listen  string // e.g. ":8322"; empty = off
	TLSCert string // PEM files; without them only digest authentication over plain RTSP (intranet)
	TLSKey  string
}

// PushServer accepts published camera streams.
type PushServer struct {
	m   *Manager
	srv *gortsplib.Server
	log *slog.Logger
	tls bool

	mu       sync.Mutex
	sessions map[*gortsplib.ServerSession]*pushConn
	targets  map[string]*gortsplib.ServerSession // camera path → current publisher
}

// authGuard counts failed relay authentications per address — RTSP publishing and the ONVIF control channel alike —
// and blocks an address that fails too often (settings push_max_fails, push_window_minutes, push_block_minutes).
type authGuard struct {
	mu    sync.Mutex
	fails map[string]*failRec
}

type failRec struct {
	n     int
	first time.Time
	until time.Time
}

// relayBlocked reports whether an address is blocked after too many failed relay authentications.
func (m *Manager) relayBlocked(ip string) bool {
	m.guard.mu.Lock()
	defer m.guard.mu.Unlock()
	f := m.guard.fails[ip]
	return f != nil && time.Now().Before(f.until)
}

// relayFail counts a failed relay authentication (a security event) and blocks the address beyond the limit.
func (m *Manager) relayFail(ip, why string) {
	set := m.Settings() // before guard.mu: the manager's lock is never taken under it
	g := &m.guard
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.fails == nil {
		g.fails = map[string]*failRec{}
	}
	f := g.fails[ip]
	if f == nil || time.Since(f.first) > time.Duration(set.PushWindowMinutes)*time.Minute {
		f = &failRec{first: time.Now()}
		g.fails[ip] = f
	}
	f.n++
	if f.n >= set.PushMaxFails {
		f.until = time.Now().Add(time.Duration(set.PushBlockMinutes) * time.Minute)
	}
	m.log.Warn("camera relay refused", "security", true, "remote", ip, "reason", why, "failures", f.n)
}

var pushPath = regexp.MustCompile(`^/?([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(/sub)?/?$`)

// StartPush opens the listener; nil when not configured.
func (m *Manager) StartPush(cfg PushConfig) (*PushServer, error) {
	if cfg.Listen == "" {
		return nil, nil
	}
	ps := &PushServer{m: m, log: m.log.With("part", "push"), sessions: map[*gortsplib.ServerSession]*pushConn{}, targets: map[string]*gortsplib.ServerSession{}}
	ps.srv = &gortsplib.Server{Handler: ps, RTSPAddress: cfg.Listen, ReadTimeout: 15 * time.Second, WriteTimeout: 10 * time.Second}
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return nil, err
		}
		ps.srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
		ps.srv.AuthMethods = []auth.VerifyMethod{auth.VerifyMethodBasic}
		ps.tls = true
	} else {
		ps.srv.AuthMethods = []auth.VerifyMethod{auth.VerifyMethodDigestMD5}
	}
	if err := ps.srv.Start(); err != nil {
		return nil, err
	}
	m.log.Info("camera push listener", "addr", cfg.Listen, "tls", ps.tls)
	m.mu.Lock()
	m.push = ps
	m.mu.Unlock()
	return ps, nil
}

// PushURL is the address relays publish to (rtsps://host:port), "" when the listener is off.
func (m *Manager) PushURL() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.push == nil {
		return ""
	}
	return m.pushURL
}

// SetPushHost sets the host name relays use to reach the server (from the configuration).
func (m *Manager) SetPushHost(host, listen string, tlsOn bool) {
	_, port, _ := net.SplitHostPort(listen)
	scheme := "rtsp"
	if tlsOn {
		scheme = "rtsps"
	}
	m.mu.Lock()
	m.pushURL = scheme + "://" + net.JoinHostPort(host, port)
	m.mu.Unlock()
}

// TLS reports whether publishers connect over RTSPS.
func (ps *PushServer) TLS() bool { return ps != nil && ps.tls }

// Close stops the listener.
func (ps *PushServer) Close() {
	if ps != nil {
		ps.srv.Close()
	}
}

func ipOf(c *gortsplib.ServerConn) string {
	if a, ok := c.NetConn().RemoteAddr().(*net.TCPAddr); ok {
		return a.IP.String()
	}
	return ""
}

func (ps *PushServer) blocked(ip string) bool { return ps.m.relayBlocked(ip) }

func (ps *PushServer) fail(ip, why string) { ps.m.relayFail(ip, why) }

// OnConnOpen implements gortsplib.ServerHandlerOnConnOpen: addresses with too many failed attempts are cut off.
func (ps *PushServer) OnConnOpen(ctx *gortsplib.ServerHandlerOnConnOpenCtx) {
	if ps.blocked(ipOf(ctx.Conn)) {
		ctx.Conn.Close()
	}
}

// OnDescribe implements gortsplib.ServerHandlerOnDescribe: nobody reads from this listener.
func (ps *PushServer) OnDescribe(*gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
	return &base.Response{StatusCode: base.StatusForbidden}, nil, errors.New("reading is not allowed")
}

// OnAnnounce implements gortsplib.ServerHandlerOnAnnounce: the publisher names the camera and proves its secret.
func (ps *PushServer) OnAnnounce(ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	ip := ipOf(ctx.Conn)
	m := pushPath.FindStringSubmatch(ctx.Path)
	if m == nil {
		ps.fail(ip, "unknown path")
		return &base.Response{StatusCode: base.StatusNotFound}, liberrors.ErrServerAuth{}
	}
	camID, sub := m[1], m[2] != ""
	src, secret := ps.m.pushTarget(camID, sub)
	if src == nil || secret == "" || !ctx.Conn.VerifyCredentials(ctx.Request, camID, secret) {
		if _, hasAuth := ctx.Request.Header["Authorization"]; hasAuth {
			ps.fail(ip, "wrong camera or secret")
		}
		return &base.Response{StatusCode: base.StatusUnauthorized}, liberrors.ErrServerAuth{}
	}
	key := camID + strings.Repeat("/sub", btoi(sub))
	pc := &pushConn{sess: ctx.Session, desc: ctx.Description, reply: make(chan error, 1), done: make(chan struct{})}
	ps.mu.Lock()
	old := ps.targets[key]
	ps.targets[key] = ctx.Session
	ps.sessions[ctx.Session] = pc
	ps.mu.Unlock()
	if old != nil {
		old.Close() // a reconnecting relay replaces its stale session
	}
	ps.log.Info("camera publishing", "camera", camID, "sub", sub, "remote", ip)
	return &base.Response{StatusCode: base.StatusOK}, nil
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// OnSetup implements gortsplib.ServerHandlerOnSetup (publishers only).
func (ps *PushServer) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	if ctx.Session.State() != gortsplib.ServerSessionStatePreRecord {
		return &base.Response{StatusCode: base.StatusForbidden}, nil, errors.New("reading is not allowed")
	}
	return &base.Response{StatusCode: base.StatusOK}, nil, nil
}

// OnRecord implements gortsplib.ServerHandlerOnRecord: the stream goes to the camera's Source.
func (ps *PushServer) OnRecord(ctx *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	ps.mu.Lock()
	pc := ps.sessions[ctx.Session]
	ps.mu.Unlock()
	if pc == nil {
		return &base.Response{StatusCode: base.StatusBadRequest}, errors.New("no announce")
	}
	m := pushPath.FindStringSubmatch(ctx.Path)
	if m == nil {
		return &base.Response{StatusCode: base.StatusNotFound}, errors.New("unknown path")
	}
	src, _ := ps.m.pushTarget(m[1], m[2] != "")
	if src == nil {
		return &base.Response{StatusCode: base.StatusNotFound}, errors.New("camera removed")
	}
	if err := src.attach(pc); err != nil {
		return &base.Response{StatusCode: base.StatusBadRequest}, err
	}
	return &base.Response{StatusCode: base.StatusOK}, nil
}

// OnSessionClose implements gortsplib.ServerHandlerOnSessionClose.
func (ps *PushServer) OnSessionClose(ctx *gortsplib.ServerHandlerOnSessionCloseCtx) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if pc, ok := ps.sessions[ctx.Session]; ok {
		close(pc.done)
		delete(ps.sessions, ctx.Session)
	}
	for k, s := range ps.targets {
		if s == ctx.Session {
			delete(ps.targets, k)
		}
	}
}

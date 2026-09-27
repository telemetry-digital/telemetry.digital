package replication

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// UpgradeProto is the protocol name of the replication tunnel (research R2): an HTTPS request to
// /api/v1/replication/tunnel is upgraded to a raw byte stream connected to the primary's PostgreSQL.
const UpgradeProto = "ctrl32-pgrepl"

// ServeTunnel upgrades an authenticated request and pipes it to the connection dial returns. The caller checks
// authentication first; ServeTunnel only checks the upgrade headers.
func ServeTunnel(w http.ResponseWriter, r *http.Request, dial func(ctx context.Context) (net.Conn, error), log *slog.Logger) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), UpgradeProto) || !headerHas(r.Header, "Connection", "upgrade") {
		http.Error(w, "upgrade to "+UpgradeProto+" required", http.StatusUpgradeRequired)
		return
	}
	rc := http.NewResponseController(w) // unwraps middleware writers (Unwrap) down to the connection
	dctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	pg, err := dial(dctx)
	cancel()
	if err != nil {
		log.Warn("replication tunnel: database not reachable", "err", err)
		http.Error(w, "database not reachable", http.StatusBadGateway)
		return
	}
	conn, buf, err := rc.Hijack()
	if err != nil {
		pg.Close()
		http.Error(w, "connection cannot be upgraded", http.StatusInternalServerError)
		return
	}
	_ = conn.SetDeadline(time.Time{}) // the HTTP server's read/write timeouts do not apply to the stream
	if _, err := io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: "+UpgradeProto+"\r\n\r\n"); err != nil {
		conn.Close()
		pg.Close()
		return
	}
	var client io.Reader = conn
	if buf != nil && buf.Reader.Buffered() > 0 {
		client = io.MultiReader(io.LimitReader(buf.Reader, int64(buf.Reader.Buffered())), conn)
	}
	pipe(conn, client, pg, pg)
}

func headerHas(h http.Header, key, token string) bool {
	for _, v := range h.Values(key) {
		for _, p := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(p), token) {
				return true
			}
		}
	}
	return false
}

// pipe copies both ways until either side closes, then closes both.
func pipe(a io.WriteCloser, ar io.Reader, b io.WriteCloser, br io.Reader) {
	var once sync.Once
	closeAll := func() { a.Close(); b.Close() }
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(b, ar); once.Do(closeAll) }()
	go func() { defer wg.Done(); _, _ = io.Copy(a, br); once.Do(closeAll) }()
	wg.Wait()
}

// TunnelClient is the replica end: every connection accepted on the local listener (pg_basebackup, the standby's
// WAL receiver) becomes one upgraded HTTPS connection to the primary.
type TunnelClient struct {
	Primary string // base URL of the primary, e.g. https://telemetry.digital
	Token   string
	TLS     *tls.Config // nil = system roots
	Log     *slog.Logger
	// OnContact is called after every successful upgrade (last contact).
	OnContact func()
}

// Dial opens one tunnel to the primary.
func (c *TunnelClient) Dial(ctx context.Context) (net.Conn, error) {
	u, err := url.Parse(strings.TrimRight(c.Primary, "/") + "/api/v1/replication/tunnel")
	if err != nil {
		return nil, err
	}
	host := u.Host
	if u.Port() == "" {
		if u.Scheme == "https" {
			host += ":443"
		} else {
			host += ":80"
		}
	}
	d := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	var conn net.Conn
	switch u.Scheme {
	case "https":
		cfg := c.TLS
		if cfg == nil {
			cfg = &tls.Config{MinVersion: tls.VersionTLS12}
		}
		cfg = cfg.Clone()
		if cfg.ServerName == "" {
			cfg.ServerName = u.Hostname()
		}
		conn, err = (&tls.Dialer{NetDialer: d, Config: cfg}).DialContext(ctx, "tcp", host)
	case "http":
		conn, err = d.DialContext(ctx, "tcp", host)
	default:
		return nil, fmt.Errorf("primary URL must be https:// or http://")
	}
	if err != nil {
		return nil, err
	}
	req := "GET " + u.RequestURI() + " HTTP/1.1\r\nHost: " + u.Host + "\r\nConnection: Upgrade\r\nUpgrade: " + UpgradeProto +
		"\r\nAuthorization: Bearer " + c.Token + "\r\nUser-Agent: ctrl32-replica\r\n\r\n"
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	if _, err := io.WriteString(conn, req); err != nil {
		conn.Close()
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		conn.Close()
		return nil, &TunnelError{Status: resp.StatusCode, Msg: strings.TrimSpace(string(body))}
	}
	_ = conn.SetDeadline(time.Time{})
	if c.OnContact != nil {
		c.OnContact()
	}
	if br.Buffered() > 0 { // bytes that arrived with the 101 belong to the stream
		return &bufConn{Conn: conn, r: io.MultiReader(io.LimitReader(br, int64(br.Buffered())), conn)}, nil
	}
	return conn, nil
}

// TunnelError is a refusal by the primary (for example 401: the token was revoked after a detach).
type TunnelError struct {
	Status int
	Msg    string
}

func (e *TunnelError) Error() string {
	return fmt.Sprintf("primary refused the tunnel: %d %s", e.Status, e.Msg)
}

type bufConn struct {
	net.Conn
	r io.Reader
}

func (b *bufConn) Read(p []byte) (int, error) { return b.r.Read(p) }

// Serve accepts local connections on ln until ctx ends.
func (c *TunnelClient) Serve(ctx context.Context, ln net.Listener) error {
	go func() { <-ctx.Done(); ln.Close() }()
	for {
		local, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}
		go func() {
			remote, err := c.Dial(ctx)
			if err != nil {
				if c.Log != nil {
					c.Log.Warn("replication tunnel: primary not reachable", "err", err)
				}
				local.Close()
				return
			}
			pipe(local, local, remote, remote)
		}()
	}
}

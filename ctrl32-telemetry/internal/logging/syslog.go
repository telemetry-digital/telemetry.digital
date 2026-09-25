package logging

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// syslogState is shared by a syslog handler and its WithAttrs/WithGroup clones: one connection, one buffer.
type syslogState struct {
	mu        sync.Mutex
	network   string
	addr      string
	conn      net.Conn
	lastTry   time.Time
	host, app string
	pid       int
	buf       bytes.Buffer
}

// syslogHandler forwards every record as an RFC 5424 message whose MSG part is the same JSON line the file log gets
// (URS I-07: export of the system log to syslog). Delivery is best effort: a lost collector never blocks the server.
type syslogHandler struct {
	st    *syslogState
	inner slog.Handler // JSON handler writing into st.buf
}

// newSyslog parses "udp://host:514" or "tcp://host:514" (default udp, port 514).
func newSyslog(target string, level slog.Level) (*syslogHandler, error) {
	if !strings.Contains(target, "://") {
		target = "udp://" + target
	}
	u, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("log.syslog: %w", err)
	}
	network := u.Scheme
	if network != "udp" && network != "tcp" {
		return nil, fmt.Errorf("log.syslog: scheme must be udp or tcp, got %q", network)
	}
	addr := u.Host
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "514")
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "-"
	}
	st := &syslogState{network: network, addr: addr, host: host, app: "ctrl32-telemetry", pid: os.Getpid()}
	h := &syslogHandler{st: st, inner: slog.NewJSONHandler(&st.buf, &slog.HandlerOptions{Level: level})}
	return h, nil
}

func (h *syslogHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *syslogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &syslogHandler{st: h.st, inner: h.inner.WithAttrs(attrs)}
}

func (h *syslogHandler) WithGroup(name string) slog.Handler {
	return &syslogHandler{st: h.st, inner: h.inner.WithGroup(name)}
}

func (h *syslogHandler) Handle(ctx context.Context, r slog.Record) error {
	st := h.st
	st.mu.Lock()
	defer st.mu.Unlock()
	st.buf.Reset()
	if err := h.inner.Handle(ctx, r); err != nil {
		return err
	}
	msg := bytes.TrimRight(st.buf.Bytes(), "\n")
	sev := 6 // informational
	switch {
	case r.Level >= slog.LevelError:
		sev = 3
	case r.Level >= slog.LevelWarn:
		sev = 4
	case r.Level < slog.LevelInfo:
		sev = 7
	}
	pri := 16*8 + sev // facility local0
	line := fmt.Sprintf("<%d>1 %s %s %s %d - - %s\n", pri, r.Time.UTC().Format(time.RFC3339Nano), st.host, st.app, st.pid, msg)
	if st.conn == nil {
		if time.Since(st.lastTry) < 10*time.Second {
			return nil // collector down; try again later, never block logging
		}
		st.lastTry = time.Now()
		c, err := net.DialTimeout(st.network, st.addr, 2*time.Second)
		if err != nil {
			return nil
		}
		st.conn = c
	}
	_ = st.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := st.conn.Write([]byte(line)); err != nil {
		st.conn.Close()
		st.conn = nil
	}
	return nil
}

// multiHandler fans a record out to several handlers.
type multiHandler []slog.Handler

func (m multiHandler) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range m {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (m multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var first error
	for _, h := range m {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make(multiHandler, len(m))
	for i, h := range m {
		out[i] = h.WithAttrs(attrs)
	}
	return out
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	out := make(multiHandler, len(m))
	for i, h := range m {
		out[i] = h.WithGroup(name)
	}
	return out
}

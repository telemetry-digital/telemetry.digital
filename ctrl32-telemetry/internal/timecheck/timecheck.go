// Package timecheck verifies that the server clock is synchronized (URS B-06): an SNTP query (RFC 4330) against the
// configured NTP server at start and periodically, plus the operating system's own view (timedatectl / w32tm). The
// state is shown on the Server page, exposed in /healthz and /metrics, and an unsynchronized clock is logged as a
// warning — records carry server timestamps, so a wrong clock is a data-integrity problem, not a cosmetic one.
package timecheck

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/metrics"
)

// Config selects the server and the tolerance.
type Config struct {
	Server    string        // host or host:port; empty = disabled
	Interval  time.Duration // between checks; default 15 min
	MaxOffset time.Duration // |offset| above this = not synchronized; default 1 s
}

// State is the result of the last check.
type State struct {
	Server    string    `json:"server"`
	Enabled   bool      `json:"enabled"`
	CheckedAt time.Time `json:"checked_at"`
	OffsetMs  float64   `json:"offset_ms"`
	RTTMs     float64   `json:"rtt_ms"`
	Stratum   int       `json:"stratum"`
	OK        bool      `json:"ok"`      // server reachable and offset within the limit
	Error     string    `json:"error"`   // why not
	OSSync    string    `json:"os_sync"` // yes, no, unknown — what the operating system reports
	MaxOffset string    `json:"max_offset"`
}

// Checker runs the periodic check.
type Checker struct {
	cfg Config
	log *slog.Logger
	mu  sync.Mutex
	st  State
}

// New prepares a checker; Start runs it.
func New(cfg Config, log *slog.Logger) *Checker {
	if cfg.Interval <= 0 {
		cfg.Interval = 15 * time.Minute
	}
	if cfg.MaxOffset <= 0 {
		cfg.MaxOffset = time.Second
	}
	return &Checker{cfg: cfg, log: log, st: State{Server: cfg.Server, Enabled: cfg.Server != "", OSSync: "unknown", MaxOffset: cfg.MaxOffset.String()}}
}

// State returns the last result.
func (c *Checker) State() State {
	if c == nil {
		return State{OSSync: "unknown"}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st
}

// Start checks immediately and then every interval until the context ends.
func (c *Checker) Start(ctx context.Context) {
	if c == nil || c.cfg.Server == "" {
		return
	}
	go func() {
		c.Check(ctx)
		t := time.NewTicker(c.cfg.Interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.Check(ctx)
			}
		}
	}()
}

// Check performs one query and records the result.
func (c *Checker) Check(ctx context.Context) State {
	st := State{Server: c.cfg.Server, Enabled: true, CheckedAt: time.Now().UTC(), OSSync: osSync(ctx), MaxOffset: c.cfg.MaxOffset.String()}
	// UDP is lossy and public pools rate-limit: three attempts before the clock counts as unverified.
	var offset, rtt time.Duration
	var stratum int
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if offset, rtt, stratum, err = Query(ctx, c.cfg.Server); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			attempt = 3
		case <-time.After(time.Duration(attempt+1) * 2 * time.Second):
		}
	}
	if err != nil {
		st.Error = err.Error()
	} else {
		st.OffsetMs = float64(offset) / float64(time.Millisecond)
		st.RTTMs = float64(rtt) / float64(time.Millisecond)
		st.Stratum = stratum
		if offset < 0 {
			offset = -offset
		}
		st.OK = offset <= c.cfg.MaxOffset
		if !st.OK {
			st.Error = fmt.Sprintf("clock offset %.0f ms exceeds %s", st.OffsetMs, c.cfg.MaxOffset)
		}
	}
	c.mu.Lock()
	prev := c.st
	c.st = st
	c.mu.Unlock()
	metrics.NTPOffset.Set(st.OffsetMs / 1000)
	metrics.NTPCheckedAt.Set(float64(st.CheckedAt.Unix()))
	if st.OK {
		metrics.NTPSynced.Set(1)
		if !prev.OK && c.log != nil {
			c.log.Info("clock synchronized", "ntp_server", st.Server, "offset_ms", fmt.Sprintf("%.1f", st.OffsetMs), "os_sync", st.OSSync)
		}
	} else {
		metrics.NTPSynced.Set(0)
		if c.log != nil {
			c.log.Warn("clock not verified against NTP", "security", true, "ntp_server", st.Server, "err", st.Error, "os_sync", st.OSSync)
		}
	}
	return st
}

const ntpEpochOffset = 2208988800 // seconds between 1900-01-01 and 1970-01-01

// Query sends one SNTP request and returns the clock offset (server − local), the round-trip time and the stratum.
func Query(ctx context.Context, server string) (offset, rtt time.Duration, stratum int, err error) {
	addr := server
	if _, _, e := net.SplitHostPort(addr); e != nil {
		addr = net.JoinHostPort(addr, "123")
	}
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "udp", addr)
	if err != nil {
		return 0, 0, 0, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	req := make([]byte, 48)
	req[0] = 0x23 // LI 0, version 4, mode 3 (client)
	t1 := time.Now()
	putNTP(req[40:], t1)
	if _, err := conn.Write(req); err != nil {
		return 0, 0, 0, err
	}
	resp := make([]byte, 48)
	n, err := conn.Read(resp)
	t4 := time.Now()
	if err != nil {
		return 0, 0, 0, err
	}
	if n < 48 {
		return 0, 0, 0, errors.New("short NTP response")
	}
	mode := resp[0] & 0x7
	stratum = int(resp[1])
	if mode != 4 && mode != 5 {
		return 0, 0, 0, fmt.Errorf("unexpected NTP mode %d", mode)
	}
	if stratum == 0 {
		return 0, 0, 0, fmt.Errorf("NTP server refused (kiss-o'-death %q)", strings.TrimRight(string(resp[12:16]), "\x00"))
	}
	if binary.BigEndian.Uint64(resp[24:32]) != binary.BigEndian.Uint64(req[40:48]) {
		return 0, 0, 0, errors.New("NTP response does not match the request")
	}
	t2 := getNTP(resp[32:])
	t3 := getNTP(resp[40:])
	offset = (t2.Sub(t1) + t3.Sub(t4)) / 2
	rtt = t4.Sub(t1) - t3.Sub(t2)
	return offset, rtt, stratum, nil
}

func putNTP(b []byte, t time.Time) {
	secs := uint64(t.Unix()) + ntpEpochOffset
	frac := uint64(float64(t.Nanosecond()) / 1e9 * (1 << 32))
	binary.BigEndian.PutUint64(b, secs<<32|frac)
}

func getNTP(b []byte) time.Time {
	v := binary.BigEndian.Uint64(b)
	secs := int64(v>>32) - ntpEpochOffset
	nanos := int64(float64(v&0xffffffff) / (1 << 32) * 1e9)
	return time.Unix(secs, nanos)
}

// osSync asks the operating system whether it considers the clock synchronized ("yes", "no" or "unknown").
func osSync(ctx context.Context) string {
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	switch runtime.GOOS {
	case "linux":
		out, err := exec.CommandContext(cctx, "timedatectl", "show", "-p", "NTPSynchronized", "--value").Output()
		if err != nil {
			return "unknown"
		}
		switch strings.TrimSpace(string(out)) {
		case "yes":
			return "yes"
		case "no":
			return "no"
		}
	case "windows":
		out, err := exec.CommandContext(cctx, "w32tm", "/query", "/status").Output()
		if err != nil {
			return "unknown"
		}
		s := strings.ToLower(string(out))
		if strings.Contains(s, "local cmos clock") || strings.Contains(s, "free-running") {
			return "no"
		}
		if strings.Contains(s, "source:") {
			return "yes"
		}
	}
	return "unknown"
}

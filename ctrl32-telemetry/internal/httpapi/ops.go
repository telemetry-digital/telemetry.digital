package httpapi

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/metrics"
)

// securityTxt serves RFC 9116 security.txt (URS H-17): where to report a vulnerability in this installation.
// The operator's own contact comes from `[security] contact`; the product's advisory page is always listed.
func (s *Server) securityTxt(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	if c := strings.TrimSpace(s.cfg.Security.Contact); c != "" {
		fmt.Fprintf(&b, "Contact: %s\n", c)
	}
	fmt.Fprintf(&b, "Contact: %s/security/advisories/new\n", productGitHub)
	fmt.Fprintf(&b, "Expires: %s\n", time.Now().UTC().AddDate(1, 0, 0).Format(time.RFC3339))
	b.WriteString("Preferred-Languages: en, sk\n")
	if base := strings.TrimRight(s.cfg.HTTP.BaseURL, "/"); base != "" {
		fmt.Fprintf(&b, "Canonical: %s/.well-known/security.txt\n", base)
	}
	fmt.Fprintf(&b, "Policy: %s/blob/main/SECURITY.md\n", productGitHub)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write([]byte(b.String()))
}

func (s *Server) securityTxtRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/.well-known/security.txt", http.StatusMovedPermanently)
}

// countRequests counts responses by status class for /metrics.
func countRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cw := &countingWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(cw, r)
		metrics.HTTPRequests.Inc(fmt.Sprintf("%dxx", cw.status/100))
	})
}

type countingWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (c *countingWriter) WriteHeader(code int) {
	if !c.wrote {
		c.status = code
		c.wrote = true
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *countingWriter) Write(b []byte) (int, error) {
	c.wrote = true
	return c.ResponseWriter.Write(b)
}

// Flush keeps server-sent events streaming through the wrapper.
func (c *countingWriter) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (c *countingWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

// Hijack lets WebSockets (camera video) take over the connection.
func (c *countingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := c.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("connection cannot be taken over")
	}
	c.status, c.wrote = http.StatusSwitchingProtocols, true
	return h.Hijack()
}

// metricsHandler exposes operational numbers in the Prometheus text format (URS K-12). Counts only, never records.
func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	samples := metrics.Default.Snapshot()
	g := func(name, help string, v float64) {
		samples = append(samples, metrics.Sample{Name: name, Help: help, Type: "gauge", Value: v})
	}
	samples = append(samples, metrics.Sample{Name: "ctrl32_info", Help: "Server version and runtime", Type: "gauge", Value: 1,
		Labels: map[string]string{"version": s.version, "go": runtime.Version(), "os": runtime.GOOS}})
	g("ctrl32_process_start_time_seconds", "Unix time the server process started", float64(s.started.Unix()))
	g("ctrl32_process_uptime_seconds", "Seconds since the server process started", time.Since(s.started).Seconds())
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	g("ctrl32_go_goroutines", "Goroutines", float64(runtime.NumGoroutine()))
	g("ctrl32_go_memstats_heap_alloc_bytes", "Heap bytes in use", float64(ms.HeapAlloc))
	g("ctrl32_go_memstats_sys_bytes", "Bytes obtained from the operating system", float64(ms.Sys))
	if s.pool != nil {
		st := s.pool.Stat()
		g("ctrl32_db_pool_total_conns", "Database connections in the pool", float64(st.TotalConns()))
		g("ctrl32_db_pool_idle_conns", "Idle database connections", float64(st.IdleConns()))
		g("ctrl32_db_pool_acquired_conns", "Database connections in use", float64(st.AcquiredConns()))
		g("ctrl32_db_pool_max_conns", "Pool limit", float64(st.MaxConns()))
	}
	if s.br != nil && s.br.Server != nil {
		g("ctrl32_mqtt_clients", "Connected MQTT clients", float64(s.br.Server.Clients.Len()))
	}
	var active, acked, gaps, pending, failed24, cmds, devices, ds, dsInt int64
	var gapSec float64
	err := s.pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM core.alarm WHERE state = 'active'), (SELECT count(*) FROM core.alarm WHERE state = 'acknowledged'),
		(SELECT count(*) FROM core.gap WHERE closed_at IS NULL),
		(SELECT count(*) FROM core.notification WHERE status = 'scheduled'),
		(SELECT count(*) FROM core.notification WHERE status = 'failed' AND due_at > now() - interval '24 hours'),
		(SELECT count(*) FROM core.device_command WHERE status IN ('queued', 'sent')),
		(SELECT count(*) FROM core.device WHERE revoked_at IS NULL), (SELECT count(*) FROM core.datastream),
		(SELECT count(*) FROM core.datastream WHERE expected_interval IS NOT NULL),
		(SELECT COALESCE(SUM(EXTRACT(EPOCH FROM (LEAST(COALESCE(gap_to, now()), now()) - GREATEST(gap_from, now() - interval '24 hours')))), 0)
		   FROM core.gap WHERE COALESCE(gap_to, now()) > now() - interval '24 hours')`).
		Scan(&active, &acked, &gaps, &pending, &failed24, &cmds, &devices, &ds, &dsInt, &gapSec)
	if err != nil {
		s.log.Warn("metrics query failed", "err", err)
	} else {
		g("ctrl32_alarms_active", "Alarms in state active", float64(active))
		g("ctrl32_alarms_acknowledged", "Alarms acknowledged and not yet cleared", float64(acked))
		g("ctrl32_gaps_open", "Datastreams currently missing data against their expected interval", float64(gaps))
		g("ctrl32_notifications_pending", "Notifications scheduled and not yet sent", float64(pending))
		g("ctrl32_notifications_failed_24h", "Notification deliveries that failed in the last 24 hours", float64(failed24))
		g("ctrl32_commands_pending", "Device commands queued or sent without acknowledgement", float64(cmds))
		g("ctrl32_devices", "Devices not revoked", float64(devices))
		g("ctrl32_datastreams", "Datastreams", float64(ds))
		if dsInt > 0 {
			c := 1 - gapSec/(float64(dsInt)*86400)
			if c < 0 {
				c = 0
			}
			if c > 1 {
				c = 1
			}
			g("ctrl32_data_completeness_ratio_24h", "Share of the last 24 hours covered by data across datastreams with an expected interval (1 = no gaps)", c)
		}
	}
	if s.flows != nil {
		running, processed, errs, dropped := s.flows.Totals()
		c := func(name, help string, v int64) {
			samples = append(samples, metrics.Sample{Name: name, Help: help, Type: "counter", Value: float64(v)})
		}
		g("ctrl32_flows_running", "Flows deployed and running", float64(running))
		c("ctrl32_flow_messages_total", "Messages processed by flow nodes since start", processed)
		c("ctrl32_flow_errors_total", "Flow node errors since start", errs)
		c("ctrl32_flow_dropped_total", "Flow messages dropped (full queue, rate limit) since start", dropped)
	}
	g("ctrl32_incidents_open", "Incidents open or under investigation", float64(s.incidents.OpenCount(ctx, "")))
	ts := s.tc.State()
	if ts.Enabled {
		g("ctrl32_ntp_enabled", "1 when an NTP server is configured", 1)
	} else {
		g("ctrl32_ntp_enabled", "1 when an NTP server is configured", 0)
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	metrics.Format(w, samples)
}

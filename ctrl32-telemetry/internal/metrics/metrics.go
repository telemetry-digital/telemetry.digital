// Package metrics keeps the process counters that the /metrics endpoint exposes in the Prometheus text format
// (URS K-12). It is deliberately tiny: counters and summaries updated from the hot paths (ingest, alarms, HTTP),
// gauges set by the owner of a value, and a formatter. Values are operational numbers only — never records.
package metrics

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Sample is one exposition line.
type Sample struct {
	Name   string
	Help   string
	Type   string // counter, gauge, summary
	Labels map[string]string
	Value  float64
}

// CounterVec is a counter with one label dimension (or none).
type CounterVec struct {
	name, help, label string
	mu                sync.Mutex
	vals              map[string]*atomic.Uint64
}

func (c *CounterVec) cell(lv string) *atomic.Uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.vals[lv]
	if !ok {
		v = &atomic.Uint64{}
		c.vals[lv] = v
	}
	return v
}

// Inc adds one for the label value ("" when the counter has no label).
func (c *CounterVec) Inc(lv string) { c.cell(lv).Add(1) }

// Add adds n for the label value.
func (c *CounterVec) Add(lv string, n uint64) { c.cell(lv).Add(n) }

// Preset creates zero cells so the metric is exposed before the first event.
func (c *CounterVec) Preset(lvs ...string) *CounterVec {
	for _, lv := range lvs {
		c.cell(lv)
	}
	return c
}

// Value returns the current count for the label value.
func (c *CounterVec) Value(lv string) uint64 { return c.cell(lv).Load() }

// Summary tracks count, sum and max of observed durations or sizes.
type Summary struct {
	name, help string
	mu         sync.Mutex
	count      uint64
	sum, max   float64
}

// Observe records one value.
func (s *Summary) Observe(v float64) {
	s.mu.Lock()
	s.count++
	s.sum += v
	if v > s.max {
		s.max = v
	}
	s.mu.Unlock()
}

// Gauge is a value set by its owner.
type Gauge struct {
	name, help string
	bits       atomic.Uint64
}

// Set stores the value.
func (g *Gauge) Set(v float64) { g.bits.Store(math.Float64bits(v)) }

// Get returns the value.
func (g *Gauge) Get() float64 { return math.Float64frombits(g.bits.Load()) }

// Registry holds the process-wide instruments.
type Registry struct {
	mu        sync.Mutex
	counters  []*CounterVec
	summaries []*Summary
	gauges    []*Gauge
}

// Default is the registry the server exposes.
var Default = &Registry{}

// Counter registers a counter; label may be "" for an unlabelled counter.
func (r *Registry) Counter(name, help, label string) *CounterVec {
	c := &CounterVec{name: name, help: help, label: label, vals: map[string]*atomic.Uint64{}}
	if label == "" {
		c.vals[""] = &atomic.Uint64{}
	}
	r.mu.Lock()
	r.counters = append(r.counters, c)
	r.mu.Unlock()
	return c
}

// Summary registers a summary.
func (r *Registry) Summary(name, help string) *Summary {
	s := &Summary{name: name, help: help}
	r.mu.Lock()
	r.summaries = append(r.summaries, s)
	r.mu.Unlock()
	return s
}

// Gauge registers a gauge.
func (r *Registry) Gauge(name, help string) *Gauge {
	g := &Gauge{name: name, help: help}
	r.mu.Lock()
	r.gauges = append(r.gauges, g)
	r.mu.Unlock()
	return g
}

// Snapshot returns the registered instruments as samples.
func (r *Registry) Snapshot() []Sample {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Sample
	for _, c := range r.counters {
		c.mu.Lock()
		keys := make([]string, 0, len(c.vals))
		for k := range c.vals {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			s := Sample{Name: c.name, Help: c.help, Type: "counter", Value: float64(c.vals[k].Load())}
			if c.label != "" {
				s.Labels = map[string]string{c.label: k}
			}
			out = append(out, s)
		}
		c.mu.Unlock()
	}
	for _, s := range r.summaries {
		s.mu.Lock()
		out = append(out,
			Sample{Name: s.name + "_count", Help: s.help, Type: "summary", Value: float64(s.count)},
			Sample{Name: s.name + "_sum", Help: s.help, Type: "summary", Value: s.sum},
			Sample{Name: s.name + "_max", Help: s.help + " (maximum since start)", Type: "gauge", Value: s.max})
		s.mu.Unlock()
	}
	for _, g := range r.gauges {
		out = append(out, Sample{Name: g.name, Help: g.help, Type: "gauge", Value: g.Get()})
	}
	return out
}

// Format writes samples in the Prometheus text exposition format (version 0.0.4), one HELP/TYPE per metric name.
func Format(w io.Writer, samples []Sample) {
	seen := map[string]bool{}
	for _, s := range samples {
		if !seen[s.Name] {
			seen[s.Name] = true
			fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", s.Name, escapeHelp(s.Help), s.Name, typeOf(s))
		}
		if len(s.Labels) == 0 {
			fmt.Fprintf(w, "%s %s\n", s.Name, num(s.Value))
			continue
		}
		keys := make([]string, 0, len(s.Labels))
		for k := range s.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf(`%s="%s"`, k, escapeLabel(s.Labels[k])))
		}
		fmt.Fprintf(w, "%s{%s} %s\n", s.Name, strings.Join(parts, ","), num(s.Value))
	}
}

func typeOf(s Sample) string {
	if s.Type == "summary" {
		return "counter" // _count and _sum are exposed as plain counters; simpler for scrapers than a summary block
	}
	return s.Type
}

func num(v float64) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	case v == math.Trunc(v) && math.Abs(v) < 1e15:
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%g", v)
}

func escapeHelp(s string) string { return strings.NewReplacer(`\`, `\\`, "\n", `\n`).Replace(s) }
func escapeLabel(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s)
}

// Instruments updated by the hot paths.
var (
	IngestMessages = Default.Counter("ctrl32_ingest_messages_total", "Device messages received by outcome (ok, error, unknown_key)", "status").Preset("ok", "error", "unknown_key")
	Observations   = Default.Counter("ctrl32_observations_total", "Observations stored", "")
	AlarmsRaised   = Default.Counter("ctrl32_alarms_raised_total", "Alarms raised by severity", "severity").Preset("warning", "critical")
	AlarmLatency   = Default.Summary("ctrl32_alarm_latency_seconds", "Seconds from message receipt to the alarm being raised")
	HTTPRequests   = Default.Counter("ctrl32_http_requests_total", "HTTP requests by status class", "class").Preset("2xx", "3xx", "4xx", "5xx")
	AuditChainOK   = Default.Gauge("ctrl32_audit_chain_ok", "1 when the audit hash chain verified at the last check, 0 when broken")
	NTPOffset      = Default.Gauge("ctrl32_ntp_offset_seconds", "Clock offset against the configured NTP server at the last check")
	NTPSynced      = Default.Gauge("ctrl32_ntp_synced", "1 when the NTP server was reachable and the offset within the limit, 0 otherwise")
	NTPCheckedAt   = Default.Gauge("ctrl32_ntp_checked_timestamp_seconds", "Unix time of the last NTP check (0 = never)")
)

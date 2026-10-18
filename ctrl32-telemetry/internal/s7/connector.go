package s7

// S7 connectors (connector kind "s7", 0.83.0): the server reads tags of a Siemens PLC every interval and feeds them
// through the normal telemetry path of one or more virtual devices (transport s7), so datastream assignment,
// alarms, gap detection, reports, dashboards and live streams work as for MQTT devices. A write is always a command
// with identity, reason, ack and audit (ADR-0008): the tag must be marked writable, the value must be within the
// configured limits, and the value is read back after writing; the platform never sits in a control loop.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/ingest"
)

var (
	keyRe   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	extIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

// MaxTags is the number of tags one connector may read.
const MaxTags = 2000

// Tag maps one PLC address to a datastream key.
type Tag struct {
	Key      string   `json:"key"`
	Address  string   `json:"address"`        // DB1.DBD0, M10.0, IW64, DB1,REAL4, VW100, ...
	Type     string   `json:"type,omitempty"` // bool | byte | char | sint | usint | int | uint | word | dint | udint | dword | real | lreal | string; empty = implied by the address
	Scale    *float64 `json:"scale,omitempty"`
	Offset   *float64 `json:"offset,omitempty"`
	Writable bool     `json:"writable,omitempty"` // not for inputs (I/E)
	Min      *float64 `json:"min,omitempty"`      // limits for writes (scaled value)
	Max      *float64 `json:"max,omitempty"`
	// Device optionally routes the value to another virtual device than the connector's own; keys are unique per
	// device.
	Device string `json:"device,omitempty"`

	addr Address
}

// Settings is the connector configuration stored in core.connector.settings.
type Settings struct {
	Host string `json:"host"`
	Port int    `json:"port"` // default 102
	// PLC family; it sets the default rack/slot and TSAPs: s7-300 (0/2), s7-400 (0/3), s7-1200 (0/1), s7-1500 (0/1),
	// et200sp (0/1), logo (TSAP 01.00 → 02.00), other (rack/slot as given).
	PLC              string `json:"plc"`
	Rack             *int   `json:"rack,omitempty"`
	Slot             *int   `json:"slot,omitempty"`
	ConnType         string `json:"conn_type,omitempty"` // pg (default) | op | s7basic
	LocalTSAP        string `json:"local_tsap,omitempty"`
	RemoteTSAP       string `json:"remote_tsap,omitempty"`
	IntervalS        int    `json:"interval_s"`
	TimeoutMs        int    `json:"timeout_ms"`
	DeviceExternalID string `json:"device_external_id"`
	Tags             []Tag  `json:"tags"`
}

// Status is what the UI shows per connector.
type Status struct {
	State      string         `json:"state"` // connecting | connected | error | stopped
	LastError  string         `json:"last_error,omitempty"`
	LastPoll   time.Time      `json:"last_poll,omitempty"`
	LastValues map[string]any `json:"last_values,omitempty"`
	BadNodes   []string       `json:"bad_nodes,omitempty"`
	Polls      int64          `json:"polls"`
	Errors     int64          `json:"errors"`
	Writes     int64          `json:"writes"`
	Since      time.Time      `json:"since,omitempty"`
	PDU        int            `json:"pdu,omitempty"`
	CPU        *Info          `json:"cpu,omitempty"`
}

var plcSlots = map[string][2]int{"s7-300": {0, 2}, "s7-400": {0, 3}, "s7-1200": {0, 1}, "s7-1500": {0, 1}, "et200sp": {0, 1}}

// ValidateSettings parses and normalizes raw settings.
func ValidateSettings(raw json.RawMessage) (Settings, error) {
	var st Settings
	if len(raw) == 0 {
		return st, errors.New("settings required")
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return st, fmt.Errorf("settings: %w", err)
	}
	st.Host = strings.TrimSpace(st.Host)
	if st.Host == "" || strings.ContainsAny(st.Host, " /:") {
		return st, errors.New("host must be a host name or IP address")
	}
	if st.Port == 0 {
		st.Port = 102
	}
	if st.Port < 1 || st.Port > 65535 {
		return st, errors.New("port must be 1..65535")
	}
	st.PLC = strings.ToLower(strings.TrimSpace(st.PLC))
	if st.PLC == "" {
		st.PLC = "s7-1200"
	}
	if def, ok := plcSlots[st.PLC]; ok {
		if st.Rack == nil {
			st.Rack = &def[0]
		}
		if st.Slot == nil {
			st.Slot = &def[1]
		}
	} else if st.PLC == "logo" {
		if st.LocalTSAP == "" {
			st.LocalTSAP = "01.00"
		}
		if st.RemoteTSAP == "" {
			st.RemoteTSAP = "02.00"
		}
	} else if st.PLC != "other" {
		return st, errors.New("plc must be s7-300, s7-400, s7-1200, s7-1500, et200sp, logo or other")
	}
	zero := 0
	if st.Rack == nil {
		st.Rack = &zero
	}
	if st.Slot == nil {
		st.Slot = &zero
	}
	if *st.Rack < 0 || *st.Rack > 7 {
		return st, errors.New("rack must be 0..7")
	}
	if *st.Slot < 0 || *st.Slot > 31 {
		return st, errors.New("slot must be 0..31")
	}
	switch st.ConnType {
	case "":
		st.ConnType = "pg"
	case "pg", "op", "s7basic":
	default:
		return st, errors.New("connection type must be pg, op or s7basic")
	}
	st.LocalTSAP, st.RemoteTSAP = strings.TrimSpace(st.LocalTSAP), strings.TrimSpace(st.RemoteTSAP)
	if st.LocalTSAP != "" {
		if _, err := ParseTSAP(st.LocalTSAP); err != nil {
			return st, fmt.Errorf("local TSAP: %w", err)
		}
	}
	if st.RemoteTSAP != "" {
		if _, err := ParseTSAP(st.RemoteTSAP); err != nil {
			return st, fmt.Errorf("remote TSAP: %w", err)
		}
	}
	if st.IntervalS == 0 {
		st.IntervalS = 10
	}
	if st.IntervalS < 1 || st.IntervalS > 3600 {
		return st, errors.New("interval must be 1..3600 seconds")
	}
	if st.TimeoutMs == 0 {
		st.TimeoutMs = 3000
	}
	if st.TimeoutMs < 100 || st.TimeoutMs > 30000 {
		return st, errors.New("timeout must be 100..30000 ms")
	}
	st.DeviceExternalID = strings.TrimSpace(st.DeviceExternalID)
	if st.DeviceExternalID != "" && !extIDRe.MatchString(st.DeviceExternalID) {
		return st, errors.New("virtual device id may contain letters, digits and . _ : - (max 128)")
	}
	if len(st.Tags) == 0 {
		return st, errors.New("at least one tag is required")
	}
	if len(st.Tags) > MaxTags {
		return st, fmt.Errorf("at most %d tags per connector", MaxTags)
	}
	seen := map[string]bool{}
	for i := range st.Tags {
		t := &st.Tags[i]
		t.Key, t.Address, t.Type = strings.TrimSpace(t.Key), strings.TrimSpace(t.Address), strings.ToLower(strings.TrimSpace(t.Type))
		if !keyRe.MatchString(t.Key) {
			return st, fmt.Errorf("tag %d: key must match ^[a-z][a-z0-9_]{0,31}$", i+1)
		}
		a, err := ParseAddress(t.Address, t.Type)
		if err != nil {
			return st, fmt.Errorf("tag %d (%s): %w", i+1, t.Key, err)
		}
		t.addr, t.Type = a, a.Type
		t.Device = strings.TrimSpace(t.Device)
		if t.Device == st.DeviceExternalID {
			t.Device = ""
		}
		if t.Device != "" && !extIDRe.MatchString(t.Device) {
			return st, fmt.Errorf("tag %d: device id may contain letters, digits and . _ : - (max 128)", i+1)
		}
		if seen[t.Device+"/"+t.Key] {
			return st, fmt.Errorf("tag %d: duplicate key %q", i+1, t.Key)
		}
		seen[t.Device+"/"+t.Key] = true
		if t.Writable && a.Area == AreaI {
			return st, fmt.Errorf("tag %d (%s): inputs cannot be written", i+1, t.Key)
		}
		if t.Min != nil && t.Max != nil && *t.Min > *t.Max {
			return st, fmt.Errorf("tag %d: min must not exceed max", i+1)
		}
		if t.Scale != nil && *t.Scale == 0 {
			return st, fmt.Errorf("tag %d: scale must not be 0", i+1)
		}
	}
	return st, nil
}

func (st Settings) addr() string { return net.JoinHostPort(st.Host, strconv.Itoa(st.Port)) }

func (st Settings) dial() DialConfig {
	return DialConfig{Host: st.Host, Port: st.Port, Rack: *st.Rack, Slot: *st.Slot, ConnType: st.ConnType,
		LocalTSAP: st.LocalTSAP, RemoteTSAP: st.RemoteTSAP, Timeout: time.Duration(st.TimeoutMs) * time.Millisecond}
}

// ref is the key of a tag's value in read results: the key alone for the connector's own device, "device/key"
// for tags routed to another device.
func (st Settings) ref(t Tag) string {
	if t.Device == "" {
		return t.Key
	}
	return t.Device + "/" + t.Key
}

func (st Settings) tag(device, key string) *Tag {
	for i := range st.Tags {
		if st.Tags[i].Key == key && st.Tags[i].Device == device {
			return &st.Tags[i]
		}
	}
	return nil
}

func scaled(t Tag, v any) any {
	f, ok := v.(float64)
	if !ok {
		return v
	}
	if t.Scale != nil {
		f *= *t.Scale
	}
	if t.Offset != nil {
		f += *t.Offset
	}
	return f
}

func unscaled(t Tag, f float64) float64 {
	if t.Offset != nil {
		f -= *t.Offset
	}
	if t.Scale != nil && *t.Scale != 0 {
		f /= *t.Scale
	}
	return f
}

// readTags reads all tags once; values are keyed by ref, bad tags reported by key. A non-nil error means the
// whole request failed (connection, PUT/GET disabled, ...).
func readTags(ctx context.Context, c *Conn, st Settings) (map[string]any, []string, error) {
	addrs := make([]Address, len(st.Tags))
	for i, t := range st.Tags {
		addrs[i] = t.addr
	}
	res, err := c.Read(ctx, addrs)
	if err != nil {
		return nil, nil, err
	}
	values := map[string]any{}
	var bad []string
	for i, r := range res {
		t := st.Tags[i]
		if r.Err != nil {
			bad = append(bad, st.ref(t)+" ("+t.Address+"): "+r.Err.Error())
			continue
		}
		v := scaled(t, r.Value)
		if f, ok := v.(float64); ok && (math.IsNaN(f) || math.IsInf(f, 0)) {
			bad = append(bad, st.ref(t)+" ("+t.Address+"): not a finite number")
			continue
		}
		values[st.ref(t)] = v
	}
	return values, bad, nil
}

// Manager runs one poller per active s7 connector and executes write commands.
type Manager struct {
	pool    *pgxpool.Pool
	ing     *ingest.Ingest
	log     *slog.Logger
	mu      sync.Mutex
	workers map[string]*worker
	status  map[string]*Status
	resync  chan struct{}
	// sink replaces ingestion in unit tests (values by device: "" = the connector's own device).
	sink func(device string, values map[string]any)
}

func NewManager(pool *pgxpool.Pool, ing *ingest.Ingest, log *slog.Logger) *Manager {
	return &Manager{pool: pool, ing: ing, log: log.With("component", "s7"), workers: map[string]*worker{}, status: map[string]*Status{}, resync: make(chan struct{}, 1)}
}

// Start loads the connectors and keeps the set of workers in sync with the database.
func (m *Manager) Start(ctx context.Context) {
	go func() {
		m.sync(ctx)
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				m.mu.Lock()
				for _, w := range m.workers {
					w.cancel()
				}
				m.mu.Unlock()
				return
			case <-t.C:
				m.sync(ctx)
			case <-m.resync:
				m.sync(ctx)
			}
		}
	}()
}

// Resync asks the manager to reload connectors now (after create/update/revoke).
func (m *Manager) Resync() {
	select {
	case m.resync <- struct{}{}:
	default:
	}
}

// Status returns a copy of the runtime state of a connector (nil when it is not running).
func (m *Manager) Status(id string) *Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.status[id]
	if !ok {
		return nil
	}
	c := *st
	return &c
}

type connRow struct {
	ID, OrgID, Name string
	Settings        Settings
	Hash            string
}

func (m *Manager) sync(ctx context.Context) {
	rows, err := m.pool.Query(ctx, `SELECT id::text, org_id::text, name, settings::text FROM core.connector WHERE kind = 's7' AND revoked_at IS NULL`)
	if err != nil {
		m.log.Error("connector query failed", "err", err)
		return
	}
	want := map[string]connRow{}
	for rows.Next() {
		var r connRow
		var raw string
		if err := rows.Scan(&r.ID, &r.OrgID, &r.Name, &raw); err != nil {
			continue
		}
		st, err := ValidateSettings(json.RawMessage(raw))
		if err != nil {
			m.log.Warn("connector settings invalid, not started", "connector", r.Name, "err", err)
			continue
		}
		r.Settings, r.Hash = st, raw
		want[r.ID] = r
	}
	rows.Close()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, w := range m.workers {
		if row, ok := want[id]; !ok || row.Hash != w.hash {
			w.cancel()
			delete(m.workers, id)
			if !ok {
				delete(m.status, id)
			}
		}
	}
	for id, row := range want {
		if _, ok := m.workers[id]; ok {
			continue
		}
		m.startLocked(ctx, id, row)
	}
}

func (m *Manager) startLocked(ctx context.Context, id string, row connRow) *worker {
	wctx, cancel := context.WithCancel(ctx)
	st := &Status{State: "connecting", Since: time.Now()}
	m.status[id] = st
	w := &worker{m: m, row: row, hash: row.Hash, cancel: cancel, st: st}
	m.workers[id] = w
	go w.run(wctx)
	return w
}

// TestResult is the answer of a test read.
type TestResult struct {
	Values map[string]any `json:"values"`
	Bad    []string       `json:"bad_nodes"`
	PDU    int            `json:"pdu"`
	CPU    *Info          `json:"cpu,omitempty"`
}

// Test connects once, identifies the CPU (best effort), reads every tag and disconnects.
func (m *Manager) Test(ctx context.Context, st Settings) (*TestResult, error) {
	c, err := Dial(ctx, st.dial())
	if err != nil {
		return nil, err
	}
	defer c.Close()
	out := &TestResult{PDU: c.PDU()}
	if info, err := c.Identify(ctx); err == nil {
		out.CPU = &info
	}
	values, bad, err := readTags(ctx, c, st)
	if err != nil {
		return nil, err
	}
	out.Values, out.Bad = values, bad
	return out, nil
}

// ---- writes ---------------------------------------------------------------------------------------

// WriteOp is a validated write for a virtual device.
type WriteOp struct {
	ConnectorID, ConnectorName string
	Settings                   Settings
	Tag                        Tag
	Value                      any // float64, bool or string
}

// Prepare validates a command for a virtual device of an S7 connector without touching the network.
func (m *Manager) Prepare(ctx context.Context, deviceID, method string, args json.RawMessage) (*WriteOp, error) {
	if method != "write" {
		return nil, errors.New("S7 devices accept the method \"write\" with args {key, value}")
	}
	var a struct {
		Key   string `json:"key"`
		Value any    `json:"value"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, errors.New("args must be {key, value}")
	}
	w, ext := m.workerFor(ctx, deviceID)
	if w == nil {
		return nil, errors.New("no running S7 connector owns this device")
	}
	t := w.row.Settings.tag(ext, a.Key)
	if t == nil {
		return nil, fmt.Errorf("unknown key %q", a.Key)
	}
	if !t.Writable {
		return nil, fmt.Errorf("key %q is not writable", a.Key)
	}
	op := &WriteOp{ConnectorID: w.row.ID, ConnectorName: w.row.Name, Settings: w.row.Settings, Tag: *t}
	switch v := a.Value.(type) {
	case bool:
		if t.Type != "bool" {
			return nil, fmt.Errorf("key %q is %s; send a number", a.Key, t.Type)
		}
		op.Value = v
	case string:
		if t.Type != "string" && t.Type != "char" {
			return nil, fmt.Errorf("key %q is %s; send a %s", a.Key, t.Type, map[bool]string{true: "true/false", false: "number"}[t.Type == "bool"])
		}
		op.Value = v
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, errors.New("value must be a finite number")
		}
		if t.Type == "string" || t.Type == "char" {
			return nil, fmt.Errorf("key %q is %s; send text", a.Key, t.Type)
		}
		if t.Type == "bool" {
			op.Value = v != 0
			break
		}
		if t.Min != nil && v < *t.Min || t.Max != nil && v > *t.Max {
			return nil, fmt.Errorf("value %v is outside the limits %s..%s", v, fmtLimit(t.Min, "-∞"), fmtLimit(t.Max, "∞"))
		}
		op.Value = v
	default:
		return nil, errors.New("value must be a number, true/false or a string")
	}
	// Encode now so range errors (e.g. 40000 into an int) are refused before anything is recorded.
	if _, err := t.addr.Encode(rawValue(*t, op.Value)); err != nil {
		return nil, err
	}
	return op, nil
}

func rawValue(t Tag, v any) any {
	if f, ok := v.(float64); ok {
		return unscaled(t, f)
	}
	return v
}

func fmtLimit(p *float64, def string) string {
	if p == nil {
		return def
	}
	return strconv.FormatFloat(*p, 'f', -1, 64)
}

// workerFor finds the running connector that feeds the device and the device's key in its settings.
func (m *Manager) workerFor(ctx context.Context, deviceID string) (*worker, string) {
	m.mu.Lock()
	ws := make([]*worker, 0, len(m.workers))
	for _, w := range m.workers {
		ws = append(ws, w)
	}
	m.mu.Unlock()
	for _, w := range ws {
		exts := map[string]bool{"": true}
		for _, t := range w.row.Settings.Tags {
			exts[t.Device] = true
		}
		for ext := range exts {
			d, err := w.device(ctx, ext)
			if err == nil && d.ID == deviceID {
				return w, ext
			}
		}
	}
	return nil, ""
}

// Do writes the value over the connector's open connection (or a new one), reads it back and returns the result
// for the ack.
func (m *Manager) Do(ctx context.Context, op *WriteOp) (map[string]any, error) {
	m.mu.Lock()
	w := m.workers[op.ConnectorID]
	m.mu.Unlock()
	var c *Conn
	if w != nil {
		c = w.live()
	}
	if c == nil {
		var err error
		if c, err = Dial(ctx, op.Settings.dial()); err != nil {
			return nil, err
		}
		defer c.Close()
	}
	if err := c.Write(ctx, op.Tag.addr, rawValue(op.Tag, op.Value)); err != nil {
		return nil, fmt.Errorf("write %s (%s): %w", op.Tag.Key, op.Tag.Address, err)
	}
	res, err := c.Read(ctx, []Address{op.Tag.addr})
	if err != nil {
		return nil, fmt.Errorf("read back %s: %w", op.Tag.Key, err)
	}
	if len(res) != 1 || res[0].Err != nil {
		return nil, fmt.Errorf("read back %s failed", op.Tag.Key)
	}
	back := scaled(op.Tag, res[0].Value)
	m.mu.Lock()
	if st, ok := m.status[op.ConnectorID]; ok {
		st.Writes++
		if st.LastValues == nil {
			st.LastValues = map[string]any{}
		}
		st.LastValues[op.Settings.ref(op.Tag)] = back
	}
	m.mu.Unlock()
	m.log.Info("s7 write", "connector", op.ConnectorName, "key", op.Tag.Key, "address", op.Tag.Address, "value", op.Value, "readback", back)
	return map[string]any{"key": op.Tag.Key, "address": op.Tag.Address, "value": op.Value, "readback": back, "written_at": time.Now().UTC()}, nil
}

// ---- worker ---------------------------------------------------------------------------------------

type worker struct {
	m      *Manager
	row    connRow
	hash   string
	cancel context.CancelFunc
	st     *Status
	cmu    sync.Mutex
	conn   *Conn
	devs   map[string]*broker.Device
}

func (w *worker) set(fn func(s *Status)) {
	w.m.mu.Lock()
	fn(w.st)
	w.m.mu.Unlock()
}

func (w *worker) live() *Conn {
	w.cmu.Lock()
	defer w.cmu.Unlock()
	if w.conn != nil && w.conn.Usable() {
		return w.conn
	}
	return nil
}

func (w *worker) setConn(c *Conn) {
	w.cmu.Lock()
	w.conn = c
	w.cmu.Unlock()
}

func (w *worker) run(ctx context.Context) {
	backoff := 5 * time.Second
	st := w.row.Settings
	for ctx.Err() == nil {
		c, err := Dial(ctx, st.dial())
		if err != nil {
			w.set(func(s *Status) { s.State, s.LastError = "error", err.Error(); s.Errors++ })
			w.m.log.Warn("s7 connect failed", "connector", w.row.Name, "addr", st.addr(), "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < time.Minute {
				backoff *= 2
			}
			continue
		}
		backoff = 5 * time.Second
		var cpu *Info
		if info, err := c.Identify(ctx); err == nil {
			cpu = &info
		}
		w.set(func(s *Status) { s.State, s.LastError, s.PDU, s.CPU = "connected", "", c.PDU(), cpu })
		w.m.log.Info("s7 connected", "connector", w.row.Name, "addr", st.addr(), "tags", len(st.Tags), "pdu", c.PDU())
		w.setConn(c)
		t := time.NewTicker(time.Duration(st.IntervalS) * time.Second)
		ok := w.poll(ctx, c)
	loop:
		for ok {
			select {
			case <-ctx.Done():
				break loop
			case <-t.C:
				ok = w.poll(ctx, c)
			}
		}
		t.Stop()
		w.setConn(nil)
		_ = c.Close()
		if ctx.Err() != nil {
			w.set(func(s *Status) { s.State = "stopped" })
			return
		}
		select {
		case <-ctx.Done():
			w.set(func(s *Status) { s.State = "stopped" })
			return
		case <-time.After(backoff):
		}
	}
}

// poll reads once and ingests; false means the connection must be re-established.
func (w *worker) poll(ctx context.Context, c *Conn) bool {
	values, bad, err := readTags(ctx, c, w.row.Settings)
	if err != nil {
		w.set(func(s *Status) { s.State, s.LastError = "error", err.Error(); s.Errors++ })
		w.m.log.Warn("s7 read failed", "connector", w.row.Name, "err", err)
		// A refused job (PUT/GET disabled, ...) keeps the connection; retry at the next interval.
		return c.Usable()
	}
	w.set(func(s *Status) {
		s.State, s.LastError, s.LastPoll, s.LastValues, s.BadNodes = "connected", "", time.Now(), values, bad
		s.Polls++
	})
	w.ingest(ctx, values, bad)
	return true
}

func (w *worker) ingest(ctx context.Context, values map[string]any, bad []string) {
	if len(values) == 0 {
		return
	}
	st := w.row.Settings
	byDev := map[string]map[string]any{}
	for _, t := range st.Tags {
		v, ok := values[st.ref(t)]
		if !ok {
			continue
		}
		if byDev[t.Device] == nil {
			byDev[t.Device] = map[string]any{}
		}
		byDev[t.Device][t.Key] = v
	}
	if w.m.sink != nil {
		for ext, vals := range byDev {
			w.m.sink(ext, vals)
		}
		return
	}
	now := time.Now().Unix()
	for ext, vals := range byDev {
		dev, err := w.device(ctx, ext)
		if err != nil {
			w.set(func(s *Status) { s.LastError = "device: " + err.Error() })
			continue
		}
		payload, _ := json.Marshal(map[string]any{"ts": now, "v": vals})
		meta, _ := json.Marshal(map[string]any{"connector_id": w.row.ID, "connector": w.row.Name, "addr": st.addr(), "plc": st.PLC, "bad_tags": bad})
		w.m.ing.IngestTelemetry(ctx, dev, "s7", "s7/"+w.row.Name, payload, string(meta))
	}
	_, _ = w.m.pool.Exec(ctx, `UPDATE core.connector SET last_seen_at = now(), messages = messages + 1 WHERE id = $1::uuid`, w.row.ID)
}

func (w *worker) ownExternalID() string {
	if ext := w.row.Settings.DeviceExternalID; ext != "" {
		return ext
	}
	return "s7-" + strings.ToLower(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, w.row.Name))
}

// device resolves (or creates) a virtual device of this connector: "" is its own device, anything else a tag's
// device override.
func (w *worker) device(ctx context.Context, ext string) (*broker.Device, error) {
	w.cmu.Lock()
	if d, ok := w.devs[ext]; ok {
		w.cmu.Unlock()
		return d, nil
	}
	w.cmu.Unlock()
	if w.m.pool == nil {
		return nil, errors.New("no database")
	}
	name, extID := w.row.Name+" (S7)", ext
	if ext == "" {
		extID = w.ownExternalID()
	} else {
		name = ext + " (S7 " + w.row.Name + ")"
	}
	d := &broker.Device{OrgID: w.row.OrgID, ExternalID: extID, Transport: "s7"}
	err := w.m.pool.QueryRow(ctx, `SELECT id::text FROM core.device WHERE org_id = $1::uuid AND external_id = $2 AND revoked_at IS NULL`, w.row.OrgID, extID).Scan(&d.ID)
	if err != nil {
		if err := w.m.pool.QueryRow(ctx, `INSERT INTO core.device (org_id, external_id, name, transport) VALUES ($1::uuid, $2, $3, 's7') RETURNING id::text`, w.row.OrgID, extID, name).Scan(&d.ID); err != nil {
			return nil, err
		}
		w.m.log.Info("virtual device created for S7 connector", "connector", w.row.Name, "device", extID)
	}
	w.cmu.Lock()
	if w.devs == nil {
		w.devs = map[string]*broker.Device{}
	}
	w.devs[ext] = d
	w.cmu.Unlock()
	return d, nil
}

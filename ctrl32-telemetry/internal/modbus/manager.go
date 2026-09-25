// Package modbus polls Modbus TCP devices (URS A-11, connector kind "modbus_tcp") and writes to them on command
// (URS A-12). Reads go through the normal telemetry path of a virtual device, so datastream assignment, alarms,
// gap detection, reports, dashboards and live streams work as for MQTT devices. A write is always a command with
// identity, reason, ack and audit (ADR-0008): the register must be marked writable, the value must be within the
// configured limits, and the value is read back after writing; the platform never sits in a control loop.
package modbus

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
	mb "github.com/simonvetter/modbus"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/ingest"
)

var keyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// Register maps one Modbus address to a datastream key.
type Register struct {
	Key      string   `json:"key"`
	Table    string   `json:"table"`   // holding | input | coil | discrete
	Address  int      `json:"address"` // 0-based register/coil address
	Type     string   `json:"type"`    // bool | int16 | uint16 | int32 | uint32 | float32 | int64 | uint64 | float64
	Scale    *float64 `json:"scale,omitempty"`
	Offset   *float64 `json:"offset,omitempty"`
	Writable bool     `json:"writable,omitempty"` // holding registers and coils only
	Min      *float64 `json:"min,omitempty"`      // limits for writes (scaled value)
	Max      *float64 `json:"max,omitempty"`
}

// Settings is the connector configuration stored in core.connector.settings.
type Settings struct {
	Host             string     `json:"host"`
	Port             int        `json:"port"`
	UnitID           int        `json:"unit_id"`
	IntervalS        int        `json:"interval_s"`
	TimeoutMs        int        `json:"timeout_ms"`
	ByteOrder        string     `json:"byte_order"` // big | big_swap | little | little_swap (word order of 32/64-bit values)
	Framing          string     `json:"framing"`    // tcp (Modbus TCP, default) | rtuovertcp (RTU frames over a TCP gateway)
	DeviceExternalID string     `json:"device_external_id"`
	Registers        []Register `json:"registers"`
	// Register profile the registers were taken from (URS A-11); informational for the poller.
	ProfileID      string `json:"profile_id,omitempty"`
	ProfileName    string `json:"profile_name,omitempty"`
	ProfileVersion int    `json:"profile_version,omitempty"`
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
}

var types = map[string]int{"bool": 1, "int16": 1, "uint16": 1, "int32": 2, "uint32": 2, "float32": 2, "int64": 4, "uint64": 4, "float64": 4}

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
		st.Port = 502
	}
	if st.Port < 1 || st.Port > 65535 {
		return st, errors.New("port must be 1..65535")
	}
	if st.UnitID == 0 {
		st.UnitID = 1
	}
	if st.UnitID < 0 || st.UnitID > 255 {
		return st, errors.New("unit id must be 0..255")
	}
	if st.IntervalS == 0 {
		st.IntervalS = 10
	}
	if st.IntervalS < 1 || st.IntervalS > 3600 {
		return st, errors.New("interval must be 1..3600 seconds")
	}
	if st.TimeoutMs == 0 {
		st.TimeoutMs = 2000
	}
	if st.TimeoutMs < 100 || st.TimeoutMs > 30000 {
		return st, errors.New("timeout must be 100..30000 ms")
	}
	switch st.ByteOrder {
	case "":
		st.ByteOrder = "big"
	case "big", "big_swap", "little", "little_swap":
	default:
		return st, errors.New("byte order must be big, big_swap, little or little_swap")
	}
	switch st.Framing {
	case "":
		st.Framing = "tcp"
	case "tcp", "rtuovertcp":
	default:
		return st, errors.New("framing must be tcp or rtuovertcp")
	}
	st.DeviceExternalID = strings.TrimSpace(st.DeviceExternalID)
	regs, err := ValidateRegisters(st.Registers)
	if err != nil {
		return st, err
	}
	st.Registers = regs
	return st, nil
}

// ValidateRegisters normalises and checks a register map (connector settings and register profiles).
func ValidateRegisters(regs []Register) ([]Register, error) {
	if len(regs) == 0 {
		return nil, errors.New("at least one register is required")
	}
	if len(regs) > 500 {
		return nil, errors.New("at most 500 registers per connector")
	}
	seen := map[string]bool{}
	for i := range regs {
		r := &regs[i]
		r.Key, r.Table, r.Type = strings.TrimSpace(r.Key), strings.ToLower(strings.TrimSpace(r.Table)), strings.ToLower(strings.TrimSpace(r.Type))
		if !keyRe.MatchString(r.Key) {
			return nil, fmt.Errorf("register %d: key must match ^[a-z][a-z0-9_]{0,31}$", i+1)
		}
		if seen[r.Key] {
			return nil, fmt.Errorf("register %d: duplicate key %q", i+1, r.Key)
		}
		seen[r.Key] = true
		switch r.Table {
		case "holding", "input":
			if r.Type == "" {
				r.Type = "int16"
			}
			if r.Type == "bool" {
				return nil, fmt.Errorf("register %d: bool is for coils and discrete inputs", i+1)
			}
		case "coil", "discrete":
			if r.Type == "" {
				r.Type = "bool"
			}
			if r.Type != "bool" {
				return nil, fmt.Errorf("register %d: coils and discrete inputs are bool", i+1)
			}
		default:
			return nil, fmt.Errorf("register %d: table must be holding, input, coil or discrete", i+1)
		}
		if _, ok := types[r.Type]; !ok {
			return nil, fmt.Errorf("register %d: unsupported type %q", i+1, r.Type)
		}
		if r.Address < 0 || r.Address+types[r.Type]-1 > 65535 {
			return nil, fmt.Errorf("register %d: address out of range", i+1)
		}
		if r.Writable && (r.Table == "input" || r.Table == "discrete") {
			return nil, fmt.Errorf("register %d: input registers and discrete inputs are read-only", i+1)
		}
		if r.Scale != nil && *r.Scale == 0 {
			return nil, fmt.Errorf("register %d: scale must not be 0", i+1)
		}
		if r.Min != nil && r.Max != nil && *r.Min > *r.Max {
			return nil, fmt.Errorf("register %d: min must not exceed max", i+1)
		}
	}
	return regs, nil
}

func (st Settings) addr() string { return net.JoinHostPort(st.Host, strconv.Itoa(st.Port)) }

func (st Settings) register(key string) *Register {
	for i := range st.Registers {
		if st.Registers[i].Key == key {
			return &st.Registers[i]
		}
	}
	return nil
}

// ---- client ---------------------------------------------------------------------------------------

func connect(st Settings) (*mb.ModbusClient, error) {
	scheme := "tcp"
	if st.Framing == "rtuovertcp" {
		scheme = "rtuovertcp"
	}
	c, err := mb.NewClient(&mb.ClientConfiguration{URL: scheme + "://" + st.addr(), Timeout: time.Duration(st.TimeoutMs) * time.Millisecond})
	if err != nil {
		return nil, err
	}
	endian, words := mb.BIG_ENDIAN, mb.HIGH_WORD_FIRST
	switch st.ByteOrder {
	case "big_swap":
		words = mb.LOW_WORD_FIRST
	case "little":
		endian, words = mb.LITTLE_ENDIAN, mb.LOW_WORD_FIRST
	case "little_swap":
		endian, words = mb.LITTLE_ENDIAN, mb.HIGH_WORD_FIRST
	}
	_ = c.SetEncoding(endian, words)
	_ = c.SetUnitId(uint8(st.UnitID))
	if err := c.Open(); err != nil {
		return nil, fmt.Errorf("connect %s: %w", st.addr(), err)
	}
	return c, nil
}

func scaled(r Register, raw float64) float64 {
	v := raw
	if r.Scale != nil {
		v *= *r.Scale
	}
	if r.Offset != nil {
		v += *r.Offset
	}
	return v
}

func unscaled(r Register, v float64) float64 {
	if r.Offset != nil {
		v -= *r.Offset
	}
	if r.Scale != nil {
		v /= *r.Scale
	}
	return v
}

// readOne reads one register and returns the scaled value (float64) or a bool.
func readOne(c *mb.ModbusClient, r Register) (any, error) {
	a := uint16(r.Address)
	switch r.Table {
	case "coil":
		return c.ReadCoil(a)
	case "discrete":
		return c.ReadDiscreteInput(a)
	}
	rt := mb.HOLDING_REGISTER
	if r.Table == "input" {
		rt = mb.INPUT_REGISTER
	}
	var raw float64
	switch r.Type {
	case "int16":
		v, err := c.ReadRegister(a, rt)
		if err != nil {
			return nil, err
		}
		raw = float64(int16(v))
	case "uint16":
		v, err := c.ReadRegister(a, rt)
		if err != nil {
			return nil, err
		}
		raw = float64(v)
	case "int32":
		v, err := c.ReadUint32(a, rt)
		if err != nil {
			return nil, err
		}
		raw = float64(int32(v))
	case "uint32":
		v, err := c.ReadUint32(a, rt)
		if err != nil {
			return nil, err
		}
		raw = float64(v)
	case "float32":
		v, err := c.ReadFloat32(a, rt)
		if err != nil {
			return nil, err
		}
		raw = float64(v)
	case "int64":
		v, err := c.ReadUint64(a, rt)
		if err != nil {
			return nil, err
		}
		raw = float64(int64(v))
	case "uint64":
		v, err := c.ReadUint64(a, rt)
		if err != nil {
			return nil, err
		}
		raw = float64(v)
	case "float64":
		v, err := c.ReadFloat64(a, rt)
		if err != nil {
			return nil, err
		}
		raw = v
	default:
		return nil, fmt.Errorf("unsupported type %s", r.Type)
	}
	if math.IsNaN(raw) || math.IsInf(raw, 0) {
		return nil, errors.New("not a number")
	}
	return math.Round(scaled(r, raw)*1e6) / 1e6, nil
}

// writeOne writes a scaled value (number or bool) to a writable register.
func writeOne(c *mb.ModbusClient, r Register, v any) error {
	a := uint16(r.Address)
	if r.Table == "coil" {
		b, ok := v.(bool)
		if !ok {
			f, isNum := v.(float64)
			if !isNum {
				return errors.New("coil needs true/false")
			}
			b = f != 0
		}
		return c.WriteCoil(a, b)
	}
	f, ok := v.(float64)
	if !ok {
		return errors.New("value must be a number")
	}
	raw := unscaled(r, f)
	switch r.Type {
	case "int16":
		if raw < math.MinInt16 || raw > math.MaxInt16 {
			return errors.New("value does not fit int16")
		}
		return c.WriteRegister(a, uint16(int16(math.Round(raw))))
	case "uint16":
		if raw < 0 || raw > math.MaxUint16 {
			return errors.New("value does not fit uint16")
		}
		return c.WriteRegister(a, uint16(math.Round(raw)))
	case "int32":
		if raw < math.MinInt32 || raw > math.MaxInt32 {
			return errors.New("value does not fit int32")
		}
		return c.WriteUint32(a, uint32(int32(math.Round(raw))))
	case "uint32":
		if raw < 0 || raw > math.MaxUint32 {
			return errors.New("value does not fit uint32")
		}
		return c.WriteUint32(a, uint32(math.Round(raw)))
	case "float32":
		return c.WriteFloat32(a, float32(raw))
	case "int64":
		return c.WriteUint64(a, uint64(int64(math.Round(raw))))
	case "uint64":
		if raw < 0 {
			return errors.New("value does not fit uint64")
		}
		return c.WriteUint64(a, uint64(math.Round(raw)))
	case "float64":
		return c.WriteFloat64(a, raw)
	}
	return fmt.Errorf("unsupported type %s", r.Type)
}

// ---- manager --------------------------------------------------------------------------------------

// Manager runs one poller per active modbus_tcp connector and executes write commands.
type Manager struct {
	pool    *pgxpool.Pool
	ing     *ingest.Ingest
	log     *slog.Logger
	mu      sync.Mutex
	workers map[string]*worker
	status  map[string]*Status
	resync  chan struct{}
}

func New(pool *pgxpool.Pool, ing *ingest.Ingest, log *slog.Logger) *Manager {
	return &Manager{pool: pool, ing: ing, log: log.With("component", "modbus"), workers: map[string]*worker{}, status: map[string]*Status{}, resync: make(chan struct{}, 1)}
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
	rows, err := m.pool.Query(ctx, `SELECT id::text, org_id::text, name, settings::text FROM core.connector WHERE kind = 'modbus_tcp' AND revoked_at IS NULL`)
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
		wctx, cancel := context.WithCancel(ctx)
		st := &Status{State: "connecting", Since: time.Now()}
		m.status[id] = st
		w := &worker{m: m, row: row, hash: row.Hash, cancel: cancel, st: st}
		m.workers[id] = w
		go w.run(wctx)
	}
}

// Test connects once, reads every register and disconnects; used by the UI before saving a connector.
func (m *Manager) Test(ctx context.Context, st Settings) (map[string]any, []string, error) {
	c, err := connect(st)
	if err != nil {
		return nil, nil, err
	}
	defer c.Close()
	values, bad := read(c, st)
	if len(values) == 0 && len(bad) > 0 {
		return values, bad, errors.New("no register could be read: " + bad[0])
	}
	return values, bad, nil
}

// ---- writes (commands) ---------------------------------------------------------------------------

// WriteOp is a validated write for a virtual device: method "write", args {key, value}.
type WriteOp struct {
	ConnectorID, ConnectorName string
	Settings                   Settings
	Register                   Register
	Value                      any // float64 or bool
}

// Prepare validates a command for a virtual device of a modbus connector without touching the network.
func (m *Manager) Prepare(ctx context.Context, deviceID, method string, args json.RawMessage) (*WriteOp, error) {
	if method != "write" {
		return nil, errors.New("modbus devices accept the method \"write\" with args {key, value}")
	}
	var a struct {
		Key   string `json:"key"`
		Value any    `json:"value"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, errors.New("args must be {key, value}")
	}
	w := m.workerFor(ctx, deviceID)
	if w == nil {
		return nil, errors.New("no running Modbus connector owns this device")
	}
	r := w.row.Settings.register(a.Key)
	if r == nil {
		return nil, fmt.Errorf("unknown key %q", a.Key)
	}
	if !r.Writable {
		return nil, fmt.Errorf("key %q is not writable", a.Key)
	}
	op := &WriteOp{ConnectorID: w.row.ID, ConnectorName: w.row.Name, Settings: w.row.Settings, Register: *r}
	switch v := a.Value.(type) {
	case bool:
		if r.Table != "coil" {
			return nil, errors.New("value must be a number")
		}
		op.Value = v
	case float64:
		if r.Table == "coil" {
			op.Value = v != 0
			break
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, errors.New("value must be a finite number")
		}
		if r.Min != nil && v < *r.Min || r.Max != nil && v > *r.Max {
			return nil, fmt.Errorf("value %v is outside the limits %s", v, limits(*r))
		}
		op.Value = v
	default:
		return nil, errors.New("value must be a number or true/false")
	}
	return op, nil
}

func limits(r Register) string {
	lo, hi := "-∞", "∞"
	if r.Min != nil {
		lo = strconv.FormatFloat(*r.Min, 'f', -1, 64)
	}
	if r.Max != nil {
		hi = strconv.FormatFloat(*r.Max, 'f', -1, 64)
	}
	return lo + ".." + hi
}

// Do performs the write on a dedicated connection, reads the value back and returns the result for the ack.
func (m *Manager) Do(ctx context.Context, op *WriteOp) (map[string]any, error) {
	c, err := connect(op.Settings)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if err := writeOne(c, op.Register, op.Value); err != nil {
		return nil, fmt.Errorf("write %s: %w", op.Register.Key, err)
	}
	back, err := readOne(c, op.Register)
	if err != nil {
		return nil, fmt.Errorf("read back %s: %w", op.Register.Key, err)
	}
	m.mu.Lock()
	if st, ok := m.status[op.ConnectorID]; ok {
		st.Writes++
		if st.LastValues == nil {
			st.LastValues = map[string]any{}
		}
		st.LastValues[op.Register.Key] = back
	}
	m.mu.Unlock()
	m.log.Info("modbus write", "connector", op.ConnectorName, "key", op.Register.Key, "table", op.Register.Table, "address", op.Register.Address, "value", op.Value, "readback", back)
	return map[string]any{"key": op.Register.Key, "table": op.Register.Table, "address": op.Register.Address, "value": op.Value, "readback": back, "written_at": time.Now().UTC()}, nil
}

func (m *Manager) workerFor(ctx context.Context, deviceID string) *worker {
	m.mu.Lock()
	ws := make([]*worker, 0, len(m.workers))
	for _, w := range m.workers {
		ws = append(ws, w)
	}
	m.mu.Unlock()
	for _, w := range ws {
		d, err := w.device(ctx)
		if err == nil && d.ID == deviceID {
			return w
		}
	}
	return nil
}

// ---- worker ---------------------------------------------------------------------------------------

type worker struct {
	m      *Manager
	row    connRow
	hash   string
	cancel context.CancelFunc
	st     *Status
	devMu  sync.Mutex
	dev    *broker.Device
}

func (w *worker) set(fn func(s *Status)) {
	w.m.mu.Lock()
	fn(w.st)
	w.m.mu.Unlock()
}

func (w *worker) run(ctx context.Context) {
	backoff := 5 * time.Second
	for ctx.Err() == nil {
		c, err := connect(w.row.Settings)
		if err != nil {
			w.set(func(s *Status) { s.State, s.LastError = "error", err.Error(); s.Errors++ })
			w.m.log.Warn("modbus connect failed", "connector", w.row.Name, "addr", w.row.Settings.addr(), "err", err)
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
		w.set(func(s *Status) { s.State, s.LastError = "connected", "" })
		w.m.log.Info("modbus connected", "connector", w.row.Name, "addr", w.row.Settings.addr(), "registers", len(w.row.Settings.Registers))
		t := time.NewTicker(time.Duration(w.row.Settings.IntervalS) * time.Second)
		ok := w.poll(ctx, c)
	loop:
		for ok {
			select {
			case <-ctx.Done():
				t.Stop()
				_ = c.Close()
				w.set(func(s *Status) { s.State = "stopped" })
				return
			case <-t.C:
				if !w.poll(ctx, c) {
					break loop
				}
			}
		}
		t.Stop()
		_ = c.Close()
	}
}

// poll reads once and ingests; false means the connection is broken and must be re-established.
func (w *worker) poll(ctx context.Context, c *mb.ModbusClient) bool {
	values, bad := read(c, w.row.Settings)
	if len(values) == 0 && len(bad) > 0 {
		w.set(func(s *Status) { s.State, s.LastError, s.BadNodes = "error", bad[0], bad; s.Errors++ })
		w.m.log.Warn("modbus read failed", "connector", w.row.Name, "err", bad[0])
		return false
	}
	w.set(func(s *Status) {
		s.State, s.LastError, s.LastPoll, s.LastValues, s.BadNodes = "connected", "", time.Now(), values, bad
		s.Polls++
	})
	dev, err := w.device(ctx)
	if err != nil {
		w.set(func(s *Status) { s.LastError = "device: " + err.Error() })
		return true
	}
	payload, _ := json.Marshal(map[string]any{"ts": time.Now().Unix(), "v": values})
	meta, _ := json.Marshal(map[string]any{"connector_id": w.row.ID, "connector": w.row.Name, "addr": w.row.Settings.addr(), "unit_id": w.row.Settings.UnitID, "bad_registers": bad})
	w.m.ing.IngestTelemetry(ctx, dev, "modbus", "modbus/"+w.row.Name, payload, string(meta))
	_, _ = w.m.pool.Exec(ctx, `UPDATE core.connector SET last_seen_at = now(), messages = messages + 1 WHERE id = $1::uuid`, w.row.ID)
	return true
}

// device resolves (or creates) the virtual device that owns the datastreams of this connector.
func (w *worker) device(ctx context.Context) (*broker.Device, error) {
	w.devMu.Lock()
	defer w.devMu.Unlock()
	if w.dev != nil {
		return w.dev, nil
	}
	ext := w.row.Settings.DeviceExternalID
	if ext == "" {
		ext = "modbus-" + strings.ToLower(strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
				return r
			}
			return '-'
		}, w.row.Name))
	}
	d := &broker.Device{OrgID: w.row.OrgID, ExternalID: ext, Transport: "modbus"}
	err := w.m.pool.QueryRow(ctx, `SELECT id::text FROM core.device WHERE org_id = $1::uuid AND external_id = $2 AND revoked_at IS NULL`, w.row.OrgID, ext).Scan(&d.ID)
	if err != nil {
		if err := w.m.pool.QueryRow(ctx, `INSERT INTO core.device (org_id, external_id, name, transport) VALUES ($1::uuid, $2, $3, 'modbus') RETURNING id::text`, w.row.OrgID, ext, w.row.Name+" (Modbus)").Scan(&d.ID); err != nil {
			return nil, err
		}
		w.m.log.Info("virtual device created for Modbus connector", "connector", w.row.Name, "device", ext)
	}
	w.dev = d
	return d, nil
}

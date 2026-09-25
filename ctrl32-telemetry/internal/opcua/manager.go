// Package opcua polls OPC UA servers (URS E-*, connector kind "opcua"): the server is the OPC UA client, reads a
// fixed list of nodes every interval and feeds the values through the normal telemetry path of a virtual device,
// so datastream assignment, alarms, gap detection, reports and exports work exactly as for MQTT devices.
package opcua

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/ingest"
)

const applicationURI = "urn:ctrl32:telemetry:opcua-client"

var (
	keyRe    = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	nodeIDRe = regexp.MustCompile(`^(ns=\d+;)?(i=\d+|s=.+|g=[0-9a-fA-F-]{36}|b=[A-Za-z0-9+/=]+)$`)
)

// Node maps one OPC UA variable to a datastream key.
type Node struct {
	NodeID string   `json:"node_id"` // e.g. ns=3;s=Sinusoid or ns=2;i=1001
	Key    string   `json:"key"`     // datastream key the value is published under
	Scale  *float64 `json:"scale,omitempty"`
	Offset *float64 `json:"offset,omitempty"`
	// Writable nodes accept the command write {key, value} (URS A-12); min/max limit the scaled value.
	Writable bool     `json:"writable,omitempty"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
}

// Settings is the connector configuration stored in core.connector.settings.
type Settings struct {
	Endpoint         string `json:"endpoint"`        // opc.tcp://host:4840/path
	SecurityPolicy   string `json:"security_policy"` // None | Basic256Sha256 | Basic256 | Basic128Rsa15 | Aes128_Sha256_RsaOaep | Aes256_Sha256_RsaPss
	SecurityMode     string `json:"security_mode"`   // None | Sign | SignAndEncrypt
	Auth             string `json:"auth"`            // anonymous | username
	Username         string `json:"username,omitempty"`
	Password         string `json:"password,omitempty"`
	IntervalS        int    `json:"interval_s"`         // polling interval, 1..3600
	DeviceExternalID string `json:"device_external_id"` // virtual device that owns the datastreams
	Nodes            []Node `json:"nodes"`
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

// ValidateSettings parses and normalizes raw settings.
func ValidateSettings(raw json.RawMessage) (Settings, error) {
	var st Settings
	if len(raw) == 0 {
		return st, errors.New("settings required")
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return st, fmt.Errorf("settings: %w", err)
	}
	st.Endpoint = strings.TrimSpace(st.Endpoint)
	u, err := url.Parse(st.Endpoint)
	if err != nil || u.Scheme != "opc.tcp" || u.Host == "" {
		return st, errors.New("endpoint must look like opc.tcp://host:4840/path")
	}
	switch st.SecurityPolicy {
	case "", "None":
		st.SecurityPolicy = "None"
	case "Basic256Sha256", "Basic256", "Basic128Rsa15", "Aes128_Sha256_RsaOaep", "Aes256_Sha256_RsaPss":
	default:
		return st, errors.New("unsupported security policy")
	}
	switch st.SecurityMode {
	case "", "None":
		st.SecurityMode = "None"
	case "Sign", "SignAndEncrypt":
	default:
		return st, errors.New("security mode must be None, Sign or SignAndEncrypt")
	}
	if (st.SecurityPolicy == "None") != (st.SecurityMode == "None") {
		return st, errors.New("security policy None requires mode None and vice versa")
	}
	switch st.Auth {
	case "", "anonymous":
		st.Auth = "anonymous"
	case "username":
		if strings.TrimSpace(st.Username) == "" {
			return st, errors.New("username required for username authentication")
		}
	default:
		return st, errors.New("auth must be anonymous or username")
	}
	if st.IntervalS == 0 {
		st.IntervalS = 10
	}
	if st.IntervalS < 1 || st.IntervalS > 3600 {
		return st, errors.New("interval must be 1..3600 seconds")
	}
	st.DeviceExternalID = strings.TrimSpace(st.DeviceExternalID)
	if len(st.Nodes) == 0 {
		return st, errors.New("at least one node is required")
	}
	if len(st.Nodes) > 500 {
		return st, errors.New("at most 500 nodes per connector")
	}
	seen := map[string]bool{}
	for i := range st.Nodes {
		n := &st.Nodes[i]
		n.NodeID, n.Key = strings.TrimSpace(n.NodeID), strings.TrimSpace(n.Key)
		if _, err := ua.ParseNodeID(n.NodeID); err != nil || !nodeIDRe.MatchString(n.NodeID) {
			return st, fmt.Errorf("node %d: invalid node id %q", i+1, n.NodeID)
		}
		if !keyRe.MatchString(n.Key) {
			return st, fmt.Errorf("node %d: key must match ^[a-z][a-z0-9_]{0,31}$", i+1)
		}
		if seen[n.Key] {
			return st, fmt.Errorf("node %d: duplicate key %q", i+1, n.Key)
		}
		seen[n.Key] = true
		if n.Min != nil && n.Max != nil && *n.Min > *n.Max {
			return st, fmt.Errorf("node %d: min must not exceed max", i+1)
		}
	}
	return st, nil
}

// Manager runs one poller per active opcua connector.
type Manager struct {
	pool    *pgxpool.Pool
	ing     *ingest.Ingest
	log     *slog.Logger
	dir     string // client certificate storage
	mu      sync.Mutex
	workers map[string]*worker
	status  map[string]*Status
	resync  chan struct{}
	cert    []byte
	key     *rsa.PrivateKey
}

func New(pool *pgxpool.Pool, ing *ingest.Ingest, log *slog.Logger, dir string) *Manager {
	return &Manager{pool: pool, ing: ing, log: log.With("component", "opcua"), dir: dir, workers: map[string]*worker{}, status: map[string]*Status{}, resync: make(chan struct{}, 1)}
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
	rows, err := m.pool.Query(ctx, `SELECT id::text, org_id::text, name, settings::text FROM core.connector WHERE kind = 'opcua' AND revoked_at IS NULL`)
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

// ---- client certificate (needed for any policy other than None) ----------------------------------

func (m *Manager) clientCert() ([]byte, *rsa.PrivateKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cert != nil {
		return m.cert, m.key, nil
	}
	_ = os.MkdirAll(m.dir, 0o750)
	certPath, keyPath := filepath.Join(m.dir, "client.crt"), filepath.Join(m.dir, "client.key")
	if cb, err := os.ReadFile(certPath); err == nil {
		if kb, err := os.ReadFile(keyPath); err == nil {
			if cblock, _ := pem.Decode(cb); cblock != nil {
				if kblock, _ := pem.Decode(kb); kblock != nil {
					if key, err := x509.ParsePKCS1PrivateKey(kblock.Bytes); err == nil {
						m.cert, m.key = cblock.Bytes, key
						return m.cert, m.key, nil
					}
				}
			}
		}
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	uri, _ := url.Parse(applicationURI)
	host, _ := os.Hostname()
	tmpl := x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "ctrl32 telemetry OPC UA client", Organization: []string{"ctrl32 telemetry"}},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageDataEncipherment | x509.KeyUsageContentCommitment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		URIs:        []*url.URL{uri}, DNSNames: []string{host}, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	_ = os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
	_ = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600)
	m.cert, m.key = der, key
	m.log.Info("generated OPC UA client certificate", "path", certPath, "uri", applicationURI)
	return m.cert, m.key, nil
}

// ClientCertificatePEM returns the client certificate so operators can trust it on their servers.
func (m *Manager) ClientCertificatePEM() (string, error) {
	der, _, err := m.clientCert()
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), nil
}

func (m *Manager) connect(ctx context.Context, st Settings) (*opcua.Client, error) {
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	eps, err := opcua.GetEndpoints(cctx, st.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("endpoints: %w", err)
	}
	mode := ua.MessageSecurityModeFromString(st.SecurityMode)
	ep, err := opcua.SelectEndpoint(eps, st.SecurityPolicy, mode)
	if err != nil || ep == nil {
		return nil, fmt.Errorf("the server offers no endpoint with policy %s / mode %s", st.SecurityPolicy, st.SecurityMode)
	}
	authType := ua.UserTokenTypeAnonymous
	if st.Auth == "username" {
		authType = ua.UserTokenTypeUserName
	}
	opts := []opcua.Option{
		opcua.SecurityFromEndpoint(ep, authType), opcua.ApplicationURI(applicationURI), opcua.AutoReconnect(false),
		opcua.DialTimeout(10 * time.Second), opcua.RequestTimeout(15 * time.Second), opcua.SessionTimeout(10 * time.Minute),
	}
	if st.Auth == "username" {
		opts = append(opts, opcua.AuthUsername(st.Username, st.Password))
	} else {
		opts = append(opts, opcua.AuthAnonymous())
	}
	if st.SecurityPolicy != "None" {
		cert, key, err := m.clientCert()
		if err != nil {
			return nil, err
		}
		opts = append(opts, opcua.Certificate(cert), opcua.PrivateKey(key))
	}
	// The endpoint the server advertises often carries a hostname that only resolves inside its own network;
	// connect to the address the operator typed and keep the negotiated security settings.
	c, err := opcua.NewClient(st.Endpoint, opts...)
	if err != nil {
		return nil, err
	}
	if err := c.Connect(cctx); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	return c, nil
}

// read reads all configured nodes once; bad nodes are reported by key.
func read(ctx context.Context, c *opcua.Client, st Settings) (map[string]any, []string, error) {
	ids := make([]*ua.ReadValueID, 0, len(st.Nodes))
	for _, n := range st.Nodes {
		id, _ := ua.ParseNodeID(n.NodeID)
		ids = append(ids, &ua.ReadValueID{NodeID: id, AttributeID: ua.AttributeIDValue})
	}
	rctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := c.Read(rctx, &ua.ReadRequest{MaxAge: 0, TimestampsToReturn: ua.TimestampsToReturnSource, NodesToRead: ids})
	if err != nil {
		return nil, nil, err
	}
	if len(resp.Results) != len(st.Nodes) {
		return nil, nil, fmt.Errorf("server returned %d results for %d nodes", len(resp.Results), len(st.Nodes))
	}
	values := map[string]any{}
	var bad []string
	for i, r := range resp.Results {
		n := st.Nodes[i]
		if r.Status != ua.StatusOK || r.Value == nil {
			bad = append(bad, n.Key+": "+r.Status.Error())
			continue
		}
		v, ok := convert(r.Value.Value(), n)
		if !ok {
			bad = append(bad, n.Key+": unsupported type "+fmt.Sprintf("%T", r.Value.Value()))
			continue
		}
		values[n.Key] = v
	}
	return values, bad, nil
}

func convert(v any, n Node) (any, bool) {
	var f float64
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		return x, true
	case time.Time:
		return x.UTC().Format(time.RFC3339Nano), true
	case float32:
		f = float64(x)
	case float64:
		f = x
	case int8:
		f = float64(x)
	case int16:
		f = float64(x)
	case int32:
		f = float64(x)
	case int64:
		f = float64(x)
	case int:
		f = float64(x)
	case uint8:
		f = float64(x)
	case uint16:
		f = float64(x)
	case uint32:
		f = float64(x)
	case uint64:
		f = float64(x)
	case uint:
		f = float64(x)
	default:
		return nil, false
	}
	if n.Scale != nil {
		f *= *n.Scale
	}
	if n.Offset != nil {
		f += *n.Offset
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, false
	}
	return f, true
}

// Test connects once, reads the nodes and disconnects; used by the UI before saving a connector.
func (m *Manager) Test(ctx context.Context, st Settings) (map[string]any, []string, error) {
	c, err := m.connect(ctx, st)
	if err != nil {
		return nil, nil, err
	}
	defer c.Close(ctx)
	return read(ctx, c, st)
}

// ---- worker ---------------------------------------------------------------------------------------

type worker struct {
	m      *Manager
	row    connRow
	hash   string
	cancel context.CancelFunc
	st     *Status
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
		c, err := w.m.connect(ctx, w.row.Settings)
		if err != nil {
			w.set(func(s *Status) { s.State, s.LastError = "error", err.Error(); s.Errors++ })
			w.m.log.Warn("opcua connect failed", "connector", w.row.Name, "endpoint", w.row.Settings.Endpoint, "err", err)
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
		w.m.log.Info("opcua connected", "connector", w.row.Name, "endpoint", w.row.Settings.Endpoint, "nodes", len(w.row.Settings.Nodes))
		interval := time.Duration(w.row.Settings.IntervalS) * time.Second
		t := time.NewTicker(interval)
		w.poll(ctx, c)
	loop:
		for {
			select {
			case <-ctx.Done():
				t.Stop()
				_ = c.Close(context.Background())
				w.set(func(s *Status) { s.State = "stopped" })
				return
			case <-t.C:
				if !w.poll(ctx, c) {
					break loop
				}
			}
		}
		t.Stop()
		_ = c.Close(context.Background())
	}
}

// poll reads once and ingests; false means the connection is broken and must be re-established.
func (w *worker) poll(ctx context.Context, c *opcua.Client) bool {
	values, bad, err := read(ctx, c, w.row.Settings)
	if err != nil {
		w.set(func(s *Status) { s.State, s.LastError = "error", err.Error(); s.Errors++ })
		w.m.log.Warn("opcua read failed", "connector", w.row.Name, "err", err)
		return false
	}
	w.set(func(s *Status) {
		s.State, s.LastError, s.LastPoll, s.LastValues, s.BadNodes = "connected", "", time.Now(), values, bad
		s.Polls++
	})
	if len(values) == 0 {
		return true
	}
	dev, err := w.device(ctx)
	if err != nil {
		w.set(func(s *Status) { s.LastError = "device: " + err.Error() })
		return true
	}
	payload, _ := json.Marshal(map[string]any{"ts": time.Now().Unix(), "v": values})
	meta, _ := json.Marshal(map[string]any{"connector_id": w.row.ID, "connector": w.row.Name, "endpoint": w.row.Settings.Endpoint, "bad_nodes": bad})
	w.m.ing.IngestTelemetry(ctx, dev, "opcua", "opcua/"+w.row.Name, payload, string(meta))
	_, _ = w.m.pool.Exec(ctx, `UPDATE core.connector SET last_seen_at = now(), messages = messages + 1 WHERE id = $1::uuid`, w.row.ID)
	return true
}

// device resolves (or creates) the virtual device that owns the datastreams of this connector.
func (w *worker) device(ctx context.Context) (*broker.Device, error) {
	if w.dev != nil {
		return w.dev, nil
	}
	ext := w.row.Settings.DeviceExternalID
	if ext == "" {
		ext = "opcua-" + strings.ToLower(strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
				return r
			}
			return '-'
		}, w.row.Name))
	}
	d := &broker.Device{OrgID: w.row.OrgID, ExternalID: ext, Transport: "opcua"}
	err := w.m.pool.QueryRow(ctx, `SELECT id::text FROM core.device WHERE org_id = $1::uuid AND external_id = $2 AND revoked_at IS NULL`, w.row.OrgID, ext).Scan(&d.ID)
	if err != nil {
		if err := w.m.pool.QueryRow(ctx, `INSERT INTO core.device (org_id, external_id, name, transport) VALUES ($1::uuid, $2, $3, 'opcua') RETURNING id::text`, w.row.OrgID, ext, w.row.Name+" (OPC UA)").Scan(&d.ID); err != nil {
			return nil, err
		}
		w.m.log.Info("virtual device created for OPC UA connector", "connector", w.row.Name, "device", ext)
	}
	w.dev = d
	return d, nil
}

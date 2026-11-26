// Package opcua reads OPC UA servers (URS E-*, connector kind "opcua"): the server is the OPC UA client, reads a
// list of nodes every interval (mode "poll") or subscribes to their changes (mode "subscribe") and feeds the values
// through the normal telemetry path of one or more virtual devices, so datastream assignment, alarms, gap detection,
// reports and exports work exactly as for MQTT devices.
package opcua

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
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
	"strconv"
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
	extIDRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	thumbRe  = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// MaxNodes is the number of nodes one connector may read.
const MaxNodes = 2000

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
	// Device optionally routes the value to another virtual device than the connector's own (one server — for
	// example a gateway — feeding several devices); keys are unique per device.
	Device string `json:"device,omitempty"`
	// Deadband (mode subscribe): absolute change below which the server does not report a new value.
	Deadband *float64 `json:"deadband,omitempty"`
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
	// Mode: poll (read all nodes every interval_s) or subscribe (the server reports changes; publishing_ms is the
	// publishing interval and the sampling interval of the monitored items).
	Mode         string `json:"mode,omitempty"`
	PublishingMs int    `json:"publishing_ms,omitempty"`
	// ServerThumbprint pins the server certificate (SHA-1 hex, as OPC UA tools show it): with a pin set the
	// connector refuses a server presenting another certificate. Empty = trust the certificate the endpoint offers.
	ServerThumbprint string `json:"server_thumbprint,omitempty"`
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
	Mode       string         `json:"mode,omitempty"`
	// Notifications counts data change notifications in mode subscribe.
	Notifications int64  `json:"notifications,omitempty"`
	Thumbprint    string `json:"server_thumbprint,omitempty"` // of the certificate the server presented
}

// ValidateConnection parses and normalizes the connection part of the settings (endpoint, security, login); the
// node list may be empty — used by browsing and endpoint discovery.
func ValidateConnection(raw json.RawMessage) (Settings, error) {
	var st Settings
	if len(raw) == 0 {
		return st, errors.New("settings required")
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return st, fmt.Errorf("settings: %w", err)
	}
	return st, st.validateConnection()
}

func (st *Settings) validateConnection() error {
	st.Endpoint = strings.TrimSpace(st.Endpoint)
	u, err := url.Parse(st.Endpoint)
	if err != nil || u.Scheme != "opc.tcp" || u.Host == "" {
		return errors.New("endpoint must look like opc.tcp://host:4840/path")
	}
	switch st.SecurityPolicy {
	case "", "None":
		st.SecurityPolicy = "None"
	case "Basic256Sha256", "Basic256", "Basic128Rsa15", "Aes128_Sha256_RsaOaep", "Aes256_Sha256_RsaPss":
	default:
		return errors.New("unsupported security policy")
	}
	switch st.SecurityMode {
	case "", "None":
		st.SecurityMode = "None"
	case "Sign", "SignAndEncrypt":
	default:
		return errors.New("security mode must be None, Sign or SignAndEncrypt")
	}
	if (st.SecurityPolicy == "None") != (st.SecurityMode == "None") {
		return errors.New("security policy None requires mode None and vice versa")
	}
	switch st.Auth {
	case "", "anonymous":
		st.Auth = "anonymous"
	case "username":
		if strings.TrimSpace(st.Username) == "" {
			return errors.New("username required for username authentication")
		}
	default:
		return errors.New("auth must be anonymous or username")
	}
	st.ServerThumbprint = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(st.ServerThumbprint), ":", ""), " ", ""))
	if st.ServerThumbprint != "" && !thumbRe.MatchString(st.ServerThumbprint) {
		return errors.New("server thumbprint must be 40 hex digits (SHA-1)")
	}
	return nil
}

// ValidateSettings parses and normalizes raw settings.
func ValidateSettings(raw json.RawMessage) (Settings, error) {
	st, err := ValidateConnection(raw)
	if err != nil {
		return st, err
	}
	switch st.Mode {
	case "", "poll":
		st.Mode = "poll"
	case "subscribe":
		if st.PublishingMs == 0 {
			st.PublishingMs = 1000
		}
		if st.PublishingMs < 100 || st.PublishingMs > 60000 {
			return st, errors.New("publishing interval must be 100..60000 ms")
		}
	default:
		return st, errors.New("mode must be poll or subscribe")
	}
	if st.Mode == "poll" {
		st.PublishingMs = 0
	}
	if st.IntervalS == 0 {
		st.IntervalS = 10
	}
	if st.IntervalS < 1 || st.IntervalS > 3600 {
		return st, errors.New("interval must be 1..3600 seconds")
	}
	st.DeviceExternalID = strings.TrimSpace(st.DeviceExternalID)
	if st.DeviceExternalID != "" && !extIDRe.MatchString(st.DeviceExternalID) {
		return st, errors.New("virtual device id may contain letters, digits and . _ : - (max 128)")
	}
	if len(st.Nodes) == 0 {
		return st, errors.New("at least one node is required")
	}
	if len(st.Nodes) > MaxNodes {
		return st, fmt.Errorf("at most %d nodes per connector", MaxNodes)
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
		n.Device = strings.TrimSpace(n.Device)
		if n.Device == st.DeviceExternalID {
			n.Device = ""
		}
		if n.Device != "" && !extIDRe.MatchString(n.Device) {
			return st, fmt.Errorf("node %d: device id may contain letters, digits and . _ : - (max 128)", i+1)
		}
		if seen[n.Device+"/"+n.Key] {
			return st, fmt.Errorf("node %d: duplicate key %q", i+1, n.Key)
		}
		seen[n.Device+"/"+n.Key] = true
		if n.Deadband != nil && (*n.Deadband < 0 || math.IsNaN(*n.Deadband) || math.IsInf(*n.Deadband, 0)) {
			return st, fmt.Errorf("node %d: deadband must be a number >= 0", i+1)
		}
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
	// sink replaces ingestion in unit tests (values by device: "" = the connector's own device).
	sink func(device string, values map[string]any)
}

func New(pool *pgxpool.Pool, ing *ingest.Ingest, log *slog.Logger, dir string) *Manager {
	return &Manager{pool: pool, ing: ing, log: log.With("component", "opcua"), dir: dir, workers: map[string]*worker{}, status: map[string]*Status{}, resync: make(chan struct{}, 1)}
}

// Start loads the connectors and keeps the set of workers in sync with the database.
func (m *Manager) Start(ctx context.Context) {
	go func() {
		if n, err := m.MigrateStored(ctx); err != nil {
			m.log.Error("stored passwords could not be encrypted", "security", true, "err", err)
		} else if n > 0 {
			m.log.Info("stored passwords encrypted", "security", true, "connectors", n)
		}
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
		st, err := ValidateSettings(OpenStored([]byte(raw)))
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

// Thumbprint is the SHA-1 thumbprint (lower-case hex) OPC UA tools show for a DER certificate.
func Thumbprint(der []byte) string {
	if len(der) == 0 {
		return ""
	}
	sum := sha1.Sum(der)
	return hex.EncodeToString(sum[:])
}

func (m *Manager) connect(ctx context.Context, st Settings) (*opcua.Client, string, error) {
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	eps, err := opcua.GetEndpoints(cctx, st.Endpoint)
	if err != nil {
		return nil, "", fmt.Errorf("endpoints: %w", err)
	}
	mode := ua.MessageSecurityModeFromString(st.SecurityMode)
	ep, err := opcua.SelectEndpoint(eps, st.SecurityPolicy, mode)
	if err != nil || ep == nil {
		return nil, "", fmt.Errorf("the server offers no endpoint with policy %s / mode %s", st.SecurityPolicy, st.SecurityMode)
	}
	thumb := Thumbprint(ep.ServerCertificate)
	if st.ServerThumbprint != "" && thumb != st.ServerThumbprint {
		if thumb == "" {
			thumb = "none"
		}
		return nil, thumb, fmt.Errorf("server certificate %s does not match the pinned thumbprint %s — check the server, then update the pin", thumb, st.ServerThumbprint)
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
			return nil, thumb, err
		}
		opts = append(opts, opcua.Certificate(cert), opcua.PrivateKey(key))
	}
	// The endpoint the server advertises often carries a hostname that only resolves inside its own network;
	// connect to the address the operator typed and keep the negotiated security settings.
	c, err := opcua.NewClient(st.Endpoint, opts...)
	if err != nil {
		return nil, thumb, err
	}
	if err := c.Connect(cctx); err != nil {
		return nil, thumb, fmt.Errorf("connect: %w", err)
	}
	return c, thumb, nil
}

// readLimit is the number of nodes sent in one Read request; larger lists are split so servers with a low
// MaxNodesPerRead (often 500 or 1000) answer all of them.
const readLimit = 250

// read reads all configured nodes once; values are keyed by "device/key" (see Settings.ref), bad nodes are
// reported by key.
func read(ctx context.Context, c *opcua.Client, st Settings) (map[string]any, []string, error) {
	values := map[string]any{}
	var bad []string
	for from := 0; from < len(st.Nodes); from += readLimit {
		to := min(from+readLimit, len(st.Nodes))
		ids := make([]*ua.ReadValueID, 0, to-from)
		for _, n := range st.Nodes[from:to] {
			id, _ := ua.ParseNodeID(n.NodeID)
			ids = append(ids, &ua.ReadValueID{NodeID: id, AttributeID: ua.AttributeIDValue})
		}
		rctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		resp, err := c.Read(rctx, &ua.ReadRequest{MaxAge: 0, TimestampsToReturn: ua.TimestampsToReturnSource, NodesToRead: ids})
		cancel()
		if err != nil {
			return nil, nil, err
		}
		if len(resp.Results) != len(ids) {
			return nil, nil, fmt.Errorf("server returned %d results for %d nodes", len(resp.Results), len(ids))
		}
		for i, r := range resp.Results {
			n := st.Nodes[from+i]
			v, problem := value(r, n)
			if problem != "" {
				bad = append(bad, st.label(n)+": "+problem)
				continue
			}
			values[st.ref(n)] = v
		}
	}
	return values, bad, nil
}

// value converts one data value of a node; a non-empty problem says why it cannot be used.
func value(r *ua.DataValue, n Node) (any, string) {
	if r == nil || r.Status != ua.StatusOK || r.Value == nil {
		st := ua.StatusBad
		if r != nil {
			st = r.Status
		}
		return nil, st.Error()
	}
	v, ok := convert(r.Value.Value(), n)
	if !ok {
		return nil, "unsupported type " + fmt.Sprintf("%T", r.Value.Value())
	}
	return v, ""
}

// ref is the key of a node's value in read results: the key alone for the connector's own device, "device/key"
// for nodes routed to another device.
func (st Settings) ref(n Node) string {
	if n.Device == "" {
		return n.Key
	}
	return n.Device + "/" + n.Key
}

func (st Settings) label(n Node) string { return st.ref(n) }

func convert(v any, n Node) (any, bool) {
	var f float64
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		return x, true
	case time.Time:
		return x.UTC().Format(time.RFC3339Nano), true
	case *ua.LocalizedText:
		if x == nil {
			return nil, false
		}
		return x.Text, true
	case float32:
		// the shortest decimal that is the same Float (21.5, not 21.500000953674316)
		f, _ = strconv.ParseFloat(strconv.FormatFloat(float64(x), 'g', -1, 32), 64)
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
func (m *Manager) Test(ctx context.Context, st Settings) (map[string]any, []string, string, error) {
	c, thumb, err := m.connect(ctx, st)
	if err != nil {
		return nil, nil, thumb, err
	}
	defer c.Close(ctx)
	values, bad, err := read(ctx, c, st)
	return values, bad, thumb, err
}

// ---- worker ---------------------------------------------------------------------------------------

type worker struct {
	m      *Manager
	row    connRow
	hash   string
	cancel context.CancelFunc
	st     *Status
	devs   map[string]*broker.Device // by external id; "" = the connector's own device
	cmu    sync.Mutex
	client *opcua.Client // live connection, reused for writes
}

func (w *worker) set(fn func(s *Status)) {
	w.m.mu.Lock()
	fn(w.st)
	w.m.mu.Unlock()
}

func (w *worker) setClient(c *opcua.Client) {
	w.cmu.Lock()
	w.client = c
	w.cmu.Unlock()
}

// live returns the open connection, or nil when the worker is reconnecting.
func (w *worker) live() *opcua.Client {
	w.cmu.Lock()
	defer w.cmu.Unlock()
	if w.client != nil && w.client.State() == opcua.Connected {
		return w.client
	}
	return nil
}

func (w *worker) run(ctx context.Context) {
	backoff := 5 * time.Second
	st := w.row.Settings
	w.set(func(s *Status) { s.Mode = st.Mode })
	for ctx.Err() == nil {
		c, thumb, err := w.m.connect(ctx, st)
		if err != nil {
			w.set(func(s *Status) {
				s.State, s.LastError, s.Thumbprint = "error", err.Error(), thumb
				s.Errors++
			})
			w.m.log.Warn("opcua connect failed", "connector", w.row.Name, "endpoint", st.Endpoint, "err", err)
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
		w.set(func(s *Status) { s.State, s.LastError, s.Thumbprint = "connected", "", thumb })
		w.m.log.Info("opcua connected", "connector", w.row.Name, "endpoint", st.Endpoint, "nodes", len(st.Nodes), "mode", st.Mode)
		w.setClient(c)
		if st.Mode == "subscribe" {
			w.subscribe(ctx, c)
		} else {
			w.pollLoop(ctx, c)
		}
		w.setClient(nil)
		_ = c.Close(context.Background())
		if ctx.Err() != nil {
			w.set(func(s *Status) { s.State = "stopped" })
			return
		}
		select {
		case <-ctx.Done():
			w.set(func(s *Status) { s.State = "stopped" })
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (w *worker) pollLoop(ctx context.Context, c *opcua.Client) {
	t := time.NewTicker(time.Duration(w.row.Settings.IntervalS) * time.Second)
	defer t.Stop()
	if !w.poll(ctx, c) {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if !w.poll(ctx, c) {
				return
			}
		}
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
	w.ingest(ctx, values, bad)
	return true
}

// subscribe creates one subscription with a monitored item per node and ingests every data change notification
// until the connection fails or the worker stops.
func (w *worker) subscribe(ctx context.Context, c *opcua.Client) {
	st := w.row.Settings
	interval := time.Duration(st.PublishingMs) * time.Millisecond
	notes := make(chan *opcua.PublishNotificationData, 64)
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sub, err := c.Subscribe(sctx, &opcua.SubscriptionParameters{Interval: interval}, notes)
	if err != nil {
		w.set(func(s *Status) { s.State, s.LastError = "error", "subscribe: "+err.Error(); s.Errors++ })
		return
	}
	defer func() { _ = sub.Cancel(context.Background()) }()
	var bad []string
	for from := 0; from < len(st.Nodes); from += readLimit {
		to := min(from+readLimit, len(st.Nodes))
		items := make([]*ua.MonitoredItemCreateRequest, 0, to-from)
		for i, n := range st.Nodes[from:to] {
			id, _ := ua.ParseNodeID(n.NodeID)
			req := opcua.NewMonitoredItemCreateRequestWithDefaults(id, ua.AttributeIDValue, uint32(from+i))
			req.RequestedParameters.SamplingInterval = float64(st.PublishingMs)
			if n.Deadband != nil && *n.Deadband > 0 {
				req.RequestedParameters.Filter = ua.NewExtensionObject(&ua.DataChangeFilter{Trigger: ua.DataChangeTriggerStatusValue, DeadbandType: uint32(ua.DeadbandTypeAbsolute), DeadbandValue: *n.Deadband})
			}
			items = append(items, req)
		}
		res, err := sub.Monitor(sctx, ua.TimestampsToReturnBoth, items...)
		if err != nil {
			w.set(func(s *Status) { s.State, s.LastError = "error", "monitor: "+err.Error(); s.Errors++ })
			return
		}
		for i, r := range res.Results {
			if r.StatusCode != ua.StatusOK {
				bad = append(bad, st.label(st.Nodes[from+i])+": "+r.StatusCode.Error())
			}
		}
	}
	w.set(func(s *Status) { s.BadNodes = bad })
	check := time.NewTicker(5 * time.Second)
	defer check.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-check.C:
			if c.State() != opcua.Connected {
				w.set(func(s *Status) { s.State, s.LastError = "error", "connection lost"; s.Errors++ })
				return
			}
		case n := <-notes:
			if n.Error != nil {
				w.set(func(s *Status) { s.State, s.LastError = "error", n.Error.Error(); s.Errors++ })
				w.m.log.Warn("opcua subscription failed", "connector", w.row.Name, "err", n.Error)
				return
			}
			dc, ok := n.Value.(*ua.DataChangeNotification)
			if !ok {
				continue
			}
			values := map[string]any{}
			var nbad []string
			for _, item := range dc.MonitoredItems {
				idx := int(item.ClientHandle)
				if idx < 0 || idx >= len(st.Nodes) {
					continue
				}
				node := st.Nodes[idx]
				v, problem := value(item.Value, node)
				if problem != "" {
					nbad = append(nbad, st.label(node)+": "+problem)
					continue
				}
				values[st.ref(node)] = v
			}
			w.set(func(s *Status) {
				s.State, s.LastError, s.LastPoll = "connected", "", time.Now()
				s.Notifications++
				if s.LastValues == nil {
					s.LastValues = map[string]any{}
				}
				for k, v := range values {
					s.LastValues[k] = v
				}
				if len(nbad) > 0 {
					s.BadNodes = append(bad, nbad...)
				}
			})
			w.ingest(ctx, values, nbad)
		}
	}
}

// ingest splits values by device and feeds each virtual device through the normal telemetry path.
func (w *worker) ingest(ctx context.Context, values map[string]any, bad []string) {
	if len(values) == 0 {
		return
	}
	st := w.row.Settings
	byDev := map[string]map[string]any{}
	defer func() {
		if w.m.sink != nil {
			for ext, vals := range byDev {
				w.m.sink(ext, vals)
			}
		}
	}()
	for _, n := range st.Nodes {
		v, ok := values[st.ref(n)]
		if !ok {
			continue
		}
		if byDev[n.Device] == nil {
			byDev[n.Device] = map[string]any{}
		}
		byDev[n.Device][n.Key] = v
	}
	if w.m.sink != nil {
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
		meta, _ := json.Marshal(map[string]any{"connector_id": w.row.ID, "connector": w.row.Name, "endpoint": st.Endpoint, "bad_nodes": bad})
		w.m.ing.IngestTelemetry(ctx, dev, "opcua", "opcua/"+w.row.Name, payload, string(meta))
	}
	_, _ = w.m.pool.Exec(ctx, `UPDATE core.connector SET last_seen_at = now(), messages = messages + 1 WHERE id = $1::uuid`, w.row.ID)
}

// ownExternalID is the external id of the connector's own virtual device.
func (w *worker) ownExternalID() string {
	if ext := w.row.Settings.DeviceExternalID; ext != "" {
		return ext
	}
	return "opcua-" + strings.ToLower(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, w.row.Name))
}

// device resolves (or creates) a virtual device of this connector: "" is its own device, anything else a node's
// device override.
func (w *worker) device(ctx context.Context, ext string) (*broker.Device, error) {
	w.cmu.Lock()
	if d, ok := w.devs[ext]; ok {
		w.cmu.Unlock()
		return d, nil
	}
	w.cmu.Unlock()
	name := w.row.Name + " (OPC UA)"
	extID := ext
	if ext == "" {
		extID = w.ownExternalID()
	} else {
		name = ext + " (OPC UA " + w.row.Name + ")"
	}
	d := &broker.Device{OrgID: w.row.OrgID, ExternalID: extID, Transport: "opcua"}
	err := w.m.pool.QueryRow(ctx, `SELECT id::text FROM core.device WHERE org_id = $1::uuid AND external_id = $2 AND revoked_at IS NULL`, w.row.OrgID, extID).Scan(&d.ID)
	if err != nil {
		if err := w.m.pool.QueryRow(ctx, `INSERT INTO core.device (org_id, external_id, name, transport) VALUES ($1::uuid, $2, $3, 'opcua') RETURNING id::text`, w.row.OrgID, extID, name).Scan(&d.ID); err != nil {
			return nil, err
		}
		w.m.log.Info("virtual device created for OPC UA connector", "connector", w.row.Name, "device", extID)
	}
	w.cmu.Lock()
	if w.devs == nil {
		w.devs = map[string]*broker.Device{}
	}
	w.devs[ext] = d
	w.cmu.Unlock()
	return d, nil
}

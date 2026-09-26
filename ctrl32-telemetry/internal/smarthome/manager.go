package smarthome

// The manager connects MQTT sources (accounts on the embedded broker and bridges to foreign brokers) with the
// discovery engine: discovery configs fill the inbox or are adopted into devices and entities, state messages update
// entities and are recorded as observations, and control actions become MQTT publishes (spec 005 T010, T013).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/ingest"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/live"
)

// Limits (spec edge cases).
const (
	MaxEntitiesPerDevice = 100
	MaxEntitiesPerSource = 2000
	commandConfirmTime   = 15 * time.Second
)

// Manager runs the smart-home integration.
type Manager struct {
	pool *pgxpool.Pool
	br   *broker.Broker
	ing  *ingest.Ingest
	hub  *live.Hub
	box  *iam.SecretBox
	log  *slog.Logger
	ctx  context.Context

	mu      sync.RWMutex
	sources map[string]*source // connector id -> source
	byNS    map[string]*source // namespace -> account source
	dropped atomic.Int64
}

type msg struct {
	topic   string
	payload []byte
}

type source struct {
	id, org, kind, name string
	ns                  string // accounts: "h/{12 hex}"
	prefix              string // discovery prefix
	adopt               string // auto | inbox
	site                string
	settings            map[string]any

	in     chan msg
	stop   context.CancelFunc
	bridge *bridge

	mu     sync.Mutex
	ents   map[string]*ent   // entity id -> entity
	topics map[string][]*ent // state topic -> entities
}

type ent struct {
	id, org, uniqueID, deviceID, dsID, dsKey, component, name, configTopic, status string
	e                                                                              *Entity
	state                                                                          string
	attrs                                                                          map[string]any
	available                                                                      bool
	err                                                                            string
	pending                                                                        []pendingCmd
}

type pendingCmd struct {
	id, method string
	exp        *Expectation
	until      time.Time
}

// New creates the manager; Start runs it.
func New(pool *pgxpool.Pool, br *broker.Broker, ing *ingest.Ingest, hub *live.Hub, secretKey string, log *slog.Logger) *Manager {
	box, _ := iam.NewSecretBox(secretKey)
	return &Manager{pool: pool, br: br, ing: ing, hub: hub, box: box, log: log.With("component", "smarthome"),
		sources: map[string]*source{}, byNS: map[string]*source{}}
}

// Start subscribes to the account namespaces, loads the sources and runs until ctx ends.
func (m *Manager) Start(ctx context.Context) error {
	m.ctx = ctx
	if m.br != nil {
		err := m.br.Server.Subscribe("h/+/#", 900, func(_ *mqtt.Client, _ packets.Subscription, pk packets.Packet) {
			parts := strings.SplitN(pk.TopicName, "/", 3)
			if len(parts) < 3 {
				return
			}
			m.mu.RLock()
			src := m.byNS[parts[0]+"/"+parts[1]]
			m.mu.RUnlock()
			if src != nil {
				src.enqueue(m, parts[2], pk.Payload)
			}
		})
		if err != nil {
			return err
		}
	}
	if err := m.Reload(ctx); err != nil {
		m.log.Error("smart-home sources not loaded", "err", err)
	}
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			m.mu.Lock()
			for _, s := range m.sources {
				s.stop()
			}
			m.mu.Unlock()
			return nil
		case <-t.C:
			m.expirePending()
		}
	}
}

func (s *source) enqueue(m *Manager, topic string, payload []byte) {
	select {
	case s.in <- msg{topic, append([]byte(nil), payload...)}:
	default:
		m.dropped.Add(1)
	}
}

// Reload reads the sources and their entities from the database (after changes through the API).
func (m *Manager) Reload(ctx context.Context) error {
	rows, err := m.pool.Query(ctx, `SELECT id::text, org_id::text, kind, name, settings FROM core.connector
		WHERE kind IN ('mqtt_account', 'mqtt_bridge') AND revoked_at IS NULL`)
	if err != nil {
		return err
	}
	type row struct {
		id, org, kind, name string
		settings            map[string]any
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.org, &r.kind, &r.name, &r.settings); err == nil {
			list = append(list, r)
		}
	}
	rows.Close()
	seen := map[string]bool{}
	for _, r := range list {
		seen[r.id] = true
		m.mu.RLock()
		old := m.sources[r.id]
		m.mu.RUnlock()
		prefix := str(r.settings, "discovery_prefix")
		if prefix == "" {
			prefix = "homeassistant"
		}
		adopt := str(r.settings, "adopt")
		if adopt != "auto" && adopt != "inbox" {
			adopt = map[string]string{"mqtt_account": "auto", "mqtt_bridge": "inbox"}[r.kind]
		}
		if old != nil && sameSettings(old.settings, r.settings) {
			old.name, old.adopt = r.name, adopt
			continue
		}
		if old != nil {
			old.stop()
		}
		sctx, cancel := context.WithCancel(m.ctx)
		src := &source{id: r.id, org: r.org, kind: r.kind, name: r.name, ns: str(r.settings, "namespace"), prefix: prefix, adopt: adopt,
			site: str(r.settings, "site_id"), settings: r.settings, in: make(chan msg, 5000), stop: cancel,
			ents: map[string]*ent{}, topics: map[string][]*ent{}}
		if err := m.loadEntities(ctx, src); err != nil {
			m.log.Error("entities not loaded", "source", r.name, "err", err)
		}
		m.mu.Lock()
		m.sources[r.id] = src
		if src.kind == "mqtt_account" && src.ns != "" {
			m.byNS[src.ns] = src
		}
		m.mu.Unlock()
		go m.worker(sctx, src)
		if src.kind == "mqtt_bridge" {
			src.bridge = m.startBridge(sctx, src)
		}
		if src.kind == "mqtt_account" {
			go m.replayRetained(src)
		}
	}
	m.mu.Lock()
	for id, s := range m.sources {
		if !seen[id] {
			s.stop()
			delete(m.sources, id)
			if s.ns != "" {
				delete(m.byNS, s.ns)
			}
		}
	}
	m.mu.Unlock()
	return nil
}

func sameSettings(a, b map[string]any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// replayRetained feeds the retained messages of an account's namespace (discovery configs, last states) to a source
// that was just loaded, so a restart does not wait for the devices to announce themselves again.
func (m *Manager) replayRetained(src *source) {
	if m.br == nil {
		return
	}
	for _, pk := range m.br.Server.Topics.Messages(src.ns + "/#") {
		src.enqueue(m, strings.TrimPrefix(pk.TopicName, src.ns+"/"), pk.Payload)
	}
}

func (m *Manager) loadEntities(ctx context.Context, src *source) error {
	rows, err := m.pool.Query(ctx, `SELECT e.id::text, e.org_id::text, e.unique_id, e.device_id::text, coalesce(e.datastream_id::text, ''), coalesce(ds.key, ''),
		e.component, e.name, e.config, e.config_topic, coalesce(e.state, ''), e.attributes, e.available, e.status, e.error
		FROM core.entity e LEFT JOIN core.datastream ds ON ds.id = e.datastream_id
		WHERE e.connector_id = $1::uuid AND e.status <> 'removed'`, src.id)
	if err != nil {
		return err
	}
	defer rows.Close()
	src.mu.Lock()
	defer src.mu.Unlock()
	for rows.Next() {
		x := &ent{}
		var cfg map[string]any
		if err := rows.Scan(&x.id, &x.org, &x.uniqueID, &x.deviceID, &x.dsID, &x.dsKey, &x.component, &x.name, &cfg, &x.configTopic, &x.state, &x.attrs, &x.available, &x.status, &x.err); err != nil {
			return err
		}
		if x.attrs == nil {
			x.attrs = map[string]any{}
		}
		x.e = NewEntity(x.component, cfg)
		src.ents[x.id] = x
	}
	src.reindex()
	return rows.Err()
}

func (s *source) reindex() {
	s.topics = map[string][]*ent{}
	for _, x := range s.ents {
		for _, t := range x.e.Topics() {
			s.topics[t] = append(s.topics[t], x)
		}
	}
}

// topicsToSubscribe lists the topics a bridge subscribes to.
func (s *source) topicsToSubscribe() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []string{s.prefix + "/#"}
	for t := range s.topics {
		if !strings.HasPrefix(t, s.prefix+"/") {
			out = append(out, t)
		}
	}
	return out
}

func (m *Manager) worker(ctx context.Context, src *source) {
	for {
		select {
		case <-ctx.Done():
			return
		case x := <-src.in:
			m.handle(ctx, src, x.topic, x.payload)
		}
	}
}

func (m *Manager) handle(ctx context.Context, src *source, topic string, payload []byte) {
	if strings.HasPrefix(topic, src.prefix+"/") && strings.HasSuffix(topic, "/config") {
		m.discover(ctx, src, topic, payload)
		return
	}
	src.mu.Lock()
	list := append([]*ent(nil), src.topics[topic]...)
	src.mu.Unlock()
	for _, x := range list {
		u, err := x.e.Apply(topic, payload)
		m.apply(ctx, src, x, u, err)
	}
}

// ---- discovery --------------------------------------------------------------------------------------------------

func (m *Manager) discover(ctx context.Context, src *source, topic string, payload []byte) {
	list, removed, err := ParseConfig(src.prefix, topic, payload)
	if errors.Is(err, ErrNotDiscovery) {
		return
	}
	if removed {
		m.removeConfig(ctx, src, topic)
		return
	}
	if err != nil {
		m.log.Warn("discovery config refused", "source", src.name, "topic", topic, "err", err)
		if len(list) == 0 {
			return
		}
	}
	var raw any
	if json.Unmarshal(payload, &raw) != nil {
		return
	}
	for _, d := range list {
		var discID, status string
		var devID *string
		err := m.pool.QueryRow(ctx, `INSERT INTO core.discovery (org_id, connector_id, device_key, name, manufacturer, model, configs)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, jsonb_build_object($7::text, $8::jsonb))
			ON CONFLICT (connector_id, device_key) DO UPDATE SET name = coalesce(nullif(EXCLUDED.name, ''), discovery.name),
			  manufacturer = coalesce(nullif(EXCLUDED.manufacturer, ''), discovery.manufacturer), model = coalesce(nullif(EXCLUDED.model, ''), discovery.model),
			  configs = discovery.configs || EXCLUDED.configs, last_seen = now()
			RETURNING id::text, status, device_id::text`,
			src.org, src.id, trunc(d.DeviceKey(), 255), trunc(d.DeviceName(), 128), trunc(d.Device.Manufacturer, 128), trunc(d.Device.Model, 128), topic, raw).
			Scan(&discID, &status, &devID)
		if err != nil {
			m.log.Error("discovery not stored", "source", src.name, "err", err)
			continue
		}
		switch {
		case status == "pending" && src.adopt == "auto":
			if _, err := m.Adopt(ctx, src.org, discID, "automatic adoption ("+src.name+")", "system", "", "smart home "+src.name); err != nil {
				m.log.Warn("automatic adoption failed", "source", src.name, "device", d.DeviceKey(), "err", err)
			}
			return // Adopt processed every config of the device, including this one
		case status == "adopted" && devID != nil:
			if err := m.upsertEntity(ctx, src, *devID, "", d); err != nil {
				m.log.Warn("entity not stored", "source", src.name, "entity", d.UniqueID, "err", err)
			}
		}
	}
}

func (m *Manager) removeConfig(ctx context.Context, src *source, topic string) {
	_, _ = m.pool.Exec(ctx, `UPDATE core.discovery SET configs = configs - $2 WHERE connector_id = $1::uuid AND configs ? $2`, src.id, topic)
	rows, err := m.pool.Query(ctx, `UPDATE core.entity SET status = 'removed' WHERE connector_id = $1::uuid AND config_topic = $2 AND status <> 'removed' RETURNING id::text`, src.id, topic)
	if err != nil {
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	src.mu.Lock()
	for _, id := range ids {
		delete(src.ents, id)
	}
	src.reindex()
	src.mu.Unlock()
	if len(ids) > 0 {
		m.log.Info("entities removed by their device", "source", src.name, "topic", topic, "count", len(ids))
	}
}

var extRe = regexp.MustCompile(`[^A-Za-z0-9_.:-]+`)

// Adopt turns a discovered device into a device with entities. actorType/actorID/actorName name who adopted it.
func (m *Manager) Adopt(ctx context.Context, org, discoveryID, reason, actorType, actorID, actorName string) (string, error) {
	var srcID, key, name, mf, model, status string
	var devID *string
	var configs map[string]json.RawMessage
	if err := m.pool.QueryRow(ctx, `SELECT connector_id::text, device_key, name, manufacturer, model, configs, status, device_id::text FROM core.discovery
		WHERE id = $1::uuid AND org_id = $2::uuid`, discoveryID, org).Scan(&srcID, &key, &name, &mf, &model, &configs, &status, &devID); err != nil {
		return "", errors.New("discovered device not found")
	}
	m.mu.RLock()
	src := m.sources[srcID]
	m.mu.RUnlock()
	if src == nil {
		return "", errors.New("the MQTT source of this device is not active")
	}
	var parsed []Discovered
	area := ""
	for topic, raw := range configs {
		list, _, err := ParseConfig(src.prefix, topic, raw)
		if err != nil && len(list) == 0 {
			continue
		}
		for _, d := range list {
			if area == "" {
				area = d.Device.SuggestedArea
			}
			parsed = append(parsed, d)
		}
	}
	if len(parsed) > MaxEntitiesPerDevice {
		parsed = parsed[:MaxEntitiesPerDevice]
	}
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	site, err := m.site(ctx, tx, src)
	if err != nil {
		return "", err
	}
	room := ""
	if area != "" {
		if room, err = ensureRoom(ctx, tx, org, site, area); err != nil {
			return "", err
		}
	}
	if name == "" {
		name = key
	}
	device := ""
	if devID != nil {
		device = *devID
	} else {
		ext := "ha-" + trunc(strings.Trim(extRe.ReplaceAllString(key, "_"), "_"), 50)
		for i := 0; i < 20; i++ {
			e := ext
			if i > 0 {
				e = fmt.Sprintf("%s-%d", ext, i+1)
			}
			err = tx.QueryRow(ctx, `INSERT INTO core.device (org_id, external_id, name, transport, status) VALUES ($1::uuid, $2, $3, 'mqtt_discovery',
				jsonb_build_object('manufacturer', $4::text, 'model', $5::text, 'source', $6::text, 'discovery_key', $7::text))
				ON CONFLICT (org_id, external_id) DO NOTHING RETURNING id::text`, org, e, trunc(name, 128), mf, model, src.name, key).Scan(&device)
			if err == nil {
				break
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return "", err
			}
		}
		if device == "" {
			return "", errors.New("no free device id")
		}
	}
	var roomPtr *string
	if room != "" {
		roomPtr = &room
	}
	if _, err := ingest.DeviceAsset(ctx, tx, org, device, trunc(name, 120), &site, roomPtr); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE core.discovery SET status = 'adopted', device_id = $2::uuid WHERE id = $1::uuid`, discoveryID, device); err != nil {
		return "", err
	}
	if _, _, err := audit.Append(ctx, tx, audit.Entry{OrgID: org, ActorType: actorType, ActorID: actorID, ActorName: actorName, Action: "device.adopt",
		EntityType: "device", EntityID: device, New: map[string]any{"name": name, "manufacturer": mf, "model": model, "source": src.name, "entities": len(parsed), "room": area},
		Reason: reason}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	for _, d := range parsed {
		if err := m.upsertEntity(ctx, src, device, room, d); err != nil {
			m.log.Warn("entity not stored", "source", src.name, "entity", d.UniqueID, "err", err)
		}
	}
	m.log.Info("device adopted", "source", src.name, "device", name, "entities", len(parsed))
	go m.replayRetained(src) // states published before the adoption
	return device, nil
}

// Ignore marks a discovered device as ignored.
func (m *Manager) Ignore(ctx context.Context, org, discoveryID string) error {
	tag, err := m.pool.Exec(ctx, `UPDATE core.discovery SET status = 'ignored' WHERE id = $1::uuid AND org_id = $2::uuid AND status = 'pending'`, discoveryID, org)
	if err != nil || tag.RowsAffected() == 0 {
		return errors.New("no discovered device waiting")
	}
	return nil
}

// site returns the site of a source (settings.site_id, else the organization's site "Home", created when missing).
func (m *Manager) site(ctx context.Context, tx pgx.Tx, src *source) (string, error) {
	if src.site != "" {
		var id string
		if tx.QueryRow(ctx, `SELECT id::text FROM core.site WHERE id = $1::uuid AND org_id = $2::uuid`, src.site, src.org).Scan(&id) == nil {
			return id, nil
		}
	}
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO core.site (org_id, name) VALUES ($1::uuid, 'Home') ON CONFLICT (org_id, name) DO UPDATE SET name = EXCLUDED.name RETURNING id::text`, src.org).Scan(&id)
	return id, err
}

func ensureRoom(ctx context.Context, tx pgx.Tx, org, site, name string) (string, error) {
	name = trunc(strings.TrimSpace(name), 120)
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM core.asset WHERE site_id = $1::uuid AND name = $2`, site, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	err = tx.QueryRow(ctx, `INSERT INTO core.asset (org_id, site_id, type, name) VALUES ($1::uuid, $2::uuid, 'room', $3) RETURNING id::text`, org, site, name).Scan(&id)
	return id, err
}

// recorded reports whether a component's states are kept as observations.
func recorded(component string) bool {
	switch component {
	case "button", "scene", "update":
		return false
	}
	return true
}

func (m *Manager) upsertEntity(ctx context.Context, src *source, deviceID, roomID string, d Discovered) error {
	src.mu.Lock()
	n := len(src.ents)
	src.mu.Unlock()
	if n >= MaxEntitiesPerSource {
		return fmt.Errorf("the source reached %d entities", MaxEntitiesPerSource)
	}
	e := NewEntity(d.Component, d.Config)
	status, errText := "active", ""
	if err := e.Check(); err != nil {
		status, errText = "unsupported", err.Error()
	}
	dsID := ""
	if recorded(d.Component) {
		kind, unit := "state", str(d.Config, "unit_of_measurement")
		if d.Component == "sensor" && (unit != "" || str(d.Config, "state_class") != "") || d.Component == "number" {
			kind = "gauge"
		}
		quantity := str(d.Config, "device_class")
		if quantity == "" {
			quantity = d.Component
		}
		srcKey := "entity:" + d.UniqueID
		key := d.ObjectID
		if d.NodeID != "" && strings.HasPrefix(key, ingest.NormalizeKey(d.NodeID)+"_") {
			key = strings.TrimPrefix(key, ingest.NormalizeKey(d.NodeID)+"_")
		}
		if key == "" {
			key = d.Component
		}
		created, err := ingest.CreateDatastreams(ctx, m.pool, src.org, deviceID,
			map[string]ingest.DatastreamSpec{srcKey: {Key: ingest.NormalizeKey(key), Kind: kind, Quantity: quantity, Unit: unit, Attrs: map[string]any{"entity": d.UniqueID, "component": d.Component}}},
			"entity "+d.Name+" discovered")
		if err != nil {
			return err
		}
		dsID = created[srcKey]
		if m.ing != nil {
			m.ing.InvalidateDevice(deviceID)
		}
	}
	cfg, _ := json.Marshal(d.Config)
	var room *string
	if roomID != "" {
		room = &roomID
	}
	var ds *string
	if dsID != "" {
		ds = &dsID
	}
	x := &ent{uniqueID: d.UniqueID, deviceID: deviceID, dsID: dsID, component: d.Component, name: d.Name, configTopic: d.Topic, status: status, e: e, err: errText}
	var attrs map[string]any
	var state *string
	err := m.pool.QueryRow(ctx, `INSERT INTO core.entity (org_id, connector_id, device_id, unique_id, component, name, config, config_topic, datastream_id, room_id, status, error)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9::uuid, coalesce($10::uuid, (SELECT parent_id FROM core.asset WHERE attributes->>'device_id' = $3::text LIMIT 1)), $11, $12)
		ON CONFLICT (connector_id, unique_id) DO UPDATE SET device_id = EXCLUDED.device_id, component = EXCLUDED.component, name = EXCLUDED.name,
		  config = EXCLUDED.config, config_topic = EXCLUDED.config_topic, datastream_id = coalesce(EXCLUDED.datastream_id, entity.datastream_id),
		  room_id = coalesce(entity.room_id, EXCLUDED.room_id), status = EXCLUDED.status, error = EXCLUDED.error
		RETURNING id::text, org_id::text, state, attributes, available`,
		src.org, src.id, deviceID, trunc(d.UniqueID, 255), d.Component, trunc(d.Name, 200), cfg, d.Topic, ds, room, status, errText).
		Scan(&x.id, &x.org, &state, &attrs, &x.available)
	if err != nil {
		return err
	}
	if state != nil {
		x.state = *state
	}
	x.attrs = attrs
	if x.attrs == nil {
		x.attrs = map[string]any{}
	}
	src.mu.Lock()
	if old := src.ents[x.id]; old != nil {
		x.pending = old.pending
	}
	src.ents[x.id] = x
	src.reindex()
	src.mu.Unlock()
	if src.bridge != nil {
		src.bridge.subscribe(e.Topics())
	}
	return nil
}

// ---- states -----------------------------------------------------------------------------------------------------

// numericState maps a state to the number recorded for it.
func numericState(component, state string) (*float64, bool) {
	if f, ok := NumericState(state); ok && (component == "sensor" || component == "number") {
		return &f, true
	}
	one, zero := 1.0, 0.0
	switch state {
	case "on", "open", "locked", "home", "opening":
		return &one, false
	case "off", "closed", "unlocked", "not_home", "closing":
		return &zero, false
	}
	return nil, false
}

func (m *Manager) apply(ctx context.Context, src *source, x *ent, u Update, applyErr error) {
	src.mu.Lock()
	changed, stateChanged := false, false
	if u.Available != nil && *u.Available != x.available {
		x.available = *u.Available
		changed = true
	}
	newState := ""
	if u.State != nil {
		newState = *u.State
		if newState != x.state {
			x.state = newState
			stateChanged, changed = true, true
		}
	}
	for k, v := range u.Attrs {
		if ToString(x.attrs[k]) != ToString(v) {
			x.attrs[k] = v
			changed = true
		}
	}
	errText := ""
	if applyErr != nil {
		errText = applyErr.Error()
	}
	errChanged := errText != x.err
	x.err = errText
	state, attrs, avail := x.state, cloneMap(x.attrs), x.available
	var acked []pendingCmd
	if changed {
		kept := x.pending[:0]
		for _, p := range x.pending {
			if p.exp.Matches(state, attrs) {
				acked = append(acked, p)
			} else {
				kept = append(kept, p)
			}
		}
		x.pending = kept
	}
	src.mu.Unlock()

	if changed || errChanged {
		a, _ := json.Marshal(attrs)
		_, err := m.pool.Exec(ctx, `UPDATE core.entity SET state = $2, attributes = $3::jsonb, available = $4, error = $5, last_updated = now(),
			last_changed = CASE WHEN $6 THEN now() ELSE last_changed END WHERE id = $1::uuid`, x.id, nullStr(state), string(a), avail, errText, stateChanged)
		if err != nil {
			m.log.Error("entity state not stored", "entity", x.uniqueID, "err", err)
		}
	}
	// history: numeric sensors every report, other states when they change
	if x.dsID != "" && m.ing != nil && u.State != nil && newState != "" {
		num, isNum := numericState(x.component, newState)
		if isNum || stateChanged {
			var text *string
			if !isNum {
				text = &newState
			}
			if err := m.ing.StoreState(ctx, x.org, x.dsID, x.dsKey, num, text, time.Now()); err != nil {
				m.log.Error("entity state not recorded", "entity", x.uniqueID, "err", err)
			}
		}
	}
	if changed && m.hub != nil {
		m.hub.Entity(x.org, x.id, x.deviceID, state, attrs, avail)
	}
	for _, p := range acked {
		m.finishCommand(ctx, x, p, "acked", map[string]any{"state": state}, "")
	}
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// ---- commands ---------------------------------------------------------------------------------------------------

// CommandArgs are the arguments of an entity command (device_command.args).
type CommandArgs struct {
	Entity string `json:"entity"`
	Params
}

// ErrNoSource means the entity's MQTT source is not connected.
var ErrNoSource = errors.New("the entity's MQTT connection is not available")

func (m *Manager) find(deviceID, entityID string) (*source, *ent) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sources {
		s.mu.Lock()
		x := s.ents[entityID]
		s.mu.Unlock()
		if x != nil && x.deviceID == deviceID {
			return s, x
		}
	}
	return nil, nil
}

// Dispatch executes a recorded command of a mqtt_discovery device: it publishes the payloads and returns "sent"
// (waiting for the state) or "acked" (optimistic entities, buttons). The command row is updated by the caller for the
// immediate outcome and by the manager when the state confirms it.
func (m *Manager) Dispatch(ctx context.Context, deviceID, commandID, method string, rawArgs json.RawMessage) (status string, result any, errText string) {
	var args CommandArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil || args.Entity == "" {
		return "failed", nil, "args.entity is required"
	}
	src, x := m.find(deviceID, args.Entity)
	if x == nil {
		return "failed", nil, "entity not found on this device"
	}
	src.mu.Lock()
	available, state, status0 := x.available, x.state, x.status
	src.mu.Unlock()
	if status0 != "active" {
		return "failed", nil, "entity is not supported: " + x.err
	}
	if !available {
		return "failed", nil, "entity is unavailable"
	}
	msgs, exp, err := x.e.Command(method, args.Params, state)
	if err != nil {
		return "failed", nil, err.Error()
	}
	for _, p := range msgs {
		if err := m.publish(ctx, src, p); err != nil {
			return "failed", nil, err.Error()
		}
	}
	out := make([]map[string]any, len(msgs))
	for i, p := range msgs {
		out[i] = map[string]any{"topic": p.Topic, "payload": p.Payload}
	}
	if exp == nil || x.e.Optimistic() {
		if exp != nil && exp.Attr == "" {
			m.apply(ctx, src, x, Update{State: &exp.Value}, nil)
		}
		return "acked", map[string]any{"published": out, "optimistic": true}, ""
	}
	src.mu.Lock()
	x.pending = append(x.pending, pendingCmd{id: commandID, method: method, exp: exp, until: time.Now().Add(commandConfirmTime)})
	src.mu.Unlock()
	return "sent", map[string]any{"published": out}, ""
}

func (m *Manager) publish(ctx context.Context, src *source, p Publish) error {
	switch src.kind {
	case "mqtt_account":
		if m.br == nil {
			return ErrNoSource
		}
		if !m.br.AccountConnected(src.id) {
			return errors.New("no device is connected with this MQTT account")
		}
		return m.br.Publish(src.ns+"/"+p.Topic, []byte(p.Payload), p.Retain)
	case "mqtt_bridge":
		if src.bridge == nil || !src.bridge.connected.Load() {
			return ErrNoSource
		}
		return src.bridge.publish(ctx, p)
	}
	return ErrNoSource
}

func (m *Manager) finishCommand(ctx context.Context, x *ent, p pendingCmd, status string, result any, errText string) {
	b, _ := json.Marshal(result)
	var org string
	err := m.pool.QueryRow(ctx, `UPDATE core.device_command SET status = $2, acked_at = now(), result = coalesce(result, '{}'::jsonb) || $3::jsonb, error = nullif($4, '')
		WHERE id = $1::uuid AND status IN ('queued', 'sent') RETURNING org_id::text`, p.id, status, string(b), errText).Scan(&org)
	if err != nil {
		return
	}
	if m.hub != nil {
		m.hub.Command(org, x.deviceID, p.id, p.method, status, map[string]any{"result": result, "error": errText})
	}
}

func (m *Manager) expirePending() {
	now := time.Now()
	m.mu.RLock()
	var srcs []*source
	for _, s := range m.sources {
		srcs = append(srcs, s)
	}
	m.mu.RUnlock()
	for _, s := range srcs {
		var late []struct {
			x *ent
			p pendingCmd
		}
		s.mu.Lock()
		for _, x := range s.ents {
			kept := x.pending[:0]
			for _, p := range x.pending {
				if now.After(p.until) {
					late = append(late, struct {
						x *ent
						p pendingCmd
					}{x, p})
				} else {
					kept = append(kept, p)
				}
			}
			x.pending = kept
		}
		s.mu.Unlock()
		for _, l := range late {
			m.finishCommand(m.ctx, l.x, l.p, "failed", nil, "the device did not report the requested state within 15 s")
		}
	}
}

// ---- status -----------------------------------------------------------------------------------------------------

// SourceStatus describes a source for the API.
type SourceStatus struct {
	Connected bool   `json:"connected"`
	Error     string `json:"error,omitempty"`
	Entities  int    `json:"entities"`
	Dropped   int64  `json:"dropped,omitempty"`
}

// Status returns the live status of a source.
func (m *Manager) Status(connectorID string) SourceStatus {
	m.mu.RLock()
	s := m.sources[connectorID]
	m.mu.RUnlock()
	if s == nil {
		return SourceStatus{}
	}
	s.mu.Lock()
	st := SourceStatus{Entities: len(s.ents), Dropped: m.dropped.Load()}
	s.mu.Unlock()
	switch s.kind {
	case "mqtt_account":
		st.Connected = m.br != nil && m.br.AccountConnected(s.id)
	case "mqtt_bridge":
		if s.bridge != nil {
			st.Connected = s.bridge.connected.Load()
			if e, ok := s.bridge.lastErr.Load().(string); ok && !st.Connected {
				st.Error = e
			}
		}
	}
	return st
}

// NewNamespace returns a random account namespace "h/{12 hex}".
func NewNamespace(seed string) string {
	sum := sha256.Sum256([]byte(seed + time.Now().String()))
	return "h/" + hex.EncodeToString(sum[:6])
}

func trunc(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// Check validates a control action without sending it; it returns the entity's device.
func (m *Manager) Check(entityID, service string, p Params) (deviceID string, err error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sources {
		s.mu.Lock()
		x := s.ents[entityID]
		var state string
		if x != nil {
			state = x.state
		}
		s.mu.Unlock()
		if x == nil {
			continue
		}
		if x.status != "active" {
			return x.deviceID, errors.New("entity is not supported: " + x.err)
		}
		if _, _, err := x.e.Command(service, p, state); err != nil {
			return x.deviceID, err
		}
		return x.deviceID, nil
	}
	return "", errors.New("entity not found or its source is not active")
}

// Services returns the control actions of an entity (nil when unknown or read-only).
func (m *Manager) Services(entityID string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sources {
		s.mu.Lock()
		x := s.ents[entityID]
		s.mu.Unlock()
		if x != nil {
			return x.e.Services()
		}
	}
	return nil
}

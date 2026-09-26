package flows

import (
	"container/heap"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Host is what flows act on; implemented by the server (commands, derived values, notifications, incidents, HTTP).
type Host interface {
	DatastreamName(ctx context.Context, org, id string) (name, key string)
	DeviceName(ctx context.Context, org, id string) string
	StoreDerived(ctx context.Context, org, datastreamID, key string, value float64, at time.Time) error
	// RunCommand sends a device command; reason and issuer name the flow as the origin (FR-005).
	RunCommand(ctx context.Context, org, deviceID, method string, args json.RawMessage, flowID, issuer, reason string) (id, status, errText string, err error)
	// EntityCommand controls a smart-home entity; issuer and reason name the flow as the origin.
	EntityCommand(ctx context.Context, org, entityID, service string, params map[string]any, flowID, issuer, reason string) (id, status, errText string, err error)
	// MQTTSubscribe delivers the messages of an MQTT source's topic filter until the returned function is called.
	MQTTSubscribe(org, source, filter string, fn func(topic string, payload []byte)) (func(), error)
	// MQTTPublish publishes a message on an MQTT source.
	MQTTPublish(ctx context.Context, org, source, topic string, payload []byte, retain bool) error
	// SceneActivate runs a scene's actions as entity commands with the flow as origin; it returns how many
	// commands were issued and how many failed.
	SceneActivate(ctx context.Context, org, sceneID, flowID, issuer, reason string) (ok, failed int, err error)
	// EntityName is the display name of an entity.
	EntityName(ctx context.Context, org, id string) string
	// SetAttribute sets one shared attribute of a device; issuer and reason name the flow as the origin.
	SetAttribute(ctx context.Context, org, deviceID, key string, value any, issuer, reason string) error
	Notify(ctx context.Context, org, channelID, flowID, subject, text string, payload map[string]any) error
	Incident(ctx context.Context, org, action, severity, title, dedup string, detail map[string]any) error
	HTTP(ctx context.Context, org, method, url string, body []byte) (int, string, error)
	Location(ctx context.Context, org string) *time.Location
}

// Limits of the engine.
const (
	QueueSize    = 1000
	DebugEntries = 500
	maxDepth     = 250
)

// DebugEntry is one line of a flow's debug panel.
type DebugEntry struct {
	Seq  int64     `json:"seq"`
	TS   time.Time `json:"ts"`
	Node string    `json:"node"`
	Kind string    `json:"kind"` // debug | error | action | dry_action | drop | info
	Text string    `json:"text"`
	Msg  any       `json:"msg,omitempty"`
	Dry  bool      `json:"dry,omitempty"`
}

type ring struct {
	mu   sync.Mutex
	seq  int64
	list []DebugEntry
}

func (r *ring) add(e DebugEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	e.Seq = r.seq
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	r.list = append(r.list, e)
	if len(r.list) > DebugEntries {
		r.list = r.list[len(r.list)-DebugEntries:]
	}
}

func (r *ring) since(after int64) []DebugEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []DebugEntry{}
	for _, e := range r.list {
		if e.Seq > after {
			out = append(out, e)
		}
	}
	return out
}

// Engine runs the deployed flows of all organizations.
type Engine struct {
	pool   *pgxpool.Pool
	host   Host
	log    *slog.Logger
	secret []byte

	mu    sync.RWMutex
	ctx   context.Context
	flows map[string]*flow // org/slug
	byDS  map[string][]trigRef
	hooks map[string]trigRef // token → webhook node
	ents  sync.Map           // entity id → last state (for the entity trigger's from/to)
	debug map[string]*ring

	queued, dropped, processed, errored atomic.Int64
}

type trigRef struct {
	f    *flow
	node string
}

// New creates the engine; Start loads the deployed flows.
func New(pool *pgxpool.Pool, host Host, secret string, log *slog.Logger) *Engine {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("ctrl32 flow webhooks"))
	return &Engine{pool: pool, host: host, log: log.With("component", "flows"), secret: mac.Sum(nil),
		flows: map[string]*flow{}, byDS: map[string][]trigRef{}, hooks: map[string]trigRef{}, debug: map[string]*ring{}}
}

func key(org, slug string) string { return org + "/" + slug }

// SetHost connects the engine to the server (set before Start).
func (e *Engine) SetHost(h Host) { e.host = h }

// Start loads and runs every deployed flow; flows stop when ctx ends (their state is saved).
func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	e.ctx = ctx
	e.mu.Unlock()
	rows, err := e.pool.Query(ctx, `SELECT org_id::text, slug FROM core.flow WHERE valid_to IS NULL AND state = 'deployed'`)
	if err != nil {
		e.log.Error("load flows", "err", err)
		return
	}
	var list [][2]string
	for rows.Next() {
		var o, s string
		if rows.Scan(&o, &s) == nil {
			list = append(list, [2]string{o, s})
		}
	}
	rows.Close()
	for _, l := range list {
		if err := e.Reload(ctx, l[0], l[1]); err != nil {
			e.log.Error("start flow", "org", l[0], "flow", l[1], "err", err)
		}
	}
	e.log.Info("flows started", "count", len(list))
	go func() {
		<-ctx.Done()
		e.mu.Lock()
		fs := make([]*flow, 0, len(e.flows))
		for _, f := range e.flows {
			fs = append(fs, f)
		}
		e.mu.Unlock()
		for _, f := range fs {
			f.stopWait()
		}
	}()
}

// Reload (re)starts a flow from its current stored version, or stops it when it is not deployed.
func (e *Engine) Reload(ctx context.Context, org, slug string) error {
	var id, name, state, createdBy string
	var version int
	var raw []byte
	err := e.pool.QueryRow(ctx, `SELECT id::text, name, version, state, spec, coalesce(created_by::text, '') FROM core.flow WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL`,
		org, slug).Scan(&id, &name, &version, &state, &raw, &createdBy)
	e.stop(org, slug)
	if err != nil || state != "deployed" {
		return nil
	}
	var sp Spec
	if err := json.Unmarshal(raw, &sp); err != nil {
		return err
	}
	f, err := e.newFlow(org, slug, id, name, version, createdBy, &sp, false)
	if err != nil {
		return err
	}
	f.loadState(ctx)
	e.register(f)
	e.mu.RLock()
	base := e.ctx
	e.mu.RUnlock()
	if base == nil {
		base = context.Background()
	}
	go f.run(base)
	return nil
}

func (e *Engine) stop(org, slug string) {
	e.mu.Lock()
	f := e.flows[key(org, slug)]
	if f != nil {
		delete(e.flows, key(org, slug))
		e.unindex(f)
	}
	e.mu.Unlock()
	if f != nil {
		for _, un := range f.unsubs {
			un()
		}
		f.stopWait()
	}
}

// Stop stops a flow (disable, delete).
func (e *Engine) Stop(org, slug string) { e.stop(org, slug) }

func (e *Engine) register(f *flow) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.flows[key(f.org, f.slug)] = f
	for _, n := range f.spec.Nodes {
		if n.Disabled {
			continue
		}
		switch n.Type {
		case "ds_value":
			ds, _ := n.Settings["datastream"].(string)
			e.byDS[ds] = append(e.byDS[ds], trigRef{f, n.ID})
		case "webhook":
			e.hooks[e.HookToken(f.org, f.slug, n.ID)] = trigRef{f, n.ID}
		case "mqtt_in":
			if e.host == nil {
				continue
			}
			src, _ := n.Settings["source"].(string)
			filter, _ := n.Settings["topic"].(string)
			node := n.ID
			un, err := e.host.MQTTSubscribe(f.org, src, filter, func(topic string, payload []byte) {
				f.enqueue(qitem{node: node, msg: mqttMessage(src, topic, payload)})
			})
			if err != nil {
				f.info(node, "MQTT in not subscribed: "+err.Error())
				continue
			}
			f.unsubs = append(f.unsubs, un)
		}
	}
}

func (e *Engine) unindex(f *flow) {
	for ds, refs := range e.byDS {
		keep := refs[:0]
		for _, r := range refs {
			if r.f != f {
				keep = append(keep, r)
			}
		}
		if len(keep) == 0 {
			delete(e.byDS, ds)
		} else {
			e.byDS[ds] = keep
		}
	}
	for t, r := range e.hooks {
		if r.f == f {
			delete(e.hooks, t)
		}
	}
}

// HookToken is the secret part of a webhook node's URL (derived from the server's secret key).
func (e *Engine) HookToken(org, slug, node string) string {
	mac := hmac.New(sha256.New, e.secret)
	mac.Write([]byte(org + "\x00" + slug + "\x00" + node))
	return "fh_" + hex.EncodeToString(mac.Sum(nil))[:40]
}

// startTest runs a flow without the database (unit tests).
func (e *Engine) startTest(ctx context.Context, sp *Spec, name string) (*flow, error) {
	f, err := e.newFlow("org", "test", "flow-id", name, 3, "", sp, false)
	if err != nil {
		return nil, err
	}
	e.register(f)
	go f.run(ctx)
	return f, nil
}

func (e *Engine) ringOf(org, slug string) *ring {
	e.mu.Lock()
	defer e.mu.Unlock()
	r := e.debug[key(org, slug)]
	if r == nil {
		r = &ring{}
		e.debug[key(org, slug)] = r
	}
	return r
}

// Debug returns the debug entries of a flow after seq.
func (e *Engine) Debug(org, slug string, after int64) []DebugEntry {
	return e.ringOf(org, slug).since(after)
}

// NodeStatus is the counters of one node (FR-009).
type NodeStatus struct {
	In     int64      `json:"in"`
	Out    int64      `json:"out"`
	Errors int64      `json:"errors"`
	Last   *time.Time `json:"last,omitempty"`
}

// FlowStatus is the running state of a flow.
type FlowStatus struct {
	Running bool                  `json:"running"`
	Version int                   `json:"version,omitempty"`
	Queue   int                   `json:"queue"`
	Dropped int64                 `json:"dropped"`
	Nodes   map[string]NodeStatus `json:"nodes"`
}

// Status returns counters of a running flow.
func (e *Engine) Status(org, slug string) FlowStatus {
	e.mu.RLock()
	f := e.flows[key(org, slug)]
	e.mu.RUnlock()
	st := FlowStatus{Nodes: map[string]NodeStatus{}}
	if f == nil {
		return st
	}
	st.Running, st.Version, st.Queue, st.Dropped = true, f.version, len(f.q), f.dropped.Load()
	for id, n := range f.nodes {
		ns := NodeStatus{In: n.in.Load(), Out: n.out.Load(), Errors: n.errs.Load()}
		if l := n.last.Load(); l > 0 {
			t := time.UnixMilli(l).UTC()
			ns.Last = &t
		}
		st.Nodes[id] = ns
	}
	return st
}

// Totals for metrics.
func (e *Engine) Totals() (running int, processed, errors, dropped int64) {
	e.mu.RLock()
	running = len(e.flows)
	e.mu.RUnlock()
	return running, e.processed.Load(), e.errored.Load(), e.dropped.Load()
}

// ---- a running flow ---------------------------------------------------------------------------------------------

type rnode struct {
	spec  *NodeSpec
	def   *NodeDef
	exprs map[string]*Expr
	rules []*Expr
	st    map[string]any // node memory (only the flow goroutine touches it)
	work  chan func()    // action nodes: jobs executed in order by one worker
	dirty bool
	in    atomic.Int64
	out   atomic.Int64
	errs  atomic.Int64
	last  atomic.Int64
}

type qitem struct {
	node string
	msg  *Message
	dry  bool
	fn   func()
}

type flow struct {
	e                   *Engine
	org, slug, id, name string
	owner               string
	version             int
	spec                *Spec
	nodes               map[string]*rnode
	outs                map[string][][]string
	q                   chan qitem
	quit                chan struct{}
	done                chan struct{}
	timers              timerHeap
	mem                 map[string]Value
	memDirty            bool
	loc                 *time.Location
	dropped             atomic.Int64
	temp                bool // a test instance of an edited flow: everything is a dry run
	depth               int
	slots               chan struct{}
	ring                *ring
	stopOnce            sync.Once
	unsubs              []func() // MQTT in subscriptions
	lastDropNote        time.Time
}

func (e *Engine) newFlow(org, slug, id, name string, version int, owner string, sp *Spec, temp bool) (*flow, error) {
	f := &flow{e: e, org: org, slug: slug, id: id, name: name, version: version, owner: owner, spec: sp, nodes: map[string]*rnode{},
		outs: map[string][][]string{}, q: make(chan qitem, QueueSize), quit: make(chan struct{}), done: make(chan struct{}), mem: map[string]Value{},
		temp: temp, ring: e.ringOf(org, slug)}
	f.loc = time.UTC
	if e.host != nil {
		if l := e.host.Location(context.Background(), org); l != nil {
			f.loc = l
		}
	}
	for i := range sp.Nodes {
		n := &sp.Nodes[i]
		d, ok := Def(n.Type)
		if !ok {
			return nil, fmt.Errorf("unknown node type %s", n.Type)
		}
		rn := &rnode{spec: n, def: d, exprs: map[string]*Expr{}, st: map[string]any{}}
		for _, s := range d.Settings {
			if s.Kind == "expr" {
				if src, _ := n.Settings[s.Name].(string); src != "" {
					ex, err := Parse(src)
					if err != nil {
						return nil, fmt.Errorf("%s: %s: %w", n.ID, s.Name, err)
					}
					rn.exprs[s.Name] = ex
				}
			}
		}
		if n.Type == "switch" {
			list, _ := toStrings(n.Settings["rules"])
			for _, r := range list {
				ex, err := Parse(r)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", n.ID, err)
				}
				rn.rules = append(rn.rules, ex)
			}
		}
		f.nodes[n.ID] = rn
		f.outs[n.ID] = make([][]string, outputsOf(n))
	}
	for _, w := range sp.Wires {
		if o, ok := f.outs[w.From]; ok && w.Port < len(o) {
			o[w.Port] = append(o[w.Port], w.To)
		}
	}
	return f, nil
}

func (f *flow) stopWait() {
	f.stopOnce.Do(func() { close(f.quit) })
	select {
	case <-f.done:
	case <-time.After(10 * time.Second):
	}
}

// enqueue hands a message to the flow without blocking the producer (FR queue limit: drop and count).
func (f *flow) enqueue(it qitem) bool {
	select {
	case f.q <- it:
		f.e.queued.Add(1)
		return true
	default:
		f.dropped.Add(1)
		f.e.dropped.Add(1)
		return false
	}
}

func (f *flow) run(ctx context.Context) {
	defer close(f.done)
	f.info("", fmt.Sprintf("flow started (version %d)", f.version))
	f.startSchedules()
	t := time.NewTimer(time.Hour)
	defer t.Stop()
	save := time.NewTicker(30 * time.Second)
	defer save.Stop()
	for {
		next := time.Hour
		if len(f.timers) > 0 {
			next = time.Until(f.timers[0].at)
			if next < 0 {
				next = 0
			}
		}
		if !t.Stop() {
			select {
			case <-t.C:
			default:
			}
		}
		t.Reset(next)
		select {
		case <-ctx.Done():
			f.saveState(context.Background())
			return
		case <-f.quit:
			f.saveState(context.Background())
			return
		case it := <-f.q:
			f.handle(it)
		case <-t.C:
			now := time.Now()
			for len(f.timers) > 0 && !f.timers[0].at.After(now) {
				tm := heap.Pop(&f.timers).(*timer)
				tm.fn()
			}
		case <-save.C:
			f.saveState(ctx)
			if f.dropped.Load() > 0 && time.Since(f.lastDropNote) > time.Minute {
				f.lastDropNote = time.Now()
				f.ring.add(DebugEntry{Kind: "drop", Text: fmt.Sprintf("%d messages dropped so far: the flow receives more than it can process (queue %d)", f.dropped.Load(), QueueSize)})
			}
		}
	}
}

func (f *flow) handle(it qitem) {
	if it.fn != nil {
		it.fn()
		return
	}
	n := f.nodes[it.node]
	if n == nil || n.spec.Disabled {
		return
	}
	f.process(n, it.msg, it.dry)
}

// after runs fn in the flow goroutine after d.
func (f *flow) after(d time.Duration, fn func()) {
	heap.Push(&f.timers, &timer{at: time.Now().Add(d), fn: fn})
}

// emit passes a message to every node wired to the port (depth-first, in the flow goroutine).
func (f *flow) emit(n *rnode, port int, m *Message, dry bool) {
	outs := f.outs[n.spec.ID]
	if port >= len(outs) {
		return
	}
	n.out.Add(1)
	targets := outs[port]
	for i, to := range targets {
		t := f.nodes[to]
		if t == nil || t.spec.Disabled {
			continue
		}
		mm := m
		if i < len(targets)-1 {
			mm = m.Clone()
		}
		f.depth++
		if f.depth > maxDepth {
			f.fail(n, m, errors.New("message path too long"))
		} else {
			f.process(t, mm, dry)
		}
		f.depth--
	}
}

func (f *flow) fail(n *rnode, m *Message, err error) {
	n.errs.Add(1)
	f.e.errored.Add(1)
	f.ring.add(DebugEntry{Node: n.spec.ID, Kind: "error", Text: err.Error(), Msg: m})
}

func (f *flow) info(node, text string) { f.ring.add(DebugEntry{Node: node, Kind: "info", Text: text}) }

// origin names the flow and the trigger of a message (audit reasons).
func (f *flow) origin(n *rnode, m *Message) string {
	s := fmt.Sprintf("flow %s v%d node %s", f.name, f.version, n.spec.ID)
	if m != nil && m.Trigger != "" {
		s += " (" + m.Trigger + ")"
	}
	return s
}

// ---- state persistence (research R10) ---------------------------------------------------------------------------

func (f *flow) loadState(ctx context.Context) {
	if f.e.pool == nil {
		return
	}
	rows, err := f.e.pool.Query(ctx, `SELECT node_id, state FROM core.flow_state WHERE org_id = $1::uuid AND flow_slug = $2`, f.org, f.slug)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var node string
		var st map[string]any
		if rows.Scan(&node, &st) != nil {
			continue
		}
		if node == "_flow" {
			for k, v := range st {
				f.mem[k] = v
			}
			continue
		}
		if n := f.nodes[node]; n != nil && (n.spec.Type == "counter" || n.spec.Type == "latch") {
			for k, v := range st {
				n.st[k] = v
			}
		}
	}
}

func (f *flow) saveState(ctx context.Context) {
	if f.temp || f.e.pool == nil {
		return
	}
	save := func(node string, st map[string]any) {
		b, _ := json.Marshal(st)
		_, err := f.e.pool.Exec(ctx, `INSERT INTO core.flow_state (org_id, flow_slug, node_id, state, updated_at) VALUES ($1::uuid, $2, $3, $4::jsonb, now())
			ON CONFLICT (org_id, flow_slug, node_id) DO UPDATE SET state = EXCLUDED.state, updated_at = now()`, f.org, f.slug, node, string(b))
		if err != nil {
			f.e.log.Warn("save flow state", "flow", f.slug, "node", node, "err", err)
		}
	}
	if f.memDirty {
		m := map[string]any{}
		for k, v := range f.mem {
			m[k] = v
		}
		save("_flow", m)
		f.memDirty = false
	}
	for id, n := range f.nodes {
		if n.dirty {
			save(id, n.st)
			n.dirty = false
		}
	}
}

// ---- timers -------------------------------------------------------------------------------------------------------

type timer struct {
	at time.Time
	fn func()
}

type timerHeap []*timer

func (h timerHeap) Len() int           { return len(h) }
func (h timerHeap) Less(i, j int) bool { return h[i].at.Before(h[j].at) }
func (h timerHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *timerHeap) Push(x any)        { *h = append(*h, x.(*timer)) }
func (h *timerHeap) Pop() any          { old := *h; n := len(old); x := old[n-1]; *h = old[:n-1]; return x }
func (h timerHeap) sorted() []time.Time {
	out := []time.Time{}
	for _, t := range h {
		out = append(out, t.at)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

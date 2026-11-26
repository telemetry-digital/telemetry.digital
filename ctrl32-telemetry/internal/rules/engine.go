package rules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/alarmmgmt"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/escalate"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/ingest"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/live"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/notify"
)

// CommandRequest is a device command issued by a rule.
type CommandRequest struct {
	OrgID, DeviceID, Method string
	Args                    json.RawMessage
	RuleID, RuleName        string
	// Flows (spec 004): the flow version as origin; Issuer and Reason replace the rule's.
	FlowID, Issuer, Reason string
}

// CommandResult is what the command layer reports back.
type CommandResult struct {
	ID, Status string
	Result     any
	Error      string
}

// Commander executes device commands (implemented by the HTTP layer, which owns the connectors and the broker).
type Commander interface {
	RunCommand(ctx context.Context, c CommandRequest) (CommandResult, error)
}

// Deriver stores derived observations (implemented by ingest).
type Deriver interface {
	StoreDerived(ctx context.Context, org, datastreamID, key string, value float64, at time.Time) error
}

type sample struct {
	v      float64
	at     time.Time
	has    bool
	loaded bool
}

type rule struct {
	ID, OrgID, Name string
	Version         int
	Spec            Spec
	c               *compiled
}

type state struct {
	mu       sync.Mutex
	active   bool
	since    time.Time // condition true since (pending hold or cooldown); zero = not pending
	lastFire time.Time
	loaded   bool
}

// Engine evaluates rules and runs their actions.
type Engine struct {
	pool  *pgxpool.Pool
	notif *notify.Notifier
	esc   *escalate.Engine
	live  *live.Hub
	log   *slog.Logger
	// Commander and Deriver are set by the owners after construction.
	Commander Commander
	Deriver   Deriver
	// HostAllowed: the organization's allow-list of HTTP hosts (Flows → Settings), which webhook actions obey like the
	// flows' HTTP request nodes; nil allows none.
	HostAllowed func(ctx context.Context, org, host string) bool
	Tick        time.Duration

	mu       sync.Mutex
	rules    map[string]*rule
	byDS     map[string][]*rule
	states   map[string]*state
	vals     map[string]sample
	keys     map[string]string
	loadedAt time.Time
	sem      chan struct{}
}

// New creates the engine.
func New(pool *pgxpool.Pool, notif *notify.Notifier, esc *escalate.Engine, hub *live.Hub, log *slog.Logger) *Engine {
	return &Engine{pool: pool, notif: notif, esc: esc, live: hub, log: log.With("component", "rules"), Tick: 5 * time.Second,
		rules: map[string]*rule{}, byDS: map[string][]*rule{}, states: map[string]*state{}, vals: map[string]sample{}, keys: map[string]string{}, sem: make(chan struct{}, 16)}
}

// Start loads the rules and runs the ticker.
func (e *Engine) Start(ctx context.Context) {
	e.reload(ctx)
	go func() {
		t := time.NewTicker(e.Tick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				e.tick(ctx)
			}
		}
	}()
}

// Resync reloads the rules from the database (after create/update/approve/revoke).
func (e *Engine) Resync() { e.reload(context.Background()) }

func (e *Engine) reload(ctx context.Context) {
	rows, err := e.pool.Query(ctx, `SELECT id::text, org_id::text, name, version, spec FROM core.rule WHERE enabled AND status = 'active' AND revoked_at IS NULL`)
	if err != nil {
		e.log.Error("load rules", "err", err)
		return
	}
	loaded := map[string]*rule{}
	for rows.Next() {
		var r rule
		var raw []byte
		if rows.Scan(&r.ID, &r.OrgID, &r.Name, &r.Version, &raw) != nil {
			continue
		}
		if json.Unmarshal(raw, &r.Spec) != nil {
			e.log.Warn("rule spec unreadable", "rule", r.ID)
			continue
		}
		c, err := Compile(r.Spec)
		if err != nil {
			e.log.Warn("rule does not compile, skipped", "rule", r.ID, "name", r.Name, "err", err)
			continue
		}
		r.c = c
		loaded[r.ID] = &r
	}
	rows.Close()
	e.mu.Lock()
	byDS := map[string][]*rule{}
	var fresh []*rule
	for _, r := range loaded {
		for _, in := range r.c.inputs {
			byDS[in.DatastreamID] = append(byDS[in.DatastreamID], r)
		}
		old, ok := e.rules[r.ID]
		if ok && old.Version != r.Version {
			delete(e.states, r.ID) // a new version starts fresh
		}
		if !ok || old.Version != r.Version {
			fresh = append(fresh, r)
		}
	}
	for id := range e.states {
		if _, ok := loaded[id]; !ok {
			delete(e.states, id)
		}
	}
	e.rules, e.byDS, e.loadedAt = loaded, byDS, time.Now()
	e.mu.Unlock()
	// A rule that just appeared (created, approved, enabled, new version, server start) is evaluated at once so a
	// condition that already holds is reported without waiting for the next observation.
	now := time.Now()
	for _, r := range fresh {
		e.evaluate(ctx, r, now)
	}
}

func (e *Engine) stateOf(ctx context.Context, r *rule) *state {
	e.mu.Lock()
	st, ok := e.states[r.ID]
	if !ok {
		st = &state{}
		e.states[r.ID] = st
	}
	e.mu.Unlock()
	st.mu.Lock()
	if !st.loaded {
		// Restore the last known transition so a restart does not fire "enter" again for a condition that was
		// already reported (the alarm, if any, is reused by its automation_rule_id).
		var event string
		var at time.Time
		if err := e.pool.QueryRow(ctx, `SELECT event, fired_at FROM core.rule_run WHERE rule_id = $1::uuid AND rule_version = $2 ORDER BY fired_at DESC LIMIT 1`, r.ID, r.Version).Scan(&event, &at); err == nil {
			st.active = event == "enter"
			st.lastFire = at
		}
		st.loaded = true
	}
	st.mu.Unlock()
	return st
}

// OnObservations is the ingest sink: cache the latest values and evaluate the rules that use them.
func (e *Engine) OnObservations(ctx context.Context, obs []ingest.Observation) {
	affected := map[string]*rule{}
	e.mu.Lock()
	for _, o := range obs {
		if o.Value == nil {
			continue
		}
		cur := e.vals[o.DatastreamID]
		if !cur.has || !o.MeasuredAt.Before(cur.at) {
			e.vals[o.DatastreamID] = sample{v: *o.Value, at: o.MeasuredAt, has: true, loaded: true}
		}
		for _, r := range e.byDS[o.DatastreamID] {
			affected[r.ID] = r
		}
	}
	e.mu.Unlock()
	for _, r := range affected {
		e.evaluate(ctx, r, time.Now())
	}
}

func (e *Engine) tick(ctx context.Context) {
	e.mu.Lock()
	stale := time.Since(e.loadedAt) > time.Minute
	list := make([]*rule, 0, len(e.rules))
	for _, r := range e.rules {
		list = append(list, r)
	}
	e.mu.Unlock()
	if stale {
		e.reload(ctx)
	}
	now := time.Now()
	for _, r := range list {
		st := e.stateOf(ctx, r)
		st.mu.Lock()
		pending := !st.since.IsZero()
		st.mu.Unlock()
		if r.c.hasStale || pending {
			e.evaluate(ctx, r, now)
		}
	}
}

// valueOf returns the latest numeric value of a datastream, loading it from the database once.
func (e *Engine) valueOf(ctx context.Context, dsID string) sample {
	e.mu.Lock()
	s := e.vals[dsID]
	e.mu.Unlock()
	if s.loaded {
		return s
	}
	var v *float64
	var at *time.Time
	_ = e.pool.QueryRow(ctx, `SELECT value::float8, measured_at FROM core.observation WHERE datastream_id = $1::uuid AND value IS NOT NULL ORDER BY measured_at DESC LIMIT 1`, dsID).Scan(&v, &at)
	s = sample{loaded: true}
	if v != nil && at != nil {
		s.v, s.at, s.has = *v, *at, true
	}
	e.mu.Lock()
	if cur := e.vals[dsID]; !cur.loaded {
		e.vals[dsID] = s
	} else {
		s = cur
	}
	e.mu.Unlock()
	return s
}

// Evaluation is the outcome of a dry run.
type Evaluation struct {
	Condition bool               `json:"condition"`
	Inputs    map[string]float64 `json:"inputs"`
	Ages      map[string]float64 `json:"ages_s"`
	Missing   []string           `json:"missing"`
	Active    bool               `json:"active"`
	Pending   bool               `json:"pending"`
	Errors    []string           `json:"errors,omitempty"`
}

func (e *Engine) gather(ctx context.Context, c *compiled, now time.Time) (map[string]float64, map[string]float64, []string) {
	vals, ages := map[string]float64{}, map[string]float64{}
	missing := []string{}
	for _, in := range c.inputs {
		s := e.valueOf(ctx, in.DatastreamID)
		if s.has {
			vals[in.Name] = s.v
			ages[in.Name] = now.Sub(s.at).Seconds()
		} else {
			missing = append(missing, in.Name)
		}
	}
	sort.Strings(missing)
	return vals, ages, missing
}

func condition(c *compiled, vals, ages map[string]float64) (bool, []string) {
	var errs []string
	if len(c.conds) == 0 {
		return true, nil
	}
	result := c.match == "all"
	for _, cd := range c.conds {
		var ok bool
		if cd.op == "stale" {
			age, has := ages[cd.input]
			ok = !has || age > cd.num
		} else {
			v, has := vals[cd.input]
			ref := cd.num
			if cd.expr != nil {
				r, err := cd.expr.Eval(vals)
				if err != nil {
					errs = append(errs, err.Error())
					has = false
				}
				ref = r
			}
			if has {
				switch cd.op {
				case ">":
					ok = v > ref
				case ">=":
					ok = v >= ref
				case "<":
					ok = v < ref
				case "<=":
					ok = v <= ref
				case "==":
					ok = v == ref
				case "!=":
					ok = v != ref
				}
			}
		}
		if c.match == "all" {
			result = result && ok
		} else {
			result = result || ok
		}
	}
	return result, errs
}

// Evaluate is a dry run of a spec (saved or not) against the current values; no actions run.
func (e *Engine) Evaluate(ctx context.Context, s Spec, ruleID string) (Evaluation, error) {
	c, err := Compile(s)
	if err != nil {
		return Evaluation{}, err
	}
	now := time.Now()
	vals, ages, missing := e.gather(ctx, c, now)
	ok, errs := condition(c, vals, ages)
	ev := Evaluation{Condition: ok, Inputs: vals, Ages: ages, Missing: missing, Errors: errs}
	if ruleID != "" {
		e.mu.Lock()
		st := e.states[ruleID]
		e.mu.Unlock()
		if st != nil {
			st.mu.Lock()
			ev.Active, ev.Pending = st.active, !st.since.IsZero()
			st.mu.Unlock()
		}
	}
	return ev, nil
}

// Status of a loaded rule for the UI.
type Status struct {
	Loaded  bool       `json:"loaded"`
	Active  bool       `json:"active"`
	Pending bool       `json:"pending"`
	Since   *time.Time `json:"since,omitempty"`
}

// StatusOf reports the in-memory state of a rule.
func (e *Engine) StatusOf(id string) Status {
	e.mu.Lock()
	_, loaded := e.rules[id]
	st := e.states[id]
	e.mu.Unlock()
	s := Status{Loaded: loaded}
	if st != nil {
		st.mu.Lock()
		s.Active, s.Pending = st.active, !st.since.IsZero()
		if !st.since.IsZero() {
			t := st.since
			s.Since = &t
		}
		st.mu.Unlock()
	}
	return s
}

func (e *Engine) evaluate(ctx context.Context, r *rule, now time.Time) {
	vals, ages, _ := e.gather(ctx, r.c, now)
	ok, errs := condition(r.c, vals, ages)
	if len(errs) > 0 {
		e.log.Debug("rule condition", "rule", r.Name, "errors", strings.Join(errs, "; "))
	}
	st := e.stateOf(ctx, r)
	st.mu.Lock()
	defer st.mu.Unlock()
	switch {
	case ok && !st.active:
		if st.since.IsZero() {
			st.since = now
		}
		if now.Sub(st.since) < time.Duration(r.Spec.ForS)*time.Second {
			return
		}
		if !st.lastFire.IsZero() && now.Sub(st.lastFire) < time.Duration(r.Spec.CooldownS)*time.Second {
			return
		}
		st.active, st.lastFire, st.since = true, now, time.Time{}
		e.fire(ctx, r, "enter", vals, now)
	case !ok && st.active:
		st.active, st.since = false, time.Time{}
		e.fire(ctx, r, "exit", vals, now)
	case !ok:
		st.since = time.Time{}
	case ok && st.active:
		e.derive(ctx, r, r.c.actions, vals, now)
	}
}

// derive runs the derive actions of a list synchronously (every evaluation while the condition holds).
func (e *Engine) derive(ctx context.Context, r *rule, list []actionC, vals map[string]float64, now time.Time) []map[string]any {
	var out []map[string]any
	for _, a := range list {
		if a.Type != "derive" {
			continue
		}
		res := map[string]any{"type": "derive", "status": "done"}
		v, err := a.expr.Eval(vals)
		if err == nil {
			if e.Deriver == nil {
				err = errors.New("derived datastreams are not available")
			} else {
				err = e.Deriver.StoreDerived(ctx, r.OrgID, a.DatastreamID, e.keyOf(ctx, a.DatastreamID), v, now)
			}
		}
		if err != nil {
			res["status"], res["detail"] = "failed", err.Error()
			e.log.Debug("derive failed", "rule", r.Name, "err", err)
		} else {
			res["detail"] = strconv.FormatFloat(v, 'g', -1, 64)
		}
		out = append(out, res)
	}
	return out
}

func (e *Engine) keyOf(ctx context.Context, dsID string) string {
	e.mu.Lock()
	k, ok := e.keys[dsID]
	e.mu.Unlock()
	if ok {
		return k
	}
	_ = e.pool.QueryRow(ctx, `SELECT key FROM core.datastream WHERE id = $1::uuid`, dsID).Scan(&k)
	e.mu.Lock()
	e.keys[dsID] = k
	e.mu.Unlock()
	return k
}

// fire records the transition and runs its actions: derive and alarm handling synchronously, the rest in the
// background (network I/O must not stall ingestion).
func (e *Engine) fire(ctx context.Context, r *rule, event string, vals map[string]float64, now time.Time) {
	list := r.c.actions
	if event == "exit" {
		list = r.c.clear
	}
	e.log.Info("rule "+event, "rule", r.Name, "inputs", vals)
	inputs, _ := json.Marshal(vals)
	var runID string
	if err := e.pool.QueryRow(ctx, `INSERT INTO core.rule_run (org_id, rule_id, rule_version, fired_at, event, inputs) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6::jsonb) RETURNING id::text`,
		r.OrgID, r.ID, r.Version, now, event, string(inputs)).Scan(&runID); err != nil {
		e.log.Error("rule run insert", "rule", r.Name, "err", err)
	}
	results := e.derive(ctx, r, list, vals, now)
	if event == "exit" {
		if res := e.clearAlarms(ctx, r, vals, now); res != nil {
			results = append(results, res)
		}
	}
	var async []actionC
	for _, a := range list {
		if a.Type == "alarm" {
			results = append(results, e.raiseAlarm(ctx, r, a, vals, now))
		} else if a.Type != "derive" {
			async = append(async, a)
		}
	}
	finish := func(results []map[string]any) {
		okAll := true
		for _, res := range results {
			if res["status"] == "failed" {
				okAll = false
			}
		}
		b, _ := json.Marshal(results)
		if runID != "" {
			_, _ = e.pool.Exec(context.Background(), `UPDATE core.rule_run SET actions = $2::jsonb, ok = $3 WHERE id = $1::uuid`, runID, string(b), okAll)
		}
	}
	if len(async) == 0 {
		finish(results)
		return
	}
	go func() {
		e.sem <- struct{}{}
		defer func() { <-e.sem }()
		bctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		for _, a := range async {
			results = append(results, e.runAction(bctx, r, a, event, vals))
		}
		finish(results)
	}()
}

func (e *Engine) inputsText(r *rule, vals map[string]float64) string {
	var lines []string
	for _, in := range r.c.inputs {
		if v, ok := vals[in.Name]; ok {
			lines = append(lines, in.Name+" = "+strconv.FormatFloat(v, 'g', -1, 64))
		} else {
			lines = append(lines, in.Name+" = (no data)")
		}
	}
	return strings.Join(lines, "\n")
}

func (e *Engine) message(r *rule, a actionC, event string, vals map[string]float64) notify.Message {
	pairs := []string{"{rule}", r.Name, "{event}", event, "{inputs}", e.inputsText(r, vals)}
	for _, in := range r.c.inputs {
		if v, ok := vals[in.Name]; ok {
			pairs = append(pairs, "{"+in.Name+"}", strconv.FormatFloat(v, 'g', -1, 64))
		} else {
			pairs = append(pairs, "{"+in.Name+"}", "(no data)")
		}
	}
	rep := strings.NewReplacer(pairs...)
	subject, text := a.Subject, a.Text
	if strings.TrimSpace(subject) == "" {
		subject = "[ctrl32] rule {rule}: {event}"
	}
	if strings.TrimSpace(text) == "" {
		text = "Rule {rule} — {event}\n{inputs}\n"
	}
	return notify.Message{Event: "rule", Subject: rep.Replace(subject), Text: rep.Replace(text),
		Rule: map[string]any{"id": r.ID, "name": r.Name, "version": r.Version, "transition": event, "inputs": vals}}
}

func (e *Engine) runAction(ctx context.Context, r *rule, a actionC, event string, vals map[string]float64) map[string]any {
	res := map[string]any{"type": a.Type, "status": "done"}
	fail := func(err error) map[string]any {
		res["status"], res["detail"] = "failed", err.Error()
		e.log.Warn("rule action failed", "rule", r.Name, "action", a.Type, "err", err)
		return res
	}
	switch a.Type {
	case "notify":
		var ch notify.Channel
		var raw []byte
		err := e.pool.QueryRow(ctx, `SELECT id::text, org_id::text, name, kind, settings FROM core.notification_channel WHERE id = $1::uuid AND org_id = $2::uuid AND enabled AND revoked_at IS NULL`, a.ChannelID, r.OrgID).
			Scan(&ch.ID, &ch.OrgID, &ch.Name, &ch.Kind, &raw)
		if err != nil {
			return fail(errors.New("channel not found or disabled"))
		}
		_ = json.Unmarshal(raw, &ch.Settings)
		msg := e.message(r, a, event, vals)
		msg.SentAt = time.Now().UTC()
		res["detail"] = ch.Name
		return e.deliverLogged(ctx, r, &ch, msg, res)
	case "webhook":
		if u, err := url.Parse(a.URL); err != nil || e.HostAllowed == nil || !e.HostAllowed(ctx, r.OrgID, u.Hostname()) {
			return fail(errors.New("the webhook host is not on the organization's allow-list (Flows → Settings)"))
		}
		ch := notify.Channel{OrgID: r.OrgID, Name: "rule:" + r.Name, Kind: "webhook", Settings: notify.ChannelSettings{URL: a.URL, Secret: a.Secret}}
		msg := e.message(r, a, event, vals)
		msg.SentAt = time.Now().UTC()
		res["detail"] = a.URL
		return e.deliverLogged(ctx, r, &ch, msg, res)
	case "command":
		if e.Commander == nil {
			return fail(errors.New("commands are not available"))
		}
		args := a.Args
		if len(args) == 0 {
			var value any
			switch {
			case a.valueExpr != nil:
				v, err := a.valueExpr.Eval(vals)
				if err != nil {
					return fail(err)
				}
				value = v
			case a.valueNum != nil:
				value = *a.valueNum
			case a.valueBool != nil:
				value = *a.valueBool
			}
			args, _ = json.Marshal(map[string]any{"key": a.Key, "value": value})
		}
		out, err := e.Commander.RunCommand(ctx, CommandRequest{OrgID: r.OrgID, DeviceID: a.DeviceID, Method: a.Method, Args: args, RuleID: r.ID, RuleName: r.Name})
		if err != nil {
			return fail(err)
		}
		res["detail"] = fmt.Sprintf("%s %s → %s", a.Method, string(args), out.Status)
		res["command_id"] = out.ID
		if out.Status == "failed" {
			res["status"], res["detail"] = "failed", out.Error
		}
		return res
	}
	return fail(fmt.Errorf("unsupported action %q", a.Type))
}

// deliverLogged sends on a channel and writes the delivery log row (core.notification, event 'rule').
func (e *Engine) deliverLogged(ctx context.Context, r *rule, ch *notify.Channel, msg notify.Message, res map[string]any) map[string]any {
	payload, _ := json.Marshal(msg)
	var nid string
	var chID *string
	if ch.ID != "" {
		chID = &ch.ID
	}
	_ = e.pool.QueryRow(ctx, `INSERT INTO core.notification (org_id, channel_id, rule_id, event, payload) VALUES ($1::uuid, $2::uuid, $3::uuid, 'rule', $4::jsonb) RETURNING id::text`, r.OrgID, chID, r.ID, string(payload)).Scan(&nid)
	err := e.notif.Deliver(ctx, *ch, msg)
	status, errText := "sent", ""
	if err != nil {
		status, errText = "failed", err.Error()
		res["status"], res["detail"] = "failed", err.Error()
		e.log.Warn("rule notification failed", "security", true, "rule", r.Name, "channel", ch.Name, "err", err)
	}
	if nid != "" {
		_, _ = e.pool.Exec(ctx, `UPDATE core.notification SET status = $2, sent_at = now(), attempts = 1, error = NULLIF($3, '') WHERE id = $1::uuid`, nid, status, errText)
	}
	return res
}

func (e *Engine) raiseAlarm(ctx context.Context, r *rule, a actionC, vals map[string]float64, now time.Time) map[string]any {
	res := map[string]any{"type": "alarm", "status": "done"}
	var existing string
	if err := e.pool.QueryRow(ctx, `SELECT id::text FROM core.alarm WHERE automation_rule_id = $1::uuid AND cleared_at IS NULL LIMIT 1`, r.ID).Scan(&existing); err == nil {
		res["detail"] = "already active: " + existing
		return res
	}
	var dsID *string
	var value *float64
	key := r.Name
	if len(r.c.inputs) > 0 {
		id := r.c.inputs[0].DatastreamID
		dsID = &id
		if v, ok := vals[r.c.inputs[0].Name]; ok {
			value = &v
		}
		key = r.c.inputs[0].Name
	}
	detail := map[string]any{"key": key, "rule": r.Name, "automation_rule_id": r.ID, "inputs": vals, "policy_id": a.PolicyID}
	d, _ := json.Marshal(detail)
	var alarmID string
	// a shelved rule (spec 035): the alarm is recorded, but not annunciated
	shelved := alarmmgmt.ShelvedUntil(ctx, e.pool, "", r.ID)
	state := "active"
	if shelved != nil {
		state = "suppressed"
	}
	err := e.pool.QueryRow(ctx, `INSERT INTO core.alarm (org_id, automation_rule_id, datastream_id, severity, value, detail, state, shelved_until) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5::numeric, $6::jsonb, $7, $8) RETURNING id::text`,
		r.OrgID, r.ID, dsID, a.Severity, value, string(d), state, shelved).Scan(&alarmID)
	if err != nil {
		res["status"], res["detail"] = "failed", err.Error()
		e.log.Error("rule alarm insert", "rule", r.Name, "err", err)
		return res
	}
	_, _ = e.pool.Exec(ctx, `INSERT INTO core.alarm_event (alarm_id, event, detail) VALUES ($1::uuid, 'raised', $2::jsonb)`, alarmID, string(d))
	res["detail"], res["alarm_id"] = a.Severity+" alarm "+alarmID, alarmID
	e.log.Warn("alarm raised by rule", "alarm", alarmID, "rule", r.Name, "severity", a.Severity)
	ds := ""
	if dsID != nil {
		ds = *dsID
	}
	if e.live != nil {
		e.live.Alarm(r.OrgID, alarmID, ds, state, a.Severity, value)
	}
	if shelved != nil {
		res["detail"] = res["detail"].(string) + " (shelved)"
		return res
	}
	v := 0.0
	if value != nil {
		v = *value
	}
	al := escalate.Alarm{OrgID: r.OrgID, ID: alarmID, DatastreamID: ds, Key: key, Severity: a.Severity, Value: v, Event: "raised", MeasuredAt: now, PolicyID: a.PolicyID}
	if a.PolicyID != "" && e.esc != nil && e.esc.OnAlarm(ctx, al) {
		return res
	}
	if e.notif != nil {
		subject := fmt.Sprintf("[ctrl32] %s alarm: rule %s", a.Severity, r.Name)
		body := fmt.Sprintf("Alarm %s\nAutomation rule: %s\nSeverity: %s\n%s\nAt: %s\n", alarmID, r.Name, a.Severity, e.inputsText(r, vals), now.Format(time.RFC3339))
		go func() {
			bctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			delivered, nerr := e.notif.Send(bctx, subject, body)
			ev, detail := "notified", `{"channel":"email"}`
			if nerr != nil {
				ev, detail = "delivery_failed", `{"channel":"email","error":`+strconv.Quote(nerr.Error())+`}`
			} else if !delivered {
				detail = `{"channel":"email","skipped":"smtp not configured"}`
			}
			_, _ = e.pool.Exec(bctx, `INSERT INTO core.alarm_event (alarm_id, event, detail) VALUES ($1::uuid, $2, $3::jsonb)`, alarmID, ev, detail)
		}()
	}
	return res
}

// clearAlarms clears the alarms raised by the rule (on exit, revoke, disable).
func (e *Engine) clearAlarms(ctx context.Context, r *rule, vals map[string]float64, now time.Time) map[string]any {
	rows, err := e.pool.Query(ctx, `SELECT id::text, severity, COALESCE(datastream_id::text, ''), detail->>'policy_id', detail->>'key' FROM core.alarm WHERE automation_rule_id = $1::uuid AND cleared_at IS NULL`, r.ID)
	if err != nil {
		return nil
	}
	type al struct{ id, sev, ds, policy, key string }
	var list []al
	for rows.Next() {
		var a al
		var policy, key *string
		if rows.Scan(&a.id, &a.sev, &a.ds, &policy, &key) == nil {
			if policy != nil {
				a.policy = *policy
			}
			if key != nil {
				a.key = *key
			}
			list = append(list, a)
		}
	}
	rows.Close()
	if len(list) == 0 {
		return nil
	}
	v := 0.0
	if len(r.c.inputs) > 0 {
		v = vals[r.c.inputs[0].Name]
	}
	for _, a := range list {
		if _, err := e.pool.Exec(ctx, `UPDATE core.alarm SET state = 'cleared', cleared_at = now() WHERE id = $1::uuid`, a.id); err != nil {
			continue
		}
		_, _ = e.pool.Exec(ctx, `INSERT INTO core.alarm_event (alarm_id, event, detail) VALUES ($1::uuid, 'cleared', jsonb_build_object('rule', $2::text))`, a.id, r.Name)
		e.log.Info("alarm cleared by rule", "alarm", a.id, "rule", r.Name)
		if e.live != nil {
			e.live.Alarm(r.OrgID, a.id, a.ds, "cleared", a.sev, &v)
		}
		if a.policy != "" && e.esc != nil {
			e.esc.OnAlarm(ctx, escalate.Alarm{OrgID: r.OrgID, ID: a.id, DatastreamID: a.ds, Key: a.key, Severity: a.sev, Value: v, Event: "cleared", MeasuredAt: now, PolicyID: a.policy})
		}
	}
	return map[string]any{"type": "alarm", "status": "done", "detail": fmt.Sprintf("cleared %d alarm(s)", len(list))}
}

// Retire clears the alarms of a rule that is revoked, disabled, rejected or waiting for approval and forgets its
// state; the rule set is reloaded by the caller through Resync.
func (e *Engine) Retire(ctx context.Context, ruleID string) {
	e.mu.Lock()
	r := e.rules[ruleID]
	delete(e.states, ruleID)
	e.mu.Unlock()
	if r == nil {
		var name, org string
		if e.pool.QueryRow(ctx, `SELECT name, org_id::text FROM core.rule WHERE id = $1::uuid`, ruleID).Scan(&name, &org) != nil {
			return
		}
		r = &rule{ID: ruleID, OrgID: org, Name: name, c: &compiled{}}
	}
	e.clearAlarms(ctx, r, map[string]float64{}, time.Now())
}

// CheckRefs verifies that the datastreams, channels, devices and policies a spec references belong to the
// organization and are usable.
func (e *Engine) CheckRefs(ctx context.Context, org string, s Spec) error {
	for _, in := range s.Inputs {
		var n int
		if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM core.datastream WHERE id = $1::uuid AND org_id = $2::uuid`, in.DatastreamID, org).Scan(&n); err != nil || n == 0 {
			return fmt.Errorf("input %q: datastream not found", in.Name)
		}
	}
	for _, a := range append(append([]Action{}, s.Actions...), s.ClearActions...) {
		var n int
		switch a.Type {
		case "notify":
			if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM core.notification_channel WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, a.ChannelID, org).Scan(&n); err != nil || n == 0 {
				return errors.New("notify: channel not found")
			}
		case "command":
			if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM core.device WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, a.DeviceID, org).Scan(&n); err != nil || n == 0 {
				return errors.New("command: device not found or revoked")
			}
		case "alarm":
			if a.PolicyID != "" {
				if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM core.escalation_policy WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, a.PolicyID, org).Scan(&n); err != nil || n == 0 {
					return errors.New("alarm: escalation policy not found")
				}
			}
		case "derive":
			if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM core.datastream WHERE id = $1::uuid AND org_id = $2::uuid`, a.DatastreamID, org).Scan(&n); err != nil || n == 0 {
				return errors.New("derive: datastream not found")
			}
		}
	}
	return nil
}

package flows

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/ingest"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/live"
)

// ---- expression environment -------------------------------------------------------------------------------------

type msgEnv struct {
	m *Message
	f *flow
}

func (e msgEnv) Lookup(name string) (Value, bool) {
	switch name {
	case "value":
		return e.m.Value, true
	case "topic":
		return e.m.Topic, true
	case "ts":
		if e.m.TS.IsZero() {
			return nil, true
		}
		return float64(e.m.TS.UnixMilli()) / 1000, true
	case "quality":
		return e.m.Quality, true
	case "source":
		if e.m.Source == nil {
			return nil, true
		}
		if e.m.Source.Name != "" {
			return e.m.Source.Name, true
		}
		return e.m.Source.Kind, true
	}
	if strings.HasPrefix(name, "msg.") {
		v, ok := e.m.Fields[name[4:]]
		return v, ok
	}
	if strings.HasPrefix(name, "flow.") {
		v, ok := e.f.mem[name[5:]]
		return v, ok
	}
	return nil, false
}
func (e msgEnv) Location() *time.Location { return e.f.loc }
func (e msgEnv) Now() time.Time           { return time.Now() }

func (f *flow) eval(n *rnode, name string, m *Message) (Value, error) {
	ex := n.exprs[name]
	if ex == nil {
		return nil, nil
	}
	return ex.Eval(msgEnv{m, f})
}

// render fills {{ expression }} placeholders.
func (f *flow) render(tmpl string, m *Message) (string, error) {
	var err error
	out := tmplRe.ReplaceAllStringFunc(tmpl, func(p string) string {
		ex, e := Parse(strings.TrimSpace(p[2 : len(p)-2]))
		if e != nil {
			err = e
			return ""
		}
		v, e := ex.Eval(msgEnv{m, f})
		if e != nil {
			err = e
			return ""
		}
		if fv, ok := v.(float64); ok {
			return trimFloat(fv)
		}
		return Str(v)
	})
	if len(out) > MaxStringLen {
		return "", fmt.Errorf("text longer than %d bytes", MaxStringLen)
	}
	return out, err
}

func trimFloat(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.6g", v)
}

func setField(m *Message, name string, v Value) {
	switch name {
	case "value":
		m.Value = v
	case "topic":
		m.Topic = Str(v)
	default:
		if m.Fields == nil {
			m.Fields = map[string]Value{}
		}
		m.Fields[strings.TrimPrefix(name, "msg.")] = v
	}
}

func getField(m *Message, name string) Value {
	switch name {
	case "value":
		return m.Value
	case "topic":
		return m.Topic
	}
	return m.Fields[strings.TrimPrefix(name, "msg.")]
}

// ---- processing -------------------------------------------------------------------------------------------------

func (f *flow) process(n *rnode, m *Message, dry bool) {
	n.in.Add(1)
	n.last.Store(time.Now().UnixMilli())
	f.e.processed.Add(1)
	if m.Size() > MaxMessageBytes {
		f.fail(n, nil, fmt.Errorf("message larger than %d bytes dropped", MaxMessageBytes))
		return
	}
	s := n.spec.Settings
	num := func(k string) float64 { v, _ := Num(s[k]); return v }
	str := func(k string) string { v, _ := s[k].(string); return v }
	boolean := func(k string) bool { v, _ := s[k].(bool); return v }
	dur := func(k string) time.Duration { return time.Duration(num(k) * float64(time.Second)) }
	topicKey := func() string { return "t:" + m.Topic }

	switch n.spec.Type {
	// ---- triggers: the engine injects the message into the trigger node itself
	case "ds_value":
		if boolean("only_changes") {
			if last, ok := n.st["last"]; ok && equal(last, m.Value) {
				return
			}
			n.st["last"] = m.Value
		}
		if iv := num("min_interval_s"); iv > 0 {
			if lt, ok := n.st["last_at"].(time.Time); ok && time.Since(lt) < time.Duration(iv*float64(time.Second)) {
				return
			}
			n.st["last_at"] = time.Now()
		}
		f.emit(n, 0, m, dry)
	case "alarm_event", "device_state", "schedule", "webhook", "entity", "command_result", "mqtt_in":
		f.emit(n, 0, m, dry)
	case "inject":
		if m.Value == nil {
			v, err := f.eval(n, "value", m)
			if err != nil {
				f.fail(n, m, err)
				return
			}
			m.Value = v
		}
		if t := str("topic"); t != "" && m.Topic == "" {
			m.Topic = t
		}
		f.emit(n, 0, m, dry)

	// ---- logic
	case "filter":
		ok, err := n.exprs["condition"].EvalBool(msgEnv{m, f})
		if err != nil {
			f.fail(n, m, err)
			return
		}
		if ok {
			f.emit(n, 0, m, dry)
		} else {
			f.emit(n, 1, m, dry)
		}
	case "switch":
		matched := false
		for i, r := range n.rules {
			ok, err := r.EvalBool(msgEnv{m, f})
			if err != nil {
				f.fail(n, m, fmt.Errorf("rule %d: %w", i+1, err))
				continue
			}
			if ok {
				matched = true
				f.emit(n, i, m.Clone(), dry)
				if str("check") != "all" {
					break
				}
			}
		}
		if !matched {
			f.emit(n, len(n.rules), m, dry)
		}
	case "change":
		list, _ := s["fields"].([]any)
		for _, x := range list {
			c, _ := x.(map[string]any)
			name, _ := c["name"].(string)
			switch c["op"] {
			case "set":
				ex, err := Parse(Str(c["expr"]))
				if err != nil {
					f.fail(n, m, err)
					return
				}
				v, err := ex.Eval(msgEnv{m, f})
				if err != nil {
					f.fail(n, m, err)
					return
				}
				setField(m, name, v)
			case "delete":
				delete(m.Fields, strings.TrimPrefix(name, "msg."))
			case "rename":
				to, _ := c["to"].(string)
				v := getField(m, name)
				if name != "value" && name != "topic" {
					delete(m.Fields, strings.TrimPrefix(name, "msg."))
				}
				setField(m, to, v)
			}
		}
		f.emit(n, 0, m, dry)
	case "math":
		v, err := f.eval(n, "expr", m)
		if err != nil {
			f.fail(n, m, err)
			return
		}
		setField(m, targetOf(str("target")), v)
		f.emit(n, 0, m, dry)
	case "template":
		txt, err := f.render(str("text"), m)
		if err != nil {
			f.fail(n, m, err)
			return
		}
		setField(m, targetOf(str("target")), txt)
		f.emit(n, 0, m, dry)
	case "delay":
		f.after(dur("seconds"), func() { f.emit(n, 0, m, dry) })
	case "debounce":
		gen, _ := n.st["gen"].(int)
		gen++
		n.st["gen"] = gen
		f.after(dur("seconds"), func() {
			if g, _ := n.st["gen"].(int); g == gen {
				f.emit(n, 0, m, dry)
			}
		})
	case "rate_limit":
		k := "all"
		if boolean("per_topic") {
			k = topicKey()
		}
		win, _ := n.st[k].([]time.Time)
		now := time.Now()
		cut := now.Add(-dur("period_s"))
		keep := win[:0]
		for _, t := range win {
			if t.After(cut) {
				keep = append(keep, t)
			}
		}
		if len(keep) >= int(num("count")) {
			n.st[k] = keep
			f.ring.add(DebugEntry{Node: n.spec.ID, Kind: "drop", Text: "rate limit: message dropped", Dry: dry})
			return
		}
		n.st[k] = append(keep, now)
		f.emit(n, 0, m, dry)
	case "deadband":
		v, ok := Num(m.Value)
		if !ok {
			f.fail(n, m, errors.New("value is not a number"))
			return
		}
		k := topicKey()
		if last, ok := n.st[k].(float64); ok {
			band := num("band")
			if boolean("percent") {
				band = math.Abs(last) * band / 100
			}
			if math.Abs(v-last) < band {
				return
			}
		}
		n.st[k] = v
		f.emit(n, 0, m, dry)
	case "aggregate":
		v, ok := Num(m.Value)
		if !ok {
			f.fail(n, m, errors.New("value is not a number"))
			return
		}
		k := topicKey()
		acc, _ := n.st[k].(*aggState)
		if acc == nil {
			acc = &aggState{min: v, max: v, first: v, msg: m}
			n.st[k] = acc
			fn := str("fn")
			f.after(dur("window_s"), func() {
				delete(n.st, k)
				out := acc.msg.Clone()
				out.Value = acc.result(fn)
				out.TS = time.Now().UTC()
				setField(out, "msg.count", float64(acc.n))
				f.emit(n, 0, out, dry)
			})
		}
		acc.add(v, m)
	case "join":
		topics := splitList(str("topics"))
		latest, _ := n.st["latest"].(map[string]joinVal)
		if latest == nil {
			latest = map[string]joinVal{}
			n.st["latest"] = latest
		}
		latest[m.Topic] = joinVal{v: m.Value, at: time.Now()}
		maxAge := dur("max_age_s")
		out := m.Clone()
		for _, t := range topics {
			jv, ok := latest[t]
			if !ok || (maxAge > 0 && time.Since(jv.at) > maxAge) {
				return
			}
			setField(out, "msg."+fieldName(t), jv.v)
		}
		f.emit(n, 0, out, dry)
	case "counter":
		if ex := n.exprs["reset"]; ex != nil {
			r, err := ex.EvalBool(msgEnv{m, f})
			if err != nil {
				f.fail(n, m, err)
				return
			}
			if r {
				n.st["count"] = 0.0
				n.dirty = true
				m.Value = 0.0
				f.emit(n, 0, m, dry)
				return
			}
		}
		add := 1.0
		if v, err := f.eval(n, "add", m); err != nil {
			f.fail(n, m, err)
			return
		} else if x, ok := Num(v); ok {
			add = x
		}
		c, _ := Num(n.st["count"])
		c += add
		n.st["count"] = c
		n.dirty = true
		m.Value = c
		f.emit(n, 0, m, dry)
	case "latch":
		on := Truthy(n.st["on"])
		env := msgEnv{m, f}
		set, err1 := n.exprs["set"].EvalBool(env)
		reset, err2 := n.exprs["reset"].EvalBool(env)
		if err := errors.Join(err1, err2); err != nil {
			f.fail(n, m, err)
			return
		}
		switch {
		case !on && set:
			on = true
		case on && reset:
			on = false
		default:
			return
		}
		n.st["on"] = on
		n.dirty = true
		out := m.Clone()
		setField(out, "msg.input", m.Value)
		out.Value = on
		f.emit(n, 0, out, dry)
	case "time_window":
		if f.inWindow(str("from"), str("to"), s["weekdays"]) {
			f.emit(n, 0, m, dry)
		} else {
			f.emit(n, 1, m, dry)
		}
	case "stable_for":
		ok, err := n.exprs["condition"].EvalBool(msgEnv{m, f})
		if err != nil {
			f.fail(n, m, err)
			return
		}
		active := Truthy(n.st["active"])
		gen, _ := n.st["gen"].(int)
		pending := Truthy(n.st["pending"])
		n.st["msg"] = m
		switch {
		case ok && !active && !pending:
			gen++
			n.st["gen"], n.st["pending"] = gen, true
			f.after(dur("seconds"), func() {
				if g, _ := n.st["gen"].(int); g == gen && Truthy(n.st["pending"]) {
					n.st["pending"], n.st["active"] = false, true
					last, _ := n.st["msg"].(*Message)
					if last == nil {
						last = m
					}
					f.emit(n, 0, last.Clone(), dry)
				}
			})
		case !ok && pending:
			n.st["pending"] = false
			n.st["gen"] = gen + 1
		case !ok && active:
			n.st["active"] = false
			f.emit(n, 1, m, dry)
		}

	// ---- actions (run outside the flow goroutine; the result continues as a message)
	case "derive":
		v, err := f.eval(n, "expr", m)
		if err != nil {
			f.fail(n, m, err)
			return
		}
		x, ok := Num(v)
		if !ok || math.IsInf(x, 0) {
			f.fail(n, m, fmt.Errorf("value %v is not a number", Str(v)))
			return
		}
		ds := str("datastream")
		if dry || f.temp {
			f.dryAction(n, m, fmt.Sprintf("would store %s in the datastream", trimFloat(x)))
			return
		}
		f.act(n, m, func(ctx context.Context) (map[string]Value, error) {
			name, key := f.e.host.DatastreamName(ctx, f.org, ds)
			ts := m.TS
			if ts.IsZero() {
				ts = time.Now()
			}
			if err := f.e.host.StoreDerived(ctx, f.org, ds, key, x, ts); err != nil {
				return nil, err
			}
			return map[string]Value{"stored": name}, nil
		}, fmt.Sprintf("stored %s", trimFloat(x)))
	case "command":
		v, err := f.eval(n, "value", m)
		if err != nil {
			f.fail(n, m, err)
			return
		}
		args, _ := json.Marshal(map[string]any{"key": str("key"), "value": v})
		dev, method := str("device"), str("method")
		if dry || f.temp {
			f.dryAction(n, m, fmt.Sprintf("would send %s %s to the device", method, string(args)))
			return
		}
		reason := f.origin(n, m)
		f.act(n, m, func(ctx context.Context) (map[string]Value, error) {
			id, status, errText, err := f.e.host.RunCommand(ctx, f.org, dev, method, args, f.id, "flow:"+f.name, reason)
			if err != nil {
				return nil, err
			}
			if status == "failed" {
				return nil, errors.New("command failed: " + errText)
			}
			return map[string]Value{"command_id": id, "status": status}, nil
		}, fmt.Sprintf("command %s %s", method, string(args)))
	case "attribute":
		v, err := f.eval(n, "value", m)
		if err != nil {
			f.fail(n, m, err)
			return
		}
		dev, key := str("device"), str("key")
		if dry || f.temp {
			f.dryAction(n, m, fmt.Sprintf("would set attribute %s = %s", key, Str(v)))
			return
		}
		reason := f.origin(n, m)
		f.act(n, m, func(ctx context.Context) (map[string]Value, error) {
			return nil, f.e.host.SetAttribute(ctx, f.org, dev, key, v, "flow:"+f.name, reason)
		}, fmt.Sprintf("attribute %s = %s", key, Str(v)))
	case "mqtt_out":
		topic, err1 := f.render(str("topic"), m)
		payload := Str(m.Value)
		var err2 error
		if str("payload") != "" {
			payload, err2 = f.render(str("payload"), m)
		}
		if err := errors.Join(err1, err2); err != nil {
			f.fail(n, m, err)
			return
		}
		topic = strings.TrimSpace(topic)
		if topic == "" || strings.ContainsAny(topic, "+#") || strings.HasPrefix(topic, "$") {
			f.fail(n, m, fmt.Errorf("invalid topic %q", topic))
			return
		}
		src, retain := str("source"), n.spec.Settings["retain"] == true
		if dry || f.temp {
			f.dryAction(n, m, fmt.Sprintf("would publish %q to %s", payload, topic))
			return
		}
		f.act(n, m, func(ctx context.Context) (map[string]Value, error) {
			return nil, f.e.host.MQTTPublish(ctx, f.org, src, topic, []byte(payload), retain)
		}, "published to "+topic)
	case "entity_action":
		service, ent := str("service"), str("entity")
		params := map[string]any{}
		if n.exprs["value"] != nil {
			v, err := f.eval(n, "value", m)
			if err != nil {
				f.fail(n, m, err)
				return
			}
			num, isNum := Num(v)
			switch service {
			case "set_position":
				params["position"] = num
			case "set_temperature":
				params["temperature"] = num
			case "set_percentage":
				params["percentage"] = num
			case "set_hvac_mode":
				params["mode"] = Str(v)
			case "set_value", "select_option":
				params["value"] = v
			case "turn_on":
				if isNum {
					params["brightness"] = num
				}
			}
			if (service == "set_position" || service == "set_temperature" || service == "set_percentage") && !isNum {
				f.fail(n, m, fmt.Errorf("value %v is not a number", Str(v)))
				return
			}
		}
		if dry || f.temp {
			f.dryAction(n, m, fmt.Sprintf("would %s the entity %v", strings.ReplaceAll(service, "_", " "), params))
			return
		}
		reason := f.origin(n, m)
		f.act(n, m, func(ctx context.Context) (map[string]Value, error) {
			id, status, errText, err := f.e.host.EntityCommand(ctx, f.org, ent, service, params, f.id, "flow:"+f.name, reason)
			if err != nil {
				return nil, err
			}
			if status == "failed" {
				return nil, errors.New("command failed: " + errText)
			}
			return map[string]Value{"command_id": id, "status": status}, nil
		}, "entity "+service)
	case "notify":
		subj, err1 := f.render(str("subject"), m)
		text, err2 := f.render(str("text"), m)
		if err := errors.Join(err1, err2); err != nil {
			f.fail(n, m, err)
			return
		}
		ch := str("channel")
		if dry || f.temp {
			f.dryAction(n, m, "would notify: "+subj)
			return
		}
		payload := map[string]any{"flow": f.name, "version": f.version, "node": n.spec.ID, "topic": m.Topic, "value": m.Value}
		f.act(n, m, func(ctx context.Context) (map[string]Value, error) {
			return nil, f.e.host.Notify(ctx, f.org, ch, f.id, subj, text, payload)
		}, "notified: "+subj)
	case "incident":
		title, err := f.render(str("title"), m)
		if err != nil {
			f.fail(n, m, err)
			return
		}
		action, sev := str("action"), str("severity")
		if dry || f.temp {
			f.dryAction(n, m, "would "+action+" incident: "+title)
			return
		}
		dedup := "flow:" + f.org + "/" + f.slug + "/" + n.spec.ID
		detail := map[string]any{"flow": f.name, "version": f.version, "node": n.spec.ID, "topic": m.Topic, "value": m.Value, "trigger": m.Trigger}
		f.act(n, m, func(ctx context.Context) (map[string]Value, error) {
			return nil, f.e.host.Incident(ctx, f.org, action, sev, title, dedup, detail)
		}, action+" incident: "+title)
	case "http_out":
		var body []byte
		if t := str("body"); t != "" {
			txt, err := f.render(t, m)
			if err != nil {
				f.fail(n, m, err)
				return
			}
			body = []byte(txt)
		} else {
			body, _ = json.Marshal(m)
		}
		method, url := str("method"), str("url")
		if method == "GET" {
			body = nil
		}
		if dry || f.temp {
			f.dryAction(n, m, fmt.Sprintf("would call %s %s", method, url))
			return
		}
		f.act(n, m, func(ctx context.Context) (map[string]Value, error) {
			code, resp, err := f.e.host.HTTP(ctx, f.org, method, url, body)
			if err != nil {
				return nil, err
			}
			return map[string]Value{"status": float64(code), "body": resp}, nil
		}, fmt.Sprintf("%s %s", method, url))
	case "debug":
		if !boolean("active") {
			return
		}
		var shown any = m
		if ex := n.exprs["expr"]; ex != nil {
			v, err := ex.Eval(msgEnv{m, f})
			if err != nil {
				f.fail(n, m, err)
				return
			}
			shown = v
		}
		text := Str(m.Value)
		if _, ok := shown.(*Message); !ok {
			text = Str(shown)
		}
		f.ring.add(DebugEntry{Node: n.spec.ID, Kind: "debug", Text: text, Msg: shown, Dry: dry})
	case "comment":
	}
}

func targetOf(t string) string {
	if t == "" {
		return "value"
	}
	return t
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// fieldName turns a topic into a msg field name (letters, digits, _).
func fieldName(t string) string {
	var b strings.Builder
	for _, r := range t {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

type joinVal struct {
	v  Value
	at time.Time
}

type aggState struct {
	n                         int
	sum, min, max, first, lst float64
	msg                       *Message
}

func (a *aggState) add(v float64, m *Message) {
	a.n++
	a.sum += v
	a.min = math.Min(a.min, v)
	a.max = math.Max(a.max, v)
	a.lst = v
	a.msg = m
}

func (a *aggState) result(fn string) float64 {
	switch fn {
	case "min":
		return a.min
	case "max":
		return a.max
	case "sum":
		return a.sum
	case "count":
		return float64(a.n)
	case "first":
		return a.first
	case "last":
		return a.lst
	}
	if a.n == 0 {
		return 0
	}
	return a.sum / float64(a.n)
}

var dayIndex = map[string]int{"mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6, "sun": 7}

func weekdayOK(now time.Time, days any) bool {
	list, ok := toStrings(days)
	if !ok || len(list) == 0 {
		return true
	}
	wd := int(now.Weekday())
	if wd == 0 {
		wd = 7
	}
	for _, d := range list {
		if dayIndex[d] == wd {
			return true
		}
	}
	return false
}

func hhmm(s string) int {
	var h, m int
	fmt.Sscanf(s, "%d:%d", &h, &m)
	return h*60 + m
}

func (f *flow) inWindow(from, to string, days any) bool {
	now := time.Now().In(f.loc)
	if !weekdayOK(now, days) {
		return false
	}
	cur, a, b := now.Hour()*60+now.Minute(), hhmm(from), hhmm(to)
	if a <= b {
		return cur >= a && cur < b
	}
	return cur >= a || cur < b // over midnight
}

// act runs an action outside the flow goroutine, in order per node (a command "on" then "off" arrives in that
// order), and continues the message with the result fields in the flow goroutine; failures go to the debug panel.
func (f *flow) act(n *rnode, m *Message, do func(ctx context.Context) (map[string]Value, error), desc string) {
	if n.work == nil {
		n.work = make(chan func(), 64)
		go func() {
			for {
				select {
				case job := <-n.work:
					job()
				case <-f.quit:
					return
				}
			}
		}()
	}
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := do(ctx)
		f.enqueue(qitem{fn: func() {
			if err != nil {
				f.fail(n, m, err)
				return
			}
			f.ring.add(DebugEntry{Node: n.spec.ID, Kind: "action", Text: desc})
			for k, v := range res {
				setField(m, "msg."+k, v)
			}
			f.emit(n, 0, m, false)
		}})
	}
	select {
	case n.work <- job:
	default:
		f.fail(n, m, errors.New("too many actions waiting at this node; message dropped"))
	}
}

func (f *flow) dryAction(n *rnode, m *Message, text string) {
	f.ring.add(DebugEntry{Node: n.spec.ID, Kind: "dry_action", Text: text, Msg: m, Dry: true})
	f.emit(n, 0, m, true)
}

// ---- schedules --------------------------------------------------------------------------------------------------

func (f *flow) startSchedules() {
	for _, n := range f.nodes {
		if n.spec.Type == "schedule" && !n.spec.Disabled {
			f.scheduleNext(n)
		}
	}
}

func (f *flow) scheduleNext(n *rnode) {
	s := n.spec.Settings
	mode, _ := s["mode"].(string)
	now := time.Now().In(f.loc)
	var next time.Time
	if mode == "daily" {
		times, _ := s["times"].(string)
		for d := 0; d < 8 && next.IsZero(); d++ {
			day := now.AddDate(0, 0, d)
			if !weekdayOK(day, s["weekdays"]) {
				continue
			}
			best := time.Time{}
			for _, t := range splitList(times) {
				mins := hhmm(t)
				at := time.Date(day.Year(), day.Month(), day.Day(), mins/60, mins%60, 0, 0, f.loc)
				if at.After(now) && (best.IsZero() || at.Before(best)) {
					best = at
				}
			}
			next = best
		}
		if next.IsZero() {
			return
		}
	} else {
		every, _ := Num(s["every_s"])
		if every < 1 {
			every = 60
		}
		next = now.Add(time.Duration(every * float64(time.Second)))
	}
	f.after(time.Until(next), func() {
		now := time.Now().UTC()
		if mode != "daily" && !weekdayOK(now.In(f.loc), s["weekdays"]) {
			f.scheduleNext(n)
			return
		}
		m := &Message{Topic: nameOr(n.spec), Value: float64(now.UnixMilli()) / 1000, TS: now, Source: &Source{Kind: "schedule", Name: nameOr(n.spec)}, Trigger: "schedule " + nameOr(n.spec)}
		f.process(n, m, false)
		f.scheduleNext(n)
	})
}

func nameOr(n *NodeSpec) string {
	if n.Name != "" {
		return n.Name
	}
	return n.ID
}

// ---- triggers from outside ----------------------------------------------------------------------------------------

// OnObservations implements the ingest sink: every stored value of a datastream a flow listens to.
func (e *Engine) OnObservations(ctx context.Context, obs []ingest.Observation) {
	e.mu.RLock()
	if len(e.byDS) == 0 {
		e.mu.RUnlock()
		return
	}
	type hit struct {
		ref trigRef
		o   ingest.Observation
	}
	var hits []hit
	for _, o := range obs {
		for _, r := range e.byDS[o.DatastreamID] {
			if r.f.org == o.OrgID {
				hits = append(hits, hit{r, o})
			}
		}
	}
	e.mu.RUnlock()
	for _, h := range hits {
		var v Value
		if h.o.Value != nil {
			v = *h.o.Value
		}
		name := h.o.DatastreamID
		if e.host != nil {
			name, _ = e.host.DatastreamName(ctx, h.o.OrgID, h.o.DatastreamID)
		}
		m := &Message{Topic: name, Value: v, TS: h.o.MeasuredAt, Quality: h.o.Quality, Source: &Source{Kind: "datastream", ID: h.o.DatastreamID, Name: name},
			Trigger: fmt.Sprintf("%s = %s", name, Str(v))}
		h.ref.f.enqueue(qitem{node: h.ref.node, msg: m})
	}
}

// OnEvent receives alarm and device events from the live hub (tap).
func (e *Engine) OnEvent(ev live.Event) {
	if ev.Type != "alarm" && ev.Type != "device" && ev.Type != "entity" && ev.Type != "command" {
		return
	}
	if ev.Type == "command" && ev.Status != "acked" && ev.Status != "failed" && ev.Status != "expired" && ev.Status != "rejected" {
		return
	}
	prevState := ""
	if ev.Type == "entity" {
		if p, ok := e.ents.Load(ev.EntityID); ok {
			prevState = p.(string)
		}
		e.ents.Store(ev.EntityID, ev.State)
	}
	e.mu.RLock()
	var targets []trigRef
	for _, f := range e.flows {
		if f.org != ev.Org {
			continue
		}
		for id, n := range f.nodes {
			if n.spec.Disabled {
				continue
			}
			if (ev.Type == "alarm" && n.spec.Type == "alarm_event") || (ev.Type == "device" && n.spec.Type == "device_state") ||
				(ev.Type == "entity" && n.spec.Type == "entity") || (ev.Type == "command" && n.spec.Type == "command_result") {
				targets = append(targets, trigRef{f, id})
			}
		}
	}
	e.mu.RUnlock()
	for _, t := range targets {
		n := t.f.nodes[t.node]
		s := n.spec.Settings
		var m *Message
		switch ev.Type {
		case "alarm":
			state := ev.State
			if state == "active" {
				state = "raised"
			}
			if evs, _ := toStrings(s["events"]); !contains(evs, state) {
				continue
			}
			if sev, _ := s["severity"].(string); sev != "" && sev != ev.Severity {
				continue
			}
			if ds, _ := s["datastream"].(string); ds != "" && ds != ev.DatastreamID {
				continue
			}
			var v Value
			if ev.Value != nil {
				v = *ev.Value
			}
			name := ev.DatastreamID
			if e.host != nil && ev.DatastreamID != "" {
				name, _ = e.host.DatastreamName(context.Background(), ev.Org, ev.DatastreamID)
			}
			m = &Message{Topic: name, Value: v, TS: time.Now().UTC(), Source: &Source{Kind: "alarm", ID: ev.AlarmID, Name: name},
				Fields: map[string]Value{"event": state, "severity": ev.Severity, "alarm_id": ev.AlarmID}, Trigger: "alarm " + state + " on " + name}
		case "device":
			if dev, _ := s["device"].(string); dev != "" && dev != ev.DeviceID {
				continue
			}
			state := "disconnected"
			if ev.Connected != nil && *ev.Connected {
				state = "connected"
			}
			if evs, _ := toStrings(s["events"]); !contains(evs, state) {
				continue
			}
			name := ev.DeviceID
			if e.host != nil {
				name = e.host.DeviceName(context.Background(), ev.Org, ev.DeviceID)
			}
			m = &Message{Topic: name, Value: state == "connected", TS: time.Now().UTC(), Source: &Source{Kind: "device", ID: ev.DeviceID, Name: name},
				Fields: map[string]Value{"event": state}, Trigger: "device " + name + " " + state}
		case "entity":
			if id, _ := s["entity"].(string); id != ev.EntityID {
				continue
			}
			changed := prevState != ev.State
			if !changed && s["attributes"] != true {
				continue
			}
			if from, _ := s["from"].(string); from != "" && (from != prevState || !changed) {
				continue
			}
			if to, _ := s["to"].(string); to != "" && (to != ev.State || !changed) {
				continue
			}
			name := ev.EntityID
			if e.host != nil {
				name = e.host.EntityName(context.Background(), ev.Org, ev.EntityID)
			}
			var v Value = ev.State
			if f, err := strconv.ParseFloat(ev.State, 64); err == nil {
				v = f
			}
			fields := map[string]Value{"from": prevState, "entity_id": ev.EntityID, "available": ev.Connected == nil || *ev.Connected}
			if attrs, ok := ev.Detail.(map[string]any); ok {
				for k, a := range attrs {
					if len(fields) >= 40 {
						break
					}
					switch a.(type) {
					case float64, string, bool, nil:
						fields[fieldName(k)] = a
					}
				}
			}
			m = &Message{Topic: name, Value: v, TS: time.Now().UTC(), Source: &Source{Kind: "entity", ID: ev.EntityID, Name: name},
				Fields: fields, Trigger: "entity " + name + " " + prevState + "→" + ev.State}
		case "command":
			if dev, _ := s["device"].(string); dev != "" && dev != ev.DeviceID {
				continue
			}
			if sts, _ := toStrings(s["statuses"]); !contains(sts, ev.Status) {
				continue
			}
			name := ev.DeviceID
			if e.host != nil {
				name = e.host.DeviceName(context.Background(), ev.Org, ev.DeviceID)
			}
			errText := ""
			if d, ok := ev.Detail.(map[string]any); ok {
				if s, ok := d["error"].(string); ok {
					errText = s
				}
			}
			m = &Message{Topic: name, Value: ev.Status, TS: time.Now().UTC(), Source: &Source{Kind: "command", ID: ev.CommandID, Name: name},
				Fields: map[string]Value{"command_id": ev.CommandID, "method": ev.Method, "error": errText, "device_id": ev.DeviceID}, Trigger: "command " + ev.Method + " " + ev.Status + " on " + name}
		}
		t.f.enqueue(qitem{node: t.node, msg: m})
	}
}

// ErrUnknownHook is returned for a token no deployed webhook node has.
var ErrUnknownHook = errors.New("unknown webhook")

// Webhook starts a message at the webhook node of token. The HMAC (when the node has a secret) is checked by the caller
// through HookSecret.
func (e *Engine) Webhook(token string, body map[string]any, verify func(secret string) bool) error {
	e.mu.RLock()
	ref, ok := e.hooks[token]
	e.mu.RUnlock()
	if !ok {
		return ErrUnknownHook
	}
	n := ref.f.nodes[ref.node]
	if sec, _ := n.spec.Settings["secret"].(string); sec != "" && (verify == nil || !verify(sec)) {
		return errors.New("signature does not match")
	}
	m := &Message{Topic: nameOr(n.spec), TS: time.Now().UTC(), Source: &Source{Kind: "webhook", Name: nameOr(n.spec)}, Fields: map[string]Value{}, Trigger: "webhook " + nameOr(n.spec)}
	for k, v := range body {
		switch k {
		case "value":
			m.Value = jsonValue(v)
		case "topic":
			m.Topic = Str(jsonValue(v))
		default:
			if len(m.Fields) < 50 {
				m.Fields[fieldName(k)] = jsonValue(v)
			}
		}
	}
	if !ref.f.enqueue(qitem{node: ref.node, msg: m}) {
		return errors.New("flow is busy")
	}
	return nil
}

// jsonValue keeps scalars; objects and arrays become their JSON text.
func jsonValue(v any) Value {
	switch x := v.(type) {
	case nil, float64, string, bool:
		return x
	}
	b, _ := json.Marshal(v)
	s := string(b)
	if len(s) > MaxStringLen {
		s = s[:MaxStringLen]
	}
	return s
}

// Inject sends a test message into a node. With spec (an edited, not deployed flow) a temporary instance runs for
// up to 30 s and everything is a dry run; otherwise the deployed flow gets the message (dry when asked).
func (e *Engine) Inject(ctx context.Context, org, slug, node string, value Value, topic string, dry bool, spec *Spec, name string) error {
	if spec != nil {
		f, err := e.newFlow(org, slug, "", name, 0, "", spec, true)
		if err != nil {
			return err
		}
		n := f.nodes[node]
		if n == nil {
			return fmt.Errorf("node %s not found", node)
		}
		c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		go func() { defer cancel(); f.run(c) }()
		f.info(node, "test message (dry run, edited flow)")
		f.enqueue(qitem{node: node, msg: testMsg(n, value, topic), dry: true})
		return nil
	}
	e.mu.RLock()
	f := e.flows[key(org, slug)]
	e.mu.RUnlock()
	if f == nil {
		return errors.New("the flow is not deployed")
	}
	n := f.nodes[node]
	if n == nil {
		return fmt.Errorf("node %s not found", node)
	}
	if dry {
		f.info(node, "test message (dry run)")
	} else {
		f.info(node, "test message")
	}
	if !f.enqueue(qitem{node: node, msg: testMsg(n, value, topic), dry: dry}) {
		return errors.New("flow is busy")
	}
	return nil
}

func testMsg(n *rnode, value Value, topic string) *Message {
	if topic == "" {
		topic = nameOr(n.spec)
	}
	return &Message{Topic: topic, Value: value, TS: time.Now().UTC(), Source: &Source{Kind: "inject", Name: nameOr(n.spec)}, Trigger: "test message"}
}

// mqttMessage turns an MQTT message into a flow message: a JSON scalar becomes the value, the scalar fields of a JSON
// object become msg.<field> (the value is then the payload text), anything else is text.
func mqttMessage(source, topic string, payload []byte) *Message {
	if len(payload) > 64<<10 {
		payload = payload[:64<<10]
	}
	m := &Message{Topic: topic, Value: string(payload), TS: time.Now().UTC(), Source: &Source{Kind: "mqtt", ID: source, Name: topic},
		Fields: map[string]Value{}, Trigger: "MQTT " + topic}
	var v any
	if json.Unmarshal(payload, &v) == nil {
		switch x := v.(type) {
		case float64, bool, string, nil:
			m.Value = x
		case map[string]any:
			flattenFields(m.Fields, "", x, 0)
		}
	}
	return m
}

// flattenFields copies the scalar members of a JSON object into msg fields; nested objects become
// parent_child (Tasmota's {"ENERGY":{"Power":5}} → msg.ENERGY_Power), at most three levels and 40 fields.
func flattenFields(dst map[string]Value, prefix string, obj map[string]any, depth int) {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if len(dst) >= 40 {
			return
		}
		switch fv := obj[k].(type) {
		case float64, string, bool, nil:
			dst[fieldName(prefix+k)] = fv
		case map[string]any:
			if depth < 2 {
				flattenFields(dst, prefix+k+"_", fv, depth+1)
			}
		}
	}
}

package flows

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Limits of a flow (research R9).
const (
	MaxNodes    = 200
	MaxWires    = 600
	MaxRules    = 10
	MaxFields   = 20
	MaxDeployed = 500
)

// Problem is one reason a flow cannot be deployed.
type Problem struct {
	Node    string `json:"node,omitempty"`
	Message string `json:"message"`
}

func (p Problem) String() string {
	if p.Node == "" {
		return p.Message
	}
	return p.Node + ": " + p.Message
}

// Refs checks references against the organization's data.
type Refs interface {
	Datastream(id string) bool
	Device(id string) bool
	Channel(id string) bool
	Entity(id string) bool
	MQTTSource(id string) bool
	Scene(id string) bool
	Camera(id string) bool
	// Subflow returns a subflow version (0 = current) of the organization.
	Subflow(slug string, version int) (SubflowRef, bool)
	HostAllowed(host string) bool
}

var (
	idRe    = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,40}$`)
	uuidRe  = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	hhmmRe  = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	fieldRe = regexp.MustCompile(`^(value|topic|msg\.[a-zA-Z_][a-zA-Z0-9_]{0,39})$`)
	tmplRe  = regexp.MustCompile(`\{\{(.*?)\}\}`)
)

// Validate checks a flow against the closed node library and normalizes its settings in place (defaults filled in,
// numbers as float64). An empty result means the flow can be deployed.
func Validate(s *Spec, refs Refs) []Problem {
	var out []Problem
	add := func(node, format string, a ...any) {
		out = append(out, Problem{Node: node, Message: fmt.Sprintf(format, a...)})
	}
	if len(s.Nodes) > MaxNodes {
		add("", "at most %d nodes per flow", MaxNodes)
		return out
	}
	if len(s.Wires) > MaxWires {
		add("", "at most %d wires per flow", MaxWires)
		return out
	}
	nodes := map[string]*NodeSpec{}
	triggers := 0
	for i := range s.Nodes {
		n := &s.Nodes[i]
		if !idRe.MatchString(n.ID) {
			add(n.ID, "node id must be 1–40 letters, digits, _ or -")
			continue
		}
		if nodes[n.ID] != nil {
			add(n.ID, "duplicate node id")
			continue
		}
		nodes[n.ID] = n
		if len(n.Name) > 64 {
			add(n.ID, "name longer than 64 characters")
		}
		d, ok := Def(n.Type)
		if !ok {
			add(n.ID, "unknown node type %q", n.Type)
			continue
		}
		if d.Group == "trigger" && !n.Disabled {
			triggers++
		}
		if n.Settings == nil {
			n.Settings = map[string]any{}
		}
		for k := range n.Settings {
			if n.Type == "subflow" && (k == "outputs" || k == "commands") {
				continue // derived from the subflow
			}
			if !hasSetting(d, k) {
				add(n.ID, "unknown setting %q", k)
			}
		}
		for _, st := range d.Settings {
			v, present := n.Settings[st.Name]
			if !present || v == nil || v == "" {
				if st.Default != nil {
					n.Settings[st.Name] = st.Default
					v, present = st.Default, true
				} else {
					if st.Required {
						add(n.ID, "%s is required", strings.ToLower(st.Label))
					}
					delete(n.Settings, st.Name)
					continue
				}
			}
			nv, err := checkSetting(st, v, refs)
			if err != "" {
				add(n.ID, "%s: %s", strings.ToLower(st.Label), err)
				continue
			}
			n.Settings[st.Name] = nv
		}
		if n.Type == "subflow" && refs != nil {
			slug, _ := n.Settings["flow"].(string)
			ver := 0
			if v, ok := Num(n.Settings["version"]); ok {
				ver = int(v)
			}
			if ref, ok := refs.Subflow(slug, ver); !ok {
				add(n.ID, "subflow %s (version %d) not found", slug, ver)
			} else {
				n.Settings["version"], n.Settings["outputs"], n.Settings["commands"] = float64(ref.Version), float64(ref.Outputs), ref.Commands
			}
		}
		if (n.Type == "sub_in" || n.Type == "sub_out") && s.Kind != "subflow" {
			add(n.ID, "subflow inputs and outputs belong in a subflow")
		}
		if s.Kind == "subflow" && d.Group == "trigger" {
			add(n.ID, "a subflow starts at its input; triggers belong in the flows that use it")
		}
		if n.Type == "schedule" {
			if m, _ := n.Settings["mode"].(string); m == "daily" {
				if t, _ := n.Settings["times"].(string); strings.TrimSpace(t) == "" {
					add(n.ID, "times of day are required for a daily schedule")
				}
			}
		}
	}
	// wires
	seen := map[string]bool{}
	adj := map[string][]string{}
	for _, w := range s.Wires {
		from, to := nodes[w.From], nodes[w.To]
		if from == nil || to == nil {
			add("", "wire %s connects a missing node", w)
			continue
		}
		key := w.String()
		if seen[key] {
			add(w.From, "duplicate wire %s", w)
			continue
		}
		seen[key] = true
		if w.Port < 0 || w.Port >= outputsOf(from) {
			add(w.From, "has no output %d", w.Port+1)
			continue
		}
		if d, _ := Def(to.Type); d == nil || d.Inputs == 0 {
			add(w.To, "has no input (a %s starts messages itself)", strings.ToLower(defLabel(to.Type)))
			continue
		}
		if w.From == w.To {
			add(w.From, "is wired to itself")
			continue
		}
		adj[w.From] = append(adj[w.From], w.To)
	}
	if cyc := cycleWithoutBreak(adj, nodes); cyc != "" {
		add(cyc, "is part of a loop without a delay, debounce, rate limit, aggregate or \"true for\" node")
	}
	if s.Kind == "subflow" {
		ins := 0
		for _, n := range s.Nodes {
			if n.Type == "sub_in" {
				ins++
			}
		}
		if ins != 1 {
			add("", "a subflow needs exactly one subflow input")
		}
	} else if s.Kind != "" {
		add("", "unknown flow kind %q", s.Kind)
	} else if triggers == 0 && len(s.Nodes) > 0 {
		add("", "the flow has no trigger (datastream value, alarm, device, schedule, webhook or inject)")
	}
	if len(s.Groups) > 50 {
		add("", "at most 50 groups")
	}
	for _, g := range s.Groups {
		if len(g.Name) > 64 || (g.Color != "" && !hexColorRe.MatchString(g.Color)) || g.W < 0 || g.H < 0 || g.W > 20000 || g.H > 20000 {
			add("", "group %q: name ≤ 64 characters, colour #rrggbb", g.Name)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Node < out[j].Node })
	return out
}

func defLabel(t string) string {
	if d, ok := Def(t); ok {
		return d.Label
	}
	return t
}

func hasSetting(d *NodeDef, name string) bool {
	for _, s := range d.Settings {
		if s.Name == name {
			return true
		}
	}
	return false
}

func checkSetting(st Setting, v any, refs Refs) (any, string) {
	str := func(max int) (string, string) {
		s, ok := v.(string)
		if !ok {
			return "", "must be text"
		}
		if len(s) > max {
			return "", fmt.Sprintf("longer than %d characters", max)
		}
		return strings.TrimSpace(s), ""
	}
	switch st.Kind {
	case "subflow":
		v, err := str(63)
		if err == "" && !slugRe.MatchString(v) {
			return nil, "choose one"
		}
		return v, err
	case "text":
		return str(500)
	case "multiline":
		return str(4000)
	case "number":
		f, ok := Num(v)
		if !ok {
			return nil, "must be a number"
		}
		if (st.Min != nil && f < *st.Min) || (st.Max != nil && f > *st.Max) {
			return nil, fmt.Sprintf("must be %s", rangeText(st))
		}
		return f, ""
	case "bool":
		b, ok := v.(bool)
		if !ok {
			return nil, "must be true or false"
		}
		return b, ""
	case "select":
		s, _ := v.(string)
		for _, o := range st.Options {
			if o == s {
				return s, ""
			}
		}
		return nil, "must be one of " + strings.Join(st.Options, ", ")
	case "multi":
		list, ok := toStrings(v)
		if !ok {
			return nil, "must be a list"
		}
		for _, s := range list {
			if !contains(st.Options, s) {
				return nil, "must contain only " + strings.Join(st.Options, ", ")
			}
		}
		return list, ""
	case "expr":
		s, e := str(MaxExprLen)
		if e != "" {
			return nil, e
		}
		if s == "" {
			return s, ""
		}
		if _, err := Parse(s); err != nil {
			return nil, err.Error()
		}
		return s, ""
	case "template":
		s, e := str(4000)
		if e != "" {
			return nil, e
		}
		if err := checkTemplate(s); err != "" {
			return nil, err
		}
		return s, ""
	case "datastream", "device", "channel", "entity", "mqtt_source", "scene", "camera":
		s, _ := v.(string)
		if !uuidRe.MatchString(s) {
			return nil, "choose one"
		}
		ok := refs == nil
		if refs != nil {
			switch st.Kind {
			case "datastream":
				ok = refs.Datastream(s)
			case "device":
				ok = refs.Device(s)
			case "channel":
				ok = refs.Channel(s)
			case "entity":
				ok = refs.Entity(s)
			case "mqtt_source":
				ok = refs.MQTTSource(s)
			case "scene":
				ok = refs.Scene(s)
			case "camera":
				ok = refs.Camera(s)
			}
		}
		if !ok {
			return nil, "not found in this organization"
		}
		return s, ""
	case "time":
		s, _ := v.(string)
		if !hhmmRe.MatchString(s) {
			return nil, "must be HH:MM"
		}
		return s, ""
	case "times":
		s, e := str(200)
		if e != "" {
			return nil, e
		}
		for _, t := range strings.Split(s, ",") {
			if t = strings.TrimSpace(t); t != "" && !hhmmRe.MatchString(t) {
				return nil, fmt.Sprintf("%q is not HH:MM", t)
			}
		}
		return s, ""
	case "url":
		s, e := str(1000)
		if e != "" {
			return nil, e
		}
		u, err := url.Parse(s)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return nil, "must be an http(s) URL"
		}
		if refs != nil && !refs.HostAllowed(u.Hostname()) {
			return nil, fmt.Sprintf("host %s is not on the organization's allow-list", u.Hostname())
		}
		return s, ""
	case "rules":
		list, ok := toStrings(v)
		if !ok || len(list) == 0 {
			return nil, "at least one rule"
		}
		if len(list) > MaxRules {
			return nil, fmt.Sprintf("at most %d rules", MaxRules)
		}
		for i, r := range list {
			if _, err := Parse(r); err != nil {
				return nil, fmt.Sprintf("rule %d: %v", i+1, err)
			}
		}
		return list, ""
	case "fields":
		raw, ok := v.([]any)
		if !ok || len(raw) == 0 {
			return nil, "at least one change"
		}
		if len(raw) > MaxFields {
			return nil, fmt.Sprintf("at most %d changes", MaxFields)
		}
		out := make([]any, 0, len(raw))
		for i, x := range raw {
			m, ok := x.(map[string]any)
			if !ok {
				return nil, fmt.Sprintf("change %d is not an object", i+1)
			}
			op, _ := m["op"].(string)
			name, _ := m["name"].(string)
			if !fieldRe.MatchString(name) {
				return nil, fmt.Sprintf("change %d: field must be value, topic or msg.<name>", i+1)
			}
			e := map[string]any{"op": op, "name": name}
			switch op {
			case "set":
				ex, _ := m["expr"].(string)
				if _, err := Parse(ex); err != nil {
					return nil, fmt.Sprintf("change %d: %v", i+1, err)
				}
				e["expr"] = ex
			case "delete":
				if name == "value" || name == "topic" {
					return nil, fmt.Sprintf("change %d: value and topic cannot be deleted (set them to null)", i+1)
				}
			case "rename":
				to, _ := m["to"].(string)
				if !fieldRe.MatchString(to) {
					return nil, fmt.Sprintf("change %d: new name must be value, topic or msg.<name>", i+1)
				}
				e["to"] = to
			default:
				return nil, fmt.Sprintf("change %d: operation must be set, delete or rename", i+1)
			}
			out = append(out, e)
		}
		return out, ""
	}
	return nil, "unknown setting kind " + st.Kind
}

func checkTemplate(s string) string {
	for _, m := range tmplRe.FindAllStringSubmatch(s, 50) {
		if _, err := Parse(strings.TrimSpace(m[1])); err != nil {
			return fmt.Sprintf("placeholder {{%s}}: %v", m[1], err)
		}
	}
	if strings.Count(s, "{{") != strings.Count(s, "}}") {
		return "unbalanced {{ }}"
	}
	return ""
}

func rangeText(st Setting) string {
	switch {
	case st.Min != nil && st.Max != nil:
		return fmt.Sprintf("%g–%g", *st.Min, *st.Max)
	case st.Min != nil:
		return fmt.Sprintf("at least %g", *st.Min)
	case st.Max != nil:
		return fmt.Sprintf("at most %g", *st.Max)
	}
	return "a number"
}

func toStrings(v any) ([]string, bool) {
	switch x := v.(type) {
	case []string:
		return x, true
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out = append(out, strings.TrimSpace(s))
		}
		return out, true
	}
	return nil, false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// cycleWithoutBreak returns a node of a cycle that contains no time-separating node, or "".
func cycleWithoutBreak(adj map[string][]string, nodes map[string]*NodeSpec) string {
	breaks := func(id string) bool { d, _ := Def(nodes[id].Type); return d != nil && d.Breaks }
	// Remove breaking nodes; any remaining cycle is illegal.
	state := map[string]int{} // 0 new, 1 on stack, 2 done
	var found string
	var dfs func(u string) bool
	dfs = func(u string) bool {
		state[u] = 1
		for _, v := range adj[u] {
			if breaks(v) {
				continue
			}
			if state[v] == 1 {
				found = v
				return true
			}
			if state[v] == 0 && dfs(v) {
				return true
			}
		}
		state[u] = 2
		return false
	}
	ids := make([]string, 0, len(adj))
	for id := range adj {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if state[id] == 0 && !breaks(id) && dfs(id) {
			return found
		}
	}
	return ""
}

// HasCommands reports whether a flow acts on devices (four-eyes for deploys).
func HasCommands(s *Spec) bool {
	for _, n := range s.Nodes {
		if d, ok := Def(n.Type); ok && d.Commands && !n.Disabled {
			return true
		}
		if n.Type == "subflow" && !n.Disabled && n.Settings["commands"] == true {
			return true
		}
	}
	return false
}

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

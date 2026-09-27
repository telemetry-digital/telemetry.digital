package flows

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/rules"
)

var identRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// ruleExpr rewrites a rule expression (bare input names) into a flow expression (msg.<name>).
func ruleExpr(src string, inputs map[string]bool) string {
	return identRe.ReplaceAllStringFunc(src, func(id string) string {
		if inputs[id] {
			return "msg." + id
		}
		return id
	})
}

// ruleTemplate rewrites rule text placeholders ({rule} {event} {inputs} {<input>}) into flow templates.
func ruleTemplate(src, rule, event string, names []string) string {
	var all []string
	for _, n := range names {
		all = append(all, n+"={{ msg."+n+" }}")
	}
	return regexp.MustCompile(`\{([a-z][a-z0-9_]*)\}`).ReplaceAllStringFunc(src, func(p string) string {
		k := p[1 : len(p)-1]
		switch k {
		case "rule":
			return rule
		case "event":
			return event
		case "inputs":
			return strings.Join(all, ", ")
		}
		for _, n := range names {
			if n == k {
				return "{{ msg." + n + " }}"
			}
		}
		return p
	})
}

// FromRule turns an automation rule into an equivalent draft flow (FR-010): one datastream trigger per input,
// a join, a "true for" node over the rule's conditions, the enter actions on its first output (behind a rate
// limit when the rule has a cooldown) and the exit actions on its second. Parts that have no flow equivalent
// are listed in notes and as comment nodes.
func FromRule(name string, s rules.Spec) (Spec, []string) {
	var sp Spec
	var notes []string
	inputs := map[string]bool{}
	var names []string
	for _, in := range s.Inputs {
		inputs[in.Name] = true
		names = append(names, in.Name)
	}
	id := 0
	add := func(typ string, x, y float64, set map[string]any) string {
		id++
		nid := fmt.Sprintf("n%d", id)
		sp.Nodes = append(sp.Nodes, NodeSpec{ID: nid, Type: typ, X: x, Y: y, Settings: set})
		return nid
	}
	wire := func(from string, port int, to string) {
		sp.Wires = append(sp.Wires, Wire{From: from, Port: port, To: to})
	}

	join := add("join", 440, 60, map[string]any{"topics": strings.Join(names, ","), "max_age_s": 0.0})
	for i, in := range s.Inputs {
		y := 60 + float64(i)*80
		ds := add("ds_value", 40, y, map[string]any{"datastream": strings.ToLower(in.DatastreamID)})
		ch := add("change", 240, y, map[string]any{"fields": []any{map[string]any{"op": "set", "name": "topic", "expr": strconv.Quote(in.Name)}}})
		sp.Nodes[len(sp.Nodes)-1].Name = in.Name
		wire(ds, 0, ch)
		wire(ch, 0, join)
	}

	var parts []string
	for _, c := range s.Conditions {
		if c.Op == "stale" {
			notes = append(notes, fmt.Sprintf("condition \"%s stale\" has no flow equivalent and was left out", c.Input))
			continue
		}
		rhs := strings.TrimSpace(string(c.Value))
		var str string
		if json.Unmarshal(c.Value, &str) == nil {
			rhs = "(" + ruleExpr(str, inputs) + ")"
		}
		parts = append(parts, fmt.Sprintf("msg.%s %s %s", c.Input, c.Op, rhs))
	}
	joiner := " && "
	if s.Match == "any" {
		joiner = " || "
	}
	cond := strings.Join(parts, joiner)
	if cond == "" {
		cond = "false"
	}
	gate := add("stable_for", 640, 60, map[string]any{"condition": cond, "seconds": float64(s.ForS)})
	sp.Nodes[len(sp.Nodes)-1].Name = name
	wire(join, 0, gate)

	enter := gate
	if s.CooldownS > 0 {
		enter = add("rate_limit", 840, 20, map[string]any{"count": 1.0, "period_s": float64(s.CooldownS)})
		sp.Nodes[len(sp.Nodes)-1].Name = "cooldown"
		wire(gate, 0, enter)
	}
	var resolves []map[string]any
	place := func(acts []rules.Action, from string, port int, event string, x, y0 float64) {
		for i, a := range acts {
			y := y0 + float64(i)*80
			var n string
			switch a.Type {
			case "notify":
				n = add("notify", x, y, map[string]any{"channel": strings.ToLower(a.ChannelID),
					"subject": ruleTemplate(a.Subject, name, event, names), "text": ruleTemplate(a.Text, name, event, names)})
			case "webhook":
				n = add("http_out", x, y, map[string]any{"method": "POST", "url": a.URL})
				if a.Secret != "" {
					notes = append(notes, "webhook signing secrets are not carried over; the HTTP request node sends unsigned requests to allow-listed hosts")
				}
			case "command":
				val := "value"
				var b bool
				var f float64
				var str string
				switch {
				case json.Unmarshal(a.Value, &b) == nil:
					val = strconv.FormatBool(b)
				case json.Unmarshal(a.Value, &f) == nil:
					val = strconv.FormatFloat(f, 'g', -1, 64)
				case json.Unmarshal(a.Value, &str) == nil:
					val = ruleExpr(str, inputs)
				}
				method := a.Method
				if method == "" {
					method = "write"
				}
				n = add("command", x, y, map[string]any{"device": strings.ToLower(a.DeviceID), "key": a.Key, "value": val, "method": method})
				if len(a.Args) > 0 {
					notes = append(notes, "custom RPC arguments of a command are not carried over")
				}
			case "alarm":
				sev := map[string]string{"warning": "minor", "action": "major", "critical": "critical"}[a.Severity]
				if sev == "" {
					sev = "major"
				}
				n = add("incident", x, y, map[string]any{"action": "open", "severity": sev, "title": name})
				resolves = append(resolves, map[string]any{"action": "resolve", "severity": sev, "title": name})
				if a.PolicyID != "" {
					notes = append(notes, "an escalation policy of an alarm action is replaced by an incident")
				}
			case "derive":
				n = add("derive", x, y, map[string]any{"datastream": strings.ToLower(a.DatastreamID), "expr": ruleExpr(a.Expr, inputs)})
			default:
				notes = append(notes, fmt.Sprintf("action type %q has no flow equivalent", a.Type))
				continue
			}
			wire(from, port, n)
		}
	}
	place(s.Actions, enter, 0, "raised", 1060, 20)
	clearY := 60 + float64(len(s.Actions))*80
	place(s.ClearActions, gate, 1, "cleared", 1060, clearY)
	for i, r := range resolves {
		wire(gate, 1, add("incident", 1060, clearY+float64(len(s.ClearActions)+i)*80, r))
	}
	if len(notes) > 0 {
		add("comment", 40, 80+float64(len(s.Inputs))*80, map[string]any{"text": "Converted from rule \"" + name + "\":\n- " + strings.Join(notes, "\n- ")})
	}
	return sp, notes
}

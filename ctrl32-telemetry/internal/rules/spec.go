// Package rules implements automation rules "condition → action" (URS C-09): named inputs (datastreams), a
// condition over them (all/any comparisons, "stale" = no data for N seconds, hold time, cooldown) and actions on
// enter/exit — notification channel, inline webhook, device command, alarm, derived datastream. Rules are evaluated
// on every stored observation of their inputs and by a ticker (hold times, cooldowns, staleness).
package rules

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Input binds a name used in conditions and expressions to a datastream.
type Input struct {
	Name         string `json:"name"`
	DatastreamID string `json:"datastream_id"`
}

// Condition compares an input with a number or an expression, or checks staleness.
type Condition struct {
	Input string          `json:"input"`
	Op    string          `json:"op"`              // > >= < <= == != stale
	Value json.RawMessage `json:"value,omitempty"` // number, or a string expression over the inputs; stale: seconds
}

// Action is one thing to do on enter or exit.
type Action struct {
	Type string `json:"type"` // notify | webhook | command | alarm | derive
	// notify
	ChannelID string `json:"channel_id,omitempty"`
	Subject   string `json:"subject,omitempty"` // templates: {rule} {event} {inputs} {<input name>}
	Text      string `json:"text,omitempty"`
	// webhook
	URL    string `json:"url,omitempty"`
	Secret string `json:"secret,omitempty"`
	// command
	DeviceID string          `json:"device_id,omitempty"`
	Method   string          `json:"method,omitempty"` // default write
	Key      string          `json:"key,omitempty"`
	Value    json.RawMessage `json:"value,omitempty"` // number, bool, or a string expression
	Args     json.RawMessage `json:"args,omitempty"`  // custom RPC arguments (MQTT devices) instead of {key, value}
	// alarm
	Severity string `json:"severity,omitempty"` // warning | action | critical
	PolicyID string `json:"escalation_policy_id,omitempty"`
	// derive
	DatastreamID string `json:"datastream_id,omitempty"`
	Expr         string `json:"expr,omitempty"`
}

// Spec is the stored rule definition.
type Spec struct {
	Inputs       []Input     `json:"inputs"`
	Match        string      `json:"match"` // all | any
	Conditions   []Condition `json:"conditions"`
	ForS         int         `json:"for_s"`
	CooldownS    int         `json:"cooldown_s"`
	Actions      []Action    `json:"actions"`
	ClearActions []Action    `json:"clear_actions"`
}

var (
	nameRe   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	uuidRe   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	methodRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	ops      = map[string]bool{">": true, ">=": true, "<": true, "<=": true, "==": true, "!=": true, "stale": true}
	sevs     = map[string]bool{"warning": true, "action": true, "critical": true}
)

type condC struct {
	input string
	op    string
	num   float64
	expr  *Expr
}

type actionC struct {
	Action
	valueExpr *Expr
	valueNum  *float64
	valueBool *bool
	expr      *Expr
}

// compiled is a validated, parsed spec.
type compiled struct {
	inputs   []Input
	byName   map[string]string // name -> datastream id
	match    string
	conds    []condC
	hasStale bool
	actions  []actionC
	clear    []actionC
}

// HasCommands reports whether the spec sends device commands (four-eyes relevant).
func (s Spec) HasCommands() bool {
	for _, a := range append(append([]Action{}, s.Actions...), s.ClearActions...) {
		if a.Type == "command" {
			return true
		}
	}
	return false
}

// Compile validates the structure of a spec: names, references between inputs/conditions/actions, expressions.
// Existence of datastreams, channels, devices and policies is checked separately against the database.
func Compile(s Spec) (*compiled, error) {
	c := &compiled{byName: map[string]string{}, match: s.Match}
	if len(s.Inputs) > 16 {
		return nil, errors.New("at most 16 inputs")
	}
	for i, in := range s.Inputs {
		in.Name = strings.TrimSpace(in.Name)
		if !nameRe.MatchString(in.Name) {
			return nil, fmt.Errorf("input %d: name must match ^[a-z][a-z0-9_]{0,31}$", i+1)
		}
		if _, ok := exprFuncs[in.Name]; ok {
			return nil, fmt.Errorf("input %q: name is a function", in.Name)
		}
		if _, dup := c.byName[in.Name]; dup {
			return nil, fmt.Errorf("input %q is declared twice", in.Name)
		}
		if !uuidRe.MatchString(in.DatastreamID) {
			return nil, fmt.Errorf("input %q: datastream_id required", in.Name)
		}
		c.byName[in.Name] = in.DatastreamID
		c.inputs = append(c.inputs, in)
	}
	if c.match == "" {
		c.match = "all"
	}
	if c.match != "all" && c.match != "any" {
		return nil, errors.New("match must be all or any")
	}
	if len(s.Conditions) > 16 {
		return nil, errors.New("at most 16 conditions")
	}
	for i, cd := range s.Conditions {
		cd.Input = strings.TrimSpace(cd.Input)
		if _, ok := c.byName[cd.Input]; !ok {
			return nil, fmt.Errorf("condition %d: unknown input %q", i+1, cd.Input)
		}
		if !ops[cd.Op] {
			return nil, fmt.Errorf("condition %d: operator must be one of > >= < <= == != stale", i+1)
		}
		cc := condC{input: cd.Input, op: cd.Op}
		if len(cd.Value) == 0 {
			return nil, fmt.Errorf("condition %d: value required", i+1)
		}
		var num float64
		var str string
		switch {
		case json.Unmarshal(cd.Value, &num) == nil:
			cc.num = num
		case json.Unmarshal(cd.Value, &str) == nil:
			if cd.Op == "stale" {
				n, err := strconv.ParseFloat(strings.TrimSpace(str), 64)
				if err != nil {
					return nil, fmt.Errorf("condition %d: stale takes seconds", i+1)
				}
				cc.num = n
			} else {
				ex, err := ParseExpr(str)
				if err != nil {
					return nil, fmt.Errorf("condition %d: %v", i+1, err)
				}
				for _, v := range ex.Vars() {
					if _, ok := c.byName[v]; !ok {
						return nil, fmt.Errorf("condition %d: unknown input %q in expression", i+1, v)
					}
				}
				cc.expr = ex
			}
		default:
			return nil, fmt.Errorf("condition %d: value must be a number or an expression", i+1)
		}
		if cd.Op == "stale" {
			if cc.num < 1 || cc.num > 366*24*3600 {
				return nil, fmt.Errorf("condition %d: stale seconds must be 1..31622400", i+1)
			}
			c.hasStale = true
		}
		c.conds = append(c.conds, cc)
	}
	if s.ForS < 0 || s.ForS > 7*24*3600 || s.CooldownS < 0 || s.CooldownS > 7*24*3600 {
		return nil, errors.New("for_s and cooldown_s must be 0..604800")
	}
	if len(s.Actions)+len(s.ClearActions) == 0 {
		return nil, errors.New("at least one action is required")
	}
	if len(s.Actions) > 16 || len(s.ClearActions) > 16 {
		return nil, errors.New("at most 16 actions per list")
	}
	var err error
	if c.actions, err = compileActions(c, s.Actions, "actions"); err != nil {
		return nil, err
	}
	if c.clear, err = compileActions(c, s.ClearActions, "clear_actions"); err != nil {
		return nil, err
	}
	return c, nil
}

func compileActions(c *compiled, list []Action, where string) ([]actionC, error) {
	var out []actionC
	for i, a := range list {
		ac := actionC{Action: a}
		pos := fmt.Sprintf("%s %d", where, i+1)
		switch a.Type {
		case "notify":
			if !uuidRe.MatchString(a.ChannelID) {
				return nil, fmt.Errorf("%s: channel_id required", pos)
			}
			if len(a.Subject) > 200 || len(a.Text) > 4000 {
				return nil, fmt.Errorf("%s: subject max 200, text max 4000 characters", pos)
			}
		case "webhook":
			if !strings.HasPrefix(a.URL, "http://") && !strings.HasPrefix(a.URL, "https://") {
				return nil, fmt.Errorf("%s: url must start with http:// or https://", pos)
			}
			if len(a.URL) > 1024 || len(a.Secret) > 256 {
				return nil, fmt.Errorf("%s: url max 1024, secret max 256 characters", pos)
			}
		case "command":
			if !uuidRe.MatchString(a.DeviceID) {
				return nil, fmt.Errorf("%s: device_id required", pos)
			}
			if a.Method == "" {
				ac.Method = "write"
			}
			if !methodRe.MatchString(ac.Method) {
				return nil, fmt.Errorf("%s: method must match ^[a-z][a-z0-9_]{0,31}$", pos)
			}
			if len(a.Args) > 0 {
				if !json.Valid(a.Args) || len(a.Args) > 4096 || a.Args[0] != '{' {
					return nil, fmt.Errorf("%s: args must be a JSON object (max 4096 bytes)", pos)
				}
			} else {
				if !nameRe.MatchString(a.Key) {
					return nil, fmt.Errorf("%s: key required (the writable register or node)", pos)
				}
				if len(a.Value) == 0 {
					return nil, fmt.Errorf("%s: value required (number, true/false or an expression)", pos)
				}
				var num float64
				var b bool
				var str string
				switch {
				case json.Unmarshal(a.Value, &num) == nil:
					ac.valueNum = &num
				case json.Unmarshal(a.Value, &b) == nil:
					ac.valueBool = &b
				case json.Unmarshal(a.Value, &str) == nil:
					ex, err := ParseExpr(str)
					if err != nil {
						return nil, fmt.Errorf("%s: value: %v", pos, err)
					}
					for _, v := range ex.Vars() {
						if _, ok := c.byName[v]; !ok {
							return nil, fmt.Errorf("%s: unknown input %q in value expression", pos, v)
						}
					}
					ac.valueExpr = ex
				default:
					return nil, fmt.Errorf("%s: value must be a number, a boolean or an expression", pos)
				}
			}
		case "alarm":
			if a.Severity == "" {
				ac.Severity = "warning"
			}
			if !sevs[ac.Severity] {
				return nil, fmt.Errorf("%s: severity must be warning, action or critical", pos)
			}
			if a.PolicyID != "" && !uuidRe.MatchString(a.PolicyID) {
				return nil, fmt.Errorf("%s: escalation_policy_id is not a uuid", pos)
			}
			if where == "clear_actions" {
				return nil, fmt.Errorf("%s: an alarm cannot be raised on exit (alarms raised on enter are cleared automatically)", pos)
			}
		case "derive":
			if !uuidRe.MatchString(a.DatastreamID) {
				return nil, fmt.Errorf("%s: datastream_id required (the derived datastream)", pos)
			}
			for _, in := range c.inputs {
				if in.DatastreamID == a.DatastreamID {
					return nil, fmt.Errorf("%s: the derived datastream cannot be an input of the same rule", pos)
				}
			}
			ex, err := ParseExpr(a.Expr)
			if err != nil {
				return nil, fmt.Errorf("%s: expr: %v", pos, err)
			}
			for _, v := range ex.Vars() {
				if _, ok := c.byName[v]; !ok {
					return nil, fmt.Errorf("%s: unknown input %q in expr", pos, v)
				}
			}
			ac.expr = ex
		default:
			return nil, fmt.Errorf("%s: type must be notify, webhook, command, alarm or derive", pos)
		}
		out = append(out, ac)
	}
	return out, nil
}

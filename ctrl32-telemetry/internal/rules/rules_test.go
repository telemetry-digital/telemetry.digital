package rules

import (
	"encoding/json"
	"math"
	"testing"
)

func TestExpr(t *testing.T) {
	cases := []struct {
		src  string
		vars map[string]float64
		want float64
	}{
		{"1 + 2 * 3", nil, 7},
		{"(1 + 2) * 3", nil, 9},
		{"-t + 2", map[string]float64{"t": 5}, -3},
		{"2 ^ 3 ^ 2", nil, 512},
		{"10 % 4", nil, 2},
		{"abs(-3.5)", nil, 3.5},
		{"min(t_in, t_out) + max(1, 2)", map[string]float64{"t_in": 4, "t_out": 9}, 6},
		{"clamp(t, 10, 20)", map[string]float64{"t": 35}, 20},
		{"round(2.5) + floor(1.9) + ceil(1.1)", nil, 3 + 1 + 2},
		{"sqrt(16) / 2", nil, 2},
		{"1e3 + 1.5e-1", nil, 1000.15},
		{"t_in - t_out", map[string]float64{"t_in": 21.5, "t_out": 20}, 1.5},
	}
	for _, c := range cases {
		ex, err := ParseExpr(c.src)
		if err != nil {
			t.Fatalf("%s: parse: %v", c.src, err)
		}
		got, err := ex.Eval(c.vars)
		if err != nil {
			t.Fatalf("%s: eval: %v", c.src, err)
		}
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s = %v, want %v", c.src, got, c.want)
		}
	}
	for _, bad := range []string{"", "1 +", "(1", "foo(1)", "abs()", "1 2", "a $ b", "min(1)"} {
		if _, err := ParseExpr(bad); err == nil {
			t.Errorf("%q should not parse", bad)
		}
	}
	ex, _ := ParseExpr("a / b")
	if _, err := ex.Eval(map[string]float64{"a": 1, "b": 0}); err == nil {
		t.Error("division by zero should fail")
	}
	if _, err := ex.Eval(map[string]float64{"a": 1}); err == nil {
		t.Error("missing input should fail")
	}
	if vs := ex.Vars(); len(vs) != 2 {
		t.Errorf("vars = %v", vs)
	}
}

func TestCompile(t *testing.T) {
	ds1, ds2, ds3 := "018f0000-0000-7000-8000-000000000001", "018f0000-0000-7000-8000-000000000002", "018f0000-0000-7000-8000-000000000003"
	ch := "018f0000-0000-7000-8000-0000000000aa"
	good := Spec{Inputs: []Input{{Name: "t_in", DatastreamID: ds1}, {Name: "t_out", DatastreamID: ds2}}, Match: "all",
		Conditions: []Condition{{Input: "t_in", Op: ">=", Value: json.RawMessage(`30`)}, {Input: "t_out", Op: "<", Value: json.RawMessage(`"t_in - 5"`)}, {Input: "t_out", Op: "stale", Value: json.RawMessage(`600`)}},
		Actions:    []Action{{Type: "notify", ChannelID: ch}, {Type: "derive", DatastreamID: ds3, Expr: "t_in - t_out"}, {Type: "command", DeviceID: ds1, Key: "setpoint", Value: json.RawMessage(`"t_in - 10"`)}, {Type: "alarm", Severity: "action"}}}
	c, err := Compile(good)
	if err != nil {
		t.Fatalf("good spec: %v", err)
	}
	if !c.hasStale || len(c.conds) != 3 || len(c.actions) != 4 || !good.HasCommands() {
		t.Errorf("compiled: stale=%v conds=%d actions=%d", c.hasStale, len(c.conds), len(c.actions))
	}
	vals := map[string]float64{"t_in": 35, "t_out": 20}
	if ok, _ := condition(c, vals, map[string]float64{"t_in": 1, "t_out": 700}); !ok {
		t.Error("condition should hold (35>=30, 20<30, stale 700>600)")
	}
	if ok, _ := condition(c, vals, map[string]float64{"t_in": 1, "t_out": 10}); ok {
		t.Error("condition should fail (t_out fresh)")
	}
	c.match = "any"
	if ok, _ := condition(c, vals, map[string]float64{"t_in": 1, "t_out": 10}); !ok {
		t.Error("any: first condition holds")
	}
	bad := []Spec{
		{Inputs: []Input{{Name: "Bad", DatastreamID: ds1}}, Actions: []Action{{Type: "alarm"}}},
		{Inputs: []Input{{Name: "a", DatastreamID: ds1}}, Conditions: []Condition{{Input: "b", Op: ">", Value: json.RawMessage(`1`)}}, Actions: []Action{{Type: "alarm"}}},
		{Inputs: []Input{{Name: "a", DatastreamID: ds1}}, Conditions: []Condition{{Input: "a", Op: "~", Value: json.RawMessage(`1`)}}, Actions: []Action{{Type: "alarm"}}},
		{Inputs: []Input{{Name: "a", DatastreamID: ds1}}, Conditions: []Condition{{Input: "a", Op: ">", Value: json.RawMessage(`"b + 1"`)}}, Actions: []Action{{Type: "alarm"}}},
		{Inputs: []Input{{Name: "a", DatastreamID: ds1}}},
		{Inputs: []Input{{Name: "a", DatastreamID: ds1}}, Actions: []Action{{Type: "derive", DatastreamID: ds1, Expr: "a"}}},
		{Inputs: []Input{{Name: "a", DatastreamID: ds1}}, Actions: []Action{{Type: "command", DeviceID: ds1, Key: "x"}}},
		{Inputs: []Input{{Name: "a", DatastreamID: ds1}}, Actions: []Action{{Type: "pigeon"}}},
		{Inputs: []Input{{Name: "a", DatastreamID: ds1}}, ClearActions: []Action{{Type: "alarm"}}},
		{Inputs: []Input{{Name: "a", DatastreamID: ds1}}, Actions: []Action{{Type: "webhook", URL: "ftp://x"}}},
		{Inputs: []Input{{Name: "a", DatastreamID: ds1}}, ForS: -1, Actions: []Action{{Type: "alarm"}}},
	}
	for i, b := range bad {
		if _, err := Compile(b); err == nil {
			t.Errorf("bad spec %d should be refused", i)
		}
	}
}

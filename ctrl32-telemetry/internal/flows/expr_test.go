package flows

import (
	"strings"
	"testing"
	"time"
)

type testEnv map[string]Value

func (e testEnv) Lookup(n string) (Value, bool) { v, ok := e[n]; return v, ok }
func (e testEnv) Location() *time.Location      { return time.UTC }
func (e testEnv) Now() time.Time                { return time.Date(2026, 9, 25, 14, 30, 0, 0, time.UTC) } // Friday

func TestExprValues(t *testing.T) {
	env := testEnv{"value": 21.5, "topic": "Mixer/temp", "msg.count": 3.0, "flow.limit": 80.0, "quality": "ok"}
	cases := map[string]Value{
		"1 + 2 * 3":                          7.0,
		"(1 + 2) * 3":                        9.0,
		"2 ^ 3 ^ 2":                          512.0,
		"-2 ^ 2":                             -4.0,
		"10 % 4":                             2.0,
		"value > 20":                         true,
		"value > flow.limit":                 false,
		"value >= 21.5 && quality == 'ok'":   true,
		"!(value < 0) || false":              true,
		"value > 30 ? 'hot' : 'ok'":          "ok",
		"round(value * 1.8 + 32, 1)":         70.7,
		"max(1, value, msg.count)":           21.5,
		"clamp(150, 0, 100)":                 100.0,
		"'T=' + value + ' C'":                "T=21.5 C",
		"upper(topic)":                       "MIXER/TEMP",
		"contains(topic, 'temp')":            true,
		"substr(topic, 0, 5)":                "Mixer",
		"replace(topic, '/', '.')":           "Mixer.temp",
		"len(topic)":                         10.0,
		"num('3.5') + 1":                     4.5,
		"fixed(value, 2)":                    "21.50",
		"fixed(7.25, 1)":                     "7.3",
		"hour()":                             14.0,
		"weekday()":                          5.0,
		"fmtTime(now(), 'YYYY-MM-DD HH:mm')": "2026-09-25 14:30",
		"coalesce(msg.missing, 7)":           7.0,
		"isNull(msg.missing)":                true,
		"msg.missing > 1":                    false,
		"if(value > 0, 'pos', 1 / 0)":        "pos",
		"true && value":                      true,
		"'b' > 'a'":                          true,
		"null == null":                       true,
		"1e3 + .5":                           1000.5,
	}
	for src, want := range cases {
		e, err := Parse(src)
		if err != nil {
			t.Errorf("%s: %v", src, err)
			continue
		}
		got, err := e.Eval(env)
		if err != nil {
			t.Errorf("%s: eval %v", src, err)
			continue
		}
		if got != want {
			t.Errorf("%s = %#v, want %#v", src, got, want)
		}
	}
}

func TestExprRefused(t *testing.T) {
	bad := []string{"", "1 +", "(1", "foo", "value.x", "msg.a.b", "system('x')", "exec()", "abs()", "abs(1,2)", "'open",
		"1 ? 2", "@", "a = 1", "value;1", strings.Repeat("1+", 600) + "1", strings.Repeat("(", 70) + "1" + strings.Repeat(")", 70)}
	for _, src := range bad {
		if _, err := Parse(src); err == nil {
			t.Errorf("%q accepted", src)
		}
	}
	for src, msg := range map[string]string{"1 / 0": "division", "'a' * 2": "not numbers", "-'x'": "not a number"} {
		e, err := Parse(src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if _, err := e.Eval(testEnv{}); err == nil || !strings.Contains(err.Error(), msg) {
			t.Errorf("%s: %v", src, err)
		}
	}
	// text growth is bounded
	e, _ := Parse("replace(replace(replace(topic, 'a', topic), 'a', topic), 'a', topic)")
	if _, err := e.Eval(testEnv{"topic": strings.Repeat("a", 100)}); err == nil {
		t.Error("unbounded text accepted")
	}
	e, _ = Parse("value + msg.x")
	names := strings.Join(e.Names(), ",")
	if names != "value,msg.x" {
		t.Errorf("names %s", names)
	}
}

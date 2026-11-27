package httpapi

import (
	"strings"
	"testing"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

func TestGroupKeyOf(t *testing.T) {
	for in, want := range map[string]string{"Shift leader": "shift_leader", "  Lab 2 / night ": "lab_2_night", "Údržba": "dr_ba", "9 lives": "lives", "Operator": "operator"} {
		if got := groupKeyOf(in); got != want {
			t.Errorf("groupKeyOf(%q) = %q, want %q", in, got, want)
		}
	}
	if k := groupKeyOf(strings.Repeat("a", 50)); len(k) != 32 || !scadaGroupKeyRe.MatchString(k) {
		t.Errorf("a long name gives %q", k)
	}
}

func TestValidGroupKeys(t *testing.T) {
	got, err := validGroupKeys([]any{"operator", "shift_leader", "operator"})
	if err != nil || strings.Join(got, ",") != "operator,shift_leader" {
		t.Errorf("list: %v %v", got, err)
	}
	if got, err := validGroupKeys("operator, technologist"); err != nil || len(got) != 2 {
		t.Errorf("text: %v %v", got, err)
	}
	for _, bad := range []any{[]any{"Not A Key"}, []any{"1x"}, "a,B"} {
		if _, err := validGroupKeys(bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
	many := make([]any, 17)
	for i := range many {
		many[i] = "g" + string(rune('a'+i))
	}
	if _, err := validGroupKeys(many); err == nil {
		t.Error("17 groups accepted")
	}
}

// keysIn finds the key of an element and the keys of its events, so that a command can be matched to its control.
func TestKeysIn(t *testing.T) {
	e := map[string]any{"id": "m1", "key": "run", "on": []any{map[string]any{"act": "command", "key": "reset", "value": 1.0}, map[string]any{"act": "popup", "target": "x"}}}
	keys := map[string]bool{}
	keysIn(e, keys)
	if len(keys) != 2 || !keys["run"] || !keys["reset"] {
		t.Errorf("keys: %v", keys)
	}
}

// SCADA users change nothing until somebody sets them up: nobody is required and the log is kept for good.
func TestScadaOffByDefault(t *testing.T) {
	var p iam.Policy
	if p.ScadaUsersRequired || p.OperatorLogDays != 0 || p.ScadaIdle() != iam.DefaultScadaIdleMinutes {
		t.Errorf("defaults: %+v", p)
	}
	p.ScadaIdleMinutes = 9999
	if p.ScadaIdle() != iam.DefaultScadaIdleMinutes {
		t.Error("an idle time out of range must fall back to the default")
	}
}

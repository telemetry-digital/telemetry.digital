package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

func TestCmdSummary(t *testing.T) {
	cases := []struct{ dev, method, args, want string }{
		{"Motor M1", "write", `{"key":"run","value":true}`, "Motor M1: run → true"},
		{"Motor M1", "write", `{"key":"mode","value":"auto"}`, "Motor M1: mode → auto"},
		{"Pump", "set_speed", `{"key":"speed","value":42.5}`, "Pump: set_speed speed → 42.5"},
		{"Pump", "reboot", `{}`, "Pump: reboot {}"},
		{"", "reboot", `{}`, "reboot {}"},
	}
	for _, c := range cases {
		if got := cmdSummary(c.dev, c.method, json.RawMessage(c.args)); got != c.want {
			t.Errorf("cmdSummary(%q, %q, %s) = %q, want %q", c.dev, c.method, c.args, got, c.want)
		}
	}
}

func TestFourEyesOffByDefault(t *testing.T) {
	var pol iam.Policy
	for area, on := range fourEyesAreas(pol) {
		if on {
			t.Errorf("area %s is on in an empty policy", area)
		}
	}
	if pol.SecondPersonMinutes() != iam.DefaultFourEyesMinutes {
		t.Errorf("default minutes = %d", pol.SecondPersonMinutes())
	}
	pol.FourEyesMinutes = 9999
	if pol.SecondPersonMinutes() != iam.DefaultFourEyesMinutes {
		t.Errorf("out of range minutes must fall back to the default")
	}
	for area := range secondAreas {
		if _, ok := fourEyesAreas(pol)[area]; !ok {
			t.Errorf("area %s has no switch", area)
		}
	}
}

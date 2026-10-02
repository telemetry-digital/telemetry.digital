package alarm

import (
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/ingest"
)

func fp(v float64) *float64 { return &v }

// Every rule type that can be saved is evaluated: limits, states and the silence of comm_loss.
func TestSeverityOfEveryType(t *testing.T) {
	cases := []struct {
		r    rule
		v    float64
		want string
	}{
		{rule{Type: "high", Warning: fp(7), Action: fp(8)}, 7.5, "warning"},
		{rule{Type: "high", Warning: fp(7), Action: fp(8)}, 8, "action"},
		{rule{Type: "high", Warning: fp(7), Action: fp(8)}, 6, ""},
		{rule{Type: "low", Action: fp(2)}, 1, "action"},
		{rule{Type: "low_battery", Warning: fp(20), Action: fp(10)}, 15, "warning"},
		{rule{Type: "low_battery", Warning: fp(20), Action: fp(10)}, 9, "action"},
		{rule{Type: "low_battery", Warning: fp(20), Action: fp(10)}, 80, ""},
		{rule{Type: "power_loss"}, 0, "action"},
		{rule{Type: "power_loss"}, 1, ""},
		{rule{Type: "door_open"}, 1, "warning"},
		{rule{Type: "door_open"}, 0, ""},
		{rule{Type: "comm_loss", Warning: fp(60), Action: fp(300)}, 61, "warning"},
		{rule{Type: "comm_loss", Warning: fp(60), Action: fp(300)}, 301, "action"},
		{rule{Type: "comm_loss", Warning: fp(60), Action: fp(300)}, 10, ""},
	}
	for _, c := range cases {
		if got := severity(c.r, c.v); got != c.want {
			t.Errorf("%s %v at %v = %q, want %q", c.r.Type, c.r, c.v, got, c.want)
		}
	}
	// clearing: back in band by the hysteresis, or the state back to normal
	if !backInBand(rule{Type: "low_battery", Warning: fp(20), Hysteresis: 5}, 26) || backInBand(rule{Type: "low_battery", Warning: fp(20), Hysteresis: 5}, 24) {
		t.Error("low_battery hysteresis")
	}
	if !backInBand(rule{Type: "door_open"}, 0) || backInBand(rule{Type: "door_open"}, 1) || !backInBand(rule{Type: "power_loss"}, 1) {
		t.Error("state types clear when the state is back")
	}
	found := false
	for _, typ := range Evaluated {
		found = found || typ == "sensor_fault"
	}
	if !found {
		t.Error("sensor_fault must be evaluated (0.67.0)")
	}
}

// A value with quality sensor_fault is not a reading: value rules skip it, sensor_fault and comm_loss see it.
func TestFaultyValueSkipsValueRules(t *testing.T) {
	v := -127.0
	faulty := ingest.Observation{Value: &v, Quality: FaultQuality}
	good := ingest.Observation{Value: &v, Quality: "ok"}
	noValue := ingest.Observation{Quality: FaultQuality}
	for _, typ := range []string{"high", "low", "low_battery", "power_loss", "door_open"} {
		if applies(rule{Type: typ}, faulty) {
			t.Errorf("%s evaluated on a faulty value", typ)
		}
		if !applies(rule{Type: typ}, good) {
			t.Errorf("%s not evaluated on a good value", typ)
		}
	}
	for _, typ := range []string{"sensor_fault", "comm_loss"} {
		if !applies(rule{Type: typ}, faulty) || !applies(rule{Type: typ}, noValue) || !applies(rule{Type: typ}, good) {
			t.Errorf("%s must see every value", typ)
		}
	}
}

// The delay: the condition must hold for the delay before the alarm is raised.
func TestDelayHeld(t *testing.T) {
	t0 := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	if !held(t0, t0, 0) {
		t.Error("no delay: at once")
	}
	if held(t0, t0.Add(59*time.Second), time.Minute) {
		t.Error("59 s of a 60 s delay raised")
	}
	if !held(t0, t0.Add(time.Minute), time.Minute) {
		t.Error("60 s of a 60 s delay not raised")
	}
	if worse("warning", "action") != "action" || worse("", "warning") != "warning" || worse("warning", "warning") != "warning" {
		t.Error("worse severity")
	}
}

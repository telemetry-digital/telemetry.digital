package protocol

import "testing"

// Sensor faults reported by the device (0.67.0): "fault" as a list, an object or true next to "v"/"values".
func TestFaultFlags(t *testing.T) {
	cases := []struct {
		name     string
		payload  string
		faulty   []string // keys flagged
		healthy  []string // keys not flagged
		values   []string // keys that must be values
		notValue []string // keys that must not be values
	}{
		{"list", `{"v":{"temp":-127,"rh":40},"fault":["temp"]}`, []string{"temp"}, []string{"rh"}, []string{"temp", "rh"}, []string{"fault"}},
		{"object with reason", `{"values":{"temp":-127,"rh":40},"fault":{"Temp":"open circuit","rh":false}}`, []string{"temp"}, []string{"rh"}, []string{"temp", "rh"}, nil},
		{"all values", `{"v":{"temp":-127,"rh":40},"fault":true}`, []string{"temp", "rh"}, nil, []string{"temp"}, nil},
		{"no fault", `{"v":{"temp":4},"fault":false}`, nil, []string{"temp"}, []string{"temp"}, nil},
		{"flat list", `{"temp":-127,"rh":40,"fault":["temp"]}`, []string{"temp"}, []string{"rh"}, []string{"temp", "rh"}, []string{"fault"}},
		{"flat number stays a value", `{"temp":4,"fault":3}`, nil, []string{"temp", "fault"}, []string{"temp", "fault"}, nil},
		{"flat boolean stays a value", `{"temp":4,"fault":true}`, nil, []string{"temp"}, []string{"fault"}, nil},
	}
	for _, c := range cases {
		s, err := ParseTelemetry([]byte(c.payload))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		for _, k := range c.faulty {
			if !s[0].Faulty(k) {
				t.Errorf("%s: %s should be faulty", c.name, k)
			}
		}
		for _, k := range c.healthy {
			if s[0].Faulty(k) {
				t.Errorf("%s: %s should not be faulty", c.name, k)
			}
		}
		for _, k := range c.values {
			if _, ok := s[0].Values[k]; !ok {
				t.Errorf("%s: %s should be a value", c.name, k)
			}
		}
		for _, k := range c.notValue {
			if _, ok := s[0].Values[k]; ok {
				t.Errorf("%s: %s should not be a value", c.name, k)
			}
		}
	}
	for _, bad := range []string{`{"v":{"temp":1},"fault":"temp"}`, `{"v":{"temp":1},"fault":["1bad"]}`, `{"v":{"temp":1},"fault":[1]}`} {
		if _, err := ParseTelemetry([]byte(bad)); err == nil {
			t.Errorf("expected error for %s", bad)
		}
	}
	// a batch: flags belong to their own sample
	s, err := ParseTelemetry([]byte(`[{"ts":1700000000,"v":{"temp":-127},"fault":["temp"]},{"ts":1700000060,"v":{"temp":4.1}}]`))
	if err != nil || !s[0].Faulty("temp") || s[1].Faulty("temp") {
		t.Errorf("batch flags: %v %+v", err, s)
	}
}

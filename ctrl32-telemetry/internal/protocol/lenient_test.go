package protocol

import (
	"testing"
	"time"
)

func TestLenientFormats(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		keys    []string
		ts      *time.Time
		wantErr bool
	}{
		{"ctrl32", `{"ts":1700000000,"v":{"temp":4.2}}`, []string{"temp"}, tp(time.Unix(1700000000, 0)), false},
		{"thingsboard values + ms", `{"ts":1700000000123,"values":{"temp":4.2,"rh":61}}`, []string{"temp", "rh"}, tp(time.UnixMilli(1700000000123)), false},
		{"flat", `{"temp":4.2,"rh":61,"door":true,"mode":"eco"}`, []string{"temp", "rh", "door", "mode"}, nil, false},
		{"flat with ts seconds", `{"ts":1700000000,"temp":4.2}`, []string{"temp"}, tp(time.Unix(1700000000, 0)), false},
		{"flat with ts milliseconds", `{"ts":1700000000500,"temp":4.2}`, []string{"temp"}, tp(time.UnixMilli(1700000000500)), false},
		{"key normalisation", `{"Temp-1":4.2,"Batt Level":80,"Sensor.RH":61}`, []string{"temp_1", "batt_level", "sensor_rh"}, nil, false},
		{"array mixed", `[{"temp":4.1,"bf":1},{"ts":1700000000,"values":{"temp":4.2}}]`, []string{"temp"}, nil, false},
		{"bad key", `{"1temp":4.2}`, nil, nil, true},
		{"empty object", `{}`, nil, nil, true},
		{"only reserved", `{"ts":1700000000}`, nil, nil, true},
		{"null value", `{"temp":null}`, nil, nil, true},
	}
	for _, c := range cases {
		samples, err := ParseTelemetry([]byte(c.payload))
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: expected error", c.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		s := samples[0]
		for _, k := range c.keys {
			if _, ok := s.Values[k]; !ok {
				t.Errorf("%s: key %q missing (got %v)", c.name, k, keysOf(s))
			}
		}
		if c.ts != nil {
			if s.MeasuredAt == nil || !s.MeasuredAt.Equal(*c.ts) {
				t.Errorf("%s: measured_at %v, want %v", c.name, s.MeasuredAt, *c.ts)
			}
		} else if s.MeasuredAt != nil && c.name == "flat" {
			t.Errorf("%s: no timestamp expected", c.name)
		}
	}
	if s, err := ParseTelemetry([]byte(`[{"temp":4.1,"bf":1},{"ts":1700000000,"values":{"temp":4.2}}]`)); err != nil || !s[0].Backfilled || s[1].Backfilled {
		t.Errorf("array flags: %v %+v", err, s)
	}
}

func tp(t time.Time) *time.Time { u := t.UTC(); return &u }

func keysOf(s Sample) []string {
	out := []string{}
	for k := range s.Values {
		out = append(out, k)
	}
	return out
}

func TestNormalizeKey(t *testing.T) {
	for in, want := range map[string]string{"temp": "temp", "Temp": "temp", "Temp-1": "temp_1", "a b": "a_b", "x.y": "x_y"} {
		if got, ok := NormalizeKey(in); !ok || got != want {
			t.Errorf("%q → %q (%v), want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "1abc", "_x", "temp/1", "ěšč", "abcdefghijabcdefghijabcdefghijabcdefghij"} {
		if _, ok := NormalizeKey(bad); ok {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

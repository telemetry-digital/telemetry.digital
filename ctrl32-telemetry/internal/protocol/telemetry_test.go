package protocol

import (
	"strings"
	"testing"
	"time"
)

// URS-A-07, URS-B-04: single sample with device timestamp and numeric/bool values.
func TestParseSingle_URS_A07_B04(t *testing.T) {
	s, err := ParseTelemetry([]byte(`{"ts": 1758620000, "v": {"temp": 4.2, "rh": 61, "door": false}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 1 {
		t.Fatalf("want 1 sample, got %d", len(s))
	}
	if s[0].MeasuredAt == nil || !s[0].MeasuredAt.Equal(time.Unix(1758620000, 0)) {
		t.Fatalf("measured_at not parsed: %v", s[0].MeasuredAt)
	}
	if v := s[0].Values["temp"]; v.Num == nil || *v.Num != 4.2 {
		t.Fatalf("temp not parsed: %+v", v)
	}
	if v := s[0].Values["door"]; v.Bool == nil || *v.Bool {
		t.Fatalf("door not parsed: %+v", v)
	}
	if s[0].Backfilled {
		t.Fatal("must not be backfilled")
	}
}

// URS-B-04, URS-L-03: batch with backfill flag and millisecond timestamps.
func TestParseBatchBackfill_URS_B04_L03(t *testing.T) {
	s, err := ParseTelemetry([]byte(`[{"tsm": 1758619400123, "v": {"temp": 4.1}, "bf": 1, "seq": 41}, {"ts": 1758620000, "v": {"temp": 4.2}, "seq": 42}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 2 {
		t.Fatalf("want 2 samples, got %d", len(s))
	}
	if !s[0].Backfilled || s[1].Backfilled {
		t.Fatalf("backfill flags wrong: %v %v", s[0].Backfilled, s[1].Backfilled)
	}
	if s[0].MeasuredAt == nil || s[0].MeasuredAt.UnixMilli() != 1758619400123 {
		t.Fatalf("tsm not parsed: %v", s[0].MeasuredAt)
	}
	if s[0].Seq == nil || *s[0].Seq != 41 {
		t.Fatalf("seq not parsed: %v", s[0].Seq)
	}
}

// URS-B-04: a device without a clock omits ts; the server will assign received_at.
func TestParseNoTimestamp_URS_B04(t *testing.T) {
	s, err := ParseTelemetry([]byte(`{"v": {"temp": 4.2}}`))
	if err != nil {
		t.Fatal(err)
	}
	if s[0].MeasuredAt != nil || s[0].Relative != nil {
		t.Fatal("expected no timestamp")
	}
	s, err = ParseTelemetry([]byte(`{"dt": -300, "v": {"temp": 4.0}}`))
	if err != nil {
		t.Fatal(err)
	}
	if s[0].Relative == nil || *s[0].Relative != -300 {
		t.Fatalf("relative time not parsed: %v", s[0].Relative)
	}
}

// URS-H-12, URS-A-06: input validation — bad keys, null values, oversized payloads and wrong protocol versions are rejected.
func TestParseRejects_URS_H12_A06(t *testing.T) {
	bad := []string{
		`{"v": {"1temp": 1}}`,        // key that cannot be normalised
		`{"v": {"temp": null}}`,      // null value
		`{"v": {}}`,                  // no values
		`{"p": 2, "v": {"temp": 1}}`, // unsupported version
		`{"v": {"s": "` + strings.Repeat("x", MaxStringValue+1) + `"}}`,
		`not json`,
	}
	for _, b := range bad {
		if _, err := ParseTelemetry([]byte(b)); err == nil {
			t.Errorf("expected error for %.40s", b)
		}
	}
	big := []byte("[" + strings.Repeat(`{"v":{"t":1}},`, 3000) + `{"v":{"t":1}}]`)
	if _, err := ParseTelemetry(big); err == nil {
		t.Error("expected error for oversized batch")
	}
}

// URS-B-02: structured values (state/event) are kept as raw JSON.
func TestParseStructured_URS_B02(t *testing.T) {
	s, err := ParseTelemetry([]byte(`{"v": {"gps": {"lat": 48.1, "lon": 17.1}, "mode": "auto"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if s[0].Values["gps"].Kind() != "json" || s[0].Values["mode"].Kind() != "str" {
		t.Fatalf("kinds wrong: %s %s", s[0].Values["gps"].Kind(), s[0].Values["mode"].Kind())
	}
}

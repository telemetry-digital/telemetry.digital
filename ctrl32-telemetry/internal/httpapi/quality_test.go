package httpapi

import (
	"testing"
	"time"
)

func TestLiveQuality(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		stored    string
		age       time.Duration
		expectedS float64
		want      string
		stale     bool
	}{
		{"ok", 10 * time.Minute, 0, "ok", false},                      // no expected interval: never stale
		{"ok", 80 * time.Second, 60, "ok", false},                     // within 1.5 × 60 s
		{"ok", 91 * time.Second, 60, "stale", true},                   // beyond
		{"backfilled", time.Hour, 600, "stale", true},                 // good data that is old
		{"sensor_fault", time.Hour, 600, "sensor_fault", true},        // the stored fault stays, stale reported
		{"out_of_range", 2 * time.Minute, 600, "out_of_range", false}, // counting clamp
	} {
		q, st := liveQuality(c.stored, now.Add(-c.age), c.expectedS, now)
		if q != c.want || st != c.stale {
			t.Errorf("%s %v/%gs: got %s %v, want %s %v", c.stored, c.age, c.expectedS, q, st, c.want, c.stale)
		}
	}
	l := map[string]any{}
	applyLive(l, "ok", now.Add(-time.Hour), 60, now)
	if l["quality"] != "stale" || l["stale"] != true || l["stored_quality"] != "ok" {
		t.Errorf("applyLive: %v", l)
	}
	l = map[string]any{}
	applyLive(l, "ok", now, 60, now)
	if l["quality"] != "ok" || l["stale"] != nil || l["stored_quality"] != nil {
		t.Errorf("applyLive fresh: %v", l)
	}
}

package agent

import (
	"strings"
	"testing"
	"time"
)

func TestUpdateEntitled(t *testing.T) {
	now := time.Date(2027, 11, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		until, out string
		ok         bool
		why        string
	}{
		{"", "ctrl32-telemetry 0.90.0 released 2030-01-01", true, "no licence limit: free servers always update"},
		{"2027-10-02", "ctrl32-telemetry 0.70.0 released 2027-10-02", true, "released on the last day of the updates"},
		{"2027-10-02", "ctrl32-telemetry 0.71.0 released 2027-10-03", false, "released after the updates ended"},
		{"2027-10-02", "ctrl32-telemetry 0.65.0", false, "unknown date after the updates ended"},
		{"2027-12-31", "ctrl32-telemetry 0.65.0", true, "unknown date while the updates run"},
		{"bad", "ctrl32-telemetry 0.70.0 released 2027-01-01", false, "invalid limit"},
	}
	for _, c := range cases {
		msg := updateEntitled(c.until, c.out, now)
		if (msg == "") != c.ok {
			t.Fatalf("%s: %q", c.why, msg)
		}
	}
	if msg := updateEntitled("2027-10-02", "ctrl32-telemetry 0.71.0 released 2027-10-03", now); !strings.Contains(msg, "keeps running") {
		t.Fatalf("message must say the system keeps running: %q", msg)
	}
}

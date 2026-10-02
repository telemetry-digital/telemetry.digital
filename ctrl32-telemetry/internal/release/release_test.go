package release

import (
	"testing"
	"time"
)

func TestFromVersionOutput(t *testing.T) {
	d, ok := FromVersionOutput("ctrl32-telemetry 0.66.0 released 2026-10-02\n")
	if !ok || d.Format(Layout) != "2026-10-02" {
		t.Fatalf("got %v %v", d, ok)
	}
	if _, ok := FromVersionOutput("ctrl32-telemetry 0.65.0\n"); ok {
		t.Fatal("old binaries print no date")
	}
	if _, ok := FromVersionOutput("ctrl32-telemetry 0.66.0 released 2026-13-45"); ok {
		t.Fatal("invalid date accepted")
	}
}

func TestCovered(t *testing.T) {
	day := func(s string) time.Time { d, _ := time.Parse(Layout, s); return d }
	if !Covered(day("2027-10-02"), time.Time{}) {
		t.Fatal("no end = unlimited")
	}
	if !Covered(day("2027-10-02"), day("2027-10-02")) {
		t.Fatal("the last day is included")
	}
	if Covered(day("2027-10-03"), day("2027-10-02")) {
		t.Fatal("a later release is not covered")
	}
}

func TestDateFromLdflags(t *testing.T) {
	old := date
	defer func() { date = old }()
	date = "2026-10-02"
	if String() != "2026-10-02" {
		t.Fatalf("got %q", String())
	}
}

package counting

import (
	"log/slog"
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics"
)

func TestRecentPassages(t *testing.T) {
	e := &Engine{status: map[string]*CameraStatus{}, log: slog.Default()}
	s := camSpec{ID: "cam", Cfg: CameraConfig{Lines: []CameraLine{{Line: analytics.Line{ID: "l1"}}}}}
	t0 := time.Date(2026, 10, 12, 20, 0, 0, 0, time.UTC)
	e.handleEvent(s, analytics.Event{Type: "cross", Cross: &analytics.Crossing{Line: "l1", Dir: "in", Class: "person", At: t0}})
	first := e.Status("cam").Recent
	if len(first) != 1 || first[0].Dir != "in" || first[0].Line != "l1" || !first[0].At.Equal(t0) {
		t.Fatalf("recent = %+v", first)
	}
	for i := 1; i <= recentPassages+5; i++ {
		e.handleEvent(s, analytics.Event{Type: "cross", Cross: &analytics.Crossing{Line: "l1", Dir: "out", Class: "person", At: t0.Add(time.Duration(i) * time.Second)}})
	}
	r := e.Status("cam").Recent
	if len(r) != recentPassages || !r[len(r)-1].At.Equal(t0.Add(time.Duration(recentPassages+5)*time.Second)) || r[0].Dir != "out" {
		t.Fatalf("kept %d, last %v", len(r), r[len(r)-1].At)
	}
	if len(first) != 1 || first[0].Dir != "in" {
		t.Fatal("a status handed out earlier changed")
	}
}

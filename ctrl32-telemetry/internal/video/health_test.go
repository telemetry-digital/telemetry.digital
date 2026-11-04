package video

import (
	"context"
	"testing"
	"time"
)

type fakeIncidents struct {
	raised, resolved []string
	open             map[string]bool
}

func (f *fakeIncidents) Raise(_ context.Context, _, _, key, _, _, _ string, _ map[string]any) error {
	f.raised = append(f.raised, key)
	f.open[key] = true
	return nil
}

func (f *fakeIncidents) Resolve(_ context.Context, key, _ string) {
	f.resolved = append(f.resolved, key)
	delete(f.open, key)
}

func (f *fakeIncidents) IsOpen(_ context.Context, key string) bool { return f.open[key] }

func TestFileTooLong(t *testing.T) {
	now := time.Now()
	const segment = 60 * time.Second
	for _, c := range []struct {
		name   string
		opened time.Time
		want   bool
	}{
		{"no file open", time.Time{}, false},
		{"a new file", now.Add(-10 * time.Second), false},
		{"key frames far apart", now.Add(-4 * time.Minute), false},
		{"just below the limit", now.Add(-5*time.Minute + time.Second), false},
		{"just above the limit", now.Add(-5*time.Minute - time.Second), true},
		{"days in one file", now.Add(-96 * time.Hour), true},
	} {
		if got := fileTooLong(c.opened, now, segment); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}

// A camera that keeps writing one file gets an incident once it lasts; a new file ends it. With the time 0 (the setting
// that switches these incidents off) nothing is raised.
func TestRotationIncident(t *testing.T) {
	ctx := context.Background()
	inc := &fakeIncidents{open: map[string]bool{}}
	m := &Manager{Incidents: inc, health: healthState{since: map[string]time.Time{}, raised: map[string]bool{}}}
	set := Settings{SegmentSeconds: 60}
	rec := &recorder{}
	check := func(after time.Duration) { m.checkRotation(ctx, "c1", "o1", "Gate", rec, set, after) }

	rec.opened.Store(time.Now().Add(-time.Hour).UnixNano())
	check(0)
	check(0)
	if len(inc.raised) != 0 {
		t.Fatalf("raised with the check switched off: %v", inc.raised)
	}
	time.Sleep(20 * time.Millisecond) // the clock of Windows moves in steps of up to about 16 ms
	check(time.Nanosecond)
	if len(inc.raised) != 1 || inc.raised[0] != "camera:c1:rotation" {
		t.Fatalf("raised: %v", inc.raised)
	}
	check(time.Nanosecond)
	if len(inc.raised) != 1 {
		t.Fatalf("raised twice: %v", inc.raised)
	}
	rec.opened.Store(time.Now().UnixNano())
	check(time.Nanosecond)
	if len(inc.resolved) != 1 || inc.open["camera:c1:rotation"] {
		t.Fatalf("not resolved: %v", inc.resolved)
	}
}

package diskwatch

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

type fakeInc struct {
	open     map[string]string // key -> severity
	raised   []string
	resolved []string
}

func (f *fakeInc) Raise(_ context.Context, _, _, key, sev, _, _ string, _ map[string]any) error {
	if f.open == nil {
		f.open = map[string]string{}
	}
	f.open[key] = sev
	f.raised = append(f.raised, key)
	return nil
}
func (f *fakeInc) Resolve(_ context.Context, key, _ string) {
	delete(f.open, key)
	f.resolved = append(f.resolved, key)
}
func (f *fakeInc) IsOpen(_ context.Context, key string) bool { _, ok := f.open[key]; return ok }

type disk struct {
	free, total uint64
	id          string
}

func watcher(inc *fakeInc, disks map[string]*disk) *Watcher {
	w := &Watcher{Paths: []string{"/", "/var/lib/x", "/var/lib/x/backups"}, Inc: inc, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	w.stat = func(p string) (uint64, uint64, string, bool) {
		if d, ok := disks[p]; ok {
			return d.free, d.total, d.id, true
		}
		d := disks["/"]
		return d.free, d.total, d.id, true
	}
	return w
}

const gb = 1_000_000_000

func TestOneDiskLowThenCriticalThenFree(t *testing.T) {
	root := &disk{free: 20 * gb, total: 38 * gb, id: "sda1"}
	inc := &fakeInc{}
	w := watcher(inc, map[string]*disk{"/": root})
	ctx := context.Background()
	if ds := w.Disks(); len(ds) != 1 || len(ds[0].Paths) != 3 {
		t.Fatalf("one file system with three paths expected, got %+v", ds)
	}
	w.Check(ctx)
	w.Check(ctx)
	if len(inc.raised) != 0 {
		t.Fatalf("plenty free, raised %v", inc.raised)
	}
	root.free = 5 * gb // 13 %
	w.Check(ctx)
	if len(inc.raised) != 0 {
		t.Fatalf("one low reading raises nothing, got %v", inc.raised)
	}
	w.Check(ctx)
	if inc.open["host:disk:/:low"] != "major" || len(inc.open) != 1 {
		t.Fatalf("low disk: %v", inc.open)
	}
	w.Check(ctx)
	if len(inc.raised) != 1 {
		t.Fatalf("raised once only, got %v", inc.raised)
	}
	root.free = 1.5 * gb // 3.9 %
	w.Check(ctx)
	w.Check(ctx)
	if inc.open["host:disk:/:critical"] != "critical" {
		t.Fatalf("critical expected: %v", inc.open)
	}
	root.free = 5.9 * gb // 15.5 %: within the hysteresis of the warning, above the critical one
	w.Check(ctx)
	if _, ok := inc.open["host:disk:/:critical"]; ok {
		t.Fatalf("critical ends at 7 %%+: %v", inc.open)
	}
	if _, ok := inc.open["host:disk:/:low"]; !ok {
		t.Fatalf("the warning stays within its hysteresis: %v", inc.open)
	}
	root.free = 10 * gb
	w.Check(ctx)
	if len(inc.open) != 0 {
		t.Fatalf("all resolved expected: %v", inc.open)
	}
}

func TestClosedByPersonRaisedAgain(t *testing.T) {
	root := &disk{free: 1 * gb, total: 100 * gb, id: "sda1"}
	inc := &fakeInc{}
	w := watcher(inc, map[string]*disk{"/": root})
	ctx := context.Background()
	w.Check(ctx)
	w.Check(ctx)
	if len(inc.open) != 2 {
		t.Fatalf("low and critical expected: %v", inc.open)
	}
	delete(inc.open, "host:disk:/:critical") // someone closed it, the disk is still full
	w.Check(ctx)
	w.Check(ctx)
	w.Check(ctx)
	if inc.open["host:disk:/:critical"] != "critical" {
		t.Fatalf("raised again: %v", inc.open)
	}
}

func TestOpenBeforeRestartIsResolved(t *testing.T) {
	root := &disk{free: 30 * gb, total: 38 * gb, id: "sda1"}
	inc := &fakeInc{open: map[string]string{"host:disk:/:low": "major"}}
	w := watcher(inc, map[string]*disk{"/": root})
	w.Check(context.Background())
	if len(inc.open) != 0 {
		t.Fatalf("an incident from before the restart ends: %v", inc.open)
	}
}

func TestVideoLimitOnSharedDisk(t *testing.T) {
	// production before 20.11.2026: 38 GB disk, 15 GB video limit, 5 GB recordings so far, 10 GB free
	root := &disk{free: 10 * gb, total: 38 * gb, id: "sda1"}
	inc := &fakeInc{}
	w := watcher(inc, map[string]*disk{"/": root})
	limit := int64(15 << 30)
	w.VideoDir = "/var/lib/x/video"
	w.VideoLimit = func() int64 { return limit }
	w.VideoUsed = func(context.Context) (int64, error) { return 5 * gb, nil }
	ctx := context.Background()
	w.Check(ctx)
	w.Check(ctx)
	if inc.open["host:disk:/:video-limit"] != "major" || len(inc.open) != 1 {
		t.Fatalf("video limit incident expected: %v", inc.open)
	}
	limit = 6 << 30 // the limit lowered: 10 + 5 - 6.4 = 8.6 GB (22 %) free at the limit
	w.Check(ctx)
	if len(inc.open) != 0 {
		t.Fatalf("resolved after lowering the limit: %v", inc.open)
	}
}

func TestVideoOnOwnDiskIsNotWatched(t *testing.T) {
	root := &disk{free: 200 * gb, total: 240 * gb, id: "sda1"}
	raid := &disk{free: 100 * gb, total: 8000 * gb, id: "md0"} // kept full by the video limit on purpose
	inc := &fakeInc{}
	w := watcher(inc, map[string]*disk{"/": root, "/srv/video": raid})
	w.VideoDir = "/srv/video"
	w.VideoLimit = func() int64 { return 7000 << 30 }
	w.VideoUsed = func(context.Context) (int64, error) { return 7400 * gb, nil }
	ctx := context.Background()
	w.Check(ctx)
	w.Check(ctx)
	if len(inc.raised) != 0 {
		t.Fatalf("the recordings' own disk is the video manager's: %v", inc.raised)
	}
}

// Package diskwatch raises incidents when the disk of the server runs low (0.87.6). A full system disk stops the
// database and with it the whole server (production 15.–20. 11. 2026: video, database and backups filled a 38 GB
// disk and nobody was told), so the warning comes while there is still room to act: below 15 % free a major incident,
// below 5 % (or 1 GB) a critical one. A video limit that, once reached, would leave less than 15 % free on a disk it
// shares with the server is reported too, before the recordings grow into it.
//
// The disk of the recordings alone (its own disk or RAID, kept full by the video limit on purpose) is watched by the
// video manager (incident video:disk); here only the file systems of the server's own data count.
package diskwatch

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Incidents receives the problems (the incident log, spec 014).
type Incidents interface {
	Raise(ctx context.Context, org, kind, key, severity, title, impact string, detail map[string]any) error
	Resolve(ctx context.Context, key, cause string)
	IsOpen(ctx context.Context, key string) bool
}

const (
	WarnPercent     = 15.0    // below this share free: major incident
	CriticalPercent = 5.0     // below this share free: critical incident
	CriticalBytes   = 1 << 30 // or below 1 GiB free, whatever the size of the disk
	hysteresis      = 2.0     // a problem ends only when the free share is this much above its threshold
)

// Disk is one file system holding some of the watched paths.
type Disk struct {
	ID    string   // device (Linux) or volume (Windows)
	Mount string   // the first watched path on it, shown to people
	Paths []string // the watched paths on it
	Free  uint64
	Total uint64
}

// FreePercent is the free share of the disk.
func (d Disk) FreePercent() float64 {
	if d.Total == 0 {
		return 100
	}
	return float64(d.Free) * 100 / float64(d.Total)
}

// Watcher checks the disks once a minute.
type Watcher struct {
	Paths      []string // the server's data: the system disk, the data directory, the backups
	VideoDir   string
	VideoLimit func() int64                             // the video limit in bytes (nil: no video)
	VideoUsed  func(ctx context.Context) (int64, error) // bytes the recordings take now
	Inc        Incidents
	Log        *slog.Logger

	stat   func(path string) (free, total uint64, id string, ok bool)
	raised map[string]bool
	seen   map[string]bool // keys looked up in the incident log after the start
	low    map[string]int  // consecutive checks a condition held (one bad reading raises nothing)
}

// DefaultPaths are the paths of the server's own data: the system disk, the data directory and the backups.
func DefaultPaths(dataDir, backupDir string) []string {
	var p []string
	if runtime.GOOS == "windows" {
		if sd := os.Getenv("SystemDrive"); sd != "" {
			p = append(p, sd+`\`)
		}
	} else {
		p = append(p, "/")
	}
	for _, d := range []string{dataDir, backupDir} {
		if d != "" {
			p = append(p, d)
		}
	}
	return p
}

// Run checks the disks every minute until ctx ends.
func (w *Watcher) Run(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		w.Check(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// existing walks up from a path that does not exist yet (a backup directory before the first backup).
func existing(p string) string {
	p = filepath.Clean(p)
	for {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		up := filepath.Dir(p)
		if up == p {
			return p
		}
		p = up
	}
}

// statFunc measures the file system of a path (of its nearest existing parent).
func (w *Watcher) statFunc() func(string) (uint64, uint64, string, bool) {
	if w.stat != nil {
		return w.stat
	}
	return func(p string) (uint64, uint64, string, bool) { return diskStat(existing(p)) }
}

// Disks returns the file systems of the watched paths, each once.
func (w *Watcher) Disks() []Disk {
	stat := w.statFunc()
	var list []Disk
	idx := map[string]int{}
	for _, p := range w.Paths {
		free, total, id, ok := stat(p)
		if !ok || total == 0 {
			continue
		}
		if i, ok := idx[id]; ok {
			list[i].Paths = append(list[i].Paths, p)
			continue
		}
		idx[id] = len(list)
		list = append(list, Disk{ID: id, Mount: p, Paths: []string{p}, Free: free, Total: total})
	}
	return list
}

// key of an incident: one per disk and level (the paths do not change while the server runs).
func key(d Disk, level string) string { return "host:disk:" + d.Mount + ":" + level }

// Check looks at every disk once.
func (w *Watcher) Check(ctx context.Context) {
	if w.Inc == nil {
		return
	}
	if w.raised == nil {
		w.raised, w.seen, w.low = map[string]bool{}, map[string]bool{}, map[string]int{}
	}
	stat := w.statFunc()
	for _, d := range w.Disks() {
		pct := d.FreePercent()
		gb := float64(d.Free) / 1e9
		detail := map[string]any{"path": d.Mount, "paths": d.Paths, "free_bytes": d.Free, "total_bytes": d.Total, "free_percent": round1(pct)}
		where := strings.Join(d.Paths, ", ")
		crit := pct < CriticalPercent || d.Free < CriticalBytes
		critEnd := pct >= CriticalPercent+hysteresis && d.Free >= CriticalBytes
		w.condition(ctx, key(d, "critical"), crit, critEnd, func() error {
			return w.Inc.Raise(ctx, "", "storage", key(d, "critical"), "critical", "Server disk almost full",
				fmt.Sprintf("Only %.1f %% (%.1f GB) is free on the disk of %s. When it is full the database stops and the whole server is down. Free space now: old backups, the video limit, logs.", pct, gb, where), detail)
		})
		w.condition(ctx, key(d, "low"), pct < WarnPercent, pct >= WarnPercent+hysteresis, func() error {
			return w.Inc.Raise(ctx, "", "storage", key(d, "low"), "major", "Server disk is running low",
				fmt.Sprintf("%.1f %% (%.1f GB) is free on the disk of %s. Below %.0f %% it is critical; a full disk stops the database and the whole server.", pct, gb, where, CriticalPercent), detail)
		})
		w.checkVideoLimit(ctx, d, stat)
	}
}

// checkVideoLimit: the recordings share the disk with the server and their limit, once reached, would leave less
// than WarnPercent free (with everything else on the disk as it is now).
func (w *Watcher) checkVideoLimit(ctx context.Context, d Disk, stat func(string) (uint64, uint64, string, bool)) {
	if w.VideoDir == "" || w.VideoLimit == nil || w.VideoUsed == nil {
		return
	}
	_, _, id, ok := stat(w.VideoDir)
	if !ok || id != d.ID {
		return
	}
	k := "host:disk:" + d.Mount + ":video-limit"
	limit := w.VideoLimit()
	used, err := w.VideoUsed(ctx)
	if err != nil || limit <= 0 {
		return
	}
	// free space once the recordings reach their limit
	after := int64(d.Free) + used - limit
	afterPct := float64(after) * 100 / float64(d.Total)
	w.condition(ctx, k, afterPct < WarnPercent, afterPct >= WarnPercent+hysteresis, func() error {
		left := float64(after) / 1e9
		if left < 0 {
			left = 0
		}
		return w.Inc.Raise(ctx, "", "storage", k, "major", "Video limit too large for the server disk",
			fmt.Sprintf("The recordings share the disk of %s with the server. Their limit of %.0f GB (now %.1f GB used) would leave only %.1f GB (%.1f %%) free there; a full disk stops the database. Lower the video limit or give the recordings their own disk.",
				d.Mount, float64(limit)/(1<<30), float64(used)/1e9, left, maxf(afterPct, 0)),
			map[string]any{"path": d.Mount, "video_dir": w.VideoDir, "video_limit_bytes": limit, "video_used_bytes": used,
				"free_bytes": d.Free, "total_bytes": d.Total, "free_at_limit_bytes": after})
	})
}

// condition raises an incident once bad held for two checks in a row and resolves it once ended holds; an incident
// closed by someone while the problem goes on is raised again after two more checks.
func (w *Watcher) condition(ctx context.Context, k string, bad, ended bool, raise func() error) {
	if !w.seen[k] {
		w.seen[k] = true
		if w.Inc.IsOpen(ctx, k) {
			w.raised[k] = true // still open from before a restart
		}
	}
	switch {
	case bad:
		if w.raised[k] {
			if !w.Inc.IsOpen(ctx, k) {
				delete(w.raised, k)
				w.low[k] = 0
			}
			return
		}
		w.low[k]++
		if w.low[k] >= 2 {
			if err := raise(); err != nil {
				w.Log.Warn("disk incident not opened; retrying next minute", "key", k, "err", err)
				return
			}
			w.raised[k] = true
			w.Log.Warn("disk incident opened", "key", k)
		}
	case ended:
		w.low[k] = 0
		if w.raised[k] {
			delete(w.raised, k)
			w.Inc.Resolve(ctx, k, "Enough space is free again.")
		}
	default: // between the threshold and its hysteresis: nothing changes
		if !w.raised[k] {
			w.low[k] = 0
		}
	}
}

func round1(f float64) float64 { return float64(int64(f*10+0.5)) / 10 }

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

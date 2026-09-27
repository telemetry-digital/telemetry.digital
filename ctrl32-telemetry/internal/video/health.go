package video

import (
	"context"
	"fmt"
	"time"
)

// Incidents receives health problems (the incident log, spec 014); set Manager.Incidents before Run.
type Incidents interface {
	Raise(ctx context.Context, org, kind, key, severity, title, impact string, detail map[string]any) error
	Resolve(ctx context.Context, key, cause string)
	IsOpen(ctx context.Context, key string) bool
}

// healthState remembers since when each problem lasts; used only by the Run goroutine.
type healthState struct {
	since  map[string]time.Time
	raised map[string]bool
}

// problem tracks one condition: an incident is raised once it lasts `after`, and resolved when it ends.
func (m *Manager) problem(ctx context.Context, key string, bad bool, after time.Duration, raise func() error) {
	h := &m.health
	if !bad {
		delete(h.since, key)
		if h.raised[key] {
			delete(h.raised, key)
			m.Incidents.Resolve(ctx, key, "The condition ended by itself.")
		}
		return
	}
	if h.raised[key] && !m.Incidents.IsOpen(ctx, key) {
		// resolved or closed by someone while the problem goes on: a new incident after the time again
		delete(h.raised, key)
		h.since[key] = time.Now()
		return
	}
	first, ok := h.since[key]
	if !ok {
		h.since[key] = time.Now()
		return
	}
	if after > 0 && !h.raised[key] && time.Since(first) >= after {
		if err := raise(); err != nil {
			m.log.Warn("health incident not opened; retrying next minute", "key", key, "err", err)
			return
		}
		h.raised[key] = true
	}
}

// checkHealth raises incidents for recording cameras that are offline or stopped writing, displays that went dark,
// and video storage that is nearly full. It runs every minute.
func (m *Manager) checkHealth(ctx context.Context) {
	if m.Incidents == nil {
		return
	}
	if m.health.since == nil {
		m.health = healthState{since: map[string]time.Time{}, raised: map[string]bool{}}
		// incidents still open from before a restart end when their condition is gone
		if rows, err := m.pool.Query(ctx, `SELECT dedup_key FROM core.incident WHERE status IN ('open', 'investigating')
			AND (dedup_key LIKE 'camera:%' OR dedup_key LIKE 'display:%' OR dedup_key = 'video:disk')`); err == nil {
			for rows.Next() {
				var k string
				if rows.Scan(&k) == nil {
					m.health.raised[k] = true
					m.health.since[k] = time.Now()
				}
			}
			rows.Close()
		}
	}
	set := m.Settings()
	after := time.Duration(set.CameraOfflineMinutes) * time.Minute
	type camInfo struct {
		id, org, name string
		rec           *recorder
		continuous    bool
	}
	m.mu.Lock()
	var list []camInfo
	for _, c := range m.cams {
		if c.rec != nil {
			list = append(list, camInfo{c.cfg.ID, c.cfg.OrgID, c.cfg.Name, c.rec, !c.rec.events})
		}
	}
	m.mu.Unlock()
	seen := map[string]bool{}
	for _, c := range list {
		st, _ := m.Status(c.id)
		offKey, recKey := "camera:"+c.id+":offline", "camera:"+c.id+":recording"
		seen[offKey], seen[recKey] = true, true
		detail := map[string]any{"camera_id": c.id, "camera": c.name, "state": st.State, "error": st.Error}
		m.problem(ctx, offKey, st.State != "online", after, func() error {
			return m.Incidents.Raise(ctx, c.org, "camera", offKey, "major", "Camera "+c.name+" is offline",
				"Nothing is recorded from this camera until it is back.", detail)
		})
		// a continuously recording camera that is online must write; the last file write shows it does
		last := c.rec.started
		if n := c.rec.written.Load(); n > 0 {
			last = time.Unix(0, n)
		}
		stalled := c.continuous && st.State == "online" && time.Since(last) > 2*time.Minute
		m.problem(ctx, recKey, stalled, after, func() error {
			return m.Incidents.Raise(ctx, c.org, "camera", recKey, "critical", "Camera "+c.name+" is not recording",
				"The camera sends video but no recording is written (storage full or not writable?).", detail)
		})
	}
	// cameras removed or switched off: their incidents end
	for key := range m.health.raised {
		if len(key) > 7 && key[:7] == "camera:" && !seen[key] {
			m.problem(ctx, key, false, 0, nil)
		}
	}
	if free, total, ok := diskFree(m.dir); ok && total > 0 && set.DiskFreePercent > 0 {
		pct := float64(free) * 100 / float64(total)
		m.problem(ctx, "video:disk", pct < float64(set.DiskFreePercent), time.Nanosecond, func() error {
			return m.Incidents.Raise(ctx, "", "storage", "video:disk", "critical", "Video storage almost full",
				fmt.Sprintf("Only %.1f %% (%.1f GB) is free on the disk of the recordings; recording stops when it is full.", pct, float64(free)/1e9),
				map[string]any{"dir": m.dir, "free_bytes": free, "total_bytes": total})
		})
	}
	m.checkDisplays(ctx, set)
}

// checkDisplays raises an incident for a paired display not seen for the configured time.
func (m *Manager) checkDisplays(ctx context.Context, set Settings) {
	rows, err := m.pool.Query(ctx, `SELECT id::text, org_id::text, name, last_seen_at FROM core.display
		WHERE revoked_at IS NULL AND token_hash IS NOT NULL AND offline_alert`)
	if err != nil {
		return
	}
	type disp struct {
		id, org, name string
		seen          *time.Time
	}
	var list []disp
	for rows.Next() {
		var d disp
		if rows.Scan(&d.id, &d.org, &d.name, &d.seen) == nil {
			list = append(list, d)
		}
	}
	rows.Close()
	after := time.Duration(set.DisplayOfflineMinutes) * time.Minute
	seen := map[string]bool{}
	for _, d := range list {
		key := "display:" + d.id
		seen[key] = true
		dark := d.seen == nil || time.Since(*d.seen) > time.Minute
		m.problem(ctx, key, dark, after, func() error {
			return m.Incidents.Raise(ctx, d.org, "display", key, "major", "Display "+d.name+" is not showing",
				"The display has not reported for a while: switched off, disconnected or the browser closed.",
				map[string]any{"display_id": d.id, "display": d.name, "last_seen_at": d.seen})
		})
	}
	for key := range m.health.raised {
		if len(key) > 8 && key[:8] == "display:" && !seen[key] {
			m.problem(ctx, key, false, 0, nil)
		}
	}
}

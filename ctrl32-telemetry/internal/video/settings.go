package video

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Settings are the server-wide video settings an administrator changes in the UI; the configuration file only gives
// the disk limit's default. Every field has safe bounds (Validate).
type Settings struct {
	MaxDiskGB          int `json:"max_disk_gb"`           // recordings of all cameras; the oldest are deleted beyond this
	SegmentSeconds     int `json:"segment_seconds"`       // target length of one recorded file
	EventMergeSeconds  int `json:"event_merge_seconds"`   // an event starting again this soon continues the last one
	LingerSeconds      int `json:"linger_seconds"`        // an unwatched, unrecorded camera stays connected this long
	TimeshiftSeconds   int `json:"timeshift_seconds"`     // a following playback ends after this long without new video
	PushMaxFails       int `json:"push_max_fails"`        // failed relay attempts from one address …
	PushWindowMinutes  int `json:"push_window_minutes"`   // … within this window …
	PushBlockMinutes   int `json:"push_block_minutes"`    // … block the address this long
	PTZAuditMinutes    int `json:"ptz_audit_minutes"`     // PTZ moves are audited once per this per user and camera (0 = every move)
	DiscoverySeconds   int `json:"discovery_seconds"`     // how long discovery waits for answers
	WallSubStreamAbove int `json:"wall_sub_stream_above"` // wall screens with more tiles than this use the sub-stream
	// health (spec 014): incidents after this long; 0 = no incident
	CameraOfflineMinutes  int `json:"camera_offline_minutes"`  // a recording camera offline or not writing
	DisplayOfflineMinutes int `json:"display_offline_minutes"` // a paired display not seen
	DiskFreePercent       int `json:"disk_free_percent"`       // free space of the video storage below this
}

// DefaultSettings are used until an administrator saves settings.
func DefaultSettings(maxDiskGB int) Settings {
	return Settings{MaxDiskGB: maxDiskGB, SegmentSeconds: 60, EventMergeSeconds: 5, LingerSeconds: 30, TimeshiftSeconds: 20,
		PushMaxFails: 10, PushWindowMinutes: 10, PushBlockMinutes: 15, PTZAuditMinutes: 5, DiscoverySeconds: 3, WallSubStreamAbove: 4,
		CameraOfflineMinutes: 5, DisplayOfflineMinutes: 5, DiskFreePercent: 5}
}

// Validate returns the first setting out of its bounds, "" when all are fine.
func (s Settings) Validate() string {
	for _, b := range []struct {
		name      string
		v, lo, hi int
	}{
		{"max_disk_gb", s.MaxDiskGB, 1, 1 << 20},
		{"segment_seconds", s.SegmentSeconds, 10, 600},
		{"event_merge_seconds", s.EventMergeSeconds, 0, 120},
		{"linger_seconds", s.LingerSeconds, 0, 600},
		{"timeshift_seconds", s.TimeshiftSeconds, 5, 300},
		{"push_max_fails", s.PushMaxFails, 3, 100},
		{"push_window_minutes", s.PushWindowMinutes, 1, 1440},
		{"push_block_minutes", s.PushBlockMinutes, 1, 10080},
		{"ptz_audit_minutes", s.PTZAuditMinutes, 0, 60},
		{"discovery_seconds", s.DiscoverySeconds, 1, 15},
		{"wall_sub_stream_above", s.WallSubStreamAbove, 0, 64},
		{"camera_offline_minutes", s.CameraOfflineMinutes, 0, 1440},
		{"display_offline_minutes", s.DisplayOfflineMinutes, 0, 1440},
		{"disk_free_percent", s.DiskFreePercent, 0, 50},
	} {
		if b.v < b.lo || b.v > b.hi {
			return fmt.Sprintf("%s: %d to %d", b.name, b.lo, b.hi)
		}
	}
	return ""
}

// Settings returns the current settings.
func (m *Manager) Settings() Settings {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.set
}

// loadSettings reads the saved settings over the defaults (a missing field keeps its default).
func (m *Manager) loadSettings(ctx context.Context) {
	var raw []byte
	if err := m.pool.QueryRow(ctx, `SELECT data FROM core.video_settings WHERE id`).Scan(&raw); err != nil {
		return
	}
	s := m.Settings()
	if json.Unmarshal(raw, &s) != nil || s.Validate() != "" {
		m.log.Warn("saved video settings ignored; using defaults")
		return
	}
	m.apply(s)
}

// SaveSettings stores and applies new settings; the stream linger applies to cameras as they reconnect.
func (m *Manager) SaveSettings(ctx context.Context, s Settings, userID string) error {
	if msg := s.Validate(); msg != "" {
		return fmt.Errorf("%s", msg)
	}
	raw, _ := json.Marshal(s)
	if _, err := m.pool.Exec(ctx, `INSERT INTO core.video_settings (id, data, updated_at, updated_by) VALUES (true, $1::jsonb, now(), NULLIF($2, '')::uuid)
		ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data, updated_at = now(), updated_by = EXCLUDED.updated_by`, string(raw), userID); err != nil {
		return err
	}
	m.apply(s)
	return nil
}

func (m *Manager) apply(s Settings) {
	m.mu.Lock()
	m.set = s
	m.maxBytes = int64(s.MaxDiskGB) << 30
	for _, c := range m.cams {
		for _, src := range []*Source{c.main, c.sub} {
			if src != nil {
				src.setLinger(m.linger())
			}
		}
	}
	m.mu.Unlock()
}

// linger is the linger of on-demand streams; m.mu held.
func (m *Manager) linger() time.Duration { return time.Duration(m.set.LingerSeconds) * time.Second }

func (m *Manager) segmentLength() time.Duration {
	return time.Duration(m.Settings().SegmentSeconds) * time.Second
}

func (m *Manager) mergeGap() time.Duration {
	return time.Duration(m.Settings().EventMergeSeconds) * time.Second
}

func (m *Manager) followLimit() time.Duration {
	if FollowLimit > 0 {
		return FollowLimit
	}
	return time.Duration(m.Settings().TimeshiftSeconds) * time.Second
}

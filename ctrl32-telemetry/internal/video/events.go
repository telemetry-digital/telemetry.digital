package video

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/video/onvif"
)

// CameraEvent is what listeners (flows, the live stream) learn about an event.
type CameraEvent struct {
	ID       int64     `json:"id"`
	CameraID string    `json:"camera_id"`
	OrgID    string    `json:"-"`
	Camera   string    `json:"camera"`
	Kind     string    `json:"kind"`   // motion, tamper, line, intrusion, input, mark, other
	Source   string    `json:"source"` // camera, flow, user
	Label    string    `json:"label,omitempty"`
	Phase    string    `json:"phase"` // start, end, pulse
	At       time.Time `json:"at"`
}

// camEvents is the event state of one camera.
type camEvents struct {
	mu        sync.Mutex
	open      map[string]int64 // key → event id
	lastClose map[string]closed
	status    string // "", "connected", or the last error
}

type closed struct {
	id int64
	at time.Time
}

// startEvents subscribes to the camera's ONVIF events until the camera is reloaded.
func (m *Manager) startEvents(c *cam) {
	if c.onvif == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.stopEvents = cancel
	go func() {
		backoff := time.Second
		for ctx.Err() == nil {
			began := time.Now()
			err := c.onvif.Subscribe(ctx, func(e onvif.Event) {
				c.ev.mu.Lock()
				c.ev.status = "connected"
				c.ev.mu.Unlock()
				if e.Init && !e.Active {
					return
				}
				m.event(c, e.Kind, "camera", "", e.Active, e.Pulse, e.Time, e.Topic, e.Source)
			})
			if ctx.Err() != nil {
				return
			}
			msg := "events ended"
			if errors.Is(err, ErrRelayNotConnected) {
				msg = ErrRelayNotConnected.Error()
			} else if err != nil {
				msg = err.Error()
			}
			c.ev.mu.Lock()
			same := c.ev.status == msg
			c.ev.status = msg
			c.ev.mu.Unlock()
			if !same {
				m.log.Warn("camera events down", "camera", c.cfg.Name, "err", msg)
			}
			m.closeOpen(c, time.Now()) // the state is unknown while disconnected
			if time.Since(began) > time.Minute {
				backoff = time.Second
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, time.Minute)
			if errors.Is(err, ErrRelayNotConnected) {
				backoff = min(backoff, 10*time.Second) // costs nothing to retry: events resume soon after the relay polls
			}
		}
	}()
}

// event records a state change or a pulse and drives event recording.
func (m *Manager) event(c *cam, kind, source, label string, active, pulse bool, at time.Time, topic string, src map[string]string) int64 {
	key := kind
	if r := src["Rule"]; r != "" {
		key += "|" + r
	}
	if now := time.Now(); at.IsZero() || at.After(now) || at.Before(now.Add(-time.Hour)) {
		at = now // camera clocks drift; an event is never in the future
	}
	data := map[string]any{}
	if topic != "" {
		data["topic"] = topic
	}
	for k, v := range src {
		data[k] = v
	}
	raw, _ := json.Marshal(data)
	triggers := slices.Contains(c.cfg.EventKinds, kind)
	post := time.Duration(c.cfg.EventPost) * time.Second
	ctx := context.Background()
	ev := &c.ev
	ev.mu.Lock()
	defer ev.mu.Unlock()
	notify := func(id int64, phase string) {
		if m.OnEvent != nil {
			go m.OnEvent(CameraEvent{ID: id, CameraID: c.cfg.ID, OrgID: c.cfg.OrgID, Camera: c.cfg.Name, Kind: kind, Source: source, Label: label, Phase: phase, At: at})
		}
	}
	switch {
	case pulse:
		var id int64
		if err := m.pool.QueryRow(ctx, `INSERT INTO core.camera_event (camera_id, org_id, kind, source, label, started_at, ended_at, data) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $6, $7::jsonb) RETURNING id`,
			c.cfg.ID, c.cfg.OrgID, kind, source, label, at, string(raw)).Scan(&id); err != nil {
			m.log.Warn("camera event not stored", "camera", c.cfg.Name, "err", err)
		}
		if triggers && c.rec != nil {
			c.rec.extend(time.Now().Add(post))
		}
		notify(id, "pulse")
		return id
	case active:
		if id, ok := ev.open[key]; ok {
			return id
		}
		var id int64
		if lc, ok := ev.lastClose[key]; ok && at.Sub(lc.at) < m.mergeGap() {
			id = lc.id
			_, _ = m.pool.Exec(ctx, `UPDATE core.camera_event SET ended_at = NULL WHERE id = $1`, id)
		} else if err := m.pool.QueryRow(ctx, `INSERT INTO core.camera_event (camera_id, org_id, kind, source, label, started_at, data) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7::jsonb) RETURNING id`,
			c.cfg.ID, c.cfg.OrgID, kind, source, label, at, string(raw)).Scan(&id); err != nil {
			m.log.Warn("camera event not stored", "camera", c.cfg.Name, "err", err)
		}
		ev.open[key] = id
		if triggers && c.rec != nil {
			c.rec.hold(key, true)
		}
		notify(id, "start")
		return id
	default:
		id, ok := ev.open[key]
		if !ok {
			return 0
		}
		delete(ev.open, key)
		ev.lastClose[key] = closed{id, at}
		_, _ = m.pool.Exec(ctx, `UPDATE core.camera_event SET ended_at = $2 WHERE id = $1`, id, at)
		if triggers && c.rec != nil {
			c.rec.hold(key, false)
			c.rec.extend(time.Now().Add(post))
		}
		notify(id, "end")
		return id
	}
}

// closeOpen ends every open event of a camera (disconnect, reload).
func (m *Manager) closeOpen(c *cam, at time.Time) {
	c.ev.mu.Lock()
	keys := make([]string, 0, len(c.ev.open))
	for k := range c.ev.open {
		keys = append(keys, k)
	}
	c.ev.mu.Unlock()
	for _, k := range keys {
		kind, _, _ := cutKey(k)
		m.event(c, kind, "camera", "", false, false, at, "", map[string]string{"Rule": ruleOf(k)})
	}
}

func cutKey(k string) (string, string, bool) {
	for i := 0; i < len(k); i++ {
		if k[i] == '|' {
			return k[:i], k[i+1:], true
		}
	}
	return k, "", false
}

func ruleOf(k string) string { _, r, _ := cutKey(k); return r }

// ErrNoCamera is returned for unknown or disabled cameras.
var ErrNoCamera = errors.New("camera not found or disabled")

// Mark records an event from a flow or a person ("the door opened"); on a camera recording on events it records
// for seconds (or the camera's post-roll) from now, with the pre-roll before it.
func (m *Manager) Mark(camID, kind, source, label string, seconds int) (int64, error) {
	m.mu.Lock()
	c := m.cams[camID]
	m.mu.Unlock()
	if c == nil {
		return 0, ErrNoCamera
	}
	if kind == "" {
		kind = "mark"
	}
	id := m.event(c, kind, source, label, true, true, time.Now(), "", nil)
	if seconds > 0 && c.rec != nil && slices.Contains(c.cfg.EventKinds, kind) {
		c.rec.extend(time.Now().Add(time.Duration(seconds) * time.Second))
	}
	return id, nil
}

// EventStatus is the state of the camera's event connection ("" without ONVIF).
func (m *Manager) EventStatus(id string) string {
	m.mu.Lock()
	c := m.cams[id]
	m.mu.Unlock()
	if c == nil || c.onvif == nil {
		return ""
	}
	c.ev.mu.Lock()
	defer c.ev.mu.Unlock()
	if c.ev.status == "" {
		return "connecting"
	}
	return c.ev.status
}

// ONVIF returns the camera's ONVIF client and PTZ profile (the configured one, else the first profile with PTZ).
func (m *Manager) ONVIF(ctx context.Context, id string) (*onvif.Client, string, error) {
	m.mu.Lock()
	c := m.cams[id]
	m.mu.Unlock()
	if c == nil {
		return nil, "", ErrNoCamera
	}
	if c.onvif == nil {
		return nil, "", errors.New("the camera has no ONVIF address")
	}
	m.mu.Lock()
	prof := c.cfg.PTZProfile
	m.mu.Unlock()
	if prof != "" {
		return c.onvif, prof, nil
	}
	ps, err := c.onvif.Profiles(ctx)
	if err != nil {
		return nil, "", err
	}
	for _, p := range ps {
		if p.PTZ {
			m.mu.Lock()
			c.cfg.PTZProfile = p.Token
			m.mu.Unlock()
			return c.onvif, p.Token, nil
		}
	}
	return nil, "", errors.New("the camera has no PTZ profile")
}

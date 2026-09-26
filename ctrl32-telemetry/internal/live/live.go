// Package live is the in-process event bus for live data (URS G-07): observations as they are stored, device
// connection changes and alarm changes. Subscribers (HTTP streams) get only the events of their organization
// that match their filter; a slow subscriber drops events instead of blocking the producers and learns the
// drop count from the periodic ping, so the client can refetch.
package live

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/ingest"
)

// Event is one live update. Type is observation, device or alarm; the other fields depend on the type.
type Event struct {
	Type         string     `json:"type"`
	Org          string     `json:"-"`
	DatastreamID string     `json:"datastream_id,omitempty"`
	MeasuredAt   *time.Time `json:"measured_at,omitempty"`
	Value        *float64   `json:"value,omitempty"`
	Quality      string     `json:"quality,omitempty"`
	Backfilled   bool       `json:"backfilled,omitempty"`
	DeviceID     string     `json:"device_id,omitempty"`
	Connected    *bool      `json:"connected,omitempty"`
	AlarmID      string     `json:"alarm_id,omitempty"`
	CommandID    string     `json:"command_id,omitempty"`
	EntityID     string     `json:"entity_id,omitempty"`
	Method       string     `json:"method,omitempty"`
	Status       string     `json:"status,omitempty"`
	Detail       any        `json:"detail,omitempty"`
	State        string     `json:"state,omitempty"`
	Severity     string     `json:"severity,omitempty"`
	At           time.Time  `json:"at"`
}

// Filter says which events of one organization a subscriber wants.
type Filter struct {
	Org              string
	Datastreams      map[string]bool
	Devices          map[string]bool
	AllDevices       bool
	Alarms           bool
	AlarmDatastreams map[string]bool // nil = alarms of any datastream of the organization
	Commands         bool            // command status changes of the filtered devices (never for public streams)
	Entities         bool            // smart-home entity states (never for public streams)
}

// Sub is one subscription; read events from C.
type Sub struct {
	F       Filter
	C       chan Event
	dropped atomic.Int64
}

// Dropped is the number of events discarded because the subscriber was too slow.
func (s *Sub) Dropped() int64 { return s.dropped.Load() }

func (s *Sub) wants(e Event) bool {
	if e.Org != s.F.Org {
		return false
	}
	switch e.Type {
	case "observation":
		return s.F.Datastreams[e.DatastreamID]
	case "device":
		return s.F.AllDevices || s.F.Devices[e.DeviceID]
	case "alarm":
		return s.F.Alarms && (s.F.AlarmDatastreams == nil || s.F.AlarmDatastreams[e.DatastreamID])
	case "command":
		return s.F.Commands && (s.F.AllDevices || s.F.Devices[e.DeviceID])
	case "entity":
		return s.F.Entities
	}
	return false
}

// Hub fans events out to subscribers.
type Hub struct {
	mu   sync.RWMutex
	subs map[*Sub]struct{}
	taps []func(Event)
}

// Tap registers a function that sees every event of every organization (the flow engine's triggers); it must not
// block.
func (h *Hub) Tap(fn func(Event)) {
	h.mu.Lock()
	h.taps = append(h.taps, fn)
	h.mu.Unlock()
}

// NewHub creates an empty hub.
func NewHub() *Hub { return &Hub{subs: map[*Sub]struct{}{}} }

// Subscribe registers a filter; the channel buffers 256 events.
func (h *Hub) Subscribe(f Filter) *Sub {
	s := &Sub{F: f, C: make(chan Event, 256)}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()
	return s
}

// Unsubscribe removes the subscription.
func (h *Hub) Unsubscribe(s *Sub) {
	h.mu.Lock()
	delete(h.subs, s)
	h.mu.Unlock()
}

// Subscribers is the number of open subscriptions (for the health page).
func (h *Hub) Subscribers() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}

// Publish delivers the event to matching subscribers without blocking.
func (h *Hub) Publish(e Event) {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, t := range h.taps {
		t(e)
	}
	for s := range h.subs {
		if !s.wants(e) {
			continue
		}
		select {
		case s.C <- e:
		default:
			s.dropped.Add(1)
		}
	}
}

// OnObservations implements ingest.Sink.
func (h *Hub) OnObservations(_ context.Context, obs []ingest.Observation) {
	if h.Subscribers() == 0 {
		return
	}
	for i := range obs {
		o := obs[i]
		t := o.MeasuredAt
		h.Publish(Event{Type: "observation", Org: o.OrgID, DatastreamID: o.DatastreamID, MeasuredAt: &t, Value: o.Value, Quality: o.Quality, Backfilled: o.Backfilled})
	}
}

// DeviceState reports a device connecting or disconnecting (broker hook).
func (h *Hub) DeviceState(org, deviceID string, connected bool) {
	c := connected
	h.Publish(Event{Type: "device", Org: org, DeviceID: deviceID, Connected: &c})
}

// Command reports a command status change (queued, sent, pending, acked, failed, rejected, expired).
func (h *Hub) Command(org, deviceID, commandID, method, status string, detail any) {
	h.Publish(Event{Type: "command", Org: org, DeviceID: deviceID, CommandID: commandID, Method: method, Status: status, Detail: detail})
}

// Entity reports a smart-home entity's state, attributes and availability (spec 005).
func (h *Hub) Entity(org, entityID, deviceID, state string, attrs map[string]any, available bool) {
	a := available
	h.Publish(Event{Type: "entity", Org: org, EntityID: entityID, DeviceID: deviceID, State: state, Detail: attrs, Connected: &a})
}

// Alarm reports an alarm raised, escalated or cleared (alarm engine hook).
func (h *Hub) Alarm(org, alarmID, datastreamID, state, severity string, value *float64) {
	h.Publish(Event{Type: "alarm", Org: org, AlarmID: alarmID, DatastreamID: datastreamID, State: state, Severity: severity, Value: value})
}

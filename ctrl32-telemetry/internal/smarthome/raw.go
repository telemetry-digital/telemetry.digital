package smarthome

// Raw MQTT access for flows (spec 006): an "MQTT in" node subscribes to a topic filter of an MQTT source (an account's
// namespace or a bridge's broker), an "MQTT out" node publishes there. Messages reach the subscribers from the
// source's worker, in order.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
)

type rawSub struct {
	src, filter string
	fn          func(topic string, payload []byte)
}

type rawSubs struct {
	mu   sync.RWMutex
	next atomic.Int64
	subs map[int64]*rawSub
}

// ValidFilter checks an MQTT topic filter (wildcards "+" as a whole level, "#" only at the end).
func ValidFilter(f string) bool {
	if f == "" || len(f) > 512 || strings.HasPrefix(f, "$") {
		return false
	}
	levels := strings.Split(f, "/")
	for i, l := range levels {
		if strings.Contains(l, "#") && (l != "#" || i != len(levels)-1) {
			return false
		}
		if strings.Contains(l, "+") && l != "+" {
			return false
		}
	}
	return true
}

// ValidTopic checks a topic to publish to (no wildcards).
func ValidTopic(t string) bool {
	return t != "" && len(t) <= 512 && !strings.ContainsAny(t, "+#") && !strings.HasPrefix(t, "$")
}

// Match reports whether a topic matches an MQTT filter.
func Match(filter, topic string) bool {
	fl, tl := strings.Split(filter, "/"), strings.Split(topic, "/")
	for i, f := range fl {
		if f == "#" {
			return true
		}
		if i >= len(tl) || (f != "+" && f != tl[i]) {
			return false
		}
	}
	return len(fl) == len(tl)
}

func (m *Manager) source(org, id string) (*source, error) {
	m.mu.RLock()
	s := m.sources[id]
	m.mu.RUnlock()
	if s == nil || s.org != org {
		return nil, errors.New("MQTT source not found or not active")
	}
	return s, nil
}

// Subscribe delivers the messages of a source matching filter to fn until the returned function is called.
func (m *Manager) Subscribe(org, sourceID, filter string, fn func(topic string, payload []byte)) (func(), error) {
	if !ValidFilter(filter) {
		return nil, errors.New("invalid topic filter")
	}
	// A source that is not running yet (the flows start before the sources, or a bridge is being reloaded) still
	// gets the subscription: bridges subscribe to rawFilters when they connect.
	m.mu.RLock()
	s := m.sources[sourceID]
	m.mu.RUnlock()
	if s != nil && s.org != org {
		return nil, errors.New("MQTT source not found or not active")
	}
	m.raw.mu.Lock()
	if m.raw.subs == nil {
		m.raw.subs = map[int64]*rawSub{}
	}
	id := m.raw.next.Add(1)
	m.raw.subs[id] = &rawSub{src: sourceID, filter: filter, fn: fn}
	m.raw.mu.Unlock()
	if s != nil && s.bridge != nil {
		s.bridge.subscribe([]string{filter})
	}
	if s != nil && s.kind == "mqtt_account" && m.br != nil { // retained messages already on the broker
		go func() {
			for _, pk := range m.br.Server.Topics.Messages(s.ns + "/" + filter) {
				fn(strings.TrimPrefix(pk.TopicName, s.ns+"/"), pk.Payload)
			}
		}()
	}
	return func() {
		m.raw.mu.Lock()
		delete(m.raw.subs, id)
		m.raw.mu.Unlock()
	}, nil
}

// rawFilters lists the filters subscribed on a source (bridges subscribe to them after reconnecting).
func (m *Manager) rawFilters(sourceID string) []string {
	m.raw.mu.RLock()
	defer m.raw.mu.RUnlock()
	var out []string
	for _, r := range m.raw.subs {
		if r.src == sourceID {
			out = append(out, r.filter)
		}
	}
	return out
}

func (m *Manager) deliverRaw(src *source, topic string, payload []byte) {
	m.raw.mu.RLock()
	var fns []func(string, []byte)
	for _, r := range m.raw.subs {
		if r.src == src.id && Match(r.filter, topic) {
			fns = append(fns, r.fn)
		}
	}
	m.raw.mu.RUnlock()
	for _, fn := range fns {
		fn(topic, payload)
	}
}

// PublishRaw publishes a message on a source (an account's namespace or a bridge's broker).
func (m *Manager) PublishRaw(ctx context.Context, org, sourceID, topic string, payload []byte, retain bool) error {
	if !ValidTopic(topic) {
		return errors.New("invalid topic")
	}
	s, err := m.source(org, sourceID)
	if err != nil {
		return err
	}
	switch s.kind {
	case "mqtt_account":
		if m.br == nil {
			return ErrNoSource
		}
		return m.br.Publish(s.ns+"/"+topic, payload, retain)
	case "mqtt_bridge":
		if s.bridge == nil || !s.bridge.connected.Load() {
			return ErrNoSource
		}
		return s.bridge.publishRaw(ctx, topic, payload, retain)
	}
	return ErrNoSource
}

// Sources lists the active sources of an organization (id → name) for pickers.
func (m *Manager) Sources(org string) map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := map[string]string{}
	for id, s := range m.sources {
		if s.org == org {
			out[id] = s.name
		}
	}
	return out
}

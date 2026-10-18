package s7

import "context"

// Test hooks for the external test package (s7_test imports s7sim, which cannot be imported here).

func (m *Manager) SetSink(fn func(device string, values map[string]any)) { m.sink = fn }

// StartWorker runs a poller for settings without a database.
func (m *Manager) StartWorker(ctx context.Context, id string, st Settings) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startLocked(ctx, id, connRow{ID: id, Name: id, Settings: st})
}

// LiveConn reports whether the worker holds an open connection.
func (m *Manager) LiveConn(id string) bool {
	m.mu.Lock()
	w := m.workers[id]
	m.mu.Unlock()
	return w != nil && w.live() != nil
}

// WriteFor builds a write without the device lookup Prepare does.
func WriteFor(id string, st Settings, device, key string, v any) *WriteOp {
	return &WriteOp{ConnectorID: id, ConnectorName: id, Settings: st, Tag: *st.tag(device, key), Value: v}
}

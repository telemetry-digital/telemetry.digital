package modbus

import (
	"context"
	"time"
)

// Current returns the values last read for the device of a running connector, the time of that poll and whether a
// running connector owns the device. Recipes compare a data record with it.
func (m *Manager) Current(ctx context.Context, deviceID string) (map[string]any, time.Time, bool) {
	w := m.workerFor(ctx, deviceID)
	if w == nil {
		return nil, time.Time{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.status[w.row.ID]
	if st == nil {
		return map[string]any{}, time.Time{}, true
	}
	out := make(map[string]any, len(st.LastValues))
	for k, v := range st.LastValues {
		out[k] = v
	}
	return out, st.LastPoll, true
}

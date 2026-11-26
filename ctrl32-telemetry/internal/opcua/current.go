package opcua

import (
	"context"
	"strings"
	"time"
)

// Current returns the values last read for a device of a running connector (keys without the device), the time of
// that poll and whether a running connector owns the device. Recipes compare a data record with it.
func (m *Manager) Current(ctx context.Context, deviceID string) (map[string]any, time.Time, bool) {
	w, ext := m.workerFor(ctx, deviceID)
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
		dev, key, routed := strings.Cut(k, "/")
		if !routed {
			dev, key = "", k
		}
		if dev == ext {
			out[key] = v
		}
	}
	return out, st.LastPoll, true
}

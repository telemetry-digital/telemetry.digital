package broker

// IsConnected reports whether the device currently holds a live MQTT session on this broker.
func (b *Broker) IsConnected(deviceID string) bool {
	if b == nil {
		return false
	}
	found := false
	b.clients.Range(func(_, v any) bool {
		if d, ok := v.(*Device); ok && d.ID == deviceID {
			found = true
			return false
		}
		return true
	})
	return found
}

package broker

import "fmt"

// Publish sends a server-originated message on a device topic (commands, shared attributes, console input).
// Retained messages are delivered to the device on its next subscribe, so configuration survives reconnects.
func (b *Broker) Publish(topic string, payload []byte, retain bool) error {
	if b == nil || b.Server == nil {
		return fmt.Errorf("broker not running")
	}
	return b.Server.Publish(topic, payload, retain, 1)
}

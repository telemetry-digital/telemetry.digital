//go:build windows

package agent

import (
	"log/slog"
	"net/netip"
	"sync"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/wghub"
)

// winHub adapts internal/wghub to the agent (Windows only, ADR-0020).
type winHub struct {
	mu  sync.Mutex
	h   *wghub.Hub
	log *slog.Logger
}

func newUserspaceHub(log *slog.Logger) userspaceHub { return &winHub{log: log} }

func (w *winHub) Start(privateKey string, port int, address netip.Prefix) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.h != nil {
		w.h.Stop()
		w.h = nil
	}
	h, err := wghub.Start(wghub.Config{PrivateKey: privateKey, ListenPort: port, Address: address, Log: w.log})
	if err != nil {
		return err
	}
	w.h = h
	return nil
}

func (w *winHub) Apply(d vpn.Desired) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.h == nil {
		return errHubDown
	}
	return w.h.Apply(d)
}

func (w *winHub) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.h != nil {
		w.h.Stop()
		w.h = nil
	}
}

func (w *winHub) Running() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.h != nil
}

func (w *winHub) Peers() []hubPeer {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.h == nil {
		return nil
	}
	var out []hubPeer
	for _, p := range w.h.Peers() {
		out = append(out, hubPeer{PublicKey: p.PublicKey, Endpoint: p.Endpoint, Handshake: p.LastHandshake, Rx: p.RxBytes, Tx: p.TxBytes})
	}
	return out
}

func (w *winHub) Describe(address netip.Prefix, d vpn.Desired) (string, error) {
	return wghub.DescribeRules(address, d)
}

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

// winUplink adapts the uplink mode of internal/wghub (VPN phase 3, ADR-0021).
type winUplink struct{ h *wghub.Hub }

func newUserspaceUplink() uplinkStartFunc {
	return func(priv string, addr netip.Prefix, hubPub, psk, endpoint string, lan []netip.Prefix, log *slog.Logger) (uplinkDevice, error) {
		h, err := wghub.StartUplink(wghub.UplinkConfig{PrivateKey: priv, Address: addr, HubPublicKey: hubPub, PresharedKey: psk, Endpoint: endpoint, LAN: lan, Log: log})
		if err != nil {
			return nil, err
		}
		return &winUplink{h: h}, nil
	}
}

func (u *winUplink) SetRules(rules []vpn.FwRule) error { return u.h.SetUplinkRules(rules) }
func (u *winUplink) SetEndpoint(hubPub, ep string) error {
	return u.h.SetEndpoint(hubPub, ep)
}
func (u *winUplink) Stop()            { u.h.Stop() }
func (u *winUplink) Describe() string { return u.h.Describe() }
func (u *winUplink) Peers() []hubPeer {
	var out []hubPeer
	for _, p := range u.h.Peers() {
		out = append(out, hubPeer{PublicKey: p.PublicKey, Endpoint: p.Endpoint, Handshake: p.LastHandshake, Rx: p.RxBytes, Tx: p.TxBytes})
	}
	return out
}

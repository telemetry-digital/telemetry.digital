package agent

import (
	"context"
	"errors"
	"net/netip"
	"strconv"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

// Userspace WireGuard hub (ADR-0020): on Windows there is no kernel WireGuard and no nftables, so the agent runs
// WireGuard in its own process (internal/wghub) with an in-process filter that enforces the same compiled access
// rules. Only the Windows build links the hub (hub_windows.go); elsewhere newUserspaceHub returns nil. Tests use a
// fake implementation on any platform.

// VPN modes reported in WireGuardState.Mode.
const (
	ModeKernel     = "kernel"     // Linux: wg-quick, kernel WireGuard, nftables table ctrl32_vpn
	ModeUserspace  = "userspace"  // Windows: WireGuard and the filter inside the agent
	ModeSimulation = "simulation" // development and tests ([agent] vpn_simulate)
)

// hubPeer are the live values of one peer of the userspace hub.
type hubPeer struct {
	PublicKey string
	Endpoint  string
	Handshake time.Time
	Rx, Tx    int64
}

// userspaceHub is what the agent needs from the hub.
type userspaceHub interface {
	Start(privateKey string, port int, address netip.Prefix) error
	Apply(d vpn.Desired) error
	Stop()
	Running() bool
	Peers() []hubPeer
	// Describe renders the rules text of a state (also when the hub is not running).
	Describe(address netip.Prefix, d vpn.Desired) (string, error)
}

const winFirewallRule = "ctrl32 WireGuard"

var errHubDown = errors.New("the VPN hub is not running")

// vpnMode tells how this agent manages the VPN ("" = not at all).
func (a *Server) vpnMode() string {
	switch {
	case a.simulate:
		return ModeSimulation
	case a.vpnHost:
		return ModeKernel
	case a.hub != nil:
		return ModeUserspace
	}
	return ""
}

// desiredFrom builds the hub's state from the stored peers (with their preshared keys) and the firewall state.
func desiredFrom(peers []wgPeerStored, vs vpnStored) vpn.Desired {
	return vpn.Desired{Peers: toDesiredPeers(peers), Rules: vs.Rules, Forward: vs.Forward}
}

// hubApply starts the hub when needed and applies the state.
func (a *Server) hubApply(st wgStored, peers []wgPeerStored, vs vpnStored) error {
	srv, err := vpn.ServerPrefix(st.Address)
	if err != nil {
		return err
	}
	if !a.hub.Running() {
		if err := a.hub.Start(a.serverKey(), st.ListenPort, srv); err != nil {
			return err
		}
	}
	return a.hub.Apply(desiredFrom(peers, vs))
}

// writeHubRules writes the rules text the hub enforces (shown in the UI instead of the nftables table).
func (a *Server) writeHubRules(st wgStored, peers []wgPeerStored, vs vpnStored) error {
	srv, err := vpn.ServerPrefix(st.Address)
	if err != nil {
		return err
	}
	txt, err := a.hub.Describe(srv, desiredFrom(peers, vs))
	if err != nil {
		return err
	}
	return writeFileAtomic(wgRulesFile, []byte(txt), 0o600)
}

// winFirewall opens (or closes) the inbound UDP port of WireGuard in Windows Firewall for the agent's binary only
// (best effort: without the rule remote peers cannot connect, which the UI shows as offline peers).
func (a *Server) winFirewall(ctx context.Context, port int, open bool) {
	_, _ = a.run(ctx, 15*time.Second, "netsh", "advfirewall", "firewall", "delete", "rule", "name="+winFirewallRule)
	if !open {
		return
	}
	if out, err := a.run(ctx, 15*time.Second, "netsh", "advfirewall", "firewall", "add", "rule", "name="+winFirewallRule, "dir=in", "action=allow",
		"protocol=UDP", "localport="+strconv.Itoa(port), "program="+binPath); err != nil {
		a.log.Warn("windows firewall rule for WireGuard not added", "err", err, "out", firstLine(out))
	}
}

// startHubFromFiles brings the hub up after the agent starts when the VPN is enabled.
func (a *Server) startHubFromFiles() {
	if a.vpnMode() != ModeUserspace {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	st, peers, vs := a.wgLoad()
	if !st.Enabled || a.serverKey() == "" {
		return
	}
	if err := a.hubApply(st, peers, vs); err != nil {
		a.log.Error("vpn: the userspace hub did not start", "err", err)
		vs.Error = err.Error()
		_ = saveJSON(wgVPNFile, vs, 0o600)
		return
	}
	_ = a.writeHubRules(st, peers, vs)
	a.log.Info("vpn: userspace hub started", "port", st.ListenPort, "address", st.Address, "peers", len(peers), "rules", len(vs.Rules))
}

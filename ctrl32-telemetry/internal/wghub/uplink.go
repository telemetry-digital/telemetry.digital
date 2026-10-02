package wghub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

// Uplink mode (VPN phase 3, ADR-0021): on a Windows customer server the uplink to a hub runs in the agent like the
// hub itself — wireguard-go on a virtual interface that is not an interface of the operating system, one peer (the
// hub) with an endpoint and a keepalive of 25 s, no listening port of its own. Everything that arrives from the hub
// passes the same kind of filter (default deny):
//   - to this server's uplink address: input rules; accepted TCP connections are relayed to 127.0.0.1:<port>;
//   - to a host of the LAN subnets the server offers: forward rules, TCP only; the connection ends in the private stack
//     and the agent opens its own TCP connection from the host to the LAN host (so the LAN host sees the server as the
//     client, like the masquerade on Linux). UDP and ping to LAN hosts are not offered on Windows; addresses of the
//     host itself are never relayed (this server is reached through its uplink address only).

// UplinkConfig starts an uplink.
type UplinkConfig struct {
	PrivateKey   string
	Address      netip.Prefix // this server's address with the hub's VPN prefix, e.g. 10.99.0.7/16
	HubPublicKey string
	PresharedKey string
	Endpoint     string // ip:port of the hub (or of the local HTTPS transport)
	LAN          []netip.Prefix
	Dial         func(ctx context.Context, port uint16) (net.Conn, error)
	DialLAN      func(ctx context.Context, dst netip.AddrPort) (net.Conn, error)
	Log          *slog.Logger
}

func uplinkPeerIPC(c UplinkConfig) (string, error) {
	pub, err := b64ToHex(c.HubPublicKey)
	if err != nil {
		return "", fmt.Errorf("hub key: %w", err)
	}
	if _, err := netip.ParseAddrPort(c.Endpoint); err != nil {
		return "", fmt.Errorf("hub endpoint %q must be ip:port", c.Endpoint)
	}
	var b strings.Builder
	b.WriteString("listen_port=0\npublic_key=" + pub + "\n")
	if c.PresharedKey != "" {
		psk, err := b64ToHex(c.PresharedKey)
		if err != nil {
			return "", fmt.Errorf("preshared key: %w", err)
		}
		b.WriteString("preshared_key=" + psk + "\n")
	}
	b.WriteString("endpoint=" + c.Endpoint + "\npersistent_keepalive_interval=25\nreplace_allowed_ips=true\nallowed_ip=" + c.Address.Masked().String() + "\n")
	return b.String(), nil
}

// StartUplink connects to the hub. Nothing from the hub is accepted except ping of the server until SetUplinkRules.
func StartUplink(c UplinkConfig) (*Hub, error) {
	for _, l := range c.LAN {
		if l.Overlaps(c.Address.Masked()) {
			return nil, fmt.Errorf("LAN subnet %s overlaps the hub's VPN range", l)
		}
	}
	ipc, err := uplinkPeerIPC(c)
	if err != nil {
		return nil, err
	}
	if c.DialLAN == nil {
		c.DialLAN = dialLANFromHost
	}
	h, err := start(Config{PrivateKey: c.PrivateKey, Address: c.Address, Dial: c.Dial, DialLAN: c.DialLAN, Log: c.Log}, true, ipc)
	if err != nil {
		return nil, err
	}
	h.lan = append([]netip.Prefix(nil), c.LAN...)
	if err := h.SetUplinkRules(nil); err != nil {
		h.Stop()
		return nil, err
	}
	return h, nil
}

// dialLANFromHost connects to a LAN host, never to an address of this host (that would bypass the input rules).
func dialLANFromHost(ctx context.Context, dst netip.AddrPort) (net.Conn, error) {
	if isHostAddr(dst.Addr()) {
		return nil, errors.New("an address of this server is reached through its uplink address only")
	}
	var d net.Dialer
	return d.DialContext(ctx, "tcp", dst.String())
}

func isHostAddr(a netip.Addr) bool {
	if a.IsLoopback() || a.IsUnspecified() {
		return true
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return true // unknown: refuse
	}
	for _, x := range addrs {
		if n, ok := x.(*net.IPNet); ok {
			if b, ok := netip.AddrFromSlice(n.IP); ok && b.Unmap() == a {
				return true
			}
		}
	}
	return false
}

// SetUplinkRules replaces the uplink's rules atomically (input to this server, forward to LAN hosts).
func (h *Hub) SetUplinkRules(rules []vpn.FwRule) error {
	return h.filter.SetUplinkRules(h.cfg.Address, h.lan, rules)
}

// SetEndpoint points the uplink at another hub endpoint (switching between UDP and the HTTPS transport).
func (h *Hub) SetEndpoint(hubPublicKey, endpoint string) error {
	pub, err := b64ToHex(hubPublicKey)
	if err != nil {
		return err
	}
	if _, err := netip.ParseAddrPort(endpoint); err != nil {
		return fmt.Errorf("endpoint %q must be ip:port", endpoint)
	}
	return h.dev.IpcSet("public_key=" + pub + "\nupdate_only=true\nendpoint=" + endpoint + "\n")
}

// compileUplink builds the ruleset of an uplink.
func compileUplink(server netip.Prefix, lan []netip.Prefix, rules []vpn.FwRule) (*ruleset, error) {
	rs, err := compile(server, vpn.Desired{Rules: rules})
	if err != nil {
		return nil, err
	}
	rs.uplink, rs.lan = true, append([]netip.Prefix(nil), lan...)
	rs.text = describeUplink(rs, rules)
	return rs, nil
}

// SetUplinkRules switches the filter to uplink mode with the given rules.
func (f *Filter) SetUplinkRules(server netip.Prefix, lan []netip.Prefix, rules []vpn.FwRule) error {
	rs, err := compileUplink(server, lan, rules)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rs.Store(rs)
	clear(f.flows)
	return nil
}

// lanTarget reports whether the stack may relay a connection to dst (uplink mode, inside the offered LAN).
func (f *Filter) lanTarget(dst netip.Addr) bool {
	rs := f.rs.Load()
	return rs.uplink && contains(rs.lan, dst)
}

// DescribeUplinkRules renders the rules text of an uplink without a running device.
func DescribeUplinkRules(server netip.Prefix, lan []netip.Prefix, rules []vpn.FwRule) (string, error) {
	rs, err := compileUplink(server, lan, rules)
	if err != nil {
		return "", err
	}
	return rs.text, nil
}

func describeUplink(rs *ruleset, rules []vpn.FwRule) string {
	var b strings.Builder
	b.WriteString("# ctrl32 telemetry — uplink access rules enforced inside the agent (userspace WireGuard, Windows).\n")
	b.WriteString("# Default deny for everything that arrives from the hub. Ping of this server " + rs.server.String() + " is allowed.\n")
	b.WriteString("# TCP to this server is relayed to 127.0.0.1:<port>; TCP to LAN hosts is relayed from this server (LAN hosts\n")
	b.WriteString("# see the server as the client). UDP and ping to LAN hosts are not offered on Windows.\n")
	lan := make([]string, len(rs.lan))
	for i, l := range rs.lan {
		lan[i] = l.String()
	}
	b.WriteString("# Networks offered to the hub: " + strings.Join(lan, ", ") + "\n")
	for _, chain := range []string{vpn.ChainInput, vpn.ChainForward} {
		name := "to this server"
		if chain == vpn.ChainForward {
			name = "to the LAN"
		}
		b.WriteString("\nchain " + chain + " (" + name + ") {\n")
		for _, r := range rules {
			if r.Chain != chain {
				continue
			}
			line := "\taccept src " + strings.Join(r.Src, ", ") + " dst " + strings.Join(r.Dst, ", ") + " proto " + r.Proto
			if len(r.Ports) > 0 {
				line += " ports " + strings.Join(r.Ports, ", ")
			}
			if chain == vpn.ChainForward && r.Proto != "tcp" && r.Proto != "any" {
				line += "   # not available on Windows (TCP only)"
			}
			b.WriteString(line + "   # rule " + r.ID + "\n")
		}
		b.WriteString("\tdrop   # default deny\n}\n")
	}
	return b.String()
}

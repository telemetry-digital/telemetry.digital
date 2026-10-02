package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/uptunnel"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

// Uplink to a hub (VPN phase 3, ADR-0021, spec 027): this server, inside a LAN without a public address, connects out
// to a hub — another telemetry server with a public address — and technicians of the hub reach it and its LAN as the
// rules on both sides allow. The web service sends the pasted uplink token once (PUT /uplink) and the compiled uplink
// access rules (PUT /uplink/rules); the agent keeps the token (private key and preshared key) root-only in
// uplink.json, validates everything again and applies it:
//
//   - Linux: a second WireGuard interface wg1 (wg-quick@wg1: no listening port, one peer — the hub — with the hub's
//     VPN range as AllowedIPs and a keepalive of 25 s) and the nftables table inet ctrl32_uplink (default deny for
//     everything from wg1, input rules to this server's uplink address, forward rules to LAN hosts with masquerade).
//     Forwarding is switched on for wg1 and, only while a LAN rule exists, on the LAN interfaces that lead to the
//     offered subnets (for the replies), with a guard in the table so the host does not become a router for its LAN.
//   - Windows: wireguard-go in the agent (internal/wghub uplink mode): the same rules in-process, TCP to LAN hosts
//     relayed from the host.
//   - HTTPS transport (both): when UDP is blocked the hub's endpoint is a local UDP socket of the agent that relays the
//     datagrams over a WebSocket through the hub's HTTPS port (internal/uptunnel).
//
// A change is applied as a whole and rolled back when a step fails; a failed connect leaves no uplink behind.

type uplinkStored struct {
	Token       vpn.UplinkToken `json:"token"`
	Transport   string          `json:"transport"`
	TunnelPort  int             `json:"tunnel_port,omitempty"`
	Rules       []vpn.FwRule    `json:"rules"`
	Forward     bool            `json:"forward"`
	LANForward  []string        `json:"lan_forward,omitempty"` // LAN interfaces on which the agent switched forwarding on
	ConnectedAt time.Time       `json:"connected_at"`
	AppliedAt   time.Time       `json:"applied_at"`
	Error       string          `json:"error,omitempty"`
}

// uplinkDevice is the userspace uplink (Windows, internal/wghub); tests use a fake.
type uplinkDevice interface {
	SetRules(rules []vpn.FwRule) error
	SetEndpoint(hubPublicKey, endpoint string) error
	Stop()
	Peers() []hubPeer
	Describe() string
}

type uplinkStartFunc func(priv string, addr netip.Prefix, hubPub, psk, endpoint string, lan []netip.Prefix, log *slog.Logger) (uplinkDevice, error)

var (
	uplinkFile, uplinkNftFile, uplinkRulesFile string
	uplinkUnit                                 = "wg-quick@" + vpn.UplinkIface
)

func uplinkConfFile() string { return filepath.Join(filepath.Dir(wgConfFile), vpn.UplinkIface+".conf") }

func deriveUplinkPaths() {
	uplinkFile = filepath.Join(wgDir, "uplink.json")
	uplinkNftFile = filepath.Join(wgDir, "uplink.nft")
	uplinkRulesFile = filepath.Join(wgDir, "uplink-rules.txt") // userspace (Windows): the rules it enforces, for review
}

// uplinkMode tells how this agent runs the uplink ("" = not at all).
func (a *Server) uplinkMode() string {
	switch {
	case a.simulate:
		return ModeSimulation
	case a.vpnHost:
		return ModeKernel
	case a.startUplink != nil:
		return ModeUserspace
	}
	return ""
}

func (a *Server) uplinkLoad() (uplinkStored, bool) {
	var u uplinkStored
	if err := loadJSON(uplinkFile, &u); err != nil || u.Token.PrivateKey == "" {
		return uplinkStored{}, false
	}
	if u.Rules == nil {
		u.Rules = []vpn.FwRule{}
	}
	return u, true
}

func uplinkInfo(t vpn.UplinkToken) vpn.UplinkInfo {
	info := vpn.UplinkInfo{VPNNet: netip.MustParsePrefix(t.VPNNet)}
	info.Address, _ = netip.ParseAddr(t.Address)
	for _, s := range t.Subnets {
		if p, err := netip.ParsePrefix(s); err == nil {
			info.Subnets = append(info.Subnets, p)
		}
	}
	return info
}

// ---- state ----------------------------------------------------------------------------------------------------------

var tunnelHostRe = regexp.MustCompile(`^https?://([^/]+)`)

func (a *Server) uplinkState(ctx context.Context) *UplinkState {
	mode := a.uplinkMode()
	if mode == "" {
		return nil
	}
	u, ok := a.uplinkLoad()
	out := &UplinkState{Mode: mode, Configured: ok}
	if !ok {
		return out
	}
	t := u.Token
	out.Name, out.HubEndpoint, out.HubPublicKey, out.Address, out.VPNNet, out.HubAddress = t.Name, t.Endpoint, t.HubPublicKey, t.Address, t.VPNNet, t.HubAddress
	out.Subnets = append([]string{}, t.Subnets...)
	out.PublicKey, _ = vpn.PublicKey(t.PrivateKey)
	out.Transport, out.HTTPSAvailable, out.ConnectedAt = u.Transport, t.TunnelURL != "", u.ConnectedAt
	if m := tunnelHostRe.FindStringSubmatch(t.TunnelURL); m != nil {
		out.TunnelHost = m[1]
	}
	out.LANForward = u.LANForward
	out.Firewall = VPNFirewall{Rules: len(u.Rules), Forward: u.Forward, AppliedAt: u.AppliedAt, Error: u.Error}
	if u.Transport == vpn.TransportHTTPS && a.tunnel != nil {
		s := a.tunnel.Status()
		out.Tunnel = &s
	}
	switch mode {
	case ModeSimulation:
		out.Running = true
		out.Firewall.Ruleset = readFile(uplinkNftFile)
	case ModeKernel:
		out.Firewall.Ruleset = readFile(uplinkNftFile)
		if dump, err := a.run(ctx, 5*time.Second, "wg", "show", vpn.UplinkIface, "dump"); err == nil {
			out.Running = true
			for i, l := range strings.Split(dump, "\n") {
				f := strings.Split(l, "\t")
				if i == 0 || len(f) < 8 {
					continue
				}
				if f[2] != "(none)" {
					out.Endpoint = f[2]
				}
				if hs, _ := strconv.ParseInt(f[4], 10, 64); hs > 0 {
					out.LatestHandshake = time.Unix(hs, 0)
				}
				out.RxBytes, _ = strconv.ParseInt(f[5], 10, 64)
				out.TxBytes, _ = strconv.ParseInt(f[6], 10, 64)
			}
		}
	case ModeUserspace:
		out.Firewall.Ruleset = readFile(uplinkRulesFile)
		if a.upDev != nil {
			out.Running = true
			for _, p := range a.upDev.Peers() {
				out.Endpoint, out.RxBytes, out.TxBytes, out.LatestHandshake = p.Endpoint, p.Rx, p.Tx, p.Handshake
			}
		}
	}
	// WireGuard renews the session every two minutes while traffic (the keepalive) flows
	out.Online = !out.LatestHandshake.IsZero() && time.Since(out.LatestHandshake) < 3*time.Minute
	out.Firewall.Applied = out.Firewall.Ruleset != "" && out.Running && u.Error == ""
	return out
}

// ---- checks ---------------------------------------------------------------------------------------------------------

// uplinkCheck validates a token against this host: the hub's range must not collide with the host's networks, this
// server's own VPN range or the routed subnets of its own sites.
func (a *Server) uplinkCheck(t vpn.UplinkToken) error {
	st, peers, _ := a.wgLoad()
	var own netip.Prefix
	if st.Enabled {
		own, _ = vpn.ServerPrefix(st.Address)
	}
	var subs []netip.Prefix
	for _, p := range peers {
		if p.Disabled {
			continue
		}
		for _, s := range p.Subnets {
			if pr, err := netip.ParsePrefix(s); err == nil {
				subs = append(subs, pr)
			}
		}
	}
	return vpn.CheckUplinkLocal(t, a.localNets(), own, subs)
}

// uplinkConflict is the check the other way round: the own VPN (wg0) must not take the uplink's range (hSetWireGuard,
// hApplyVPN).
func (a *Server) uplinkConflict(own netip.Prefix, subnets []string) error {
	u, ok := a.uplinkLoad()
	if !ok {
		return nil
	}
	hub := netip.MustParsePrefix(u.Token.VPNNet)
	if own.IsValid() && own.Masked().Overlaps(hub) {
		return fmt.Errorf("the VPN range %s overlaps the range %s of the hub this server is connected to (uplink); choose another range", own.Masked(), hub)
	}
	for _, s := range subnets {
		if p, err := netip.ParsePrefix(s); err == nil && p.Overlaps(hub) {
			return fmt.Errorf("routed subnet %s overlaps the range %s of the hub this server is connected to (uplink)", p, hub)
		}
	}
	return nil
}

// ---- forwarding into the LAN (Linux) --------------------------------------------------------------------------------

var routeDevRe = regexp.MustCompile(`\bdev (\S+)`)

// lanIfaces finds the interfaces that lead to the offered subnets (`ip route get`), without the VPN interfaces.
func (a *Server) lanIfaces(ctx context.Context, subnets []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range subnets {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			continue
		}
		probe := p.Addr()
		if p.Bits() < 32 {
			probe = probe.Next()
		}
		res, err := a.run(ctx, 5*time.Second, "ip", "-4", "-o", "route", "get", probe.String())
		if err != nil {
			continue
		}
		m := routeDevRe.FindStringSubmatch(res)
		if m == nil || !vpn.ValidIface(m[1]) || m[1] == vpn.Iface || m[1] == vpn.UplinkIface || m[1] == "lo" || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

func sysctlPath(iface string) string { return "/proc/sys/net/ipv4/conf/" + iface + "/forwarding" }

func readForwarding(iface string) (bool, error) {
	b, err := os.ReadFile(sysctlPath(iface))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(b)) == "1", nil
}

func writeForwarding(iface string, on bool) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	v := "0\n"
	if on {
		v = "1\n"
	}
	return os.WriteFile(sysctlPath(iface), []byte(v), 0o644)
}

// planLANForward decides on which LAN interfaces the agent switches forwarding on for the replies to the hub: those
// where it is off. Interfaces it switched on before stay in the list. When the host already forwards on all
// interfaces (a router, or Docker) nothing is switched and nothing is guarded.
func (a *Server) planLANForward(ctx context.Context, u uplinkStored, forward bool) []string {
	if !forward {
		return nil
	}
	if all, err := a.ifForwarding("all"); err == nil && all {
		return nil
	}
	had := map[string]bool{}
	for _, l := range u.LANForward {
		had[l] = true
	}
	var out []string
	for _, l := range a.lanIfaces(ctx, u.Token.Subnets) {
		if on, err := a.ifForwarding(l); err == nil && (!on || had[l]) {
			out = append(out, l)
		}
	}
	return out
}

// setUplinkForwarding switches forwarding on wg1 and on the planned LAN interfaces, and back off on interfaces the
// agent switched on before but no longer needs.
func (a *Server) setUplinkForwarding(old, now []string, forward bool) error {
	if err := a.setIfForwarding(vpn.UplinkIface, forward); err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, l := range now {
		keep[l] = true
		if err := a.setIfForwarding(l, true); err != nil {
			return err
		}
	}
	for _, l := range old {
		if !keep[l] {
			_ = a.setIfForwarding(l, false)
		}
	}
	return nil
}

// ---- transport -------------------------------------------------------------------------------------------------------

// uplinkEndpoint returns the endpoint WireGuard uses: the hub's UDP endpoint, or the local end of the HTTPS transport
// (started here when needed; a running tunnel client is kept when its URL and key are the same).
func (a *Server) uplinkEndpoint(u *uplinkStored) (string, error) {
	if u.Transport != vpn.TransportHTTPS {
		a.stopTunnel()
		return u.Token.Endpoint, nil
	}
	if u.Token.TunnelURL == "" {
		return "", errors.New("this uplink configuration has no HTTPS transport (the hub has no HTTPS address); use UDP")
	}
	if a.tunnel != nil && a.tunnelURL == u.Token.TunnelURL && a.tunnelKey == u.Token.TunnelKey {
		u.TunnelPort = a.tunnel.Port()
		return a.tunnel.Endpoint(), nil
	}
	a.stopTunnel()
	c, err := uptunnel.NewClient(u.Token.TunnelURL, u.Token.TunnelKey, u.TunnelPort, a.log)
	if err != nil {
		return "", fmt.Errorf("the HTTPS transport could not listen on the loopback address: %w", err)
	}
	a.tunnel, a.tunnelURL, a.tunnelKey = c, u.Token.TunnelURL, u.Token.TunnelKey
	u.TunnelPort = c.Port()
	return c.Endpoint(), nil
}

func (a *Server) stopTunnel() {
	if a.tunnel != nil {
		a.tunnel.Close()
		a.tunnel, a.tunnelURL, a.tunnelKey = nil, "", ""
	}
}

// resolveEndpoint turns host:port into ip:port for the userspace device (wg-quick resolves on Linux itself).
func resolveEndpoint(ep string) (string, error) {
	host, port, err := net.SplitHostPort(ep)
	if err != nil {
		return "", err
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return netip.AddrPortFrom(ip, mustPort(port)).String(), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip4", host)
	if err != nil || len(ips) == 0 {
		return "", fmt.Errorf("the hub %s could not be resolved", host)
	}
	return netip.AddrPortFrom(ips[0].Unmap(), mustPort(port)).String(), nil
}

func mustPort(s string) uint16 {
	n, _ := strconv.Atoi(s)
	return uint16(n)
}

// ---- applying -------------------------------------------------------------------------------------------------------

// uplinkApply makes the host match u (connect, replace, new rules or transport). prev is the state before (ok=false:
// no uplink yet); on failure the host is put back to it.
func (a *Server) uplinkApply(ctx context.Context, u *uplinkStored, prev uplinkStored, hadPrev bool, restart bool) error {
	info := uplinkInfo(u.Token)
	if err := vpn.ValidateUplinkRules(u.Rules, info); err != nil {
		return err
	}
	u.Forward = false
	for _, r := range u.Rules {
		if r.Chain == vpn.ChainForward {
			u.Forward = true
		}
	}
	switch a.uplinkMode() {
	case ModeUserspace:
		return a.uplinkApplyUserspace(u, restart)
	case ModeSimulation:
		ep, err := a.uplinkEndpoint(u)
		if err != nil {
			return err
		}
		u.LANForward = nil
		return a.writeUplinkFiles(*u, ep)
	}
	return a.uplinkApplyKernel(ctx, u, prev, hadPrev, restart)
}

func (a *Server) writeUplinkFiles(u uplinkStored, endpoint string) error {
	if err := os.MkdirAll(wgDir, 0o700); err != nil {
		return err
	}
	if err := writeFileAtomic(uplinkNftFile, []byte(vpn.RenderUplinkNft(uplinkInfo(u.Token).Address, u.Rules, u.LANForward)), 0o600); err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(uplinkConfFile()), 0o700)
	return writeFileAtomic(uplinkConfFile(), []byte(vpn.UplinkConf(u.Token, endpoint, uplinkNftFile, u.Forward, u.LANForward)), 0o600)
}

func (a *Server) uplinkApplyKernel(ctx context.Context, u *uplinkStored, prev uplinkStored, hadPrev bool, restart bool) error {
	if conf := readFile(uplinkConfFile()); conf != "" && !strings.HasPrefix(conf, vpn.UplinkConfHeader) {
		return errors.New(uplinkConfFile() + " exists and was not written by this server; remove it or rename that interface first")
	}
	if restart {
		missing := []string{}
		if !a.wgInstalled(ctx) {
			missing = append(missing, "wireguard-tools")
		}
		if _, err := a.run(ctx, 5*time.Second, "nft", "--version"); err != nil {
			missing = append(missing, "nftables")
		}
		if len(missing) > 0 {
			if out, err := a.run(ctx, 4*time.Minute, "apt-get", append([]string{"install", "-y", "-qq"}, missing...)...); err != nil {
				return fmt.Errorf("%s could not be installed: %s", strings.Join(missing, " and "), firstLine(out))
			}
		}
	}
	u.LANForward = a.planLANForward(ctx, *u, u.Forward)
	ep, err := a.uplinkEndpoint(u)
	if err != nil {
		return err
	}
	oldNft, oldConf := readFile(uplinkNftFile), readFile(uplinkConfFile())
	newNft := vpn.RenderUplinkNft(uplinkInfo(u.Token).Address, u.Rules, u.LANForward)
	check := uplinkNftFile + ".check"
	_ = os.MkdirAll(wgDir, 0o700)
	if err := writeFileAtomic(check, []byte(newNft), 0o600); err != nil {
		return err
	}
	defer os.Remove(check)
	if out, err := a.run(ctx, 20*time.Second, "nft", "-c", "-f", check); err != nil {
		return fmt.Errorf("nftables rejected the uplink rules: %s", firstLine(out))
	}
	restore := func(cause error) error {
		if !hadPrev {
			_, _ = a.run(ctx, time.Minute, "systemctl", "disable", "--now", uplinkUnit)
			_, _ = a.run(ctx, 10*time.Second, "nft", "delete", "table", "inet", vpn.UplinkNftTable)
			_ = os.Remove(uplinkConfFile()) // ours: a foreign wg1.conf was refused before anything was written
			_ = os.Remove(uplinkNftFile)
			_ = a.setUplinkForwarding(u.LANForward, nil, false)
			a.stopTunnel()
		} else {
			_ = writeFileAtomic(uplinkConfFile(), []byte(oldConf), 0o600)
			_ = writeFileAtomic(uplinkNftFile, []byte(oldNft), 0o600)
			_, _ = a.run(ctx, 20*time.Second, "nft", "-f", uplinkNftFile)
			if restart {
				_, _ = a.run(ctx, time.Minute, "systemctl", "restart", uplinkUnit)
			}
			_ = a.setUplinkForwarding(u.LANForward, prev.LANForward, prev.Forward)
		}
		a.log.Error("vpn uplink apply failed, previous state restored", "err", cause)
		return cause
	}
	if err := a.writeUplinkFiles(*u, ep); err != nil {
		return restore(err)
	}
	_, upErr := a.run(ctx, 5*time.Second, "wg", "show", vpn.UplinkIface)
	if restart || upErr != nil {
		// a new configuration (address or keys) or the interface is down: (re)start it; PreUp loads the table before
		// the interface exists
		if out, err := a.run(ctx, time.Minute, "systemctl", "enable", uplinkUnit); err != nil {
			return restore(fmt.Errorf("the uplink could not be enabled: %s", firstLine(out)))
		}
		if out, err := a.run(ctx, time.Minute, "systemctl", "restart", uplinkUnit); err != nil {
			return restore(fmt.Errorf("the uplink could not be started: %s", firstLine(out)))
		}
	} else {
		if out, err := a.run(ctx, 20*time.Second, "nft", "-f", uplinkNftFile); err != nil {
			return restore(fmt.Errorf("loading the uplink firewall failed: %s", firstLine(out)))
		}
		if out, err := a.run(ctx, 10*time.Second, "wg", "set", vpn.UplinkIface, "peer", u.Token.HubPublicKey, "endpoint", ep); err != nil {
			return restore(fmt.Errorf("setting the hub's endpoint failed: %s", firstLine(out)))
		}
	}
	if err := a.setUplinkForwarding(prev.LANForward, u.LANForward, u.Forward); err != nil {
		return restore(fmt.Errorf("switching forwarding failed: %w", err))
	}
	a.uplinkUfw(ctx, true, u.Forward)
	if !a.simulate {
		if changed, err := a.setConfigValue("mqtt", "uplink_listen", u.Token.Address+":1883"); err == nil && changed {
			a.restartAppLater()
		}
	}
	return nil
}

// uplinkUfw lets an active ufw pass uplink traffic to the ctrl32_uplink table, which then decides (best effort).
func (a *Server) uplinkUfw(ctx context.Context, on, forward bool) {
	out, err := a.run(ctx, 5*time.Second, "ufw", "status")
	if err != nil || !strings.Contains(out, "Status: active") {
		return
	}
	if !on {
		_, _ = a.run(ctx, 10*time.Second, "ufw", "delete", "allow", "in", "on", vpn.UplinkIface)
		_, _ = a.run(ctx, 10*time.Second, "ufw", "route", "delete", "allow", "in", "on", vpn.UplinkIface)
		return
	}
	_, _ = a.run(ctx, 10*time.Second, "ufw", "allow", "in", "on", vpn.UplinkIface, "comment", "ctrl32 uplink (filtered by nftables table ctrl32_uplink)")
	if forward {
		_, _ = a.run(ctx, 10*time.Second, "ufw", "route", "allow", "in", "on", vpn.UplinkIface, "comment", "ctrl32 uplink (filtered by ctrl32_uplink)")
	} else {
		_, _ = a.run(ctx, 10*time.Second, "ufw", "route", "delete", "allow", "in", "on", vpn.UplinkIface)
	}
}

func (a *Server) uplinkApplyUserspace(u *uplinkStored, restart bool) error {
	ep, err := a.uplinkEndpoint(u)
	if err != nil {
		return err
	}
	if u.Transport != vpn.TransportHTTPS {
		if ep, err = resolveEndpoint(ep); err != nil {
			return err
		}
	}
	info := uplinkInfo(u.Token)
	u.LANForward = nil
	if restart || a.upDev == nil {
		if a.upDev != nil {
			a.upDev.Stop()
			a.upDev = nil
		}
		dev, err := a.startUplink(u.Token.PrivateKey, u.Token.Prefix(), u.Token.HubPublicKey, u.Token.PresharedKey, ep, info.Subnets, a.log)
		if err != nil {
			a.stopTunnel()
			return fmt.Errorf("the uplink could not be started: %w", err)
		}
		a.upDev = dev
	} else if err := a.upDev.SetEndpoint(u.Token.HubPublicKey, ep); err != nil {
		return err
	}
	if err := a.upDev.SetRules(u.Rules); err != nil {
		return err
	}
	_ = os.MkdirAll(wgDir, 0o700)
	return writeFileAtomic(uplinkRulesFile, []byte(a.upDev.Describe()), 0o600)
}

// uplinkTeardown removes the uplink from the host (disconnect, or a failed first connect).
func (a *Server) uplinkTeardown(ctx context.Context, u uplinkStored) {
	switch a.uplinkMode() {
	case ModeKernel:
		_, _ = a.run(ctx, time.Minute, "systemctl", "disable", "--now", uplinkUnit)
		_, _ = a.run(ctx, 10*time.Second, "nft", "delete", "table", "inet", vpn.UplinkNftTable)
		_ = a.setUplinkForwarding(u.LANForward, nil, false)
		a.uplinkUfw(ctx, false, false)
		if changed, err := a.setConfigValue("mqtt", "uplink_listen", ""); err == nil && changed {
			a.restartAppLater()
		}
	case ModeUserspace:
		if a.upDev != nil {
			a.upDev.Stop()
			a.upDev = nil
		}
	}
	a.stopTunnel()
	if strings.HasPrefix(readFile(uplinkConfFile()), vpn.UplinkConfHeader) {
		_ = os.Remove(uplinkConfFile()) // never somebody else's wg1.conf
	}
	for _, f := range []string{uplinkNftFile, uplinkRulesFile, uplinkFile} {
		_ = os.Remove(f)
	}
}

// ---- handlers -------------------------------------------------------------------------------------------------------

func (a *Server) uplinkRoutes(mux *http.ServeMux) {
	mux.HandleFunc("PUT /uplink", a.hUplinkConnect)
	mux.HandleFunc("PUT /uplink/rules", a.hUplinkRules)
	mux.HandleFunc("PUT /uplink/transport", a.hUplinkTransport)
	mux.HandleFunc("DELETE /uplink", a.hUplinkDisconnect)
}

func (a *Server) uplinkReady(w http.ResponseWriter) bool {
	if a.uplinkMode() == "" {
		writeErr(w, 501, "the uplink is not managed on this platform")
		return false
	}
	return true
}

func (a *Server) respondUplink(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, a.wgState(r.Context()))
}

// hUplinkConnect connects to a hub or replaces the configuration (a rotated key): PUT /uplink.
func (a *Server) hUplinkConnect(w http.ResponseWriter, r *http.Request) {
	if !a.uplinkReady(w) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var in UplinkRequest
	if err := decodeLarge(r, &in, 1<<20); err != nil {
		writeErr(w, 400, "bad request: "+err.Error())
		return
	}
	tok, err := vpn.ParseUplinkToken(in.Token)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.Transport == "" {
		in.Transport = vpn.TransportUDP
	}
	if in.Transport != vpn.TransportUDP && in.Transport != vpn.TransportHTTPS {
		writeErr(w, 400, "transport must be udp or https")
		return
	}
	if in.Transport == vpn.TransportHTTPS && tok.TunnelURL == "" {
		writeErr(w, 400, "this uplink configuration has no HTTPS transport (the hub has no HTTPS address); use UDP")
		return
	}
	if err := a.uplinkCheck(tok); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	prev, had := a.uplinkLoad()
	u := uplinkStored{Token: tok, Transport: in.Transport, TunnelPort: prev.TunnelPort, Rules: in.Rules, ConnectedAt: time.Now(), AppliedAt: time.Now()}
	if u.Rules == nil {
		u.Rules = []vpn.FwRule{}
	}
	if had {
		u.LANForward = prev.LANForward
	}
	if a.uplinkMode() == ModeKernel {
		if conf := readFile(uplinkConfFile()); conf != "" && !strings.HasPrefix(conf, vpn.UplinkConfHeader) {
			writeErr(w, 500, uplinkConfFile()+" exists and was not written by this server; remove it or rename that interface first")
			return
		}
	}
	if err := a.uplinkApply(r.Context(), &u, prev, had, true); err != nil {
		if !had {
			a.uplinkTeardown(r.Context(), u)
		}
		writeErr(w, 500, err.Error())
		return
	}
	if err := saveJSON(uplinkFile, u, 0o600); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.log.Info("vpn uplink connected", "hub", tok.Endpoint, "address", tok.Address, "transport", u.Transport, "rules", len(u.Rules), "replaced", had)
	a.respondUplink(w, r)
}

// hUplinkRules applies new uplink access rules: PUT /uplink/rules.
func (a *Server) hUplinkRules(w http.ResponseWriter, r *http.Request) {
	if !a.uplinkReady(w) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var in UplinkRulesRequest
	if err := decodeLarge(r, &in, 1<<20); err != nil {
		writeErr(w, 400, "bad request: "+err.Error())
		return
	}
	u, ok := a.uplinkLoad()
	if !ok {
		writeErr(w, 409, "no uplink is connected")
		return
	}
	prev := u
	u.Rules = in.Rules
	if u.Rules == nil {
		u.Rules = []vpn.FwRule{}
	}
	u.AppliedAt, u.Error = time.Now(), ""
	if err := a.uplinkApply(r.Context(), &u, prev, true, false); err != nil {
		prev.Error = err.Error()
		_ = saveJSON(uplinkFile, prev, 0o600)
		writeErr(w, 500, err.Error())
		return
	}
	if err := saveJSON(uplinkFile, u, 0o600); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.log.Info("vpn uplink rules applied", "rules", len(u.Rules), "forward", u.Forward)
	a.respondUplink(w, r)
}

// hUplinkTransport switches between UDP and the HTTPS transport: PUT /uplink/transport.
func (a *Server) hUplinkTransport(w http.ResponseWriter, r *http.Request) {
	if !a.uplinkReady(w) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var in UplinkTransportRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request: "+err.Error())
		return
	}
	u, ok := a.uplinkLoad()
	if !ok {
		writeErr(w, 409, "no uplink is connected")
		return
	}
	if in.Transport != vpn.TransportUDP && in.Transport != vpn.TransportHTTPS {
		writeErr(w, 400, "transport must be udp or https")
		return
	}
	if in.Transport == vpn.TransportHTTPS && u.Token.TunnelURL == "" {
		writeErr(w, 400, "this uplink configuration has no HTTPS transport (the hub has no HTTPS address); use UDP")
		return
	}
	prev := u
	u.Transport, u.AppliedAt, u.Error = in.Transport, time.Now(), ""
	if err := a.uplinkApply(r.Context(), &u, prev, true, false); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := saveJSON(uplinkFile, u, 0o600); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.log.Info("vpn uplink transport", "transport", u.Transport)
	a.respondUplink(w, r)
}

// hUplinkDisconnect removes the uplink and its keys: DELETE /uplink.
func (a *Server) hUplinkDisconnect(w http.ResponseWriter, r *http.Request) {
	if !a.uplinkReady(w) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	u, ok := a.uplinkLoad()
	if ok {
		a.uplinkTeardown(r.Context(), u)
		a.log.Info("vpn uplink disconnected", "hub", u.Token.Endpoint)
	}
	a.respondUplink(w, r)
}

// startUplinkFromFiles brings the uplink up after the agent starts: the userspace device (Windows) and the HTTPS
// transport (both; on Linux wg-quick@wg1 is started by systemd and needs the local end of the transport).
func (a *Server) startUplinkFromFiles() {
	if a.uplinkMode() == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	u, ok := a.uplinkLoad()
	if !ok {
		return
	}
	switch a.uplinkMode() {
	case ModeUserspace:
		if err := a.uplinkApplyUserspace(&u, true); err != nil {
			a.log.Error("vpn uplink did not start", "err", err)
			u.Error = err.Error()
		}
	case ModeKernel, ModeSimulation:
		if u.Transport != vpn.TransportHTTPS {
			return
		}
		old := u.TunnelPort
		ep, err := a.uplinkEndpoint(&u)
		if err != nil {
			a.log.Error("vpn uplink: the HTTPS transport did not start", "err", err)
			return
		}
		if u.TunnelPort != old {
			// the stored port was taken: point WireGuard at the new local end
			_ = a.writeUplinkFiles(u, ep)
			if a.uplinkMode() == ModeKernel {
				_, _ = a.run(context.Background(), 10*time.Second, "wg", "set", vpn.UplinkIface, "peer", u.Token.HubPublicKey, "endpoint", ep)
			}
		}
	}
	_ = saveJSON(uplinkFile, u, 0o600)
	a.log.Info("vpn uplink started", "hub", u.Token.Endpoint, "transport", u.Transport)
}

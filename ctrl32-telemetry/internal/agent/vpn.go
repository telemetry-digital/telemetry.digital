package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics/setup"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

// WireGuard server and VPN firewall (ADR-0012, ADR-0018, spec 024). The web service owns the peers and the access
// rules (database) and sends the complete state with PUT /vpn; the agent keeps what must stay root-only — the server
// private key and the peers' preshared keys — validates the state again, renders /etc/wireguard/wg0.conf and the
// nftables table ctrl32_vpn, and applies both together: the firewall first (checked with `nft -c` before), then the
// peers (`wg syncconf`, no tunnel drops), then the routes of site subnets and forwarding. If a step fails everything
// is put back as it was and the error is reported. The interface itself is still brought up by wg-quick@wg0, whose
// PreUp loads the firewall before the interface exists. On Windows (no kernel WireGuard, no nftables) the agent runs
// a userspace hub instead (hub.go, internal/wghub, ADR-0020) that enforces the same compiled rules in-process.
//
// wgctrl (netlink) was considered for applying peers; the wg CLI stays (ADR-0018): `wg syncconf` is atomic, proven in
// production, and the wgctrl module would add five dependencies to the root process for structured statistics only.
// Keys are generated in Go (internal/vpn), so `wg genkey` is no longer run.

type wgStored struct {
	Enabled    bool      `json:"enabled"`
	ListenPort int       `json:"listen_port"`
	Address    string    `json:"address"`
	Endpoint   string    `json:"endpoint"`
	DNS        string    `json:"dns"`
	PublicKey  string    `json:"public_key"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// wgPeerStored is one peer in peers.json (0600). Older agents wrote the same fields without subnets and disabled.
type wgPeerStored struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	PublicKey    string    `json:"public_key"`
	PresharedKey string    `json:"preshared_key"`
	Address      string    `json:"address"` // with /32
	Subnets      []string  `json:"subnets,omitempty"`
	Disabled     bool      `json:"disabled,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	CreatedBy    string    `json:"created_by"`
}

// vpnStored is the last applied firewall state (vpn.json).
type vpnStored struct {
	Rules     []vpn.FwRule `json:"rules"`
	Forward   bool         `json:"forward"`
	AppliedAt time.Time    `json:"applied_at"`
	Error     string       `json:"error,omitempty"`
}

// SetSimulation makes the agent a VPN simulation for development and tests: state and rendered files live in dir,
// nothing on the host changes, and only the VPN, detector, update and backup operations are served (routes; backups
// with a stand-in dump and without a database restore). An update in the
// simulation verifies the release exactly like a real one and installs its binary into dir/bin (it runs that binary's
// `version` and `migrate`, never restarts a service and never restores a database).
func (a *Server) SetSimulation(dir string) {
	a.simulate = true
	a.an.sim = &setup.SimFetcher{Files: setup.SimulatedArtifacts(analyticsPlatform()), Delay: 150 * time.Millisecond}
	optDir, etcDir, libDir = dir, dir, dir
	// updates install into the simulation directory: never the running binary, never the real backups
	binPath = filepath.Join(dir, "bin", "ctrl32-telemetry"+map[bool]string{true: ".exe"}[isWindows()])
	BackupDir = filepath.Join(dir, "backups")
	wgConfFile = filepath.Join(dir, "wireguard", "wg0.conf")
	derivePaths()
	var mu sync.Mutex
	a.exec = func(_ context.Context, _ time.Duration, name string, args ...string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		if f, err := os.OpenFile(filepath.Join(dir, "commands.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			fmt.Fprintf(f, "%s %s %s\n", time.Now().Format(time.RFC3339), name, strings.Join(args, " "))
			_ = f.Close()
		}
		if name == "wg-quick" && len(args) == 2 && args[0] == "strip" {
			return readFile(wgConfFile), nil
		}
		return "", nil
	}
	a.localNets = func() []netip.Prefix { return nil }
	a.setForwarding = func(bool) error { return nil }
	a.ifForwarding = func(string) (bool, error) { return false, nil }
	a.setIfForwarding = func(string, bool) error { return nil }
	a.tunnelWatchEvery = 0
}

// setWGForwarding switches IPv4 forwarding for packets that arrive on wg0 only (the host's other interfaces keep
// their setting). Linux only.
func setWGForwarding(on bool) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	v := "0\n"
	if on {
		v = "1\n"
	}
	return os.WriteFile("/proc/sys/net/ipv4/conf/"+vpn.Iface+"/forwarding", []byte(v), 0o644)
}

func (a *Server) run(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	return a.exec(ctx, timeout, name, args...)
}

func (a *Server) wgSupported() bool { return a.vpnMode() != "" }

func (a *Server) wgInstalled(ctx context.Context) bool {
	_, err := a.run(ctx, 5*time.Second, "wg", "--version")
	return err == nil
}

func (a *Server) wgLoad() (wgStored, []wgPeerStored, vpnStored) {
	var st wgStored
	_ = loadJSON(wgStateFile, &st)
	peers := []wgPeerStored{}
	_ = loadJSON(wgPeersFile, &peers)
	var vs vpnStored
	_ = loadJSON(wgVPNFile, &vs)
	return st, peers, vs
}

func (a *Server) wgState(ctx context.Context) WireGuardState {
	st, peers, vs := a.wgLoad()
	out := WireGuardState{Supported: a.wgSupported(), Simulated: a.simulate, Enabled: st.Enabled, ListenPort: st.ListenPort, Address: st.Address,
		Endpoint: st.Endpoint, DNS: st.DNS, PublicKey: st.PublicKey, UpdatedAt: st.UpdatedAt, Peers: []WireGuardPeer{}}
	if !out.Supported {
		return out
	}
	out.Uplink = a.uplinkState(ctx)
	out.Mode = a.vpnMode()
	if out.Mode == ModeUserspace {
		return a.hubState(out, peers, vs)
	}
	out.Installed = a.wgInstalled(ctx)
	if ip, _, err := net.ParseCIDR(st.Address); err == nil {
		out.MQTT = ip.String() + ":1883"
	}
	live := map[string][]string{}
	if a.simulate {
		out.Running = st.Enabled
	} else if out.Installed {
		if dump, err := a.run(ctx, 5*time.Second, "wg", "show", vpn.Iface, "dump"); err == nil {
			out.Running = true
			for i, l := range strings.Split(dump, "\n") {
				f := strings.Split(l, "\t")
				if i == 0 || len(f) < 8 {
					continue
				}
				live[f[0]] = f
			}
		}
	}
	out.Firewall = VPNFirewall{Rules: len(vs.Rules), Forward: vs.Forward, AppliedAt: vs.AppliedAt, Error: vs.Error, Ruleset: readFile(wgNftFile)}
	out.Firewall.Applied = out.Firewall.Ruleset != "" && (out.Running || a.simulate) && vs.Error == ""
	for _, p := range peers {
		wp := WireGuardPeer{ID: p.ID, Name: p.Name, PublicKey: p.PublicKey, Address: p.Address, Subnets: p.Subnets, Enabled: !p.Disabled, CreatedAt: p.CreatedAt, CreatedBy: p.CreatedBy}
		if f, ok := live[p.PublicKey]; ok {
			if f[2] != "(none)" {
				wp.Endpoint = f[2]
			}
			if hs, _ := strconv.ParseInt(f[4], 10, 64); hs > 0 {
				wp.LatestHandshake = time.Unix(hs, 0)
				wp.Online = time.Since(wp.LatestHandshake) < 3*time.Minute
			}
			wp.RxBytes, _ = strconv.ParseInt(f[5], 10, 64)
			wp.TxBytes, _ = strconv.ParseInt(f[6], 10, 64)
		}
		out.Peers = append(out.Peers, wp)
	}
	return out
}

// hubState fills the state of the userspace hub: WireGuard is built in, the filter is the hub's own.
func (a *Server) hubState(out WireGuardState, peers []wgPeerStored, vs vpnStored) WireGuardState {
	out.Installed = true
	out.Running = a.hub.Running()
	if ip, _, err := net.ParseCIDR(out.Address); err == nil {
		out.MQTT = ip.String() + ":1883"
	}
	live := map[string]hubPeer{}
	for _, p := range a.hub.Peers() {
		live[p.PublicKey] = p
	}
	out.Firewall = VPNFirewall{Rules: len(vs.Rules), Forward: vs.Forward, AppliedAt: vs.AppliedAt, Error: vs.Error, Ruleset: readFile(wgRulesFile)}
	out.Firewall.Applied = out.Firewall.Ruleset != "" && out.Running && vs.Error == ""
	for _, p := range peers {
		wp := WireGuardPeer{ID: p.ID, Name: p.Name, PublicKey: p.PublicKey, Address: p.Address, Subnets: p.Subnets, Enabled: !p.Disabled, CreatedAt: p.CreatedAt, CreatedBy: p.CreatedBy}
		if l, ok := live[p.PublicKey]; ok {
			wp.Endpoint, wp.RxBytes, wp.TxBytes = l.Endpoint, l.Rx, l.Tx
			if !l.Handshake.IsZero() {
				wp.LatestHandshake = l.Handshake
				wp.Online = time.Since(l.Handshake) < 3*time.Minute
			}
		}
		out.Peers = append(out.Peers, wp)
	}
	return out
}

func (a *Server) hWireGuard(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, a.wgState(r.Context()))
}

// hStatusSimulated is the status of the simulation: the agent and the VPN only.
func (a *Server) hStatusSimulated(w http.ResponseWriter, r *http.Request) {
	st := Status{Agent: AgentInfo{Version: a.version + " (VPN simulation)", StartedAt: a.started, Socket: a.socket, OS: runtime.GOOS},
		Versions: map[string]string{"ctrl32-telemetry": a.version}, Services: []ServiceState{}, Containers: []ContainerInfo{}}
	st.Host = HostInfo{Arch: runtime.GOARCH, CPUs: runtime.NumCPU(), Time: time.Now()}
	st.Host.Hostname, _ = os.Hostname()
	st.WireGuard = a.wgState(r.Context())
	writeJSON(w, 200, st)
}

func toDesiredPeers(peers []wgPeerStored) []vpn.DesiredPeer {
	out := make([]vpn.DesiredPeer, 0, len(peers))
	for _, p := range peers {
		out = append(out, vpn.DesiredPeer{ID: p.ID, Name: p.Name, PublicKey: p.PublicKey, PresharedKey: p.PresharedKey,
			Address: strings.TrimSuffix(p.Address, "/32"), Subnets: p.Subnets, Enabled: !p.Disabled})
	}
	return out
}

func (a *Server) serverKey() string { return strings.TrimSpace(readFile(wgKeyFile)) }

// writeConfFiles renders wg0.conf (when the server key exists) and the nftables file from a state.
func (a *Server) writeConfFiles(st wgStored, peers []wgPeerStored, vs vpnStored) error {
	srv, err := vpn.ServerPrefix(st.Address)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(wgDir, 0o700); err != nil {
		return err
	}
	if err := writeFileAtomic(wgNftFile, []byte(vpn.RenderNft(srv.Addr(), vs.Rules)), 0o600); err != nil {
		return err
	}
	key := a.serverKey()
	if key == "" {
		return nil
	}
	_ = os.MkdirAll(filepath.Dir(wgConfFile), 0o700)
	return writeFileAtomic(wgConfFile, []byte(vpn.ServerConf(st.Address, st.ListenPort, key, wgNftFile, vs.Forward, toDesiredPeers(peers))), 0o600)
}

// wgSync applies peer changes to the live interface without dropping tunnels.
func (a *Server) wgSync(ctx context.Context) error {
	stripped, err := a.run(ctx, 20*time.Second, "wg-quick", "strip", vpn.Iface)
	if err != nil {
		return err
	}
	// Ubuntu confines wg with AppArmor: it may read files only under /etc/wireguard, never /tmp.
	syncFile := filepath.Dir(wgConfFile) + "/wg0.sync.conf"
	if err := writeFileAtomic(syncFile, []byte(stripped+"\n"), 0o600); err != nil {
		return err
	}
	defer os.Remove(syncFile)
	_, err = a.run(ctx, 20*time.Second, "wg", "syncconf", vpn.Iface, syncFile)
	return err
}

// syncRoutes makes the kernel routes through wg0 match the routed subnets of the enabled site peers (wg-quick adds
// them only when the interface comes up; `wg syncconf` does not touch routes).
func (a *Server) syncRoutes(ctx context.Context, peers []wgPeerStored) error {
	want := map[string]bool{}
	for _, p := range peers {
		if !p.Disabled {
			for _, s := range p.Subnets {
				want[s] = true
			}
		}
	}
	out, err := a.run(ctx, 10*time.Second, "ip", "-4", "-o", "route", "show", "dev", vpn.Iface)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) == 0 || strings.Contains(l, "proto kernel") {
			continue
		}
		if p, err := netip.ParsePrefix(f[0]); err == nil {
			have[p.String()] = true
		} else if ad, err := netip.ParseAddr(f[0]); err == nil {
			have[netip.PrefixFrom(ad, 32).String()] = true
		}
	}
	keys := make([]string, 0, len(want))
	for s := range want {
		keys = append(keys, s)
	}
	sort.Strings(keys)
	for _, s := range keys {
		if !have[s] {
			if _, err := a.run(ctx, 10*time.Second, "ip", "-4", "route", "replace", s, "dev", vpn.Iface); err != nil {
				return err
			}
		}
	}
	for s := range have {
		if !want[s] {
			_, _ = a.run(ctx, 10*time.Second, "ip", "-4", "route", "del", s, "dev", vpn.Iface)
		}
	}
	return nil
}

// ufwOpen lets an active ufw pass VPN traffic to the ctrl32_vpn table, which is then the single place that decides
// (best effort; without ufw nothing is done).
func (a *Server) ufwOpen(ctx context.Context, forward bool) {
	out, err := a.run(ctx, 5*time.Second, "ufw", "status")
	if err != nil || !strings.Contains(out, "Status: active") {
		return
	}
	_, _ = a.run(ctx, 10*time.Second, "ufw", "allow", "in", "on", vpn.Iface, "comment", "ctrl32 VPN (filtered by nftables table ctrl32_vpn)")
	if forward {
		_, _ = a.run(ctx, 10*time.Second, "ufw", "route", "allow", "in", "on", vpn.Iface, "out", "on", vpn.Iface, "comment", "ctrl32 VPN (filtered by ctrl32_vpn)")
	} else {
		_, _ = a.run(ctx, 10*time.Second, "ufw", "route", "delete", "allow", "in", "on", vpn.Iface, "out", "on", vpn.Iface)
	}
}

// applyLive puts a rendered state on the running interface: firewall, peers, routes, forwarding. On error it
// restores the previous files and state and returns the error.
func (a *Server) applyLive(ctx context.Context, st wgStored, oldPeers, newPeers []wgPeerStored, oldVS, newVS vpnStored) error {
	oldNft, oldConf := readFile(wgNftFile), readFile(wgConfFile)
	restore := func(cause error) error {
		_ = writeFileAtomic(wgConfFile, []byte(oldConf), 0o600)
		// no earlier table (first apply after an upgrade): the new one stays loaded — a table is safer than none
		if oldNft != "" {
			_ = writeFileAtomic(wgNftFile, []byte(oldNft), 0o600)
			_, _ = a.run(ctx, 20*time.Second, "nft", "-f", wgNftFile)
		}
		_ = a.wgSync(ctx)
		_ = a.syncRoutes(ctx, oldPeers)
		_ = a.setForwarding(oldVS.Forward)
		a.log.Error("vpn apply failed, previous state restored", "err", cause)
		return cause
	}
	// 1. firewall: check the new table, then load it (one transaction replaces the old table)
	srv, err := vpn.ServerPrefix(st.Address)
	if err != nil {
		return err
	}
	newNft := vpn.RenderNft(srv.Addr(), newVS.Rules)
	check := wgNftFile + ".check"
	if err := writeFileAtomic(check, []byte(newNft), 0o600); err != nil {
		return err
	}
	defer os.Remove(check)
	if out, err := a.run(ctx, 20*time.Second, "nft", "-c", "-f", check); err != nil {
		return fmt.Errorf("nftables rejected the access rules: %s", firstLine(out))
	}
	if err := a.writeConfFiles(st, newPeers, newVS); err != nil {
		return restore(err)
	}
	if _, err := a.run(ctx, 5*time.Second, "wg", "show", vpn.Iface); err != nil {
		// the interface is down (it failed earlier or was stopped): bring it up with the new files; PreUp loads the
		// firewall before the interface exists
		if out, err := a.run(ctx, time.Minute, "systemctl", "restart", wgUnit); err != nil {
			return restore(fmt.Errorf("WireGuard could not be started: %s", firstLine(out)))
		}
		_ = a.setForwarding(newVS.Forward)
		a.ufwOpen(ctx, newVS.Forward)
		return nil
	}
	if out, err := a.run(ctx, 20*time.Second, "nft", "-f", wgNftFile); err != nil {
		return restore(fmt.Errorf("loading the firewall failed: %s", firstLine(out)))
	}
	// 2. peers
	if err := a.wgSync(ctx); err != nil {
		return restore(fmt.Errorf("applying the peers failed: %w", err))
	}
	// 3. routes of site subnets and forwarding through wg0
	if err := a.syncRoutes(ctx, newPeers); err != nil {
		return restore(fmt.Errorf("routing the site subnets failed: %w", err))
	}
	if err := a.setForwarding(newVS.Forward); err != nil {
		return restore(fmt.Errorf("switching forwarding failed: %w", err))
	}
	a.ufwOpen(ctx, newVS.Forward)
	return nil
}

// hApplyVPN receives the complete VPN state from the web service.
func (a *Server) hApplyVPN(w http.ResponseWriter, r *http.Request) {
	if !a.wgSupported() {
		writeErr(w, 501, "the VPN is not managed on this platform")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var d vpn.Desired
	if err := decodeLarge(r, &d, 4<<20); err != nil {
		writeErr(w, 400, "bad request: "+err.Error())
		return
	}
	st, oldPeers, oldVS := a.wgLoad()
	srv, err := vpn.ServerPrefix(st.Address)
	if err != nil {
		writeErr(w, 400, "configure the VPN (address) first")
		return
	}
	if err := vpn.ValidateDesired(&d, srv, a.localNets()); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var subnets []string
	for _, p := range d.Peers {
		if p.Enabled {
			subnets = append(subnets, p.Subnets...)
		}
	}
	if err := a.uplinkConflict(netip.Prefix{}, subnets); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	// carry the preshared keys over by public key (a rotation or a new peer sends a new one)
	psk := map[string]string{}
	created := map[string]wgPeerStored{}
	for _, p := range oldPeers {
		psk[p.PublicKey] = p.PresharedKey
		created[p.PublicKey] = p
	}
	newPeers := make([]wgPeerStored, 0, len(d.Peers))
	for _, p := range d.Peers {
		k := p.PresharedKey
		if k == "" {
			k = psk[p.PublicKey]
		}
		np := wgPeerStored{ID: p.ID, Name: p.Name, PublicKey: p.PublicKey, PresharedKey: k, Address: p.Address + "/32", Subnets: p.Subnets, Disabled: !p.Enabled, CreatedAt: time.Now()}
		if o, ok := created[p.PublicKey]; ok {
			np.CreatedAt, np.CreatedBy = o.CreatedAt, o.CreatedBy
		}
		newPeers = append(newPeers, np)
	}
	newVS := vpnStored{Rules: d.Rules, Forward: d.Forward, AppliedAt: time.Now()}
	if newVS.Rules == nil {
		newVS.Rules = []vpn.FwRule{}
	}
	ctx := r.Context()
	if a.vpnMode() == ModeUserspace {
		if st.Enabled {
			if err := a.hubApply(st, newPeers, newVS); err != nil {
				a.log.Error("vpn apply failed, previous state kept", "err", err)
				oldVS.Error = err.Error()
				_ = saveJSON(wgVPNFile, oldVS, 0o600)
				writeErr(w, 500, err.Error())
				return
			}
		}
		if err := a.writeHubRules(st, newPeers, newVS); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	} else if st.Enabled && !a.simulate {
		if err := a.applyLive(ctx, st, oldPeers, newPeers, oldVS, newVS); err != nil {
			oldVS.Error = err.Error()
			_ = saveJSON(wgVPNFile, oldVS, 0o600)
			writeErr(w, 500, err.Error())
			return
		}
	} else if err := a.writeConfFiles(st, newPeers, newVS); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := saveJSON(wgPeersFile, newPeers, 0o600); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := saveJSON(wgVPNFile, newVS, 0o600); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.log.Info("vpn applied", "peers", len(newPeers), "rules", len(newVS.Rules), "forward", newVS.Forward, "live", st.Enabled && !a.simulate)
	writeJSON(w, 200, a.wgState(ctx))
}

func decodeLarge(r *http.Request, dst any, max int64) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, max))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// hSetWireGuard configures the interface: enabled, port, VPN address, endpoint and DNS for clients.
func (a *Server) hSetWireGuard(w http.ResponseWriter, r *http.Request) {
	if !a.wgSupported() {
		writeErr(w, 501, "WireGuard is not managed on this platform")
		return
	}
	userspace := a.vpnMode() == ModeUserspace
	a.mu.Lock()
	defer a.mu.Unlock()
	var in WireGuardRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request: "+err.Error())
		return
	}
	ctx := r.Context()
	if in.ListenPort == 0 {
		in.ListenPort = 51820
	}
	if in.ListenPort < 1 || in.ListenPort > 65535 {
		writeErr(w, 400, "invalid port")
		return
	}
	if in.Address == "" {
		in.Address = "10.66.0.1/24"
	}
	srv, err := vpn.ServerPrefix(in.Address)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Address = srv.String()
	if in.Enabled {
		if err := a.uplinkConflict(srv, nil); err != nil {
			writeErr(w, 409, err.Error())
			return
		}
	}
	in.Endpoint = strings.TrimSpace(in.Endpoint)
	if in.Endpoint != "" {
		host, port, err := net.SplitHostPort(in.Endpoint)
		if err != nil || host == "" || strings.ContainsAny(host, " '\"\\") {
			writeErr(w, 400, "endpoint must be host:port")
			return
		}
		if _, err := strconv.Atoi(port); err != nil {
			writeErr(w, 400, "endpoint must be host:port")
			return
		}
	} else if !a.simulate && !userspace {
		if cs := a.caddyState(); cs.Domain != "" {
			in.Endpoint = cs.Domain + ":" + strconv.Itoa(in.ListenPort)
		}
	}
	in.DNS = strings.TrimSpace(in.DNS)
	if in.DNS != "" && net.ParseIP(in.DNS) == nil {
		writeErr(w, 400, "DNS must be an IP address")
		return
	}
	st, peers, vs := a.wgLoad()
	for _, p := range peers {
		ad, err := netip.ParseAddr(strings.TrimSuffix(p.Address, "/32"))
		if err == nil && (!srv.Masked().Contains(ad) || ad == srv.Addr()) {
			writeErr(w, 400, "peer "+p.Name+" ("+p.Address+") is outside the new VPN range or on the server address; remove it first or keep the range")
			return
		}
	}
	if in.Enabled && !a.simulate && !userspace {
		missing := []string{}
		if !a.wgInstalled(ctx) {
			missing = append(missing, "wireguard-tools")
		}
		if _, err := a.run(ctx, 5*time.Second, "nft", "--version"); err != nil {
			missing = append(missing, "nftables")
		}
		if len(missing) > 0 {
			a.log.Info("installing", "packages", missing)
			if out, err := a.run(ctx, 4*time.Minute, "apt-get", append([]string{"install", "-y", "-qq"}, missing...)...); err != nil {
				writeErr(w, 500, strings.Join(missing, " and ")+" could not be installed: "+firstLine(out))
				return
			}
		}
	}
	if in.Enabled || st.PublicKey != "" {
		if a.serverKey() == "" {
			priv, _, err := vpn.GenerateKey()
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			_ = os.MkdirAll(wgDir, 0o700)
			if err := writeFileAtomic(wgKeyFile, []byte(priv+"\n"), 0o600); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		}
		pub, err := vpn.PublicKey(a.serverKey())
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		st.PublicKey = pub
	}
	st.Enabled, st.ListenPort, st.Address, st.Endpoint, st.DNS, st.UpdatedAt = in.Enabled, in.ListenPort, in.Address, in.Endpoint, in.DNS, time.Now()
	if err := saveJSON(wgStateFile, st, 0o600); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if vs.Rules == nil {
		vs.Rules = []vpn.FwRule{}
	}
	if userspace {
		// the hub is restarted with the new port, address and key; peers and rules are applied again
		a.hub.Stop()
		vs.Error = ""
		if in.Enabled {
			if err := a.hubApply(st, peers, vs); err != nil {
				a.hub.Stop()
				vs.Error = err.Error()
				_ = saveJSON(wgVPNFile, vs, 0o600)
				writeErr(w, 500, "the VPN could not be started: "+err.Error())
				return
			}
		}
		_ = saveJSON(wgVPNFile, vs, 0o600)
		if err := a.writeHubRules(st, peers, vs); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		a.winFirewall(ctx, in.ListenPort, in.Enabled)
		a.log.Info("wireguard configured (userspace hub)", "enabled", in.Enabled, "port", in.ListenPort, "address", in.Address, "endpoint", in.Endpoint)
		writeJSON(w, 200, a.wgState(ctx))
		return
	}
	// the firewall file must exist before wg-quick starts (PreUp loads it; without it wg0 does not come up)
	if err := a.writeConfFiles(st, peers, vs); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !a.simulate {
		if in.Enabled {
			if out, err := a.run(ctx, time.Minute, "systemctl", "enable", "--now", wgUnit); err != nil {
				writeErr(w, 500, "WireGuard could not be started: "+firstLine(out))
				return
			}
			if out, err := a.run(ctx, time.Minute, "systemctl", "restart", wgUnit); err != nil {
				writeErr(w, 500, "WireGuard could not be restarted: "+firstLine(out))
				return
			}
			if out, err := a.run(ctx, 5*time.Second, "ufw", "status"); err == nil && strings.Contains(out, "Status: active") {
				_, _ = a.run(ctx, 10*time.Second, "ufw", "allow", strconv.Itoa(in.ListenPort)+"/udp", "comment", "ctrl32 WireGuard")
			}
			a.ufwOpen(ctx, vs.Forward)
		} else {
			_, _ = a.run(ctx, time.Minute, "systemctl", "disable", "--now", wgUnit)
		}
		// the broker listens inside the VPN so devices reach MQTT without TLS/public exposure; the firewall decides
		// which peers may (access rules)
		vpnListen := ""
		if in.Enabled {
			vpnListen = srv.Addr().String() + ":1883"
		}
		if changed, err := a.setConfigValue("mqtt", "vpn_listen", vpnListen); err != nil {
			writeErr(w, 500, "config update failed: "+err.Error())
			return
		} else if changed {
			a.restartAppLater()
		}
	}
	a.log.Info("wireguard configured", "enabled", in.Enabled, "port", in.ListenPort, "address", in.Address, "endpoint", in.Endpoint, "simulated", a.simulate)
	writeJSON(w, 200, a.wgState(ctx))
}

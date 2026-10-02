package vpn

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

// Interface and nftables names used on the host.
const (
	Iface    = "wg0"
	NftTable = "ctrl32_vpn"
)

var idRe = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// ValidateDesired is the agent's own check of a state before it touches the host (the web service validated it too,
// but the root process trusts nothing it receives). localNets are the networks of the host's other interfaces: a
// routed subnet that overlaps one of them would break the host's own networking.
func ValidateDesired(d *Desired, server netip.Prefix, localNets []netip.Prefix) error {
	vpnNet := server.Masked()
	if len(d.Peers) > MaxPeers || len(d.Rules) > MaxRules {
		return errors.New("too many peers or rules")
	}
	addrs, keys, ids := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var allSubnets []netip.Prefix
	for _, p := range d.Peers {
		if !idRe.MatchString(p.ID) || ids[p.ID] {
			return fmt.Errorf("peer id %q invalid or duplicate", p.ID)
		}
		ids[p.ID] = true
		if err := ValidName(p.Name); err != nil {
			return fmt.Errorf("peer %s: %w", p.ID, err)
		}
		if !ValidKey(p.PublicKey) || keys[p.PublicKey] {
			return fmt.Errorf("peer %s: public key invalid or duplicate", p.Name)
		}
		keys[p.PublicKey] = true
		if p.PresharedKey != "" && !ValidKey(p.PresharedKey) {
			return fmt.Errorf("peer %s: preshared key invalid", p.Name)
		}
		a, err := netip.ParseAddr(p.Address)
		if err != nil || !a.Is4() || !vpnNet.Contains(a) || a == server.Addr() || a == vpnNet.Addr() || addrs[p.Address] {
			return fmt.Errorf("peer %s: address %q must be a free host address of %s", p.Name, p.Address, vpnNet)
		}
		addrs[p.Address] = true
		subs, err := ParseSubnets(p.Subnets, server)
		if err != nil {
			return fmt.Errorf("peer %s: %w", p.Name, err)
		}
		for _, s := range subs {
			for _, o := range allSubnets {
				if s.Overlaps(o) {
					return fmt.Errorf("peer %s: %s overlaps %s of another peer", p.Name, s, o)
				}
			}
			for _, l := range localNets {
				if s.Overlaps(l) {
					return fmt.Errorf("peer %s: routed subnet %s overlaps the host network %s; routing it into the VPN would cut the server off", p.Name, s, l)
				}
			}
		}
		allSubnets = append(allSubnets, subs...)
	}
	within := func(s string) (netip.Prefix, bool) {
		p, err := netip.ParsePrefix(s)
		if err != nil || !p.Addr().Is4() || p.Masked() != p {
			return p, false
		}
		if vpnNet.Contains(p.Addr()) && p.Bits() >= vpnNet.Bits() {
			return p, true
		}
		for _, q := range allSubnets {
			if q.Contains(p.Addr()) && p.Bits() >= q.Bits() {
				return p, true
			}
		}
		return p, false
	}
	for _, r := range d.Rules {
		if !idRe.MatchString(r.ID) {
			return fmt.Errorf("rule id %q invalid", r.ID)
		}
		if r.Chain != ChainInput && r.Chain != ChainForward {
			return fmt.Errorf("rule %s: chain must be input or forward", r.ID)
		}
		if len(r.Src) == 0 || len(r.Dst) == 0 {
			return fmt.Errorf("rule %s: empty source or destination", r.ID)
		}
		for _, s := range r.Src {
			if _, ok := within(s); !ok {
				return fmt.Errorf("rule %s: source %q is not a VPN address or a routed subnet", r.ID, s)
			}
		}
		for _, s := range r.Dst {
			p, ok := within(s)
			if r.Chain == ChainInput {
				ok = p == netip.PrefixFrom(server.Addr(), 32)
			}
			if !ok {
				return fmt.Errorf("rule %s: destination %q not allowed", r.ID, s)
			}
		}
		switch r.Proto {
		case "tcp", "udp":
			if _, err := ParsePorts(strings.Join(r.Ports, ",")); err != nil {
				return fmt.Errorf("rule %s: %w", r.ID, err)
			}
		case "icmp", "any":
			if len(r.Ports) > 0 {
				return fmt.Errorf("rule %s: ports only for tcp and udp", r.ID)
			}
		default:
			return fmt.Errorf("rule %s: protocol must be tcp, udp, icmp or any", r.ID)
		}
	}
	return nil
}

// LocalNets lists the IPv4 networks of the host's interfaces except the VPN interface (loopback included).
func LocalNets() []netip.Prefix {
	var out []netip.Prefix
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, i := range ifs {
		if i.Name == Iface {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
				ones, _ := n.Mask.Size()
				if ad, ok := netip.AddrFromSlice(n.IP.To4()); ok {
					out = append(out, netip.PrefixFrom(ad, ones).Masked())
				}
			}
		}
	}
	return out
}

// ---- nftables ---------------------------------------------------------------------------------------------------

// RenderNft renders the dedicated nftables table. The file replaces the table atomically (`nft -f`): it creates the
// table if missing, deletes it and defines it again in one transaction. Traffic that does not arrive on (or leave
// through) the VPN interface is never judged here; from the VPN everything is dropped unless a rule accepts it.
// Replies to connections the server itself opens (e.g. polling a PLC at a site) pass as established.
func RenderNft(server netip.Addr, rules []FwRule) string {
	var b strings.Builder
	b.WriteString("# ctrl32 telemetry — VPN access rules, generated by the server agent (System → VPN). Do not edit.\n")
	b.WriteString("# Default deny for everything that arrives from the VPN interface " + Iface + ".\n")
	b.WriteString("table inet " + NftTable + "\ndelete table inet " + NftTable + "\n")
	b.WriteString("table inet " + NftTable + " {\n")
	chain := func(name, guard string, extra []string, chainName string) {
		b.WriteString("\tchain " + name + " {\n")
		b.WriteString("\t\ttype filter hook " + name + " priority filter - 10; policy accept;\n")
		b.WriteString("\t\t" + guard + " accept\n")
		b.WriteString("\t\tct state established,related accept\n")
		b.WriteString("\t\tct state invalid drop\n")
		for _, l := range extra {
			b.WriteString("\t\t" + l + "\n")
		}
		for _, r := range rules {
			if r.Chain == chainName {
				b.WriteString("\t\t" + nftRule(r) + "\n")
			}
		}
		b.WriteString("\t\tcounter drop comment \"default deny\"\n\t}\n")
	}
	chain("input", `iifname != "`+Iface+`"`, []string{"ip daddr " + server.String() + " icmp type echo-request accept comment \"ping the server\""}, ChainInput)
	chain("forward", `iifname != "`+Iface+`" oifname != "`+Iface+`"`, nil, ChainForward)
	b.WriteString("}\n")
	return b.String()
}

func nftSet(items []string) string {
	if len(items) == 1 {
		return items[0]
	}
	return "{ " + strings.Join(items, ", ") + " }"
}

func nftRule(r FwRule) string {
	var parts []string
	if r.Chain == ChainForward {
		parts = append(parts, `iifname "`+Iface+`" oifname "`+Iface+`"`)
	}
	parts = append(parts, "ip saddr "+nftSet(r.Src), "ip daddr "+nftSet(r.Dst))
	switch r.Proto {
	case "tcp", "udp":
		if len(r.Ports) > 0 {
			parts = append(parts, r.Proto+" dport "+nftSet(r.Ports))
		} else {
			parts = append(parts, "meta l4proto "+r.Proto)
		}
	case "icmp":
		parts = append(parts, "ip protocol icmp")
	}
	parts = append(parts, `counter accept comment "rule `+r.ID+`"`)
	return strings.Join(parts, " ")
}

// ---- WireGuard configuration files ------------------------------------------------------------------------------

// ServerConf renders /etc/wireguard/wg0.conf. The firewall table is loaded before the interface exists (PreUp), so
// there is no moment in which peers are reachable without it; if loading fails wg-quick does not bring wg0 up.
func ServerConf(address string, port int, privateKey, nftFile string, forward bool, peers []DesiredPeer) string {
	var b strings.Builder
	b.WriteString("# ctrl32 telemetry — generated by the server agent; edit in the UI under System → VPN\n[Interface]\n")
	b.WriteString("Address = " + address + "\nListenPort = " + strconv.Itoa(port) + "\nPrivateKey = " + privateKey + "\n")
	if nftFile != "" {
		fw := "0"
		if forward {
			fw = "1"
		}
		b.WriteString("PreUp = nft -f " + nftFile + "\n")
		b.WriteString("PostUp = sysctl -q -w net.ipv4.conf.%i.forwarding=" + fw + "\n")
		b.WriteString("PostDown = nft delete table inet " + NftTable + "\n")
	}
	for _, p := range peers {
		if !p.Enabled {
			continue
		}
		b.WriteString("\n# " + strings.ReplaceAll(p.Name, "\n", " ") + "\n[Peer]\nPublicKey = " + p.PublicKey + "\n")
		if p.PresharedKey != "" {
			b.WriteString("PresharedKey = " + p.PresharedKey + "\n")
		}
		b.WriteString("AllowedIPs = " + strings.Join(p.AllowedIPs(), ", ") + "\n")
	}
	return b.String()
}

// Client holds what a peer's configuration needs.
type Client struct {
	Name            string
	Kind            string
	PrivateKey      string
	Address         string // host address
	VPNNet          string // e.g. 10.66.0.0/24
	VPNBits         int
	DNS             string
	ServerPublicKey string
	PresharedKey    string
	Endpoint        string // host:port
	AllowedIPs      []string
	Subnets         []string // a site's own routed subnets (router templates)
}

// ClientConf renders the wg-quick configuration of a peer (shown once; the private key is not stored).
func ClientConf(c Client) string {
	var b strings.Builder
	b.WriteString("[Interface]\nPrivateKey = " + c.PrivateKey + "\nAddress = " + c.Address + "/32\n")
	if c.DNS != "" {
		b.WriteString("DNS = " + c.DNS + "\n")
	}
	b.WriteString("\n[Peer]\nPublicKey = " + c.ServerPublicKey + "\n")
	if c.PresharedKey != "" {
		b.WriteString("PresharedKey = " + c.PresharedKey + "\n")
	}
	b.WriteString("AllowedIPs = " + strings.Join(c.AllowedIPs, ", ") + "\nPersistentKeepalive = 25\n")
	if c.Endpoint != "" {
		b.WriteString("Endpoint = " + c.Endpoint + "\n")
	}
	return b.String()
}

// RouterTemplates are configuration texts for a site router (check before use; text only). The site's LAN devices
// must reach the VPN through the router: when the router is their default gateway nothing else is needed, otherwise
// (a Raspberry Pi beside the gateway) the generic template masquerades VPN traffic into the LAN.
func RouterTemplates(c Client) map[string]string {
	host, port := c.Endpoint, "51820"
	if h, p, err := net.SplitHostPort(c.Endpoint); err == nil {
		host, port = h, p
	}
	lan := strings.Join(c.Subnets, ", ")
	others := []string{}
	for _, a := range c.AllowedIPs {
		if a != c.VPNNet {
			others = append(others, a)
		}
	}
	out := map[string]string{}

	// generic Linux / Raspberry Pi with wg-quick
	var g strings.Builder
	g.WriteString("# " + c.Name + " — site router with wg-quick (Linux, Raspberry Pi). Routed LAN: " + lan + "\n")
	g.WriteString("# 1) apt install wireguard-tools   2) save as /etc/wireguard/ctrl32.conf (chmod 600)\n")
	g.WriteString("# 3) echo net.ipv4.ip_forward=1 > /etc/sysctl.d/90-ctrl32-vpn.conf && sysctl --system\n")
	g.WriteString("# 4) systemctl enable --now wg-quick@ctrl32\n")
	g.WriteString("# If this machine is NOT the default gateway of the LAN, uncomment the two lines with MASQUERADE\n")
	g.WriteString("# (replace eth0 with the LAN interface) so LAN devices answer through it.\n")
	g.WriteString(strings.Replace(ClientConf(c), "\n\n[Peer]", "\n# PostUp = iptables -t nat -A POSTROUTING -s "+c.VPNNet+" -o eth0 -j MASQUERADE\n# PostDown = iptables -t nat -D POSTROUTING -s "+c.VPNNet+" -o eth0 -j MASQUERADE\n\n[Peer]", 1))
	out["wg_quick"] = g.String()

	// OpenWrt (uci)
	var o strings.Builder
	o.WriteString("# " + c.Name + " — OpenWrt 22.03 or newer (packages: wireguard-tools, luci-proto-wireguard). Routed LAN: " + lan + "\n")
	o.WriteString("uci set network.ctrl32vpn=interface\nuci set network.ctrl32vpn.proto='wireguard'\n")
	o.WriteString("uci set network.ctrl32vpn.private_key='" + c.PrivateKey + "'\n")
	o.WriteString("uci add_list network.ctrl32vpn.addresses='" + c.Address + "/32'\n")
	o.WriteString("uci set network.ctrl32hub=wireguard_ctrl32vpn\n")
	o.WriteString("uci set network.ctrl32hub.description='ctrl32 telemetry'\n")
	o.WriteString("uci set network.ctrl32hub.public_key='" + c.ServerPublicKey + "'\n")
	if c.PresharedKey != "" {
		o.WriteString("uci set network.ctrl32hub.preshared_key='" + c.PresharedKey + "'\n")
	}
	o.WriteString("uci set network.ctrl32hub.endpoint_host='" + host + "'\nuci set network.ctrl32hub.endpoint_port='" + port + "'\n")
	o.WriteString("uci set network.ctrl32hub.persistent_keepalive='25'\nuci set network.ctrl32hub.route_allowed_ips='1'\n")
	for _, a := range c.AllowedIPs {
		o.WriteString("uci add_list network.ctrl32hub.allowed_ips='" + a + "'\n")
	}
	o.WriteString("uci commit network\n")
	o.WriteString("# firewall: a zone for the tunnel that may forward into the LAN (the server decides who may)\n")
	o.WriteString("uci set firewall.ctrl32vpn=zone\nuci set firewall.ctrl32vpn.name='ctrl32vpn'\nuci add_list firewall.ctrl32vpn.network='ctrl32vpn'\n")
	o.WriteString("uci set firewall.ctrl32vpn.input='REJECT'\nuci set firewall.ctrl32vpn.output='ACCEPT'\nuci set firewall.ctrl32vpn.forward='REJECT'\n")
	o.WriteString("uci set firewall.ctrl32vpn_lan=forwarding\nuci set firewall.ctrl32vpn_lan.src='ctrl32vpn'\nuci set firewall.ctrl32vpn_lan.dest='lan'\n")
	o.WriteString("uci set firewall.lan_ctrl32vpn=forwarding\nuci set firewall.lan_ctrl32vpn.src='lan'\nuci set firewall.lan_ctrl32vpn.dest='ctrl32vpn'\n")
	o.WriteString("uci commit firewall\n/etc/init.d/network reload\n/etc/init.d/firewall reload\n")
	out["openwrt"] = o.String()

	// MikroTik RouterOS 7
	var m strings.Builder
	m.WriteString("# " + c.Name + " — MikroTik RouterOS 7 (terminal). Routed LAN: " + lan + "\n")
	m.WriteString("/interface wireguard add name=ctrl32-vpn private-key=\"" + c.PrivateKey + "\" comment=\"ctrl32 telemetry\"\n")
	m.WriteString("/interface wireguard peers add interface=ctrl32-vpn public-key=\"" + c.ServerPublicKey + "\"")
	if c.PresharedKey != "" {
		m.WriteString(" preshared-key=\"" + c.PresharedKey + "\"")
	}
	m.WriteString(" endpoint-address=" + host + " endpoint-port=" + port + " allowed-address=" + strings.Join(c.AllowedIPs, ",") + " persistent-keepalive=25s\n")
	m.WriteString("/ip address add address=" + c.Address + "/" + strconv.Itoa(c.VPNBits) + " interface=ctrl32-vpn\n")
	for _, a := range others {
		m.WriteString("/ip route add dst-address=" + a + " gateway=ctrl32-vpn\n")
	}
	m.WriteString("# allow the tunnel into the LAN (the server's access rules decide who may), above any drop rule:\n")
	m.WriteString("/ip firewall filter add chain=forward in-interface=ctrl32-vpn action=accept comment=\"ctrl32 VPN to LAN\" place-before=0\n")
	out["mikrotik"] = m.String()

	out["rutos"] = rutosTemplate(c, host, port, lan)
	out["edgerouter"] = edgeRouterTemplate(c, host, port, lan)
	out["pfsense"] = pfTemplate(c, host, port, lan, others, "pfSense")
	out["opnsense"] = pfTemplate(c, host, port, lan, others, "OPNsense")
	return out
}

// RouterTemplateKeys lists the keys RouterTemplates returns, in display order.
var RouterTemplateKeys = []string{"wg_quick", "openwrt", "mikrotik", "rutos", "edgerouter", "pfsense", "opnsense"}

// ClientTemplateKeys lists the keys ClientTemplates returns, in display order.
var ClientTemplateKeys = []string{"windows", "mobile"}

// rutosTemplate: Teltonika RUT routers with RutOS 7.x (uci, the same model as OpenWrt).
func rutosTemplate(c Client, host, port, lan string) string {
	var b strings.Builder
	b.WriteString("# " + c.Name + " — Teltonika RUT with RutOS 7.x (SSH or System → Maintenance → CLI). Routed LAN: " + lan + "\n")
	b.WriteString("# The same settings in the WebUI: Services → VPN → WireGuard → add instance \"ctrl32vpn\"\n")
	b.WriteString("#   General: private key, IP address " + c.Address + "/32; Peers: add \"ctrl32hub\" with the values below,\n")
	b.WriteString("#   enable \"Route allowed IPs\"; then Network → Firewall: allow forwarding from the zone of the tunnel to lan.\n")
	b.WriteString("uci set network.ctrl32vpn=interface\nuci set network.ctrl32vpn.proto='wireguard'\n")
	b.WriteString("uci set network.ctrl32vpn.private_key='" + c.PrivateKey + "'\n")
	b.WriteString("uci add_list network.ctrl32vpn.addresses='" + c.Address + "/32'\n")
	b.WriteString("uci set network.ctrl32hub=wireguard_ctrl32vpn\n")
	b.WriteString("uci set network.ctrl32hub.description='ctrl32 telemetry'\n")
	b.WriteString("uci set network.ctrl32hub.public_key='" + c.ServerPublicKey + "'\n")
	if c.PresharedKey != "" {
		b.WriteString("uci set network.ctrl32hub.preshared_key='" + c.PresharedKey + "'\n")
	}
	b.WriteString("uci set network.ctrl32hub.endpoint_host='" + host + "'\nuci set network.ctrl32hub.endpoint_port='" + port + "'\n")
	b.WriteString("uci set network.ctrl32hub.persistent_keepalive='25'\nuci set network.ctrl32hub.route_allowed_ips='1'\n")
	for _, a := range c.AllowedIPs {
		b.WriteString("uci add_list network.ctrl32hub.allowed_ips='" + a + "'\n")
	}
	b.WriteString("uci commit network\n")
	b.WriteString("# firewall: a zone for the tunnel that may forward into the LAN (the server decides who may)\n")
	b.WriteString("uci set firewall.ctrl32vpn=zone\nuci set firewall.ctrl32vpn.name='ctrl32vpn'\nuci add_list firewall.ctrl32vpn.network='ctrl32vpn'\n")
	b.WriteString("uci set firewall.ctrl32vpn.input='REJECT'\nuci set firewall.ctrl32vpn.output='ACCEPT'\nuci set firewall.ctrl32vpn.forward='REJECT'\n")
	b.WriteString("uci set firewall.ctrl32vpn_lan=forwarding\nuci set firewall.ctrl32vpn_lan.src='ctrl32vpn'\nuci set firewall.ctrl32vpn_lan.dest='lan'\n")
	b.WriteString("uci set firewall.lan_ctrl32vpn=forwarding\nuci set firewall.lan_ctrl32vpn.src='lan'\nuci set firewall.lan_ctrl32vpn.dest='ctrl32vpn'\n")
	b.WriteString("uci commit firewall\n/etc/init.d/network reload\n/etc/init.d/firewall reload\n")
	return b.String()
}

// edgeRouterTemplate: Ubiquiti EdgeRouter (EdgeOS) with the WireGuard package (wireguard-vyatta-ubnt) installed.
func edgeRouterTemplate(c Client, host, port, lan string) string {
	var b strings.Builder
	q := func(s string) string { return "'" + s + "'" }
	b.WriteString("# " + c.Name + " — Ubiquiti EdgeRouter (EdgeOS 2.x) with the WireGuard package installed. Routed LAN: " + lan + "\n")
	b.WriteString("# Paste in the CLI (ssh). If wg0 is already used on this router, replace it with a free name (wg1, …).\n")
	b.WriteString("configure\n")
	b.WriteString("set interfaces wireguard wg0 description 'ctrl32 telemetry'\n")
	b.WriteString("set interfaces wireguard wg0 address " + c.Address + "/32\n")
	b.WriteString("set interfaces wireguard wg0 private-key " + q(c.PrivateKey) + "\n")
	b.WriteString("set interfaces wireguard wg0 route-allowed-ip true\n")
	peer := "set interfaces wireguard wg0 peer " + q(c.ServerPublicKey) + " "
	b.WriteString(peer + "endpoint " + host + ":" + port + "\n")
	for _, a := range c.AllowedIPs {
		b.WriteString(peer + "allowed-ips " + a + "\n")
	}
	if c.PresharedKey != "" {
		b.WriteString(peer + "preshared-key " + q(c.PresharedKey) + "\n")
	}
	b.WriteString(peer + "persistent-keepalive 25\n")
	b.WriteString("# firewall: the tunnel only connects outwards, no WAN rule is needed. If a firewall ruleset is attached to\n")
	b.WriteString("# the LAN interface (out) or to wg0 (in), allow traffic from wg0 into the LAN; the server's access rules decide who may.\n")
	b.WriteString("commit\nsave\nexit\n")
	return b.String()
}

// pfTemplate: step-by-step text for the web GUI of pfSense (WireGuard package) and OPNsense (built in).
func pfTemplate(c Client, host, port, lan string, others []string, product string) string {
	var b strings.Builder
	menu, peerMenu := "VPN → WireGuard → Tunnels → Add", "VPN → WireGuard → Peers → Add"
	if product == "OPNsense" {
		menu, peerMenu = "VPN → WireGuard → Instances → Add", "VPN → WireGuard → Peers → Add"
	}
	b.WriteString("# " + c.Name + " — " + product + " (web interface). Routed LAN: " + lan + "\n")
	if product == "pfSense" {
		b.WriteString("0. System → Package Manager → Available Packages: install \"WireGuard\".\n")
	}
	b.WriteString("1. " + menu + "\n")
	b.WriteString("   Description: ctrl32 telemetry, Listen port: leave empty or any free port\n")
	b.WriteString("   Private key: " + c.PrivateKey + "\n")
	b.WriteString("   Address: " + c.Address + "/32\n")
	b.WriteString("2. " + peerMenu + "\n")
	b.WriteString("   Public key: " + c.ServerPublicKey + "\n")
	if c.PresharedKey != "" {
		b.WriteString("   Pre-shared key: " + c.PresharedKey + "\n")
	}
	b.WriteString("   Endpoint: " + host + ", port " + port + ", Keep alive: 25\n")
	b.WriteString("   Allowed IPs: " + strings.Join(c.AllowedIPs, ", ") + "\n")
	b.WriteString("   Enable WireGuard and apply.\n")
	b.WriteString("3. Interfaces → Assignments: add the WireGuard interface (e.g. WG_CTRL32), enable it, IPv4 none / static " + c.Address + "/32.\n")
	b.WriteString("4. System → Routing → Gateways: add a gateway on WG_CTRL32 with the address " + c.Address + " (mark it far gateway if asked).\n")
	b.WriteString("   System → Routing → Static Routes: route " + c.VPNNet + " through that gateway")
	if len(others) > 0 {
		b.WriteString(", and also " + strings.Join(others, ", "))
	}
	b.WriteString(".\n")
	b.WriteString("5. Firewall → Rules → WG_CTRL32 (and the WireGuard group): pass traffic from " + c.VPNNet + " to " + lan + ";\n")
	b.WriteString("   the server's access rules decide which peers may connect and to which ports.\n")
	b.WriteString("6. Outbound NAT is not needed when this firewall is the default gateway of the LAN. Only if it is not,\n")
	b.WriteString("   add an outbound NAT rule on the LAN interface for source " + c.VPNNet + ".\n")
	return b.String()
}

// ClientTemplates are texts for a peer's own device (every kind): a .conf for the official WireGuard app for Windows
// and instructions for the mobile apps (the QR code is shown next to it).
func ClientTemplates(c Client) map[string]string {
	var w strings.Builder
	w.WriteString("# " + c.Name + " — WireGuard for Windows (official app from wireguard.com)\n")
	w.WriteString("# Save as ctrl32-vpn.conf, then in the app: Import tunnel(s) from file → select it → Activate.\n")
	w.WriteString("# Keep this file private: it contains the private key. Delete it after importing.\n")
	w.WriteString(ClientConf(c))
	var m strings.Builder
	m.WriteString("# " + c.Name + " — Android and iPhone/iPad (official WireGuard app)\n")
	m.WriteString("1. Install WireGuard from Google Play or the App Store.\n")
	m.WriteString("2. Tap + → Scan from QR code, and scan the QR code shown on this page.\n")
	m.WriteString("   Or: copy the configuration file to the phone and tap + → Import from file or archive.\n")
	m.WriteString("3. Name the tunnel (e.g. ctrl32) and switch it on. The address in the VPN is " + c.Address + ".\n")
	return map[string]string{"windows": w.String(), "mobile": m.String()}
}

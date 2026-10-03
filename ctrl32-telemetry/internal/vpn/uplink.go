package vpn

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Uplink (VPN phase 3, ADR-0021, spec 027). A customer server inside a LAN without a public address connects out to a
// hub: another telemetry server with a public address (e.g. a cloud server). On the hub the customer server is a site
// peer marked as an uplink: its VPN address and the LAN subnets behind it are routed like any site, and the hub's
// access rules decide which technicians may reach them. The hub issues the configuration once as an uplink token;
// the customer server pastes it, brings up a second WireGuard interface (wg1, outbound only, keepalive 25 s) and
// applies its own default-deny firewall to everything that arrives from the hub: its uplink access rules decide what
// the hub's range may reach — this server's uplink address (input) or hosts in its LAN (forwarding with masquerade).
// Access therefore needs a rule on both sides.

// UplinkIface is the WireGuard interface of the uplink on the customer server; UplinkNftTable its firewall table.
const (
	UplinkIface    = "wg1"
	UplinkNftTable = "ctrl32_uplink"
)

// Uplink transports: plain WireGuard over UDP, or WireGuard datagrams relayed over a WebSocket through the hub's HTTPS
// port (TCP 443) when UDP is blocked.
const (
	TransportUDP   = "udp"
	TransportHTTPS = "https"
)

// UplinkTunnelPath is the hub's WebSocket endpoint of the HTTPS transport.
const UplinkTunnelPath = "/api/v1/vpn/uplink/tunnel"

// uplinkTokenPrefix starts every uplink token; the version is part of it.
const uplinkTokenPrefix = "ctrl32-uplink-v1."

// MaxUplinkRules limits the customer side's rules.
const MaxUplinkRules = 200

// UplinkToken is everything the customer server needs to connect to the hub. It holds the private key: shown once on
// the hub (after the password again), never stored there.
type UplinkToken struct {
	Name         string   `json:"name"`     // the uplink peer's name on the hub
	Endpoint     string   `json:"endpoint"` // hub host:port (UDP)
	HubPublicKey string   `json:"hub_public_key"`
	PresharedKey string   `json:"preshared_key"`
	PrivateKey   string   `json:"private_key"`
	Address      string   `json:"address"`     // this server's address in the hub's VPN, without prefix
	VPNNet       string   `json:"vpn_net"`     // the hub's VPN range, e.g. 10.99.0.0/16
	HubAddress   string   `json:"hub_address"` // the hub's own VPN address
	Subnets      []string `json:"subnets"`     // the LAN subnets the hub routes to this server (may be empty)
	TunnelURL    string   `json:"tunnel_url,omitempty"`
	TunnelKey    string   `json:"tunnel_key,omitempty"` // bearer secret of the HTTPS transport; the hub keeps its hash only
}

// EncodeUplinkToken renders the token text (prefix + base64url JSON).
func EncodeUplinkToken(t UplinkToken) string {
	b, _ := json.Marshal(t)
	return uplinkTokenPrefix + base64.RawURLEncoding.EncodeToString(b)
}

var (
	hostRe      = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
	tunnelKeyRe = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
)

// ValidEndpoint checks host:port of a hub (a DNS name or an IP address; the port 1 to 65535).
func ValidEndpoint(s string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(s))
	if err != nil || host == "" {
		return errors.New("endpoint must be host:port")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return errors.New("endpoint port must be 1 to 65535")
	}
	if _, err := netip.ParseAddr(host); err != nil && !hostRe.MatchString(host) {
		return errors.New("endpoint host must be a DNS name or an IP address")
	}
	return nil
}

// ValidTunnelURL checks the hub's WebSocket URL: https, or http only towards the loopback (tests and development on one
// machine): a loopback IP address or the name "localhost", which resolves only to the loopback (RFC 6761).
func ValidTunnelURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("tunnel URL must be https://host/path")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !isLoopbackHost(u.Hostname()) {
			return errors.New("tunnel URL must use https")
		}
	default:
		return errors.New("tunnel URL must use https")
	}
	return nil
}

// isLoopbackHost reports whether host is a loopback IP address or exactly "localhost" (any case, optional trailing dot).
func isLoopbackHost(host string) bool {
	if strings.EqualFold(strings.TrimSuffix(host, "."), "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

// ParseUplinkToken decodes and checks a token pasted on the customer server. Nothing in it is trusted.
func ParseUplinkToken(s string) (UplinkToken, error) {
	var t UplinkToken
	s = strings.Join(strings.Fields(s), "") // pasted text may be wrapped
	if len(s) > 8192 {
		return t, errors.New("the uplink configuration is too long")
	}
	body, ok := strings.CutPrefix(s, uplinkTokenPrefix)
	if !ok {
		return t, errors.New("not an uplink configuration (it starts with " + uplinkTokenPrefix + ")")
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return t, errors.New("the uplink configuration is damaged (copy it again)")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		return t, errors.New("the uplink configuration is damaged (copy it again)")
	}
	return t, t.Validate()
}

// Validate checks every field of a token.
func (t *UplinkToken) Validate() error {
	if err := ValidName(t.Name); err != nil {
		return err
	}
	if err := ValidEndpoint(t.Endpoint); err != nil {
		return err
	}
	if !ValidKey(t.HubPublicKey) || !ValidKey(t.PrivateKey) || (t.PresharedKey != "" && !ValidKey(t.PresharedKey)) {
		return errors.New("invalid key in the uplink configuration")
	}
	if pub, err := PublicKey(t.PrivateKey); err != nil || pub == t.HubPublicKey {
		return errors.New("invalid key in the uplink configuration")
	}
	vpnNet, err := netip.ParsePrefix(t.VPNNet)
	if err != nil || !vpnNet.Addr().Is4() || vpnNet.Masked() != vpnNet || vpnNet.Bits() < 8 || vpnNet.Bits() > 30 {
		return errors.New("the hub's VPN range must be an IPv4 network from /8 to /30")
	}
	if reserved(vpnNet) {
		return errors.New("the hub's VPN range is a reserved range")
	}
	addr, err1 := netip.ParseAddr(t.Address)
	hub, err2 := netip.ParseAddr(t.HubAddress)
	if err1 != nil || err2 != nil || !vpnNet.Contains(addr) || !vpnNet.Contains(hub) || addr == hub || addr == vpnNet.Addr() || hub == vpnNet.Addr() {
		return errors.New("the addresses must be host addresses inside the hub's VPN range")
	}
	subs, err := ParseSubnets(t.Subnets, vpnNet)
	if err != nil {
		return err
	}
	t.Subnets = prefixStrings(subs)
	if (t.TunnelURL == "") != (t.TunnelKey == "") {
		return errors.New("the HTTPS transport needs both the tunnel URL and its key")
	}
	if t.TunnelURL != "" {
		if err := ValidTunnelURL(t.TunnelURL); err != nil {
			return err
		}
		if !tunnelKeyRe.MatchString(t.TunnelKey) {
			return errors.New("invalid tunnel key")
		}
	}
	return nil
}

// Prefix is this server's address with the hub's VPN prefix (e.g. 10.99.0.7/16).
func (t UplinkToken) Prefix() netip.Prefix {
	a, _ := netip.ParseAddr(t.Address)
	n, _ := netip.ParsePrefix(t.VPNNet)
	return netip.PrefixFrom(a, n.Bits())
}

// GenerateTunnelKey returns a new bearer secret of the HTTPS transport and its hash (stored on the hub).
func GenerateTunnelKey() (key, hash string, err error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", err
	}
	key = base64.RawURLEncoding.EncodeToString(b[:])
	return key, HashTunnelKey(key), nil
}

// HashTunnelKey is the stored form of a tunnel key (SHA-256, hex).
func HashTunnelKey(key string) string {
	s := sha256.Sum256([]byte(key))
	return hex.EncodeToString(s[:])
}

// CheckUplinkLocal refuses a hub range that would collide with this server's own networks: the host's interfaces, its
// own VPN range and the routed subnets of its own sites. Routing such a range into wg1 would cut the server off or
// hijack its own peers.
func CheckUplinkLocal(t UplinkToken, localNets []netip.Prefix, ownVPN netip.Prefix, ownSubnets []netip.Prefix) error {
	vpnNet, _ := netip.ParsePrefix(t.VPNNet)
	for _, l := range localNets {
		if vpnNet.Overlaps(l) {
			return fmt.Errorf("the hub's VPN range %s overlaps the host network %s; ask the hub's administrator for another range", vpnNet, l)
		}
	}
	if ownVPN.IsValid() && vpnNet.Overlaps(ownVPN.Masked()) {
		return fmt.Errorf("the hub's VPN range %s overlaps this server's own VPN range %s; one of them must change", vpnNet, ownVPN.Masked())
	}
	for _, s := range t.Subnets {
		if p, err := netip.ParsePrefix(s); err == nil && ownVPN.IsValid() && p.Overlaps(ownVPN.Masked()) {
			return fmt.Errorf("the offered network %s overlaps this server's own VPN range %s", p, ownVPN.Masked())
		}
	}
	for _, s := range ownSubnets {
		if vpnNet.Overlaps(s) {
			return fmt.Errorf("the hub's VPN range %s overlaps the routed subnet %s of a site of this server", vpnNet, s)
		}
	}
	return nil
}

// ---- customer side: uplink access rules --------------------------------------------------------------------------

// Uplink rule destinations.
const (
	UplinkDstServer = "server" // this server's address in the hub's VPN (input)
	UplinkDstLAN    = "lan"    // a host or subnet of this server's LAN (forwarding with masquerade)
)

// UplinkRule is an access rule of the customer server for traffic arriving from the hub: from (the hub's VPN range,
// or an address or subnet inside it, e.g. one technician) → this server or a LAN host : protocol/ports.
type UplinkRule struct {
	ID       string
	Name     string
	SrcCIDR  string // "" = the hub's whole VPN range
	DestType string // server | lan
	DestCIDR string // lan: host or subnet inside the offered subnets ("" = all of them)
	Protocol string
	Ports    string
	Enabled  bool
}

// UplinkInfo is what the rules are checked and compiled against: the connected uplink.
type UplinkInfo struct {
	Address netip.Addr
	VPNNet  netip.Prefix
	Subnets []netip.Prefix
}

// Connected reports whether an uplink is known.
func (u UplinkInfo) Connected() bool { return u.Address.IsValid() && u.VPNNet.IsValid() }

func parseCIDR(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "/") {
		s += "/32"
	}
	p, err := netip.ParsePrefix(s)
	if err != nil || !p.Addr().Is4() {
		return netip.Prefix{}, fmt.Errorf("%s: not an IPv4 address or subnet", s)
	}
	if p.Masked() != p {
		return netip.Prefix{}, fmt.Errorf("%s is not a network address; did you mean %s?", s, p.Masked())
	}
	return p, nil
}

func inside(p netip.Prefix, nets []netip.Prefix) bool {
	for _, q := range nets {
		if q.Bits() <= p.Bits() && q.Contains(p.Addr()) {
			return true
		}
	}
	return false
}

// ValidateUplinkRule checks a rule and makes its fields canonical. With a connected uplink the source must lie in the
// hub's range and a LAN destination inside the offered subnets; without one only the syntax is checked (the rule is
// kept and applied when an uplink is connected; out-of-range parts are then left out).
func ValidateUplinkRule(r *UplinkRule, u UplinkInfo) error {
	if err := ValidRuleName(r.Name); err != nil {
		return err
	}
	if strings.TrimSpace(r.SrcCIDR) != "" {
		p, err := parseCIDR(r.SrcCIDR)
		if err != nil {
			return fmt.Errorf("source: %w", err)
		}
		if u.Connected() && !inside(p, []netip.Prefix{u.VPNNet}) {
			return fmt.Errorf("source %s is not inside the hub's VPN range %s", p, u.VPNNet)
		}
		r.SrcCIDR = p.String()
	} else {
		r.SrcCIDR = ""
	}
	switch r.DestType {
	case UplinkDstServer:
		r.DestCIDR = ""
	case UplinkDstLAN:
		if strings.TrimSpace(r.DestCIDR) != "" {
			p, err := ParseSubnet(r.DestCIDR)
			if err != nil {
				return fmt.Errorf("destination: %w", err)
			}
			if u.Connected() && !inside(p, u.Subnets) {
				return fmt.Errorf("destination %s is not inside the networks this server offers to the hub", p)
			}
			r.DestCIDR = p.String()
		} else {
			r.DestCIDR = ""
			if u.Connected() && len(u.Subnets) == 0 {
				return errors.New("this server offers no networks to the hub (the uplink has no routed subnets)")
			}
		}
	default:
		return errors.New("destination type must be server or lan")
	}
	switch r.Protocol {
	case "tcp", "udp":
		ps, err := ParsePorts(r.Ports)
		if err != nil {
			return err
		}
		r.Ports = FormatPorts(ps)
	case "icmp", "any":
		if strings.TrimSpace(r.Ports) != "" {
			return errors.New("ports apply to tcp and udp only")
		}
		r.Ports = ""
	default:
		return errors.New("protocol must be tcp, udp, icmp or any")
	}
	return nil
}

// CompileUplink turns the customer's uplink rules into firewall rules for the uplink table: towards this server the
// input chain, towards the LAN the forward chain. Disabled rules and parts outside the connected uplink's ranges are
// left out. Forward reports whether any rule needs forwarding.
func CompileUplink(u UplinkInfo, rules []UplinkRule) (out []FwRule, forward bool) {
	out = []FwRule{}
	if !u.Connected() {
		return out, false
	}
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		src := u.VPNNet
		if r.SrcCIDR != "" {
			p, err := parseCIDR(r.SrcCIDR)
			if err != nil || !inside(p, []netip.Prefix{u.VPNNet}) {
				continue
			}
			src = p
		}
		fr := FwRule{ID: r.ID, Src: []string{src.String()}, Proto: r.Protocol}
		switch r.DestType {
		case UplinkDstServer:
			fr.Chain, fr.Dst = ChainInput, []string{netip.PrefixFrom(u.Address, 32).String()}
		case UplinkDstLAN:
			fr.Chain = ChainForward
			if r.DestCIDR != "" {
				p, err := ParseSubnet(r.DestCIDR)
				if err != nil || !inside(p, u.Subnets) {
					continue
				}
				fr.Dst = []string{p.String()}
			} else {
				fr.Dst = prefixStrings(u.Subnets)
			}
		}
		if len(fr.Dst) == 0 {
			continue
		}
		if r.Protocol == "tcp" || r.Protocol == "udp" {
			if ps, err := ParsePorts(r.Ports); err == nil {
				for _, pr := range ps {
					fr.Ports = append(fr.Ports, pr.String())
				}
			}
		}
		out = append(out, fr)
		if fr.Chain == ChainForward {
			forward = true
		}
	}
	return out, forward
}

// ValidateUplinkRules is the agent's own check of compiled uplink rules: sources inside the hub's range, input only to
// this server's uplink address, forwarding only into the offered subnets.
func ValidateUplinkRules(rules []FwRule, u UplinkInfo) error {
	if len(rules) > MaxUplinkRules {
		return errors.New("too many uplink rules")
	}
	self := netip.PrefixFrom(u.Address, 32)
	for _, r := range rules {
		if !idRe.MatchString(r.ID) {
			return fmt.Errorf("rule id %q invalid", r.ID)
		}
		if len(r.Src) == 0 || len(r.Dst) == 0 {
			return fmt.Errorf("rule %s: empty source or destination", r.ID)
		}
		for _, s := range r.Src {
			p, err := parseCIDR(s)
			if err != nil || !inside(p, []netip.Prefix{u.VPNNet}) {
				return fmt.Errorf("rule %s: source %q is not inside the hub's VPN range", r.ID, s)
			}
		}
		for _, s := range r.Dst {
			p, err := parseCIDR(s)
			switch {
			case err != nil:
				return fmt.Errorf("rule %s: destination %q invalid", r.ID, s)
			case r.Chain == ChainInput && p != self:
				return fmt.Errorf("rule %s: destination %q is not this server's uplink address", r.ID, s)
			case r.Chain == ChainForward && !inside(p, u.Subnets):
				return fmt.Errorf("rule %s: destination %q is not inside the offered networks", r.ID, s)
			case r.Chain != ChainInput && r.Chain != ChainForward:
				return fmt.Errorf("rule %s: chain must be input or forward", r.ID)
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

// ---- customer side: rendering (Linux) ----------------------------------------------------------------------------

var ifaceRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,15}$`)

// ValidIface checks a network interface name before it goes into a ruleset.
func ValidIface(s string) bool { return ifaceRe.MatchString(s) }

// RenderUplinkNft renders the uplink's own nftables table (replaced atomically like ctrl32_vpn). Everything that
// arrives from the hub on wg1 is dropped unless a rule accepts it; replies pass as established. Traffic that does not
// come from or go to wg1 is not judged — except on the LAN interfaces in lanGuard: the agent switched forwarding on
// there only so replies from the LAN can return to the hub, so anything else forwarded from them is dropped (the host
// does not become a router for its LAN). Forwarded traffic towards the LAN is masqueraded behind the server's LAN
// address: LAN devices see the server, need no route back and are not otherwise exposed.
func RenderUplinkNft(address netip.Addr, rules []FwRule, lanGuard []string) string {
	var b strings.Builder
	q := func(s string) string { return `"` + s + `"` }
	b.WriteString("# ctrl32 telemetry — uplink access rules (traffic from the hub), generated by the server agent. Do not edit.\n")
	b.WriteString("# Default deny for everything that arrives from the uplink interface " + UplinkIface + ".\n")
	b.WriteString("table inet " + UplinkNftTable + "\ndelete table inet " + UplinkNftTable + "\n")
	b.WriteString("table inet " + UplinkNftTable + " {\n")
	b.WriteString("\tchain input {\n\t\ttype filter hook input priority filter - 10; policy accept;\n")
	b.WriteString("\t\tiifname != " + q(UplinkIface) + " accept\n\t\tct state established,related accept\n\t\tct state invalid drop\n")
	b.WriteString("\t\tip daddr " + address.String() + " icmp type echo-request accept comment \"ping the server\"\n")
	for _, r := range rules {
		if r.Chain == ChainInput {
			b.WriteString("\t\t" + uplinkNftRule(r) + "\n")
		}
	}
	b.WriteString("\t\tcounter drop comment \"default deny\"\n\t}\n")
	b.WriteString("\tchain forward {\n\t\ttype filter hook forward priority filter - 10; policy accept;\n")
	guard := []string{}
	for _, g := range lanGuard {
		if ValidIface(g) && g != UplinkIface {
			guard = append(guard, q(g))
		}
	}
	sort.Strings(guard)
	if len(guard) > 0 {
		b.WriteString("\t\tiifname " + nftSet(guard) + " oifname != " + q(UplinkIface) + " counter drop comment \"forwarding on the LAN only for replies to the hub\"\n")
	}
	b.WriteString("\t\tiifname != " + q(UplinkIface) + " oifname != " + q(UplinkIface) + " accept\n")
	b.WriteString("\t\tct state established,related accept\n\t\tct state invalid drop\n")
	for _, r := range rules {
		if r.Chain == ChainForward {
			b.WriteString("\t\t" + uplinkNftRule(r) + "\n")
		}
	}
	b.WriteString("\t\tcounter drop comment \"default deny\"\n\t}\n")
	b.WriteString("\tchain postrouting {\n\t\ttype nat hook postrouting priority srcnat; policy accept;\n")
	b.WriteString("\t\tiifname " + q(UplinkIface) + " oifname != " + q(UplinkIface) + " masquerade comment \"LAN devices answer the server\"\n\t}\n")
	b.WriteString("}\n")
	return b.String()
}

func uplinkNftRule(r FwRule) string {
	parts := []string{}
	if r.Chain == ChainForward {
		parts = append(parts, `iifname "`+UplinkIface+`" oifname != "`+UplinkIface+`"`)
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

// UplinkConf renders /etc/wireguard/wg1.conf: no listen port (outbound only), one peer (the hub) with the hub's VPN
// range as AllowedIPs (wg-quick routes it into wg1) and a keepalive of 25 s so NAT keeps the path open. The table is
// loaded before the interface exists (PreUp) and removed with it; if loading fails the interface does not come up.
// lanForward are the LAN interfaces on which the agent switched forwarding on for replies (lanGuard of the table):
// switched on again after a reboot with the interface, and off when it goes down.
func UplinkConf(t UplinkToken, endpoint, nftFile string, forward bool, lanForward []string) string {
	var b strings.Builder
	b.WriteString(UplinkConfHeader + "\n[Interface]\n")
	b.WriteString("Address = " + t.Address + "/32\nPrivateKey = " + t.PrivateKey + "\n")
	if nftFile != "" {
		fw := "0"
		if forward {
			fw = "1"
		}
		b.WriteString("PreUp = nft -f " + nftFile + "\n")
		b.WriteString("PostUp = sysctl -q -w net.ipv4.conf.%i.forwarding=" + fw + "\n")
		for _, l := range lanForward {
			if forward && ValidIface(l) && l != UplinkIface {
				b.WriteString("PostUp = sysctl -q -w net.ipv4.conf." + l + ".forwarding=1\n")
				b.WriteString("PostDown = sysctl -q -w net.ipv4.conf." + l + ".forwarding=0\n")
			}
		}
		b.WriteString("PostDown = nft delete table inet " + UplinkNftTable + "\n")
	}
	b.WriteString("\n# hub " + strings.ReplaceAll(t.Name, "\n", " ") + "\n[Peer]\nPublicKey = " + t.HubPublicKey + "\n")
	if t.PresharedKey != "" {
		b.WriteString("PresharedKey = " + t.PresharedKey + "\n")
	}
	b.WriteString("AllowedIPs = " + t.VPNNet + "\nEndpoint = " + endpoint + "\nPersistentKeepalive = 25\n")
	return b.String()
}

// UplinkConfHeader marks wg1.conf as ours: the agent never overwrites a wg1.conf without it.
const UplinkConfHeader = "# ctrl32 telemetry uplink — generated by the server agent; edit in the UI under System → VPN"

// HubAllowedIPs is what an uplink peer routes into its tunnel on the customer side: the hub's VPN range only (the
// technicians and the hub itself). Other sites of the hub are never routed to a customer server.
func HubAllowedIPs(server netip.Prefix) []string { return []string{server.Masked().String()} }

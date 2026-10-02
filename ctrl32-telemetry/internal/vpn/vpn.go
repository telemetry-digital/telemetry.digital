// Package vpn holds the pure logic of the own WireGuard VPN management (spec 024, ADR-0018): the peer and access
// rule model, validation of addresses, routed subnets, ports and overlaps, the license split, and the translation of
// peers and rules into the state the privileged agent applies (wg0.conf and an nftables table). Nothing here touches
// the host or the database, so every rule is unit-tested.
//
// Model: the server is the hub. A peer is a device (sends data, e.g. MQTT through the tunnel), a user (a technician's
// laptop or phone) or a site (a router or Raspberry Pi that connects the LAN behind it: routed subnets). Access from
// peers is default deny: only access rules "sources → destination : protocol/ports" open anything, towards the server
// itself (input) or towards other peers and site subnets (forwarding through the hub).
package vpn

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Peer kinds.
const (
	KindDevice = "device"
	KindUser   = "user"
	KindSite   = "site"
)

// Rule source and destination types.
const (
	SrcAll  = "all"  // every peer
	SrcKind = "kind" // every peer of a kind
	SrcPeer = "peer" // one peer
	SrcOrg  = "org"  // every peer of an organization

	DstServer = "server" // this server's VPN address (input)
	DstPeer   = "peer"   // a peer's VPN address
	DstSite   = "site"   // a site's routed subnets, or a host/subnet inside them
)

// Firewall chains of the generated table.
const (
	ChainInput   = "input"
	ChainForward = "forward"
)

// FreePeerLimit is how many peers the VPN allows without a licence (device and user peers only).
const FreePeerLimit = 5

// Limits of the model (keep the generated ruleset small and readable).
const (
	MaxSubnets      = 16
	MaxSources      = 20
	MaxPortEntries  = 20
	MaxRules        = 500
	MaxPeers        = 1000
	minSubnetPrefix = 8
)

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,62}$`)

// Peer is a VPN member as stored by the web service.
type Peer struct {
	ID        string
	Name      string
	Kind      string
	OrgID     string
	DeviceID  string
	Address   netip.Addr
	Subnets   []netip.Prefix
	PublicKey string
	Enabled   bool
	ExpiresAt *time.Time
}

// Active reports whether the peer may connect now: enabled and not expired.
func (p Peer) Active(now time.Time) bool {
	return p.Enabled && (p.ExpiresAt == nil || now.Before(*p.ExpiresAt))
}

// Source selects peers in a rule.
type Source struct {
	Type string `json:"type"`           // all | kind | peer | org
	Kind string `json:"kind,omitempty"` // for type kind
	ID   string `json:"id,omitempty"`   // peer or organization id
}

// Rule is an access rule: sources → destination : protocol/ports.
type Rule struct {
	ID         string
	Name       string
	Sources    []Source
	DestType   string // server | peer | site
	DestPeerID string // peer or site
	DestCIDR   string // site only: a host or subnet inside the site's routed subnets ("" = all of them)
	Protocol   string // tcp | udp | icmp | any
	Ports      string // "102, 502, 8000-8100" (tcp/udp; "" = all ports)
	Enabled    bool
}

// ---- validation -------------------------------------------------------------------------------------------------

// ValidName checks a peer name (it goes into config files as a comment and into file names).
func ValidName(s string) error {
	if !nameRe.MatchString(s) {
		return errors.New("name: letters, digits, space, dot, dash or underscore, 1 to 63 characters, starting with a letter or digit")
	}
	return nil
}

// ValidKind checks a peer kind.
func ValidKind(k string) bool { return k == KindDevice || k == KindUser || k == KindSite }

// ServerPrefix parses the server's VPN address with prefix (e.g. 10.66.0.1/24).
func ServerPrefix(s string) (netip.Prefix, error) {
	p, err := netip.ParsePrefix(strings.TrimSpace(s))
	if err != nil || !p.Addr().Is4() {
		return netip.Prefix{}, errors.New("VPN address must be an IPv4 address with prefix, e.g. 10.66.0.1/24")
	}
	if p.Bits() < 8 || p.Bits() > 30 {
		return netip.Prefix{}, errors.New("VPN prefix must be between /8 and /30")
	}
	return p, nil
}

// reserved reports address ranges that must never be routed through the VPN.
func reserved(p netip.Prefix) bool {
	for _, r := range []string{"0.0.0.0/8", "127.0.0.0/8", "169.254.0.0/16", "224.0.0.0/4", "240.0.0.0/4"} {
		if p.Overlaps(netip.MustParsePrefix(r)) {
			return true
		}
	}
	return false
}

// ParseSubnet parses one routed subnet of a site: IPv4, canonical network address, /8 to /32, not reserved.
func ParseSubnet(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Prefix{}, errors.New("empty subnet")
	}
	if !strings.Contains(s, "/") {
		s += "/32"
	}
	p, err := netip.ParsePrefix(s)
	if err != nil || !p.Addr().Is4() {
		return netip.Prefix{}, fmt.Errorf("%s: not an IPv4 subnet (e.g. 192.168.10.0/24)", s)
	}
	if p.Bits() < minSubnetPrefix {
		return netip.Prefix{}, fmt.Errorf("%s: too large; the smallest prefix is /%d (a default route is never accepted)", s, minSubnetPrefix)
	}
	if m := p.Masked(); m != p {
		return netip.Prefix{}, fmt.Errorf("%s is not a network address; did you mean %s?", s, m)
	}
	if reserved(p) {
		return netip.Prefix{}, fmt.Errorf("%s: reserved range (loopback, link-local, multicast) is not allowed", s)
	}
	return p, nil
}

// ParseSubnets parses and checks a list of routed subnets: no duplicates or overlaps among themselves and none with
// the VPN range.
func ParseSubnets(list []string, vpnNet netip.Prefix) ([]netip.Prefix, error) {
	if len(list) > MaxSubnets {
		return nil, fmt.Errorf("at most %d routed subnets per site", MaxSubnets)
	}
	out := make([]netip.Prefix, 0, len(list))
	for _, s := range list {
		if strings.TrimSpace(s) == "" {
			continue
		}
		p, err := ParseSubnet(s)
		if err != nil {
			return nil, err
		}
		if vpnNet.IsValid() && p.Overlaps(vpnNet.Masked()) {
			return nil, fmt.Errorf("%s overlaps the VPN range %s", p, vpnNet.Masked())
		}
		for _, q := range out {
			if p.Overlaps(q) {
				return nil, fmt.Errorf("%s overlaps %s", p, q)
			}
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out, nil
}

// CheckSubnetConflicts refuses subnets that overlap the routed subnets of another (not removed) peer.
func CheckSubnetConflicts(subnets []netip.Prefix, selfID string, others []Peer) error {
	for _, p := range subnets {
		for _, o := range others {
			if o.ID == selfID {
				continue
			}
			for _, q := range o.Subnets {
				if p.Overlaps(q) {
					return fmt.Errorf("%s overlaps %s of %q", p, q, o.Name)
				}
			}
		}
	}
	return nil
}

// PortRange is one port or an inclusive range.
type PortRange struct{ Lo, Hi uint16 }

func (r PortRange) String() string {
	if r.Lo == r.Hi {
		return strconv.Itoa(int(r.Lo))
	}
	return strconv.Itoa(int(r.Lo)) + "-" + strconv.Itoa(int(r.Hi))
}

// ParsePorts parses "102, 502, 8000-8100" (empty = all ports).
func ParsePorts(s string) ([]PortRange, error) {
	var out []PortRange
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lo, hi, isRange := strings.Cut(part, "-")
		a, err1 := strconv.Atoi(strings.TrimSpace(lo))
		b := a
		var err2 error
		if isRange {
			b, err2 = strconv.Atoi(strings.TrimSpace(hi))
		}
		if err1 != nil || err2 != nil || a < 1 || b > 65535 || a > b {
			return nil, fmt.Errorf("%q: ports are numbers 1 to 65535 or ranges like 8000-8100", part)
		}
		out = append(out, PortRange{uint16(a), uint16(b)})
	}
	if len(out) > MaxPortEntries {
		return nil, fmt.Errorf("at most %d ports or ranges per rule", MaxPortEntries)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Lo < out[j].Lo })
	for i := 1; i < len(out); i++ {
		if out[i].Lo <= out[i-1].Hi {
			return nil, fmt.Errorf("ports %s and %s overlap", out[i-1], out[i])
		}
	}
	return out, nil
}

// FormatPorts is the canonical text of a port list.
func FormatPorts(rs []PortRange) string {
	s := make([]string, len(rs))
	for i, r := range rs {
		s[i] = r.String()
	}
	return strings.Join(s, ", ")
}

// ValidRuleName checks a rule name.
func ValidRuleName(s string) error {
	if strings.TrimSpace(s) == "" || len([]rune(s)) > 80 {
		return errors.New("rule name: 1 to 80 characters")
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return errors.New("rule name: no control characters")
		}
	}
	return nil
}

// ValidateRule checks a rule against the current (not removed) peers. It returns the canonical port text.
func ValidateRule(r *Rule, peers map[string]Peer) error {
	if err := ValidRuleName(r.Name); err != nil {
		return err
	}
	if len(r.Sources) == 0 {
		return errors.New("a rule needs at least one source")
	}
	if len(r.Sources) > MaxSources {
		return fmt.Errorf("at most %d sources per rule", MaxSources)
	}
	for i, s := range r.Sources {
		switch s.Type {
		case SrcAll:
			r.Sources[i] = Source{Type: SrcAll}
		case SrcKind:
			if !ValidKind(s.Kind) {
				return fmt.Errorf("source kind must be device, user or site")
			}
			r.Sources[i] = Source{Type: SrcKind, Kind: s.Kind}
		case SrcPeer:
			if _, ok := peers[s.ID]; !ok {
				return fmt.Errorf("source peer %s does not exist", s.ID)
			}
			r.Sources[i] = Source{Type: SrcPeer, ID: s.ID}
		case SrcOrg:
			if s.ID == "" {
				return errors.New("source organization missing")
			}
			r.Sources[i] = Source{Type: SrcOrg, ID: s.ID}
		default:
			return fmt.Errorf("source type must be all, kind, peer or org")
		}
	}
	switch r.DestType {
	case DstServer:
		r.DestPeerID, r.DestCIDR = "", ""
	case DstPeer:
		if _, ok := peers[r.DestPeerID]; !ok {
			return errors.New("destination peer does not exist")
		}
		r.DestCIDR = ""
	case DstSite:
		site, ok := peers[r.DestPeerID]
		if !ok || site.Kind != KindSite {
			return errors.New("destination site does not exist")
		}
		if strings.TrimSpace(r.DestCIDR) != "" {
			p, err := ParseSubnet(r.DestCIDR)
			if err != nil {
				return fmt.Errorf("destination: %w", err)
			}
			inside := false
			for _, q := range site.Subnets {
				if q.Bits() <= p.Bits() && q.Contains(p.Addr()) {
					inside = true
				}
			}
			if !inside {
				return fmt.Errorf("destination %s is not inside the routed subnets of %q", p, site.Name)
			}
			r.DestCIDR = p.String()
		} else {
			r.DestCIDR = ""
		}
	default:
		return errors.New("destination type must be server, peer or site")
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

// ---- licence ----------------------------------------------------------------------------------------------------

// LicenceError explains which licensed capability a change needs. Without a licence existing peers keep working.
type LicenceError struct{ Msg string }

func (e *LicenceError) Error() string { return e.Msg }

// CheckAdd decides whether a new peer may be added: free are up to FreePeerLimit peers of kinds device and user
// without an expiry; site peers (site-to-site), time-limited access and more peers require a license.
func CheckAdd(licensed bool, existing int, kind string, expires bool) error {
	if licensed {
		if existing >= MaxPeers {
			return fmt.Errorf("at most %d peers", MaxPeers)
		}
		return nil
	}
	if kind == KindSite {
		return &LicenceError{"Site peers (routed subnets, site-to-site) require a license."}
	}
	if expires {
		return &LicenceError{"Time-limited access (an expiry date) requires a license."}
	}
	if existing >= FreePeerLimit {
		return &LicenceError{fmt.Sprintf("The free VPN allows up to %d peers; more peers require a license. Existing peers keep working.", FreePeerLimit)}
	}
	return nil
}

// CheckEdit decides whether a change of an existing peer needs the license: new routed subnets and setting an expiry
// do; renaming, disabling, enabling, clearing the expiry, rotating the key and removing never do.
func CheckEdit(licensed bool, subnetsChanged, expirySet bool) error {
	if licensed {
		return nil
	}
	if subnetsChanged {
		return &LicenceError{"Changing routed subnets (site-to-site) requires a license."}
	}
	if expirySet {
		return &LicenceError{"Time-limited access (an expiry date) requires a license."}
	}
	return nil
}

// ---- addresses --------------------------------------------------------------------------------------------------

// NextAddress returns the first free host address of the VPN range (the server's address and the network and
// broadcast addresses excluded).
func NextAddress(server netip.Prefix, used map[netip.Addr]bool) (netip.Addr, error) {
	net := server.Masked()
	a := net.Addr().Next()
	for ; net.Contains(a); a = a.Next() {
		if a == server.Addr() || used[a] {
			continue
		}
		if !net.Contains(a.Next()) { // broadcast
			break
		}
		return a, nil
	}
	return netip.Addr{}, fmt.Errorf("no free address in %s", net)
}

// ---- desired state ----------------------------------------------------------------------------------------------

// DesiredPeer is a peer as the agent configures it. PresharedKey is sent only when the key is new (creation or
// rotation); the agent keeps it and the web service never stores it.
type DesiredPeer struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	PublicKey    string   `json:"public_key"`
	PresharedKey string   `json:"preshared_key,omitempty"`
	Address      string   `json:"address"`           // host address in the VPN, without prefix
	Subnets      []string `json:"subnets,omitempty"` // routed subnets of a site
	Enabled      bool     `json:"enabled"`           // false: kept (keys) but not configured on the interface
}

// FwRule is one compiled access rule: concrete source and destination prefixes.
type FwRule struct {
	ID    string   `json:"id"`
	Chain string   `json:"chain"` // input (to the server) | forward (to peers and sites)
	Src   []string `json:"src"`
	Dst   []string `json:"dst"`
	Proto string   `json:"proto"`           // tcp | udp | icmp | any
	Ports []string `json:"ports,omitempty"` // "102", "8000-8100"
}

// Desired is the complete VPN state the agent applies atomically.
type Desired struct {
	Peers   []DesiredPeer `json:"peers"`
	Rules   []FwRule      `json:"rules"`
	Forward bool          `json:"forward"` // forwarding through wg0 is needed (rules towards peers or sites)
}

// AllowedIPs of a peer on the server side: its address and its routed subnets.
func (p DesiredPeer) AllowedIPs() []string {
	out := []string{p.Address + "/32"}
	return append(out, p.Subnets...)
}

// Compile turns stored peers and rules into the state the agent applies. Only active peers (enabled, not expired)
// are configured and only they appear in rules; rules whose sources or destination resolve to nothing are left out.
func Compile(server netip.Prefix, peers []Peer, rules []Rule, now time.Time) Desired {
	d := Desired{Peers: []DesiredPeer{}, Rules: []FwRule{}}
	active := map[string]Peer{}
	sorted := append([]Peer(nil), peers...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Address.Less(sorted[j].Address) })
	for _, p := range sorted {
		dp := DesiredPeer{ID: p.ID, Name: p.Name, PublicKey: p.PublicKey, Address: p.Address.String(), Enabled: p.Active(now)}
		for _, s := range p.Subnets {
			dp.Subnets = append(dp.Subnets, s.String())
		}
		d.Peers = append(d.Peers, dp)
		if dp.Enabled {
			active[p.ID] = p
		}
	}
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		src := resolveSources(r.Sources, sorted, active)
		var dst []string
		chain := ChainForward
		switch r.DestType {
		case DstServer:
			chain, dst = ChainInput, []string{server.Addr().String() + "/32"}
		case DstPeer:
			if p, ok := active[r.DestPeerID]; ok {
				dst = []string{p.Address.String() + "/32"}
			}
		case DstSite:
			if p, ok := active[r.DestPeerID]; ok && p.Kind == KindSite {
				if r.DestCIDR != "" {
					dst = []string{r.DestCIDR}
				} else {
					for _, s := range p.Subnets {
						dst = append(dst, s.String())
					}
				}
			}
		}
		if len(src) == 0 || len(dst) == 0 {
			continue
		}
		fr := FwRule{ID: r.ID, Chain: chain, Src: src, Dst: dst, Proto: r.Protocol}
		if r.Protocol == "tcp" || r.Protocol == "udp" {
			if ps, err := ParsePorts(r.Ports); err == nil {
				for _, pr := range ps {
					fr.Ports = append(fr.Ports, pr.String())
				}
			}
		}
		d.Rules = append(d.Rules, fr)
		if chain == ChainForward {
			d.Forward = true
		}
	}
	return d
}

func resolveSources(srcs []Source, peers []Peer, active map[string]Peer) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p Peer) {
		for _, s := range append([]string{p.Address.String() + "/32"}, prefixStrings(p.Subnets)...) {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	for _, p := range peers {
		if _, ok := active[p.ID]; !ok {
			continue
		}
		for _, s := range srcs {
			if (s.Type == SrcAll) || (s.Type == SrcKind && s.Kind == p.Kind) || (s.Type == SrcPeer && s.ID == p.ID) || (s.Type == SrcOrg && s.ID != "" && s.ID == p.OrgID) {
				add(p)
				break
			}
		}
	}
	return out
}

func prefixStrings(ps []netip.Prefix) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.String()
	}
	return out
}

// ClientAllowedIPs is what a peer routes into the tunnel: the VPN range, and for users and sites also the routed
// subnets of the other sites (the server's firewall decides what is actually reachable).
func ClientAllowedIPs(server netip.Prefix, kind, selfID string, peers []Peer) []string {
	out := []string{server.Masked().String()}
	if kind == KindDevice {
		return out
	}
	for _, p := range peers {
		if p.ID == selfID || p.Kind != KindSite {
			continue
		}
		out = append(out, prefixStrings(p.Subnets)...)
	}
	return out
}

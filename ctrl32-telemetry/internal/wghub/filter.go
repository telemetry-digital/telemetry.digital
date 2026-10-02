package wghub

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

// The in-process packet filter of the userspace hub (ADR-0020). It enforces the same compiled access rules as the
// nftables table on Linux (vpn.FwRule, chains input and forward): everything that arrives from a peer is dropped
// unless a rule accepts it; ping of the server passes; replies of accepted forwarded flows pass (connection
// tracking, like "ct state established"). Packets the server itself sends (replies of its services) are not judged.
// Only IPv4 is handled; IPv6 and IP fragments are dropped (the VPN is IPv4 only and peers use MTU 1420).

// Verdict is what happens to a packet that arrived from a peer.
type Verdict int

const (
	Drop    Verdict = iota // default deny
	Input                  // to the server's own services (the gvisor stack)
	Forward                // to another peer or a site subnet (back into WireGuard)
)

const (
	protoICMP = 1
	protoTCP  = 6
	protoUDP  = 17

	// connection tracking: idle timeouts and the table size (a full table refuses new forwarded flows)
	tcpIdle   = 10 * time.Minute
	udpIdle   = 60 * time.Second
	icmpIdle  = 30 * time.Second
	maxFlows  = 65536
	icmpEcho  = 8
	icmpReply = 0
)

type portRange struct{ lo, hi uint16 }

type rule struct {
	id    string
	chain string
	src   []netip.Prefix
	dst   []netip.Prefix
	proto uint8 // 0 = any
	ports []portRange
}

// ruleset is immutable once built; the filter swaps it atomically.
type ruleset struct {
	server netip.Addr
	vpnNet netip.Prefix
	routed []netip.Prefix // addresses (/32) and routed subnets of the configured peers
	rules  []rule
	text   string
}

// Packet is the parsed header of an IPv4 packet.
type Packet struct {
	Src, Dst     netip.Addr
	Proto        uint8
	Sport, Dport uint16 // TCP/UDP ports; for ICMP echo Sport is the identifier
	ICMPType     uint8
}

// ParseIPv4 reads the header fields the filter needs. ok is false for anything that is not a complete, unfragmented
// IPv4 packet.
func ParseIPv4(b []byte) (Packet, bool) {
	var p Packet
	if len(b) < 20 || b[0]>>4 != 4 {
		return p, false
	}
	ihl := int(b[0]&0x0f) * 4
	total := int(binary.BigEndian.Uint16(b[2:4]))
	if ihl < 20 || total < ihl || total > len(b) {
		return p, false
	}
	if frag := binary.BigEndian.Uint16(b[6:8]); frag&0x3fff != 0 { // more fragments or an offset
		return p, false
	}
	p.Proto = b[9]
	p.Src = netip.AddrFrom4([4]byte(b[12:16]))
	p.Dst = netip.AddrFrom4([4]byte(b[16:20]))
	l4 := b[ihl:total]
	switch p.Proto {
	case protoTCP, protoUDP:
		if len(l4) < 4 {
			return p, false
		}
		p.Sport, p.Dport = binary.BigEndian.Uint16(l4[0:2]), binary.BigEndian.Uint16(l4[2:4])
	case protoICMP:
		if len(l4) < 8 {
			return p, false
		}
		p.ICMPType = l4[0]
		p.Sport = binary.BigEndian.Uint16(l4[4:6])
	}
	return p, true
}

type flowKey struct {
	proto        uint8
	src, dst     netip.Addr
	sport, dport uint16
}

type flow struct {
	orig    Packet // the packet that opened the flow (re-checked when the rules change)
	expires time.Time
}

// Filter decides about packets from peers. It is safe for concurrent use.
type Filter struct {
	rs        atomic.Pointer[ruleset]
	mu        sync.Mutex
	flows     map[flowKey]*flow
	now       func() time.Time
	dropped   atomic.Uint64
	input     atomic.Uint64
	forwarded atomic.Uint64
}

// NewFilter returns a filter that drops everything until rules are set.
func NewFilter(server netip.Prefix) *Filter {
	f := &Filter{flows: map[flowKey]*flow{}, now: time.Now}
	f.rs.Store(&ruleset{server: server.Addr(), vpnNet: server.Masked()})
	return f
}

// Counters returns the dropped, input and forwarded packet counts.
func (f *Filter) Counters() (dropped, input, forwarded uint64) {
	return f.dropped.Load(), f.input.Load(), f.forwarded.Load()
}

// Compile builds a ruleset from the compiled VPN state. It validates the prefixes and ports again.
func compile(server netip.Prefix, d vpn.Desired) (*ruleset, error) {
	rs := &ruleset{server: server.Addr(), vpnNet: server.Masked()}
	for _, p := range d.Peers {
		if !p.Enabled {
			continue
		}
		for _, s := range p.AllowedIPs() {
			pr, err := netip.ParsePrefix(s)
			if err != nil {
				return nil, fmt.Errorf("peer %s: %w", p.Name, err)
			}
			rs.routed = append(rs.routed, pr)
		}
	}
	for _, r := range d.Rules {
		cr := rule{id: r.ID, chain: r.Chain}
		if r.Chain != vpn.ChainInput && r.Chain != vpn.ChainForward {
			return nil, fmt.Errorf("rule %s: unknown chain %q", r.ID, r.Chain)
		}
		for _, s := range r.Src {
			pr, err := netip.ParsePrefix(s)
			if err != nil {
				return nil, fmt.Errorf("rule %s: %w", r.ID, err)
			}
			cr.src = append(cr.src, pr)
		}
		for _, s := range r.Dst {
			pr, err := netip.ParsePrefix(s)
			if err != nil {
				return nil, fmt.Errorf("rule %s: %w", r.ID, err)
			}
			cr.dst = append(cr.dst, pr)
		}
		switch r.Proto {
		case "tcp":
			cr.proto = protoTCP
		case "udp":
			cr.proto = protoUDP
		case "icmp":
			cr.proto = protoICMP
		case "any":
		default:
			return nil, fmt.Errorf("rule %s: unknown protocol %q", r.ID, r.Proto)
		}
		if len(r.Ports) > 0 {
			ps, err := vpn.ParsePorts(strings.Join(r.Ports, ","))
			if err != nil {
				return nil, fmt.Errorf("rule %s: %w", r.ID, err)
			}
			for _, p := range ps {
				cr.ports = append(cr.ports, portRange{p.Lo, p.Hi})
			}
		}
		rs.rules = append(rs.rules, cr)
	}
	rs.text = describe(rs, d.Rules)
	return rs, nil
}

func describe(rs *ruleset, rules []vpn.FwRule) string {
	var b strings.Builder
	b.WriteString("# ctrl32 telemetry — VPN access rules enforced inside the agent (userspace WireGuard hub, Windows).\n")
	b.WriteString("# Default deny for everything that arrives from a peer. Ping of the server " + rs.server.String() + " is allowed.\n")
	b.WriteString("# Replies of accepted forwarded connections pass (connection tracking). TCP services of the server are\n")
	b.WriteString("# reached through the agent on 127.0.0.1:<port>; UDP services of the server are not offered.\n")
	for _, chain := range []string{vpn.ChainInput, vpn.ChainForward} {
		b.WriteString("\nchain " + chain + " {\n")
		for _, r := range rules {
			if r.Chain != chain {
				continue
			}
			line := "\taccept src " + strings.Join(r.Src, ", ") + " dst " + strings.Join(r.Dst, ", ") + " proto " + r.Proto
			if len(r.Ports) > 0 {
				line += " ports " + strings.Join(r.Ports, ", ")
			}
			b.WriteString(line + "   # rule " + r.ID + "\n")
		}
		b.WriteString("\tdrop   # default deny\n}\n")
	}
	return b.String()
}

// SetRules replaces the rules atomically. Tracked flows that the new rules no longer allow are removed, so taking
// access away ends existing forwarded connections too (stricter than Linux, where established connections survive).
func (f *Filter) SetRules(server netip.Prefix, d vpn.Desired) error {
	rs, err := compile(server, d)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rs.Store(rs)
	for k, fl := range f.flows {
		if rs.match(vpn.ChainForward, fl.orig) == "" {
			delete(f.flows, k)
		}
	}
	return nil
}

// DescribeRules renders the ruleset text of a state without a running hub.
func DescribeRules(server netip.Prefix, d vpn.Desired) (string, error) {
	rs, err := compile(server, d)
	if err != nil {
		return "", err
	}
	return rs.text, nil
}

// Describe is the human-readable ruleset (shown in the UI instead of the nftables table).
func (f *Filter) Describe() string { return f.rs.Load().text }

func contains(ps []netip.Prefix, a netip.Addr) bool {
	for _, p := range ps {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// match returns the id of the first rule of chain that accepts the packet, or "".
func (rs *ruleset) match(chain string, p Packet) string {
	for _, r := range rs.rules {
		if r.chain != chain || (r.proto != 0 && r.proto != p.Proto) || !contains(r.src, p.Src) || !contains(r.dst, p.Dst) {
			continue
		}
		if len(r.ports) > 0 {
			ok := false
			for _, pr := range r.ports {
				if p.Dport >= pr.lo && p.Dport <= pr.hi {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		return r.id
	}
	return ""
}

func idle(proto uint8) time.Duration {
	switch proto {
	case protoTCP:
		return tcpIdle
	case protoUDP:
		return udpIdle
	}
	return icmpIdle
}

func keyOf(p Packet) flowKey {
	if p.Proto == protoICMP {
		return flowKey{proto: p.Proto, src: p.Src, dst: p.Dst, sport: p.Sport}
	}
	return flowKey{proto: p.Proto, src: p.Src, dst: p.Dst, sport: p.Sport, dport: p.Dport}
}

func replyKey(p Packet) flowKey {
	if p.Proto == protoICMP {
		return flowKey{proto: p.Proto, src: p.Dst, dst: p.Src, sport: p.Sport}
	}
	return flowKey{proto: p.Proto, src: p.Dst, dst: p.Src, sport: p.Dport, dport: p.Sport}
}

// Decide judges one packet that arrived from a peer.
func (f *Filter) Decide(b []byte) Verdict {
	p, ok := ParseIPv4(b)
	if !ok {
		f.dropped.Add(1)
		return Drop
	}
	rs := f.rs.Load()
	if p.Dst == rs.server {
		if (p.Proto == protoICMP && p.ICMPType == icmpEcho) || rs.match(vpn.ChainInput, p) != "" {
			f.input.Add(1)
			return Input
		}
		f.dropped.Add(1)
		return Drop
	}
	if !contains(rs.routed, p.Dst) || !(rs.vpnNet.Contains(p.Src) || contains(rs.routed, p.Src)) {
		f.dropped.Add(1)
		return Drop
	}
	now := f.now()
	f.mu.Lock()
	defer f.mu.Unlock()
	// an established flow in either direction (the opening packet's direction or the reply)
	if fl, ok := f.flows[keyOf(p)]; ok && now.Before(fl.expires) {
		fl.expires = now.Add(idle(p.Proto))
		f.forwarded.Add(1)
		return Forward
	}
	if p.Proto == protoICMP && p.ICMPType == icmpReply {
		// echo replies only as part of a tracked echo flow (handled above)
		f.dropped.Add(1)
		return Drop
	}
	if rs.match(vpn.ChainForward, p) == "" {
		f.dropped.Add(1)
		return Drop
	}
	if len(f.flows) >= maxFlows {
		for k, fl := range f.flows {
			if !now.Before(fl.expires) {
				delete(f.flows, k)
			}
		}
		if len(f.flows) >= maxFlows {
			f.dropped.Add(1)
			return Drop
		}
	}
	fl := &flow{orig: p, expires: now.Add(idle(p.Proto))}
	f.flows[keyOf(p)] = fl
	f.flows[replyKey(p)] = fl
	f.forwarded.Add(1)
	return Forward
}

// Flows is the number of tracked flow directions (tests and diagnostics).
func (f *Filter) Flows() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.flows)
}

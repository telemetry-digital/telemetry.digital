package wghub

import (
	"encoding/binary"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

var srvPrefix = netip.MustParsePrefix("10.66.0.1/24")

// pkt builds a minimal IPv4 packet (header + 8 bytes of transport header).
func pkt(proto uint8, src, dst string, sport, dport uint16, icmpType uint8) []byte {
	b := make([]byte, 28)
	b[0] = 0x45
	binary.BigEndian.PutUint16(b[2:4], 28)
	b[8] = 64
	b[9] = proto
	s, d := netip.MustParseAddr(src).As4(), netip.MustParseAddr(dst).As4()
	copy(b[12:16], s[:])
	copy(b[16:20], d[:])
	switch proto {
	case protoTCP, protoUDP:
		binary.BigEndian.PutUint16(b[20:22], sport)
		binary.BigEndian.PutUint16(b[22:24], dport)
	case protoICMP:
		b[20] = icmpType
		binary.BigEndian.PutUint16(b[24:26], sport)
	}
	return b
}

func desired() vpn.Desired {
	return vpn.Desired{
		Peers: []vpn.DesiredPeer{
			{ID: "tech", Name: "tech", Address: "10.66.0.2", Enabled: true},
			{ID: "plc", Name: "plc", Address: "10.66.0.3", Enabled: true},
			{ID: "site", Name: "site", Address: "10.66.0.4", Subnets: []string{"192.168.10.0/24"}, Enabled: true},
			{ID: "off", Name: "off", Address: "10.66.0.5", Enabled: false},
		},
		Rules: []vpn.FwRule{
			{ID: "mqtt", Chain: vpn.ChainInput, Src: []string{"10.66.0.3/32"}, Dst: []string{"10.66.0.1/32"}, Proto: "tcp", Ports: []string{"1883"}},
			{ID: "plc", Chain: vpn.ChainForward, Src: []string{"10.66.0.2/32"}, Dst: []string{"192.168.10.0/24"}, Proto: "tcp", Ports: []string{"102", "502"}},
			{ID: "ping", Chain: vpn.ChainForward, Src: []string{"10.66.0.2/32"}, Dst: []string{"10.66.0.3/32"}, Proto: "icmp"},
		},
		Forward: true,
	}
}

func newTestFilter(t *testing.T) *Filter {
	t.Helper()
	f := NewFilter(srvPrefix)
	if err := f.SetRules(srvPrefix, desired()); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestFilterDefaultDenyWithoutRules(t *testing.T) {
	f := NewFilter(srvPrefix)
	if v := f.Decide(pkt(protoTCP, "10.66.0.3", "10.66.0.1", 40000, 1883, 0)); v != Drop {
		t.Fatalf("no rules: %v", v)
	}
	if v := f.Decide(pkt(protoICMP, "10.66.0.3", "10.66.0.1", 7, 0, icmpEcho)); v != Input {
		t.Fatalf("ping of the server must pass like the nft table: %v", v)
	}
}

func TestFilterInput(t *testing.T) {
	f := newTestFilter(t)
	cases := []struct {
		name string
		b    []byte
		want Verdict
	}{
		{"allowed port", pkt(protoTCP, "10.66.0.3", "10.66.0.1", 40000, 1883, 0), Input},
		{"other source", pkt(protoTCP, "10.66.0.2", "10.66.0.1", 40000, 1883, 0), Drop},
		{"other port", pkt(protoTCP, "10.66.0.3", "10.66.0.1", 40000, 22, 0), Drop},
		{"udp same port", pkt(protoUDP, "10.66.0.3", "10.66.0.1", 40000, 1883, 0), Drop},
		{"ping server", pkt(protoICMP, "10.66.0.2", "10.66.0.1", 1, 0, icmpEcho), Input},
		{"not ipv4", []byte{0x60, 0, 0, 0}, Drop},
		{"outside the vpn", pkt(protoTCP, "10.66.0.2", "8.8.8.8", 40000, 53, 0), Drop},
		{"disabled peer as destination", pkt(protoICMP, "10.66.0.2", "10.66.0.5", 1, 0, icmpEcho), Drop},
	}
	for _, c := range cases {
		if got := f.Decide(c.b); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFilterForwardAndConntrack(t *testing.T) {
	f := newTestFilter(t)
	now := time.Unix(1000, 0)
	f.now = func() time.Time { return now }
	// technician -> PLC at the site: allowed, the reply passes, a new connection from the site does not
	if v := f.Decide(pkt(protoTCP, "10.66.0.2", "192.168.10.20", 40000, 502, 0)); v != Forward {
		t.Fatalf("forward: %v", v)
	}
	if v := f.Decide(pkt(protoTCP, "192.168.10.20", "10.66.0.2", 502, 40000, 0)); v != Forward {
		t.Fatalf("reply: %v", v)
	}
	if v := f.Decide(pkt(protoTCP, "192.168.10.20", "10.66.0.2", 502, 40001, 0)); v != Drop {
		t.Fatalf("unrelated packet from the site: %v", v)
	}
	if v := f.Decide(pkt(protoTCP, "10.66.0.2", "192.168.10.20", 40000, 80, 0)); v != Drop {
		t.Fatalf("port not in the rule: %v", v)
	}
	// echo: request allowed, reply passes, an unsolicited reply does not
	if v := f.Decide(pkt(protoICMP, "10.66.0.2", "10.66.0.3", 9, 0, icmpEcho)); v != Forward {
		t.Fatalf("echo: %v", v)
	}
	if v := f.Decide(pkt(protoICMP, "10.66.0.3", "10.66.0.2", 9, 0, icmpReply)); v != Forward {
		t.Fatalf("echo reply: %v", v)
	}
	if v := f.Decide(pkt(protoICMP, "10.66.0.3", "10.66.0.2", 10, 0, icmpReply)); v != Drop {
		t.Fatalf("unsolicited echo reply: %v", v)
	}
	if v := f.Decide(pkt(protoICMP, "10.66.0.3", "10.66.0.2", 11, 0, icmpEcho)); v != Drop {
		t.Fatalf("echo in the other direction has no rule: %v", v)
	}
	// idle timeout
	now = now.Add(tcpIdle + time.Second)
	if v := f.Decide(pkt(protoTCP, "192.168.10.20", "10.66.0.2", 502, 40000, 0)); v != Drop {
		t.Fatalf("reply after the idle timeout: %v", v)
	}
	d, in, fw := f.Counters()
	if d == 0 || fw == 0 || in != 0 {
		t.Fatalf("counters %d %d %d", d, in, fw)
	}
}

func TestFilterRuleSwapRemovesFlows(t *testing.T) {
	f := newTestFilter(t)
	if v := f.Decide(pkt(protoTCP, "10.66.0.2", "192.168.10.20", 40000, 502, 0)); v != Forward {
		t.Fatal(v)
	}
	if v := f.Decide(pkt(protoICMP, "10.66.0.2", "10.66.0.3", 9, 0, icmpEcho)); v != Forward {
		t.Fatal(v)
	}
	if f.Flows() != 4 {
		t.Fatalf("flows %d", f.Flows())
	}
	d := desired()
	d.Rules = d.Rules[:2] // the ping rule is taken away
	if err := f.SetRules(srvPrefix, d); err != nil {
		t.Fatal(err)
	}
	if f.Flows() != 2 {
		t.Fatalf("flows after the swap %d", f.Flows())
	}
	if v := f.Decide(pkt(protoICMP, "10.66.0.3", "10.66.0.2", 9, 0, icmpReply)); v != Drop {
		t.Fatalf("a flow whose rule is gone must end: %v", v)
	}
	if v := f.Decide(pkt(protoTCP, "192.168.10.20", "10.66.0.2", 502, 40000, 0)); v != Forward {
		t.Fatalf("a flow whose rule stays keeps working: %v", v)
	}
	// a broken ruleset is refused and the old one stays
	bad := desired()
	bad.Rules = []vpn.FwRule{{ID: "x", Chain: "output", Src: []string{"10.66.0.2/32"}, Dst: []string{"10.66.0.1/32"}, Proto: "tcp"}}
	if err := f.SetRules(srvPrefix, bad); err == nil {
		t.Fatal("unknown chain accepted")
	}
	if v := f.Decide(pkt(protoTCP, "10.66.0.3", "10.66.0.1", 40000, 1883, 0)); v != Input {
		t.Fatalf("old rules lost: %v", v)
	}
}

func TestFilterFragmentsDropped(t *testing.T) {
	f := newTestFilter(t)
	b := pkt(protoTCP, "10.66.0.3", "10.66.0.1", 40000, 1883, 0)
	b[6] = 0x20 // more fragments
	if v := f.Decide(b); v != Drop {
		t.Fatalf("fragment: %v", v)
	}
}

func TestDescribe(t *testing.T) {
	f := newTestFilter(t)
	txt := f.Describe()
	for _, want := range []string{"chain input", "chain forward", "rule mqtt", "ports 102, 502", "default deny"} {
		if !strings.Contains(txt, want) {
			t.Errorf("describe lacks %q:\n%s", want, txt)
		}
	}
}

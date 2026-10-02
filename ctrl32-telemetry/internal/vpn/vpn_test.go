package vpn

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func mustPeer(id, name, kind, addr string, subnets ...string) Peer {
	p := Peer{ID: id, Name: name, Kind: kind, Address: netip.MustParseAddr(addr), Enabled: true}
	_, pub, _ := GenerateKey()
	p.PublicKey = pub
	for _, s := range subnets {
		p.Subnets = append(p.Subnets, netip.MustParsePrefix(s))
	}
	return p
}

func TestKeys(t *testing.T) {
	priv, pub, err := GenerateKey()
	if err != nil || !ValidKey(priv) || !ValidKey(pub) || priv == pub {
		t.Fatalf("keys %q %q %v", priv, pub, err)
	}
	again, err := PublicKey(priv)
	if err != nil || again != pub {
		t.Fatalf("public key not reproducible")
	}
	// RFC 7748 test vector (Alice)
	got, err := PublicKey("dwdtCnMYpX08FsFyUbJmRd9ML4frwJkqsXf7pR25LCo=")
	if err != nil || got != "hSDwCYkwp1R0i33ctD73Wg2/Og0mOBr066SpjqqbTmo=" {
		t.Fatalf("X25519 vector: %q %v", got, err)
	}
	psk, _ := GeneratePSK()
	if !ValidKey(psk) {
		t.Fatalf("psk %q", psk)
	}
	if ValidKey("not a key") || ValidKey(strings.Repeat("A", 44)) {
		t.Fatal("invalid key accepted")
	}
}

func TestParseSubnets(t *testing.T) {
	vpn := netip.MustParsePrefix("10.66.0.1/24")
	good, err := ParseSubnets([]string{"192.168.10.0/24", " 172.20.5.7 ", ""}, vpn)
	if err != nil || len(good) != 2 || good[0].String() != "172.20.5.7/32" {
		t.Fatalf("good: %v %v", good, err)
	}
	for in, want := range map[string]string{
		"192.168.10.5/24": "did you mean 192.168.10.0/24",
		"10.66.0.0/16":    "overlaps the VPN range",
		"0.0.0.0/0":       "too large",
		"127.0.0.0/8":     "reserved",
		"169.254.1.0/24":  "reserved",
		"fd00::/64":       "not an IPv4 subnet",
		"nonsense":        "not an IPv4 subnet",
	} {
		if _, err := ParseSubnets([]string{in}, vpn); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", in, err, want)
		}
	}
	if _, err := ParseSubnets([]string{"192.168.0.0/16", "192.168.10.0/24"}, vpn); err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Errorf("self overlap: %v", err)
	}
	many := make([]string, MaxSubnets+1)
	for i := range many {
		many[i] = netip.AddrFrom4([4]byte{192, 168, byte(i), 0}).String() + "/24"
	}
	if _, err := ParseSubnets(many, vpn); err == nil {
		t.Error("too many subnets accepted")
	}
	other := mustPeer("b", "site-b", KindSite, "10.66.0.3", "192.168.20.0/24")
	if err := CheckSubnetConflicts([]netip.Prefix{netip.MustParsePrefix("192.168.20.128/25")}, "a", []Peer{other}); err == nil {
		t.Error("overlap with another site accepted")
	}
	if err := CheckSubnetConflicts(other.Subnets, "b", []Peer{other}); err != nil {
		t.Errorf("own subnets: %v", err)
	}
}

func TestParsePorts(t *testing.T) {
	ps, err := ParsePorts(" 502, 102,8000-8100 ")
	if err != nil || FormatPorts(ps) != "102, 502, 8000-8100" {
		t.Fatalf("%v %v", ps, err)
	}
	if ps, err := ParsePorts(""); err != nil || len(ps) != 0 {
		t.Fatal("empty = all ports")
	}
	for _, bad := range []string{"0", "65536", "80-79", "x", "1-2-3", "80, 70-90"} {
		if _, err := ParsePorts(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestLicence(t *testing.T) {
	var le *LicenceError
	if err := CheckAdd(false, 4, KindDevice, false); err != nil {
		t.Fatalf("5th free peer: %v", err)
	}
	if err := CheckAdd(false, FreePeerLimit, KindUser, false); !errors.As(err, &le) || !strings.Contains(err.Error(), "up to 5 peers") {
		t.Fatalf("6th free peer: %v", err)
	}
	if err := CheckAdd(false, 0, KindSite, false); !errors.As(err, &le) {
		t.Fatalf("site without licence: %v", err)
	}
	if err := CheckAdd(false, 0, KindUser, true); !errors.As(err, &le) {
		t.Fatalf("expiry without licence: %v", err)
	}
	if err := CheckAdd(true, 50, KindSite, true); err != nil {
		t.Fatalf("licensed: %v", err)
	}
	if err := CheckEdit(false, false, false); err != nil {
		t.Fatal("rename/disable must stay free")
	}
	if err := CheckEdit(false, true, false); !errors.As(err, &le) {
		t.Fatal("subnet change without licence")
	}
	if err := CheckEdit(false, false, true); !errors.As(err, &le) {
		t.Fatal("expiry without licence")
	}
}

func TestNextAddress(t *testing.T) {
	srv := netip.MustParsePrefix("10.66.0.1/30")
	a, err := NextAddress(srv, nil)
	if err != nil || a.String() != "10.66.0.2" {
		t.Fatalf("%v %v", a, err)
	}
	if _, err := NextAddress(srv, map[netip.Addr]bool{a: true}); err == nil {
		t.Fatal("broadcast handed out")
	}
	srv = netip.MustParsePrefix("10.66.0.5/24")
	a, _ = NextAddress(srv, map[netip.Addr]bool{netip.MustParseAddr("10.66.0.1"): true})
	if a.String() != "10.66.0.2" {
		t.Fatalf("got %v", a)
	}
}

func testWorld() (netip.Prefix, []Peer, map[string]Peer) {
	srv := netip.MustParsePrefix("10.66.0.1/24")
	tech := mustPeer("u1", "technician", KindUser, "10.66.0.2")
	tech.OrgID = "org-a"
	dev := mustPeer("d1", "gateway", KindDevice, "10.66.0.3")
	site := mustPeer("s1", "plant-1", KindSite, "10.66.0.4", "192.168.10.0/24", "192.168.11.0/24")
	off := mustPeer("u2", "old-laptop", KindUser, "10.66.0.5")
	off.Enabled = false
	past := time.Now().Add(-time.Hour)
	exp := mustPeer("u3", "contractor", KindUser, "10.66.0.6")
	exp.ExpiresAt = &past
	peers := []Peer{tech, dev, site, off, exp}
	m := map[string]Peer{}
	for _, p := range peers {
		m[p.ID] = p
	}
	return srv, peers, m
}

func TestValidateRule(t *testing.T) {
	_, _, m := testWorld()
	r := Rule{Name: "PLC programming", Sources: []Source{{Type: SrcPeer, ID: "u1", Kind: "ignored"}}, DestType: DstSite, DestPeerID: "s1", DestCIDR: "192.168.10.20", Protocol: "tcp", Ports: "502,102"}
	if err := ValidateRule(&r, m); err != nil {
		t.Fatal(err)
	}
	if r.DestCIDR != "192.168.10.20/32" || r.Ports != "102, 502" || r.Sources[0].Kind != "" {
		t.Fatalf("not canonical: %+v", r)
	}
	bad := []Rule{
		{Name: "", Sources: []Source{{Type: SrcAll}}, DestType: DstServer, Protocol: "tcp"},
		{Name: "x", DestType: DstServer, Protocol: "tcp"},
		{Name: "x", Sources: []Source{{Type: SrcPeer, ID: "nope"}}, DestType: DstServer, Protocol: "tcp"},
		{Name: "x", Sources: []Source{{Type: SrcKind, Kind: "robot"}}, DestType: DstServer, Protocol: "tcp"},
		{Name: "x", Sources: []Source{{Type: SrcAll}}, DestType: DstSite, DestPeerID: "u1", Protocol: "tcp"},
		{Name: "x", Sources: []Source{{Type: SrcAll}}, DestType: DstSite, DestPeerID: "s1", DestCIDR: "10.1.0.0/24", Protocol: "tcp"},
		{Name: "x", Sources: []Source{{Type: SrcAll}}, DestType: DstServer, Protocol: "icmp", Ports: "80"},
		{Name: "x", Sources: []Source{{Type: SrcAll}}, DestType: DstServer, Protocol: "sctp"},
		{Name: "x", Sources: []Source{{Type: SrcAll}}, DestType: "internet", Protocol: "any"},
	}
	for i, b := range bad {
		if err := ValidateRule(&b, m); err == nil {
			t.Errorf("bad rule %d accepted", i)
		}
	}
}

func TestCompile(t *testing.T) {
	srv, peers, _ := testWorld()
	rules := []Rule{
		{ID: "r-mqtt", Name: "MQTT", Sources: []Source{{Type: SrcKind, Kind: KindDevice}, {Type: SrcKind, Kind: KindSite}}, DestType: DstServer, Protocol: "tcp", Ports: "1883", Enabled: true},
		{ID: "r-plc", Name: "PLC", Sources: []Source{{Type: SrcOrg, ID: "org-a"}}, DestType: DstSite, DestPeerID: "s1", DestCIDR: "192.168.10.20/32", Protocol: "tcp", Ports: "102, 502", Enabled: true},
		{ID: "r-off", Name: "disabled rule", Sources: []Source{{Type: SrcAll}}, DestType: DstServer, Protocol: "any", Enabled: false},
		{ID: "r-gone", Name: "only inactive sources", Sources: []Source{{Type: SrcPeer, ID: "u2"}, {Type: SrcPeer, ID: "u3"}}, DestType: DstServer, Protocol: "any", Enabled: true},
		{ID: "r-site", Name: "whole site", Sources: []Source{{Type: SrcPeer, ID: "u1"}}, DestType: DstSite, DestPeerID: "s1", Protocol: "icmp", Enabled: true},
	}
	d := Compile(srv, peers, rules, time.Now())
	if len(d.Peers) != 5 {
		t.Fatalf("peers %d", len(d.Peers))
	}
	en := map[string]bool{}
	for _, p := range d.Peers {
		en[p.ID] = p.Enabled
	}
	if !en["u1"] || en["u2"] || en["u3"] {
		t.Fatalf("enabled flags %v", en)
	}
	if len(d.Rules) != 3 || !d.Forward {
		t.Fatalf("rules %+v forward %v", d.Rules, d.Forward)
	}
	mqtt := d.Rules[0]
	if mqtt.Chain != ChainInput || strings.Join(mqtt.Src, " ") != "10.66.0.3/32 10.66.0.4/32 192.168.10.0/24 192.168.11.0/24" || mqtt.Dst[0] != "10.66.0.1/32" {
		t.Fatalf("mqtt rule %+v", mqtt)
	}
	plc := d.Rules[1]
	if plc.Chain != ChainForward || strings.Join(plc.Src, " ") != "10.66.0.2/32" || strings.Join(plc.Ports, " ") != "102 502" {
		t.Fatalf("plc rule %+v", plc)
	}
	if strings.Join(d.Rules[2].Dst, " ") != "192.168.10.0/24 192.168.11.0/24" {
		t.Fatalf("site rule %+v", d.Rules[2])
	}
	// without rules towards peers or sites there is no forwarding
	d2 := Compile(srv, peers, rules[:1], time.Now())
	if d2.Forward {
		t.Fatal("forwarding enabled for input-only rules")
	}
	// the expired peer becomes active again only with a new expiry
	if ValidateDesired(&d, srv, nil) != nil {
		t.Fatalf("compiled state rejected: %v", ValidateDesired(&d, srv, nil))
	}
	// client routes: devices only the VPN, users also the sites
	if got := ClientAllowedIPs(srv, KindDevice, "d1", peers); strings.Join(got, ",") != "10.66.0.0/24" {
		t.Fatalf("device %v", got)
	}
	if got := ClientAllowedIPs(srv, KindUser, "u1", peers); strings.Join(got, ",") != "10.66.0.0/24,192.168.10.0/24,192.168.11.0/24" {
		t.Fatalf("user %v", got)
	}
}

func TestValidateDesiredRejects(t *testing.T) {
	srv, peers, _ := testWorld()
	d := Compile(srv, peers, nil, time.Now())
	if err := ValidateDesired(&d, srv, []netip.Prefix{netip.MustParsePrefix("192.168.11.0/24")}); err == nil || !strings.Contains(err.Error(), "host network") {
		t.Fatalf("host network overlap: %v", err)
	}
	cases := map[string]func(d *Desired){
		"address outside": func(d *Desired) { d.Peers[0].Address = "10.67.0.2" },
		"server address":  func(d *Desired) { d.Peers[0].Address = "10.66.0.1" },
		"duplicate key":   func(d *Desired) { d.Peers[1].PublicKey = d.Peers[0].PublicKey },
		"bad name":        func(d *Desired) { d.Peers[0].Name = "x\n[Peer]" },
		"bad psk":         func(d *Desired) { d.Peers[0].PresharedKey = "abc" },
		"subnet overlap":  func(d *Desired) { d.Peers[0].Subnets = []string{"192.168.10.0/25"} },
		"input elsewhere": func(d *Desired) {
			d.Rules = []FwRule{{ID: "r", Chain: ChainInput, Src: []string{"10.66.0.2/32"}, Dst: []string{"10.66.0.7/32"}, Proto: "any"}}
		},
		"forward internet": func(d *Desired) {
			d.Rules = []FwRule{{ID: "r", Chain: ChainForward, Src: []string{"10.66.0.2/32"}, Dst: []string{"8.8.8.8/32"}, Proto: "any"}}
		},
		"source internet": func(d *Desired) {
			d.Rules = []FwRule{{ID: "r", Chain: ChainInput, Src: []string{"0.0.0.0/0"}, Dst: []string{"10.66.0.1/32"}, Proto: "any"}}
		},
		"rule id injection": func(d *Desired) {
			d.Rules = []FwRule{{ID: "r\" accept", Chain: ChainInput, Src: []string{"10.66.0.2/32"}, Dst: []string{"10.66.0.1/32"}, Proto: "any"}}
		},
		"bad ports": func(d *Desired) {
			d.Rules = []FwRule{{ID: "r", Chain: ChainInput, Src: []string{"10.66.0.2/32"}, Dst: []string{"10.66.0.1/32"}, Proto: "tcp", Ports: []string{"1883; drop"}}}
		},
	}
	for name, mut := range cases {
		d := Compile(srv, peers, nil, time.Now())
		mut(&d)
		if err := ValidateDesired(&d, srv, nil); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestRenderNft(t *testing.T) {
	srv, peers, _ := testWorld()
	rules := []Rule{
		{ID: "r1", Sources: []Source{{Type: SrcKind, Kind: KindDevice}}, DestType: DstServer, Protocol: "tcp", Ports: "1883", Enabled: true},
		{ID: "r2", Sources: []Source{{Type: SrcPeer, ID: "u1"}}, DestType: DstSite, DestPeerID: "s1", DestCIDR: "192.168.10.20/32", Protocol: "tcp", Ports: "102, 502", Enabled: true},
		{ID: "r3", Sources: []Source{{Type: SrcPeer, ID: "u1"}}, DestType: DstPeer, DestPeerID: "d1", Protocol: "udp", Enabled: true},
		{ID: "r4", Sources: []Source{{Type: SrcAll}}, DestType: DstServer, Protocol: "any", Enabled: true},
	}
	d := Compile(srv, peers, rules, time.Now())
	out := RenderNft(srv.Addr(), d.Rules)
	for _, want := range []string{
		"table inet ctrl32_vpn\ndelete table inet ctrl32_vpn\ntable inet ctrl32_vpn {",
		"type filter hook input priority filter - 10; policy accept;",
		`iifname != "wg0" accept`,
		`iifname != "wg0" oifname != "wg0" accept`,
		"ct state established,related accept",
		`ip saddr 10.66.0.3/32 ip daddr 10.66.0.1/32 tcp dport 1883 counter accept comment "rule r1"`,
		`iifname "wg0" oifname "wg0" ip saddr 10.66.0.2/32 ip daddr 192.168.10.20/32 tcp dport { 102, 502 } counter accept comment "rule r2"`,
		`ip daddr 10.66.0.3/32 meta l4proto udp counter accept comment "rule r3"`,
		`ip daddr 10.66.0.1/32 counter accept comment "rule r4"`,
		`counter drop comment "default deny"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	// the default deny is the last rule of both chains
	if strings.Count(out, `counter drop comment "default deny"`+"\n\t}") != 2 {
		t.Errorf("default deny not last:\n%s", out)
	}
	// no rules: everything from the VPN dropped except ping of the server
	empty := RenderNft(srv.Addr(), nil)
	if strings.Contains(empty, "comment \"rule") || !strings.Contains(empty, "icmp type echo-request") {
		t.Errorf("empty ruleset:\n%s", empty)
	}
}

func TestServerAndClientConf(t *testing.T) {
	srv, peers, _ := testWorld()
	d := Compile(srv, peers, nil, time.Now())
	d.Peers[2].PresharedKey, _ = GeneratePSK()
	conf := ServerConf("10.66.0.1/24", 51820, "PRIV", "/etc/ctrl32-telemetry/wireguard/vpn.nft", true, d.Peers)
	for _, want := range []string{"PreUp = nft -f /etc/ctrl32-telemetry/wireguard/vpn.nft", "PostUp = sysctl -q -w net.ipv4.conf.%i.forwarding=1", "PostDown = nft delete table inet ctrl32_vpn",
		"AllowedIPs = 10.66.0.4/32, 192.168.10.0/24, 192.168.11.0/24", "# plant-1"} {
		if !strings.Contains(conf, want) {
			t.Errorf("server conf misses %q:\n%s", want, conf)
		}
	}
	if strings.Contains(conf, "old-laptop") || strings.Contains(conf, "contractor") {
		t.Errorf("inactive peers configured:\n%s", conf)
	}
	if strings.Count(conf, "PresharedKey") != 1 {
		t.Errorf("psk lines:\n%s", conf)
	}
	c := Client{Name: "plant-1", Kind: KindSite, PrivateKey: "CPRIV", Address: "10.66.0.4", VPNNet: "10.66.0.0/24", VPNBits: 24, ServerPublicKey: "SPUB", PresharedKey: "PSK", Endpoint: "vpn.example.com:51820",
		AllowedIPs: []string{"10.66.0.0/24", "192.168.30.0/24"}, Subnets: []string{"192.168.10.0/24"}}
	cc := ClientConf(c)
	if !strings.Contains(cc, "Address = 10.66.0.4/32") || !strings.Contains(cc, "Endpoint = vpn.example.com:51820") || !strings.Contains(cc, "AllowedIPs = 10.66.0.0/24, 192.168.30.0/24") {
		t.Errorf("client conf:\n%s", cc)
	}
	tp := RouterTemplates(c)
	for k, want := range map[string]string{
		"wg_quick": "# PostUp = iptables -t nat -A POSTROUTING -s 10.66.0.0/24 -o eth0 -j MASQUERADE",
		"openwrt":  "uci set network.ctrl32hub.endpoint_host='vpn.example.com'",
		"mikrotik": "/ip route add dst-address=192.168.30.0/24 gateway=ctrl32-vpn",
	} {
		if !strings.Contains(tp[k], want) || !strings.Contains(tp[k], "CPRIV") {
			t.Errorf("%s template misses %q:\n%s", k, want, tp[k])
		}
	}
}

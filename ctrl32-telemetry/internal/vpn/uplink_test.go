package vpn

import (
	"net/netip"
	"strings"
	"testing"
)

func testToken(t *testing.T) UplinkToken {
	t.Helper()
	priv, _, _ := GenerateKey()
	_, hub, _ := GenerateKey()
	psk, _ := GeneratePSK()
	key, _, _ := GenerateTunnelKey()
	return UplinkToken{Name: "Customer A server", Endpoint: "hub.example.com:51820", HubPublicKey: hub, PresharedKey: psk, PrivateKey: priv,
		Address: "10.99.0.7", VPNNet: "10.99.0.0/16", HubAddress: "10.99.0.1", Subnets: []string{"192.168.1.0/24"},
		TunnelURL: "https://hub.example.com" + UplinkTunnelPath, TunnelKey: key}
}

func TestUplinkTokenRoundTrip(t *testing.T) {
	tok := testToken(t)
	s := EncodeUplinkToken(tok)
	if !strings.HasPrefix(s, "ctrl32-uplink-v1.") {
		t.Fatal(s)
	}
	// pasted with line breaks and spaces
	wrapped := s[:40] + "\n  " + s[40:80] + " \r\n" + s[80:]
	got, err := ParseUplinkToken(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if got.PrivateKey != tok.PrivateKey || got.Prefix().String() != "10.99.0.7/16" || got.Subnets[0] != "192.168.1.0/24" || got.TunnelKey != tok.TunnelKey {
		t.Fatalf("%+v", got)
	}
}

func TestUplinkTokenRejects(t *testing.T) {
	for name, mod := range map[string]func(*UplinkToken){
		"bad name":            func(u *UplinkToken) { u.Name = "" },
		"endpoint no port":    func(u *UplinkToken) { u.Endpoint = "hub.example.com" },
		"endpoint injection":  func(u *UplinkToken) { u.Endpoint = "hub'; rm -rf /:51820" },
		"endpoint port":       func(u *UplinkToken) { u.Endpoint = "hub.example.com:70000" },
		"bad key":             func(u *UplinkToken) { u.PrivateKey = "x" },
		"hub key is own key":  func(u *UplinkToken) { u.HubPublicKey, _ = PublicKey(u.PrivateKey) },
		"range not canonical": func(u *UplinkToken) { u.VPNNet = "10.99.0.1/16" },
		"range too large":     func(u *UplinkToken) { u.VPNNet = "10.0.0.0/7" },
		"range reserved":      func(u *UplinkToken) { u.VPNNet, u.Address, u.HubAddress = "127.0.0.0/8", "127.0.0.7", "127.0.0.1" },
		"address outside":     func(u *UplinkToken) { u.Address = "10.98.0.7" },
		"address is hub":      func(u *UplinkToken) { u.Address = "10.99.0.1" },
		"address network":     func(u *UplinkToken) { u.Address = "10.99.0.0" },
		"subnet overlaps vpn": func(u *UplinkToken) { u.Subnets = []string{"10.99.5.0/24"} },
		"default route":       func(u *UplinkToken) { u.Subnets = []string{"0.0.0.0/0"} },
		"tunnel http":         func(u *UplinkToken) { u.TunnelURL = "http://hub.example.com/x" },
		"tunnel without key":  func(u *UplinkToken) { u.TunnelKey = "" },
		"tunnel key":          func(u *UplinkToken) { u.TunnelKey = "short" },
		"tunnel userinfo":     func(u *UplinkToken) { u.TunnelURL = "https://a:b@hub.example.com/x" },
	} {
		tok := testToken(t)
		mod(&tok)
		if _, err := ParseUplinkToken(EncodeUplinkToken(tok)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	for _, s := range []string{"", "wg-quick conf", "ctrl32-uplink-v1.!!!", "ctrl32-uplink-v1." + strings.Repeat("A", 9000)} {
		if _, err := ParseUplinkToken(s); err == nil {
			t.Errorf("%q accepted", s[:min(len(s), 30)])
		}
	}
	// unknown fields are refused (a newer hub's token is not half-understood)
	tok := testToken(t)
	s := EncodeUplinkToken(tok)
	if _, err := ParseUplinkToken(s); err != nil {
		t.Fatal(err)
	}
	// http to loopback is allowed (tests), without a tunnel the key must be empty too
	tok.TunnelURL = "http://127.0.0.1:8080" + UplinkTunnelPath
	if _, err := ParseUplinkToken(EncodeUplinkToken(tok)); err != nil {
		t.Fatal(err)
	}
	tok.TunnelURL, tok.TunnelKey, tok.Subnets = "", "", nil
	if _, err := ParseUplinkToken(EncodeUplinkToken(tok)); err != nil {
		t.Fatalf("uplink without networks and without HTTPS transport: %v", err)
	}
}

func TestCheckUplinkLocal(t *testing.T) {
	tok := testToken(t)
	if err := CheckUplinkLocal(tok, []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")}, netip.MustParsePrefix("10.66.0.1/24"), nil); err != nil {
		t.Fatalf("LAN subnet is expected to be local: %v", err)
	}
	if err := CheckUplinkLocal(tok, []netip.Prefix{netip.MustParsePrefix("10.99.3.0/24")}, netip.Prefix{}, nil); err == nil || !strings.Contains(err.Error(), "host network") {
		t.Fatalf("host network: %v", err)
	}
	if err := CheckUplinkLocal(tok, nil, netip.MustParsePrefix("10.99.0.1/24"), nil); err == nil || !strings.Contains(err.Error(), "own VPN range") {
		t.Fatalf("own VPN: %v", err)
	}
	if err := CheckUplinkLocal(tok, nil, netip.Prefix{}, []netip.Prefix{netip.MustParsePrefix("10.99.200.0/24")}); err == nil {
		t.Fatal("own site subnet accepted")
	}
}

func uplinkInfo() UplinkInfo {
	return UplinkInfo{Address: netip.MustParseAddr("10.99.0.7"), VPNNet: netip.MustParsePrefix("10.99.0.0/16"),
		Subnets: []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")}}
}

func TestValidateUplinkRule(t *testing.T) {
	u := uplinkInfo()
	r := UplinkRule{Name: "web", SrcCIDR: "10.99.0.5", DestType: UplinkDstServer, DestCIDR: "x", Protocol: "tcp", Ports: "443, 80"}
	if err := ValidateUplinkRule(&r, u); err != nil || r.SrcCIDR != "10.99.0.5/32" || r.DestCIDR != "" || r.Ports != "80, 443" {
		t.Fatalf("%+v %v", r, err)
	}
	for name, r := range map[string]UplinkRule{
		"no name":          {DestType: UplinkDstServer, Protocol: "any"},
		"source outside":   {Name: "x", SrcCIDR: "10.98.0.5", DestType: UplinkDstServer, Protocol: "any"},
		"source not canon": {Name: "x", SrcCIDR: "10.99.0.5/16", DestType: UplinkDstServer, Protocol: "any"},
		"lan outside":      {Name: "x", DestType: UplinkDstLAN, DestCIDR: "192.168.2.10", Protocol: "tcp", Ports: "502"},
		"lan default":      {Name: "x", DestType: UplinkDstLAN, DestCIDR: "0.0.0.0/0", Protocol: "any"},
		"bad dest":         {Name: "x", DestType: "site", Protocol: "any"},
		"icmp ports":       {Name: "x", DestType: UplinkDstServer, Protocol: "icmp", Ports: "1"},
		"bad proto":        {Name: "x", DestType: UplinkDstServer, Protocol: "gre"},
	} {
		if err := ValidateUplinkRule(&r, u); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// without a connected uplink only the syntax is checked
	r = UplinkRule{Name: "plc", SrcCIDR: "10.50.0.0/24", DestType: UplinkDstLAN, DestCIDR: "172.16.0.5", Protocol: "tcp", Ports: "102"}
	if err := ValidateUplinkRule(&r, UplinkInfo{}); err != nil || r.DestCIDR != "172.16.0.5/32" {
		t.Fatalf("%+v %v", r, err)
	}
	// a connected uplink without networks: a LAN rule has nothing to reach
	noLAN := u
	noLAN.Subnets = nil
	r = UplinkRule{Name: "lan", DestType: UplinkDstLAN, Protocol: "any"}
	if err := ValidateUplinkRule(&r, noLAN); err == nil {
		t.Fatal("LAN rule without offered networks accepted")
	}
}

func TestCompileUplink(t *testing.T) {
	u := uplinkInfo()
	rules := []UplinkRule{
		{ID: "a", Name: "web", DestType: UplinkDstServer, Protocol: "tcp", Ports: "443", Enabled: true},
		{ID: "b", Name: "plc", SrcCIDR: "10.99.0.5/32", DestType: UplinkDstLAN, DestCIDR: "192.168.1.20/32", Protocol: "tcp", Ports: "102, 502", Enabled: true},
		{ID: "c", Name: "off", DestType: UplinkDstServer, Protocol: "any", Enabled: false},
		{ID: "d", Name: "stale", SrcCIDR: "10.50.0.0/24", DestType: UplinkDstServer, Protocol: "any", Enabled: true},
		{ID: "e", Name: "stale lan", DestType: UplinkDstLAN, DestCIDR: "172.16.0.5/32", Protocol: "any", Enabled: true},
		{ID: "f", Name: "lan ping", DestType: UplinkDstLAN, Protocol: "icmp", Enabled: true},
	}
	out, fwd := CompileUplink(u, rules)
	if !fwd || len(out) != 3 {
		t.Fatalf("%v %+v", fwd, out)
	}
	if out[0].Chain != ChainInput || out[0].Src[0] != "10.99.0.0/16" || out[0].Dst[0] != "10.99.0.7/32" || out[0].Ports[0] != "443" {
		t.Fatalf("input %+v", out[0])
	}
	if out[1].Chain != ChainForward || out[1].Src[0] != "10.99.0.5/32" || out[1].Dst[0] != "192.168.1.20/32" || len(out[1].Ports) != 2 {
		t.Fatalf("forward %+v", out[1])
	}
	if out[2].Dst[0] != "192.168.1.0/24" || out[2].Proto != "icmp" {
		t.Fatalf("all networks %+v", out[2])
	}
	if err := ValidateUplinkRules(out, u); err != nil {
		t.Fatal(err)
	}
	if o, f := CompileUplink(UplinkInfo{}, rules); len(o) != 0 || f {
		t.Fatal("rules without an uplink")
	}
	if _, f := CompileUplink(u, rules[:1]); f {
		t.Fatal("forwarding without a LAN rule")
	}
	// the agent refuses what the web service must never send
	for name, bad := range map[string]FwRule{
		"src outside":   {ID: "x", Chain: ChainInput, Src: []string{"10.98.0.0/16"}, Dst: []string{"10.99.0.7/32"}, Proto: "any"},
		"input other":   {ID: "x", Chain: ChainInput, Src: []string{"10.99.0.0/16"}, Dst: []string{"10.99.0.8/32"}, Proto: "any"},
		"forward wider": {ID: "x", Chain: ChainForward, Src: []string{"10.99.0.0/16"}, Dst: []string{"192.168.0.0/16"}, Proto: "any"},
		"forward vpn":   {ID: "x", Chain: ChainForward, Src: []string{"10.99.0.0/16"}, Dst: []string{"10.99.0.8/32"}, Proto: "any"},
		"bad id":        {ID: "x y", Chain: ChainInput, Src: []string{"10.99.0.0/16"}, Dst: []string{"10.99.0.7/32"}, Proto: "any"},
		"bad chain":     {ID: "x", Chain: "output", Src: []string{"10.99.0.0/16"}, Dst: []string{"10.99.0.7/32"}, Proto: "any"},
		"bad ports":     {ID: "x", Chain: ChainInput, Src: []string{"10.99.0.0/16"}, Dst: []string{"10.99.0.7/32"}, Proto: "tcp", Ports: []string{"0"}},
	} {
		if err := ValidateUplinkRules([]FwRule{bad}, u); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestRenderUplink(t *testing.T) {
	u := uplinkInfo()
	out, fwd := CompileUplink(u, []UplinkRule{
		{ID: "a", Name: "web", DestType: UplinkDstServer, Protocol: "tcp", Ports: "443", Enabled: true},
		{ID: "b", Name: "plc", DestType: UplinkDstLAN, DestCIDR: "192.168.1.20", Protocol: "tcp", Ports: "502", Enabled: true},
	})
	nft := RenderUplinkNft(u.Address, out, []string{"eth0", "bad name;", "wg1"})
	for _, want := range []string{
		"table inet ctrl32_uplink\ndelete table inet ctrl32_uplink\n",
		`iifname != "wg1" accept`,
		"ip daddr 10.99.0.7 icmp type echo-request accept",
		`ip saddr 10.99.0.0/16 ip daddr 10.99.0.7/32 tcp dport 443 counter accept comment "rule a"`,
		`iifname "eth0" oifname != "wg1" counter drop`,
		`iifname "wg1" oifname != "wg1" ip saddr 10.99.0.0/16 ip daddr 192.168.1.20/32 tcp dport 502 counter accept comment "rule b"`,
		`iifname "wg1" oifname != "wg1" masquerade`,
		`counter drop comment "default deny"`,
	} {
		if !strings.Contains(nft, want) {
			t.Errorf("missing %q in\n%s", want, nft)
		}
	}
	if strings.Contains(nft, "bad name") || strings.Count(nft, `iifname "wg1" oifname != "wg1" counter drop`) != 0 {
		t.Fatalf("invalid interface names must be left out:\n%s", nft)
	}
	// the guard comes before the "not ours" accept, the rules after established
	if strings.Index(nft, `iifname "eth0"`) > strings.Index(nft, `iifname != "wg1" oifname != "wg1" accept`) {
		t.Fatal("LAN guard after the accept of other traffic")
	}
	tok := testToken(t)
	conf := UplinkConf(tok, "127.0.0.1:40000", "/etc/ctrl32-telemetry/wireguard/uplink.nft", fwd, []string{"eth0"})
	for _, want := range []string{UplinkConfHeader, "Address = 10.99.0.7/32", "PrivateKey = " + tok.PrivateKey, "PreUp = nft -f /etc/ctrl32-telemetry/wireguard/uplink.nft",
		"PostUp = sysctl -q -w net.ipv4.conf.%i.forwarding=1", "PostUp = sysctl -q -w net.ipv4.conf.eth0.forwarding=1", "PostDown = nft delete table inet ctrl32_uplink",
		"PublicKey = " + tok.HubPublicKey, "PresharedKey = " + tok.PresharedKey, "AllowedIPs = 10.99.0.0/16", "Endpoint = 127.0.0.1:40000", "PersistentKeepalive = 25"} {
		if !strings.Contains(conf, want) {
			t.Errorf("missing %q in\n%s", want, conf)
		}
	}
	if strings.Contains(conf, "ListenPort") {
		t.Fatal("the uplink listens on no port")
	}
	if c := UplinkConf(tok, tok.Endpoint, "x.nft", false, []string{"eth0"}); strings.Contains(c, "eth0") || !strings.Contains(c, "forwarding=0") {
		t.Fatalf("no forwarding without LAN rules:\n%s", c)
	}
}

func TestUplinkPeerAllowedIPs(t *testing.T) {
	srv := netip.MustParsePrefix("10.99.0.1/16")
	up := mustPeer("u", "customer", KindSite, "10.99.0.7", "192.168.1.0/24")
	up.Uplink = true
	site := mustPeer("s", "plant", KindSite, "10.99.0.8", "192.168.20.0/24")
	tech := mustPeer("t", "tech", KindUser, "10.99.0.9")
	peers := []Peer{up, site, tech}
	if got := ClientAllowedIPs(srv, KindSite, "u", peers); strings.Join(got, ",") != "10.99.0.0/16" {
		t.Fatalf("uplink routes only the hub's range: %v", got)
	}
	if got := ClientAllowedIPs(srv, KindUser, "t", peers); strings.Join(got, ",") != "10.99.0.0/16,192.168.1.0/24,192.168.20.0/24" {
		t.Fatalf("technicians route the uplink's LAN too: %v", got)
	}
	key, hash, err := GenerateTunnelKey()
	if err != nil || len(key) != 43 || HashTunnelKey(key) != hash || len(hash) != 64 {
		t.Fatalf("%q %q %v", key, hash, err)
	}
}

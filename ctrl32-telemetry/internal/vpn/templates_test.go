package vpn

import (
	"sort"
	"strings"
	"testing"
)

func tplClient() Client {
	return Client{Name: "Plant Nitra", Kind: KindSite, PrivateKey: "cHJpdmF0ZWtleXByaXZhdGVrZXlwcml2YXRla2V5MTI=", Address: "10.66.0.7",
		VPNNet: "10.66.0.0/24", VPNBits: 24, ServerPublicKey: "c2VydmVycHVia2V5c2VydmVycHVia2V5c2VydmVyMTI=",
		PresharedKey: "cHNrcHNrcHNrcHNrcHNrcHNrcHNrcHNrcHNrcHNrMTI=", Endpoint: "vpn.example.com:51820",
		AllowedIPs: []string{"10.66.0.0/24", "192.168.20.0/24"}, Subnets: []string{"192.168.10.0/24"}}
}

func keysOf(m map[string]string) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestRouterTemplateKeys(t *testing.T) {
	got := keysOf(RouterTemplates(tplClient()))
	want := append([]string(nil), RouterTemplateKeys...)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("keys %v, want %v", got, want)
	}
	gotC := keysOf(ClientTemplates(tplClient()))
	wantC := append([]string(nil), ClientTemplateKeys...)
	sort.Strings(wantC)
	if strings.Join(gotC, ",") != strings.Join(wantC, ",") {
		t.Fatalf("client keys %v, want %v", gotC, wantC)
	}
}

func TestRouterTemplatesContent(t *testing.T) {
	c := tplClient()
	tpl := RouterTemplates(c)
	for _, k := range RouterTemplateKeys {
		s := tpl[k]
		for _, want := range []string{c.PrivateKey, c.ServerPublicKey, c.PresharedKey, "vpn.example.com", "51820", "192.168.10.0/24", c.Address, "192.168.20.0/24"} {
			if !strings.Contains(s, want) {
				t.Errorf("%s: missing %q", k, want)
			}
		}
		for _, bad := range []string{"<", "TODO", "%s", "{{", "%!"} {
			if strings.Contains(s, bad) {
				t.Errorf("%s: placeholder leftover %q", k, bad)
			}
		}
		if s != RouterTemplates(c)[k] {
			t.Errorf("%s: not deterministic", k)
		}
	}
	if !strings.Contains(tpl["rutos"], "Services → VPN → WireGuard") || !strings.Contains(tpl["rutos"], "route_allowed_ips='1'") {
		t.Error("rutos: WebUI path or route option missing")
	}
	if !strings.Contains(tpl["edgerouter"], "configure\n") || !strings.Contains(tpl["edgerouter"], "commit\nsave\n") ||
		!strings.Contains(tpl["edgerouter"], "allowed-ips 10.66.0.0/24") {
		t.Error("edgerouter: structure")
	}
	if !strings.Contains(tpl["pfsense"], "Package Manager") || strings.Contains(tpl["opnsense"], "Package Manager") {
		t.Error("pfSense needs the package, OPNsense has WireGuard built in")
	}
	// without a preshared key nothing mentions one
	c.PresharedKey = ""
	for k, s := range RouterTemplates(c) {
		if strings.Contains(strings.ToLower(s), "preshared") || strings.Contains(s, "Pre-shared") {
			t.Errorf("%s: mentions a preshared key although none is set", k)
		}
	}
}

func TestClientTemplates(t *testing.T) {
	c := tplClient()
	c.Kind = KindUser
	c.Subnets = nil
	tpl := ClientTemplates(c)
	w := tpl["windows"]
	if !strings.Contains(w, ClientConf(c)) {
		t.Fatal("windows: must contain the wg-quick configuration")
	}
	for _, l := range strings.Split(strings.TrimSpace(strings.SplitN(w, "[Interface]", 2)[0]), "\n") {
		if !strings.HasPrefix(l, "#") {
			t.Errorf("windows: line before [Interface] is not a comment: %q", l)
		}
	}
	m := tpl["mobile"]
	if strings.Contains(m, c.PrivateKey) || strings.Contains(m, c.PresharedKey) {
		t.Error("mobile instructions must not contain secrets")
	}
	if !strings.Contains(m, "QR") || !strings.Contains(m, c.Address) {
		t.Error("mobile: QR hint or address missing")
	}
}

package httpapi

import (
	"errors"
	"strings"
	"testing"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

func TestPermOKAnyOf(t *testing.T) {
	admin := &iam.Principal{Permissions: map[string]bool{"system.admin": true}}
	orgVPN := &iam.Principal{Permissions: map[string]bool{"vpn.manage": true}}
	viewer := &iam.Principal{Permissions: map[string]bool{"data.read": true}}
	for _, c := range []struct {
		p    *iam.Principal
		perm string
		want bool
	}{
		{viewer, "", true},
		{admin, "system.admin|vpn.manage", true},
		{orgVPN, "system.admin|vpn.manage", true},
		{viewer, "system.admin|vpn.manage", false},
		{orgVPN, "system.admin", false},
		{admin, "system.admin", true},
	} {
		if got := permOK(c.p, c.perm); got != c.want {
			t.Errorf("permOK(%v, %q) = %v", c.p.Permissions, c.perm, got)
		}
	}
}

func TestVPNScope(t *testing.T) {
	srv := vpnScopeOf(&iam.Principal{OrgID: "o1", Permissions: map[string]bool{"system.admin": true, "vpn.manage": true}})
	if !srv.server || !srv.owns("") || !srv.owns("o2") {
		t.Fatal("a server administrator manages everything")
	}
	org := vpnScopeOf(&iam.Principal{OrgID: "o1", Permissions: map[string]bool{"vpn.manage": true}})
	if org.server || !org.owns("o1") || org.owns("o2") || org.owns("") {
		t.Fatal("an organization's VPN manager manages the own organization only (not the whole server)")
	}
}

func TestCheckOrgRule(t *testing.T) {
	c := &config.Config{}
	c.HTTP.Listen, c.HTTP.BaseURL = ":8080", "http://plant:8080"
	s := &Server{cfg: c}
	peers := map[string]vpnPeerRow{
		"mine":  {Peer: vpn.Peer{ID: "mine", Kind: vpn.KindUser, OrgID: "o1"}},
		"site":  {Peer: vpn.Peer{ID: "site", Kind: vpn.KindSite, OrgID: "o1"}},
		"other": {Peer: vpn.Peer{ID: "other", Kind: vpn.KindSite, OrgID: "o2"}},
		"srv":   {Peer: vpn.Peer{ID: "srv", Kind: vpn.KindDevice}},
	}
	code := func(err error) int {
		var ve *vpnErr
		if errors.As(err, &ve) {
			return ve.code
		}
		if err != nil {
			return 500
		}
		return 0
	}
	for _, tc := range []struct {
		name string
		rule vpn.Rule
		want int
	}{
		{"own technician to own site", vpn.Rule{Sources: []vpn.Source{{Type: vpn.SrcPeer, ID: "mine"}}, DestType: vpn.DstSite, DestPeerID: "site", Protocol: "tcp", Ports: "502"}, 0},
		{"own organization to MQTT", vpn.Rule{Sources: []vpn.Source{{Type: vpn.SrcOrg, ID: "o1"}}, DestType: vpn.DstServer, Protocol: "tcp", Ports: "1883"}, 0},
		{"web interface", vpn.Rule{Sources: []vpn.Source{{Type: vpn.SrcOrg, ID: "o1"}}, DestType: vpn.DstServer, Protocol: "tcp", Ports: "8080"}, 0},
		{"ping the server", vpn.Rule{Sources: []vpn.Source{{Type: vpn.SrcPeer, ID: "mine"}}, DestType: vpn.DstServer, Protocol: "icmp"}, 0},
		{"SSH to the host", vpn.Rule{Sources: []vpn.Source{{Type: vpn.SrcPeer, ID: "mine"}}, DestType: vpn.DstServer, Protocol: "tcp", Ports: "22"}, 403},
		{"all ports of the server", vpn.Rule{Sources: []vpn.Source{{Type: vpn.SrcPeer, ID: "mine"}}, DestType: vpn.DstServer, Protocol: "tcp"}, 403},
		{"any protocol to the server", vpn.Rule{Sources: []vpn.Source{{Type: vpn.SrcPeer, ID: "mine"}}, DestType: vpn.DstServer, Protocol: "any"}, 403},
		{"another organization's site", vpn.Rule{Sources: []vpn.Source{{Type: vpn.SrcPeer, ID: "mine"}}, DestType: vpn.DstSite, DestPeerID: "other", Protocol: "any"}, 404},
		{"a server-wide peer as destination", vpn.Rule{Sources: []vpn.Source{{Type: vpn.SrcPeer, ID: "mine"}}, DestType: vpn.DstPeer, DestPeerID: "srv", Protocol: "any"}, 404},
		{"another organization's peer as source", vpn.Rule{Sources: []vpn.Source{{Type: vpn.SrcPeer, ID: "other"}}, DestType: vpn.DstSite, DestPeerID: "site", Protocol: "any"}, 404},
		{"another organization as source", vpn.Rule{Sources: []vpn.Source{{Type: vpn.SrcOrg, ID: "o2"}}, DestType: vpn.DstSite, DestPeerID: "site", Protocol: "any"}, 403},
		{"all peers as source", vpn.Rule{Sources: []vpn.Source{{Type: vpn.SrcAll}}, DestType: vpn.DstSite, DestPeerID: "site", Protocol: "any"}, 403},
		{"a kind as source", vpn.Rule{Sources: []vpn.Source{{Type: vpn.SrcKind, Kind: vpn.KindUser}}, DestType: vpn.DstSite, DestPeerID: "site", Protocol: "any"}, 403},
	} {
		r := tc.rule
		if got := code(s.checkOrgRule(&r, "o1", peers)); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestGrantDuration(t *testing.T) {
	for min, want := range map[int]string{120: "2 hours", 480: "8 hours", 1440: "1 day", 2880: "2 days", 60: "1 hour", 45: "45 minutes"} {
		if got := grantDuration(min); got != want {
			t.Errorf("%d: %q", min, got)
		}
	}
	if grantMinMinutes != 15 || grantMaxMinutes != 43200 || !strings.Contains(grantDuration(grantMaxMinutes), "30 days") {
		t.Error("grant window bounds")
	}
}

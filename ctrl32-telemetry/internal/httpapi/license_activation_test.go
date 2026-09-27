package httpapi

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/license"
)

func TestFetchActivation(t *testing.T) {
	onPub, onPriv, _ := ed25519.GenerateKey(rand.Reader)
	maPub, maPriv, _ := ed25519.GenerateKey(rand.Reader)
	keys := map[string]ed25519.PublicKey{"online": onPub, "master": maPub}
	limits := map[string]int{"online": 31}
	now := time.Now().UTC().Truncate(time.Second)
	sign := func(priv ed25519.PrivateKey, l license.License) string {
		l.ID, l.Licensee = "T-1", "Acme"
		if l.Features == nil {
			l.Features = []string{license.WhiteLabel}
		}
		tok, err := license.Sign(l, priv)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	var status int
	var reply map[string]any
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(reply)
	}))
	defer srv.Close()
	fetch := func() (*license.License, string, error) {
		l, _, kind, err := fetchActivation(context.Background(), srv.Client(), srv.URL, "test", "txn_1", "scada.acme.sk", keys, limits)
		return l, kind, err
	}

	status, reply = 200, map[string]any{"license": sign(onPriv, license.License{Issued: now, Expires: now.AddDate(0, 0, 30), Domains: []string{"scada.acme.sk"}}), "kind": "starter"}
	l, kind, err := fetch()
	if err != nil || kind != "starter" || l.KeyID != "online" || got["order"] != "txn_1" || got["host"] != "scada.acme.sk" {
		t.Fatalf("starter: %v %q %+v %v", err, kind, l, got)
	}
	status, reply = 200, map[string]any{"license": sign(maPriv, license.License{Issued: now, Domains: []string{"scada.acme.sk"}}), "kind": "permanent"}
	if l, kind, err = fetch(); err != nil || !l.Perpetual() || kind != "permanent" {
		t.Fatalf("permanent: %v %q", err, kind)
	}

	var ae *activationError
	// a perpetual token from the ONLINE key: refused, flagged as untrusted
	status, reply = 200, map[string]any{"license": sign(onPriv, license.License{Issued: now, Domains: []string{"scada.acme.sk"}}), "kind": "permanent"}
	if _, _, err = fetch(); !errors.As(err, &ae) || !ae.Untrusted || !strings.Contains(err.Error(), "online activation key") {
		t.Fatalf("perpetual online token accepted: %v", err)
	}
	// another server's license, a foreign key, no white_label
	for name, tok := range map[string]string{
		"other host": sign(maPriv, license.License{Issued: now, Domains: []string{"evil.sk"}}),
		"foreign key": func() string {
			_, p, _ := ed25519.GenerateKey(rand.Reader)
			return sign(p, license.License{Issued: now})
		}(),
		"no feature": sign(maPriv, license.License{Issued: now, Features: []string{"other"}}),
	} {
		status, reply = 200, map[string]any{"license": tok}
		if _, _, err = fetch(); !errors.As(err, &ae) || !ae.Untrusted {
			t.Fatalf("%s accepted: %v", name, err)
		}
	}
	// refusals stop the renewal, temporary errors do not
	for code, st := range map[string]int{"refunded": 410, "claimed-by-editor": 409, "host-cap": 409, "unknown-order": 404, "unpaid": 402} {
		status, reply = st, map[string]any{"error": code}
		if _, _, err = fetch(); !errors.As(err, &ae) || !ae.Refusal || ae.Code != code {
			t.Fatalf("%s: %v", code, err)
		}
	}
	if status, reply = 409, map[string]any{"error": "claimed-by-editor"}; true {
		_, _, err = fetch()
		if !strings.Contains(err.Error(), "editor") {
			t.Fatalf("message %v", err)
		}
	}
	for code, st := range map[string]int{"pending": 503, "server": 500, "not-configured": 503} {
		status, reply = st, map[string]any{"error": code}
		if _, _, err = fetch(); !errors.As(err, &ae) || ae.Refusal {
			t.Fatalf("%s must be temporary: %v", code, err)
		}
	}
	// not reachable
	_, _, _, err = fetchActivation(context.Background(), srv.Client(), "http://127.0.0.1:1/x", "test", "o", "h", keys, limits)
	if !errors.As(err, &ae) || ae.Refusal || ae.Status != 0 {
		t.Fatalf("unreachable: %v", err)
	}
}

func TestRenewDueAndBetter(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	host := "scada.acme.sk"
	lic := func(days int) *license.License {
		l := &license.License{Issued: now.AddDate(0, 0, -5), Features: []string{license.WhiteLabel}}
		if days != 0 {
			l.Expires = now.AddDate(0, 0, days)
		}
		return l
	}
	ago := func(h int) *time.Time { t := now.Add(-time.Duration(h) * time.Hour); return &t }
	a := &activation{Order: "o"}
	cases := []struct {
		name string
		a    *activation
		cur  *license.License
		want bool
	}{
		{"no order", nil, lic(10), false},
		{"expires in 25 days", a, lic(25), false},
		{"expires in 19 days, never tried", a, lic(19), true},
		{"expired", a, lic(-3), true},
		{"no license", a, nil, true},
		{"perpetual", a, lic(0), false},
		{"tried 2 hours ago", &activation{Order: "o", LastAttempt: ago(2)}, lic(10), false},
		{"tried a day ago", &activation{Order: "o", LastAttempt: ago(24)}, lic(10), true},
		{"refused", &activation{Order: "o", Refused: true}, lic(3), false},
	}
	for _, c := range cases {
		if got := renewDue(c.a, c.cur, now); got != c.want {
			t.Errorf("renewDue %s = %v", c.name, got)
		}
	}
	if !betterLicense(nil, lic(30), host, now) || !betterLicense(lic(10), lic(30), host, now) || !betterLicense(lic(10), lic(0), host, now) {
		t.Fatal("a longer or perpetual license must replace")
	}
	if betterLicense(lic(0), lic(30), host, now) || betterLicense(lic(30), lic(20), host, now) {
		t.Fatal("never shorten, never replace a perpetual one")
	}
	if !betterLicense(lic(-1), lic(20), host, now) {
		t.Fatal("an expired license is replaced")
	}
	if !validOrderRef("txn_01h2") || validOrderRef("") || validOrderRef(strings.Repeat("x", 129)) || validOrderRef("a\nb") || validOrderRef("č") {
		t.Fatal("order reference check")
	}
}

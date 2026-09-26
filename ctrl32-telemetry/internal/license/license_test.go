package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestSignParseCheck(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	keys := map[string]ed25519.PublicKey{"test": pub}
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	tok, err := Sign(License{ID: "L-1", Licensee: "Acme s.r.o.", Issued: now, Expires: now.AddDate(1, 0, 0), Features: []string{WhiteLabel},
		Domains: []string{"scada.acme.sk", "*.acme.cz"}}, priv)
	if err != nil {
		t.Fatal(err)
	}
	l, err := Parse(tok[:40]+"\n  "+tok[40:], keys) // wrapped when pasted
	if err != nil || l.Licensee != "Acme s.r.o." || l.KeyID != "test" {
		t.Fatalf("parse %+v %v", l, err)
	}
	if !l.Has(WhiteLabel, "scada.acme.sk:443", now) || !l.Has(WhiteLabel, "a.b.acme.cz", now) {
		t.Fatal("feature on allowed hosts")
	}
	if l.Has(WhiteLabel, "acme.cz", now) || l.Has(WhiteLabel, "evil.sk", now) || l.Has(WhiteLabel, "127.0.0.1", now) {
		t.Fatal("feature on other hosts")
	}
	if l.Has("other", "scada.acme.sk", now) {
		t.Fatal("feature not granted")
	}
	if st := l.Check("scada.acme.sk", now.AddDate(1, 0, 1)); st.Valid || !strings.Contains(st.Problem, "expired") {
		t.Fatalf("expiry %+v", st)
	}
	if _, err := Parse(tok, map[string]ed25519.PublicKey{"x": other}); err != ErrSignature {
		t.Fatalf("other key: %v", err)
	}
	// tampered payload
	parts := strings.Split(tok, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	forged := strings.Replace(string(raw), "Acme", "Evil", 1)
	if _, err := Parse(parts[0]+"."+base64.RawURLEncoding.EncodeToString([]byte(forged))+"."+parts[2], keys); err != ErrSignature {
		t.Fatalf("tampered: %v", err)
	}
	for _, bad := range []string{"", "abc", "c32l1.", "c32l1.x.y", strings.Repeat("c", 9000)} {
		if _, err := Parse(bad, keys); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	// perpetual, any domain
	tok2, _ := Sign(License{ID: "L-2", Licensee: "B", Features: []string{WhiteLabel}}, priv)
	l2, _ := Parse(tok2, keys)
	if !l2.Has(WhiteLabel, "localhost", now.AddDate(50, 0, 0)) {
		t.Fatal("perpetual unrestricted")
	}
	var nilL *License
	if nilL.Has(WhiteLabel, "x", now) {
		t.Fatal("nil license")
	}
	if _, err := Sign(License{ID: "x"}, priv); err == nil {
		t.Fatal("empty license signed")
	}
}

func TestTrustedKeysEmbedded(t *testing.T) {
	// release builds may have no key yet; the README must not count as a key
	for id := range TrustedKeys() {
		if id == "README" {
			t.Fatal("README parsed as key")
		}
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	extraKey = base64.StdEncoding.EncodeToString(pub)
	defer func() { extraKey = "" }()
	if _, ok := TrustedKeys()["build"]; !ok {
		t.Fatal("build key")
	}
}

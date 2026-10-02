package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
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
	// no-expiry, any domain
	tok2, _ := Sign(License{ID: "L-2", Licensee: "B", Features: []string{WhiteLabel}}, priv)
	l2, _ := Parse(tok2, keys)
	if !l2.Has(WhiteLabel, "localhost", now.AddDate(50, 0, 0)) {
		t.Fatal("no-expiry unrestricted")
	}
	if !l2.NoExpiry() || l.NoExpiry() {
		t.Fatal("no-expiry flag")
	}
	if raw, _ := base64.RawURLEncoding.DecodeString(strings.Split(tok2, ".")[1]); strings.Contains(string(raw), "expires") {
		t.Fatalf("no-expiry token carries an expiry: %s", raw)
	}
	if st := l2.Check("localhost", now.AddDate(100, 0, 0)); !st.Valid {
		t.Fatalf("no-expiry after 100 years: %+v", st)
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
	if _, ok := TrustedKeys()["telemetry-digital-2026"]; !ok {
		t.Fatal("production licensor key not embedded")
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	extraKey = base64.StdEncoding.EncodeToString(pub)
	defer func() { extraKey = "" }()
	if _, ok := TrustedKeys()["build"]; !ok {
		t.Fatal("build key")
	}
}

func TestOnlineKeyLimit(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	keys := map[string]ed25519.PublicKey{"online": pub}
	limits := map[string]int{"online": 31}
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	sign := func(l License) string {
		l.ID, l.Licensee, l.Features = "T-1", "Acme", []string{WhiteLabel}
		tok, err := Sign(l, priv)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	if l, err := ParseLimited(sign(License{Issued: now, Expires: now.AddDate(0, 0, 30)}), keys, limits); err != nil || l.KeyID != "online" {
		t.Fatalf("30-day online license refused: %v", err)
	}
	if _, err := ParseLimited(sign(License{Issued: now, Expires: now.AddDate(0, 0, 31)}), keys, limits); err != nil {
		t.Fatalf("31-day online license refused: %v", err)
	}
	for name, l := range map[string]License{
		"no-expiry":  {Issued: now},
		"32 days":    {Issued: now, Expires: now.AddDate(0, 0, 32)},
		"a year":     {Issued: now, Expires: now.AddDate(1, 0, 0)},
		"no issue":   {Expires: now.AddDate(0, 0, 10)},
		"back-dated": {Issued: now.AddDate(0, 0, -400), Expires: now.AddDate(0, 0, 20)},
	} {
		if _, err := ParseLimited(sign(l), keys, limits); !errors.Is(err, ErrOnlineTerm) {
			t.Fatalf("%s: online key accepted it (%v)", name, err)
		}
	}
	// the same no-expiry token is fine from a key without a limit (the master key)
	if _, err := ParseLimited(sign(License{Issued: now}), keys, nil); err != nil {
		t.Fatalf("no-expiry from the master key: %v", err)
	}
}

func TestTrustedOnlineKeyDeclared(t *testing.T) {
	if got := KeyLimits()["telemetry-online-2026"]; got != OnlineMaxDays {
		t.Fatalf("telemetry-online-2026 max-days = %d, want %d", got, OnlineMaxDays)
	}
	if _, ok := KeyLimits()["telemetry-digital-2026"]; ok {
		t.Fatal("the master key must have no limit")
	}
	if _, ok := TrustedKeys()["telemetry-online-2026"]; !ok {
		t.Fatal("online key not embedded")
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	extraOnlineKey = base64.StdEncoding.EncodeToString(pub)
	defer func() { extraOnlineKey = "" }()
	if KeyLimits()["build-online"] != OnlineMaxDays {
		t.Fatal("build-online key without a limit")
	}
	for _, bad := range []string{"", "# max-days: x\nKj2BjPNu72LggB4XsC7ZKL9vxzGDpD0YOmdPRSirCjI=", "# max-days: 0\nKj2BjPNu72LggB4XsC7ZKL9vxzGDpD0YOmdPRSirCjI=", "nope"} {
		if _, err := ParseKeyFile([]byte(bad)); err == nil {
			t.Fatalf("ParseKeyFile accepted %q", bad)
		}
	}
	kf, err := ParseKeyFile([]byte("# x\n#  max-days: 7\nKj2BjPNu72LggB4XsC7ZKL9vxzGDpD0YOmdPRSirCjI=\n"))
	if err != nil || kf.MaxDays != 7 {
		t.Fatalf("key file %+v %v", kf, err)
	}
}

func TestUpdatesUntil(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	keys := map[string]ed25519.PublicKey{"k": pub}
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	tok, err := Sign(License{ID: "U-1", Licensee: "Acme", Issued: now, Features: []string{WhiteLabel}, UpdatesUntil: "2027-10-02"}, priv)
	if err != nil {
		t.Fatal(err)
	}
	l, err := Parse(tok, keys)
	if err != nil {
		t.Fatal(err)
	}
	if d, ok := l.UpdatesEnd(); !ok || d.Format(DateLayout) != "2027-10-02" {
		t.Fatalf("updates end %v %v", d, ok)
	}
	if l.UpdatesEnded(time.Date(2027, 10, 2, 23, 59, 0, 0, time.UTC)) || !l.UpdatesEnded(time.Date(2027, 10, 3, 0, 0, 1, 0, time.UTC)) {
		t.Fatal("the last day is included, the next one is not")
	}
	// updates ending never makes the license invalid: the system keeps running with everything it has
	if !l.Has(WhiteLabel, "x", now.AddDate(30, 0, 0)) {
		t.Fatal("the license must stay valid after its updates ended")
	}
	// licenses issued before 0.66.0 have no updates_until: unlimited updates
	old, _ := Sign(License{ID: "U-2", Licensee: "Acme", Issued: now, Features: []string{WhiteLabel}}, priv)
	lo, _ := Parse(old, keys)
	if _, ok := lo.UpdatesEnd(); ok || lo.UpdatesEnded(now.AddDate(50, 0, 0)) {
		t.Fatal("no updates_until = unlimited")
	}
	if _, err := Sign(License{ID: "U-3", Licensee: "Acme", Features: []string{WhiteLabel}, UpdatesUntil: "next year"}, priv); err == nil {
		t.Fatal("bad date signed")
	}
	// an online key may include updates for at most OnlineMaxUpdateDays
	limits := map[string]int{"k": 31}
	year, _ := Sign(License{ID: "U-4", Licensee: "Acme", Issued: now, Expires: now.AddDate(0, 0, 30), Features: []string{WhiteLabel}, UpdatesUntil: now.AddDate(1, 0, 0).Format(DateLayout)}, priv)
	if _, err := ParseLimited(year, keys, limits); err != nil {
		t.Fatalf("a year of updates from the online key: %v", err)
	}
	long, _ := Sign(License{ID: "U-5", Licensee: "Acme", Issued: now, Expires: now.AddDate(0, 0, 30), Features: []string{WhiteLabel}, UpdatesUntil: now.AddDate(5, 0, 0).Format(DateLayout)}, priv)
	if _, err := ParseLimited(long, keys, limits); !errors.Is(err, ErrOnlineTerm) {
		t.Fatalf("five years of updates from the online key accepted: %v", err)
	}
}

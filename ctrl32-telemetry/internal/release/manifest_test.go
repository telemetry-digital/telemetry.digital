package release

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
)

func testKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func testManifest(t *testing.T) []byte {
	t.Helper()
	m := Manifest{Format: 1, Product: Product, Version: "0.76.0", Released: "2026-10-04", Files: []File{
		{Name: "ctrl32-telemetry-linux-amd64", Size: 3, SHA256: HexSHA256([]byte("abc"))},
		{Name: "SHA256SUMS", Size: 1, SHA256: HexSHA256([]byte("x"))},
		{Name: "analytics/yolox_nano.onnx", Size: 1, SHA256: HexSHA256([]byte("y"))},
	}}
	b, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSignVerify(t *testing.T) {
	pub, priv := testKey(t)
	body := testManifest(t)
	line, err := Sign(body, "test-1", priv)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]ed25519.PublicKey{"test-1": pub}
	id, err := VerifySignature(body, []byte(line+"\n"), keys)
	if err != nil || id != "test-1" {
		t.Fatalf("verify: %v %q", err, id)
	}
	m, err := ParseManifest(body)
	if err != nil || m.Version != "0.76.0" || len(m.Files) != 3 {
		t.Fatalf("parse: %v %+v", err, m)
	}
	if f, ok := m.File("analytics/yolox_nano.onnx"); !ok || f.Size != 1 {
		t.Fatal("file lookup")
	}
}

func TestTamperRefused(t *testing.T) {
	pub, priv := testKey(t)
	body := testManifest(t)
	line, _ := Sign(body, "test-1", priv)
	keys := map[string]ed25519.PublicKey{"test-1": pub}
	// one byte of the manifest changed: a different size claimed for the binary
	bad := []byte(strings.Replace(string(body), `"size": 3`, `"size": 4`, 1))
	if string(bad) == string(body) {
		t.Fatal("tamper did not change the manifest")
	}
	if _, err := VerifySignature(bad, []byte(line), keys); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("tampered manifest accepted: %v", err)
	}
	// a signature of another key under the trusted key's id
	_, other := testKey(t)
	forged, _ := Sign(body, "test-1", other)
	if _, err := VerifySignature(body, []byte(forged), keys); err == nil {
		t.Fatal("forged signature accepted")
	}
	// no signature, an unknown key, a malformed line
	if _, err := VerifySignature(body, []byte("\n# nothing\n"), keys); err != ErrUnsigned {
		t.Fatalf("unsigned: %v", err)
	}
	unknown, _ := Sign(body, "someone-else", other)
	if _, err := VerifySignature(body, []byte(unknown), keys); err == nil || !strings.Contains(err.Error(), "does not trust") {
		t.Fatalf("unknown key: %v", err)
	}
	if _, err := VerifySignature(body, []byte("ed25519 test-1"), keys); err == nil {
		t.Fatal("malformed line accepted")
	}
}

// TestKeyRotation: a release signed by the new and the old key verifies on servers that trust either; a server that
// trusts only a retired key refuses a release signed by the new key alone.
func TestKeyRotation(t *testing.T) {
	oldPub, oldPriv := testKey(t)
	newPub, newPriv := testKey(t)
	body := testManifest(t)
	l1, _ := Sign(body, "release-2026", oldPriv)
	l2, _ := Sign(body, "release-2027", newPriv)
	both := []byte(l2 + "\n" + l1 + "\n")
	if id, err := VerifySignature(body, both, map[string]ed25519.PublicKey{"release-2026": oldPub}); err != nil || id != "release-2026" {
		t.Fatalf("old server: %v %s", err, id)
	}
	if id, err := VerifySignature(body, both, map[string]ed25519.PublicKey{"release-2027": newPub}); err != nil || id != "release-2027" {
		t.Fatalf("new server: %v %s", err, id)
	}
	if _, err := VerifySignature(body, []byte(l2), map[string]ed25519.PublicKey{"release-2026": oldPub}); err == nil {
		t.Fatal("a server without the new key accepted a release signed by it alone")
	}
}

func TestParseManifestRefuses(t *testing.T) {
	good := `{"format":1,"product":"ctrl32-telemetry","version":"0.76.0","released":"2026-10-04","files":[{"name":"a","size":1,"sha256":"` + strings.Repeat("a", 64) + `"}]}`
	if _, err := ParseManifest([]byte(good)); err != nil {
		t.Fatal(err)
	}
	for why, s := range map[string]string{
		"format":       strings.Replace(good, `"format":1`, `"format":2`, 1),
		"product":      strings.Replace(good, `ctrl32-telemetry"`, `other"`, 1),
		"version":      strings.Replace(good, `0.76.0`, `latest`, 1),
		"date":         strings.Replace(good, `2026-10-04`, `04.10.2026`, 1),
		"path escape":  strings.Replace(good, `"name":"a"`, `"name":"../a"`, 1),
		"deep path":    strings.Replace(good, `"name":"a"`, `"name":"x/y/a"`, 1),
		"bad hash":     strings.Replace(good, strings.Repeat("a", 64), "zz", 1),
		"unknown":      strings.Replace(good, `"format":1`, `"format":1,"extra":true`, 1),
		"no files":     `{"format":1,"product":"ctrl32-telemetry","version":"0.76.0","released":"2026-10-04","files":[]}`,
		"duplicate":    strings.Replace(good, `}]}`, `},{"name":"a","size":1,"sha256":"`+strings.Repeat("a", 64)+`"}]}`, 1),
		"negative":     strings.Replace(good, `"size":1`, `"size":-1`, 1),
		"windows path": strings.Replace(good, `"name":"a"`, `"name":"analytics\\a"`, 1),
	} {
		if _, err := ParseManifest([]byte(s)); err == nil {
			t.Errorf("%s: accepted", why)
		}
	}
}

func TestCompareAndChange(t *testing.T) {
	cases := []struct{ a, b, change string }{
		{"0.75.0", "0.75.1", "patch"},
		{"0.75.3", "0.76.0", "minor"},
		{"0.76.0", "1.0.0", "major"},
		{"0.76.0", "0.76.0", "same"},
		{"0.76.1", "0.76.0", "older"},
		{"0.76.0-rc1", "0.76.0", "patch"},
		{"0.76.0", "bad", ""},
	}
	for _, c := range cases {
		if got := Change(c.a, c.b); got != c.change {
			t.Errorf("Change(%s, %s) = %q, want %q", c.a, c.b, got, c.change)
		}
	}
	if Compare("0.10.0", "0.9.9") != 1 || Compare("0.9.9", "0.10.0") != -1 {
		t.Fatal("numeric comparison")
	}
}

func TestEmbeddedReleaseKey(t *testing.T) {
	keys := TrustedKeys()
	k, ok := keys["release-2026"]
	if !ok {
		t.Fatal("the release key release-2026 is not embedded")
	}
	if Fingerprint(k) != "SHA256:iC83AVh5wuOics/M7GDv7w1mjNuIw332rNZ7DjZvPCE" {
		t.Fatalf("unexpected fingerprint %s", Fingerprint(k))
	}
}

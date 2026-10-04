package agent

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/release"
)

// a test key pair generated here; the release key is never used by tests
func testRelease(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

// buildPackage returns a zip with a manifest over files (signed with priv as key id "test" unless priv is nil) plus
// the extra members (not in the manifest).
func buildPackage(t *testing.T, priv ed25519.PrivateKey, files, extra map[string][]byte, tamper func(name string, b []byte) []byte) []byte {
	t.Helper()
	m := release.Manifest{Format: 1, Product: release.Product, Version: "9.9.9", Released: "2026-10-04"}
	for n, b := range files {
		m.Files = append(m.Files, release.File{Name: n, Size: int64(len(b)), SHA256: release.HexSHA256(b)})
	}
	mb, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(n string, b []byte) {
		w, _ := zw.Create(n)
		_, _ = w.Write(b)
	}
	add(release.ManifestFile, mb)
	if priv != nil {
		line, _ := release.Sign(mb, "test", priv)
		add(release.SignatureFile, []byte(line+"\n"))
	}
	for n, b := range files {
		if tamper != nil {
			b = tamper(n, b)
		}
		add(n, b)
	}
	for n, b := range extra {
		add(n, b)
	}
	_ = zw.Close()
	return buf.Bytes()
}

func packageServer(t *testing.T, pub ed25519.PublicKey) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	_ = os.WriteFile(cfg, []byte("[http]\nlisten = \":18080\"\n"), 0o640)
	a := NewServer(filepath.Join(dir, "a.sock"), cfg, "9.9.8", "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	a.SetSimulation(dir)
	a.testKeys = map[string]ed25519.PublicKey{"test": pub}
	return a, dir
}

func TestPackageStagingAndRefusals(t *testing.T) {
	pub, priv := testRelease(t)
	a, _ := packageServer(t, pub)
	bin := release.BinaryName(runtime.GOOS, runtime.GOARCH)
	files := map[string][]byte{bin: []byte("binary"), "SHA256SUMS": []byte("sums"), "analytics/yolox_nano.onnx": []byte("model")}

	info, e := a.stagePackage(bytes.NewReader(buildPackage(t, priv, files, nil, nil)), 1<<20)
	if e != nil {
		t.Fatalf("good package refused: %v", e)
	}
	if info.Version != "9.9.9" || !info.HasBinary || info.KeyID != "test" || len(info.Detector) != 1 || info.Files != 3 {
		t.Fatalf("info %+v", info)
	}
	if b, _ := os.ReadFile(filepath.Join(stagedDir(), "analytics", "yolox_nano.onnx")); string(b) != "model" {
		t.Fatal("detector part not staged")
	}
	if st := a.updateState(); st.Staged == nil || st.Staged.Version != "9.9.9" {
		t.Fatalf("staged state %+v", st.Staged)
	}

	_, other := testRelease(t)
	cases := map[string][]byte{
		"unsigned":         buildPackage(t, nil, files, nil, nil),
		"untrusted key":    buildPackage(t, other, files, nil, nil),
		"tampered member":  buildPackage(t, priv, files, nil, func(n string, b []byte) []byte { return append([]byte("X"), b[1:]...) }),
		"member too long":  buildPackage(t, priv, files, nil, func(n string, b []byte) []byte { return append(b, 'x') }),
		"extra member":     buildPackage(t, priv, files, map[string][]byte{"evil.sh": []byte("rm -rf /")}, nil),
		"path traversal":   buildPackage(t, priv, files, map[string][]byte{"../../etc/passwd": []byte("x")}, nil),
		"nested directory": buildPackage(t, priv, files, map[string][]byte{"a/b/c": []byte("x")}, nil),
		"not a zip":        []byte("this is not a package"),
	}
	for why, pkg := range cases {
		if _, e := a.stagePackage(bytes.NewReader(pkg), 1<<20); e == nil {
			t.Errorf("%s: accepted", why)
		}
	}
	// the staged release of the good package is still there after the refusals
	if b, _ := os.ReadFile(filepath.Join(stagedDir(), bin)); string(b) != "binary" {
		t.Fatal("a refused package replaced the staged one")
	}
	// size limit
	noise := make([]byte, 4096)
	_, _ = rand.Read(noise)
	big := buildPackage(t, priv, map[string][]byte{bin: noise}, nil, nil)
	if _, e := a.stagePackage(bytes.NewReader(big), 1024); e == nil || e.status != 413 {
		t.Fatalf("over the limit: %v", e)
	}
}

func TestVerifyPackageFileNeedsTrustedKey(t *testing.T) {
	_, priv := testRelease(t)
	pkg := buildPackage(t, priv, map[string][]byte{"SHA256SUMS": []byte("x")}, nil, nil)
	f := filepath.Join(t.TempDir(), "p.zip")
	_ = os.WriteFile(f, pkg, 0o644)
	// signed by a key this binary does not trust (only the release key is embedded)
	if _, err := VerifyPackageFile(f); err == nil || !strings.Contains(err.Error(), "does not trust") {
		t.Fatalf("package of an untrusted key: %v", err)
	}
}

// The online path refuses a distribution point without a signed manifest, a wrong version and a tampered binary
// before anything on the system changes.
func TestOnlineSourceRefusals(t *testing.T) {
	pub, priv := testRelease(t)
	a, _ := packageServer(t, pub)
	bin := release.BinaryName(runtime.GOOS, runtime.GOARCH)
	body := []byte("not really a binary")
	m := release.Manifest{Format: 1, Product: release.Product, Version: "9.9.9", Released: "2026-10-04", Files: []release.File{{Name: bin, Size: int64(len(body)), SHA256: release.HexSHA256(body)}}}
	mb, _ := m.Marshal()
	line, _ := release.Sign(mb, "test", priv)
	serve := map[string][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := serve[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
	defer srv.Close()
	src := httpSource{base: srv.URL}
	ctx := context.Background()
	if _, e := a.prepare(ctx, src, "9.9.9", ""); e == nil || e.code != "unsigned" {
		t.Fatalf("no manifest: %v", e)
	}
	serve[release.ManifestFile], serve[release.SignatureFile] = mb, []byte(line)
	if _, e := a.prepare(ctx, src, "9.9.10", ""); e == nil || e.code != "version" {
		t.Fatalf("other version: %v", e)
	}
	serve[bin] = []byte("not really a binarY")
	if _, e := a.prepare(ctx, src, "9.9.9", ""); e == nil || e.code != "checksum" {
		t.Fatalf("tampered binary: %v", e)
	}
	if _, err := os.Stat(binPath + ".new"); err == nil {
		t.Fatal("a refused binary was left behind")
	}
	serve[release.SignatureFile] = []byte(strings.Replace(line, "ed25519 test ", "ed25519 other ", 1))
	if _, e := a.prepare(ctx, src, "9.9.9", ""); e == nil || e.code != "signature" {
		t.Fatalf("unknown key: %v", e)
	}
}

func TestGuardDecisionAndRollbackPlan(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	if guardDecision(now, now.Add(time.Minute), 3) != "done" || guardDecision(now, now.Add(time.Minute), 2) != "wait" ||
		guardDecision(now, now.Add(-time.Second), 0) != "rollback" || guardDecision(now, now.Add(-time.Second), 3) != "done" {
		t.Fatal("guard decision")
	}
	if b, d := rollbackPlan(true, "ctrl32-x.tar.gz"); !b || !d {
		t.Fatal("failed migrations with a backup restore the database")
	}
	if _, d := rollbackPlan(true, ""); d {
		t.Fatal("no backup, no database restore")
	}
	if _, d := rollbackPlan(false, "ctrl32-x.tar.gz"); d {
		t.Fatal("successful migrations keep the database")
	}
}

func TestHealthURLAndProbe(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "c.toml")
	_ = os.WriteFile(cfg, []byte("[http]\nlisten = \"0.0.0.0:9443\"\ntls_cert = \"x.pem\"\n"), 0o640)
	if u := healthURL(cfg); u != "https://127.0.0.1:9443/healthz" {
		t.Fatal(u)
	}
	ver := "0.76.0"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": ver})
	}))
	defer srv.Close()
	if !probeHealth(context.Background(), srv.URL, "0.76.0") || probeHealth(context.Background(), srv.URL, "0.76.1") {
		t.Fatal("probe")
	}
}

type countFetcher struct{ calls int }

func (c *countFetcher) Fetch(_ context.Context, _ string, w io.Writer, _ func(n, total int64)) error {
	c.calls++
	_, err := w.Write([]byte("official"))
	return err
}

func TestMirrorFetcherOrder(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/analytics/b.tgz" {
			_, _ = w.Write([]byte("mirror"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	_ = os.WriteFile(filepath.Join(dir, "a.tgz"), []byte("package"), 0o644)
	off := &countFetcher{}
	m := mirrorSource{names: map[string]string{"https://x/a.tgz": "a.tgz", "https://x/b.tgz": "b.tgz", "https://x/c.tgz": "c.tgz"}, dirs: []string{dir}, base: srv.URL, next: off}
	for url, want := range map[string]string{"https://x/a.tgz": "package", "https://x/b.tgz": "mirror", "https://x/c.tgz": "official", "https://x/unknown": "official"} {
		var b bytes.Buffer
		if err := m.Fetch(context.Background(), url, &b, nil); err != nil || b.String() != want {
			t.Errorf("%s: %q %v", url, b.String(), err)
		}
	}
	if off.calls != 2 {
		t.Fatalf("official source used %d times", off.calls)
	}
}

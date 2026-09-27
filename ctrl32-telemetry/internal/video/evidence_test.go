package video

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writePkg(t *testing.T, key ed25519.PrivateKey, video []byte) string {
	t.Helper()
	var buf bytes.Buffer
	man := EvidenceManifest{Camera: map[string]string{"id": "c1", "name": "Gate"}, From: time.Unix(0, 0), To: time.Unix(60, 0),
		ExportedAt: time.Unix(100, 0), ExportedBy: map[string]string{"username": "tester"}, Reason: "case",
		Segments: []EvidenceSegment{{File: "a.mp4", SHA256: "x", SHA256Now: "x", Unchanged: true}}}
	if err := writeEvidence(&buf, key, &man, func(w io.Writer) error { _, err := w.Write(video); return err }); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "e.zip")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// rewrite copies a package and lets change modify one file.
func rewrite(t *testing.T, src string, name string, change func([]byte) []byte) string {
	t.Helper()
	zr, err := zip.OpenReader(src)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		if f.Name == name {
			b = change(b)
		}
		fw, _ := zw.Create(f.Name)
		_, _ = fw.Write(b)
	}
	_ = zw.Close()
	p := filepath.Join(t.TempDir(), "t.zip")
	_ = os.WriteFile(p, buf.Bytes(), 0o600)
	return p
}

func TestEvidencePackage(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	video := bytes.Repeat([]byte("frame"), 1000)
	p := writePkg(t, key, video)
	rep, err := VerifyEvidence(p, key.Public().(ed25519.PublicKey))
	if err != nil || !rep.SignatureValid || !rep.VideoHashValid || len(rep.Problems) != 0 {
		t.Fatalf("valid package: %+v %v", rep, err)
	}
	if rep.Manifest.Format != EvidenceFormat || rep.Manifest.Video["bytes"].(float64) != float64(len(video)) {
		t.Fatalf("manifest: %+v", rep.Manifest)
	}
	// a changed video is detected
	bad := rewrite(t, p, "video.mp4", func(b []byte) []byte { b[10] ^= 1; return b })
	if rep, _ := VerifyEvidence(bad, nil); rep.VideoHashValid {
		t.Fatal("changed video not detected")
	}
	// a changed manifest (e.g. another time range) breaks the signature
	bad = rewrite(t, p, "manifest.json", func(b []byte) []byte { return []byte(strings.Replace(string(b), "tester", "someone", 1)) })
	if rep, _ := VerifyEvidence(bad, nil); rep.SignatureValid {
		t.Fatal("changed manifest not detected")
	}
	// another server's key does not verify it
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	if rep, _ := VerifyEvidence(p, other.Public().(ed25519.PublicKey)); rep.SignatureValid {
		t.Fatal("foreign key accepted")
	}
	if fp := KeyFingerprint(key.Public().(ed25519.PublicKey)); len(fp) != 39 {
		t.Fatalf("fingerprint %q", fp)
	}
}

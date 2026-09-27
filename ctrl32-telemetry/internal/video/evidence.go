package video

// Evidence packages (spec 015): recorded video exported for the police, an insurer or a court as a ZIP with
//
//	video.mp4        the recording, remuxed without re-encoding (the pictures are the camera's own bytes)
//	manifest.json    camera, time range, who exported it and why, the SHA-256 of video.mp4 and of every recorded file
//	                 (as computed while it was recorded and as found now), the events in the range
//	manifest.sig     Ed25519 signature of manifest.json by the server's evidence key
//	public_key.txt   the server's public key (compare its fingerprint with the one shown in the application)
//	README.txt       how to verify
//
// `ctrl32-telemetry verify-evidence package.zip` checks the signature and the hash of video.mp4.

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EvidenceFormat identifies the manifest format.
const EvidenceFormat = "ctrl32-evidence/1"

// EvidenceKey returns the server's evidence signing key, creating it on first use (private key sealed in the DB).
func (m *Manager) EvidenceKey(ctx context.Context) (ed25519.PrivateKey, error) {
	if m.box == nil {
		return nil, errors.New("secret key not configured")
	}
	var pub, sealed string
	err := m.pool.QueryRow(ctx, `SELECT public_key, private_sealed FROM core.server_key WHERE name = 'evidence'`).Scan(&pub, &sealed)
	if err == nil {
		raw, err := base64.StdEncoding.DecodeString(sealed)
		if err != nil {
			return nil, err
		}
		seed, err := m.box.Open(raw)
		if err != nil {
			return nil, fmt.Errorf("evidence key cannot be opened (was the secret key changed?): %w", err)
		}
		return ed25519.NewKeyFromSeed(seed), nil
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	s, err := m.box.Seal(priv.Seed())
	if err != nil {
		return nil, err
	}
	pubB64 := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	if _, err := m.pool.Exec(ctx, `INSERT INTO core.server_key (name, public_key, private_sealed) VALUES ('evidence', $1, $2) ON CONFLICT (name) DO NOTHING`,
		pubB64, base64.StdEncoding.EncodeToString(s)); err != nil {
		return nil, err
	}
	return m.EvidenceKey(ctx) // the stored one wins if two requests raced
}

// KeyFingerprint is a short, comparable form of a public key: SHA-256 in groups of four hex digits.
func KeyFingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	h := hex.EncodeToString(sum[:16])
	var parts []string
	for i := 0; i < len(h); i += 4 {
		parts = append(parts, h[i:i+4])
	}
	return strings.ToUpper(strings.Join(parts, " "))
}

// EvidenceSegment is one recorded file overlapping the exported range.
type EvidenceSegment struct {
	File         string    `json:"file"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at"`
	Bytes        int64     `json:"bytes"`
	SHA256       string    `json:"sha256_recorded"` // computed while recording ("" when the server stopped while writing)
	SHA256Now    string    `json:"sha256_now"`      // computed at export
	Unchanged    bool      `json:"unchanged"`
	HeldEvidence bool      `json:"held"`
}

// EvidenceSegments lists the recorded files of a camera in a range with their fingerprints then and now.
func (m *Manager) EvidenceSegments(ctx context.Context, camID string, from, to time.Time) ([]EvidenceSegment, error) {
	rows, err := m.pool.Query(ctx, `SELECT s.path, s.started_at, s.ended_at, s.bytes, COALESCE(s.sha256, ''),
			EXISTS (SELECT 1 FROM core.video_hold h WHERE h.camera_id = s.camera_id AND h.released_at IS NULL AND s.started_at < h.to_ts AND s.ended_at > h.from_ts)
		FROM core.video_segment s WHERE s.camera_id = $1::uuid AND s.started_at < $3 AND s.ended_at > $2 ORDER BY s.started_at`, camID, from, to)
	if err != nil {
		return nil, err
	}
	var out []EvidenceSegment
	for rows.Next() {
		var e EvidenceSegment
		if rows.Scan(&e.File, &e.StartedAt, &e.EndedAt, &e.Bytes, &e.SHA256, &e.HeldEvidence) == nil {
			out = append(out, e)
		}
	}
	rows.Close()
	for i := range out {
		f, err := os.Open(filepath.Join(m.dir, filepath.FromSlash(out[i].File)))
		if err != nil {
			continue
		}
		h := sha256.New()
		_, _ = io.Copy(h, f)
		f.Close()
		out[i].SHA256Now = hex.EncodeToString(h.Sum(nil))
		out[i].Unchanged = out[i].SHA256 != "" && out[i].SHA256 == out[i].SHA256Now
	}
	return out, nil
}

// EvidenceManifest is the signed description of a package.
type EvidenceManifest struct {
	Format       string            `json:"format"`
	Server       map[string]string `json:"server"`
	Organization string            `json:"organization"`
	Camera       map[string]string `json:"camera"`
	From         time.Time         `json:"from"`
	To           time.Time         `json:"to"`
	ExportedAt   time.Time         `json:"exported_at"`
	ExportedBy   map[string]string `json:"exported_by"`
	Reason       string            `json:"reason"`
	CaseRef      string            `json:"case_ref,omitempty"`
	Video        map[string]any    `json:"video"`
	Segments     []EvidenceSegment `json:"recorded_files"`
	Events       []map[string]any  `json:"events"`
	KeyFP        string            `json:"key_fingerprint"`
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// WriteEvidence streams a signed evidence package: writeMP4 writes the video; the manifest gets its hash.
func (m *Manager) WriteEvidence(ctx context.Context, w io.Writer, man EvidenceManifest, writeMP4 func(io.Writer) error) (EvidenceManifest, error) {
	key, err := m.EvidenceKey(ctx)
	if err != nil {
		return man, err
	}
	err = writeEvidence(w, key, &man, writeMP4)
	return man, err
}

func writeEvidence(w io.Writer, key ed25519.PrivateKey, man *EvidenceManifest, writeMP4 func(io.Writer) error) error {
	pub := key.Public().(ed25519.PublicKey)
	zw := zip.NewWriter(w)
	vf, err := zw.CreateHeader(&zip.FileHeader{Name: "video.mp4", Method: zip.Store, Modified: man.ExportedAt})
	if err != nil {
		return err
	}
	h := sha256.New()
	cw := &countWriter{w: io.MultiWriter(vf, h)}
	if err := writeMP4(cw); err != nil {
		return err
	}
	man.Format = EvidenceFormat
	man.KeyFP = KeyFingerprint(pub)
	man.Video = map[string]any{"file": "video.mp4", "sha256": hex.EncodeToString(h.Sum(nil)), "bytes": cw.n,
		"note": "remuxed from the recorded files without re-encoding; the compressed pictures are the camera's own"}
	body, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return err
	}
	files := []struct {
		name string
		data []byte
	}{
		{"manifest.json", body},
		{"manifest.sig", []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, body)) + "\n")},
		{"public_key.txt", []byte("ed25519 " + base64.StdEncoding.EncodeToString(pub) + "\nfingerprint " + man.KeyFP + "\n")},
		{"README.txt", []byte(evidenceReadme(*man))},
	}
	for _, f := range files {
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: f.name, Method: zip.Deflate, Modified: man.ExportedAt})
		if err != nil {
			return err
		}
		if _, err := fw.Write(f.data); err != nil {
			return err
		}
	}
	return zw.Close()
}

func evidenceReadme(man EvidenceManifest) string {
	return fmt.Sprintf(`Evidence package — %s
Camera: %s (%s)
Recorded: %s – %s (UTC)
Exported: %s by %s
Reason: %s

video.mp4 is the recording as stored by the server, remuxed into one file without re-encoding.
manifest.json lists the SHA-256 of video.mp4 and of every recorded file (as computed while recording and now).
manifest.sig is the Ed25519 signature of manifest.json by the server's key (public_key.txt).

To verify:
  1. Compare the key fingerprint (%s) with the one the application shows under Cameras -> Evidence.
  2. Run: ctrl32-telemetry verify-evidence <this file.zip>
     or check by hand: sha256sum video.mp4 must equal manifest.json video.sha256, and the signature must verify
     with the public key (for example with openssl pkeyutl -verify -rawin).
`, man.Format, man.Camera["name"], man.Camera["id"], man.From.UTC().Format(time.RFC3339), man.To.UTC().Format(time.RFC3339),
		man.ExportedAt.UTC().Format(time.RFC3339), man.ExportedBy["username"], man.Reason, man.KeyFP)
}

// EvidenceReport is what verification found.
type EvidenceReport struct {
	SignatureValid bool
	VideoHashValid bool
	KeyFingerprint string
	Manifest       EvidenceManifest
	Problems       []string
}

// VerifyEvidence checks a package: the manifest signature against the included key (or trusted, when given) and
// the hash of video.mp4 against the manifest.
func VerifyEvidence(zipPath string, trusted ed25519.PublicKey) (*EvidenceReport, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	read := func(name string) ([]byte, error) {
		for _, f := range zr.File {
			if f.Name == name {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(io.LimitReader(rc, 64<<20))
			}
		}
		return nil, fmt.Errorf("%s missing", name)
	}
	rep := &EvidenceReport{}
	body, err := read("manifest.json")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &rep.Manifest); err != nil {
		return nil, fmt.Errorf("manifest.json: %w", err)
	}
	sigTxt, err := read("manifest.sig")
	if err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigTxt)))
	if err != nil {
		return nil, fmt.Errorf("manifest.sig: %w", err)
	}
	pub := trusted
	if pub == nil {
		keyTxt, err := read("public_key.txt")
		if err != nil {
			return nil, err
		}
		fields := strings.Fields(string(keyTxt))
		if len(fields) < 2 || fields[0] != "ed25519" {
			return nil, errors.New("public_key.txt: unexpected format")
		}
		raw, err := base64.StdEncoding.DecodeString(fields[1])
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, errors.New("public_key.txt: not an Ed25519 key")
		}
		pub = raw
		rep.Problems = append(rep.Problems, "the key was taken from the package: compare its fingerprint with the application")
	}
	rep.KeyFingerprint = KeyFingerprint(pub)
	rep.SignatureValid = ed25519.Verify(pub, body, sig)
	if !rep.SignatureValid {
		rep.Problems = append(rep.Problems, "the signature of manifest.json does not match: the manifest was changed or signed by another key")
	}
	for _, f := range zr.File {
		if f.Name != "video.mp4" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		h := sha256.New()
		_, err = io.Copy(h, rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		want, _ := rep.Manifest.Video["sha256"].(string)
		rep.VideoHashValid = bytes.Equal([]byte(hex.EncodeToString(h.Sum(nil))), []byte(want))
	}
	if !rep.VideoHashValid {
		rep.Problems = append(rep.Problems, "video.mp4 does not match the hash in the manifest: the video was changed")
	}
	for _, s := range rep.Manifest.Segments {
		if !s.Unchanged {
			rep.Problems = append(rep.Problems, "recorded file "+s.File+": fingerprint from recording time missing or different (noted at export)")
		}
	}
	return rep, nil
}

package release

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Signed releases (ADR-0024, 0.76.0). Every release publishes a manifest next to its files:
//
//	release.json       {"format":1,"product":"ctrl32-telemetry","version":"0.76.0","released":"2026-10-04",
//	                    "files":[{"name":"ctrl32-telemetry-linux-amd64","size":…,"sha256":"…"}, …]}
//	release.json.sig   one line per signature: "ed25519 <key id> <base64 signature of release.json>"
//
// The manifest lists every published file with its size and SHA-256, including SHA256SUMS and VERSION, so the
// signature covers the checksums, the version, the release date and the sizes. The public keys the binary trusts are
// embedded (trusted/*.pub, file name = key id); a release is accepted when at least one signature line verifies with a
// trusted key. Key rotation: a new key is added to trusted/ one release before it signs, and during the change a
// release carries a signature of both keys; a retired key is removed from trusted/.
//
// The private key never enters the repository: tools/relsign keeps it outside (%USERPROFILE%\.ctrl32-release on the
// release PC) and deploy/publish.sh refuses to publish without it.

//go:embed trusted
var trustedFS embed.FS

// extraKey is a base64 Ed25519 public key added at build time for test builds only
// (go build -ldflags "-X github.com/telemetry-digital/ctrl32-telemetry/internal/release.extraKey=…"); it is trusted as
// key id "build-test". deploy/publish.sh never sets it.
var extraKey string

// Product is the product name a manifest must carry.
const Product = "ctrl32-telemetry"

// ManifestFile and SignatureFile are the names of the signed manifest and its signature on the distribution point and
// in an offline package.
const (
	ManifestFile  = "release.json"
	SignatureFile = "release.json.sig"
)

// MaxManifestBytes caps the manifest (a release lists a few dozen files).
const MaxManifestBytes = 1 << 20

// File is one published file of a release.
type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest is the signed description of a release.
type Manifest struct {
	Format   int    `json:"format"`
	Product  string `json:"product"`
	Version  string `json:"version"`
	Released string `json:"released"`
	Files    []File `json:"files"`
}

var (
	versionRe  = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
	fileNameRe = regexp.MustCompile(`^(analytics/)?[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)
	hexRe      = regexp.MustCompile(`^[0-9a-f]{64}$`)
	keyIDRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,63}$`)
)

// ValidVersion reports whether v is a release version (1.2.3 or 1.2.3-suffix).
func ValidVersion(v string) bool { return versionRe.MatchString(v) }

// ValidFileName reports whether a file name may appear in a manifest (a plain name, or one under analytics/).
func ValidFileName(n string) bool { return fileNameRe.MatchString(n) && !strings.Contains(n, "..") }

// ParseManifest decodes and checks a manifest (it does not verify the signature: Verify does both).
func ParseManifest(b []byte) (*Manifest, error) {
	if len(b) > MaxManifestBytes {
		return nil, errors.New("release manifest too large")
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("release manifest: %w", err)
	}
	if m.Format != 1 {
		return nil, fmt.Errorf("release manifest: unsupported format %d", m.Format)
	}
	if m.Product != Product {
		return nil, fmt.Errorf("release manifest: product %q is not %s", m.Product, Product)
	}
	if !ValidVersion(m.Version) {
		return nil, fmt.Errorf("release manifest: invalid version %q", m.Version)
	}
	if _, err := time.Parse(Layout, m.Released); err != nil {
		return nil, fmt.Errorf("release manifest: invalid release date %q", m.Released)
	}
	if len(m.Files) == 0 {
		return nil, errors.New("release manifest: no files")
	}
	seen := map[string]bool{}
	for _, f := range m.Files {
		if !ValidFileName(f.Name) {
			return nil, fmt.Errorf("release manifest: invalid file name %q", f.Name)
		}
		if seen[f.Name] {
			return nil, fmt.Errorf("release manifest: %s listed twice", f.Name)
		}
		seen[f.Name] = true
		if f.Size < 0 || !hexRe.MatchString(f.SHA256) {
			return nil, fmt.Errorf("release manifest: bad size or SHA-256 of %s", f.Name)
		}
	}
	return &m, nil
}

// File returns the entry of a file.
func (m *Manifest) File(name string) (File, bool) {
	for _, f := range m.Files {
		if f.Name == name {
			return f, true
		}
	}
	return File{}, false
}

// Marshal encodes a manifest the way tools/relsign writes it (files sorted by name, indented).
func (m *Manifest) Marshal() ([]byte, error) {
	c := *m
	c.Files = append([]File(nil), m.Files...)
	sort.Slice(c.Files, func(i, j int) bool { return c.Files[i].Name < c.Files[j].Name })
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// BinaryName is the published file name of the server binary for a platform.
func BinaryName(goos, goarch string) string {
	n := "ctrl32-telemetry-" + goos + "-" + goarch
	if goos == "windows" {
		n += ".exe"
	}
	return n
}

// ---- keys and signatures ------------------------------------------------------------------------------------------

// ParsePublicKey decodes a base64 Ed25519 public key.
func ParsePublicKey(s string) (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("not a base64 Ed25519 public key")
	}
	return ed25519.PublicKey(b), nil
}

// ParseKeyFile reads a .pub file: comment lines (#) and one base64 public key.
func ParseKeyFile(b []byte) (ed25519.PublicKey, error) {
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return ParsePublicKey(line)
	}
	return nil, errors.New("no public key in the file")
}

// Fingerprint is a short, printable identity of a public key: "SHA256:" and the base64 of its SHA-256 (as ssh shows
// key fingerprints).
func Fingerprint(k ed25519.PublicKey) string {
	s := sha256.Sum256(k)
	return "SHA256:" + strings.TrimRight(base64.StdEncoding.EncodeToString(s[:]), "=")
}

// TrustedKeys returns the release keys this binary trusts by key id: the embedded trusted/*.pub and, in test builds,
// the build-time key "build-test".
func TrustedKeys() map[string]ed25519.PublicKey {
	out := map[string]ed25519.PublicKey{}
	entries, _ := trustedFS.ReadDir("trusted")
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".pub") {
			continue
		}
		b, err := trustedFS.ReadFile("trusted/" + e.Name())
		if err != nil {
			continue
		}
		if k, err := ParseKeyFile(b); err == nil {
			out[strings.TrimSuffix(e.Name(), ".pub")] = k
		}
	}
	if extraKey != "" {
		if k, err := ParsePublicKey(extraKey); err == nil {
			out["build-test"] = k
		}
	}
	return out
}

// TestBuild reports whether this binary trusts a build-time test key (never a published release).
func TestBuild() bool { return extraKey != "" }

// DevBuild reports whether this binary was built without a release date (development or a source build): only such a
// build honours [agent] allow_unsigned_updates.
func DevBuild() bool { return date == "" }

// Sign returns one signature line for the manifest bytes.
func Sign(manifest []byte, keyID string, priv ed25519.PrivateKey) (string, error) {
	if !keyIDRe.MatchString(keyID) {
		return "", fmt.Errorf("invalid key id %q", keyID)
	}
	return "ed25519 " + keyID + " " + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, manifest)), nil
}

// ErrUnsigned: the release has no signature (or none this binary can check).
var ErrUnsigned = errors.New("the release is not signed")

// VerifySignature checks the signature lines against the keys and returns the id of the key that verified. Lines of
// unknown keys are skipped (a release signed by a newer key and an older one verifies on both old and new servers).
func VerifySignature(manifest, sig []byte, keys map[string]ed25519.PublicKey) (string, error) {
	lines := 0
	unknown := []string{}
	for _, l := range strings.Split(string(sig), "\n") {
		f := strings.Fields(l)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		lines++
		if len(f) != 3 || f[0] != "ed25519" {
			return "", errors.New("malformed signature line")
		}
		k, ok := keys[f[1]]
		if !ok {
			unknown = append(unknown, f[1])
			continue
		}
		s, err := base64.StdEncoding.DecodeString(f[2])
		if err != nil || len(s) != ed25519.SignatureSize {
			return "", errors.New("malformed signature")
		}
		if ed25519.Verify(k, manifest, s) {
			return f[1], nil
		}
		return "", fmt.Errorf("the signature of key %s does not match the release manifest — release refused", f[1])
	}
	if lines == 0 {
		return "", ErrUnsigned
	}
	return "", fmt.Errorf("the release is signed by keys this server does not trust (%s) — release refused", strings.Join(unknown, ", "))
}

// Verify parses the manifest and checks its signature against the trusted keys.
func Verify(manifest, sig []byte) (*Manifest, string, error) {
	keyID, err := VerifySignature(manifest, sig, TrustedKeys())
	if err != nil {
		return nil, "", err
	}
	m, err := ParseManifest(manifest)
	if err != nil {
		return nil, "", err
	}
	return m, keyID, nil
}

// HexSHA256 is the lower-case hex SHA-256 of b.
func HexSHA256(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// ---- versions ----------------------------------------------------------------------------------------------------

// parts returns major, minor, patch and the pre-release suffix.
func parts(v string) ([3]int, string, bool) {
	var p [3]int
	if !ValidVersion(v) {
		return p, "", false
	}
	core, pre, _ := strings.Cut(v, "-")
	for i, s := range strings.SplitN(core, ".", 3) {
		n, err := strconv.Atoi(s)
		if err != nil {
			return p, "", false
		}
		p[i] = n
	}
	return p, pre, true
}

// Compare orders two versions: -1, 0 or 1 (a pre-release sorts before its release; invalid versions compare as 0).
func Compare(a, b string) int {
	pa, prea, oka := parts(a)
	pb, preb, okb := parts(b)
	if !oka || !okb {
		return 0
	}
	for i := range 3 {
		if pa[i] != pb[i] {
			if pa[i] < pb[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case prea == preb:
		return 0
	case prea == "":
		return 1
	case preb == "":
		return -1
	case prea < preb:
		return -1
	}
	return 1
}

// Change classifies the step from one version to another: "patch", "minor", "major", "same" or "older" ("" when
// either version is invalid). Automatic updates install patch versions, and minor ones when allowed; never major.
func Change(from, to string) string {
	pf, _, okf := parts(from)
	pt, _, okt := parts(to)
	if !okf || !okt {
		return ""
	}
	switch c := Compare(from, to); {
	case c == 0:
		return "same"
	case c > 0:
		return "older"
	case pf[0] != pt[0]:
		return "major"
	case pf[1] != pt[1]:
		return "minor"
	}
	return "patch"
}

// Package license verifies the paid licenses of ctrl32 telemetry (spec 007). The core product is free; paid features
// (white labeling) need a license token issued by the licensor with tools/licgen and signed with Ed25519. The binary
// carries only the public keys (trusted/*.pub); the private key never leaves the licensor. Tokens are verified offline.
//
// Token: "c32l1." + base64url(JSON payload) + "." + base64url(Ed25519 signature of "c32l1." + base64url(payload)).
package license

import (
	"crypto/ed25519"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Feature names.
const (
	WhiteLabel = "white_label"
)

const prefix = "c32l1."

// License is the signed payload.
type License struct {
	ID       string    `json:"id"`
	Licensee string    `json:"licensee"`
	Email    string    `json:"email,omitempty"`
	Issued   time.Time `json:"issued"`
	Expires  time.Time `json:"expires,omitzero"`  // zero = perpetual (never expires)
	Features []string  `json:"features"`          // e.g. white_label
	Domains  []string  `json:"domains,omitempty"` // server host names ("*.example.com" allowed); empty = any
	Notes    string    `json:"notes,omitempty"`
	KeyID    string    `json:"-"` // which trusted key verified it
}

var (
	ErrFormat    = errors.New("not a ctrl32 license token")
	ErrSignature = errors.New("license signature is not valid (not issued by the licensor or changed)")
	// ErrOnlineTerm: a token of an online key without an expiry, or valid longer than the key allows.
	ErrOnlineTerm = errors.New("license signed with the online activation key must expire within its limit")
)

//go:embed trusted
var trustedFS embed.FS

// extraKey is a base64 Ed25519 public key added at build time for test builds only
// (go build -ldflags "-X github.com/telemetry-digital/ctrl32-telemetry/internal/license.extraKey=…"); release builds
// leave it empty. extraOnlineKey is the same for a test ONLINE key (key id "build-online", max-days 31), so the tests
// can prove the limit of online keys end to end.
var extraKey, extraOnlineKey string

// OnlineMaxDays is the limit of the online key of the licence worker (lic.ctrl32.com): its licenses run 30 days.
const OnlineMaxDays = 31

// KeyFile is one trusted key file: the base64 public key and optional "# max-days: N" line. A key with a max-days
// limit is an ONLINE key (held by the licence worker): a token it signed is accepted only with an expiry at most N days
// after its issue date, so a stolen online key can never issue a perpetual license. The master key has no limit.
type KeyFile struct {
	Key     ed25519.PublicKey
	MaxDays int // 0 = no limit
}

// ParseKeyFile reads a trusted key file (.pub).
func ParseKeyFile(b []byte) (KeyFile, error) {
	var kf KeyFile
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(strings.TrimPrefix(line, "#")), "max-days:"); ok {
				n, err := strconv.Atoi(strings.TrimSpace(v))
				if err != nil || n <= 0 {
					return kf, fmt.Errorf("bad max-days line %q", line)
				}
				kf.MaxDays = n
			}
			continue
		}
		k, err := ParsePublicKey(line)
		if err != nil {
			return kf, err
		}
		kf.Key = k
	}
	if kf.Key == nil {
		return kf, errors.New("no public key in the file")
	}
	return kf, nil
}

// trusted returns the embedded key files by key id (file name without .pub) plus the build-time test keys.
func trusted() map[string]KeyFile {
	out := map[string]KeyFile{}
	entries, _ := trustedFS.ReadDir("trusted")
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".pub") {
			continue
		}
		b, err := trustedFS.ReadFile("trusted/" + e.Name())
		if err != nil {
			continue
		}
		if kf, err := ParseKeyFile(b); err == nil {
			out[strings.TrimSuffix(e.Name(), ".pub")] = kf
		}
	}
	if extraKey != "" {
		if k, err := ParsePublicKey(extraKey); err == nil {
			out["build"] = KeyFile{Key: k}
		}
	}
	if extraOnlineKey != "" {
		if k, err := ParsePublicKey(extraOnlineKey); err == nil {
			out["build-online"] = KeyFile{Key: k, MaxDays: OnlineMaxDays}
		}
	}
	return out
}

// TrustedKeys returns the public keys licenses are verified with.
func TrustedKeys() map[string]ed25519.PublicKey {
	out := map[string]ed25519.PublicKey{}
	for id, kf := range trusted() {
		out[id] = kf.Key
	}
	return out
}

// KeyLimits returns the max-days limit of each trusted online key (keys without a limit are absent).
func KeyLimits() map[string]int {
	out := map[string]int{}
	for id, kf := range trusted() {
		if kf.MaxDays > 0 {
			out[id] = kf.MaxDays
		}
	}
	return out
}

// Online reports whether the license was verified by an online key (a renewable 30-day license of the licence
// worker) rather than the master key.
func (l *License) Online() bool {
	if l == nil {
		return false
	}
	_, ok := KeyLimits()[l.KeyID]
	return ok
}

// ParsePublicKey decodes a base64 (standard or URL) Ed25519 public key.
func ParsePublicKey(s string) (ed25519.PublicKey, error) {
	s = strings.TrimSpace(s)
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		if b, err = base64.RawURLEncoding.DecodeString(s); err != nil {
			return nil, err
		}
	}
	if len(b) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key must be %d bytes", ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(b), nil
}

// Sign creates a token (used by tools/licgen and tests).
func Sign(l License, key ed25519.PrivateKey) (string, error) {
	if l.ID == "" || l.Licensee == "" || len(l.Features) == 0 {
		return "", errors.New("id, licensee and at least one feature are required")
	}
	b, err := json.Marshal(l)
	if err != nil {
		return "", err
	}
	body := prefix + base64.RawURLEncoding.EncodeToString(b)
	return body + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, []byte(body))), nil
}

// Parse verifies a token with the given keys (TrustedKeys() in the server) and returns its payload; the max-days
// limits of the trusted online keys (KeyLimits) apply to keys of the same id. Expiry and domains are not checked here;
// see Check.
func Parse(token string, keys map[string]ed25519.PublicKey) (*License, error) {
	return ParseLimited(token, keys, KeyLimits())
}

// ParseLimited is Parse with explicit max-days limits by key id: a token verified by a key with a limit must carry an
// issue date and an expiry at most that many days later.
func ParseLimited(token string, keys map[string]ed25519.PublicKey, limits map[string]int) (*License, error) {
	l, err := parse(token, keys)
	if err != nil {
		return nil, err
	}
	if d := limits[l.KeyID]; d > 0 {
		if l.Issued.IsZero() || l.Expires.IsZero() || l.Expires.Sub(l.Issued) > time.Duration(d)*24*time.Hour {
			return nil, fmt.Errorf("%w (%d days)", ErrOnlineTerm, d)
		}
	}
	return l, nil
}

func parse(token string, keys map[string]ed25519.PublicKey) (*License, error) {
	token = strings.Join(strings.Fields(token), "") // pasted tokens may be wrapped
	if !strings.HasPrefix(token, prefix) || len(token) > 8192 {
		return nil, ErrFormat
	}
	i := strings.LastIndexByte(token, '.')
	if i <= len(prefix) {
		return nil, ErrFormat
	}
	body, sig64 := token[:i], token[i+1:]
	sig, err := base64.RawURLEncoding.DecodeString(sig64)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, ErrFormat
	}
	keyID := ""
	for id, k := range keys {
		if ed25519.Verify(k, []byte(body), sig) {
			keyID = id
			break
		}
	}
	if keyID == "" {
		return nil, ErrSignature
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(body, prefix))
	if err != nil {
		return nil, ErrFormat
	}
	var l License
	if err := json.Unmarshal(raw, &l); err != nil {
		return nil, ErrFormat
	}
	l.KeyID = keyID
	return &l, nil
}

// Status is the result of checking a license for this server.
type Status struct {
	Valid   bool   `json:"valid"`
	Problem string `json:"problem,omitempty"` // expired, wrong domain
}

// Check reports whether the license is usable on a server reached as host at time now.
func (l *License) Check(host string, now time.Time) Status {
	if l == nil {
		return Status{Problem: "no license"}
	}
	if !l.Expires.IsZero() && now.After(l.Expires) {
		return Status{Problem: "expired on " + l.Expires.UTC().Format("2006-01-02")}
	}
	if len(l.Domains) > 0 && !DomainAllowed(l.Domains, host) {
		return Status{Problem: "issued for " + strings.Join(l.Domains, ", ") + ", not for " + host}
	}
	return Status{Valid: true}
}

// Perpetual reports whether the license never expires (no expiry date; the white-label license is sold this way).
func (l *License) Perpetual() bool { return l != nil && l.Expires.IsZero() }

// Has reports whether a usable license grants a feature.
func (l *License) Has(feature, host string, now time.Time) bool {
	if !l.Check(host, now).Valid {
		return false
	}
	for _, f := range l.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// DomainAllowed matches a host name (port ignored) against the license domains; "*.example.com" matches
// sub-domains, localhost and IP literals never match a restricted license.
func DomainAllowed(domains []string, host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == host {
			return true
		}
		if strings.HasPrefix(d, "*.") && strings.HasSuffix(host, d[1:]) && len(host) > len(d)-1 {
			return true
		}
	}
	return false
}

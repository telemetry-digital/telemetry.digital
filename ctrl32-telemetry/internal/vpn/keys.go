package vpn

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"regexp"

	"golang.org/x/crypto/curve25519"
)

// WireGuard keys are 32-byte Curve25519 values in standard base64 (44 characters). They are generated here, in
// Go, instead of running `wg genkey` (no external program, works in the agent's simulation and in tests).

var keyRe = regexp.MustCompile(`^[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]=$`)

// ValidKey reports whether s looks like a WireGuard key (public, private or preshared).
func ValidKey(s string) bool { return keyRe.MatchString(s) }

// GenerateKey returns a new private key and its public key.
func GenerateKey() (priv, pub string, err error) {
	var k [32]byte
	if _, err := rand.Read(k[:]); err != nil {
		return "", "", err
	}
	// clamping as in RFC 7748 (what `wg genkey` does)
	k[0] &= 248
	k[31] = (k[31] & 127) | 64
	priv = base64.StdEncoding.EncodeToString(k[:])
	pub, err = PublicKey(priv)
	return priv, pub, err
}

// PublicKey derives the public key of a private key.
func PublicKey(priv string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(priv)
	if err != nil || len(b) != 32 {
		return "", errors.New("invalid private key")
	}
	pub, err := curve25519.X25519(b, curve25519.Basepoint)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(pub), nil
}

// GeneratePSK returns a new preshared key (32 random bytes).
func GeneratePSK() (string, error) {
	var k [32]byte
	if _, err := rand.Read(k[:]); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(k[:]), nil
}

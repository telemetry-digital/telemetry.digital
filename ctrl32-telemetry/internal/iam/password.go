// Package iam implements identity and access: password hashing (Argon2id + pepper), sessions,
// TOTP, recovery codes, API tokens, roles and permission checks (ADR-0007, URS H-*).
package iam

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters per OWASP (2025/2026): m = 19 MiB, t = 2, p = 1.
const (
	argonMemoryKiB = 19 * 1024
	argonTime      = 2
	argonThreads   = 1
	argonKeyLen    = 32
	argonSaltLen   = 16
)

// Hasher hashes and verifies passwords with a pepper kept outside the database (config security.pepper).
type Hasher struct{ pepper []byte }

// NewHasher requires a pepper of at least 16 bytes.
func NewHasher(pepper string) (*Hasher, error) {
	if len(pepper) < 16 {
		return nil, errors.New("iam: security.pepper must be at least 16 characters")
	}
	return &Hasher{pepper: []byte(pepper)}, nil
}

// Hash returns an encoded string: $argon2id$v=19$m=...,t=...,p=...$salt$hash (salt/hash base64 raw).
func (h *Hasher) Hash(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey(h.peppered(password), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemoryKiB, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify checks a password against an encoded hash in constant time.
func (h *Hasher) Verify(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey(h.peppered(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func (h *Hasher) peppered(password string) []byte {
	b := make([]byte, 0, len(password)+len(h.pepper))
	b = append(b, []byte(password)...)
	return append(b, h.pepper...)
}

// CheckPasswordPolicy enforces length and character classes (URS H-02).
func CheckPasswordPolicy(password string, minLen int) error {
	if minLen < 8 {
		minLen = 8
	}
	if len(password) < minLen {
		return fmt.Errorf("password must be at least %d characters", minLen)
	}
	var lower, upper, digit, other bool
	for _, r := range password {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		default:
			other = true
		}
	}
	classes := 0
	for _, b := range []bool{lower, upper, digit, other} {
		if b {
			classes++
		}
	}
	if classes < 3 {
		return errors.New("password must contain at least three of: lowercase, uppercase, digits, other characters")
	}
	return nil
}

// RandomToken returns n random bytes encoded as URL-safe base64 (no padding).
func RandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// RandomPassword generates a one-time initial password that satisfies the policy.
func RandomPassword() (string, error) {
	const alphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789-+.!"
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, len(b))
	for i, v := range b {
		out[i] = alphabet[int(v)%len(alphabet)]
	}
	// guarantee the classes
	out[0], out[1], out[2], out[3] = 'A', 'a', '7', '!'
	return string(out), nil
}

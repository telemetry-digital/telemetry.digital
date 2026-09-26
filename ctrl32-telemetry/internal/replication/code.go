// Package replication pairs, feeds, monitors, detaches and promotes redundant copies of the ctrl32 database
// (spec 003). The copy itself is PostgreSQL physical streaming replication; this package supplies what PostgreSQL
// does not: one-time pairing, a transport through the existing HTTPS endpoint, the replica mode of the server,
// monitoring with incidents, a verified safe detach and fencing after a failover.
package replication

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"math/big"
	"strings"
)

// codeAlphabet has no look-alike characters (no 0/O, 1/I/L).
const codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

const codeLen = 12

// NewCode returns a one-time pairing code formatted XXXX-XXXX-XXXX (~59 bits).
func NewCode() (string, error) {
	var b strings.Builder
	for i := 0; i < codeLen; i++ {
		if i > 0 && i%4 == 0 {
			b.WriteByte('-')
		}
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		if err != nil {
			return "", err
		}
		b.WriteByte(codeAlphabet[n.Int64()])
	}
	return b.String(), nil
}

// NormalizeCode accepts a code typed with or without dashes, spaces or lower case; ok is false for anything that
// cannot be a code.
func NormalizeCode(s string) (string, bool) {
	s = strings.ToUpper(strings.NewReplacer("-", "", " ", "", "\t", "").Replace(s))
	if len(s) != codeLen {
		return "", false
	}
	for _, c := range s {
		if !strings.ContainsRune(codeAlphabet, c) {
			return "", false
		}
	}
	return s, true
}

// Hash is the stored form of codes and tokens.
func Hash(s string) []byte { h := sha256.Sum256([]byte(s)); return h[:] }

// NewToken returns a bearer token for a replica (256 bits, URL-safe).
func NewToken() string { return "c32r_" + base64.RawURLEncoding.EncodeToString(random(32)) }

// NewPassword returns the password of a replication role (256 bits, hex — safe inside a connection string).
func NewPassword() string { return hex.EncodeToString(random(32)) }

// shortID names the replication role and slot: 12 lowercase hex characters.
func shortID() string { return hex.EncodeToString(random(6)) }

func random(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("replication: no randomness: " + err.Error())
	}
	return b
}

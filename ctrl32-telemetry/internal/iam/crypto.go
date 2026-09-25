package iam

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// SecretBox encrypts small secrets (TOTP seeds, connector credentials) with AES-256-GCM.
// The key is derived from config security.secret_key, which never enters the database.
type SecretBox struct{ aead cipher.AEAD }

// NewSecretBox derives a 256-bit key from the configured secret.
func NewSecretBox(secret string) (*SecretBox, error) {
	if len(secret) < 16 {
		return nil, errors.New("iam: security.secret_key must be at least 16 characters")
	}
	key := sha256.Sum256([]byte("ctrl32-telemetry/secretbox/v1|" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretBox{aead: aead}, nil
}

// Seal returns nonce||ciphertext.
func (s *SecretBox) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return append(nonce, s.aead.Seal(nil, nonce, plaintext, nil)...), nil
}

// Open decrypts nonce||ciphertext.
func (s *SecretBox) Open(blob []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(blob) < n {
		return nil, errors.New("iam: ciphertext too short")
	}
	return s.aead.Open(nil, blob[:n], blob[n:], nil)
}

// HashToken is the storage form of session ids, API tokens, claim codes and recovery codes
// (high-entropy random values, so SHA-256 is sufficient and fast).
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

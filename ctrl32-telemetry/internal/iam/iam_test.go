package iam

import (
	"strings"
	"testing"
)

// URS-H-02: Argon2id hashing with pepper; verification succeeds only with the right password and pepper.
func TestPasswordHash_URS_H02(t *testing.T) {
	h, err := NewHasher("unit-test-pepper-0123456789")
	if err != nil {
		t.Fatal(err)
	}
	enc, err := h.Hash("Correct-Horse-7")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("unexpected encoding: %s", enc)
	}
	if !h.Verify("Correct-Horse-7", enc) {
		t.Fatal("verify should succeed")
	}
	if h.Verify("wrong", enc) {
		t.Fatal("verify should fail for wrong password")
	}
	other, _ := NewHasher("another-pepper-0123456789")
	if other.Verify("Correct-Horse-7", enc) {
		t.Fatal("verify should fail with a different pepper")
	}
	if h.Verify("Correct-Horse-7", "garbage") {
		t.Fatal("verify should fail for malformed hash")
	}
}

// URS-H-02: password policy — length and three character classes.
func TestPasswordPolicy_URS_H02(t *testing.T) {
	if err := CheckPasswordPolicy("short1A!", 12); err == nil {
		t.Fatal("too short must fail")
	}
	if err := CheckPasswordPolicy("alllowercaseletters", 12); err == nil {
		t.Fatal("one class must fail")
	}
	if err := CheckPasswordPolicy("Correct-Horse-7-Battery", 12); err != nil {
		t.Fatalf("valid password rejected: %v", err)
	}
	p, err := RandomPassword()
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckPasswordPolicy(p, 12); err != nil {
		t.Fatalf("generated password violates policy: %v (%s)", err, p)
	}
}

// URS-H-10: secrets are encrypted with AES-GCM and cannot be opened with another key.
func TestSecretBox_URS_H10(t *testing.T) {
	a, err := NewSecretBox("key-a-0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewSecretBox("key-b-0123456789abcdef")
	blob, err := a.Seal([]byte("JBSWY3DPEHPK3PXP"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := a.Open(blob)
	if err != nil || string(out) != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("open failed: %v %q", err, out)
	}
	if _, err := b.Open(blob); err == nil {
		t.Fatal("open with another key must fail")
	}
	blob[len(blob)-1] ^= 0xff
	if _, err := a.Open(blob); err == nil {
		t.Fatal("tampered ciphertext must fail")
	}
}

// URS-H-10: tokens are stored hashed.
func TestHashToken_URS_H10(t *testing.T) {
	if HashToken("abc") == "abc" || len(HashToken("abc")) != 64 {
		t.Fatal("token hash must be a 64-hex sha256")
	}
	if HashToken("abc") != HashToken("abc") {
		t.Fatal("hash must be deterministic")
	}
}

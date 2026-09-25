package iam

import (
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// URS-H-03: a code generated for the current step validates with the server-side options; a wrong code does not.
func TestTOTPRoundTrip_URS_H03(t *testing.T) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: totpIssuer, AccountName: "u", Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	code, err := totp.GenerateCode(key.Secret(), now)
	if err != nil {
		t.Fatal(err)
	}
	opts := totp.ValidateOpts{Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}
	ok, err := totp.ValidateCustom(code, key.Secret(), now, opts)
	if err != nil || !ok {
		t.Fatalf("valid code rejected: ok=%v err=%v", ok, err)
	}
	ok, _ = totp.ValidateCustom("000000", key.Secret(), now, opts)
	if ok {
		t.Fatal("wrong code accepted")
	}
}

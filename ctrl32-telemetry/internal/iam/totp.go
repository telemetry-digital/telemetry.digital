package iam

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const totpIssuer = "ctrl32 telemetry"

// TOTPSetup holds a freshly generated (not yet confirmed) secret.
type TOTPSetup struct {
	Secret string // base32, shown once
	URL    string // otpauth:// URI for QR codes
}

// BeginTOTP generates a secret, stores it encrypted (unconfirmed) and returns the provisioning URI. issuer is the name
// authenticator apps show (the product name; empty = ctrl32 telemetry).
func (s *Service) BeginTOTP(ctx context.Context, userID, username, issuer string) (*TOTPSetup, error) {
	if issuer == "" {
		issuer = totpIssuer
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: issuer, AccountName: username, Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		return nil, err
	}
	enc, err := s.box.Seal([]byte(key.Secret()))
	if err != nil {
		return nil, err
	}
	if _, err := s.pool.Exec(ctx, `UPDATE core.user_account SET totp_secret_enc = $2, totp_confirmed_at = NULL, mfa_enabled = false, totp_last_step = 0 WHERE id = $1::uuid`, userID, enc); err != nil {
		return nil, err
	}
	return &TOTPSetup{Secret: key.Secret(), URL: key.URL()}, nil
}

// ConfirmTOTP validates the first code, enables MFA and issues recovery codes (returned once).
func (s *Service) ConfirmTOTP(ctx context.Context, userID, code string) ([]string, error) {
	ok, err := s.verifyTOTP(ctx, userID, code, false)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidCode
	}
	codes := make([]string, 0, 8)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE core.user_account SET mfa_enabled = true, totp_confirmed_at = now() WHERE id = $1::uuid`, userID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE core.user_recovery_code SET used_at = now() WHERE user_id = $1::uuid AND used_at IS NULL`, userID); err != nil {
		return nil, err
	}
	for i := 0; i < 8; i++ {
		raw, err := RandomToken(6)
		if err != nil {
			return nil, err
		}
		c := strings.ToLower(raw[:8])
		codes = append(codes, c[:4]+"-"+c[4:])
		if _, err := tx.Exec(ctx, `INSERT INTO core.user_recovery_code (user_id, code_hash) VALUES ($1::uuid, $2)`, userID, HashToken(c[:4]+"-"+c[4:])); err != nil {
			return nil, err
		}
	}
	return codes, tx.Commit(ctx)
}

// verifyTOTP checks a 6-digit code against the stored secret with a ±1 step window and replay protection.
// requireEnabled=true rejects users whose MFA is not yet confirmed.
func (s *Service) verifyTOTP(ctx context.Context, userID, code string, requireEnabled bool) (bool, error) {
	var enc []byte
	var enabled bool
	var lastStep int64
	err := s.pool.QueryRow(ctx, `SELECT totp_secret_enc, mfa_enabled, totp_last_step FROM core.user_account WHERE id = $1::uuid`, userID).Scan(&enc, &enabled, &lastStep)
	if err != nil {
		return false, err
	}
	if enc == nil || (requireEnabled && !enabled) {
		return false, errors.New("iam: totp not configured")
	}
	secret, err := s.box.Open(enc)
	if err != nil {
		return false, fmt.Errorf("iam: cannot decrypt totp secret (wrong security.secret_key?): %w", err)
	}
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	now := time.Now()
	step := now.Unix() / 30
	ok, err := totp.ValidateCustom(code, string(secret), now, totp.ValidateOpts{Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil || !ok {
		return false, nil
	}
	// A code is accepted for steps step-1..step+1; reject anything at or before the last accepted step.
	if step <= lastStep {
		return false, nil
	}
	if _, err := s.pool.Exec(ctx, `UPDATE core.user_account SET totp_last_step = $2 WHERE id = $1::uuid AND totp_last_step < $2`, userID, step); err != nil {
		return false, err
	}
	return true, nil
}

// useRecoveryCode consumes an unused recovery code.
func (s *Service) useRecoveryCode(ctx context.Context, userID, code string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE core.user_recovery_code SET used_at = now() WHERE user_id = $1::uuid AND code_hash = $2 AND used_at IS NULL`, userID, HashToken(strings.ToLower(strings.TrimSpace(code))))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

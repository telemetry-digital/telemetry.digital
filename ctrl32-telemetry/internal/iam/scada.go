package iam

// SCADA users (spec 041, ADR-0026 step 3): people who sign in at the controls — inside a picture or a dashboard — on
// top of the session of the station. They are not accounts: no session, no permissions, no API. Their secret is a PIN
// or a password, as the administrator chose for each of them.

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

// ScadaUser is a person checked at the controls.
type ScadaUser struct {
	ID       string
	Name     string
	FullName string
	Kind     string   // pin | password
	Groups   []string // keys of the groups
	Approves bool     // member of a group that may approve as the second person
}

var (
	// ErrScadaSecret: the PIN or the password does not meet the rules.
	ErrScadaSecret = errors.New("a PIN is 4–12 digits, a password at least 8 characters")
	scadaPINRe     = regexp.MustCompile(`^[0-9]{4,12}$`)
	// ScadaNameRe is the form of the name a person types at the controls.
	ScadaNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`)
)

// ScadaHash checks a new secret against the rules of its kind and hashes it.
func (s *Service) ScadaHash(kind, secret string) (string, error) {
	switch kind {
	case "pin":
		if !scadaPINRe.MatchString(secret) {
			return "", ErrScadaSecret
		}
	case "password":
		if len(secret) < 8 || len(secret) > 128 {
			return "", ErrScadaSecret
		}
	default:
		return "", errors.New("secret_kind must be pin or password")
	}
	return s.hasher.Hash(secret)
}

// ScadaVerify checks a name and its PIN or password. Wrong secrets count towards the lockout of that SCADA user with
// the thresholds of the organization's policy; every attempt is written to the access log. purpose says what the check
// was for ("scada_login", "scada_second").
func (s *Service) ScadaVerify(ctx context.Context, orgID, name, secret, purpose, ip, ua string) (*ScadaUser, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	var u ScadaUser
	var hash string
	var enabled bool
	var failed int
	var lockedUntil *time.Time
	err := s.pool.QueryRow(ctx, `SELECT id::text, name, full_name, secret_kind, secret_hash, enabled, failed_count, locked_until
		FROM core.scada_user WHERE org_id = $1::uuid AND name = $2`, orgID, name).Scan(&u.ID, &u.Name, &u.FullName, &u.Kind, &hash, &enabled, &failed, &lockedUntil)
	if err != nil {
		s.hasher.Verify(secret, "$argon2id$v=19$m=19456,t=2,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
		s.access(ctx, orgID, "", name, purpose+"_failed", false, ip, ua, map[string]any{"reason": "unknown_user", "scada": true})
		return nil, ErrInvalidCredentials
	}
	if !enabled {
		s.access(ctx, orgID, "", name, purpose+"_failed", false, ip, ua, map[string]any{"reason": "disabled", "scada": true})
		return nil, ErrDisabled
	}
	if lockedUntil != nil && lockedUntil.After(time.Now()) {
		s.access(ctx, orgID, "", name, purpose+"_failed", false, ip, ua, map[string]any{"reason": "locked", "scada": true})
		return nil, ErrLocked
	}
	if !s.hasher.Verify(secret, hash) {
		pol := s.policy(ctx, orgID)
		failed++
		var lock *time.Time
		if failed >= pol.LockoutThreshold {
			t := time.Now().Add(time.Duration(pol.LockoutMinutes) * time.Minute)
			lock = &t
			failed = 0
		}
		_, _ = s.pool.Exec(ctx, `UPDATE core.scada_user SET failed_count = $2, locked_until = $3 WHERE id = $1::uuid`, u.ID, failed, lock)
		s.access(ctx, orgID, "", name, purpose+"_failed", false, ip, ua, map[string]any{"reason": "bad_secret", "locked": lock != nil, "scada": true})
		if lock != nil {
			return nil, ErrLocked
		}
		return nil, ErrInvalidCredentials
	}
	rows, err := s.pool.Query(ctx, `SELECT g.key, g.approves FROM core.scada_user_group ug JOIN core.scada_group g ON g.id = ug.group_id WHERE ug.user_id = $1::uuid ORDER BY g.position, g.key`, u.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var a bool
		if err := rows.Scan(&k, &a); err != nil {
			return nil, err
		}
		u.Groups = append(u.Groups, k)
		u.Approves = u.Approves || a
	}
	_, _ = s.pool.Exec(ctx, `UPDATE core.scada_user SET failed_count = 0, locked_until = NULL, last_used_at = now() WHERE id = $1::uuid`, u.ID)
	s.access(ctx, orgID, "", name, purpose+"_ok", true, ip, ua, map[string]any{"scada": true})
	return &u, nil
}

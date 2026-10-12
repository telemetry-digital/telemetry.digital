package iam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors returned to handlers (mapped to HTTP codes there).
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrLocked             = errors.New("account locked")
	ErrDisabled           = errors.New("account disabled")
	ErrOrgDisabled        = errors.New("organization disabled")
	ErrMFARequired        = errors.New("mfa required")
	ErrInvalidCode        = errors.New("invalid code")
	ErrNoSession          = errors.New("no session")
	ErrForbidden          = errors.New("forbidden")
)

// Policy is the organization security policy (organization.settings->'security').
type Policy struct {
	SessionIdleMinutes   int      `json:"session_idle_minutes"`
	SessionAbsoluteHours int      `json:"session_absolute_hours"`
	LockoutThreshold     int      `json:"lockout_threshold"`
	LockoutMinutes       int      `json:"lockout_minutes"`
	PasswordMinLength    int      `json:"password_min_length"`
	MFARequiredRoles     []string `json:"mfa_required_roles"`
	MFARequiredAll       bool     `json:"mfa_required_all"`
	MFAOptional          bool     `json:"mfa_optional"`          // demo instances only: nobody is forced to enrol MFA (logged as a security warning)
	FourEyesCommands     bool     `json:"four_eyes_commands"`    // URS H-08: a command waits for approval by a different user before it is executed
	FourEyesLimits       bool     `json:"four_eyes_limits"`      // URS H-08: alarm rule changes become change requests
	FourEyesAssignments  bool     `json:"four_eyes_assignments"` // URS H-08: device ↔ datastream assignments become change requests
	FourEyesVPN          bool     `json:"four_eyes_vpn"`         // ADR-0018: VPN access of technicians (user peers) needs a second person's approval
	// RememberDeviceDays allows "Stay signed in on this device" at login: such a session lasts this many days instead of
	// the idle and absolute limits (0 = not allowed, the default). MFA is still required at the sign-in itself.
	RememberDeviceDays int `json:"remember_device_days"`
}

// MaxRememberDeviceDays caps RememberDeviceDays.
const MaxRememberDeviceDays = 90

// sessionLimits returns the idle and absolute lifetime of a new session; remembered only when the policy allows it.
func sessionLimits(pol Policy, remember bool) (idle, abs time.Duration, remembered bool) {
	if remember && pol.RememberDeviceDays > 0 {
		d := time.Duration(min(pol.RememberDeviceDays, MaxRememberDeviceDays)) * 24 * time.Hour
		return d, d, true
	}
	return time.Duration(pol.SessionIdleMinutes) * time.Minute, time.Duration(pol.SessionAbsoluteHours) * time.Hour, false
}

func defaultPolicy() Policy {
	return Policy{SessionIdleMinutes: 30, SessionAbsoluteHours: 8, LockoutThreshold: 5, LockoutMinutes: 15, PasswordMinLength: 12,
		MFARequiredRoles: []string{"server_admin", "org_admin", "qa", "engineer"}}
}

// Service is the IAM facade used by HTTP handlers and the CLI.
type Service struct {
	pool         *pgxpool.Pool
	hasher       *Hasher
	box          *SecretBox
	log          *slog.Logger
	internetMode bool // profile "internet": MFA for everyone
}

// New wires the service.
func New(pool *pgxpool.Pool, hasher *Hasher, box *SecretBox, log *slog.Logger, internetMode bool) *Service {
	return &Service{pool: pool, hasher: hasher, box: box, log: log.With("component", "iam"), internetMode: internetMode}
}

// Principal is an authenticated actor (user session or API token).
type Principal struct {
	UserID      string
	OrgID       string
	CustomerID  string
	Username    string
	FullName    string
	Roles       []string
	Permissions map[string]bool
	MFAVerified bool
	SessionID   string // hashed id; empty for API tokens
	TokenID     string // API token id; empty for sessions
	// GrantID is set for an access token issued over OAuth to an AI client (spec 022): such a token is valid only at
	// /mcp, ClientName names the client, ReadOnly limits it to reading.
	GrantID     string
	ClientName  string
	ReadOnly    bool
	CSRFSecret  string
	MustChange  bool
	MFAEnabled  bool
	MFARequired bool
	// CameraLimited: the user sees only the cameras of CameraGroups (spec 014); otherwise all cameras.
	CameraLimited bool
	CameraGroups  []string
}

// SeesCameraGroup reports whether the principal may see cameras of the group ("" = cameras without a group).
func (p *Principal) SeesCameraGroup(group string) bool {
	if p == nil || !p.CameraLimited {
		return true
	}
	for _, g := range p.CameraGroups {
		if g == group {
			return true
		}
	}
	return false
}

// Has reports whether the principal holds a permission.
func (p *Principal) Has(perm string) bool { return p != nil && p.Permissions[perm] }

// --- policy -------------------------------------------------------------------------------------

// Policy returns the effective security policy of the organization (defaults merged with settings->security).
func (s *Service) Policy(ctx context.Context, orgID string) Policy { return s.policy(ctx, orgID) }

func (s *Service) policy(ctx context.Context, orgID string) Policy {
	p := defaultPolicy()
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT settings->'security' FROM core.organization WHERE id = $1::uuid`, orgID).Scan(&raw); err == nil && raw != nil {
		_ = json.Unmarshal(raw, &p)
	}
	return p
}

// --- users --------------------------------------------------------------------------------------

// User is the public view of an account.
type User struct {
	ID         string     `json:"id"`
	OrgID      string     `json:"org_id"`
	CustomerID *string    `json:"customer_id,omitempty"`
	Username   string     `json:"username"`
	Email      *string    `json:"email,omitempty"`
	FullName   string     `json:"full_name"`
	Roles      []string   `json:"roles"`
	MFAEnabled bool       `json:"mfa_enabled"`
	MustChange bool       `json:"must_change_password"`
	DisabledAt *time.Time `json:"disabled_at,omitempty"`
	LockedTo   *time.Time `json:"locked_until,omitempty"`
	LastLogin  *time.Time `json:"last_login_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// CreateUser creates an account with must_change_password=true and the given built-in/org roles.
func (s *Service) CreateUser(ctx context.Context, orgID, username, fullName, email, initialPassword string, roles []string, actor string, reason string) (*User, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if len(username) < 2 || len(username) > 64 {
		return nil, errors.New("username must be 2–64 characters")
	}
	if err := CheckPasswordPolicy(initialPassword, s.policy(ctx, orgID).PasswordMinLength); err != nil {
		return nil, err
	}
	hash, err := s.hasher.Hash(initialPassword)
	if err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO core.user_account (org_id, username, email, full_name, password_hash, must_change_password)
		VALUES ($1::uuid, $2, NULLIF($3,''), $4, $5, true) RETURNING id::text`, orgID, username, email, fullName, hash).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	for _, r := range roles {
		if err := grantRole(ctx, tx, orgID, id, r, actor, reason); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetUser(ctx, id)
}

func grantRole(ctx context.Context, tx pgx.Tx, orgID, userID, role, actor, reason string) error {
	var roleID string
	err := tx.QueryRow(ctx, `SELECT id::text FROM core.role WHERE name = $1 AND (org_id IS NULL OR org_id = $2::uuid) ORDER BY org_id NULLS LAST LIMIT 1`, role, orgID).Scan(&roleID)
	if err != nil {
		return fmt.Errorf("role %q not found", role)
	}
	_, err = tx.Exec(ctx, `INSERT INTO core.user_role_history (user_id, role_id, event, actor_id, reason) VALUES ($1::uuid, $2::uuid, 'granted', NULLIF($3,'')::uuid, $4)`, userID, roleID, actor, reason)
	return err
}

// SetRoles revokes roles not in the list and grants missing ones (append-only history).
func (s *Service) SetRoles(ctx context.Context, orgID, userID string, roles []string, actorID, reason string) error {
	current, err := s.currentRoles(ctx, userID)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, r := range roles {
		want[r] = true
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for name, roleID := range current {
		if !want[name] {
			if _, err := tx.Exec(ctx, `INSERT INTO core.user_role_history (user_id, role_id, event, actor_id, reason) VALUES ($1::uuid, $2::uuid, 'revoked', NULLIF($3,'')::uuid, $4)`, userID, roleID, actorID, reason); err != nil {
				return err
			}
		}
	}
	for name := range want {
		if _, ok := current[name]; !ok {
			if err := grantRole(ctx, tx, orgID, userID, name, actorID, reason); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

// currentRoles returns role name -> role id for grants that were not revoked later.
func (s *Service) currentRoles(ctx context.Context, userID string) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.name, r.id::text FROM core.role r
		WHERE r.id IN (
		  SELECT role_id FROM core.user_role_history h
		  WHERE h.user_id = $1::uuid
		  GROUP BY role_id
		  HAVING (array_agg(event ORDER BY ts DESC, id DESC))[1] = 'granted')`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var n, id string
		if err := rows.Scan(&n, &id); err != nil {
			return nil, err
		}
		out[n] = id
	}
	return out, rows.Err()
}

func (s *Service) permissionsFor(ctx context.Context, userID string) ([]string, map[string]bool, error) {
	roles, err := s.currentRoles(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	names := make([]string, 0, len(roles))
	ids := make([]string, 0, len(roles))
	for n, id := range roles {
		names = append(names, n)
		ids = append(ids, id)
	}
	perms := map[string]bool{}
	if len(ids) > 0 {
		rows, err := s.pool.Query(ctx, `SELECT unnest(permissions) FROM core.role WHERE id::text = ANY($1)`, ids)
		if err != nil {
			return nil, nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				return nil, nil, err
			}
			perms[p] = true
		}
	}
	return names, perms, nil
}

// GetUser loads a user with current roles.
func (s *Service) GetUser(ctx context.Context, id string) (*User, error) {
	u := &User{}
	err := s.pool.QueryRow(ctx, `SELECT id::text, org_id::text, customer_id::text, username, email, full_name, mfa_enabled, must_change_password, disabled_at, locked_until, last_login_at, created_at
		FROM core.user_account WHERE id = $1::uuid`, id).
		Scan(&u.ID, &u.OrgID, &u.CustomerID, &u.Username, &u.Email, &u.FullName, &u.MFAEnabled, &u.MustChange, &u.DisabledAt, &u.LockedTo, &u.LastLogin, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	roles, err := s.currentRoles(ctx, id)
	if err != nil {
		return nil, err
	}
	u.Roles = make([]string, 0, len(roles))
	for n := range roles {
		u.Roles = append(u.Roles, n)
	}
	return u, nil
}

// ListUsers returns users of an organization.
func (s *Service) ListUsers(ctx context.Context, orgID string) ([]*User, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text FROM core.user_account WHERE org_id = $1::uuid ORDER BY username`, orgID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	out := make([]*User, 0, len(ids))
	for _, id := range ids {
		u, err := s.GetUser(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

// SetDisabled enables/disables an account and revokes its sessions when disabling.
func (s *Service) SetDisabled(ctx context.Context, userID string, disabled bool) error {
	if disabled {
		if _, err := s.pool.Exec(ctx, `UPDATE core.user_account SET disabled_at = now() WHERE id = $1::uuid AND disabled_at IS NULL`, userID); err != nil {
			return err
		}
		_, err := s.pool.Exec(ctx, `UPDATE core.session SET revoked_at = now() WHERE user_id = $1::uuid AND revoked_at IS NULL`, userID)
		return err
	}
	_, err := s.pool.Exec(ctx, `UPDATE core.user_account SET disabled_at = NULL, failed_logins = 0, locked_until = NULL WHERE id = $1::uuid`, userID)
	return err
}

// ChangePassword verifies the current password (unless admin reset) and sets a new one.
func (s *Service) ChangePassword(ctx context.Context, userID, current, next string, adminReset bool) error {
	var hash, orgID string
	if err := s.pool.QueryRow(ctx, `SELECT password_hash, org_id::text FROM core.user_account WHERE id = $1::uuid`, userID).Scan(&hash, &orgID); err != nil {
		return err
	}
	if !adminReset && !s.hasher.Verify(current, hash) {
		return ErrInvalidCredentials
	}
	if err := CheckPasswordPolicy(next, s.policy(ctx, orgID).PasswordMinLength); err != nil {
		return err
	}
	if s.hasher.Verify(next, hash) {
		return errors.New("new password must differ from the current one")
	}
	newHash, err := s.hasher.Hash(next)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE core.user_account SET password_hash = $2, password_changed_at = now(), must_change_password = $3 WHERE id = $1::uuid`, userID, newHash, adminReset)
	if err != nil {
		return err
	}
	if !adminReset {
		// Password change invalidates all other sessions of the user.
		_, _ = s.pool.Exec(ctx, `UPDATE core.session SET revoked_at = now() WHERE user_id = $1::uuid AND revoked_at IS NULL`, userID)
	}
	return nil
}

// SetPassword sets a password from the CLI. enforcePolicy=false bypasses the password policy (demo instances only;
// the caller records it in the audit trail). mustChange forces a new password at the next login.
func (s *Service) SetPassword(ctx context.Context, userID, next string, mustChange, enforcePolicy bool) error {
	var orgID string
	if err := s.pool.QueryRow(ctx, `SELECT org_id::text FROM core.user_account WHERE id = $1::uuid`, userID).Scan(&orgID); err != nil {
		return err
	}
	if enforcePolicy {
		if err := CheckPasswordPolicy(next, s.policy(ctx, orgID).PasswordMinLength); err != nil {
			return err
		}
	} else if len(next) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	newHash, err := s.hasher.Hash(next)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE core.user_account SET password_hash = $2, password_changed_at = now(), must_change_password = $3, failed_logins = 0, locked_until = NULL WHERE id = $1::uuid`, userID, newHash, mustChange)
	if err != nil {
		return err
	}
	_, _ = s.pool.Exec(ctx, `UPDATE core.session SET revoked_at = now() WHERE user_id = $1::uuid AND revoked_at IS NULL`, userID)
	return nil
}

// --- authentication -----------------------------------------------------------------------------

// LoginResult describes the outcome of the password step.
type LoginResult struct {
	SessionToken string // raw cookie value (only if no MFA pending)
	PreAuthToken string // raw token for the MFA step (if MFA pending)
	MFAPending   bool
	MustChange   bool
	User         *User
	// Remember is the lifetime of a remembered session (the cookie then outlives the browser); 0 = a normal session.
	Remember time.Duration
}

// Login verifies username/password, applies lockout, and opens a session (or a pending-MFA session). remember asks for
// a remembered session ("Stay signed in on this device"), granted only when the organization policy allows it.
func (s *Service) Login(ctx context.Context, username, password, ip, ua string, remember bool) (*LoginResult, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	var id, orgID, hash string
	var disabled, lockedUntil, orgDisabled *time.Time
	var failed int
	var mfaEnabled, mustChange bool
	err := s.pool.QueryRow(ctx, `SELECT u.id::text, u.org_id::text, u.password_hash, u.disabled_at, u.locked_until, u.failed_logins, u.mfa_enabled, u.must_change_password, o.disabled_at
		FROM core.user_account u JOIN core.organization o ON o.id = u.org_id WHERE u.username = $1 OR lower(u.email) = $1`, username).Scan(&id, &orgID, &hash, &disabled, &lockedUntil, &failed, &mfaEnabled, &mustChange, &orgDisabled)
	if err != nil {
		// Burn the same time as a real verification to blunt username enumeration.
		s.hasher.Verify(password, "$argon2id$v=19$m=19456,t=2,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
		s.access(ctx, "", "", username, "login_failed", false, ip, ua, map[string]any{"reason": "unknown_user"})
		return nil, ErrInvalidCredentials
	}
	pol := s.policy(ctx, orgID)
	if disabled != nil {
		s.access(ctx, orgID, id, username, "login_failed", false, ip, ua, map[string]any{"reason": "disabled"})
		return nil, ErrDisabled
	}
	if orgDisabled != nil {
		s.access(ctx, orgID, id, username, "login_failed", false, ip, ua, map[string]any{"reason": "org_disabled"})
		return nil, ErrOrgDisabled
	}
	if lockedUntil != nil && lockedUntil.After(time.Now()) {
		s.access(ctx, orgID, id, username, "login_failed", false, ip, ua, map[string]any{"reason": "locked"})
		return nil, ErrLocked
	}
	if !s.hasher.Verify(password, hash) {
		failed++
		var lock *time.Time
		if failed >= pol.LockoutThreshold {
			t := time.Now().Add(time.Duration(pol.LockoutMinutes) * time.Minute)
			lock = &t
			failed = 0
		}
		_, _ = s.pool.Exec(ctx, `UPDATE core.user_account SET failed_logins = $2, locked_until = $3 WHERE id = $1::uuid`, id, failed, lock)
		s.access(ctx, orgID, id, username, "login_failed", false, ip, ua, map[string]any{"reason": "bad_password", "locked": lock != nil})
		if lock != nil {
			return nil, ErrLocked
		}
		return nil, ErrInvalidCredentials
	}
	roles, _, err := s.permissionsFor(ctx, id)
	if err != nil {
		return nil, err
	}
	mfaRequired := s.mfaRequired(pol, roles)
	res := &LoginResult{MustChange: mustChange, MFAPending: mfaEnabled}
	raw, err := RandomToken(32)
	if err != nil {
		return nil, err
	}
	csrf, err := RandomToken(24)
	if err != nil {
		return nil, err
	}
	idle, abs, remembered := sessionLimits(pol, remember)
	if mfaEnabled {
		// Pending session: 5 minutes to complete the TOTP step; it carries the wish to be remembered to that step.
		idle = 5 * time.Minute
	} else if remembered {
		res.Remember = abs
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO core.session (id, user_id, expires_at, absolute_expires_at, ip, user_agent, csrf_secret, mfa_pending, remember)
		VALUES ($1, $2::uuid, now() + $3::interval, now() + $4::interval, NULLIF($5,'')::inet, $6, $7, $8, $9)`,
		HashToken(raw), id, idle.String(), abs.String(), ipOnly(ip), truncate(ua, 300), csrf, mfaEnabled, remembered)
	if err != nil {
		return nil, err
	}
	_, _ = s.pool.Exec(ctx, `UPDATE core.user_account SET failed_logins = 0, locked_until = NULL, last_login_at = now() WHERE id = $1::uuid`, id)
	if mfaEnabled {
		res.PreAuthToken = raw
		s.access(ctx, orgID, id, username, "login_password_ok", true, ip, ua, map[string]any{"mfa_pending": true})
	} else {
		res.SessionToken = raw
		s.access(ctx, orgID, id, username, "login_ok", true, ip, ua, map[string]any{"mfa": false, "mfa_required_but_missing": mfaRequired, "remember": remembered})
		if mfaRequired {
			s.log.Warn("user without MFA logged in although policy requires it", "security", true, "user", username)
		}
	}
	res.User, err = s.GetUser(ctx, id)
	return res, err
}

func (s *Service) mfaRequired(pol Policy, roles []string) bool {
	if pol.MFAOptional {
		// Explicit per-organization relaxation (demo instances). Overrides the internet-profile default.
		return false
	}
	if s.internetMode || pol.MFARequiredAll {
		return true
	}
	for _, r := range roles {
		for _, req := range pol.MFARequiredRoles {
			if r == req {
				return true
			}
		}
	}
	return false
}

// CompleteMFA verifies a TOTP or recovery code for a pending session and activates it. It returns the session token
// and, for a remembered session, its lifetime (0 otherwise).
func (s *Service) CompleteMFA(ctx context.Context, preAuthToken, code, ip, ua string) (string, time.Duration, error) {
	h := HashToken(preAuthToken)
	var userID, orgID, username string
	var wantRemember bool
	err := s.pool.QueryRow(ctx, `SELECT s.user_id::text, u.org_id::text, u.username, s.remember FROM core.session s JOIN core.user_account u ON u.id = s.user_id
		WHERE s.id = $1 AND s.mfa_pending AND s.revoked_at IS NULL AND s.expires_at > now()`, h).Scan(&userID, &orgID, &username, &wantRemember)
	if err != nil {
		return "", 0, ErrInvalidCredentials
	}
	ok, err := s.verifyTOTP(ctx, userID, code, true)
	if err != nil {
		return "", 0, err
	}
	method := "totp"
	if !ok {
		ok, err = s.useRecoveryCode(ctx, userID, code)
		if err != nil {
			return "", 0, err
		}
		method = "recovery_code"
	}
	if !ok {
		s.access(ctx, orgID, userID, username, "mfa_failed", false, ip, ua, nil)
		return "", 0, ErrInvalidCode
	}
	pol := s.policy(ctx, orgID)
	// Rotate the session id on privilege change (OWASP): revoke the pending session, open the real one.
	raw, err := RandomToken(32)
	if err != nil {
		return "", 0, err
	}
	csrf, _ := RandomToken(24)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE core.session SET revoked_at = now() WHERE id = $1`, h); err != nil {
		return "", 0, err
	}
	idle, abs, remembered := sessionLimits(pol, wantRemember)
	if _, err := tx.Exec(ctx, `INSERT INTO core.session (id, user_id, expires_at, absolute_expires_at, ip, user_agent, csrf_secret, mfa_pending, mfa_at, remember)
		VALUES ($1, $2::uuid, now() + $3::interval, now() + $4::interval, NULLIF($5,'')::inet, $6, $7, false, now(), $8)`,
		HashToken(raw), userID, idle.String(), abs.String(), ipOnly(ip), truncate(ua, 300), csrf, remembered); err != nil {
		return "", 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", 0, err
	}
	s.access(ctx, orgID, userID, username, "login_ok", true, ip, ua, map[string]any{"mfa": true, "method": method, "remember": remembered})
	if remembered {
		return raw, abs, nil
	}
	return raw, 0, nil
}

// ReauthPassword re-checks the password of the current user (for signing and sensitive actions).
func (s *Service) ReauthPassword(ctx context.Context, userID, password string) bool {
	var hash string
	if err := s.pool.QueryRow(ctx, `SELECT password_hash FROM core.user_account WHERE id = $1::uuid`, userID).Scan(&hash); err != nil {
		return false
	}
	return s.hasher.Verify(password, hash)
}

// ReauthTOTP re-checks a TOTP code for the current user.
func (s *Service) ReauthTOTP(ctx context.Context, userID, code string) bool {
	ok, err := s.verifyTOTP(ctx, userID, code, true)
	return err == nil && ok
}

// Logout revokes a session.
func (s *Service) Logout(ctx context.Context, sessionToken, ip, ua string) {
	h := HashToken(sessionToken)
	var userID, orgID, username string
	if err := s.pool.QueryRow(ctx, `UPDATE core.session SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL RETURNING user_id::text`, h).Scan(&userID); err == nil {
		_ = s.pool.QueryRow(ctx, `SELECT org_id::text, username FROM core.user_account WHERE id = $1::uuid`, userID).Scan(&orgID, &username)
		s.access(ctx, orgID, userID, username, "logout", true, ip, ua, nil)
	}
}

// Authenticate resolves a session cookie value into a Principal, sliding the idle expiry.
func (s *Service) Authenticate(ctx context.Context, sessionToken string) (*Principal, error) {
	if sessionToken == "" {
		return nil, ErrNoSession
	}
	h := HashToken(sessionToken)
	var p Principal
	var orgSettings []byte
	var remember bool
	var created time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT u.id::text, u.org_id::text, COALESCE(u.customer_id::text,''), u.username, u.full_name, u.must_change_password, u.mfa_enabled,
		       s.mfa_at IS NOT NULL, s.csrf_secret, o.settings->'security', u.camera_groups IS NOT NULL, COALESCE(u.camera_groups, '{}'),
		       s.remember, s.created_at
		FROM core.session s JOIN core.user_account u ON u.id = s.user_id JOIN core.organization o ON o.id = u.org_id
		WHERE s.id = $1 AND s.revoked_at IS NULL AND NOT s.mfa_pending AND s.expires_at > now()
		  AND (s.absolute_expires_at IS NULL OR s.absolute_expires_at > now()) AND u.disabled_at IS NULL`, h).
		Scan(&p.UserID, &p.OrgID, &p.CustomerID, &p.Username, &p.FullName, &p.MustChange, &p.MFAEnabled, &p.MFAVerified, &p.CSRFSecret, &orgSettings, &p.CameraLimited, &p.CameraGroups,
			&remember, &created)
	if err != nil {
		return nil, ErrNoSession
	}
	pol := defaultPolicy()
	if orgSettings != nil {
		_ = json.Unmarshal(orgSettings, &pol)
	}
	idle, err := sessionIdle(pol, remember, time.Since(created))
	if err != nil {
		return nil, err
	}
	p.SessionID = h
	p.Roles, p.Permissions, err = s.permissionsFor(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	p.MFARequired = s.mfaRequired(pol, p.Roles)
	// Slide idle expiry (at most once per minute to limit writes).
	_, _ = s.pool.Exec(ctx, `UPDATE core.session SET last_seen_at = now(), expires_at = now() + $2::interval WHERE id = $1 AND last_seen_at < now() - interval '1 minute'`,
		h, idle.String())
	return &p, nil
}

// sessionIdle is the sliding idle lifetime of a session of the given age. A remembered session follows the policy as it
// is now, so shortening or switching off "Stay signed in" applies to the sessions already open: switched off, it ends
// like a normal one at the absolute limit (its idle limit slides back to the normal one).
func sessionIdle(pol Policy, remember bool, age time.Duration) (time.Duration, error) {
	idle := time.Duration(pol.SessionIdleMinutes) * time.Minute
	if !remember {
		return idle, nil
	}
	days := min(pol.RememberDeviceDays, MaxRememberDeviceDays)
	if days <= 0 {
		if age > time.Duration(pol.SessionAbsoluteHours)*time.Hour {
			return 0, ErrNoSession
		}
		return idle, nil
	}
	life := time.Duration(days) * 24 * time.Hour
	if age > life {
		return 0, ErrNoSession
	}
	return life, nil
}

// --- API tokens ---------------------------------------------------------------------------------

// APIToken is the public view.
type APIToken struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	LastUsed  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// CreateAPIToken issues a token whose scopes are a subset of the user's permissions. Returns the raw token once.
func (s *Service) CreateAPIToken(ctx context.Context, p *Principal, name string, scopes []string, expires *time.Time) (string, *APIToken, error) {
	if strings.TrimSpace(name) == "" {
		return "", nil, errors.New("name required")
	}
	for _, sc := range scopes {
		if !p.Has(sc) {
			return "", nil, fmt.Errorf("scope %q exceeds your permissions", sc)
		}
	}
	raw, err := RandomToken(32)
	if err != nil {
		return "", nil, err
	}
	raw = "c32_" + raw
	var t APIToken
	err = s.pool.QueryRow(ctx, `INSERT INTO core.api_token (org_id, user_id, name, token_hash, scopes, expires_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6) RETURNING id::text, name, scopes, expires_at, created_at`,
		p.OrgID, p.UserID, name, HashToken(raw), scopes, expires).Scan(&t.ID, &t.Name, &t.Scopes, &t.ExpiresAt, &t.CreatedAt)
	if err != nil {
		return "", nil, err
	}
	return raw, &t, nil
}

// ListAPITokens returns the user's tokens.
func (s *Service) ListAPITokens(ctx context.Context, userID string) ([]APIToken, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, name, scopes, expires_at, last_used_at, created_at, revoked_at FROM core.api_token WHERE user_id = $1::uuid AND grant_id IS NULL ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIToken
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.Name, &t.Scopes, &t.ExpiresAt, &t.LastUsed, &t.CreatedAt, &t.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeAPIToken revokes one of the user's tokens.
func (s *Service) RevokeAPIToken(ctx context.Context, userID, tokenID string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE core.api_token SET revoked_at = now() WHERE id = $1::uuid AND user_id = $2::uuid AND revoked_at IS NULL AND grant_id IS NULL`, tokenID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrForbidden
	}
	return nil
}

// AuthenticateToken resolves a bearer token into a Principal limited to the token scopes.
func (s *Service) AuthenticateToken(ctx context.Context, raw string) (*Principal, error) {
	if !strings.HasPrefix(raw, "c32_") {
		return nil, ErrNoSession
	}
	var p Principal
	var scopes []string
	var expires *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT t.id::text, t.scopes, t.expires_at, u.id::text, u.org_id::text, COALESCE(u.customer_id::text,''), u.username, u.full_name,
		       u.camera_groups IS NOT NULL, COALESCE(u.camera_groups, '{}'),
		       COALESCE(g.id::text, ''), COALESCE(c.name, ''), COALESCE(g.read_only, false)
		FROM core.api_token t JOIN core.user_account u ON u.id = t.user_id
		LEFT JOIN core.oauth_grant g ON g.id = t.grant_id
		LEFT JOIN core.oauth_client c ON c.id = g.client_id
		WHERE t.token_hash = $1 AND t.revoked_at IS NULL AND u.disabled_at IS NULL
		  AND (t.grant_id IS NULL OR g.revoked_at IS NULL)`, HashToken(raw)).
		Scan(&p.TokenID, &scopes, &expires, &p.UserID, &p.OrgID, &p.CustomerID, &p.Username, &p.FullName, &p.CameraLimited, &p.CameraGroups,
			&p.GrantID, &p.ClientName, &p.ReadOnly)
	if err != nil {
		return nil, ErrNoSession
	}
	if expires != nil && expires.Before(time.Now()) {
		return nil, ErrNoSession
	}
	// Effective permissions = token scopes ∩ current user permissions (revoking a role also shrinks tokens).
	_, userPerms, err := s.permissionsFor(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	p.Permissions = map[string]bool{}
	for _, sc := range scopes {
		if userPerms[sc] {
			p.Permissions[sc] = true
		}
	}
	p.MFAVerified = true // tokens are created from an authenticated session
	_, _ = s.pool.Exec(ctx, `UPDATE core.api_token SET last_used_at = now() WHERE id = $1::uuid AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute')`, p.TokenID)
	return &p, nil
}

// --- access log ---------------------------------------------------------------------------------

func (s *Service) access(ctx context.Context, orgID, userID, username, event string, success bool, ip, ua string, detail map[string]any) {
	d, _ := json.Marshal(detail)
	if detail == nil {
		d = []byte("{}")
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO core.access_log (org_id, user_id, username, event, success, ip, user_agent, detail)
		VALUES (NULLIF($1,'')::uuid, NULLIF($2,'')::uuid, $3, $4, $5, NULLIF($6,'')::inet, $7, $8::jsonb)`,
		orgID, userID, username, event, success, ipOnly(ip), truncate(ua, 300), string(d)); err != nil {
		s.log.Error("access log write failed", "err", err)
	}
	if !success {
		s.log.Warn("access event", "security", true, "event", event, "user", username, "ip", ip)
	}
}

func ipOnly(remote string) string {
	if a, err := netip.ParseAddrPort(remote); err == nil {
		return a.Addr().String()
	}
	if a, err := netip.ParseAddr(remote); err == nil {
		return a.String()
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

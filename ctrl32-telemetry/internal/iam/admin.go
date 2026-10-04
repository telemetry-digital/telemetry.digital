package iam

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Administration of roles and permissions (URS H-04, H-05): built-in roles are immutable, organizations may add
// their own roles from the permission catalog; every grant and revoke is a row in user_role_history.

// Permission is one entry of the catalog.
type Permission struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Role is a built-in (org_id NULL) or organization role with its permission set and current member count.
type Role struct {
	ID          string   `json:"id"`
	OrgID       string   `json:"org_id,omitempty"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
	BuiltIn     bool     `json:"builtin"`
	IsAdmin     bool     `json:"is_admin"`
	IsQA        bool     `json:"is_qa"`
	Users       int      `json:"users"`
}

// RoleEvent is one grant/revoke of the role history.
type RoleEvent struct {
	TS        time.Time `json:"ts"`
	Role      string    `json:"role"`
	Event     string    `json:"event"`
	ActorName string    `json:"actor_name"`
	Reason    string    `json:"reason"`
}

var roleName = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

// ListPermissions returns the catalog.
func (s *Service) ListPermissions(ctx context.Context) ([]Permission, error) {
	rows, err := s.pool.Query(ctx, `SELECT name, description FROM core.permission ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Permission{}
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.Name, &p.Description); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

const roleUsersSQL = `(SELECT count(*) FROM (
	SELECT user_id, (array_agg(event ORDER BY ts DESC, id DESC))[1] AS ev FROM core.user_role_history WHERE role_id = r.id GROUP BY user_id
) h WHERE h.ev = 'granted')`

// ListRoles returns built-in roles and the organization's own roles.
func (s *Service) ListRoles(ctx context.Context, orgID string) ([]Role, error) {
	rows, err := s.pool.Query(ctx, `SELECT r.id::text, COALESCE(r.org_id::text,''), r.name, r.permissions, r.is_admin, r.is_qa, `+roleUsersSQL+`
		FROM core.role r WHERE r.org_id IS NULL OR r.org_id = $1::uuid ORDER BY r.org_id NULLS FIRST, r.name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Role{}
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.OrgID, &r.Name, &r.Permissions, &r.IsAdmin, &r.IsQA, &r.Users); err != nil {
			return nil, err
		}
		r.BuiltIn = r.OrgID == ""
		if r.Permissions == nil {
			r.Permissions = []string{}
		}
		sort.Strings(r.Permissions)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) validatePermissions(ctx context.Context, perms []string) ([]string, error) {
	catalog, err := s.ListPermissions(ctx)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, p := range catalog {
		known[p.Name] = true
	}
	seen := map[string]bool{}
	out := []string{}
	for _, p := range perms {
		p = strings.TrimSpace(p)
		if !known[p] {
			return nil, fmt.Errorf("unknown permission %q", p)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

// CreateRole adds an organization role. Names must not shadow built-in roles.
func (s *Service) CreateRole(ctx context.Context, orgID, name string, perms []string) (*Role, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !roleName.MatchString(name) {
		return nil, errors.New("role name must be 2–32 characters: a-z, 0-9, _")
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM core.role WHERE name = $1 AND (org_id IS NULL OR org_id = $2::uuid)`, name, orgID).Scan(&n); err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, fmt.Errorf("role %q already exists", name)
	}
	perms, err := s.validatePermissions(ctx, perms)
	if err != nil {
		return nil, err
	}
	var id string
	if err := s.pool.QueryRow(ctx, `INSERT INTO core.role (org_id, name, permissions) VALUES ($1::uuid, $2, $3) RETURNING id::text`, orgID, name, perms).Scan(&id); err != nil {
		return nil, err
	}
	return s.GetRole(ctx, orgID, id)
}

// UpdateRole replaces the permission set of an organization role. Built-in roles cannot be changed.
func (s *Service) UpdateRole(ctx context.Context, orgID, roleID string, perms []string) (*Role, error) {
	r, err := s.GetRole(ctx, orgID, roleID)
	if err != nil {
		return nil, err
	}
	if r.BuiltIn {
		return nil, ErrForbidden
	}
	perms, err = s.validatePermissions(ctx, perms)
	if err != nil {
		return nil, err
	}
	if _, err := s.pool.Exec(ctx, `UPDATE core.role SET permissions = $3 WHERE id = $1::uuid AND org_id = $2::uuid`, roleID, orgID, perms); err != nil {
		return nil, err
	}
	return s.GetRole(ctx, orgID, roleID)
}

// GetRole loads one role visible to the organization.
func (s *Service) GetRole(ctx context.Context, orgID, roleID string) (*Role, error) {
	var r Role
	err := s.pool.QueryRow(ctx, `SELECT r.id::text, COALESCE(r.org_id::text,''), r.name, r.permissions, r.is_admin, r.is_qa, `+roleUsersSQL+`
		FROM core.role r WHERE r.id = $2::uuid AND (r.org_id IS NULL OR r.org_id = $1::uuid)`, orgID, roleID).
		Scan(&r.ID, &r.OrgID, &r.Name, &r.Permissions, &r.IsAdmin, &r.IsQA, &r.Users)
	if err != nil {
		return nil, err
	}
	r.BuiltIn = r.OrgID == ""
	if r.Permissions == nil {
		r.Permissions = []string{}
	}
	sort.Strings(r.Permissions)
	return &r, nil
}

// UserPermissions returns the effective permission list of a user (union of current roles), sorted.
func (s *Service) UserPermissions(ctx context.Context, userID string) ([]string, error) {
	_, perms, err := s.permissionsFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(perms))
	for p := range perms {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

// ServerPermission is the permission of server administration (ADR-0019): organizations, server settings, the VPN
// of the whole server, backups, updates, licence. Only the built-in role server_admin holds it by default.
const ServerPermission = "system.admin"

// ServerAdminRole is the built-in role of server administrators (ADR-0019).
const ServerAdminRole = "server_admin"

// PermissionsOfRoles returns the union of the permissions of the named roles as the organization resolves them
// (an organization role shadows a built-in one of the same name, as when granting). Unknown names are an error.
func (s *Service) PermissionsOfRoles(ctx context.Context, orgID string, names []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, n := range names {
		var perms []string
		err := s.pool.QueryRow(ctx, `SELECT permissions FROM core.role WHERE name = $1 AND (org_id IS NULL OR org_id = $2::uuid) ORDER BY org_id NULLS LAST LIMIT 1`, n, orgID).Scan(&perms)
		if err != nil {
			return nil, fmt.Errorf("role %q not found", n)
		}
		for _, p := range perms {
			out[p] = true
		}
	}
	return out, nil
}

// ServerAdmins counts enabled accounts (in enabled organizations) that hold server administration through any
// current role, leaving out one user (excludeUser) and/or one role (excludeRole). It is the "last server
// administrator" check: a change that would leave zero is refused.
func (s *Service) ServerAdmins(ctx context.Context, excludeUser, excludeRole string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `WITH cur AS (
		SELECT user_id, role_id, (array_agg(event ORDER BY ts DESC, id DESC))[1] AS ev FROM core.user_role_history GROUP BY user_id, role_id)
		SELECT count(DISTINCT u.id) FROM cur JOIN core.role r ON r.id = cur.role_id JOIN core.user_account u ON u.id = cur.user_id
		JOIN core.organization o ON o.id = u.org_id
		WHERE cur.ev = 'granted' AND $1 = ANY (r.permissions) AND u.disabled_at IS NULL AND o.disabled_at IS NULL
		  AND ($2 = '' OR u.id::text <> $2) AND ($3 = '' OR r.id::text <> $3)`, ServerPermission, excludeUser, excludeRole).Scan(&n)
	return n, err
}

// AdminContact is a server administrator with an e-mail address (notices of the server: updates).
type AdminContact struct {
	OrgID, UserID, Username, Email string
}

// ServerAdminContacts lists the enabled server administrators that have an e-mail address.
func (s *Service) ServerAdminContacts(ctx context.Context) ([]AdminContact, error) {
	rows, err := s.pool.Query(ctx, `WITH cur AS (
		SELECT user_id, role_id, (array_agg(event ORDER BY ts DESC, id DESC))[1] AS ev FROM core.user_role_history GROUP BY user_id, role_id)
		SELECT DISTINCT u.org_id::text, u.id::text, u.username, u.email FROM cur JOIN core.role r ON r.id = cur.role_id JOIN core.user_account u ON u.id = cur.user_id
		JOIN core.organization o ON o.id = u.org_id
		WHERE cur.ev = 'granted' AND $1 = ANY (r.permissions) AND u.disabled_at IS NULL AND o.disabled_at IS NULL AND COALESCE(u.email, '') <> ''`, ServerPermission)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminContact
	for rows.Next() {
		var c AdminContact
		if err := rows.Scan(&c.OrgID, &c.UserID, &c.Username, &c.Email); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// RoleHistory lists grants and revokes of a user, newest first.
func (s *Service) RoleHistory(ctx context.Context, userID string) ([]RoleEvent, error) {
	rows, err := s.pool.Query(ctx, `SELECT h.ts, r.name, h.event, COALESCE(a.username, ''), h.reason
		FROM core.user_role_history h JOIN core.role r ON r.id = h.role_id LEFT JOIN core.user_account a ON a.id = h.actor_id
		WHERE h.user_id = $1::uuid ORDER BY h.ts DESC, h.id DESC LIMIT 200`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RoleEvent{}
	for rows.Next() {
		var e RoleEvent
		if err := rows.Scan(&e.TS, &e.Role, &e.Event, &e.ActorName, &e.Reason); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ResetMFA removes the TOTP enrolment and recovery codes of a user and revokes their sessions; the user must
// enrol again at the next login if the policy requires it.
func (s *Service) ResetMFA(ctx context.Context, userID string) error {
	if _, err := s.pool.Exec(ctx, `UPDATE core.user_account SET totp_secret_enc = NULL, totp_confirmed_at = NULL, mfa_enabled = false, totp_last_step = 0 WHERE id = $1::uuid`, userID); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `UPDATE core.user_recovery_code SET used_at = now() WHERE user_id = $1::uuid AND used_at IS NULL`, userID); err != nil {
		return err
	}
	_, err := s.RevokeSessions(ctx, userID)
	return err
}

// RevokeSessions signs the user out everywhere.
func (s *Service) RevokeSessions(ctx context.Context, userID string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE core.session SET revoked_at = now() WHERE user_id = $1::uuid AND revoked_at IS NULL`, userID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Unlock clears a lockout without touching the disabled state.
func (s *Service) Unlock(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE core.user_account SET failed_logins = 0, locked_until = NULL WHERE id = $1::uuid`, userID)
	return err
}

// UpdateProfile changes display name and e-mail.
func (s *Service) UpdateProfile(ctx context.Context, userID, fullName, email string) error {
	fullName = strings.TrimSpace(fullName)
	email = strings.TrimSpace(email)
	if len(fullName) > 128 || len(email) > 254 {
		return errors.New("name or e-mail too long")
	}
	if email != "" && !strings.Contains(email, "@") {
		return errors.New("invalid e-mail")
	}
	_, err := s.pool.Exec(ctx, `UPDATE core.user_account SET full_name = $2, email = NULLIF($3,'') WHERE id = $1::uuid`, userID, fullName, email)
	return err
}

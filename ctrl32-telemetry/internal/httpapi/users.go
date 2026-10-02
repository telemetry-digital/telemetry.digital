package httpapi

import (
	"net/http"
	"strings"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

// User and role administration (URS H-04, H-05, I-01). Everything an administrator can see in the UI comes
// from these endpoints; every change carries a reason and lands in the audit trail.
//
//	GET  /api/v1/permissions                       catalog                                  (signed in)
//	GET  /api/v1/roles                             built-in + organization roles            (signed in)
//	POST /api/v1/roles                             {name, permissions, reason}              (user.admin)
//	PUT  /api/v1/roles/{id}                        {permissions, reason}                    (user.admin, org roles only)
//	GET  /api/v1/users/{id}                        user + effective permissions + history   (user.admin)
//	POST /api/v1/users/{id}/mfa-reset              {reason}                                 (user.admin)
//	POST /api/v1/users/{id}/sessions/revoke        {reason}                                 (user.admin)
//	POST /api/v1/users/{id}/unlock                 {reason}                                 (user.admin)

func (s *Server) listPermissions(w http.ResponseWriter, r *http.Request) {
	out, err := s.iam.ListPermissions(r.Context())
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) listRoles(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	out, err := s.iam.ListRoles(r.Context(), p.OrgID)
	if err != nil {
		s.log.Error("list roles", "err", err)
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) createRole(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	var in struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
		Reason      string   `json:"reason"`
	}
	if err := decodeJSON(r, &in, 8192); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason required")
		return
	}
	if hasPerm(in.Permissions, iam.ServerPermission) && !p.Has(iam.ServerPermission) {
		writeJSON(w, 403, map[string]string{"error": "a role with server administration (system.admin) can be created by a server administrator only", "code": "server_admin_required"})
		return
	}
	role, err := s.iam.CreateRole(r.Context(), p.OrgID, in.Name, in.Permissions)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.audit(r, "role.create", "role", role.ID, nil, map[string]any{"name": role.Name, "permissions": role.Permissions}, in.Reason)
	writeJSON(w, 201, role)
}

func (s *Server) updateRole(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var in struct {
		Permissions []string `json:"permissions"`
		Reason      string   `json:"reason"`
	}
	if err := decodeJSON(r, &in, 8192); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason required")
		return
	}
	before, err := s.iam.GetRole(r.Context(), p.OrgID, id)
	if err != nil {
		writeErr(w, 404, "role not found")
		return
	}
	if before.BuiltIn {
		writeErr(w, 403, "built-in roles cannot be changed; create an organization role instead")
		return
	}
	had, has := hasPerm(before.Permissions, iam.ServerPermission), hasPerm(in.Permissions, iam.ServerPermission)
	if (had || has) && !p.Has(iam.ServerPermission) {
		writeJSON(w, 403, map[string]string{"error": "a role with server administration (system.admin) can be changed by a server administrator only", "code": "server_admin_required"})
		return
	}
	if had && !has {
		if n, err := s.iam.ServerAdmins(r.Context(), "", id); err != nil || n == 0 {
			writeJSON(w, 409, map[string]string{"error": "the members of this role are the last server administrators; grant server_admin to someone else first", "code": "last_server_admin"})
			return
		}
	}
	after, err := s.iam.UpdateRole(r.Context(), p.OrgID, id, in.Permissions)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.audit(r, "role.update", "role", id, map[string]any{"permissions": before.Permissions}, map[string]any{"permissions": after.Permissions}, in.Reason)
	writeJSON(w, 200, after)
}

func hasPerm(list []string, perm string) bool {
	for _, x := range list {
		if strings.TrimSpace(x) == perm {
			return true
		}
	}
	return false
}

// GET /api/v1/users/{id}: the full picture of one account.
func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	u, err := s.iam.GetUser(r.Context(), id)
	if err != nil || u.OrgID != p.OrgID {
		writeErr(w, 404, "user not found")
		return
	}
	perms, err := s.iam.UserPermissions(r.Context(), id)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	hist, err := s.iam.RoleHistory(r.Context(), id)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	tokens, _ := s.iam.ListAPITokens(r.Context(), id)
	if tokens == nil {
		tokens = []iam.APIToken{}
	}
	writeJSON(w, 200, map[string]any{"user": u, "permissions": perms, "role_history": hist, "api_tokens": tokens})
}

// ---- server administration is separate from organization administration (ADR-0019) ----------------------------------

// guardServerAdminTarget refuses changes to an account that administers the server (holds system.admin) unless the
// caller holds system.admin too: otherwise an organization administrator could take that account over (reset its
// password, two-factor sign-in or e-mail) and with it the whole server.
func (s *Server) guardServerAdminTarget(w http.ResponseWriter, r *http.Request, p *iam.Principal, userID string) bool {
	if p.Has(iam.ServerPermission) {
		return true
	}
	perms, err := s.iam.UserPermissions(r.Context(), userID)
	if err != nil {
		writeErr(w, 500, "query failed")
		return false
	}
	for _, x := range perms {
		if x == iam.ServerPermission {
			writeJSON(w, 403, map[string]string{"error": "this account administers the server; only a server administrator (system.admin) may change it", "code": "server_admin_required"})
			return false
		}
	}
	return true
}

// checkRoleChange decides whether p may give user userID the roles want (nil = the roles stay): granting or
// removing server administration needs system.admin, nobody removes their own administration roles, and the last
// server administrator keeps server_admin. disable reports a request to disable the account.
func (s *Server) checkRoleChange(w http.ResponseWriter, r *http.Request, p *iam.Principal, userID string, want []string, disable bool) bool {
	ctx := r.Context()
	curList, err := s.iam.UserPermissions(ctx, userID)
	if err != nil {
		writeErr(w, 500, "query failed")
		return false
	}
	cur := map[string]bool{}
	for _, x := range curList {
		cur[x] = true
	}
	next := cur
	if want != nil {
		if next, err = s.iam.PermissionsOfRoles(ctx, p.OrgID, want); err != nil {
			writeErr(w, 400, err.Error())
			return false
		}
	}
	if disable {
		next = map[string]bool{}
	}
	if cur[iam.ServerPermission] != next[iam.ServerPermission] && !p.Has(iam.ServerPermission) {
		writeJSON(w, 403, map[string]string{"error": "granting or removing server administration (server_admin) needs system.admin", "code": "server_admin_required"})
		return false
	}
	if userID == p.UserID {
		if disable {
			writeErr(w, 400, "cannot disable yourself")
			return false
		}
		for _, perm := range []string{"user.admin", iam.ServerPermission} {
			if cur[perm] && !next[perm] {
				writeErr(w, 400, "you cannot remove your own administration roles (org_admin, server_admin); ask another administrator")
				return false
			}
		}
	}
	if cur[iam.ServerPermission] && !next[iam.ServerPermission] {
		n, err := s.iam.ServerAdmins(ctx, userID, "")
		if err != nil {
			writeErr(w, 500, "query failed")
			return false
		}
		if n == 0 {
			writeJSON(w, 409, map[string]string{"error": "this is the last server administrator; grant server_admin to someone else first", "code": "last_server_admin"})
			return false
		}
	}
	return true
}

// userAction runs an administrative action on another account of the same organization, with a reason.
func (s *Server) userAction(action string, fn func(r *http.Request, p *iam.Principal, u *iam.User) (map[string]any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := PrincipalFrom(r.Context())
		id := r.PathValue("id")
		var in struct{ Reason string }
		if err := decodeJSON(r, &in, 1024); err != nil || strings.TrimSpace(in.Reason) == "" {
			writeErr(w, 400, "reason required")
			return
		}
		u, err := s.iam.GetUser(r.Context(), id)
		if err != nil || u.OrgID != p.OrgID {
			writeErr(w, 404, "user not found")
			return
		}
		if !s.guardServerAdminTarget(w, r, p, u.ID) {
			return
		}
		detail, err := fn(r, p, u)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		s.audit(r, action, "user", u.ID, nil, detail, in.Reason)
		writeJSON(w, 200, map[string]any{"ok": true, "detail": detail})
	}
}

func (s *Server) resetUserMFA() http.HandlerFunc {
	return s.userAction("user.mfa_reset", func(r *http.Request, p *iam.Principal, u *iam.User) (map[string]any, error) {
		if err := s.iam.ResetMFA(r.Context(), u.ID); err != nil {
			return nil, err
		}
		return map[string]any{"username": u.Username, "by": p.Username}, nil
	})
}

func (s *Server) revokeUserSessions() http.HandlerFunc {
	return s.userAction("user.sessions_revoke", func(r *http.Request, p *iam.Principal, u *iam.User) (map[string]any, error) {
		n, err := s.iam.RevokeSessions(r.Context(), u.ID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"username": u.Username, "sessions": n, "by": p.Username}, nil
	})
}

func (s *Server) unlockUser() http.HandlerFunc {
	return s.userAction("user.unlock", func(r *http.Request, p *iam.Principal, u *iam.User) (map[string]any, error) {
		if err := s.iam.Unlock(r.Context(), u.ID); err != nil {
			return nil, err
		}
		return map[string]any{"username": u.Username, "by": p.Username}, nil
	})
}

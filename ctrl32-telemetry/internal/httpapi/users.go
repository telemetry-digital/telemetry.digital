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
	after, err := s.iam.UpdateRole(r.Context(), p.OrgID, id, in.Permissions)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.audit(r, "role.update", "role", id, map[string]any{"permissions": before.Permissions}, map[string]any{"permissions": after.Permissions}, in.Reason)
	writeJSON(w, 200, after)
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

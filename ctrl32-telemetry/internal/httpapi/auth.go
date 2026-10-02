package httpapi

import (
	"encoding/base64"

	qrcode "github.com/skip2/go-qrcode"

	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

// POST /api/v1/auth/login
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.loginLimiter.allow(ip) {
		s.log.Warn("login rate limited", "security", true, "ip", ip)
		writeErr(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	var in struct{ Username, Password string }
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	res, err := s.iam.Login(r.Context(), in.Username, in.Password, ip, r.UserAgent())
	switch {
	case errors.Is(err, iam.ErrLocked):
		writeErr(w, http.StatusLocked, "account temporarily locked")
		return
	case errors.Is(err, iam.ErrDisabled), errors.Is(err, iam.ErrOrgDisabled), errors.Is(err, iam.ErrInvalidCredentials):
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	case err != nil:
		s.log.Error("login failed", "err", err)
		writeErr(w, 500, "login failed")
		return
	}
	if res.MFAPending {
		writeJSON(w, 200, map[string]any{"mfa_required": true, "pre_auth_token": res.PreAuthToken})
		return
	}
	s.setSessionCookie(w, r, res.SessionToken, 0)
	writeJSON(w, 200, map[string]any{"mfa_required": false, "must_change_password": res.MustChange, "user": res.User})
}

// POST /api/v1/auth/mfa
func (s *Server) loginMFA(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.loginLimiter.allow(ip) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	var in struct {
		PreAuthToken string `json:"pre_auth_token"`
		Code         string `json:"code"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	token, err := s.iam.CompleteMFA(r.Context(), in.PreAuthToken, in.Code, ip, r.UserAgent())
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid code")
		return
	}
	s.setSessionCookie(w, r, token, 0)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/v1/auth/logout
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.iam.Logout(r.Context(), c.Value, clientIP(r), r.UserAgent())
	}
	s.setSessionCookie(w, r, "", -1)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/v1/auth/me
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	if p == nil {
		writeErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	perms := make([]string, 0, len(p.Permissions))
	for k := range p.Permissions {
		perms = append(perms, k)
	}
	locale := "en"
	_ = s.pool.QueryRow(r.Context(), `SELECT locale FROM core.user_account WHERE id = $1::uuid`, p.UserID).Scan(&locale)
	writeJSON(w, 200, map[string]any{
		"user_id": p.UserID, "org_id": p.OrgID, "username": p.Username, "full_name": p.FullName, "locale": locale,
		"roles": p.Roles, "permissions": perms, "csrf_token": p.CSRFSecret,
		"must_change_password": p.MustChange, "mfa_enabled": p.MFAEnabled, "mfa_required": p.MFARequired,
		"auth": map[string]bool{"session": p.SessionID != "", "token": p.TokenID != ""},
	})
}

// POST /api/v1/auth/password {current, new}
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	var in struct{ Current, New string }
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.iam.ChangePassword(r.Context(), p.UserID, in.Current, in.New, false); err != nil {
		if errors.Is(err, iam.ErrInvalidCredentials) {
			writeErr(w, http.StatusUnauthorized, "current password is wrong")
			return
		}
		writeErr(w, 400, err.Error())
		return
	}
	s.audit(r, "user.password_change", "user", p.UserID, nil, map[string]any{"self": true}, "")
	// Other sessions were revoked; this one keeps working until the cookie is replaced on next login.
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/v1/auth/totp/setup
func (s *Server) totpSetup(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	setup, err := s.iam.BeginTOTP(r.Context(), p.UserID, p.Username, s.productName(r, p.OrgID))
	if err != nil {
		writeErr(w, 500, "totp setup failed")
		return
	}
	// QR code for authenticator apps (served inline as a data URL; the CSP allows img-src data:)
	qr := ""
	if png, err := qrcode.Encode(setup.URL, qrcode.Medium, 256); err == nil {
		qr = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	}
	writeJSON(w, 200, map[string]any{"secret": setup.Secret, "otpauth_url": setup.URL, "qr": qr})
}

// POST /api/v1/auth/totp/confirm {code}
func (s *Server) totpConfirm(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	var in struct{ Code string }
	if err := decodeJSON(r, &in, 1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	codes, err := s.iam.ConfirmTOTP(r.Context(), p.UserID, in.Code)
	if err != nil {
		s.log.Warn("totp confirm rejected", "security", true, "user", p.Username, "err", err)
		writeErr(w, 400, "code not accepted")
		return
	}
	s.audit(r, "user.mfa_enable", "user", p.UserID, nil, map[string]any{"method": "totp"}, "")
	writeJSON(w, 200, map[string]any{"ok": true, "recovery_codes": codes})
}

// --- users (user.admin) -------------------------------------------------------------------------

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	users, err := s.iam.ListUsers(r.Context(), p.OrgID)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, users)
}

// POST /api/v1/users {username, full_name, email, roles, reason} -> user + one-time initial password
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	var in struct {
		Username string   `json:"username"`
		FullName string   `json:"full_name"`
		Email    string   `json:"email"`
		Roles    []string `json:"roles"`
		Reason   string   `json:"reason"`
	}
	if err := decodeJSON(r, &in, 8192); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason required")
		return
	}
	pw, err := iam.RandomPassword()
	if err != nil {
		writeErr(w, 500, "password generation failed")
		return
	}
	if perms, err := s.iam.PermissionsOfRoles(r.Context(), p.OrgID, in.Roles); err != nil {
		writeErr(w, 400, err.Error())
		return
	} else if perms[iam.ServerPermission] && !p.Has(iam.ServerPermission) {
		writeJSON(w, 403, map[string]string{"error": "granting server administration (server_admin) needs system.admin", "code": "server_admin_required"})
		return
	}
	u, err := s.iam.CreateUser(r.Context(), p.OrgID, in.Username, in.FullName, in.Email, pw, in.Roles, p.UserID, in.Reason)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	detail := map[string]any{"username": u.Username, "roles": u.Roles}
	// an administrator limited to camera groups creates users limited to the same groups (never wider)
	if p.CameraLimited {
		groups := append([]string{}, p.CameraGroups...) // never nil: NULL would mean all cameras
		if _, err := s.pool.Exec(r.Context(), `UPDATE core.user_account SET camera_groups = $2::text[] WHERE id = $1::uuid`, u.ID, groups); err != nil {
			writeErr(w, 500, "camera groups not set")
			return
		}
		detail["camera_groups"] = p.CameraGroups
	}
	s.audit(r, "user.create", "user", u.ID, nil, detail, in.Reason)
	writeJSON(w, 201, map[string]any{"user": u, "initial_password": pw})
}

// PATCH /api/v1/users/{id} {roles?, disabled?, reason}
func (s *Server) patchUser(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var in struct {
		Roles    *[]string `json:"roles"`
		Disabled *bool     `json:"disabled"`
		FullName *string   `json:"full_name"`
		Email    *string   `json:"email"`
		Reason   string    `json:"reason"`
	}
	if err := decodeJSON(r, &in, 8192); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason required")
		return
	}
	before, err := s.iam.GetUser(r.Context(), id)
	if err != nil || before.OrgID != p.OrgID {
		writeErr(w, 404, "user not found")
		return
	}
	// Server-side protection (not only the dialog): accounts of server administrators are changed by server
	// administrators only, nobody disables themselves or drops their own administration roles, and the last server
	// administrator stays.
	if id != p.UserID && !s.guardServerAdminTarget(w, r, p, id) {
		return
	}
	var wantRoles []string
	if in.Roles != nil {
		wantRoles = *in.Roles
	}
	if (in.Roles != nil || (in.Disabled != nil && *in.Disabled)) && !s.checkRoleChange(w, r, p, id, wantRoles, in.Disabled != nil && *in.Disabled) {
		return
	}
	if in.FullName != nil || in.Email != nil {
		name, email := before.FullName, ""
		if before.Email != nil {
			email = *before.Email
		}
		if in.FullName != nil {
			name = *in.FullName
		}
		if in.Email != nil {
			email = *in.Email
		}
		if err := s.iam.UpdateProfile(r.Context(), id, name, email); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	if in.Roles != nil {
		if err := s.iam.SetRoles(r.Context(), p.OrgID, id, *in.Roles, p.UserID, in.Reason); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	if in.Disabled != nil {
		if *in.Disabled && id == p.UserID {
			writeErr(w, 400, "cannot disable yourself")
			return
		}
		if err := s.iam.SetDisabled(r.Context(), id, *in.Disabled); err != nil {
			writeErr(w, 500, "update failed")
			return
		}
	}
	after, _ := s.iam.GetUser(r.Context(), id)
	s.audit(r, "user.update", "user", id, before, after, in.Reason)
	writeJSON(w, 200, after)
}

// POST /api/v1/users/{id}/reset-password {reason} -> one-time password
func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
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
	if !s.guardServerAdminTarget(w, r, p, id) {
		return
	}
	pw, _ := iam.RandomPassword()
	if err := s.iam.ChangePassword(r.Context(), id, "", pw, true); err != nil {
		writeErr(w, 500, "reset failed")
		return
	}
	s.audit(r, "user.password_reset", "user", id, nil, map[string]any{"by": p.Username}, in.Reason)
	writeJSON(w, 200, map[string]any{"initial_password": pw})
}

// --- API tokens (token.manage) ------------------------------------------------------------------

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	t, err := s.iam.ListAPITokens(r.Context(), p.UserID)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	if t == nil {
		t = []iam.APIToken{}
	}
	writeJSON(w, 200, t)
}

// POST /api/v1/tokens {name, scopes, expires_at?}
func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	var in struct {
		Name      string     `json:"name"`
		Scopes    []string   `json:"scopes"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	raw, t, err := s.iam.CreateAPIToken(r.Context(), p, in.Name, in.Scopes, in.ExpiresAt)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.audit(r, "api_token.create", "api_token", t.ID, nil, map[string]any{"name": t.Name, "scopes": t.Scopes}, "")
	writeJSON(w, 201, map[string]any{"token": raw, "info": t})
}

// DELETE /api/v1/tokens/{id}
func (s *Server) revokeToken(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	if err := s.iam.RevokeAPIToken(r.Context(), p.UserID, id); err != nil {
		writeErr(w, 404, "token not found")
		return
	}
	s.audit(r, "api_token.revoke", "api_token", id, nil, nil, "revoked by owner")
	writeJSON(w, 200, map[string]any{"ok": true})
}

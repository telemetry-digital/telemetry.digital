package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

// Self-service flows kept off the sign-in form (URS H-*): forgotten password by e-mail link, account requests.
//
//	POST /api/v1/auth/forgot            {"login": "username or e-mail"}       always 200; sends a link when possible
//	POST /api/v1/auth/reset             {"token", "password"}                  consumes the link
//	POST /api/v1/auth/register-request  {"full_name", "email", "message"}      stored for administrators
//	GET  /api/v1/account-requests       (user.admin)                            open requests
//	POST /api/v1/account-requests/{id}/dismiss  {"reason"}                      (user.admin)

func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	if !s.loginLimiter.allow(clientIP(r)) {
		writeErr(w, 429, "too many requests")
		return
	}
	var in struct{ Login string }
	if err := decodeJSON(r, &in, 1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	login := strings.ToLower(strings.TrimSpace(in.Login))
	nf := s.mailer()
	resp := map[string]any{"ok": true, "mail_configured": nf.Available()}
	if login == "" {
		writeJSON(w, 200, resp)
		return
	}
	var id, email, username string
	err := s.pool.QueryRow(r.Context(), `SELECT id::text, COALESCE(email,''), username FROM core.user_account WHERE (username = $1 OR lower(email) = $1) AND disabled_at IS NULL`, login).Scan(&id, &email, &username)
	if err != nil || email == "" || !nf.Available() {
		// never reveal whether the account exists
		s.log.Info("password reset requested", "login", login, "delivered", false)
		writeJSON(w, 200, resp)
		return
	}
	raw, err := iam.RandomToken(32)
	if err != nil {
		writeJSON(w, 200, resp)
		return
	}
	if _, err := s.pool.Exec(r.Context(), `INSERT INTO core.password_reset (user_id, token_hash, expires_at, ip) VALUES ($1::uuid, $2, now() + interval '1 hour', NULLIF($3,'')::inet)`, id, iam.HashToken(raw), clientIP(r)); err != nil {
		writeJSON(w, 200, resp)
		return
	}
	link := s.cfg.HTTP.BaseURL + "/reset?token=" + raw
	product := s.productName(r, "")
	body := "Hello " + username + ",\n\nsomeone (probably you) asked for a new password for your " + product + " account.\nOpen this link within one hour to set a new password:\n\n" + link + "\n\nIf you did not ask for it, ignore this message; your password stays unchanged.\n"
	if err := nf.SendTo(r.Context(), email, product+" — password reset", body); err != nil {
		s.log.Warn("password reset mail failed", "err", err)
	}
	s.audit(r, "user.password_reset_link", "user", id, nil, map[string]any{"ip": clientIP(r)}, "self-service")
	writeJSON(w, 200, resp)
}

func (s *Server) resetPasswordByLink(w http.ResponseWriter, r *http.Request) {
	if !s.loginLimiter.allow(clientIP(r)) {
		writeErr(w, 429, "too many requests")
		return
	}
	var in struct{ Token, Password string }
	if err := decodeJSON(r, &in, 2048); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var rid, uid string
	err := s.pool.QueryRow(r.Context(), `SELECT id::text, user_id::text FROM core.password_reset WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()`, iam.HashToken(in.Token)).Scan(&rid, &uid)
	if err != nil {
		writeErr(w, 400, "the link is invalid or has expired")
		return
	}
	if err := s.iam.SetPassword(r.Context(), uid, in.Password, false, true); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.password_reset SET used_at = now() WHERE id = $1::uuid`, rid)
	s.audit(r, "user.password_reset", "user", uid, nil, map[string]any{"via": "reset link", "ip": clientIP(r)}, "self-service")
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) registerRequest(w http.ResponseWriter, r *http.Request) {
	if !s.provisionLimiter.allow(clientIP(r)) {
		writeErr(w, 429, "too many requests")
		return
	}
	var in struct {
		FullName string `json:"full_name"`
		Email    string `json:"email"`
		Message  string `json:"message"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.FullName, in.Email = strings.TrimSpace(in.FullName), strings.ToLower(strings.TrimSpace(in.Email))
	if in.FullName == "" || len(in.FullName) > 128 || !strings.Contains(in.Email, "@") || len(in.Email) > 254 || len(in.Message) > 1000 {
		writeErr(w, 400, "name and a valid e-mail are required")
		return
	}
	orgID, _ := s.brandingOrg(r)
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.account_request (org_id, full_name, email, message, ip) VALUES (NULLIF($1,'')::uuid, $2, $3, NULLIF($4,''), NULLIF($5,'')::inet) RETURNING id::text`, orgID, in.FullName, in.Email, in.Message, clientIP(r)).Scan(&id); err != nil {
		writeErr(w, 500, "could not store the request")
		return
	}
	nf := s.mailer()
	if nf.Configured() {
		_, _ = nf.Send(r.Context(), s.productName(r, "")+" — account request", "Account request from "+in.FullName+" <"+in.Email+">\n\n"+in.Message+"\n\nHandle it under Users on "+s.cfg.HTTP.BaseURL+"/users\n")
	}
	s.audit(r, "account.request", "account_request", id, nil, map[string]any{"email": in.Email, "ip": clientIP(r)}, "self-service")
	writeJSON(w, 201, map[string]any{"ok": true})
}

func (s *Server) listAccountRequests(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT id::text, full_name, email, COALESCE(message,''), created_at FROM core.account_request WHERE (org_id = $1::uuid OR org_id IS NULL) AND handled_at IS NULL ORDER BY created_at DESC LIMIT 200`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, email, msg string
		var at time.Time
		if rows.Scan(&id, &name, &email, &msg, &at) == nil {
			out = append(out, map[string]any{"id": id, "full_name": name, "email": email, "message": msg, "created_at": at})
		}
	}
	writeJSON(w, 200, out)
}

// handleAccountRequest marks a request as created (after the admin created the account) or dismissed.
func (s *Server) handleAccountRequest(outcome string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org, ok := s.mustOrg(w, r)
		if !ok {
			return
		}
		p := PrincipalFrom(r.Context())
		id := r.PathValue("id")
		var in struct{ Reason string }
		_ = decodeJSON(r, &in, 1024)
		tag, err := s.pool.Exec(r.Context(), `UPDATE core.account_request SET handled_at = now(), handled_by = $3, outcome = $4 WHERE id = $1::uuid AND (org_id = $2::uuid OR org_id IS NULL) AND handled_at IS NULL`, id, org, p.Username, outcome)
		if err != nil || tag.RowsAffected() == 0 {
			writeErr(w, 404, "request not found")
			return
		}
		s.audit(r, "account.request_"+outcome, "account_request", id, nil, nil, in.Reason)
		writeJSON(w, 200, map[string]any{"ok": true})
	}
}

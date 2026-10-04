package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/notify"
)

// Outgoing e-mail of the server from the user interface (0.76.0, spec 031), System → Server → E-mail (SMTP),
// system.admin:
//
//	GET    /api/v1/server/smtp        the settings in use and their source (ui, config, none); never the password
//	PUT    /api/v1/server/smtp        {host, port, security, user, password, clear_password, from, from_name, reply_to,
//	                                   to, reason}: an empty password keeps the stored one
//	DELETE /api/v1/server/smtp        {reason}: back to [smtp] of config.toml
//	POST   /api/v1/server/smtp/test   {to, settings?}: a test message with the SMTP conversation
//
// Precedence: the settings saved here apply instead of [smtp] of config.toml; with [smtp] override = true the file
// applies and this page is read-only. The password is stored encrypted with the server's secret box (the key comes
// from security.secret_key, which never enters the database) and is never returned; an AI assistant cannot set it.

type smtpStored struct {
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	Security    string   `json:"security"`
	User        string   `json:"user,omitempty"`
	PasswordEnc string   `json:"password_enc,omitempty"`
	From        string   `json:"from"`
	FromName    string   `json:"from_name,omitempty"`
	ReplyTo     string   `json:"reply_to,omitempty"`
	To          []string `json:"to,omitempty"`
}

type smtpInput struct {
	Host          string   `json:"host"`
	Port          int      `json:"port"`
	Security      string   `json:"security"`
	User          string   `json:"user"`
	Password      string   `json:"password"`
	ClearPassword bool     `json:"clear_password"`
	From          string   `json:"from"`
	FromName      string   `json:"from_name"`
	ReplyTo       string   `json:"reply_to"`
	To            []string `json:"to"`
}

// localSMTPHost reports whether a host is this machine or in a local network: the only places an unencrypted SMTP
// connection is accepted.
func localSMTPHost(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	if ip := net.ParseIP(strings.Trim(h, "[]")); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	if !strings.Contains(h, ".") {
		return true // a name of the local network (no domain)
	}
	for _, suf := range []string{".local", ".lan", ".internal", ".home.arpa", ".intranet", ".corp"} {
		if strings.HasSuffix(h, suf) {
			return true
		}
	}
	return false
}

// validateSMTP checks the settings and returns warnings worth showing.
func validateSMTP(in *smtpInput) ([]string, error) {
	in.Host, in.User, in.From, in.FromName, in.ReplyTo = strings.TrimSpace(in.Host), strings.TrimSpace(in.User), strings.TrimSpace(in.From), strings.TrimSpace(in.FromName), strings.TrimSpace(in.ReplyTo)
	in.Security = strings.ToLower(strings.TrimSpace(in.Security))
	var warn []string
	if in.Host == "" || len(in.Host) > 253 || strings.ContainsAny(in.Host, " /\\@:") && net.ParseIP(strings.Trim(in.Host, "[]")) == nil {
		return nil, errors.New("host: a host name or an IP address of the SMTP server")
	}
	if in.Port == 0 {
		in.Port = 587
		if in.Security == notify.SecurityTLS {
			in.Port = 465
		}
	}
	if in.Port < 1 || in.Port > 65535 {
		return nil, errors.New("port must be between 1 and 65535")
	}
	switch in.Security {
	case notify.SecuritySTARTTLS, notify.SecurityTLS:
	case notify.SecurityNone:
		if !localSMTPHost(in.Host) {
			return nil, errors.New("an unencrypted connection is allowed only to a relay on this server or in the local network; use STARTTLS or SSL/TLS")
		}
		warn = append(warn, "the connection is not encrypted: messages and the password travel in plain text inside the local network")
	default:
		return nil, errors.New("security must be starttls, tls or none")
	}
	if in.Security == notify.SecuritySTARTTLS && in.Port == 465 {
		warn = append(warn, "port 465 usually expects SSL/TLS, not STARTTLS")
	}
	if in.Security == notify.SecurityTLS && in.Port == 587 {
		warn = append(warn, "port 587 usually expects STARTTLS, not SSL/TLS")
	}
	if len(in.User) > 256 || len(in.Password) > 512 || strings.ContainsAny(in.Password, "\r\n") {
		return nil, errors.New("user or password too long")
	}
	if a, err := mail.ParseAddress(in.From); err != nil || a.Address != in.From {
		return nil, errors.New("from must be an e-mail address (the name goes into from_name)")
	}
	if len(in.FromName) > 100 || strings.ContainsAny(in.FromName, "\r\n<>\"") {
		return nil, errors.New("from_name: up to 100 characters, no quotes or angle brackets")
	}
	if in.ReplyTo != "" {
		if a, err := mail.ParseAddress(in.ReplyTo); err != nil || a.Address != in.ReplyTo {
			return nil, errors.New("reply_to must be an e-mail address")
		}
	}
	var to []string
	for _, t := range in.To {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if a, err := mail.ParseAddress(t); err != nil || a.Address != t {
			return nil, errors.New("alarm recipient " + t + " is not an e-mail address")
		}
		to = append(to, t)
	}
	if len(to) > 50 {
		return nil, errors.New("at most 50 alarm recipients")
	}
	in.To = to
	return warn, nil
}

func (s *Server) secretBox() (*iam.SecretBox, error) {
	return iam.NewSecretBox(s.cfg.Security.SecretKey)
}

// mailer is the notifier with the outgoing e-mail in use (a notifier of config.toml when none is wired, e.g. in tests).
func (s *Server) mailer() *notify.Notifier {
	if s.notifier != nil {
		return s.notifier
	}
	return notify.New(s.cfg.SMTP, s.log)
}

// loadSMTPStored returns the saved settings (ok false when none are saved).
func (s *Server) loadSMTPStored(ctx context.Context) (smtpStored, bool) {
	var st smtpStored
	if s.pool == nil {
		return st, false
	}
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT data FROM core.server_setting WHERE key = 'smtp'`).Scan(&raw); err != nil {
		return st, false
	}
	return st, json.Unmarshal(raw, &st) == nil
}

// smtpConfig turns saved settings into the configuration of the notifier (the password decrypted).
func (s *Server) smtpConfig(st smtpStored) (config.SMTPConfig, error) {
	c := config.SMTPConfig{Host: st.Host, Port: st.Port, Security: st.Security, User: st.User, From: st.From, FromName: st.FromName, ReplyTo: st.ReplyTo, To: st.To}
	if st.PasswordEnc != "" {
		box, err := s.secretBox()
		if err != nil {
			return c, err
		}
		blob, err := base64.StdEncoding.DecodeString(st.PasswordEnc)
		if err != nil {
			return c, err
		}
		pw, err := box.Open(blob)
		if err != nil {
			return c, errors.New("the stored SMTP password cannot be decrypted (security.secret_key changed?)")
		}
		c.Password = string(pw)
	}
	return c, nil
}

// smtpSource is where the settings in use come from: "ui", "config" or "none".
func (s *Server) smtpSource(ctx context.Context) (string, smtpStored, bool) {
	st, saved := s.loadSMTPStored(ctx)
	switch {
	case saved && !s.cfg.SMTP.Override:
		return "ui", st, true
	case s.cfg.SMTP.Host != "":
		return "config", st, saved
	}
	return "none", st, saved
}

// applySMTP hands the settings in use to the notifier (at start and after every change).
func (s *Server) applySMTP(ctx context.Context) {
	if s.notifier == nil {
		return
	}
	src, st, _ := s.smtpSource(ctx)
	if src != "ui" {
		s.notifier.SetSMTP(s.cfg.SMTP)
		return
	}
	c, err := s.smtpConfig(st)
	if err != nil {
		s.log.Error("smtp settings of the user interface not applied", "err", err)
		s.notifier.SetSMTP(s.cfg.SMTP)
		return
	}
	s.notifier.SetSMTP(c)
}

func smtpView(c config.SMTPConfig) map[string]any {
	to := c.To
	if to == nil {
		to = []string{}
	}
	return map[string]any{"host": c.Host, "port": c.Port, "security": c.Security, "user": c.User, "from": c.From, "from_name": c.FromName, "reply_to": c.ReplyTo, "to": to}
}

func (s *Server) serverSMTP(w http.ResponseWriter, r *http.Request) {
	src, st, saved := s.smtpSource(r.Context())
	out := map[string]any{"source": src, "override": s.cfg.SMTP.Override, "saved": saved, "config": smtpView(s.cfg.SMTP), "config_has_password": s.cfg.SMTP.Password != ""}
	switch src {
	case "ui":
		c, _ := s.smtpConfig(smtpStored{Host: st.Host, Port: st.Port, Security: st.Security, User: st.User, From: st.From, FromName: st.FromName, ReplyTo: st.ReplyTo, To: st.To})
		out["settings"], out["has_password"] = smtpView(c), st.PasswordEnc != ""
	default:
		out["settings"], out["has_password"] = smtpView(s.cfg.SMTP), s.cfg.SMTP.Password != ""
	}
	if s.notifier != nil {
		out["available"] = s.notifier.Available()
	}
	writeJSON(w, 200, out)
}

// noMCPSecret refuses a password sent by an AI assistant.
func noMCPSecret(w http.ResponseWriter, r *http.Request, password string) bool {
	p := PrincipalFrom(r.Context())
	if password != "" && (r.Context().Value(viaMCPKey) != nil || (p != nil && p.GrantID != "")) {
		writeJSON(w, 403, map[string]string{"error": "the SMTP password is set by a person, not by an AI assistant", "code": "session_required"})
		return false
	}
	return true
}

func (s *Server) serverSetSMTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		smtpInput
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) || !noMCPSecret(w, r, in.Password) {
		return
	}
	if s.cfg.SMTP.Override {
		writeErr(w, 409, "[smtp] override = true in config.toml: the outgoing e-mail is set in the file, not here")
		return
	}
	warn, err := validateSMTP(&in.smtpInput)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	old, saved := s.loadSMTPStored(r.Context())
	st := smtpStored{Host: in.Host, Port: in.Port, Security: in.Security, User: in.User, From: in.From, FromName: in.FromName, ReplyTo: in.ReplyTo, To: in.To}
	switch {
	case in.Password != "":
		box, err := s.secretBox()
		if err != nil {
			writeErr(w, 500, "the secret box is not available")
			return
		}
		blob, err := box.Seal([]byte(in.Password))
		if err != nil {
			writeErr(w, 500, "the password could not be encrypted")
			return
		}
		st.PasswordEnc = base64.StdEncoding.EncodeToString(blob)
	case !in.ClearPassword && saved:
		st.PasswordEnc = old.PasswordEnc
	}
	if st.User == "" {
		st.PasswordEnc = ""
	}
	b, _ := json.Marshal(st)
	p := PrincipalFrom(r.Context())
	if _, err := s.pool.Exec(r.Context(), `INSERT INTO core.server_setting (key, data, updated_at, updated_by) VALUES ('smtp', $1::jsonb, now(), $2::uuid)
		ON CONFLICT (key) DO UPDATE SET data = EXCLUDED.data, updated_at = now(), updated_by = EXCLUDED.updated_by`, string(b), p.UserID); err != nil {
		writeErr(w, 500, "could not save the settings")
		return
	}
	s.applySMTP(r.Context())
	view := func(x smtpStored, ok bool) map[string]any {
		if !ok {
			return nil
		}
		return map[string]any{"host": x.Host, "port": x.Port, "security": x.Security, "user": x.User, "from": x.From, "from_name": x.FromName, "reply_to": x.ReplyTo, "to": x.To, "has_password": x.PasswordEnc != ""}
	}
	nv := view(st, true)
	nv["password_changed"] = in.Password != "" || (in.ClearPassword && old.PasswordEnc != "")
	s.audit(r, "server.smtp", "server", "smtp", view(old, saved), nv, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true, "warnings": warn})
}

func (s *Server) serverResetSMTP(w http.ResponseWriter, r *http.Request) {
	var in struct{ Reason string }
	_ = decodeJSON(r, &in, 1024)
	if !reasonOf(w, in.Reason) {
		return
	}
	old, saved := s.loadSMTPStored(r.Context())
	if _, err := s.pool.Exec(r.Context(), `DELETE FROM core.server_setting WHERE key = 'smtp'`); err != nil {
		writeErr(w, 500, "could not remove the settings")
		return
	}
	s.applySMTP(r.Context())
	if saved {
		s.audit(r, "server.smtp_reset", "server", "smtp", map[string]any{"host": old.Host, "port": old.Port, "security": old.Security, "user": old.User, "from": old.From}, map[string]any{"source": "config.toml"}, in.Reason)
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// serverTestSMTP sends a test message with the settings of the form (unsaved) or the ones in use; an empty password in
// the form takes the stored one.
func (s *Server) serverTestSMTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		To       string     `json:"to"`
		Settings *smtpInput `json:"settings"`
	}
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.To = strings.TrimSpace(in.To)
	if a, err := mail.ParseAddress(in.To); err != nil || a.Address != in.To {
		writeErr(w, 400, "to: an e-mail address for the test message")
		return
	}
	var c config.SMTPConfig
	if in.Settings != nil {
		if !noMCPSecret(w, r, in.Settings.Password) {
			return
		}
		if _, err := validateSMTP(in.Settings); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		c = config.SMTPConfig{Host: in.Settings.Host, Port: in.Settings.Port, Security: in.Settings.Security, User: in.Settings.User, Password: in.Settings.Password,
			From: in.Settings.From, FromName: in.Settings.FromName, ReplyTo: in.Settings.ReplyTo}
		if c.Password == "" && c.User != "" {
			cur := s.mailer().SMTP()
			if src, st, _ := s.smtpSource(r.Context()); src == "ui" {
				if sc, err := s.smtpConfig(st); err == nil {
					cur = sc
				}
			}
			c.Password = cur.Password
		}
	} else {
		c = s.mailer().SMTP()
		if c.Host == "" {
			writeErr(w, 400, "no SMTP server is configured")
			return
		}
	}
	subject := productName + ": test e-mail"
	body := "This is a test message of the outgoing e-mail of " + strings.TrimRight(s.cfg.HTTP.BaseURL, "/") + ", sent " + time.Now().UTC().Format(time.RFC1123) +
		" from System → Server → E-mail (SMTP).\n"
	transcript, err := notify.TestSend(r.Context(), c, in.To, subject, body)
	ok := err == nil
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	p := PrincipalFrom(r.Context())
	status := map[bool]string{true: "sent", false: "failed"}[ok]
	payload, _ := json.Marshal(map[string]any{"subject": subject, "to": in.To, "kind": "smtp_test", "host": c.Host, "port": c.Port})
	_, _ = s.pool.Exec(r.Context(), `INSERT INTO core.notification (org_id, event, status, sent_at, attempts, error, payload) VALUES ($1::uuid, 'test', $2, now(), 1, NULLIF($3,''), $4::jsonb)`,
		p.OrgID, status, errText, string(payload))
	s.audit(r, "server.smtp_test", "server", "smtp", nil, map[string]any{"to": in.To, "host": c.Host, "port": c.Port, "security": c.Security, "ok": ok, "error": errText}, "test e-mail")
	writeJSON(w, 200, map[string]any{"ok": ok, "error": errText, "transcript": transcript})
}

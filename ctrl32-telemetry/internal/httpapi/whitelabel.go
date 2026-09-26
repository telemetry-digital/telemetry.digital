package httpapi

// White labeling (spec 007): with a paid license granting "white_label", an organization replaces the product name,
// the product footer, the browser icon and the PWA name with its own, and its name appears in e-mails, authenticator
// apps and PDF report footers. Without a valid license the settings are kept but not applied: the fixed product
// footer of ADR-0011 returns.
//
//	GET    /api/v1/server/license   system.admin: installed license and whether it is valid on this server
//	PUT    /api/v1/server/license   system.admin: {"token","reason"} installs a license issued with tools/licgen
//	DELETE /api/v1/server/license   system.admin: ?reason=… removes it
//	GET    /api/v1/branding/icon    public: the browser icon (white label icon, else the default)
//	GET    /manifest.webmanifest    public: PWA manifest with the product name

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/license"
)

const (
	whiteLabelIconMaxBytes = 64 * 1024
	licenseCacheFor        = time.Minute
)

// WhiteLabel is an organization's own product identity (stored in organization.branding.white_label).
type WhiteLabel struct {
	Enabled     bool         `json:"enabled"`
	ProductName string       `json:"product_name"` // replaces "ctrl32 telemetry" (titles, landing, e-mails, 2FA, PDF)
	ShortName   string       `json:"short_name"`   // installed app (PWA) name
	FooterText  string       `json:"footer_text"`  // replaces "ctrl32 · telemetry" in the footer
	Intro       string       `json:"intro"`        // start page introduction (empty = the standard text)
	FooterLinks []FooterLink `json:"footer_links"` // at most 4 (support, imprint, terms …)
	ShowVersion bool         `json:"show_version"`
	Icon        *logoData    `json:"icon,omitempty"` // browser icon, PNG or SVG ≤ 64 KiB
}

// FooterLink is one link of a white-labelled footer.
type FooterLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type licenseState struct {
	lic         *license.License
	token       string
	err         string
	installedAt time.Time
	installedBy string
	loaded      time.Time
}

type licenseCache struct {
	mu sync.Mutex
	st *licenseState
}

// licenseHost is the server's host name as configured (base URL); a license restricted to domains must name it.
func (s *Server) licenseHost() string {
	u, err := url.Parse(s.cfg.HTTP.BaseURL)
	if err != nil {
		return ""
	}
	return u.Host
}

// currentLicense returns the installed license (cached for a minute; reloaded after changes).
func (s *Server) currentLicense(ctx context.Context) *licenseState {
	s.lic.mu.Lock()
	defer s.lic.mu.Unlock()
	if s.lic.st != nil && time.Since(s.lic.st.loaded) < licenseCacheFor {
		return s.lic.st
	}
	st := &licenseState{loaded: time.Now()}
	err := s.pool.QueryRow(ctx, `SELECT token, installed_at, installed_by FROM core.product_license WHERE id = 1`).Scan(&st.token, &st.installedAt, &st.installedBy)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		st.err = "license not readable"
		st.loaded = time.Time{} // retry soon
	default:
		l, perr := license.Parse(st.token, license.TrustedKeys())
		if perr != nil {
			st.err = perr.Error()
		}
		st.lic = l
	}
	s.lic.st = st
	return st
}

func (s *Server) dropLicenseCache() {
	s.lic.mu.Lock()
	s.lic.st = nil
	s.lic.mu.Unlock()
}

// whiteLabelLicensed reports whether this server may apply white labeling now.
func (s *Server) whiteLabelLicensed(ctx context.Context) bool {
	return s.currentLicense(ctx).lic.Has(license.WhiteLabel, s.licenseHost(), time.Now())
}

// activeWhiteLabel returns the organization's white label when it is enabled and licensed, else nil.
func (s *Server) activeWhiteLabel(r *http.Request, orgID string, b branding) *WhiteLabel {
	if b.WhiteLabel == nil || !b.WhiteLabel.Enabled || !s.whiteLabelLicensed(r.Context()) {
		return nil
	}
	return b.WhiteLabel
}

// productName is the product name shown to an organization's users ("ctrl32 telemetry" unless white-labelled).
func (s *Server) productName(r *http.Request, orgID string) string {
	if orgID == "" {
		orgID, _ = s.brandingOrg(r)
	}
	if wl := s.activeWhiteLabel(r, orgID, s.loadBranding(r, orgID)); wl != nil && wl.ProductName != "" {
		return wl.ProductName
	}
	return productName
}

// reportProduct names the product in PDF report footers.
func (s *Server) reportProduct(ctx context.Context, orgID string) string {
	if orgID == "" || !s.whiteLabelLicensed(ctx) {
		return ""
	}
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT branding FROM core.organization WHERE id = $1::uuid`, orgID).Scan(&raw); err != nil {
		return ""
	}
	var b branding
	if json.Unmarshal(raw, &b) != nil || b.WhiteLabel == nil || !b.WhiteLabel.Enabled {
		return ""
	}
	if b.WhiteLabel.FooterText != "" {
		return b.WhiteLabel.FooterText
	}
	return b.WhiteLabel.ProductName
}

func wlText(s string, max int) bool {
	return len([]rune(s)) <= max && !strings.ContainsAny(s, "<>\r\n\t")
}

// validateWhiteLabel checks the settings from a request; icon is the optional new icon.
func validateWhiteLabel(in *WhiteLabel) string {
	in.ProductName, in.ShortName, in.FooterText = strings.TrimSpace(in.ProductName), strings.TrimSpace(in.ShortName), strings.TrimSpace(in.FooterText)
	if in.Enabled && in.ProductName == "" {
		return "white_label.product_name is required"
	}
	in.Intro = strings.TrimSpace(in.Intro)
	if !wlText(in.ProductName, 64) || !wlText(in.ShortName, 24) || !wlText(in.FooterText, 120) {
		return "white_label: product_name ≤ 64, short_name ≤ 24, footer_text ≤ 120 characters of plain text"
	}
	if len([]rune(in.Intro)) > 400 || strings.ContainsAny(in.Intro, "<>") {
		return "white_label.intro: plain text up to 400 characters"
	}
	if len(in.FooterLinks) > 4 {
		return "white_label.footer_links: at most 4"
	}
	for i := range in.FooterLinks {
		l := &in.FooterLinks[i]
		l.Label, l.URL = strings.TrimSpace(l.Label), strings.TrimSpace(l.URL)
		if l.Label == "" || !wlText(l.Label, 40) {
			return "white_label.footer_links: label of 1 to 40 characters"
		}
		ok := strings.HasPrefix(l.URL, "https://") || strings.HasPrefix(l.URL, "http://") || strings.HasPrefix(l.URL, "mailto:") ||
			(strings.HasPrefix(l.URL, "/") && !strings.HasPrefix(l.URL, "//"))
		if !ok || len(l.URL) > 300 || strings.ContainsAny(l.URL, " <>\"'") {
			return "white_label.footer_links: url must be http(s)://…, mailto:… or a path"
		}
	}
	return ""
}

// ---- license API ----------------------------------------------------------------------------------------------------

func (s *Server) licenseJSON(ctx context.Context) map[string]any {
	st := s.currentLicense(ctx)
	out := map[string]any{"installed": st.token != "", "valid": false, "server_host": s.licenseHost(), "trusted_keys": len(license.TrustedKeys())}
	if st.err != "" {
		out["problem"] = st.err
	}
	if st.lic != nil {
		c := st.lic.Check(s.licenseHost(), time.Now())
		out["valid"], out["id"], out["licensee"], out["email"] = c.Valid, st.lic.ID, st.lic.Licensee, st.lic.Email
		out["features"], out["domains"], out["issued"] = st.lic.Features, st.lic.Domains, st.lic.Issued
		if !st.lic.Expires.IsZero() {
			out["expires"] = st.lic.Expires
		}
		if c.Problem != "" {
			out["problem"] = c.Problem
		}
		out["installed_at"], out["installed_by"] = st.installedAt, st.installedBy
	}
	out["white_label"] = st.lic.Has(license.WhiteLabel, s.licenseHost(), time.Now())
	return out
}

func (s *Server) getLicense(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.licenseJSON(r.Context()))
}

func (s *Server) putLicense(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token  string `json:"token"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason is required")
		return
	}
	in.Token = strings.Join(strings.Fields(in.Token), "")
	l, err := license.Parse(in.Token, license.TrustedKeys())
	if err != nil {
		s.securityEvent(r, "license.rejected", map[string]any{"error": err.Error()})
		writeErr(w, 400, err.Error())
		return
	}
	if c := l.Check(s.licenseHost(), time.Now()); !c.Valid {
		writeErr(w, 400, "license not usable on this server: "+c.Problem)
		return
	}
	p := PrincipalFrom(r.Context())
	by := "system"
	if p != nil {
		by = p.Username
	}
	old := s.licenseJSON(r.Context())
	if _, err := s.pool.Exec(r.Context(), `INSERT INTO core.product_license (id, token, installed_at, installed_by) VALUES (1, $1, now(), $2)
		ON CONFLICT (id) DO UPDATE SET token = EXCLUDED.token, installed_at = now(), installed_by = EXCLUDED.installed_by`, in.Token, by); err != nil {
		writeErr(w, 500, "license not stored")
		return
	}
	s.dropLicenseCache()
	now := s.licenseJSON(r.Context())
	s.audit(r, "license.install", "license", l.ID, auditLicense(old), auditLicense(now), in.Reason)
	writeJSON(w, 200, now)
}

func (s *Server) deleteLicense(w http.ResponseWriter, r *http.Request) {
	reason := r.URL.Query().Get("reason")
	if strings.TrimSpace(reason) == "" {
		writeErr(w, 400, "reason is required")
		return
	}
	old := s.licenseJSON(r.Context())
	if _, err := s.pool.Exec(r.Context(), `DELETE FROM core.product_license WHERE id = 1`); err != nil {
		writeErr(w, 500, "license not removed")
		return
	}
	s.dropLicenseCache()
	id, _ := old["id"].(string)
	s.audit(r, "license.remove", "license", id, auditLicense(old), nil, reason)
	writeJSON(w, 200, s.licenseJSON(r.Context()))
}

func auditLicense(m map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{"id", "licensee", "features", "domains", "expires", "valid"} {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}

// securityEvent logs a security-relevant event (bad license tokens may be tampering attempts).
func (s *Server) securityEvent(r *http.Request, event string, detail map[string]any) {
	args := []any{"security", true, "event", event, "remote", clientIP(r)}
	for k, v := range detail {
		args = append(args, k, v)
	}
	s.log.Warn("security event", args...)
}

// ---- icon and manifest ----------------------------------------------------------------------------------------------

func (s *Server) getBrandingIcon(w http.ResponseWriter, r *http.Request) {
	orgID, _ := s.brandingOrg(r)
	b := s.loadBranding(r, orgID)
	if wl := s.activeWhiteLabel(r, orgID, b); wl != nil && wl.Icon != nil {
		raw, err := base64.StdEncoding.DecodeString(wl.Icon.Data)
		if err == nil {
			w.Header().Set("Content-Type", wl.Icon.Mime)
			w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "public, max-age=300")
			_, _ = w.Write(raw)
			return
		}
	}
	http.Redirect(w, r, "/favicon.svg", http.StatusFound)
}

func (s *Server) manifest(w http.ResponseWriter, r *http.Request) {
	sub, _ := fs.Sub(static, "static")
	raw, err := fs.ReadFile(sub, "manifest.webmanifest")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	orgID, _ := s.brandingOrg(r)
	b := s.loadBranding(r, orgID)
	wl := s.activeWhiteLabel(r, orgID, b)
	if wl == nil {
		_, _ = w.Write(raw)
		return
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		_, _ = w.Write(raw)
		return
	}
	m["name"] = wl.ProductName
	m["short_name"] = wl.ShortName
	if wl.ShortName == "" {
		m["short_name"] = wl.ProductName
	}
	m["description"] = wl.ProductName
	m["theme_color"] = b.Theme.Primary
	if wl.Icon != nil {
		m["icons"] = []map[string]any{{"src": "/api/v1/branding/icon", "sizes": "any", "type": wl.Icon.Mime, "purpose": "any"}}
	}
	out, _ := json.Marshal(m)
	_, _ = w.Write(out)
}

// whiteLabelOut is the stored white label without the icon bytes (settings page).
func whiteLabelOut(wl *WhiteLabel) map[string]any {
	if wl == nil {
		return map[string]any{"enabled": false, "footer_links": []FooterLink{}}
	}
	links := wl.FooterLinks
	if links == nil {
		links = []FooterLink{}
	}
	out := map[string]any{"enabled": wl.Enabled, "product_name": wl.ProductName, "short_name": wl.ShortName, "footer_text": wl.FooterText, "intro": wl.Intro,
		"footer_links": links, "show_version": wl.ShowVersion, "icon": wl.Icon != nil}
	if wl.Icon != nil {
		out["icon_url"] = "/api/v1/branding/icon?v=" + wl.Icon.UpdatedAt.UTC().Format("20060102150405")
	}
	return out
}

// getWhiteLabel returns the organization's stored white label (applied or not) and whether the server is licensed.
func (s *Server) getWhiteLabel(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	b := s.loadBranding(r, orgOf(p))
	out := whiteLabelOut(b.WhiteLabel)
	out["licensed"] = s.whiteLabelLicensed(r.Context())
	out["active"] = s.activeWhiteLabel(r, orgOf(p), b) != nil
	writeJSON(w, 200, out)
}

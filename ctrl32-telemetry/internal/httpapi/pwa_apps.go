package httpapi

// Installable app profiles (spec 021): an organization defines several PWAs over the same web application — for
// example a phone app that opens the live cameras and shows only the Cameras menu. Each profile has an install page
// and its own manifest; the app restricts the menu to the chosen sections (permissions still apply: an app never
// shows what the user may not see, and it is a view, not a permission boundary). Profiles are content (ADR-0010):
// every change is a new version with a reason, the previous version gets valid_to.
//
//	GET    /api/v1/pwa-apps                 content.write  current profiles of the organization
//	GET    /api/v1/pwa-apps/options         content.write  menu sections and start pages to choose from
//	POST   /api/v1/pwa-apps                 content.write  {slug, name, short_name, start_url, sections, theme_color, enabled, reason}
//	GET    /api/v1/pwa-apps/{slug}          content.write  one profile
//	PUT    /api/v1/pwa-apps/{slug}          content.write  new version
//	DELETE /api/v1/pwa-apps/{slug}?reason=  content.write  retire
//	GET    /api/v1/pwa-apps/{slug}/launch   public         name, start page and sections of an enabled app (no data)
//	GET    /app/{slug}                      public         install and launch page
//	GET    /app/{slug}/manifest.webmanifest public         the app's manifest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// PWAApp is one installable app profile.
type PWAApp struct {
	ID         string    `json:"id,omitempty"`
	Slug       string    `json:"slug"`
	Name       string    `json:"name"`
	ShortName  string    `json:"short_name"`
	StartURL   string    `json:"start_url"`
	Sections   []string  `json:"sections"`
	ThemeColor string    `json:"theme_color"`
	Enabled    bool      `json:"enabled"`
	Version    int       `json:"version,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitzero"`
	orgID      string
}

// pwaSection is a top-level menu group an app may show (keys match the `sec` of navItems() in app.js).
type pwaSection struct {
	Key, Label, URL string
}

var pwaSections = []pwaSection{
	{"dashboards", "Dashboards", "/dashboards"}, {"home", "Home", "/home"}, {"energy", "Energy", "/energy"},
	{"cameras", "Cameras", "/video"}, {"displays", "Displays", "/displays"}, {"flows", "Flows", "/flows"},
	{"devices", "Devices", "/devices"}, {"incidents", "Incidents", "/incidents"}, {"assets", "Assets", "/assets"},
	{"firmware", "Firmware", "/fota"}, {"users", "Users", "/users"}, {"settings", "Settings", "/settings"},
	{"system", "System", "/organizations"},
}

const (
	pwaShortNameHint = 12 // longer names are cut on most home screens
	pwaMaxShortcuts  = 4  // Android shows at most four
)

var (
	pwaSlugRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	pwaStartRe = regexp.MustCompile(`^/[A-Za-z0-9._~/-]*(\?[A-Za-z0-9._~=&%+-]*)?$`)
	// paths that are not pages of the application (or would loop back into the app page)
	pwaStartRefused = []string{"/api/v1", "/app/", "/tiles/", "/metrics", "/healthz", "/sw.js", "/manifest.webmanifest", "/.well-known"}
	pwaSlugReserved = []string{"options"}
)

// validPWAStart reports whether a start page is a same-origin path of the application.
func validPWAStart(u string) bool {
	if len(u) > 300 || strings.HasPrefix(u, "//") || strings.Contains(u, "..") || !pwaStartRe.MatchString(u) {
		return false
	}
	path := u
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	if path == "/app" || strings.Contains(path, "//") {
		return false
	}
	for _, p := range pwaStartRefused {
		if strings.HasPrefix(path, p) {
			return false
		}
	}
	return true
}

// validatePWAApp normalizes the profile from a request and returns an error message ("" = valid) and warnings.
func validatePWAApp(in *PWAApp) (string, []string) {
	in.Slug, in.Name, in.ShortName = strings.TrimSpace(in.Slug), strings.TrimSpace(in.Name), strings.TrimSpace(in.ShortName)
	in.StartURL, in.ThemeColor = strings.TrimSpace(in.StartURL), strings.TrimSpace(in.ThemeColor)
	warnings := []string{}
	if !pwaSlugRe.MatchString(in.Slug) || slices.Contains(pwaSlugReserved, in.Slug) {
		return "slug: 1 to 40 lowercase letters, digits and dashes, starting with a letter or digit", nil
	}
	if in.Name == "" || !wlText(in.Name, 64) {
		return "name: 1 to 64 characters of plain text", nil
	}
	if in.ShortName == "" {
		in.ShortName = in.Name
		if len([]rune(in.ShortName)) > 24 {
			in.ShortName = string([]rune(in.ShortName)[:24])
		}
	}
	if !wlText(in.ShortName, 24) {
		return "short_name: at most 24 characters of plain text", nil
	}
	if len([]rune(in.ShortName)) > pwaShortNameHint {
		warnings = append(warnings, "short_name is longer than 12 characters and may be cut on the home screen")
	}
	if !validPWAStart(in.StartURL) {
		return "start_url: a path of this application such as /video (no address of another site, no //, no javascript:)", nil
	}
	seen := map[string]bool{}
	secs := []string{}
	for _, s := range in.Sections {
		s = strings.TrimSpace(s)
		if !slices.ContainsFunc(pwaSections, func(p pwaSection) bool { return p.Key == s }) {
			return "sections: unknown section " + strings.ToValidUTF8(firstN(s, 40), "") + " (known: " + strings.Join(pwaSectionKeys(), ", ") + ")", nil
		}
		if !seen[s] {
			seen[s] = true
			secs = append(secs, s)
		}
	}
	if len(secs) == 0 {
		return "sections: choose at least one menu section", nil
	}
	// keep the menu order, whatever order the request used
	slices.SortFunc(secs, func(a, b string) int { return pwaSectionIndex(a) - pwaSectionIndex(b) })
	in.Sections = secs
	if in.ThemeColor != "" && !hexColor.MatchString(in.ThemeColor) {
		return "theme_color: #rrggbb or empty (the organization's colour)", nil
	}
	return "", warnings
}

func firstN(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func pwaSectionKeys() []string {
	out := make([]string, len(pwaSections))
	for i, s := range pwaSections {
		out[i] = s.Key
	}
	return out
}

func pwaSectionIndex(key string) int {
	return slices.IndexFunc(pwaSections, func(p pwaSection) bool { return p.Key == key })
}

func (a *PWAApp) checksum() string {
	b, _ := json.Marshal([]any{a.Slug, a.Name, a.ShortName, a.StartURL, a.Sections, a.ThemeColor, a.Enabled})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (a *PWAApp) out() map[string]any {
	return map[string]any{"id": a.ID, "slug": a.Slug, "name": a.Name, "short_name": a.ShortName, "start_url": a.StartURL,
		"sections": a.Sections, "theme_color": a.ThemeColor, "enabled": a.Enabled, "version": a.Version, "created_at": a.CreatedAt,
		"install_url": "/app/" + a.Slug, "manifest_url": "/app/" + a.Slug + "/manifest.webmanifest"}
}

const pwaCols = `id::text, org_id::text, slug, name, short_name, start_url, sections, theme_color, enabled, version, created_at`

func scanPWAApp(row pgx.Row) (*PWAApp, error) {
	a := &PWAApp{}
	err := row.Scan(&a.ID, &a.orgID, &a.Slug, &a.Name, &a.ShortName, &a.StartURL, &a.Sections, &a.ThemeColor, &a.Enabled, &a.Version, &a.CreatedAt)
	if a.Sections == nil {
		a.Sections = []string{}
	}
	return a, err
}

// publicPWAApp finds an enabled app for the public pages: in the signed-in user's organization, else in the oldest
// organization that has one with this slug (browsers fetch manifests without cookies).
func (s *Server) publicPWAApp(r *http.Request, slug string) (*PWAApp, bool) {
	if !pwaSlugRe.MatchString(slug) {
		return nil, false
	}
	q := `SELECT ` + pwaCols + ` FROM core.pwa_app a WHERE slug = $1 AND valid_to IS NULL AND enabled`
	args := []any{slug}
	if p := PrincipalFrom(r.Context()); p != nil && p.OrgID != "" {
		q += ` AND org_id = $2::uuid`
		args = append(args, p.OrgID)
	} else {
		q += ` ORDER BY (SELECT created_at FROM core.organization o WHERE o.id = a.org_id) LIMIT 1`
	}
	a, err := scanPWAApp(s.pool.QueryRow(r.Context(), q, args...))
	if err != nil {
		return nil, false
	}
	return a, true
}

// ---- management API ---------------------------------------------------------------------------------------------------

func (s *Server) listPWAApps(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT `+pwaCols+` FROM core.pwa_app WHERE org_id = $1::uuid AND valid_to IS NULL ORDER BY name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		if a, err := scanPWAApp(rows); err == nil {
			out = append(out, a.out())
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) getPWAApp(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	a, err := scanPWAApp(s.pool.QueryRow(r.Context(), `SELECT `+pwaCols+` FROM core.pwa_app WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL`, org, r.PathValue("slug")))
	if err != nil {
		writeErr(w, 404, "app not found")
		return
	}
	writeJSON(w, 200, a.out())
}

// pwaAppOptions lists what the settings form offers: menu sections and start pages from existing objects.
func (s *Server) pwaAppOptions(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	secs := []map[string]string{}
	for _, sec := range pwaSections {
		secs = append(secs, map[string]string{"key": sec.Key, "label": sec.Label, "url": sec.URL})
	}
	pages := []map[string]string{
		{"group": "Pages", "label": "Dashboard (home screen)", "url": "/"},
		{"group": "Pages", "label": "Dashboards", "url": "/dashboards"},
		{"group": "Pages", "label": "Cameras — live view", "url": "/video"},
		{"group": "Pages", "label": "Video walls", "url": "/video/walls"},
		{"group": "Pages", "label": "Home", "url": "/home"},
		{"group": "Pages", "label": "Energy", "url": "/energy"},
		{"group": "Pages", "label": "Incidents", "url": "/incidents"},
		{"group": "Pages", "label": "Displays", "url": "/displays"},
		{"group": "Pages", "label": "Devices", "url": "/devices"},
		{"group": "Pages", "label": "Flows", "url": "/flows"},
		{"group": "Pages", "label": "Assets", "url": "/assets"},
	}
	add := func(ctx context.Context, group, q, prefix string, filter func(extra string) bool) {
		rows, err := s.pool.Query(ctx, q, org)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var key, name, extra string
			if rows.Scan(&key, &name, &extra) == nil && (filter == nil || filter(extra)) {
				pages = append(pages, map[string]string{"group": group, "label": name, "url": prefix + key})
			}
		}
	}
	add(r.Context(), "Dashboards", `SELECT slug, name, '' FROM core.dashboard WHERE org_id = $1::uuid AND valid_to IS NULL ORDER BY name`, "/dashboards/", nil)
	if p.Has("video.view") {
		add(r.Context(), "Video walls", `SELECT id::text, name, '' FROM core.video_wall WHERE org_id = $1::uuid AND removed_at IS NULL ORDER BY name`, "/video/walls/", nil)
		add(r.Context(), "Cameras", `SELECT id::text, name, camera_group FROM core.camera WHERE org_id = $1::uuid AND removed_at IS NULL ORDER BY name`, "/video/cameras/", p.SeesCameraGroup)
	}
	writeJSON(w, 200, map[string]any{"sections": secs, "start_pages": pages, "short_name_hint": pwaShortNameHint})
}

func (s *Server) savePWAApp(w http.ResponseWriter, r *http.Request, create bool) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	var in struct {
		PWAApp
		Enabled *bool  `json:"enabled"`
		Reason  string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	a := in.PWAApp
	if !create {
		a.Slug = r.PathValue("slug")
	}
	a.Enabled = in.Enabled == nil || *in.Enabled
	msg, warnings := validatePWAApp(&a)
	if msg != "" {
		writeErr(w, 400, msg)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, 500, "begin failed")
		return
	}
	defer tx.Rollback(r.Context())
	prev, err := scanPWAApp(tx.QueryRow(r.Context(), `SELECT `+pwaCols+` FROM core.pwa_app WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL FOR UPDATE`, org, a.Slug))
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 500, "query failed")
		return
	}
	if create && exists {
		writeErr(w, 409, "an app with this address exists")
		return
	}
	if !create && !exists {
		writeErr(w, 404, "app not found")
		return
	}
	a.Version = 1
	if exists {
		if in.Enabled == nil {
			a.Enabled = prev.Enabled
		}
		a.Version = prev.Version + 1
		if _, err := tx.Exec(r.Context(), `UPDATE core.pwa_app SET valid_to = now() WHERE id = $1::uuid`, prev.ID); err != nil {
			writeErr(w, 500, "supersede failed")
			return
		}
	}
	if err := tx.QueryRow(r.Context(), `INSERT INTO core.pwa_app (org_id, slug, name, short_name, start_url, sections, theme_color, enabled, version, checksum, created_by, reason)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11,'')::uuid, $12) RETURNING id::text, created_at`,
		org, a.Slug, a.Name, a.ShortName, a.StartURL, a.Sections, a.ThemeColor, a.Enabled, a.Version, a.checksum(), p.UserID, in.Reason).Scan(&a.ID, &a.CreatedAt); err != nil {
		writeErr(w, 500, "insert failed")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, 500, "commit failed")
		return
	}
	action, code := "pwa_app.update", 200
	var old any
	if exists {
		old = pwaAudit(prev)
	} else {
		action, code = "pwa_app.create", 201
	}
	s.audit(r, action, "pwa_app", a.ID, old, pwaAudit(&a), in.Reason)
	out := a.out()
	out["warnings"] = warnings
	writeJSON(w, code, out)
}

func pwaAudit(a *PWAApp) map[string]any {
	return map[string]any{"slug": a.Slug, "name": a.Name, "short_name": a.ShortName, "start_url": a.StartURL, "sections": a.Sections,
		"theme_color": a.ThemeColor, "enabled": a.Enabled, "version": a.Version}
}

func (s *Server) createPWAApp(w http.ResponseWriter, r *http.Request) { s.savePWAApp(w, r, true) }
func (s *Server) updatePWAApp(w http.ResponseWriter, r *http.Request) { s.savePWAApp(w, r, false) }

func (s *Server) deletePWAApp(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	prev, err := scanPWAApp(s.pool.QueryRow(r.Context(), `UPDATE core.pwa_app SET valid_to = now() WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL RETURNING `+pwaCols, org, r.PathValue("slug")))
	if err != nil {
		writeErr(w, 404, "app not found")
		return
	}
	s.audit(r, "pwa_app.retire", "pwa_app", prev.ID, pwaAudit(prev), nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// pwaAppLaunch gives app.js what the app mode needs: name, start page and sections. No data of the organization.
func (s *Server) pwaAppLaunch(w http.ResponseWriter, r *http.Request) {
	a, ok := s.publicPWAApp(r, r.PathValue("slug"))
	if !ok {
		writeErr(w, 404, "app not found")
		return
	}
	writeJSON(w, 200, map[string]any{"slug": a.Slug, "name": a.Name, "short_name": a.ShortName, "start_url": a.StartURL, "sections": a.Sections, "theme_color": a.ThemeColor})
}

// ---- manifest and install page --------------------------------------------------------------------------------------

// pwaIdentity is what an app looks like on the device: names, colour and icons, white-label aware. Without an active
// white label the product stays visible in the full name and the product icon is used (ADR-0011, ADR-0016); the
// home-screen label (short name) is always the app's own.
type pwaIdentity struct {
	Name, ShortName, Description, Theme, Product string
	Icons                                        []map[string]any
	TouchIcon                                    string
}

func pwaIdentityOf(a *PWAApp, b branding, wl *WhiteLabel) pwaIdentity {
	id := pwaIdentity{ShortName: a.ShortName, Theme: a.ThemeColor, Product: productName, TouchIcon: "/icon-192.png",
		Icons: []map[string]any{
			{"src": "/icon.svg", "sizes": "any", "type": "image/svg+xml", "purpose": "any"},
			{"src": "/icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any"},
			{"src": "/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any maskable"},
		}}
	if id.Theme == "" {
		id.Theme = b.Theme.Primary
	}
	if id.Theme == "" {
		id.Theme = "#e30613"
	}
	if wl != nil {
		id.Product = wl.ProductName
		id.Name = a.Name
		if wl.Icon != nil {
			id.Icons = []map[string]any{{"src": "/api/v1/branding/icon", "sizes": "any", "type": wl.Icon.Mime, "purpose": "any"}}
			if wl.Icon.Mime == "image/png" {
				id.TouchIcon = "/api/v1/branding/icon"
			}
		}
	} else {
		id.Name = a.Name + " · " + productName
		if strings.Contains(strings.ToLower(a.Name), "ctrl32") {
			id.Name = a.Name
		}
	}
	id.Description = a.Name + " — " + id.Product
	return id
}

// pwaManifest builds the manifest of an app: its own id and start page, scope "/" so sign-in and sub-pages stay in
// the app window, shortcuts to the chosen sections.
func pwaManifest(a *PWAApp, id pwaIdentity) map[string]any {
	shortcuts := []map[string]any{}
	for _, key := range a.Sections {
		if len(shortcuts) == pwaMaxShortcuts {
			break
		}
		sec := pwaSections[pwaSectionIndex(key)]
		shortcuts = append(shortcuts, map[string]any{"name": sec.Label, "url": sec.URL, "icons": []map[string]any{{"src": "/icon-192.png", "sizes": "192x192"}}})
	}
	return map[string]any{
		"id": "/app/" + a.Slug, "name": id.Name, "short_name": id.ShortName, "description": id.Description,
		"start_url": "/app/" + a.Slug, "scope": "/", "display": "standalone", "orientation": "any",
		"background_color": "#ffffff", "theme_color": id.Theme, "icons": id.Icons, "shortcuts": shortcuts,
	}
}

func (s *Server) pwaIdentity(r *http.Request, a *PWAApp) pwaIdentity {
	b := s.loadBranding(r, a.orgID)
	return pwaIdentityOf(a, b, s.activeWhiteLabel(r, a.orgID, b))
}

func (s *Server) pwaAppManifest(w http.ResponseWriter, r *http.Request) {
	a, ok := s.publicPWAApp(r, r.PathValue("slug"))
	if !ok {
		writeErr(w, 404, "app not found")
		return
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(w).Encode(pwaManifest(a, s.pwaIdentity(r, a)))
}

var pwaPage = template.Must(template.New("app").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>{{.Name}}</title>
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="theme-color" content="{{.Theme}}">
<meta name="apple-mobile-web-app-capable" content="yes">
<meta name="mobile-web-app-capable" content="yes">
<meta name="apple-mobile-web-app-title" content="{{.ShortName}}">
<link rel="manifest" href="/app/{{.Slug}}/manifest.webmanifest">
<link rel="icon" href="/favicon.svg" type="image/svg+xml">
<link rel="apple-touch-icon" href="{{.TouchIcon}}">
<link rel="stylesheet" href="/app.css">
<style>
  .app-card { max-width: 560px; margin: 24px auto; }
  .app-head { display: flex; gap: 14px; align-items: center; }
  .app-head img { width: 64px; height: 64px; border-radius: 14px; flex: none; }
  .app-head h1 { margin: 0; }
  .app-qr { display: flex; gap: 14px; align-items: center; flex-wrap: wrap; margin-top: 12px; }
  .app-qr svg { width: 148px; height: 148px; border: 1px solid var(--line); border-radius: 4px; flex: none; }
  .app-qr code { word-break: break-all; }
  ol.app-steps { padding-left: 20px; line-height: 1.7; }
</style>
</head>
<body>
<header class="topbar"></header>
<main class="container">
<div class="card app-card" id="appCard" data-slug="{{.Slug}}" data-start="{{.Start}}">
  <div class="app-head"><img src="{{.TouchIcon}}" alt=""><div><h1 class="notranslate">{{.Name}}</h1><div class="muted notranslate">{{.Product}}</div></div></div>
  <p id="launching" hidden>Opening the app…</p>
  <div id="installBox">
    <h2>Install this app</h2>
    <div id="ios" hidden>
      <p>On iPhone or iPad:</p>
      <ol class="app-steps"><li>Open this page in Safari.</li><li>Tap Share (the square with the arrow).</li><li>Choose Add to Home Screen, then Add.</li></ol>
    </div>
    <div id="other">
      <p>On Android and on a computer: install it with the button below, or from the browser menu (Install app, Add to Home screen).</p>
      <p><button class="btn primary" id="installBtn" type="button" hidden>Install</button> <span class="muted" id="installed" hidden>Installed. Open it from the home screen or the app list.</span></p>
    </div>
    <div class="app-qr"><div id="qr"></div><div><p>Scan with the phone to open this page there:</p><code id="appUrl" class="notranslate"></code></div></div>
  </div>
  <p style="margin-top:16px"><a class="btn" id="openBtn" href="/app/{{.Slug}}?launch=1">Open</a> <a class="btn ghost" href="/?full=1">Full application</a></p>
</div>
</main>
<script>
// The install prompt may fire before the page script runs: keep it.
window.deferredPrompt = null;
window.addEventListener('beforeinstallprompt', (e) => { e.preventDefault(); window.deferredPrompt = e; const b = document.getElementById('installBtn'); if (b) b.hidden = false; });
</script>
<script src="/app.js"></script>
<script src="/app-install.js"></script>
</body>
</html>
`))

func (s *Server) pwaAppPage(w http.ResponseWriter, r *http.Request) {
	a, ok := s.publicPWAApp(r, r.PathValue("slug"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>App not found</title>
<meta name="viewport" content="width=device-width, initial-scale=1"><link rel="stylesheet" href="/app.css"><link rel="icon" href="/favicon.svg" type="image/svg+xml"></head>
<body><header class="topbar"></header><main class="container"><div class="card" style="max-width:560px;margin:24px auto"><h1>App not found</h1>
<p class="muted">This app does not exist or is switched off. Ask your administrator for the right address.</p><p><a class="btn" href="/">Open the full application</a></p></div></main>
<script src="/app.js"></script><script>ctrl32.ready();</script></body></html>`))
		return
	}
	id := s.pwaIdentity(r, a)
	_ = pwaPage.Execute(w, map[string]string{"Slug": a.Slug, "Name": a.Name, "ShortName": a.ShortName, "Start": a.StartURL,
		"Theme": id.Theme, "TouchIcon": id.TouchIcon, "Product": id.Product})
}

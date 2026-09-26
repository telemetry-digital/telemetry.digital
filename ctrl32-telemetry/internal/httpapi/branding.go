package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Branding (URS J-16, ADR-0010): an organization may set its own logo, theme colours and font; they apply to the
// web UI (all pages, light and dark mode) and to PDF reports. The product footer ("ctrl32 · telemetry", GitHub,
// license) is fixed in the binary and not part of the editable branding.
//
//	GET /api/v1/branding          public: logo URL, theme, product footer (used by the landing and login pages)
//	GET /api/v1/branding/logo     public: the logo bytes (custom or built-in)
//	PUT /api/v1/branding          content.write: {"logo": {"mime","data"} | null, "theme": {...}}

const (
	brandingLogoMaxBytes = 256 * 1024
	productName          = "ctrl32 telemetry"
	productGitHub        = "https://github.com/telemetry-digital/ctrl32-telemetry"
	productLicensePath   = "/license"
)

// Theme is the editable part of the look: CSS custom properties are derived from it on the client.
type Theme struct {
	Primary   string `json:"primary"`    // accent colour, #rrggbb
	Font      string `json:"font"`       // font family name (system stack when empty)
	Mode      string `json:"mode"`       // default mode: auto|light|dark (users may override per browser)
	Radius    string `json:"radius"`     // corner radius: none|small|medium|large
	FontScale string `json:"font_scale"` // small|normal|large
}

type logoData struct {
	Mime      string    `json:"mime"`
	Data      string    `json:"data"` // base64
	UpdatedAt time.Time `json:"updated_at"`
}

type branding struct {
	Logo       *logoData   `json:"logo,omitempty"`
	Theme      Theme       `json:"theme"`
	Map        MapSettings `json:"map"`
	WhiteLabel *WhiteLabel `json:"white_label,omitempty"` // applied only with a license (spec 007)
}

// MapSettings selects the raster tile source for map widgets. Empty tile_url = built-in OpenStreetMap tiles (their policy
// allows light use only); for regular use the organization sets its own tile server or a provider with a key.
type MapSettings struct {
	TileURL     string  `json:"tile_url"`    // e.g. https://tiles.example.com/{z}/{x}/{y}.png
	Attribution string  `json:"attribution"` // shown on the map, e.g. © OpenStreetMap contributors
	MaxZoom     int     `json:"max_zoom"`
	CenterLat   float64 `json:"center_lat"`
	CenterLon   float64 `json:"center_lon"`
	Zoom        int     `json:"zoom"`
}

// mapOrDefault returns the organization's tile settings; with an empty tile URL the server's PMTiles archive
// ([map] pmtiles) is used when it is configured, otherwise the built-in OpenStreetMap source.
func (s *Server) mapOrDefault(m MapSettings) map[string]any {
	source := "custom"
	if m.TileURL == "" {
		if s.tiles.ok() {
			source = "pmtiles"
			h := s.tiles.a.Header()
			m.TileURL = s.tiles.tileURL()
			m.Attribution = s.tiles.attribution
			if m.MaxZoom == 0 {
				m.MaxZoom = int(h.MaxZoom)
			}
			if m.CenterLat == 0 && m.CenterLon == 0 && (h.CenterLat != 0 || h.CenterLon != 0) {
				m.CenterLat, m.CenterLon = h.CenterLat, h.CenterLon
				if m.Zoom == 0 {
					m.Zoom = int(h.CenterZoom)
				}
			}
		} else {
			source = "builtin"
			m.TileURL = "https://tile.openstreetmap.org/{z}/{x}/{y}.png"
			m.Attribution = "© OpenStreetMap contributors"
		}
	}
	if m.MaxZoom == 0 {
		m.MaxZoom = 19
	}
	if m.Zoom == 0 {
		m.Zoom = 7
	}
	if m.CenterLat == 0 && m.CenterLon == 0 {
		m.CenterLat, m.CenterLon = 48.7, 19.5
	}
	return map[string]any{"tile_url": m.TileURL, "attribution": m.Attribution, "max_zoom": m.MaxZoom, "center_lat": m.CenterLat, "center_lon": m.CenterLon, "zoom": m.Zoom,
		"builtin": source == "builtin", "local": source == "pmtiles", "source": source}
}

var (
	hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	fontName = regexp.MustCompile(`^[A-Za-z0-9 _\-]{0,64}$`)
	logoMime = map[string]string{"image/png": "logo.png", "image/jpeg": "logo.jpg", "image/svg+xml": "logo.svg", "image/webp": "logo.webp", "image/gif": "logo.gif"}
)

func defaultTheme() Theme {
	return Theme{Primary: "#e30613", Font: "", Mode: "auto", Radius: "small", FontScale: "normal"}
}

// brandingOrg picks the organization whose branding applies: the signed-in principal's, otherwise the instance's
// first organization (single-tenant installs show it on the landing and login pages).
func (s *Server) brandingOrg(r *http.Request) (orgID, name string) {
	if p := PrincipalFrom(r.Context()); p != nil && p.OrgID != "" {
		_ = s.pool.QueryRow(r.Context(), `SELECT id::text, name FROM core.organization WHERE id = $1::uuid`, p.OrgID).Scan(&orgID, &name)
		return orgID, name
	}
	_ = s.pool.QueryRow(r.Context(), `SELECT id::text, name FROM core.organization ORDER BY created_at LIMIT 1`).Scan(&orgID, &name)
	return orgID, name
}

func (s *Server) loadBranding(r *http.Request, orgID string) branding {
	b := branding{Theme: defaultTheme()}
	if orgID == "" {
		return b
	}
	var raw []byte
	if err := s.pool.QueryRow(r.Context(), `SELECT branding FROM core.organization WHERE id = $1::uuid`, orgID).Scan(&raw); err != nil {
		return b
	}
	var stored branding
	if json.Unmarshal(raw, &stored) == nil {
		if stored.Logo != nil && stored.Logo.Data != "" {
			b.Logo = stored.Logo
		}
		b.WhiteLabel = stored.WhiteLabel
		if hexColor.MatchString(stored.Theme.Primary) {
			b.Theme.Primary = stored.Theme.Primary
		}
		if fontName.MatchString(stored.Theme.Font) {
			b.Theme.Font = stored.Theme.Font
		}
		switch stored.Theme.Mode {
		case "light", "dark", "auto":
			b.Theme.Mode = stored.Theme.Mode
		}
		switch stored.Theme.Radius {
		case "none", "small", "medium", "large":
			b.Theme.Radius = stored.Theme.Radius
		}
		switch stored.Theme.FontScale {
		case "small", "normal", "large":
			b.Theme.FontScale = stored.Theme.FontScale
		}
		b.Map = stored.Map // validated on save (putBranding)
	}
	return b
}

// orgLogo returns the organization's custom logo bytes and the asset file name for PDF templates ("" when none).
func (s *Server) orgLogo(r *http.Request, orgID string) ([]byte, string) {
	b := s.loadBranding(r, orgID)
	if b.Logo == nil {
		return nil, ""
	}
	raw, err := base64.StdEncoding.DecodeString(b.Logo.Data)
	if err != nil || len(raw) == 0 {
		return nil, ""
	}
	name := logoMime[b.Logo.Mime]
	if name == "" {
		return nil, ""
	}
	return raw, name
}

func (s *Server) getBranding(w http.ResponseWriter, r *http.Request) {
	orgID, name := s.brandingOrg(r)
	b := s.loadBranding(r, orgID)
	logoURL := "/logo.svg"
	custom := false
	if b.Logo != nil {
		logoURL = "/api/v1/branding/logo?v=" + b.Logo.UpdatedAt.UTC().Format("20060102150405")
		custom = true
	}
	product, icon := productName, "/favicon.svg"
	footer := map[string]any{"text": "ctrl32 · telemetry", "github": productGitHub, "license": productLicensePath, "fixed": true, "links": []FooterLink{},
		"show_version": true, "white_label": false}
	licensed := s.whiteLabelLicensed(r.Context())
	if wl := s.activeWhiteLabel(r, orgID, b); wl != nil {
		product = wl.ProductName
		links := wl.FooterLinks
		if links == nil {
			links = []FooterLink{}
		}
		footer = map[string]any{"text": wl.FooterText, "github": "", "license": "", "fixed": false, "links": links, "show_version": wl.ShowVersion, "white_label": true, "intro": wl.Intro}
		if wl.Icon != nil {
			icon = "/api/v1/branding/icon?v=" + wl.Icon.UpdatedAt.UTC().Format("20060102150405")
		}
	}
	writeJSON(w, 200, map[string]any{
		"product":     product,
		"version":     s.version,
		"org_name":    name,
		"logo_url":    logoURL,
		"logo_custom": custom,
		"icon_url":    icon,
		"public_read": s.cfg.HTTP.PublicRead,
		"theme":       b.Theme,
		"map":         s.mapOrDefault(b.Map),
		"footer":      footer,
		"white_label": map[string]any{"licensed": licensed},
	})
}

func (s *Server) getBrandingLogo(w http.ResponseWriter, r *http.Request) {
	orgID, _ := s.brandingOrg(r)
	raw, name := s.orgLogo(r, orgID)
	if raw == nil {
		http.Redirect(w, r, "/logo.svg", http.StatusFound)
		return
	}
	mime := "application/octet-stream"
	for m, n := range logoMime {
		if n == name {
			mime = m
		}
	}
	h := w.Header()
	h.Set("Content-Type", mime)
	h.Set("Content-Disposition", "inline; filename=\""+name+"\"")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox") // SVG logos must not run scripts
	h.Set("Cache-Control", "private, max-age=3600")
	w.WriteHeader(200)
	_, _ = w.Write(raw)
}

func (s *Server) putBranding(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	if orgOf(p) == "" {
		writeErr(w, 403, "branding is stored per organization")
		return
	}
	var in struct {
		Logo *struct {
			Mime string `json:"mime"`
			Data string `json:"data"`
		} `json:"logo"`
		ClearLogo bool         `json:"clear_logo"`
		Theme     *Theme       `json:"theme"`
		Map       *MapSettings `json:"map"`
		// WhiteLabel replaces the settings (icon: new icon; clear_icon removes it; omitted icon keeps it)
		WhiteLabel *struct {
			WhiteLabel
			Icon *struct {
				Mime string `json:"mime"`
				Data string `json:"data"`
			} `json:"icon"`
			ClearIcon bool `json:"clear_icon"`
		} `json:"white_label"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 1<<20); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	cur := s.loadBranding(r, p.OrgID)
	next := cur
	if in.ClearLogo {
		next.Logo = nil
	}
	if in.Logo != nil {
		if _, ok := logoMime[in.Logo.Mime]; !ok {
			writeErr(w, 400, "logo mime must be image/png, image/jpeg, image/svg+xml, image/webp or image/gif")
			return
		}
		raw, err := base64.StdEncoding.DecodeString(in.Logo.Data)
		if err != nil || len(raw) == 0 {
			writeErr(w, 400, "logo data must be base64")
			return
		}
		if len(raw) > brandingLogoMaxBytes {
			writeErr(w, 413, "logo must be at most 256 KiB")
			return
		}
		if in.Logo.Mime == "image/svg+xml" && (strings.Contains(strings.ToLower(string(raw)), "<script") || strings.Contains(strings.ToLower(string(raw)), "onload=")) {
			writeErr(w, 400, "SVG logo must not contain scripts")
			return
		}
		next.Logo = &logoData{Mime: in.Logo.Mime, Data: base64.StdEncoding.EncodeToString(raw), UpdatedAt: time.Now().UTC()}
	}
	if in.WhiteLabel != nil {
		wl := in.WhiteLabel.WhiteLabel
		if msg := validateWhiteLabel(&wl); msg != "" {
			writeErr(w, 400, msg)
			return
		}
		if wl.Enabled && !s.whiteLabelLicensed(r.Context()) {
			writeErr(w, 402, "white labeling needs a license (System → License)")
			return
		}
		if cur.WhiteLabel != nil && !in.WhiteLabel.ClearIcon {
			wl.Icon = cur.WhiteLabel.Icon
		}
		if ic := in.WhiteLabel.Icon; ic != nil {
			if ic.Mime != "image/png" && ic.Mime != "image/svg+xml" {
				writeErr(w, 400, "white_label.icon must be image/png or image/svg+xml")
				return
			}
			raw, err := base64.StdEncoding.DecodeString(ic.Data)
			if err != nil || len(raw) == 0 {
				writeErr(w, 400, "white_label.icon data must be base64")
				return
			}
			if len(raw) > whiteLabelIconMaxBytes {
				writeErr(w, 413, "white_label.icon must be at most 64 KiB")
				return
			}
			low := strings.ToLower(string(raw))
			if ic.Mime == "image/svg+xml" && (strings.Contains(low, "<script") || strings.Contains(low, "onload=") || strings.Contains(low, "javascript:")) {
				writeErr(w, 400, "SVG icon must not contain scripts")
				return
			}
			wl.Icon = &logoData{Mime: ic.Mime, Data: base64.StdEncoding.EncodeToString(raw), UpdatedAt: time.Now().UTC()}
		}
		next.WhiteLabel = &wl
	}
	if in.Map != nil {
		m := *in.Map
		m.TileURL = strings.TrimSpace(m.TileURL)
		if m.TileURL != "" && (len(m.TileURL) > 512 || !(strings.HasPrefix(m.TileURL, "https://") || strings.HasPrefix(m.TileURL, "http://")) || !strings.Contains(m.TileURL, "{z}") || !strings.Contains(m.TileURL, "{x}") || !strings.Contains(m.TileURL, "{y}")) {
			writeErr(w, 400, "map.tile_url must be an http(s) XYZ template with {z}, {x} and {y}")
			return
		}
		if len(m.Attribution) > 300 || strings.ContainsAny(m.Attribution, "<>") {
			writeErr(w, 400, "map.attribution: plain text up to 300 characters")
			return
		}
		if m.MaxZoom < 0 || m.MaxZoom > 22 || m.Zoom < 0 || m.Zoom > 22 || m.CenterLat < -90 || m.CenterLat > 90 || m.CenterLon < -180 || m.CenterLon > 180 {
			writeErr(w, 400, "map: zoom 0..22, centre within the world")
			return
		}
		next.Map = m
	}
	if in.Theme != nil {
		t := in.Theme
		if t.Primary != "" && !hexColor.MatchString(t.Primary) {
			writeErr(w, 400, "theme.primary must be #rrggbb")
			return
		}
		if !fontName.MatchString(t.Font) {
			writeErr(w, 400, "theme.font must be a plain font family name")
			return
		}
		if t.Primary != "" {
			next.Theme.Primary = t.Primary
		}
		next.Theme.Font = t.Font
		if t.Mode != "" {
			next.Theme.Mode = t.Mode
		}
		if t.Radius != "" {
			next.Theme.Radius = t.Radius
		}
		if t.FontScale != "" {
			next.Theme.FontScale = t.FontScale
		}
		// re-validate through loadBranding rules
		switch next.Theme.Mode {
		case "light", "dark", "auto":
		default:
			writeErr(w, 400, "theme.mode must be auto, light or dark")
			return
		}
		switch next.Theme.Radius {
		case "none", "small", "medium", "large":
		default:
			writeErr(w, 400, "theme.radius must be none, small, medium or large")
			return
		}
		switch next.Theme.FontScale {
		case "small", "normal", "large":
		default:
			writeErr(w, 400, "theme.font_scale must be small, normal or large")
			return
		}
	}
	raw, _ := json.Marshal(next)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.organization SET branding = $2::jsonb WHERE id = $1::uuid`, p.OrgID, string(raw)); err != nil {
		s.log.Error("branding update", "err", err)
		writeErr(w, 500, "update failed")
		return
	}
	summary := func(b branding) map[string]any {
		m := map[string]any{"theme": b.Theme, "logo": nil}
		if b.Logo != nil {
			m["logo"] = map[string]any{"mime": b.Logo.Mime, "bytes": len(b.Logo.Data) / 4 * 3, "updated_at": b.Logo.UpdatedAt}
		}
		if wl := b.WhiteLabel; wl != nil {
			m["white_label"] = map[string]any{"enabled": wl.Enabled, "product_name": wl.ProductName, "short_name": wl.ShortName,
				"footer_text": wl.FooterText, "intro": wl.Intro, "footer_links": wl.FooterLinks, "show_version": wl.ShowVersion, "icon": wl.Icon != nil}
		}
		return m
	}
	s.audit(r, "branding.update", "organization", p.OrgID, summary(cur), summary(next), in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true, "theme": next.Theme, "logo_custom": next.Logo != nil, "white_label": whiteLabelOut(next.WhiteLabel)})
}

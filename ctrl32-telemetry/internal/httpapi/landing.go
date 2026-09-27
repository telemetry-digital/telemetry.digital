package httpapi

// The public front page (spec 012): what visitors see before signing in, edited as blocks in a graphical editor
// (static/landing-edit.html) and rendered by static/landing.js. Safety first: the page is data, never HTML — the server
// accepts only known block types and fields, plain text, hex colours, catalogue icons, relative/https/mailto/tel
// links and its own images; the browser inserts all text as text. A sign-in button is always part of the page header
// and cannot be removed, so an edited page can never lock users out.
//
//	GET  /api/v1/landing                          public: the published page ({spec: null} = built-in page)
//	GET  /api/v1/landing/assets/{id}              public: an image of the page
//	GET  /api/v1/landing/draft                    system.admin: newest version (or the built-in template), published version
//	GET  /api/v1/landing/versions                 system.admin
//	GET  /api/v1/landing/versions/{v}             system.admin
//	POST /api/v1/landing/versions                 system.admin {spec, reason, publish}
//	POST /api/v1/landing/versions/{v}/publish     system.admin {reason}
//	POST /api/v1/landing/unpublish                system.admin {reason}: back to the built-in page
//	GET  /api/v1/landing/assets                   system.admin
//	POST /api/v1/landing/assets                   system.admin multipart {file, reason}

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // image.DecodeConfig for uploads
	_ "image/png"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	landingMaxBlocks = 40
	landingMaxItems  = 12
	landingMaxAsset  = 2 << 20
)

// LandingSpec is the page.
type LandingSpec struct {
	Version     int            `json:"version"`
	Title       string         `json:"title"`       // browser title
	Description string         `json:"description"` // search engines
	Theme       LandingTheme   `json:"theme"`
	Blocks      []LandingBlock `json:"blocks"`
}

// LandingTheme colours the page (empty = the organization theme).
type LandingTheme struct {
	Background string `json:"background"`
	Text       string `json:"text"`
	Accent     string `json:"accent"`
	Width      string `json:"width"` // normal | wide | full
}

// LandingBlock is one section of the page; which fields apply depends on the type.
type LandingBlock struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"` // hero, features, text, image, split, stats, cta, contact, spacer
	Title       string          `json:"title,omitempty"`
	Subtitle    string          `json:"subtitle,omitempty"`
	Text        string          `json:"text,omitempty"`
	Image       string          `json:"image,omitempty"` // "logo" or an asset id
	ImageSide   string          `json:"image_side,omitempty"`
	Align       string          `json:"align,omitempty"`
	Columns     int             `json:"columns,omitempty"`
	Background  string          `json:"background,omitempty"`
	Background2 string          `json:"background2,omitempty"` // gradient end
	TextColor   string          `json:"text_color,omitempty"`
	Size        string          `json:"size,omitempty"` // small | medium | large (spacer height, image width, hero height)
	Items       []LandingItem   `json:"items,omitempty"`
	Buttons     []LandingButton `json:"buttons,omitempty"`
	Email       string          `json:"email,omitempty"`
	Phone       string          `json:"phone,omitempty"`
	Address     string          `json:"address,omitempty"`
}

// LandingItem is a feature, a number or a list entry.
type LandingItem struct {
	Icon  string `json:"icon,omitempty"`
	Title string `json:"title,omitempty"`
	Text  string `json:"text,omitempty"`
	Value string `json:"value,omitempty"`
}

// LandingButton is a link styled as a button.
type LandingButton struct {
	Label string `json:"label"`
	Href  string `json:"href"`
	Style string `json:"style,omitempty"` // primary | secondary
}

var (
	landingTypes = setOf("hero", "features", "text", "image", "split", "stats", "cta", "contact", "spacer")
	landingIDRe  = regexp.MustCompile(`^[a-z0-9_-]{1,24}$`)
	// links: a path on this server (not //host), https, mailto or tel — never javascript:, data: or http:
	landingHrefRe  = regexp.MustCompile(`^(/([A-Za-z0-9._~!$&'()*+,;=:@%/?#-]*)?|https://[A-Za-z0-9.-]+(:\d+)?([/?#][^\s"'<>\\]*)?|mailto:[^\s"'<>\\]+|tel:\+?[0-9 ()/-]+)$`)
	landingEmailRe = regexp.MustCompile(`^[^\s"'<>\\@]+@[^\s"'<>\\@]+$`)
	landingPhoneRe = regexp.MustCompile(`^\+?[0-9 ()/-]{3,30}$`)
)

func textOK(s string, max int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= max && !strings.ContainsRune(s, 0)
}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

// validateLanding checks every field; assetOK tells whether an image id exists.
func validateLanding(sp *LandingSpec, assetOK func(string) bool) error {
	if sp.Version != 1 {
		return fmt.Errorf("version must be 1")
	}
	if !textOK(sp.Title, 80) || !textOK(sp.Description, 200) {
		return fmt.Errorf("title ≤ 80, description ≤ 200 characters")
	}
	for _, c := range []string{sp.Theme.Background, sp.Theme.Text, sp.Theme.Accent} {
		if c != "" && !hexColor.MatchString(c) {
			return fmt.Errorf("theme colours must be #rrggbb")
		}
	}
	if !oneOf(sp.Theme.Width, "", "normal", "wide", "full") {
		return fmt.Errorf("theme width: normal, wide or full")
	}
	if len(sp.Blocks) > landingMaxBlocks {
		return fmt.Errorf("at most %d blocks", landingMaxBlocks)
	}
	ids := map[string]bool{}
	for i := range sp.Blocks {
		b := &sp.Blocks[i]
		at := fmt.Sprintf("block %d", i+1)
		if !landingTypes[b.Type] {
			return fmt.Errorf("%s: unknown type %q", at, b.Type)
		}
		if !landingIDRe.MatchString(b.ID) || ids[b.ID] {
			return fmt.Errorf("%s: id must be unique [a-z0-9_-]", at)
		}
		ids[b.ID] = true
		if !textOK(b.Title, 160) || !textOK(b.Subtitle, 300) || !textOK(b.Text, 4000) || !textOK(b.Address, 300) {
			return fmt.Errorf("%s: text too long", at)
		}
		for _, c := range []string{b.Background, b.Background2, b.TextColor} {
			if c != "" && !hexColor.MatchString(c) {
				return fmt.Errorf("%s: colours must be #rrggbb", at)
			}
		}
		if b.Image != "" && b.Image != "logo" && !(uuidRe.MatchString(b.Image) && assetOK(b.Image)) {
			return fmt.Errorf("%s: image not found", at)
		}
		if !oneOf(b.ImageSide, "", "left", "right") || !oneOf(b.Align, "", "left", "center") || !oneOf(b.Size, "", "small", "medium", "large") {
			return fmt.Errorf("%s: image_side left|right, align left|center, size small|medium|large", at)
		}
		if b.Columns != 0 && (b.Columns < 1 || b.Columns > 4) {
			return fmt.Errorf("%s: columns 1 to 4", at)
		}
		if len(b.Items) > landingMaxItems || len(b.Buttons) > 3 {
			return fmt.Errorf("%s: at most %d items and 3 buttons", at, landingMaxItems)
		}
		for _, it := range b.Items {
			if !textOK(it.Title, 120) || !textOK(it.Text, 600) || !textOK(it.Value, 40) {
				return fmt.Errorf("%s: item text too long", at)
			}
			if it.Icon != "" && !tablerIcons[it.Icon] {
				return fmt.Errorf("%s: unknown icon %q", at, it.Icon)
			}
		}
		for _, bt := range b.Buttons {
			if !textOK(bt.Label, 40) || strings.TrimSpace(bt.Label) == "" || !landingHrefRe.MatchString(bt.Href) || strings.HasPrefix(bt.Href, "//") || !oneOf(bt.Style, "", "primary", "secondary") {
				return fmt.Errorf("%s: a button needs a label and a link to a page here, https://, mailto: or tel:", at)
			}
		}
		if b.Email != "" && !landingEmailRe.MatchString(b.Email) || b.Phone != "" && !landingPhoneRe.MatchString(b.Phone) {
			return fmt.Errorf("%s: invalid e-mail or phone", at)
		}
	}
	return nil
}

// defaultLanding is the built-in page as blocks: the starting point of the editor.
func defaultLanding() LandingSpec {
	return LandingSpec{Version: 1, Title: "ctrl32 telemetry", Description: "Industrial telemetry on your own infrastructure.", Theme: LandingTheme{Width: "normal"}, Blocks: []LandingBlock{
		{ID: "hero", Type: "hero", Image: "logo", Align: "center", Title: "Industrial telemetry on your own infrastructure",
			Text:    "ctrl32 telemetry collects measurements from devices, PLCs and gateways, stores every value as a verifiable record and turns the data into alarms, reports and exports. Self-hosted, multi-tenant and free of charge for every deployment.",
			Buttons: []LandingButton{{Label: "Sign in", Href: "/login", Style: "primary"}}},
		{ID: "features", Type: "features", Columns: 3, Items: []LandingItem{
			{Icon: "plug-connected", Title: "Connectivity", Text: "MQTT over TLS with per-device tokens, HTTP, LoRaWAN through ChirpStack and OPC UA polling of PLCs and servers. Client libraries for Arduino, ESP-IDF and STM32."},
			{Icon: "device-desktop", Title: "Device management", Text: "Provisioning with claim codes, firmware campaigns with rollback, remote console, commands with acknowledgement and shared attributes."},
			{Icon: "shield-check", Title: "Records you can prove", Text: "Append-only observations with device and server timestamps, gap detection, a hash-chained audit trail and integrity-checked exports."},
			{Icon: "bell", Title: "Alarms and reports", Text: "Warning and action limits with hysteresis, e-mail notification, CSV and JSON Lines exports and PDF reports built from blocks or your own templates."},
			{Icon: "users", Title: "Organizations and access", Text: "Separate tenants on one server, roles and permissions, two-factor authentication, 19 languages, your logo and colours."},
			{Icon: "server", Title: "Operations", Text: "One-command installer for Linux and Windows, TLS, WireGuard for devices, backups, updates and server settings from the UI. The data stays with you."},
		}},
	}}
}

func (s *Server) landingAssetExists(ctx context.Context, id string) bool {
	var n int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM core.landing_asset WHERE id = $1::uuid`, id).Scan(&n)
	return n > 0
}

// publicLanding serves the published page to everyone.
func (s *Server) publicLanding(w http.ResponseWriter, r *http.Request) {
	var raw []byte
	var v *int
	_ = s.pool.QueryRow(r.Context(), `SELECT st.published_version, lv.spec FROM core.landing_state st LEFT JOIN core.landing_version lv ON lv.version = st.published_version WHERE st.id = 1`).Scan(&v, &raw)
	if v == nil || raw == nil {
		writeJSON(w, 200, map[string]any{"spec": nil})
		return
	}
	writeJSON(w, 200, map[string]any{"version": *v, "spec": json.RawMessage(raw)})
}

func (s *Server) landingDraft(w http.ResponseWriter, r *http.Request) {
	var ver int
	var raw []byte
	var pub *int
	_ = s.pool.QueryRow(r.Context(), `SELECT published_version FROM core.landing_state WHERE id = 1`).Scan(&pub)
	err := s.pool.QueryRow(r.Context(), `SELECT version, spec FROM core.landing_version ORDER BY version DESC LIMIT 1`).Scan(&ver, &raw)
	out := map[string]any{"published_version": pub}
	if err != nil {
		out["version"], out["spec"] = nil, defaultLanding()
	} else {
		out["version"], out["spec"] = ver, json.RawMessage(raw)
	}
	writeJSON(w, 200, out)
}

func (s *Server) landingVersions(w http.ResponseWriter, r *http.Request) {
	var pub *int
	_ = s.pool.QueryRow(r.Context(), `SELECT published_version FROM core.landing_state WHERE id = 1`).Scan(&pub)
	rows, err := s.pool.Query(r.Context(), `SELECT version, reason, created_at, created_by, jsonb_array_length(spec->'blocks') FROM core.landing_version ORDER BY version DESC LIMIT 200`)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var v, n int
		var reason, by string
		var at time.Time
		if rows.Scan(&v, &reason, &at, &by, &n) == nil {
			out = append(out, map[string]any{"version": v, "reason": reason, "created_at": at, "created_by": by, "blocks": n, "published": pub != nil && *pub == v})
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) landingVersion(w http.ResponseWriter, r *http.Request) {
	v, err := strconv.Atoi(r.PathValue("v"))
	var raw []byte
	if err == nil {
		err = s.pool.QueryRow(r.Context(), `SELECT spec FROM core.landing_version WHERE version = $1`, v).Scan(&raw)
	}
	if err != nil {
		writeErr(w, 404, "version not found")
		return
	}
	writeJSON(w, 200, map[string]any{"version": v, "spec": json.RawMessage(raw)})
}

func (s *Server) saveLanding(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Spec    LandingSpec `json:"spec"`
		Reason  string      `json:"reason"`
		Publish bool        `json:"publish"`
	}
	if err := decodeJSON(r, &in, 512<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if err := validateLanding(&in.Spec, func(id string) bool { return s.landingAssetExists(r.Context(), id) }); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	raw, _ := json.Marshal(in.Spec)
	by := PrincipalFrom(r.Context()).Username
	var v int
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.landing_version (spec, reason, created_by) VALUES ($1::jsonb, $2, $3) RETURNING version`, string(raw), in.Reason, by).Scan(&v); err != nil {
		writeErr(w, 500, "not stored")
		return
	}
	s.audit(r, "landing.save", "landing", "", nil, map[string]any{"version": v, "blocks": len(in.Spec.Blocks)}, in.Reason)
	if in.Publish {
		s.publishLandingVersion(r, v, in.Reason)
	}
	writeJSON(w, 201, map[string]any{"version": v, "published": in.Publish})
}

func (s *Server) publishLandingVersion(r *http.Request, v int, reason string) {
	var old *int
	_ = s.pool.QueryRow(r.Context(), `SELECT published_version FROM core.landing_state WHERE id = 1`).Scan(&old)
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.landing_state SET published_version = $1, published_at = now(), published_by = $2 WHERE id = 1`, v, PrincipalFrom(r.Context()).Username)
	s.audit(r, "landing.publish", "landing", "", map[string]any{"published_version": old}, map[string]any{"published_version": v}, reason)
}

func (s *Server) publishLanding(w http.ResponseWriter, r *http.Request) {
	v, err := strconv.Atoi(r.PathValue("v"))
	var in struct {
		Reason string `json:"reason"`
	}
	if derr := decodeJSON(r, &in, 4096); derr != nil {
		writeErr(w, 400, derr.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var n int
	if err == nil {
		_ = s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.landing_version WHERE version = $1`, v).Scan(&n)
	}
	if n == 0 {
		writeErr(w, 404, "version not found")
		return
	}
	s.publishLandingVersion(r, v, in.Reason)
	writeJSON(w, 200, map[string]any{"published_version": v})
}

func (s *Server) unpublishLanding(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var old *int
	_ = s.pool.QueryRow(r.Context(), `SELECT published_version FROM core.landing_state WHERE id = 1`).Scan(&old)
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.landing_state SET published_version = NULL, published_at = now(), published_by = $1 WHERE id = 1`, PrincipalFrom(r.Context()).Username)
	s.audit(r, "landing.unpublish", "landing", "", map[string]any{"published_version": old}, map[string]any{"published_version": nil}, in.Reason)
	writeJSON(w, 200, map[string]any{"published_version": nil})
}

func (s *Server) landingAssets(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `SELECT id::text, name, mime, bytes, created_at FROM core.landing_asset ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, mime string
		var n int
		var at time.Time
		if rows.Scan(&id, &name, &mime, &n, &at) == nil {
			out = append(out, map[string]any{"id": id, "name": name, "mime": mime, "bytes": n, "created_at": at, "url": "/api/v1/landing/assets/" + id})
		}
	}
	writeJSON(w, 200, out)
}

// uploadLandingAsset accepts PNG, JPEG, WebP (checked by content, not by name) and SVG (sanitised) up to 2 MiB.
func (s *Server) uploadLandingAsset(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, landingMaxAsset+64<<10)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeErr(w, 413, "multipart form with an image of at most 2 MiB required")
		return
	}
	if !reasonOf(w, r.FormValue("reason")) {
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "file field required")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, landingMaxAsset+1))
	if err != nil || len(data) == 0 || len(data) > landingMaxAsset {
		writeErr(w, 413, "the image must be at most 2 MiB")
		return
	}
	mime := http.DetectContentType(data)
	switch {
	case mime == "image/png" || mime == "image/jpeg":
		if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
			writeErr(w, 400, "not a valid image")
			return
		}
	case mime == "image/webp":
	case bytes.Contains(bytes.ToLower(data[:min(len(data), 1024)]), []byte("<svg")):
		clean, err := sanitizeSVG(string(data))
		if err != nil {
			writeErr(w, 400, "SVG: "+err.Error())
			return
		}
		data, mime = []byte(clean.SVG), "image/svg+xml"
	default:
		writeErr(w, 415, "PNG, JPEG, WebP or SVG images only")
		return
	}
	name := strings.TrimSpace(hdr.Filename)
	if !textOK(name, 120) || name == "" {
		name = "image"
	}
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.landing_asset (name, mime, data, bytes, created_by) VALUES ($1, $2, $3, $4, $5) RETURNING id::text`,
		name, mime, data, len(data), PrincipalFrom(r.Context()).Username).Scan(&id); err != nil {
		writeErr(w, 500, "not stored")
		return
	}
	s.audit(r, "landing.asset", "landing_asset", id, nil, map[string]any{"name": name, "mime": mime, "bytes": len(data)}, r.FormValue("reason"))
	writeJSON(w, 201, map[string]any{"id": id, "name": name, "mime": mime, "bytes": len(data), "url": "/api/v1/landing/assets/" + id})
}

// landingAsset serves an image; the sandbox policy keeps SVG from running anything even when opened directly.
func (s *Server) landingAsset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	var mime string
	var data []byte
	if err := s.pool.QueryRow(r.Context(), `SELECT mime, data FROM core.landing_asset WHERE id = $1::uuid`, id).Scan(&mime, &data); err != nil {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", mime)
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	h.Set("Cache-Control", "public, max-age=86400, immutable")
	_, _ = w.Write(data)
}

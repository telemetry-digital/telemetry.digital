package httpapi

// Displays (spec 014): kiosk devices — a PC or a TV at a wall — that show dashboards, process screens and video wall
// screens without a user session. A display is paired once: the kiosk page (/display) asks for a pairing, shows a
// short code and keeps a secret in an HttpOnly cookie; an administrator enters the code and chooses the content;
// the kiosk then collects its own credential (a random token in an HttpOnly cookie, only its hash stored). Knowing
// the code alone never yields the credential. A display reads only what it was given, changes nothing, and is revoked
// at any time. Operators send cameras and commands to displays (alarm views on a wall).
//
//	POST /api/v1/display/hello                    kiosk      {code_known}: paired, or a pairing code to show
//	GET  /api/v1/display/state                    display    content, current command, active camera events
//	GET  /api/v1/display/walls/{id}               display    a wall of its content with camera names
//	GET  /api/v1/display/cameras/{id}/live        display    live video of a camera it may show (WebSocket)
//	GET  /api/v1/public/~{slug}/…                 display    dashboards of its content (public_dashboard.go)
//	GET  /api/v1/displays                         display.control
//	POST /api/v1/displays/pair                    display.manage  {code, name, content, reason}
//	PUT  /api/v1/displays/{id}                    display.manage  {name, content, offline_alert, reason}
//	DELETE /api/v1/displays/{id}?reason=          display.manage  revoke
//	POST /api/v1/displays/{id}/command            display.control {action: camera|identify|reload|clear, camera, seconds, quality}

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

var localeRe = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z]{2,4})?$`)

const (
	displayCookie     = "ctrl32_display"
	displayPairCookie = "ctrl32_display_pair"
	pairingTTL        = 15 * time.Minute
	codeAlphabet      = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O, 1/I
)

type displayItem struct {
	Kind    string `json:"kind"` // dashboard (also process screens) or wall
	Slug    string `json:"slug,omitempty"`
	Wall    string `json:"wall,omitempty"`
	Screen  int    `json:"screen"`
	Seconds int    `json:"seconds,omitempty"` // how long it stays when the display shows several items in turn
}

type displayContent struct {
	Items        []displayItem `json:"items"`
	Popup        string        `json:"popup"`            // none, wall (cameras of its walls) or all (every camera)
	PopupSeconds int           `json:"popup_seconds"`    // how long a camera with an event stays enlarged
	Locale       string        `json:"locale,omitempty"` // language of the display ("" = the browser's)
}

type displayCommand struct {
	Seq     int64     `json:"seq"`
	Action  string    `json:"action"` // camera, identify, reload, clear
	Camera  string    `json:"camera,omitempty"`
	Name    string    `json:"name,omitempty"`
	Quality string    `json:"quality,omitempty"`
	Until   time.Time `json:"until"`
	By      string    `json:"by"`
}

type displayRow struct {
	ID, Org, Name string
	Content       displayContent
	Command       *displayCommand
	Updated       time.Time
	Paired        bool
	LastSeen      *time.Time
	LastIP        string
	LastAgent     string
	OfflineAlert  bool
	Created       time.Time
}

func (d *displayRow) json() map[string]any {
	out := map[string]any{"id": d.ID, "name": d.Name, "content": d.Content, "paired": d.Paired, "last_seen_at": d.LastSeen,
		"last_ip": d.LastIP, "last_agent": d.LastAgent, "offline_alert": d.OfflineAlert, "created_at": d.Created, "updated_at": d.Updated,
		"online": d.LastSeen != nil && time.Since(*d.LastSeen) < 90*time.Second}
	if d.Command != nil && d.Command.Until.After(time.Now()) {
		out["command"] = d.Command
	}
	return out
}

const displayCols = `id::text, org_id::text, name, content, command, updated_at, token_hash IS NOT NULL, last_seen_at, COALESCE(last_ip, ''), COALESCE(last_agent, ''), offline_alert, created_at`

func scanDisplay(row pgx.Row) (*displayRow, error) {
	d := &displayRow{}
	var content, cmd []byte
	if err := row.Scan(&d.ID, &d.Org, &d.Name, &content, &cmd, &d.Updated, &d.Paired, &d.LastSeen, &d.LastIP, &d.LastAgent, &d.OfflineAlert, &d.Created); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(content, &d.Content)
	if len(cmd) > 0 {
		d.Command = &displayCommand{}
		if json.Unmarshal(cmd, d.Command) != nil {
			d.Command = nil
		}
	}
	return d, nil
}

// displayFromCookie authenticates a kiosk by its cookie.
func (s *Server) displayFromCookie(r *http.Request) (*displayRow, bool) {
	c, err := r.Cookie(displayCookie)
	if err != nil || !strings.HasPrefix(c.Value, "c32d_") || len(c.Value) > 100 {
		return nil, false
	}
	d, err := scanDisplay(s.pool.QueryRow(r.Context(), `SELECT `+displayCols+` FROM core.display WHERE token_hash = $1 AND revoked_at IS NULL`, iam.HashToken(c.Value)))
	if err != nil {
		return nil, false
	}
	return d, true
}

func (s *Server) setDisplayCookie(w http.ResponseWriter, r *http.Request, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: maxAge, HttpOnly: true, Secure: s.isSecure(r), SameSite: http.SameSiteStrictMode})
}

func randomCode() (string, error) {
	b := make([]byte, 8)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		if err != nil {
			return "", err
		}
		b[i] = codeAlphabet[n.Int64()]
	}
	return string(b), nil
}

func normalizeCode(c string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(c) {
		if strings.ContainsRune(codeAlphabet, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ---- kiosk --------------------------------------------------------------------------------------------------------

func (s *Server) displayHello(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if d, ok := s.displayFromCookie(r); ok {
		writeJSON(w, 200, map[string]any{"paired": true, "name": d.Name})
		return
	}
	var in struct {
		CodeKnown bool `json:"code_known"`
	}
	if r.ContentLength > 0 {
		_ = decodeJSON(r, &in, 256)
	}
	ctx := r.Context()
	_, _ = s.pool.Exec(ctx, `DELETE FROM core.display_pairing WHERE expires_at < now()`)
	if c, err := r.Cookie(displayPairCookie); err == nil && c.Value != "" && len(c.Value) < 100 {
		sh := iam.HashToken(c.Value)
		var displayID *string
		var expires time.Time
		if s.pool.QueryRow(ctx, `SELECT display_id::text, expires_at FROM core.display_pairing WHERE secret_hash = $1`, sh).Scan(&displayID, &expires) == nil {
			if displayID != nil {
				// paired by an administrator: this kiosk (and only it, by its secret) collects the credential
				raw, err := iam.RandomToken(32)
				if err != nil {
					writeErr(w, 500, "random failed")
					return
				}
				raw = "c32d_" + raw
				tag, err := s.pool.Exec(ctx, `UPDATE core.display SET token_hash = $2, last_seen_at = now(), last_ip = $3, last_agent = $4
					WHERE id = $1::uuid AND revoked_at IS NULL AND token_hash IS NULL`, *displayID, iam.HashToken(raw), clientIP(r), truncateStr(r.UserAgent(), 300))
				_, _ = s.pool.Exec(ctx, `DELETE FROM core.display_pairing WHERE secret_hash = $1`, sh)
				s.setDisplayCookie(w, r, displayPairCookie, "", -1)
				if err != nil || tag.RowsAffected() != 1 {
					writeErr(w, 409, "the pairing was withdrawn; start again")
					return
				}
				s.setDisplayCookie(w, r, displayCookie, raw, 400*24*3600)
				s.log.Info("display paired", "display", *displayID, "remote", clientIP(r))
				writeJSON(w, 200, map[string]any{"paired": true})
				return
			}
			if in.CodeKnown {
				writeJSON(w, 200, map[string]any{"paired": false, "expires_at": expires})
				return
			}
			_, _ = s.pool.Exec(ctx, `DELETE FROM core.display_pairing WHERE secret_hash = $1`, sh)
		}
	}
	ip := clientIP(r)
	var perIP, total int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE ip = $1), count(*) FROM core.display_pairing`, ip).Scan(&perIP, &total)
	if perIP >= 20 || total >= 500 {
		s.log.Warn("display pairing refused: too many pending", "security", true, "remote", ip)
		writeErr(w, 429, "too many pairings waiting; try again later")
		return
	}
	code, err := randomCode()
	secret, err2 := iam.RandomToken(32)
	if err != nil || err2 != nil {
		writeErr(w, 500, "random failed")
		return
	}
	expires := time.Now().Add(pairingTTL)
	if _, err := s.pool.Exec(ctx, `INSERT INTO core.display_pairing (code_hash, secret_hash, ip, agent, expires_at) VALUES ($1, $2, $3, $4, $5)`,
		iam.HashToken(code), iam.HashToken(secret), ip, truncateStr(r.UserAgent(), 300), expires); err != nil {
		writeErr(w, 500, "pairing not stored")
		return
	}
	s.setDisplayCookie(w, r, displayPairCookie, secret, int(pairingTTL.Seconds()))
	writeJSON(w, 200, map[string]any{"paired": false, "code": code[:4] + "-" + code[4:], "expires_at": expires})
}

func truncateStr(v string, n int) string {
	if len(v) > n {
		return v[:n]
	}
	return v
}

// displayCameras are the cameras a display may play now: on its walls, in camera widgets of its dashboards, the
// camera an operator sent, and (with popups for all cameras) any camera with an active event.
func (s *Server) displayCameras(ctx context.Context, d *displayRow) map[string]bool {
	out := map[string]bool{}
	for _, it := range d.Content.Items {
		switch it.Kind {
		case "wall":
			for _, id := range s.wallTiles(ctx, d.Org, it.Wall, it.Screen) {
				out[id] = true
			}
		case "dashboard":
			if si, ok := s.shareOf(ctx, d.Org, it.Slug); ok {
				for id := range si.Cams {
					out[id] = true
				}
			}
		}
	}
	if d.Command != nil && d.Command.Action == "camera" && d.Command.Until.After(time.Now()) {
		out[d.Command.Camera] = true
	}
	if d.Content.Popup == "all" && s.video != nil {
		for _, id := range s.orgCameraIDs(ctx, d.Org) {
			if st, ok := s.video.Status(id); ok && len(st.Active) > 0 {
				out[id] = true
			}
		}
	}
	return out
}

func (s *Server) orgCameraIDs(ctx context.Context, org string) []string {
	rows, err := s.pool.Query(ctx, `SELECT id::text FROM core.camera WHERE org_id = $1::uuid AND removed_at IS NULL AND enabled`, org)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// wallTiles returns the cameras of one screen of a wall (all screens with screen < 0).
func (s *Server) wallTiles(ctx context.Context, org, wall string, screen int) []string {
	var raw []byte
	if s.pool.QueryRow(ctx, `SELECT screens FROM core.video_wall WHERE id::text = $1 AND org_id = $2::uuid AND removed_at IS NULL`, wall, org).Scan(&raw) != nil {
		return nil
	}
	var screens []WallScreen
	if json.Unmarshal(raw, &screens) != nil {
		return nil
	}
	var ids []string
	for i, sc := range screens {
		if screen >= 0 && i != screen {
			continue
		}
		for _, t := range sc.Tiles {
			if t != nil {
				ids = append(ids, *t)
			}
		}
	}
	return ids
}

func (s *Server) displayState(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	d, ok := s.displayFromCookie(r)
	if !ok {
		writeErr(w, 401, "this display is not paired")
		return
	}
	ctx := r.Context()
	_, _ = s.pool.Exec(ctx, `UPDATE core.display SET last_seen_at = now(), last_ip = $2, last_agent = $3
		WHERE id = $1::uuid AND (last_seen_at IS NULL OR last_seen_at < now() - interval '20 seconds')`, d.ID, clientIP(r), truncateStr(r.UserAgent(), 300))
	items := []map[string]any{}
	for _, it := range d.Content.Items {
		m := map[string]any{"kind": it.Kind, "seconds": it.Seconds}
		switch it.Kind {
		case "dashboard":
			var name, kind string
			if s.pool.QueryRow(ctx, `SELECT name, COALESCE(spec->>'kind', 'dashboard') FROM core.dashboard WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL`, d.Org, it.Slug).Scan(&name, &kind) != nil {
				continue // removed since; the display skips it
			}
			m["slug"], m["title"], m["screen_kind"] = it.Slug, name, kind
		case "wall":
			wj, ok := s.wallJSON(ctx, d.Org, it.Wall)
			if !ok {
				continue
			}
			m["wall"], m["screen"], m["title"] = it.Wall, it.Screen, wj["name"]
		}
		items = append(items, m)
	}
	// cameras with events: those of its walls, or all cameras of the organization
	active := map[string]any{}
	if s.video != nil && d.Content.Popup != "none" {
		var ids []string
		if d.Content.Popup == "all" {
			ids = s.orgCameraIDs(ctx, d.Org)
		} else {
			for _, it := range d.Content.Items {
				if it.Kind == "wall" {
					ids = append(ids, s.wallTiles(ctx, d.Org, it.Wall, it.Screen)...)
				}
			}
		}
		for _, id := range ids {
			if st, ok := s.video.Status(id); ok && len(st.Active) > 0 {
				active[id] = map[string]any{"kinds": st.Active}
			}
		}
	}
	allowed := s.displayCameras(ctx, d)
	ids := make([]string, 0, len(allowed))
	for id := range allowed {
		ids = append(ids, id)
	}
	cams := s.cameraNames(ctx, d.Org, ids)
	for id, a := range active {
		a.(map[string]any)["name"] = cams[id]
		if cams[id] == "" {
			a.(map[string]any)["name"] = s.cameraName(ctx, d.Org, id)
		}
	}
	out := map[string]any{"id": d.ID, "name": d.Name, "version": d.Updated.UnixMilli(), "items": items, "popup": d.Content.Popup, "cameras": cams,
		"popup_seconds": d.Content.PopupSeconds, "locale": d.Content.Locale, "active": active, "server_time": time.Now()}
	if d.Command != nil && d.Command.Until.After(time.Now()) {
		out["command"] = d.Command
	}
	writeJSON(w, 200, out)
}

// cameraNames returns the names of cameras of the organization in one query.
func (s *Server) cameraNames(ctx context.Context, org string, ids []string) map[string]string {
	out := map[string]string{}
	rows, err := s.pool.Query(ctx, `SELECT id::text, name FROM core.camera WHERE org_id = $1::uuid AND id::text = ANY($2::text[]) AND removed_at IS NULL`, org, ids)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, n string
		if rows.Scan(&id, &n) == nil {
			out[id] = n
		}
	}
	return out
}

func (s *Server) cameraName(ctx context.Context, org, id string) string {
	var n string
	_ = s.pool.QueryRow(ctx, `SELECT name FROM core.camera WHERE id::text = $1 AND org_id = $2::uuid`, id, org).Scan(&n)
	return n
}

func (s *Server) displayWall(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	d, ok := s.displayFromCookie(r)
	if !ok {
		writeErr(w, 401, "this display is not paired")
		return
	}
	id := r.PathValue("id")
	if !slices.ContainsFunc(d.Content.Items, func(it displayItem) bool { return it.Kind == "wall" && it.Wall == id }) {
		writeErr(w, 404, "wall not found")
		return
	}
	wj, ok := s.wallJSON(r.Context(), d.Org, id)
	if !ok {
		writeErr(w, 404, "wall not found")
		return
	}
	wj["cameras"] = s.cameraNames(r.Context(), d.Org, s.wallTiles(r.Context(), d.Org, id, -1))
	writeJSON(w, 200, wj)
}

// displayLive plays a camera the display may show, under a principal of the display's organization.
func (s *Server) displayLive(w http.ResponseWriter, r *http.Request) {
	d, ok := s.displayFromCookie(r)
	if !ok {
		writeErr(w, 401, "this display is not paired")
		return
	}
	if !s.displayCameras(r.Context(), d)[r.PathValue("id")] {
		writeErr(w, 403, "this camera is not part of the display's content")
		return
	}
	p := &iam.Principal{OrgID: d.Org, Username: "display:" + d.Name, Permissions: map[string]bool{"video.view": true}}
	s.cameraLive(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
}

// displayShare lets a display read the dashboards of its content like a public link (and their cameras).
func (s *Server) displayShare(r *http.Request, slug string) (*shareInfo, bool) {
	d, ok := s.displayFromCookie(r)
	if !ok {
		return nil, false
	}
	if !slices.ContainsFunc(d.Content.Items, func(it displayItem) bool { return it.Kind == "dashboard" && it.Slug == slug }) {
		return nil, false
	}
	return s.shareOf(r.Context(), d.Org, slug)
}

// ---- administration -----------------------------------------------------------------------------------------------

func (s *Server) validateDisplayContent(ctx context.Context, org string, c *displayContent) string {
	if len(c.Items) == 0 || len(c.Items) > 20 {
		return "content: 1 to 20 items"
	}
	for i := range c.Items {
		it := &c.Items[i]
		if it.Seconds == 0 {
			it.Seconds = 60
		}
		if it.Seconds < 10 || it.Seconds > 3600 {
			return "seconds: 10 to 3600"
		}
		switch it.Kind {
		case "dashboard":
			it.Wall, it.Screen = "", 0
			var n int
			_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM core.dashboard WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL`, org, it.Slug).Scan(&n)
			if n == 0 {
				return "dashboard " + it.Slug + " not found"
			}
		case "wall":
			it.Slug = ""
			wj, ok := s.wallJSON(ctx, org, it.Wall)
			if !ok {
				return "video wall not found"
			}
			var screens []WallScreen
			_ = json.Unmarshal(wj["screens"].(json.RawMessage), &screens)
			if it.Screen < 0 || it.Screen >= len(screens) {
				return "screen: not a screen of the wall"
			}
		default:
			return "kind: dashboard or wall"
		}
	}
	switch c.Popup {
	case "":
		c.Popup = "none"
	case "none", "wall", "all":
	default:
		return "popup: none, wall or all"
	}
	if c.PopupSeconds == 0 {
		c.PopupSeconds = 30
	}
	if c.PopupSeconds < 5 || c.PopupSeconds > 600 {
		return "popup_seconds: 5 to 600"
	}
	if c.Locale != "" && !localeRe.MatchString(c.Locale) {
		return "locale: a language code such as sk or en"
	}
	return ""
}

func (s *Server) loadDisplay(ctx context.Context, org, id string) (*displayRow, bool) {
	d, err := scanDisplay(s.pool.QueryRow(ctx, `SELECT `+displayCols+` FROM core.display WHERE id::text = $1 AND org_id = $2::uuid AND revoked_at IS NULL`, id, org))
	return d, err == nil
}

func (s *Server) listDisplays(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT `+displayCols+` FROM core.display WHERE org_id = $1::uuid AND revoked_at IS NULL ORDER BY name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		if d, err := scanDisplay(rows); err == nil {
			out = append(out, d.json())
		}
	}
	writeJSON(w, 200, out)
}

// pairFails limits wrong pairing codes per user (10 in 10 minutes).
var pairFails = struct {
	sync.Mutex
	m map[string][]time.Time
}{m: map[string][]time.Time{}}

func pairLimited(user string, fail bool) bool {
	pairFails.Lock()
	defer pairFails.Unlock()
	var recent []time.Time
	for _, t := range pairFails.m[user] {
		if time.Since(t) < 10*time.Minute {
			recent = append(recent, t)
		}
	}
	if fail {
		recent = append(recent, time.Now())
	}
	pairFails.m[user] = recent
	return len(recent) >= 10
}

func (s *Server) pairDisplay(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	var in struct {
		Code         string         `json:"code"`
		Name         string         `json:"name"`
		Content      displayContent `json:"content"`
		OfflineAlert *bool          `json:"offline_alert"`
		Reason       string         `json:"reason"`
	}
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if pairLimited(p.UserID, false) {
		writeErr(w, 429, "too many wrong codes; wait 10 minutes")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 80 {
		writeErr(w, 400, "name: 1 to 80 characters")
		return
	}
	if msg := s.validateDisplayContent(r.Context(), org, &in.Content); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	code := normalizeCode(in.Code)
	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		writeErr(w, 500, "begin failed")
		return
	}
	defer tx.Rollback(ctx)
	var ip, agent string
	if len(code) != 8 || tx.QueryRow(ctx, `SELECT ip, agent FROM core.display_pairing WHERE code_hash = $1 AND expires_at > now() AND display_id IS NULL FOR UPDATE`,
		iam.HashToken(code)).Scan(&ip, &agent) != nil {
		pairLimited(p.UserID, true)
		s.log.Warn("display pairing: unknown code", "security", true, "user", p.Username, "remote", clientIP(r))
		writeErr(w, 404, "no display is waiting with this code (codes last 15 minutes)")
		return
	}
	content, _ := json.Marshal(in.Content)
	alert := in.OfflineAlert == nil || *in.OfflineAlert
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO core.display (org_id, name, content, created_by, offline_alert) VALUES ($1::uuid, $2, $3::jsonb, NULLIF($4, '')::uuid, $5) RETURNING id::text`,
		org, in.Name, string(content), p.UserID, alert).Scan(&id); err != nil {
		writeErr(w, 500, "display not stored")
		return
	}
	// the kiosk collects its credential within 10 minutes
	if _, err := tx.Exec(ctx, `UPDATE core.display_pairing SET display_id = $2::uuid, expires_at = now() + interval '10 minutes' WHERE code_hash = $1`, iam.HashToken(code), id); err != nil {
		writeErr(w, 500, "pairing failed")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeErr(w, 500, "commit failed")
		return
	}
	s.audit(r, "display.pair", "display", id, nil, map[string]any{"name": in.Name, "content": in.Content, "device_ip": ip, "device_agent": agent}, in.Reason)
	d, _ := s.loadDisplay(ctx, org, id)
	out := d.json()
	out["device_ip"], out["device_agent"] = ip, agent
	writeJSON(w, 201, out)
}

func (s *Server) updateDisplay(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	d, ok := s.loadDisplay(r.Context(), org, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "display not found")
		return
	}
	var in struct {
		Name         *string         `json:"name"`
		Content      *displayContent `json:"content"`
		OfflineAlert *bool           `json:"offline_alert"`
		Reason       string          `json:"reason"`
	}
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	old := d.json()
	if in.Name != nil {
		d.Name = strings.TrimSpace(*in.Name)
		if d.Name == "" || len([]rune(d.Name)) > 80 {
			writeErr(w, 400, "name: 1 to 80 characters")
			return
		}
	}
	if in.Content != nil {
		if msg := s.validateDisplayContent(r.Context(), org, in.Content); msg != "" {
			writeErr(w, 400, msg)
			return
		}
		d.Content = *in.Content
	}
	if in.OfflineAlert != nil {
		d.OfflineAlert = *in.OfflineAlert
	}
	content, _ := json.Marshal(d.Content)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.display SET name = $2, content = $3::jsonb, offline_alert = $4, updated_at = now() WHERE id = $1::uuid`,
		d.ID, d.Name, string(content), d.OfflineAlert); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	s.audit(r, "display.update", "display", d.ID, map[string]any{"name": old["name"], "content": old["content"], "offline_alert": old["offline_alert"]},
		map[string]any{"name": d.Name, "content": d.Content, "offline_alert": d.OfflineAlert}, in.Reason)
	d, _ = s.loadDisplay(r.Context(), org, d.ID)
	writeJSON(w, 200, d.json())
}

func (s *Server) revokeDisplay(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	d, ok := s.loadDisplay(r.Context(), org, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "display not found")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.display SET revoked_at = now(), token_hash = NULL, updated_at = now() WHERE id = $1::uuid`, d.ID); err != nil {
		writeErr(w, 500, "revoke failed")
		return
	}
	_, _ = s.pool.Exec(r.Context(), `DELETE FROM core.display_pairing WHERE display_id = $1::uuid`, d.ID)
	s.audit(r, "display.revoke", "display", d.ID, map[string]any{"name": d.Name, "last_ip": d.LastIP}, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

var errNoCamera = errors.New("camera not found")

func (s *Server) commandDisplay(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	d, ok := s.loadDisplay(r.Context(), org, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "display not found")
		return
	}
	var in struct {
		Action  string `json:"action"`
		Camera  string `json:"camera"`
		Seconds int    `json:"seconds"`
		Quality string `json:"quality"`
	}
	if err := decodeJSON(r, &in, 1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.Seconds == 0 {
		in.Seconds = 60
	}
	if in.Seconds < 5 || in.Seconds > 3600 {
		writeErr(w, 400, "seconds: 5 to 3600")
		return
	}
	p := PrincipalFrom(r.Context())
	cmd := displayCommand{Seq: time.Now().UnixNano(), Action: in.Action, Until: time.Now().Add(time.Duration(in.Seconds) * time.Second), By: p.Username}
	switch in.Action {
	case "camera":
		// the operator may send only cameras they may see themselves
		c, ok := s.cameraRow(r.Context(), org, in.Camera)
		if !ok {
			writeErr(w, 404, errNoCamera.Error())
			return
		}
		if !p.Has("video.view") {
			writeErr(w, 403, "permission denied: video.view")
			return
		}
		cmd.Camera, cmd.Name = c.id, c.name
		cmd.Quality = "main"
		if in.Quality == "sub" {
			cmd.Quality = "sub"
		}
	case "identify", "reload":
		if in.Action == "reload" {
			cmd.Until = time.Now().Add(2 * time.Minute)
		} else if in.Seconds > 60 {
			cmd.Until = time.Now().Add(time.Minute)
		}
	case "clear":
		cmd.Until = time.Now().Add(2 * time.Minute)
	default:
		writeErr(w, 400, "action: camera, identify, reload or clear")
		return
	}
	raw, _ := json.Marshal(cmd)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.display SET command = $2::jsonb WHERE id = $1::uuid`, d.ID, string(raw)); err != nil {
		writeErr(w, 500, "command not stored")
		return
	}
	s.audit(r, "display.command", "display", d.ID, nil, map[string]any{"display": d.Name, "action": cmd.Action, "camera": cmd.Name, "seconds": in.Seconds}, "operator command")
	writeJSON(w, 200, cmd)
}

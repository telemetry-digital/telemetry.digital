package httpapi

// Kiosk PCs (0.86.0, spec 034, ADR-0025): the Windows computers behind the displays. An administrator registers a PC
// and downloads its setup script (it carries the PC's credential, shown once); the script prepares the PC — kiosk
// user with automatic logon, power, Windows Update at night, browser policies — and installs two readable PowerShell
// scripts: the launcher (one Chrome kiosk per monitor, each its own paired display) and the agent (a SYSTEM task that
// reports the PC's state and performs a closed list of operations). The agent asks; the server never connects to the
// PC. A screenshot can show unmasked camera pictures: it is off per PC until turned on with a reason, needs
// display.screenshot and a reason each time, is audited, and the pictures stay in memory only until they are fetched.
// Since 0.86.5 the pictures come from the kiosk browsers themselves (one per monitor, DevTools protocol on a port of
// 127.0.0.1), not from a screen grab of the desktop, which Microsoft Defender blocks as a hacking tool.
//
//	GET    /api/v1/kiosks                      display.manage   PCs with their last report and their displays
//	POST   /api/v1/kiosks                      display.manage   {name, monitors, server_url, reason} → 201 {kiosk, token}
//	PUT    /api/v1/kiosks/{id}                 display.manage   {name, monitors, server_url, mesh_node, screenshots, displays, reason}
//	DELETE /api/v1/kiosks/{id}?reason=         display.manage   remove (the agent is locked out, displays unlinked)
//	POST   /api/v1/kiosks/{id}/token           display.manage   {reason} → {token}: a new credential, the old one stops
//	POST   /api/v1/kiosks/{id}/setup           display.manage   {token} → setup-kiosk-….ps1
//	POST   /api/v1/kiosks/{id}/command         display.manage   {action: report|restart_browsers|reboot|update_scripts|screenshot, reason}
//	GET    /api/v1/kiosks/{id}/screenshot      display.screenshot  the pictures asked for by this user (once), one per monitor
//	POST   /api/v1/kiosks/{id}/remote          display.manage   {reason} → {url}: emergency remote desktop (MeshCentral)
//	POST   /api/v1/kiosk/report                kiosk token      {report, result} → {command, scripts_version, poll_s}
//	POST   /api/v1/kiosk/screenshot?seq=&monitor=  kiosk token  image/jpeg
//	GET    /api/v1/kiosk/scripts               kiosk token      {version, files}

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

//go:embed kiosk/*.ps1
var kioskFS embed.FS

const (
	kioskTokenHeader = "X-Kiosk-Token"
	kioskPoll        = 15 // seconds between the agent's reports
	shotTTL          = 3 * time.Minute
	maxShotBytes     = 8 << 20
	kioskCDPPort     = 9222 // DevTools port of monitor 0 on 127.0.0.1 of the PC; monitor i uses kioskCDPPort+i
)

// kioskScriptNames are the files the agent installs and keeps up to date. capture.ps1 is no longer one of them
// (0.86.5), but agents of 0.86.0–0.86.4 refuse an update without it: they get a stub, which the new agent deletes.
var kioskScriptNames = []string{"kiosk-agent.ps1", "kiosk-launcher.ps1", "launcher.json", "capture.ps1"}

const captureStub = "# ctrl32 telemetry: not used since 0.86.5 (screenshots come from the kiosk browsers). The agent deletes this file.\r\n"

var kioskActions = []string{"report", "restart_browsers", "reboot", "update_scripts", "screenshot"}

type kioskCommand struct {
	Seq    int64     `json:"seq"`
	Action string    `json:"action"`
	By     string    `json:"by"`
	Until  time.Time `json:"until"`
}

type kioskRow struct {
	ID, Org, Name, ServerURL, MeshNode, LastIP string
	Monitors                                   int
	Screenshots, HasToken                      bool
	Report, Result                             map[string]any
	Command                                    *kioskCommand
	Created, Updated                           time.Time
	LastSeen                                   *time.Time
}

const kioskCols = `id::text, org_id::text, name, monitors, server_url, mesh_node, screenshots, token_hash IS NOT NULL, report, result, command, created_at, updated_at, last_seen_at, COALESCE(last_ip, '')`

func scanKiosk(row pgx.Row) (*kioskRow, error) {
	k := &kioskRow{}
	var rep, res, cmd []byte
	if err := row.Scan(&k.ID, &k.Org, &k.Name, &k.Monitors, &k.ServerURL, &k.MeshNode, &k.Screenshots, &k.HasToken, &rep, &res, &cmd, &k.Created, &k.Updated, &k.LastSeen, &k.LastIP); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(rep, &k.Report)
	_ = json.Unmarshal(res, &k.Result)
	if len(cmd) > 0 {
		k.Command = &kioskCommand{}
		if json.Unmarshal(cmd, k.Command) != nil {
			k.Command = nil
		}
	}
	return k, nil
}

func (k *kioskRow) json(scriptsVersion string) map[string]any {
	out := map[string]any{"id": k.ID, "name": k.Name, "monitors": k.Monitors, "server_url": k.ServerURL, "mesh_node": k.MeshNode, "screenshots": k.Screenshots,
		"set_up": k.HasToken, "report": k.Report, "result": k.Result, "created_at": k.Created, "updated_at": k.Updated, "last_seen_at": k.LastSeen, "last_ip": k.LastIP,
		"online": k.LastSeen != nil && time.Since(*k.LastSeen) < 90*time.Second, "scripts_version": scriptsVersion}
	if k.Command != nil && k.Command.Until.After(time.Now()) {
		out["command"] = k.Command
	}
	v, _ := k.Report["scripts_version"].(string)
	out["scripts_current"] = v != "" && v == scriptsVersion
	return out
}

func (s *Server) loadKiosk(ctx context.Context, org, id string) (*kioskRow, bool) {
	k, err := scanKiosk(s.pool.QueryRow(ctx, `SELECT `+kioskCols+` FROM core.kiosk_pc WHERE id::text = $1 AND org_id = $2::uuid AND revoked_at IS NULL`, id, org))
	return k, err == nil
}

// kioskFromToken authenticates the agent of a kiosk PC.
func (s *Server) kioskFromToken(r *http.Request) (*kioskRow, bool) {
	t := r.Header.Get(kioskTokenHeader)
	if !strings.HasPrefix(t, "c32k_") || len(t) > 100 {
		return nil, false
	}
	k, err := scanKiosk(s.pool.QueryRow(r.Context(), `SELECT `+kioskCols+` FROM core.kiosk_pc WHERE token_hash = $1 AND revoked_at IS NULL`, iam.HashToken(t)))
	if err != nil {
		s.log.Warn("invalid kiosk token", "security", true, "remote", clientIP(r))
		return nil, false
	}
	return k, true
}

// ---- the scripts ----------------------------------------------------------------------------------------------------

// validServerURL: the address of the server as the kiosk PC reaches it (often the local one, e.g. http://10.0.0.5:8080).
func validServerURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(raw) > 200 ||
		strings.ContainsAny(raw, "'\"`$ ") {
		return "", false
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host+u.Path, "/"), true
}

func (s *Server) kioskServerURL(k *kioskRow) string {
	if k.ServerURL != "" {
		return k.ServerURL
	}
	return strings.TrimRight(s.cfg.HTTP.BaseURL, "/")
}

// kioskScripts are the files the agent installs: the scripts as they are in the binary and the launcher settings of
// this PC. version changes with any of them.
func (s *Server) kioskScripts(k *kioskRow) (string, map[string]string) {
	files := map[string]string{"capture.ps1": captureStub}
	for _, n := range []string{"kiosk-agent.ps1", "kiosk-launcher.ps1"} {
		b, _ := kioskFS.ReadFile("kiosk/" + n)
		files[n] = string(b)
	}
	srv := s.kioskServerURL(k)
	origin := srv
	if u, err := url.Parse(srv); err == nil {
		origin = u.Scheme + "://" + u.Host
	}
	lj, _ := json.MarshalIndent(map[string]any{"url": srv + "/display", "origin": origin, "monitors": k.Monitors, "kiosk": k.ID, "name": k.Name,
		"cdp_port": kioskCDPPort}, "", "  ")
	files["launcher.json"] = string(lj)
	h := sha256.New()
	for _, n := range kioskScriptNames {
		h.Write([]byte(n + "\x00" + files[n] + "\x00"))
	}
	return hex.EncodeToString(h.Sum(nil))[:12], files
}

// psQuote makes a value safe inside a single-quoted PowerShell string.
func psQuote(s string) string { return strings.ReplaceAll(s, "'", "''") }

var shortNameRe = regexp.MustCompile(`[^a-z0-9]+`)

func (s *Server) kioskSetupScript(k *kioskRow, token string) (string, string) {
	b, _ := kioskFS.ReadFile("kiosk/setup-kiosk.ps1")
	short := strings.Trim(shortNameRe.ReplaceAllString(strings.ToLower(k.Name), "-"), "-")
	if short == "" || len(short) > 40 {
		short = k.ID[:8]
	}
	// the name goes into comments and single-quoted strings: ASCII only (Windows PowerShell reads the file as ANSI)
	name := strings.Map(func(r rune) rune {
		if r < 32 || r > 126 {
			return '?'
		}
		return r
	}, k.Name)
	text := strings.NewReplacer("__CTRL32_SERVER__", psQuote(s.kioskServerURL(k)), "__CTRL32_TOKEN__", psQuote(token), "__CTRL32_KIOSK_ID__", k.ID,
		"__CTRL32_NAME__", psQuote(name), "__CTRL32_SHORT__", short).Replace(string(b))
	return "setup-kiosk-" + short + ".ps1", strings.ReplaceAll(text, "\n", "\r\n")
}

// ---- operators ------------------------------------------------------------------------------------------------------

func (s *Server) listKiosks(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT `+kioskCols+` FROM core.kiosk_pc WHERE org_id = $1::uuid AND revoked_at IS NULL ORDER BY name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	var list []*kioskRow
	for rows.Next() {
		if k, err := scanKiosk(rows); err == nil {
			list = append(list, k)
		}
	}
	rows.Close()
	disp := map[string][]map[string]any{}
	drows, err := s.pool.Query(r.Context(), `SELECT id::text, name, kiosk_pc_id::text, kiosk_monitor, last_seen_at FROM core.display
		WHERE org_id = $1::uuid AND revoked_at IS NULL AND kiosk_pc_id IS NOT NULL ORDER BY kiosk_monitor NULLS LAST, name`, org)
	if err == nil {
		for drows.Next() {
			var id, name, kid string
			var mon *int
			var seen *time.Time
			if drows.Scan(&id, &name, &kid, &mon, &seen) == nil {
				disp[kid] = append(disp[kid], map[string]any{"id": id, "name": name, "monitor": mon, "online": seen != nil && time.Since(*seen) < 90*time.Second})
			}
		}
		drows.Close()
	}
	out := []map[string]any{}
	for _, k := range list {
		v, _ := s.kioskScripts(k)
		j := k.json(v)
		j["displays"] = disp[k.ID]
		if j["displays"] == nil {
			j["displays"] = []map[string]any{}
		}
		out = append(out, j)
	}
	writeJSON(w, 200, out)
}

func newKioskToken() (string, error) {
	raw, err := iam.RandomToken(32)
	if err != nil {
		return "", err
	}
	return "c32k_" + raw, nil
}

func (s *Server) createKiosk(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		Name      string `json:"name"`
		Monitors  int    `json:"monitors"`
		ServerURL string `json:"server_url"`
		Reason    string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 80 {
		writeErr(w, 400, "name: 1 to 80 characters")
		return
	}
	if in.Monitors == 0 {
		in.Monitors = 1
	}
	if in.Monitors < 1 || in.Monitors > 8 {
		writeErr(w, 400, "monitors: 1 to 8")
		return
	}
	srv := ""
	if strings.TrimSpace(in.ServerURL) != "" {
		var ok bool
		if srv, ok = validServerURL(in.ServerURL); !ok {
			writeErr(w, 400, "server_url: the address of this server as the PC reaches it, e.g. http://10.0.0.5:8080")
			return
		}
	}
	tok, err := newKioskToken()
	if err != nil {
		writeErr(w, 500, "random failed")
		return
	}
	p := PrincipalFrom(r.Context())
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.kiosk_pc (org_id, name, monitors, server_url, token_hash, created_by) VALUES ($1::uuid, $2, $3, $4, $5, NULLIF($6,'')::uuid) RETURNING id::text`,
		org, in.Name, in.Monitors, srv, iam.HashToken(tok), p.UserID).Scan(&id); err != nil {
		writeErr(w, 500, "not created")
		return
	}
	s.audit(r, "kiosk.create", "kiosk_pc", id, nil, map[string]any{"name": in.Name, "monitors": in.Monitors, "server_url": srv}, in.Reason)
	k, _ := s.loadKiosk(r.Context(), org, id)
	v, _ := s.kioskScripts(k)
	writeJSON(w, 201, map[string]any{"kiosk": k.json(v), "token": tok})
}

func (s *Server) updateKiosk(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	k, ok := s.loadKiosk(r.Context(), org, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "kiosk PC not found")
		return
	}
	var in struct {
		Name        *string `json:"name"`
		Monitors    *int    `json:"monitors"`
		ServerURL   *string `json:"server_url"`
		MeshNode    *string `json:"mesh_node"`
		Screenshots *bool   `json:"screenshots"`
		Displays    *[]struct {
			ID      string `json:"id"`
			Monitor int    `json:"monitor"`
		} `json:"displays"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	old := map[string]any{"name": k.Name, "monitors": k.Monitors, "server_url": k.ServerURL, "mesh_node": k.MeshNode, "screenshots": k.Screenshots}
	if in.Name != nil {
		k.Name = strings.TrimSpace(*in.Name)
		if k.Name == "" || len([]rune(k.Name)) > 80 {
			writeErr(w, 400, "name: 1 to 80 characters")
			return
		}
	}
	if in.Monitors != nil {
		if *in.Monitors < 1 || *in.Monitors > 8 {
			writeErr(w, 400, "monitors: 1 to 8")
			return
		}
		k.Monitors = *in.Monitors
	}
	if in.ServerURL != nil {
		k.ServerURL = ""
		if strings.TrimSpace(*in.ServerURL) != "" {
			var ok bool
			if k.ServerURL, ok = validServerURL(*in.ServerURL); !ok {
				writeErr(w, 400, "server_url: the address of this server as the PC reaches it, e.g. http://10.0.0.5:8080")
				return
			}
		}
	}
	if in.MeshNode != nil {
		n, ok := normalizeMeshNode(*in.MeshNode)
		if !ok {
			writeErr(w, 400, "mesh_node: the node id of this PC in MeshCentral")
			return
		}
		k.MeshNode = n
	}
	if in.Screenshots != nil {
		// allowing screenshots is for those who may see them (they can show unmasked camera pictures)
		if *in.Screenshots && !k.Screenshots && !PrincipalFrom(r.Context()).Has("display.screenshot") {
			writeErr(w, 403, "permission denied: display.screenshot (needed to allow screenshots of a kiosk PC)")
			return
		}
		k.Screenshots = *in.Screenshots
		if !k.Screenshots {
			dropShot(k.ID)
		}
	}
	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE core.kiosk_pc SET name = $2, monitors = $3, server_url = $4, mesh_node = $5, screenshots = $6, updated_at = now() WHERE id = $1::uuid`,
		k.ID, k.Name, k.Monitors, k.ServerURL, k.MeshNode, k.Screenshots); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	newVals := map[string]any{"name": k.Name, "monitors": k.Monitors, "server_url": k.ServerURL, "mesh_node": k.MeshNode, "screenshots": k.Screenshots}
	if in.Displays != nil {
		// the displays on this PC's monitors: one display per monitor, displays of this organization only
		seenMon := map[int]bool{}
		ids := []string{}
		for _, d := range *in.Displays {
			if d.Monitor < 0 || d.Monitor >= k.Monitors || seenMon[d.Monitor] || slices.Contains(ids, d.ID) {
				writeErr(w, 400, "displays: one display per monitor (0 to monitors − 1)")
				return
			}
			seenMon[d.Monitor] = true
			ids = append(ids, d.ID)
		}
		if _, err := tx.Exec(ctx, `UPDATE core.display SET kiosk_pc_id = NULL, kiosk_monitor = NULL WHERE kiosk_pc_id = $1::uuid`, k.ID); err != nil {
			writeErr(w, 500, "update failed")
			return
		}
		for _, d := range *in.Displays {
			tag, err := tx.Exec(ctx, `UPDATE core.display SET kiosk_pc_id = $2::uuid, kiosk_monitor = $3 WHERE id::text = $1 AND org_id = $4::uuid AND revoked_at IS NULL`, d.ID, k.ID, d.Monitor, org)
			if err != nil || tag.RowsAffected() != 1 {
				writeErr(w, 400, "display not found: "+d.ID)
				return
			}
		}
		newVals["displays"] = *in.Displays
	}
	if err := tx.Commit(ctx); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	s.audit(r, "kiosk.update", "kiosk_pc", k.ID, old, newVals, in.Reason)
	k, _ = s.loadKiosk(ctx, org, k.ID)
	v, _ := s.kioskScripts(k)
	writeJSON(w, 200, k.json(v))
}

func (s *Server) deleteKiosk(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	k, ok := s.loadKiosk(r.Context(), org, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "kiosk PC not found")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.kiosk_pc SET revoked_at = now(), token_hash = NULL, command = NULL, updated_at = now() WHERE id = $1::uuid`, k.ID); err != nil {
		writeErr(w, 500, "remove failed")
		return
	}
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.display SET kiosk_pc_id = NULL, kiosk_monitor = NULL WHERE kiosk_pc_id = $1::uuid`, k.ID)
	dropShot(k.ID)
	s.audit(r, "kiosk.remove", "kiosk_pc", k.ID, map[string]any{"name": k.Name, "last_ip": k.LastIP}, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) kioskToken(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct{ Reason string }
	if err := decodeJSON(r, &in, 1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	k, ok := s.loadKiosk(r.Context(), org, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "kiosk PC not found")
		return
	}
	tok, err := newKioskToken()
	if err != nil {
		writeErr(w, 500, "random failed")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.kiosk_pc SET token_hash = $2, updated_at = now() WHERE id = $1::uuid`, k.ID, iam.HashToken(tok)); err != nil {
		writeErr(w, 500, "not stored")
		return
	}
	s.audit(r, "kiosk.token", "kiosk_pc", k.ID, nil, map[string]any{"name": k.Name, "replaced": k.HasToken}, in.Reason)
	writeJSON(w, 200, map[string]string{"token": tok})
}

// kioskSetup returns the setup script with the credential the caller holds (shown once when the PC was registered or
// its credential replaced); the server keeps only the hash, so it checks the credential against it.
func (s *Server) kioskSetup(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct{ Token string }
	if err := decodeJSON(r, &in, 1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	k, ok := s.loadKiosk(r.Context(), org, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "kiosk PC not found")
		return
	}
	var match bool
	_ = s.pool.QueryRow(r.Context(), `SELECT token_hash = $2 FROM core.kiosk_pc WHERE id = $1::uuid`, k.ID, iam.HashToken(in.Token)).Scan(&match)
	if !strings.HasPrefix(in.Token, "c32k_") || !match {
		writeErr(w, 400, "token: the credential shown when the PC was registered (or a new one: Replace credential)")
		return
	}
	name, text := s.kioskSetupScript(k, in.Token)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, text)
}

// ---- operations -------------------------------------------------------------------------------------------------------

func (s *Server) commandKiosk(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct{ Action, Reason string }
	if err := decodeJSON(r, &in, 1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !slices.Contains(kioskActions, in.Action) {
		writeErr(w, 400, "action: "+strings.Join(kioskActions, ", "))
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	k, ok := s.loadKiosk(r.Context(), org, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "kiosk PC not found")
		return
	}
	p := PrincipalFrom(r.Context())
	if in.Action == "screenshot" {
		if !p.Has("display.screenshot") {
			writeErr(w, 403, "permission denied: display.screenshot")
			return
		}
		if !k.Screenshots {
			writeJSON(w, 409, map[string]string{"error": "screenshots are off for this PC; an administrator turns them on (Edit, with a reason)", "code": "screenshots_off"})
			return
		}
	}
	now := time.Now()
	// milliseconds: exact as a number in PowerShell and JavaScript alike
	c := kioskCommand{Seq: now.UnixMilli(), Action: in.Action, By: p.Username, Until: now.Add(10 * time.Minute)}
	raw, _ := json.Marshal(c)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.kiosk_pc SET command = $2::jsonb WHERE id = $1::uuid`, k.ID, string(raw)); err != nil {
		writeErr(w, 500, "not stored")
		return
	}
	if in.Action == "screenshot" {
		pendingShot(k.ID, c.Seq, p.UserID, k.Monitors)
	}
	s.audit(r, "kiosk.command", "kiosk_pc", k.ID, nil, map[string]any{"action": in.Action, "kiosk": k.Name, "seq": c.Seq}, in.Reason)
	writeJSON(w, 200, c)
}

// screenshots in memory: asked for by one user, delivered by the agent one picture per monitor, fetched once
var shots = struct {
	sync.Mutex
	m map[string]*shot
}{m: map[string]*shot{}}

type shot struct {
	seq   int64
	user  string
	asked time.Time
	want  int               // the monitors of the PC
	imgs  map[int]shotImage // by monitor (0-based)
	done  bool              // the agent reported the result of the operation
	msg   string            // …and its message
	at    time.Time         // the first picture or the result arrived
}

type shotImage struct {
	data  []byte
	ctype string
}

// complete: every monitor sent its picture, or the agent has finished (some monitors may have failed)
func (x *shot) complete() bool { return x.done || len(x.imgs) >= x.want }

func pendingShot(kiosk string, seq int64, user string, monitors int) {
	shots.Lock()
	defer shots.Unlock()
	shots.m[kiosk] = &shot{seq: seq, user: user, asked: time.Now(), want: max(monitors, 1), imgs: map[int]shotImage{}}
}

// shotDone records the agent's result of a screenshot operation.
func shotDone(kiosk string, seq int64, msg string) {
	shots.Lock()
	defer shots.Unlock()
	if x := shots.m[kiosk]; x != nil && x.seq == seq && !x.done {
		x.done, x.msg = true, msg
		if x.at.IsZero() {
			x.at = time.Now()
		}
	}
}

func dropShot(kiosk string) {
	shots.Lock()
	delete(shots.m, kiosk)
	shots.Unlock()
}

func expireShots() {
	for id, x := range shots.m {
		if time.Since(x.asked) > shotTTL+7*time.Minute || (!x.at.IsZero() && time.Since(x.at) > shotTTL) {
			delete(shots.m, id)
		}
	}
}

func (s *Server) kioskScreenshot(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	k, ok := s.loadKiosk(r.Context(), org, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "kiosk PC not found")
		return
	}
	p := PrincipalFrom(r.Context())
	w.Header().Set("Cache-Control", "no-store")
	shots.Lock()
	expireShots()
	x := shots.m[k.ID]
	if x == nil || x.user != p.UserID {
		shots.Unlock()
		writeErr(w, 404, "no screenshot asked for by you")
		return
	}
	if !x.complete() {
		got := len(x.imgs)
		shots.Unlock()
		writeJSON(w, 202, map[string]any{"waiting": true, "asked_at": x.asked, "received": got, "monitors": x.want})
		return
	}
	delete(shots.m, k.ID) // fetched once; never stored
	shots.Unlock()
	if len(x.imgs) == 0 {
		writeJSON(w, 502, map[string]string{"error": "the PC took no screenshot: " + x.msg, "code": "capture_failed"})
		return
	}
	mons := make([]int, 0, len(x.imgs))
	for m := range x.imgs {
		mons = append(mons, m)
	}
	slices.Sort(mons)
	out := make([]map[string]any, 0, len(mons))
	for _, m := range mons {
		out = append(out, map[string]any{"monitor": m, "content_type": x.imgs[m].ctype, "image": x.imgs[m].data}) // []byte → base64
	}
	writeJSON(w, 200, map[string]any{"taken_at": x.at, "message": x.msg, "monitors": out})
}

func (s *Server) kioskRemote(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct{ Reason string }
	if err := decodeJSON(r, &in, 1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	k, ok := s.loadKiosk(r.Context(), org, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "kiosk PC not found")
		return
	}
	st := s.loadRemoteSettings(r.Context())
	if st.MeshURL == "" || k.MeshNode == "" {
		writeJSON(w, 409, map[string]string{"error": "the MeshCentral address (a server setting) and the node of this PC (Edit) are needed", "code": "not_configured"})
		return
	}
	link := meshLink(st.MeshURL, k.MeshNode)
	s.audit(r, "remote.mesh_open", "kiosk_pc", k.ID, nil, map[string]any{"target": "kiosk", "kiosk": k.Name, "url": link}, in.Reason)
	writeJSON(w, 200, map[string]string{"url": link})
}

// ---- the agent on the PC ----------------------------------------------------------------------------------------------

func (s *Server) kioskReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	k, ok := s.kioskFromToken(r)
	if !ok {
		writeErr(w, 401, "unknown kiosk credential")
		return
	}
	var in struct {
		Report map[string]any  `json:"report"`
		Result json.RawMessage `json:"result"`
	}
	if err := decodeJSON(r, &in, 64<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	rep, _ := json.Marshal(in.Report)
	if len(rep) > 48<<10 {
		rep = []byte(`{"error":"report too large"}`)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE core.kiosk_pc SET report = $2::jsonb, last_seen_at = now(), last_ip = $3 WHERE id = $1::uuid`, k.ID, string(rep), clientIP(r)); err != nil {
		writeErr(w, 500, "not stored")
		return
	}
	if len(in.Result) > 0 && string(in.Result) != "null" {
		var res struct {
			Seq     json.Number `json:"seq"`
			Action  string      `json:"action"`
			OK      bool        `json:"ok"`
			Message string      `json:"message"`
		}
		d := json.NewDecoder(bytes.NewReader(in.Result))
		d.UseNumber()
		if d.Decode(&res) == nil {
			seq, _ := res.Seq.Int64()
			raw, _ := json.Marshal(map[string]any{"seq": seq, "action": truncateStr(res.Action, 40), "ok": res.OK, "message": truncateStr(res.Message, 500), "at": time.Now()})
			// the pending operation is done when its result arrives
			_, _ = s.pool.Exec(ctx, `UPDATE core.kiosk_pc SET result = $2::jsonb, command = CASE WHEN (command->>'seq')::bigint = $3 THEN NULL ELSE command END WHERE id = $1::uuid`,
				k.ID, string(raw), seq)
			if k.Command != nil && k.Command.Seq == seq {
				k.Command = nil
			}
			if res.Action == "screenshot" {
				shotDone(k.ID, seq, truncateStr(res.Message, 500))
			}
			if res.Action == "reboot" || !res.OK {
				s.log.Info("kiosk operation result", "kiosk", k.Name, "action", res.Action, "ok", res.OK, "message", truncateStr(res.Message, 200))
			}
		}
	}
	v, _ := s.kioskScripts(k)
	out := map[string]any{"poll_s": kioskPoll, "scripts_version": v, "kiosk": map[string]any{"id": k.ID, "name": k.Name, "monitors": k.Monitors}}
	if k.Command != nil && k.Command.Until.After(time.Now()) {
		out["command"] = k.Command
	}
	writeJSON(w, 200, out)
}

func (s *Server) kioskScriptsGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	k, ok := s.kioskFromToken(r)
	if !ok {
		writeErr(w, 401, "unknown kiosk credential")
		return
	}
	v, files := s.kioskScripts(k)
	writeJSON(w, 200, map[string]any{"version": v, "files": files})
}

func (s *Server) kioskScreenshotUpload(w http.ResponseWriter, r *http.Request) {
	k, ok := s.kioskFromToken(r)
	if !ok {
		writeErr(w, 401, "unknown kiosk credential")
		return
	}
	seq, _ := strconv.ParseInt(r.URL.Query().Get("seq"), 10, 64)
	monitor := 0 // agents before 0.86.5 send one picture without a monitor
	if v := r.URL.Query().Get("monitor"); v != "" {
		m, err := strconv.Atoi(v)
		if err != nil || m < 0 || m >= max(k.Monitors, 1) {
			writeErr(w, 400, "monitor: 0 to the number of monitors of the PC minus 1")
			return
		}
		monitor = m
	}
	ct := r.Header.Get("Content-Type")
	if ct != "image/jpeg" && ct != "image/png" {
		writeErr(w, 415, "image/jpeg or image/png")
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxShotBytes))
	if err != nil || len(b) < 8 {
		writeErr(w, 413, "the picture is larger than 8 MB or empty")
		return
	}
	if (ct == "image/jpeg" && !bytes.HasPrefix(b, []byte{0xFF, 0xD8})) || (ct == "image/png" && !bytes.HasPrefix(b, []byte("\x89PNG"))) {
		writeErr(w, 400, "not a picture")
		return
	}
	shots.Lock()
	defer shots.Unlock()
	expireShots()
	x := shots.m[k.ID]
	// only the pictures that were asked for, one per monitor, and only while the PC still allows screenshots
	if x == nil || x.seq != seq || x.complete() || !k.Screenshots {
		writeErr(w, 409, "no screenshot was asked for")
		return
	}
	if _, dup := x.imgs[monitor]; dup {
		writeErr(w, 409, "this monitor already sent its picture")
		return
	}
	x.imgs[monitor] = shotImage{data: b, ctype: ct}
	if x.at.IsZero() {
		x.at = time.Now()
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

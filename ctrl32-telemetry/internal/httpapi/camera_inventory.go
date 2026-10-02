package httpapi

// Camera inventory (spec 028): every setting of the cameras of an organization in one file — Excel, CSV or PDF — for
// the customer to keep, and the import of the same file to put the cameras back after a crash or on a new server.
//
//	GET  /api/v1/video/inventory?format=xlsx|csv|pdf             video.manage      without passwords
//	POST /api/v1/video/inventory/credentials                     video.manage + video.credentials_export
//	                                                             {format, reason, password, code}: with passwords
//	GET  /api/v1/video/inventory/template?format=xlsx|csv        video.manage      empty file with examples
//	POST /api/v1/video/inventory/import                          video.manage      multipart: file, update, create,
//	                                                             apply, reason — a preview, or applied at once
//
// Passwords and relay secrets leave the server only with video.credentials_export, in the browser after the user
// confirmed their password (and authenticator code), with a reason, a few times a minute at most; never through an AI
// assistant or an API token; the file is marked CONFIDENTIAL; a PDF with them is not stored as a report. Every such
// export is audited (video.credentials_export). An import is checked completely first and applied in one
// transaction, all rows or none; it never deletes a camera; without a licence the cameras beyond the free limit are
// imported disabled. The command line (ctrl32-telemetry cameras export|import) runs the same code.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/audit"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/camerainv"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/report"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/video"
)

// Import limits.
const (
	inventoryMaxBytes = 2 << 20
	inventoryMaxRows  = 1000
)

type invQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// invCamera is a stored camera with what the inventory needs beyond cameraRow.
type invCamera struct {
	cameraRow
	removed bool
}

const invColumns = `id::text, name, location, url, sub_url, username, password_sealed, transport, record, retention_days, sort, enabled, source, push_secret_sealed,
	onvif_url, ptz_profile, event_pre_s, event_post_s, event_kinds, camera_group, privacy_masks, motion_detect, motion_sensitivity, motion_zones, snapshot_url, audio, audio_record, note, removed_at IS NOT NULL`

// inventoryCameras reads the cameras of an organization (removed ones too, for restoring them by id).
func inventoryCameras(ctx context.Context, q invQuerier, org string) ([]invCamera, error) {
	rows, err := q.Query(ctx, `SELECT `+invColumns+` FROM core.camera WHERE org_id = $1::uuid ORDER BY sort, name, id`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []invCamera
	for rows.Next() {
		var c invCamera
		if err := rows.Scan(&c.id, &c.name, &c.location, &c.url, &c.subURL, &c.username, &c.sealed, &c.transport, &c.record, &c.retention, &c.sort, &c.enabled, &c.source, &c.pushSealed,
			&c.onvifURL, &c.ptzProfile, &c.eventPre, &c.eventPost, &c.eventKinds, &c.group, &c.masks, &c.motion, &c.motionSens, &c.zones, &c.snapshotURL, &c.audio, &c.audioRecord, &c.note, &c.removed); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---- export ---------------------------------------------------------------------------------------------------------

// inventoryOf converts the cameras the principal may see; passwords only when withCredentials.
func (s *Server) inventoryOf(ctx context.Context, p *iam.Principal, org string, withCredentials bool) ([]camerainv.Camera, error) {
	cams, err := inventoryCameras(ctx, s.pool, org)
	if err != nil {
		return nil, err
	}
	pins := s.cameraPins(ctx, org)
	var out []camerainv.Camera
	for _, c := range cams {
		if c.removed || !p.SeesCameraGroup(c.group) {
			continue
		}
		out = append(out, s.inventoryCamera(c, pins[c.id], withCredentials))
	}
	return out, nil
}

// inventoryCamera is one stored camera as the inventory shows it.
func (s *Server) inventoryCamera(c invCamera, pins []string, withCredentials bool) camerainv.Camera {
	ip := camerainv.HostOf(c.url)
	if c.source == "push" || ip == "" {
		ip = camerainv.HostOf(c.onvifURL)
	}
	kinds := c.eventKinds
	if kinds == nil {
		kinds = []string{}
	}
	x := camerainv.Camera{ID: c.id, Name: c.name, Location: c.location, Group: c.group, Enabled: c.enabled, Connection: c.source,
		Make: camerainv.DetectMake(c.url, c.subURL, c.onvifURL), IP: ip, URL: c.url, SubURL: c.subURL, Transport: c.transport, Username: c.username,
		OnvifURL: c.onvifURL, PTZProfile: c.ptzProfile, SnapshotURL: c.snapshotURL, Recording: c.record, PreEvent: c.eventPre, PostEvent: c.eventPost,
		EventKinds: kinds, RetentionDays: c.retention, MotionDetect: c.motion, MotionSensitivity: c.motionSens, MotionZones: string(orEmpty(c.zones)),
		PrivacyMasks: string(orEmpty(c.masks)), Audio: c.audio, AudioRecord: c.audioRecord, Sort: c.sort, Note: c.note, FloorPlans: strings.Join(pins, "; ")}
	if s.video != nil {
		if st, ok := s.video.Status(c.id); ok {
			x.State = st.State
		} else if !c.enabled {
			x.State = "disabled"
		}
		if withCredentials {
			x.Password = s.video.OpenPassword(c.sealed)
			if c.source == "push" {
				x.RelaySecret = s.video.OpenPassword(c.pushSealed)
			}
		}
	}
	return x
}

// cameraPins names the dashboards (floor plans) with a pin of each camera: "Plan of the hall (90°)".
func (s *Server) cameraPins(ctx context.Context, org string) map[string][]string {
	out := map[string][]string{}
	rows, err := s.pool.Query(ctx, `SELECT name, spec FROM core.dashboard WHERE org_id = $1::uuid AND valid_to IS NULL AND spec::text LIKE '%camera_pin%' ORDER BY name`, org)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var raw []byte
		if rows.Scan(&name, &raw) != nil {
			continue
		}
		var spec struct {
			Widgets []struct {
				Type   string         `json:"type"`
				Params map[string]any `json:"params"`
			} `json:"widgets"`
		}
		if json.Unmarshal(raw, &spec) != nil {
			continue
		}
		for _, w := range spec.Widgets {
			id, _ := w.Params["camera"].(string)
			if w.Type != "camera_pin" || id == "" {
				continue
			}
			label := name
			if dir, ok := w.Params["direction"].(float64); ok {
				label += fmt.Sprintf(" (%.0f°)", dir)
			}
			out[id] = append(out[id], label)
		}
	}
	return out
}

// inventoryMeta is the header of an exported file.
func (s *Server) inventoryMeta(ctx context.Context, org, user string, confidential bool) (camerainv.Meta, string) {
	var name, slug, tz string
	_ = s.pool.QueryRow(ctx, `SELECT name, slug, tz FROM core.organization WHERE id = $1::uuid`, org).Scan(&name, &slug, &tz)
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	server := ""
	if s.cfg != nil {
		server = strings.TrimRight(s.cfg.HTTP.BaseURL, "/")
	}
	now := time.Now().In(loc)
	return camerainv.Meta{Org: name, Server: server, User: user, Version: s.version, ExportedAt: now.Format("2006-01-02 15:04 MST"), Confidential: confidential},
		fileSafe(slug) + "_" + now.Format("20060102-1504")
}

// InventoryFile is an exported inventory.
type InventoryFile struct {
	Name        string
	ContentType string
	Body        []byte
	Cameras     int
}

// inventoryFile builds the export in a format; PDFs with passwords are compiled without being stored.
func (s *Server) inventoryFile(ctx context.Context, p *iam.Principal, org, format string, withCredentials bool, logo []byte, logoName string) (*InventoryFile, *report.Data, error) {
	cams, err := s.inventoryOf(ctx, p, org, withCredentials)
	if err != nil {
		return nil, nil, err
	}
	m, base := s.inventoryMeta(ctx, org, p.Username, withCredentials)
	prefix := "cameras_"
	if withCredentials {
		prefix = "CONFIDENTIAL_cameras_"
	}
	f := &InventoryFile{Name: prefix + base + "." + format, Cameras: len(cams)}
	switch format {
	case "xlsx":
		f.ContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		f.Body, err = camerainv.XLSX(m, cams)
	case "csv":
		f.ContentType = "text/csv; charset=utf-8"
		f.Body, err = camerainv.CSV(m, cams)
	case "pdf":
		f.ContentType = "application/pdf"
		d := inventoryReport(m, cams)
		if logoName != "" && len(logo) > 0 {
			d.Logo = logoName
		}
		return f, &d, nil // rendered by the caller (stored or not)
	default:
		return nil, nil, errors.New("format: xlsx, csv or pdf")
	}
	return f, nil, err
}

// inventoryReport is the data of the PDF: one card per camera.
func inventoryReport(m camerainv.Meta, cams []camerainv.Camera) report.Data {
	keys := camerainv.Headers(m.Confidential)
	cols := make([]report.Column, 0, len(keys))
	for _, k := range keys {
		cols = append(cols, report.Column{Key: k, Label: k, Align: "left"})
	}
	rows := make([]map[string]any, 0, len(cams))
	enabled := 0
	for _, c := range cams {
		vals := c.Values(m.Confidential)
		row := map[string]any{}
		for i, k := range keys {
			v := vals[i]
			if (k == "motion_zones" || k == "privacy_masks") && v == "[]" {
				v = ""
			}
			row[k] = breakable(v)
		}
		rows = append(rows, row)
		if c.Enabled {
			enabled++
		}
	}
	sub := fmt.Sprintf("%s · %d cameras · %s", m.Org, len(cams), m.ExportedAt)
	return report.Data{Kind: "cameras", Title: "Camera inventory", Subtitle: sub, Org: m.Org, Locale: "en",
		Scope:   map[string]any{"cameras": len(cams), "confidential": m.Confidential, "server": m.Server},
		Summary: map[string]any{"cameras": len(cams), "enabled": enabled, "server": m.Server, "version": m.Version},
		Columns: cols, Rows: rows, System: map[string]any{"version": m.Version, "instance": m.Server}}
}

// breakable lets long addresses wrap in the PDF (a zero-width space after / ? & and ,).
func breakable(s string) string {
	if len(s) < 30 {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(r)
		if r == '/' || r == '?' || r == '&' || r == ',' {
			b.WriteRune('\u200b')
		}
	}
	return b.String()
}

func writeInventory(w http.ResponseWriter, f *InventoryFile) {
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, f.Name))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Cameras", fmt.Sprint(f.Cameras))
	_, _ = w.Write(f.Body)
}

// cameraInventory: the export without passwords.
func (s *Server) cameraInventory(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "xlsx"
	}
	if format == "pdf" && s.reportsUnavailable(w) {
		return
	}
	logo, logoName := s.orgLogo(r, org)
	f, d, err := s.inventoryFile(r.Context(), p, org, format, false, logo, logoName)
	if err != nil {
		if strings.HasPrefix(err.Error(), "format:") {
			writeErr(w, 400, err.Error())
			return
		}
		writeErr(w, 500, "export failed")
		return
	}
	if d != nil {
		assets := map[string][]byte{}
		if d.Logo != "" {
			assets[d.Logo] = logo
		}
		res, err := s.renderReport(r, p, *d, assets) // stored as a report and audited like other PDF exports
		if err != nil {
			writeErr(w, 500, "report failed: "+err.Error())
			return
		}
		servePDF(w, res.PDF, f.Name, res.Record.ID)
		return
	}
	s.audit(r, "export.read", "export", "", nil, map[string]any{"export": "camera_inventory", "format": format, "cameras": f.Cameras, "passwords": false}, "")
	writeInventory(w, f)
}

// cameraInventoryCredentials: the export with passwords and relay secrets.
func (s *Server) cameraInventoryCredentials(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	if !p.Has("video.manage") {
		writeErr(w, 403, "forbidden: video.manage is required as well")
		return
	}
	if p.TokenID != "" || p.GrantID != "" || r.Context().Value(viaMCPKey) != nil {
		writeErr(w, 403, "camera passwords are exported only by a person in the browser (or with the command line on the server), not with an API token or an AI assistant")
		return
	}
	var in struct {
		Format   string `json:"format"`
		Reason   string `json:"reason"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := decodeJSON(r, &in, 8<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.Format == "" {
		in.Format = "xlsx"
	}
	if in.Format != "xlsx" && in.Format != "csv" && in.Format != "pdf" {
		writeErr(w, 400, "format: xlsx, csv or pdf")
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if in.Format == "pdf" && s.reportsUnavailable(w) {
		return
	}
	if s.video == nil {
		writeErr(w, 409, "video is not available on this server")
		return
	}
	if !s.stepUp(w, r, in.Password, in.Code) {
		return
	}
	if !s.credExportLimiter.allow(p.UserID) {
		writeErr(w, 429, "too many exports with passwords; wait a minute")
		return
	}
	logo, logoName := s.orgLogo(r, org)
	f, d, err := s.inventoryFile(r.Context(), p, org, in.Format, true, logo, logoName)
	if err != nil {
		writeErr(w, 500, "export failed")
		return
	}
	if d != nil {
		assets := map[string][]byte{}
		if d.Logo != "" {
			assets[d.Logo] = logo
		}
		pdf, err := s.rep.RenderBuiltin(r.Context(), org, *d, assets, p.Username)
		if err != nil {
			writeErr(w, 500, "report failed: "+err.Error())
			return
		}
		f.Body = pdf
	}
	s.log.Warn("camera passwords exported", "security", true, "user", p.Username, "cameras", f.Cameras, "format", in.Format, "remote", clientIP(r))
	s.audit(r, "video.credentials_export", "camera_inventory", "", nil, map[string]any{"cameras": f.Cameras, "format": in.Format, "file": f.Name}, in.Reason)
	writeInventory(w, f)
}

// cameraInventoryTemplate: an empty file with the columns and two example rows.
func (s *Server) cameraInventoryTemplate(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "xlsx"
	}
	if format != "xlsx" && format != "csv" {
		writeErr(w, 400, "format: xlsx or csv")
		return
	}
	b, err := camerainv.Template(format)
	if err != nil {
		writeErr(w, 500, "template failed")
		return
	}
	ct := "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	if format == "csv" {
		ct = "text/csv; charset=utf-8"
	}
	writeInventory(w, &InventoryFile{Name: "cameras_template." + format, ContentType: ct, Body: b})
}

// ---- import ---------------------------------------------------------------------------------------------------------

// InventoryOptions choose what an import may do; it never deletes.
type InventoryOptions struct {
	Update bool // change existing cameras (matched by id, else by name)
	Create bool // add cameras that are not there
}

// invItem is the plan for one row of the file.
type invItem struct {
	Line     int      `json:"line"`
	Action   string   `json:"action"` // create, update, restore, unchanged, skip, error
	ID       string   `json:"id,omitempty"`
	Name     string   `json:"name"`
	IP       string   `json:"ip,omitempty"`
	Enabled  bool     `json:"enabled"`
	Changes  []string `json:"changes,omitempty"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`

	row           cameraRow
	old           *invCamera
	keepID        bool
	newSecret     bool
	pwChanged     bool // the camera password changes
	secretChanged bool // the relay secret changes
}

// InventoryPlan is the preview of an import (and, applied, its result).
type InventoryPlan struct {
	Format       string         `json:"format"`
	Sheet        string         `json:"sheet,omitempty"`
	Columns      []string       `json:"columns"`
	Ignored      []string       `json:"ignored"`
	Rows         []*invItem     `json:"rows"`
	Counts       map[string]int `json:"counts"`
	Licensed     bool           `json:"licensed"`
	FreeLimit    int            `json:"free_limit"`
	EnabledAfter int            `json:"enabled_after"` // enabled cameras of the server after the import
	Passwords    int            `json:"passwords"`     // rows that set a password
	Applied      bool           `json:"applied"`
}

// Errors counts the rows with errors.
func (p *InventoryPlan) Errors() int { return p.Counts["error"] }

func strPtr(s string) *string { return &s }

// planInventory checks every row against the stored cameras and decides what happens to it. q is the pool for a
// preview, the transaction (after the camera limit lock) when applying.
func (s *Server) planInventory(ctx context.Context, q invQuerier, p *iam.Principal, org string, f *camerainv.File, opt InventoryOptions) (*InventoryPlan, error) {
	cams, err := inventoryCameras(ctx, q, org)
	if err != nil {
		return nil, err
	}
	byID := map[string]*invCamera{}
	byName := map[string][]*invCamera{}
	for i := range cams {
		c := &cams[i]
		byID[c.id] = c
		if !c.removed {
			k := strings.ToLower(strings.TrimSpace(c.name))
			byName[k] = append(byName[k], c)
		}
	}
	// ids of the file that belong to another organization cannot be kept
	var foreign []string
	for _, r := range f.Rows {
		id := strings.ToLower(strings.TrimSpace(r.ID))
		if uuidRe.MatchString(id) && byID[id] == nil {
			foreign = append(foreign, id)
		}
	}
	taken := map[string]bool{}
	if len(foreign) > 0 {
		rows, err := q.Query(ctx, `SELECT id::text FROM core.camera WHERE id = ANY($1::uuid[])`, foreign)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				taken[id] = true
			}
		}
		rows.Close()
	}
	plan := &InventoryPlan{Format: f.Format, Sheet: f.Sheet, Columns: f.Columns, Ignored: f.Ignored, Counts: map[string]int{}, FreeLimit: FreeCameraLimit, Rows: []*invItem{}}
	if plan.Columns == nil {
		plan.Columns = []string{}
	}
	if plan.Ignored == nil {
		plan.Ignored = []string{}
	}
	seen := map[string]int{}
	for i := range f.Rows {
		it := s.planRow(p, &f.Rows[i], byID, byName, taken, opt)
		key := it.ID // the matched camera, or the id a new camera keeps
		if it.old == nil && !it.keepID {
			key = ""
		}
		if key != "" && it.Action != "error" {
			if line, dup := seen[key]; dup {
				it.Action, it.Errors = "error", append(it.Errors, fmt.Sprintf("the same camera as line %d", line))
			} else {
				seen[key] = it.Line
			}
		}
		plan.Rows = append(plan.Rows, it)
	}
	if err := s.planLicence(ctx, q, plan); err != nil {
		return nil, err
	}
	for _, it := range plan.Rows {
		plan.Counts[it.Action]++
		if it.pwChanged {
			plan.Passwords++
		}
	}
	return plan, nil
}

// planRow decides one row: match, fill in addresses, validate with the rules of the camera form.
func (s *Server) planRow(p *iam.Principal, r *camerainv.Row, byID map[string]*invCamera, byName map[string][]*invCamera, taken map[string]bool, opt InventoryOptions) *invItem {
	it := &invItem{Line: r.Line}
	if r.Name != nil {
		it.Name = *r.Name
	}
	fail := func(msg ...string) *invItem {
		it.Action = "error"
		it.Errors = append(it.Errors, msg...)
		return it
	}
	if len(r.Errors) > 0 {
		return fail(r.Errors...)
	}
	// match by id, else by name (and ip when several cameras have the name)
	id := strings.ToLower(strings.TrimSpace(r.ID))
	var old *invCamera
	switch {
	case id != "" && !uuidRe.MatchString(id):
		return fail("id: not a camera id (leave it empty for a new camera)")
	case id != "":
		old = byID[id]
		if old == nil {
			if taken[id] {
				it.Warnings = append(it.Warnings, "the id belongs to a camera of another organization: the camera gets a new id")
			} else {
				it.keepID = true
				it.ID = id
			}
		}
	case r.Name != nil:
		cands := byName[strings.ToLower(strings.TrimSpace(*r.Name))]
		if len(cands) > 1 && r.IP != nil && *r.IP != "" {
			var same []*invCamera
			for _, c := range cands {
				if strings.EqualFold(camerainv.HostOf(c.url), *r.IP) || strings.EqualFold(camerainv.HostOf(c.onvifURL), *r.IP) {
					same = append(same, c)
				}
			}
			cands = same
		}
		switch len(cands) {
		case 0:
		case 1:
			old = cands[0]
		default:
			return fail(fmt.Sprintf("%d cameras are called %q: add their id (or ip) to the row", len(cands), *r.Name))
		}
	default:
		return fail("name or id is required")
	}
	if old != nil {
		it.ID, it.Name = old.id, old.name
		if r.Name != nil {
			it.Name = *r.Name
		}
		if !p.SeesCameraGroup(old.group) {
			return fail("the camera is outside your camera groups")
		}
	}
	// build the form input from the cells
	in := cameraIn{Name: r.Name, Location: r.Location, Group: r.Group, Enabled: r.Enabled, Source: r.Connection, URL: r.URL, SubURL: r.SubURL,
		Username: r.Username, Transport: r.Transport, OnvifURL: r.OnvifURL, PTZProfile: r.PTZProfile, SnapshotURL: r.SnapshotURL, Record: r.Recording,
		EventPre: r.PreEvent, EventPost: r.PostEvent, EventKinds: r.EventKinds, RetentionDays: r.RetentionDays, MotionDetect: r.MotionDetect,
		MotionSensitivity: r.MotionSensitivity, MotionZones: r.MotionZones, PrivacyMasks: r.PrivacyMasks, Audio: r.Audio, AudioRecord: r.AudioRecord,
		Sort: r.Sort, Note: r.Note, Reason: "import"}
	if r.Password != nil && *r.Password != "" {
		in.Password = r.Password
	}
	c := cameraRow{transport: "auto", record: "continuous", retention: 7, enabled: true, source: "pull", eventPre: 10, eventPost: 20, motionSens: 50, audioRecord: true,
		eventKinds: []string{"motion", "tamper", "line", "intrusion", "input", "mark"}}
	if old != nil {
		c = old.cameraRow
		it.old = old
	}
	source := c.source
	if r.Connection != nil {
		source = *r.Connection
	}
	cur := func(in *string, stored string) string {
		if in != nil {
			return *in
		}
		return stored
	}
	// a make with an ip fills in the addresses of a camera without a main stream
	if r.Make != nil && *r.Make != "" && *r.Make != "other" {
		pr, ok := camerainv.PresetOf(*r.Make)
		if !ok {
			return fail("make: tapo, hikvision, dahua, reolink, axis, uniview, ezviz or other")
		}
		if source == "pull" && cur(r.URL, c.url) == "" {
			if r.IP == nil || *r.IP == "" {
				return fail("make " + *r.Make + " needs the ip (or main_stream_url)")
			}
			main, sub, onvif, snap := pr.Addresses(*r.IP)
			in.URL = &main
			if r.SubURL == nil && sub != "" {
				in.SubURL = &sub
			}
			if r.OnvifURL == nil && onvif != "" {
				in.OnvifURL = &onvif
			}
			if r.SnapshotURL == nil && snap != "" {
				in.SnapshotURL = &snap
			}
			it.Warnings = append(it.Warnings, "addresses filled in for "+pr.Name+" at "+*r.IP)
		}
	}
	// a different ip moves every address that used the old one
	if r.IP != nil && *r.IP != "" {
		if !camerainv.ValidHost(*r.IP) {
			return fail("ip: an IP address or host name")
		}
		base := cur(in.URL, c.url)
		if source == "push" || base == "" {
			base = cur(in.OnvifURL, c.onvifURL)
		}
		if oldHost := camerainv.HostOf(base); oldHost != "" && !strings.EqualFold(oldHost, *r.IP) {
			moved := false
			for _, f := range []struct {
				in     **string
				stored string
			}{{&in.URL, c.url}, {&in.SubURL, c.subURL}, {&in.OnvifURL, c.onvifURL}, {&in.SnapshotURL, c.snapshotURL}} {
				if v, ok := camerainv.MoveHost(cur(*f.in, f.stored), oldHost, *r.IP); ok {
					*f.in = strPtr(v)
					moved = true
				}
			}
			if moved {
				it.Warnings = append(it.Warnings, "ip "+oldHost+" → "+*r.IP+": the addresses of the camera moved")
			}
		}
		it.IP = *r.IP
	}
	var pw *string
	if msg := s.applyCamera(&in, &c, &pw); msg != "" {
		return fail(msg)
	}
	if !p.SeesCameraGroup(c.group) {
		return fail("group: only the camera groups you may see")
	}
	if it.IP == "" {
		it.IP = camerainv.HostOf(c.url)
		if it.IP == "" {
			it.IP = camerainv.HostOf(c.onvifURL)
		}
	}
	// passwords: an empty cell keeps the stored one; the same one is no change
	if pw != nil {
		if s.video == nil {
			return fail("passwords cannot be stored: video is not available")
		}
		if old == nil || s.video.OpenPassword(old.sealed) != *pw {
			sealed, err := s.video.SealPassword(*pw)
			if err != nil {
				return fail("password not stored: " + err.Error())
			}
			c.sealed = sealed
			it.pwChanged = true
		}
	}
	// a push camera keeps its relay secret, takes the one in the file, or gets a new one
	if c.source == "push" {
		secret := ""
		if r.RelaySecret != nil {
			secret = strings.TrimSpace(*r.RelaySecret)
		}
		switch {
		case secret != "" && (len(secret) < 16 || len(secret) > 200):
			return fail("relay_secret: 16 to 200 characters")
		case secret != "":
			if s.video == nil {
				return fail("relay secrets cannot be stored: video is not available")
			}
			if old == nil || s.video.OpenPassword(old.pushSealed) != secret {
				c.pushSealed, _ = s.video.SealPassword(secret)
				it.secretChanged = true
			}
		case old == nil || old.source != "push" || old.pushSealed == "":
			if s.video == nil {
				return fail("relay secrets cannot be made: video is not available")
			}
			c.pushSealed, _ = s.video.SealPassword(video.NewPushSecret())
			it.newSecret, it.secretChanged = true, true
			it.Warnings = append(it.Warnings, "a new relay secret: open the camera in Camera management and choose New secret to get the relay configuration")
		}
	} else {
		c.pushSealed = ""
	}
	it.row = c
	it.Name = c.name
	switch {
	case old == nil && !opt.Create:
		it.Action = "skip"
		it.Warnings = append(it.Warnings, "a new camera; adding cameras is off")
	case old == nil:
		it.Action = "create"
	case old.removed && !opt.Create:
		it.Action = "skip"
		it.Warnings = append(it.Warnings, "a removed camera; adding cameras is off")
	case old.removed:
		it.Action = "restore"
	case !opt.Update:
		it.Action = "skip"
		it.Warnings = append(it.Warnings, "an existing camera; updating is off")
	default:
		it.Changes = cameraChanges(old.cameraRow, c, it.pwChanged, it.secretChanged)
		it.Action = "update"
		if len(it.Changes) == 0 {
			it.Action = "unchanged"
		}
	}
	it.Enabled = c.enabled
	return it
}

// cameraChanges lists the settings that differ (passwords are compared by the caller).
func cameraChanges(a, b cameraRow, password, secret bool) []string {
	x, y := cameraAudit(a), cameraAudit(b)
	var out []string
	for k, v := range y {
		if k == "password_set" {
			continue
		}
		if !reflect.DeepEqual(normJSON(x[k]), normJSON(v)) {
			out = append(out, k)
		}
	}
	if password {
		out = append(out, "password")
	}
	if secret {
		out = append(out, "relay_secret")
	}
	slices.Sort(out)
	return out
}

// normJSON compares JSON columns by value, not by spelling.
func normJSON(v any) any {
	switch t := v.(type) {
	case json.RawMessage:
		var x any
		if json.Unmarshal(t, &x) == nil {
			return x
		}
	case []string:
		if t == nil {
			return []string{}
		}
	}
	return v
}

// planLicence applies the camera limit of a server without a licence: cameras that are enabled stay enabled, newly
// enabled ones take the free places in file order, the rest are imported disabled.
func (s *Server) planLicence(ctx context.Context, q invQuerier, plan *InventoryPlan) error {
	plan.Licensed = s.serverLicensed(ctx)
	touched := []string{} // not nil: NOT (id = ANY(NULL)) would count nothing
	for _, it := range plan.Rows {
		if it.old != nil && (it.Action == "update" || it.Action == "restore" || it.Action == "unchanged") {
			touched = append(touched, it.old.id)
		}
	}
	var n int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM core.camera WHERE enabled AND removed_at IS NULL AND NOT (id::text = ANY($1::text[]))`, touched).Scan(&n); err != nil {
		return err
	}
	applyCameraLimit(plan, n)
	return nil
}

// applyCameraLimit counts the enabled cameras after the import; others are the enabled cameras of the server that the
// file does not touch.
func applyCameraLimit(plan *InventoryPlan, others int) {
	n := others
	active := func(it *invItem) bool {
		return it.Action == "create" || it.Action == "update" || it.Action == "restore" || it.Action == "unchanged"
	}
	for _, it := range plan.Rows {
		if active(it) && it.row.enabled && it.old != nil && it.old.enabled && !it.old.removed {
			n++ // enabled cameras keep running
		}
	}
	for _, it := range plan.Rows {
		if !active(it) || !it.row.enabled || (it.old != nil && it.old.enabled && !it.old.removed) {
			continue
		}
		if plan.Licensed || n < FreeCameraLimit {
			n++
			continue
		}
		it.row.enabled, it.Enabled = false, false
		plan.Counts["disabled_by_licence"]++
		it.Warnings = append(it.Warnings, fmt.Sprintf("imported disabled: without a licence this server runs up to %d cameras (System → License)", FreeCameraLimit))
		if it.Action == "update" {
			it.Changes = cameraChanges(it.old.cameraRow, it.row, it.pwChanged, it.secretChanged)
			if len(it.Changes) == 0 {
				it.Action = "unchanged"
			}
		}
	}
	plan.EnabledAfter = n
}

// applyInventory stores the plan in one transaction: all rows or none.
func (s *Server) applyInventory(ctx context.Context, p *iam.Principal, org string, f *camerainv.File, opt InventoryOptions, reason string, record auditFunc) (*InventoryPlan, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// one import or camera change at a time decides the free camera places
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('ctrl32.camera_limit'))`); err != nil {
		return nil, err
	}
	plan, err := s.planInventory(ctx, tx, p, org, f, opt)
	if err != nil {
		return nil, err
	}
	if n := plan.Errors(); n > 0 {
		return plan, &inventoryRowsError{n}
	}
	by := p.Username
	for _, it := range plan.Rows {
		c := &it.row
		switch it.Action {
		case "create":
			id := ""
			if it.keepID {
				id = it.ID
			}
			err = tx.QueryRow(ctx, `INSERT INTO core.camera (id, org_id, name, location, url, sub_url, username, password_sealed, transport, record, retention_days, enabled, sort, created_by, source, push_secret_sealed,
				onvif_url, ptz_profile, event_pre_s, event_post_s, event_kinds, camera_group, privacy_masks, motion_detect, motion_sensitivity, motion_zones, snapshot_url, audio, audio_record, note)
				VALUES (coalesce(nullif($1, '')::uuid, uuidv7()), $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23::jsonb, $24, $25, $26::jsonb, $27, $28, $29, $30) RETURNING id::text`,
				id, org, c.name, c.location, c.url, c.subURL, c.username, c.sealed, c.transport, c.record, c.retention, c.enabled, c.sort, by, c.source, c.pushSealed,
				c.onvifURL, c.ptzProfile, c.eventPre, c.eventPost, c.eventKinds, c.group, string(orEmpty(c.masks)), c.motion, c.motionSens, string(orEmpty(c.zones)), c.snapshotURL, c.audio, c.audioRecord, c.note).Scan(&c.id)
			it.ID = c.id
		case "update", "restore":
			_, err = tx.Exec(ctx, `UPDATE core.camera SET name = $3, location = $4, url = $5, sub_url = $6, username = $7, password_sealed = $8, transport = $9, record = $10, retention_days = $11,
				enabled = $12, sort = $13, source = $14, push_secret_sealed = $15, onvif_url = $16, ptz_profile = $17, event_pre_s = $18, event_post_s = $19, event_kinds = $20, camera_group = $21,
				privacy_masks = $22::jsonb, motion_detect = $23, motion_sensitivity = $24, motion_zones = $25::jsonb, snapshot_url = $26, audio = $27, audio_record = $28, note = $29,
				removed_at = NULL, updated_at = now() WHERE id = $1::uuid AND org_id = $2::uuid`,
				c.id, org, c.name, c.location, c.url, c.subURL, c.username, c.sealed, c.transport, c.record, c.retention, c.enabled, c.sort, c.source, c.pushSealed,
				c.onvifURL, c.ptzProfile, c.eventPre, c.eventPost, c.eventKinds, c.group, string(orEmpty(c.masks)), c.motion, c.motionSens, string(orEmpty(c.zones)), c.snapshotURL, c.audio, c.audioRecord, c.note)
		}
		if err != nil {
			return nil, fmt.Errorf("line %d (%s): %w", it.Line, it.Name, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	plan.Applied = true
	why := "import: " + reason
	var list []map[string]any
	for _, it := range plan.Rows {
		switch it.Action {
		case "create":
			record("camera.create", "camera", it.ID, nil, cameraAudit(it.row), why)
		case "update", "restore":
			record("camera."+it.Action, "camera", it.ID, cameraAudit(it.old.cameraRow), cameraAudit(it.row), why)
		default:
			continue
		}
		list = append(list, map[string]any{"id": it.ID, "name": it.Name, "action": it.Action, "enabled": it.Enabled})
		if s.video != nil {
			s.video.Reload(context.Background(), it.ID)
		}
	}
	record("video.import", "camera_inventory", "", nil, map[string]any{"format": plan.Format, "rows": len(plan.Rows), "created": plan.Counts["create"], "updated": plan.Counts["update"],
		"restored": plan.Counts["restore"], "unchanged": plan.Counts["unchanged"], "skipped": plan.Counts["skip"], "disabled_by_licence": plan.Counts["disabled_by_licence"],
		"passwords": plan.Passwords, "update_existing": opt.Update, "create_missing": opt.Create, "cameras": list}, reason)
	return plan, nil
}

type inventoryRowsError struct{ n int }

func (e *inventoryRowsError) Error() string {
	return fmt.Sprintf("%d rows have errors: correct them or remove the rows; nothing was changed", e.n)
}

// auditFunc writes an audit entry of the current actor (a request, or the command line).
type auditFunc func(action, entityType, entityID string, old, new any, reason string)

// cameraInventoryImport: POST multipart file (+ update, create, apply, reason).
func (s *Server) cameraInventoryImport(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, inventoryMaxBytes+64<<10)
	if err := r.ParseMultipartForm(inventoryMaxBytes); err != nil {
		writeErr(w, 400, "multipart form with the file (at most 2 MB)")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "file is required (Excel .xlsx or CSV)")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, inventoryMaxBytes+1))
	if err != nil || len(data) > inventoryMaxBytes {
		writeErr(w, 400, "the file is larger than 2 MB")
		return
	}
	flag := func(k string, def bool) bool {
		v := r.FormValue(k)
		if v == "" {
			return def
		}
		b, ok := camerainv.ParseBool(v)
		return ok && b
	}
	opt := InventoryOptions{Update: flag("update", true), Create: flag("create", true)}
	f, err := camerainv.Parse(data, inventoryMaxRows)
	if err != nil {
		writeErr(w, 400, hdr.Filename+": "+err.Error())
		return
	}
	p := PrincipalFrom(r.Context())
	if !flag("apply", false) {
		plan, err := s.planInventory(r.Context(), s.pool, p, org, f, opt)
		if err != nil {
			writeErr(w, 500, "import check failed")
			return
		}
		writeJSON(w, 200, plan)
		return
	}
	reason := r.FormValue("reason")
	if !reasonOf(w, reason) {
		return
	}
	plan, err := s.applyInventory(r.Context(), p, org, f, opt, reason, func(a, et, id string, o, n any, rs string) { s.audit(r, a, et, id, o, n, rs) })
	var re *inventoryRowsError
	switch {
	case errors.As(err, &re):
		writeJSON(w, 422, map[string]any{"error": re.Error(), "code": "rows_invalid", "plan": plan})
		return
	case err != nil:
		s.log.Error("camera import failed", "err", err)
		writeErr(w, 500, "import failed, nothing was changed: "+err.Error())
		return
	}
	writeJSON(w, 200, plan)
}

// ---- the command line -----------------------------------------------------------------------------------------------

// Inventory runs the camera export and import without the web server: `ctrl32-telemetry cameras export|import` for
// when the web interface is not available (disaster recovery).
type Inventory struct{ s *Server }

// NewInventory prepares the export and import over the database; vm only seals and opens passwords (it is not run);
// rep may be nil (no PDF).
func NewInventory(cfg *config.Config, pool *pgxpool.Pool, vm *video.Manager, rep *report.Engine, log *slog.Logger, version string) *Inventory {
	return &Inventory{s: &Server{cfg: cfg, pool: pool, video: vm, rep: rep, log: log, version: version}}
}

// cliPrincipal is the operator at the server's command line: all cameras of the organization.
func cliPrincipal(actor string) *iam.Principal { return &iam.Principal{Username: actor} }

func (inv *Inventory) audit(ctx context.Context, org, actor string) auditFunc {
	return func(action, et, id string, o, n any, reason string) {
		if id != "" && !uuidRe.MatchString(id) {
			et, id = et+":"+id, ""
		}
		e := audit.Entry{OrgID: org, ActorType: "system", ActorName: actor, Action: action, EntityType: et, EntityID: id, Old: o, New: n, Reason: reason}
		if _, _, err := audit.Append(ctx, inv.s.pool, e); err != nil {
			inv.s.log.Error("audit append failed", "action", action, "err", err)
		}
	}
}

// Export writes the inventory of an organization; with passwords it is audited (video.credentials_export).
func (inv *Inventory) Export(ctx context.Context, org, format, actor, reason string, withCredentials bool) (*InventoryFile, error) {
	if withCredentials && strings.TrimSpace(reason) == "" {
		return nil, errors.New("a reason is required to export passwords")
	}
	p := cliPrincipal(actor)
	f, d, err := inv.s.inventoryFile(ctx, p, org, format, withCredentials, nil, "")
	if err != nil {
		return nil, err
	}
	if d != nil {
		if inv.s.rep == nil {
			return nil, errors.New("PDF needs typst (report.typst_path or typst next to the binary)")
		}
		if f.Body, err = inv.s.rep.RenderBuiltin(ctx, org, *d, nil, actor); err != nil {
			return nil, err
		}
	}
	if withCredentials {
		inv.audit(ctx, org, actor)("video.credentials_export", "camera_inventory", "", nil, map[string]any{"cameras": f.Cameras, "format": format, "file": f.Name, "via": "command line"}, reason)
	} else {
		inv.audit(ctx, org, actor)("export.read", "export", "", nil, map[string]any{"export": "camera_inventory", "format": format, "cameras": f.Cameras, "passwords": false, "via": "command line"}, "")
	}
	return f, nil
}

// Import checks a file (apply false) or applies it; the plan tells what happened to every row.
func (inv *Inventory) Import(ctx context.Context, org string, data []byte, opt InventoryOptions, actor, reason string, apply bool) (*InventoryPlan, error) {
	f, err := camerainv.Parse(data, inventoryMaxRows)
	if err != nil {
		return nil, err
	}
	p := cliPrincipal(actor)
	if !apply {
		return inv.s.planInventory(ctx, inv.s.pool, p, org, f, opt)
	}
	if strings.TrimSpace(reason) == "" {
		return nil, errors.New("a reason is required")
	}
	return inv.s.applyInventory(ctx, p, org, f, opt, reason, inv.audit(ctx, org, actor))
}

// Actions of a plan, for the command line.
func (it *invItem) Summary() string {
	s := fmt.Sprintf("line %d: %-9s %s", it.Line, it.Action, it.Name)
	if it.IP != "" {
		s += " (" + it.IP + ")"
	}
	if len(it.Changes) > 0 {
		s += " — " + strings.Join(it.Changes, ", ")
	}
	for _, e := range it.Errors {
		s += "\n    error: " + e
	}
	for _, w := range it.Warnings {
		s += "\n    note: " + w
	}
	return s
}

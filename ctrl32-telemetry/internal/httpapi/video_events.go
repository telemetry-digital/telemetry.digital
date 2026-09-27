package httpapi

// Camera events, ONVIF and PTZ (spec 013).
//
//	POST /api/v1/video/discover                       video.manage  {hosts?: ["192.168.1.64:3702"]}: WS-Discovery
//	POST /api/v1/video/onvif/profiles                 video.manage  {url, username, password | id}: profiles with stream addresses
//	GET  /api/v1/video/cameras/{id}/events            video.playback ?from&to
//	POST /api/v1/video/cameras/{id}/events            video.view    {label, seconds}: a mark (records on "events" cameras)
//	GET  /api/v1/video/events                         video.playback ?from&to&kind: events of all cameras
//	GET  /api/v1/video/cameras/{id}/ptz/presets       video.ptz
//	POST /api/v1/video/cameras/{id}/ptz               video.ptz     {action: move|stop|preset, pan, tilt, zoom, preset}
//	GET  /api/v1/video/settings                       video.manage  server-wide video settings with defaults and bounds
//	PUT  /api/v1/video/settings                       system.admin  {settings…, reason}

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/video"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/video/onvif"
)

func (s *Server) videoDiscover(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Hosts []string `json:"hosts"`
	}
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &in, 8<<10); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	if len(in.Hosts) > 16 {
		writeErr(w, 400, "at most 16 extra hosts")
		return
	}
	var extra []string
	for _, h := range in.Hosts {
		h = strings.TrimSpace(h)
		if _, _, err := net.SplitHostPort(h); err != nil {
			h = net.JoinHostPort(h, "3702")
		}
		if host, _, _ := net.SplitHostPort(h); net.ParseIP(host) == nil {
			writeErr(w, 400, "hosts: IP addresses only")
			return
		}
		extra = append(extra, h)
	}
	wait := 3 * time.Second
	if s.video != nil {
		wait = time.Duration(s.video.Settings().DiscoverySeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), wait)
	defer cancel()
	devs, err := onvif.Discover(ctx, extra...)
	if err != nil {
		writeErr(w, 500, "discovery failed: "+err.Error())
		return
	}
	writeJSON(w, 200, devs)
}

// onvifProfiles lists a camera's profiles with stream addresses, so the form fills itself.
func (s *Server) onvifProfiles(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		URL      string  `json:"url"`
		Username string  `json:"username"`
		Password *string `json:"password"`
		ID       string  `json:"id"`
	}
	if err := decodeJSON(r, &in, 8<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	u, err := url.Parse(strings.TrimSpace(in.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		writeErr(w, 400, "url: http(s)://camera/onvif/device_service")
		return
	}
	pw := ""
	if in.Password != nil {
		pw = *in.Password
	} else if in.ID != "" && s.video != nil {
		if c, ok := s.cameraRow(r.Context(), org, in.ID); ok {
			pw = s.video.OpenPassword(c.sealed)
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	cl := &onvif.Client{Device: u.String(), Username: in.Username, Password: pw}
	ps, err := cl.Profiles(ctx)
	if err != nil {
		code := 502
		if errors.Is(err, onvif.ErrAuth) {
			code = 401
		}
		writeJSON(w, code, map[string]any{"error": err.Error()})
		return
	}
	caps, _ := cl.Capabilities(ctx)
	writeJSON(w, 200, map[string]any{"profiles": ps, "events": caps != nil && caps.Events != "", "ptz": caps != nil && caps.PTZ != ""})
}

func (s *Server) cameraEvents(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id != "" {
		if _, ok := s.cameraRow(r.Context(), org, id); !ok {
			writeErr(w, 404, "camera not found")
			return
		}
	}
	from, to, msg := parseRange(r.URL.Query(), 32*24*time.Hour)
	if msg != "" {
		writeErr(w, 400, msg)
		return
	}
	kind := r.URL.Query().Get("kind")
	p := PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `SELECT e.id, e.camera_id::text, c.name, e.kind, e.source, e.label, e.started_at, e.ended_at
		FROM core.camera_event e JOIN core.camera c ON c.id = e.camera_id
		WHERE e.org_id = $1::uuid AND ($2 = '' OR e.camera_id::text = $2) AND ($3 = '' OR e.kind = $3)
		  AND e.started_at < $5 AND coalesce(e.ended_at, now()) >= $4
		  AND (NOT $6::boolean OR c.camera_group = ANY($7::text[]))
		ORDER BY e.started_at DESC LIMIT 2000`, org, id, kind, from, to, p.CameraLimited, p.CameraGroups)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var eid int64
		var cam, name, k, src, label string
		var start time.Time
		var end *time.Time
		if rows.Scan(&eid, &cam, &name, &k, &src, &label, &start, &end) == nil {
			out = append(out, map[string]any{"id": eid, "camera_id": cam, "camera": name, "kind": k, "source": src, "label": label, "started_at": start, "ended_at": end})
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) markCamera(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	c, ok := s.cameraRow(r.Context(), org, r.PathValue("id"))
	if !ok || s.video == nil {
		writeErr(w, 404, "camera not found")
		return
	}
	var in struct {
		Label   string `json:"label"`
		Seconds int    `json:"seconds"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Label = strings.TrimSpace(in.Label)
	if in.Label == "" || len([]rune(in.Label)) > 200 || in.Seconds < 0 || in.Seconds > 3600 {
		writeErr(w, 400, "label (1–200 characters) required; seconds 0 to 3600")
		return
	}
	id, err := s.video.Mark(c.id, "mark", "user", in.Label, in.Seconds)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	s.audit(r, "camera.mark", "camera", c.id, nil, map[string]any{"camera": c.name, "label": in.Label, "seconds": in.Seconds, "event": id}, in.Label)
	writeJSON(w, 201, map[string]any{"id": id})
}

// ptzAudit remembers when a user last had a PTZ move audited per camera: moves are audited once per the configured
// interval (every move with 0), preset changes always.
var ptzAudit sync.Map

func (s *Server) ptzPresets(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	c, ok := s.cameraRow(r.Context(), org, r.PathValue("id"))
	if !ok || s.video == nil {
		writeErr(w, 404, "camera not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	cl, prof, err := s.video.ONVIF(ctx, c.id)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	ps, err := cl.Presets(ctx, prof)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, ps)
}

func (s *Server) cameraPTZ(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	c, ok := s.cameraRow(r.Context(), org, r.PathValue("id"))
	if !ok || s.video == nil {
		writeErr(w, 404, "camera not found")
		return
	}
	var in struct {
		Action string  `json:"action"`
		Pan    float64 `json:"pan"`
		Tilt   float64 `json:"tilt"`
		Zoom   float64 `json:"zoom"`
		Preset string  `json:"preset"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	cl, prof, err := s.video.ONVIF(ctx, c.id)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	p := PrincipalFrom(r.Context())
	switch in.Action {
	case "move":
		if in.Pan < -1 || in.Pan > 1 || in.Tilt < -1 || in.Tilt > 1 || in.Zoom < -1 || in.Zoom > 1 {
			writeErr(w, 400, "pan, tilt and zoom speeds from -1 to 1")
			return
		}
		err = cl.Move(ctx, prof, in.Pan, in.Tilt, in.Zoom)
		key := p.UserID + "|" + c.id
		every := time.Duration(s.video.Settings().PTZAuditMinutes) * time.Minute
		if last, ok := ptzAudit.Load(key); !ok || time.Since(last.(time.Time)) >= every {
			ptzAudit.Store(key, time.Now())
			s.audit(r, "camera.ptz", "camera", c.id, nil, map[string]any{"camera": c.name, "action": "move"}, "PTZ control")
		}
	case "stop":
		err = cl.Stop(ctx, prof)
	case "preset":
		if in.Preset == "" || len(in.Preset) > 64 {
			writeErr(w, 400, "preset required")
			return
		}
		err = cl.GotoPreset(ctx, prof, in.Preset)
		s.audit(r, "camera.ptz", "camera", c.id, nil, map[string]any{"camera": c.name, "action": "preset", "preset": in.Preset}, "PTZ preset")
	default:
		writeErr(w, 400, "action: move, stop or preset")
		return
	}
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// cameraEventForFlows is what the live hub carries to flows and pages.
func cameraEventDetail(e video.CameraEvent) map[string]any {
	return map[string]any{"camera_id": e.CameraID, "camera": e.Camera, "kind": e.Kind, "source": e.Source, "label": e.Label, "phase": e.Phase, "event_id": e.ID}
}

func (s *Server) videoSettings(w http.ResponseWriter, r *http.Request) {
	if s.video == nil {
		writeErr(w, 503, "video is not running")
		return
	}
	p := PrincipalFrom(r.Context())
	writeJSON(w, 200, map[string]any{"settings": s.video.Settings(), "defaults": video.DefaultSettings(s.cfg.Video.MaxDiskGB),
		"can_edit": p.Has("system.admin"), "storage_dir": s.video.Dir()})
}

func (s *Server) putVideoSettings(w http.ResponseWriter, r *http.Request) {
	if s.video == nil {
		writeErr(w, 503, "video is not running")
		return
	}
	old := s.video.Settings()
	in := struct {
		video.Settings
		Reason string `json:"reason"`
	}{Settings: old}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" || len([]rune(in.Reason)) > 200 {
		writeErr(w, 400, "reason required (at most 200 characters)")
		return
	}
	if msg := in.Settings.Validate(); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if err := s.video.SaveSettings(r.Context(), in.Settings, PrincipalFrom(r.Context()).UserID); err != nil {
		writeErr(w, 500, "settings not saved")
		return
	}
	s.audit(r, "video.settings", "server", "video", old, in.Settings, in.Reason)
	writeJSON(w, 200, in.Settings)
}

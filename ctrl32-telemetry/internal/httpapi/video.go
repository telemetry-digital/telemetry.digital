package httpapi

// Cameras, recordings and video walls (spec 010). The server pulls the camera streams itself (RTSP with H.264, H.265
// or MJPEG; HTTP MJPEG), records them and serves video to browsers over a WebSocket: H.264/H.265 as fragmented MP4 for
// Media Source Extensions, MJPEG as single JPEG pictures. Recordings are personal data: playback and export need
// video.playback and are written to the audit trail.
//
//	GET    /api/v1/video/cameras                      video.view
//	POST   /api/v1/video/cameras                      video.manage
//	POST   /api/v1/video/cameras/test                 video.manage
//	GET    /api/v1/video/cameras/{id}                 video.view
//	PUT    /api/v1/video/cameras/{id}                 video.manage
//	DELETE /api/v1/video/cameras/{id}?reason=         video.manage
//	GET    /api/v1/video/cameras/{id}/live            video.view      WebSocket, ?quality=sub|main
//	GET    /api/v1/video/cameras/{id}/snapshot        video.view      JPEG (MJPEG cameras)
//	GET    /api/v1/video/cameras/{id}/recordings      video.playback  ?from&to: recorded spans
//	GET    /api/v1/video/cameras/{id}/playback        video.playback  WebSocket, ?from&speed
//	GET    /api/v1/video/cameras/{id}/export          video.playback  MP4 download, ?from&to (≤ 1 h)
//	GET    /api/v1/video/storage                      video.manage
//	GET    /api/v1/video/walls …                      video.view / video.manage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/video"
)

type cameraIn struct {
	Name          *string          `json:"name"`
	Location      *string          `json:"location"`
	URL           *string          `json:"url"`
	SubURL        *string          `json:"sub_url"`
	Username      *string          `json:"username"`
	Password      *string          `json:"password"`
	Transport     *string          `json:"transport"`
	Record        *string          `json:"record"`
	RetentionDays *int             `json:"retention_days"`
	Enabled       *bool            `json:"enabled"`
	Sort          *int             `json:"sort"`
	Source        *string          `json:"source"` // pull (the server connects to the camera) or push (a relay sends the video)
	OnvifURL      *string          `json:"onvif_url"`
	PTZProfile    *string          `json:"ptz_profile"`
	EventPre      *int             `json:"event_pre_s"`
	EventPost     *int             `json:"event_post_s"`
	EventKinds    *[]string        `json:"event_kinds"`
	Group         *string          `json:"group"`
	PrivacyMasks  *json.RawMessage `json:"privacy_masks"`
	// motion detection on the server (spec 019)
	MotionDetect      *bool            `json:"motion_detect"`
	MotionSensitivity *int             `json:"motion_sensitivity"`
	MotionZones       *json.RawMessage `json:"motion_zones"`
	SnapshotURL       *string          `json:"snapshot_url"`
	Audio             *bool            `json:"audio"` // record and play the camera's sound (spec 020)
	Reason            string           `json:"reason"`
}

type cameraRow struct {
	id, name, location, url, subURL, username, sealed, transport, record string
	source, pushSealed                                                   string
	onvifURL, ptzProfile                                                 string
	eventPre, eventPost                                                  int
	eventKinds                                                           []string
	group                                                                string
	masks                                                                []byte // privacy masks (JSON), spec 016
	motion                                                               bool   // motion detection on the server, spec 019
	motionSens                                                           int
	zones                                                                []byte
	snapshotURL                                                          string
	audio                                                                bool // sound, spec 020
	retention, sort                                                      int
	enabled                                                              bool
}

func (s *Server) cameraRow(ctx context.Context, org, id string) (cameraRow, bool) {
	var c cameraRow
	err := s.pool.QueryRow(ctx, `SELECT id::text, name, location, url, sub_url, username, password_sealed, transport, record, retention_days, sort, enabled, source, push_secret_sealed,
		onvif_url, ptz_profile, event_pre_s, event_post_s, event_kinds, camera_group, privacy_masks, motion_detect, motion_sensitivity, motion_zones, snapshot_url, audio
		FROM core.camera WHERE id = $1::uuid AND org_id = $2::uuid AND removed_at IS NULL`, id, org).
		Scan(&c.id, &c.name, &c.location, &c.url, &c.subURL, &c.username, &c.sealed, &c.transport, &c.record, &c.retention, &c.sort, &c.enabled, &c.source, &c.pushSealed,
			&c.onvifURL, &c.ptzProfile, &c.eventPre, &c.eventPost, &c.eventKinds, &c.group, &c.masks, &c.motion, &c.motionSens, &c.zones, &c.snapshotURL, &c.audio)
	if err != nil {
		return c, false
	}
	// every camera endpoint looks the camera up here: a user limited to some camera groups never reaches others
	if p := PrincipalFrom(ctx); !p.SeesCameraGroup(c.group) {
		return c, false
	}
	return c, true
}

// cameraJSON shows stream addresses only to managers (they reveal the network).
func (s *Server) cameraJSON(r *http.Request, c cameraRow) map[string]any {
	out := map[string]any{"id": c.id, "name": c.name, "location": c.location, "record": c.record, "retention_days": c.retention,
		"sort": c.sort, "enabled": c.enabled, "has_sub": c.subURL != "" || c.source == "push", "source": c.source,
		"onvif": c.onvifURL != "", "event_pre_s": c.eventPre, "event_post_s": c.eventPost, "event_kinds": c.eventKinds, "group": c.group, "privacy_masks": json.RawMessage(orEmpty(c.masks)),
		"motion_detect": c.motion, "motion_sensitivity": c.motionSens, "motion_zones": json.RawMessage(orEmpty(c.zones)), "audio": c.audio}
	if p := PrincipalFrom(r.Context()); p.Has("video.manage") {
		out["snapshot_url"] = c.snapshotURL
		out["url"], out["sub_url"], out["username"], out["transport"], out["has_password"] = c.url, c.subURL, c.username, c.transport, c.sealed != ""
		out["onvif_url"], out["ptz_profile"] = c.onvifURL, c.ptzProfile
		if c.source == "push" {
			base := ""
			if s.video != nil {
				base = s.video.PushURL()
			}
			out["push"] = map[string]any{"enabled": base != "", "url": base + "/" + c.id, "sub_url": base + "/" + c.id + "/sub", "user": c.id}
		}
	}
	st := video.Status{State: "disabled"}
	if s.video != nil {
		st, _ = s.video.Status(c.id)
	}
	out["status"] = st
	return out
}

func (s *Server) listCameras(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT id::text FROM core.camera WHERE org_id = $1::uuid AND removed_at IS NULL ORDER BY sort, name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	out := []map[string]any{}
	for _, id := range ids {
		if c, ok := s.cameraRow(r.Context(), org, id); ok {
			out = append(out, s.cameraJSON(r, c))
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) getCamera(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	c, ok := s.cameraRow(r.Context(), org, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "camera not found")
		return
	}
	writeJSON(w, 200, s.cameraJSON(r, c))
}

// splitCredentials moves user:password out of a pasted stream URL.
func splitCredentials(raw string) (clean, user, pw string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", "", "", errors.New("invalid URL")
	}
	switch u.Scheme {
	case "rtsp", "rtsps", "http", "https":
	default:
		return "", "", "", errors.New("the URL must start with rtsp://, rtsps://, http:// or https://")
	}
	if u.User != nil {
		user = u.User.Username()
		pw, _ = u.User.Password()
		u.User = nil
	}
	return u.String(), user, pw, nil
}

// applyCamera validates the input onto a row; it returns an error message for the client.
func (s *Server) applyCamera(in *cameraIn, c *cameraRow, pw **string) string {
	if strings.TrimSpace(in.Reason) == "" {
		return "reason is required"
	}
	if in.Name != nil {
		c.name = strings.TrimSpace(*in.Name)
	}
	if c.name == "" || len([]rune(c.name)) > 80 {
		return "name: 1 to 80 characters"
	}
	if in.Location != nil {
		c.location = strings.TrimSpace(*in.Location)
		if len([]rune(c.location)) > 120 {
			return "location: at most 120 characters"
		}
	}
	if in.PrivacyMasks != nil {
		m, msg := validateMasks(*in.PrivacyMasks)
		if msg != "" {
			return msg
		}
		c.masks, _ = json.Marshal(m)
	}
	if in.Group != nil {
		c.group = strings.TrimSpace(*in.Group)
		if len([]rune(c.group)) > 60 {
			return "group: at most 60 characters"
		}
	}
	if in.Source != nil {
		c.source = *in.Source
	}
	switch c.source {
	case "pull":
	case "push":
		// the relay at the site knows the camera addresses; the server keeps none
		c.url, c.subURL, c.username, c.sealed, c.transport = "", "", "", "", "auto"
		in.URL, in.SubURL, in.Username, in.Password, in.Transport = nil, nil, nil, nil, nil
	default:
		return "source: pull or push"
	}
	for _, f := range []struct {
		in  *string
		dst *string
		req bool
	}{{in.URL, &c.url, true}, {in.SubURL, &c.subURL, false}} {
		if f.in == nil {
			continue
		}
		clean, user, p, err := splitCredentials(*f.in)
		if err != nil {
			return "url: " + err.Error()
		}
		*f.dst = clean
		if user != "" && in.Username == nil {
			in.Username = &user
			if p != "" && in.Password == nil {
				in.Password = &p
			}
		}
	}
	if c.url == "" && c.source != "push" {
		return "url is required (rtsp://… or http://… of the camera's stream)"
	}
	if in.Username != nil {
		c.username = strings.TrimSpace(*in.Username)
	}
	if in.Password != nil {
		*pw = in.Password
	}
	if in.Transport != nil {
		c.transport = *in.Transport
	}
	switch c.transport {
	case "auto", "tcp", "udp":
	default:
		return "transport: auto, tcp or udp"
	}
	if in.Record != nil {
		c.record = *in.Record
	}
	if c.record != "off" && c.record != "continuous" && c.record != "events" {
		return "record: off, continuous or events"
	}
	if msg := applyMotion(in, c); msg != "" {
		return msg
	}
	if in.OnvifURL != nil {
		c.onvifURL = strings.TrimSpace(*in.OnvifURL)
	}
	if c.onvifURL != "" {
		if u, err := url.Parse(c.onvifURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			return "onvif_url: http(s)://camera/onvif/device_service without credentials"
		}
		if c.source == "push" {
			return "onvif_url: not for cameras that send their video through a relay"
		}
	}
	if in.PTZProfile != nil {
		c.ptzProfile = strings.TrimSpace(*in.PTZProfile)
		if len(c.ptzProfile) > 64 {
			return "ptz_profile: at most 64 characters"
		}
	}
	if in.EventPre != nil {
		c.eventPre = *in.EventPre
	}
	if in.EventPost != nil {
		c.eventPost = *in.EventPost
	}
	if c.eventPre < 0 || c.eventPre > 60 || c.eventPost < 1 || c.eventPost > 600 {
		return "event_pre_s 0 to 60, event_post_s 1 to 600"
	}
	if in.EventKinds != nil {
		c.eventKinds = []string{}
		for _, k := range *in.EventKinds {
			switch k {
			case "motion", "tamper", "line", "intrusion", "input", "mark", "other":
				if !slices.Contains(c.eventKinds, k) {
					c.eventKinds = append(c.eventKinds, k)
				}
			default:
				return "event_kinds: motion, tamper, line, intrusion, input, mark, other"
			}
		}
	}
	if in.RetentionDays != nil {
		c.retention = *in.RetentionDays
	}
	if c.retention < 1 || c.retention > 3650 {
		return "retention_days: 1 to 3650"
	}
	if in.Enabled != nil {
		c.enabled = *in.Enabled
	}
	if in.Sort != nil {
		c.sort = *in.Sort
	}
	return ""
}

func (s *Server) createCamera(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in cameraIn
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	c := cameraRow{transport: "auto", record: "continuous", retention: 7, enabled: true, source: "pull", eventPre: 10, eventPost: 20, motionSens: 50,
		eventKinds: []string{"motion", "tamper", "line", "intrusion", "input", "mark"}}
	var pw *string
	if msg := s.applyCamera(&in, &c, &pw); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if !PrincipalFrom(r.Context()).SeesCameraGroup(c.group) {
		writeErr(w, 403, "group: only the camera groups you may see")
		return
	}
	var secret string
	if c.source == "push" {
		if s.video == nil {
			writeErr(w, 409, "video is not available")
			return
		}
		secret = video.NewPushSecret()
		sealed, err := s.video.SealPassword(secret)
		if err != nil {
			writeErr(w, 500, "secret not stored: "+err.Error())
			return
		}
		c.pushSealed = sealed
	}
	if pw != nil && s.video != nil {
		sealed, err := s.video.SealPassword(*pw)
		if err != nil {
			writeErr(w, 500, "password not stored: "+err.Error())
			return
		}
		c.sealed = sealed
	}
	by := PrincipalFrom(r.Context()).Username
	err := s.pool.QueryRow(r.Context(), `INSERT INTO core.camera (org_id, name, location, url, sub_url, username, password_sealed, transport, record, retention_days, enabled, sort, created_by, source, push_secret_sealed,
		onvif_url, ptz_profile, event_pre_s, event_post_s, event_kinds, camera_group)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21) RETURNING id::text`,
		org, c.name, c.location, c.url, c.subURL, c.username, c.sealed, c.transport, c.record, c.retention, c.enabled, c.sort, by, c.source, c.pushSealed,
		c.onvifURL, c.ptzProfile, c.eventPre, c.eventPost, c.eventKinds, c.group).Scan(&c.id)
	if err != nil {
		writeErr(w, 500, "camera not stored")
		return
	}
	if _, err := s.pool.Exec(r.Context(), storeExtras, extrasArgs(c)...); err != nil {
		writeErr(w, 500, "privacy masks not stored")
		return
	}
	s.audit(r, "camera.create", "camera", c.id, nil, cameraAudit(c), in.Reason)
	if s.video != nil {
		s.video.Reload(context.Background(), c.id)
	}
	out := s.cameraJSON(r, c)
	if secret != "" {
		s.withSecret(out, c, secret)
	}
	writeJSON(w, 201, out)
}

// withSecret adds the push secret (shown once) and a ready relay configuration for the site.
func (s *Server) withSecret(out map[string]any, c cameraRow, secret string) {
	server := s.video.PushURL()
	if server == "" {
		server = "rtsps://SERVER:8322"
	}
	out["push_secret"] = secret
	out["relay_config"] = fmt.Sprintf("# relay.toml at the remote site — keep it private (chmod 600)\nserver = %q\n\n[[camera]]\n# %s\nid = %q\nsecret = %q\nsource = \"rtsp://user:password@192.168.1.64:554/stream1\"   # main stream of the local camera\n# sub_source = \"rtsp://user:password@192.168.1.64:554/stream2\"   # optional, for tiles\n# audio = true   # forward the camera's sound too (AAC or Opus; switch the sound on for the camera on the server as well)\n",
		server, strings.ReplaceAll(c.name, "\n", " "), c.id, secret)
}

// rotatePushSecret issues a new secret for a push camera; the old one stops working at once.
func (s *Server) rotatePushSecret(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil || strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason is required")
		return
	}
	c, ok := s.cameraRow(r.Context(), org, r.PathValue("id"))
	if !ok || s.video == nil {
		writeErr(w, 404, "camera not found")
		return
	}
	if c.source != "push" {
		writeErr(w, 409, "the camera does not send its video (source is pull)")
		return
	}
	secret := video.NewPushSecret()
	sealed, err := s.video.SealPassword(secret)
	if err != nil {
		writeErr(w, 500, "secret not stored")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.camera SET push_secret_sealed = $2, updated_at = now() WHERE id = $1::uuid`, c.id, sealed); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	c.pushSealed = sealed
	s.audit(r, "camera.push_secret", "camera", c.id, nil, map[string]any{"rotated": true}, in.Reason)
	s.video.Reload(context.Background(), c.id) // disconnects a relay with the old secret
	out := s.cameraJSON(r, c)
	s.withSecret(out, c, secret)
	writeJSON(w, 200, out)
}

func cameraAudit(c cameraRow) map[string]any {
	return map[string]any{"name": c.name, "location": c.location, "source": c.source, "url": c.url, "sub_url": c.subURL, "username": c.username,
		"password_set": c.sealed != "", "transport": c.transport, "record": c.record, "retention_days": c.retention, "enabled": c.enabled,
		"onvif_url": c.onvifURL, "ptz_profile": c.ptzProfile, "event_pre_s": c.eventPre, "event_post_s": c.eventPost, "event_kinds": c.eventKinds, "group": c.group, "privacy_masks": json.RawMessage(orEmpty(c.masks)),
		"motion_detect": c.motion, "motion_sensitivity": c.motionSens, "motion_zones": json.RawMessage(orEmpty(c.zones)), "snapshot_url": c.snapshotURL, "audio": c.audio}
}

func orEmpty(b []byte) []byte {
	if len(b) == 0 {
		return []byte("[]")
	}
	return b
}

func (s *Server) updateCamera(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	c, ok := s.cameraRow(r.Context(), org, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "camera not found")
		return
	}
	old := cameraAudit(c)
	var in cameraIn
	if err := decodeJSON(r, &in, 16<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	wasPush := c.source == "push"
	var pw *string
	if msg := s.applyCamera(&in, &c, &pw); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if !PrincipalFrom(r.Context()).SeesCameraGroup(c.group) {
		writeErr(w, 403, "group: only the camera groups you may see")
		return
	}
	if pw != nil && s.video != nil {
		sealed, err := s.video.SealPassword(*pw)
		if err != nil {
			writeErr(w, 500, "password not stored: "+err.Error())
			return
		}
		c.sealed = sealed
	}
	var secret string
	if c.source == "push" && !wasPush && s.video != nil {
		secret = video.NewPushSecret()
		c.pushSealed, _ = s.video.SealPassword(secret)
	}
	if c.source == "pull" {
		c.pushSealed = ""
	}
	_, err := s.pool.Exec(r.Context(), `UPDATE core.camera SET name = $3, location = $4, url = $5, sub_url = $6, username = $7, password_sealed = $8,
		transport = $9, record = $10, retention_days = $11, enabled = $12, sort = $13, source = $14, push_secret_sealed = $15,
		onvif_url = $16, ptz_profile = $17, event_pre_s = $18, event_post_s = $19, event_kinds = $20, camera_group = $21, updated_at = now() WHERE id = $1::uuid AND org_id = $2::uuid`,
		c.id, org, c.name, c.location, c.url, c.subURL, c.username, c.sealed, c.transport, c.record, c.retention, c.enabled, c.sort, c.source, c.pushSealed,
		c.onvifURL, c.ptzProfile, c.eventPre, c.eventPost, c.eventKinds, c.group)
	if err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	if _, err := s.pool.Exec(r.Context(), storeExtras, extrasArgs(c)...); err != nil {
		writeErr(w, 500, "privacy masks not stored")
		return
	}
	s.audit(r, "camera.update", "camera", c.id, old, cameraAudit(c), in.Reason)
	if s.video != nil {
		s.video.Reload(context.Background(), c.id)
	}
	out := s.cameraJSON(r, c)
	if secret != "" {
		s.withSecret(out, c, secret)
	}
	writeJSON(w, 200, out)
}

func (s *Server) deleteCamera(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	reason := r.URL.Query().Get("reason")
	if strings.TrimSpace(reason) == "" {
		writeErr(w, 400, "reason is required")
		return
	}
	c, ok := s.cameraRow(r.Context(), org, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "camera not found")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.camera SET removed_at = now() WHERE id = $1::uuid`, c.id); err != nil {
		writeErr(w, 500, "delete failed")
		return
	}
	s.audit(r, "camera.delete", "camera", c.id, cameraAudit(c), nil, reason)
	if s.video != nil {
		s.video.Reload(context.Background(), c.id)
	}
	writeJSON(w, 200, map[string]any{"ok": true, "note": "recordings stay until the camera's retention ends"})
}

// testCamera connects once: POST {url, username, password, transport, id?}; the stored password is used when the
// form leaves it empty.
func (s *Server) testCamera(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		ID        string  `json:"id"`
		URL       string  `json:"url"`
		Username  string  `json:"username"`
		Password  *string `json:"password"`
		Transport string  `json:"transport"`
	}
	if err := decodeJSON(r, &in, 8<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	clean, user, pw, err := splitCredentials(in.URL)
	if err != nil || clean == "" {
		writeErr(w, 400, "url: invalid")
		return
	}
	if in.Username != "" {
		user = in.Username
	}
	if in.Password != nil {
		pw = *in.Password
	} else if in.ID != "" && pw == "" && s.video != nil {
		if c, ok := s.cameraRow(r.Context(), org, in.ID); ok {
			pw = s.video.OpenPassword(c.sealed)
		}
	}
	u, err := video.StreamURL(clean, user, pw)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	st, _ := video.Probe(ctx, u, in.Transport)
	writeJSON(w, 200, st)
}

func (s *Server) cameraSnapshot(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	c, ok := s.cameraRow(r.Context(), org, r.PathValue("id"))
	if !ok || s.video == nil {
		writeErr(w, 404, "camera not found")
		return
	}
	src := s.video.Source(c.id, r.URL.Query().Get("quality") != "main")
	if src == nil {
		writeErr(w, 409, "camera is disabled")
		return
	}
	img := src.Snapshot()
	if img == nil {
		sub := src.Subscribe()
		timeout := time.After(6 * time.Second)
	wait:
		for {
			select {
			case p, ok := <-sub.C:
				if !ok {
					break wait
				}
				if p.Sample.Audio != nil {
					continue
				}
				if p.Track.Codec != video.MJPEG {
					break wait
				}
				img = p.Sample.JPEG
				break wait
			case <-timeout:
				break wait
			}
		}
		sub.Close()
	}
	if img == nil {
		writeErr(w, 404, "no picture (snapshots need an MJPEG stream, or the camera is offline)")
		return
	}
	if m := parseMasks(c.masks); len(m) > 0 && !s.unmasked(r, c) {
		masked, err := maskJPEG(img, m)
		if err != nil {
			writeErr(w, 500, "picture could not be masked")
			return
		}
		img = masked
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(img)
}

func parseRange(q url.Values, maxSpan time.Duration) (time.Time, time.Time, string) {
	from, err := time.Parse(time.RFC3339Nano, q.Get("from"))
	if err != nil {
		return time.Time{}, time.Time{}, "from: RFC 3339 time required"
	}
	to := time.Now()
	if v := q.Get("to"); v != "" {
		if to, err = time.Parse(time.RFC3339Nano, v); err != nil {
			return time.Time{}, time.Time{}, "to: RFC 3339 time"
		}
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, "to must be after from"
	}
	if to.Sub(from) > maxSpan {
		return time.Time{}, time.Time{}, fmt.Sprintf("the range may span at most %s", maxSpan)
	}
	return from, to, ""
}

// cameraRecordings lists the recorded spans of a range (adjacent files merged), for the timeline.
func (s *Server) cameraRecordings(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	c, ok := s.cameraRow(r.Context(), org, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "camera not found")
		return
	}
	from, to, msg := parseRange(r.URL.Query(), 32*24*time.Hour)
	if msg != "" {
		writeErr(w, 400, msg)
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT started_at, ended_at, bytes FROM core.video_segment
		WHERE camera_id = $1::uuid AND ended_at > $2 AND started_at < $3 ORDER BY started_at`, c.id, from, to)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	type span struct {
		From time.Time `json:"from"`
		To   time.Time `json:"to"`
	}
	spans := []span{}
	var total int64
	n := 0
	for rows.Next() {
		var a, b time.Time
		var bytes int64
		if rows.Scan(&a, &b, &bytes) != nil {
			continue
		}
		n++
		total += bytes
		if l := len(spans); l > 0 && a.Sub(spans[l-1].To) <= 5*time.Second {
			if b.After(spans[l-1].To) {
				spans[l-1].To = b
			}
			continue
		}
		spans = append(spans, span{a, b})
	}
	writeJSON(w, 200, map[string]any{"camera": c.id, "from": from, "to": to, "spans": spans, "files": n, "bytes": total})
}

var videoUpgrader = websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 64 << 10}

// videoConn wraps a WebSocket that carries video to one player.
type videoConn struct {
	ws    *websocket.Conn
	track *video.Track
	mux   *video.Muxer
	ctx   context.Context
	audio bool         // the player asked for the sound and may hear it (spec 020)
	amux  *video.Muxer // the sound alone, sent as binary messages starting with 'A'
}

func (s *Server) upgradeVideo(w http.ResponseWriter, r *http.Request) (*videoConn, context.CancelFunc, bool) {
	ws, err := videoUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, nil, false // the upgrader answered
	}
	_ = ws.NetConn().SetDeadline(time.Time{}) // the HTTP server's timeouts do not apply to a hijacked connection
	ctx, cancel := context.WithCancel(r.Context())
	go func() { // reads detect the player going away (and answer pings)
		defer cancel()
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}()
	go func() {
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
			}
		}
	}()
	return &videoConn{ws: ws, ctx: ctx}, cancel, true
}

func (v *videoConn) json(m map[string]any) error {
	_ = v.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return v.ws.WriteJSON(m)
}

func (v *videoConn) binary(b []byte) error {
	_ = v.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return v.ws.WriteMessage(websocket.BinaryMessage, b)
}

func (v *videoConn) close(reason string) {
	_ = v.ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, reason), time.Now().Add(2*time.Second))
	_ = v.ws.Close()
}

// send writes one frame: a new track first sends {"type":"init"} (and the MP4 initialization for MSE players); key
// frames carry {"type":"time"} with the recording time and the position in the player's timeline.
func (v *videoConn) send(p video.Packet) error {
	if p.Track != v.track {
		v.track = p.Track
		v.mux = &video.Muxer{Track: p.Track, NoAudio: true}
		v.amux = nil
		if v.audio && p.Track.Audio != nil {
			v.amux = &video.Muxer{Track: p.Track, AudioOnly: true}
		}
		if err := v.json(map[string]any{"type": "init", "codec": p.Track.Codec, "mime": p.Track.MIME(), "width": p.Track.Width, "height": p.Track.Height}); err != nil {
			return err
		}
		if p.Track.Browser() {
			init, err := v.mux.Init()
			if err != nil {
				return err
			}
			if err := v.binary(init); err != nil {
				return err
			}
		}
		if v.amux != nil {
			init, err := v.amux.Init()
			if err != nil {
				return err
			}
			if err := v.json(map[string]any{"type": "audio", "mime": p.Track.Audio.MIME(), "codec": p.Track.Audio.Codec}); err != nil {
				return err
			}
			if err := v.binary(append([]byte{'A'}, init...)); err != nil {
				return err
			}
		}
	}
	if p.Sample.Audio != nil {
		if v.amux == nil {
			return nil
		}
		frag, err := v.amux.Push(p.Sample)
		if err != nil || frag == nil {
			return err
		}
		return v.binary(append([]byte{'A'}, frag...))
	}
	if v.amux != nil {
		v.amux.Start(p.Sample.DTS) // the sound's time line is the picture's
	}
	if p.Sample.Key {
		pos := v.mux.Pos(p.Sample)
		if err := v.json(map[string]any{"type": "time", "t": p.Sample.Time.UnixMilli(), "pos": pos}); err != nil {
			return err
		}
	}
	if !v.track.Browser() {
		return v.binary(p.Sample.JPEG)
	}
	frag, err := v.mux.Push(p.Sample)
	if err != nil || frag == nil {
		return err
	}
	return v.binary(frag)
}

// hears decides whether a player gets the camera's sound: asked for (?audio=1), switched on at the camera and
// permitted (video.audio); listening is audited.
func (s *Server) hears(r *http.Request, c cameraRow) bool {
	if !c.audio || r.URL.Query().Get("audio") != "1" || !PrincipalFrom(r.Context()).Has("video.audio") {
		return false
	}
	s.audit(r, "video.listen", "camera", c.id, nil, map[string]any{"camera": c.name, "path": r.URL.Path}, "camera sound heard")
	return true
}

// cameraLive streams the live picture: GET …/live?quality=sub|main (WebSocket).
func (s *Server) cameraLive(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	c, ok := s.cameraRow(r.Context(), org, r.PathValue("id"))
	if !ok || s.video == nil {
		writeErr(w, 404, "camera not found")
		return
	}
	src := s.video.Source(c.id, r.URL.Query().Get("quality") != "main")
	if src == nil {
		writeErr(w, 409, "camera is disabled")
		return
	}
	unmasked := s.unmasked(r, c)
	hears := r.URL.Query().Get("quality") == "main" && s.hears(r, c) // the sound comes with the main stream
	vc, cancel, ok := s.upgradeVideo(w, r)
	if !ok {
		return
	}
	defer cancel()
	vc.audio = hears
	if err := vc.json(masksMessage(c, unmasked)); err != nil {
		return
	}
	sub := src.Subscribe()
	defer sub.Close()
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	lastState := ""
	for {
		select {
		case <-vc.ctx.Done():
			vc.close("")
			return
		case <-tick.C:
			st := src.Status()
			if st.State != lastState || st.State != "online" {
				lastState = st.State
				if err := vc.json(map[string]any{"type": "status", "state": st.State, "error": st.Error}); err != nil {
					return
				}
			}
		case p, ok := <-sub.C:
			if !ok {
				vc.close("stream restarted")
				return
			}
			if err := vc.send(p); err != nil {
				vc.close("")
				return
			}
		}
	}
}

// cameraPlayback streams recorded video from a point in time: GET …/playback?from=&speed= (WebSocket). The pace
// follows the recording; periods without recording are skipped.
func (s *Server) cameraPlayback(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	c, ok := s.cameraRow(r.Context(), org, r.PathValue("id"))
	if !ok || s.video == nil {
		writeErr(w, 404, "camera not found")
		return
	}
	q := r.URL.Query()
	from, err := time.Parse(time.RFC3339Nano, q.Get("from"))
	if err != nil {
		writeErr(w, 400, "from: RFC 3339 time required")
		return
	}
	speed := 1.0
	if v := q.Get("speed"); v != "" {
		if speed, err = strconv.ParseFloat(v, 64); err != nil || speed < 0.25 || speed > 16 {
			writeErr(w, 400, "speed: 0.25 to 16")
			return
		}
	}
	s.audit(r, "video.playback", "camera", c.id, nil, map[string]any{"camera": c.name, "from": from, "speed": speed}, "recording viewed")
	unmasked := s.unmasked(r, c)
	hears := speed == 1 && s.hears(r, c)
	vc, cancel, ok := s.upgradeVideo(w, r)
	if !ok {
		return
	}
	defer cancel()
	vc.audio = hears
	if err := vc.json(masksMessage(c, unmasked)); err != nil {
		return
	}
	pb := s.video.Playback(c.id, from, time.Time{})
	defer pb.Close()
	var t0 time.Time
	var d0 int64
	for {
		p, err := pb.Next(vc.ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				_ = vc.json(map[string]any{"type": "end"})
			}
			vc.close("")
			return
		}
		if t0.IsZero() {
			t0, d0 = time.Now(), p.Sample.DTS
		}
		due := t0.Add(time.Duration(float64(p.Sample.DTS-d0) / video.ClockRate / speed * float64(time.Second)))
		// send up to half a second ahead so the player keeps a small buffer
		if wait := time.Until(due) - 500*time.Millisecond; wait > 0 {
			select {
			case <-vc.ctx.Done():
				vc.close("")
				return
			case <-time.After(wait):
			}
		}
		if err := vc.send(p); err != nil {
			vc.close("")
			return
		}
	}
}

// cameraExport downloads recorded video as one MP4 file: GET …/export?from&to (at most one hour). A change of the
// camera's resolution or codec ends the file early.
func (s *Server) cameraExport(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	c, ok := s.cameraRow(r.Context(), org, r.PathValue("id"))
	if !ok || s.video == nil {
		writeErr(w, 404, "camera not found")
		return
	}
	if !s.maskedExportAllowed(w, r, c) {
		return
	}
	from, to, msg := parseRange(r.URL.Query(), time.Hour)
	if msg != "" {
		writeErr(w, 400, msg)
		return
	}
	pb := s.video.Playback(c.id, from, to)
	defer pb.Close()
	first, err := pb.Next(r.Context())
	if err != nil {
		writeErr(w, 404, "no recording in this range")
		return
	}
	s.audit(r, "video.export", "camera", c.id, nil, map[string]any{"camera": c.name, "from": from, "to": to}, "recording exported")
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
	name := fmt.Sprintf("%s-%s.mp4", safeFileName(c.name), from.UTC().Format("20060102-150405"))
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_ = writeMP4(r.Context(), pb, first, w, s.exportsSound(r, c))
}

// exportsSound: the recorded sound goes into an export only for people who may hear it.
func (s *Server) exportsSound(r *http.Request, c cameraRow) bool {
	return c.audio && PrincipalFrom(r.Context()).Has("video.audio")
}

// writeMP4 remuxes recorded frames into one MP4 (no re-encoding) until the range ends or the stream changes.
func writeMP4(ctx context.Context, pb *video.Playback, first video.Packet, w io.Writer, sound bool) error {
	mux := &video.Muxer{Track: first.Track, Batch: video.ClockRate, NoAudio: !sound}
	init, err := mux.Init()
	if err != nil {
		return err
	}
	if _, err := w.Write(init); err != nil {
		return err
	}
	p := first
	for {
		if frag, err := mux.Push(p.Sample); err != nil {
			return err
		} else if frag != nil {
			if _, err := w.Write(frag); err != nil {
				return err
			}
		}
		if p, err = pb.Next(ctx); err != nil || !p.Track.SameParams(first.Track) {
			break
		}
	}
	if frag, err := mux.Flush(); err == nil && frag != nil {
		if _, err := w.Write(frag); err != nil {
			return err
		}
	}
	return nil
}

func safeFileName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "camera"
	}
	return b.String()
}

// videoStorage reports the space used by recordings (per camera of the organization) and the limit.
func (s *Server) videoStorage(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var all int64
	_ = s.pool.QueryRow(r.Context(), `SELECT COALESCE(sum(bytes), 0) FROM core.video_segment`).Scan(&all)
	p := PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `SELECT c.id::text, c.name, c.removed_at IS NOT NULL, COALESCE(sum(s.bytes), 0), count(s.id), min(s.started_at), c.camera_group
		FROM core.camera c LEFT JOIN core.video_segment s ON s.camera_id = c.id
		WHERE c.org_id = $1::uuid GROUP BY c.id HAVING c.removed_at IS NULL OR count(s.id) > 0 ORDER BY c.name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	cams := []map[string]any{}
	for rows.Next() {
		var id, name string
		var removed bool
		var bytes, files int64
		var oldest *time.Time
		var group string
		if rows.Scan(&id, &name, &removed, &bytes, &files, &oldest, &group) == nil && p.SeesCameraGroup(group) {
			cams = append(cams, map[string]any{"id": id, "name": name, "removed": removed, "bytes": bytes, "files": files, "oldest": oldest})
		}
	}
	out := map[string]any{"cameras": cams, "used_bytes": all, "max_bytes": int64(0)}
	if s.video != nil {
		out["max_bytes"] = s.video.MaxBytes()
	}
	writeJSON(w, 200, out)
}

// ---- video walls ---------------------------------------------------------------------------------------------------

// WallScreen is one monitor of a video wall: a grid of camera tiles (null = empty tile).
type WallScreen struct {
	Name  string    `json:"name"`
	Cols  int       `json:"cols"`
	Rows  int       `json:"rows"`
	Tiles []*string `json:"tiles"`
	// Quality of the tiles: "" or "auto" (sub-stream above the configured number of tiles), "main" or "sub".
	Quality string `json:"quality,omitempty"`
}

type wallIn struct {
	Name    *string      `json:"name"`
	Screens []WallScreen `json:"screens"`
	Reason  string       `json:"reason"`
}

func (s *Server) validateWall(ctx context.Context, org string, screens []WallScreen) string {
	if len(screens) == 0 || len(screens) > 8 {
		return "screens: 1 to 8"
	}
	for i := range screens {
		sc := &screens[i]
		if sc.Cols < 1 || sc.Cols > 8 || sc.Rows < 1 || sc.Rows > 8 {
			return "each screen has 1 to 8 columns and rows"
		}
		if len(sc.Tiles) > sc.Cols*sc.Rows {
			return fmt.Sprintf("screen %d: more tiles than cells", i+1)
		}
		switch sc.Quality {
		case "", "auto", "main", "sub":
		default:
			return "screen quality: auto, main or sub"
		}
		if len([]rune(sc.Name)) > 40 {
			return "screen name: at most 40 characters"
		}
		for _, t := range sc.Tiles {
			if t == nil {
				continue
			}
			var n int
			_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM core.camera WHERE id::text = $1 AND org_id = $2::uuid AND removed_at IS NULL`, *t, org).Scan(&n)
			if n == 0 {
				return "camera " + *t + " not found"
			}
		}
	}
	var ids []string
	for _, sc := range screens {
		for _, t := range sc.Tiles {
			if t != nil {
				ids = append(ids, *t)
			}
		}
	}
	if msg := s.camerasVisible(ctx, org, ids); msg != "" {
		return msg
	}
	return ""
}

func (s *Server) wallJSON(ctx context.Context, org, id string) (map[string]any, bool) {
	var name string
	var screens []byte
	var updated time.Time
	err := s.pool.QueryRow(ctx, `SELECT name, screens, updated_at FROM core.video_wall WHERE id = $1::uuid AND org_id = $2::uuid AND removed_at IS NULL`, id, org).
		Scan(&name, &screens, &updated)
	if err != nil {
		return nil, false
	}
	subAbove := 4
	if s.video != nil {
		subAbove = s.video.Settings().WallSubStreamAbove
	}
	return map[string]any{"id": id, "name": name, "screens": json.RawMessage(screens), "updated_at": updated, "sub_stream_above": subAbove}, true
}

func (s *Server) listWalls(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT id::text FROM core.video_wall WHERE org_id = $1::uuid AND removed_at IS NULL ORDER BY name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	out := []map[string]any{}
	for _, id := range ids {
		if wl, ok := s.wallJSON(r.Context(), org, id); ok {
			out = append(out, wl)
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) getWall(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	wl, ok := s.wallJSON(r.Context(), org, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "video wall not found")
		return
	}
	writeJSON(w, 200, wl)
}

func (s *Server) saveWall(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var in wallIn
	if err := decodeJSON(r, &in, 64<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeErr(w, 400, "reason is required")
		return
	}
	var old map[string]any
	if id != "" {
		if old, ok = s.wallJSON(r.Context(), org, id); !ok {
			writeErr(w, 404, "video wall not found")
			return
		}
		if in.Name == nil {
			n := old["name"].(string)
			in.Name = &n
		}
		if in.Screens == nil {
			_ = json.Unmarshal(old["screens"].(json.RawMessage), &in.Screens)
		}
	}
	if in.Name == nil || strings.TrimSpace(*in.Name) == "" || len([]rune(*in.Name)) > 80 {
		writeErr(w, 400, "name: 1 to 80 characters")
		return
	}
	if msg := s.validateWall(r.Context(), org, in.Screens); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	scr, _ := json.Marshal(in.Screens)
	name := strings.TrimSpace(*in.Name)
	if id == "" {
		by := PrincipalFrom(r.Context()).Username
		if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.video_wall (org_id, name, screens, created_by) VALUES ($1::uuid, $2, $3::jsonb, $4) RETURNING id::text`,
			org, name, string(scr), by).Scan(&id); err != nil {
			writeErr(w, 500, "video wall not stored")
			return
		}
	} else if _, err := s.pool.Exec(r.Context(), `UPDATE core.video_wall SET name = $3, screens = $4::jsonb, updated_at = now() WHERE id = $1::uuid AND org_id = $2::uuid`,
		id, org, name, string(scr)); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	wl, _ := s.wallJSON(r.Context(), org, id)
	code, action := 200, "video_wall.update"
	if old == nil {
		code, action = 201, "video_wall.create"
	}
	s.audit(r, action, "video_wall", id, old, wl, in.Reason)
	writeJSON(w, code, wl)
}

func (s *Server) deleteWall(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	reason := r.URL.Query().Get("reason")
	if strings.TrimSpace(reason) == "" {
		writeErr(w, 400, "reason is required")
		return
	}
	old, ok := s.wallJSON(r.Context(), org, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "video wall not found")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.video_wall SET removed_at = now() WHERE id = $1::uuid`, r.PathValue("id")); err != nil {
		writeErr(w, 500, "delete failed")
		return
	}
	s.audit(r, "video_wall.delete", "video_wall", r.PathValue("id"), old, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

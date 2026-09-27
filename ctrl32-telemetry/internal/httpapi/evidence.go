package httpapi

// Evidence (spec 015): holding recordings so retention and the disk limit never delete them, and signed evidence
// packages (see internal/video/evidence.go).
//
//	GET    /api/v1/video/holds                        video.playback  ?camera_id&all: held ranges (limited users: their cameras)
//	POST   /api/v1/video/holds                        video.evidence  {camera_id, from, to, title, case_ref, reason}
//	DELETE /api/v1/video/holds/{id}?reason=           video.evidence  release
//	GET    /api/v1/video/cameras/{id}/evidence        video.evidence  ?from&to&reason&case_ref: signed ZIP (≤ 1 h)
//	GET    /api/v1/video/evidence/key                 video.playback  the server's public key and its fingerprint

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/video"
)

func (s *Server) listHolds(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	cam := r.URL.Query().Get("camera_id")
	all := r.URL.Query().Get("all") == "1"
	rows, err := s.pool.Query(r.Context(), `SELECT h.id::text, h.camera_id::text, c.name, c.camera_group, h.from_ts, h.to_ts, h.title, h.case_ref, h.reason,
			h.created_by, h.created_at, h.released_at, COALESCE(h.released_by, ''), COALESCE(h.release_reason, ''),
			(SELECT COALESCE(sum(s.bytes), 0) FROM core.video_segment s WHERE s.camera_id = h.camera_id AND s.started_at < h.to_ts AND s.ended_at > h.from_ts)
		FROM core.video_hold h JOIN core.camera c ON c.id = h.camera_id
		WHERE h.org_id = $1::uuid AND ($2 = '' OR h.camera_id::text = $2) AND ($3 OR h.released_at IS NULL)
		ORDER BY h.created_at DESC LIMIT 500`, org, cam, all)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, camID, name, group, title, caseRef, reason, by, relBy, relReason string
		var from, to, created time.Time
		var released *time.Time
		var bytes int64
		if rows.Scan(&id, &camID, &name, &group, &from, &to, &title, &caseRef, &reason, &by, &created, &released, &relBy, &relReason, &bytes) != nil || !p.SeesCameraGroup(group) {
			continue
		}
		out = append(out, map[string]any{"id": id, "camera_id": camID, "camera": name, "from": from, "to": to, "title": title, "case_ref": caseRef,
			"reason": reason, "created_by": by, "created_at": created, "released_at": released, "released_by": relBy, "release_reason": relReason, "bytes": bytes})
	}
	writeJSON(w, 200, out)
}

func (s *Server) createHold(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		CameraID string    `json:"camera_id"`
		From     time.Time `json:"from"`
		To       time.Time `json:"to"`
		Title    string    `json:"title"`
		CaseRef  string    `json:"case_ref"`
		Reason   string    `json:"reason"`
	}
	if err := decodeJSON(r, &in, 8<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	c, ok := s.cameraRow(r.Context(), org, in.CameraID)
	if !ok {
		writeErr(w, 404, "camera not found")
		return
	}
	in.Title, in.CaseRef = strings.TrimSpace(in.Title), strings.TrimSpace(in.CaseRef)
	switch {
	case in.Title == "" || len([]rune(in.Title)) > 120:
		writeErr(w, 400, "title: 1 to 120 characters")
		return
	case len([]rune(in.CaseRef)) > 80:
		writeErr(w, 400, "case_ref: at most 80 characters")
		return
	case !in.To.After(in.From) || in.To.Sub(in.From) > 31*24*time.Hour:
		writeErr(w, 400, "from before to, at most 31 days")
		return
	case in.To.After(time.Now().Add(24 * time.Hour)):
		writeErr(w, 400, "to: at most a day ahead")
		return
	}
	p := PrincipalFrom(r.Context())
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.video_hold (org_id, camera_id, from_ts, to_ts, title, case_ref, reason, created_by)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8) RETURNING id::text`, org, c.id, in.From, in.To, in.Title, in.CaseRef, in.Reason, p.Username).Scan(&id); err != nil {
		writeErr(w, 500, "hold not stored")
		return
	}
	s.audit(r, "video.hold", "camera", c.id, nil, map[string]any{"hold": id, "camera": c.name, "from": in.From, "to": in.To, "title": in.Title, "case_ref": in.CaseRef}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id, "camera_id": c.id, "from": in.From, "to": in.To, "title": in.Title, "case_ref": in.CaseRef})
}

func (s *Server) releaseHold(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	var camID, title string
	if err := s.pool.QueryRow(r.Context(), `SELECT camera_id::text, title FROM core.video_hold WHERE id::text = $1 AND org_id = $2::uuid AND released_at IS NULL`,
		r.PathValue("id"), org).Scan(&camID, &title); err != nil {
		writeErr(w, 404, "hold not found")
		return
	}
	c, ok := s.cameraRow(r.Context(), org, camID)
	if !ok {
		writeErr(w, 404, "hold not found")
		return
	}
	p := PrincipalFrom(r.Context())
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.video_hold SET released_at = now(), released_by = $2, release_reason = $3 WHERE id::text = $1`,
		r.PathValue("id"), p.Username, reason); err != nil {
		writeErr(w, 500, "release failed")
		return
	}
	s.audit(r, "video.hold_release", "camera", c.id, map[string]any{"hold": r.PathValue("id"), "title": title}, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) evidenceKey(w http.ResponseWriter, r *http.Request) {
	if s.video == nil {
		writeErr(w, 503, "video is not running")
		return
	}
	key, err := s.video.EvidenceKey(r.Context())
	if err != nil {
		writeErr(w, 500, "evidence key unavailable: "+err.Error())
		return
	}
	pub := key.Public().(ed25519.PublicKey)
	writeJSON(w, 200, map[string]any{"algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(pub), "fingerprint": video.KeyFingerprint(pub)})
}

// cameraEvidence builds the signed package in a temporary file first, so the audit entry carries the video's hash
// before anything leaves the server.
func (s *Server) cameraEvidence(w http.ResponseWriter, r *http.Request) {
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
	reason, caseRef := strings.TrimSpace(q.Get("reason")), strings.TrimSpace(q.Get("case_ref"))
	if !reasonOf(w, reason) {
		return
	}
	if len([]rune(caseRef)) > 80 || len([]rune(reason)) > 500 {
		writeErr(w, 400, "case_ref at most 80, reason at most 500 characters")
		return
	}
	from, to, msg := parseRange(q, time.Hour)
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
	segs, err := s.video.EvidenceSegments(r.Context(), c.id, from, to)
	if err != nil {
		writeErr(w, 500, "recorded files not listed")
		return
	}
	p := PrincipalFrom(r.Context())
	var orgName string
	_ = s.pool.QueryRow(r.Context(), `SELECT name FROM core.organization WHERE id = $1::uuid`, org).Scan(&orgName)
	events := []map[string]any{}
	if rows, err := s.pool.Query(r.Context(), `SELECT kind, source, label, started_at, ended_at FROM core.camera_event
		WHERE camera_id = $1::uuid AND started_at < $3 AND coalesce(ended_at, now()) >= $2 ORDER BY started_at LIMIT 1000`, c.id, from, to); err == nil {
		for rows.Next() {
			var kind, src, label string
			var st time.Time
			var en *time.Time
			if rows.Scan(&kind, &src, &label, &st, &en) == nil {
				events = append(events, map[string]any{"kind": kind, "source": src, "label": label, "started_at": st, "ended_at": en})
			}
		}
		rows.Close()
	}
	man := video.EvidenceManifest{
		Server:       map[string]string{"base_url": s.cfg.HTTP.BaseURL, "version": s.version},
		Organization: orgName,
		Camera:       map[string]string{"id": c.id, "name": c.name, "location": c.location},
		From:         from, To: to, ExportedAt: time.Now().UTC(),
		ExportedBy: map[string]string{"username": p.Username, "full_name": p.FullName, "user_id": p.UserID},
		Reason:     reason, CaseRef: caseRef, Segments: segs, Events: events,
	}
	tmp, err := os.CreateTemp(s.video.Dir(), ".evidence-*.zip")
	if err != nil {
		writeErr(w, 500, "no room for the package")
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	man, err = s.video.WriteEvidence(r.Context(), tmp, man, func(out io.Writer) error { return writeMP4(r.Context(), pb, first, out) })
	if err != nil {
		writeErr(w, 500, "package failed: "+err.Error())
		return
	}
	st, _ := tmp.Stat()
	s.audit(r, "video.evidence_export", "camera", c.id, nil, map[string]any{"camera": c.name, "from": from, "to": to, "case_ref": caseRef,
		"video_sha256": man.Video["sha256"], "files": len(segs), "key_fingerprint": man.KeyFP}, reason)
	if _, err := tmp.Seek(0, 0); err != nil {
		return
	}
	name := fmt.Sprintf("evidence-%s-%s.zip", safeFileName(c.name), from.UTC().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Length", fmt.Sprint(st.Size()))
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
	_, _ = io.Copy(w, tmp)
}

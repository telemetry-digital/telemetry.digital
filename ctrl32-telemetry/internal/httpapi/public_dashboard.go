package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

// Public dashboards: an organization can publish a dashboard read-only at /p/<token> (anyone with the link, no
// sign-in). The token is bound to the dashboard slug and exposes only what the dashboard's widgets bind: the
// datastreams, the devices (all devices when a map/devices widget is unfiltered) and the alarms of those
// datastreams. Data requests are delegated to the ordinary handlers under a synthetic read-only principal of the
// organization, with the ids checked against the bindings first. One active token per dashboard; regenerating or
// disabling revokes the previous one (rows stay for the audit trail).

var shareTokenRe = regexp.MustCompile(`^[A-Za-z0-9_-]{32,64}$`)

type shareInfo struct {
	Org, Slug, Name string
	Spec            map[string]any
	DS              map[string]bool
	Devs            map[string]bool
	AllDevs         bool
	HasAlarms       bool
	Cams            map[string]bool // camera widgets; only a paired display ever plays them
}

func (s *Server) resolveShare(ctx context.Context, token string) (*shareInfo, bool) {
	if !shareTokenRe.MatchString(token) {
		return nil, false
	}
	var org, slug string
	if err := s.pool.QueryRow(ctx, `SELECT org_id::text, slug FROM core.dashboard_share WHERE token = $1 AND revoked_at IS NULL`, token).Scan(&org, &slug); err != nil {
		return nil, false
	}
	return s.shareOf(ctx, org, slug)
}

// shareOf reads the current version of a dashboard and what its widgets may read (public links and displays).
func (s *Server) shareOf(ctx context.Context, org, slug string) (*shareInfo, bool) {
	si := &shareInfo{DS: map[string]bool{}, Devs: map[string]bool{}, Cams: map[string]bool{}}
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT org_id::text, slug, name, spec FROM core.dashboard WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL`, org, slug).
		Scan(&si.Org, &si.Slug, &si.Name, &raw)
	if err != nil {
		return nil, false
	}
	if json.Unmarshal(raw, &si.Spec) != nil {
		return nil, false
	}
	si.collect()
	return si, true
}

// collect reads from the spec what its widgets and drawing elements may read.
func (si *shareInfo) collect() {
	widgets, _ := si.Spec["widgets"].([]any)
	for _, x := range widgets {
		w, _ := x.(map[string]any)
		typ, _ := w["type"].(string)
		binds, _ := w["bindings"].(map[string]any)
		if ds, ok := binds["datastreams"].([]any); ok {
			for _, id := range ds {
				if str, ok := id.(string); ok {
					si.DS[str] = true
				}
			}
		}
		if dv, ok := binds["devices"].([]any); ok {
			if len(dv) == 0 && (typ == "map" || typ == "devices") {
				si.AllDevs = true
			}
			for _, id := range dv {
				if str, ok := id.(string); ok {
					si.Devs[str] = true
				}
			}
		}
		// the Alarms list and the Alarm count read the same public alarms: those of the dashboard's datastreams
		if typ == "alarms" || typ == "alarm_count" {
			si.HasAlarms = true
		}
		if typ == "camera" || typ == "camera_pin" {
			params, _ := w["params"].(map[string]any)
			if id, ok := params["camera"].(string); ok && id != "" {
				si.Cams[id] = true
			}
		}
	}
	// Drawing elements of a process picture: their tags and the tags of their dynamics are readable too.
	elems, _ := si.Spec["elements"].([]any)
	for _, x := range elems {
		e, _ := x.(map[string]any)
		if t, ok := e["tag"].(string); ok && t != "" {
			si.DS[t] = true
		}
		dyn, _ := e["dyn"].([]any)
		for _, y := range dyn {
			d, _ := y.(map[string]any)
			if t, ok := d["tag"].(string); ok && t != "" {
				si.DS[t] = true
			}
		}
	}
}

// withPublicPrincipal delegates to an ordinary data handler under a read-only principal of the organization.
func (s *Server) withPublicPrincipal(r *http.Request, si *shareInfo) *http.Request {
	p := &iam.Principal{OrgID: si.Org, Username: "public:" + si.Slug, Permissions: map[string]bool{"data.read": true}}
	return r.WithContext(context.WithValue(r.Context(), principalKey, p))
}

// delegateFiltered runs a list handler for the organization and keeps only the rows the filter accepts.
func (s *Server) delegateFiltered(w http.ResponseWriter, r *http.Request, si *shareInfo, h http.HandlerFunc, keep func(row map[string]any) bool) {
	rec := httptest.NewRecorder()
	h(rec, s.withPublicPrincipal(r, si))
	if rec.Code != 200 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
		return
	}
	var rows []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		writeErr(w, 500, "unexpected list format")
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if keep(row) {
			out = append(out, row)
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) publicShare(w http.ResponseWriter, r *http.Request) (*shareInfo, bool) {
	w.Header().Set("Cache-Control", "no-store")
	token := r.PathValue("token")
	var si *shareInfo
	var ok bool
	if slug, isDisplay := strings.CutPrefix(token, "~"); isDisplay {
		// a paired display (its cookie) reads the dashboards it was given, like a public link (displays.go)
		si, ok = s.displayShare(r, slug)
	} else {
		si, ok = s.resolveShare(r.Context(), token)
	}
	if !ok {
		writeErr(w, 404, "this link is no longer valid")
	}
	return si, ok
}

// GET /api/v1/public/{token} → the dashboard (name, slug, spec); no organization data beyond the spec.
func (s *Server) publicDashboard(w http.ResponseWriter, r *http.Request) {
	si, ok := s.publicShare(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, map[string]any{"slug": si.Slug, "name": si.Name, "spec": si.Spec})
}

func (s *Server) publicDatastreams(w http.ResponseWriter, r *http.Request) {
	si, ok := s.publicShare(w, r)
	if !ok {
		return
	}
	s.delegateFiltered(w, r, si, s.datastreams, func(row map[string]any) bool { id, _ := row["id"].(string); return si.DS[id] })
}

func (s *Server) publicLatest(w http.ResponseWriter, r *http.Request) {
	si, ok := s.publicShare(w, r)
	if !ok {
		return
	}
	for _, id := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if id = strings.TrimSpace(id); id != "" && !si.DS[id] {
			writeErr(w, 403, "datastream is not part of this dashboard")
			return
		}
	}
	s.latestValues(w, s.withPublicPrincipal(r, si))
}

func (s *Server) publicObservations(w http.ResponseWriter, r *http.Request) {
	si, ok := s.publicShare(w, r)
	if !ok {
		return
	}
	if !si.DS[r.URL.Query().Get("datastream_id")] {
		writeErr(w, 403, "datastream is not part of this dashboard")
		return
	}
	s.observations(w, s.withPublicPrincipal(r, si))
}

func (s *Server) publicDevices(w http.ResponseWriter, r *http.Request) {
	si, ok := s.publicShare(w, r)
	if !ok {
		return
	}
	s.delegateFiltered(w, r, si, s.devices, func(row map[string]any) bool {
		id, _ := row["id"].(string)
		return si.AllDevs || si.Devs[id]
	})
}

func (s *Server) publicAlarms(w http.ResponseWriter, r *http.Request) {
	si, ok := s.publicShare(w, r)
	if !ok {
		return
	}
	out := []map[string]any{}
	if !si.HasAlarms || len(si.DS) == 0 {
		writeJSON(w, 200, out)
		return
	}
	ids := make([]string, 0, len(si.DS))
	for id := range si.DS {
		ids = append(ids, id)
	}
	rows, err := s.pool.Query(r.Context(), `SELECT a.id::text, a.raised_at, a.cleared_at, a.state, a.severity, a.retrospective, a.value::float8, a.detail->>'key'
		FROM core.alarm a WHERE a.org_id = $1::uuid AND a.datastream_id = ANY($2::uuid[]) ORDER BY a.raised_at DESC LIMIT 100`, si.Org, ids)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, state, sev string
		var raised time.Time
		var cleared *time.Time
		var retro bool
		var val *float64
		var key *string
		if rows.Scan(&id, &raised, &cleared, &state, &sev, &retro, &val, &key) == nil {
			out = append(out, map[string]any{"id": id, "raised_at": raised, "cleared_at": cleared, "state": state, "severity": sev, "retrospective": retro, "value": val, "key": key})
		}
	}
	writeJSON(w, 200, out)
}

// ---- share management (content.write) ------------------------------------------------------------

func (s *Server) shareURL(token string) string {
	return strings.TrimRight(s.cfg.HTTP.BaseURL, "/") + "/p/" + token
}

func (s *Server) sharePayload(enabled bool, token string, created *time.Time) map[string]any {
	out := map[string]any{"enabled": enabled}
	if enabled {
		url := s.shareURL(token)
		out["token"] = token
		out["url"] = url
		out["created_at"] = created
		if png, err := qrcode.Encode(url, qrcode.Medium, 256); err == nil {
			out["qr"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
		}
	}
	return out
}

func (s *Server) dashboardIDForShare(w http.ResponseWriter, r *http.Request, org, slug string) (string, bool) {
	var id string
	if err := s.pool.QueryRow(r.Context(), `SELECT id::text FROM core.dashboard WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL`, org, slug).Scan(&id); err != nil {
		writeErr(w, 404, "dashboard not found")
		return "", false
	}
	return id, true
}

// GET /api/v1/dashboards/{slug}/share → {enabled, url, token, qr, created_at}
func (s *Server) getShare(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	slug := r.PathValue("slug")
	if _, ok := s.dashboardIDForShare(w, r, org, slug); !ok {
		return
	}
	var token string
	var created time.Time
	err := s.pool.QueryRow(r.Context(), `SELECT token, created_at FROM core.dashboard_share WHERE org_id = $1::uuid AND slug = $2 AND revoked_at IS NULL`, org, slug).Scan(&token, &created)
	if err != nil {
		writeJSON(w, 200, s.sharePayload(false, "", nil))
		return
	}
	writeJSON(w, 200, s.sharePayload(true, token, &created))
}

// POST /api/v1/dashboards/{slug}/share {reason} → creates the public link (revokes a previous one).
func (s *Server) createShare(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	slug := r.PathValue("slug")
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
	dashID, ok := s.dashboardIDForShare(w, r, org, slug)
	if !ok {
		return
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		writeErr(w, 500, "random failed")
		return
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, 500, "begin failed")
		return
	}
	defer tx.Rollback(r.Context())
	var prev *string
	var old string
	if tx.QueryRow(r.Context(), `UPDATE core.dashboard_share SET revoked_at = now() WHERE org_id = $1::uuid AND slug = $2 AND revoked_at IS NULL RETURNING token`, org, slug).Scan(&old) == nil {
		prev = &old
	}
	var created time.Time
	if err := tx.QueryRow(r.Context(), `INSERT INTO core.dashboard_share (org_id, slug, token, created_by, reason) VALUES ($1::uuid, $2, $3, NULLIF($4,'')::uuid, $5) RETURNING created_at`, org, slug, token, p.UserID, in.Reason).Scan(&created); err != nil {
		writeErr(w, 500, "insert failed")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, 500, "commit failed")
		return
	}
	var oldV any
	if prev != nil {
		oldV = map[string]any{"public": true, "token_prefix": (*prev)[:8]}
	}
	s.audit(r, "dashboard.share", "dashboard", dashID, oldV, map[string]any{"public": true, "slug": slug, "token_prefix": token[:8], "url": s.shareURL(token)}, in.Reason)
	writeJSON(w, 201, s.sharePayload(true, token, &created))
}

// DELETE /api/v1/dashboards/{slug}/share?reason= → disables the public link.
func (s *Server) deleteShare(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	slug := r.PathValue("slug")
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	dashID, ok := s.dashboardIDForShare(w, r, org, slug)
	if !ok {
		return
	}
	var old string
	if err := s.pool.QueryRow(r.Context(), `UPDATE core.dashboard_share SET revoked_at = now() WHERE org_id = $1::uuid AND slug = $2 AND revoked_at IS NULL RETURNING token`, org, slug).Scan(&old); err != nil {
		writeErr(w, 404, "no public link")
		return
	}
	s.audit(r, "dashboard.unshare", "dashboard", dashID, map[string]any{"public": true, "token_prefix": old[:8]}, map[string]any{"public": false, "slug": slug}, reason)
	writeJSON(w, 200, s.sharePayload(false, "", nil))
}

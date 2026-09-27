package httpapi

// Camera groups (spec 014): a camera belongs to at most one group; a user may be limited to some groups and then
// sees only those cameras everywhere (cameraRow enforces it for every camera endpoint).
//
//	GET /api/v1/video/camera-groups             video.view   groups in use in the organization (limited users: theirs)
//	GET /api/v1/users/{id}/camera-groups        user.admin   {limited, groups}
//	PUT /api/v1/users/{id}/camera-groups        user.admin   {limited, groups, reason}

import (
	"context"
	"net/http"
	"slices"
	"strings"
)

func (s *Server) cameraGroups(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `SELECT camera_group, count(*) FROM core.camera WHERE org_id = $1::uuid AND removed_at IS NULL
		GROUP BY camera_group ORDER BY camera_group`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var g string
		var n int
		if rows.Scan(&g, &n) == nil && p.SeesCameraGroup(g) {
			out = append(out, map[string]any{"group": g, "cameras": n})
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) userCameraGroups(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	u, err := s.iam.GetUser(r.Context(), r.PathValue("id"))
	if err != nil || u.OrgID != p.OrgID {
		writeErr(w, 404, "user not found")
		return
	}
	var limited bool
	var groups []string
	if err := s.pool.QueryRow(r.Context(), `SELECT camera_groups IS NOT NULL, COALESCE(camera_groups, '{}') FROM core.user_account WHERE id = $1::uuid`, u.ID).Scan(&limited, &groups); err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, map[string]any{"limited": limited, "groups": groups})
}

func (s *Server) putUserCameraGroups(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	u, err := s.iam.GetUser(r.Context(), r.PathValue("id"))
	if err != nil || u.OrgID != p.OrgID {
		writeErr(w, 404, "user not found")
		return
	}
	var in struct {
		Limited bool     `json:"limited"`
		Groups  []string `json:"groups"`
		Reason  string   `json:"reason"`
	}
	if err := decodeJSON(r, &in, 8<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	groups := []string{}
	for _, g := range in.Groups {
		g = strings.TrimSpace(g)
		if len([]rune(g)) > 60 {
			writeErr(w, 400, "groups: at most 60 characters each")
			return
		}
		if !slices.Contains(groups, g) {
			groups = append(groups, g)
		}
	}
	if len(groups) > 100 {
		writeErr(w, 400, "groups: at most 100")
		return
	}
	// an administrator limited to some groups cannot hand out others
	if p.CameraLimited {
		if !in.Limited {
			writeErr(w, 403, "you are limited to some camera groups yourself")
			return
		}
		for _, g := range groups {
			if !p.SeesCameraGroup(g) {
				writeErr(w, 403, "group "+g+": not one of yours")
				return
			}
		}
	}
	var oldLimited bool
	var oldGroups []string
	_ = s.pool.QueryRow(r.Context(), `SELECT camera_groups IS NOT NULL, COALESCE(camera_groups, '{}') FROM core.user_account WHERE id = $1::uuid`, u.ID).Scan(&oldLimited, &oldGroups)
	var arg any // nil = NULL = all cameras
	if in.Limited {
		arg = groups // never nil: an empty list means no cameras
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.user_account SET camera_groups = $2::text[] WHERE id = $1::uuid`, u.ID, arg); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	s.audit(r, "user.camera_groups", "user", u.ID, map[string]any{"limited": oldLimited, "groups": oldGroups},
		map[string]any{"user": u.Username, "limited": in.Limited, "groups": groups}, in.Reason)
	writeJSON(w, 200, map[string]any{"limited": in.Limited, "groups": groups})
}

// camerasVisible returns an error text when a camera among ids belongs to a group the principal in ctx may not see.
// Walls, dashboards and displays call it, so a user limited to camera groups cannot route other cameras through them.
func (s *Server) camerasVisible(ctx context.Context, org string, ids []string) string {
	p := PrincipalFrom(ctx)
	if p == nil || !p.CameraLimited || len(ids) == 0 {
		return ""
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text, camera_group FROM core.camera WHERE org_id = $1::uuid AND id::text = ANY($2::text[])`, org, ids)
	if err != nil {
		return "cameras could not be checked"
	}
	defer rows.Close()
	for rows.Next() {
		var id, g string
		if rows.Scan(&id, &g) == nil && !p.SeesCameraGroup(g) {
			return "camera " + id + " is outside your camera groups"
		}
	}
	return ""
}

// specCameras lists the cameras of the camera widgets of a dashboard spec.
func specCameras(spec map[string]any) []string {
	var ids []string
	// a validated spec holds []map[string]any, a stored one (decoded JSON) []any
	var widgets []map[string]any
	switch ws := spec["widgets"].(type) {
	case []map[string]any:
		widgets = ws
	case []any:
		for _, x := range ws {
			if w, ok := x.(map[string]any); ok {
				widgets = append(widgets, w)
			}
		}
	}
	for _, w := range widgets {
		if t, _ := w["type"].(string); t != "camera" && t != "camera_pin" {
			continue
		}
		params, _ := w["params"].(map[string]any)
		if id, ok := params["camera"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

package httpapi

// Global search (0.68.0): GET /api/v1/search?q=…&limit=… finds devices, datastreams, assets, sites, dashboards,
// cameras, flows, incidents and users of the caller's organization by name — each kind only with the permission that
// shows it elsewhere (data.read; cameras video.view and the user's camera groups; users user.admin). At most `limit`
// hits per kind (default 5, at most 20); "more" names the kinds that have further hits. The header search field and
// Ctrl+K open it in the UI (static/app.js).

import (
	"context"
	"net/http"
	"strconv"
	"strings"
)

type searchHit struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	URL      string `json:"url"`
	group    string
}

type searchKind struct {
	kind, perm, sql string
	row             func(cols []string) searchHit
}

// Each query takes $1 org, $2 the contains-pattern, $3 the prefix-pattern (ranking) and $4 the row limit; it returns
// text columns for row().
var searchKinds = []searchKind{
	{"device", "data.read", `SELECT id::text, name, external_id, transport FROM core.device WHERE org_id = $1::uuid AND revoked_at IS NULL
		AND (name ILIKE $2 OR external_id ILIKE $2) ORDER BY (name ILIKE $3) DESC, name LIMIT $4`,
		func(c []string) searchHit {
			return searchHit{ID: c[0], Title: c[1], Subtitle: c[2] + " · " + c[3], URL: "/devices/" + c[0]}
		}},
	{"datastream", "data.read", `SELECT ds.id::text, a.name || ' / ' || ds.key, st.name, ds.quantity, ds.unit, a.id::text FROM core.datastream ds
		JOIN core.asset a ON a.id = ds.asset_id JOIN core.site st ON st.id = a.site_id WHERE ds.org_id = $1::uuid
		AND (ds.key ILIKE $2 OR a.name ILIKE $2 OR (a.name || ' / ' || ds.key) ILIKE $2 OR ds.quantity ILIKE $2 OR COALESCE(ds.attributes->>'name', '') ILIKE $2)
		ORDER BY (ds.key ILIKE $3 OR a.name ILIKE $3) DESC, a.name, ds.key LIMIT $4`,
		func(c []string) searchHit {
			sub := c[2] + " · " + c[3]
			if c[4] != "" {
				sub += " [" + c[4] + "]"
			}
			return searchHit{ID: c[0], Title: c[1], Subtitle: sub, URL: "/assets/" + c[5] + "/datastreams?ds=" + c[0]}
		}},
	{"asset", "data.read", `SELECT a.id::text, a.name, a.type, st.name, st.id::text FROM core.asset a JOIN core.site st ON st.id = a.site_id
		WHERE a.org_id = $1::uuid AND (a.name ILIKE $2 OR a.type ILIKE $2) ORDER BY (a.name ILIKE $3) DESC, a.name LIMIT $4`,
		func(c []string) searchHit {
			return searchHit{ID: c[0], Title: c[1], Subtitle: c[2] + " · " + c[3], URL: "/assets/" + c[0]}
		}},
	{"site", "data.read", `SELECT id::text, name, COALESCE(address, '') FROM core.site WHERE org_id = $1::uuid AND (name ILIKE $2 OR COALESCE(address, '') ILIKE $2)
		ORDER BY (name ILIKE $3) DESC, name LIMIT $4`,
		func(c []string) searchHit {
			return searchHit{ID: c[0], Title: c[1], Subtitle: c[2], URL: "/assets/sites/" + c[0]}
		}},
	{"dashboard", "data.read", `SELECT id::text, name, slug, COALESCE(spec->>'kind', 'dashboard') FROM core.dashboard WHERE org_id = $1::uuid AND valid_to IS NULL
		AND (name ILIKE $2 OR slug ILIKE $2) ORDER BY (name ILIKE $3) DESC, name LIMIT $4`,
		func(c []string) searchHit {
			sub := "dashboard"
			if c[3] == "mimic" {
				sub = "process picture"
			}
			return searchHit{ID: c[0], Title: c[1], Subtitle: sub, URL: "/dashboards/" + c[2]}
		}},
	{"camera", "video.view", `SELECT id::text, name, location, camera_group FROM core.camera WHERE org_id = $1::uuid AND removed_at IS NULL
		AND (name ILIKE $2 OR location ILIKE $2) ORDER BY (name ILIKE $3) DESC, sort, name LIMIT $4`,
		func(c []string) searchHit {
			return searchHit{ID: c[0], Title: c[1], Subtitle: c[2], URL: "/video/cameras/" + c[0], group: c[3]}
		}},
	{"flow", "data.read", `SELECT id::text, name, slug, state FROM core.flow WHERE org_id = $1::uuid AND valid_to IS NULL AND (name ILIKE $2 OR slug ILIKE $2)
		ORDER BY (name ILIKE $3) DESC, name LIMIT $4`,
		func(c []string) searchHit {
			return searchHit{ID: c[0], Title: c[1], Subtitle: c[3], URL: "/flows/" + c[2]}
		}},
	{"incident", "data.read", `SELECT id::text, title, status, severity FROM core.incident WHERE org_id = $1::uuid AND title ILIKE $2
		ORDER BY (status IN ('open', 'investigating')) DESC, (title ILIKE $3) DESC, opened_at DESC LIMIT $4`,
		func(c []string) searchHit {
			return searchHit{ID: c[0], Title: c[1], Subtitle: c[2] + " · " + c[3], URL: "/incidents?id=" + c[0]}
		}},
	{"user", "user.admin", `SELECT id::text, username, full_name, COALESCE(email, '') FROM core.user_account WHERE org_id = $1::uuid
		AND (username ILIKE $2 OR full_name ILIKE $2 OR COALESCE(email, '') ILIKE $2) ORDER BY (username ILIKE $3 OR full_name ILIKE $3) DESC, username LIMIT $4`,
		func(c []string) searchHit {
			sub := c[2]
			if c[3] != "" {
				sub += " · " + c[3]
			}
			return searchHit{ID: c[0], Title: c[1], Subtitle: sub, URL: "/users?id=" + c[0]}
		}},
}

func (s *Server) searchKind(ctx context.Context, k searchKind, org, contains, prefix string, limit int) ([]searchHit, bool, error) {
	n := limit + 1
	if k.kind == "camera" {
		n = limit*4 + 1 // some may be outside the user's camera groups
	}
	rows, err := s.pool.Query(ctx, k.sql, org, contains, prefix, n)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	p := PrincipalFrom(ctx)
	var out []searchHit
	more := false
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, false, err
		}
		cols := make([]string, len(vals))
		for i, v := range vals {
			if sv, ok := v.(string); ok {
				cols[i] = sv
			}
		}
		h := k.row(cols)
		if k.kind == "camera" && !p.SeesCameraGroup(h.group) {
			continue
		}
		if len(out) == limit {
			more = true
			break
		}
		h.Kind = k.kind
		out = append(out, h)
	}
	return out, more, rows.Err()
}

// GET /api/v1/search?q=&limit=&kinds=
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	v := r.URL.Query()
	q := strings.TrimSpace(v.Get("q"))
	if q == "" || len([]rune(q)) > 100 {
		writeErr(w, 400, "q: 1..100 characters")
		return
	}
	limit := 5
	if v.Get("limit") != "" {
		n, err := strconv.Atoi(v.Get("limit"))
		if err != nil || n < 1 || n > 20 {
			writeErr(w, 400, "limit must be 1..20")
			return
		}
		limit = n
	}
	only := map[string]bool{}
	for _, k := range splitList(v.Get("kinds")) {
		only[k] = true
	}
	results := []searchHit{}
	more := []string{}
	for _, k := range searchKinds {
		if (len(only) > 0 && !only[k.kind]) || !p.Has(k.perm) {
			continue
		}
		hits, m, err := s.searchKind(r.Context(), k, org, likeContains(q), likeRepl.Replace(q)+"%", limit)
		if err != nil {
			writeErr(w, 500, "search failed")
			return
		}
		results = append(results, hits...)
		if m {
			more = append(more, k.kind)
		}
	}
	writeJSON(w, 200, map[string]any{"q": q, "results": results, "more": more})
}

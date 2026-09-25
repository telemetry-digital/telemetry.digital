package httpapi

// HMI object library (spec 001, US6; contracts/hmi-objects-api.md). Versioned content of the organization:
//   kind "svg"       — customer graphics, sanitised on import (svgsanitize.go), served with a sandbox CSP and shown
//                      in pictures through <image> (element type "image", src = slug)
//   kind "composite" — a group of drawing elements with tag placeholders ("$name"), copied into a picture on placement
//
//	GET    /api/v1/hmi/objects?kind=                    (data.read)      current versions
//	GET    /api/v1/hmi/objects/{slug}                   (data.read)
//	POST   /api/v1/hmi/objects                          (content.write)  {name, slug?, kind, category?, body, reason}
//	POST   /api/v1/hmi/objects/import-svg               (content.write)  {name, slug?, category?, svg, reason}
//	PUT    /api/v1/hmi/objects/{slug}                   (content.write)  {name?, category?, body, reason} → new version
//	DELETE /api/v1/hmi/objects/{slug}?reason=           (content.write)  retire the current version
//	GET    /api/v1/hmi/objects/{slug}/svg               (data.read)      the sanitised SVG
//	GET    /api/v1/public/{token}/hmi/objects/{slug}/svg                 same, only if the linked picture uses it

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var placeholderRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

type hmiObject struct {
	Slug      string          `json:"slug"`
	Kind      string          `json:"kind"`
	Name      string          `json:"name"`
	Category  string          `json:"category"`
	Version   int             `json:"version"`
	Body      json.RawMessage `json:"body,omitempty"`
	Checksum  string          `json:"checksum,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	Reason    string          `json:"reason,omitempty"`
	Width     float64         `json:"width"`
	Height    float64         `json:"height"`
}

// validateHMIBody normalizes the body of a library object.
func validateHMIBody(kind string, raw json.RawMessage) (map[string]any, error) {
	switch kind {
	case "svg":
		var b struct {
			SVG string `json:"svg"`
		}
		if json.Unmarshal(raw, &b) != nil || b.SVG == "" {
			return nil, errors.New("body.svg required")
		}
		clean, err := sanitizeSVG(b.SVG)
		if err != nil {
			return nil, err
		}
		return map[string]any{"svg": clean.SVG, "width": clean.Width, "height": clean.Height}, nil
	case "composite":
		var b struct {
			Elements []any    `json:"elements"`
			Tags     []string `json:"tags"`
			Width    float64  `json:"width"`
			Height   float64  `json:"height"`
		}
		if json.Unmarshal(raw, &b) != nil || len(b.Elements) == 0 {
			return nil, errors.New("body.elements required")
		}
		if len(b.Elements) > 500 {
			return nil, errors.New("a composite has at most 500 elements")
		}
		if b.Width <= 0 || b.Height <= 0 || b.Width > 4000 || b.Height > 3000 {
			return nil, errors.New("body.width and body.height must be 1..4000 × 1..3000")
		}
		if len(b.Tags) > 24 {
			return nil, errors.New("at most 24 tag placeholders")
		}
		// Placeholders "$name" become stand-in ids for the element validation and are restored afterwards.
		stand := map[string]string{}
		back := map[string]string{}
		for i, t := range b.Tags {
			if !placeholderRe.MatchString(t) || stand["$"+t] != "" {
				return nil, fmt.Errorf("tag placeholder %q must be unique, lowercase letters, digits and _", t)
			}
			id := fmt.Sprintf("00000000-0000-7000-8000-%012d", i+1)
			stand["$"+t], back[id] = id, "$"+t
		}
		swap := func(m map[string]any, key string) error {
			if v, ok := m[key].(string); ok && strings.HasPrefix(v, "$") {
				if stand[v] == "" {
					return fmt.Errorf("tag placeholder %s is not listed in body.tags", v)
				}
				m[key] = stand[v]
			}
			return nil
		}
		for _, x := range b.Elements {
			e, _ := x.(map[string]any)
			if e == nil {
				continue
			}
			if err := swap(e, "tag"); err != nil {
				return nil, err
			}
			dyn, _ := e["dyn"].([]any)
			for _, y := range dyn {
				if d, ok := y.(map[string]any); ok {
					if err := swap(d, "tag"); err != nil {
						return nil, err
					}
				}
			}
		}
		els, err := validateElements(b.Elements, int(b.Width), int(b.Height))
		if err != nil {
			return nil, err
		}
		for _, e := range els {
			if t, ok := e["tag"].(string); ok && back[t] != "" {
				e["tag"] = back[t]
			}
			if dyn, ok := e["dyn"].([]map[string]any); ok {
				for _, d := range dyn {
					if t, ok := d["tag"].(string); ok && back[t] != "" {
						d["tag"] = back[t]
					}
				}
			}
		}
		tags := b.Tags
		if tags == nil {
			tags = []string{}
		}
		return map[string]any{"elements": els, "tags": tags, "width": b.Width, "height": b.Height}, nil
	}
	return nil, errors.New("kind must be svg or composite")
}

func (s *Server) listHMIObjects(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	kind := r.URL.Query().Get("kind")
	rows, err := s.pool.Query(r.Context(), `SELECT slug, kind, name, category, version, created_at, coalesce((body->>'width')::float8, 0), coalesce((body->>'height')::float8, 0)
		FROM core.hmi_object WHERE org_id = $1::uuid AND valid_to IS NULL AND ($2 = '' OR kind = $2) ORDER BY category, name`, org, kind)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []hmiObject{}
	for rows.Next() {
		var o hmiObject
		if rows.Scan(&o.Slug, &o.Kind, &o.Name, &o.Category, &o.Version, &o.CreatedAt, &o.Width, &o.Height) == nil {
			out = append(out, o)
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) getHMIObject(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var o hmiObject
	err := s.pool.QueryRow(r.Context(), `SELECT slug, kind, name, category, version, body, checksum, created_at, reason FROM core.hmi_object WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL`,
		org, r.PathValue("slug")).Scan(&o.Slug, &o.Kind, &o.Name, &o.Category, &o.Version, &o.Body, &o.Checksum, &o.CreatedAt, &o.Reason)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 404, "library object not found")
		return
	}
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, o)
}

// saveHMIObject creates (slug == "") or versions an object; removed lists what an SVG import stripped.
func (s *Server) saveHMIObject(w http.ResponseWriter, r *http.Request, org, slug string, create bool, kind string, body map[string]any, name, category, reason string, removed []string) {
	p := PrincipalFrom(r.Context())
	name, category = strings.TrimSpace(name), strings.TrimSpace(category)
	if len(category) > 64 {
		writeErr(w, 400, "category too long (max 64)")
		return
	}
	if create {
		if slug == "" {
			slug = slugify(name)
		}
		if name == "" || len(name) > 128 {
			writeErr(w, 400, "name required (max 128)")
			return
		}
	}
	if !dashSlugRe.MatchString(slug) {
		writeErr(w, 400, "slug must be lowercase letters, digits and dashes")
		return
	}
	bodyJSON, _ := json.Marshal(body)
	sum := sha256.Sum256(bodyJSON)
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, 500, "begin failed")
		return
	}
	defer tx.Rollback(r.Context())
	var prevID, prevName, prevKind, prevCat string
	var prevVersion int
	err = tx.QueryRow(r.Context(), `SELECT id::text, name, kind, category, version FROM core.hmi_object WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL FOR UPDATE`, org, slug).Scan(&prevID, &prevName, &prevKind, &prevCat, &prevVersion)
	exists := err == nil
	if create && exists {
		writeErr(w, 409, "a library object with this slug exists")
		return
	}
	if !create && !exists {
		writeErr(w, 404, "library object not found")
		return
	}
	if exists {
		if kind != prevKind {
			writeErr(w, 400, "the kind of a library object cannot change")
			return
		}
		if name == "" {
			name = prevName
		}
		if category == "" {
			category = prevCat
		}
		if _, err := tx.Exec(r.Context(), `UPDATE core.hmi_object SET valid_to = now() WHERE id = $1::uuid`, prevID); err != nil {
			writeErr(w, 500, "supersede failed")
			return
		}
	}
	var id string
	version := prevVersion + 1
	if err := tx.QueryRow(r.Context(), `INSERT INTO core.hmi_object (org_id, slug, kind, name, category, version, body, checksum, created_by, reason)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7::jsonb, $8, NULLIF($9,'')::uuid, $10) RETURNING id::text`,
		org, slug, kind, name, category, version, string(bodyJSON), hex.EncodeToString(sum[:]), p.UserID, reason).Scan(&id); err != nil {
		writeErr(w, 500, "insert failed")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, 500, "commit failed")
		return
	}
	action := "hmi_object.update"
	var old any
	if create {
		action = "hmi_object.create"
	} else {
		old = map[string]any{"id": prevID, "version": prevVersion}
	}
	s.audit(r, action, "hmi_object", id, old, map[string]any{"slug": slug, "kind": kind, "name": name, "version": version, "checksum": hex.EncodeToString(sum[:]), "removed": removed}, reason)
	code := 200
	if create {
		code = 201
	}
	if removed == nil {
		removed = []string{}
	}
	writeJSON(w, code, map[string]any{"slug": slug, "kind": kind, "name": name, "category": category, "version": version, "body": body, "checksum": hex.EncodeToString(sum[:]), "removed": removed})
}

func (s *Server) createHMIObject(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		Name, Slug, Kind, Category, Reason string
		Body                               json.RawMessage
	}
	if err := decodeJSON(r, &in, 2<<20); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	body, err := validateHMIBody(in.Kind, in.Body)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.saveHMIObject(w, r, org, in.Slug, true, in.Kind, body, in.Name, in.Category, in.Reason, nil)
}

func (s *Server) importSVG(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct{ Name, Slug, Category, SVG, Reason string }
	if err := decodeJSON(r, &in, maxSVGBytes+64<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	clean, err := sanitizeSVG(in.SVG)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	body := map[string]any{"svg": clean.SVG, "width": clean.Width, "height": clean.Height}
	s.saveHMIObject(w, r, org, in.Slug, true, "svg", body, in.Name, in.Category, in.Reason, clean.Removed)
}

func (s *Server) updateHMIObject(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		Name, Category, Reason string
		Body                   json.RawMessage
	}
	if err := decodeJSON(r, &in, 2<<20); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var kind string
	if err := s.pool.QueryRow(r.Context(), `SELECT kind FROM core.hmi_object WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL`, org, r.PathValue("slug")).Scan(&kind); err != nil {
		writeErr(w, 404, "library object not found")
		return
	}
	body, err := validateHMIBody(kind, in.Body)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.saveHMIObject(w, r, org, r.PathValue("slug"), false, kind, body, in.Name, in.Category, in.Reason, nil)
}

func (s *Server) deleteHMIObject(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	slug := r.PathValue("slug")
	var id string
	if err := s.pool.QueryRow(r.Context(), `UPDATE core.hmi_object SET valid_to = now() WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL RETURNING id::text`, org, slug).Scan(&id); err != nil {
		writeErr(w, 404, "library object not found")
		return
	}
	s.audit(r, "hmi_object.retire", "hmi_object", id, map[string]any{"slug": slug}, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// writeSVG serves a stored, sanitised SVG so that even a direct visit cannot run anything.
func (s *Server) writeSVG(w http.ResponseWriter, r *http.Request, org, slug string) {
	var svg string
	err := s.pool.QueryRow(r.Context(), `SELECT body->>'svg' FROM core.hmi_object WHERE org_id = $1::uuid AND slug = $2 AND kind = 'svg' AND valid_to IS NULL`, org, slug).Scan(&svg)
	if err != nil || svg == "" {
		writeErr(w, 404, "graphic not found")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "image/svg+xml")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, max-age=60")
	_, _ = w.Write([]byte(svg))
}

func (s *Server) hmiObjectSVG(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	s.writeSVG(w, r, org, r.PathValue("slug"))
}

// publicHMIObjectSVG serves a library graphic through a public link only when the linked picture shows it.
func (s *Server) publicHMIObjectSVG(w http.ResponseWriter, r *http.Request) {
	si, ok := s.publicShare(w, r)
	if !ok {
		return
	}
	slug := r.PathValue("slug")
	elems, _ := si.Spec["elements"].([]any)
	for _, x := range elems {
		if e, ok := x.(map[string]any); ok && e["type"] == "image" && e["src"] == slug {
			s.writeSVG(w, r, si.Org, slug)
			return
		}
	}
	writeErr(w, 404, "graphic not found")
}

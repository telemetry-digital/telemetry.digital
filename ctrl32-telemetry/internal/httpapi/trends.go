package httpapi

// Trends (spec 042, 0.96.0): the trend page reads its curves with GET /api/v1/observations; this file keeps the saved
// trend views — which curves belong together, their colours, axes and time window.
//
// A view is shared with the organization or personal. A shared view is configuration: content.write, a reason, the
// audit trail. A personal view is a person's own selection, like the remembered filters of a dashboard: only its owner
// reads and changes it, no reason.
//
//	GET    /api/v1/trends                  shared views and the caller's own           (data.read)
//	POST   /api/v1/trends {name, shared, spec, reason}
//	GET    /api/v1/trends/{id}
//	PUT    /api/v1/trends/{id} {name, shared, spec, reason}
//	DELETE /api/v1/trends/{id}?reason=

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	maxTrendCurves = 16
	maxTrendViews  = 500
)

var (
	trendColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	trendRanges  = map[string]bool{"5m": true, "15m": true, "1h": true, "6h": true, "8h": true, "12h": true, "24h": true, "7d": true, "30d": true, "90d": true}
)

// TrendCurve is one curve of a trend view.
type TrendCurve struct {
	Datastream string   `json:"datastream_id"`
	Label      string   `json:"label,omitempty"`
	Color      string   `json:"color,omitempty"`
	Style      string   `json:"style,omitempty"` // line (default), step, points
	Axis       string   `json:"axis,omitempty"`  // "" = shared by unit; otherwise the name of an axis of its own
	Min        *float64 `json:"min,omitempty"`   // fixed range of its axis
	Max        *float64 `json:"max,omitempty"`
	Width      float64  `json:"width,omitempty"`
	Hidden     bool     `json:"hidden,omitempty"`
}

// TrendSpec is what a saved view remembers.
type TrendSpec struct {
	Curves []TrendCurve `json:"curves"`
	Range  string       `json:"range,omitempty"`  // rolling window when the view opens live
	From   *time.Time   `json:"from,omitempty"`   // a fixed window instead (both)
	To     *time.Time   `json:"to,omitempty"`     //
	Limits bool         `json:"limits,omitempty"` // draw the alarm limits of the curves
	Alarms bool         `json:"alarms,omitempty"` // mark the alarms of the curves
	Stats  bool         `json:"stats,omitempty"`  // show the statistics table
}

// validateTrendSpec normalizes a trend view; ids are checked against the organization by the caller.
func validateTrendSpec(raw json.RawMessage) (*TrendSpec, error) {
	var sp TrendSpec
	if len(raw) == 0 || json.Unmarshal(raw, &sp) != nil {
		return nil, errors.New("spec must be an object with curves")
	}
	if len(sp.Curves) == 0 || len(sp.Curves) > maxTrendCurves {
		return nil, fmt.Errorf("a trend view has 1 to %d curves", maxTrendCurves)
	}
	seen := map[string]bool{}
	for i := range sp.Curves {
		c := &sp.Curves[i]
		c.Datastream = strings.ToLower(strings.TrimSpace(c.Datastream))
		if !uuidRe.MatchString(c.Datastream) {
			return nil, fmt.Errorf("curve %d: datastream_id must be an id", i+1)
		}
		if seen[c.Datastream] {
			return nil, fmt.Errorf("curve %d: the datastream is in the view already", i+1)
		}
		seen[c.Datastream] = true
		c.Label, c.Axis = strings.TrimSpace(c.Label), strings.TrimSpace(c.Axis)
		if len(c.Label) > 80 || len(c.Axis) > 24 {
			return nil, fmt.Errorf("curve %d: label at most 80 characters, axis at most 24", i+1)
		}
		if c.Color != "" && !trendColorRe.MatchString(c.Color) {
			return nil, fmt.Errorf("curve %d: color must be #rrggbb", i+1)
		}
		switch c.Style {
		case "", "line", "step", "points":
		default:
			return nil, fmt.Errorf("curve %d: style must be line, step or points", i+1)
		}
		for _, v := range []*float64{c.Min, c.Max} {
			if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0) || math.Abs(*v) > 1e15) {
				return nil, fmt.Errorf("curve %d: min and max must be numbers", i+1)
			}
		}
		if c.Min != nil && c.Max != nil && *c.Max <= *c.Min {
			return nil, fmt.Errorf("curve %d: max must be above min", i+1)
		}
		if c.Width != 0 && (c.Width < 0.5 || c.Width > 6) {
			return nil, fmt.Errorf("curve %d: width 0.5 to 6", i+1)
		}
	}
	if sp.Range != "" && !trendRanges[sp.Range] {
		return nil, errors.New("range must be one of 5m, 15m, 1h, 6h, 8h, 12h, 24h, 7d, 30d, 90d")
	}
	if (sp.From == nil) != (sp.To == nil) {
		return nil, errors.New("from and to go together")
	}
	if sp.From != nil && (!sp.To.After(*sp.From) || sp.To.Sub(*sp.From) > 5*366*24*time.Hour) {
		return nil, errors.New("to must be after from, at most 5 years apart")
	}
	if sp.From == nil && sp.Range == "" {
		sp.Range = "1h"
	}
	return &sp, nil
}

// trendDatastreams checks that every curve is a datastream of the organization.
func (s *Server) trendDatastreams(ctx context.Context, org string, sp *TrendSpec) error {
	ids := make([]string, len(sp.Curves))
	for i, c := range sp.Curves {
		ids[i] = c.Datastream
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM core.datastream WHERE id = ANY($1::uuid[]) AND org_id = $2::uuid`, ids, org).Scan(&n); err != nil {
		return errors.New("query failed")
	}
	if n != len(ids) {
		return errors.New("a curve names a datastream that does not exist")
	}
	return nil
}

type trendIn struct {
	Name   string          `json:"name"`
	Shared bool            `json:"shared"`
	Spec   json.RawMessage `json:"spec"`
	Reason string          `json:"reason"`
}

func trendRow(id, name string, shared bool, ownerID, owner string, spec json.RawMessage, created, updated time.Time, by string, p string, canWrite bool) map[string]any {
	mine := ownerID != "" && ownerID == p
	return map[string]any{"id": id, "name": name, "shared": shared, "owner": owner, "mine": mine, "spec": spec, "created_at": created, "updated_at": updated, "updated_by": by,
		"can_edit": (shared && canWrite) || (!shared && mine)}
}

const trendCols = `id::text, name, shared, COALESCE(owner_id::text, ''), owner_name, spec, created_at, updated_at, updated_by`

// GET /api/v1/trends
func (s *Server) listTrends(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `SELECT `+trendCols+` FROM core.trend_view WHERE org_id = $1::uuid AND (shared OR owner_id = NULLIF($2,'')::uuid) ORDER BY shared DESC, lower(name)`, org, p.UserID)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, ownerID, owner, by string
		var shared bool
		var spec json.RawMessage
		var created, updated time.Time
		if err := rows.Scan(&id, &name, &shared, &ownerID, &owner, &spec, &created, &updated, &by); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, trendRow(id, name, shared, ownerID, owner, spec, created, updated, by, p.UserID, p.Has("content.write")))
	}
	writeJSON(w, 200, out)
}

// loadTrend reads a view the caller may see.
func (s *Server) loadTrend(w http.ResponseWriter, r *http.Request, org, id string) (map[string]any, bool) {
	p := PrincipalFrom(r.Context())
	var name, ownerID, owner, by string
	var shared bool
	var spec json.RawMessage
	var created, updated time.Time
	if err := s.pool.QueryRow(r.Context(), `SELECT `+trendCols+` FROM core.trend_view WHERE id = $1::uuid AND org_id = $2::uuid AND (shared OR owner_id = NULLIF($3,'')::uuid)`, id, org, p.UserID).
		Scan(&id, &name, &shared, &ownerID, &owner, &spec, &created, &updated, &by); err != nil {
		writeErr(w, 404, "trend view not found")
		return nil, false
	}
	return trendRow(id, name, shared, ownerID, owner, spec, created, updated, by, p.UserID, p.Has("content.write")), true
}

// GET /api/v1/trends/{id}
func (s *Server) getTrend(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	if t, ok := s.loadTrend(w, r, org, r.PathValue("id")); ok {
		writeJSON(w, 200, t)
	}
}

// trendCheck validates the body of a create or an update. A shared view is configuration of the organization.
func (s *Server) trendCheck(w http.ResponseWriter, r *http.Request, org string, in *trendIn) (*TrendSpec, bool) {
	p := PrincipalFrom(r.Context())
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 80 {
		writeErr(w, 400, "name: 1 to 80 characters")
		return nil, false
	}
	if in.Shared {
		if !p.Has("content.write") {
			writeErr(w, 403, "a view shared with the organization needs the permission content.write")
			return nil, false
		}
		if !reasonOf(w, in.Reason) {
			return nil, false
		}
	}
	sp, err := validateTrendSpec(in.Spec)
	if err != nil {
		writeErr(w, 400, err.Error())
		return nil, false
	}
	if err := s.trendDatastreams(r.Context(), org, sp); err != nil {
		writeErr(w, 400, err.Error())
		return nil, false
	}
	return sp, true
}

// POST /api/v1/trends
func (s *Server) createTrend(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	var in trendIn
	if err := decodeJSON(r, &in, 65536); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	sp, ok := s.trendCheck(w, r, org, &in)
	if !ok {
		return
	}
	var n int
	_ = s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.trend_view WHERE org_id = $1::uuid`, org).Scan(&n)
	if n >= maxTrendViews {
		writeErr(w, 409, "at most 500 trend views")
		return
	}
	sj, _ := json.Marshal(sp)
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.trend_view (org_id, name, shared, owner_id, owner_name, spec, updated_by) VALUES ($1::uuid, $2, $3, NULLIF($4,'')::uuid, $5, $6::jsonb, $5) RETURNING id::text`,
		org, in.Name, in.Shared, p.UserID, p.Username, string(sj)).Scan(&id); err != nil {
		s.log.Error("trend view insert", "err", err)
		writeErr(w, 500, "insert failed")
		return
	}
	if in.Shared {
		s.audit(r, "trend_view.create", "trend_view", id, nil, map[string]any{"name": in.Name, "shared": true, "curves": len(sp.Curves)}, in.Reason)
	}
	t, ok := s.loadTrend(w, r, org, id)
	if ok {
		writeJSON(w, 201, t)
	}
}

// PUT /api/v1/trends/{id}
func (s *Server) updateTrend(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	old, ok := s.loadTrend(w, r, org, id)
	if !ok {
		return
	}
	if old["can_edit"] != true {
		writeErr(w, 403, "a shared view is changed with the permission content.write, a personal one by its owner")
		return
	}
	var in trendIn
	if err := decodeJSON(r, &in, 65536); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	wasShared := old["shared"] == true
	if wasShared && !in.Shared && !reasonOf(w, in.Reason) { // taking a view away from the organization is a change of its configuration too
		return
	}
	sp, ok := s.trendCheck(w, r, org, &in)
	if !ok {
		return
	}
	sj, _ := json.Marshal(sp)
	// a view that becomes personal belongs to the person who made it so
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.trend_view SET name = $2, shared = $3, spec = $4::jsonb, updated_at = now(), updated_by = $5,
		owner_id = CASE WHEN shared AND NOT $3 THEN NULLIF($6,'')::uuid ELSE owner_id END, owner_name = CASE WHEN shared AND NOT $3 THEN $5 ELSE owner_name END WHERE id = $1::uuid`,
		id, in.Name, in.Shared, string(sj), p.Username, p.UserID); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	if in.Shared || wasShared {
		s.audit(r, "trend_view.update", "trend_view", id, map[string]any{"name": old["name"], "shared": wasShared}, map[string]any{"name": in.Name, "shared": in.Shared, "curves": len(sp.Curves)}, in.Reason)
	}
	if t, ok := s.loadTrend(w, r, org, id); ok {
		writeJSON(w, 200, t)
	}
}

// DELETE /api/v1/trends/{id}?reason=
func (s *Server) deleteTrend(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	old, ok := s.loadTrend(w, r, org, id)
	if !ok {
		return
	}
	if old["can_edit"] != true {
		writeErr(w, 403, "a shared view is removed with the permission content.write, a personal one by its owner")
		return
	}
	reason := r.URL.Query().Get("reason")
	if old["shared"] == true && !reasonOf(w, reason) {
		return
	}
	if _, err := s.pool.Exec(r.Context(), `DELETE FROM core.trend_view WHERE id = $1::uuid AND org_id = $2::uuid`, id, org); err != nil {
		writeErr(w, 500, "delete failed")
		return
	}
	if old["shared"] == true {
		s.audit(r, "trend_view.delete", "trend_view", id, map[string]any{"name": old["name"]}, nil, reason)
	}
	writeJSON(w, 200, map[string]any{"id": id, "deleted": true})
}

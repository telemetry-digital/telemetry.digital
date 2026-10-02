package httpapi

// Object counting (spec 029): areas (building, car park, zone) with entrances; people and vehicles counted by the
// server's own detector on camera pictures (lines drawn on the picture, worker processes) or by sensors through the
// device protocol; occupancy with a daily reset, never below zero, corrections with a reason; vehicle speed between
// two lines. Every count is a datastream observation (internal/counting).
//
//	GET  /api/v1/counting/areas                         data.read    areas, entrances, occupancy, today's totals
//	GET  /api/v1/counting/summary?area=|datastream=     data.read    one area: now, a day by hours, entrances, days, speeds
//	GET  /api/v1/counting/log?area=                     data.read    resets, corrections, clamps
//	GET  /api/v1/counting/cameras                       data.read    counting cameras and their state
//	GET  /api/v1/counting/cameras/{id}                  data.read    one camera: lines, sections, state, boxes of the last frame
//	POST/PUT/DELETE areas, entrances, inputs; PUT cameras/{id}; POST areas/{id}/occupancy   config.write, with a reason

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/counting"
)

var hhmmRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

func (s *Server) countingReady(w http.ResponseWriter) bool {
	if s.counting == nil {
		writeErr(w, 503, "object counting is not available")
		return false
	}
	return true
}

// ---- areas and entrances -----------------------------------------------------------------------------------

type countingEntranceJSON struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	AssetID string           `json:"asset_id"`
	Sort    int              `json:"sort"`
	Today   map[string]inOut `json:"today"`
}

type inOut struct {
	In  float64 `json:"in"`
	Out float64 `json:"out"`
}

type countingAreaJSON struct {
	ID        string                 `json:"id"`
	SiteID    string                 `json:"site_id"`
	Site      string                 `json:"site"`
	AssetID   string                 `json:"asset_id"`
	Name      string                 `json:"name"`
	Kind      string                 `json:"kind"`
	Groups    []string               `json:"groups"`
	ResetTime string                 `json:"reset_time"`
	Limits    map[string]float64     `json:"limits"`
	Occupancy map[string]int64       `json:"occupancy"`
	Clamped   map[string]int64       `json:"clamped"`
	LastReset map[string]*time.Time  `json:"last_reset"`
	Today     map[string]inOut       `json:"today"`
	Entrances []countingEntranceJSON `json:"entrances"`
}

func (s *Server) countingAreas(ctx context.Context, org, onlyID string) ([]countingAreaJSON, error) {
	rows, err := s.pool.Query(ctx, `SELECT a.id::text, a.site_id::text, si.name, a.asset_id::text, a.name, a.kind, a.groups, coalesce(a.reset_time, ''), a.limits::text
		FROM core.counting_area a JOIN core.site si ON si.id = a.site_id
		WHERE a.org_id = $1::uuid AND a.archived_at IS NULL AND ($2 = '' OR a.id = $2::uuid) ORDER BY si.name, a.name`, org, onlyID)
	if err != nil {
		return nil, err
	}
	var out []countingAreaJSON
	idx := map[string]int{}
	for rows.Next() {
		var a countingAreaJSON
		var lim string
		if err := rows.Scan(&a.ID, &a.SiteID, &a.Site, &a.AssetID, &a.Name, &a.Kind, &a.Groups, &a.ResetTime, &lim); err != nil {
			rows.Close()
			return nil, err
		}
		a.Limits = map[string]float64{}
		_ = json.Unmarshal([]byte(lim), &a.Limits)
		a.Occupancy, a.Clamped, a.LastReset, a.Today = map[string]int64{}, map[string]int64{}, map[string]*time.Time{}, map[string]inOut{}
		for _, g := range a.Groups {
			a.Occupancy[g] = 0
			a.Today[g] = inOut{}
		}
		a.Entrances = []countingEntranceJSON{}
		idx[a.ID] = len(out)
		out = append(out, a)
	}
	rows.Close()
	if len(out) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(out))
	for _, a := range out {
		ids = append(ids, a.ID)
	}
	rows, err = s.pool.Query(ctx, `SELECT area_id::text, grp, occupancy, clamped, last_reset_at FROM core.counting_occupancy WHERE area_id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, g string
		var occ, cl int64
		var lr *time.Time
		if rows.Scan(&id, &g, &occ, &cl, &lr) == nil {
			a := &out[idx[id]]
			a.Occupancy[g], a.Clamped[g], a.LastReset[g] = occ, cl, lr
		}
	}
	rows.Close()
	ent := map[string]int{} // entrance asset → index in its area
	entArea := map[string]string{}
	rows, err = s.pool.Query(ctx, `SELECT id::text, area_id::text, name, asset_id::text, sort FROM core.counting_entrance WHERE area_id = ANY($1::uuid[]) AND archived_at IS NULL ORDER BY sort, name`, ids)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var e countingEntranceJSON
		var area string
		if rows.Scan(&e.ID, &area, &e.Name, &e.AssetID, &e.Sort) == nil {
			e.Today = map[string]inOut{}
			a := &out[idx[area]]
			ent[e.AssetID] = len(a.Entrances)
			entArea[e.AssetID] = area
			a.Entrances = append(a.Entrances, e)
		}
	}
	rows.Close()
	// today's passages, by asset and key
	loc := s.orgLoc(ctx, org)
	now := time.Now().In(loc)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	assets := []string{}
	areaOfAsset := map[string]string{}
	for _, a := range out {
		assets = append(assets, a.AssetID)
		areaOfAsset[a.AssetID] = a.ID
	}
	for as := range entArea {
		assets = append(assets, as)
	}
	rows, err = s.pool.Query(ctx, `SELECT ds.asset_id::text, ds.key, coalesce(sum(o.value), 0)::float8
		FROM core.datastream ds JOIN core.observation o ON o.datastream_id = ds.id AND o.measured_at >= $2 AND o.measured_at < $3
		WHERE ds.asset_id = ANY($1::uuid[]) AND (ds.key LIKE '%\_in' OR ds.key LIKE '%\_out') GROUP BY 1, 2`, assets, from, from.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var as, key string
		var v float64
		if rows.Scan(&as, &key, &v) != nil {
			continue
		}
		g, dir := splitKey(key)
		var m map[string]inOut
		if id, ok := areaOfAsset[as]; ok {
			m = out[idx[id]].Today
		} else if area, ok := entArea[as]; ok {
			m = out[idx[area]].Entrances[ent[as]].Today
		}
		if m == nil {
			continue
		}
		io := m[g]
		if dir == "in" {
			io.In = v
		} else {
			io.Out = v
		}
		m[g] = io
	}
	rows.Close()
	return out, nil
}

func splitKey(key string) (group, dir string) {
	i := strings.LastIndexByte(key, '_')
	if i < 0 {
		return key, ""
	}
	return key[:i], key[i+1:]
}

// GET /api/v1/counting/areas
func (s *Server) listCountingAreas(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	areas, err := s.countingAreas(r.Context(), org, "")
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	if areas == nil {
		areas = []countingAreaJSON{}
	}
	installed, why := false, "object counting is not available"
	if s.counting != nil {
		installed, why = s.counting.Installed()
	}
	writeJSON(w, 200, map[string]any{"areas": areas, "groups": counting.Groups, "classes": analytics.Classes, "cameras_installed": installed, "cameras_note": map[bool]string{true: "", false: why}[installed]})
}

type areaInput struct {
	SiteID    string             `json:"site_id"`
	Name      string             `json:"name"`
	Kind      string             `json:"kind"`
	Groups    []string           `json:"groups"`
	ResetTime string             `json:"reset_time"`
	Limits    map[string]float64 `json:"limits"`
	Entrances []string           `json:"entrances"` // names of entrances created with a new area
	Reason    string             `json:"reason"`
}

func (in *areaInput) validate(create bool) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 80 {
		return fmt.Errorf("name: 1 to 80 characters")
	}
	if in.Kind == "" {
		in.Kind = "building"
	}
	if !slices.Contains([]string{"building", "car_park", "zone"}, in.Kind) {
		return fmt.Errorf("kind: building, car_park or zone")
	}
	if len(in.Groups) == 0 {
		in.Groups = []string{"people"}
		if in.Kind == "car_park" {
			in.Groups = []string{"vehicles"}
		}
	}
	if err := counting.ValidGroups(in.Groups); err != nil {
		return err
	}
	if in.ResetTime != "" && !hhmmRe.MatchString(in.ResetTime) {
		return fmt.Errorf("reset_time: HH:MM (local time of the organization), or empty for no daily reset")
	}
	for g, v := range in.Limits {
		if !slices.Contains(in.Groups, g) || v < 0 || v > 1e7 || math.IsNaN(v) {
			return fmt.Errorf("limits: a capacity 0 to 10 000 000 for a counted group, got %s", g)
		}
	}
	if len(in.Entrances) > 50 {
		return fmt.Errorf("at most 50 entrances")
	}
	if create && !uuidRe.MatchString(strings.ToLower(in.SiteID)) {
		return fmt.Errorf("site_id required")
	}
	return nil
}

// POST /api/v1/counting/areas
func (s *Server) createCountingArea(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.countingReady(w) {
		return
	}
	var in areaInput
	if err := decodeJSON(r, &in, 32768); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := in.validate(true); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		writeErr(w, 500, "database")
		return
	}
	defer tx.Rollback(ctx)
	var asset string
	attrs, _ := json.Marshal(map[string]any{"object_counting": in.Kind})
	if err := tx.QueryRow(ctx, `INSERT INTO core.asset (org_id, site_id, type, name, attributes) SELECT $1::uuid, s.id, 'counting_area', $3, $4::jsonb
		FROM core.site s WHERE s.id = $2::uuid AND s.org_id = $1::uuid RETURNING id::text`, org, in.SiteID, in.Name, string(attrs)).Scan(&asset); err != nil {
		writeErr(w, 409, "the area could not be created (site not found, or an asset of this name exists on the site)")
		return
	}
	lim, _ := json.Marshal(orMap(in.Limits))
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO core.counting_area (org_id, site_id, asset_id, name, kind, groups, reset_time, limits)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, NULLIF($7, ''), $8::jsonb) RETURNING id::text`, org, in.SiteID, asset, in.Name, in.Kind, in.Groups, in.ResetTime, string(lim)).Scan(&id); err != nil {
		writeErr(w, 409, "an area of this name exists")
		return
	}
	for i, name := range in.Entrances {
		if _, err := s.addEntrance(ctx, tx, org, id, in.SiteID, asset, strings.TrimSpace(name), i); err != nil {
			writeErr(w, 409, err.Error())
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		writeErr(w, 500, "database")
		return
	}
	if err := s.counting.EnsureDatastreams(ctx, id); err != nil {
		s.log.Warn("counting datastreams", "err", err)
	}
	s.counting.Reload()
	s.audit(r, "counting_area.create", "counting_area", id, nil, in, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id, "asset_id": asset})
}

func orMap(m map[string]float64) map[string]float64 {
	if m == nil {
		return map[string]float64{}
	}
	return m
}

func (s *Server) addEntrance(ctx context.Context, tx pgx.Tx, org, areaID, siteID, areaAsset, name string, sort int) (string, error) {
	if name == "" || len([]rune(name)) > 80 {
		return "", fmt.Errorf("entrance name: 1 to 80 characters")
	}
	var asset, id string
	if err := tx.QueryRow(ctx, `INSERT INTO core.asset (org_id, site_id, parent_id, type, name, attributes) VALUES ($1::uuid, $2::uuid, $3::uuid, 'entrance', $4, '{}'::jsonb) RETURNING id::text`,
		org, siteID, areaAsset, name).Scan(&asset); err != nil {
		return "", fmt.Errorf("entrance %q: an asset of this name exists on the site", name)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO core.counting_entrance (org_id, area_id, asset_id, name, sort) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5) RETURNING id::text`,
		org, areaID, asset, name, sort).Scan(&id); err != nil {
		return "", fmt.Errorf("entrance %q exists", name)
	}
	return id, nil
}

// PUT /api/v1/counting/areas/{id}
func (s *Server) updateCountingArea(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.countingReady(w) {
		return
	}
	id := r.PathValue("id")
	var in areaInput
	if err := decodeJSON(r, &in, 32768); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := in.validate(false); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	old, err := s.countingAreas(r.Context(), org, id)
	if err != nil || len(old) == 0 || !uuidRe.MatchString(id) {
		writeErr(w, 404, "area not found")
		return
	}
	lim, _ := json.Marshal(orMap(in.Limits))
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.counting_area SET name = $3, kind = $4, groups = $5, reset_time = NULLIF($6, ''), limits = $7::jsonb, updated_at = now()
		WHERE id = $1::uuid AND org_id = $2::uuid`, id, org, in.Name, in.Kind, in.Groups, in.ResetTime, string(lim)); err != nil {
		writeErr(w, 409, "an area of this name exists")
		return
	}
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.asset SET name = $2 WHERE id = $1::uuid`, old[0].AssetID, in.Name)
	_ = s.counting.EnsureDatastreams(r.Context(), id)
	s.counting.Reload()
	o := old[0]
	s.audit(r, "counting_area.update", "counting_area", id, map[string]any{"name": o.Name, "kind": o.Kind, "groups": o.Groups, "reset_time": o.ResetTime, "limits": o.Limits}, in, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/v1/counting/areas/{id}?reason=
func (s *Server) deleteCountingArea(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.countingReady(w) {
		return
	}
	id, reason := r.PathValue("id"), r.URL.Query().Get("reason")
	if !reasonOf(w, reason) || !uuidRe.MatchString(id) {
		return
	}
	ct, err := s.pool.Exec(r.Context(), `UPDATE core.counting_area SET archived_at = now() WHERE id = $1::uuid AND org_id = $2::uuid AND archived_at IS NULL`, id, org)
	if err != nil || ct.RowsAffected() == 0 {
		writeErr(w, 404, "area not found")
		return
	}
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.counting_entrance SET archived_at = now() WHERE area_id = $1::uuid AND archived_at IS NULL`, id)
	s.counting.Reload()
	s.audit(r, "counting_area.delete", "counting_area", id, nil, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true, "note": "the area and its entrances are archived; their datastreams and observations stay"})
}

// POST /api/v1/counting/areas/{id}/entrances
func (s *Server) createCountingEntrance(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.countingReady(w) {
		return
	}
	areaID := r.PathValue("id")
	var in struct {
		Name   string `json:"name"`
		Sort   int    `json:"sort"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	a, err := s.countingAreas(r.Context(), org, areaID)
	if err != nil || len(a) == 0 || !uuidRe.MatchString(areaID) {
		writeErr(w, 404, "area not found")
		return
	}
	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		writeErr(w, 500, "database")
		return
	}
	defer tx.Rollback(ctx)
	id, err := s.addEntrance(ctx, tx, org, areaID, a[0].SiteID, a[0].AssetID, strings.TrimSpace(in.Name), in.Sort)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeErr(w, 500, "database")
		return
	}
	_ = s.counting.EnsureDatastreams(ctx, areaID)
	s.counting.Reload()
	s.audit(r, "counting_entrance.create", "counting_entrance", id, nil, map[string]any{"area_id": areaID, "name": in.Name}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id})
}

// PUT /api/v1/counting/entrances/{id}
func (s *Server) updateCountingEntrance(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.countingReady(w) {
		return
	}
	id := r.PathValue("id")
	var in struct {
		Name   string `json:"name"`
		Sort   int    `json:"sort"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 80 {
		writeErr(w, 400, "name: 1 to 80 characters")
		return
	}
	if !reasonOf(w, in.Reason) || !uuidRe.MatchString(id) {
		return
	}
	var oldName, asset string
	if err := s.pool.QueryRow(r.Context(), `UPDATE core.counting_entrance e SET name = $3, sort = $4 FROM core.counting_entrance o
		WHERE e.id = $1::uuid AND e.org_id = $2::uuid AND e.archived_at IS NULL AND o.id = e.id RETURNING o.name, e.asset_id::text`, id, org, in.Name, in.Sort).Scan(&oldName, &asset); err != nil {
		writeErr(w, 404, "entrance not found (or the name exists)")
		return
	}
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.asset SET name = $2 WHERE id = $1::uuid`, asset, in.Name)
	s.counting.Reload()
	s.audit(r, "counting_entrance.update", "counting_entrance", id, map[string]any{"name": oldName}, map[string]any{"name": in.Name, "sort": in.Sort}, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/v1/counting/entrances/{id}?reason=
func (s *Server) deleteCountingEntrance(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.countingReady(w) {
		return
	}
	id, reason := r.PathValue("id"), r.URL.Query().Get("reason")
	if !reasonOf(w, reason) || !uuidRe.MatchString(id) {
		return
	}
	ct, err := s.pool.Exec(r.Context(), `UPDATE core.counting_entrance SET archived_at = now() WHERE id = $1::uuid AND org_id = $2::uuid AND archived_at IS NULL`, id, org)
	if err != nil || ct.RowsAffected() == 0 {
		writeErr(w, 404, "entrance not found")
		return
	}
	s.counting.Reload()
	s.audit(r, "counting_entrance.delete", "counting_entrance", id, nil, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/v1/counting/areas/{id}/occupancy — a correction by hand
func (s *Server) correctOccupancy(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.countingReady(w) {
		return
	}
	id := r.PathValue("id")
	var in struct {
		Group  string `json:"group"`
		Value  int64  `json:"value"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	a, err := s.countingAreas(r.Context(), org, id)
	if err != nil || len(a) == 0 || !uuidRe.MatchString(id) {
		writeErr(w, 404, "area not found")
		return
	}
	if !slices.Contains(a[0].Groups, in.Group) || in.Value < 0 || in.Value > 10000000 {
		writeErr(w, 400, "group: one the area counts; value: 0 to 10 000 000")
		return
	}
	p := PrincipalFrom(r.Context())
	before, err := s.counting.Correct(r.Context(), id, in.Group, in.Value, in.Reason, p.Username)
	if err != nil {
		writeErr(w, 500, "correction failed")
		return
	}
	s.audit(r, "counting_occupancy.update", "counting_area", id, map[string]any{"group": in.Group, "occupancy": before}, map[string]any{"group": in.Group, "occupancy": in.Value}, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true, "before": before, "occupancy": in.Value})
}

// GET /api/v1/counting/log?area=&limit=
func (s *Server) countingLog(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	area := r.URL.Query().Get("area")
	if !uuidRe.MatchString(area) {
		writeErr(w, 400, "area required")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.pool.Query(r.Context(), `SELECT at, grp, kind, before, after, coalesce(reason, ''), actor FROM core.counting_log WHERE area_id = $1::uuid AND org_id = $2::uuid ORDER BY at DESC LIMIT $3`, area, org, limit)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var at time.Time
		var g, kind, reason, actor string
		var b, a int64
		if rows.Scan(&at, &g, &kind, &b, &a, &reason, &actor) == nil {
			out = append(out, map[string]any{"at": at, "group": g, "kind": kind, "before": b, "after": a, "reason": reason, "actor": actor})
		}
	}
	writeJSON(w, 200, out)
}

// ---- summary ----------------------------------------------------------------------------------------------

type countingHour struct {
	Start     time.Time          `json:"start"`
	In        map[string]float64 `json:"in"`
	Out       map[string]float64 `json:"out"`
	Occupancy map[string]float64 `json:"occupancy"` // at the end of the hour (last value)
}

type countingDay struct {
	Date string             `json:"date"`
	In   map[string]float64 `json:"in"`
	Out  map[string]float64 `json:"out"`
	Peak map[string]float64 `json:"peak"`
}

// GET /api/v1/counting/summary?area=|datastream=&date=YYYY-MM-DD&days=7
func (s *Server) countingSummary(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	q := r.URL.Query()
	areaID := q.Get("area")
	if ds := q.Get("datastream"); areaID == "" && uuidRe.MatchString(ds) {
		// the area of a datastream: of the area's asset, an entrance's asset or a speed section's asset
		_ = s.pool.QueryRow(ctx, `SELECT a.id::text FROM core.datastream d JOIN core.asset x ON x.id = d.asset_id
			JOIN core.counting_area a ON a.asset_id = x.id OR a.asset_id = x.parent_id
			WHERE d.id = $1::uuid AND d.org_id = $2::uuid AND a.archived_at IS NULL LIMIT 1`, ds, org).Scan(&areaID)
	}
	if !uuidRe.MatchString(areaID) {
		writeErr(w, 404, "no counting area for this area or datastream")
		return
	}
	areas, err := s.countingAreas(ctx, org, areaID)
	if err != nil || len(areas) == 0 {
		writeErr(w, 404, "area not found")
		return
	}
	a := areas[0]
	loc := s.orgLoc(ctx, org)
	now := time.Now().In(loc)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	if d := q.Get("date"); d != "" {
		t, err := time.ParseInLocation("2006-01-02", d, loc)
		if err != nil {
			writeErr(w, 400, "date: YYYY-MM-DD")
			return
		}
		day = t
	}
	days, _ := strconv.Atoi(q.Get("days"))
	if days <= 0 || days > 62 {
		days = 7
	}
	end := day.AddDate(0, 0, 1)
	// datastreams of the area and its entrances
	assets := []string{a.AssetID}
	entByAsset := map[string]int{}
	for i, e := range a.Entrances {
		assets = append(assets, e.AssetID)
		entByAsset[e.AssetID] = i
	}
	type dsInfo struct{ asset, key string }
	dsOf := map[string]dsInfo{}
	var areaIDs []string
	rows, err := s.pool.Query(ctx, `SELECT id::text, asset_id::text, key FROM core.datastream WHERE asset_id = ANY($1::uuid[])`, assets)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	for rows.Next() {
		var id, as, key string
		if rows.Scan(&id, &as, &key) == nil {
			dsOf[id] = dsInfo{as, key}
			if as == a.AssetID {
				areaIDs = append(areaIDs, id)
			}
		}
	}
	rows.Close()
	allIDs := make([]string, 0, len(dsOf))
	for id := range dsOf {
		allIDs = append(allIDs, id)
	}
	// entrances for the day
	ents := make([]map[string]inOut, len(a.Entrances))
	for i := range ents {
		ents[i] = map[string]inOut{}
	}
	dayTotal := map[string]inOut{}
	rows, err = s.pool.Query(ctx, `SELECT datastream_id::text, coalesce(sum(value), 0)::float8 FROM core.observation
		WHERE datastream_id = ANY($1::uuid[]) AND measured_at >= $2 AND measured_at < $3 GROUP BY 1`, allIDs, day, end)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	for rows.Next() {
		var id string
		var v float64
		if rows.Scan(&id, &v) != nil {
			continue
		}
		info := dsOf[id]
		g, dir := splitKey(info.key)
		if dir != "in" && dir != "out" {
			continue
		}
		var m map[string]inOut
		if info.asset == a.AssetID {
			m = dayTotal
		} else if i, ok := entByAsset[info.asset]; ok {
			m = ents[i]
		} else {
			continue
		}
		io := m[g]
		if dir == "in" {
			io.In = v
		} else {
			io.Out = v
		}
		m[g] = io
	}
	rows.Close()
	type entOut struct {
		ID    string           `json:"id"`
		Name  string           `json:"name"`
		Asset string           `json:"asset_id"`
		Count map[string]inOut `json:"counts"`
		Share map[string]inOut `json:"share"` // % of the area's in and out
	}
	entrances := []entOut{}
	for i, e := range a.Entrances {
		sh := map[string]inOut{}
		for _, g := range a.Groups {
			c := ents[i][g]
			var tin, tout float64
			for j := range ents {
				tin += ents[j][g].In
				tout += ents[j][g].Out
			}
			sh[g] = inOut{In: counting.Share(c.In, tin), Out: counting.Share(c.Out, tout)}
		}
		entrances = append(entrances, entOut{e.ID, e.Name, e.AssetID, ents[i], sh})
	}
	// hours of the day (area totals) and the occupancy at the end of each hour
	hours := []countingHour{}
	hIdx := map[int64]int{}
	for t := day; t.Before(end); t = t.Add(time.Hour) {
		hIdx[t.Unix()] = len(hours)
		hours = append(hours, countingHour{Start: t, In: map[string]float64{}, Out: map[string]float64{}, Occupancy: map[string]float64{}})
	}
	peak := map[string]float64{}
	rows, err = s.pool.Query(ctx, `SELECT datastream_id::text, date_trunc('hour', measured_at AT TIME ZONE $4) AT TIME ZONE $4, coalesce(sum(value), 0)::float8, max(value)::float8,
		(array_agg(value ORDER BY measured_at DESC))[1]::float8
		FROM core.observation WHERE datastream_id = ANY($1::uuid[]) AND measured_at >= $2 AND measured_at < $3 GROUP BY 1, 2`, areaIDs, day, end, loc.String())
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	for rows.Next() {
		var id string
		var h time.Time
		var sum, mx, last float64
		if rows.Scan(&id, &h, &sum, &mx, &last) != nil {
			continue
		}
		i, ok := hIdx[h.Unix()]
		if !ok {
			continue
		}
		g, dir := splitKey(dsOf[id].key)
		switch dir {
		case "in":
			hours[i].In[g] = sum
		case "out":
			hours[i].Out[g] = sum
		case "occupancy":
			hours[i].Occupancy[g] = last
			if mx > peak[g] {
				peak[g] = mx
			}
		}
	}
	rows.Close()
	// days before (comparison)
	from := day.AddDate(0, 0, -(days - 1))
	dl := []countingDay{}
	dIdx := map[string]int{}
	for t := from; t.Before(end); t = t.AddDate(0, 0, 1) {
		dIdx[t.Format("2006-01-02")] = len(dl)
		dl = append(dl, countingDay{Date: t.Format("2006-01-02"), In: map[string]float64{}, Out: map[string]float64{}, Peak: map[string]float64{}})
	}
	rows, err = s.pool.Query(ctx, `SELECT datastream_id::text, to_char(measured_at AT TIME ZONE $4, 'YYYY-MM-DD'), coalesce(sum(value), 0)::float8, max(value)::float8
		FROM core.observation WHERE datastream_id = ANY($1::uuid[]) AND measured_at >= $2 AND measured_at < $3 GROUP BY 1, 2`, areaIDs, from, end, loc.String())
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	for rows.Next() {
		var id, d string
		var sum, mx float64
		if rows.Scan(&id, &d, &sum, &mx) != nil {
			continue
		}
		i, ok := dIdx[d]
		if !ok {
			continue
		}
		g, dir := splitKey(dsOf[id].key)
		switch dir {
		case "in":
			dl[i].In[g] = sum
		case "out":
			dl[i].Out[g] = sum
		case "occupancy":
			dl[i].Peak[g] = mx
		}
	}
	rows.Close()
	ids := map[string]string{}
	for id, info := range dsOf {
		if info.asset == a.AssetID {
			ids[info.key] = id
		}
	}
	writeJSON(w, 200, map[string]any{"area": a, "tz": loc.String(), "date": day.Format("2006-01-02"), "from": day, "to": end,
		"totals": dayTotal, "peak": peak, "entrances": entrances, "hours": hours, "days": dl, "datastreams": ids,
		"speeds": s.countingSpeeds(ctx, org, a.ID, day, end)})
}

type speedSection struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Camera   string           `json:"camera"`
	AssetID  string           `json:"asset_id"`
	LimitKmh float64          `json:"limit_kmh"`
	Count    int64            `json:"count"`
	Avg      *float64         `json:"avg_kmh"`
	Max      *float64         `json:"max_kmh"`
	Over     int64            `json:"over_limit"`
	Last     []map[string]any `json:"last"`
	Histo    []int64          `json:"histogram"` // vehicles per 10 km/h: 0–10, 10–20 … 130+
	SpeedDS  string           `json:"speed_datastream,omitempty"`
}

// countingSpeeds summarises the speed sections of an area for a period.
func (s *Server) countingSpeeds(ctx context.Context, org, areaID string, from, to time.Time) []speedSection {
	out := []speedSection{}
	rows, err := s.pool.Query(ctx, `SELECT cc.camera_id::text, c.name, cc.config::text FROM core.counting_camera cc JOIN core.camera c ON c.id = cc.camera_id
		WHERE cc.org_id = $1::uuid AND c.removed_at IS NULL`, org)
	if err != nil {
		return out
	}
	for rows.Next() {
		var id, name, raw string
		if rows.Scan(&id, &name, &raw) != nil {
			continue
		}
		var cfg counting.CameraConfig
		if json.Unmarshal([]byte(raw), &cfg) != nil {
			continue
		}
		for _, sec := range cfg.Sections {
			if sec.AreaID == areaID && sec.AssetID != "" {
				out = append(out, speedSection{ID: sec.ID, Name: sec.Name, Camera: name, AssetID: sec.AssetID, LimitKmh: sec.LimitKmh, Last: []map[string]any{}, Histo: make([]int64, 14)})
			}
		}
	}
	rows.Close()
	for i := range out {
		sec := &out[i]
		var sp, ol string
		_ = s.pool.QueryRow(ctx, `SELECT coalesce((SELECT id::text FROM core.datastream WHERE asset_id = $1::uuid AND key = 'vehicle_speed'), ''),
			coalesce((SELECT id::text FROM core.datastream WHERE asset_id = $1::uuid AND key = 'over_limit'), '')`, sec.AssetID).Scan(&sp, &ol)
		sec.SpeedDS = sp
		if sp == "" {
			continue
		}
		_ = s.pool.QueryRow(ctx, `SELECT count(*), avg(value)::float8, max(value)::float8 FROM core.observation WHERE datastream_id = $1::uuid AND measured_at >= $2 AND measured_at < $3`, sp, from, to).Scan(&sec.Count, &sec.Avg, &sec.Max)
		if sec.Avg != nil {
			v := math.Round(*sec.Avg*10) / 10
			sec.Avg = &v
		}
		if ol != "" {
			_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM core.observation WHERE datastream_id = $1::uuid AND measured_at >= $2 AND measured_at < $3`, ol, from, to).Scan(&sec.Over)
		}
		hr, err := s.pool.Query(ctx, `SELECT least(floor(value / 10), 13)::int, count(*) FROM core.observation WHERE datastream_id = $1::uuid AND measured_at >= $2 AND measured_at < $3 GROUP BY 1`, sp, from, to)
		if err == nil {
			for hr.Next() {
				var b int
				var n int64
				if hr.Scan(&b, &n) == nil && b >= 0 && b < len(sec.Histo) {
					sec.Histo[b] = n
				}
			}
			hr.Close()
		}
		lr, err := s.pool.Query(ctx, `SELECT measured_at, value::float8, coalesce(value_json->>'class', ''), coalesce(value_json->>'direction', '') FROM core.observation
			WHERE datastream_id = $1::uuid AND measured_at >= $2 AND measured_at < $3 ORDER BY measured_at DESC LIMIT 10`, sp, from, to)
		if err == nil {
			for lr.Next() {
				var at time.Time
				var v float64
				var cls, dir string
				if lr.Scan(&at, &v, &cls, &dir) == nil {
					sec.Last = append(sec.Last, map[string]any{"at": at, "kmh": v, "class": cls, "direction": dir, "over": sec.LimitKmh > 0 && v > sec.LimitKmh})
				}
			}
			lr.Close()
		}
	}
	return out
}

// ---- cameras ----------------------------------------------------------------------------------------------

// GET /api/v1/counting/cameras
func (s *Server) listCountingCameras(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `SELECT c.id::text, c.name, c.camera_group, c.enabled, coalesce(cc.enabled, false), coalesce(cc.fps, 5)::float8, coalesce(cc.model, 'nano'),
		coalesce(cc.min_score, 0.35)::float8, coalesce(cc.config::text, '{"lines": []}')
		FROM core.camera c LEFT JOIN core.counting_camera cc ON cc.camera_id = c.id
		WHERE c.org_id = $1::uuid AND c.removed_at IS NULL ORDER BY c.sort, c.name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, group, model, raw string
		var camOn, on bool
		var fps, score float64
		if rows.Scan(&id, &name, &group, &camOn, &on, &fps, &model, &score, &raw) != nil || !p.SeesCameraGroup(group) {
			continue
		}
		st := counting.CameraStatus{State: "off"}
		if s.counting != nil && on {
			st = s.counting.Status(id)
		}
		st.Stats = nil
		out = append(out, map[string]any{"camera_id": id, "camera": name, "camera_enabled": camOn, "enabled": on, "fps": fps, "model": model, "min_score": score, "config": json.RawMessage(raw), "status": st})
	}
	writeJSON(w, 200, out)
}

// GET /api/v1/counting/cameras/{id}
func (s *Server) getCountingCamera(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	c, ok := s.cameraRow(r.Context(), org, id)
	if !ok {
		writeErr(w, 404, "camera not found")
		return
	}
	var on bool
	var fps, score float64
	model, raw := "nano", `{"lines": []}`
	_ = s.pool.QueryRow(r.Context(), `SELECT enabled, fps::float8, model, min_score::float8, config::text FROM core.counting_camera WHERE camera_id = $1::uuid`, id).Scan(&on, &fps, &model, &score, &raw)
	if fps == 0 {
		fps, score = 5, 0.35
	}
	st := counting.CameraStatus{State: "off"}
	if s.counting != nil && on {
		st = s.counting.Status(id)
	}
	installed, why := false, ""
	if s.counting != nil {
		installed, why = s.counting.Installed()
	}
	writeJSON(w, 200, map[string]any{"camera_id": id, "camera": c.name, "enabled": on, "fps": fps, "model": model, "min_score": score, "config": json.RawMessage(raw), "status": st,
		"installed": installed, "installed_note": why})
}

// PUT /api/v1/counting/cameras/{id}
func (s *Server) putCountingCamera(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.countingReady(w) {
		return
	}
	id := r.PathValue("id")
	if _, ok := s.cameraRow(r.Context(), org, id); !ok {
		writeErr(w, 404, "camera not found")
		return
	}
	var in struct {
		Enabled  bool                  `json:"enabled"`
		FPS      float64               `json:"fps"`
		Model    string                `json:"model"`
		MinScore float64               `json:"min_score"`
		Config   counting.CameraConfig `json:"config"`
		Reason   string                `json:"reason"`
	}
	if err := decodeJSON(r, &in, 65536); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if in.FPS == 0 {
		in.FPS = 5
	}
	if in.FPS < 1 || in.FPS > 15 {
		writeErr(w, 400, "fps: 1 to 15 analysed pictures per second")
		return
	}
	if in.Model == "" {
		in.Model = "nano"
	}
	if in.Model != "nano" && in.Model != "tiny" {
		writeErr(w, 400, "model: nano or tiny")
		return
	}
	if in.MinScore == 0 {
		in.MinScore = 0.35
	}
	if in.MinScore < 0.1 || in.MinScore > 0.95 {
		writeErr(w, 400, "min_score: 0.1 to 0.95")
		return
	}
	if in.Config.Lines == nil {
		in.Config.Lines = []counting.CameraLine{}
	}
	cfg := in.Config.Analytics()
	if err := cfg.Validate(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	// every line counts for an entrance of this organization (or for none: a speed line)
	for _, l := range in.Config.Lines {
		if l.EntranceID == "" {
			continue
		}
		var n int
		if !uuidRe.MatchString(l.EntranceID) || s.pool.QueryRow(ctx, `SELECT count(*) FROM core.counting_entrance WHERE id = $1::uuid AND org_id = $2::uuid AND archived_at IS NULL`, l.EntranceID, org).Scan(&n) != nil || n == 0 {
			writeErr(w, 400, fmt.Sprintf("line %s: entrance not found", l.Name))
			return
		}
	}
	// a speed section stores its values on an asset of its own under the area
	for i := range in.Config.Sections {
		sec := &in.Config.Sections[i]
		if sec.LimitKmh < 0 || sec.LimitKmh > 300 {
			writeErr(w, 400, "speed limit: 0 (none) to 300 km/h")
			return
		}
		a, err := s.countingAreas(ctx, org, sec.AreaID)
		if err != nil || len(a) == 0 || !uuidRe.MatchString(sec.AreaID) {
			writeErr(w, 400, fmt.Sprintf("speed section %s: area not found", sec.Name))
			return
		}
		name := strings.TrimSpace(sec.Name)
		if name == "" || len([]rune(name)) > 60 {
			writeErr(w, 400, "speed section: a name of 1 to 60 characters")
			return
		}
		var asset string
		if sec.AssetID != "" {
			_ = s.pool.QueryRow(ctx, `SELECT id::text FROM core.asset WHERE id = $1::uuid AND org_id = $2::uuid AND type = 'speed_section'`, sec.AssetID, org).Scan(&asset)
		}
		if asset == "" {
			if err := s.pool.QueryRow(ctx, `INSERT INTO core.asset (org_id, site_id, parent_id, type, name, attributes) VALUES ($1::uuid, $2::uuid, $3::uuid, 'speed_section', $4, $5::jsonb)
				ON CONFLICT (site_id, name) DO UPDATE SET parent_id = EXCLUDED.parent_id RETURNING id::text`, org, a[0].SiteID, a[0].AssetID, name, `{"object_counting": "speed"}`).Scan(&asset); err != nil {
				writeErr(w, 409, "speed section: the asset could not be created")
				return
			}
		}
		sec.AssetID = asset
		if _, err := s.counting.Datastream(ctx, org, asset, "vehicle_speed", "gauge", "speed", "km/h"); err != nil {
			writeErr(w, 500, "datastream")
			return
		}
		if _, err := s.counting.Datastream(ctx, org, asset, "over_limit", "event", "over_limit", "vehicles"); err != nil {
			writeErr(w, 500, "datastream")
			return
		}
	}
	raw, _ := json.Marshal(in.Config)
	var oldRaw string
	_ = s.pool.QueryRow(ctx, `SELECT jsonb_build_object('enabled', enabled, 'fps', fps, 'model', model, 'min_score', min_score, 'config', config)::text FROM core.counting_camera WHERE camera_id = $1::uuid`, id).Scan(&oldRaw)
	if _, err := s.pool.Exec(ctx, `INSERT INTO core.counting_camera (camera_id, org_id, enabled, fps, model, min_score, config, updated_at) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7::jsonb, now())
		ON CONFLICT (camera_id) DO UPDATE SET enabled = $3, fps = $4, model = $5, min_score = $6, config = $7::jsonb, updated_at = now()`, id, org, in.Enabled, in.FPS, in.Model, in.MinScore, string(raw)); err != nil {
		writeErr(w, 500, "save failed")
		return
	}
	s.counting.Reload()
	var old any
	if oldRaw != "" {
		old = json.RawMessage(oldRaw)
	}
	s.audit(r, "counting_camera.update", "camera", id, old, map[string]any{"enabled": in.Enabled, "fps": in.FPS, "model": in.Model, "min_score": in.MinScore, "config": in.Config}, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true, "config": in.Config})
}

// ---- sensor inputs ------------------------------------------------------------------------------------------

// GET /api/v1/counting/inputs
func (s *Server) listCountingInputs(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT i.id::text, i.entrance_id::text, en.name, a.name, i.name, i.grp, coalesce(i.in_datastream_id::text, ''), coalesce(i.out_datastream_id::text, ''), i.mode, i.enabled
		FROM core.counting_input i JOIN core.counting_entrance en ON en.id = i.entrance_id JOIN core.counting_area a ON a.id = en.area_id
		WHERE i.org_id = $1::uuid AND i.archived_at IS NULL ORDER BY a.name, en.name, i.name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, en, enName, area, name, g, in, outDS, mode string
		var on bool
		if rows.Scan(&id, &en, &enName, &area, &name, &g, &in, &outDS, &mode, &on) == nil {
			out = append(out, map[string]any{"id": id, "entrance_id": en, "entrance": enName, "area": area, "name": name, "group": g, "in_datastream_id": in, "out_datastream_id": outDS, "mode": mode, "enabled": on})
		}
	}
	writeJSON(w, 200, out)
}

type inputBody struct {
	EntranceID string `json:"entrance_id"`
	Name       string `json:"name"`
	Group      string `json:"group"`
	InDS       string `json:"in_datastream_id"`
	OutDS      string `json:"out_datastream_id"`
	Mode       string `json:"mode"`
	Enabled    *bool  `json:"enabled"`
	Reason     string `json:"reason"`
}

func (s *Server) validInput(ctx context.Context, org string, in *inputBody) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 80 {
		return fmt.Errorf("name: 1 to 80 characters")
	}
	if in.Group == "" {
		in.Group = "people"
	}
	if !slices.Contains(counting.Groups, in.Group) {
		return fmt.Errorf("group: %s", strings.Join(counting.Groups, ", "))
	}
	if in.Mode == "" {
		in.Mode = "increment"
	}
	if in.Mode != "increment" && in.Mode != "total" {
		return fmt.Errorf("mode: increment (each value is a number of passages) or total (a running total)")
	}
	if in.InDS == "" && in.OutDS == "" {
		return fmt.Errorf("in_datastream_id or out_datastream_id required")
	}
	var n int
	if !uuidRe.MatchString(in.EntranceID) || s.pool.QueryRow(ctx, `SELECT count(*) FROM core.counting_entrance WHERE id = $1::uuid AND org_id = $2::uuid AND archived_at IS NULL`, in.EntranceID, org).Scan(&n) != nil || n == 0 {
		return fmt.Errorf("entrance not found")
	}
	for _, ds := range []string{in.InDS, in.OutDS} {
		if ds == "" {
			continue
		}
		if !uuidRe.MatchString(ds) || s.pool.QueryRow(ctx, `SELECT count(*) FROM core.datastream WHERE id = $1::uuid AND org_id = $2::uuid`, ds, org).Scan(&n) != nil || n == 0 {
			return fmt.Errorf("datastream %s not found", ds)
		}
		if s.counting.IsCountingAsset(ctx, ds) {
			return fmt.Errorf("datastream %s is a result of object counting, not a sensor", ds)
		}
	}
	return nil
}

// POST /api/v1/counting/inputs
func (s *Server) createCountingInput(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.countingReady(w) {
		return
	}
	var in inputBody
	if err := decodeJSON(r, &in, 8192); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if err := s.validInput(r.Context(), org, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	on := in.Enabled == nil || *in.Enabled
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.counting_input (org_id, entrance_id, name, grp, in_datastream_id, out_datastream_id, mode, enabled)
		VALUES ($1::uuid, $2::uuid, $3, $4, NULLIF($5, '')::uuid, NULLIF($6, '')::uuid, $7, $8) RETURNING id::text`, org, in.EntranceID, in.Name, in.Group, in.InDS, in.OutDS, in.Mode, on).Scan(&id); err != nil {
		writeErr(w, 409, "the input could not be created")
		return
	}
	s.counting.Reload()
	s.audit(r, "counting_input.create", "counting_input", id, nil, in, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id})
}

// PUT /api/v1/counting/inputs/{id}
func (s *Server) updateCountingInput(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.countingReady(w) {
		return
	}
	id := r.PathValue("id")
	var in inputBody
	if err := decodeJSON(r, &in, 8192); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) || !uuidRe.MatchString(id) {
		return
	}
	if err := s.validInput(r.Context(), org, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	on := in.Enabled == nil || *in.Enabled
	ct, err := s.pool.Exec(r.Context(), `UPDATE core.counting_input SET entrance_id = $3::uuid, name = $4, grp = $5, in_datastream_id = NULLIF($6, '')::uuid, out_datastream_id = NULLIF($7, '')::uuid,
		mode = $8, enabled = $9, last_values = '{}'::jsonb WHERE id = $1::uuid AND org_id = $2::uuid AND archived_at IS NULL`, id, org, in.EntranceID, in.Name, in.Group, in.InDS, in.OutDS, in.Mode, on)
	if err != nil || ct.RowsAffected() == 0 {
		writeErr(w, 404, "input not found")
		return
	}
	s.counting.Reload()
	s.audit(r, "counting_input.update", "counting_input", id, nil, in, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/v1/counting/inputs/{id}?reason=
func (s *Server) deleteCountingInput(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.countingReady(w) {
		return
	}
	id, reason := r.PathValue("id"), r.URL.Query().Get("reason")
	if !reasonOf(w, reason) || !uuidRe.MatchString(id) {
		return
	}
	ct, err := s.pool.Exec(r.Context(), `UPDATE core.counting_input SET archived_at = now() WHERE id = $1::uuid AND org_id = $2::uuid AND archived_at IS NULL`, id, org)
	if err != nil || ct.RowsAffected() == 0 {
		writeErr(w, 404, "input not found")
		return
	}
	s.counting.Reload()
	s.audit(r, "counting_input.delete", "counting_input", id, nil, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

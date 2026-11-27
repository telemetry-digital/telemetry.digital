package httpapi

// Batch record (spec 043, 0.97.0): when a batch ran on a production unit, what was loaded for it, who began and ended
// it, and how its values went — the trend of the batch and the statistics of its curves.
//
// A unit says how its batches begin and end:
//
//   - recipe: the load of a recipe record into the unit (its device and key prefix) begins a batch and ends the one
//     that ran; a person ends the last one.
//   - tag: a datastream of the controller. "run": not zero while a batch runs. "number": the number of the batch —
//     a new number ends the batch that ran and begins the next one, zero means none.
//
// Either way a person may begin and end a batch by hand (device.command, a reason, audited). One batch runs on a unit
// at a time. A batch is a record: it is never deleted, and a finished one keeps its statistics.
//
//	GET    /api/v1/batch-units                           (data.read)
//	POST   /api/v1/batch-units {name, mode, device_id, prefix, tag_datastream, tag_kind, curves, enabled, reason}   (device.manage)
//	PUT    /api/v1/batch-units/{id}
//	DELETE /api/v1/batch-units/{id}?reason=
//	GET    /api/v1/batches?unit=&status=&from=&to=&q=&limit=    newest first      (data.read)
//	GET    /api/v1/batches/{id}                          with the statistics of its curves
//	POST   /api/v1/batches {unit_id, code, reason}       begin by hand             (device.command)
//	POST   /api/v1/batches/{id}/end {reason}             end by hand

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const maxBatchUnits = 200

type batchUnit struct {
	ID, Name, Mode, DeviceID, Prefix, Tag, TagKind string
	Curves                                         []string
	Enabled                                        bool
}

// batchStats is what a batch keeps about one curve.
type batchStats struct {
	N     int64    `json:"n"`
	Min   *float64 `json:"min"`
	Max   *float64 `json:"max"`
	Avg   *float64 `json:"avg"`
	First *float64 `json:"first"`
	Last  *float64 `json:"last"`
}

// ---- beginning and ending ---------------------------------------------------------------------------------------------

type batchStart struct {
	org, unitID, unitName, source, code, by, operator, reason string
	at                                                        time.Time
	downloadID, recipeName, recordName                        string
	recordNumber                                              *int
}

// beginBatch ends the batch that runs on the unit and begins the next one, in one transaction.
func (s *Server) beginBatch(ctx context.Context, b batchStart) (string, int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", 0, err
	}
	defer tx.Rollback(ctx)
	var number int
	if err := tx.QueryRow(ctx, `UPDATE core.batch_unit SET next_number = next_number + 1 WHERE id = $1::uuid AND org_id = $2::uuid AND removed_at IS NULL RETURNING next_number - 1`, b.unitID, b.org).Scan(&number); err != nil {
		return "", 0, errors.New("unit not found")
	}
	var prev string
	var prevStart time.Time
	if err := tx.QueryRow(ctx, `SELECT id::text, started_at FROM core.batch WHERE unit_id = $1::uuid AND status = 'running' FOR UPDATE`, b.unitID).Scan(&prev, &prevStart); err == nil {
		end := b.at
		if end.Before(prevStart) {
			end = prevStart
		}
		if _, err := tx.Exec(ctx, `UPDATE core.batch SET status = 'finished', ended_at = $2, end_source = $3, ended_by = $4, end_reason = 'the next batch began' WHERE id = $1::uuid`, prev, end, b.source, b.by); err != nil {
			return "", 0, err
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", 0, err
	}
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO core.batch (org_id, unit_id, unit_name, number, code, started_at, start_source, recipe_download_id, recipe_name, record_name, record_number, started_by, operator, reason)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, NULLIF($8,'')::uuid, $9, $10, $11, $12, $13, $14) RETURNING id::text`,
		b.org, b.unitID, b.unitName, number, b.code, b.at, b.source, b.downloadID, b.recipeName, b.recordName, b.recordNumber, b.by, b.operator, b.reason).Scan(&id); err != nil {
		return "", 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", 0, err
	}
	if prev != "" {
		s.keepBatchStats(ctx, prev)
	}
	return id, number, nil
}

// finishBatch ends the batch that runs on a unit. It reports whether there was one.
func (s *Server) finishBatch(ctx context.Context, unitID string, at time.Time, source, by, reason string) (string, bool) {
	var id string
	if err := s.pool.QueryRow(ctx, `UPDATE core.batch SET status = 'finished', ended_at = GREATEST($2, started_at), end_source = $3, ended_by = $4, end_reason = $5
		WHERE unit_id = $1::uuid AND status = 'running' RETURNING id::text`, unitID, at, source, by, reason).Scan(&id); err != nil {
		return "", false
	}
	s.keepBatchStats(ctx, id)
	return id, true
}

// curveStats reads the statistics of datastreams between two moments.
func (s *Server) curveStats(ctx context.Context, org string, ids []string, from, to time.Time) map[string]batchStats {
	out := map[string]batchStats{}
	if len(ids) == 0 {
		return out
	}
	rows, err := s.pool.Query(ctx, `SELECT datastream_id::text, count(value), min(value)::float8, max(value)::float8, avg(value)::float8,
			(array_agg(value::float8 ORDER BY measured_at) FILTER (WHERE value IS NOT NULL))[1],
			(array_agg(value::float8 ORDER BY measured_at DESC) FILTER (WHERE value IS NOT NULL))[1]
		FROM core.observation WHERE org_id = $1::uuid AND datastream_id = ANY($2::uuid[]) AND measured_at >= $3 AND measured_at <= $4 GROUP BY datastream_id`, org, ids, from, to)
	if err != nil {
		s.log.Error("batch statistics", "err", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var st batchStats
		if rows.Scan(&id, &st.N, &st.Min, &st.Max, &st.Avg, &st.First, &st.Last) == nil {
			out[id] = st
		}
	}
	return out
}

// keepBatchStats stores the curves of the unit and their statistics with a finished batch: the record then says the
// same whatever is changed on the unit later.
func (s *Server) keepBatchStats(ctx context.Context, id string) {
	ctx = context.WithoutCancel(ctx)
	var org string
	var from time.Time
	var to *time.Time
	var curves []string
	if err := s.pool.QueryRow(ctx, `SELECT b.org_id::text, b.started_at, b.ended_at, ARRAY(SELECT jsonb_array_elements_text(u.curves)) FROM core.batch b JOIN core.batch_unit u ON u.id = b.unit_id WHERE b.id = $1::uuid`, id).
		Scan(&org, &from, &to, &curves); err != nil || to == nil {
		return
	}
	st, _ := json.Marshal(s.curveStats(ctx, org, curves, from, *to))
	cj, _ := json.Marshal(curves)
	if _, err := s.pool.Exec(ctx, `UPDATE core.batch SET stats = $2::jsonb, curves = $3::jsonb WHERE id = $1::uuid`, id, string(st), string(cj)); err != nil {
		s.log.Error("batch statistics not kept", "batch", id, "err", err)
	}
}

// batchFromRecipe begins a batch on every unit that follows the recipe loads of this device and prefix.
func (s *Server) batchFromRecipe(ctx context.Context, l recipeLoad, operator string) {
	ctx = context.WithoutCancel(ctx)
	rows, err := s.pool.Query(ctx, `SELECT id::text, name FROM core.batch_unit WHERE org_id = $1::uuid AND mode = 'recipe' AND enabled AND removed_at IS NULL AND device_id = $2::uuid AND prefix = $3`, l.org, l.deviceID, l.prefix)
	if err != nil {
		return
	}
	type u struct{ id, name string }
	var units []u
	for rows.Next() {
		var x u
		if rows.Scan(&x.id, &x.name) == nil {
			units = append(units, x)
		}
	}
	rows.Close()
	for _, x := range units {
		n := l.recordNumber
		if _, _, err := s.beginBatch(ctx, batchStart{org: l.org, unitID: x.id, unitName: x.name, source: "recipe", code: strconv.Itoa(l.recordNumber) + " " + l.recordName, by: l.by, operator: operator,
			at: time.Now(), downloadID: l.id, recipeName: l.recipeName, recordName: l.recordName, recordNumber: &n}); err != nil {
			s.log.Error("batch not begun after a recipe load", "unit", x.name, "err", err)
		}
	}
}

// ---- the tag of the controller ----------------------------------------------------------------------------------------

// batchTagStep decides what a new sample of the tag means. prev is the value before it (0 when none was seen).
func batchTagStep(kind string, prev, v float64) (end, begin bool, code string) {
	if math.IsNaN(v) || v == prev {
		return false, false, ""
	}
	switch kind {
	case "number":
		end = prev != 0
		begin = v != 0
		if begin {
			code = strconv.FormatFloat(v, 'f', -1, 64)
		}
	default: // run
		end = prev != 0 && v == 0
		begin = prev == 0 && v != 0
	}
	return end, begin, code
}

// batchTagTick reads the samples of the tags that arrived since the last look and begins and ends batches with the
// time of the sample. A unit that was just set up starts watching from now: what the tag did before is not a batch.
func (s *Server) batchTagTick(ctx context.Context) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, org_id::text, name, tag_datastream::text, tag_kind, tag_seen_at, COALESCE(tag_last, 0) FROM core.batch_unit WHERE mode = 'tag' AND enabled AND removed_at IS NULL`)
	if err != nil {
		return
	}
	type unit struct {
		id, org, name, ds, kind string
		seen                    *time.Time
		last                    float64
	}
	var units []unit
	for rows.Next() {
		var u unit
		if rows.Scan(&u.id, &u.org, &u.name, &u.ds, &u.kind, &u.seen, &u.last) == nil {
			units = append(units, u)
		}
	}
	rows.Close()
	for _, u := range units {
		if u.seen == nil {
			// the first look: remember where the tag stands, begin nothing
			_, _ = s.pool.Exec(ctx, `UPDATE core.batch_unit SET tag_seen_at = now(), tag_last = COALESCE((SELECT value::float8 FROM core.observation WHERE datastream_id = $2::uuid AND org_id = $3::uuid AND value IS NOT NULL
				AND measured_at > now() - interval '7 days' ORDER BY measured_at DESC LIMIT 1), 0) WHERE id = $1::uuid`, u.id, u.ds, u.org)
			continue
		}
		obs, err := s.pool.Query(ctx, `SELECT measured_at, value::float8 FROM core.observation WHERE datastream_id = $1::uuid AND org_id = $2::uuid AND value IS NOT NULL AND measured_at > $3
			AND quality IN ('ok', 'backfilled') ORDER BY measured_at LIMIT 2000`, u.ds, u.org, *u.seen)
		if err != nil {
			continue
		}
		type sample struct {
			at time.Time
			v  float64
		}
		var ss []sample
		for obs.Next() {
			var x sample
			if obs.Scan(&x.at, &x.v) == nil {
				ss = append(ss, x)
			}
		}
		obs.Close()
		if len(ss) == 0 {
			continue
		}
		last := u.last
		for _, x := range ss {
			end, begin, code := batchTagStep(u.kind, last, x.v)
			if begin {
				if _, _, err := s.beginBatch(ctx, batchStart{org: u.org, unitID: u.id, unitName: u.name, source: "tag", code: code, at: x.at}); err != nil {
					s.log.Error("batch not begun from its tag", "unit", u.name, "err", err)
				}
			} else if end {
				s.finishBatch(ctx, u.id, x.at, "tag", "", "")
			}
			last = x.v
		}
		_, _ = s.pool.Exec(ctx, `UPDATE core.batch_unit SET tag_seen_at = $2, tag_last = $3 WHERE id = $1::uuid`, u.id, ss[len(ss)-1].at, last)
	}
}

func (s *Server) batchLoop() {
	time.Sleep(20 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		s.batchTagTick(ctx)
		cancel()
		time.Sleep(5 * time.Second)
	}
}

// ---- units ------------------------------------------------------------------------------------------------------------

const batchUnitCols = `u.id::text, u.name, u.mode, COALESCE(u.device_id::text, ''), u.prefix, COALESCE(u.tag_datastream::text, ''), u.tag_kind, ARRAY(SELECT jsonb_array_elements_text(u.curves)), u.enabled`

func (s *Server) batchUnitByID(ctx context.Context, org, id string) (*batchUnit, error) {
	var u batchUnit
	err := s.pool.QueryRow(ctx, `SELECT `+batchUnitCols+` FROM core.batch_unit u WHERE u.id = $1::uuid AND u.org_id = $2::uuid AND u.removed_at IS NULL`, id, org).
		Scan(&u.ID, &u.Name, &u.Mode, &u.DeviceID, &u.Prefix, &u.Tag, &u.TagKind, &u.Curves, &u.Enabled)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GET /api/v1/batch-units
func (s *Server) listBatchUnits(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT `+batchUnitCols+`, COALESCE(d.name, ''), COALESCE(a.name || ' / ' || ds.key, ''),
			COALESCE((SELECT b.id::text FROM core.batch b WHERE b.unit_id = u.id AND b.status = 'running'), ''),
			(SELECT count(*) FROM core.batch b WHERE b.unit_id = u.id)
		FROM core.batch_unit u LEFT JOIN core.device d ON d.id = u.device_id LEFT JOIN core.datastream ds ON ds.id = u.tag_datastream LEFT JOIN core.asset a ON a.id = ds.asset_id
		WHERE u.org_id = $1::uuid AND u.removed_at IS NULL ORDER BY lower(u.name)`, org)
	if err != nil {
		s.log.Error("batch units", "err", err)
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var u batchUnit
		var dev, tag, running string
		var n int
		if err := rows.Scan(&u.ID, &u.Name, &u.Mode, &u.DeviceID, &u.Prefix, &u.Tag, &u.TagKind, &u.Curves, &u.Enabled, &dev, &tag, &running, &n); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, map[string]any{"id": u.ID, "name": u.Name, "mode": u.Mode, "device_id": u.DeviceID, "device": dev, "prefix": u.Prefix, "tag_datastream": u.Tag, "tag": tag, "tag_kind": u.TagKind,
			"curves": u.Curves, "enabled": u.Enabled, "running": running, "batches": n})
	}
	writeJSON(w, 200, out)
}

type batchUnitIn struct {
	Name    string   `json:"name"`
	Mode    string   `json:"mode"`
	Device  string   `json:"device_id"`
	Prefix  string   `json:"prefix"`
	Tag     string   `json:"tag_datastream"`
	TagKind string   `json:"tag_kind"`
	Curves  []string `json:"curves"`
	Enabled *bool    `json:"enabled"`
	Reason  string   `json:"reason"`
}

// checkBatchUnit validates a unit against the organization.
func (s *Server) checkBatchUnit(ctx context.Context, org string, in *batchUnitIn) error {
	in.Name, in.Prefix = strings.TrimSpace(in.Name), strings.TrimSpace(in.Prefix)
	in.Device, in.Tag = strings.ToLower(strings.TrimSpace(in.Device)), strings.ToLower(strings.TrimSpace(in.Tag))
	if in.Name == "" || len(in.Name) > 80 {
		return errors.New("name: 1 to 80 characters")
	}
	switch in.Mode {
	case "recipe":
		if !uuidRe.MatchString(in.Device) {
			return errors.New("a unit that follows recipe loads needs device_id")
		}
		if in.Prefix != "" && !cmdKeyRe.MatchString(in.Prefix) || len(in.Prefix) > 32 {
			return errors.New("prefix: at most 32 characters — letters, digits and _ . : / -")
		}
		var n int
		if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM core.device WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, in.Device, org).Scan(&n); err != nil || n != 1 {
			return errors.New("device not found")
		}
		in.Tag, in.TagKind = "", "run"
	case "tag":
		if !uuidRe.MatchString(in.Tag) {
			return errors.New("a unit that follows a tag needs tag_datastream")
		}
		if in.TagKind == "" {
			in.TagKind = "run"
		}
		if in.TagKind != "run" && in.TagKind != "number" {
			return errors.New("tag_kind must be run or number")
		}
		var n int
		if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM core.datastream WHERE id = $1::uuid AND org_id = $2::uuid`, in.Tag, org).Scan(&n); err != nil || n != 1 {
			return errors.New("the datastream of the tag was not found")
		}
		in.Device, in.Prefix = "", ""
	default:
		return errors.New("mode must be recipe or tag")
	}
	if len(in.Curves) > maxTrendCurves {
		return fmt.Errorf("at most %d curves", maxTrendCurves)
	}
	seen := map[string]bool{}
	for i, c := range in.Curves {
		c = strings.ToLower(strings.TrimSpace(c))
		if !uuidRe.MatchString(c) || seen[c] {
			return errors.New("curves must be different datastream ids")
		}
		seen[c] = true
		in.Curves[i] = c
	}
	if len(in.Curves) > 0 {
		var n int
		if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM core.datastream WHERE id = ANY($1::uuid[]) AND org_id = $2::uuid`, in.Curves, org).Scan(&n); err != nil || n != len(in.Curves) {
			return errors.New("a curve names a datastream that does not exist")
		}
	}
	if in.Curves == nil {
		in.Curves = []string{}
	}
	return nil
}

func (in *batchUnitIn) doc() map[string]any {
	return map[string]any{"name": in.Name, "mode": in.Mode, "device_id": in.Device, "prefix": in.Prefix, "tag_datastream": in.Tag, "tag_kind": in.TagKind, "curves": in.Curves, "enabled": in.Enabled == nil || *in.Enabled}
}

// POST /api/v1/batch-units
func (s *Server) createBatchUnit(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in batchUnitIn
	if err := decodeJSON(r, &in, 16384); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if err := s.checkBatchUnit(r.Context(), org, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var n int
	_ = s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.batch_unit WHERE org_id = $1::uuid AND removed_at IS NULL`, org).Scan(&n)
	if n >= maxBatchUnits {
		writeErr(w, 409, "at most 200 units")
		return
	}
	cj, _ := json.Marshal(in.Curves)
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.batch_unit (org_id, name, mode, device_id, prefix, tag_datastream, tag_kind, curves, enabled)
		VALUES ($1::uuid, $2, $3, NULLIF($4,'')::uuid, $5, NULLIF($6,'')::uuid, $7, $8::jsonb, $9) ON CONFLICT DO NOTHING RETURNING id::text`,
		org, in.Name, in.Mode, in.Device, in.Prefix, in.Tag, in.TagKind, string(cj), in.Enabled == nil || *in.Enabled).Scan(&id); err != nil {
		writeErr(w, 409, "a unit with this name exists")
		return
	}
	d := in.doc()
	d["id"] = id
	s.audit(r, "batch_unit.create", "batch_unit", id, nil, d, in.Reason)
	writeJSON(w, 201, d)
}

// PUT /api/v1/batch-units/{id}
func (s *Server) updateBatchUnit(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	old, err := s.batchUnitByID(r.Context(), org, id)
	if err != nil {
		writeErr(w, 404, "unit not found")
		return
	}
	var in batchUnitIn
	if err := decodeJSON(r, &in, 16384); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if err := s.checkBatchUnit(r.Context(), org, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	cj, _ := json.Marshal(in.Curves)
	// another tag, or another way of beginning, starts the watching anew: the past of the new tag is not a batch
	rewatch := in.Mode != old.Mode || in.Tag != old.Tag || in.TagKind != old.TagKind
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.batch_unit SET name = $2, mode = $3, device_id = NULLIF($4,'')::uuid, prefix = $5, tag_datastream = NULLIF($6,'')::uuid, tag_kind = $7, curves = $8::jsonb, enabled = $9, updated_at = now(),
		tag_seen_at = CASE WHEN $10 THEN NULL ELSE tag_seen_at END, tag_last = CASE WHEN $10 THEN NULL ELSE tag_last END WHERE id = $1::uuid`,
		id, in.Name, in.Mode, in.Device, in.Prefix, in.Tag, in.TagKind, string(cj), in.Enabled == nil || *in.Enabled, rewatch); err != nil {
		writeErr(w, 409, "a unit with this name exists")
		return
	}
	d := in.doc()
	d["id"] = id
	s.audit(r, "batch_unit.update", "batch_unit", id, map[string]any{"name": old.Name, "mode": old.Mode, "device_id": old.DeviceID, "prefix": old.Prefix, "tag_datastream": old.Tag, "tag_kind": old.TagKind, "curves": old.Curves, "enabled": old.Enabled}, d, in.Reason)
	writeJSON(w, 200, d)
}

// DELETE /api/v1/batch-units/{id}?reason= — the unit goes from the lists; its batches stay.
func (s *Server) deleteBatchUnit(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	u, err := s.batchUnitByID(r.Context(), org, id)
	if err != nil {
		writeErr(w, 404, "unit not found")
		return
	}
	p := PrincipalFrom(r.Context())
	s.finishBatch(r.Context(), id, time.Now(), "manual", p.Username, "the unit was removed: "+reason)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.batch_unit SET removed_at = now(), enabled = false WHERE id = $1::uuid`, id); err != nil {
		writeErr(w, 500, "delete failed")
		return
	}
	s.audit(r, "batch_unit.delete", "batch_unit", id, map[string]any{"name": u.Name}, nil, reason)
	writeJSON(w, 200, map[string]any{"id": id, "deleted": true})
}

// ---- batches ----------------------------------------------------------------------------------------------------------

const batchCols = `b.id::text, b.unit_id::text, b.unit_name, b.number, b.code, b.status, b.started_at, b.ended_at, b.start_source, b.end_source, COALESCE(b.recipe_download_id::text, ''),
	b.recipe_name, b.record_name, b.record_number, b.started_by, b.operator, b.ended_by, b.reason, b.end_reason`

func scanBatch(row pgx.Row) (map[string]any, error) {
	var id, unit, unitName, code, status, src, esrc, dl, recipe, record, by, operator, endedBy, reason, endReason string
	var number int
	var recNo *int
	var from time.Time
	var to *time.Time
	if err := row.Scan(&id, &unit, &unitName, &number, &code, &status, &from, &to, &src, &esrc, &dl, &recipe, &record, &recNo, &by, &operator, &endedBy, &reason, &endReason); err != nil {
		return nil, err
	}
	end := time.Now()
	if to != nil {
		end = *to
	}
	return map[string]any{"id": id, "unit_id": unit, "unit": unitName, "number": number, "code": code, "status": status, "started_at": from, "ended_at": to, "seconds": int(end.Sub(from).Seconds()),
		"start_source": src, "end_source": esrc, "recipe_download_id": dl, "recipe": recipe, "record": record, "record_number": recNo, "started_by": by, "operator": operator, "ended_by": endedBy,
		"reason": reason, "end_reason": endReason}, nil
}

// GET /api/v1/batches
func (s *Server) listBatches(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	from, to, ok := timeRange(w, r, 30*24*time.Hour)
	if !ok {
		return
	}
	if q.Get("to") == "" {
		to = to.Add(time.Hour)
	}
	unit := strings.ToLower(strings.TrimSpace(q.Get("unit")))
	if unit != "" && !uuidRe.MatchString(unit) {
		writeErr(w, 400, "unit must be an id")
		return
	}
	status := q.Get("status")
	if status != "" && status != "running" && status != "finished" {
		writeErr(w, 400, "status must be running or finished")
		return
	}
	text := strings.TrimSpace(q.Get("q"))
	if len(text) > 80 {
		writeErr(w, 400, "q: at most 80 characters")
		return
	}
	limit := 200
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 && n <= 1000 {
		limit = n
	}
	// a batch belongs to the range when it ran in it; a running batch always shows
	rows, err := s.pool.Query(r.Context(), `SELECT `+batchCols+` FROM core.batch b
		WHERE b.org_id = $1::uuid AND ($2 = '' OR b.unit_id = $2::uuid) AND ($3 = '' OR b.status = $3)
		  AND (b.status = 'running' OR (b.started_at < $5 AND COALESCE(b.ended_at, now()) >= $4))
		  AND ($6 = '' OR b.code ILIKE '%' || $6 || '%' OR b.unit_name ILIKE '%' || $6 || '%' OR b.recipe_name ILIKE '%' || $6 || '%' OR b.record_name ILIKE '%' || $6 || '%' OR b.number::text = $6)
		ORDER BY b.started_at DESC LIMIT $7`, org, unit, status, from, to, strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text), limit)
	if err != nil {
		s.log.Error("batches", "err", err)
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, b)
	}
	writeJSON(w, 200, out)
}

// GET /api/v1/batches/{id} — with its curves and their statistics: kept for a finished batch, read now for a running one.
func (s *Server) getBatch(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	b, err := scanBatch(s.pool.QueryRow(r.Context(), `SELECT `+batchCols+` FROM core.batch b WHERE b.id = $1::uuid AND b.org_id = $2::uuid`, id, org))
	if err != nil {
		writeErr(w, 404, "batch not found")
		return
	}
	var kept json.RawMessage
	var curves, unitCurves []string
	_ = s.pool.QueryRow(r.Context(), `SELECT b.stats, ARRAY(SELECT jsonb_array_elements_text(b.curves)), ARRAY(SELECT jsonb_array_elements_text(u.curves)) FROM core.batch b JOIN core.batch_unit u ON u.id = b.unit_id WHERE b.id = $1::uuid`, id).
		Scan(&kept, &curves, &unitCurves)
	stats := map[string]batchStats{}
	if b["status"] == "running" || len(kept) == 0 || string(kept) == "null" {
		curves = unitCurves
		end := time.Now()
		if t, _ := b["ended_at"].(*time.Time); t != nil {
			end = *t
		}
		stats = s.curveStats(r.Context(), org, curves, b["started_at"].(time.Time), end)
	} else {
		_ = json.Unmarshal(kept, &stats)
	}
	if curves == nil {
		curves = []string{}
	}
	b["curves"], b["stats"] = curves, stats
	// the batch before and after it on the unit, for the comparison
	var prev, next string
	_ = s.pool.QueryRow(r.Context(), `SELECT COALESCE((SELECT id::text FROM core.batch WHERE unit_id = $1::uuid AND number < $2 ORDER BY number DESC LIMIT 1), ''),
		COALESCE((SELECT id::text FROM core.batch WHERE unit_id = $1::uuid AND number > $2 ORDER BY number LIMIT 1), '')`, b["unit_id"], b["number"]).Scan(&prev, &next)
	b["previous"], b["next"] = prev, next
	writeJSON(w, 200, b)
}

// POST /api/v1/batches {unit_id, code, reason} — a person begins a batch.
func (s *Server) startBatch(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	var in struct {
		Unit   string `json:"unit_id"`
		Code   string `json:"code"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Code = strings.TrimSpace(in.Code)
	if len(in.Code) > 60 {
		writeErr(w, 400, "code: at most 60 characters")
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	u, err := s.batchUnitByID(r.Context(), org, strings.ToLower(strings.TrimSpace(in.Unit)))
	if err != nil {
		writeErr(w, 404, "unit not found")
		return
	}
	operator, ok := s.scadaGate(w, r, org, "batch", nil)
	if !ok {
		return
	}
	id, number, err := s.beginBatch(r.Context(), batchStart{org: org, unitID: u.ID, unitName: u.Name, source: "manual", code: in.Code, by: p.Username, operator: operator, reason: strings.TrimSpace(in.Reason), at: time.Now()})
	if err != nil {
		s.log.Error("batch begin", "err", err)
		writeErr(w, 500, "the batch could not be begun")
		return
	}
	s.audit(r, "batch.start", "batch", id, nil, map[string]any{"name": u.Name + " #" + strconv.Itoa(number), "unit": u.Name, "number": number, "code": in.Code}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id, "unit_id": u.ID, "unit": u.Name, "number": number, "code": in.Code, "status": "running"})
}

// POST /api/v1/batches/{id}/end {reason}
func (s *Server) endBatch(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
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
	var unit, unitName, status string
	var number int
	if err := s.pool.QueryRow(r.Context(), `SELECT unit_id::text, unit_name, number, status FROM core.batch WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).Scan(&unit, &unitName, &number, &status); err != nil {
		writeErr(w, 404, "batch not found")
		return
	}
	if status != "running" {
		writeErr(w, 409, "the batch has ended already")
		return
	}
	operator, ok := s.scadaGate(w, r, org, "batch", nil)
	if !ok {
		return
	}
	by := p.Username
	if operator != "" {
		by += " / " + operator
	}
	if got, ok := s.finishBatch(r.Context(), unit, time.Now(), "manual", by, strings.TrimSpace(in.Reason)); !ok || got != id {
		writeErr(w, 409, "the batch has ended already")
		return
	}
	s.audit(r, "batch.end", "batch", id, map[string]any{"status": "running"}, map[string]any{"name": unitName + " #" + strconv.Itoa(number), "status": "finished"}, in.Reason)
	writeJSON(w, 200, map[string]any{"id": id, "status": "finished"})
}

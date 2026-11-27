package httpapi

// Recipes (spec 038, 0.92.0). A recipe has elements (one writable key of a device each), units it can be loaded into
// and data records (named sets of values). Loading a record writes its values one after another through the path of
// every command: the connector checks the key, the type and its limits, the organization's four-eyes policy applies,
// every write is a command in the command log and the load itself is kept with what the controller answered.
//
//	GET    /api/v1/recipes                                  recipes with the number of records            (data.read)
//	POST   /api/v1/recipes                                  {name, description, elements, targets, reason} (config.write)
//	GET    /api/v1/recipes/{id}                             the recipe with its records                   (data.read)
//	PUT    /api/v1/recipes/{id}                             the same body as POST                         (config.write)
//	DELETE /api/v1/recipes/{id}?reason=                                                                   (config.write)
//	POST   /api/v1/recipes/{id}/records                     {number, name, values, note, reason}          (recipe.edit)
//	PUT    /api/v1/recipes/{id}/records/{rid}                                                             (recipe.edit)
//	DELETE /api/v1/recipes/{id}/records/{rid}?reason=                                                     (recipe.edit)
//	GET    /api/v1/recipes/{id}/compare?target=&record=     the values last read from the unit            (data.read)
//	POST   /api/v1/recipes/{id}/records/{rid}/download      {target, dry_run, reason}                     (device.command)
//	GET    /api/v1/recipes/{id}/export                      the records as CSV                            (data.export)
//	POST   /api/v1/recipes/{id}/import                      {csv, reason}                                 (recipe.edit)
//	GET    /api/v1/recipe-downloads?recipe=&limit=          loads, newest first                           (data.read)
//	POST   /api/v1/recipe-downloads/{id}/approve|reject     {reason}                                      (device.command)

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	recipeMaxElements = 200
	recipeMaxTargets  = 50
	recipeMaxRecords  = 1000
	recipeMaxText     = 254
	recipePendingFor  = 24 * time.Hour
)

var (
	recipePrefixRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,15}$`)
	recipeTargetRe = regexp.MustCompile(`^[a-z0-9]{1,12}$`)
	// one load at a time into a device: two interleaved loads would leave a mix of two records in the controller
	recipeLocks sync.Map
)

type recipeElement struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Unit     string   `json:"unit"`
	Type     string   `json:"type"` // number, bool, text
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
	Decimals int      `json:"decimals"`
}

type recipeTarget struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	DeviceID string `json:"device_id"`
	Prefix   string `json:"prefix"`
}

type recipeIn struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Elements    []recipeElement `json:"elements"`
	Targets     []recipeTarget  `json:"targets"`
	Reason      string          `json:"reason"`
}

// checkRecipe validates and normalizes the structure of a recipe; "" means it is usable.
func checkRecipe(in *recipeIn) string {
	in.Name, in.Description = strings.TrimSpace(in.Name), strings.TrimSpace(in.Description)
	if n := len([]rune(in.Name)); n < 1 || n > 80 {
		return "name: 1 to 80 characters"
	}
	if len([]rune(in.Description)) > 500 {
		return "description: at most 500 characters"
	}
	if len(in.Elements) < 1 || len(in.Elements) > recipeMaxElements {
		return fmt.Sprintf("elements: 1 to %d", recipeMaxElements)
	}
	seen := map[string]bool{}
	for i := range in.Elements {
		e := &in.Elements[i]
		e.Key, e.Label, e.Unit = strings.TrimSpace(e.Key), strings.TrimSpace(e.Label), strings.TrimSpace(e.Unit)
		at := fmt.Sprintf("element %d (%s)", i+1, e.Key)
		if !dsKeyRe.MatchString(e.Key) {
			return at + ": key must match ^[a-z][a-z0-9_]{0,31}$"
		}
		if seen[e.Key] {
			return at + ": the key is used twice"
		}
		seen[e.Key] = true
		if e.Label == "" {
			e.Label = e.Key
		}
		if len([]rune(e.Label)) > 80 || len([]rune(e.Unit)) > 16 {
			return at + ": label at most 80 and unit at most 16 characters"
		}
		switch e.Type {
		case "":
			e.Type = "number"
		case "number", "bool", "text":
		default:
			return at + ": type must be number, bool or text"
		}
		if e.Type != "number" {
			e.Min, e.Max, e.Decimals = nil, nil, 0
			continue
		}
		if e.Decimals < 0 || e.Decimals > 6 {
			return at + ": decimals 0 to 6"
		}
		for _, v := range []*float64{e.Min, e.Max} {
			if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0)) {
				return at + ": limits must be finite numbers"
			}
		}
		if e.Min != nil && e.Max != nil && *e.Min > *e.Max {
			return at + ": min is above max"
		}
	}
	if len(in.Targets) > recipeMaxTargets {
		return fmt.Sprintf("targets: at most %d", recipeMaxTargets)
	}
	ids, pairs := map[string]bool{}, map[string]bool{}
	for i := range in.Targets {
		t := &in.Targets[i]
		t.Name, t.Prefix, t.DeviceID = strings.TrimSpace(t.Name), strings.TrimSpace(t.Prefix), strings.ToLower(strings.TrimSpace(t.DeviceID))
		at := fmt.Sprintf("unit %d (%s)", i+1, t.Name)
		if n := len([]rune(t.Name)); n < 1 || n > 60 {
			return at + ": name 1 to 60 characters"
		}
		if !uuidRe.MatchString(t.DeviceID) {
			return at + ": choose a device"
		}
		if t.Prefix != "" && !recipePrefixRe.MatchString(t.Prefix) {
			return at + ": key prefix must match ^[a-z][a-z0-9_]{0,15}$"
		}
		for _, e := range in.Elements {
			if len(t.Prefix)+len(e.Key) > 32 {
				return at + ": prefix + key " + e.Key + " is longer than 32 characters"
			}
		}
		if t.ID == "" {
			for n := i + 1; ; n++ {
				if id := "u" + strconv.Itoa(n); !ids[id] && !recipeHasTarget(in.Targets, id) {
					t.ID = id
					break
				}
			}
		}
		if !recipeTargetRe.MatchString(t.ID) || ids[t.ID] {
			return at + ": unit id must be unique, ^[a-z0-9]{1,12}$"
		}
		ids[t.ID] = true
		if k := t.DeviceID + "|" + t.Prefix; pairs[k] {
			return at + ": the same device and prefix is listed twice"
		} else {
			pairs[k] = true
		}
	}
	return ""
}

func recipeHasTarget(ts []recipeTarget, id string) bool {
	for _, t := range ts {
		if t.ID == id {
			return true
		}
	}
	return false
}

// checkRecipeValues validates the values of a record against the elements: every element needs a value of its type
// within its limits. It returns the values to store (only the keys of the elements, numbers rounded to the decimals).
func checkRecipeValues(els []recipeElement, values map[string]any) (map[string]any, string) {
	out := make(map[string]any, len(els))
	for _, e := range els {
		v, ok := values[e.Key]
		if !ok || v == nil {
			return nil, e.Label + ": a value is needed"
		}
		switch e.Type {
		case "bool":
			b, ok := v.(bool)
			if !ok {
				return nil, e.Label + ": true or false is needed"
			}
			out[e.Key] = b
		case "text":
			t, ok := v.(string)
			if !ok {
				return nil, e.Label + ": text is needed"
			}
			if len(t) > recipeMaxText {
				return nil, fmt.Sprintf("%s: at most %d bytes of text", e.Label, recipeMaxText)
			}
			out[e.Key] = t
		default:
			var f float64
			switch n := v.(type) {
			case float64:
				f = n
			case json.Number:
				x, err := n.Float64()
				if err != nil {
					return nil, e.Label + ": a number is needed"
				}
				f = x
			default:
				return nil, e.Label + ": a number is needed"
			}
			if math.IsNaN(f) || math.IsInf(f, 0) {
				return nil, e.Label + ": a finite number is needed"
			}
			p := math.Pow(10, float64(e.Decimals))
			f = math.Round(f*p) / p
			if e.Min != nil && f < *e.Min {
				return nil, fmt.Sprintf("%s: %s is below the minimum %s", e.Label, fnum(f), fnum(*e.Min))
			}
			if e.Max != nil && f > *e.Max {
				return nil, fmt.Sprintf("%s: %s is above the maximum %s", e.Label, fnum(f), fnum(*e.Max))
			}
			out[e.Key] = f
		}
	}
	return out, ""
}

func fnum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// recipeNumber converts a value a connector read to a number.
func recipeNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	}
	return 0, false
}

// recipeSame reports whether a value read from the unit equals the value of the record (numbers within half of the
// last shown decimal: a REAL in the controller is not the decimal number of the record).
func recipeSame(e recipeElement, want, cur any) bool {
	switch e.Type {
	case "bool":
		w, _ := want.(bool)
		if b, ok := cur.(bool); ok {
			return b == w
		}
		n, ok := recipeNumber(cur)
		return ok && (n != 0) == w
	case "text":
		w, _ := want.(string)
		t, ok := cur.(string)
		return ok && t == w
	}
	w, ok := want.(float64)
	n, ok2 := recipeNumber(cur)
	if !ok || !ok2 {
		return false
	}
	return math.Abs(n-w) <= 0.5*math.Pow(10, -float64(e.Decimals))+1e-9*math.Max(1, math.Abs(w))
}

type recipeRow struct {
	ID, Name, Description string
	Elements              []recipeElement
	Targets               []recipeTarget
	UpdatedAt             time.Time
	UpdatedBy             string
}

func (s *Server) loadRecipe(ctx context.Context, org, id string) (*recipeRow, error) {
	if !uuidRe.MatchString(strings.ToLower(id)) {
		return nil, pgx.ErrNoRows
	}
	var rc recipeRow
	var els, tgs []byte
	if err := s.pool.QueryRow(ctx, `SELECT id::text, name, description, elements, targets, updated_at, updated_by FROM core.recipe
		WHERE id = $1::uuid AND org_id = $2::uuid AND deleted_at IS NULL`, id, org).Scan(&rc.ID, &rc.Name, &rc.Description, &els, &tgs, &rc.UpdatedAt, &rc.UpdatedBy); err != nil {
		return nil, err
	}
	if json.Unmarshal(els, &rc.Elements) != nil || json.Unmarshal(tgs, &rc.Targets) != nil {
		return nil, errors.New("stored recipe is not readable")
	}
	return &rc, nil
}

// recipeDevices returns name, external id and transport of the devices of an organization among ids.
func (s *Server) recipeDevices(ctx context.Context, org string, ids []string) (map[string][3]string, error) {
	out := map[string][3]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text, COALESCE(NULLIF(name, ''), external_id), external_id, transport FROM core.device
		WHERE org_id = $1::uuid AND revoked_at IS NULL AND id = ANY($2::uuid[])`, org, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, ext, tr string
		if err := rows.Scan(&id, &name, &ext, &tr); err != nil {
			return nil, err
		}
		out[id] = [3]string{name, ext, tr}
	}
	return out, rows.Err()
}

func recipeTargetsOut(ts []recipeTarget, devs map[string][3]string) []map[string]any {
	out := make([]map[string]any, 0, len(ts))
	for _, t := range ts {
		d, ok := devs[t.DeviceID]
		out = append(out, map[string]any{"id": t.ID, "name": t.Name, "device_id": t.DeviceID, "prefix": t.Prefix, "device": d[0], "transport": d[2], "available": ok})
	}
	return out
}

func (s *Server) listRecipes(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT c.id::text, c.name, c.description, jsonb_array_length(c.elements), jsonb_array_length(c.targets), c.updated_at, c.updated_by,
			(SELECT count(*) FROM core.recipe_record x WHERE x.recipe_id = c.id AND x.deleted_at IS NULL),
			(SELECT max(d.requested_at) FROM core.recipe_download d WHERE d.recipe_id = c.id AND d.status = 'done')
		FROM core.recipe c WHERE c.org_id = $1::uuid AND c.deleted_at IS NULL ORDER BY lower(c.name)`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, desc, by string
		var els, tgs, recs int
		var at time.Time
		var last *time.Time
		if err := rows.Scan(&id, &name, &desc, &els, &tgs, &at, &by, &recs, &last); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, map[string]any{"id": id, "name": name, "description": desc, "elements": els, "targets": tgs, "records": recs, "updated_at": at, "updated_by": by, "last_download": last})
	}
	writeJSON(w, 200, out)
}

type recipeRecord struct {
	ID        string         `json:"id"`
	Number    int            `json:"number"`
	Name      string         `json:"name"`
	Values    map[string]any `json:"values"`
	Note      string         `json:"note"`
	Version   int            `json:"version"`
	UpdatedAt time.Time      `json:"updated_at"`
	UpdatedBy string         `json:"updated_by"`
	Complete  bool           `json:"complete"`
	Problem   string         `json:"problem,omitempty"`
}

func (s *Server) recipeRecords(ctx context.Context, rc *recipeRow, only string) ([]recipeRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, number, name, "values", note, version, updated_at, updated_by FROM core.recipe_record
		WHERE recipe_id = $1::uuid AND deleted_at IS NULL AND ($2 = '' OR id = NULLIF($2, '')::uuid) ORDER BY number`, rc.ID, only)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []recipeRecord{}
	for rows.Next() {
		var x recipeRecord
		var raw []byte
		if err := rows.Scan(&x.ID, &x.Number, &x.Name, &raw, &x.Note, &x.Version, &x.UpdatedAt, &x.UpdatedBy); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &x.Values)
		// a record written before the elements changed may miss a value or break a new limit: it cannot be loaded
		_, x.Problem = checkRecipeValues(rc.Elements, x.Values)
		x.Complete = x.Problem == ""
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Server) getRecipe(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rc, err := s.loadRecipe(r.Context(), org, r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "recipe not found")
		return
	}
	recs, err := s.recipeRecords(r.Context(), rc, "")
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	ids := []string{}
	for _, t := range rc.Targets {
		ids = append(ids, t.DeviceID)
	}
	devs, _ := s.recipeDevices(r.Context(), org, ids)
	writeJSON(w, 200, map[string]any{"id": rc.ID, "name": rc.Name, "description": rc.Description, "elements": rc.Elements, "targets": recipeTargetsOut(rc.Targets, devs),
		"records": recs, "updated_at": rc.UpdatedAt, "updated_by": rc.UpdatedBy})
}

// saveRecipe creates (id == "") or changes a recipe.
func (s *Server) saveRecipe(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var in recipeIn
	if err := decodeJSON(r, &in, 256<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if msg := checkRecipe(&in); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	ids := []string{}
	for _, t := range in.Targets {
		ids = append(ids, t.DeviceID)
	}
	devs, err := s.recipeDevices(r.Context(), org, ids)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	for _, t := range in.Targets {
		d, ok := devs[t.DeviceID]
		if !ok {
			writeErr(w, 400, "unit "+t.Name+": device not found")
			return
		}
		if d[2] != "s7" && d[2] != "opcua" && d[2] != "modbus" {
			writeErr(w, 400, "unit "+t.Name+": recipes are loaded into devices of an S7, OPC UA or Modbus connector")
			return
		}
	}
	els, _ := json.Marshal(in.Elements)
	if in.Targets == nil {
		in.Targets = []recipeTarget{}
	}
	tgs, _ := json.Marshal(in.Targets)
	summary := map[string]any{"name": in.Name, "elements": len(in.Elements), "keys": recipeKeys(in.Elements), "targets": in.Targets}
	if id == "" {
		err := s.pool.QueryRow(r.Context(), `INSERT INTO core.recipe (org_id, name, description, elements, targets, updated_by) VALUES ($1::uuid, $2, $3, $4::jsonb, $5::jsonb, $6)
			ON CONFLICT (org_id, lower(name)) WHERE deleted_at IS NULL DO NOTHING RETURNING id::text`, org, in.Name, in.Description, string(els), string(tgs), p.Username).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, 409, "a recipe with this name exists")
			return
		}
		if err != nil {
			s.log.Error("recipe insert", "err", err)
			writeErr(w, 500, "recipe not saved")
			return
		}
		s.audit(r, "recipe.create", "recipe", id, nil, summary, in.Reason)
		writeJSON(w, 201, map[string]any{"id": id})
		return
	}
	old, err := s.loadRecipe(r.Context(), org, id)
	if err != nil {
		writeErr(w, 404, "recipe not found")
		return
	}
	var dup bool
	_ = s.pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM core.recipe WHERE org_id = $1::uuid AND lower(name) = lower($2) AND deleted_at IS NULL AND id <> $3::uuid)`, org, in.Name, id).Scan(&dup)
	if dup {
		writeErr(w, 409, "a recipe with this name exists")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.recipe SET name = $3, description = $4, elements = $5::jsonb, targets = $6::jsonb, updated_at = now(), updated_by = $7
		WHERE id = $1::uuid AND org_id = $2::uuid`, id, org, in.Name, in.Description, string(els), string(tgs), p.Username); err != nil {
		writeErr(w, 500, "recipe not saved")
		return
	}
	s.audit(r, "recipe.update", "recipe", id, map[string]any{"name": old.Name, "elements": len(old.Elements), "keys": recipeKeys(old.Elements), "targets": old.Targets}, summary, in.Reason)
	writeJSON(w, 200, map[string]any{"id": id})
}

func recipeKeys(els []recipeElement) []string {
	out := make([]string, len(els))
	for i, e := range els {
		out[i] = e.Key
	}
	return out
}

func (s *Server) deleteRecipe(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	rc, err := s.loadRecipe(r.Context(), org, r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "recipe not found")
		return
	}
	var pending bool
	_ = s.pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM core.recipe_download WHERE recipe_id = $1::uuid AND status IN ('pending', 'running'))`, rc.ID).Scan(&pending)
	if pending {
		writeErr(w, 409, "a load of this recipe is waiting or running")
		return
	}
	// the recipe and its records stay in the database for the loads that refer to them
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.recipe SET deleted_at = now() WHERE id = $1::uuid AND org_id = $2::uuid`, rc.ID, org); err != nil {
		writeErr(w, 500, "recipe not deleted")
		return
	}
	s.audit(r, "recipe.delete", "recipe", rc.ID, map[string]any{"name": rc.Name, "elements": len(rc.Elements)}, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

type recipeRecordIn struct {
	Number int            `json:"number"`
	Name   string         `json:"name"`
	Values map[string]any `json:"values"`
	Note   string         `json:"note"`
	Reason string         `json:"reason"`
}

func (in *recipeRecordIn) check(els []recipeElement) (map[string]any, string) {
	in.Name, in.Note = strings.TrimSpace(in.Name), strings.TrimSpace(in.Note)
	if n := len([]rune(in.Name)); n < 1 || n > 80 {
		return nil, "name: 1 to 80 characters"
	}
	if len([]rune(in.Note)) > 500 {
		return nil, "note: at most 500 characters"
	}
	if in.Number < 0 || in.Number > 99999 {
		return nil, "number: 1 to 99999"
	}
	return checkRecipeValues(els, in.Values)
}

// saveRecipeRecord creates (no {rid}) or changes a data record.
func (s *Server) saveRecipeRecord(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	rid := strings.ToLower(r.PathValue("rid"))
	rc, err := s.loadRecipe(r.Context(), org, r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "recipe not found")
		return
	}
	var in recipeRecordIn
	if err := decodeJSON(r, &in, 256<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	vals, msg := in.check(rc.Elements)
	if msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	vj, _ := json.Marshal(vals)
	if rid == "" {
		var n int
		_ = s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.recipe_record WHERE recipe_id = $1::uuid AND deleted_at IS NULL`, rc.ID).Scan(&n)
		if n >= recipeMaxRecords {
			writeErr(w, 409, fmt.Sprintf("a recipe holds at most %d records", recipeMaxRecords))
			return
		}
		var id string
		var number int
		err := s.pool.QueryRow(r.Context(), `INSERT INTO core.recipe_record (org_id, recipe_id, number, name, "values", note, updated_by)
			VALUES ($1::uuid, $2::uuid, CASE WHEN $3::int > 0 THEN $3::int ELSE (SELECT coalesce(max(number), 0) + 1 FROM core.recipe_record WHERE recipe_id = $2::uuid AND deleted_at IS NULL) END, $4, $5::jsonb, $6, $7)
			ON CONFLICT (recipe_id, number) WHERE deleted_at IS NULL DO NOTHING RETURNING id::text, number`, org, rc.ID, in.Number, in.Name, string(vj), in.Note, p.Username).Scan(&id, &number)
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, 409, "a record with this number exists")
			return
		}
		if err != nil {
			s.log.Error("recipe record insert", "err", err)
			writeErr(w, 500, "record not saved")
			return
		}
		s.audit(r, "recipe_record.create", "recipe_record", id, nil, map[string]any{"recipe": rc.Name, "number": number, "name": in.Name, "values": vals, "note": in.Note}, in.Reason)
		writeJSON(w, 201, map[string]any{"id": id, "number": number})
		return
	}
	olds, err := s.recipeRecords(r.Context(), rc, rid)
	if !uuidRe.MatchString(rid) || err != nil || len(olds) != 1 {
		writeErr(w, 404, "record not found")
		return
	}
	old := olds[0]
	if in.Number == 0 {
		in.Number = old.Number
	}
	tag, err := s.pool.Exec(r.Context(), `UPDATE core.recipe_record x SET number = $3, name = $4, "values" = $5::jsonb, note = $6, version = version + 1, updated_at = now(), updated_by = $7
		WHERE x.id = $1::uuid AND x.recipe_id = $2::uuid AND x.deleted_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM core.recipe_record y WHERE y.recipe_id = $2::uuid AND y.number = $3 AND y.deleted_at IS NULL AND y.id <> x.id)`,
		rid, rc.ID, in.Number, in.Name, string(vj), in.Note, p.Username)
	if err != nil {
		writeErr(w, 500, "record not saved")
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, 409, "a record with this number exists")
		return
	}
	s.audit(r, "recipe_record.update", "recipe_record", rid, map[string]any{"recipe": rc.Name, "number": old.Number, "name": old.Name, "values": old.Values, "note": old.Note, "version": old.Version},
		map[string]any{"number": in.Number, "name": in.Name, "values": vals, "note": in.Note, "version": old.Version + 1}, in.Reason)
	writeJSON(w, 200, map[string]any{"id": rid, "number": in.Number})
}

func (s *Server) deleteRecipeRecord(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	rid := strings.ToLower(r.PathValue("rid"))
	rc, err := s.loadRecipe(r.Context(), org, r.PathValue("id"))
	if err != nil || !uuidRe.MatchString(rid) {
		writeErr(w, 404, "record not found")
		return
	}
	olds, err := s.recipeRecords(r.Context(), rc, rid)
	if err != nil || len(olds) != 1 {
		writeErr(w, 404, "record not found")
		return
	}
	var pending bool
	_ = s.pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM core.recipe_download WHERE record_id = $1::uuid AND status IN ('pending', 'running'))`, rid).Scan(&pending)
	if pending {
		writeErr(w, 409, "a load of this record is waiting or running")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.recipe_record SET deleted_at = now() WHERE id = $1::uuid AND recipe_id = $2::uuid`, rid, rc.ID); err != nil {
		writeErr(w, 500, "record not deleted")
		return
	}
	s.audit(r, "recipe_record.delete", "recipe_record", rid, map[string]any{"recipe": rc.Name, "number": olds[0].Number, "name": olds[0].Name, "values": olds[0].Values}, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---- the values in the unit ----------------------------------------------------------------------

// recipeCurrent returns the values the connector last read for a device and the time of that poll.
func (s *Server) recipeCurrent(ctx context.Context, deviceID, transport string) (map[string]any, time.Time, bool) {
	switch {
	case transport == "s7" && s.s7 != nil:
		return s.s7.Current(ctx, deviceID)
	case transport == "opcua" && s.opc != nil:
		return s.opc.Current(ctx, deviceID)
	case transport == "modbus" && s.mb != nil:
		return s.mb.Current(ctx, deviceID)
	}
	return nil, time.Time{}, false
}

// GET /api/v1/recipes/{id}/compare?target=&record= — the value of every element as the connector last read it from
// the unit (its poll, not a read on demand) and, with a record, whether it equals the record.
func (s *Server) compareRecipe(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rc, err := s.loadRecipe(r.Context(), org, r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "recipe not found")
		return
	}
	var tg *recipeTarget
	for i := range rc.Targets {
		if rc.Targets[i].ID == r.URL.Query().Get("target") {
			tg = &rc.Targets[i]
		}
	}
	if tg == nil {
		writeErr(w, 400, "target: a unit of the recipe")
		return
	}
	var rec *recipeRecord
	if rid := strings.ToLower(r.URL.Query().Get("record")); rid != "" {
		recs, err := s.recipeRecords(r.Context(), rc, rid)
		if !uuidRe.MatchString(rid) || err != nil || len(recs) != 1 {
			writeErr(w, 404, "record not found")
			return
		}
		rec = &recs[0]
	}
	devs, _ := s.recipeDevices(r.Context(), org, []string{tg.DeviceID})
	now, at, connected := s.recipeCurrent(r.Context(), tg.DeviceID, devs[tg.DeviceID][2])
	items, differ, missing := []map[string]any{}, 0, 0
	for _, e := range rc.Elements {
		it := map[string]any{"key": e.Key, "label": e.Label, "unit": e.Unit, "type": e.Type, "decimals": e.Decimals}
		cur, have := now[tg.Prefix+e.Key]
		if have {
			it["current"] = cur
		} else {
			missing++
		}
		if rec != nil {
			it["value"] = rec.Values[e.Key]
			same := have && recipeSame(e, rec.Values[e.Key], cur)
			it["same"] = same
			if have && !same {
				differ++
			}
		}
		items = append(items, it)
	}
	out := map[string]any{"target": tg.ID, "connected": connected, "items": items, "differ": differ, "missing": missing}
	if !at.IsZero() {
		out["read_at"] = at
	}
	writeJSON(w, 200, out)
}

// ---- loading a record into a unit ----------------------------------------------------------------

type recipeItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Unit  string `json:"unit"`
	Value any    `json:"value"`
}

type recipeResult struct {
	Key       string `json:"key"`
	Status    string `json:"status"` // written, failed, not_written
	Readback  any    `json:"readback,omitempty"`
	Error     string `json:"error,omitempty"`
	CommandID string `json:"command_id,omitempty"`
}

type recipePrepared struct {
	item recipeItem
	args json.RawMessage
	op   connOp
}

// prepareRecipe validates every write of a load against the connector; problems lists what cannot be written.
func (s *Server) prepareRecipe(ctx context.Context, deviceID, transport, prefix string, items []recipeItem) (ops []recipePrepared, problems []map[string]string) {
	for _, it := range items {
		args, _ := json.Marshal(map[string]any{"key": prefix + it.Key, "value": it.Value})
		op, _, err := s.prepareConnector(ctx, deviceID, transport, "write", args)
		if err == nil && op.mb == nil && op.opc == nil && op.s7 == nil {
			err = errors.New("the device does not belong to an S7, OPC UA or Modbus connector")
		}
		if err != nil {
			problems = append(problems, map[string]string{"key": it.Key, "label": it.Label, "error": err.Error()})
			continue
		}
		ops = append(ops, recipePrepared{item: it, args: args, op: op})
	}
	return ops, problems
}

type recipeLoad struct {
	id, org, deviceID, ext, transport, prefix string
	recipeName, recordName, by, byID, reason  string
	recordNumber                              int
}

// runRecipeLoad writes the prepared values in order and stops at the first one the controller refuses: the values
// after it stay unwritten and are reported so. The caller holds the lock of the device.
func (s *Server) runRecipeLoad(ctx context.Context, l recipeLoad, ops []recipePrepared) (string, []recipeResult, string) {
	results := make([]recipeResult, 0, len(ops))
	status, firstErr := "done", ""
	why := fmt.Sprintf("recipe %s, record %d %s: %s", l.recipeName, l.recordNumber, l.recordName, l.reason)
	for _, o := range ops {
		res := recipeResult{Key: o.item.Key, Status: "not_written"}
		if status == "failed" {
			results = append(results, res)
			continue
		}
		exp := time.Now().Add(time.Minute).UTC()
		var cid string
		if err := s.pool.QueryRow(ctx, `INSERT INTO core.device_command (org_id, device_id, method, args, expires_at, issued_by, issued_by_name, reason, status)
			VALUES ($1::uuid, $2::uuid, 'write', $3::jsonb, $4, NULLIF($5,'')::uuid, $6, $7, 'queued') RETURNING id::text`, l.org, l.deviceID, string(o.args), exp, l.byID, l.by, why).Scan(&cid); err != nil {
			res.Status, res.Error = "failed", "the command could not be recorded"
			status, firstErr = "failed", o.item.Label+": "+res.Error
			results = append(results, res)
			continue
		}
		res.CommandID = cid
		st, result, cmdErr := s.dispatchCommand(ctx, cmdExec{org: l.org, deviceID: l.deviceID, ext: l.ext, transport: l.transport, cid: cid, method: "write", args: o.args, exp: exp, op: o.op})
		if st != "acked" {
			if cmdErr == "" {
				cmdErr = "not confirmed by the controller (status " + st + ")"
			}
			res.Status, res.Error = "failed", cmdErr
			status, firstErr = "failed", o.item.Label+": "+cmdErr
		} else {
			res.Status = "written"
			if m, ok := result.(map[string]any); ok {
				res.Readback = m["readback"]
			}
		}
		results = append(results, res)
	}
	rj, _ := json.Marshal(results)
	_, _ = s.pool.Exec(ctx, `UPDATE core.recipe_download SET status = $2, results = $3::jsonb, error = $4, finished_at = now() WHERE id = $1::uuid`, l.id, status, string(rj), firstErr)
	return status, results, firstErr
}

// POST /api/v1/recipes/{id}/records/{rid}/download
func (s *Server) downloadRecipe(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	var in struct {
		Target string `json:"target"`
		DryRun bool   `json:"dry_run"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	rc, err := s.loadRecipe(r.Context(), org, r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "recipe not found")
		return
	}
	rid := strings.ToLower(r.PathValue("rid"))
	recs, err := s.recipeRecords(r.Context(), rc, rid)
	if !uuidRe.MatchString(rid) || err != nil || len(recs) != 1 {
		writeErr(w, 404, "record not found")
		return
	}
	rec := recs[0]
	var tg *recipeTarget
	for i := range rc.Targets {
		if rc.Targets[i].ID == in.Target {
			tg = &rc.Targets[i]
		}
	}
	if tg == nil {
		writeErr(w, 400, "target: a unit of the recipe")
		return
	}
	if !rec.Complete {
		writeErr(w, 409, "the record does not fit the recipe ("+rec.Problem+"): open it, correct it and save it")
		return
	}
	if !in.DryRun && !reasonOf(w, in.Reason) {
		return
	}
	devs, err := s.recipeDevices(r.Context(), org, []string{tg.DeviceID})
	dev, found := devs[tg.DeviceID]
	if err != nil || !found {
		writeErr(w, 409, "the device of unit "+tg.Name+" does not exist any more")
		return
	}
	items := make([]recipeItem, len(rc.Elements))
	for i, e := range rc.Elements {
		items[i] = recipeItem{Key: e.Key, Label: e.Label, Unit: e.Unit, Value: rec.Values[e.Key]}
	}
	ops, problems := s.prepareRecipe(r.Context(), tg.DeviceID, dev[2], tg.Prefix, items)
	fourEyes := s.iam != nil && s.iam.Policy(r.Context(), org).FourEyesCommands
	if in.DryRun {
		if problems == nil {
			problems = []map[string]string{}
		}
		writeJSON(w, 200, map[string]any{"ok": len(problems) == 0, "problems": problems, "items": items, "four_eyes": fourEyes})
		return
	}
	// all or nothing before the first write: one value the connector would refuse and nothing is written
	if len(problems) > 0 {
		writeJSON(w, 409, map[string]any{"error": "nothing was written: " + problems[0]["label"] + ": " + problems[0]["error"], "problems": problems})
		return
	}
	initial := "running"
	if fourEyes {
		initial = "pending"
	}
	mu := recipeLock(tg.DeviceID)
	if !fourEyes {
		if !mu.TryLock() {
			writeErr(w, 409, "another recipe is being loaded into this device")
			return
		}
		defer mu.Unlock()
	}
	ij, _ := json.Marshal(items)
	var did string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.recipe_download (org_id, recipe_id, record_id, recipe_name, record_name, record_number, record_version, device_id, target_name, prefix, items, status, requested_by, requested_by_name, reason)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8::uuid, $9, $10, $11::jsonb, $12, NULLIF($13,'')::uuid, $14, $15) RETURNING id::text`,
		org, rc.ID, rec.ID, rc.Name, rec.Name, rec.Number, rec.Version, tg.DeviceID, tg.Name, tg.Prefix, string(ij), initial, p.UserID, p.Username, in.Reason).Scan(&did); err != nil {
		s.log.Error("recipe download insert", "err", err)
		writeErr(w, 500, "load not recorded")
		return
	}
	detail := map[string]any{"download_id": did, "recipe": rc.Name, "record": rec.Name, "number": rec.Number, "version": rec.Version, "unit": tg.Name, "device": dev[0], "prefix": tg.Prefix, "values": rec.Values}
	if fourEyes {
		detail["status"] = "pending"
		s.audit(r, "recipe.download", "recipe", rc.ID, nil, detail, in.Reason)
		writeJSON(w, 202, map[string]any{"id": did, "status": "pending", "second_person": map[string]any{"kind": "recipe", "id": did, "summary": "recipe " + rc.Name + " / " + rec.Name + " → " + tg.Name}})
		return
	}
	// the load is finished even when the browser goes away in the middle
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Minute)
	defer cancel()
	status, results, firstErr := s.runRecipeLoad(ctx, recipeLoad{id: did, org: org, deviceID: tg.DeviceID, ext: dev[1], transport: dev[2], prefix: tg.Prefix,
		recipeName: rc.Name, recordName: rec.Name, recordNumber: rec.Number, by: p.Username, byID: p.UserID, reason: in.Reason}, ops)
	detail["status"], detail["results"] = status, results
	s.audit(r, "recipe.download", "recipe", rc.ID, nil, detail, in.Reason)
	writeJSON(w, 201, map[string]any{"id": did, "status": status, "results": results, "error": firstErr})
}

func recipeLock(deviceID string) *sync.Mutex {
	m, _ := recipeLocks.LoadOrStore(deviceID, &sync.Mutex{})
	return m.(*sync.Mutex)
}

func (s *Server) listRecipeDownloads(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	limit := 50
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 500 {
		limit = n
	}
	recipe := strings.ToLower(r.URL.Query().Get("recipe"))
	if recipe != "" && !uuidRe.MatchString(recipe) {
		writeErr(w, 400, "recipe: an id")
		return
	}
	// a load nobody decided within a day is over: the plant has moved on since it was asked for
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.recipe_download SET status = 'expired', finished_at = now() WHERE org_id = $1::uuid AND status = 'pending' AND requested_at < now() - interval '24 hours'`, org)
	rows, err := s.pool.Query(r.Context(), `SELECT d.id::text, d.recipe_id::text, d.record_id::text, d.recipe_name, d.record_name, d.record_number, d.record_version, d.device_id::text, d.target_name, d.prefix,
			d.items, d.results, d.status, d.error, COALESCE(d.requested_by::text, ''), d.requested_by_name, d.requested_at, d.reason, d.decided_by_name, d.decided_at, d.decide_reason, d.finished_at
		FROM core.recipe_download d WHERE d.org_id = $1::uuid AND ($2 = '' OR d.recipe_id = NULLIF($2, '')::uuid) ORDER BY d.requested_at DESC LIMIT $3`, org, recipe, limit)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, rcp, rec, rname, recname, dev, tname, prefix, status, errText, byID, by, reason string
		var number, version int
		var items, results json.RawMessage
		var at time.Time
		var decidedBy, decideReason *string
		var decidedAt, finished *time.Time
		if err := rows.Scan(&id, &rcp, &rec, &rname, &recname, &number, &version, &dev, &tname, &prefix, &items, &results, &status, &errText, &byID, &by, &at, &reason, &decidedBy, &decidedAt, &decideReason, &finished); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, map[string]any{"id": id, "recipe_id": rcp, "record_id": rec, "recipe": rname, "record": recname, "number": number, "version": version, "device_id": dev, "unit": tname, "prefix": prefix,
			"items": items, "results": results, "status": status, "error": errText, "requested_by": by, "requested_at": at, "reason": reason,
			"decided_by": decidedBy, "decided_at": decidedAt, "decide_reason": decideReason, "finished_at": finished})
	}
	writeJSON(w, 200, out)
}

// POST /api/v1/recipe-downloads/{id}/approve and …/reject (four eyes: not the user who asked for the load)
func (s *Server) decideRecipeDownload(approve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org, ok := s.mustOrg(w, r)
		if !ok {
			return
		}
		p := PrincipalFrom(r.Context())
		id := strings.ToLower(r.PathValue("id"))
		var in struct {
			Reason string `json:"reason"`
		}
		if err := decodeJSON(r, &in, 4096); err != nil || !uuidRe.MatchString(id) {
			writeErr(w, 400, "invalid JSON")
			return
		}
		if !reasonOf(w, in.Reason) {
			return
		}
		var l recipeLoad
		var status, recipeID string
		var raw []byte
		var at time.Time
		if err := s.pool.QueryRow(r.Context(), `SELECT d.status, d.recipe_id::text, d.recipe_name, d.record_name, d.record_number, d.device_id::text, d.prefix, d.items, COALESCE(d.requested_by::text, ''), d.requested_by_name, d.requested_at, d.reason
			FROM core.recipe_download d WHERE d.id = $1::uuid AND d.org_id = $2::uuid`, id, org).
			Scan(&status, &recipeID, &l.recipeName, &l.recordName, &l.recordNumber, &l.deviceID, &l.prefix, &raw, &l.byID, &l.by, &at, &l.reason); err != nil {
			writeErr(w, 404, "load not found")
			return
		}
		if status != "pending" {
			writeErr(w, 409, "the load is not waiting for approval (status "+status+")")
			return
		}
		if time.Since(at) > recipePendingFor {
			_, _ = s.pool.Exec(r.Context(), `UPDATE core.recipe_download SET status = 'expired', finished_at = now() WHERE id = $1::uuid AND status = 'pending'`, id)
			writeErr(w, 409, "the load expired before approval")
			return
		}
		old := map[string]any{"download_id": id, "status": "pending", "requested_by": l.by}
		if !approve {
			if _, err := s.pool.Exec(r.Context(), `UPDATE core.recipe_download SET status = 'rejected', decided_by_name = $2, decided_at = now(), decide_reason = $3, finished_at = now() WHERE id = $1::uuid AND status = 'pending'`, id, p.Username, in.Reason); err != nil {
				writeErr(w, 500, "update failed")
				return
			}
			s.audit(r, "recipe.download_reject", "recipe", recipeID, old, map[string]any{"download_id": id, "status": "rejected", "rejected_by": p.Username}, in.Reason)
			writeJSON(w, 200, map[string]any{"id": id, "status": "rejected"})
			return
		}
		if (l.byID != "" && l.byID == p.UserID) || (l.byID == "" && l.by == p.Username) {
			writeErr(w, 403, "four eyes: a different user must approve this load")
			return
		}
		var items []recipeItem
		if json.Unmarshal(raw, &items) != nil {
			writeErr(w, 500, "stored load is not readable")
			return
		}
		devs, err := s.recipeDevices(r.Context(), org, []string{l.deviceID})
		dev, found := devs[l.deviceID]
		if err != nil || !found {
			writeErr(w, 409, "the device does not exist any more")
			return
		}
		// the connector may have changed since the load was asked for: everything is checked again before the first write
		ops, problems := s.prepareRecipe(r.Context(), l.deviceID, dev[2], l.prefix, items)
		if len(problems) > 0 {
			writeJSON(w, 409, map[string]any{"error": "nothing was written: " + problems[0]["label"] + ": " + problems[0]["error"], "problems": problems})
			return
		}
		mu := recipeLock(l.deviceID)
		if !mu.TryLock() {
			writeErr(w, 409, "another recipe is being loaded into this device")
			return
		}
		defer mu.Unlock()
		tag, err := s.pool.Exec(r.Context(), `UPDATE core.recipe_download SET status = 'running', decided_by_name = $2, decided_at = now(), decide_reason = $3 WHERE id = $1::uuid AND status = 'pending'`, id, p.Username, in.Reason)
		if err != nil || tag.RowsAffected() != 1 {
			writeErr(w, 409, "the load is not waiting for approval any more")
			return
		}
		l.id, l.org, l.ext, l.transport = id, org, dev[1], dev[2]
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Minute)
		defer cancel()
		st, results, firstErr := s.runRecipeLoad(ctx, l, ops)
		s.audit(r, "recipe.download_approve", "recipe", recipeID, old, map[string]any{"download_id": id, "status": st, "approved_by": p.Username, "results": results}, in.Reason)
		writeJSON(w, 200, map[string]any{"id": id, "status": st, "results": results, "error": firstErr})
	}
}

// ---- CSV -----------------------------------------------------------------------------------------

// recipeCSV writes the records as a table: number, name, note and one column per element (its key).
func recipeCSV(els []recipeElement, recs []recipeRecord) []byte {
	var b bytes.Buffer
	b.WriteString("\ufeff") // spreadsheets read UTF-8 with the mark
	cw := csv.NewWriter(&b)
	head := []string{"number", "name", "note"}
	for _, e := range els {
		head = append(head, e.Key)
	}
	_ = cw.Write(head)
	for _, x := range recs {
		row := []string{strconv.Itoa(x.Number), x.Name, x.Note}
		for _, e := range els {
			switch v := x.Values[e.Key].(type) {
			case float64:
				row = append(row, fnum(v))
			case bool:
				row = append(row, strconv.FormatBool(v))
			case string:
				row = append(row, v)
			default:
				row = append(row, "")
			}
		}
		_ = cw.Write(row)
	}
	cw.Flush()
	return b.Bytes()
}

// parseRecipeCSV reads records from the table recipeCSV writes (comma or semicolon; a decimal comma is accepted with
// semicolons). Every element needs its column; unknown columns are refused, so a typing error is not lost silently.
func parseRecipeCSV(els []recipeElement, text string) ([]recipeRecordIn, error) {
	text = strings.TrimPrefix(text, "\ufeff")
	first, _, _ := strings.Cut(text, "\n")
	semi := strings.Count(first, ";") > strings.Count(first, ",")
	cr := csv.NewReader(strings.NewReader(text))
	cr.FieldsPerRecord, cr.TrimLeadingSpace = -1, true
	if semi {
		cr.Comma = ';'
	}
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("the table cannot be read: %v", err)
	}
	if len(rows) < 2 {
		return nil, errors.New("the table needs a header line and at least one record")
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		h = strings.ToLower(strings.TrimSpace(h))
		if _, dup := col[h]; dup {
			return nil, fmt.Errorf("column %q is there twice", h)
		}
		col[h] = i
	}
	byKey := map[string]recipeElement{}
	for _, e := range els {
		byKey[e.Key] = e
		if _, ok := col[e.Key]; !ok {
			return nil, fmt.Errorf("column %q is missing", e.Key)
		}
	}
	for _, need := range []string{"number", "name"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("column %q is missing", need)
		}
	}
	for h := range col {
		if _, ok := byKey[h]; !ok && h != "number" && h != "name" && h != "note" {
			return nil, fmt.Errorf("column %q is not an element of the recipe", h)
		}
	}
	cell := func(row []string, name string) string {
		i, ok := col[name]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	var out []recipeRecordIn
	nums := map[int]bool{}
	for n, row := range rows[1:] {
		if len(row) == 0 || (len(row) == 1 && strings.TrimSpace(row[0]) == "") {
			continue
		}
		line := fmt.Sprintf("line %d", n+2)
		num, err := strconv.Atoi(cell(row, "number"))
		if err != nil || num < 1 || num > 99999 {
			return nil, errors.New(line + ": number must be 1 to 99999")
		}
		if nums[num] {
			return nil, fmt.Errorf("%s: number %d is there twice", line, num)
		}
		nums[num] = true
		in := recipeRecordIn{Number: num, Name: cell(row, "name"), Note: cell(row, "note"), Values: map[string]any{}}
		for _, e := range els {
			c := cell(row, e.Key)
			switch e.Type {
			case "text":
				in.Values[e.Key] = c
			case "bool":
				switch strings.ToLower(c) {
				case "true", "1":
					in.Values[e.Key] = true
				case "false", "0":
					in.Values[e.Key] = false
				default:
					return nil, fmt.Errorf("%s, %s: true or false is needed", line, e.Key)
				}
			default:
				if semi {
					c = strings.Replace(c, ",", ".", 1)
				}
				f, err := strconv.ParseFloat(c, 64)
				if err != nil {
					return nil, fmt.Errorf("%s, %s: %q is not a number", line, e.Key, c)
				}
				in.Values[e.Key] = f
			}
		}
		vals, msg := in.check(els)
		if msg != "" {
			return nil, errors.New(line + ": " + msg)
		}
		in.Values = vals
		out = append(out, in)
	}
	if len(out) == 0 {
		return nil, errors.New("the table holds no record")
	}
	if len(out) > recipeMaxRecords {
		return nil, fmt.Errorf("a recipe holds at most %d records", recipeMaxRecords)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}

func (s *Server) exportRecipe(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rc, err := s.loadRecipe(r.Context(), org, r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "recipe not found")
		return
	}
	recs, err := s.recipeRecords(r.Context(), rc, "")
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	s.audit(r, "recipe.export", "recipe", rc.ID, nil, map[string]any{"name": rc.Name, "records": len(recs)}, "export")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="recipe-records.csv"`)
	_, _ = w.Write(recipeCSV(rc.Elements, recs))
}

// POST /api/v1/recipes/{id}/import — records by number: an existing number is changed, a new one is added; all or nothing.
func (s *Server) importRecipe(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	rc, err := s.loadRecipe(r.Context(), org, r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "recipe not found")
		return
	}
	var in struct {
		CSV    string `json:"csv"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 2<<20); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	recs, err := parseRecipeCSV(rc.Elements, in.CSV)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, 500, "import failed")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	created, updated := 0, 0
	for _, x := range recs {
		vj, _ := json.Marshal(x.Values)
		tag, err := tx.Exec(r.Context(), `UPDATE core.recipe_record SET name = $3, "values" = $4::jsonb, note = $5, version = version + 1, updated_at = now(), updated_by = $6
			WHERE recipe_id = $1::uuid AND number = $2 AND deleted_at IS NULL`, rc.ID, x.Number, x.Name, string(vj), x.Note, p.Username)
		if err == nil && tag.RowsAffected() == 0 {
			_, err = tx.Exec(r.Context(), `INSERT INTO core.recipe_record (org_id, recipe_id, number, name, "values", note, updated_by) VALUES ($1::uuid, $2::uuid, $3, $4, $5::jsonb, $6, $7)`,
				org, rc.ID, x.Number, x.Name, string(vj), x.Note, p.Username)
			created++
		} else {
			updated++
		}
		if err != nil {
			s.log.Error("recipe import", "err", err)
			writeErr(w, 500, "import failed, nothing was changed")
			return
		}
	}
	var total int
	_ = tx.QueryRow(r.Context(), `SELECT count(*) FROM core.recipe_record WHERE recipe_id = $1::uuid AND deleted_at IS NULL`, rc.ID).Scan(&total)
	if total > recipeMaxRecords {
		writeErr(w, 409, fmt.Sprintf("a recipe holds at most %d records", recipeMaxRecords))
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, 500, "import failed, nothing was changed")
		return
	}
	numbers := make([]int, len(recs))
	for i, x := range recs {
		numbers[i] = x.Number
	}
	s.audit(r, "recipe.import", "recipe", rc.ID, nil, map[string]any{"name": rc.Name, "created": created, "updated": updated, "numbers": numbers}, in.Reason)
	writeJSON(w, 200, map[string]any{"created": created, "updated": updated})
}

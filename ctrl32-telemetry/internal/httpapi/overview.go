package httpapi

// Overviews (0.68.0): datastreams selected by a query instead of a fixed list, for dashboard filters, dynamic widget
// data sources and overview tables.
//
//	GET  /api/v1/datastreams/query    (data.read)   datastreams matching site/asset type/quantity/device/attribute/
//	                                                text/name, with latest value, active alarm, alarm-rule limits and
//	                                                (from/to) minimum, maximum and change in a window
//	GET  /api/v1/datastreams/facets   (data.read)   values to offer in dashboard filters, narrowed by the other filters
//	POST /api/v1/datastreams/export   (data.export) the rows of an overview table as Excel or CSV
//	GET  /api/v1/public/{token}/datastreams/query, /facets   the same for a public link, within the dashboard's scope
//
// Every query is scoped to the caller's organization; filters only narrow (several values of one filter are OR,
// different filters AND). Plain parameters (site=…) and filter parameters (f_site=…) both apply, so a widget's own
// source and the dashboard filters combine. Limits: 500 rows per query, 2000 per export.

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/i18n"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/xlsx"
)

// dsSel is one set of constraints on datastreams; the constraints of a query are the AND of its sets.
type dsSel struct {
	Sites, Devices, IDs    []string
	AssetTypes, Quantities []string
	Keys                   []string // datastream keys (aliases narrowed to some keys, 0.73.0)
	Attrs                  map[string][]string
	Text, Name             string
}

type sqlArgs struct{ args []any }

func (a *sqlArgs) add(v any) string {
	a.args = append(a.args, v)
	return "$" + strconv.Itoa(len(a.args))
}

const dsFrom = ` FROM core.datastream ds JOIN core.asset a ON a.id = ds.asset_id JOIN core.site st ON st.id = a.site_id
	LEFT JOIN LATERAL (SELECT d.id, d.name FROM core.assignment x JOIN core.device d ON d.id = x.from_id
		WHERE x.kind = 'device_datastream' AND x.to_id = ds.id AND x.valid_to IS NULL ORDER BY x.valid_from DESC LIMIT 1) dv ON true`

var likeRepl = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// likeContains is an ILIKE pattern for "contains the text"; likeName also understands * and ? as wildcards.
func likeContains(s string) string { return "%" + likeRepl.Replace(s) + "%" }
func likeName(p string) string {
	if !strings.ContainsAny(p, "*?") {
		return likeContains(p)
	}
	return strings.NewReplacer("*", "%", "?", "_").Replace(likeRepl.Replace(p))
}

// where returns the SQL conditions of the set; skip leaves out one dimension (facets: a filter does not narrow its
// own choices). IDs are never skipped: they are the scope.
func (d dsSel) where(a *sqlArgs, skip string) []string {
	var w []string
	if len(d.IDs) > 0 {
		w = append(w, "ds.id = ANY("+a.add(d.IDs)+"::uuid[])")
	}
	if len(d.Sites) > 0 && skip != "site" {
		w = append(w, "st.id = ANY("+a.add(d.Sites)+"::uuid[])")
	}
	if len(d.AssetTypes) > 0 && skip != "asset_type" {
		w = append(w, inList("a.type", d.AssetTypes, a))
	}
	if len(d.Quantities) > 0 && skip != "quantity" {
		w = append(w, inList("ds.quantity", d.Quantities, a))
	}
	if len(d.Keys) > 0 {
		w = append(w, "ds.key = ANY("+a.add(d.Keys)+"::text[])")
	}
	if len(d.Devices) > 0 && skip != "device" {
		w = append(w, "dv.id = ANY("+a.add(d.Devices)+"::uuid[])")
	}
	for _, k := range sortedKeys(d.Attrs) {
		if skip == "attr:"+k {
			continue
		}
		kk := a.add(k)
		w = append(w, "COALESCE(ds.attributes->>"+kk+", a.attributes->>"+kk+") = ANY("+a.add(d.Attrs[k])+"::text[])")
	}
	if d.Text != "" && skip != "text" {
		t := a.add(likeContains(d.Text))
		w = append(w, "(ds.key ILIKE "+t+" OR ds.quantity ILIKE "+t+" OR a.name ILIKE "+t+" OR a.type ILIKE "+t+" OR st.name ILIKE "+t+" OR COALESCE(dv.name, '') ILIKE "+t+" OR COALESCE(ds.attributes->>'name', '') ILIKE "+t+")")
	}
	if d.Name != "" {
		n := a.add(likeName(d.Name))
		w = append(w, "((a.name || ' / ' || ds.key) ILIKE "+n+" OR a.name ILIKE "+n+" OR ds.key ILIKE "+n+")")
	}
	return w
}

// inList matches a column against values; a value ending in * matches every value starting with it (energy*).
func inList(col string, vals []string, a *sqlArgs) string {
	var exact, pre []string
	for _, v := range vals {
		if p, ok := strings.CutSuffix(v, "*"); ok && p != "" && !strings.ContainsAny(p, "*?") {
			pre = append(pre, likeRepl.Replace(p)+"%")
		} else {
			exact = append(exact, v)
		}
	}
	var parts []string
	if len(exact) > 0 {
		parts = append(parts, col+" = ANY("+a.add(exact)+"::text[])")
	}
	if len(pre) > 0 {
		parts = append(parts, col+" LIKE ANY("+a.add(pre)+"::text[])")
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

func whereOf(org string, sels []dsSel, a *sqlArgs, skip string) string {
	parts := []string{"ds.org_id = " + a.add(org) + "::uuid"}
	for _, s := range sels {
		parts = append(parts, s.where(a, skip)...)
	}
	return " WHERE " + strings.Join(parts, " AND ")
}

// parseSel reads one set of constraints from query parameters with the prefix ("" for a source, "f_" for filters).
func parseSel(v url.Values, prefix string) (dsSel, error) {
	var d dsSel
	var err error
	if d.Sites, err = uuidList(splitList(v.Get(prefix+"site")), maxFilterValues, prefix+"site"); err != nil {
		return d, err
	}
	if d.Devices, err = uuidList(splitList(v.Get(prefix+"device")), maxFilterValues, prefix+"device"); err != nil {
		return d, err
	}
	if d.AssetTypes, err = stringList(splitList(v.Get(prefix+"asset_type")), maxFilterValues, prefix+"asset_type"); err != nil {
		return d, err
	}
	if d.Quantities, err = stringList(splitList(v.Get(prefix+"quantity")), maxFilterValues, prefix+"quantity"); err != nil {
		return d, err
	}
	for _, kv := range v[prefix+"attr"] {
		k, val, ok := strings.Cut(kv, "=")
		k = strings.TrimSpace(k)
		if !ok || !attrKeyRe.MatchString(k) {
			return d, fmt.Errorf("%sattr must be key=value", prefix)
		}
		if val = strings.TrimSpace(val); val == "" {
			continue
		}
		if len(val) > 120 {
			return d, fmt.Errorf("%sattr: value too long", prefix)
		}
		if d.Attrs == nil {
			d.Attrs = map[string][]string{}
		}
		if len(d.Attrs) >= 8 || len(d.Attrs[k]) >= maxFilterValues {
			return d, fmt.Errorf("%sattr: too many values", prefix)
		}
		d.Attrs[k] = append(d.Attrs[k], val)
	}
	if t := strings.TrimSpace(v.Get(prefix + "q")); t != "" {
		if len(t) > 80 {
			return d, fmt.Errorf("%sq: at most 80 characters", prefix)
		}
		d.Text = t
	}
	if prefix == "" {
		if n := strings.TrimSpace(v.Get("name")); n != "" {
			if len(n) > 80 {
				return d, errors.New("name: at most 80 characters")
			}
			d.Name = n
		}
		if d.IDs, err = uuidList(splitList(v.Get("ids")), maxSourceRows, "ids"); err != nil {
			return d, err
		}
	}
	return d, nil
}

func anyStrings(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if x != "" {
			return []string{x}
		}
	}
	return nil
}

// selFromSource converts a canonical widget source (stored spec) into constraints.
func selFromSource(src map[string]any) dsSel {
	d := dsSel{Sites: anyStrings(src["site"]), Devices: anyStrings(src["device"]), AssetTypes: anyStrings(src["asset_type"]), Quantities: anyStrings(src["quantity"])}
	d.Name, _ = src["name"].(string)
	if m, ok := src["attr"].(map[string]any); ok {
		d.Attrs = map[string][]string{}
		for k, v := range m {
			if attrKeyRe.MatchString(k) {
				d.Attrs[k] = anyStrings(v)
			}
		}
	}
	return d
}

func sourceLimitOf(src map[string]any, typ string) int {
	n := sourceLimit(typ)
	if f, ok := numOf(src["limit"]); ok && f >= 1 && int(f) < n {
		n = int(f)
	}
	return n
}

// window of a query: minimum, maximum and change of each datastream between from and to.
type dsWindow struct{ From, To time.Time }

func parseWindow(v url.Values) (*dsWindow, error) {
	if v.Get("from") == "" {
		return nil, nil
	}
	from, err := time.Parse(time.RFC3339, v.Get("from"))
	if err != nil {
		return nil, errors.New("from must be RFC3339")
	}
	to := time.Now()
	if v.Get("to") != "" {
		if to, err = time.Parse(time.RFC3339, v.Get("to")); err != nil {
			return nil, errors.New("to must be RFC3339")
		}
	}
	if !to.After(from) || to.Sub(from) > 400*24*time.Hour {
		return nil, errors.New("from/to: a window of at most 400 days")
	}
	return &dsWindow{From: from, To: to}, nil
}

// queryDS returns the datastreams matching all sets, ordered by site, asset and key, and the number of all matches.
func (s *Server) queryDS(ctx context.Context, org string, sels []dsSel, limit int) ([]map[string]any, []string, int, error) {
	a := &sqlArgs{}
	where := whereOf(org, sels, a, "")
	q := `SELECT ds.id::text, ds.key, ds.quantity, ds.unit, ds.kind, COALESCE(ds.attributes->>'name', ''), a.id::text, a.name, a.type, st.id::text, st.name,
		COALESCE(EXTRACT(EPOCH FROM ds.expected_interval)::bigint, 0), COALESCE(dv.id::text, ''), COALESCE(dv.name, ''), count(*) OVER ()` + dsFrom + where +
		` ORDER BY st.name, a.name, ds.key LIMIT ` + a.add(limit)
	rows, err := s.pool.Query(ctx, q, a.args...)
	if err != nil {
		return nil, nil, 0, err
	}
	defer rows.Close()
	items := []map[string]any{}
	ids := []string{}
	total := 0
	for rows.Next() {
		var id, key, qty, unit, kind, label, assetID, asset, atype, siteID, site, devID, dev string
		var interval int64
		if err := rows.Scan(&id, &key, &qty, &unit, &kind, &label, &assetID, &asset, &atype, &siteID, &site, &interval, &devID, &dev, &total); err != nil {
			return nil, nil, 0, err
		}
		it := map[string]any{"id": id, "key": key, "quantity": qty, "unit": unit, "kind": kind, "asset_id": assetID, "asset": asset, "asset_type": atype, "site_id": siteID, "site": site}
		if label != "" {
			it["label"] = label
		}
		if interval > 0 {
			it["interval_s"] = interval
		}
		if devID != "" {
			it["device_id"], it["device"] = devID, dev
		}
		items = append(items, it)
		ids = append(ids, id)
	}
	return items, ids, total, rows.Err()
}

var sevName = map[int]string{1: "warning", 2: "action", 3: "critical"}

// enrichDS adds the latest value, the active alarm, the alarm-rule limits and (with a window) min/max/change.
func (s *Server) enrichDS(ctx context.Context, org string, items []map[string]any, ids []string, latest bool, win *dsWindow) error {
	if len(ids) == 0 {
		return nil
	}
	byID := make(map[string]map[string]any, len(items))
	for _, it := range items {
		byID[it["id"].(string)] = it
	}
	if latest {
		rows, err := s.pool.Query(ctx, `SELECT x.id::text, lv.measured_at, lv.value::float8, lv.value_json, lv.quality FROM unnest($1::uuid[]) AS x(id)
			JOIN LATERAL (SELECT o.measured_at, o.value, o.value_json, o.quality FROM core.observation o
				WHERE o.datastream_id = x.id AND o.org_id = $2::uuid AND o.measured_at > now() - interval '400 days' ORDER BY o.measured_at DESC LIMIT 1) lv ON true`, ids, org)
		if err != nil {
			return err
		}
		now := time.Now()
		for rows.Next() {
			var id, quality string
			var at time.Time
			var val *float64
			var vj json.RawMessage
			if rows.Scan(&id, &at, &val, &vj, &quality) == nil {
				l := map[string]any{"measured_at": at, "value": val}
				if len(vj) > 0 && string(vj) != "null" {
					l["value_json"] = vj
				}
				iv, _ := byID[id]["interval_s"].(int64)
				applyLive(l, quality, at, float64(iv), now) // "stale" is computed, never stored (quality.go)
				byID[id]["latest"] = l
			}
		}
		rows.Close()
		rows, err = s.pool.Query(ctx, `SELECT datastream_id::text, max(CASE severity WHEN 'critical' THEN 3 WHEN 'action' THEN 2 ELSE 1 END), count(*), bool_and(state = 'acknowledged')
			FROM core.alarm WHERE org_id = $1::uuid AND datastream_id = ANY($2::uuid[]) AND cleared_at IS NULL AND state <> 'cleared' GROUP BY datastream_id`, org, ids)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			var sev, n int
			var acked bool
			if rows.Scan(&id, &sev, &n, &acked) == nil {
				byID[id]["alarm"] = map[string]any{"severity": sevName[sev], "count": n, "acknowledged": acked}
			}
		}
		rows.Close()
		rows, err = s.pool.Query(ctx, `SELECT datastream_id::text, type, warning::float8, action::float8 FROM core.alarm_rule
			WHERE org_id = $1::uuid AND datastream_id = ANY($2::uuid[]) AND valid_to IS NULL AND enabled AND type IN ('high', 'low')`, org, ids)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id, typ string
			var warn, act *float64
			if rows.Scan(&id, &typ, &warn, &act) == nil {
				lim, _ := byID[id]["limits"].(map[string]any)
				if lim == nil {
					lim = map[string]any{}
					byID[id]["limits"] = lim
				}
				if warn != nil {
					lim[typ+"_warn"] = *warn
				}
				if act != nil {
					lim[typ+"_action"] = *act
				}
			}
		}
		rows.Close()
	}
	if win != nil {
		rows, err := s.pool.Query(ctx, `SELECT x.id::text, f.value::float8, l.value::float8, m.mn, m.mx, m.n FROM unnest($1::uuid[]) AS x(id)
			LEFT JOIN LATERAL (SELECT o.value FROM core.observation o WHERE o.datastream_id = x.id AND o.org_id = $2::uuid AND o.measured_at >= $3 AND o.measured_at < $4 AND o.value IS NOT NULL ORDER BY o.measured_at LIMIT 1) f ON true
			LEFT JOIN LATERAL (SELECT o.value FROM core.observation o WHERE o.datastream_id = x.id AND o.org_id = $2::uuid AND o.measured_at >= $3 AND o.measured_at < $4 AND o.value IS NOT NULL ORDER BY o.measured_at DESC LIMIT 1) l ON true
			LEFT JOIN LATERAL (SELECT min(o.value)::float8 mn, max(o.value)::float8 mx, count(*) n FROM core.observation o WHERE o.datastream_id = x.id AND o.org_id = $2::uuid AND o.measured_at >= $3 AND o.measured_at < $4 AND o.value IS NOT NULL) m ON true`,
			ids, org, win.From, win.To)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			var first, last, mn, mx *float64
			var n int64
			if rows.Scan(&id, &first, &last, &mn, &mx, &n) == nil {
				wv := map[string]any{"n": n, "min": mn, "max": mx, "first": first, "last": last, "change": nil}
				if first != nil && last != nil {
					wv["change"] = *last - *first
				}
				byID[id]["window"] = wv
			}
		}
		rows.Close()
	}
	return nil
}

func queryLimit(v url.Values, def, max int) (int, error) {
	if v.Get("limit") == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v.Get("limit"))
	if err != nil || n < 1 || n > max {
		return 0, fmt.Errorf("limit must be 1..%d", max)
	}
	return n, nil
}

func (s *Server) answerQuery(w http.ResponseWriter, r *http.Request, org string, sels []dsSel, limit int) {
	v := r.URL.Query()
	win, err := parseWindow(v)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	items, ids, total, err := s.queryDS(r.Context(), org, sels, limit)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	if err := s.enrichDS(r.Context(), org, items, ids, v.Get("latest") == "1", win); err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "truncated": total > len(items), "limit": limit})
}

// GET /api/v1/datastreams/query
func (s *Server) datastreamQuery(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	v := r.URL.Query()
	src, err := parseSel(v, "")
	if err == nil {
		var flt dsSel
		if flt, err = parseSel(v, "f_"); err == nil {
			limit, lerr := queryLimit(v, 100, maxSourceRows)
			if lerr != nil {
				writeErr(w, 400, lerr.Error())
				return
			}
			s.answerQuery(w, r, org, []dsSel{src, flt}, limit)
			return
		}
	}
	writeErr(w, 400, err.Error())
}

// ---- facets: the values offered by the dashboard filters ---------------------------------------------------------

type facetDef struct{ id, name, extra string }

func facetOf(kind string, a *sqlArgs) (facetDef, bool) {
	switch kind {
	case "site":
		return facetDef{"st.id::text", "st.name", ""}, true
	case "asset_type":
		return facetDef{"a.type", "a.type", ""}, true
	case "quantity":
		return facetDef{"ds.quantity", "ds.quantity", ""}, true
	case "device":
		return facetDef{"dv.id::text", "dv.name", " AND dv.id IS NOT NULL"}, true
	}
	if k, ok := strings.CutPrefix(kind, "attr:"); ok && attrKeyRe.MatchString(k) {
		kk := a.add(k)
		e := "COALESCE(ds.attributes->>" + kk + ", a.attributes->>" + kk + ")"
		return facetDef{e, e, " AND " + e + " IS NOT NULL"}, true
	}
	return facetDef{}, false
}

func (s *Server) facets(ctx context.Context, org string, kinds []string, sels []dsSel) (map[string]any, error) {
	out := map[string]any{}
	for _, kind := range kinds {
		if kind == "attr_keys" {
			rows, err := s.pool.Query(ctx, `SELECT k FROM (SELECT jsonb_object_keys(a.attributes) k FROM core.asset a WHERE a.org_id = $1::uuid AND jsonb_typeof(a.attributes) = 'object'
				UNION SELECT jsonb_object_keys(ds.attributes) FROM core.datastream ds WHERE ds.org_id = $1::uuid AND jsonb_typeof(ds.attributes) = 'object') x ORDER BY k LIMIT 200`, org)
			if err != nil {
				return nil, err
			}
			keys := []string{}
			for rows.Next() {
				var k string
				if rows.Scan(&k) == nil && attrKeyRe.MatchString(k) {
					keys = append(keys, k)
				}
			}
			rows.Close()
			out[kind] = keys
			continue
		}
		a := &sqlArgs{}
		fd, ok := facetOf(kind, a)
		if !ok {
			return nil, fmt.Errorf("kinds: unknown %q (site, asset_type, quantity, device, attr:<key>, attr_keys)", kind)
		}
		q := `SELECT ` + fd.id + `, ` + fd.name + `, count(*)` + dsFrom + whereOf(org, sels, a, kind) + fd.extra + ` GROUP BY 1, 2 ORDER BY 2 LIMIT 500`
		rows, err := s.pool.Query(ctx, q, a.args...)
		if err != nil {
			return nil, err
		}
		list := []map[string]any{}
		for rows.Next() {
			var id, name string
			var n int
			if rows.Scan(&id, &name, &n) == nil {
				list = append(list, map[string]any{"value": id, "label": name, "n": n})
			}
		}
		rows.Close()
		out[kind] = list
	}
	return out, nil
}

func facetKinds(v url.Values) ([]string, error) {
	kinds := splitList(v.Get("kinds"))
	if len(kinds) == 0 {
		kinds = []string{"site", "asset_type", "quantity", "device"}
	}
	if len(kinds) > 12 {
		return nil, errors.New("kinds: at most 12")
	}
	return kinds, nil
}

// GET /api/v1/datastreams/facets?kinds=site,asset_type,quantity,device,attr:<key>,attr_keys&f_site=…
func (s *Server) datastreamFacets(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	v := r.URL.Query()
	kinds, err := facetKinds(v)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	src, err := parseSel(v, "")
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	flt, err := parseSel(v, "f_")
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	// a facet narrows the filters (f_), never the scope (plain parameters): skip applies to both, ids stay
	out, err := s.facets(r.Context(), org, kinds, []dsSel{src, flt})
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

// ---- public links: within the dashboard's scope, only the filters the owner allows -------------------------------

// publicFilterSel builds the filter constraints of a public request: a filter marked public takes the viewer's value
// (f_… parameters), any other filter its default. Parameters of filters the dashboard does not have are ignored.
func publicFilterSel(spec map[string]any, v url.Values) (dsSel, error) {
	in := url.Values{}
	for _, f := range specFilters(spec) {
		kind, _ := f["kind"].(string)
		key, _ := f["key"].(string)
		pub := f["public"] == true
		vals := anyStrings(f["default"])
		switch kind {
		case "site", "asset_type", "quantity", "device":
			if pub && v.Has("f_"+kind) {
				vals = splitList(v.Get("f_" + kind))
			}
			if len(vals) > 0 {
				in.Set("f_"+kind, strings.Join(vals, ","))
			}
		case "search":
			if pub && v.Has("f_q") {
				vals = []string{v.Get("f_q")}
			}
			if len(vals) > 0 {
				in.Set("f_q", vals[0])
			}
		case "attribute":
			if pub {
				var mine []string
				given := false
				for _, kv := range v["f_attr"] {
					if k, val, ok := strings.Cut(kv, "="); ok && k == key {
						given = true
						mine = append(mine, val)
					}
				}
				if given {
					vals = mine
				}
			}
			for _, x := range vals {
				in.Add("f_attr", key+"="+x)
			}
		}
	}
	return parseSel(in, "f_")
}

func (si *shareInfo) scope() []string {
	out := make([]string, 0, len(si.DS))
	for id := range si.DS {
		out = append(out, id)
	}
	return out
}

func (si *shareInfo) widget(id string) map[string]any {
	ws, _ := si.Spec["widgets"].([]any)
	for _, x := range ws {
		if w, ok := x.(map[string]any); ok && w["id"] == id {
			return w
		}
	}
	return nil
}

// GET /api/v1/public/{token}/datastreams/query?widget=<id>&f_…
func (s *Server) publicDatastreamQuery(w http.ResponseWriter, r *http.Request) {
	si, ok := s.publicShare(w, r)
	if !ok {
		return
	}
	v := r.URL.Query()
	wd := si.widget(v.Get("widget"))
	if wd == nil {
		writeErr(w, 404, "widget not found")
		return
	}
	scope := si.scope()
	if len(scope) == 0 {
		writeJSON(w, 200, map[string]any{"items": []any{}, "total": 0, "truncated": false, "limit": 0})
		return
	}
	typ, _ := wd["type"].(string)
	sels := []dsSel{{IDs: scope}}
	limit := maxSourceRows
	if src, ok := wd["source"].(map[string]any); ok {
		sels = append(sels, selFromSource(src))
		limit = sourceLimitOf(src, typ)
	} else if b, _ := wd["bindings"].(map[string]any); len(anyStrings(b["datastreams"])) > 0 {
		sels = append(sels, dsSel{IDs: anyStrings(b["datastreams"])})
	}
	if wd["follow"] == true {
		flt, err := publicFilterSel(si.Spec, v)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		sels = append(sels, flt)
	}
	if n, err := queryLimit(v, limit, limit); err == nil {
		limit = n
	}
	s.answerQuery(w, r, si.Org, sels, limit)
}

// GET /api/v1/public/{token}/datastreams/facets?kinds=… — only for the filters a viewer of the link may change
func (s *Server) publicFacets(w http.ResponseWriter, r *http.Request) {
	si, ok := s.publicShare(w, r)
	if !ok {
		return
	}
	v := r.URL.Query()
	allowed := map[string]bool{}
	for _, f := range specFilters(si.Spec) {
		if f["public"] != true {
			continue
		}
		kind, _ := f["kind"].(string)
		if kind == "attribute" {
			key, _ := f["key"].(string)
			kind = "attr:" + key
		}
		allowed[kind] = true
	}
	kinds, err := facetKinds(v)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var keep []string
	for _, k := range kinds {
		if allowed[k] {
			keep = append(keep, k)
		}
	}
	scope := si.scope()
	if len(scope) == 0 || len(keep) == 0 {
		writeJSON(w, 200, map[string]any{})
		return
	}
	flt, err := publicFilterSel(si.Spec, v)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	out, err := s.facets(r.Context(), si.Org, keep, []dsSel{{IDs: scope}, flt})
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

// resolveSources adds the datastreams of widgets with a query source to what a public link (or a display) may read:
// everything the source selects without any filter — filters only narrow it.
func (s *Server) resolveSources(ctx context.Context, si *shareInfo) {
	ws, _ := si.Spec["widgets"].([]any)
	for _, x := range ws {
		wd, _ := x.(map[string]any)
		src, ok := wd["source"].(map[string]any)
		if !ok {
			continue
		}
		typ, _ := wd["type"].(string)
		_, ids, _, err := s.queryDS(ctx, si.Org, []dsSel{selFromSource(src)}, sourceLimitOf(src, typ))
		if err != nil {
			continue
		}
		for _, id := range ids {
			si.DS[id] = true
		}
	}
}

// ---- export of an overview table -----------------------------------------------------------------------------------

var exportCols = map[string]string{"site": "Site", "asset": "Asset", "asset_type": "Asset type", "device": "Device", "datastream": "Datastream", "quantity": "Quantity",
	"value": "Value", "measured": "Measured", "age": "Age", "quality": "Quality", "alarm": "Alarm", "limits": "Limits", "min": "Minimum", "max": "Maximum", "change": "Change"}

func ageText(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d s", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h", int(d.Hours()))
	}
	return fmt.Sprintf("%d d", int(d.Hours()/24))
}

func limitsText(lim map[string]any) string {
	f := func(k string) string {
		if v, ok := lim[k].(float64); ok {
			return strconv.FormatFloat(v, 'f', -1, 64)
		}
		return ""
	}
	var parts []string
	if lo := f("low_action"); lo != "" {
		parts = append(parts, "≥ "+lo)
	} else if lo := f("low_warn"); lo != "" {
		parts = append(parts, "≥ "+lo)
	}
	if hi := f("high_action"); hi != "" {
		parts = append(parts, "≤ "+hi)
	} else if hi := f("high_warn"); hi != "" {
		parts = append(parts, "≤ "+hi)
	}
	return strings.Join(parts, " ")
}

// POST /api/v1/datastreams/export {ids, columns, format: xlsx|csv, title, from?, to?, decimals?}
func (s *Server) datastreamExport(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	var in struct {
		IDs      []string `json:"ids"`
		Columns  []string `json:"columns"`
		Format   string   `json:"format"`
		Title    string   `json:"title"`
		From     string   `json:"from"`
		To       string   `json:"to"`
		Decimals *int     `json:"decimals"`
	}
	if err := decodeJSON(r, &in, 256<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.Format == "" {
		in.Format = "xlsx"
	}
	if in.Format != "xlsx" && in.Format != "csv" {
		writeErr(w, 400, "format: xlsx or csv")
		return
	}
	if len(in.IDs) == 0 || len(in.IDs) > 2000 {
		writeErr(w, 400, "ids: 1..2000 datastreams")
		return
	}
	for _, id := range in.IDs {
		if !uuidRe.MatchString(id) {
			writeErr(w, 400, "ids must be datastream ids")
			return
		}
	}
	if len(in.Columns) == 0 {
		in.Columns = []string{"site", "asset", "datastream", "value", "measured", "quality", "alarm"}
	}
	for _, c := range in.Columns {
		if exportCols[c] == "" {
			writeErr(w, 400, "unknown column "+c)
			return
		}
	}
	if len([]rune(in.Title)) > 120 {
		writeErr(w, 400, "title: at most 120 characters")
		return
	}
	dec := 2
	if in.Decimals != nil && *in.Decimals >= 0 && *in.Decimals <= 6 {
		dec = *in.Decimals
	}
	wv := url.Values{}
	if in.From != "" {
		wv.Set("from", in.From)
		wv.Set("to", in.To)
	}
	win, err := parseWindow(wv)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	// the ids are read in chunks of the query limit, then put back into the order of the table
	byID := map[string]map[string]any{}
	for i := 0; i < len(in.IDs); i += maxSourceRows {
		chunk := in.IDs[i:min(i+maxSourceRows, len(in.IDs))]
		items, ids, _, err := s.queryDS(r.Context(), org, []dsSel{{IDs: chunk}}, len(chunk))
		if err == nil {
			err = s.enrichDS(r.Context(), org, items, ids, true, win)
		}
		if err != nil {
			writeErr(w, 500, "query failed")
			return
		}
		for _, it := range items {
			byID[it["id"].(string)] = it
		}
	}
	lang := s.exportLanguage(r, p)
	cat := i18n.Builtin(lang)
	tr := func(k string) string {
		if v := cat[k]; v != "" {
			return v
		}
		return k
	}
	loc := s.orgLoc(r.Context(), org)
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = tr("Overview")
	}
	var header []string
	for _, c := range in.Columns {
		header = append(header, tr(exportCols[c]))
		if c == "value" {
			header = append(header, tr("Unit"))
		}
	}
	numCell := func(v any) xlsx.Cell {
		if f, ok := v.(*float64); ok && f != nil && !math.IsNaN(*f) {
			return xlsx.N(*f, dec)
		}
		if f, ok := v.(float64); ok && !math.IsNaN(f) {
			return xlsx.N(f, dec)
		}
		return xlsx.T("")
	}
	var rows [][]xlsx.Cell
	now := time.Now()
	for _, id := range in.IDs {
		it := byID[id]
		if it == nil {
			continue // not in the organization, or removed
		}
		lv, _ := it["latest"].(map[string]any)
		wnd, _ := it["window"].(map[string]any)
		var row []xlsx.Cell
		for _, c := range in.Columns {
			switch c {
			case "site", "asset", "asset_type", "device", "quantity":
				sv, _ := it[c].(string)
				row = append(row, xlsx.T(sv))
			case "datastream":
				lbl, _ := it["label"].(string)
				if lbl == "" {
					lbl, _ = it["key"].(string)
				}
				row = append(row, xlsx.T(lbl))
			case "value":
				if lv != nil {
					row = append(row, numCell(lv["value"]))
				} else {
					row = append(row, xlsx.T(""))
				}
				u, _ := it["unit"].(string)
				row = append(row, xlsx.T(u))
			case "measured", "age":
				at, ok := lv["measured_at"].(time.Time)
				switch {
				case !ok:
					row = append(row, xlsx.T(""))
				case c == "measured":
					row = append(row, xlsx.T(at.In(loc).Format("2006-01-02 15:04:05")))
				default:
					row = append(row, xlsx.T(ageText(now.Sub(at))))
				}
			case "quality":
				q, _ := lv["quality"].(string)
				row = append(row, xlsx.T(q))
			case "alarm":
				al, _ := it["alarm"].(map[string]any)
				sv, _ := al["severity"].(string)
				row = append(row, xlsx.T(sv))
			case "limits":
				lim, _ := it["limits"].(map[string]any)
				row = append(row, xlsx.T(limitsText(lim)))
			case "min", "max", "change":
				if wnd != nil {
					row = append(row, numCell(wnd[c]))
				} else {
					row = append(row, xlsx.T(""))
				}
			}
		}
		rows = append(rows, row)
	}
	stamp := now.In(loc).Format("20060102-1504")
	base := fileSafe(title)
	switch in.Format {
	case "csv":
		var b bytes.Buffer
		b.WriteString("\ufeff")
		cw := csv.NewWriter(&b)
		_ = cw.Write(header)
		for _, row := range rows {
			rec := make([]string, len(row))
			for i, c := range row {
				if c.IsNum {
					rec[i] = strconv.FormatFloat(c.Num, 'f', c.Decimals, 64)
				} else {
					rec[i] = c.Text
				}
			}
			_ = cw.Write(rec)
		}
		cw.Flush()
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s_%s.csv"`, base, stamp))
		_, _ = w.Write(b.Bytes())
	default:
		sheet := xlsx.Sheet{Name: title, FreezeRows: 4}
		sub := fmt.Sprintf("%s · %s %s", s.orgName(r, org), now.In(loc).Format("2006-01-02 15:04"), p.Username)
		if win != nil {
			sub += " · " + win.From.In(loc).Format("2006-01-02 15:04") + " – " + win.To.In(loc).Format("2006-01-02 15:04")
		}
		sheet.Rows = append(sheet.Rows, []xlsx.Cell{xlsx.B(title)}, []xlsx.Cell{xlsx.T(sub)}, nil)
		hdr := make([]xlsx.Cell, len(header))
		for i, h := range header {
			hdr[i] = xlsx.B(h)
			sheet.Widths = append(sheet.Widths, max(10, min(float64(len([]rune(h)))+4, 36)))
		}
		sheet.Rows = append(sheet.Rows, hdr)
		sheet.Rows = append(sheet.Rows, rows...)
		b, err := xlsx.Bytes(sheet)
		if err != nil {
			writeErr(w, 500, "xlsx failed")
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s_%s.xlsx"`, base, stamp))
		_, _ = w.Write(b)
	}
	s.audit(r, "export.read", "export", "", nil, map[string]any{"export": "overview_table", "format": in.Format, "rows": len(rows), "columns": in.Columns}, "")
}

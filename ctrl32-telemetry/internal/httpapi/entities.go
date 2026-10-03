package httpapi

// Entities of dashboard aliases (0.73.0): the server resolves an alias (dashboard_aliases.go) to devices, assets,
// datastreams or sites of the caller's organization, applies a key filter, and answers with the entities, their
// datastreams or their devices — whatever the widget shows.
//
//	GET /api/v1/entities/query                   (data.read) alias and key filter given as JSON in the parameters
//	GET /api/v1/public/{token}/entities/query    a widget of a public dashboard: alias, filter and columns come from
//	                                             the stored spec, never from the request, and every answer stays
//	                                             within the dashboard's published scope
//	GET/PUT /api/v1/organization/attributes      attributes of the organization (dynamic values of key filters)
//
// Limits: an alias yields at most 2000 candidates (sorted by name), an answer at most 500 rows; a hierarchy goes at
// most 10 levels deep. The dashboard filters (widget.follow) narrow entities to those with a datastream that matches
// them; they never widen anything.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxCandidates = 2000

type entRef struct{ Type, ID string }

func (r *entRef) key() string { return r.Type + ":" + r.ID }

// parseRef reads "type:id" (the current entity of a dashboard state).
func parseRef(s string) (*entRef, error) {
	if s == "" {
		return nil, nil
	}
	t, id, ok := strings.Cut(s, ":")
	if !ok || !entityTypes[t] || !uuidRe.MatchString(id) {
		return nil, errors.New("current must be type:id (device, asset, datastream or site)")
	}
	return &entRef{t, id}, nil
}

func refOf(v any) *entRef {
	m, _ := v.(map[string]any)
	t, _ := m["type"].(string)
	id, _ := m["id"].(string)
	if !entityTypes[t] || !uuidRe.MatchString(id) {
		return nil
	}
	return &entRef{t, id}
}

// latestVal is the latest value of a datastream as key filters see it.
type latestVal struct {
	Value   *float64
	JSON    json.RawMessage
	At      time.Time
	Quality string
	Stale   bool
}

// entity is one resolved entity with what key filters and entity tables need.
type entity struct {
	Type, ID, Name, Label, ExtID, Profile, Transport  string
	SiteID, Site, AssetID, Asset, AssetType, ParentID string
	DeviceID, Device, Key, Quantity, Unit, Kind       string
	IntervalS                                         int64
	LastSeen                                          *string
	Attrs, AssetAttrs, SiteAttrs                      map[string]any
	DS                                                []string          // datastreams of the entity (device: assigned; asset, site: its own; datastream: itself)
	ByKey                                             map[string]string // datastream key → id
	Units                                             map[string]string // datastream id → unit
}

func (e *entity) ref() entRef { return entRef{e.Type, e.ID} }

// attr is an attribute of the entity, else of its asset, else of its site.
func (e *entity) attr(k string) (any, bool) {
	if e == nil {
		return nil, false
	}
	for _, m := range []map[string]any{e.Attrs, e.AssetAttrs, e.SiteAttrs} {
		if v, ok := m[k]; ok && v != nil {
			return v, true
		}
	}
	return nil, false
}

func jsonMap(b []byte) map[string]any {
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	return m
}

// inListCond is inList where no values, or the value *, match everything.
func inListCond(col string, vals []string, a *sqlArgs) string {
	for _, v := range vals {
		if v == "*" {
			return "true"
		}
	}
	if len(vals) == 0 {
		return "true"
	}
	return inList(col, vals, a)
}

// ---- candidates of an alias ----------------------------------------------------------------------------------------

// aliasCandidates returns the entity type and ids an alias selects (at most maxCandidates, in display order) and
// whether there were more.
func (s *Server) aliasCandidates(ctx context.Context, org string, al map[string]any, cur *entRef) (string, []string, bool, error) {
	kind, _ := al["kind"].(string)
	et, _ := al["entity_type"].(string)
	switch kind {
	case "single":
		r := refOf(al["entity"])
		if r == nil {
			return "", nil, false, nil
		}
		return r.Type, []string{r.ID}, false, nil
	case "list":
		return et, anyStrings(al["ids"]), false, nil
	case "type":
		a := &sqlArgs{}
		o := a.add(org)
		var q string
		types := anyStrings(al["types"])
		switch et {
		case "device":
			q = `SELECT d.id::text FROM core.device d LEFT JOIN core.device_profile p ON p.id = d.profile_id WHERE d.org_id = ` + o + `::uuid AND d.revoked_at IS NULL AND ` + inListCond("COALESCE(p.name, '')", types, a) + ` ORDER BY d.name`
		case "asset":
			q = `SELECT a.id::text FROM core.asset a JOIN core.site st ON st.id = a.site_id WHERE a.org_id = ` + o + `::uuid AND ` + inListCond("a.type", types, a) + ` ORDER BY st.name, a.name`
		default:
			q = `SELECT ds.id::text` + dsFrom + ` WHERE ds.org_id = ` + o + `::uuid AND ` + inListCond("ds.quantity", types, a) + ` ORDER BY st.name, a.name, ds.key`
		}
		ids, more, err := s.idList(ctx, q+` LIMIT `+strconv.Itoa(maxCandidates+1), a.args)
		return et, ids, more, err
	case "name":
		txt, _ := al["text"].(string)
		p := likeRepl.Replace(txt)
		switch al["match"] {
		case "starts":
			p += "%"
		case "ends":
			p = "%" + p
		default:
			p = "%" + p + "%"
		}
		a := &sqlArgs{}
		o, n := a.add(org), a.add(p)
		var q string
		switch et {
		case "device":
			q = `SELECT d.id::text FROM core.device d WHERE d.org_id = ` + o + `::uuid AND d.revoked_at IS NULL AND (d.name ILIKE ` + n + ` OR d.external_id ILIKE ` + n + `) ORDER BY d.name`
		case "asset":
			q = `SELECT a.id::text FROM core.asset a JOIN core.site st ON st.id = a.site_id WHERE a.org_id = ` + o + `::uuid AND a.name ILIKE ` + n + ` ORDER BY st.name, a.name`
		case "site":
			q = `SELECT st.id::text FROM core.site st WHERE st.org_id = ` + o + `::uuid AND st.name ILIKE ` + n + ` ORDER BY st.name`
		default:
			q = `SELECT ds.id::text` + dsFrom + ` WHERE ds.org_id = ` + o + `::uuid AND (ds.key ILIKE ` + n + ` OR COALESCE(ds.attributes->>'name', '') ILIKE ` + n + ` OR (a.name || ' / ' || ds.key) ILIKE ` + n + `) ORDER BY st.name, a.name, ds.key`
		}
		ids, more, err := s.idList(ctx, q+` LIMIT `+strconv.Itoa(maxCandidates+1), a.args)
		return et, ids, more, err
	case "hierarchy":
		root := refOf(al["root"])
		if al["root_current"] == true {
			root = cur
		}
		if root == nil {
			return et, nil, false, nil
		}
		depth := maxTreeDepth
		if f, ok := numAny(al["depth"]); ok && f >= 1 && f <= maxTreeDepth {
			depth = int(f)
		}
		if root.Type == "device" || root.Type == "datastream" {
			// the current entity of a detail state may be a device or a datastream: what is "under" it is what it
			// carries (its datastreams, its device, its assets)
			ids, err := s.convert(ctx, org, root.Type, []string{root.ID}, et)
			return et, ids, false, err
		}
		return s.treeCandidates(ctx, org, *root, depth, et, anyStrings(al["asset_type"]))
	case "current":
		r := cur
		if r == nil {
			r = refOf(al["default"])
		}
		if r == nil {
			return et, nil, false, nil
		}
		if et == "" || et == r.Type {
			return r.Type, []string{r.ID}, false, nil
		}
		ids, err := s.convert(ctx, org, r.Type, []string{r.ID}, et)
		return et, ids, false, err
	case "query":
		src, _ := al["source"].(map[string]any)
		_, ids, total, err := s.queryDS(ctx, org, []dsSel{selFromSource(src)}, maxCandidates)
		return "datastream", ids, total > len(ids), err
	}
	return "", nil, false, fmt.Errorf("unknown alias kind %q", kind)
}

func (s *Server) idList(ctx context.Context, q string, args []any) ([]string, bool, error) {
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	ids := []string{}
	seen := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, false, err
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	more := len(ids) > maxCandidates
	if more {
		ids = ids[:maxCandidates]
	}
	return ids, more, rows.Err()
}

// treeCandidates: the assets under an asset or a site (depth levels; a site's top-level assets are level 1), then
// their datastreams or devices, or the assets themselves (without the root), optionally only assets of some types.
func (s *Server) treeCandidates(ctx context.Context, org string, root entRef, depth int, et string, types []string) (string, []string, bool, error) {
	a := &sqlArgs{}
	o, r, d := a.add(org), a.add(root.ID), a.add(depth)
	start := `SELECT a.id, 0 FROM core.asset a WHERE a.org_id = ` + o + `::uuid AND a.id = ` + r + `::uuid`
	if root.Type == "site" {
		start = `SELECT a.id, 1 FROM core.asset a LEFT JOIN core.asset p ON p.id = a.parent_id WHERE a.org_id = ` + o + `::uuid AND a.site_id = ` + r + `::uuid AND (p.id IS NULL OR p.site_id <> a.site_id)`
	}
	tree := `WITH RECURSIVE t(id, depth) AS (` + start + ` UNION SELECT c.id, t.depth + 1 FROM core.asset c JOIN t ON c.parent_id = t.id WHERE c.org_id = ` + o + `::uuid AND t.depth < ` + d + `)`
	// the assets of the tree, optionally of some types; the root asset itself only carries datastreams and devices
	sel := `SELECT DISTINCT t.id FROM t JOIN core.asset x ON x.id = t.id WHERE ` + inListCond("x.type", types, a)
	var q string
	switch et {
	case "asset":
		q = tree + ` SELECT a.id::text FROM core.asset a JOIN core.site st ON st.id = a.site_id WHERE a.id IN (` + sel + `) AND a.id <> ` + r + `::uuid ORDER BY st.name, a.name`
	case "device":
		q = tree + ` SELECT d.id::text FROM core.device d WHERE d.org_id = ` + o + `::uuid AND d.revoked_at IS NULL AND d.id IN (SELECT x.from_id FROM core.assignment x JOIN core.datastream ds ON ds.id = x.to_id
			WHERE x.kind = 'device_datastream' AND x.valid_to IS NULL AND ds.asset_id IN (` + sel + `)) ORDER BY d.name`
	default:
		q = tree + ` SELECT ds.id::text` + dsFrom + ` WHERE ds.org_id = ` + o + `::uuid AND ds.asset_id IN (` + sel + `) ORDER BY st.name, a.name, ds.key`
	}
	ids, more, err := s.idList(ctx, q+` LIMIT `+strconv.Itoa(maxCandidates+1), a.args)
	return et, ids, more, err
}

// dsOwners is SQL (owner text, ds uuid) for the datastreams of entities of a type; $1 org, $2 the entity ids.
func dsOwners(typ string) string {
	switch typ {
	case "device":
		return `SELECT x.from_id::text o, x.to_id ds FROM core.assignment x JOIN core.datastream d0 ON d0.id = x.to_id WHERE x.kind = 'device_datastream' AND x.valid_to IS NULL AND x.from_id = ANY($2::uuid[]) AND d0.org_id = $1::uuid`
	case "asset":
		return `SELECT d0.asset_id::text o, d0.id ds FROM core.datastream d0 WHERE d0.asset_id = ANY($2::uuid[]) AND d0.org_id = $1::uuid`
	case "site":
		return `SELECT a0.site_id::text o, d0.id ds FROM core.datastream d0 JOIN core.asset a0 ON a0.id = d0.asset_id WHERE a0.site_id = ANY($2::uuid[]) AND d0.org_id = $1::uuid`
	}
	return `SELECT d0.id::text o, d0.id ds FROM core.datastream d0 WHERE d0.id = ANY($2::uuid[]) AND d0.org_id = $1::uuid`
}

// convert turns entities of one type into the related entities of another (in display order).
func (s *Server) convert(ctx context.Context, org, from string, ids []string, to string) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}
	if from == to {
		return ids, nil
	}
	var q string
	switch {
	case from == "site" && to == "asset":
		q = `SELECT a.id::text FROM core.asset a JOIN core.site st ON st.id = a.site_id WHERE a.org_id = $1::uuid AND a.site_id = ANY($2::uuid[]) ORDER BY st.name, a.name`
	case to == "datastream":
		q = `SELECT ds.id::text` + dsFrom + ` WHERE ds.org_id = $1::uuid AND ds.id IN (SELECT ds FROM (` + dsOwners(from) + `) q) ORDER BY st.name, a.name, ds.key`
	case to == "device":
		q = `SELECT d.id::text FROM core.device d WHERE d.org_id = $1::uuid AND d.revoked_at IS NULL AND d.id IN (SELECT x.from_id FROM core.assignment x
			WHERE x.kind = 'device_datastream' AND x.valid_to IS NULL AND x.to_id IN (SELECT ds FROM (` + dsOwners(from) + `) q)) ORDER BY d.name`
	case to == "asset":
		q = `SELECT a.id::text FROM core.asset a JOIN core.site st ON st.id = a.site_id WHERE a.org_id = $1::uuid AND a.id IN (SELECT d1.asset_id FROM core.datastream d1 WHERE d1.id IN (SELECT ds FROM (` + dsOwners(from) + `) q)) ORDER BY st.name, a.name`
	case to == "site":
		q = `SELECT st.id::text FROM core.site st WHERE st.org_id = $1::uuid AND st.id IN (SELECT a1.site_id FROM core.asset a1 JOIN core.datastream d1 ON d1.asset_id = a1.id WHERE d1.id IN (SELECT ds FROM (` + dsOwners(from) + `) q)
			UNION SELECT a2.site_id FROM core.asset a2 WHERE $3 = 'asset' AND a2.id = ANY($2::uuid[])) ORDER BY st.name`
		out, _, err := s.idList(ctx, q+` LIMIT `+strconv.Itoa(maxCandidates), []any{org, ids, from})
		return out, err
	default:
		return nil, fmt.Errorf("cannot convert %s to %s", from, to)
	}
	out, _, err := s.idList(ctx, q+` LIMIT `+strconv.Itoa(maxCandidates), []any{org, ids})
	return out, err
}

// ---- loading entities ----------------------------------------------------------------------------------------------

// loadEntities reads the entities of a type within the organization, in display order; ids of other organizations
// or removed entities are left out.
func (s *Server) loadEntities(ctx context.Context, org, typ string, ids []string) ([]*entity, error) {
	out := []*entity{}
	if len(ids) == 0 {
		return out, nil
	}
	var q string
	switch typ {
	case "device":
		q = `SELECT d.id::text, d.name, d.external_id, COALESCE(p.name, ''), d.transport, d.status->>'last_seen', d.shared_attributes,
			COALESCE(x.aid, ''), COALESCE(x.aname, ''), COALESCE(x.atype, ''), COALESCE(x.aattrs, '{}'::jsonb), COALESCE(x.sid, ''), COALESCE(x.sname, ''), COALESCE(x.sattrs, '{}'::jsonb)
			FROM core.device d LEFT JOIN core.device_profile p ON p.id = d.profile_id
			LEFT JOIN LATERAL (SELECT a.id::text aid, a.name aname, a.type atype, a.attributes aattrs, st.id::text sid, st.name sname, st.attributes sattrs
				FROM core.assignment x JOIN core.datastream ds ON ds.id = x.to_id JOIN core.asset a ON a.id = ds.asset_id JOIN core.site st ON st.id = a.site_id
				WHERE x.kind = 'device_datastream' AND x.from_id = d.id AND x.valid_to IS NULL ORDER BY a.name LIMIT 1) x ON true
			WHERE d.org_id = $1::uuid AND d.id = ANY($2::uuid[]) AND d.revoked_at IS NULL ORDER BY d.name`
	case "asset":
		// an asset answers with its first device too (drill-down "with its device")
		q = `SELECT a.id::text, a.name, a.type, COALESCE(a.parent_id::text, ''), a.attributes, st.id::text, st.name, st.attributes, COALESCE(dv.did, ''), COALESCE(dv.dname, '')
			FROM core.asset a JOIN core.site st ON st.id = a.site_id
			LEFT JOIN LATERAL (SELECT d.id::text did, d.name dname FROM core.datastream ds JOIN core.assignment x ON x.to_id = ds.id AND x.kind = 'device_datastream' AND x.valid_to IS NULL
				JOIN core.device d ON d.id = x.from_id WHERE ds.asset_id = a.id AND d.revoked_at IS NULL ORDER BY d.name LIMIT 1) dv ON true WHERE a.org_id = $1::uuid AND a.id = ANY($2::uuid[]) ORDER BY st.name, a.name`
	case "site":
		q = `SELECT st.id::text, st.name, st.attributes FROM core.site st WHERE st.org_id = $1::uuid AND st.id = ANY($2::uuid[]) ORDER BY st.name`
	case "datastream":
		q = `SELECT ds.id::text, ds.key, ds.quantity, ds.unit, ds.kind, COALESCE(ds.attributes->>'name', ''), ds.attributes, a.id::text, a.name, a.type, a.attributes, st.id::text, st.name, st.attributes,
			COALESCE(EXTRACT(EPOCH FROM ds.expected_interval)::bigint, 0), COALESCE(dv.id::text, ''), COALESCE(dv.name, '')` + dsFrom + `
			WHERE ds.org_id = $1::uuid AND ds.id = ANY($2::uuid[]) ORDER BY st.name, a.name, ds.key`
	default:
		return nil, fmt.Errorf("unknown entity type %q", typ)
	}
	rows, err := s.pool.Query(ctx, q, org, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		e := &entity{Type: typ}
		var err error
		switch typ {
		case "device":
			var sa, aa, sta []byte
			err = rows.Scan(&e.ID, &e.Name, &e.ExtID, &e.Profile, &e.Transport, &e.LastSeen, &sa, &e.AssetID, &e.Asset, &e.AssetType, &aa, &e.SiteID, &e.Site, &sta)
			e.Attrs, e.AssetAttrs, e.SiteAttrs = jsonMap(sa), jsonMap(aa), jsonMap(sta)
			e.DeviceID, e.Device = e.ID, e.Name
		case "asset":
			var aa, sta []byte
			err = rows.Scan(&e.ID, &e.Name, &e.AssetType, &e.ParentID, &aa, &e.SiteID, &e.Site, &sta, &e.DeviceID, &e.Device)
			e.Attrs, e.SiteAttrs = jsonMap(aa), jsonMap(sta)
			e.AssetID, e.Asset = e.ID, e.Name
		case "site":
			var sta []byte
			err = rows.Scan(&e.ID, &e.Name, &sta)
			e.Attrs = jsonMap(sta)
			e.SiteID, e.Site = e.ID, e.Name
		case "datastream":
			var da, aa, sta []byte
			err = rows.Scan(&e.ID, &e.Key, &e.Quantity, &e.Unit, &e.Kind, &e.Label, &da, &e.AssetID, &e.Asset, &e.AssetType, &aa, &e.SiteID, &e.Site, &sta, &e.IntervalS, &e.DeviceID, &e.Device)
			e.Attrs, e.AssetAttrs, e.SiteAttrs = jsonMap(da), jsonMap(aa), jsonMap(sta)
			e.Name = e.Asset + " / " + e.Key
			if e.Label != "" {
				e.Name = e.Label
			}
		}
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// loadDS fills the datastreams of the entities (all of them, and by key).
func (s *Server) loadDS(ctx context.Context, org string, ents []*entity) error {
	byType := map[string][]string{}
	idx := map[string]*entity{}
	for _, e := range ents {
		byType[e.Type] = append(byType[e.Type], e.ID)
		idx[e.Type+":"+e.ID] = e
		e.ByKey = map[string]string{}
		e.Units = map[string]string{}
		e.DS = nil
	}
	for typ, ids := range byType {
		rows, err := s.pool.Query(ctx, `SELECT q.o, q.ds::text, d.key, d.unit FROM (`+dsOwners(typ)+`) q JOIN core.datastream d ON d.id = q.ds JOIN core.asset a ON a.id = d.asset_id ORDER BY a.name, d.key LIMIT 50000`, org, ids)
		if err != nil {
			return err
		}
		for rows.Next() {
			var o, ds, key, unit string
			if rows.Scan(&o, &ds, &key, &unit) != nil {
				continue
			}
			e := idx[typ+":"+o]
			if e == nil {
				continue
			}
			e.DS = append(e.DS, ds)
			e.Units[ds] = unit
			if _, ok := e.ByKey[key]; !ok {
				e.ByKey[key] = ds
			}
		}
		rows.Close()
	}
	// a datastream answers for its own key and finds its siblings (other keys of its asset)
	var assets []string
	for _, e := range ents {
		if e.Type == "datastream" {
			assets = append(assets, e.AssetID)
		}
	}
	if len(assets) > 0 {
		rows, err := s.pool.Query(ctx, `SELECT d0.asset_id::text, d0.id::text, d0.key, d0.unit FROM core.datastream d0 WHERE d0.asset_id = ANY($2::uuid[]) AND d0.org_id = $1::uuid`, org, assets)
		if err != nil {
			return err
		}
		sib := map[string]map[string]string{}
		units := map[string]string{}
		for rows.Next() {
			var a, id, key, unit string
			if rows.Scan(&a, &id, &key, &unit) == nil {
				if sib[a] == nil {
					sib[a] = map[string]string{}
				}
				sib[a][key] = id
				units[id] = unit
			}
		}
		rows.Close()
		for _, e := range ents {
			if e.Type == "datastream" {
				for k, id := range sib[e.AssetID] {
					e.ByKey[k] = id
					e.Units[id] = units[id]
				}
				e.ByKey[e.Key] = e.ID
			}
		}
	}
	return nil
}

// latestOf reads the latest value of datastreams.
func (s *Server) latestOf(ctx context.Context, org string, ids []string) (map[string]*latestVal, error) {
	out := map[string]*latestVal{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT x.id::text, lv.measured_at, lv.value::float8, lv.value_json, lv.quality, COALESCE(EXTRACT(EPOCH FROM ds.expected_interval)::float8, 0)
		FROM unnest($1::uuid[]) AS x(id) JOIN core.datastream ds ON ds.id = x.id AND ds.org_id = $2::uuid
		JOIN LATERAL (SELECT o.measured_at, o.value, o.value_json, o.quality FROM core.observation o
			WHERE o.datastream_id = x.id AND o.org_id = $2::uuid AND o.measured_at > now() - interval '400 days' ORDER BY o.measured_at DESC LIMIT 1) lv ON true`, ids, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now()
	for rows.Next() {
		var id, q string
		var at time.Time
		var v *float64
		var vj json.RawMessage
		var iv float64
		if rows.Scan(&id, &at, &v, &vj, &q, &iv) != nil {
			continue
		}
		lv := &latestVal{Value: v, At: at, Quality: q}
		if len(vj) > 0 && string(vj) != "null" {
			lv.JSON = vj
		}
		// stale: older than 1.5 × the expected interval, or than an hour when the datastream has none
		if iv > 0 {
			lv.Stale = isStale(at, iv, now)
		} else {
			lv.Stale = now.Sub(at) > time.Hour
		}
		lq, _ := liveQuality(q, at, iv, now)
		lv.Quality = lq
		out[id] = lv
	}
	return out, rows.Err()
}

// alarmsOf returns the active alarm of each entity (any of its datastreams): {severity, count, acknowledged}.
func (s *Server) alarmsOf(ctx context.Context, org string, ents []*entity) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	owner := map[string][]*entity{}
	var ids []string
	for _, e := range ents {
		for _, d := range e.DS {
			if owner[d] == nil {
				ids = append(ids, d)
			}
			owner[d] = append(owner[d], e)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT datastream_id::text, max(CASE severity WHEN 'critical' THEN 3 WHEN 'action' THEN 2 ELSE 1 END), count(*), bool_and(state = 'acknowledged')
		FROM core.alarm WHERE org_id = $1::uuid AND datastream_id = ANY($2::uuid[]) AND cleared_at IS NULL AND state <> 'cleared' GROUP BY datastream_id`, org, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var sev, n int
		var acked bool
		if rows.Scan(&id, &sev, &n, &acked) != nil {
			continue
		}
		for _, e := range owner[id] {
			k := e.Type + ":" + e.ID
			cur := out[k]
			if cur == nil {
				out[k] = map[string]any{"severity": sevName[sev], "count": n, "acknowledged": acked, "rank": sev}
				continue
			}
			cur["count"] = cur["count"].(int) + n
			cur["acknowledged"] = cur["acknowledged"].(bool) && acked
			if sev > cur["rank"].(int) {
				cur["rank"], cur["severity"] = sev, sevName[sev]
			}
		}
	}
	for _, a := range out {
		delete(a, "rank")
	}
	return out, rows.Err()
}

// orgAttributes are the attributes of the organization (settings.attributes).
func (s *Server) orgAttributes(ctx context.Context, org string) map[string]any {
	var b []byte
	_ = s.pool.QueryRow(ctx, `SELECT COALESCE(settings->'attributes', '{}'::jsonb) FROM core.organization WHERE id = $1::uuid`, org).Scan(&b)
	return jsonMap(b)
}

// ---- key filters ---------------------------------------------------------------------------------------------------

// evalEnv is what a key filter compares with.
type evalEnv struct {
	Values  map[string]string // value filters of the dashboard
	Current *entity           // the entity of the dashboard state
	Org     map[string]any    // attributes of the organization
	Latest  map[string]*latestVal
	Alarms  map[string]map[string]any
}

func (env *evalEnv) latest(e *entity, key string) *latestVal {
	id := ""
	if key == "" {
		if e.Type == "datastream" {
			id = e.ID
		}
	} else if e.ByKey != nil {
		id = e.ByKey[key]
	}
	if id == "" {
		return nil
	}
	return env.Latest[id]
}

func numAny(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, !math.IsNaN(x)
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(strings.ReplaceAll(x, ",", ".")), 64)
		return f, err == nil
	}
	return 0, false
}

func toBool(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case float64:
		return x != 0, true
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "1", "yes", "on":
			return true, true
		case "false", "0", "no", "off":
			return false, true
		}
	}
	return false, false
}

func toText(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(x), true
	case nil:
		return "", false
	}
	b, err := json.Marshal(v)
	return string(b), err == nil
}

// valueOf is the latest value as a Go value: the number, or the JSON value (text, boolean).
func (lv *latestVal) valueOf() any {
	if lv == nil {
		return nil
	}
	if lv.Value != nil {
		return *lv.Value
	}
	if len(lv.JSON) > 0 {
		var v any
		if json.Unmarshal(lv.JSON, &v) == nil {
			return v
		}
	}
	return nil
}

// compare applies op of a rule of the type to the entity's value x and the rule's value y (and y2 for between).
func compare(typ, op string, x, y, y2 any) bool {
	switch typ {
	case "number":
		a, ok1 := numAny(x)
		if !ok1 {
			return false
		}
		if op == "between" {
			lo, ok2 := numAny(y)
			hi, ok3 := numAny(y2)
			if !ok2 || !ok3 {
				return false
			}
			if lo > hi {
				lo, hi = hi, lo
			}
			return a >= lo && a <= hi
		}
		b, ok2 := numAny(y)
		if !ok2 {
			return false
		}
		switch op {
		case "eq":
			return a == b
		case "ne":
			return a != b
		case "gt":
			return a > b
		case "ge":
			return a >= b
		case "lt":
			return a < b
		case "le":
			return a <= b
		}
	case "boolean":
		a, ok1 := toBool(x)
		b, ok2 := toBool(y)
		if !ok1 || !ok2 {
			return false
		}
		if op == "ne" {
			return a != b
		}
		return a == b
	default:
		a, ok := toText(x)
		if !ok {
			return false
		}
		a = strings.ToLower(a)
		if op == "in" {
			for _, v := range anyStrings(y) {
				if strings.ToLower(v) == a {
					return true
				}
			}
			return false
		}
		bs, ok := toText(y)
		if !ok {
			return false
		}
		b := strings.ToLower(bs)
		switch op {
		case "eq":
			return a == b
		case "ne":
			return a != b
		case "contains":
			return strings.Contains(a, b)
		case "starts":
			return strings.HasPrefix(a, b)
		case "ends":
			return strings.HasSuffix(a, b)
		}
	}
	return false
}

// evalRule tells whether the entity satisfies one condition (or group).
func evalRule(r map[string]any, e *entity, env *evalEnv) bool {
	if rules, ok := r["rules"]; ok {
		return evalRules(r["match"], rules, e, env)
	}
	src, _ := r["source"].(string)
	key, _ := r["key"].(string)
	op, _ := r["op"].(string)
	switch src {
	case "value", "attribute":
		var x any
		if src == "value" {
			x = env.latest(e, key).valueOf()
		} else {
			x, _ = e.attr(key)
		}
		if x == nil {
			return false
		}
		y := r["value"]
		if d, ok := r["dynamic"].(map[string]any); ok {
			dk, _ := d["key"].(string)
			var dv any
			var found bool
			switch d["from"] {
			case "filter":
				var sv string
				sv, found = env.Values[dk]
				found = found && sv != ""
				dv = sv
			case "self":
				dv, found = e.attr(dk)
			case "current":
				dv, found = env.Current.attr(dk)
			case "org":
				dv, found = env.Org[dk]
				found = found && dv != nil
			}
			if found {
				y = dv
			}
		}
		if y == nil {
			return false
		}
		typ, _ := r["type"].(string)
		if typ == "" {
			typ = "number"
		}
		return compare(typ, op, x, y, r["value2"])
	case "quality":
		lv := env.latest(e, key)
		want, _ := r["value"].(string)
		q := "none"
		if lv != nil {
			q = lv.Quality
		}
		var is bool
		switch want {
		case "good":
			is = q == "ok" || q == "backfilled"
		case "doubtful":
			is = lv != nil && q != "ok" && q != "backfilled"
		default:
			is = q == want
		}
		if op == "ne" {
			return !is
		}
		return is
	case "stale":
		lv := env.latest(e, key)
		stale := lv == nil || lv.Stale
		want, _ := r["value"].(bool)
		if op == "ne" {
			return stale != want
		}
		return stale == want
	case "alarm":
		al := env.Alarms[e.Type+":"+e.ID]
		want, _ := r["value"].(string)
		is := al != nil && (want == "any" || want == "" || al["severity"] == want)
		if op == "ne" {
			return !is
		}
		return is
	}
	return false
}

func evalRules(match any, rules any, e *entity, env *evalEnv) bool {
	var list []map[string]any
	switch xs := rules.(type) {
	case []map[string]any:
		list = xs
	case []any:
		for _, x := range xs {
			if m, ok := x.(map[string]any); ok {
				list = append(list, m)
			}
		}
	}
	if len(list) == 0 {
		return true
	}
	anyOf := match == "any"
	for _, r := range list {
		ok := evalRule(r, e, env)
		if anyOf && ok {
			return true
		}
		if !anyOf && !ok {
			return false
		}
	}
	return !anyOf
}

// evalFilter tells whether the entity matches the key filter (nil = no filter: everything matches).
func evalFilter(f map[string]any, e *entity, env *evalEnv) bool {
	if f == nil {
		return true
	}
	return evalRules(f["match"], f["rules"], e, env)
}

// filterNeeds lists the datastream keys a filter compares (values, quality, staleness), and whether it needs the
// alarm state or the attributes of the organization.
func filterNeeds(f map[string]any, keys map[string]bool) (alarm, org bool) {
	if f == nil {
		return false, false
	}
	var walk func(rules any)
	walk = func(rules any) {
		var list []map[string]any
		switch xs := rules.(type) {
		case []map[string]any:
			list = xs
		case []any:
			for _, x := range xs {
				if m, ok := x.(map[string]any); ok {
					list = append(list, m)
				}
			}
		}
		for _, r := range list {
			if sub, ok := r["rules"]; ok {
				walk(sub)
				continue
			}
			switch r["source"] {
			case "value", "quality", "stale":
				k, _ := r["key"].(string)
				keys[k] = true
			case "alarm":
				alarm = true
			}
			if d, ok := r["dynamic"].(map[string]any); ok && d["from"] == "org" {
				org = true
			}
		}
	}
	walk(f["rules"])
	return alarm, org
}

// ---- resolution ----------------------------------------------------------------------------------------------------

type resolveReq struct {
	Alias, Filter, Highlight map[string]any
	Current                  *entRef
	Values                   map[string]string // value filters
	Flt                      *dsSel            // dashboard filters of a widget that follows them
	Want                     string            // entities, datastreams, devices
	Quantity, Keys           []string          // narrow the datastreams of the entities
	Cols, Attrs              []string          // latest values and attributes shown per entity
	Limit                    int
	Latest                   bool
	Win                      *dsWindow
	Scope                    *shareInfo // public link: nothing outside the dashboard's scope
}

func (rq *resolveReq) inScope(e *entity) bool {
	si := rq.Scope
	if si == nil {
		return true
	}
	if si.Ents[e.Type+":"+e.ID] || (e.Type == "device" && si.Devs[e.ID]) || (e.Type == "datastream" && si.DS[e.ID]) {
		return true
	}
	for _, d := range e.DS {
		if si.DS[d] {
			return true
		}
	}
	return false
}

// resolve runs an alias with its key filter and answers with entities, datastreams or devices.
func (s *Server) resolve(ctx context.Context, org string, rq *resolveReq) (map[string]any, error) {
	typ, ids, more, err := s.aliasCandidates(ctx, org, rq.Alias, rq.Current)
	if err != nil {
		return nil, err
	}
	empty := map[string]any{"type": typ, "items": []any{}, "total": 0, "truncated": false, "limit": rq.Limit}
	if typ == "" || len(ids) == 0 {
		return empty, nil
	}
	ents, err := s.loadEntities(ctx, org, typ, ids)
	if err != nil {
		return nil, err
	}
	if err := s.loadDS(ctx, org, ents); err != nil {
		return nil, err
	}
	if rq.Scope != nil {
		kept := ents[:0]
		for _, e := range ents {
			if rq.inScope(e) {
				kept = append(kept, e)
			}
		}
		ents = kept
	}
	// dashboard filters: an entity stays when one of its datastreams matches them
	if rq.Flt != nil && len(ents) > 0 {
		var all []string
		for _, e := range ents {
			all = append(all, e.DS...)
		}
		_, match, _, err := s.queryDS(ctx, org, []dsSel{{IDs: uniq(all)}, *rq.Flt}, 50000)
		if err != nil {
			return nil, err
		}
		ok := setOf(match...)
		kept := ents[:0]
		for _, e := range ents {
			for _, d := range e.DS {
				if ok[d] {
					kept = append(kept, e)
					break
				}
			}
		}
		ents = kept
	}
	// what the key filters and the columns need: latest values by key, alarms, attributes of the organization
	keys := map[string]bool{}
	for _, k := range rq.Cols {
		keys[k] = true
	}
	al1, org1 := filterNeeds(rq.Filter, keys)
	al2, org2 := filterNeeds(rq.Highlight, keys)
	env := &evalEnv{Values: rq.Values, Latest: map[string]*latestVal{}, Alarms: map[string]map[string]any{}}
	if org1 || org2 {
		env.Org = s.orgAttributes(ctx, org)
	}
	if rq.Current != nil {
		if cs, err := s.loadEntities(ctx, org, rq.Current.Type, []string{rq.Current.ID}); err == nil && len(cs) > 0 {
			env.Current = cs[0]
		}
	}
	if len(keys) > 0 {
		var need []string
		for _, e := range ents {
			for k := range keys {
				if k == "" && e.Type == "datastream" {
					need = append(need, e.ID)
				} else if id := e.ByKey[k]; id != "" {
					need = append(need, id)
				}
			}
		}
		if env.Latest, err = s.latestOf(ctx, org, uniq(need)); err != nil {
			return nil, err
		}
	}
	if al1 || al2 || rq.Want == "entities" {
		if env.Alarms, err = s.alarmsOf(ctx, org, ents); err != nil {
			return nil, err
		}
	}
	if rq.Filter != nil {
		kept := ents[:0]
		for _, e := range ents {
			if evalFilter(rq.Filter, e, env) {
				kept = append(kept, e)
			}
		}
		ents = kept
	}
	switch rq.Want {
	case "datastreams", "devices":
		var all []string
		for _, e := range ents {
			all = append(all, e.DS...)
		}
		all = uniq(all)
		if rq.Scope != nil {
			kept := all[:0]
			for _, d := range all {
				if rq.Scope.DS[d] {
					kept = append(kept, d)
				}
			}
			all = kept
		}
		if len(all) == 0 {
			return empty, nil
		}
		sels := []dsSel{{IDs: all, Quantities: rq.Quantity, Keys: rq.Keys}}
		if rq.Flt != nil {
			sels = append(sels, *rq.Flt)
		}
		if rq.Want == "devices" {
			_, dsIDs, _, err := s.queryDS(ctx, org, sels, 50000)
			if err != nil {
				return nil, err
			}
			devIDs, err := s.convert(ctx, org, "datastream", dsIDs, "device")
			if err != nil {
				return nil, err
			}
			if rq.Scope != nil && !rq.Scope.AllDevs {
				kept := devIDs[:0]
				for _, d := range devIDs {
					if rq.Scope.Devs[d] {
						kept = append(kept, d)
					}
				}
				devIDs = kept
			}
			total := len(devIDs)
			if len(devIDs) > rq.Limit {
				devIDs = devIDs[:rq.Limit]
			}
			devs, err := s.loadEntities(ctx, org, "device", devIDs)
			if err != nil {
				return nil, err
			}
			items := []map[string]any{}
			for _, d := range devs {
				items = append(items, map[string]any{"type": "device", "id": d.ID, "name": d.Name, "site": d.Site, "asset": d.Asset})
			}
			return map[string]any{"type": "device", "items": items, "total": total, "truncated": total > len(items) || more, "limit": rq.Limit}, nil
		}
		items, dsIDs, total, err := s.queryDS(ctx, org, sels, rq.Limit)
		if err != nil {
			return nil, err
		}
		if err := s.enrichDS(ctx, org, items, dsIDs, rq.Latest, rq.Win); err != nil {
			return nil, err
		}
		if rq.Highlight != nil && len(dsIDs) > 0 {
			dents, err := s.loadEntities(ctx, org, "datastream", dsIDs)
			if err == nil {
				err = s.loadDS(ctx, org, dents)
			}
			if err != nil {
				return nil, err
			}
			hk := map[string]bool{}
			hAlarm, _ := filterNeeds(rq.Highlight, hk)
			var need []string
			for _, e := range dents {
				for k := range hk {
					if k == "" {
						need = append(need, e.ID)
					} else if id := e.ByKey[k]; id != "" {
						need = append(need, id)
					}
				}
			}
			henv := &evalEnv{Values: env.Values, Current: env.Current, Org: env.Org, Alarms: map[string]map[string]any{}}
			if henv.Latest, err = s.latestOf(ctx, org, uniq(need)); err != nil {
				return nil, err
			}
			if hAlarm {
				if henv.Alarms, err = s.alarmsOf(ctx, org, dents); err != nil {
					return nil, err
				}
			}
			match := map[string]bool{}
			for _, e := range dents {
				match[e.ID] = evalFilter(rq.Highlight, e, henv)
			}
			for _, it := range items {
				it["match"] = match[it["id"].(string)]
			}
		}
		return map[string]any{"type": "datastream", "items": items, "total": total, "truncated": total > len(items) || more, "limit": rq.Limit}, nil
	}
	total := len(ents)
	if len(ents) > rq.Limit {
		ents = ents[:rq.Limit]
	}
	items := make([]map[string]any, 0, len(ents))
	for _, e := range ents {
		items = append(items, entityItem(e, rq, env))
	}
	return map[string]any{"type": typ, "items": items, "total": total, "truncated": total > len(items) || more, "limit": rq.Limit}, nil
}

// entityItem is the answer for one entity.
func entityItem(e *entity, rq *resolveReq, env *evalEnv) map[string]any {
	it := map[string]any{"type": e.Type, "id": e.ID, "name": e.Name}
	set := func(k, v string) {
		if v != "" {
			it[k] = v
		}
	}
	set("site_id", e.SiteID)
	set("site", e.Site)
	set("asset_id", e.AssetID)
	set("asset", e.Asset)
	set("asset_type", e.AssetType)
	set("device_id", e.DeviceID)
	set("device", e.Device)
	set("profile", e.Profile)
	set("external_id", e.ExtID)
	set("key", e.Key)
	set("quantity", e.Quantity)
	set("unit", e.Unit)
	set("parent_id", e.ParentID)
	if e.LastSeen != nil {
		it["last_seen"] = *e.LastSeen
	}
	it["datastreams"] = len(e.DS)
	if len(rq.Cols) > 0 {
		vals := map[string]any{}
		for _, k := range rq.Cols {
			id := e.ByKey[k]
			if k == "" {
				continue
			}
			lv := env.Latest[id]
			if id == "" || lv == nil {
				continue
			}
			v := map[string]any{"datastream_id": id, "value": lv.Value, "measured_at": lv.At, "quality": lv.Quality}
			if u := e.Units[id]; u != "" {
				v["unit"] = u
			}
			if lv.Stale {
				v["stale"] = true
			}
			if len(lv.JSON) > 0 {
				v["value_json"] = lv.JSON
			}
			vals[k] = v
		}
		it["values"] = vals
	}
	if len(rq.Attrs) > 0 {
		attrs := map[string]any{}
		for _, k := range rq.Attrs {
			if v, ok := e.attr(k); ok {
				attrs[k] = v
			}
		}
		it["attributes"] = attrs
	}
	if a := env.Alarms[e.Type+":"+e.ID]; a != nil {
		it["alarm"] = a
	}
	if rq.Highlight != nil {
		it["match"] = evalFilter(rq.Highlight, e, env)
	}
	return it
}

func uniq(list []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(list))
	for _, x := range list {
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// columnKeys reads the telemetry (t:<key>) and attribute (a:<key>) columns of an entity table.
func columnKeys(cols []string) (vals, attrs []string) {
	for _, c := range cols {
		c, _, _ = strings.Cut(c, "=")
		c = strings.TrimSpace(c)
		if k, ok := strings.CutPrefix(c, "t:"); ok && dsKeyRe.MatchString(k) {
			vals = append(vals, k)
		}
		if k, ok := strings.CutPrefix(c, "a:"); ok && attrKeyRe.MatchString(k) {
			attrs = append(attrs, k)
		}
	}
	return uniq(vals), uniq(attrs)
}

// ---- handlers ------------------------------------------------------------------------------------------------------

func jsonParam(v url.Values, name string) (map[string]any, error) {
	s := v.Get(name)
	if s == "" {
		return nil, nil
	}
	if len(s) > 32<<10 {
		return nil, fmt.Errorf("%s: too long", name)
	}
	m := map[string]any{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, fmt.Errorf("%s must be a JSON object", name)
	}
	return m, nil
}

var wantKinds = setOf("entities", "datastreams", "devices")

// GET /api/v1/entities/query?alias={json}&filter={json}&highlight={json}&current=type:id&want=…
func (s *Server) entitiesQuery(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	v := r.URL.Query()
	rq := &resolveReq{}
	fail := func(err error) { writeErr(w, 400, err.Error()) }
	raw, err := jsonParam(v, "alias")
	if err != nil {
		fail(err)
		return
	}
	if raw == nil {
		fail(errors.New("alias required"))
		return
	}
	if raw["id"] == nil {
		raw["id"] = "a"
	}
	if rq.Alias, err = validateAlias(0, raw); err != nil {
		fail(err)
		return
	}
	for _, p := range []struct {
		name string
		dst  *map[string]any
	}{{"filter", &rq.Filter}, {"highlight", &rq.Highlight}} {
		f, err := jsonParam(v, p.name)
		if err != nil {
			fail(err)
			return
		}
		if f != nil {
			if f["id"] == nil {
				f["id"] = "f"
			}
			if *p.dst, err = validateKeyFilter(0, f); err != nil {
				fail(fmt.Errorf("%s: %v", p.name, err))
				return
			}
		}
	}
	if rq.Current, err = parseRef(v.Get("current")); err != nil {
		fail(err)
		return
	}
	if rq.Values, err = valueFilters(v); err != nil {
		fail(err)
		return
	}
	if err := rq.common(v); err != nil {
		fail(err)
		return
	}
	if rq.Quantity, err = stringList(splitList(v.Get("quantity")), maxFilterValues, "quantity"); err != nil {
		fail(err)
		return
	}
	if rq.Keys, err = keyList(v.Get("key"), "key"); err != nil {
		fail(err)
		return
	}
	if rq.Cols, err = keyList(v.Get("values"), "values"); err != nil {
		fail(err)
		return
	}
	if rq.Attrs, err = stringList(splitList(v.Get("attrs")), 24, "attrs"); err != nil {
		fail(err)
		return
	}
	for _, a := range rq.Attrs {
		if !attrKeyRe.MatchString(a) {
			fail(fmt.Errorf("attrs: %q is not an attribute name", a))
			return
		}
	}
	if v.Get("follow") == "1" {
		flt, err := parseSel(v, "f_")
		if err != nil {
			fail(err)
			return
		}
		rq.Flt = &flt
	}
	out, err := s.resolve(r.Context(), org, rq)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, out)
}

// common reads want, limit, latest and the window.
func (rq *resolveReq) common(v url.Values) error {
	rq.Want = v.Get("want")
	if rq.Want == "" {
		rq.Want = "entities"
	}
	if !wantKinds[rq.Want] {
		return errors.New("want must be entities, datastreams or devices")
	}
	var err error
	if rq.Limit, err = queryLimit(v, 100, maxSourceRows); err != nil {
		return err
	}
	rq.Latest = v.Get("latest") == "1"
	rq.Win, err = parseWindow(v)
	return err
}

func keyList(s, what string) ([]string, error) {
	l, err := stringList(splitList(s), maxFilterValues, what)
	if err != nil {
		return nil, err
	}
	for _, k := range l {
		if !dsKeyRe.MatchString(k) {
			return nil, fmt.Errorf("%s: %q is not a datastream key", what, k)
		}
	}
	return l, nil
}

// wantOf is what a widget of the type shows from its alias.
func wantOf(typ string) string {
	def := widgetDefOf(typ)
	switch {
	case def == nil:
		return "entities"
	case def.Entities:
		return "entities"
	case hasBind(def, "datastreams") || typ == "alarms" || typ == "alarm_count":
		return "datastreams"
	case hasBind(def, "devices"):
		return "devices"
	}
	return "entities"
}

// publicCurrent accepts the current entity of a public viewer only within the dashboard's scope.
func (s *Server) publicCurrent(ctx context.Context, si *shareInfo, ref *entRef) *entRef {
	if ref == nil {
		return nil
	}
	ents, err := s.loadEntities(ctx, si.Org, ref.Type, []string{ref.ID})
	if err != nil || len(ents) == 0 || s.loadDS(ctx, si.Org, ents) != nil {
		return nil
	}
	if (&resolveReq{Scope: si}).inScope(ents[0]) {
		return ref
	}
	return nil
}

// GET /api/v1/public/{token}/entities/query?widget=<id>&current=type:id&f_…&f_value=…
func (s *Server) publicEntitiesQuery(w http.ResponseWriter, r *http.Request) {
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
	wa, _ := wd["alias"].(map[string]any)
	id, _ := wa["id"].(string)
	al := specByID(si.Spec, "aliases", id)
	if al == nil {
		writeErr(w, 404, "the widget takes no alias")
		return
	}
	typ, _ := wd["type"].(string)
	params, _ := wd["params"].(map[string]any)
	fid, _ := wa["filter"].(string)
	hid, _ := params["highlight_filter"].(string)
	rq := &resolveReq{Alias: al, Filter: specByID(si.Spec, "key_filters", fid), Highlight: specByID(si.Spec, "key_filters", hid), Scope: si,
		Quantity: anyStrings(wa["quantity"]), Keys: anyStrings(wa["key"]), Values: publicValueFilters(si.Spec, v)}
	cur, err := parseRef(v.Get("current"))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	rq.Current = s.publicCurrent(r.Context(), si, cur)
	if err := rq.common(v); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	rq.Want = wantOf(typ)
	if rq.Want == "entities" {
		rq.Cols, rq.Attrs = columnKeys(anyStrings(params["columns"]))
	}
	if wd["follow"] == true {
		flt, err := publicFilterSel(si.Spec, v)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		rq.Flt = &flt
	}
	out, err := s.resolve(r.Context(), si.Org, rq)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, out)
}

// resolveAliases adds what the aliases of a dashboard select (without key filters and without a current entity) to
// what a public link or a display may read: the entities, all their datastreams and their devices. An alias of the
// current entity (and a hierarchy under it) adds nothing: the current entity of a viewer must be in this scope.
func (s *Server) resolveAliases(ctx context.Context, si *shareInfo) {
	for _, al := range specList(si.Spec, "aliases") {
		if al["kind"] == "current" || al["root_current"] == true {
			continue
		}
		typ, ids, _, err := s.aliasCandidates(ctx, si.Org, al, nil)
		if err != nil || len(ids) == 0 {
			continue
		}
		ents, err := s.loadEntities(ctx, si.Org, typ, ids)
		if err != nil || s.loadDS(ctx, si.Org, ents) != nil {
			continue
		}
		var all []string
		for _, e := range ents {
			si.Ents[e.Type+":"+e.ID] = true
			for _, d := range e.DS {
				si.DS[d] = true
				all = append(all, d)
			}
		}
		if devs, err := s.convert(ctx, si.Org, "datastream", uniq(all), "device"); err == nil {
			for _, d := range devs {
				si.Devs[d] = true
			}
		}
	}
}

// ---- attributes of the organization -------------------------------------------------------------------------------

// GET /api/v1/organization/attributes
func (s *Server) getOrgAttributes(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, map[string]any{"attributes": s.orgAttributes(r.Context(), org)})
}

// PUT /api/v1/organization/attributes {attributes: {key: text | number | boolean}, reason}
func (s *Server) putOrgAttributes(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		Attributes map[string]any `json:"attributes"`
		Reason     string         `json:"reason"`
	}
	if err := decodeJSON(r, &in, 64<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if len(in.Attributes) > 50 {
		writeErr(w, 400, "attributes: at most 50")
		return
	}
	for k, v := range in.Attributes {
		if !attrKeyRe.MatchString(k) {
			writeErr(w, 400, fmt.Sprintf("attributes: %q is not a valid name (letters, digits, . _ -)", k))
			return
		}
		switch x := v.(type) {
		case string:
			if len(x) > 200 {
				writeErr(w, 400, "attributes: a value is longer than 200 characters")
				return
			}
		case float64, bool:
		default:
			writeErr(w, 400, "attributes: values must be text, numbers or true/false")
			return
		}
	}
	if in.Attributes == nil {
		in.Attributes = map[string]any{}
	}
	old := s.orgAttributes(r.Context(), org)
	b, _ := json.Marshal(in.Attributes)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.organization SET settings = jsonb_set(coalesce(settings, '{}'::jsonb), '{attributes}', $2::jsonb) WHERE id = $1::uuid`, org, string(b)); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	s.audit(r, "organization.attributes", "organization", org, old, in.Attributes, in.Reason)
	writeJSON(w, 200, map[string]any{"attributes": in.Attributes})
}

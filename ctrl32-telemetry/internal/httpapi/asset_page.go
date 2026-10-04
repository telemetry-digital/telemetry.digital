package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"
)

// Asset and site pages (0.74.0): the assets list as a table, one page per asset (overview, telemetry, datastreams,
// alarm rules, history) and one per site (assets tree, devices, alarms, location). Everything is scoped to the
// caller's organization; the queries are bounded by the asset (or site) and use the (datastream_id, measured_at)
// index for the latest values.
//
//	GET /api/v1/assets?site_id=                       rows with datastream, device and child counts, worst active alarm, last value time
//	GET /api/v1/assets/{id}                           asset, site, ancestors, children, datastreams, devices, active alarms, location
//	GET /api/v1/assets/{id}/history?before=&limit=    audit entries of the asset, its datastreams, their alarm rules and assignments (audit.read)
//	GET /api/v1/assets/{id}/telemetry?from=&to=&children=1          every datastream: latest value, statistics, sparkline
//	GET /api/v1/assets/{id}/telemetry/series|values|export?datastream_id=   as on the device page
//	GET /api/v1/sites/{id}                            site with its assets, devices and active alarms

// assetMaxDepth bounds walks of the asset hierarchy (a cycle written directly into the database cannot loop).
const assetMaxDepth = 32

type assetRow struct {
	ID          string          `json:"id"`
	SiteID      string          `json:"site_id"`
	Site        string          `json:"site"`
	ParentID    string          `json:"parent_id"`
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Attributes  json.RawMessage `json:"attributes"`
	CreatedAt   time.Time       `json:"created_at"`
	Datastreams int             `json:"datastreams"`
	Devices     int             `json:"devices"`
	Children    int             `json:"children"`
	Alarm       map[string]any  `json:"alarm"`   // worst active alarm of its datastreams: {severity, count, acknowledged}; null = none
	LastAt      *time.Time      `json:"last_at"` // newest value of any of its datastreams
}

// assetRows lists assets with their counts, worst active alarm and last value time. Filters: site, parent, one asset.
func (s *Server) assetRows(ctx context.Context, org, site, parent, id string) ([]assetRow, error) {
	rows, err := s.pool.Query(ctx, `
		WITH a AS (
			SELECT a.id, a.org_id, a.site_id, a.parent_id, a.type, a.name, a.attributes, a.created_at, st.name AS site
			FROM core.asset a JOIN core.site st ON st.id = a.site_id
			WHERE ($1 = '' OR a.org_id = $1::uuid) AND ($2 = '' OR a.site_id = $2::uuid) AND ($3 = '' OR a.parent_id = $3::uuid) AND ($4 = '' OR a.id = $4::uuid)
		), ds AS (
			SELECT d.id, d.asset_id FROM core.datastream d JOIN a ON a.id = d.asset_id
		), dev AS (
			SELECT ds.asset_id, count(DISTINCT g.from_id) AS n FROM core.assignment g JOIN ds ON ds.id = g.to_id
			WHERE g.kind = 'device_datastream' AND g.valid_to IS NULL GROUP BY ds.asset_id
		), al AS (
			SELECT ds.asset_id, max(CASE x.severity WHEN 'critical' THEN 3 WHEN 'action' THEN 2 ELSE 1 END) AS sev, count(*) AS n, bool_and(x.state = 'acknowledged') AS ack
			FROM core.alarm x JOIN ds ON ds.id = x.datastream_id
			WHERE ($1 = '' OR x.org_id = $1::uuid) AND x.cleared_at IS NULL AND x.state <> 'cleared' GROUP BY ds.asset_id
		), last AS (
			SELECT ds.asset_id, max(l.measured_at) AS at FROM ds
			CROSS JOIN LATERAL (SELECT o.measured_at FROM core.observation o WHERE o.datastream_id = ds.id ORDER BY o.measured_at DESC LIMIT 1) l
			GROUP BY ds.asset_id
		)
		SELECT a.id::text, a.site_id::text, a.site, COALESCE(a.parent_id::text, ''), a.type, a.name, a.attributes, a.created_at,
		       (SELECT count(*) FROM ds WHERE ds.asset_id = a.id), COALESCE(dev.n, 0),
		       (SELECT count(*) FROM core.asset c WHERE c.parent_id = a.id),
		       COALESCE(al.sev, 0), COALESCE(al.n, 0), COALESCE(al.ack, false), last.at
		FROM a LEFT JOIN dev ON dev.asset_id = a.id LEFT JOIN al ON al.asset_id = a.id LEFT JOIN last ON last.asset_id = a.id
		ORDER BY a.site, a.name LIMIT 20000`, org, site, parent, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []assetRow{}
	for rows.Next() {
		var a assetRow
		var sev, n int
		var ack bool
		if err := rows.Scan(&a.ID, &a.SiteID, &a.Site, &a.ParentID, &a.Type, &a.Name, &a.Attributes, &a.CreatedAt, &a.Datastreams, &a.Devices, &a.Children, &sev, &n, &ack, &a.LastAt); err != nil {
			return nil, err
		}
		if n > 0 {
			a.Alarm = map[string]any{"severity": sevName[sev], "count": n, "acknowledged": ack}
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Server) listAssets(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	site := r.URL.Query().Get("site_id")
	if site != "" && !uuidRe.MatchString(site) {
		writeErr(w, 400, "site_id must be a uuid")
		return
	}
	out, err := s.assetRows(r.Context(), orgOf(p), site, "", "")
	if err != nil {
		s.log.Warn("list assets", "err", err)
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, out)
}

// location reads {lat, lon} kept in the attributes of an asset or a site.
func locationOf(attrs json.RawMessage) map[string]any {
	var a struct {
		Lat *float64 `json:"lat"`
		Lon *float64 `json:"lon"`
	}
	if json.Unmarshal(attrs, &a) != nil || a.Lat == nil || a.Lon == nil {
		return nil
	}
	return map[string]any{"lat": *a.Lat, "lon": *a.Lon}
}

// assetIDs returns the asset and, with children, every asset below it (bounded depth); nil when the asset is not
// in the organization.
func (s *Server) assetIDs(ctx context.Context, org, id string, children bool) ([]string, error) {
	if !uuidRe.MatchString(id) {
		return nil, nil
	}
	depth := 0
	if children {
		depth = assetMaxDepth
	}
	rows, err := s.pool.Query(ctx, `
		WITH RECURSIVE t AS (
			SELECT a.id, 0 AS d FROM core.asset a WHERE a.id = $1::uuid AND a.org_id = $2::uuid
			UNION SELECT c.id, t.d + 1 FROM core.asset c JOIN t ON c.parent_id = t.id WHERE t.d < $3
		) SELECT DISTINCT id::text FROM t`, id, org, depth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var x string
		if err := rows.Scan(&x); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// assetDatastreams lists the datastreams of the given assets with their current source device.
func (s *Server) assetDatastreams(ctx context.Context, org string, assets []string) ([]telDS, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id::text, d.key, d.quantity, d.unit, d.kind, a.name, a.id::text, COALESCE(dv.id::text, ''), COALESCE(NULLIF(dv.name, ''), dv.external_id, '')
		FROM core.datastream d JOIN core.asset a ON a.id = d.asset_id
		LEFT JOIN LATERAL (SELECT g.from_id FROM core.assignment g WHERE g.kind = 'device_datastream' AND g.to_id = d.id AND g.valid_to IS NULL
		                   ORDER BY g.valid_from DESC LIMIT 1) g ON true
		LEFT JOIN core.device dv ON dv.id = g.from_id
		WHERE d.org_id = $1::uuid AND d.asset_id = ANY($2::uuid[]) ORDER BY a.name, d.key LIMIT 2000`, org, assets)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []telDS
	for rows.Next() {
		var d telDS
		if err := rows.Scan(&d.ID, &d.Key, &d.Quantity, &d.Unit, &d.Kind, &d.Asset, &d.AssetID, &d.DeviceID, &d.Device); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// activeAlarms lists the active (not cleared) alarms of the datastreams, newest first.
func (s *Server) activeAlarms(ctx context.Context, org string, ds []string) []map[string]any {
	out := []map[string]any{}
	if len(ds) == 0 {
		return out
	}
	rows, err := s.pool.Query(ctx, `
		SELECT x.id::text, x.raised_at, x.state, x.severity, x.value::float8, COALESCE(d.key, x.detail->>'key', ''), x.datastream_id::text, a.id::text, a.name
		FROM core.alarm x JOIN core.datastream d ON d.id = x.datastream_id JOIN core.asset a ON a.id = d.asset_id
		WHERE x.org_id = $1::uuid AND x.datastream_id = ANY($2::uuid[]) AND x.cleared_at IS NULL AND x.state <> 'cleared'
		ORDER BY x.raised_at DESC LIMIT 200`, org, ds)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, state, sev, key, dsID, assetID, asset string
		var raised time.Time
		var val *float64
		if rows.Scan(&id, &raised, &state, &sev, &val, &key, &dsID, &assetID, &asset) == nil {
			out = append(out, map[string]any{"id": id, "raised_at": raised, "state": state, "severity": sev, "value": val, "key": key, "datastream_id": dsID, "asset_id": assetID, "asset": asset})
		}
	}
	return out
}

// feedingDevices lists the devices that currently feed the datastreams, with the keys each feeds.
func (s *Server) feedingDevices(ctx context.Context, org string, ds []string) []map[string]any {
	out := []map[string]any{}
	if len(ds) == 0 {
		return out
	}
	rows, err := s.pool.Query(ctx, `
		SELECT dv.id::text, dv.external_id, dv.name, dv.transport, dv.status->>'last_seen', dv.revoked_at, dv.location, array_agg(d.key ORDER BY d.key)
		FROM core.assignment g JOIN core.device dv ON dv.id = g.from_id JOIN core.datastream d ON d.id = g.to_id
		WHERE g.kind = 'device_datastream' AND g.valid_to IS NULL AND g.org_id = $1::uuid AND g.to_id = ANY($2::uuid[])
		GROUP BY dv.id ORDER BY dv.name, dv.external_id LIMIT 500`, org, ds)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, ext, name, tr string
		var lastSeen *string
		var revoked *time.Time
		var loc json.RawMessage
		var keys []string
		if rows.Scan(&id, &ext, &name, &tr, &lastSeen, &revoked, &loc, &keys) == nil {
			out = append(out, map[string]any{"id": id, "external_id": ext, "name": name, "transport": tr, "last_seen": lastSeen, "revoked_at": revoked,
				"location": loc, "keys": keys, "connected": s.br != nil && s.br.IsConnected(id)})
		}
	}
	return out
}

// GET /api/v1/assets/{id}
func (s *Server) getAsset(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	ctx := r.Context()
	if !uuidRe.MatchString(id) {
		writeErr(w, 404, "asset not found")
		return
	}
	rows, err := s.assetRows(ctx, org, "", "", id)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	if len(rows) == 0 {
		writeErr(w, 404, "asset not found")
		return
	}
	a := rows[0]
	var siteName, tz, addr string
	var siteAttrs json.RawMessage
	_ = s.pool.QueryRow(ctx, `SELECT name, tz, COALESCE(address, ''), attributes FROM core.site WHERE id = $1::uuid`, a.SiteID).Scan(&siteName, &tz, &addr, &siteAttrs)
	// ancestors, the root first
	path := []map[string]any{}
	if pr, err := s.pool.Query(ctx, `
		WITH RECURSIVE up AS (
			SELECT p.id, p.parent_id, p.name, p.type, 1 AS d FROM core.asset p WHERE p.id = NULLIF($1, '')::uuid AND p.org_id = $2::uuid
			UNION SELECT p.id, p.parent_id, p.name, p.type, up.d + 1 FROM core.asset p JOIN up ON p.id = up.parent_id WHERE up.d < $3
		) SELECT DISTINCT ON (id) id::text, name, type, d FROM up ORDER BY id, d`, a.ParentID, org, assetMaxDepth); err == nil {
		type anc struct {
			id, name, typ string
			d             int
		}
		var list []anc
		for pr.Next() {
			var x anc
			if pr.Scan(&x.id, &x.name, &x.typ, &x.d) == nil {
				list = append(list, x)
			}
		}
		pr.Close()
		sort.Slice(list, func(i, j int) bool { return list[i].d > list[j].d })
		for _, x := range list {
			path = append(path, map[string]any{"id": x.id, "name": x.name, "type": x.typ})
		}
	}
	children, err := s.assetRows(ctx, org, "", id, "")
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	dss, err := s.assetDatastreams(ctx, org, []string{id})
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	dsIDs := make([]string, len(dss))
	for i, d := range dss {
		dsIDs[i] = d.ID
	}
	// per datastream: physical range, expected interval, current alarm rules
	type dsInfo struct {
		Interval           string
		RangeMin, RangeMax *float64
		Rules              int
	}
	info := map[string]*dsInfo{}
	if len(dsIDs) > 0 {
		if ir, err := s.pool.Query(ctx, `SELECT d.id::text, COALESCE(d.expected_interval::text, ''),
				CASE WHEN jsonb_typeof(d.attributes->'range_min') = 'number' THEN (d.attributes->>'range_min')::float8 END,
				CASE WHEN jsonb_typeof(d.attributes->'range_max') = 'number' THEN (d.attributes->>'range_max')::float8 END,
				(SELECT count(*) FROM core.alarm_rule ar WHERE ar.datastream_id = d.id AND ar.valid_to IS NULL)
			FROM core.datastream d WHERE d.id = ANY($1::uuid[])`, dsIDs); err == nil {
			for ir.Next() {
				var dID string
				x := &dsInfo{}
				if ir.Scan(&dID, &x.Interval, &x.RangeMin, &x.RangeMax, &x.Rules) == nil {
					info[dID] = x
				}
			}
			ir.Close()
		}
	}
	now := time.Now().UTC()
	keys, err := s.datastreamKeys(r, dss, now.Add(-24*time.Hour), now.Add(time.Minute), now.Add(-24*time.Hour).Truncate(telSparkBucket), now)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	streams := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		m := map[string]any{"id": k.DatastreamID, "key": k.Key, "quantity": k.Quantity, "unit": k.Unit, "kind": k.Kind, "last": k.Last,
			"device_id": k.DeviceID, "device": k.Device}
		if x := info[k.DatastreamID]; x != nil {
			m["expected_interval"], m["range_min"], m["range_max"], m["alarm_rules"] = x.Interval, x.RangeMin, x.RangeMax, x.Rules
		}
		streams = append(streams, m)
	}
	devices := s.feedingDevices(ctx, org, dsIDs)
	// where it is: its own position, else the site's, else the devices'
	var loc map[string]any
	if l := locationOf(a.Attributes); l != nil {
		l["source"] = "asset"
		loc = l
	} else if l := locationOf(siteAttrs); l != nil {
		l["source"] = "site"
		loc = l
	}
	writeJSON(w, 200, map[string]any{
		"asset": a, "site": map[string]any{"id": a.SiteID, "name": siteName, "tz": tz, "address": addr, "location": locationOf(siteAttrs)},
		"path": path, "children": children, "datastreams": streams, "devices": devices, "alarms": s.activeAlarms(ctx, org, dsIDs), "location": loc,
	})
}

// GET /api/v1/sites/{id}
func (s *Server) getSite(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	ctx := r.Context()
	if !uuidRe.MatchString(id) {
		writeErr(w, 404, "site not found")
		return
	}
	var name, tz, addr string
	var attrs json.RawMessage
	var created time.Time
	if err := s.pool.QueryRow(ctx, `SELECT name, tz, COALESCE(address, ''), attributes, created_at FROM core.site WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).
		Scan(&name, &tz, &addr, &attrs, &created); err != nil {
		writeErr(w, 404, "site not found")
		return
	}
	assets, err := s.assetRows(ctx, org, id, "", "")
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	var dsIDs []string
	if dr, err := s.pool.Query(ctx, `SELECT d.id::text FROM core.datastream d JOIN core.asset a ON a.id = d.asset_id WHERE a.site_id = $1::uuid AND d.org_id = $2::uuid LIMIT 20000`, id, org); err == nil {
		for dr.Next() {
			var x string
			if dr.Scan(&x) == nil {
				dsIDs = append(dsIDs, x)
			}
		}
		dr.Close()
	}
	writeJSON(w, 200, map[string]any{
		"site":   map[string]any{"id": id, "name": name, "tz": tz, "address": addr, "attributes": attrs, "created_at": created, "location": locationOf(attrs)},
		"assets": assets, "datastreams": len(dsIDs), "devices": s.feedingDevices(ctx, org, dsIDs), "alarms": s.activeAlarms(ctx, org, dsIDs),
	})
}

// GET /api/v1/assets/{id}/history: the audit entries of the asset, its datastreams, their alarm rules and device
// assignments, newest first (paged by `before`, a sequence number).
func (s *Server) assetHistory(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	ids, err := s.assetIDs(r.Context(), org, id, false)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	if len(ids) == 0 {
		writeErr(w, 404, "asset not found")
		return
	}
	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	before := int64(0)
	if v := q.Get("before"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			writeErr(w, 400, "before must be a sequence number")
			return
		}
		before = n
	}
	rows, err := s.pool.Query(r.Context(), `
		WITH ds AS (SELECT id FROM core.datastream WHERE asset_id = $2::uuid AND org_id = $1::uuid),
		ents AS (
			SELECT 'asset'::text AS t, $2::uuid AS id
			UNION ALL SELECT 'datastream', id FROM ds
			UNION ALL SELECT 'alarm_rule', ar.id FROM core.alarm_rule ar WHERE ar.datastream_id IN (SELECT id FROM ds)
			UNION ALL SELECT 'assignment', g.id FROM core.assignment g WHERE g.kind = 'device_datastream' AND g.to_id IN (SELECT id FROM ds)
		)
		SELECT a.seq, a.ts, a.actor_name, a.action, a.entity_type, COALESCE(a.entity_id::text, ''), COALESCE(a.reason, ''), a.old_value, a.new_value
		FROM ents e JOIN core.audit_trail a ON a.entity_type = e.t AND a.entity_id = e.id
		WHERE a.org_id = $1::uuid AND ($3 = 0 OR a.seq < $3)
		ORDER BY a.seq DESC LIMIT $4`, org, id, before, limit+1)
	if err != nil {
		s.log.Warn("asset history", "err", err)
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	next := ""
	for rows.Next() {
		var seq int64
		var ts time.Time
		var actor, action, etype, eid, reason string
		var oldV, newV json.RawMessage
		if err := rows.Scan(&seq, &ts, &actor, &action, &etype, &eid, &reason, &oldV, &newV); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		if len(out) == limit {
			next = strconv.FormatInt(out[len(out)-1]["seq"].(int64), 10)
			break
		}
		out = append(out, map[string]any{"seq": seq, "ts": ts, "actor": actor, "action": action, "entity_type": etype, "entity_id": eid, "reason": reason, "old_value": oldV, "new_value": newV})
	}
	writeJSON(w, 200, map[string]any{"entries": out, "next_before": next})
}

// GET /api/v1/assets/{id}/telemetry
func (s *Server) assetTelemetry(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	children := r.URL.Query().Get("children") == "1" || r.URL.Query().Get("children") == "true"
	ids, err := s.assetIDs(r.Context(), org, id, children)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	if len(ids) == 0 {
		writeErr(w, 404, "asset not found")
		return
	}
	from, to, ok := telRange(w, r)
	if !ok {
		return
	}
	dss, err := s.assetDatastreams(r.Context(), org, ids)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	now := time.Now().UTC()
	keys, err := s.datastreamKeys(r, dss, from, to, now.Add(-24*time.Hour).Truncate(telSparkBucket), now)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, map[string]any{"asset_id": id, "children": children, "assets": len(ids), "from": from, "to": to, "generated_at": now, "keys": keys})
}

// assetTelSource resolves ?datastream_id= to a datastream of the asset or of an asset below it.
func (s *Server) assetTelSource(w http.ResponseWriter, r *http.Request) (string, string, telSrc, bool) {
	var src telSrc
	org, ok := s.mustOrg(w, r)
	if !ok {
		return "", "", src, false
	}
	id := r.PathValue("id")
	dsID := r.URL.Query().Get("datastream_id")
	if !uuidRe.MatchString(dsID) {
		writeErr(w, 400, "datastream_id required")
		return "", "", src, false
	}
	ids, err := s.assetIDs(r.Context(), org, id, true)
	if err != nil {
		writeErr(w, 500, "query failed")
		return "", "", src, false
	}
	if len(ids) == 0 {
		writeErr(w, 404, "asset not found")
		return "", "", src, false
	}
	var d telDS
	if err := s.pool.QueryRow(r.Context(), `SELECT d.id::text, d.key, d.quantity, d.unit, d.kind, a.name, a.id::text FROM core.datastream d JOIN core.asset a ON a.id = d.asset_id
		WHERE d.id = $1::uuid AND d.org_id = $2::uuid AND d.asset_id = ANY($3::uuid[])`, dsID, org, ids).Scan(&d.ID, &d.Key, &d.Quantity, &d.Unit, &d.Kind, &d.Asset, &d.AssetID); err != nil {
		writeErr(w, 404, "no such datastream on this asset")
		return "", "", src, false
	}
	return org, id, telSrc{Source: "datastream", Key: d.Key, DS: &d}, true
}

// GET /api/v1/assets/{id}/telemetry/series
func (s *Server) assetTelemetrySeries(w http.ResponseWriter, r *http.Request) {
	if org, _, src, ok := s.assetTelSource(w, r); ok {
		s.telSeries(w, r, org, "", src)
	}
}

// GET /api/v1/assets/{id}/telemetry/values
func (s *Server) assetTelemetryValues(w http.ResponseWriter, r *http.Request) {
	if org, _, src, ok := s.assetTelSource(w, r); ok {
		s.telValues(w, r, org, "", src)
	}
}

// GET /api/v1/assets/{id}/telemetry/export
func (s *Server) assetTelemetryExport(w http.ResponseWriter, r *http.Request) {
	if org, id, src, ok := s.assetTelSource(w, r); ok {
		s.telExport(w, r, org, "", src, "asset", id, src.DS.Asset, "asset_telemetry")
	}
}

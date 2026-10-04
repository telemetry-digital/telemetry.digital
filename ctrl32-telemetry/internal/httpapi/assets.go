package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Configuration of the data model (ADR-0004): sites, assets, datastreams, device-to-datastream assignments and
// alarm rules. Reads need data.read; sites/assets/datastreams/alarm rules need config.write; assignments and
// device edits need device.manage. Every change carries a reason and is written to the audit trail.
// Assignments and alarm rules are history rows: a change closes the current row (valid_to) and inserts a new one.
//
//	GET    /api/v1/sites                              POST /api/v1/sites                 PATCH /api/v1/sites/{id}
//	GET    /api/v1/assets?site_id= (asset_page.go)    POST /api/v1/assets                PATCH /api/v1/assets/{id}
//	GET    /api/v1/datastreams/{id}                   POST /api/v1/datastreams           PATCH /api/v1/datastreams/{id}
//	GET    /api/v1/alarm-rules?datastream_id=         POST /api/v1/alarm-rules           DELETE /api/v1/alarm-rules/{id}
//	GET    /api/v1/devices/{id}                       PATCH /api/v1/devices/{id}
//	POST   /api/v1/devices/{id}/datastreams           DELETE /api/v1/devices/{id}/datastreams/{ds}

var (
	dsKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	tzRe    = regexp.MustCompile(`^[A-Za-z0-9_+\-/]{1,64}$`)
)

func reasonOf(w http.ResponseWriter, reason string) bool {
	if strings.TrimSpace(reason) == "" {
		writeErr(w, 400, "reason required")
		return false
	}
	return true
}

func (s *Server) mustOrg(w http.ResponseWriter, r *http.Request) (string, bool) {
	p := PrincipalFrom(r.Context())
	if p == nil || p.OrgID == "" {
		writeErr(w, 403, "organization scope required")
		return "", false
	}
	return p.OrgID, true
}

// ---- sites --------------------------------------------------------------------------------------

func (s *Server) listSites(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `
		WITH st AS (SELECT s.* FROM core.site s WHERE ($1 = '' OR s.org_id = $1::uuid)),
		ds AS (SELECT d.id, a.site_id FROM core.datastream d JOIN core.asset a ON a.id = d.asset_id JOIN st ON st.id = a.site_id),
		dev AS (SELECT ds.site_id, count(DISTINCT g.from_id) AS n FROM core.assignment g JOIN ds ON ds.id = g.to_id
		        WHERE g.kind = 'device_datastream' AND g.valid_to IS NULL GROUP BY ds.site_id),
		al AS (SELECT ds.site_id, max(CASE x.severity WHEN 'critical' THEN 3 WHEN 'action' THEN 2 ELSE 1 END) AS sev, count(*) AS n, bool_and(x.state = 'acknowledged') AS ack
		       FROM core.alarm x JOIN ds ON ds.id = x.datastream_id WHERE ($1 = '' OR x.org_id = $1::uuid) AND x.cleared_at IS NULL AND x.state <> 'cleared' GROUP BY ds.site_id)
		SELECT st.id::text, st.name, st.tz, COALESCE(st.address,''), COALESCE(st.customer_id::text,''), st.created_at,
		       (SELECT count(*) FROM core.asset a WHERE a.site_id = st.id), (SELECT count(*) FROM ds WHERE ds.site_id = st.id), COALESCE(dev.n, 0),
		       COALESCE(al.sev, 0), COALESCE(al.n, 0), COALESCE(al.ack, false), st.attributes
		FROM st LEFT JOIN dev ON dev.site_id = st.id LEFT JOIN al ON al.site_id = st.id ORDER BY st.name`, orgOf(p))
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, tz, addr, cust string
		var created time.Time
		var assets, streams, devices, sev, alarms int
		var acked bool
		var attrs json.RawMessage
		if err := rows.Scan(&id, &name, &tz, &addr, &cust, &created, &assets, &streams, &devices, &sev, &alarms, &acked, &attrs); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		var alarm map[string]any
		if alarms > 0 {
			alarm = map[string]any{"severity": sevName[sev], "count": alarms, "acknowledged": acked}
		}
		out = append(out, map[string]any{"id": id, "name": name, "tz": tz, "address": addr, "customer_id": cust, "created_at": created, "assets": assets,
			"datastreams": streams, "devices": devices, "alarm": alarm, "location": locationOf(attrs)})
	}
	writeJSON(w, 200, out)
}

func (s *Server) createSite(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct{ Name, TZ, Address, Reason string }
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 128 {
		writeErr(w, 400, "name required (max 128)")
		return
	}
	if in.TZ == "" {
		in.TZ = "UTC"
	}
	if !tzRe.MatchString(in.TZ) {
		writeErr(w, 400, "invalid tz")
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.site (org_id, name, tz, address) VALUES ($1::uuid, $2, $3, NULLIF($4,'')) RETURNING id::text`, org, in.Name, in.TZ, in.Address).Scan(&id); err != nil {
		writeErr(w, 409, "site could not be created (duplicate name?)")
		return
	}
	s.audit(r, "site.create", "site", id, nil, map[string]any{"name": in.Name, "tz": in.TZ, "address": in.Address}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id, "name": in.Name, "tz": in.TZ, "address": in.Address})
}

func (s *Server) patchSite(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var in struct {
		Name, TZ, Address *string
		Location          *geoPoint `json:"location"`       // map position of the site (kept in its attributes)
		ClearLocation     bool      `json:"clear_location"` // forget the position
		Reason            string
	}
	if err := decodeJSON(r, &in, 4096); err != nil || !reasonOf(w, in.Reason) {
		if err != nil {
			writeErr(w, 400, err.Error())
		}
		return
	}
	if msg := in.Location.problem(); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	var oldName, oldTZ, oldAddr string
	var oldAttrs json.RawMessage
	if err := s.pool.QueryRow(r.Context(), `SELECT name, tz, COALESCE(address,''), attributes FROM core.site WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).Scan(&oldName, &oldTZ, &oldAddr, &oldAttrs); err != nil {
		writeErr(w, 404, "site not found")
		return
	}
	name, tz, addr := oldName, oldTZ, oldAddr
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	if in.TZ != nil {
		tz = *in.TZ
	}
	if in.Address != nil {
		addr = *in.Address
	}
	if name == "" || len(name) > 128 || !tzRe.MatchString(tz) {
		writeErr(w, 400, "invalid name or tz")
		return
	}
	attrs := withLocation(oldAttrs, in.Location, in.ClearLocation)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.site SET name = $3, tz = $4, address = NULLIF($5,''), attributes = $6::jsonb WHERE id = $1::uuid AND org_id = $2::uuid`, id, org, name, tz, addr, string(attrs)); err != nil {
		writeErr(w, 409, "update failed (duplicate name?)")
		return
	}
	before, after := map[string]any{"name": oldName, "tz": oldTZ, "address": oldAddr}, map[string]any{"name": name, "tz": tz, "address": addr}
	if in.Location != nil || in.ClearLocation {
		before["location"], after["location"] = locationOf(oldAttrs), locationOf(attrs)
	}
	s.audit(r, "site.update", "site", id, before, after, in.Reason)
	writeJSON(w, 200, map[string]any{"id": id, "name": name, "tz": tz, "address": addr, "location": locationOf(attrs)})
}

// ---- assets -------------------------------------------------------------------------------------

func (s *Server) createAsset(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		SiteID     string          `json:"site_id"`
		ParentID   string          `json:"parent_id"`
		Type       string          `json:"type"`
		Name       string          `json:"name"`
		Attributes json.RawMessage `json:"attributes"`
		Reason     string          `json:"reason"`
	}
	if err := decodeJSON(r, &in, 16384); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Name, in.Type = strings.TrimSpace(in.Name), strings.TrimSpace(strings.ToLower(in.Type))
	if in.Name == "" || len(in.Name) > 128 || in.Type == "" || len(in.Type) > 32 {
		writeErr(w, 400, "name and type required")
		return
	}
	if len(in.Attributes) == 0 {
		in.Attributes = json.RawMessage(`{}`)
	}
	if !json.Valid(in.Attributes) {
		writeErr(w, 400, "attributes must be a JSON object")
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var id string
	err := s.pool.QueryRow(r.Context(), `INSERT INTO core.asset (org_id, site_id, parent_id, type, name, attributes)
		SELECT $1::uuid, s.id, NULLIF($3,'')::uuid, $4, $5, $6::jsonb FROM core.site s WHERE s.id = $2::uuid AND s.org_id = $1::uuid RETURNING id::text`,
		org, in.SiteID, in.ParentID, in.Type, in.Name, string(in.Attributes)).Scan(&id)
	if err != nil {
		writeErr(w, 409, "asset could not be created (site not found or duplicate name)")
		return
	}
	s.audit(r, "asset.create", "asset", id, nil, map[string]any{"site_id": in.SiteID, "type": in.Type, "name": in.Name, "attributes": in.Attributes}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id, "site_id": in.SiteID, "type": in.Type, "name": in.Name})
}

func (s *Server) patchAsset(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var in struct {
		Name          *string         `json:"name"`
		Type          *string         `json:"type"`
		Attributes    json.RawMessage `json:"attributes"`
		ParentID      *string         `json:"parent_id"`      // move under another asset of the same site; "" = top level
		Location      *geoPoint       `json:"location"`       // map position (kept in the attributes as lat, lon)
		ClearLocation bool            `json:"clear_location"` // forget the position
		Reason        string          `json:"reason"`
	}
	if err := decodeJSON(r, &in, 16384); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if msg := in.Location.problem(); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if !uuidRe.MatchString(id) {
		writeErr(w, 404, "asset not found")
		return
	}
	var oldName, oldType, oldParent, siteID string
	var oldAttrs json.RawMessage
	if err := s.pool.QueryRow(r.Context(), `SELECT name, type, attributes, COALESCE(parent_id::text, ''), site_id::text FROM core.asset WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).Scan(&oldName, &oldType, &oldAttrs, &oldParent, &siteID); err != nil {
		writeErr(w, 404, "asset not found")
		return
	}
	name, typ, attrs, parent := oldName, oldType, oldAttrs, oldParent
	if in.ParentID != nil && *in.ParentID != oldParent {
		parent = *in.ParentID
		if parent != "" {
			if parent == id || !uuidRe.MatchString(parent) {
				writeErr(w, 400, "an asset cannot be its own parent")
				return
			}
			// the new parent must be on the same site and must not lie below the asset (no cycles)
			var sameSite, below bool
			_ = s.pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM core.asset WHERE id = $1::uuid AND org_id = $2::uuid AND site_id = $3::uuid)`, parent, org, siteID).Scan(&sameSite)
			if !sameSite {
				writeErr(w, 400, "the parent must be an asset of the same site")
				return
			}
			ids, err := s.assetIDs(r.Context(), org, id, true)
			if err != nil {
				writeErr(w, 500, "query failed")
				return
			}
			for _, x := range ids {
				below = below || x == parent
			}
			if below {
				writeErr(w, 400, "the parent cannot be the asset itself or one of the assets below it")
				return
			}
		}
	}
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	if in.Type != nil {
		typ = strings.TrimSpace(strings.ToLower(*in.Type))
	}
	if len(in.Attributes) > 0 {
		if !json.Valid(in.Attributes) {
			writeErr(w, 400, "attributes must be JSON")
			return
		}
		var obj map[string]any
		if json.Unmarshal(in.Attributes, &obj) != nil {
			writeErr(w, 400, "attributes must be a JSON object")
			return
		}
		attrs = in.Attributes
	}
	attrs = withLocation(attrs, in.Location, in.ClearLocation)
	if name == "" || len(name) > 128 || typ == "" || len(typ) > 32 {
		writeErr(w, 400, "name and type required")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.asset SET name = $3, type = $4, attributes = $5::jsonb, parent_id = NULLIF($6, '')::uuid WHERE id = $1::uuid AND org_id = $2::uuid`, id, org, name, typ, string(attrs), parent); err != nil {
		writeErr(w, 409, "update failed (duplicate name?)")
		return
	}
	before, after := map[string]any{"name": oldName, "type": oldType, "attributes": oldAttrs}, map[string]any{"name": name, "type": typ, "attributes": attrs}
	if parent != oldParent {
		before["parent_id"], after["parent_id"] = oldParent, parent
	}
	s.audit(r, "asset.update", "asset", id, before, after, in.Reason)
	writeJSON(w, 200, map[string]any{"id": id, "name": name, "type": typ, "attributes": attrs, "parent_id": parent})
}

// ---- datastreams --------------------------------------------------------------------------------

func (s *Server) createDatastream(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	var in struct {
		AssetID          string   `json:"asset_id"`
		Key              string   `json:"key"`
		Quantity         string   `json:"quantity"`
		Unit             string   `json:"unit"`
		Kind             string   `json:"kind"`
		ExpectedInterval string   `json:"expected_interval"`
		Accuracy         *float64 `json:"accuracy"`
		Range            *dsRange `json:"range"`
		Reason           string   `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if msg := in.Range.problem(); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if in.Range == nil {
		in.Range = &dsRange{}
	}
	in.Key = strings.TrimSpace(strings.ToLower(in.Key))
	if !dsKeyRe.MatchString(in.Key) {
		writeErr(w, 400, "key must match ^[a-z][a-z0-9_]{0,31}$ (it is the key the device publishes)")
		return
	}
	in.Quantity = strings.TrimSpace(strings.ToLower(in.Quantity))
	if in.Quantity == "" || len(in.Quantity) > 64 || len(in.Unit) > 16 {
		writeErr(w, 400, "quantity required (e.g. temperature), unit max 16 chars")
		return
	}
	switch in.Kind {
	case "gauge", "counter", "state", "event":
	case "":
		in.Kind = "gauge"
	default:
		writeErr(w, 400, "kind must be gauge, counter, state or event")
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var id string
	err := s.pool.QueryRow(r.Context(), `INSERT INTO core.datastream (org_id, asset_id, key, quantity, unit, kind, expected_interval, accuracy, attributes)
		SELECT $1::uuid, a.id, $3, $4, $5, $6, NULLIF($7,'')::interval, $8, jsonb_strip_nulls(jsonb_build_object('range_min', $9::float8, 'range_max', $10::float8))
		FROM core.asset a WHERE a.id = $2::uuid AND a.org_id = $1::uuid RETURNING id::text`,
		org, in.AssetID, in.Key, in.Quantity, in.Unit, in.Kind, in.ExpectedInterval, in.Accuracy, in.Range.Min, in.Range.Max).Scan(&id)
	if err != nil {
		s.log.Warn("datastream create", "err", err)
		writeErr(w, 409, "datastream could not be created (asset not found, duplicate key or invalid interval)")
		return
	}
	s.audit(r, "datastream.create", "datastream", id, nil, map[string]any{"asset_id": in.AssetID, "key": in.Key, "quantity": in.Quantity, "unit": in.Unit, "kind": in.Kind, "expected_interval": in.ExpectedInterval, "accuracy": in.Accuracy, "range": in.Range}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id, "asset_id": in.AssetID, "key": in.Key, "quantity": in.Quantity, "unit": in.Unit, "kind": in.Kind, "expected_interval": in.ExpectedInterval, "range_min": in.Range.Min, "range_max": in.Range.Max})
}

func (s *Server) patchDatastream(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var in struct {
		Quantity         *string  `json:"quantity"`
		Unit             *string  `json:"unit"`
		ExpectedInterval *string  `json:"expected_interval"`
		Accuracy         *float64 `json:"accuracy"`
		Range            *dsRange `json:"range"` // physical range; a null bound removes it
		Reason           string   `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if msg := in.Range.problem(); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	var q, u, iv string
	var acc *float64
	var rng dsRange
	if err := s.pool.QueryRow(r.Context(), `SELECT quantity, unit, COALESCE(expected_interval::text,''), accuracy::float8,
		CASE WHEN jsonb_typeof(attributes->'range_min') = 'number' THEN (attributes->>'range_min')::float8 END,
		CASE WHEN jsonb_typeof(attributes->'range_max') = 'number' THEN (attributes->>'range_max')::float8 END
		FROM core.datastream WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).Scan(&q, &u, &iv, &acc, &rng.Min, &rng.Max); err != nil {
		writeErr(w, 404, "datastream not found")
		return
	}
	old := map[string]any{"quantity": q, "unit": u, "expected_interval": iv, "accuracy": acc}
	if in.Range != nil {
		old["range"] = rng
	}
	if in.Quantity != nil {
		q = strings.TrimSpace(strings.ToLower(*in.Quantity))
	}
	if in.Unit != nil {
		u = *in.Unit
	}
	if in.ExpectedInterval != nil {
		iv = *in.ExpectedInterval
	}
	if in.Accuracy != nil {
		acc = in.Accuracy
	}
	if q == "" || len(u) > 16 {
		writeErr(w, 400, "invalid quantity or unit")
		return
	}
	if in.Range != nil {
		rng = *in.Range
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.datastream SET quantity = $3, unit = $4, expected_interval = NULLIF($5,'')::interval, accuracy = $6,
		attributes = CASE WHEN $7 THEN (attributes - 'range_min' - 'range_max') || jsonb_strip_nulls(jsonb_build_object('range_min', $8::float8, 'range_max', $9::float8)) ELSE attributes END
		WHERE id = $1::uuid AND org_id = $2::uuid`, id, org, q, u, iv, acc, in.Range != nil, rng.Min, rng.Max); err != nil {
		writeErr(w, 400, "update failed (invalid interval?)")
		return
	}
	if in.Range != nil && s.ing != nil {
		s.ing.InvalidateAll()
	}
	after := map[string]any{"quantity": q, "unit": u, "expected_interval": iv, "accuracy": acc}
	if in.Range != nil {
		after["range"] = rng
	}
	s.audit(r, "datastream.update", "datastream", id, old, after, in.Reason)
	writeJSON(w, 200, map[string]any{"id": id, "quantity": q, "unit": u, "expected_interval": iv, "accuracy": acc, "range_min": rng.Min, "range_max": rng.Max})
}

// geoPoint is a map position set by an operator.
type geoPoint struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

func (g *geoPoint) problem() string {
	if g == nil {
		return ""
	}
	if math.IsNaN(g.Lat) || math.IsNaN(g.Lon) || g.Lat < -90 || g.Lat > 90 || g.Lon < -180 || g.Lon > 180 {
		return "location out of range"
	}
	return ""
}

// withLocation sets (or removes) lat and lon in a JSON object of attributes.
func withLocation(attrs json.RawMessage, loc *geoPoint, clear bool) json.RawMessage {
	if loc == nil && !clear {
		return attrs
	}
	m := map[string]any{}
	_ = json.Unmarshal(attrs, &m)
	if m == nil {
		m = map[string]any{}
	}
	delete(m, "lat")
	delete(m, "lon")
	if loc != nil && !clear {
		m["lat"], m["lon"] = loc.Lat, loc.Lon
	}
	b, _ := json.Marshal(m)
	return b
}

// dsRange is the physical (measuring) range of a datastream, kept in its attributes (range_min, range_max): a value
// outside it is stored with quality sensor_fault (internal/ingest).
type dsRange struct {
	Min *float64 `json:"min"`
	Max *float64 `json:"max"`
}

func (g *dsRange) problem() string {
	if g == nil {
		return ""
	}
	for _, v := range []*float64{g.Min, g.Max} {
		if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0)) {
			return "range: bounds must be finite numbers"
		}
	}
	if g.Min != nil && g.Max != nil && *g.Min >= *g.Max {
		return "range: min must be below max"
	}
	return ""
}

// GET /api/v1/datastreams/{id}: datastream with asset, site, current device, current alarm rules and last observation.
func (s *Server) getDatastream(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var ds struct {
		ID, Key, Quantity, Unit, Kind, Interval, AssetID, Asset, AssetType, SiteID, Site string
		Accuracy, RangeMin, RangeMax                                                     *float64
		Created                                                                          time.Time
	}
	err := s.pool.QueryRow(r.Context(), `SELECT d.id::text, d.key, d.quantity, d.unit, d.kind, COALESCE(d.expected_interval::text,''), d.accuracy::float8, d.created_at,
		a.id::text, a.name, a.type, st.id::text, st.name,
		CASE WHEN jsonb_typeof(d.attributes->'range_min') = 'number' THEN (d.attributes->>'range_min')::float8 END,
		CASE WHEN jsonb_typeof(d.attributes->'range_max') = 'number' THEN (d.attributes->>'range_max')::float8 END
		FROM core.datastream d JOIN core.asset a ON a.id = d.asset_id JOIN core.site st ON st.id = a.site_id
		WHERE d.id = $1::uuid AND ($2 = '' OR d.org_id = $2::uuid)`, id, orgOf(p)).
		Scan(&ds.ID, &ds.Key, &ds.Quantity, &ds.Unit, &ds.Kind, &ds.Interval, &ds.Accuracy, &ds.Created, &ds.AssetID, &ds.Asset, &ds.AssetType, &ds.SiteID, &ds.Site, &ds.RangeMin, &ds.RangeMax)
	if err != nil {
		writeErr(w, 404, "datastream not found")
		return
	}
	var device map[string]any
	var devID, devExt, devName string
	var since time.Time
	if err := s.pool.QueryRow(r.Context(), `SELECT dv.id::text, dv.external_id, dv.name, a.valid_from FROM core.assignment a JOIN core.device dv ON dv.id = a.from_id
		WHERE a.kind = 'device_datastream' AND a.to_id = $1::uuid AND a.valid_to IS NULL ORDER BY a.valid_from DESC LIMIT 1`, id).Scan(&devID, &devExt, &devName, &since); err == nil {
		device = map[string]any{"id": devID, "external_id": devExt, "name": devName, "since": since}
	}
	rules := s.alarmRulesFor(r, orgOf(p), id, "")
	var last map[string]any
	var mAt time.Time
	var val *float64
	var quality string
	if err := s.pool.QueryRow(r.Context(), `SELECT measured_at, value::float8, quality FROM core.observation WHERE datastream_id = $1::uuid ORDER BY measured_at DESC LIMIT 1`, id).Scan(&mAt, &val, &quality); err == nil {
		last = map[string]any{"measured_at": mAt, "value": val, "quality": quality}
	}
	ruleRefs := []map[string]any{}
	if rr, err := s.pool.Query(r.Context(), `SELECT id::text, name, status, enabled,
			EXISTS (SELECT 1 FROM jsonb_array_elements(COALESCE(spec->'inputs','[]'::jsonb)) i WHERE i->>'datastream_id' = $1),
			EXISTS (SELECT 1 FROM jsonb_array_elements(COALESCE(spec->'actions','[]'::jsonb) || COALESCE(spec->'clear_actions','[]'::jsonb)) a WHERE a->>'type' = 'derive' AND a->>'datastream_id' = $1)
		FROM core.rule WHERE revoked_at IS NULL AND ($2 = '' OR org_id = $2::uuid)
			AND (spec::text LIKE '%' || $1 || '%') ORDER BY name`, id, orgOf(p)); err == nil {
		for rr.Next() {
			var rid, name, status string
			var enabled, input, derive bool
			if rr.Scan(&rid, &name, &status, &enabled, &input, &derive) == nil && (input || derive) {
				role := "input"
				if derive {
					role = "derive"
				}
				ruleRefs = append(ruleRefs, map[string]any{"id": rid, "name": name, "status": status, "enabled": enabled, "role": role})
			}
		}
		rr.Close()
	}
	writeJSON(w, 200, map[string]any{
		"datastream": map[string]any{"id": ds.ID, "key": ds.Key, "quantity": ds.Quantity, "unit": ds.Unit, "kind": ds.Kind, "expected_interval": ds.Interval, "accuracy": ds.Accuracy, "range_min": ds.RangeMin, "range_max": ds.RangeMax, "created_at": ds.Created},
		"asset":      map[string]any{"id": ds.AssetID, "name": ds.Asset, "type": ds.AssetType}, "site": map[string]any{"id": ds.SiteID, "name": ds.Site},
		"device": device, "rules": ruleRefs, "alarm_rules": rules, "last_observation": last,
	})
}

// ---- alarm rules --------------------------------------------------------------------------------

func (s *Server) alarmRulesFor(r *http.Request, org, dsID, assetID string) []map[string]any {
	rows, err := s.pool.Query(r.Context(), `SELECT ar.id::text, ar.datastream_id::text, d.key, a.name, a.id::text, ar.type, ar.warning::float8, ar.action::float8, EXTRACT(EPOCH FROM ar.delay)::float8, ar.hysteresis::float8, ar.enabled, ar.version, ar.valid_from, ar.reason, ar.escalation_policy_id::text
		FROM core.alarm_rule ar JOIN core.datastream d ON d.id = ar.datastream_id JOIN core.asset a ON a.id = d.asset_id
		WHERE ar.valid_to IS NULL AND ($1 = '' OR ar.org_id = $1::uuid) AND ($2 = '' OR ar.datastream_id = $2::uuid)
		  AND ($3 = '' OR d.asset_id = $3::uuid) ORDER BY a.name, d.key, ar.type`, org, dsID, assetID)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, ds, key, asset, assetID, typ, reason string
		var warning, action *float64
		var delay, hyst float64
		var enabled bool
		var version int
		var from time.Time
		var policyID *string
		if err := rows.Scan(&id, &ds, &key, &asset, &assetID, &typ, &warning, &action, &delay, &hyst, &enabled, &version, &from, &reason, &policyID); err != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "datastream_id": ds, "key": key, "asset": asset, "asset_id": assetID, "type": typ, "warning": warning, "action": action, "delay_seconds": delay, "hysteresis": hyst, "enabled": enabled, "version": version, "valid_from": from, "reason": reason, "escalation_policy_id": policyID})
	}
	return out
}

func (s *Server) listAlarmRules(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	q := r.URL.Query()
	for _, k := range []string{"datastream_id", "asset_id"} {
		if v := q.Get(k); v != "" && !uuidRe.MatchString(v) {
			writeErr(w, 400, k+" must be a uuid")
			return
		}
	}
	writeJSON(w, 200, s.alarmRulesFor(r, orgOf(p), q.Get("datastream_id"), q.Get("asset_id")))
}

// alarmRuleProblem checks that a rule of the type can be evaluated as given (see internal/alarm): a type the engine
// cannot evaluate, or limits it would ignore, are refused rather than stored silently.
func (s *Server) alarmRuleProblem(r *http.Request, org, dsID, typ string, warning, action *float64, hysteresis float64) string {
	switch typ {
	case "high", "low":
		if warning == nil && action == nil {
			return "warning or action limit required"
		}
	case "low_battery":
		if warning == nil && action == nil {
			return "low_battery: a warning or action limit (battery level in the datastream's unit) required"
		}
	case "power_loss", "door_open":
		if warning != nil || action != nil || hysteresis != 0 {
			if typ == "power_loss" {
				return "power_loss uses no limits or hysteresis: a value of 0 (false) means the power is lost"
			}
			return "door_open uses no limits or hysteresis: a value other than 0 (true) means open; the delay is how long it may stay open"
		}
	case "comm_loss":
		if hysteresis != 0 {
			return "comm_loss uses no hysteresis"
		}
		if (warning != nil && *warning <= 0) || (action != nil && *action <= 0) {
			return "comm_loss: the limits are seconds without a new value (more than 0)"
		}
		if warning == nil && action == nil {
			var has bool
			_ = s.pool.QueryRow(r.Context(), `SELECT expected_interval IS NOT NULL FROM core.datastream WHERE id = $1::uuid AND org_id = $2::uuid`, dsID, org).Scan(&has)
			if !has {
				return "comm_loss: give a limit in seconds without a new value (warning or action), or an expected interval of the datastream"
			}
		}
	case "sensor_fault":
		if warning != nil || action != nil || hysteresis != 0 {
			return "sensor_fault uses no limits or hysteresis: a value with quality sensor_fault (flagged by the device, or outside the datastream's physical range) raises it; the delay is how long the fault may last"
		}
	default:
		return "type must be high, low, low_battery, power_loss, door_open, comm_loss or sensor_fault"
	}
	return ""
}

// POST /api/v1/alarm-rules: closes the current rule of the same datastream+type and inserts the next version.
func (s *Server) createAlarmRule(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	var in struct {
		DatastreamID string   `json:"datastream_id"`
		Type         string   `json:"type"`
		Warning      *float64 `json:"warning"`
		Action       *float64 `json:"action"`
		DelaySeconds float64  `json:"delay_seconds"`
		Hysteresis   float64  `json:"hysteresis"`
		PolicyID     string   `json:"escalation_policy_id"`
		Reason       string   `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if msg := s.alarmRuleProblem(r, org, in.DatastreamID, in.Type, in.Warning, in.Action, in.Hysteresis); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if in.DelaySeconds < 0 || in.DelaySeconds > 86400*7 || in.Hysteresis < 0 {
		writeErr(w, 400, "invalid delay or hysteresis")
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var policyID *string
	if in.PolicyID != "" {
		var ok bool
		if err := s.pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM core.escalation_policy WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL)`, in.PolicyID, org).Scan(&ok); err != nil || !ok {
			writeErr(w, 400, "unknown escalation policy")
			return
		}
		policyID = &in.PolicyID
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, 500, "tx failed")
		return
	}
	defer tx.Rollback(r.Context())
	var exists bool
	if err := tx.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM core.datastream WHERE id = $1::uuid AND org_id = $2::uuid)`, in.DatastreamID, org).Scan(&exists); err != nil || !exists {
		writeErr(w, 404, "datastream not found")
		return
	}
	var prevID string
	var version int
	err = tx.QueryRow(r.Context(), `SELECT id::text, version FROM core.alarm_rule WHERE datastream_id = $1::uuid AND type = $2 AND valid_to IS NULL ORDER BY valid_from DESC LIMIT 1`, in.DatastreamID, in.Type).Scan(&prevID, &version)
	if err != nil && err != pgx.ErrNoRows {
		writeErr(w, 500, "query failed")
		return
	}
	if prevID != "" {
		if _, err := tx.Exec(r.Context(), `UPDATE core.alarm_rule SET valid_to = now() WHERE id = $1::uuid`, prevID); err != nil {
			s.log.Error("close alarm rule", "err", err)
			writeErr(w, 500, "could not close previous version")
			return
		}
	}
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO core.alarm_rule (org_id, datastream_id, type, warning, action, delay, hysteresis, version, changed_by, reason, escalation_policy_id)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, make_interval(secs => $6), $7, $8, NULLIF($9,'')::uuid, $10, $11::uuid) RETURNING id::text`,
		org, in.DatastreamID, in.Type, in.Warning, in.Action, in.DelaySeconds, in.Hysteresis, version+1, p.UserID, in.Reason, policyID).Scan(&id)
	if err != nil {
		s.log.Error("insert alarm rule", "err", err)
		writeErr(w, 500, "insert failed")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, 500, "commit failed")
		return
	}
	s.audit(r, "alarm_rule.create", "alarm_rule", id, map[string]any{"previous": prevID}, map[string]any{"datastream_id": in.DatastreamID, "type": in.Type, "warning": in.Warning, "action": in.Action, "delay_seconds": in.DelaySeconds, "hysteresis": in.Hysteresis, "version": version + 1}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id, "version": version + 1, "previous": prevID})
}

// DELETE /api/v1/alarm-rules/{id}?reason=: closes the current version (the rule stops applying).
func (s *Server) closeAlarmRule(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	tag, err := s.pool.Exec(r.Context(), `UPDATE core.alarm_rule SET valid_to = now() WHERE id = $1::uuid AND org_id = $2::uuid AND valid_to IS NULL`, id, org)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, 404, "current alarm rule not found")
		return
	}
	s.audit(r, "alarm_rule.close", "alarm_rule", id, nil, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---- devices: detail, rename, assignments ----------------------------------------------------------

func (s *Server) getDevice(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var dev struct {
		ID, Ext, Name, Transport string
		Status, Location         json.RawMessage
		Created                  time.Time
		Revoked                  *time.Time
	}
	err := s.pool.QueryRow(r.Context(), `SELECT id::text, external_id, name, transport, status, created_at, revoked_at, location FROM core.device WHERE id = $1::uuid AND ($2 = '' OR org_id = $2::uuid)`, id, orgOf(p)).
		Scan(&dev.ID, &dev.Ext, &dev.Name, &dev.Transport, &dev.Status, &dev.Created, &dev.Revoked, &dev.Location)
	if err != nil {
		writeErr(w, 404, "device not found")
		return
	}
	creds := []map[string]any{}
	if rows, err := s.pool.Query(r.Context(), `SELECT kind, valid_from, valid_to, revoked_at, used_at FROM core.device_credential WHERE device_id = $1::uuid ORDER BY valid_from DESC LIMIT 20`, id); err == nil {
		for rows.Next() {
			var kind string
			var from time.Time
			var to, revoked, used *time.Time
			if rows.Scan(&kind, &from, &to, &revoked, &used) == nil {
				creds = append(creds, map[string]any{"kind": kind, "valid_from": from, "valid_to": to, "revoked_at": revoked, "used_at": used})
			}
		}
		rows.Close()
	}
	assigned := []map[string]any{}
	if rows, err := s.pool.Query(r.Context(), `SELECT a.id::text, d.id::text, d.key, d.unit, d.quantity, ast.name, ast.id::text, a.valid_from FROM core.assignment a JOIN core.datastream d ON d.id = a.to_id JOIN core.asset ast ON ast.id = d.asset_id
		WHERE a.kind = 'device_datastream' AND a.from_id = $1::uuid AND a.valid_to IS NULL ORDER BY ast.name, d.key`, id); err == nil {
		for rows.Next() {
			var aid, dsID, key, unit, quantity, asset, assetID string
			var since time.Time
			if rows.Scan(&aid, &dsID, &key, &unit, &quantity, &asset, &assetID, &since) == nil {
				assigned = append(assigned, map[string]any{"assignment_id": aid, "datastream_id": dsID, "key": key, "unit": unit, "quantity": quantity, "asset": asset, "asset_id": assetID, "since": since})
			}
		}
		rows.Close()
	}
	var lastMsg *time.Time
	var n24 int
	_ = s.pool.QueryRow(r.Context(), `SELECT max(received_at), count(*) FILTER (WHERE received_at > now() - interval '24 hours') FROM core.raw_message WHERE device_id = $1::uuid AND received_at > now() - interval '30 days'`, id).Scan(&lastMsg, &n24)
	writeJSON(w, 200, map[string]any{
		"device":      map[string]any{"id": dev.ID, "external_id": dev.Ext, "name": dev.Name, "transport": dev.Transport, "status": dev.Status, "created_at": dev.Created, "revoked_at": dev.Revoked, "location": dev.Location},
		"connected":   s.br != nil && s.br.IsConnected(dev.ID),
		"credentials": creds, "datastreams": assigned, "last_message_at": lastMsg, "messages_24h": n24,
	})
}

func (s *Server) patchDevice(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var in struct {
		Name          string                      `json:"name"`
		Reason        string                      `json:"reason"`
		Location      *struct{ Lat, Lon float64 } `json:"location"`       // fixed position set by the operator
		ClearLocation bool                        `json:"clear_location"` // forget the fixed position (reported ones apply again)
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !reasonOf(w, in.Reason) {
		return
	}
	if len(in.Name) > 128 {
		writeErr(w, 400, "name max 128")
		return
	}
	if in.Name == "" && in.Location == nil && !in.ClearLocation {
		writeErr(w, 400, "nothing to change (name, location or clear_location)")
		return
	}
	if in.Location != nil && (in.Location.Lat < -90 || in.Location.Lat > 90 || in.Location.Lon < -180 || in.Location.Lon > 180) {
		writeErr(w, 400, "location out of range")
		return
	}
	var oldName string
	var oldLoc json.RawMessage
	if err := s.pool.QueryRow(r.Context(), `SELECT name, location FROM core.device WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).Scan(&oldName, &oldLoc); err != nil {
		writeErr(w, 404, "device not found")
		return
	}
	newName := oldName
	if in.Name != "" {
		newName = in.Name
	}
	var newLoc any = "keep"
	if in.ClearLocation {
		newLoc = nil
	} else if in.Location != nil {
		newLoc = map[string]any{"lat": in.Location.Lat, "lon": in.Location.Lon, "source": "fixed", "updated_at": time.Now().UTC()}
	}
	if newLoc == "keep" {
		if _, err := s.pool.Exec(r.Context(), `UPDATE core.device SET name = $3 WHERE id = $1::uuid AND org_id = $2::uuid`, id, org, newName); err != nil {
			writeErr(w, 500, "update failed")
			return
		}
	} else {
		var locJSON *string
		if newLoc != nil {
			b, _ := json.Marshal(newLoc)
			str := string(b)
			locJSON = &str
		}
		if _, err := s.pool.Exec(r.Context(), `UPDATE core.device SET name = $3, location = $4::jsonb WHERE id = $1::uuid AND org_id = $2::uuid`, id, org, newName, locJSON); err != nil {
			writeErr(w, 500, "update failed")
			return
		}
	}
	after := map[string]any{"name": newName}
	if newLoc != "keep" {
		after["location"] = newLoc
	}
	s.audit(r, "device.update", "device", id, map[string]any{"name": oldName, "location": oldLoc}, after, in.Reason)
	writeJSON(w, 200, map[string]any{"id": id, "name": newName, "location": newLoc})
}

// POST /api/v1/devices/{id}/datastreams {datastream_id, reason}: the datastream's previous source (any device) is closed.
func (s *Server) assignDatastream(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	devID := r.PathValue("id")
	var in struct {
		DatastreamID string `json:"datastream_id"`
		Reason       string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, 500, "tx failed")
		return
	}
	defer tx.Rollback(r.Context())
	var okDev, okDS bool
	_ = tx.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM core.device WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL)`, devID, org).Scan(&okDev)
	_ = tx.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM core.datastream WHERE id = $1::uuid AND org_id = $2::uuid)`, in.DatastreamID, org).Scan(&okDS)
	if !okDev || !okDS {
		writeErr(w, 404, "device or datastream not found")
		return
	}
	var closed int64
	if tag, err := tx.Exec(r.Context(), `UPDATE core.assignment SET valid_to = now() WHERE kind = 'device_datastream' AND to_id = $1::uuid AND valid_to IS NULL`, in.DatastreamID); err != nil {
		s.log.Error("close assignment", "err", err)
		writeErr(w, 500, "could not close previous assignment")
		return
	} else {
		closed = tag.RowsAffected()
	}
	var id string
	if err := tx.QueryRow(r.Context(), `INSERT INTO core.assignment (org_id, kind, from_id, to_id, changed_by, reason) VALUES ($1::uuid, 'device_datastream', $2::uuid, $3::uuid, NULLIF($4,'')::uuid, $5) RETURNING id::text`,
		org, devID, in.DatastreamID, p.UserID, in.Reason).Scan(&id); err != nil {
		writeErr(w, 500, "insert failed")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, 500, "commit failed")
		return
	}
	if s.ing != nil {
		s.ing.InvalidateDevice(devID)
	}
	s.audit(r, "assignment.create", "assignment", id, map[string]any{"closed_previous": closed}, map[string]any{"device_id": devID, "datastream_id": in.DatastreamID}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id, "device_id": devID, "datastream_id": in.DatastreamID, "closed_previous": closed})
}

// DELETE /api/v1/devices/{id}/datastreams/{ds}?reason=
func (s *Server) unassignDatastream(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	devID, dsID := r.PathValue("id"), r.PathValue("ds")
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	var asgID string
	if err := s.pool.QueryRow(r.Context(), `UPDATE core.assignment SET valid_to = now() WHERE kind = 'device_datastream' AND org_id = $1::uuid AND from_id = $2::uuid AND to_id = $3::uuid AND valid_to IS NULL RETURNING id::text`, org, devID, dsID).Scan(&asgID); err != nil {
		writeErr(w, 404, "current assignment not found")
		return
	}
	if s.ing != nil {
		s.ing.InvalidateDevice(devID)
	}
	s.audit(r, "assignment.close", "assignment", asgID, map[string]any{"device_id": devID, "datastream_id": dsID}, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

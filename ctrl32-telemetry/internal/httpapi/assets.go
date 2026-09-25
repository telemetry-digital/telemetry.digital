package httpapi

import (
	"encoding/json"
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
//	GET    /api/v1/assets?site_id=                    POST /api/v1/assets                PATCH /api/v1/assets/{id}
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
	rows, err := s.pool.Query(r.Context(), `SELECT s.id::text, s.name, s.tz, COALESCE(s.address,''), COALESCE(s.customer_id::text,''), s.created_at,
		(SELECT count(*) FROM core.asset a WHERE a.site_id = s.id) FROM core.site s WHERE ($1 = '' OR s.org_id = $1::uuid) ORDER BY s.name`, orgOf(p))
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, tz, addr, cust string
		var created time.Time
		var assets int
		if err := rows.Scan(&id, &name, &tz, &addr, &cust, &created, &assets); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, map[string]any{"id": id, "name": name, "tz": tz, "address": addr, "customer_id": cust, "created_at": created, "assets": assets})
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
		Reason            string
	}
	if err := decodeJSON(r, &in, 4096); err != nil || !reasonOf(w, in.Reason) {
		if err != nil {
			writeErr(w, 400, err.Error())
		}
		return
	}
	var oldName, oldTZ, oldAddr string
	if err := s.pool.QueryRow(r.Context(), `SELECT name, tz, COALESCE(address,'') FROM core.site WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).Scan(&oldName, &oldTZ, &oldAddr); err != nil {
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
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.site SET name = $3, tz = $4, address = NULLIF($5,'') WHERE id = $1::uuid AND org_id = $2::uuid`, id, org, name, tz, addr); err != nil {
		writeErr(w, 409, "update failed (duplicate name?)")
		return
	}
	s.audit(r, "site.update", "site", id, map[string]any{"name": oldName, "tz": oldTZ, "address": oldAddr}, map[string]any{"name": name, "tz": tz, "address": addr}, in.Reason)
	writeJSON(w, 200, map[string]any{"id": id, "name": name, "tz": tz, "address": addr})
}

// ---- assets -------------------------------------------------------------------------------------

func (s *Server) listAssets(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	site := r.URL.Query().Get("site_id")
	rows, err := s.pool.Query(r.Context(), `SELECT a.id::text, a.site_id::text, COALESCE(a.parent_id::text,''), a.type, a.name, a.attributes, a.created_at, st.name,
		(SELECT count(*) FROM core.datastream d WHERE d.asset_id = a.id)
		FROM core.asset a JOIN core.site st ON st.id = a.site_id WHERE ($1 = '' OR a.org_id = $1::uuid) AND ($2 = '' OR a.site_id = $2::uuid) ORDER BY st.name, a.name`, orgOf(p), site)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, siteID, parent, typ, name, siteName string
		var attrs json.RawMessage
		var created time.Time
		var n int
		if err := rows.Scan(&id, &siteID, &parent, &typ, &name, &attrs, &created, &siteName, &n); err != nil {
			writeErr(w, 500, "scan failed")
			return
		}
		out = append(out, map[string]any{"id": id, "site_id": siteID, "site": siteName, "parent_id": parent, "type": typ, "name": name, "attributes": attrs, "created_at": created, "datastreams": n})
	}
	writeJSON(w, 200, out)
}

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
		Name       *string         `json:"name"`
		Type       *string         `json:"type"`
		Attributes json.RawMessage `json:"attributes"`
		Reason     string          `json:"reason"`
	}
	if err := decodeJSON(r, &in, 16384); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var oldName, oldType string
	var oldAttrs json.RawMessage
	if err := s.pool.QueryRow(r.Context(), `SELECT name, type, attributes FROM core.asset WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).Scan(&oldName, &oldType, &oldAttrs); err != nil {
		writeErr(w, 404, "asset not found")
		return
	}
	name, typ, attrs := oldName, oldType, oldAttrs
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
		attrs = in.Attributes
	}
	if name == "" || typ == "" {
		writeErr(w, 400, "name and type required")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.asset SET name = $3, type = $4, attributes = $5::jsonb WHERE id = $1::uuid AND org_id = $2::uuid`, id, org, name, typ, string(attrs)); err != nil {
		writeErr(w, 409, "update failed (duplicate name?)")
		return
	}
	s.audit(r, "asset.update", "asset", id, map[string]any{"name": oldName, "type": oldType, "attributes": oldAttrs}, map[string]any{"name": name, "type": typ, "attributes": attrs}, in.Reason)
	writeJSON(w, 200, map[string]any{"id": id, "name": name, "type": typ, "attributes": attrs})
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
		Reason           string   `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
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
	err := s.pool.QueryRow(r.Context(), `INSERT INTO core.datastream (org_id, asset_id, key, quantity, unit, kind, expected_interval, accuracy)
		SELECT $1::uuid, a.id, $3, $4, $5, $6, NULLIF($7,'')::interval, $8 FROM core.asset a WHERE a.id = $2::uuid AND a.org_id = $1::uuid RETURNING id::text`,
		org, in.AssetID, in.Key, in.Quantity, in.Unit, in.Kind, in.ExpectedInterval, in.Accuracy).Scan(&id)
	if err != nil {
		s.log.Warn("datastream create", "err", err)
		writeErr(w, 409, "datastream could not be created (asset not found, duplicate key or invalid interval)")
		return
	}
	s.audit(r, "datastream.create", "datastream", id, nil, map[string]any{"asset_id": in.AssetID, "key": in.Key, "quantity": in.Quantity, "unit": in.Unit, "kind": in.Kind, "expected_interval": in.ExpectedInterval, "accuracy": in.Accuracy}, in.Reason)
	writeJSON(w, 201, map[string]any{"id": id, "asset_id": in.AssetID, "key": in.Key, "quantity": in.Quantity, "unit": in.Unit, "kind": in.Kind, "expected_interval": in.ExpectedInterval})
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
		Reason           string   `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	var q, u, iv string
	var acc *float64
	if err := s.pool.QueryRow(r.Context(), `SELECT quantity, unit, COALESCE(expected_interval::text,''), accuracy::float8 FROM core.datastream WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).Scan(&q, &u, &iv, &acc); err != nil {
		writeErr(w, 404, "datastream not found")
		return
	}
	old := map[string]any{"quantity": q, "unit": u, "expected_interval": iv, "accuracy": acc}
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
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.datastream SET quantity = $3, unit = $4, expected_interval = NULLIF($5,'')::interval, accuracy = $6 WHERE id = $1::uuid AND org_id = $2::uuid`, id, org, q, u, iv, acc); err != nil {
		writeErr(w, 400, "update failed (invalid interval?)")
		return
	}
	s.audit(r, "datastream.update", "datastream", id, old, map[string]any{"quantity": q, "unit": u, "expected_interval": iv, "accuracy": acc}, in.Reason)
	writeJSON(w, 200, map[string]any{"id": id, "quantity": q, "unit": u, "expected_interval": iv, "accuracy": acc})
}

// GET /api/v1/datastreams/{id}: datastream with asset, site, current device, current alarm rules and last observation.
func (s *Server) getDatastream(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	id := r.PathValue("id")
	var ds struct {
		ID, Key, Quantity, Unit, Kind, Interval, AssetID, Asset, AssetType, SiteID, Site string
		Accuracy                                                                         *float64
		Created                                                                          time.Time
	}
	err := s.pool.QueryRow(r.Context(), `SELECT d.id::text, d.key, d.quantity, d.unit, d.kind, COALESCE(d.expected_interval::text,''), d.accuracy::float8, d.created_at,
		a.id::text, a.name, a.type, st.id::text, st.name FROM core.datastream d JOIN core.asset a ON a.id = d.asset_id JOIN core.site st ON st.id = a.site_id
		WHERE d.id = $1::uuid AND ($2 = '' OR d.org_id = $2::uuid)`, id, orgOf(p)).
		Scan(&ds.ID, &ds.Key, &ds.Quantity, &ds.Unit, &ds.Kind, &ds.Interval, &ds.Accuracy, &ds.Created, &ds.AssetID, &ds.Asset, &ds.AssetType, &ds.SiteID, &ds.Site)
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
	rules := s.alarmRulesFor(r, orgOf(p), id)
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
		"datastream": map[string]any{"id": ds.ID, "key": ds.Key, "quantity": ds.Quantity, "unit": ds.Unit, "kind": ds.Kind, "expected_interval": ds.Interval, "accuracy": ds.Accuracy, "created_at": ds.Created},
		"asset":      map[string]any{"id": ds.AssetID, "name": ds.Asset, "type": ds.AssetType}, "site": map[string]any{"id": ds.SiteID, "name": ds.Site},
		"device": device, "rules": ruleRefs, "alarm_rules": rules, "last_observation": last,
	})
}

// ---- alarm rules --------------------------------------------------------------------------------

func (s *Server) alarmRulesFor(r *http.Request, org, dsID string) []map[string]any {
	rows, err := s.pool.Query(r.Context(), `SELECT ar.id::text, ar.datastream_id::text, d.key, a.name, ar.type, ar.warning::float8, ar.action::float8, EXTRACT(EPOCH FROM ar.delay)::float8, ar.hysteresis::float8, ar.enabled, ar.version, ar.valid_from, ar.reason, ar.escalation_policy_id::text
		FROM core.alarm_rule ar JOIN core.datastream d ON d.id = ar.datastream_id JOIN core.asset a ON a.id = d.asset_id
		WHERE ar.valid_to IS NULL AND ($1 = '' OR ar.org_id = $1::uuid) AND ($2 = '' OR ar.datastream_id = $2::uuid) ORDER BY a.name, d.key, ar.type`, org, dsID)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, ds, key, asset, typ, reason string
		var warning, action *float64
		var delay, hyst float64
		var enabled bool
		var version int
		var from time.Time
		var policyID *string
		if err := rows.Scan(&id, &ds, &key, &asset, &typ, &warning, &action, &delay, &hyst, &enabled, &version, &from, &reason, &policyID); err != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "datastream_id": ds, "key": key, "asset": asset, "type": typ, "warning": warning, "action": action, "delay_seconds": delay, "hysteresis": hyst, "enabled": enabled, "version": version, "valid_from": from, "reason": reason, "escalation_policy_id": policyID})
	}
	return out
}

func (s *Server) listAlarmRules(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	writeJSON(w, 200, s.alarmRulesFor(r, orgOf(p), r.URL.Query().Get("datastream_id")))
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
	switch in.Type {
	case "high", "low":
		if in.Warning == nil && in.Action == nil {
			writeErr(w, 400, "warning or action limit required")
			return
		}
	case "comm_loss", "sensor_fault", "low_battery", "power_loss", "door_open":
	default:
		writeErr(w, 400, "type must be high, low, comm_loss, sensor_fault, low_battery, power_loss or door_open")
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
	if rows, err := s.pool.Query(r.Context(), `SELECT a.id::text, d.id::text, d.key, d.unit, d.quantity, ast.name, a.valid_from FROM core.assignment a JOIN core.datastream d ON d.id = a.to_id JOIN core.asset ast ON ast.id = d.asset_id
		WHERE a.kind = 'device_datastream' AND a.from_id = $1::uuid AND a.valid_to IS NULL ORDER BY ast.name, d.key`, id); err == nil {
		for rows.Next() {
			var aid, dsID, key, unit, quantity, asset string
			var since time.Time
			if rows.Scan(&aid, &dsID, &key, &unit, &quantity, &asset, &since) == nil {
				assigned = append(assigned, map[string]any{"assignment_id": aid, "datastream_id": dsID, "key": key, "unit": unit, "quantity": quantity, "asset": asset, "since": since})
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
	tag, err := s.pool.Exec(r.Context(), `UPDATE core.assignment SET valid_to = now() WHERE kind = 'device_datastream' AND org_id = $1::uuid AND from_id = $2::uuid AND to_id = $3::uuid AND valid_to IS NULL`, org, devID, dsID)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, 404, "current assignment not found")
		return
	}
	if s.ing != nil {
		s.ing.InvalidateDevice(devID)
	}
	s.audit(r, "assignment.close", "assignment", "", map[string]any{"device_id": devID, "datastream_id": dsID}, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

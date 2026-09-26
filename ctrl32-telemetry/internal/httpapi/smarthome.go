package httpapi

// Smart-home API (spec 005 US2–US4): MQTT sources (accounts on the embedded broker with their own topic namespace,
// bridges to foreign brokers), the inbox of discovered devices, entities with their state and control, the Home page
// data and onboarding details.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/broker"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/smarthome"
)

func (s *Server) smartAvailable(w http.ResponseWriter) bool {
	if s.smart == nil {
		writeErr(w, 503, "smart home is not available on this server")
		return false
	}
	return true
}

// publicSettings removes secrets from source settings.
func publicSettings(settings map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range settings {
		if k == "password_sealed" {
			out["password_set"] = v != ""
			continue
		}
		out[k] = v
	}
	return out
}

func (s *Server) listMQTTSources(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.smartAvailable(w) {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT c.id::text, c.kind, c.name, c.settings, c.created_at,
		(SELECT count(*) FROM core.discovery d WHERE d.connector_id = c.id AND d.status = 'pending')
		FROM core.connector c WHERE c.org_id = $1::uuid AND c.kind IN ('mqtt_account', 'mqtt_bridge') AND c.revoked_at IS NULL ORDER BY c.name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, kind, name string
		var settings map[string]any
		var created time.Time
		var pending int
		if rows.Scan(&id, &kind, &name, &settings, &created, &pending) != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "kind": strings.TrimPrefix(kind, "mqtt_"), "name": name, "settings": publicSettings(settings),
			"created_at": created, "pending": pending, "status": s.smart.Status(id)})
	}
	writeJSON(w, 200, out)
}

type mqttSourceIn struct {
	Kind               string    `json:"kind"`
	Name               *string   `json:"name"`
	Host               *string   `json:"host"`
	Port               *int      `json:"port"`
	TLS                *bool     `json:"tls"`
	InsecureSkipVerify *bool     `json:"insecure_skip_verify"`
	Username           *string   `json:"username"`
	Password           *string   `json:"password"`
	ClientID           *string   `json:"client_id"`
	DiscoveryPrefix    *string   `json:"discovery_prefix"`
	Adopt              *string   `json:"adopt"`
	SiteID             *string   `json:"site_id"`
	PublishDatastreams *[]string `json:"publish_datastreams"`
	Reason             string    `json:"reason"`
}

// applySourceSettings merges the request into the settings; it returns an error message or "".
func (s *Server) applySourceSettings(r *http.Request, org, kind string, in *mqttSourceIn, st map[string]any) string {
	if in.DiscoveryPrefix != nil {
		p := strings.Trim(strings.TrimSpace(*in.DiscoveryPrefix), "/")
		if p == "" || strings.ContainsAny(p, "#+") || len(p) > 64 {
			return "discovery_prefix must be a topic without wildcards"
		}
		st["discovery_prefix"] = p
	}
	if in.Adopt != nil {
		if *in.Adopt != "auto" && *in.Adopt != "inbox" {
			return "adopt must be auto or inbox"
		}
		st["adopt"] = *in.Adopt
	}
	if in.SiteID != nil {
		if *in.SiteID != "" {
			var n int
			_ = s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.site WHERE id = $1::uuid AND org_id = $2::uuid`, *in.SiteID, org).Scan(&n)
			if n == 0 {
				return "site not found"
			}
		}
		st["site_id"] = *in.SiteID
	}
	if kind != "mqtt_bridge" {
		return ""
	}
	if in.PublishDatastreams != nil {
		if len(*in.PublishDatastreams) > 200 {
			return "at most 200 datastreams can be published"
		}
		ids := []any{}
		for _, id := range *in.PublishDatastreams {
			var n int
			_ = s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.datastream WHERE id = $1::uuid AND org_id = $2::uuid`, id, org).Scan(&n)
			if n == 0 {
				return "datastream " + id + " not found"
			}
			ids = append(ids, id)
		}
		st["publish_datastreams"] = ids
	}
	if in.Host != nil {
		h := strings.TrimSpace(*in.Host)
		if h == "" || strings.ContainsAny(h, "/ ") || len(h) > 255 {
			return "host must be a host name or address"
		}
		st["host"] = h
	}
	if in.Port != nil {
		if *in.Port < 1 || *in.Port > 65535 {
			return "port out of range"
		}
		st["port"] = *in.Port
	}
	if in.TLS != nil {
		st["tls"] = *in.TLS
	}
	if in.InsecureSkipVerify != nil {
		st["insecure_skip_verify"] = *in.InsecureSkipVerify
	}
	if in.Username != nil {
		st["username"] = strings.TrimSpace(*in.Username)
	}
	if in.ClientID != nil {
		st["client_id"] = strings.TrimSpace(*in.ClientID)
	}
	if in.Password != nil {
		if *in.Password == "" {
			st["password_sealed"] = ""
		} else {
			sealed, err := s.smart.SealPassword(*in.Password)
			if err != nil {
				return "password could not be stored: " + err.Error()
			}
			st["password_sealed"] = sealed
		}
	}
	if str, _ := st["host"].(string); str == "" {
		return "host required"
	}
	return ""
}

func (s *Server) createMQTTSource(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.smartAvailable(w) {
		return
	}
	var in mqttSourceIn
	if err := decodeJSON(r, &in, 8192); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if in.Name == nil || strings.TrimSpace(*in.Name) == "" || len(*in.Name) > 128 {
		writeErr(w, 400, "name required (max 128 characters)")
		return
	}
	kind := map[string]string{"account": "mqtt_account", "bridge": "mqtt_bridge"}[in.Kind]
	if kind == "" {
		writeErr(w, 400, "kind must be account or bridge")
		return
	}
	st := map[string]any{"discovery_prefix": "homeassistant"}
	resp := map[string]any{}
	secretHash := "-"
	if kind == "mqtt_account" {
		st["adopt"] = "auto"
		user, err := iam.RandomToken(6)
		if err != nil {
			writeErr(w, 500, "credential generation failed")
			return
		}
		pw, err := iam.RandomToken(18)
		if err != nil {
			writeErr(w, 500, "credential generation failed")
			return
		}
		username := broker.AccountPrefix + strings.ToLower(strings.NewReplacer("-", "0", "_", "1").Replace(user))
		st["username"] = username
		st["namespace"] = smarthome.NewNamespace(org + username)
		secretHash = broker.TokenHash(pw)
		resp["username"], resp["password"] = username, pw
	} else {
		st["adopt"] = "inbox"
		st["port"] = 1883
	}
	if msg := s.applySourceSettings(r, org, kind, &in, st); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	var id string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO core.connector (org_id, kind, name, secret_hash, settings) VALUES ($1::uuid, $2, $3, $4, $5) RETURNING id::text`,
		org, kind, strings.TrimSpace(*in.Name), secretHash, st).Scan(&id); err != nil {
		writeErr(w, 409, "source could not be created (name already used?)")
		return
	}
	s.audit(r, "mqtt_source.create", "connector", id, nil, map[string]any{"kind": kind, "name": *in.Name, "settings": publicSettings(st)}, in.Reason)
	_ = s.smart.Reload(r.Context())
	resp["id"], resp["kind"], resp["name"], resp["settings"] = id, strings.TrimPrefix(kind, "mqtt_"), strings.TrimSpace(*in.Name), publicSettings(st)
	writeJSON(w, 201, resp)
}

func (s *Server) loadSource(w http.ResponseWriter, r *http.Request, org string) (kind, name string, st map[string]any, ok bool) {
	if err := s.pool.QueryRow(r.Context(), `SELECT kind, name, settings FROM core.connector WHERE id = $1::uuid AND org_id = $2::uuid
		AND kind IN ('mqtt_account', 'mqtt_bridge') AND revoked_at IS NULL`, r.PathValue("id"), org).Scan(&kind, &name, &st); err != nil {
		writeErr(w, 404, "MQTT source not found")
		return "", "", nil, false
	}
	return kind, name, st, true
}

func (s *Server) updateMQTTSource(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.smartAvailable(w) {
		return
	}
	var in mqttSourceIn
	if err := decodeJSON(r, &in, 8192); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	kind, name, st, ok := s.loadSource(w, r, org)
	if !ok {
		return
	}
	old := publicSettings(st)
	if msg := s.applySourceSettings(r, org, kind, &in, st); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		name = strings.TrimSpace(*in.Name)
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.connector SET name = $2, settings = $3 WHERE id = $1::uuid`, r.PathValue("id"), name, st); err != nil {
		writeErr(w, 409, "source could not be updated (name already used?)")
		return
	}
	s.audit(r, "mqtt_source.update", "connector", r.PathValue("id"), old, publicSettings(st), in.Reason)
	_ = s.smart.Reload(r.Context())
	writeJSON(w, 200, map[string]any{"id": r.PathValue("id"), "name": name, "settings": publicSettings(st)})
}

func (s *Server) mqttAccountPassword(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.smartAvailable(w) {
		return
	}
	var in struct{ Reason string }
	if err := decodeJSON(r, &in, 2048); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	kind, _, st, ok := s.loadSource(w, r, org)
	if !ok {
		return
	}
	if kind != "mqtt_account" {
		writeErr(w, 400, "only MQTT accounts have a password issued by ctrl32")
		return
	}
	pw, err := iam.RandomToken(18)
	if err != nil {
		writeErr(w, 500, "password generation failed")
		return
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.connector SET secret_hash = $2 WHERE id = $1::uuid`, r.PathValue("id"), broker.TokenHash(pw)); err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	if s.br != nil {
		s.br.DisconnectAccount(r.PathValue("id"))
	}
	s.audit(r, "mqtt_source.password", "connector", r.PathValue("id"), nil, nil, in.Reason)
	writeJSON(w, 200, map[string]any{"id": r.PathValue("id"), "username": st["username"], "password": pw})
}

func (s *Server) deleteMQTTSource(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.smartAvailable(w) {
		return
	}
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	if _, _, _, ok := s.loadSource(w, r, org); !ok {
		return
	}
	id := r.PathValue("id")
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.connector SET revoked_at = now() WHERE id = $1::uuid`, id)
	_, _ = s.pool.Exec(r.Context(), `UPDATE core.entity SET status = 'removed' WHERE connector_id = $1::uuid`, id)
	if s.br != nil {
		s.br.DisconnectAccount(id)
	}
	s.audit(r, "mqtt_source.delete", "connector", id, nil, nil, reason)
	_ = s.smart.Reload(r.Context())
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---- inbox --------------------------------------------------------------------------------------------------------

func (s *Server) listDiscovery(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.smartAvailable(w) {
		return
	}
	status := r.URL.Query().Get("status")
	rows, err := s.pool.Query(r.Context(), `SELECT d.id::text, d.connector_id::text, c.name, d.device_key, d.name, d.manufacturer, d.model, d.status, d.device_id::text,
		d.first_seen, d.last_seen, (SELECT count(*) FROM jsonb_object_keys(d.configs)), (SELECT array_agg(DISTINCT split_part(k, '/', 2)) FROM jsonb_object_keys(d.configs) k)
		FROM core.discovery d JOIN core.connector c ON c.id = d.connector_id
		WHERE d.org_id = $1::uuid AND c.revoked_at IS NULL AND ($2 = '' OR d.status = $2) ORDER BY d.status, d.last_seen DESC LIMIT 500`, org, status)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, src, srcName, key, name, mf, model, st string
		var dev *string
		var first, last time.Time
		var n int
		var comps []string
		if rows.Scan(&id, &src, &srcName, &key, &name, &mf, &model, &st, &dev, &first, &last, &n, &comps) != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "source_id": src, "source": srcName, "device_key": key, "name": name, "manufacturer": mf, "model": model,
			"status": st, "device_id": dev, "first_seen": first, "last_seen": last, "entities": n, "components": comps})
	}
	writeJSON(w, 200, out)
}

func (s *Server) adoptDiscovery(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.smartAvailable(w) {
		return
	}
	var in struct{ Reason string }
	if err := decodeJSON(r, &in, 2048); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	p := PrincipalFrom(r.Context())
	dev, err := s.smart.Adopt(r.Context(), org, r.PathValue("id"), in.Reason, "user", p.UserID, p.Username)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"id": r.PathValue("id"), "device_id": dev, "status": "adopted"})
}

func (s *Server) ignoreDiscovery(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.smartAvailable(w) {
		return
	}
	var in struct{ Reason string }
	if err := decodeJSON(r, &in, 2048); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if err := s.smart.Ignore(r.Context(), org, r.PathValue("id")); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	s.audit(r, "discovery.ignore", "discovery", r.PathValue("id"), nil, nil, in.Reason)
	writeJSON(w, 200, map[string]any{"id": r.PathValue("id"), "status": "ignored"})
}

// ---- entities -----------------------------------------------------------------------------------------------------

const entityCols = `e.id::text, e.component, e.name, e.state, e.attributes, e.available, e.status, e.error, e.hidden, e.last_changed, e.last_updated,
	e.device_id::text, d.name, e.room_id::text, coalesce(rm.name, ''), e.datastream_id::text, coalesce(ds.unit, ''), e.config`

func (s *Server) scanEntity(row interface{ Scan(...any) error }, withConfig bool) (map[string]any, error) {
	var id, comp, name, status, errText, dev, devName, roomName, unit string
	var state, room, ds *string
	var attrs, cfg map[string]any
	var avail, hidden bool
	var changed, updated *time.Time
	if err := row.Scan(&id, &comp, &name, &state, &attrs, &avail, &status, &errText, &hidden, &changed, &updated, &dev, &devName, &room, &roomName, &ds, &unit, &cfg); err != nil {
		return nil, err
	}
	m := map[string]any{"id": id, "component": comp, "name": name, "state": state, "attributes": attrs, "available": avail, "status": status, "error": errText,
		"hidden": hidden, "last_changed": changed, "last_updated": updated, "device_id": dev, "device": devName, "room_id": room, "room": roomName,
		"datastream_id": ds, "unit": unit, "services": s.smart.Services(id)}
	// what the UI needs to draw controls
	ui := map[string]any{}
	for _, k := range []string{"device_class", "state_class", "icon", "min", "max", "step", "mode", "options", "min_temp", "max_temp", "temp_step",
		"modes", "brightness", "supported_color_modes", "schema", "effect_list", "preset_modes", "entity_category", "suggested_display_precision"} {
		if v, ok := cfg[k]; ok {
			ui[k] = v
		}
	}
	m["ui"] = ui
	if withConfig {
		m["config"] = cfg
	}
	return m, nil
}

func (s *Server) listEntities(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.smartAvailable(w) {
		return
	}
	q := r.URL.Query()
	rows, err := s.pool.Query(r.Context(), `SELECT `+entityCols+` FROM core.entity e JOIN core.device d ON d.id = e.device_id
		LEFT JOIN core.asset rm ON rm.id = e.room_id LEFT JOIN core.datastream ds ON ds.id = e.datastream_id
		WHERE e.org_id = $1::uuid AND e.status <> 'removed' AND d.revoked_at IS NULL
		  AND ($2 = '' OR e.room_id::text = $2) AND ($3 = '' OR e.device_id::text = $3) AND ($4 = '' OR e.component = $4)
		ORDER BY rm.name NULLS LAST, d.name, e.sort, e.name LIMIT 2000`, org, q.Get("room"), q.Get("device"), q.Get("component"))
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		if m, err := s.scanEntity(rows, false); err == nil {
			out = append(out, m)
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) getEntity(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.smartAvailable(w) {
		return
	}
	m, err := s.scanEntity(s.pool.QueryRow(r.Context(), `SELECT `+entityCols+` FROM core.entity e JOIN core.device d ON d.id = e.device_id
		LEFT JOIN core.asset rm ON rm.id = e.room_id LEFT JOIN core.datastream ds ON ds.id = e.datastream_id
		WHERE e.id = $1::uuid AND e.org_id = $2::uuid`, r.PathValue("id"), org), true)
	if err != nil {
		writeErr(w, 404, "entity not found")
		return
	}
	writeJSON(w, 200, m)
}

func (s *Server) patchEntity(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.smartAvailable(w) {
		return
	}
	var in struct {
		Name   *string `json:"name"`
		RoomID *string `json:"room_id"`
		Hidden *bool   `json:"hidden"`
		Sort   *int    `json:"sort"`
		Reason string  `json:"reason"`
	}
	if err := decodeJSON(r, &in, 4096); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	if in.Name != nil && (strings.TrimSpace(*in.Name) == "" || len(*in.Name) > 200) {
		writeErr(w, 400, "name required (max 200 characters)")
		return
	}
	if in.RoomID != nil && *in.RoomID != "" {
		var n int
		_ = s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.asset WHERE id = $1::uuid AND org_id = $2::uuid`, *in.RoomID, org).Scan(&n)
		if n == 0 {
			writeErr(w, 400, "room not found")
			return
		}
	}
	var oldName string
	var oldRoom *string
	var oldHidden bool
	if err := s.pool.QueryRow(r.Context(), `SELECT name, room_id::text, hidden FROM core.entity WHERE id = $1::uuid AND org_id = $2::uuid`, r.PathValue("id"), org).
		Scan(&oldName, &oldRoom, &oldHidden); err != nil {
		writeErr(w, 404, "entity not found")
		return
	}
	_, err := s.pool.Exec(r.Context(), `UPDATE core.entity SET name = coalesce($2, name), room_id = CASE WHEN $3::boolean THEN nullif($4, '')::uuid ELSE room_id END,
		hidden = coalesce($5, hidden), sort = coalesce($6, sort) WHERE id = $1::uuid`, r.PathValue("id"), in.Name, in.RoomID != nil, deref(in.RoomID), in.Hidden, in.Sort)
	if err != nil {
		writeErr(w, 500, "update failed")
		return
	}
	s.audit(r, "entity.update", "entity", r.PathValue("id"), map[string]any{"name": oldName, "room_id": oldRoom, "hidden": oldHidden},
		map[string]any{"name": in.Name, "room_id": in.RoomID, "hidden": in.Hidden}, in.Reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// entityCommand turns a control action into a device command (same permissions, four eyes, audit and statuses as any
// other command) after checking that the entity can do it.
func (s *Server) entityCommand(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.smartAvailable(w) {
		return
	}
	var in struct {
		Service string `json:"service"`
		smarthome.Params
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &in, 8192); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	id := r.PathValue("id")
	var devID, name string
	if err := s.pool.QueryRow(r.Context(), `SELECT device_id::text, name FROM core.entity WHERE id = $1::uuid AND org_id = $2::uuid AND status <> 'removed'`, id, org).Scan(&devID, &name); err != nil {
		writeErr(w, 404, "entity not found")
		return
	}
	if _, err := s.smart.Check(id, in.Service, in.Params); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		in.Reason = "Home: " + strings.ReplaceAll(in.Service, "_", " ") + " " + name
	}
	args, _ := json.Marshal(smarthome.CommandArgs{Entity: id, Params: in.Params})
	body, _ := json.Marshal(map[string]any{"method": in.Service, "args": json.RawMessage(args), "ttl_seconds": 60, "reason": in.Reason})
	r2 := r.Clone(r.Context())
	r2.Body = io.NopCloser(bytes.NewReader(body))
	r2.ContentLength = int64(len(body))
	r2.SetPathValue("id", devID)
	s.sendCommand(w, r2)
}

// home returns rooms with their entities for the Home page in one call.
func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok || !s.smartAvailable(w) {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT `+entityCols+` FROM core.entity e JOIN core.device d ON d.id = e.device_id
		LEFT JOIN core.asset rm ON rm.id = e.room_id LEFT JOIN core.datastream ds ON ds.id = e.datastream_id
		WHERE e.org_id = $1::uuid AND e.status <> 'removed' AND NOT e.hidden AND d.revoked_at IS NULL
		ORDER BY rm.name NULLS LAST, d.name, e.sort, e.name LIMIT 2000`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	type room struct {
		ID       *string          `json:"id"`
		Name     string           `json:"name"`
		Entities []map[string]any `json:"entities"`
	}
	var rooms []*room
	index := map[string]*room{}
	for rows.Next() {
		m, err := s.scanEntity(rows, false)
		if err != nil {
			continue
		}
		key, _ := m["room_id"].(*string)
		k := ""
		if key != nil {
			k = *key
		}
		rm := index[k]
		if rm == nil {
			rm = &room{ID: key, Name: m["room"].(string)}
			index[k] = rm
			rooms = append(rooms, rm)
		}
		rm.Entities = append(rm.Entities, m)
	}
	var pending int
	_ = s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.discovery d JOIN core.connector c ON c.id = d.connector_id WHERE d.org_id = $1::uuid AND d.status = 'pending' AND c.revoked_at IS NULL`, org).Scan(&pending)
	p := PrincipalFrom(r.Context())
	writeJSON(w, 200, map[string]any{"rooms": rooms, "pending_devices": pending, "can_control": p != nil && p.Has("device.command"), "can_manage": p != nil && p.Has("device.manage")})
}

// onboarding returns the connection details devices need, for the "Add device" pages.
func (s *Server) onboarding(w http.ResponseWriter, r *http.Request) {
	host := strings.TrimPrefix(strings.TrimPrefix(s.cfg.HTTP.BaseURL, "https://"), "http://")
	if i := strings.IndexAny(host, "/"); i >= 0 {
		host = host[:i]
	}
	if i := strings.LastIndex(host, ":"); i >= 0 && !strings.HasSuffix(host, "]") {
		host = host[:i]
	}
	port := func(listen string) int {
		if i := strings.LastIndex(listen, ":"); i >= 0 {
			n, _ := strconv.Atoi(listen[i+1:])
			return n
		}
		return 0
	}
	mqtt := map[string]any{"host": host, "port": port(s.cfg.MQTT.Listen), "tls_port": port(s.cfg.MQTT.TLSListen), "ws_port": port(s.cfg.MQTT.WSListen)}
	writeJSON(w, 200, map[string]any{"base_url": s.cfg.HTTP.BaseURL, "mqtt": mqtt, "provision_url": strings.TrimRight(s.cfg.HTTP.BaseURL, "/") + "/api/v1/provision"})
}

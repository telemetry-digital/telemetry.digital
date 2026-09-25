package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Starter dashboards are built from the organization's own datastreams and devices, so a new dashboard shows real
// data right away: "overview" (devices, alarms, latest values) or "showcase" (every widget type of screen format v1,
// for demos and as a starting point). The UI offers them in the New dashboard dialog; the CLI seeds them
// (`admin seed-dashboard`) where no browser session is at hand.

// StarterSpec returns the name and the JSON spec of a starter dashboard for the organization.
func StarterSpec(ctx context.Context, pool *pgxpool.Pool, org, kind string) (string, json.RawMessage, error) {
	ds := []string{}
	rows, err := pool.Query(ctx, `SELECT id::text FROM core.datastream WHERE org_id = $1::uuid ORDER BY id LIMIT 3`, org)
	if err != nil {
		return "", nil, err
	}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ds = append(ds, id)
		}
	}
	rows.Close()
	first := []string{}
	if len(ds) > 0 {
		first = ds[:1]
	}
	w := func(id, typ string, x, y, wd, h int, binds map[string][]string, params map[string]any) map[string]any {
		if binds == nil {
			binds = map[string][]string{}
		}
		return map[string]any{"id": id, "type": typ, "geometry": map[string]int{"x": x, "y": y, "w": wd, "h": h}, "bindings": binds, "params": params}
	}
	dsb := func(ids []string) map[string][]string { return map[string][]string{"datastreams": ids} }
	var name string
	var widgets []map[string]any
	switch kind {
	case "overview":
		name = "Overview"
		widgets = []map[string]any{
			w("values", "table", 0, 0, 12, 2, dsb(ds), map[string]any{"title": "Latest values"}),
			w("devices", "devices", 0, 2, 6, 3, map[string][]string{"devices": {}}, map[string]any{"title": "Devices", "limit": 20}),
			w("alarms", "alarms", 6, 2, 6, 3, nil, map[string]any{"title": "Alarms", "limit": 10}),
		}
	case "showcase":
		name = "Showcase — all widgets"
		widgets = []map[string]any{
			w("value", "value", 0, 0, 3, 1, dsb(first), map[string]any{"title": "Latest value", "decimals": 1}),
			w("status", "status", 0, 1, 3, 1, dsb(first), map[string]any{"title": "State indicator", "states": []string{"0=Off:#94a3b8", "1=On:#059669", "2=Fault:#dc2626"}}),
			w("gauge", "gauge", 3, 0, 3, 2, dsb(first), map[string]any{"title": "Gauge", "y_min": 0, "y_max": 100}),
			w("trend", "chart", 6, 0, 6, 3, dsb(ds), map[string]any{"title": "Trend (area)", "chart": "area", "band": true, "legend": true}),
			w("bars", "bars", 0, 2, 3, 2, dsb(ds), map[string]any{"title": "Compare values"}),
			w("donut", "donut", 3, 2, 3, 2, dsb(ds), map[string]any{"title": "Share"}),
			w("trend2", "chart", 6, 3, 6, 3, dsb(first), map[string]any{"title": "Trend (bars, 7 d)", "chart": "bar", "range": "7d", "band": false, "legend": false}),
			w("heat", "heatmap", 0, 4, 6, 2, dsb(first), map[string]any{"title": "Heatmap (hour × day)", "range": "7d"}),
			w("map", "map", 0, 6, 6, 4, map[string][]string{"devices": {}}, map[string]any{"title": "Devices on the map"}),
			w("table", "table", 6, 6, 6, 2, dsb(ds), map[string]any{"title": "Values table"}),
			w("alarms", "alarms", 6, 8, 3, 2, nil, map[string]any{"title": "Alarms", "limit": 10}),
			w("devices", "devices", 9, 8, 3, 2, map[string][]string{"devices": {}}, map[string]any{"title": "Devices", "limit": 20}),
			w("note", "text", 0, 10, 12, 1, nil, map[string]any{"title": "Note", "style": "note", "text": "Showcase of every widget type. Click Edit to move, resize and rebind the widgets to your own datastreams."}),
		}
	case "mimic":
		name = "Process picture — showcase"
		second, third := first, first
		if len(ds) > 1 {
			second = ds[1:2]
		}
		if len(ds) > 2 {
			third = ds[2:3]
		}
		widgets = []map[string]any{
			w("title", "text", 20, 20, 700, 60, nil, map[string]any{"title": "Process picture", "text": "Tank → pump → valve; symbols follow the bound datastreams. Edit to rebind.", "frame": true}),
			w("tank", "tank", 120, 120, 220, 320, dsb(first), map[string]any{"title": "T-101", "y_min": 0, "y_max": 100, "decimals": 1}),
			w("pipe1", "pipe", 340, 250, 300, 40, dsb(first), map[string]any{"orientation": "horizontal", "thickness": 40, "flow_min": 0}),
			w("pump", "pump", 640, 190, 160, 160, dsb(third), map[string]any{"title": "P-101", "on_value": 1}),
			w("pipe2", "pipe", 800, 250, 300, 40, dsb(third), map[string]any{"orientation": "horizontal", "thickness": 40, "flow_min": 0}),
			w("valve", "valve", 1100, 190, 160, 160, dsb(second), map[string]any{"title": "V-101", "open_value": 1}),
			w("pipe3", "pipe", 1260, 250, 240, 40, dsb(second), map[string]any{"orientation": "horizontal", "thickness": 40, "flow_min": 0}),
			w("tt101", "instrument", 700, 420, 140, 140, dsb(second), map[string]any{"tag": "TT-101", "decimals": 1}),
			w("val", "value", 1000, 420, 320, 140, dsb(ds), map[string]any{"title": "Latest values", "decimals": 1, "sparkline": false}),
			w("note", "text", 120, 700, 1380, 80, nil, map[string]any{"title": "Note", "style": "note", "text": "ISA-101: the process is grey, colour appears only for abnormal states. Drag symbols freely; pipes animate while the bound value is above the flow threshold."}),
		}
	default:
		return "", nil, errors.New("kind must be overview, showcase or mimic")
	}
	spec := map[string]any{"version": 1, "kind": "dashboard", "layout": map[string]any{"type": "grid", "columns": 12, "row_height": 96}, "range": "24h", "refresh_s": 30, "widgets": widgets}
	if kind == "mimic" {
		spec["kind"] = "mimic"
		spec["layout"] = map[string]any{"type": "canvas", "width": 1600, "height": 900}
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return "", nil, err
	}
	return name, raw, nil
}

// GET /api/v1/dashboards/starter?kind=overview|showcase → {name, spec}
func (s *Server) dashboardStarter(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "overview"
	}
	name, raw, err := StarterSpec(r.Context(), s.pool, org, kind)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	spec, err := validateScreenSpec(raw)
	if err != nil {
		writeErr(w, 500, "starter spec invalid: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"kind": kind, "name": name, "spec": spec})
}

// SeededDashboard describes a dashboard created outside an HTTP session (CLI).
type SeededDashboard struct {
	ID      string
	Slug    string
	Name    string
	Version int
	Widgets int
}

// CreateDashboard validates the spec and inserts version 1 of a new dashboard (used by the CLI; the HTTP handler
// has its own versioning path). createdBy may be empty for system actors.
func CreateDashboard(ctx context.Context, pool *pgxpool.Pool, org, name, slug string, raw json.RawMessage, isDefault bool, createdBy, reason string) (*SeededDashboard, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 {
		return nil, errors.New("name required (max 128)")
	}
	if slug == "" {
		slug = slugify(name)
	}
	if !dashSlugRe.MatchString(slug) || slug == "default" || slug == "catalog" || slug == "starter" {
		return nil, errors.New("slug must be lowercase letters, digits and dashes")
	}
	spec, err := validateScreenSpec(raw)
	if err != nil {
		return nil, err
	}
	specJSON, _ := json.Marshal(spec)
	sum := sha256.Sum256(specJSON)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM core.dashboard WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL)`, org, slug).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("a dashboard with slug %q exists", slug)
	}
	if isDefault {
		if _, err := tx.Exec(ctx, `UPDATE core.dashboard SET is_default = false WHERE org_id = $1::uuid AND valid_to IS NULL AND is_default`, org); err != nil {
			return nil, err
		}
	}
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO core.dashboard (org_id, slug, name, version, spec, checksum, is_default, created_by, reason) VALUES ($1::uuid, $2, $3, 1, $4::jsonb, $5, $6, NULLIF($7,'')::uuid, $8) RETURNING id::text`,
		org, slug, name, string(specJSON), hex.EncodeToString(sum[:]), isDefault, createdBy, reason).Scan(&id); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &SeededDashboard{ID: id, Slug: slug, Name: name, Version: 1, Widgets: len(spec["widgets"].([]map[string]any))}, nil
}

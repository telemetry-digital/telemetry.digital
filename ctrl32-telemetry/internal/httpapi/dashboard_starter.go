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
	case "plant":
		name = "Mixing plant — process picture"
		return name, plantPicture(ds), nil
	default:
		return "", nil, errors.New("kind must be overview, showcase, mimic or plant")
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

// plantPicture is the demonstration SCADA picture (spec 001, US5): a mixing plant drawn with the element layer —
// silos, screw conveyors, a jacketed reactor with agitator, steam control valve, heat exchanger, transfer pump,
// product tank, ISA instruments, pipes with flow and a status panel — plus a trend and an alarm list as widgets.
// Tags cycle through the organization's first datastreams (level, temperature, run state), so it moves with demo data.
func plantPicture(ds []string) json.RawMessage {
	tag := func(i int) string {
		if len(ds) == 0 {
			return ""
		}
		return ds[i%len(ds)]
	}
	lv, tt, run := tag(0), tag(1), tag(2)
	els := []map[string]any{}
	add := func(e map[string]any) {
		for k, v := range e {
			if v == "" || v == nil {
				delete(e, k)
			}
		}
		els = append(els, e)
	}
	level := func(t string) []map[string]any {
		return []map[string]any{{"prop": "level", "tag": t, "lin": []float64{0, 100, 0, 1}}}
	}
	running := []map[string]any{{"prop": "state", "tag": run, "map": []map[string]any{{"op": "==", "v": 2, "to": "fault"}, {"op": ">=", "v": 1, "to": "run"}, {"op": "<", "v": 1, "to": "stop"}}}}
	flow := func(extra ...map[string]any) []map[string]any {
		return append([]map[string]any{{"prop": "flow", "tag": run, "op": ">=", "v": 1, "speed": 1.2}}, extra...)
	}
	sym := func(id, symbol string, x, y, w, h int, t, name string, dyn []map[string]any, style map[string]any) {
		if style == nil {
			style = map[string]any{"shade": true}
		}
		add(map[string]any{"id": id, "type": "symbol", "symbol": symbol, "x": x, "y": y, "w": w, "h": h, "tag": t, "name": name, "dyn": dyn, "style": style})
	}
	pipe := func(id string, pts [][2]int, medium string, sw int, dyn []map[string]any) {
		add(map[string]any{"id": id, "type": "pipe", "points": pts, "style": map[string]any{"sw": sw, "medium": medium}, "dyn": dyn})
	}
	label := func(id string, x, y, w int, text string) {
		add(map[string]any{"id": id, "type": "text", "x": x, "y": y, "w": w, "h": 20, "text": text, "style": map[string]any{"font": 13, "bold": true, "color": "#1f2937", "align": "middle"}})
	}
	value := func(id string, x, y, w int, t string, dyn []map[string]any) {
		add(map[string]any{"id": id, "type": "text", "x": x, "y": y, "w": w, "h": 26, "text": "{value} {unit}", "tag": t, "decimals": 1, "style": map[string]any{"font": 14, "bold": true, "color": "#0f172a", "align": "middle", "bg": "#ffffff", "stroke": "#64748b", "radius": 3}, "dyn": dyn})
	}
	// header
	add(map[string]any{"id": "hdr", "type": "rect", "x": 0, "y": 0, "w": 1600, "h": 54, "style": map[string]any{"fill": "#1e293b"}})
	add(map[string]any{"id": "hdrt", "type": "text", "x": 20, "y": 10, "w": 700, "h": 34, "text": "MIXING PLANT · AREA 1", "style": map[string]any{"font": 22, "bold": true, "color": "#f8fafc"}})
	add(map[string]any{"id": "hdrs", "type": "text", "x": 760, "y": 14, "w": 380, "h": 26, "text": "Line state: {value}", "tag": run, "style": map[string]any{"font": 15, "color": "#e2e8f0", "align": "end"}, "dyn": []map[string]any{{"prop": "text", "map": []map[string]any{{"op": "==", "v": 2, "to": "FAULT"}, {"op": ">=", "v": 1, "to": "RUNNING"}, {"op": "<", "v": 1, "to": "STOPPED"}}}, {"prop": "blink", "op": "==", "v": 2, "color": "#ef4444"}}})
	// raw material silos and conveyors
	sym("s101", "silo", 70, 100, 120, 220, lv, "S-101 Sugar", level(lv), nil)
	sym("s102", "silo", 250, 100, 120, 220, lv, "S-102 Starch", level(lv), nil)
	label("s101l", 60, 80, 140, "S-101")
	label("s102l", 240, 80, 140, "S-102")
	sym("sc101", "screw_conveyor", 110, 350, 220, 44, run, "SC-101 Screw conveyor", running, nil)
	sym("m101", "motor", 60, 346, 52, 38, run, "M-101", running, nil)
	pipe("pr1", [][2]int{{330, 372}, {560, 372}, {560, 190}, {640, 190}}, "#a16207", 14, flow())
	// reactor with jacket, agitator and steam
	sym("r201", "reactor", 640, 110, 200, 300, lv, "R-201 Mixing reactor", append(level(lv), map[string]any{"prop": "rotate", "tag": run, "op": ">=", "v": 1, "speed": 1.5}), nil)
	label("r201l", 640, 90, 200, "R-201")
	value("r201v", 610, 420, 100, lv, nil)
	sym("cv201", "valve_control", 900, 120, 70, 80, run, "CV-201 Steam", []map[string]any{{"prop": "state", "tag": run, "map": []map[string]any{{"op": ">=", "v": 1, "to": "open"}, {"op": "<", "v": 1, "to": "closed"}}}}, nil)
	pipe("ps1", [][2]int{{1100, 180}, {970, 180}}, "#ef4444", 10, flow())
	pipe("ps2", [][2]int{{900, 180}, {845, 180}, {845, 230}}, "#ef4444", 10, flow())
	label("cv201l", 880, 96, 110, "CV-201")
	sym("hx201", "shell_tube", 1010, 250, 190, 80, tt, "HX-201 Heat exchanger", nil, nil)
	label("hx201l", 1010, 334, 190, "HX-201")
	pipe("pc1", [][2]int{{840, 360}, {950, 360}, {950, 290}, {1010, 290}}, "#0891b2", 12, flow())
	pipe("pc2", [][2]int{{1200, 290}, {1230, 290}, {1230, 420}, {840, 420}}, "#0891b2", 12, flow())
	// instruments on the reactor
	sym("tt201", "instr_field", 560, 250, 70, 70, tt, "TT-201 Reactor temperature", []map[string]any{{"prop": "state", "tag": tt, "map": []map[string]any{{"op": ">=", "v": 60, "to": "fault"}, {"op": ">=", "v": 45, "to": "warn"}}}}, map[string]any{})
	els[len(els)-1]["text"] = "TT-201"
	sym("lt201", "instr_field", 560, 330, 70, 70, lv, "LT-201 Reactor level", nil, map[string]any{})
	els[len(els)-1]["text"] = "LT-201"
	// outlet: reactor → pump → valve → product tank
	pipe("po1", [][2]int{{740, 410}, {740, 560}, {820, 560}}, "#0891b2", 14, flow())
	sym("p201", "pump_centrifugal", 820, 510, 90, 84, run, "P-201 Transfer pump", running, nil)
	label("p201l", 815, 600, 100, "P-201")
	pipe("po2", [][2]int{{905, 530}, {1010, 530}}, "#0891b2", 14, flow())
	sym("v202", "valve_ball", 1010, 492, 70, 60, run, "V-202", []map[string]any{{"prop": "state", "tag": run, "map": []map[string]any{{"op": ">=", "v": 1, "to": "open"}, {"op": "<", "v": 1, "to": "closed"}}}}, nil)
	label("v202l", 1005, 470, 80, "V-202")
	sym("ft203", "flow_meter", 1100, 505, 90, 60, run, "FT-203 Flow", nil, nil)
	pipe("po3", [][2]int{{1080, 530}, {1100, 530}}, "#0891b2", 14, flow())
	pipe("po4", [][2]int{{1190, 530}, {1340, 530}, {1340, 610}}, "#0891b2", 14, flow())
	sym("t301", "tank_vertical", 1230, 600, 150, 260, lv, "T-301 Product tank", level(lv), map[string]any{"shade": true, "medium": "#0891b2"})
	label("t301l", 1230, 580, 150, "T-301")
	value("t301v", 1255, 866, 100, lv, nil)
	value("tt201v", 470, 262, 84, tt, []map[string]any{{"prop": "color", "tag": tt, "map": []map[string]any{{"op": ">=", "v": 60, "to": "#dc2626"}, {"op": ">=", "v": 45, "to": "#b45309"}}}})
	// status panel
	add(map[string]any{"id": "pan", "type": "rect", "x": 40, "y": 470, "w": 380, "h": 150, "style": map[string]any{"fill": "#f1f5f9", "stroke": "#94a3b8", "radius": 6, "shadow": true}})
	add(map[string]any{"id": "pant", "type": "text", "x": 56, "y": 480, "w": 300, "h": 22, "text": "Line status", "style": map[string]any{"font": 15, "bold": true, "color": "#1f2937"}})
	for i, row := range []struct{ id, text string }{{"l1", "Agitator"}, {"l2", "Transfer pump"}, {"l3", "Steam valve"}} {
		y := 512 + i*34
		sym(row.id, "lamp", 56, y, 26, 26, run, row.text, []map[string]any{{"prop": "state", "tag": run, "map": []map[string]any{{"op": "==", "v": 2, "to": "fault"}, {"op": ">=", "v": 1, "to": "run"}, {"op": "<", "v": 1, "to": "off"}}}}, map[string]any{})
		add(map[string]any{"id": row.id + "t", "type": "text", "x": 92, "y": y + 2, "w": 300, "h": 22, "text": row.text, "style": map[string]any{"font": 14, "color": "#1f2937"}})
	}
	spec := map[string]any{"version": 1, "kind": "mimic", "layout": map[string]any{"type": "canvas", "width": 1600, "height": 900, "bg": "#dfe3e8", "grid": 10},
		"scheme": "classic", "range": "6h", "refresh_s": 30, "elements": els,
		"widgets": []map[string]any{
			{"id": "trend", "type": "chart", "geometry": map[string]int{"x": 1260, "y": 70, "w": 320, "h": 210}, "bindings": map[string][]string{"datastreams": ds}, "params": map[string]any{"title": "Trend", "chart": "line", "legend": true, "frame": true}},
			{"id": "alarms", "type": "alarms", "geometry": map[string]int{"x": 40, "y": 640, "w": 560, "h": 230}, "bindings": map[string][]string{}, "params": map[string]any{"title": "Alarms", "limit": 6, "frame": true}},
		}}
	raw, _ := json.Marshal(spec)
	return raw
}

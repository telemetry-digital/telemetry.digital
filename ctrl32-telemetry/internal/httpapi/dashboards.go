package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Screens (ADR-0008, URS J-02/J-03): a screen is a versioned content item — widgets with geometry on a grid, bindings to
// datastreams/devices and parameters. Dashboards are screens of kind "dashboard" (grid layout); process mimics are
// screens of kind "mimic" with a canvas layout (absolute positions in pixels) and ISA-101 process symbols (0.31.0). static/widgets.js renders it; data comes from the
// normal read endpoints plus two helpers:
//
//	GET  /api/v1/observations?datastream_id=&from=&to=&bucket=SECONDS   aggregated avg/min/max/count per bucket
//	GET  /api/v1/datastreams/latest?ids=a,b,c                            latest value of each datastream
//	GET  /api/v1/dashboards                                              current versions (no specs)
//	GET  /api/v1/dashboards/catalog                                      widget types, parameters, ranges
//	GET  /api/v1/dashboards/{slug}                                       current version with spec ("default" = the default one)
//	POST /api/v1/dashboards            (content.write) {slug?, name, spec, is_default?, reason}
//	PUT  /api/v1/dashboards/{slug}     (content.write) {name?, spec, is_default?, reason} -> new version
//	DELETE /api/v1/dashboards/{slug}?reason=  (content.write) retire (valid_to)

var (
	dashSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	uuidRe     = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	widgetIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)
)

// ScreenParam describes one widget parameter for the editor.
type ScreenParam struct {
	Name    string   `json:"name"`
	Label   string   `json:"label"`
	Kind    string   `json:"kind"` // text, multiline, number, bool, select, list, multi, color, icon, datetime, series, ranges, keyfilter
	Default any      `json:"default,omitempty"`
	Options []string `json:"options,omitempty"`
	Help    string   `json:"help,omitempty"`
	Group   string   `json:"group,omitempty"` // editor tab: data (default), appearance, series, time, actions
	Min     *float64 `json:"min,omitempty"`   // number bounds (optional)
	Max     *float64 `json:"max,omitempty"`
}

type WidgetDef struct {
	Type     string        `json:"type"`
	Label    string        `json:"label"`
	Group    string        `json:"group"` // charts, values, lists, other
	Help     string        `json:"help"`
	Binds    []string      `json:"binds"` // datastreams, devices
	MinBinds int           `json:"min_binds"`
	Entities bool          `json:"entities,omitempty"` // shows entities of an alias (entity table, entity selector; 0.73.0)
	Default  [2]int        `json:"default_size"`       // w, h in grid units
	Params   []ScreenParam `json:"params"`
}

var (
	pRange    = ScreenParam{Name: "range", Label: "Range (empty = dashboard range)", Kind: "select", Options: []string{"", "1h", "6h", "8h", "12h", "today", "24h", "7d", "30d", "90d"}}
	pDecimals = ScreenParam{Name: "decimals", Label: "Decimals", Kind: "number", Default: 2}
	pYMin     = ScreenParam{Name: "y_min", Label: "Axis minimum (empty = auto)", Kind: "number"}
	pYMax     = ScreenParam{Name: "y_max", Label: "Axis maximum (empty = auto)", Kind: "number"}
	pWarn     = ScreenParam{Name: "warn", Label: "Warning limit (high)", Kind: "number"}
	pAction   = ScreenParam{Name: "action", Label: "Action limit (high)", Kind: "number"}
	pLowWarn  = ScreenParam{Name: "low_warn", Label: "Warning limit (low)", Kind: "number"}
	pLowAct   = ScreenParam{Name: "low_action", Label: "Action limit (low)", Kind: "number"}
	pTitle    = ScreenParam{Name: "title", Label: "Title", Kind: "text"}
	pLimit    = ScreenParam{Name: "limit", Label: "Rows", Kind: "number", Default: 10}
	pFrame    = ScreenParam{Name: "frame", Label: "Show as a framed card", Kind: "bool", Default: false}
)

var widgetCatalog = []WidgetDef{
	{Type: "chart", Label: "Trend chart", Group: "charts", Help: "One or more datastreams over time. Long ranges are aggregated on the server (average with a min–max band).", Binds: []string{"datastreams"}, MinBinds: 1, Default: [2]int{6, 3}, Params: []ScreenParam{pTitle,
		{Name: "chart", Label: "Style", Kind: "select", Default: "line", Options: []string{"line", "area", "step", "bar", "scatter"}}, pRange, pDecimals, pYMin, pYMax, pWarn, pAction,
		{Name: "band", Label: "Show min–max band", Kind: "bool", Default: true}, {Name: "legend", Label: "Show legend", Kind: "bool", Default: true},
		{Name: "annotations", Label: "Show annotations", Kind: "bool", Default: true}}},
	{Type: "bars", Label: "Compare values", Group: "charts", Help: "Horizontal bars with the latest value of each datastream.", Binds: []string{"datastreams"}, MinBinds: 1, Default: [2]int{4, 3}, Params: []ScreenParam{pTitle, pDecimals, pYMin, pYMax, pWarn, pAction}},
	{Type: "donut", Label: "Share (donut)", Group: "charts", Help: "Latest values as shares of their sum.", Binds: []string{"datastreams"}, MinBinds: 1, Default: [2]int{4, 3}, Params: []ScreenParam{pTitle, pDecimals}},
	{Type: "heatmap", Label: "Heatmap (hour × day)", Group: "charts", Help: "Hourly averages of one datastream laid out by day — patterns at a glance.", Binds: []string{"datastreams"}, MinBinds: 1, Default: [2]int{6, 3}, Params: []ScreenParam{pTitle, {Name: "range", Label: "Range", Kind: "select", Default: "7d", Options: []string{"7d", "30d"}}, pDecimals, pYMin, pYMax}},
	{Type: "value", Label: "Latest value", Group: "values", Help: "Latest value with time, sparkline and threshold colours.", Binds: []string{"datastreams"}, MinBinds: 1, Default: [2]int{3, 2}, Params: []ScreenParam{pTitle, pDecimals, pWarn, pAction, pLowWarn, pLowAct, {Name: "sparkline", Label: "Sparkline (6 h)", Kind: "bool", Default: true}}},
	{Type: "gauge", Label: "Gauge", Group: "values", Help: "Latest value on an arc between min and max with warning and action zones.", Binds: []string{"datastreams"}, MinBinds: 1, Default: [2]int{3, 2}, Params: []ScreenParam{pTitle, pDecimals, {Name: "y_min", Label: "Minimum", Kind: "number", Default: 0}, {Name: "y_max", Label: "Maximum", Kind: "number", Default: 100}, pWarn, pAction}},
	{Type: "status", Label: "State indicator", Group: "values", Help: "Maps the latest value to a labelled colour, e.g. 0=Stopped:#6b7280, 1=Running:#059669, 2=Fault:#dc2626.", Binds: []string{"datastreams"}, MinBinds: 1, Default: [2]int{3, 2}, Params: []ScreenParam{pTitle,
		{Name: "states", Label: "States (value=Label:#colour, comma separated)", Kind: "list", Default: []string{"0=Off:#6b7280", "1=On:#059669"}}}},
	{Type: "table", Label: "Values table", Group: "lists", Help: "Latest value, unit, time and quality of the selected datastreams.", Binds: []string{"datastreams"}, MinBinds: 1, Default: [2]int{6, 3}, Params: []ScreenParam{pTitle, pDecimals}},
	{Type: "alarms", Label: "Alarms", Group: "lists", Help: "Recent alarms of the organization, active first.", Default: [2]int{6, 3}, Params: []ScreenParam{pTitle, pLimit}},
	{Type: "devices", Label: "Devices", Group: "lists", Help: "Connection state of the devices (all, or the selected ones).", Binds: []string{"devices"}, Default: [2]int{6, 3}, Params: []ScreenParam{pTitle, {Name: "limit", Label: "Rows", Kind: "number", Default: 20}}},
	{Type: "map", Label: "Map", Group: "other", Help: "Devices with a position (fixed or reported) on a map. Tiles come from Settings → Branding.", Binds: []string{"devices"}, Default: [2]int{6, 4}, Params: []ScreenParam{pTitle, {Name: "zoom", Label: "Initial zoom when empty", Kind: "number", Default: 7}}},
	{Type: "text", Label: "Text", Group: "other", Help: "A note or heading; one paragraph per line.", Default: [2]int{4, 2}, Params: []ScreenParam{pTitle, {Name: "text", Label: "Text", Kind: "multiline"}, {Name: "style", Label: "Style", Kind: "select", Default: "normal", Options: []string{"normal", "note", "warning", "heading"}}}},
	// Process symbols for mimics (ISA-101: grey process, colour only for abnormal states). They work on grid dashboards too.
	{Type: "tank", Label: "Tank", Group: "mimic", Help: "Vessel with the level of one datastream between min and max; threshold colours for abnormal levels.", Binds: []string{"datastreams"}, MinBinds: 1, Default: [2]int{2, 3}, Params: []ScreenParam{pTitle, {Name: "y_min", Label: "Level at empty", Kind: "number", Default: 0}, {Name: "y_max", Label: "Level at full", Kind: "number", Default: 100}, pDecimals, pWarn, pAction, pLowWarn, pLowAct, pFrame}},
	{Type: "pump", Label: "Pump / motor", Group: "mimic", Help: "Running state of one datastream: filled when running, outlined when stopped, red on the fault value.", Binds: []string{"datastreams"}, MinBinds: 1, Default: [2]int{2, 2}, Params: []ScreenParam{pTitle, {Name: "on_value", Label: "Value meaning running", Kind: "number", Default: 1}, {Name: "fault_value", Label: "Value meaning fault (optional)", Kind: "number"}, {Name: "on_label", Label: "Label when running", Kind: "text", Default: "RUNNING"}, {Name: "off_label", Label: "Label when stopped", Kind: "text", Default: "STOPPED"}, pFrame}},
	{Type: "valve", Label: "Valve", Group: "mimic", Help: "Open/closed state of one datastream: filled when open, outlined when closed, red on the fault value.", Binds: []string{"datastreams"}, MinBinds: 1, Default: [2]int{2, 2}, Params: []ScreenParam{pTitle, {Name: "open_value", Label: "Value meaning open", Kind: "number", Default: 1}, {Name: "open_label", Label: "Label when open", Kind: "text", Default: "OPEN"}, {Name: "closed_label", Label: "Label when closed", Kind: "text", Default: "CLOSED"}, {Name: "fault_value", Label: "Value meaning fault (optional)", Kind: "number"}, {Name: "orientation", Label: "Orientation", Kind: "select", Default: "horizontal", Options: []string{"horizontal", "vertical"}}, pFrame}},
	{Type: "pipe", Label: "Pipe", Group: "mimic", Help: "A pipe segment; with a bound datastream it shows flow (animated) while the value is above the flow threshold.", Binds: []string{"datastreams"}, Default: [2]int{3, 1}, Params: []ScreenParam{{Name: "orientation", Label: "Orientation", Kind: "select", Default: "horizontal", Options: []string{"horizontal", "vertical"}}, {Name: "thickness", Label: "Thickness (% of the widget)", Kind: "number", Default: 40}, {Name: "flow_min", Label: "Flow when value is above", Kind: "number", Default: 0}}},
	{Type: "instrument", Label: "Instrument", Group: "mimic", Help: "ISA-style instrument bubble with a tag (e.g. TT-101) and the latest value; threshold colours.", Binds: []string{"datastreams"}, MinBinds: 1, Default: [2]int{2, 2}, Params: []ScreenParam{{Name: "tag", Label: "Tag", Kind: "text"}, pDecimals, pWarn, pAction, pLowWarn, pLowAct, pFrame}},
	// Control (URS A-12, J-04): every send is a command on the bound device (reason, permission device.command, audit); connector registers/nodes must be writable.
	{Type: "setpoint", Label: "Setpoint", Group: "control", Help: "Sends a value to a device or a connector register as a command (reason, limits, read-back, audit). Bind the device; optionally a datastream that shows the current value.", Binds: []string{"devices", "datastreams"}, Default: [2]int{3, 2}, Params: []ScreenParam{{Name: "title", Label: "Title", Kind: "text"}, {Name: "key", Label: "Key (datastream key / register)", Kind: "text"}, {Name: "second_person", Label: "Needs a second person", Kind: "bool", Default: false}, pGroups, {Name: "method", Label: "Command method", Kind: "text", Default: "write"}, {Name: "min", Label: "Minimum", Kind: "number"}, {Name: "max", Label: "Maximum", Kind: "number"}, {Name: "step", Label: "Step", Kind: "number", Default: 0.1}, pDecimals, {Name: "unit", Label: "Unit", Kind: "text"}, {Name: "confirm", Label: "Ask for confirmation", Kind: "bool", Default: true}}},
	{Type: "switch", Label: "Switch", Group: "control", Help: "Turns something on or off with a command (true/false). Bind the device; optionally a datastream that shows the current state.", Binds: []string{"devices", "datastreams"}, Default: [2]int{3, 2}, Params: []ScreenParam{{Name: "title", Label: "Title", Kind: "text"}, {Name: "key", Label: "Key (datastream key / register)", Kind: "text"}, {Name: "second_person", Label: "Needs a second person", Kind: "bool", Default: false}, pGroups, {Name: "method", Label: "Command method", Kind: "text", Default: "write"}, {Name: "on_label", Label: "Label for on", Kind: "text", Default: "On"}, {Name: "off_label", Label: "Label for off", Kind: "text", Default: "Off"}, {Name: "confirm", Label: "Ask for confirmation", Kind: "bool", Default: true}}},
	{Type: "button", Label: "Button", Group: "control", Help: "Sends a fixed value or method as a command, e.g. a reset pulse.", Binds: []string{"devices"}, Default: [2]int{3, 2}, Params: []ScreenParam{{Name: "title", Label: "Title", Kind: "text"}, {Name: "key", Label: "Key (datastream key / register)", Kind: "text"}, {Name: "second_person", Label: "Needs a second person", Kind: "bool", Default: false}, pGroups, {Name: "method", Label: "Command method", Kind: "text", Default: "write"}, {Name: "value", Label: "Value to send", Kind: "text", Default: "1"}, {Name: "label", Label: "Button label", Kind: "text", Default: "Send"}, {Name: "confirm", Label: "Ask for confirmation", Kind: "bool", Default: true}}},
}

func hasBind(def *WidgetDef, b string) bool {
	for _, x := range def.Binds {
		if x == b {
			return true
		}
	}
	return false
}

func widgetDefOf(t string) *WidgetDef {
	for i := range widgetCatalog {
		if widgetCatalog[i].Type == t {
			return &widgetCatalog[i]
		}
	}
	return nil
}

var ranges = map[string]bool{"1h": true, "6h": true, "8h": true, "12h": true, "today": true, "24h": true, "7d": true, "30d": true, "90d": true}

type screenSpec struct {
	Version int    `json:"version"`
	Kind    string `json:"kind"`
	Layout  struct {
		Type      string `json:"type"` // grid (dashboard) | canvas (mimic: absolute positions in pixels)
		Columns   int    `json:"columns"`
		RowHeight int    `json:"row_height"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Bg        string `json:"bg"`   // canvas background colour (process pictures)
		Grid      int    `json:"grid"` // editor snap grid in pixels (process pictures)
	} `json:"layout"`
	Scheme   string `json:"scheme"` // state colours of process symbols: classic (WinCC-like) | isa101 (high-performance HMI)
	Range    string `json:"range"`
	RefreshS int    `json:"refresh_s"`
	Widgets  []struct {
		ID       string                   `json:"id"`
		Type     string                   `json:"type"`
		Geometry struct{ X, Y, W, H int } `json:"geometry"`
		Bindings map[string][]string      `json:"bindings"`
		Params   map[string]any           `json:"params"`
		Source   map[string]any           `json:"source"`  // dynamic data source (dashboard_filters.go)
		Follow   bool                     `json:"follow"`  // follows the dashboard filters
		State    string                   `json:"state"`   // dashboard state the widget belongs to (dashboard_aliases.go)
		Alias    map[string]any           `json:"alias"`   // entities from an alias, optionally a key filter
		Actions  []map[string]any         `json:"actions"` // row/marker/chart/widget click and buttons
	} `json:"widgets"`
	Elements []any            `json:"elements"` // drawing layer of process pictures (screen_elements.go)
	Filters  []map[string]any `json:"filters"`  // dashboard filter bar (dashboard_filters.go)
	// entity aliases, key filters and states (dashboard_aliases.go, 0.73.0)
	Aliases    []map[string]any `json:"aliases"`
	KeyFilters []map[string]any `json:"key_filters"`
	States     []map[string]any `json:"states"`
	// PublicAnnotations lets viewers of a public link (and displays) see the annotations of the charts; off by default
	// because annotation texts are internal notes (0.72.0).
	PublicAnnotations bool `json:"public_annotations"`
	// place in the picture tree of the operator view and faceplate type (operator.go, spec 036)
	Nav       map[string]any `json:"nav"`
	Faceplate map[string]any `json:"faceplate"`
}

// validateScreenSpec normalizes a screen (dashboard) spec and returns the canonical document.
func validateScreenSpec(raw json.RawMessage) (map[string]any, error) {
	spec, _, err := validateScreenSpecW(raw)
	return spec, err
}

// validateScreenSpecW also returns warnings: settings the catalogue does not know, which are dropped (spec 002 FR-006).
func validateScreenSpecW(raw json.RawMessage) (map[string]any, []string, error) {
	var warnings []string
	var sp screenSpec
	if len(raw) == 0 {
		return nil, nil, errors.New("spec required")
	}
	if err := json.Unmarshal(raw, &sp); err != nil {
		return nil, nil, fmt.Errorf("spec: %w", err)
	}
	if sp.Version == 0 {
		sp.Version = 1
	}
	if sp.Version != 1 {
		return nil, nil, errors.New("unsupported spec version")
	}
	if sp.Kind == "" {
		sp.Kind = "dashboard"
	}
	if sp.Kind != "dashboard" && sp.Kind != "mimic" {
		return nil, nil, errors.New("kind must be dashboard or mimic")
	}
	if sp.Layout.Type == "" {
		if sp.Kind == "mimic" {
			sp.Layout.Type = "canvas"
		} else {
			sp.Layout.Type = "grid"
		}
	}
	canvas := sp.Layout.Type == "canvas"
	if !canvas && sp.Layout.Type != "grid" {
		return nil, nil, errors.New("layout.type must be grid or canvas")
	}
	if canvas {
		if sp.Layout.Width == 0 {
			sp.Layout.Width = 1600
		}
		if sp.Layout.Height == 0 {
			sp.Layout.Height = 900
		}
		if sp.Layout.Width < 600 || sp.Layout.Width > 4000 || sp.Layout.Height < 300 || sp.Layout.Height > 3000 {
			return nil, nil, errors.New("layout.width must be 600..4000 and layout.height 300..3000")
		}
	} else {
		if sp.Layout.Columns == 0 {
			sp.Layout.Columns = 12
		}
		if sp.Layout.Columns != 12 {
			return nil, nil, errors.New("layout.columns must be 12")
		}
		if sp.Layout.RowHeight == 0 {
			sp.Layout.RowHeight = 96
		}
		if sp.Layout.RowHeight < 60 || sp.Layout.RowHeight > 240 {
			return nil, nil, errors.New("layout.row_height must be 60..240")
		}
	}
	if sp.Range == "" {
		sp.Range = "24h"
	}
	if !ranges[sp.Range] {
		return nil, nil, errors.New("range must be 1h, 6h, 24h, 7d, 30d or 90d")
	}
	if sp.RefreshS == 0 {
		sp.RefreshS = 30
	}
	if sp.RefreshS < 5 || sp.RefreshS > 3600 {
		return nil, nil, errors.New("refresh_s must be 5..3600")
	}
	if len(sp.Elements) > 0 && !canvas {
		return nil, nil, errors.New("drawing elements need the canvas layout (kind mimic)")
	}
	aliases, keyFilters, states, aIDs, fIDs, sIDs, err := validateDashModel(sp.Aliases, sp.KeyFilters, sp.States)
	if err != nil {
		return nil, nil, err
	}
	if canvas && len(states) > 0 {
		return nil, nil, errNoStates
	}
	// an empty dashboard is allowed: "Dashboard — empty" starts with no widgets, and removing the last widget saves
	if len(sp.Widgets) > 60 {
		return nil, nil, errors.New("at most 60 widgets")
	}
	ids := map[string]bool{}
	widgets := make([]map[string]any, 0, len(sp.Widgets))
	for i, w := range sp.Widgets {
		def := widgetDefOf(w.Type)
		if def == nil {
			return nil, nil, fmt.Errorf("widget %d: unknown type %q", i+1, w.Type)
		}
		id := w.ID
		if id == "" {
			id = "w" + strconv.Itoa(i+1)
		}
		if !widgetIDRe.MatchString(id) || ids[id] {
			return nil, nil, fmt.Errorf("widget %d: id must be unique and simple", i+1)
		}
		ids[id] = true
		g := w.Geometry
		if canvas {
			if g.W == 0 {
				g.W, g.H = def.Default[0]*100, def.Default[1]*100
			}
			if g.H == 0 {
				g.H = def.Default[1] * 100
			}
			if g.X < 0 || g.Y < 0 || g.W < 20 || g.H < 20 || g.X+g.W > sp.Layout.Width || g.Y+g.H > sp.Layout.Height {
				return nil, nil, fmt.Errorf("widget %d: geometry outside the %d×%d canvas", i+1, sp.Layout.Width, sp.Layout.Height)
			}
		} else {
			if g.W == 0 {
				g.W, g.H = def.Default[0], def.Default[1]
			}
			if g.H == 0 {
				g.H = def.Default[1]
			}
			if g.X < 0 || g.X > 11 || g.Y < 0 || g.Y > 999 || g.W < 1 || g.W > 12 || g.H < 1 || g.H > 40 || g.X+g.W > 12 {
				return nil, nil, fmt.Errorf("widget %d: geometry out of the 12-column grid", i+1)
			}
		}
		binds := map[string][]string{}
		for _, b := range def.Binds {
			list := w.Bindings[b]
			if len(list) > 24 {
				return nil, nil, fmt.Errorf("widget %d: at most 24 %s", i+1, b)
			}
			out := []string{}
			for _, v := range list {
				if !uuidRe.MatchString(v) {
					return nil, nil, fmt.Errorf("widget %d: %s must be ids", i+1, b)
				}
				out = append(out, v)
			}
			// Widgets may be saved before their datastreams are chosen (the renderer shows a hint); min_binds stays a UI hint.
			binds[b] = out
		}
		warnings = append(warnings, unknownParams(i, def, w.Params)...)
		params, err := validateParams(i, def, w.Params)
		if err != nil {
			return nil, nil, err
		}
		for _, d := range def.Params {
			if f, _ := params[d.Name].(string); d.Kind == "keyfilter" && f != "" && !fIDs[f] {
				return nil, nil, fmt.Errorf("widget %d: %s: the dashboard has no key filter %q", i+1, d.Name, f)
			}
		}
		wm := map[string]any{"id": id, "type": w.Type, "geometry": map[string]int{"x": g.X, "y": g.Y, "w": g.W, "h": g.H}, "bindings": binds, "params": params}
		if w.Source != nil {
			if !hasBind(def, "datastreams") {
				return nil, nil, fmt.Errorf("widget %d: %s has no datastreams to select by a query", i+1, w.Type)
			}
			src, err := validateSource(i, w.Type, w.Source)
			if err != nil {
				return nil, nil, err
			}
			wm["source"] = src
		}
		if w.Follow {
			wm["follow"] = true
		}
		if w.Alias != nil {
			if w.Source != nil {
				return nil, nil, fmt.Errorf("widget %d: either a query source or an alias, not both", i+1)
			}
			al, err := validateWidgetAlias(i, def, w.Alias, aIDs, fIDs)
			if err != nil {
				return nil, nil, err
			}
			wm["alias"] = al // an entity table or selector without an alias stays empty while it is being set up
		}
		if w.State != "" {
			if !sIDs[w.State] {
				return nil, nil, fmt.Errorf("widget %d: the dashboard has no state %q", i+1, w.State)
			}
			if w.State != states[0]["id"] { // absent = the root state
				wm["state"] = w.State
			}
		}
		if len(w.Actions) > 0 {
			acts, err := validateActions(i, w.Actions, sIDs)
			if err != nil {
				return nil, nil, err
			}
			wm["actions"] = acts
		}
		widgets = append(widgets, wm)
	}
	layout := map[string]any{"type": "grid", "columns": 12, "row_height": sp.Layout.RowHeight}
	out := map[string]any{"version": 1, "kind": sp.Kind, "layout": layout, "range": sp.Range, "refresh_s": sp.RefreshS, "widgets": widgets}
	if sp.PublicAnnotations {
		out["public_annotations"] = true
	}
	if sp.Nav != nil {
		nav, err := validateNav(sp.Nav)
		if err != nil {
			return nil, nil, err
		}
		out["nav"] = nav
	}
	if sp.Faceplate != nil {
		fp, err := validateFaceplate(sp.Faceplate)
		if err != nil {
			return nil, nil, err
		}
		out["faceplate"] = fp
	}
	if len(sp.Filters) > 0 {
		fs, err := validateFilters(sp.Filters)
		if err != nil {
			return nil, nil, err
		}
		out["filters"] = fs
	}
	if len(aliases) > 0 {
		out["aliases"] = aliases
	}
	if len(keyFilters) > 0 {
		out["key_filters"] = keyFilters
	}
	if len(states) > 0 {
		out["states"] = states
	}
	if canvas {
		lay := map[string]any{"type": "canvas", "width": sp.Layout.Width, "height": sp.Layout.Height}
		if sp.Layout.Bg != "" {
			if !colorRe.MatchString(sp.Layout.Bg) {
				return nil, nil, errors.New("layout.bg must be a colour like #e5e7eb")
			}
			lay["bg"] = strings.ToLower(sp.Layout.Bg)
		}
		if sp.Layout.Grid != 0 {
			if sp.Layout.Grid < 1 || sp.Layout.Grid > 100 {
				return nil, nil, errors.New("layout.grid must be 1..100")
			}
			lay["grid"] = sp.Layout.Grid
		}
		out["layout"] = lay
		switch sp.Scheme {
		case "":
		case "classic", "isa101":
			out["scheme"] = sp.Scheme
		default:
			return nil, nil, errors.New("scheme must be classic or isa101")
		}
		elems, err := validateElements(sp.Elements, sp.Layout.Width, sp.Layout.Height)
		if err != nil {
			return nil, nil, err
		}
		if len(elems) > 0 {
			out["elements"] = elems
		}
	}
	return out, warnings, nil
}

func validateParams(i int, def *WidgetDef, src map[string]any) (map[string]any, error) {
	p := map[string]any{}
	if src == nil {
		src = map[string]any{}
	}
	for _, d := range def.Params {
		v, ok := src[d.Name]
		if !ok || v == nil || v == "" {
			// Defaults of data settings are stored; appearance/series/time/action settings stay absent until set,
			// so older dashboards keep their canonical form (spec 002 FR-007).
			if d.Default != nil && (d.Group == "" || d.Group == "data") {
				p[d.Name] = d.Default
			}
			continue
		}
		switch d.Kind {
		case "text", "multiline":
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("widget %d: %s must be text", i+1, d.Name)
			}
			if len(s) > 4000 {
				return nil, fmt.Errorf("widget %d: %s too long", i+1, d.Name)
			}
			p[d.Name] = strings.TrimSpace(s)
		case "number":
			var f float64
			switch x := v.(type) {
			case float64:
				f = x
			case string:
				var err error
				if f, err = strconv.ParseFloat(strings.TrimSpace(x), 64); err != nil {
					return nil, fmt.Errorf("widget %d: %s must be a number", i+1, d.Name)
				}
			default:
				return nil, fmt.Errorf("widget %d: %s must be a number", i+1, d.Name)
			}
			if f < -1e12 || f > 1e12 || (d.Min != nil && f < *d.Min) || (d.Max != nil && f > *d.Max) {
				return nil, fmt.Errorf("widget %d: %s out of range", i+1, d.Name)
			}
			p[d.Name] = f
		case "bool":
			b, ok := v.(bool)
			if !ok {
				return nil, fmt.Errorf("widget %d: %s must be true or false", i+1, d.Name)
			}
			p[d.Name] = b
		case "camera":
			s, _ := v.(string)
			if !uuidRe.MatchString(s) {
				return nil, fmt.Errorf("widget %d: %s must be a camera id", i+1, d.Name)
			}
			p[d.Name] = s
		case "select":
			s, _ := v.(string)
			okOpt := false
			for _, o := range d.Options {
				if o == s {
					okOpt = true
				}
			}
			if !okOpt {
				return nil, fmt.Errorf("widget %d: %s must be one of %s", i+1, d.Name, strings.Join(d.Options, ", "))
			}
			p[d.Name] = s
		case "keyfilter":
			s, _ := v.(string)
			if !widgetIDRe.MatchString(s) {
				return nil, fmt.Errorf("widget %d: %s must be the id of a key filter", i+1, d.Name)
			}
			p[d.Name] = s
		case "color", "icon", "datetime", "series", "ranges", "multi":
			val, err := validateRichParam(d, v)
			if err != nil {
				return nil, fmt.Errorf("widget %d: %s %v", i+1, d.Name, err)
			}
			p[d.Name] = val
		case "list":
			var list []string
			switch x := v.(type) {
			case []any:
				for _, e := range x {
					s, ok := e.(string)
					if !ok {
						return nil, fmt.Errorf("widget %d: %s must be a list of strings", i+1, d.Name)
					}
					list = append(list, strings.TrimSpace(s))
				}
			case string:
				for _, part := range strings.Split(x, ",") {
					if t := strings.TrimSpace(part); t != "" {
						list = append(list, t)
					}
				}
			default:
				return nil, fmt.Errorf("widget %d: %s must be a list", i+1, d.Name)
			}
			if len(list) > 40 {
				return nil, fmt.Errorf("widget %d: %s too long", i+1, d.Name)
			}
			if d.Name == "scada_groups" {
				gs, err := validGroupKeys(list)
				if err != nil {
					return nil, fmt.Errorf("widget %d: %v", i+1, err)
				}
				list = gs
			}
			for _, s := range list {
				if len(s) > 120 {
					return nil, fmt.Errorf("widget %d: %s entry too long", i+1, d.Name)
				}
			}
			if list == nil {
				list = []string{}
			}
			p[d.Name] = list
		}
	}
	return p, nil
}

func slugify(s string) string {
	repl := map[rune]rune{'á': 'a', 'ä': 'a', 'č': 'c', 'ď': 'd', 'é': 'e', 'ě': 'e', 'í': 'i', 'ľ': 'l', 'ĺ': 'l', 'ň': 'n', 'ó': 'o', 'ô': 'o', 'ö': 'o', 'ŕ': 'r', 'ř': 'r', 'š': 's', 'ť': 't', 'ú': 'u', 'ů': 'u', 'ü': 'u', 'ý': 'y', 'ž': 'z'}
	var b strings.Builder
	last := '-'
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if x, ok := repl[r]; ok {
			r = x
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			last = r
		} else if last != '-' {
			b.WriteRune('-')
			last = '-'
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 63 {
		out = out[:63]
	}
	return out
}

func (s *Server) listDashboards(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT id::text, slug, name, version, is_default, created_at, jsonb_array_length(spec->'widgets') FROM core.dashboard WHERE org_id = $1::uuid AND valid_to IS NULL ORDER BY is_default DESC, name`, org)
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, slug, name string
		var version, widgets int
		var def bool
		var created time.Time
		if rows.Scan(&id, &slug, &name, &version, &def, &created, &widgets) == nil {
			out = append(out, map[string]any{"id": id, "slug": slug, "name": name, "version": version, "is_default": def, "created_at": created, "widgets": widgets})
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) dashboardCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"widgets": widgetCatalog, "ranges": []string{"1h", "6h", "8h", "12h", "today", "24h", "7d", "30d", "90d"}, "format": "screen/v1", "format_minor": 2, "layouts": []string{"grid"},
		"filter_kinds": filterKinds, "source_limits": map[string]int{"tables": maxSourceRows, "other": maxSourceSeries},
		"alias_kinds": aliasKinds, "action_events": sortedKeys(actionEvents), "limits": map[string]int{"aliases": maxAliases, "key_filters": maxKeyFilters, "states": maxStates, "actions": maxActions, "entities": maxSourceRows, "depth": maxTreeDepth}})
}

func (s *Server) getDashboard(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	slug := r.PathValue("slug")
	var id, name, checksum string
	var version int
	var def bool
	var spec json.RawMessage
	var created time.Time
	q := `SELECT id::text, slug, name, version, is_default, spec, checksum, created_at FROM core.dashboard WHERE org_id = $1::uuid AND valid_to IS NULL AND slug = $2`
	if slug == "default" {
		q = `SELECT id::text, slug, name, version, is_default, spec, checksum, created_at FROM core.dashboard WHERE org_id = $1::uuid AND valid_to IS NULL AND ($2 = '' OR is_default) ORDER BY is_default DESC, created_at LIMIT 1`
		slug = ""
	}
	err := s.pool.QueryRow(r.Context(), q, org, slug).Scan(&id, &slug, &name, &version, &def, &spec, &checksum, &created)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, 404, "dashboard not found")
		return
	}
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "slug": slug, "name": name, "version": version, "is_default": def, "spec": spec, "checksum": checksum, "created_at": created})
}

func (s *Server) saveDashboard(w http.ResponseWriter, r *http.Request, org, slug string, create bool) {
	p := PrincipalFrom(r.Context())
	var in struct {
		Slug      string          `json:"slug"`
		Name      string          `json:"name"`
		Spec      json.RawMessage `json:"spec"`
		IsDefault *bool           `json:"is_default"`
		Reason    string          `json:"reason"`
	}
	if err := decodeJSON(r, &in, 512<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !reasonOf(w, in.Reason) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if create {
		if in.Slug == "" {
			in.Slug = slugify(in.Name)
		}
		slug = in.Slug
		if in.Name == "" || len(in.Name) > 128 {
			writeErr(w, 400, "name required (max 128)")
			return
		}
	}
	if !dashSlugRe.MatchString(slug) || slug == "default" || slug == "catalog" || slug == "starter" {
		writeErr(w, 400, "slug must be lowercase letters, digits and dashes")
		return
	}
	spec, warnings, err := validateScreenSpecW(in.Spec)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if nav, _ := spec["nav"].(map[string]any); nav != nil {
		if parent, _ := nav["parent"].(string); parent != "" {
			if err := s.checkNavParent(r.Context(), org, slug, parent); err != nil {
				writeErr(w, 400, err.Error())
				return
			}
		}
	}
	if fp, _ := spec["faceplate"].(map[string]any); fp != nil {
		var n int
		if err := s.pool.QueryRow(r.Context(), `SELECT count(*) FROM core.asset WHERE id = $1::uuid AND org_id = $2::uuid`, fp["asset"], org).Scan(&n); err != nil || n == 0 {
			writeErr(w, 400, "faceplate.asset: no such asset")
			return
		}
	}
	if msg := s.camerasVisible(r.Context(), org, specCameras(spec)); msg != "" {
		writeErr(w, 403, msg)
		return
	}
	if warnings == nil {
		warnings = []string{}
	}
	specJSON, _ := json.Marshal(spec)
	sum := sha256.Sum256(specJSON)
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeErr(w, 500, "begin failed")
		return
	}
	defer tx.Rollback(r.Context())
	var prevName, prevID string
	var prevVersion int
	var prevDefault bool
	err = tx.QueryRow(r.Context(), `SELECT id::text, name, version, is_default FROM core.dashboard WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL`, org, slug).Scan(&prevID, &prevName, &prevVersion, &prevDefault)
	exists := err == nil
	if create && exists {
		writeErr(w, 409, "a dashboard with this slug exists")
		return
	}
	if !create && !exists {
		writeErr(w, 404, "dashboard not found")
		return
	}
	if in.Name == "" {
		in.Name = prevName
	}
	def := prevDefault
	if in.IsDefault != nil {
		def = *in.IsDefault
	}
	if exists {
		if _, err := tx.Exec(r.Context(), `UPDATE core.dashboard SET valid_to = now() WHERE id = $1::uuid`, prevID); err != nil {
			writeErr(w, 500, "supersede failed")
			return
		}
	}
	if def {
		if _, err := tx.Exec(r.Context(), `UPDATE core.dashboard SET is_default = false WHERE org_id = $1::uuid AND valid_to IS NULL AND is_default`, org); err != nil {
			writeErr(w, 500, "default update failed")
			return
		}
	}
	var id string
	version := prevVersion + 1
	if err := tx.QueryRow(r.Context(), `INSERT INTO core.dashboard (org_id, slug, name, version, spec, checksum, is_default, created_by, reason) VALUES ($1::uuid, $2, $3, $4, $5::jsonb, $6, $7, NULLIF($8,'')::uuid, $9) RETURNING id::text`,
		org, slug, in.Name, version, string(specJSON), hex.EncodeToString(sum[:]), def, p.UserID, in.Reason).Scan(&id); err != nil {
		writeErr(w, 500, "insert failed")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, 500, "commit failed")
		return
	}
	action := "dashboard.update"
	if create {
		action = "dashboard.create"
	}
	var old any
	if exists {
		old = map[string]any{"id": prevID, "version": prevVersion}
	}
	s.audit(r, action, "dashboard", id, old, map[string]any{"slug": slug, "name": in.Name, "version": version, "widgets": len(spec["widgets"].([]map[string]any)), "elements": len(elementsOf(spec)), "is_default": def}, in.Reason)
	code := 200
	if create {
		code = 201
	}
	writeJSON(w, code, map[string]any{"id": id, "slug": slug, "name": in.Name, "version": version, "is_default": def, "spec": spec, "warnings": warnings})
}

func (s *Server) createDashboard(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	s.saveDashboard(w, r, org, "", true)
}

func (s *Server) updateDashboard(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	s.saveDashboard(w, r, org, r.PathValue("slug"), false)
}

func (s *Server) deleteDashboard(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	reason := r.URL.Query().Get("reason")
	if !reasonOf(w, reason) {
		return
	}
	slug := r.PathValue("slug")
	var id string
	if err := s.pool.QueryRow(r.Context(), `UPDATE core.dashboard SET valid_to = now(), is_default = false WHERE org_id = $1::uuid AND slug = $2 AND valid_to IS NULL RETURNING id::text`, org, slug).Scan(&id); err != nil {
		writeErr(w, 404, "dashboard not found")
		return
	}
	s.audit(r, "dashboard.retire", "dashboard", id, map[string]any{"slug": slug}, nil, reason)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// latestValues: GET /api/v1/datastreams/latest?ids=a,b,c
func (s *Server) latestValues(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	raw := strings.Split(r.URL.Query().Get("ids"), ",")
	ids := make([]string, 0, len(raw))
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if uuidRe.MatchString(id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 || len(ids) > 50 {
		writeErr(w, 400, "ids: 1..50 datastream ids")
		return
	}
	// the newest row of each datastream through the (datastream_id, measured_at DESC) index — DISTINCT ON over all
	// rows of 400 days sorted every reading of the 50 datastreams (seconds with per-second data, 0.87.4)
	rows, err := s.pool.Query(r.Context(), `
		SELECT ds.id::text, o.measured_at, o.value::float8, o.value_json, o.quality, ds.unit, ds.key, a.name,
		       COALESCE(EXTRACT(EPOCH FROM ds.expected_interval), 0)::float8
		FROM unnest($1::uuid[]) AS x(id)
		JOIN core.datastream ds ON ds.id = x.id
		JOIN core.asset a ON a.id = ds.asset_id
		JOIN LATERAL (SELECT ob.measured_at, ob.value, ob.value_json, ob.quality FROM core.observation ob
			WHERE ob.datastream_id = x.id AND ($2 = '' OR ob.org_id = $2::uuid) AND ob.measured_at > now() - interval '400 days'
			ORDER BY ob.measured_at DESC LIMIT 1) o ON true`, ids, orgOf(p))
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := map[string]any{}
	now := time.Now()
	for rows.Next() {
		var id, quality, unit, key, asset string
		var at time.Time
		var val *float64
		var vj json.RawMessage
		var expected float64
		if rows.Scan(&id, &at, &val, &vj, &quality, &unit, &key, &asset, &expected) == nil {
			l := map[string]any{"measured_at": at, "value": val, "value_json": vj, "unit": unit, "key": key, "asset": asset}
			applyLive(l, quality, at, expected, now) // "stale" is computed, never stored (quality.go)
			out[id] = l
		}
	}
	writeJSON(w, 200, out)
}

// bucketedObservations serves ?bucket=SECONDS for the observations endpoint: avg/min/max/count per bucket.
func (s *Server) bucketedObservations(w http.ResponseWriter, r *http.Request, dsID string, from, to time.Time, bucket int) {
	p := PrincipalFrom(r.Context())
	if bucket < 1 || bucket > 86400*7 {
		writeErr(w, 400, "bucket must be 1..604800 seconds")
		return
	}
	rows, err := s.pool.Query(r.Context(), `
		SELECT date_bin(make_interval(secs => $4), measured_at, $2) AS t, avg(value)::float8, min(value)::float8, max(value)::float8, count(*), bool_or(quality NOT IN ('ok', 'backfilled'))
		FROM core.observation WHERE datastream_id = $1::uuid AND ($5 = '' OR org_id = $5::uuid) AND measured_at >= $2 AND measured_at < $3 AND value IS NOT NULL
		GROUP BY t ORDER BY t LIMIT 5000`, dsID, from, to, bucket, orgOf(p))
	if err != nil {
		writeErr(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var t time.Time
		var avg, min, max *float64
		var n int64
		var flagged bool
		if rows.Scan(&t, &avg, &min, &max, &n, &flagged) == nil {
			out = append(out, map[string]any{"t": t, "avg": avg, "min": min, "max": max, "n": n, "flagged": flagged})
		}
	}
	writeJSON(w, 200, map[string]any{"bucket_s": bucket, "from": from, "to": to, "points": out})
}

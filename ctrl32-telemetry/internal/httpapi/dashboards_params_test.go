package httpapi

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func richDef(kind string, opts ...string) ScreenParam {
	return ScreenParam{Name: "x", Kind: kind, Options: opts}
}

func TestRichParamKinds(t *testing.T) {
	ok := []struct {
		kind string
		opts []string
		in   string
		want string
	}{
		{"color", nil, `"#0F172A"`, `"#0f172a"`},
		{"color", nil, `"#abc"`, `"#abc"`},
		{"icon", nil, `"activity"`, `"activity"`},
		{"datetime", nil, `"2026-09-25T10:00:00+02:00"`, `"2026-09-25T08:00:00Z"`},
		{"multi", []string{"min", "max"}, `["max","min"]`, `["max","min"]`},
		{"series", nil, `[{"label":"T","color":"#f00","unit":"°C","decimals":1,"style":"step","axis":"right","points":true,"fill":false}]`,
			`[{"axis":"right","color":"#f00","decimals":1,"label":"T","points":true,"style":"step","unit":"°C"}]`},
		{"ranges", nil, `[{"op":"between","v":2,"v2":8,"color":"#16a34a"},{"op":">","v":8,"target":"background","color":"#dc2626"}]`,
			`[{"color":"#16a34a","op":"between","target":"value","v":2,"v2":8},{"color":"#dc2626","op":"\u003e","target":"background","v":8}]`},
	}
	for _, c := range ok {
		var v any
		_ = json.Unmarshal([]byte(c.in), &v)
		got, err := validateRichParam(richDef(c.kind, c.opts...), v)
		if err != nil {
			t.Fatalf("%s %s: %v", c.kind, c.in, err)
		}
		b, _ := json.Marshal(got)
		if string(b) != c.want {
			t.Errorf("%s %s: got %s want %s", c.kind, c.in, b, c.want)
		}
	}

	bad := []struct {
		kind string
		in   string
	}{
		{"color", `"red"`},
		{"color", `"none"`},
		{"color", `"#12345"`},
		{"color", `"expression(alert(1))"`},
		{"icon", `"<svg>"`},
		{"icon", `"not-an-icon"`},
		{"datetime", `"yesterday"`},
		{"multi", `["min","median"]`},
		{"multi", `"min"`},
		{"series", `[{"axis":"middle"}]`},
		{"series", `[{"style":"pie"}]`},
		{"series", `[{"decimals":7}]`},
		{"series", `[{"color":"blue"}]`},
		{"series", `[{"label":"` + strings.Repeat("x", 65) + `"}]`},
		{"series", `[{"unit":"` + strings.Repeat("x", 17) + `"}]`},
		{"series", `[{"points":"yes"}]`},
		{"series", `[{"script":"x"}]`},
		{"series", `[` + strings.TrimSuffix(strings.Repeat(`{},`, 25), ",") + `]`},
		{"ranges", `[{"op":"~","v":1,"color":"#fff"}]`},
		{"ranges", `[{"op":">","v":"a","color":"#fff"}]`},
		{"ranges", `[{"op":"between","v":1,"color":"#fff"}]`},
		{"ranges", `[{"op":">","v":1,"target":"border","color":"#fff"}]`},
		{"ranges", `[{"op":">","v":1,"color":"url(x)"}]`},
		{"ranges", `[` + strings.TrimSuffix(strings.Repeat(`{"op":">","v":1,"color":"#fff"},`, 9), ",") + `]`},
	}
	for _, c := range bad {
		var v any
		if err := json.Unmarshal([]byte(c.in), &v); err != nil {
			t.Fatalf("bad fixture %s", c.in)
		}
		if _, err := validateRichParam(richDef(c.kind, "min", "max"), v); err == nil {
			t.Errorf("%s %s: accepted", c.kind, c.in)
		}
	}
}

func TestCatalogueCommonParams(t *testing.T) {
	if n := len(widgetCatalog); n < 44 {
		t.Fatalf("catalogue has %d types, want >= 44 (SC-001)", n)
	}
	for _, d := range widgetCatalog {
		if len(d.Params) < 15 {
			t.Errorf("%s: %d settings, want >= 15 (SC-002)", d.Type, len(d.Params))
		}
		seen := map[string]bool{}
		for _, p := range d.Params {
			if seen[p.Name] {
				t.Errorf("%s: duplicate param %s", d.Type, p.Name)
			}
			seen[p.Name] = true
		}
		for _, name := range []string{"bg_color", "show_title", "fullscreen", "click"} {
			if !seen[name] {
				t.Errorf("%s: missing common param %s", d.Type, name)
			}
		}
		if timeWidgets[d.Type] && !seen["series"] {
			t.Errorf("%s: time widget without series settings", d.Type)
		}
	}
}

func TestParamsBoundsDefaultsWarnings(t *testing.T) {
	spec := func(params string) json.RawMessage {
		return json.RawMessage(`{"version":1,"widgets":[{"id":"w1","type":"value","x":0,"y":0,"w":3,"h":2,"bindings":{"datastreams":["00000000-0000-0000-0000-000000000001"]},"params":` + params + `}]}`)
	}
	out, warn, err := validateScreenSpecW(spec(`{"bg_color":"#0F172A","title_size":20,"legacy_thing":1,"zzz":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(warn) != 2 || !strings.Contains(warn[0], "legacy_thing") || !strings.Contains(warn[1], "zzz") {
		t.Fatalf("warnings %v", warn)
	}
	b, _ := json.Marshal(out)
	s := string(b)
	if strings.Contains(s, "legacy_thing") || !strings.Contains(s, `"bg_color":"#0f172a"`) {
		t.Fatalf("canonical %s", s)
	}
	// Appearance defaults are not written into the stored spec (FR-007).
	if strings.Contains(s, "show_title") || strings.Contains(s, "fullscreen") {
		t.Fatalf("appearance defaults stored: %s", s)
	}
	for _, p := range []string{`{"title_size":7}`, `{"title_size":49}`, `{"border_width":9}`, `{"text_color":"red"}`, `{"title_icon":"nope"}`, `{"click":"script"}`} {
		if _, _, err := validateScreenSpecW(spec(p)); err == nil {
			t.Errorf("%s accepted", p)
		}
	}
}

// The spec 002 part of the showcase passes validation without warnings and, with the original showcase types,
// covers every widget type of the catalogue except the process symbols and the older control widgets (FR-010).
// An empty dashboard (grid or canvas) is valid: "Dashboard — empty" and saving after removing every widget.
func TestEmptyDashboardValid(t *testing.T) {
	for _, js := range []string{`{"version":1,"kind":"dashboard","layout":{"type":"grid","columns":12,"row_height":96},"widgets":[]}`, `{"version":1}`,
		`{"version":1,"kind":"mimic","layout":{"type":"canvas","width":1600,"height":900},"widgets":[],"elements":[]}`} {
		if _, err := validateScreenSpec(json.RawMessage(js)); err != nil {
			t.Errorf("%s: %v", js, err)
		}
	}
}

func TestShowcaseMoreValid(t *testing.T) {
	w := func(id, typ string, x, y, wd, h int, binds map[string][]string, params map[string]any) map[string]any {
		if binds == nil {
			binds = map[string][]string{}
		}
		return map[string]any{"id": id, "type": typ, "geometry": map[string]int{"x": x, "y": y, "w": wd, "h": h}, "bindings": binds, "params": params}
	}
	dsb := func(ids []string) map[string][]string { return map[string][]string{"datastreams": ids} }
	ds := []string{tagA, tagA, tagA}
	widgets := showcaseMore(w, dsb, ds, ds[:1])
	raw, _ := json.Marshal(map[string]any{"version": 1, "widgets": widgets})
	_, warn, err := validateScreenSpecW(raw)
	if err != nil || len(warn) > 0 {
		t.Fatalf("err %v warnings %v", err, warn)
	}
	have := setOf("value", "status", "gauge", "chart", "bars", "donut", "heatmap", "map", "table", "alarms", "devices", "text")
	for _, x := range widgets {
		have[x["type"].(string)] = true
	}
	skip := setOf("tank", "pump", "valve", "pipe", "instrument", "setpoint", "switch", "button", "camera", "camera_pin") // camera, camera_pin: need a configured camera
	for _, d := range widgetCatalog {
		if !have[d.Type] && !skip[d.Type] {
			t.Errorf("showcase lacks %s", d.Type)
		}
	}
}

// Every setting the catalogue offers is read by the browser code that renders widgets — no setting without effect
// ("widgets should be fully configurable", spec 002 FR-001).
func TestEveryParamIsRendered(t *testing.T) {
	var js strings.Builder
	for _, f := range []string{"static/widgets.js", "static/widgets-more.js"} {
		b, err := static.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		js.Write(b)
	}
	src := js.String()
	for _, d := range widgetCatalog {
		for _, p := range d.Params {
			re := regexp.MustCompile(`(\bp|params)\.` + p.Name + `\b|'` + p.Name + `'`)
			if !re.MatchString(src) {
				t.Errorf("%s: setting %s is not used by any renderer", d.Type, p.Name)
			}
		}
	}
}

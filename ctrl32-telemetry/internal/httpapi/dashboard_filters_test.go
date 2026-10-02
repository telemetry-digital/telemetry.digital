package httpapi

import (
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

const tagB = "0198a1b2-0000-7000-8000-000000000002"

func specWith(t *testing.T, extra string) (map[string]any, []string, error) {
	t.Helper()
	raw := `{"version":1,"widgets":[{"id":"t","type":"overview_table","geometry":{"x":0,"y":0,"w":12,"h":5},"bindings":{"datastreams":[]},"params":{}` + extra + `}]}`
	return validateScreenSpecW(json.RawMessage(raw))
}

func TestFiltersValidated(t *testing.T) {
	raw := `{"version":1,"widgets":[],"filters":[{"kind":"site","multi":true,"public":true,"default":["` + tagA + `","` + tagB + `"]},{"kind":"attribute","key":"storage_class","label":" Storage class ","default":"2-8C"},{"kind":"search","default":" fridge "},{"kind":"range","default":"7d","public":false}]}`
	spec, _, err := validateScreenSpecW(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(spec["filters"])
	want := `[{"default":["` + tagA + `","` + tagB + `"],"kind":"site","multi":true,"public":true},{"default":["2-8C"],"key":"storage_class","kind":"attribute","label":"Storage class"},{"default":"fridge","kind":"search"},{"default":"7d","kind":"range"}]`
	if string(got) != want {
		t.Fatalf("canonical filters\n got %s\nwant %s", got, want)
	}
	for _, bad := range []string{
		`[{"kind":"customer"}]`,
		`[{"kind":"attribute"}]`,
		`[{"kind":"attribute","key":"a b"}]`,
		`[{"kind":"site"},{"kind":"site"}]`,
		`[{"kind":"site","default":["x"]}]`,
		`[{"kind":"quantity","default":["a","b"]}]`,
		`[{"kind":"search","multi":true}]`,
		`[{"kind":"range","default":"2h"}]`,
		`[{"kind":"site","script":"x"}]`,
		`[{"kind":"site","public":"yes"}]`,
		`[{"kind":"site"},{"kind":"device"},{"kind":"quantity"},{"kind":"asset_type"},{"kind":"search"},{"kind":"range"},{"kind":"attribute","key":"a"},{"kind":"attribute","key":"b"},{"kind":"attribute","key":"c"}]`,
	} {
		if _, _, err := validateScreenSpecW(json.RawMessage(`{"version":1,"widgets":[],"filters":` + bad + `}`)); err == nil {
			t.Errorf("accepted filters %s", bad)
		}
	}
	// two attribute filters with different keys are fine
	if _, _, err := validateScreenSpecW(json.RawMessage(`{"version":1,"widgets":[],"filters":[{"kind":"attribute","key":"a"},{"kind":"attribute","key":"b"}]}`)); err != nil {
		t.Error(err)
	}
}

func TestSourceAndFollow(t *testing.T) {
	spec, warn, err := specWith(t, `,"source":{"site":["`+tagA+`"],"quantity":["temperature","energy*"],"attr":{"storage_class":"2-8C"},"name":" Fridge* "},"follow":true`)
	if err != nil || len(warn) > 0 {
		t.Fatal(err, warn)
	}
	w := spec["widgets"].([]map[string]any)[0]
	got, _ := json.Marshal(w["source"])
	if string(got) != `{"attr":{"storage_class":["2-8C"]},"limit":500,"name":"Fridge*","quantity":["temperature","energy*"],"site":["`+tagA+`"]}` {
		t.Fatalf("source %s", got)
	}
	if w["follow"] != true {
		t.Fatal("follow lost")
	}
	// an empty source selects every datastream of the organization (overview of all sensors)
	spec, _, err = specWith(t, `,"source":{}`)
	if err != nil || spec["widgets"].([]map[string]any)[0]["source"].(map[string]any)["limit"] != 500 {
		t.Fatal("empty source", err)
	}
	for _, bad := range []string{`,"source":{"site":["nope"]}`, `,"source":{"limit":501}`, `,"source":{"kind":"x"}`, `,"source":{"attr":{"a b":"c"}}`, `,"source":{"name":5}`} {
		if _, _, err := specWith(t, bad); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	// a chart resolves at most 24 series; a text widget has no datastreams to select
	if _, _, err := validateScreenSpecW(json.RawMessage(`{"version":1,"widgets":[{"type":"chart","bindings":{},"params":{},"source":{"limit":25}}]}`)); err == nil {
		t.Error("chart limit 25 accepted")
	}
	if _, _, err := validateScreenSpecW(json.RawMessage(`{"version":1,"widgets":[{"type":"text","params":{},"source":{}}]}`)); err == nil {
		t.Error("source on a text widget accepted")
	}
}

// Older dashboards keep their stored form: no filters, source or follow appear in the canonical spec.
func TestOlderSpecUnchanged(t *testing.T) {
	raw := `{"version":1,"widgets":[{"id":"a","type":"value","geometry":{"x":0,"y":0,"w":3,"h":2},"bindings":{"datastreams":["` + tagA + `"]},"params":{"title":"T"}}]}`
	spec, _, err := validateScreenSpecW(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := spec["filters"]; ok {
		t.Error("filters added")
	}
	w := spec["widgets"].([]map[string]any)[0]
	if _, ok := w["source"]; ok {
		t.Error("source added")
	}
	if _, ok := w["follow"]; ok {
		t.Error("follow added")
	}
}

func TestOverviewStartersValid(t *testing.T) {
	for _, k := range []string{"sensors", "energy", "coldchain"} {
		name, spec := overviewStarter(k)
		raw, _ := json.Marshal(spec)
		out, warn, err := validateScreenSpecW(raw)
		if err != nil || len(warn) > 0 {
			t.Fatalf("%s: %v %v", k, err, warn)
		}
		if name == "" || len(out["filters"].([]map[string]any)) < 3 {
			t.Fatalf("%s: name %q, filters %v", k, name, out["filters"])
		}
		sources := 0
		for _, w := range out["widgets"].([]map[string]any) {
			if w["source"] != nil {
				sources++
			}
			if w["follow"] != true {
				t.Errorf("%s: widget %s does not follow the filters", k, w["id"])
			}
		}
		if sources == 0 {
			t.Errorf("%s: no query source", k)
		}
	}
}

func TestLikePatterns(t *testing.T) {
	for in, want := range map[string]string{"fridge": "%fridge%", "50%_x": `%50\%\_x%`, "Fridge*": "Fridge%", "T?-1*": "T_-1%", `a\b`: `%a\\b%`} {
		if got := likeName(in); got != want {
			t.Errorf("likeName(%q) = %q, want %q", in, got, want)
		}
	}
	a := &sqlArgs{}
	if got := inList("ds.quantity", []string{"temperature", "energy*", "*"}, a); got != "(ds.quantity = ANY($1::text[]) OR ds.quantity LIKE ANY($2::text[]))" {
		t.Errorf("inList %s", got)
	}
	if !reflect.DeepEqual(a.args, []any{[]string{"temperature", "*"}, []string{"energy%"}}) {
		t.Errorf("args %v", a.args)
	}
}

func TestSelWhere(t *testing.T) {
	d := dsSel{Sites: []string{tagA}, Quantities: []string{"temperature"}, Attrs: map[string][]string{"storage_class": {"2-8C"}}, Text: "fri", IDs: []string{tagB}}
	a := &sqlArgs{}
	w := strings.Join(d.where(a, "site"), " AND ")
	if strings.Contains(w, "st.id") || !strings.Contains(w, "ds.id = ANY") || !strings.Contains(w, "storage_class") && !strings.Contains(w, "attributes->>$") {
		t.Errorf("where with site skipped: %s", w)
	}
	if len(a.args) != 5 { // ids, quantity, attr key, attr values, text
		t.Errorf("args %d: %v", len(a.args), a.args)
	}
	v := url.Values{"site": {"x"}}
	if _, err := parseSel(v, ""); err == nil {
		t.Error("bad site id accepted")
	}
	v = url.Values{"f_attr": {"storage_class=2-8C", "storage_class=-25..-15C", "bad key=1"}}
	if _, err := parseSel(v, "f_"); err == nil {
		t.Error("bad attribute key accepted")
	}
	v = url.Values{"f_attr": {"storage_class=2-8C", "storage_class=-25..-15C"}, "f_q": {"  fridge "}}
	s, err := parseSel(v, "f_")
	if err != nil || len(s.Attrs["storage_class"]) != 2 || s.Text != "fridge" {
		t.Errorf("parseSel %+v %v", s, err)
	}
}

// A public link: a filter marked public takes the viewer's value, any other its default; filters the dashboard
// does not have are ignored.
func TestPublicFilterSel(t *testing.T) {
	spec := map[string]any{"filters": []any{
		map[string]any{"kind": "site", "public": true, "multi": true},
		map[string]any{"kind": "quantity", "default": []any{"temperature"}},
		map[string]any{"kind": "attribute", "key": "storage_class", "public": true},
	}}
	v := url.Values{"f_site": {tagA}, "f_quantity": {"energy"}, "f_device": {tagB}, "f_attr": {"storage_class=2-8C", "other=x"}, "f_q": {"secret"}}
	s, err := publicFilterSel(spec, v)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Sites, []string{tagA}) || !reflect.DeepEqual(s.Quantities, []string{"temperature"}) || len(s.Devices) != 0 || s.Text != "" {
		t.Errorf("public selection %+v", s)
	}
	if !reflect.DeepEqual(s.Attrs, map[string][]string{"storage_class": {"2-8C"}}) {
		t.Errorf("attributes %v", s.Attrs)
	}
}

package httpapi

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"
)

const (
	idA = "0190a1b2-0000-7000-8000-000000000001"
	idB = "0190a1b2-0000-7000-8000-000000000002"
)

func TestAliasValidation(t *testing.T) {
	ok := []string{
		`{"id":"one","kind":"single","entity":{"type":"device","id":"` + idA + `"}}`,
		`{"id":"few","name":"Two fridges","kind":"list","entity_type":"asset","ids":["` + idA + `","` + idB + `"]}`,
		`{"id":"fr","kind":"type","entity_type":"asset","types":["fridge","freezer"]}`,
		`{"id":"all","kind":"type","entity_type":"device","types":["*"]}`,
		`{"id":"nm","kind":"name","entity_type":"device","match":"starts","text":"Logger"}`,
		`{"id":"tree","kind":"hierarchy","root":{"type":"site","id":"` + idA + `"},"depth":2,"entity_type":"device","asset_type":["fridge"]}`,
		`{"id":"under","kind":"hierarchy","root_current":true}`,
		`{"id":"cur","kind":"current","entity_type":"device","default":{"type":"device","id":"` + idB + `"}}`,
		`{"id":"q","kind":"query","source":{"quantity":["temperature"],"limit":5}}`,
	}
	for _, js := range ok {
		var m map[string]any
		_ = json.Unmarshal([]byte(js), &m)
		c, err := validateAlias(0, m)
		if err != nil {
			t.Errorf("%s: %v", js, err)
			continue
		}
		if c["name"] == "" {
			t.Errorf("%s: no name", js)
		}
	}
	// canonical forms: a hierarchy without depth goes all the way (10) and shows datastreams; a query loses its limit
	var m map[string]any
	_ = json.Unmarshal([]byte(`{"id":"under","kind":"hierarchy","root_current":true}`), &m)
	c, _ := validateAlias(0, m)
	if c["depth"] != maxTreeDepth || c["entity_type"] != "datastream" || c["name"] != "under" {
		t.Errorf("hierarchy defaults: %v", c)
	}
	m = nil
	_ = json.Unmarshal([]byte(`{"id":"q","kind":"query","source":{"quantity":["temperature"],"limit":5}}`), &m)
	if c, _ := validateAlias(0, m); c["source"].(map[string]any)["limit"] != nil {
		t.Errorf("query keeps a limit: %v", c)
	}
	bad := map[string]string{
		`{"id":"x","kind":"magic"}`:                                                                   "kind must be",
		`{"id":"a b","kind":"current"}`:                                                               "id must be",
		`{"id":"x","kind":"single","entity":{"type":"user","id":"` + idA + `"}}`:                      "type must be",
		`{"id":"x","kind":"list","entity_type":"device","ids":[]}`:                                    "at least one",
		`{"id":"x","kind":"list","entity_type":"device","ids":["nope"]}`:                              "must be ids",
		`{"id":"x","kind":"type","entity_type":"site","types":["x"]}`:                                 "entity_type must be",
		`{"id":"x","kind":"name","entity_type":"device","text":""}`:                                   "required",
		`{"id":"x","kind":"hierarchy","root_current":true,"root":{"type":"site","id":"` + idA + `"}}`: "not both",
		`{"id":"x","kind":"hierarchy","root":{"type":"device","id":"` + idA + `"}}`:                   "type must be",
		`{"id":"x","kind":"hierarchy","root_current":true,"depth":11}`:                                "depth",
		`{"id":"x","kind":"current","extra":1}`:                                                       "unknown setting",
		`{"id":"x","kind":"query","source":{"site":["nope"]}}`:                                        "must be ids",
	}
	for js, want := range bad {
		var m map[string]any
		_ = json.Unmarshal([]byte(js), &m)
		if _, err := validateAlias(0, m); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want %q", js, err, want)
		}
	}
}

func TestKeyFilterValidation(t *testing.T) {
	ok := []string{
		`{"id":"warm","rules":[{"source":"value","key":"temp","op":"gt","value":8}]}`,
		`{"id":"own","match":"any","rules":[{"source":"value","key":"temp","op":"gt","dynamic":{"from":"self","key":"max_temp"}},{"source":"alarm","op":"eq","value":"action"}]}`,
		`{"id":"txt","rules":[{"source":"attribute","key":"storage_class","type":"string","op":"in","value":["2-8C","15-25C"]}]}`,
		`{"id":"b","rules":[{"source":"value","key":"door","type":"boolean","op":"eq","value":true}]}`,
		`{"id":"g","rules":[{"source":"stale","key":"temp"},{"match":"any","rules":[{"source":"quality","key":"temp","op":"ne","value":"good"},{"source":"value","op":"between","value":2,"value2":8}]}]}`,
		`{"id":"viewer","rules":[{"source":"value","op":"ge","dynamic":{"from":"filter","key":"limit"}}]}`,
	}
	for _, js := range ok {
		var m map[string]any
		_ = json.Unmarshal([]byte(js), &m)
		if _, err := validateKeyFilter(0, m); err != nil {
			t.Errorf("%s: %v", js, err)
		}
	}
	bad := map[string]string{
		`{"id":"x","rules":[]}`: "at least one",
		`{"id":"x","rules":[{"source":"value","op":"contains","value":1}]}`:                                      "number comparison",
		`{"id":"x","rules":[{"source":"value","op":"gt"}]}`:                                                      "required",
		`{"id":"x","rules":[{"source":"value","op":"between","value":1}]}`:                                       "value2",
		`{"id":"x","rules":[{"source":"attribute","op":"gt","value":1}]}`:                                        "attribute needs",
		`{"id":"x","rules":[{"source":"alarm","key":"temp"}]}`:                                                   "no key",
		`{"id":"x","rules":[{"source":"quality","op":"eq","value":"good","dynamic":{"from":"self","key":"q"}}]}`: "dynamic",
		`{"id":"x","rules":[{"source":"value","op":"gt","dynamic":{"from":"user","key":"q"}}]}`:                  "dynamic.from",
		`{"id":"x","rules":[{"match":"any","rules":[{"match":"all","rules":[{"source":"stale"}]}]}]}`:            "cannot contain groups",
		`{"id":"x","rules":[{"source":"value","key":"Temp","op":"gt","value":1}]}`:                               "datastream key",
		`{"id":"x","match":"some","rules":[{"source":"stale"}]}`:                                                 "match must be",
		`{"id":"x","rules":[{"source":"alarm","value":"red"}]}`:                                                  "alarm value",
	}
	for js, want := range bad {
		var m map[string]any
		_ = json.Unmarshal([]byte(js), &m)
		if _, err := validateKeyFilter(0, m); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want %q", js, err, want)
		}
	}
}

// A dashboard of format 1.2: aliases, key filters, states, widgets in states with aliases and actions.
func TestScreenSpecAliasesStates(t *testing.T) {
	spec := `{"version":1,"aliases":[{"id":"fridges","name":"Fridges","kind":"type","entity_type":"asset","types":["fridge"]},{"id":"sel","name":"Selected","kind":"current"}],
		"key_filters":[{"id":"warm","name":"Above 8","rules":[{"source":"value","key":"temp","op":"gt","value":8}]}],
		"states":[{"id":"main","name":"Overview"},{"id":"detail","name":"Fridge detail"}],
		"widgets":[
			{"id":"t","type":"entities_table","state":"main","alias":{"id":"fridges"},"params":{"columns":["name","t:temp"],"highlight_filter":"warm"},"actions":[{"event":"row","action":"state","target":"detail"},{"event":"row_button","action":"device","label":"Device"}]},
			{"id":"s","type":"entity_selector","alias":{"id":"fridges","filter":"warm"}},
			{"id":"c","type":"chart","state":"detail","alias":{"id":"sel","quantity":["temperature"]},"bindings":{"datastreams":[]}},
			{"id":"m","type":"map","alias":{"id":"fridges"},"bindings":{"devices":[]},"actions":[{"event":"marker","action":"state","target":"detail","pass":"asset"}]}
		]}`
	out, err := validateScreenSpec(json.RawMessage(spec))
	if err != nil {
		t.Fatal(err)
	}
	ws := out["widgets"].([]map[string]any)
	if _, ok := ws[0]["state"]; ok {
		t.Errorf("the root state is kept on a widget: %v", ws[0])
	}
	if ws[2]["state"] != "detail" || ws[2]["alias"].(map[string]any)["quantity"] == nil {
		t.Errorf("detail widget: %v", ws[2])
	}
	if len(out["aliases"].([]map[string]any)) != 2 || len(out["states"].([]map[string]any)) != 2 || len(out["key_filters"].([]map[string]any)) != 1 {
		t.Errorf("model: %v", out)
	}
	if acts := ws[3]["actions"].([]map[string]any); acts[0]["pass"] != "asset" {
		t.Errorf("actions: %v", acts)
	}
	refuse := map[string]string{
		strings.Replace(spec, `"alias":{"id":"fridges"},"params"`, `"alias":{"id":"nope"},"params"`, 1):                "no alias",
		strings.Replace(spec, `"highlight_filter":"warm"`, `"highlight_filter":"cold"`, 1):                             "no key filter",
		strings.Replace(spec, `"state":"detail","alias"`, `"state":"other","alias"`, 1):                                "no state",
		strings.Replace(spec, `"target":"detail"},{"event":"row_button"`, `"target":"gone"},{"event":"row_button"`, 1): "no state",
		strings.Replace(spec, `"action":"device","label":"Device"`, `"action":"device"`, 1):                            "needs a label",
		strings.Replace(spec, `"filter":"warm"`, `"filter":"cold"`, 1):                                                 "no key filter",
		strings.Replace(spec, `"event":"marker"`, `"event":"hover"`, 1):                                                "event must be",
	}
	for js, want := range refuse {
		if _, err := validateScreenSpec(json.RawMessage(js)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("got %v, want %q", err, want)
		}
	}
	// a control widget never takes an alias; a source and an alias exclude each other; states need the grid
	for js, want := range map[string]string{
		`{"version":1,"aliases":[{"id":"a","kind":"current"}],"widgets":[{"type":"switch","alias":{"id":"a"}}]}`:            "control widget",
		`{"version":1,"aliases":[{"id":"a","kind":"current"}],"widgets":[{"type":"chart","alias":{"id":"a"},"source":{}}]}`: "not both",
		`{"version":1,"aliases":[{"id":"a","kind":"current"}],"widgets":[{"type":"text","alias":{"id":"a"}}]}`:              "shows no entities",
		`{"version":1,"kind":"mimic","states":[{"id":"a"}],"widgets":[]}`:                                                   "grid layout",
		`{"version":1,"aliases":[{"id":"a","kind":"current"},{"id":"a","kind":"current"}],"widgets":[]}`:                    "used twice",
		`{"version":1,"states":[{"id":"a"},{"id":"a"}],"widgets":[]}`:                                                       "unique",
		`{"version":1,"widgets":[{"type":"chart","state":"x"}]}`:                                                            "no state",
		`{"version":1,"filters":[{"kind":"value","key":"limit","multi":true}],"widgets":[]}`:                                "one value",
		`{"version":1,"filters":[{"kind":"value"}],"widgets":[]}`:                                                           "needs a key",
	} {
		if _, err := validateScreenSpec(json.RawMessage(js)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want %q", js, err, want)
		}
	}
	// a value filter (a limit typed by viewers) with a default
	if out, err := validateScreenSpec(json.RawMessage(`{"version":1,"filters":[{"kind":"value","key":"limit","label":"Limit","default":"8","public":true}],"widgets":[]}`)); err != nil || out["filters"].([]map[string]any)[0]["default"] != "8" {
		t.Errorf("value filter: %v %v", out, err)
	}
}

// A dashboard of format 1.1 (no aliases, key filters or states) keeps its canonical form: no new keys appear.
func TestOldSpecUnchanged(t *testing.T) {
	old := `{"version":1,"kind":"dashboard","layout":{"type":"grid","columns":12,"row_height":96},"range":"24h","refresh_s":30,"filters":[{"kind":"site","multi":true}],
		"widgets":[{"id":"w1","type":"overview_table","geometry":{"x":0,"y":0,"w":12,"h":5},"bindings":{"datastreams":[]},"source":{"quantity":["temperature"]},"follow":true,"params":{"title":"T"}}]}`
	a, err := validateScreenSpec(json.RawMessage(old))
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := json.Marshal(a)
	b, err := validateScreenSpec(b1)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := json.Marshal(b)
	if string(b1) != string(b2) {
		t.Errorf("not idempotent:\n%s\n%s", b1, b2)
	}
	for _, k := range []string{"aliases", "key_filters", "states", `"state"`, `"alias"`, `"actions"`, "highlight_filter"} {
		if strings.Contains(string(b1), k) {
			t.Errorf("a 1.1 dashboard gained %s: %s", k, b1)
		}
	}
}

func TestEvalFilter(t *testing.T) {
	now := time.Now()
	fridge := &entity{Type: "asset", ID: "a1", Name: "Fridge 1", Attrs: map[string]any{"max_temp": 8.0, "class": "2-8C", "vaccines": true}, SiteAttrs: map[string]any{"region": "west"},
		ByKey: map[string]string{"temp": "d1", "door": "d2", "note": "d3"}}
	ds := &entity{Type: "datastream", ID: "d1", Key: "temp", ByKey: map[string]string{"temp": "d1"}, AssetAttrs: map[string]any{"max_temp": "9"}}
	env := &evalEnv{Values: map[string]string{"limit": "8.5"}, Org: map[string]any{"limit": 7.0}, Current: &entity{Attrs: map[string]any{"max_temp": 9.5}},
		Latest: map[string]*latestVal{"d1": {Value: fptr(8.6), At: now, Quality: "ok"}, "d2": {JSON: json.RawMessage(`true`), At: now, Quality: "ok"}, "d3": {JSON: json.RawMessage(`"Door OPEN"`), At: now.Add(-3 * time.Hour), Quality: "ok", Stale: true}},
		Alarms: map[string]map[string]any{"asset:a1": {"severity": "warning", "count": 1}}}
	rule := func(js string) map[string]any {
		var m map[string]any
		if err := json.Unmarshal([]byte(js), &m); err != nil {
			t.Fatal(err)
		}
		c, err := validateRule("t", m, false)
		if err != nil {
			t.Fatalf("%s: %v", js, err)
		}
		return c
	}
	cases := []struct {
		e    *entity
		js   string
		want bool
	}{
		{fridge, `{"source":"value","key":"temp","op":"gt","value":8}`, true},
		{fridge, `{"source":"value","key":"temp","op":"le","value":8}`, false},
		{fridge, `{"source":"value","key":"temp","op":"between","value":9,"value2":2}`, true}, // the bounds in any order
		{fridge, `{"source":"value","key":"temp","op":"between","value":2,"value2":9}`, true},
		{fridge, `{"source":"value","key":"missing","op":"gt","value":0}`, false},
		{fridge, `{"source":"value","key":"temp","op":"gt","dynamic":{"from":"self","key":"max_temp"}}`, true},
		{fridge, `{"source":"value","key":"temp","op":"gt","dynamic":{"from":"current","key":"max_temp"}}`, false},
		{fridge, `{"source":"value","key":"temp","op":"gt","dynamic":{"from":"filter","key":"limit"}}`, true},
		{fridge, `{"source":"value","key":"temp","op":"gt","dynamic":{"from":"org","key":"limit"}}`, true},
		{fridge, `{"source":"value","key":"temp","op":"gt","dynamic":{"from":"self","key":"nope"},"value":9}`, false}, // missing dynamic: the constant
		{fridge, `{"source":"value","key":"temp","op":"gt","dynamic":{"from":"self","key":"nope"}}`, false},
		{fridge, `{"source":"value","key":"door","type":"boolean","op":"eq","value":true}`, true},
		{fridge, `{"source":"value","key":"note","type":"string","op":"contains","value":"open"}`, true},
		{fridge, `{"source":"value","key":"note","type":"string","op":"starts","value":"door"}`, true},
		{fridge, `{"source":"value","key":"note","type":"string","op":"ends","value":"closed"}`, false},
		{fridge, `{"source":"attribute","key":"class","type":"string","op":"in","value":["15-25C","2-8c"]}`, true},
		{fridge, `{"source":"attribute","key":"region","type":"string","op":"eq","value":"West"}`, true}, // the site's attribute
		{fridge, `{"source":"attribute","key":"vaccines","type":"boolean","op":"ne","value":false}`, true},
		{fridge, `{"source":"quality","key":"temp","op":"eq","value":"good"}`, true},
		{fridge, `{"source":"quality","key":"missing","op":"ne","value":"good"}`, true},
		{fridge, `{"source":"stale","key":"note"}`, true},
		{fridge, `{"source":"stale","key":"temp"}`, false},
		{fridge, `{"source":"stale","key":"missing"}`, true},
		{fridge, `{"source":"alarm","op":"eq","value":"any"}`, true},
		{fridge, `{"source":"alarm","op":"eq","value":"action"}`, false},
		{fridge, `{"source":"alarm","op":"ne","value":"any"}`, false},
		{ds, `{"source":"value","op":"gt","value":8.5}`, true},                                             // own value
		{ds, `{"source":"value","op":"gt","dynamic":{"from":"self","key":"max_temp"}}`, false},             // the asset's attribute "9"
		{ds, `{"source":"value","key":"temp","op":"lt","dynamic":{"from":"self","key":"max_temp"}}`, true}, // text numbers compare as numbers
	}
	for _, c := range cases {
		if got := evalRule(rule(c.js), c.e, env); got != c.want {
			t.Errorf("%s on %s: got %v", c.js, c.e.Type, got)
		}
	}
	var f map[string]any
	_ = json.Unmarshal([]byte(`{"id":"f","match":"all","rules":[{"source":"value","key":"temp","op":"gt","value":8},{"match":"any","rules":[{"source":"alarm","value":"critical"},{"source":"stale","key":"note"}]}]}`), &f)
	kf, err := validateKeyFilter(0, f)
	if err != nil {
		t.Fatal(err)
	}
	if !evalFilter(kf, fridge, env) {
		t.Error("all of (value, any of (alarm, stale)) should match")
	}
	if !evalFilter(nil, fridge, env) {
		t.Error("no filter matches everything")
	}
	keys := map[string]bool{}
	if al, org := filterNeeds(kf, keys); !al || org || !keys["temp"] || !keys["note"] {
		t.Errorf("needs: %v %v %v", al, org, keys)
	}
}

func TestEntityHelpers(t *testing.T) {
	if r, err := parseRef("device:" + idA); err != nil || r.Type != "device" || r.ID != idA {
		t.Errorf("parseRef: %v %v", r, err)
	}
	for _, s := range []string{"device", "user:" + idA, "device:x"} {
		if _, err := parseRef(s); err == nil {
			t.Errorf("parseRef accepts %q", s)
		}
	}
	v, a := columnKeys([]string{"name", "t:temp=Temperature", "a:max_temp", "t:Bad", "t:temp", "a:x y"})
	if strings.Join(v, ",") != "temp" || strings.Join(a, ",") != "max_temp" {
		t.Errorf("columnKeys: %v %v", v, a)
	}
	if wantOf("entities_table") != "entities" || wantOf("chart") != "datastreams" || wantOf("map") != "devices" || wantOf("alarms") != "datastreams" {
		t.Error("wantOf")
	}
	spec := map[string]any{"filters": []any{map[string]any{"kind": "value", "key": "limit", "default": "8", "public": true}, map[string]any{"kind": "value", "key": "fixed", "default": "3"}}}
	got := publicValueFilters(spec, url.Values{"f_value": {"limit=9", "fixed=99", "other=1"}})
	if got["limit"] != "9" || got["fixed"] != "3" || got["other"] != "" {
		t.Errorf("public value filters: %v", got)
	}
	if _, err := valueFilters(url.Values{"f_value": {"bad key=1"}}); err == nil {
		t.Error("valueFilters accepts a bad key")
	}
}

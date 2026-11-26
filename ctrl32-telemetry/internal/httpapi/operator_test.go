package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func slugs(nodes []*treeNode) string {
	var s []string
	for _, n := range nodes {
		s = append(s, strings.Repeat(">", n.Depth-1)+n.Slug)
	}
	return strings.Join(s, " ")
}

// Siblings sort by order and then name; a missing parent, a cycle and a branch that is too deep end at the top level.
func TestBuildScreenTreeOrder(t *testing.T) {
	tree := buildScreenTree([]*treeNode{
		{Slug: "plant", Name: "Plant"},
		{Slug: "b", Name: "Blowers", Parent: "plant", Order: 20},
		{Slug: "a", Name: "Aeration", Parent: "plant", Order: 20},
		{Slug: "inlet", Name: "Inlet", Parent: "plant", Order: 10},
		{Slug: "b1", Name: "Blower 1", Parent: "b"},
		{Slug: "orphan", Name: "Orphan", Parent: "deleted"},
		{Slug: "x", Name: "X", Parent: "y"},
		{Slug: "y", Name: "Y", Parent: "x"},
	}, nil)
	if got, want := slugs(tree), "orphan plant >inlet >a >b >>b1 x >y"; got != want {
		t.Fatalf("tree order:\n got %s\nwant %s", got, want)
	}
	deep := []*treeNode{{Slug: "l1", Name: "1"}}
	for i, s := range []string{"l2", "l3", "l4", "l5", "l6"} {
		deep = append(deep, &treeNode{Slug: s, Name: s, Parent: deep[i].Slug})
	}
	for _, n := range buildScreenTree(deep, nil) {
		if n.Depth > maxNavDepth {
			t.Errorf("%s at depth %d", n.Slug, n.Depth)
		}
	}
}

// A node carries the alarms of its own tags and of every picture below it, each alarm once; the colour is that of the
// most important alarm: an unacknowledged one first, then the class priority.
func TestBuildScreenTreeGroupAlarms(t *testing.T) {
	alarms := map[string][]alarmMark{
		"t-level": {{unack: true, active: true, priority: 2, color: "#f59f00"}},
		"t-motor": {{unack: false, active: true, priority: 1, color: "#d63939"}, {unack: true, active: false, priority: 2, color: "#f59f00"}},
		"t-other": {{unack: true, active: true, priority: 1, color: "#d63939"}},
	}
	tree := buildScreenTree([]*treeNode{
		{Slug: "plant", Name: "Plant", tags: []string{"t-level"}},
		{Slug: "tank", Name: "Tank", Parent: "plant", tags: []string{"t-level"}},
		{Slug: "motor", Name: "Motor", Parent: "plant", tags: []string{"t-motor"}},
		{Slug: "quiet", Name: "Quiet", Parent: "plant", tags: []string{"t-none"}},
		{Slug: "acked", Name: "Acked"},
	}, alarms)
	by := map[string]*treeNode{}
	for _, n := range tree {
		by[n.Slug] = n
	}
	if n := by["plant"]; n.Unack != 2 || n.Active != 2 || n.Color != "#f59f00" || n.Priority != 2 || n.Children != 3 {
		t.Errorf("plant: %+v", *n)
	}
	if n := by["tank"]; n.Unack != 1 || n.Active != 1 || n.Color != "#f59f00" {
		t.Errorf("tank: %+v", *n)
	}
	if n := by["motor"]; n.Unack != 1 || n.Active != 1 || n.Priority != 2 {
		t.Errorf("motor (the unacknowledged alarm leads): %+v", *n)
	}
	if n := by["quiet"]; n.Unack != 0 || n.Active != 0 || n.Color != "" || n.Priority != 0 {
		t.Errorf("quiet: %+v", *n)
	}
}

func TestParseScreenMeta(t *testing.T) {
	a, b, c := "0198a000-0000-7000-8000-000000000001", "0198a000-0000-7000-8000-000000000002", "0198a000-0000-7000-8000-000000000003"
	m := parseScreenMeta([]byte(`{"kind":"mimic","nav":{"parent":"plant","order":3,"label":"M1"},
		"elements":[{"tag":"` + a + `","dyn":[{"prop":"fill","tag":"` + b + `"},{"prop":"blink"}]},{"tag":"` + a + `"},{"tag":"$speed"}],
		"widgets":[{"bindings":{"datastreams":["` + c + `"],"devices":["` + a + `"]}}]}`))
	if !m.inTree || m.parent != "plant" || m.order != 3 || m.label != "M1" || m.kind != "mimic" || strings.Join(m.tags, ",") != a+","+b+","+c {
		t.Fatalf("%+v", *m)
	}
	if parseScreenMeta([]byte(`{"kind":"dashboard","widgets":[]}`)).inTree {
		t.Fatal("a screen without nav is not in the tree")
	}
}

// Re-binding a faceplate type: tags and devices of the example move to the instance, a tag without a counterpart is
// unbound, an example device without a counterpart loses its commands, foreign tags and devices stay.
func TestRebindSpec(t *testing.T) {
	var spec map[string]any
	if err := json.Unmarshal([]byte(`{"elements":[
		{"id":"run","tag":"ds-run","device":"dev-1","key":"m1_run","dyn":[{"prop":"state","tag":"ds-fault"},{"prop":"fill","tag":"ds-gone"},{"prop":"blink"}],
		 "on":[{"act":"command","key":"m1_start","value":true},{"act":"faceplate"}]},
		{"id":"gone","tag":"ds-gone"},
		{"id":"supply","tag":"ds-supply","device":"dev-shared","key":"m1_x","on":[{"act":"command","key":"m1_reset"}]},
		{"id":"btn","device":"dev-lost","key":"stop","on":[{"act":"command","key":"stop"},{"act":"trend"}]}],
	 "widgets":[{"id":"w","bindings":{"datastreams":["ds-run","ds-gone","ds-supply"],"devices":["dev-1","dev-lost","dev-shared"]}}]}`), &spec); err != nil {
		t.Fatal(err)
	}
	rebindSpec(spec, map[string]string{"ds-run": "ds2-run", "ds-fault": "ds2-fault", "ds-gone": ""}, map[string]string{"dev-1": "dev-2", "dev-lost": ""}, "m1_", "m2_")
	out, _ := json.Marshal(spec)
	want := `{"elements":[` +
		`{"device":"dev-2","dyn":[{"prop":"state","tag":"ds2-fault"},{"prop":"blink"}],"id":"run","key":"m2_run","on":[{"act":"command","key":"m2_start","value":true},{"act":"faceplate"}],"tag":"ds2-run"},` +
		`{"id":"gone"},` +
		`{"device":"dev-shared","id":"supply","key":"m1_x","on":[{"act":"command","key":"m1_reset"}],"tag":"ds-supply"},` +
		`{"id":"btn","on":[{"act":"trend"}]}],` +
		`"widgets":[{"bindings":{"datastreams":["ds2-run","ds-supply"],"devices":["dev-2","dev-shared"]},"id":"w"}]}`
	if string(out) != want {
		t.Fatalf("rebound spec:\n got %s\nwant %s", out, want)
	}
}

func TestValidateNavAndFaceplate(t *testing.T) {
	spec, err := validateScreenSpec([]byte(`{"kind":"mimic","nav":{"parent":"plant","order":20,"label":" Aeration "},"faceplate":{"asset":"0198a000-0000-7000-8000-000000000001","prefix":"m1_"}}`))
	if err != nil {
		t.Fatal(err)
	}
	nav, fp := spec["nav"].(map[string]any), spec["faceplate"].(map[string]any)
	if nav["parent"] != "plant" || nav["order"] != 20 || nav["label"] != "Aeration" || fp["prefix"] != "m1_" {
		t.Fatalf("nav %v faceplate %v", nav, fp)
	}
	if spec, err = validateScreenSpec([]byte(`{"kind":"mimic","nav":{}}`)); err != nil || spec["nav"].(map[string]any)["order"] != 0 {
		t.Fatalf("an empty nav is a top-level picture: %v %v", spec["nav"], err)
	}
	if spec, _ = validateScreenSpec([]byte(`{"kind":"mimic"}`)); spec["nav"] != nil || spec["faceplate"] != nil {
		t.Fatal("no nav, no faceplate by default")
	}
	for _, bad := range []string{
		`{"nav":{"parent":"Not A Slug"}}`, `{"nav":{"order":-1}}`, `{"nav":{"order":1.5}}`, `{"nav":{"label":"<b>x</b>"}}`,
		`{"nav":{"label":"a label that is much longer than a button"}}`, `{"faceplate":{}}`,
		`{"faceplate":{"asset":"0198a000-0000-7000-8000-000000000001","prefix":"M1."}}`,
		`{"kind":"mimic","elements":[{"type":"rect","w":10,"h":10,"on":[{"act":"popup","target":"motor","asset":"x"}]}]}`,
		`{"kind":"mimic","elements":[{"type":"rect","w":10,"h":10,"on":[{"act":"popup","target":"motor","prefix":"M 2"}]}]}`,
	} {
		if _, err := validateScreenSpec([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	spec, err = validateScreenSpec([]byte(`{"kind":"mimic","elements":[{"type":"rect","w":10,"h":10,"on":[{"act":"popup","target":"motor","asset":"0198a000-0000-7000-8000-000000000002","prefix":"m2_"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if ev := elementsOf(spec)[0]["on"].([]map[string]any)[0]; ev["asset"] != "0198a000-0000-7000-8000-000000000002" || ev["prefix"] != "m2_" {
		t.Fatalf("popup parameters: %v", ev)
	}
}

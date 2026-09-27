package flows

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// test subflows: "double" doubles the value (output 1) and passes values above 10 on output 2; "quad" uses "double"
// twice; "loop" uses itself; "cmd" sends a command.
var testSubflows = map[string]string{
	"double": `{"kind":"subflow","nodes":[{"id":"in","type":"sub_in","x":0,"y":0,"settings":{}},
		{"id":"x2","type":"math","x":100,"y":0,"settings":{"expr":"value * 2"}},
		{"id":"big","type":"filter","x":200,"y":0,"settings":{"condition":"value > 10"}},
		{"id":"o1","type":"sub_out","x":300,"y":0,"settings":{"port":1}},
		{"id":"o2","type":"sub_out","x":300,"y":100,"settings":{"port":2}}],
		"wires":[{"from":"in","port":0,"to":"x2"},{"from":"x2","port":0,"to":"o1"},{"from":"x2","port":0,"to":"big"},{"from":"big","port":0,"to":"o2"}]}`,
	"quad": `{"kind":"subflow","nodes":[{"id":"in","type":"sub_in","x":0,"y":0,"settings":{}},
		{"id":"a","type":"subflow","x":100,"y":0,"settings":{"flow":"double","version":1,"outputs":2}},
		{"id":"b","type":"subflow","x":200,"y":0,"settings":{"flow":"double","version":1,"outputs":2}},
		{"id":"o","type":"sub_out","x":300,"y":0,"settings":{"port":1}}],
		"wires":[{"from":"in","port":0,"to":"a"},{"from":"a","port":0,"to":"b"},{"from":"b","port":0,"to":"o"}]}`,
	"loop": `{"kind":"subflow","nodes":[{"id":"in","type":"sub_in","x":0,"y":0,"settings":{}},
		{"id":"me","type":"subflow","x":100,"y":0,"settings":{"flow":"loop","version":1,"outputs":1}}],
		"wires":[{"from":"in","port":0,"to":"me"}]}`,
	"cmd": `{"kind":"subflow","nodes":[{"id":"in","type":"sub_in","x":0,"y":0,"settings":{}},
		{"id":"c","type":"command","x":100,"y":0,"settings":{"device":"` + devA + `","key":"x","value":"value"}}],
		"wires":[{"from":"in","port":0,"to":"c"}]}`,
}

func testLoadSubflow(_ context.Context, _, slug string, _ int) (*Spec, error) {
	js, ok := testSubflows[slug]
	if !ok {
		return nil, errors.New("not found")
	}
	var sp Spec
	if err := json.Unmarshal([]byte(js), &sp); err != nil {
		return nil, err
	}
	return &sp, nil
}

func testSubflowRef(slug string, v int) (SubflowRef, bool) {
	sp, err := testLoadSubflow(context.Background(), "", slug, v)
	if err != nil {
		return SubflowRef{}, false
	}
	o, c := SubflowInfo(sp)
	return SubflowRef{Name: slug, Version: 1, Outputs: o, Commands: c}, true
}

func TestSubflowRun(t *testing.T) {
	e, _, _, cancel := testEngine(t, `{"nodes":[{"id":"i","type":"inject","x":0,"y":0,"settings":{"value":"1"}},
		{"id":"s","type":"subflow","x":200,"y":0,"settings":{"flow":"double"}},
		{"id":"q","type":"subflow","x":200,"y":200,"settings":{"flow":"quad"}},
		{"id":"d1","type":"debug","x":400,"y":0,"settings":{"expr":"'o1 ' + value"}},
		{"id":"d2","type":"debug","x":400,"y":100,"settings":{"expr":"'o2 ' + value"}},
		{"id":"d3","type":"debug","x":400,"y":200,"settings":{"expr":"'quad ' + value"}}],
		"wires":[{"from":"i","port":0,"to":"s"},{"from":"s","port":0,"to":"d1"},{"from":"s","port":1,"to":"d2"},{"from":"i","port":0,"to":"q"},{"from":"q","port":0,"to":"d3"}]}`)
	defer cancel()
	for _, v := range []float64{3, 7} {
		if err := e.Inject(context.Background(), "org", "test", "i", v, "", false, nil, ""); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "outputs", func() bool { return len(debugTexts(e, "debug")) == 5 })
	got := strings.Join(debugTexts(e, "debug"), ",")
	for _, want := range []string{"o1 6", "o1 14", "o2 14", "quad 12", "quad 28"} {
		if !strings.Contains(got, want) {
			t.Fatalf("debug %s, missing %s", got, want)
		}
	}
	if strings.Contains(got, "o2 6") {
		t.Fatalf("filter inside the subflow: %s", got)
	}
}

func TestSubflowValidate(t *testing.T) {
	// pinned version and derived outputs
	sp := Spec{Nodes: []NodeSpec{{ID: "i", Type: "inject", Settings: map[string]any{}}, {ID: "s", Type: "subflow", Settings: map[string]any{"flow": "double"}}},
		Wires: []Wire{{From: "i", To: "s"}}}
	if p := Validate(&sp, fakeRefs{}); len(p) != 0 {
		t.Fatal(p)
	}
	if sp.Nodes[1].Settings["version"] != 1.0 || outputsOf(&sp.Nodes[1]) != 2 {
		t.Fatalf("pinned %v outputs %d", sp.Nodes[1].Settings, outputsOf(&sp.Nodes[1]))
	}
	// a subflow with a command makes the flow commanding
	sp2 := Spec{Nodes: []NodeSpec{{ID: "i", Type: "inject", Settings: map[string]any{}}, {ID: "s", Type: "subflow", Settings: map[string]any{"flow": "cmd"}}},
		Wires: []Wire{{From: "i", To: "s"}}}
	_ = Validate(&sp2, fakeRefs{})
	if !HasCommands(&sp2) {
		t.Fatal("commands through the subflow")
	}
	cases := map[string]string{
		`{"nodes":[{"id":"i","type":"inject","x":0,"y":0,"settings":{}},{"id":"s","type":"subflow","x":0,"y":0,"settings":{"flow":"nope"}}],"wires":[{"from":"i","port":0,"to":"s"}]}`: "not found",
		`{"nodes":[{"id":"i","type":"inject","x":0,"y":0,"settings":{}},{"id":"o","type":"sub_out","x":0,"y":0,"settings":{}}],"wires":[]}`:                                            "belong in a subflow",
		`{"kind":"subflow","nodes":[{"id":"x","type":"math","x":0,"y":0,"settings":{"expr":"1"}}],"wires":[]}`:                                                                         "exactly one subflow input",
		`{"kind":"subflow","nodes":[{"id":"in","type":"sub_in","x":0,"y":0,"settings":{}},{"id":"t","type":"inject","x":0,"y":0,"settings":{}}],"wires":[]}`:                           "triggers belong",
		`{"nodes":[{"id":"i","type":"inject","x":0,"y":0,"settings":{}}],"wires":[],"groups":[{"id":"g","name":"x","color":"red"}]}`:                                                   "group",
	}
	for js, want := range cases {
		var s Spec
		_ = json.Unmarshal([]byte(js), &s)
		p := Validate(&s, fakeRefs{})
		if !strings.Contains(probs(p), want) {
			t.Errorf("%s: want %q, got %v", js, want, p)
		}
	}
	// recursion
	loop, _ := testLoadSubflow(context.Background(), "", "loop", 1)
	if _, err := Expand(&Spec{Nodes: []NodeSpec{{ID: "x", Type: "subflow", Settings: map[string]any{"flow": "loop"}}}}, func(s string, v int) (*Spec, error) {
		return testLoadSubflow(context.Background(), "", s, v)
	}); err == nil || !strings.Contains(err.Error(), "uses itself") {
		t.Fatalf("recursion: %v (%v)", err, loop)
	}
}

func probs(p []Problem) string {
	var b strings.Builder
	for _, x := range p {
		b.WriteString(x.Node + ": " + x.Message + "; ")
	}
	return b.String()
}

func TestSplit(t *testing.T) {
	e, _, _, cancel := testEngine(t, `{"nodes":[{"id":"i","type":"inject","x":0,"y":0,"settings":{"value":"1"}},
		{"id":"s","type":"split","x":200,"y":0,"settings":{"list":"value","max":3}},
		{"id":"d","type":"debug","x":400,"y":0,"settings":{"expr":"msg.index + '/' + msg.count + ' ' + msg.key + '=' + value"}}],
		"wires":[{"from":"i","port":0,"to":"s"},{"from":"s","port":0,"to":"d"}]}`)
	defer cancel()
	_ = e.Inject(context.Background(), "org", "test", "i", `[10,20,30,40]`, "", false, nil, "")
	waitFor(t, "list", func() bool { return len(debugTexts(e, "debug")) == 3 })
	_ = e.Inject(context.Background(), "org", "test", "i", `{"b":2,"a":1}`, "", false, nil, "")
	waitFor(t, "object", func() bool { return len(debugTexts(e, "debug")) == 5 })
	got := strings.Join(debugTexts(e, "debug"), ",")
	for _, want := range []string{"0/3 =10", "2/3 =30", "0/2 a=1", "1/2 b=2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("split %s, missing %s", got, want)
		}
	}
	if strings.Contains(got, "=40") {
		t.Fatalf("max ignored: %s", got)
	}
}

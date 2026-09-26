package flows

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	dsA  = "11111111-1111-4111-8111-111111111111"
	devA = "22222222-2222-4222-8222-222222222222"
	chA  = "33333333-3333-4333-8333-333333333333"
	entA = "55555555-5555-4555-8555-555555555555"
)

type fakeRefs struct{}

func (fakeRefs) Datastream(id string) bool { return id == dsA }
func (fakeRefs) Device(id string) bool     { return id == devA }
func (fakeRefs) Channel(id string) bool    { return id == chA }
func (fakeRefs) Entity(id string) bool     { return id == entA }
func (fakeRefs) HostAllowed(h string) bool { return h == "hooks.example.com" }

// The US1 example: temperature > 80 for 2 minutes → notify and command.
const exampleFlow = `{"nodes":[
 {"id":"t","type":"ds_value","x":40,"y":40,"settings":{"datastream":"` + dsA + `"}},
 {"id":"hot","type":"stable_for","x":240,"y":40,"settings":{"condition":"value > 80","seconds":120}},
 {"id":"mail","type":"notify","x":440,"y":20,"settings":{"channel":"` + chA + `","subject":"Too hot: {{ fixed(value, 1) }}"}},
 {"id":"cool","type":"command","x":440,"y":100,"settings":{"device":"` + devA + `","key":"cooling","value":"true"}},
 {"id":"note","type":"comment","x":40,"y":200,"settings":{"text":"cooling when hot"}}],
 "wires":[{"from":"t","port":0,"to":"hot"},{"from":"hot","port":0,"to":"mail"},{"from":"hot","port":0,"to":"cool"}]}`

func parseSpec(t *testing.T, s string) *Spec {
	t.Helper()
	var sp Spec
	if err := json.Unmarshal([]byte(s), &sp); err != nil {
		t.Fatal(err)
	}
	return &sp
}

func TestValidateExample(t *testing.T) {
	sp := parseSpec(t, exampleFlow)
	if p := Validate(sp, fakeRefs{}); len(p) != 0 {
		t.Fatalf("problems: %v", p)
	}
	if sp.Nodes[3].Settings["method"] != "write" {
		t.Error("default not filled in")
	}
	if !HasCommands(sp) {
		t.Error("command flow not detected")
	}
}

func TestValidateRefusesInvalidFlows(t *testing.T) {
	trig := `{"id":"t","type":"inject","settings":{}}`
	cases := map[string]string{
		"unknown type":          `{"nodes":[` + trig + `,{"id":"x","type":"script","settings":{}}],"wires":[]}`,
		"unknown setting":       `{"nodes":[{"id":"t","type":"inject","settings":{"code":"rm -rf /"}}],"wires":[]}`,
		"missing required":      `{"nodes":[` + trig + `,{"id":"c","type":"command","settings":{"device":"` + devA + `"}}],"wires":[]}`,
		"bad expression":        `{"nodes":[` + trig + `,{"id":"f","type":"filter","settings":{"condition":"value >"}}],"wires":[]}`,
		"unknown function":      `{"nodes":[` + trig + `,{"id":"f","type":"filter","settings":{"condition":"eval('x')"}}],"wires":[]}`,
		"bad template":          `{"nodes":[` + trig + `,{"id":"m","type":"template","settings":{"text":"{{ value + }}"}}],"wires":[]}`,
		"foreign datastream":    `{"nodes":[{"id":"t","type":"ds_value","settings":{"datastream":"44444444-4444-4444-8444-444444444444"}}],"wires":[]}`,
		"host not allowed":      `{"nodes":[` + trig + `,{"id":"h","type":"http_out","settings":{"url":"https://evil.example.org/x"}}],"wires":[]}`,
		"number out of range":   `{"nodes":[` + trig + `,{"id":"d","type":"delay","settings":{"seconds":999999}}],"wires":[]}`,
		"bad select":            `{"nodes":[` + trig + `,{"id":"a","type":"aggregate","settings":{"fn":"median"}}],"wires":[]}`,
		"bad time":              `{"nodes":[` + trig + `,{"id":"w","type":"time_window","settings":{"from":"25:00"}}],"wires":[]}`,
		"duplicate id":          `{"nodes":[` + trig + `,` + trig + `],"wires":[]}`,
		"bad id":                `{"nodes":[{"id":"a b","type":"inject","settings":{}}],"wires":[]}`,
		"wire to missing":       `{"nodes":[` + trig + `],"wires":[{"from":"t","port":0,"to":"nope"}]}`,
		"wire into a trigger":   `{"nodes":[` + trig + `,{"id":"u","type":"inject","settings":{}}],"wires":[{"from":"t","port":0,"to":"u"}]}`,
		"output does not exist": `{"nodes":[` + trig + `,{"id":"m","type":"math","settings":{}}],"wires":[{"from":"t","port":3,"to":"m"}]}`,
		"loop without delay":    `{"nodes":[` + trig + `,{"id":"a","type":"math","settings":{}},{"id":"b","type":"math","settings":{}}],"wires":[{"from":"t","port":0,"to":"a"},{"from":"a","port":0,"to":"b"},{"from":"b","port":0,"to":"a"}]}`,
		"self wire":             `{"nodes":[` + trig + `,{"id":"a","type":"math","settings":{}}],"wires":[{"from":"a","port":0,"to":"a"}]}`,
		"no trigger":            `{"nodes":[{"id":"a","type":"math","settings":{}}],"wires":[]}`,
		"change deletes value":  `{"nodes":[` + trig + `,{"id":"c","type":"change","settings":{"fields":[{"op":"delete","name":"value"}]}}],"wires":[]}`,
		"change bad field":      `{"nodes":[` + trig + `,{"id":"c","type":"change","settings":{"fields":[{"op":"set","name":"msg.a.b","expr":"1"}]}}],"wires":[]}`,
		"too many rules":        `{"nodes":[` + trig + `,{"id":"s","type":"switch","settings":{"rules":["1","1","1","1","1","1","1","1","1","1","1"]}}],"wires":[]}`,
		"daily without times":   `{"nodes":[{"id":"s","type":"schedule","settings":{"mode":"daily","times":" "}}],"wires":[]}`,
	}
	for name, js := range cases {
		sp := parseSpec(t, js)
		if p := Validate(sp, fakeRefs{}); len(p) == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
	// a loop through a delay is fine
	ok := parseSpec(t, `{"nodes":[`+trig+`,{"id":"a","type":"math","settings":{}},{"id":"d","type":"delay","settings":{"seconds":1}}],"wires":[{"from":"t","port":0,"to":"a"},{"from":"a","port":0,"to":"d"},{"from":"d","port":0,"to":"a"}]}`)
	if p := Validate(ok, fakeRefs{}); len(p) != 0 {
		t.Errorf("loop through a delay refused: %v", p)
	}
	// too many nodes
	var b strings.Builder
	b.WriteString(`{"nodes":[`)
	for i := 0; i <= MaxNodes; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"id":"n` + strings.Repeat("x", i%30) + string(rune('a'+i%26)) + `","type":"comment","settings":{}}`)
	}
	b.WriteString(`],"wires":[]}`)
	if p := Validate(parseSpec(t, b.String()), fakeRefs{}); len(p) == 0 {
		t.Error("too many nodes accepted")
	}
}

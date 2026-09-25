package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

const tagA = "01a0da28-f735-7640-bacf-fcf8aac176cf"

func mimicSpec(elements string) json.RawMessage {
	return json.RawMessage(`{"kind":"mimic","layout":{"type":"canvas","width":1600,"height":900},"widgets":[],"elements":` + elements + `}`)
}

// A process picture with only drawing elements is valid; the canonical form keeps whitelisted keys only.
func TestElementsValid(t *testing.T) {
	spec, err := validateScreenSpec(mimicSpec(`[
		{"id":"t1","type":"symbol","symbol":"tank_vertical","x":100,"y":80,"w":120,"h":220,"tag":"` + tagA + `",
		 "style":{"shade":true,"stroke":"#334155","junk":1},
		 "dyn":[{"prop":"level","lin":[0,100,0,1]},{"prop":"state","map":[{"op":">=","v":1,"to":"run"},{"op":"==","v":0,"to":"stop"}]}],
		 "on":[{"act":"faceplate"},{"act":"screen","target":"boiler-house"}]},
		{"type":"pipe","points":[[10,10],[200,10],[200,300]],"style":{"sw":14,"medium":"#0891b2"},"dyn":[{"prop":"flow","op":">","v":0.5}]},
		{"type":"text","x":20,"y":20,"w":300,"h":30,"text":"<b>Mixer</b> {value}","style":{"font":22,"bold":true,"color":"#111827"}}
	]`))
	if err == nil {
		t.Fatal("unknown style key must be rejected")
	}
	spec, err = validateScreenSpec(mimicSpec(`[
		{"id":"t1","type":"symbol","symbol":"tank_vertical","x":100,"y":80,"w":120,"h":220,"tag":"` + tagA + `",
		 "style":{"shade":true,"stroke":"#334155"},
		 "dyn":[{"prop":"level","lin":[0,100,0,1]},{"prop":"state","map":[{"op":">=","v":1,"to":"run"},{"op":"==","v":0,"to":"stop"}]}],
		 "on":[{"act":"faceplate"},{"act":"screen","target":"boiler-house"}]},
		{"type":"pipe","points":[[10,10],[200,10],[200,300]],"style":{"sw":14,"medium":"#0891b2"},"dyn":[{"prop":"flow","op":">","v":0.5}]},
		{"type":"text","x":20,"y":20,"w":300,"h":30,"text":"<b>Mixer</b> {value}","style":{"font":22,"bold":true,"color":"#111827"},"group":"g1"},
		{"type":"image","src":"customer-logo","x":0,"y":0,"w":100,"h":40,"group":"g1"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	els := elementsOf(spec)
	if len(els) != 4 || els[3]["src"] != "customer-logo" || els[2]["group"] != "g1" || els[1]["id"] != "e2" || els[2]["text"] != "<b>Mixer</b> {value}" {
		t.Fatalf("canonical elements: %+v", els)
	}
	if _, ok := spec["elements"]; !ok {
		t.Fatal("elements dropped")
	}
}

func TestElementsRejected(t *testing.T) {
	cases := map[string]string{
		"unknown type":        `[{"type":"script"}]`,
		"css in colour":       `[{"type":"rect","style":{"fill":"red;background:url(x)"}}]`,
		"bad symbol id":       `[{"type":"symbol","symbol":"../x"}]`,
		"bad tag":             `[{"type":"rect","tag":"x"}]`,
		"pipe one point":      `[{"type":"pipe","points":[[1,1]]}]`,
		"nan-like point":      `[{"type":"line","points":[[1,1],["a",2]]}]`,
		"unknown dynamic":     `[{"type":"rect","dyn":[{"prop":"onclick"}]}]`,
		"bad operator":        `[{"type":"rect","dyn":[{"prop":"blink","op":"=~","v":1}]}]`,
		"bad state":           `[{"type":"symbol","symbol":"pump","dyn":[{"prop":"state","map":[{"op":"==","v":1,"to":"exploding"}]}]}]`,
		"empty lin range":     `[{"type":"rect","dyn":[{"prop":"level","lin":[5,5,0,1]}]}]`,
		"url event":           `[{"type":"rect","on":[{"act":"url","target":"https://evil"}]}]`,
		"screen bad slug":     `[{"type":"rect","on":[{"act":"screen","target":"../admin"}]}]`,
		"command without key": `[{"type":"rect","on":[{"act":"command"}]}]`,
		"duplicate id":        `[{"id":"a","type":"rect"},{"id":"a","type":"rect"}]`,
		"far outside":         `[{"type":"rect","x":999999}]`,
		"image without src":   `[{"type":"image"}]`,
		"image src path":      `[{"type":"image","src":"../etc/passwd"}]`,
		"bad group":           `[{"type":"rect","group":"a b"}]`,
	}
	for name, els := range cases {
		if _, err := validateScreenSpec(mimicSpec(els)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// Grid dashboards have no drawing layer.
	_, err := validateScreenSpec(json.RawMessage(`{"kind":"dashboard","widgets":[{"type":"text"}],"elements":[{"type":"rect"}]}`))
	if err == nil || !strings.Contains(err.Error(), "canvas") {
		t.Errorf("grid with elements: %v", err)
	}
	// An empty picture is still refused.
	if _, err := validateScreenSpec(mimicSpec(`[]`)); err == nil {
		t.Error("empty picture accepted")
	}
}

// One valid and one invalid case per dynamic kind (spec 001, T021).
func TestElementDynamicsPerKind(t *testing.T) {
	valid := map[string]string{
		"fill":    `{"prop":"fill","map":[{"op":">=","v":80,"to":"#dc2626"}]}`,
		"stroke":  `{"prop":"stroke","map":[{"op":"<","v":0,"to":"#2563eb"}]}`,
		"color":   `{"prop":"color","map":[{"op":"!=","v":0,"to":"#16a34a"}]}`,
		"state":   `{"prop":"state","map":[{"op":"==","v":1,"to":"run"}]}`,
		"text":    `{"prop":"text","map":[{"op":"==","v":2,"to":"Fault"}]}`,
		"text0":   `{"prop":"text"}`,
		"visible": `{"prop":"visible","op":">","v":0}`,
		"blink":   `{"prop":"blink","op":">=","v":90,"color":"#dc2626"}`,
		"flow":    `{"prop":"flow","op":">","v":0.5,"speed":2}`,
		"rotate":  `{"prop":"rotate","op":"==","v":1,"speed":0.5}`,
		"level":   `{"prop":"level","lin":[0,100,0,1]}`,
		"opacity": `{"prop":"opacity","lin":[0,1,0.2,1]}`,
		"angle":   `{"prop":"angle","lin":[0,100,-90,90]}`,
		"move_x":  `{"prop":"move_x","lin":[0,100,0,300]}`,
		"move_y":  `{"prop":"move_y","tag":"` + tagA + `","lin":[0,100,0,-200]}`,
	}
	invalid := map[string]string{
		"fill":    `{"prop":"fill","map":[{"op":">=","v":80,"to":"red"}]}`,
		"stroke":  `{"prop":"stroke"}`,
		"color":   `{"prop":"color","map":[{"op":"~","v":0,"to":"#16a34a"}]}`,
		"state":   `{"prop":"state","map":[{"op":"==","v":1,"to":"running"}]}`,
		"text":    `{"prop":"text","map":[{"op":"==","v":2,"to":1}]}`,
		"visible": `{"prop":"visible","v":0}`,
		"blink":   `{"prop":"blink","op":">","v":1,"color":"url(#x)"}`,
		"flow":    `{"prop":"flow","op":">","v":0,"speed":99}`,
		"rotate":  `{"prop":"rotate","op":">","v":"a"}`,
		"level":   `{"prop":"level","lin":[0,100,0]}`,
		"opacity": `{"prop":"opacity"}`,
		"angle":   `{"prop":"angle","lin":[1,1,0,90]}`,
		"move_x":  `{"prop":"move_x","lin":["0",100,0,1]}`,
		"move_y":  `{"prop":"move_y","tag":"nope","lin":[0,100,0,1]}`,
	}
	for name, d := range valid {
		if _, err := validateScreenSpec(mimicSpec(`[{"type":"rect","dyn":[` + d + `]}]`)); err != nil {
			t.Errorf("valid %s refused: %v", name, err)
		}
	}
	for name, d := range invalid {
		if _, err := validateScreenSpec(mimicSpec(`[{"type":"rect","dyn":[` + d + `]}]`)); err == nil {
			t.Errorf("invalid %s accepted", name)
		}
	}
}

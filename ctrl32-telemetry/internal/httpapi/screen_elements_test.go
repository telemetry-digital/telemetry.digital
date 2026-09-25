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
		{"type":"text","x":20,"y":20,"w":300,"h":30,"text":"<b>Mixer</b> {value}","style":{"font":22,"bold":true,"color":"#111827"}}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	els := elementsOf(spec)
	if len(els) != 3 || els[1]["id"] != "e2" || els[2]["text"] != "<b>Mixer</b> {value}" {
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

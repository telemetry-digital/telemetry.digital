package flows

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/rules"
)

type okRefs struct{}

func (okRefs) Datastream(string) bool  { return true }
func (okRefs) Device(string) bool      { return true }
func (okRefs) Channel(string) bool     { return true }
func (okRefs) HostAllowed(string) bool { return true }

func TestFromRule(t *testing.T) {
	const a, b, dev, ch = "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", "33333333-3333-3333-3333-333333333333", "44444444-4444-4444-4444-444444444444"
	var s rules.Spec
	if err := json.Unmarshal([]byte(`{"inputs":[{"name":"temp","datastream_id":"`+a+`"},{"name":"limit","datastream_id":"`+b+`"}],
		"match":"all","conditions":[{"input":"temp","op":">","value":"limit + 2"},{"input":"temp","op":"<","value":120},{"input":"limit","op":"stale","value":600}],
		"for_s":30,"cooldown_s":300,
		"actions":[{"type":"notify","channel_id":"`+ch+`","subject":"{rule}: {event}","text":"{inputs} / {temp}"},{"type":"command","device_id":"`+dev+`","key":"fan","value":true},{"type":"alarm","severity":"critical"}],
		"clear_actions":[{"type":"command","device_id":"`+dev+`","key":"fan","value":"temp - limit"}]}`), &s); err != nil {
		t.Fatal(err)
	}
	sp, notes := FromRule("Overheat", s)
	if p := Validate(&sp, okRefs{}); len(p) > 0 {
		t.Fatalf("converted flow does not validate: %v", p)
	}
	byType := map[string][]NodeSpec{}
	for _, n := range sp.Nodes {
		byType[n.Type] = append(byType[n.Type], n)
	}
	if len(byType["ds_value"]) != 2 || len(byType["join"]) != 1 || len(byType["rate_limit"]) != 1 || len(byType["incident"]) != 2 || len(byType["comment"]) != 1 {
		t.Fatalf("unexpected nodes: %+v", byType)
	}
	cond := byType["stable_for"][0].Settings["condition"].(string)
	if cond != "msg.temp > (msg.limit + 2) && msg.temp < 120" {
		t.Fatalf("condition = %q", cond)
	}
	if v := byType["command"][1].Settings["value"]; v != "msg.temp - msg.limit" {
		t.Fatalf("clear command value = %v", v)
	}
	if v := byType["notify"][0].Settings["subject"]; v != "Overheat: raised" {
		t.Fatalf("subject = %v", v)
	}
	if v := byType["notify"][0].Settings["text"].(string); !strings.Contains(v, "temp={{ msg.temp }}, limit={{ msg.limit }} / {{ msg.temp }}") {
		t.Fatalf("text = %v", v)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "stale") {
		t.Fatalf("notes = %v", notes)
	}
	if !HasCommands(&sp) {
		t.Fatal("converted flow should send commands")
	}
}

func TestFromRuleRuns(t *testing.T) {
	var s rules.Spec
	_ = json.Unmarshal([]byte(`{"inputs":[{"name":"t","datastream_id":"11111111-1111-1111-1111-111111111111"}],"conditions":[{"input":"t","op":">","value":80}],"for_s":0,
		"actions":[{"type":"derive","datastream_id":"22222222-2222-2222-2222-222222222222","expr":"t * 2"}]}`), &s)
	sp, _ := FromRule("r", s)
	if p := Validate(&sp, okRefs{}); len(p) > 0 {
		t.Fatal(p)
	}
}

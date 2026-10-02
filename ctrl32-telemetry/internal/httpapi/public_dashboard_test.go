package httpapi

import (
	"encoding/json"
	"testing"
)

// On a public link the Alarm count reads the same alarms as the Alarms list: those of the dashboard's datastreams,
// and only when the dashboard shows alarms at all.
func TestPublicShareAlarmWidgets(t *testing.T) {
	share := func(js string) *shareInfo {
		si := &shareInfo{DS: map[string]bool{}, Devs: map[string]bool{}, Cams: map[string]bool{}}
		if err := json.Unmarshal([]byte(js), &si.Spec); err != nil {
			t.Fatal(err)
		}
		si.collect()
		return si
	}
	si := share(`{"widgets":[{"type":"alarm_count","bindings":{}},{"type":"value","bindings":{"datastreams":["` + tagA + `"]}}]}`)
	if !si.HasAlarms || !si.DS[tagA] {
		t.Fatalf("alarm count on a public link: alarms %v, datastreams %v", si.HasAlarms, si.DS)
	}
	if si := share(`{"widgets":[{"type":"alarms","bindings":{}}]}`); !si.HasAlarms {
		t.Fatal("alarms list")
	}
	if si := share(`{"widgets":[{"type":"value","bindings":{"datastreams":["` + tagA + `"]}}]}`); si.HasAlarms {
		t.Fatal("a dashboard without alarm widgets exposes alarms")
	}
}

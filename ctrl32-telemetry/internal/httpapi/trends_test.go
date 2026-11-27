package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateTrendSpec(t *testing.T) {
	id := func(n string) string { return "00000000-0000-7000-8000-00000000000" + n }
	ok := `{"curves":[{"datastream_id":"` + strings.ToUpper(id("a")) + `","label":" Temp ","style":"step","min":0,"max":10}],"limits":true}`
	sp, err := validateTrendSpec(json.RawMessage(ok))
	if err != nil {
		t.Fatal(err)
	}
	if sp.Range != "1h" || sp.Curves[0].Datastream != id("a") || sp.Curves[0].Label != "Temp" || !sp.Limits {
		t.Errorf("normalized: %+v", sp)
	}
	for name, bad := range map[string]string{
		"no curves":   `{"curves":[]}`,
		"not an id":   `{"curves":[{"datastream_id":"x"}]}`,
		"twice":       `{"curves":[{"datastream_id":"` + id("a") + `"},{"datastream_id":"` + id("a") + `"}]}`,
		"colour":      `{"curves":[{"datastream_id":"` + id("a") + `","color":"#12"}]}`,
		"style":       `{"curves":[{"datastream_id":"` + id("a") + `","style":"bars"}]}`,
		"axis range":  `{"curves":[{"datastream_id":"` + id("a") + `","min":3,"max":3}]}`,
		"width":       `{"curves":[{"datastream_id":"` + id("a") + `","width":20}]}`,
		"range":       `{"curves":[{"datastream_id":"` + id("a") + `"}],"range":"2h"}`,
		"half window": `{"curves":[{"datastream_id":"` + id("a") + `"}],"to":"2026-01-01T00:00:00Z"}`,
		"backwards":   `{"curves":[{"datastream_id":"` + id("a") + `"}],"from":"2026-01-02T00:00:00Z","to":"2026-01-01T00:00:00Z"}`,
		"not json":    `[]`,
	} {
		if _, err := validateTrendSpec(json.RawMessage(bad)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	fixed, err := validateTrendSpec(json.RawMessage(`{"curves":[{"datastream_id":"` + id("a") + `"}],"from":"2026-01-01T00:00:00Z","to":"2026-01-01T08:00:00Z"}`))
	if err != nil || fixed.Range != "" || fixed.From == nil {
		t.Errorf("a fixed window keeps no range: %+v %v", fixed, err)
	}
}

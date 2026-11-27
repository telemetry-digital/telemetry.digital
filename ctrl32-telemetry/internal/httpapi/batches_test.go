package httpapi

import (
	"math"
	"testing"
)

// What a new sample of the tag means for the batch of a unit.
func TestBatchTagStep(t *testing.T) {
	cases := []struct {
		kind       string
		prev, v    float64
		end, begin bool
		code       string
	}{
		{"run", 0, 1, false, true, ""},
		{"run", 1, 0, true, false, ""},
		{"run", 1, 1, false, false, ""},
		{"run", 1, 2, false, false, ""}, // still running: another value that is not zero begins nothing
		{"run", 0, 0, false, false, ""},
		{"number", 0, 101, false, true, "101"},
		{"number", 101, 102, true, true, "102"},
		{"number", 102, 0, true, false, ""},
		{"number", 102, 102, false, false, ""},
		{"number", 0, 7.5, false, true, "7.5"},
		{"run", 0, math.NaN(), false, false, ""},
	}
	for _, c := range cases {
		end, begin, code := batchTagStep(c.kind, c.prev, c.v)
		if end != c.end || begin != c.begin || code != c.code {
			t.Errorf("%s %v → %v: end %v begin %v code %q, want %v %v %q", c.kind, c.prev, c.v, end, begin, code, c.end, c.begin, c.code)
		}
	}
}

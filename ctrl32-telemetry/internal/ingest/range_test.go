package ingest

import "testing"

func TestRangeOutside(t *testing.T) {
	lo, hi := -55.0, 125.0
	cases := []struct {
		r    Range
		v    float64
		want bool
	}{
		{Range{}, -1e9, false},
		{Range{Min: &lo, Max: &hi}, 20, false},
		{Range{Min: &lo, Max: &hi}, -55, false},
		{Range{Min: &lo, Max: &hi}, 125, false},
		{Range{Min: &lo, Max: &hi}, -127, true},
		{Range{Min: &lo, Max: &hi}, 125.5, true},
		{Range{Min: &lo}, 1e9, false},
		{Range{Max: &hi}, 1e9, true},
	}
	for _, c := range cases {
		if got := c.r.Outside(c.v); got != c.want {
			t.Errorf("%+v Outside(%v) = %v, want %v", c.r, c.v, got, c.want)
		}
	}
}

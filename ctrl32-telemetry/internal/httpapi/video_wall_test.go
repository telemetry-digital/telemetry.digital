package httpapi

import (
	"strings"
	"testing"
)

func TestCheckWallRects(t *testing.T) {
	// one large 3×2, a tall 1×2 beside it and single cells under them on 6×4
	ok := []WallRect{{0, 0, 3, 2}, {3, 0, 1, 2}, {4, 0, 1, 1}, {5, 0, 1, 1}, {0, 2, 1, 1}, {5, 3, 1, 1}}
	if msg := checkWallRects(6, 4, ok); msg != "" {
		t.Fatalf("valid layout refused: %s", msg)
	}
	for _, c := range []struct {
		rects []WallRect
		want  string
	}{
		{[]WallRect{{0, 0, 2, 2}, {1, 1, 1, 1}}, "tiles 1 and 2 overlap"},
		{[]WallRect{{5, 0, 2, 1}}, "outside"},
		{[]WallRect{{0, 3, 1, 2}}, "outside"},
		{[]WallRect{{-1, 0, 1, 1}}, "outside"},
		{[]WallRect{{0, 0, 0, 1}}, "outside"},
	} {
		if msg := checkWallRects(6, 4, c.rects); !strings.Contains(msg, c.want) {
			t.Errorf("%v: got %q, want %q", c.rects, msg, c.want)
		}
	}
}

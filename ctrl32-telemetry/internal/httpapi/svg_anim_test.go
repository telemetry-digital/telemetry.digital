package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWithAnimation(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><g class="c32-spin"><rect width="4" height="1"/></g></svg>`
	out := withAnimation(svg, 2)
	if !strings.HasPrefix(out, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><style>`) {
		t.Fatalf("style sheet not the first child: %.120s", out)
	}
	for _, want := range []string{"@keyframes c32-spin", ".c32-spin{", "c32-spin 0.500s", ".c32-d2{animation-delay:-0.600s}", "prefers-reduced-motion", `<g class="c32-spin">`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if got := withAnimation(`<svg/>`, 1); got != `<svg/>` {
		t.Errorf("self-closing root changed: %s", got)
	}
	if got := withAnimation(`<g/>`, 1); got != `<g/>` {
		t.Errorf("no root changed: %s", got)
	}
}

func TestAnimParams(t *testing.T) {
	for q, want := range map[string]float64{"": -1, "?anim=0": -1, "?anim=1": 1, "?anim=1&speed=3": 3, "?anim=1&speed=0": 0.05,
		"?anim=1&speed=500": 20, "?anim=1&speed=NaN": 1, "?anim=1&speed=x": 1} {
		s, ok := animParams(httptest.NewRequest("GET", "/svg"+q, nil))
		if !ok {
			s = -1
		}
		if s != want {
			t.Errorf("%s: speed %v, want %v", q, s, want)
		}
	}
}

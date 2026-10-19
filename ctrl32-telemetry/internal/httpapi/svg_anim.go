package httpapi

// Animation effects for imported SVG graphics (library objects of kind "svg"). Imported files never carry their own
// animations, style sheets or scripts (svgsanitize.go drops them); instead parts of a graphic are marked with one of
// the classes below and the server adds its own fixed style sheet when the picture asks for animation
// (GET …/svg?anim=1&speed=s). The picture decides when: always, or while an "animate" dynamic of the image element is
// true (e.g. while a blower runs). The style sheet is the only CSS that ever reaches the graphic and contains no
// value from the request other than the clamped speed.

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// svgEffects: class → animation (name, duration at speed 1, timing, extra declarations). Ordered for the docs/UI.
var svgEffects = []struct{ Class, Keyframes, Rule string }{
	{"c32-spin", "@keyframes c32-spin{to{transform:rotate(360deg)}}", "transform-box:fill-box;transform-origin:50% 50%;animation:c32-spin %.3fs linear infinite"},
	{"c32-spin-ccw", "@keyframes c32-spin-ccw{to{transform:rotate(-360deg)}}", "transform-box:fill-box;transform-origin:50% 50%;animation:c32-spin-ccw %.3fs linear infinite"},
	{"c32-sway", "@keyframes c32-sway{from{transform:scaleX(1)}to{transform:scaleX(-1)}}", "transform-box:fill-box;transform-origin:50% 50%;animation:c32-sway %.3fs ease-in-out infinite alternate"},
	{"c32-pulse", "@keyframes c32-pulse{50%{transform:scale(1.15)}}", "transform-box:fill-box;transform-origin:50% 50%;animation:c32-pulse %.3fs ease-in-out infinite"},
	{"c32-rise", "@keyframes c32-rise{0%{transform:translateY(0);opacity:0}15%{opacity:1}85%{opacity:1}100%{transform:translateY(-100%);opacity:0}}", "transform-box:fill-box;animation:c32-rise %.3fs ease-in infinite"},
	{"c32-fall", "@keyframes c32-fall{0%{transform:translateY(0);opacity:0}15%{opacity:1}85%{opacity:1}100%{transform:translateY(100%);opacity:0}}", "transform-box:fill-box;animation:c32-fall %.3fs ease-in infinite"},
	{"c32-flow", "@keyframes c32-flow{to{stroke-dashoffset:-24}}", "stroke-dasharray:8 4;animation:c32-flow %.3fs linear infinite"},
	{"c32-flow-rev", "@keyframes c32-flow-rev{to{stroke-dashoffset:24}}", "stroke-dasharray:8 4;animation:c32-flow-rev %.3fs linear infinite"},
	{"c32-blink", "@keyframes c32-blink{50%{opacity:.15}}", "animation:c32-blink %.3fs steps(2,jump-none) infinite"},
}

// Base durations in seconds at speed 1.
var svgEffectSeconds = map[string]float64{"c32-spin": 1, "c32-spin-ccw": 1, "c32-sway": 1, "c32-pulse": 1.2, "c32-rise": 2.4,
	"c32-fall": 2.4, "c32-flow": 1, "c32-flow-rev": 1, "c32-blink": 1}

// svgAnimStyle returns the style sheet for the given speed (0.05…20; parts with c32-d1…c32-d4 start ¼…1 period later).
func svgAnimStyle(speed float64) string {
	var b strings.Builder
	b.WriteString("<style>")
	for _, e := range svgEffects {
		b.WriteString(e.Keyframes)
		b.WriteString("." + e.Class + "{" + fmt.Sprintf(e.Rule, svgEffectSeconds[e.Class]/speed) + "}")
	}
	for k := 1; k <= 4; k++ {
		fmt.Fprintf(&b, ".c32-d%d{animation-delay:-%.3fs}", k, float64(k)*0.25*2.4/speed)
	}
	b.WriteString("@media (prefers-reduced-motion:reduce){*{animation:none!important}}</style>")
	return b.String()
}

// animParams reads ?anim=1&speed=s; ok=false means a static graphic.
func animParams(r *http.Request) (speed float64, ok bool) {
	q := r.URL.Query()
	if q.Get("anim") != "1" {
		return 0, false
	}
	speed = 1
	if v, err := strconv.ParseFloat(q.Get("speed"), 64); err == nil && v == v {
		speed = min(20, max(0.05, v))
	}
	return speed, true
}

// withAnimation inserts the style sheet as the first child of the root <svg> element.
func withAnimation(svg string, speed float64) string {
	i := strings.Index(svg, "<svg")
	if i < 0 {
		return svg
	}
	j := strings.IndexByte(svg[i:], '>')
	if j < 0 || svg[i+j-1] == '/' {
		return svg
	}
	j += i + 1
	return svg[:j] + svgAnimStyle(speed) + svg[j:]
}

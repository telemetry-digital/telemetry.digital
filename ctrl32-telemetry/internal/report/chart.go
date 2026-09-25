package report

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Point is one chart sample.
type Point struct {
	T time.Time
	V float64
	Q string // quality: "" or "ok" = normal; anything else is marked
}

// ChartSVG renders a deterministic line chart (no external dependency) for embedding in PDFs.
// Optional warning/action limits are drawn as dashed lines.
func ChartSVG(pts []Point, unit string, warning, action *float64) string {
	const w, h, left, right, top, bottom = 900.0, 300.0, 60.0, 20.0, 20.0, 40.0
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" font-family="Helvetica, Arial, sans-serif" font-size="11">`, int(w), int(h), int(w), int(h))
	b.WriteString(`<rect x="0" y="0" width="900" height="300" fill="#ffffff"/>`)
	if len(pts) == 0 {
		b.WriteString(`<text x="450" y="150" text-anchor="middle" fill="#6b7280">no data</text></svg>`)
		return b.String()
	}
	t0, t1 := pts[0].T, pts[len(pts)-1].T
	ymin, ymax := math.Inf(1), math.Inf(-1)
	for _, p := range pts {
		if p.T.Before(t0) {
			t0 = p.T
		}
		if p.T.After(t1) {
			t1 = p.T
		}
		ymin = math.Min(ymin, p.V)
		ymax = math.Max(ymax, p.V)
	}
	for _, l := range []*float64{warning, action} {
		if l != nil {
			ymin = math.Min(ymin, *l)
			ymax = math.Max(ymax, *l)
		}
	}
	if ymax-ymin < 1e-9 {
		ymin -= 1
		ymax += 1
	}
	pad := (ymax - ymin) * 0.08
	ymin -= pad
	ymax += pad
	span := t1.Sub(t0).Seconds()
	if span <= 0 {
		span = 1
	}
	px := func(t time.Time) float64 { return left + (t.Sub(t0).Seconds()/span)*(w-left-right) }
	py := func(v float64) float64 { return top + (1-(v-ymin)/(ymax-ymin))*(h-top-bottom) }

	// grid + y labels
	for i := 0; i <= 4; i++ {
		v := ymin + (ymax-ymin)*float64(i)/4
		y := py(v)
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#e5e7eb"/>`, left, y, w-right, y)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="end" fill="#374151">%s</text>`, left-6, y+4, fmtNum(v))
	}
	// x labels (5)
	for i := 0; i <= 4; i++ {
		t := t0.Add(time.Duration(float64(i) / 4 * span * float64(time.Second)))
		x := px(t)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" fill="#374151">%s</text>`, x, h-bottom+16, t.Format("01-02 15:04"))
	}
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="#374151">%s</text>`, left, top-6, unit)
	// limits
	for _, l := range []struct {
		v     *float64
		color string
		name  string
	}{{warning, "#d97706", "warning"}, {action, "#dc2626", "action"}} {
		if l.v != nil {
			y := py(*l.v)
			fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-dasharray="6,4"/>`, left, y, w-right, y, l.color)
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="end" fill="%s">%s %s</text>`, w-right-4, y-4, l.color, l.name, fmtNum(*l.v))
		}
	}
	// line
	b.WriteString(`<path fill="none" stroke="#2563eb" stroke-width="1.5" d="`)
	for i, p := range pts {
		if i == 0 {
			fmt.Fprintf(&b, "M%.1f,%.1f", px(p.T), py(p.V))
		} else {
			fmt.Fprintf(&b, " L%.1f,%.1f", px(p.T), py(p.V))
		}
	}
	b.WriteString(`"/>`)
	// marked points
	for _, p := range pts {
		if p.Q != "" && p.Q != "ok" {
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="3" fill="#d97706"><title>%s</title></circle>`, px(p.T), py(p.V), p.Q)
		}
	}
	// axes
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#9ca3af"/>`, left, top, left, h-bottom)
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#9ca3af"/>`, left, h-bottom, w-right, h-bottom)
	b.WriteString(`</svg>`)
	return b.String()
}

func fmtNum(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

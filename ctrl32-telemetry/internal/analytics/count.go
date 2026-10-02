package analytics

import (
	"fmt"
	"math"
	"slices"
	"time"
)

// Point is a position in normalized picture coordinates (0…1 of width and height, y downwards).
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Line is a counting line drawn on the picture from A to B. In names the side of the line (looking from A towards B
// on the picture) where the counted area lies: crossing towards it counts "in", away from it "out".
type Line struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	A       Point    `json:"a"`
	B       Point    `json:"b"`
	In      string   `json:"in"`                // left | right
	Classes []string `json:"classes,omitempty"` // empty = every known class
}

// Section measures speed between two lines a known distance apart (on the ground, in metres).
type Section struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	LineA     string   `json:"line_a"`
	LineB     string   `json:"line_b"`
	DistanceM float64  `json:"distance_m"`
	Classes   []string `json:"classes,omitempty"` // empty = vehicles (car, motorcycle, bus, truck)
	MinKmh    float64  `json:"min_kmh,omitempty"` // slower is discarded (default 1)
	MaxKmh    float64  `json:"max_kmh,omitempty"` // faster is discarded as an error (default 250)
}

// Config is what a camera counts.
type Config struct {
	Lines    []Line    `json:"lines"`
	Sections []Section `json:"sections,omitempty"`
	Anchor   string    `json:"anchor,omitempty"` // bottom (default: the point where the object touches the ground) | centre
	Margin   float64   `json:"margin,omitempty"` // dead band around a line, picture fraction (default 0.02)
}

// Validate checks a configuration.
func (c *Config) Validate() error {
	if len(c.Lines) > 16 || len(c.Sections) > 8 {
		return fmt.Errorf("at most 16 lines and 8 speed sections per camera")
	}
	ids := map[string]bool{}
	in := func(p Point) bool { return p.X >= 0 && p.X <= 1 && p.Y >= 0 && p.Y <= 1 }
	for _, l := range c.Lines {
		if l.ID == "" || len(l.ID) > 40 || ids[l.ID] {
			return fmt.Errorf("line: missing or repeated id %q", l.ID)
		}
		ids[l.ID] = true
		if !in(l.A) || !in(l.B) || math.Hypot(l.B.X-l.A.X, l.B.Y-l.A.Y) < 0.02 {
			return fmt.Errorf("line %s: both ends inside the picture (0…1) and not on top of each other", l.Name)
		}
		if l.In != "left" && l.In != "right" {
			return fmt.Errorf("line %s: in must be left or right", l.Name)
		}
		for _, k := range l.Classes {
			if !slices.Contains(Classes, k) {
				return fmt.Errorf("line %s: unknown class %s (%v)", l.Name, k, Classes)
			}
		}
	}
	for _, s := range c.Sections {
		if !ids[s.LineA] || !ids[s.LineB] || s.LineA == s.LineB {
			return fmt.Errorf("speed section %s: two different lines of this camera", s.Name)
		}
		if s.DistanceM < 1 || s.DistanceM > 500 {
			return fmt.Errorf("speed section %s: distance between the lines 1 to 500 m", s.Name)
		}
		for _, k := range s.Classes {
			if !slices.Contains(Classes, k) {
				return fmt.Errorf("speed section %s: unknown class %s", s.Name, k)
			}
		}
	}
	if c.Anchor != "" && c.Anchor != "bottom" && c.Anchor != "centre" {
		return fmt.Errorf("anchor: bottom or centre")
	}
	if c.Margin < 0 || c.Margin > 0.1 {
		return fmt.Errorf("margin: 0 to 0.1")
	}
	return nil
}

// Crossing is one object crossing a line.
type Crossing struct {
	Line  string    `json:"line"`
	Dir   string    `json:"dir"` // in | out
	Class string    `json:"cls"`
	Track int       `json:"track"`
	At    time.Time `json:"t"`
}

// Speed is the measured speed of one object through a section.
type Speed struct {
	Section string    `json:"section"`
	Kmh     float64   `json:"kmh"`
	Class   string    `json:"cls"`
	Dir     string    `json:"dir"` // ab (from line A to line B) | ba
	Track   int       `json:"track"`
	At      time.Time `json:"t"`       // when it crossed the second line
	Seconds float64   `json:"seconds"` // time between the lines
}

// Counter turns detections into crossings and speeds.
type Counter struct {
	Cfg Config
	Tr  Tracker
}

func (c *Counter) anchor(b Box) Point {
	if c.Cfg.Anchor == "centre" {
		return Point{(b.X1 + b.X2) / 2, (b.Y1 + b.Y2) / 2}
	}
	return Point{(b.X1 + b.X2) / 2, b.Y2}
}

func (c *Counter) margin() float64 {
	if c.Cfg.Margin <= 0 {
		return 0.02
	}
	return c.Cfg.Margin
}

// SignedDistance is the distance of p from the line through A and B, positive on the right side of A→B on the
// picture (y downwards), negative on the left.
func SignedDistance(l Line, p Point) float64 {
	dx, dy := l.B.X-l.A.X, l.B.Y-l.A.Y
	n := math.Hypot(dx, dy)
	if n == 0 {
		return 0
	}
	return (dx*(p.Y-l.A.Y) - dy*(p.X-l.A.X)) / n
}

// onSegment reports whether the path p0→p1 meets the line within the drawn segment (5 % tolerance at the ends).
func onSegment(l Line, p0, p1 Point, d0, d1 float64) bool {
	if d0 == d1 {
		return false
	}
	f := d0 / (d0 - d1)
	x := Point{p0.X + f*(p1.X-p0.X), p0.Y + f*(p1.Y-p0.Y)}
	dx, dy := l.B.X-l.A.X, l.B.Y-l.A.Y
	u := ((x.X-l.A.X)*dx + (x.Y-l.A.Y)*dy) / (dx*dx + dy*dy)
	return u >= -0.05 && u <= 1.05
}

// CrossTime interpolates when the object passed the line between two positions at signed distances d0 (at t0) and
// d1 (at t1) of opposite signs: the moment of the crossing lies between the frames, not on one of them.
func CrossTime(d0, d1 float64, t0, t1 time.Time) time.Time {
	if d0 == d1 || t1.Before(t0) {
		return t1
	}
	f := d0 / (d0 - d1)
	f = math.Max(0, math.Min(1, f))
	return t0.Add(time.Duration(f * float64(t1.Sub(t0))))
}

// SpeedKmh computes the speed over a distance from the times an object crossed line A and line B. ok is false for
// equal times or speeds outside [minKmh, maxKmh] (a mismatched track, a reflection, a stopped vehicle).
func SpeedKmh(distanceM float64, tA, tB time.Time, minKmh, maxKmh float64) (kmh float64, dir string, ok bool) {
	dt := tB.Sub(tA).Seconds()
	dir = "ab"
	if dt < 0 {
		dt, dir = -dt, "ba"
	}
	if dt <= 0 || distanceM <= 0 {
		return 0, dir, false
	}
	if minKmh <= 0 {
		minKmh = 1
	}
	if maxKmh <= 0 {
		maxKmh = 250
	}
	kmh = distanceM / dt * 3.6
	return kmh, dir, kmh >= minKmh && kmh <= maxKmh
}

func classOK(list []string, c string, vehiclesByDefault bool) bool {
	if len(list) == 0 {
		return !vehiclesByDefault || IsVehicle(c)
	}
	return slices.Contains(list, c)
}

func sign(v float64) int {
	if v > 0 {
		return 1
	}
	return -1
}

// Frame processes the detections of one frame taken at `at`.
func (c *Counter) Frame(dets []Box, at time.Time) ([]Crossing, []Speed) {
	var cs []Crossing
	var sp []Speed
	m := c.margin()
	for _, t := range c.Tr.Update(dets, at) {
		p := c.anchor(t.Box)
		confirmed := c.Tr.Confirmed(t)
		cls := t.Class()
		// a box cut by the picture border (an object coming into view) has no reliable ground point
		truncated := t.Box.X1 < 0.01 || t.Box.X2 > 0.99 || t.Box.Y2 > 0.99 || (c.Cfg.Anchor == "centre" && t.Box.Y1 < 0.01)
		for _, l := range c.Cfg.Lines {
			if truncated {
				continue
			}
			d := SignedDistance(l, p)
			s := sign(d)
			last, ok := t.side[l.ID]
			if !ok {
				// the first side, even inside the dead band: an object that appears right at the line and then
				// moves clearly to the other side has crossed it
				t.side[l.ID], t.sideP[l.ID], t.sideT[l.ID] = s, p, at
				continue
			}
			if math.Abs(d) <= m {
				continue // on the line: wait until the object is clearly on one side
			}
			if s == last {
				if confirmed {
					t.sideP[l.ID], t.sideT[l.ID] = p, at
				}
				continue
			}
			if !confirmed {
				continue // keep the first side until the object is confirmed, then count the crossing
			}
			p0, t0 := t.sideP[l.ID], t.sideT[l.ID]
			d0 := SignedDistance(l, p0)
			t.side[l.ID], t.sideP[l.ID], t.sideT[l.ID] = s, p, at
			if !onSegment(l, p0, p, d0, d) {
				continue // passed beside the drawn line
			}
			// the moment of the crossing: between the previous frame and this one when the previous position was
			// still on the other side, else between the last clear position and now
			ct := CrossTime(d0, d, t0, at)
			if !t.prevT.IsZero() {
				if dp := SignedDistance(l, t.prevP); sign(dp) != s {
					ct = CrossTime(dp, d, t.prevT, at)
				}
			}
			// an object counts once per line: a track that seems to cross back (jitter at the line, two people
			// taken for one) does not add or cancel anything
			_, done := t.crossT[l.ID]
			if !done {
				t.crossT[l.ID] = ct
			}
			if !done && classOK(l.Classes, cls, false) {
				dir := "out"
				if (s > 0) == (l.In == "right") {
					dir = "in"
				}
				cs = append(cs, Crossing{Line: l.ID, Dir: dir, Class: cls, Track: t.ID, At: ct})
			}
			for _, sec := range c.Cfg.Sections {
				if t.counted[sec.ID] || (sec.LineA != l.ID && sec.LineB != l.ID) {
					continue
				}
				ta, okA := t.crossT[sec.LineA]
				tb, okB := t.crossT[sec.LineB]
				if !okA || !okB {
					continue
				}
				t.counted[sec.ID] = true
				if !classOK(sec.Classes, cls, true) {
					continue
				}
				if kmh, dir, ok := SpeedKmh(sec.DistanceM, ta, tb, sec.MinKmh, sec.MaxKmh); ok {
					sp = append(sp, Speed{Section: sec.ID, Kmh: math.Round(kmh*10) / 10, Class: cls, Dir: dir, Track: t.ID, At: ct, Seconds: math.Abs(tb.Sub(ta).Seconds())})
				}
			}
		}
		t.prevP, t.prevT = p, at
	}
	return cs, sp
}

package analytics

import (
	"bytes"
	"math"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)

// a person box (tall, narrow) standing with its feet at (x, y)
func person(x, y float64) Box {
	return Box{X1: x - 0.03, Y1: y - 0.25, X2: x + 0.03, Y2: y, Score: 0.8, Class: "person"}
}

func car(x, y float64) Box {
	return Box{X1: x - 0.08, Y1: y - 0.08, X2: x + 0.08, Y2: y, Score: 0.8, Class: "car"}
}

// walk feeds frames at fps with one box per frame from pos(i).
func walk(c *Counter, fps float64, n int, box func(i int) []Box) ([]Crossing, []Speed) {
	var cs []Crossing
	var sp []Speed
	for i := 0; i < n; i++ {
		at := t0.Add(time.Duration(float64(i) / fps * float64(time.Second)))
		a, b := c.Frame(box(i), at)
		cs, sp = append(cs, a...), append(sp, b...)
	}
	return cs, sp
}

var door = Line{ID: "door", Name: "Door", A: Point{0.1, 0.5}, B: Point{0.9, 0.5}, In: "right"}

func TestSignedDistanceSides(t *testing.T) {
	// looking from A (left) to B (right) on the picture, "right" is below the line (y grows downwards)
	if d := SignedDistance(door, Point{0.5, 0.7}); d <= 0 {
		t.Fatalf("below the line should be the right side, got %v", d)
	}
	if d := SignedDistance(door, Point{0.5, 0.3}); d >= 0 {
		t.Fatalf("above the line should be the left side, got %v", d)
	}
	if d := SignedDistance(door, Point{0.5, 0.6}); math.Abs(d-0.1) > 1e-9 {
		t.Fatalf("distance %v, want 0.1", d)
	}
}

func TestDirectionMapping(t *testing.T) {
	down := func(i int) []Box { return []Box{person(0.5, 0.3+0.05*float64(i))} } // 0.30 … 0.75
	up := func(i int) []Box { return []Box{person(0.5, 0.75-0.05*float64(i))} }
	cases := []struct {
		name string
		in   string
		mv   func(int) []Box
		want string
	}{
		{"down, inside below", "right", down, "in"},
		{"up, inside below", "right", up, "out"},
		{"down, inside above", "left", down, "out"},
		{"up, inside above", "left", up, "in"},
	}
	for _, tc := range cases {
		l := door
		l.In = tc.in
		c := &Counter{Cfg: Config{Lines: []Line{l}}}
		cs, _ := walk(c, 5, 10, tc.mv)
		if len(cs) != 1 || cs[0].Dir != tc.want || cs[0].Class != "person" || cs[0].Line != "door" {
			t.Errorf("%s: got %+v, want one %s", tc.name, cs, tc.want)
		}
	}
}

func TestCrossingTimeInterpolated(t *testing.T) {
	// feet at 0.30, 0.35 … at 5 fps: the feet reach 0.5 after exactly 4 steps (0.8 s); the dead band is ±0.02 so the
	// crossing is seen at 0.55 (1.0 s) but dated between the frames
	c := &Counter{Cfg: Config{Lines: []Line{door}}}
	cs, _ := walk(c, 5, 10, func(i int) []Box { return []Box{person(0.5, 0.31+0.05*float64(i))} })
	if len(cs) != 1 {
		t.Fatalf("%+v", cs)
	}
	want := t0.Add(760 * time.Millisecond) // 0.31 + 0.05·3.8 = 0.50
	if d := cs[0].At.Sub(want); d < -5*time.Millisecond || d > 5*time.Millisecond {
		t.Fatalf("crossing at %v, want %v", cs[0].At.Sub(t0), want.Sub(t0))
	}
}

func TestJitterAtLineCountsOnce(t *testing.T) {
	ys := []float64{0.30, 0.38, 0.46, 0.53, 0.47, 0.54, 0.46, 0.55, 0.62, 0.7}
	c := &Counter{Cfg: Config{Lines: []Line{door}}}
	cs, _ := walk(c, 5, len(ys), func(i int) []Box { return []Box{person(0.5, ys[i])} })
	if len(cs) != 1 || cs[0].Dir != "in" {
		t.Fatalf("jitter: %+v", cs)
	}
}

func TestNotConfirmedNotCounted(t *testing.T) {
	// seen only twice (a flicker), on both sides
	c := &Counter{Cfg: Config{Lines: []Line{door}}}
	cs, _ := walk(c, 5, 2, func(i int) []Box { return []Box{person(0.5, 0.4+0.2*float64(i))} })
	if len(cs) != 0 {
		t.Fatalf("a track seen twice counted: %+v", cs)
	}
}

func TestCountedWhenConfirmedAfterCrossing(t *testing.T) {
	// crosses between the 1st and the 2nd detection, confirmed at the 3rd: still counted once
	ys := []float64{0.45, 0.56, 0.6, 0.65}
	c := &Counter{Cfg: Config{Lines: []Line{door}}}
	cs, _ := walk(c, 5, len(ys), func(i int) []Box { return []Box{person(0.5, ys[i])} })
	if len(cs) != 1 || cs[0].Dir != "in" {
		t.Fatalf("%+v", cs)
	}
}

func TestBesideTheLineNotCounted(t *testing.T) {
	short := Line{ID: "s", A: Point{0.0, 0.5}, B: Point{0.3, 0.5}, In: "right"}
	c := &Counter{Cfg: Config{Lines: []Line{short}}}
	cs, _ := walk(c, 5, 10, func(i int) []Box { return []Box{person(0.8, 0.3+0.05*float64(i))} })
	if len(cs) != 0 {
		t.Fatalf("passing beside the drawn line counted: %+v", cs)
	}
}

func TestClassesOfALine(t *testing.T) {
	l := door
	l.Classes = []string{"car", "truck"}
	c := &Counter{Cfg: Config{Lines: []Line{l}}}
	cs, _ := walk(c, 5, 10, func(i int) []Box {
		return []Box{person(0.2, 0.3+0.05*float64(i)), car(0.7, 0.3+0.05*float64(i))}
	})
	if len(cs) != 1 || cs[0].Class != "car" {
		t.Fatalf("only the car: %+v", cs)
	}
}

func TestTruncatedBoxIgnored(t *testing.T) {
	// an object coming into view at the bottom border: its box ends at the border, the ground point is unknown
	c := &Counter{Cfg: Config{Lines: []Line{door}}}
	cs, _ := walk(c, 5, 6, func(i int) []Box {
		b := person(0.5, 1.0)
		b.Y1 = 0.9 - 0.1*float64(i) // more of the body appears, the box grows upwards
		return []Box{b}
	})
	if len(cs) != 0 {
		t.Fatalf("truncated boxes counted: %+v", cs)
	}
}

func TestTrackerKeepsIdentities(t *testing.T) {
	// a small, far-away person standing still and a big one walking past next to it
	var tr Tracker
	far := Box{X1: 0.83, Y1: 0.33, X2: 0.87, Y2: 0.57, Score: 0.5, Class: "person"}
	ids := map[int]bool{}
	for i := 0; i < 8; i++ {
		at := t0.Add(time.Duration(i) * 200 * time.Millisecond)
		near := Box{X1: 0.95 - 0.02*float64(i), Y1: 0.25, X2: 1.05 - 0.02*float64(i), Y2: 0.66, Score: 0.8, Class: "person"}
		for _, s := range tr.Update([]Box{far, near}, at) {
			if s.Box == far {
				ids[s.ID] = true
			}
		}
	}
	if len(ids) != 1 {
		t.Fatalf("the far-away person changed identity: %v", ids)
	}
	if n := len(tr.Tracks()); n != 2 {
		t.Fatalf("%d tracks, want 2", n)
	}
	tr.Update(nil, t0.Add(10*time.Second))
	if n := len(tr.Tracks()); n != 0 {
		t.Fatalf("lost tracks not dropped: %d", n)
	}
}

func TestVehicleClassVotes(t *testing.T) {
	var tr Tracker
	for i, cls := range []string{"car", "truck", "car", "car"} {
		b := car(0.5, 0.3+0.02*float64(i))
		b.Class = cls
		tr.Update([]Box{b}, t0.Add(time.Duration(i)*100*time.Millisecond))
	}
	if n := len(tr.Tracks()); n != 1 || tr.Tracks()[0].Class() != "car" {
		t.Fatalf("one car track expected, got %d", n)
	}
}

func TestSpeedKmh(t *testing.T) {
	kmh, dir, ok := SpeedKmh(20, t0, t0.Add(1440*time.Millisecond), 0, 0)
	if !ok || dir != "ab" || math.Abs(kmh-50) > 1e-9 {
		t.Fatalf("20 m in 1.44 s: %v %s %v", kmh, dir, ok)
	}
	if _, dir, ok := SpeedKmh(20, t0.Add(1440*time.Millisecond), t0, 0, 0); !ok || dir != "ba" {
		t.Fatalf("reverse direction: %s %v", dir, ok)
	}
	if _, _, ok := SpeedKmh(20, t0, t0, 0, 0); ok {
		t.Fatal("equal times accepted")
	}
	if _, _, ok := SpeedKmh(20, t0, t0.Add(100*time.Millisecond), 0, 0); ok { // 720 km/h
		t.Fatal("impossible speed accepted")
	}
	if _, _, ok := SpeedKmh(20, t0, t0.Add(2*time.Minute), 0, 0); ok { // 0.6 km/h: a parked car
		t.Fatal("standing vehicle accepted")
	}
	if _, _, ok := SpeedKmh(20, t0, t0.Add(1440*time.Millisecond), 0, 40); ok {
		t.Fatal("above the section maximum accepted")
	}
}

func TestCrossTime(t *testing.T) {
	got := CrossTime(-0.1, 0.3, t0, t0.Add(400*time.Millisecond))
	if got != t0.Add(100*time.Millisecond) {
		t.Fatalf("%v", got.Sub(t0))
	}
	if got := CrossTime(0.2, 0.2, t0, t0.Add(time.Second)); got != t0.Add(time.Second) {
		t.Fatalf("degenerate: %v", got.Sub(t0))
	}
}

func speedCounter() *Counter {
	return &Counter{Cfg: Config{
		Lines: []Line{
			{ID: "a", Name: "A", A: Point{0, 0.3}, B: Point{1, 0.3}, In: "right"},
			{ID: "b", Name: "B", A: Point{0, 0.7}, B: Point{1, 0.7}, In: "right"},
		},
		Sections: []Section{{ID: "s", Name: "Plant road", LineA: "a", LineB: "b", DistanceM: 20}},
	}}
}

func TestSpeedThroughSection(t *testing.T) {
	// a car whose ground point moves 0.4 of the picture (= 20 m between the lines) in 1.44 s: 50 km/h, at 10 fps
	c := speedCounter()
	v := 0.4 / 1.44 // picture heights per second
	_, sp := walk(c, 10, 40, func(i int) []Box { return []Box{car(0.5, 0.15+v*float64(i)/10)} })
	if len(sp) != 1 {
		t.Fatalf("%+v", sp)
	}
	if sp[0].Dir != "ab" || sp[0].Class != "car" || math.Abs(sp[0].Kmh-50) > 0.5 {
		t.Fatalf("%+v", sp[0])
	}
	// the other way
	c = speedCounter()
	_, sp = walk(c, 10, 40, func(i int) []Box { return []Box{car(0.5, 0.85-v*float64(i)/10)} })
	if len(sp) != 1 || sp[0].Dir != "ba" || math.Abs(sp[0].Kmh-50) > 0.5 {
		t.Fatalf("reverse: %+v", sp)
	}
}

func TestSpeedFrameRateError(t *testing.T) {
	// without interpolation the time between the lines is known to ±1 frame: at 10 fps over 10 m at 50 km/h that is
	// ±0.1 s of 0.72 s ≈ ±14 %; with interpolation of the crossing moments the error of straight, steady motion stays
	// far below 1 %. The documentation quotes the frame-quantisation bound for real (uneven) motion.
	c := &Counter{Cfg: Config{
		Lines:    []Line{{ID: "a", A: Point{0, 0.3}, B: Point{1, 0.3}, In: "right"}, {ID: "b", A: Point{0, 0.7}, B: Point{1, 0.7}, In: "right"}},
		Sections: []Section{{ID: "s", LineA: "a", LineB: "b", DistanceM: 10}},
	}}
	v := 0.4 / 0.72
	_, sp := walk(c, 10, 30, func(i int) []Box { return []Box{car(0.5, 0.13+v*float64(i)/10)} })
	if len(sp) != 1 || math.Abs(sp[0].Kmh-50) > 0.5 {
		t.Fatalf("%+v", sp)
	}
}

func TestSpeedOnlyForVehiclesByDefault(t *testing.T) {
	c := speedCounter()
	v := 0.4 / 3.0
	_, sp := walk(c, 10, 60, func(i int) []Box { return []Box{person(0.5, 0.15+v*float64(i)/10)} })
	if len(sp) != 0 {
		t.Fatalf("a person got a speed: %+v", sp)
	}
}

func TestSpeedOutlierRejected(t *testing.T) {
	c := speedCounter()
	c.Cfg.Sections[0].MaxKmh = 30
	v := 0.4 / 1.44 // 50 km/h
	_, sp := walk(c, 10, 40, func(i int) []Box { return []Box{car(0.5, 0.15+v*float64(i)/10)} })
	if len(sp) != 0 {
		t.Fatalf("above the plausible maximum: %+v", sp)
	}
}

func TestConfigValidate(t *testing.T) {
	ok := speedCounter().Cfg
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := []Config{
		{Lines: []Line{{ID: "x", A: Point{0, 0}, B: Point{0.001, 0}, In: "right"}}},
		{Lines: []Line{{ID: "x", A: Point{0, 0}, B: Point{1, 1}, In: "up"}}},
		{Lines: []Line{{ID: "x", A: Point{0, 0}, B: Point{1.5, 1}, In: "left"}}},
		{Lines: []Line{{ID: "x", A: Point{0, 0}, B: Point{1, 1}, In: "left", Classes: []string{"dog"}}}},
		{Lines: []Line{{ID: "x", A: Point{0, 0}, B: Point{1, 1}, In: "left"}}, Sections: []Section{{ID: "s", LineA: "x", LineB: "x", DistanceM: 10}}},
		{Lines: ok.Lines, Sections: []Section{{ID: "s", LineA: "a", LineB: "b", DistanceM: 0.5}}},
	}
	for i, c := range bad {
		if err := c.Validate(); err == nil {
			t.Errorf("config %d accepted", i)
		}
	}
}

func TestLetterboxAndDecode(t *testing.T) {
	lb := NewLetterbox(1920, 1080)
	if lb.SW != 416 || lb.SH != 234 {
		t.Fatalf("letterbox %+v", lb)
	}
	out := make([]float32, 3549*85)
	// one person in the stride-8 grid at cell (10, 12): centre (10.5·8, 12.5·8) = (84, 100), 32 × 80 px
	row := 12*52 + 10
	o := row * 85
	out[o], out[o+1] = 0.5, 0.5
	out[o+2], out[o+3] = float32(math.Log(4)), float32(math.Log(10))
	out[o+4] = 0.9
	out[o+5+0] = 0.95 // person
	// a dog nearby (class 16) is not a person
	o2 := (12*52 + 30) * 85
	out[o2+4], out[o2+5+16], out[o2+5] = 0.9, 0.9, 0.2
	boxes := Decode(out, lb, 0.3)
	if len(boxes) != 1 || boxes[0].Class != "person" {
		t.Fatalf("%+v", boxes)
	}
	b := boxes[0]
	if math.Abs(b.X1-68.0/416) > 1e-6 || math.Abs(b.Y2-140.0/234) > 1e-6 || math.Abs(b.Score-0.855) > 1e-6 {
		t.Fatalf("box %+v", b)
	}
}

func TestNMS(t *testing.T) {
	a := Box{X1: 0.1, Y1: 0.1, X2: 0.3, Y2: 0.5, Score: 0.9, Class: "car"}
	b := Box{X1: 0.11, Y1: 0.1, X2: 0.31, Y2: 0.5, Score: 0.7, Class: "truck"}  // the same vehicle, other class
	p := Box{X1: 0.11, Y1: 0.1, X2: 0.31, Y2: 0.5, Score: 0.6, Class: "person"} // a person in front of it stays
	keep := NMS([]Box{b, p, a}, 0.45)
	if len(keep) != 2 || keep[0] != a || keep[1] != p {
		t.Fatalf("%+v", keep)
	}
}

func TestTensorLayout(t *testing.T) {
	bgr := make([]byte, frameBytes)
	bgr[0], bgr[1], bgr[2] = 10, 20, 30
	dst := make([]float32, 3*InputSize*InputSize)
	Tensor(bgr, dst)
	n := InputSize * InputSize
	if dst[0] != 10 || dst[n] != 20 || dst[2*n] != 30 {
		t.Fatalf("%v %v %v", dst[0], dst[n], dst[2*n])
	}
}

func TestFraming(t *testing.T) {
	var b bytes.Buffer
	at := time.Unix(1759392000, 123456789)
	if err := WriteFrame(&b, at, []byte{0, 0, 0, 1, 0x65}); err != nil {
		t.Fatal(err)
	}
	pr := &pipeRecorder{}
	stamps := make(chan time.Time, 2)
	feed(&b, pr, stamps)
	if got := <-stamps; !got.Equal(at) || !bytes.Equal(pr.buf.Bytes(), []byte{0, 0, 0, 1, 0x65}) || !pr.closed {
		t.Fatalf("%v %x %v", got, pr.buf.Bytes(), pr.closed)
	}
}

type pipeRecorder struct {
	buf    bytes.Buffer
	closed bool
}

func (p *pipeRecorder) Write(b []byte) (int, error) { return p.buf.Write(b) }
func (p *pipeRecorder) Close() error                { p.closed = true; return nil }

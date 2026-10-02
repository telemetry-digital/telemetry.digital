package analytics

import (
	"math"
	"sort"
	"time"
)

// Tracker follows detections from frame to frame: greedy matching of predicted boxes (constant velocity) by overlap,
// or by distance for small, fast objects at a low frame rate. Written for this product (no third-party tracker code).
type Tracker struct {
	MinHits  int           // detections before a track counts (default 3)
	MaxAge   time.Duration // a track not seen for longer is dropped (default 1.5 s)
	MinIoU   float64       // overlap for a match (default 0.15)
	NewScore float64       // a new track needs at least this score (default: any detection passed in)

	next   int
	tracks []*Track
}

// Track is one followed object.
type Track struct {
	ID     int
	Box    Box
	VX, VY float64 // velocity of the box centre, picture widths/heights per second
	Hits   int
	First  time.Time
	Last   time.Time
	votes  map[string]float64

	// per line: the side the object was last clearly on, and where/when it was there (line.go)
	side    map[string]int
	sideP   map[string]Point
	sideT   map[string]time.Time
	prevP   Point
	prevT   time.Time
	counted map[string]bool // speed sections already reported
	crossT  map[string]time.Time
}

// Class is the class the track was seen as most often (weighted by score).
func (t *Track) Class() string {
	best, c := -1.0, t.Box.Class
	for k, v := range t.votes {
		if v > best || (v == best && k < c) {
			best, c = v, k
		}
	}
	return c
}

// Confirmed reports whether the track has been seen often enough to count.
func (tr *Tracker) Confirmed(t *Track) bool { return t.Hits >= tr.minHits() }

func (tr *Tracker) minHits() int {
	if tr.MinHits <= 0 {
		return 3
	}
	return tr.MinHits
}

func (tr *Tracker) maxAge() time.Duration {
	if tr.MaxAge <= 0 {
		return 1500 * time.Millisecond
	}
	return tr.MaxAge
}

// Tracks are the current tracks.
func (tr *Tracker) Tracks() []*Track { return tr.tracks }

func centre(b Box) (float64, float64) { return (b.X1 + b.X2) / 2, (b.Y1 + b.Y2) / 2 }

func (t *Track) predict(at time.Time) Box {
	dt := at.Sub(t.Last).Seconds()
	b := t.Box
	dx, dy := t.VX*dt, t.VY*dt
	b.X1, b.X2, b.Y1, b.Y2 = b.X1+dx, b.X2+dx, b.Y1+dy, b.Y2+dy
	return b
}

// Update matches the detections of a frame taken at `at` with the tracks and returns the tracks seen in it (new
// ones included). Tracks not seen for MaxAge are dropped.
func (tr *Tracker) Update(dets []Box, at time.Time) []*Track {
	type pair struct {
		t, d int
		cost float64
	}
	var pairs []pair
	pred := make([]Box, len(tr.tracks))
	for i, t := range tr.tracks {
		pred[i] = t.predict(at)
	}
	for i, t := range tr.tracks {
		for j, d := range dets {
			if group(t.Box.Class) != group(d.Class) {
				continue
			}
			// the size changes slowly between frames: a small far-away object is never the big one next to it
			if ra := d.area() / math.Max(t.Box.area(), 1e-9); ra < 0.5 || ra > 2 {
				continue
			}
			iou := IoU(pred[i], d)
			if iou >= tr.minIoU() {
				pairs = append(pairs, pair{i, j, 1 - iou})
				continue
			}
			// no overlap (fast object, few frames): close to where it should be now, measured in its own width and
			// height (a person is tall and narrow: a jump of a body width sideways is another person)
			px, py := centre(pred[i])
			dx, dy := centre(d)
			w, h := t.Box.X2-t.Box.X1, t.Box.Y2-t.Box.Y1
			if w > 0 && h > 0 && math.Abs(px-dx) < 0.75*w && math.Abs(py-dy) < 0.4*h {
				pairs = append(pairs, pair{i, j, 1 + math.Abs(px-dx)/w + math.Abs(py-dy)/h})
			}
		}
	}
	sort.Slice(pairs, func(a, b int) bool { return pairs[a].cost < pairs[b].cost })
	usedT := make([]bool, len(tr.tracks))
	usedD := make([]bool, len(dets))
	var seen []*Track
	for _, p := range pairs {
		if usedT[p.t] || usedD[p.d] {
			continue
		}
		usedT[p.t], usedD[p.d] = true, true
		t, d := tr.tracks[p.t], dets[p.d]
		dt := at.Sub(t.Last).Seconds()
		if dt > 0 {
			ox, oy := centre(t.Box)
			nx, ny := centre(d)
			vx, vy := (nx-ox)/dt, (ny-oy)/dt
			if t.Hits == 1 {
				t.VX, t.VY = vx, vy
			} else {
				t.VX, t.VY = 0.5*t.VX+0.5*vx, 0.5*t.VY+0.5*vy
			}
		}
		t.Box = d
		t.Hits++
		t.Last = at
		t.votes[d.Class] += d.Score
		seen = append(seen, t)
	}
	for j, d := range dets {
		if usedD[j] || d.Score < tr.NewScore {
			continue
		}
		tr.next++
		t := &Track{ID: tr.next, Box: d, Hits: 1, First: at, Last: at, votes: map[string]float64{d.Class: d.Score},
			side: map[string]int{}, sideP: map[string]Point{}, sideT: map[string]time.Time{}, counted: map[string]bool{}, crossT: map[string]time.Time{}}
		tr.tracks = append(tr.tracks, t)
		seen = append(seen, t)
	}
	keep := tr.tracks[:0]
	for _, t := range tr.tracks {
		if at.Sub(t.Last) <= tr.maxAge() {
			keep = append(keep, t)
		}
	}
	tr.tracks = keep
	return seen
}

func (tr *Tracker) minIoU() float64 {
	if tr.MinIoU <= 0 {
		return 0.15
	}
	return tr.MinIoU
}

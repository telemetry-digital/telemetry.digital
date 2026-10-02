// Package analytics counts objects (people and vehicles) crossing lines drawn on a camera picture and measures the
// speed of vehicles between two lines (spec 029). It runs in a separate worker process (cmd: analytics-worker): the
// server feeds it the camera's sub-stream, an unmodified ffmpeg decodes it to small pictures, a person/vehicle detector
// (YOLOX, Apache-2.0, through ONNX Runtime, MIT) finds the objects, a tracker follows them and the counter reports
// crossings and speeds as JSON lines. No picture leaves the worker and nothing is stored but the numbers.
//
// This file: the detector's pre- and post-processing (letterbox geometry, YOLOX output decoding, non-maximum
// suppression) — pure Go, unit-tested without the model.
package analytics

import (
	"math"
	"sort"
)

// InputSize is the square input of the YOLOX models (nano and tiny are exported at 416 × 416).
const InputSize = 416

// COCO class indices the counter knows. The models detect 80 classes; only these are kept.
var cocoClass = map[int]string{0: "person", 1: "bicycle", 2: "car", 3: "motorcycle", 5: "bus", 7: "truck"}

// Classes are the object classes a line can count, in display order.
var Classes = []string{"person", "bicycle", "car", "motorcycle", "bus", "truck"}

// VehicleClasses are counted together as "vehicles".
var VehicleClasses = []string{"car", "motorcycle", "bus", "truck"}

// IsVehicle reports whether a class is a motor vehicle.
func IsVehicle(c string) bool {
	for _, v := range VehicleClasses {
		if v == c {
			return true
		}
	}
	return false
}

// group is the tracking group of a class: a vehicle may be seen as car in one frame and truck in the next.
func group(c string) string {
	if IsVehicle(c) {
		return "vehicle"
	}
	return c
}

// Box is a detection in normalized picture coordinates (0…1 of width and height).
type Box struct {
	X1, Y1, X2, Y2 float64
	Score          float64
	Class          string
}

func (b Box) area() float64 { return math.Max(0, b.X2-b.X1) * math.Max(0, b.Y2-b.Y1) }

// IoU is the intersection over union of two boxes.
func IoU(a, b Box) float64 {
	ix := math.Min(a.X2, b.X2) - math.Max(a.X1, b.X1)
	iy := math.Min(a.Y2, b.Y2) - math.Max(a.Y1, b.Y1)
	if ix <= 0 || iy <= 0 {
		return 0
	}
	in := ix * iy
	u := a.area() + b.area() - in
	if u <= 0 {
		return 0
	}
	return in / u
}

// Letterbox describes how a W × H picture is scaled into the square model input: scaled by R, placed at the top-left
// corner, the rest padded (grey 114, as YOLOX was trained). SW × SH is the scaled picture inside the input.
type Letterbox struct {
	W, H   int
	R      float64
	SW, SH int
}

// NewLetterbox computes the geometry for a picture of w × h.
func NewLetterbox(w, h int) Letterbox {
	if w <= 0 || h <= 0 {
		w, h = InputSize, InputSize
	}
	r := math.Min(float64(InputSize)/float64(w), float64(InputSize)/float64(h))
	sw, sh := int(math.Round(float64(w)*r)), int(math.Round(float64(h)*r))
	// ffmpeg scales to even sizes for most pixel formats; keep the same rounding
	if sw%2 == 1 {
		sw--
	}
	if sh%2 == 1 {
		sh--
	}
	return Letterbox{W: w, H: h, R: r, SW: max(2, sw), SH: max(2, sh)}
}

// Tensor turns a 416 × 416 BGR picture (3 bytes per pixel, rows top to bottom) into the CHW float input of YOLOX
// (raw 0…255 values, no normalisation; the models of release 0.1.1 expect that). dst must hold 3·416·416 values.
func Tensor(bgr []byte, dst []float32) {
	const n = InputSize * InputSize
	for i := 0; i < n && 3*i+2 < len(bgr); i++ {
		dst[i] = float32(bgr[3*i])
		dst[n+i] = float32(bgr[3*i+1])
		dst[2*n+i] = float32(bgr[3*i+2])
	}
}

// Decode turns the raw YOLOX output (rows of 85 values: x, y, w, h relative to the grid cell, objectness, 80 class
// probabilities — objectness and classes already passed through a sigmoid) into boxes of the known classes with a
// score of at least minScore, in normalized picture coordinates of the letterboxed picture.
func Decode(out []float32, lb Letterbox, minScore float64) []Box {
	const stride = 85
	var boxes []Box
	row := 0
	for _, s := range []int{8, 16, 32} {
		g := InputSize / s
		for gy := 0; gy < g; gy++ {
			for gx := 0; gx < g; gx++ {
				o := row * stride
				row++
				if o+stride > len(out) {
					return boxes
				}
				obj := float64(out[o+4])
				if obj < minScore {
					continue
				}
				best, bc := 0.0, -1
				for c := 0; c < 80; c++ { // the most likely of all 80 classes; a dog is not a person
					if p := float64(out[o+5+c]); p > best {
						best, bc = p, c
					}
				}
				score := obj * best
				if _, known := cocoClass[bc]; !known || score < minScore {
					continue
				}
				cx := (float64(out[o]) + float64(gx)) * float64(s)
				cy := (float64(out[o+1]) + float64(gy)) * float64(s)
				w := math.Exp(math.Min(float64(out[o+2]), 10)) * float64(s)
				h := math.Exp(math.Min(float64(out[o+3]), 10)) * float64(s)
				b := Box{X1: (cx - w/2) / float64(lb.SW), Y1: (cy - h/2) / float64(lb.SH), X2: (cx + w/2) / float64(lb.SW), Y2: (cy + h/2) / float64(lb.SH), Score: score, Class: cocoClass[bc]}
				b.X1, b.Y1, b.X2, b.Y2 = clamp01(b.X1), clamp01(b.Y1), clamp01(b.X2), clamp01(b.Y2)
				if b.X2-b.X1 < 0.002 || b.Y2-b.Y1 < 0.002 {
					continue
				}
				boxes = append(boxes, b)
			}
		}
	}
	return boxes
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// NMS keeps the best box among boxes of the same tracking group that overlap more than iou.
func NMS(boxes []Box, iou float64) []Box {
	sort.SliceStable(boxes, func(i, j int) bool { return boxes[i].Score > boxes[j].Score })
	var keep []Box
	for _, b := range boxes {
		ok := true
		for _, k := range keep {
			if group(k.Class) == group(b.Class) && IoU(k, b) > iou {
				ok = false
				break
			}
		}
		if ok {
			keep = append(keep, b)
		}
	}
	return keep
}

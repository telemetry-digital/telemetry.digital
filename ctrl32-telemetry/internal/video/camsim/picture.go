package camsim

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"strings"
	"time"
)

// font5x7 holds the glyphs the picture needs; each row is five bits, the most significant bit on the left.
var font5x7 = map[rune][7]byte{
	'0': {0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E}, '1': {0x04, 0x0C, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'2': {0x0E, 0x11, 0x01, 0x02, 0x04, 0x08, 0x1F}, '3': {0x1F, 0x02, 0x04, 0x02, 0x01, 0x11, 0x0E},
	'4': {0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02}, '5': {0x1F, 0x10, 0x1E, 0x01, 0x01, 0x11, 0x0E},
	'6': {0x06, 0x08, 0x10, 0x1E, 0x11, 0x11, 0x0E}, '7': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08},
	'8': {0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E}, '9': {0x0E, 0x11, 0x11, 0x0F, 0x01, 0x02, 0x0C},
	'A': {0x0E, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11}, 'B': {0x1E, 0x11, 0x11, 0x1E, 0x11, 0x11, 0x1E},
	'C': {0x0E, 0x11, 0x10, 0x10, 0x10, 0x11, 0x0E}, 'D': {0x1C, 0x12, 0x11, 0x11, 0x11, 0x12, 0x1C},
	'E': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x1F}, 'F': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x10},
	'G': {0x0E, 0x11, 0x10, 0x17, 0x11, 0x11, 0x0F}, 'H': {0x11, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'I': {0x0E, 0x04, 0x04, 0x04, 0x04, 0x04, 0x0E}, 'J': {0x07, 0x02, 0x02, 0x02, 0x02, 0x12, 0x0C},
	'K': {0x11, 0x12, 0x14, 0x18, 0x14, 0x12, 0x11}, 'L': {0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1F},
	'M': {0x11, 0x1B, 0x15, 0x15, 0x11, 0x11, 0x11}, 'N': {0x11, 0x11, 0x19, 0x15, 0x13, 0x11, 0x11},
	'O': {0x0E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E}, 'P': {0x1E, 0x11, 0x11, 0x1E, 0x10, 0x10, 0x10},
	'Q': {0x0E, 0x11, 0x11, 0x11, 0x15, 0x12, 0x0D}, 'R': {0x1E, 0x11, 0x11, 0x1E, 0x14, 0x12, 0x11},
	'S': {0x0F, 0x10, 0x10, 0x0E, 0x01, 0x01, 0x1E}, 'T': {0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04},
	'U': {0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E}, 'V': {0x11, 0x11, 0x11, 0x11, 0x11, 0x0A, 0x04},
	'W': {0x11, 0x11, 0x11, 0x15, 0x15, 0x15, 0x0A}, 'X': {0x11, 0x11, 0x0A, 0x04, 0x0A, 0x11, 0x11},
	'Y': {0x11, 0x11, 0x11, 0x0A, 0x04, 0x04, 0x04}, 'Z': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x10, 0x1F},
	':': {0x00, 0x0C, 0x0C, 0x00, 0x0C, 0x0C, 0x00}, '-': {0x00, 0x00, 0x00, 0x1F, 0x00, 0x00, 0x00},
	'.': {0x00, 0x00, 0x00, 0x00, 0x00, 0x0C, 0x0C}, '+': {0x00, 0x04, 0x04, 0x1F, 0x04, 0x04, 0x00}, '/': {0x01, 0x01, 0x02, 0x04, 0x08, 0x10, 0x10},
}

// Picture draws a test scene: a background in the camera's own colour, a moving bar and a ball, the camera label
// and a clock, so a viewer can tell cameras apart and see that the picture is live.
func Picture(label string, w, h int, t time.Time, frame int) *image.RGBA {
	return PictureView(label, w, h, t, frame, 0, 0, 0)
}

// PictureView draws the scene as seen by a PTZ camera: pan and tilt (-1…1) shift the scene, zoom (0…1) enlarges it.
func PictureView(label string, w, h int, t time.Time, frame int, pan, tilt, zoom float64) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	sc := 1 + 2*zoom
	// maps a scene point (x, y) to the picture
	px := func(x int) int { return int(float64(w)/2 + (float64(x)-float64(w)/2-pan*float64(w)/2)*sc) }
	py := func(y int) int { return int(float64(h)/2 + (float64(y)-float64(h)/2+tilt*float64(h)/2)*sc) }
	ps := func(n int) int { return int(float64(n) * sc) }
	hs := fnv.New32a()
	hs.Write([]byte(label))
	hue := float64(hs.Sum32()%360) / 360
	for y := 0; y < h; y++ {
		c := hsv(hue, 0.55, 0.25+0.35*float64(y)/float64(h))
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	// a floor line and a moving bar
	fill(img, 0, py(h*3/4), w, 2, color.RGBA{255, 255, 255, 80})
	bx := (frame * 4) % (w + w/8)
	fill(img, px(bx-w/8), py(h/2), ps(w/10), ps(h/4), hsv(hue+0.5, 0.7, 0.9))
	// a fixed landmark (a house) so that pan and tilt are visible
	fill(img, px(w/8), py(h/2-h/16), ps(w/8), ps(h/4+h/16), color.RGBA{200, 200, 210, 255})
	fill(img, px(w/8+w/32), py(h/2+h/16), ps(w/24), ps(h/6), color.RGBA{90, 60, 40, 255})
	// the ball
	bxx := px(w/2 + int(float64(w/3)*sinTri(float64(frame)/40)))
	byy := py(h/3 + int(float64(h/8)*sinTri(float64(frame)/13)))
	r := max(3, ps(h/20))
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			if x*x+y*y <= r*r {
				setSafe(img, bxx+x, byy+y, color.RGBA{250, 240, 90, 255})
			}
		}
	}
	scale := max(1, h/60)
	text(img, 6, 6, scale, strings.ToUpper(label), color.RGBA{255, 255, 255, 255})
	if pan != 0 || tilt != 0 || zoom != 0 {
		ptz := fmt.Sprintf("P%+.1f T%+.1f Z%.1f", pan, tilt, zoom)
		text(img, 6, 6+9*scale, max(1, scale/2+1), strings.ToUpper(ptz), color.RGBA{250, 240, 90, 255})
	}
	ts := t.Format("15:04:05")
	text(img, w-6-len(ts)*6*scale, h-6-7*scale, scale, ts, color.RGBA{255, 255, 255, 255})
	return img
}

// sinTri is a cheap periodic wave in [-1, 1].
func sinTri(x float64) float64 {
	f := x - float64(int(x))
	if f < 0.5 {
		return 4*f - 1
	}
	return 3 - 4*f
}

func text(img *image.RGBA, x, y, scale int, s string, c color.RGBA) {
	fill(img, x-2, y-2, len(s)*6*scale+3, 7*scale+4, color.RGBA{0, 0, 0, 255})
	for _, ch := range s {
		g, ok := font5x7[ch]
		if ok {
			for row := 0; row < 7; row++ {
				for col := 0; col < 5; col++ {
					if g[row]&(0x10>>col) != 0 {
						fill(img, x+col*scale, y+row*scale, scale, scale, c)
					}
				}
			}
		}
		x += 6 * scale
	}
}

func fill(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			if c.A == 255 {
				setSafe(img, xx, yy, c)
			} else if image.Pt(xx, yy).In(img.Rect) {
				o := img.RGBAAt(xx, yy)
				a := uint32(c.A)
				mix := func(p, q uint8) uint8 { return uint8((uint32(p)*(255-a) + uint32(q)*a) / 255) }
				img.SetRGBA(xx, yy, color.RGBA{mix(o.R, c.R), mix(o.G, c.G), mix(o.B, c.B), 255})
			}
		}
	}
}

func setSafe(img *image.RGBA, x, y int, c color.RGBA) {
	if image.Pt(x, y).In(img.Rect) {
		img.SetRGBA(x, y, c)
	}
}

func hsv(h, s, v float64) color.RGBA {
	h -= float64(int(h))
	i := int(h * 6)
	f := h*6 - float64(i)
	p, q, t := v*(1-s), v*(1-f*s), v*(1-(1-f)*s)
	var r, g, b float64
	switch i % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return color.RGBA{uint8(r * 255), uint8(g * 255), uint8(b * 255), 255}
}

// toYCbCr converts to a 4:2:0 picture for the H.264 encoder.
func toYCbCr(img *image.RGBA) *image.YCbCr {
	b := img.Bounds()
	out := image.NewYCbCr(b, image.YCbCrSubsampleRatio420)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := img.RGBAAt(x, y)
			yy, cb, cr := color.RGBToYCbCr(c.R, c.G, c.B)
			// H.264 without colour information means limited ("TV") range
			out.Y[out.YOffset(x, y)] = uint8(16 + int(yy)*219/255)
			if x%2 == 0 && y%2 == 0 {
				ci := out.COffset(x, y)
				out.Cb[ci], out.Cr[ci] = uint8(16+int(cb)*224/255), uint8(16+int(cr)*224/255)
			}
		}
	}
	return out
}

// genicon renders the PWA icons (PNG 192 and 512) with the standard library only, so the repository carries no
// binary tooling. Usage: go run ./tools/genicon internal/httpapi/static
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	dir := "internal/httpapi/static"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	for _, size := range []int{192, 512} {
		img := render(size)
		f, err := os.Create(filepath.Join(dir, "icon-"+itoa(size)+".png"))
		if err != nil {
			panic(err)
		}
		if err := png.Encode(f, img); err != nil {
			panic(err)
		}
		f.Close()
	}
}

func render(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	blue := color.RGBA{0x0b, 0x5f, 0xff, 0xff}
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}
	s := float64(size) / 512
	radius := 96 * s
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if inRoundedRect(float64(x)+0.5, float64(y)+0.5, 0, 0, float64(size), float64(size), radius) {
				img.Set(x, y, blue)
			}
		}
	}
	bars := [][4]float64{{112, 296, 56, 112}, {200, 232, 56, 176}, {288, 168, 56, 240}}
	for _, b := range bars {
		fillRounded(img, b[0]*s, b[1]*s, b[2]*s, b[3]*s, 12*s, white)
	}
	fillCircle(img, 376*s, 120*s, 32*s, white)
	// short diagonal "signal" stroke from the dot toward the bars
	for t := 0.0; t <= 1; t += 0.002 {
		fillCircle(img, (376-60*t)*s, (120+96*t)*s, 10*s, white)
	}
	return img
}

func inRoundedRect(px, py, x, y, w, h, r float64) bool {
	if px < x || py < y || px > x+w || py > y+h {
		return false
	}
	cx, cy := px, py
	if px < x+r {
		cx = x + r
	} else if px > x+w-r {
		cx = x + w - r
	}
	if py < y+r {
		cy = y + r
	} else if py > y+h-r {
		cy = y + h - r
	}
	dx, dy := px-cx, py-cy
	return dx*dx+dy*dy <= r*r
}

func fillRounded(img *image.RGBA, x, y, w, h, r float64, c color.RGBA) {
	for py := int(y); py <= int(y+h); py++ {
		for px := int(x); px <= int(x+w); px++ {
			if inRoundedRect(float64(px)+0.5, float64(py)+0.5, x, y, w, h, r) {
				img.Set(px, py, c)
			}
		}
	}
}

func fillCircle(img *image.RGBA, cx, cy, r float64, c color.RGBA) {
	for py := int(cy - r); py <= int(cy+r); py++ {
		for px := int(cx - r); px <= int(cx+r); px++ {
			dx, dy := float64(px)+0.5-cx, float64(py)+0.5-cy
			if dx*dx+dy*dy <= r*r {
				img.Set(px, py, c)
			}
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

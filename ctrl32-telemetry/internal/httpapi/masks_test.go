package httpapi

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestMaskJPEG(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 100; x++ {
			src.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, src, &jpeg.Options{Quality: 95})
	out, err := maskJPEG(buf.Bytes(), []maskPolygon{{{0, 0}, {0.5, 0}, {0.5, 1}, {0, 1}}})
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	lum := func(x, y int) uint32 { r, g, b, _ := img.At(x, y).RGBA(); return (r + g + b) / 3 >> 8 }
	if lum(20, 25) > 30 {
		t.Fatalf("masked half not black: %d", lum(20, 25))
	}
	if lum(80, 25) < 220 {
		t.Fatalf("unmasked half changed: %d", lum(80, 25))
	}
}

func TestValidateMasks(t *testing.T) {
	for _, bad := range []string{`{}`, `[[[0,0],[1,0]]]`, `[[[0,0],[1.5,0],[1,1]]]`} {
		if _, msg := validateMasks(json.RawMessage(bad)); msg == "" {
			t.Errorf("accepted %s", bad)
		}
	}
	m, msg := validateMasks(json.RawMessage(`[[[0,0],[0.5,0],[0.5,0.5]]]`))
	if msg != "" || len(m) != 1 {
		t.Fatalf("valid mask refused: %s", msg)
	}
	if !inPolygon(0.4, 0.1, m[0]) || inPolygon(0.1, 0.4, m[0]) {
		t.Fatal("point in triangle")
	}
}

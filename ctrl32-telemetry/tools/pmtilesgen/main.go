// pmtilesgen writes a small raster PMTiles archive with synthetic 256×256 PNG tiles (a coloured grid with the
// tile coordinates drawn as bars) for tests and demonstrations of self-hosted maps:
//
//	go run ./tools/pmtilesgen -o bin/sample.pmtiles -maxzoom 4
//
// Real maps: create an MBTiles raster set (e.g. with tilemaker/tileserver or from OpenMapTiles raster exports)
// and convert it with `pmtiles convert map.mbtiles map.pmtiles` (https://github.com/protomaps/go-pmtiles), or
// download a ready raster PMTiles archive of your region; then set [map] pmtiles in config.toml.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/pmtiles"
)

func tilePNG(z uint8, x, y uint32) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	base := color.RGBA{uint8(200 - 30*int(z)), uint8(210 - 10*int(x%8)), uint8(220 - 10*int(y%8)), 255}
	for py := 0; py < 256; py++ {
		for px := 0; px < 256; px++ {
			c := base
			if px < 2 || py < 2 || px > 253 || py > 253 {
				c = color.RGBA{60, 60, 60, 255} // border
			} else if (px/32+py/32)%2 == 0 {
				c = color.RGBA{base.R - 12, base.G - 12, base.B - 12, 255} // checkerboard
			}
			img.Set(px, py, c)
		}
	}
	// coordinate "bars": z bars on top, x bars on the left, y bars on the right (readable without fonts)
	bar := func(x0, y0, w, h int, c color.RGBA) {
		for py := y0; py < y0+h && py < 256; py++ {
			for px := x0; px < x0+w && px < 256; px++ {
				img.Set(px, py, c)
			}
		}
	}
	for i := 0; i < int(z); i++ {
		bar(16+i*14, 12, 10, 10, color.RGBA{30, 30, 30, 255})
	}
	for i := 0; i < int(x%16); i++ {
		bar(12, 40+i*12, 8, 8, color.RGBA{180, 30, 30, 255})
	}
	for i := 0; i < int(y%16); i++ {
		bar(236, 40+i*12, 8, 8, color.RGBA{30, 30, 180, 255})
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func main() {
	out := flag.String("o", "sample.pmtiles", "output file")
	maxZoom := flag.Int("maxzoom", 4, "highest zoom level (tiles = sum of 4^z)")
	leaf := flag.Int("leaf", 64, "entries per leaf directory (0 = root only)")
	flag.Parse()
	tiles := map[pmtiles.TileKey][]byte{}
	for z := 0; z <= *maxZoom; z++ {
		n := uint32(1) << z
		for x := uint32(0); x < n; x++ {
			for y := uint32(0); y < n; y++ {
				tiles[pmtiles.TileKey{Z: uint8(z), X: x, Y: y}] = tilePNG(uint8(z), x, y)
			}
		}
	}
	err := pmtiles.Write(*out, tiles, pmtiles.WriteOptions{TileType: pmtiles.TileTypePNG, LeafSize: *leaf,
		Metadata: map[string]any{"name": "ctrl32 sample tiles", "attribution": "synthetic sample tiles", "format": "png"},
		MinLon:   -180, MinLat: -85.05, MaxLon: 180, MaxLat: 85.05, CenterZoom: 2, CenterLon: 19.5, CenterLat: 48.7})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	st, _ := os.Stat(*out)
	fmt.Printf("wrote %s: %d tiles, %d bytes\n", *out, len(tiles), st.Size())
}

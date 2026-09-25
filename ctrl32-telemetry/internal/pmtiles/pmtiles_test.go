package pmtiles

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestHilbert(t *testing.T) {
	// reference values from the PMTiles specification / JS implementation
	cases := []struct {
		z    uint8
		x, y uint32
		id   uint64
	}{{0, 0, 0, 0}, {1, 0, 0, 1}, {1, 0, 1, 2}, {1, 1, 1, 3}, {1, 1, 0, 4}, {2, 0, 0, 5}, {12, 3423, 1763, 19078479}}
	for _, c := range cases {
		if got := ZxyToID(c.z, c.x, c.y); got != c.id {
			t.Errorf("ZxyToID(%d,%d,%d) = %d, want %d", c.z, c.x, c.y, got, c.id)
		}
		z, x, y := IDToZxy(c.id)
		if z != c.z || x != c.x || y != c.y {
			t.Errorf("IDToZxy(%d) = %d/%d/%d, want %d/%d/%d", c.id, z, x, y, c.z, c.x, c.y)
		}
	}
}

func TestEntriesRoundTrip(t *testing.T) {
	in := []Entry{{TileID: 0, Offset: 0, Length: 10, RunLength: 1}, {TileID: 1, Offset: 10, Length: 20, RunLength: 4}, {TileID: 9, Offset: 100, Length: 5, RunLength: 0}}
	out, err := DeserializeEntries(SerializeEntries(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(in) {
		t.Fatalf("%d entries", len(out))
	}
	for i := range in {
		if out[i] != in[i] {
			t.Errorf("entry %d: %+v != %+v", i, out[i], in[i])
		}
	}
	h := Header{RootOffset: 127, RootLength: 9, TileOffset: 500, TileLength: 7, TileType: TileTypePNG, MinZoom: 1, MaxZoom: 5, MinLon: 16.8, MinLat: 47.7, MaxLon: 22.6, MaxLat: 49.6, CenterZoom: 7, CenterLon: 19.5, CenterLat: 48.7, Clustered: true, InternalCompression: CompressionGzip, TileCompression: CompressionNone}
	h2, err := DeserializeHeader(SerializeHeader(h))
	if err != nil {
		t.Fatal(err)
	}
	if h2 != h {
		t.Errorf("header round trip:\n%+v\n%+v", h, h2)
	}
}

func TestWriteAndRead(t *testing.T) {
	for _, leaf := range []int{0, 8} {
		path := filepath.Join(t.TempDir(), fmt.Sprintf("t%d.pmtiles", leaf))
		tiles := map[TileKey][]byte{}
		for z := uint8(0); z <= 3; z++ {
			n := uint32(1) << z
			for x := uint32(0); x < n; x++ {
				for y := uint32(0); y < n; y++ {
					if z == 3 && (x+y)%2 == 1 {
						continue // holes: absent tiles
					}
					tiles[TileKey{z, x, y}] = []byte(fmt.Sprintf("tile-%d-%d-%d", z, x, y))
				}
			}
		}
		tiles[TileKey{2, 0, 0}] = []byte("same")
		tiles[TileKey{2, 1, 0}] = []byte("same") // deduplicated content
		if err := Write(path, tiles, WriteOptions{TileType: TileTypePNG, LeafSize: leaf, Metadata: map[string]any{"name": "test"}}); err != nil {
			t.Fatal(err)
		}
		a, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		h := a.Header()
		if h.MinZoom != 0 || h.MaxZoom != 3 || h.TileType != TileTypePNG || a.ContentType() != "image/png" || a.Ext() != "png" {
			t.Errorf("header %+v", h)
		}
		if leaf > 0 && h.LeafLength == 0 {
			t.Errorf("expected leaf directories")
		}
		for k, want := range tiles {
			got, err := a.Tile(k.Z, k.X, k.Y)
			if err != nil || !bytes.Equal(got, want) {
				t.Errorf("leaf=%d tile %v: %q %v", leaf, k, got, err)
			}
		}
		if _, err := a.Tile(3, 1, 0); !errors.Is(err, ErrNotFound) {
			t.Errorf("hole should be not found, got %v", err)
		}
		if _, err := a.Tile(4, 0, 0); !errors.Is(err, ErrNotFound) {
			t.Errorf("zoom outside range should be not found")
		}
		if _, err := a.Tile(2, 9, 0); !errors.Is(err, ErrNotFound) {
			t.Errorf("x outside level should be not found")
		}
		a.Close()
	}
}

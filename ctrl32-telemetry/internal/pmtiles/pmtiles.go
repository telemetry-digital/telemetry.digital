// Package pmtiles reads PMTiles v3 archives (https://github.com/protomaps/PMTiles/blob/main/spec/v3/spec.md):
// a single file with a header, a root directory, optional leaf directories and the tile data, addressed by
// Hilbert-curve tile ids. The server serves raster archives (PNG/JPEG/WebP) as XYZ tiles for the map widgets, so
// an intranet installation needs no tile service on the Internet (URS J-02, self-hosted maps). Vector (MVT)
// archives are readable too but need a vector renderer in the browser.
package pmtiles

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"sync"
)

const (
	CompressionUnknown = 0
	CompressionNone    = 1
	CompressionGzip    = 2
	CompressionBrotli  = 3
	CompressionZstd    = 4

	TileTypeUnknown = 0
	TileTypeMVT     = 1
	TileTypePNG     = 2
	TileTypeJPEG    = 3
	TileTypeWebP    = 4
	TileTypeAVIF    = 5

	headerSize = 127
)

// ErrNotFound is returned for a tile the archive does not contain.
var ErrNotFound = errors.New("tile not found")

// Header is the fixed 127-byte archive header.
type Header struct {
	RootOffset, RootLength, MetaOffset, MetaLength, LeafOffset, LeafLength, TileOffset, TileLength uint64
	AddressedTiles, TileEntries, TileContents                                                      uint64
	Clustered                                                                                      bool
	InternalCompression, TileCompression, TileType, MinZoom, MaxZoom, CenterZoom                   uint8
	MinLon, MinLat, MaxLon, MaxLat, CenterLon, CenterLat                                           float64
}

// Entry is one directory entry: a run of tiles (RunLength ≥ 1) or a pointer to a leaf directory (RunLength 0).
type Entry struct {
	TileID    uint64
	Offset    uint64
	Length    uint32
	RunLength uint32
}

// Archive is an open PMTiles file.
type Archive struct {
	f      *os.File
	h      Header
	root   []Entry
	mu     sync.Mutex
	leaves map[uint64][]Entry // leaf directory offset → entries (bounded cache)
}

// Open reads the header and the root directory.
func Open(path string) (*Archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	hb := make([]byte, headerSize)
	if _, err := io.ReadFull(f, hb); err != nil {
		f.Close()
		return nil, fmt.Errorf("pmtiles: header: %w", err)
	}
	h, err := DeserializeHeader(hb)
	if err != nil {
		f.Close()
		return nil, err
	}
	a := &Archive{f: f, h: h, leaves: map[uint64][]Entry{}}
	root, err := a.readDir(h.RootOffset, h.RootLength)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("pmtiles: root directory: %w", err)
	}
	a.root = root
	return a, nil
}

// Close closes the file.
func (a *Archive) Close() error { return a.f.Close() }

// Header returns the archive header.
func (a *Archive) Header() Header { return a.h }

// ContentType and Ext describe the tile format.
func (a *Archive) ContentType() string {
	switch a.h.TileType {
	case TileTypeMVT:
		return "application/x-protobuf"
	case TileTypePNG:
		return "image/png"
	case TileTypeJPEG:
		return "image/jpeg"
	case TileTypeWebP:
		return "image/webp"
	case TileTypeAVIF:
		return "image/avif"
	}
	return "application/octet-stream"
}

func (a *Archive) Ext() string {
	switch a.h.TileType {
	case TileTypeMVT:
		return "mvt"
	case TileTypePNG:
		return "png"
	case TileTypeJPEG:
		return "jpg"
	case TileTypeWebP:
		return "webp"
	case TileTypeAVIF:
		return "avif"
	}
	return "bin"
}

// Metadata returns the archive's JSON metadata (name, attribution, …).
func (a *Archive) Metadata() (map[string]any, error) {
	if a.h.MetaLength == 0 {
		return map[string]any{}, nil
	}
	if a.h.MetaLength > 16<<20 {
		return nil, errors.New("pmtiles: metadata too large")
	}
	buf := make([]byte, a.h.MetaLength)
	if _, err := a.f.ReadAt(buf, int64(a.h.MetaOffset)); err != nil {
		return nil, err
	}
	raw, err := decompress(buf, a.h.InternalCompression)
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("pmtiles: metadata: %w", err)
	}
	return m, nil
}

// Tile returns the decompressed data of tile z/x/y or ErrNotFound.
func (a *Archive) Tile(z uint8, x, y uint32) ([]byte, error) {
	if z < a.h.MinZoom || z > a.h.MaxZoom || uint64(x) >= 1<<z || uint64(y) >= 1<<z {
		return nil, ErrNotFound
	}
	id := ZxyToID(z, x, y)
	e, ok := findEntry(a.root, id)
	if !ok {
		return nil, ErrNotFound
	}
	if e.RunLength == 0 {
		leaf, err := a.leaf(e.Offset, uint64(e.Length))
		if err != nil {
			return nil, err
		}
		if e, ok = findEntry(leaf, id); !ok || e.RunLength == 0 {
			return nil, ErrNotFound
		}
	}
	buf := make([]byte, e.Length)
	if _, err := a.f.ReadAt(buf, int64(a.h.TileOffset+e.Offset)); err != nil {
		return nil, fmt.Errorf("pmtiles: tile data: %w", err)
	}
	return decompress(buf, a.h.TileCompression)
}

func (a *Archive) leaf(offset, length uint64) ([]Entry, error) {
	a.mu.Lock()
	if l, ok := a.leaves[offset]; ok {
		a.mu.Unlock()
		return l, nil
	}
	a.mu.Unlock()
	l, err := a.readDir(a.h.LeafOffset+offset, length)
	if err != nil {
		return nil, fmt.Errorf("pmtiles: leaf directory: %w", err)
	}
	a.mu.Lock()
	if len(a.leaves) > 256 {
		a.leaves = map[uint64][]Entry{}
	}
	a.leaves[offset] = l
	a.mu.Unlock()
	return l, nil
}

func (a *Archive) readDir(offset, length uint64) ([]Entry, error) {
	if length > 64<<20 {
		return nil, errors.New("directory too large")
	}
	buf := make([]byte, length)
	if _, err := a.f.ReadAt(buf, int64(offset)); err != nil {
		return nil, err
	}
	raw, err := decompress(buf, a.h.InternalCompression)
	if err != nil {
		return nil, err
	}
	return DeserializeEntries(raw)
}

// findEntry returns the entry covering id: the last entry with TileID ≤ id, when id falls into its run or it is
// a leaf pointer.
func findEntry(entries []Entry, id uint64) (Entry, bool) {
	i := sort.Search(len(entries), func(i int) bool { return entries[i].TileID > id }) - 1
	if i < 0 {
		return Entry{}, false
	}
	e := entries[i]
	if e.RunLength == 0 || id < e.TileID+uint64(e.RunLength) {
		return e, true
	}
	return Entry{}, false
}

func decompress(b []byte, compression uint8) ([]byte, error) {
	switch compression {
	case CompressionNone, CompressionUnknown:
		return b, nil
	case CompressionGzip:
		r, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(io.LimitReader(r, 256<<20))
	case CompressionBrotli:
		return nil, errors.New("brotli-compressed archives are not supported (re-create with gzip or no compression)")
	case CompressionZstd:
		return nil, errors.New("zstd-compressed archives are not supported (re-create with gzip or no compression)")
	}
	return nil, fmt.Errorf("unknown compression %d", compression)
}

// DeserializeHeader parses the 127-byte header.
func DeserializeHeader(b []byte) (Header, error) {
	var h Header
	if len(b) < headerSize || string(b[:7]) != "PMTiles" {
		return h, errors.New("pmtiles: not a PMTiles file")
	}
	if b[7] != 3 {
		return h, fmt.Errorf("pmtiles: unsupported version %d (only v3)", b[7])
	}
	u := func(off int) uint64 { return binary.LittleEndian.Uint64(b[off : off+8]) }
	i32 := func(off int) float64 { return float64(int32(binary.LittleEndian.Uint32(b[off:off+4]))) / 1e7 }
	h.RootOffset, h.RootLength = u(8), u(16)
	h.MetaOffset, h.MetaLength = u(24), u(32)
	h.LeafOffset, h.LeafLength = u(40), u(48)
	h.TileOffset, h.TileLength = u(56), u(64)
	h.AddressedTiles, h.TileEntries, h.TileContents = u(72), u(80), u(88)
	h.Clustered = b[96] == 1
	h.InternalCompression, h.TileCompression, h.TileType = b[97], b[98], b[99]
	h.MinZoom, h.MaxZoom = b[100], b[101]
	h.MinLon, h.MinLat, h.MaxLon, h.MaxLat = i32(102), i32(106), i32(110), i32(114)
	h.CenterZoom = b[118]
	h.CenterLon, h.CenterLat = i32(119), i32(123)
	return h, nil
}

// SerializeHeader writes the 127-byte header.
func SerializeHeader(h Header) []byte {
	b := make([]byte, headerSize)
	copy(b, "PMTiles")
	b[7] = 3
	p := func(off int, v uint64) { binary.LittleEndian.PutUint64(b[off:off+8], v) }
	e7 := func(off int, v float64) {
		binary.LittleEndian.PutUint32(b[off:off+4], uint32(int32(math.Round(v*1e7))))
	}
	p(8, h.RootOffset)
	p(16, h.RootLength)
	p(24, h.MetaOffset)
	p(32, h.MetaLength)
	p(40, h.LeafOffset)
	p(48, h.LeafLength)
	p(56, h.TileOffset)
	p(64, h.TileLength)
	p(72, h.AddressedTiles)
	p(80, h.TileEntries)
	p(88, h.TileContents)
	if h.Clustered {
		b[96] = 1
	}
	b[97], b[98], b[99] = h.InternalCompression, h.TileCompression, h.TileType
	b[100], b[101] = h.MinZoom, h.MaxZoom
	e7(102, h.MinLon)
	e7(106, h.MinLat)
	e7(110, h.MaxLon)
	e7(114, h.MaxLat)
	b[118] = h.CenterZoom
	e7(119, h.CenterLon)
	e7(123, h.CenterLat)
	return b
}

// DeserializeEntries decodes a directory (delta-encoded tile ids, run lengths, lengths, offsets).
func DeserializeEntries(b []byte) ([]Entry, error) {
	r := bytes.NewReader(b)
	n, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, err
	}
	if n > 1<<24 {
		return nil, errors.New("directory has too many entries")
	}
	entries := make([]Entry, n)
	var last uint64
	for i := range entries {
		d, err := binary.ReadUvarint(r)
		if err != nil {
			return nil, err
		}
		last += d
		entries[i].TileID = last
	}
	for i := range entries {
		v, err := binary.ReadUvarint(r)
		if err != nil {
			return nil, err
		}
		entries[i].RunLength = uint32(v)
	}
	for i := range entries {
		v, err := binary.ReadUvarint(r)
		if err != nil {
			return nil, err
		}
		entries[i].Length = uint32(v)
	}
	for i := range entries {
		v, err := binary.ReadUvarint(r)
		if err != nil {
			return nil, err
		}
		if v == 0 && i > 0 {
			entries[i].Offset = entries[i-1].Offset + uint64(entries[i-1].Length)
		} else {
			entries[i].Offset = v - 1
		}
	}
	return entries, nil
}

// SerializeEntries encodes a directory (entries must be sorted by TileID).
func SerializeEntries(entries []Entry) []byte {
	var out bytes.Buffer
	tmp := make([]byte, binary.MaxVarintLen64)
	put := func(v uint64) { out.Write(tmp[:binary.PutUvarint(tmp, v)]) }
	put(uint64(len(entries)))
	var last uint64
	for _, e := range entries {
		put(e.TileID - last)
		last = e.TileID
	}
	for _, e := range entries {
		put(uint64(e.RunLength))
	}
	for _, e := range entries {
		put(uint64(e.Length))
	}
	for i, e := range entries {
		if i > 0 && e.Offset == entries[i-1].Offset+uint64(entries[i-1].Length) {
			put(0)
		} else {
			put(e.Offset + 1)
		}
	}
	return out.Bytes()
}

// ZxyToID maps a tile to its Hilbert-curve id (all tiles of zoom levels below z come first).
func ZxyToID(z uint8, x, y uint32) uint64 {
	var acc uint64
	for tz := uint8(0); tz < z; tz++ {
		acc += (1 << tz) * (1 << tz)
	}
	n := uint64(1) << z
	tx, ty := uint64(x), uint64(y)
	var d uint64
	for s := n / 2; s > 0; s /= 2 {
		var rx, ry uint64
		if tx&s > 0 {
			rx = 1
		}
		if ty&s > 0 {
			ry = 1
		}
		d += s * s * ((3 * rx) ^ ry)
		rotate(s, &tx, &ty, rx, ry)
	}
	return acc + d
}

func rotate(n uint64, x, y *uint64, rx, ry uint64) {
	if ry == 0 {
		if rx == 1 {
			*x = n - 1 - *x
			*y = n - 1 - *y
		}
		*x, *y = *y, *x
	}
}

// IDToZxy is the inverse of ZxyToID.
func IDToZxy(id uint64) (uint8, uint32, uint32) {
	var acc uint64
	for z := uint8(0); z < 32; z++ {
		numTiles := uint64(1) << (2 * z)
		if acc+numTiles > id {
			x, y := tOnLevel(z, id-acc)
			return z, x, y
		}
		acc += numTiles
	}
	return 0, 0, 0
}

func tOnLevel(z uint8, pos uint64) (uint32, uint32) {
	n := uint64(1) << z
	t := pos
	var x, y uint64
	for s := uint64(1); s < n; s *= 2 {
		rx := 1 & (t / 2)
		ry := 1 & (t ^ rx)
		rotate(s, &x, &y, rx, ry)
		x += s * rx
		y += s * ry
		t /= 4
	}
	return uint32(x), uint32(y)
}

package pmtiles

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"sort"
)

// TileKey addresses one tile.
type TileKey struct {
	Z    uint8
	X, Y uint32
}

// WriteOptions controls the archive layout.
type WriteOptions struct {
	TileType                       uint8          // TileTypePNG …
	LeafSize                       int            // entries per leaf directory; 0 = everything in the root directory
	Metadata                       map[string]any // JSON metadata (name, attribution, …)
	MinLon, MinLat, MaxLon, MaxLat float64
	CenterZoom                     uint8
	CenterLon, CenterLat           float64
}

// Write creates a PMTiles v3 archive (gzip directories, uncompressed tiles, identical tiles stored once).
func Write(path string, tiles map[TileKey][]byte, o WriteOptions) error {
	type item struct {
		id   uint64
		data []byte
	}
	items := make([]item, 0, len(tiles))
	minZ, maxZ := uint8(255), uint8(0)
	for k, d := range tiles {
		items = append(items, item{ZxyToID(k.Z, k.X, k.Y), d})
		if k.Z < minZ {
			minZ = k.Z
		}
		if k.Z > maxZ {
			maxZ = k.Z
		}
	}
	if len(items) == 0 {
		minZ, maxZ = 0, 0
	}
	sort.Slice(items, func(i, j int) bool { return items[i].id < items[j].id })
	var data bytes.Buffer
	offsets := map[string]uint64{}
	var entries []Entry
	contents := 0
	for _, it := range items {
		key := string(it.data)
		off, seen := offsets[key]
		if !seen {
			off = uint64(data.Len())
			data.Write(it.data)
			offsets[key] = off
			contents++
		}
		// merge consecutive ids pointing at the same data into a run
		if n := len(entries); n > 0 && entries[n-1].Offset == off && entries[n-1].TileID+uint64(entries[n-1].RunLength) == it.id {
			entries[n-1].RunLength++
			continue
		}
		entries = append(entries, Entry{TileID: it.id, Offset: off, Length: uint32(len(it.data)), RunLength: 1})
	}
	gz := func(b []byte) []byte {
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		w.Write(b)
		w.Close()
		return buf.Bytes()
	}
	var root, leaves []byte
	if o.LeafSize > 0 && len(entries) > o.LeafSize {
		var rootEntries []Entry
		var leafBuf bytes.Buffer
		for i := 0; i < len(entries); i += o.LeafSize {
			end := i + o.LeafSize
			if end > len(entries) {
				end = len(entries)
			}
			chunk := gz(SerializeEntries(entries[i:end]))
			rootEntries = append(rootEntries, Entry{TileID: entries[i].TileID, Offset: uint64(leafBuf.Len()), Length: uint32(len(chunk)), RunLength: 0})
			leafBuf.Write(chunk)
		}
		root = gz(SerializeEntries(rootEntries))
		leaves = leafBuf.Bytes()
	} else {
		root = gz(SerializeEntries(entries))
	}
	meta, _ := json.Marshal(o.Metadata)
	if o.Metadata == nil {
		meta = []byte("{}")
	}
	meta = gz(meta)
	h := Header{
		RootOffset: headerSize, RootLength: uint64(len(root)),
		MetaOffset: headerSize + uint64(len(root)), MetaLength: uint64(len(meta)),
		LeafOffset: headerSize + uint64(len(root)) + uint64(len(meta)), LeafLength: uint64(len(leaves)),
		TileOffset: headerSize + uint64(len(root)) + uint64(len(meta)) + uint64(len(leaves)), TileLength: uint64(data.Len()),
		AddressedTiles: uint64(len(items)), TileEntries: uint64(len(entries)), TileContents: uint64(contents),
		Clustered: true, InternalCompression: CompressionGzip, TileCompression: CompressionNone, TileType: o.TileType,
		MinZoom: minZ, MaxZoom: maxZ, MinLon: o.MinLon, MinLat: o.MinLat, MaxLon: o.MaxLon, MaxLat: o.MaxLat,
		CenterZoom: o.CenterZoom, CenterLon: o.CenterLon, CenterLat: o.CenterLat,
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, part := range [][]byte{SerializeHeader(h), root, meta, leaves, data.Bytes()} {
		if _, err := f.Write(part); err != nil {
			return err
		}
	}
	return nil
}

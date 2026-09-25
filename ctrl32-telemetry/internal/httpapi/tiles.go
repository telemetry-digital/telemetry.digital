package httpapi

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/pmtiles"
)

// tileSource is the server's optional local map: a raster PMTiles archive named by `[map] pmtiles` in config.toml.
// Map widgets then load their tiles from this server (`/tiles/{z}/{x}/{y}.png`), so an intranet installation needs no
// tile provider on the Internet. Tiles are public like the logo — they are map imagery, not telemetry — and cacheable.
type tileSource struct {
	a           *pmtiles.Archive
	err         string
	attribution string
	name        string
	size        int64
}

func openTiles(cfg config.MapConfig, log *slog.Logger) *tileSource {
	if cfg.PMTiles == "" {
		return nil
	}
	t := &tileSource{attribution: cfg.Attribution}
	a, err := pmtiles.Open(cfg.PMTiles)
	if err != nil {
		t.err = err.Error()
		log.Error("map tiles: archive not usable", "path", cfg.PMTiles, "err", err)
		return t
	}
	h := a.Header()
	if h.TileType == pmtiles.TileTypeMVT || h.TileType == pmtiles.TileTypeUnknown {
		a.Close()
		t.err = "the archive holds vector tiles; the map widgets need a raster archive (png, jpg or webp)"
		log.Error("map tiles: archive not usable", "path", cfg.PMTiles, "err", t.err)
		return t
	}
	t.a = a
	if meta, err := a.Metadata(); err == nil {
		t.name, _ = meta["name"].(string)
		if t.attribution == "" {
			t.attribution, _ = meta["attribution"].(string)
		}
	}
	if st, err := os.Stat(cfg.PMTiles); err == nil {
		t.size = st.Size()
	}
	log.Info("map tiles: serving the local archive", "path", cfg.PMTiles, "type", a.Ext(), "zoom", fmt.Sprintf("%d-%d", h.MinZoom, h.MaxZoom), "tiles", h.AddressedTiles)
	return t
}

func (t *tileSource) ok() bool { return t != nil && t.a != nil }

func (t *tileSource) tileURL() string { return "/tiles/{z}/{x}/{y}." + t.a.Ext() }

func (t *tileSource) info() map[string]any {
	if t == nil {
		return map[string]any{"configured": false, "ok": false}
	}
	m := map[string]any{"configured": true, "ok": t.a != nil, "name": t.name, "attribution": t.attribution}
	if t.a == nil {
		m["error"] = t.err
		return m
	}
	h := t.a.Header()
	m["tile_url"] = t.tileURL()
	m["tile_type"] = t.a.Ext()
	m["min_zoom"] = h.MinZoom
	m["max_zoom"] = h.MaxZoom
	m["bounds"] = []float64{h.MinLon, h.MinLat, h.MaxLon, h.MaxLat}
	m["center"] = []float64{h.CenterLon, h.CenterLat, float64(h.CenterZoom)}
	m["tiles"] = h.AddressedTiles
	m["size_bytes"] = t.size
	return m
}

func (s *Server) tilesInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.tiles.info())
}

// tile serves one XYZ tile; the extension in {y} (".png") is accepted and ignored — the archive decides the format.
func (s *Server) tile(w http.ResponseWriter, r *http.Request) {
	if !s.tiles.ok() {
		http.Error(w, "no local map tiles configured", http.StatusNotFound)
		return
	}
	z, err1 := strconv.Atoi(r.PathValue("z"))
	x, err2 := strconv.Atoi(r.PathValue("x"))
	yy, _, _ := strings.Cut(r.PathValue("y"), ".")
	y, err3 := strconv.Atoi(yy)
	if err1 != nil || err2 != nil || err3 != nil || z < 0 || z > 30 || x < 0 || y < 0 {
		http.Error(w, "bad tile address", http.StatusBadRequest)
		return
	}
	data, err := s.tiles.a.Tile(uint8(z), uint32(x), uint32(y))
	if errors.Is(err, pmtiles.ErrNotFound) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		http.Error(w, "tile not found", http.StatusNotFound)
		return
	}
	if err != nil {
		s.log.Error("map tiles", "z", z, "x", x, "y", y, "err", err)
		http.Error(w, "tile read failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", s.tiles.a.ContentType())
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

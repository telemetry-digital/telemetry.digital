package httpapi

// Display health (0.86.0, spec 034): with its poll the display page reports what it shows — every camera tile with
// its state (playing, stalled, connecting, offline, unsupported), the screen size and the version of the page — so the
// Displays screen tells "39/39 playing" and names the cameras that are not, without anybody standing at the wall.
//
//	POST /api/v1/display/health   display cookie   {page_version, screen, item, tiles: [{camera, state}], kiosk, monitor}

import (
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

type displayTile struct {
	Camera string `json:"camera"`
	State  string `json:"state"` // playing, stalled, connecting, offline, unsupported
	Since  int    `json:"since_s,omitempty"`
}

type displayHealth struct {
	PageVersion string         `json:"page_version"`
	Screen      map[string]any `json:"screen,omitempty"` // w, h, vw, vh, dpr
	Item        string         `json:"item,omitempty"`   // title of what it shows now
	Tiles       []displayTile  `json:"tiles"`
	Kiosk       string         `json:"kiosk,omitempty"`   // the kiosk PC the launcher named in the address (a hint for linking)
	Monitor     *int           `json:"monitor,omitempty"` // its monitor
	Agent       string         `json:"agent,omitempty"`
}

var tileStates = []string{"playing", "stalled", "connecting", "offline", "unsupported"}

// summary is what the Displays screen shows: counts and the cameras that are not playing.
func (h *displayHealth) summary() map[string]any {
	playing := 0
	bad := []map[string]any{}
	for _, t := range h.Tiles {
		if t.State == "playing" {
			playing++
		} else {
			bad = append(bad, map[string]any{"camera": t.Camera, "state": t.State, "since_s": t.Since})
		}
	}
	return map[string]any{"page_version": h.PageVersion, "screen": h.Screen, "item": h.Item, "tiles": len(h.Tiles), "playing": playing, "problems": bad,
		"kiosk": h.Kiosk, "monitor": h.Monitor}
}

var healthLimit = struct {
	sync.Mutex
	at map[string]time.Time
}{at: map[string]time.Time{}}

var versionLikeRe = regexp.MustCompile(`^[0-9A-Za-z.+-]{1,40}$`)

func (s *Server) displayHealthReport(w http.ResponseWriter, r *http.Request) {
	d, ok := s.displayFromCookie(r)
	if !ok {
		writeErr(w, 401, "this display is not paired")
		return
	}
	var in displayHealth
	if err := decodeJSON(r, &in, 32<<10); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	// at most every 10 s per display (a page reports every 30 s)
	healthLimit.Lock()
	last := healthLimit.at[d.ID]
	if time.Since(last) < 10*time.Second {
		healthLimit.Unlock()
		writeJSON(w, 429, map[string]string{"error": "too often"})
		return
	}
	healthLimit.at[d.ID] = time.Now()
	for id, t := range healthLimit.at {
		if time.Since(t) > 10*time.Minute {
			delete(healthLimit.at, id)
		}
	}
	healthLimit.Unlock()
	if !versionLikeRe.MatchString(in.PageVersion) {
		in.PageVersion = ""
	}
	in.Item = truncateStr(strings.TrimSpace(in.Item), 120)
	if len(in.Tiles) > 128 {
		in.Tiles = in.Tiles[:128]
	}
	// only cameras the display may show, and only the known states
	allowed := s.displayCameras(r.Context(), d)
	tiles := make([]displayTile, 0, len(in.Tiles))
	for _, t := range in.Tiles {
		if !allowed[t.Camera] || !slices.Contains(tileStates, t.State) {
			continue
		}
		t.Since = min(max(t.Since, 0), 86400)
		tiles = append(tiles, t)
	}
	in.Tiles = tiles
	if !uuidRe.MatchString(strings.ToLower(in.Kiosk)) {
		in.Kiosk = ""
	}
	if in.Monitor != nil && (*in.Monitor < 0 || *in.Monitor > 7) {
		in.Monitor = nil
	}
	scr := map[string]any{}
	for _, k := range []string{"w", "h", "vw", "vh", "dpr"} {
		if v, ok := in.Screen[k].(float64); ok && v >= 0 && v < 100000 {
			scr[k] = v
		}
	}
	in.Screen = scr
	in.Agent = truncateStr(r.UserAgent(), 300)
	raw, _ := json.Marshal(in)
	if _, err := s.pool.Exec(r.Context(), `UPDATE core.display SET health = $2::jsonb, health_at = now() WHERE id = $1::uuid`, d.ID, string(raw)); err != nil {
		writeErr(w, 500, "not stored")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

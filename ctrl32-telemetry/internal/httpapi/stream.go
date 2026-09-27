package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/live"
)

// Live data over Server-Sent Events (URS G-07): GET /api/v1/stream?datastreams=a,b&devices=all|x,y&alarms=1 with
// the session or API token and the same data.read permission as the REST endpoints; the public variant
// /api/v1/public/{token}/stream is limited to what the dashboard binds. Events: observation, device, alarm, plus a
// ping every 25 s carrying the number of dropped events (0 unless the client was too slow). SSE was chosen over a
// WebSocket because the flow is one-directional, it needs no library, browsers reconnect on their own and it passes
// through the reverse proxy unchanged; MQTT-over-WebSocket stays reserved for the HMI viewer (ADR-0008 S2).

const maxStreamIDs = 200

func idSet(raw string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range strings.Split(raw, ",") {
		if id = strings.TrimSpace(id); id == "" {
			continue
		}
		if !uuidRe.MatchString(id) {
			return nil, fmt.Errorf("invalid id %q", id)
		}
		out[id] = true
		if len(out) > maxStreamIDs {
			return nil, fmt.Errorf("at most %d ids", maxStreamIDs)
		}
	}
	return out, nil
}

// GET /api/v1/stream
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	ds, err := idSet(q.Get("datastreams"))
	if err != nil {
		writeErr(w, 400, "datastreams: "+err.Error())
		return
	}
	f := live.Filter{Org: org, Datastreams: ds, Alarms: q.Get("alarms") == "1", Commands: true, Entities: q.Get("entities") == "1"}
	if q.Get("devices") == "all" {
		f.AllDevices = true
	} else if f.Devices, err = idSet(q.Get("devices")); err != nil {
		writeErr(w, 400, "devices: "+err.Error())
		return
	}
	s.serveStream(w, r, f)
}

// GET /api/v1/public/{token}/stream
func (s *Server) publicStream(w http.ResponseWriter, r *http.Request) {
	si, ok := s.publicShare(w, r)
	if !ok {
		return
	}
	s.serveStream(w, r, live.Filter{Org: si.Org, Datastreams: si.DS, Devices: si.Devs, AllDevices: si.AllDevs, Alarms: si.HasAlarms, AlarmDatastreams: si.DS})
}

func (s *Server) serveStream(w http.ResponseWriter, r *http.Request, f live.Filter) {
	if s.live == nil {
		writeErr(w, 503, "live data not available")
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "streaming unsupported")
		return
	}
	// The server's read/write timeouts are for ordinary requests; a stream lives as long as the client listens.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
	_ = rc.SetReadDeadline(time.Time{})
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	sub := s.live.Subscribe(f)
	defer s.live.Unsubscribe(sub)
	hello, _ := json.Marshal(map[string]any{"datastreams": len(f.Datastreams), "devices": len(f.Devices), "all_devices": f.AllDevices, "alarms": f.Alarms})
	fmt.Fprintf(w, "retry: 3000\nevent: hello\ndata: %s\n\n", hello)
	fl.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-sub.C:
			b, err := json.Marshal(e)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, b); err != nil {
				return
			}
			fl.Flush()
		case <-ping.C:
			if _, err := fmt.Fprintf(w, "event: ping\ndata: {\"dropped\":%d}\n\n", sub.Dropped()); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

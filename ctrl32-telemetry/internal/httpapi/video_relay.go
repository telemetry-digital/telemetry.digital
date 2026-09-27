package httpapi

// Camera control through the relay (0.58.0): the relay of a push camera long-polls for the camera's ONVIF requests and
// returns the camera's answers (internal/video/tunnel.go). Authenticated with the camera's push secret in the
// X-Camera-Secret header — no session, no CSRF token; wrong secrets count towards blocking the address like wrong
// secrets on the RTSP push port.
//
//	GET  /api/v1/video/relay/{id}/onvif/next?wait=25    204 without a request; wait=0 only tests the secret
//	POST /api/v1/video/relay/{id}/onvif/{rid}           {status, headers, body (base64)} or {error}

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/video"
)

func (s *Server) relayErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, video.ErrRelayBlocked):
		writeErr(w, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, video.ErrRelayAuth):
		writeErr(w, http.StatusUnauthorized, err.Error())
	default:
		writeErr(w, http.StatusConflict, err.Error())
	}
}

func (s *Server) relayONVIFNext(w http.ResponseWriter, r *http.Request) {
	if s.video == nil {
		writeErr(w, 404, "video is not available")
		return
	}
	wait := 25
	if v := r.URL.Query().Get("wait"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 25 {
			writeErr(w, 400, "wait: 0 to 25 seconds")
			return
		}
		wait = n
	}
	rq, err := s.video.RelayNext(r.Context(), r.PathValue("id"), r.Header.Get("X-Camera-Secret"), clientIP(r), time.Duration(wait)*time.Second)
	if err != nil {
		s.relayErr(w, err)
		return
	}
	if rq == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, 200, rq)
}

func (s *Server) relayONVIFAnswer(w http.ResponseWriter, r *http.Request) {
	if s.video == nil {
		writeErr(w, 404, "video is not available")
		return
	}
	id := r.PathValue("id")
	if err := s.video.RelayAuth(id, r.Header.Get("X-Camera-Secret"), clientIP(r)); err != nil {
		s.relayErr(w, err)
		return
	}
	var in video.TunnelResponse
	if err := decodeJSON(r, &in, 3<<20/2); err != nil { // 1 MiB of body in base64 and the headers
		writeErr(w, 400, err.Error())
		return
	}
	if !s.video.RelayAnswer(id, r.PathValue("rid"), in) {
		writeErr(w, http.StatusGone, "the request is no longer waiting")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

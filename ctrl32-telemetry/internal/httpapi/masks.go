package httpapi

// Privacy masks (spec 016): areas of a camera's picture covered wherever the picture is shown. The server does not
// decode video, so the masks travel with every live and recorded stream (a "masks" message before the first frame)
// and every player covers them; JPEG snapshots are masked on the server. The recording stays complete: seeing it
// unmasked (?unmask=1) and exporting a masked camera need video.unmask and are audited. Masks set in the camera
// itself (its web page or ONVIF) keep the areas out of the recording altogether.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"net/http"
)

// maskPolygon is a closed polygon in picture coordinates 0..1 (x to the right, y down).
type maskPolygon [][2]float64

func parseMasks(raw []byte) []maskPolygon {
	var m []maskPolygon
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return m
}

// validateMasks checks masks from the camera form: at most 16 polygons of 3 to 16 points inside the picture.
func validateMasks(raw json.RawMessage) ([]maskPolygon, string) {
	var m []maskPolygon
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, "privacy_masks: a list of polygons [[x, y], …] with x and y from 0 to 1"
	}
	if len(m) > 16 {
		return nil, "privacy_masks: at most 16"
	}
	for i, p := range m {
		if len(p) < 3 || len(p) > 16 {
			return nil, fmt.Sprintf("privacy_masks %d: 3 to 16 points", i+1)
		}
		for _, pt := range p {
			if pt[0] < 0 || pt[0] > 1 || pt[1] < 0 || pt[1] > 1 {
				return nil, fmt.Sprintf("privacy_masks %d: coordinates from 0 to 1", i+1)
			}
		}
	}
	if m == nil {
		m = []maskPolygon{}
	}
	return m, ""
}

// unmasked reports whether this request shows the camera without masks: asked for and permitted (audited).
func (s *Server) unmasked(r *http.Request, c cameraRow) bool {
	if len(parseMasks(c.masks)) == 0 || r.URL.Query().Get("unmask") != "1" {
		return false
	}
	if !PrincipalFrom(r.Context()).Has("video.unmask") {
		return false
	}
	s.audit(r, "video.unmask", "camera", c.id, nil, map[string]any{"camera": c.name, "path": r.URL.Path}, "picture viewed without privacy masks")
	return true
}

// masksMessage is sent to a player before the first frame.
func masksMessage(c cameraRow, unmasked bool) map[string]any {
	m := parseMasks(c.masks)
	if unmasked || m == nil {
		m = []maskPolygon{}
	}
	return map[string]any{"type": "masks", "masks": m, "unmasked": unmasked, "has_masks": len(parseMasks(c.masks)) > 0}
}

// maskJPEG covers the masks of a JPEG picture in black.
func maskJPEG(img []byte, masks []maskPolygon) ([]byte, error) {
	src, err := jpeg.Decode(bytes.NewReader(img))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	w, h := float64(b.Dx()), float64(b.Dy())
	black := color.RGBA{0, 0, 0, 255}
	for _, poly := range masks {
		minY, maxY := 1.0, 0.0
		for _, pt := range poly {
			minY, maxY = min(minY, pt[1]), max(maxY, pt[1])
		}
		for y := int(minY * h); y <= int(maxY*h) && y < b.Dy(); y++ {
			py := (float64(y) + 0.5) / h
			for x := 0; x < b.Dx(); x++ {
				if inPolygon((float64(x)+0.5)/w, py, poly) {
					dst.SetRGBA(b.Min.X+x, b.Min.Y+y, black)
				}
			}
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// inPolygon is the even-odd rule.
func inPolygon(x, y float64, poly maskPolygon) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		xi, yi, xj, yj := poly[i][0], poly[i][1], poly[j][0], poly[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}

// maskedExportAllowed: a masked camera's full picture leaves the server only with video.unmask.
func (s *Server) maskedExportAllowed(w http.ResponseWriter, r *http.Request, c cameraRow) bool {
	if len(parseMasks(c.masks)) > 0 && !PrincipalFrom(r.Context()).Has("video.unmask") {
		writeErr(w, 403, "this camera has privacy masks: exporting its recordings needs the permission video.unmask")
		return false
	}
	return true
}

// pictureSaved records a picture saved from the player in the browser. The player burns the privacy masks into the
// picture; a picture without them (the stream was opened with ?unmask=1) may be saved only with video.unmask and is
// audited, like every other way the full picture of a masked camera leaves the server.
func (s *Server) pictureSaved(w http.ResponseWriter, r *http.Request) {
	org, ok := s.mustOrg(w, r)
	if !ok {
		return
	}
	c, ok := s.cameraRow(r.Context(), org, r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "camera not found")
		return
	}
	var in struct {
		Unmasked bool   `json:"unmasked"`
		At       string `json:"at"`
	}
	if err := decodeJSON(r, &in, 1024); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	masked := len(parseMasks(c.masks)) > 0
	if !in.Unmasked || !masked {
		writeJSON(w, 200, map[string]any{"masked": masked, "audited": false})
		return
	}
	if !PrincipalFrom(r.Context()).Has("video.unmask") {
		writeErr(w, 403, "this camera has privacy masks: saving its picture without them needs the permission video.unmask")
		return
	}
	if len(in.At) > 40 {
		in.At = in.At[:40]
	}
	s.audit(r, "video.unmask", "camera", c.id, nil, map[string]any{"camera": c.name, "path": r.URL.Path, "at": in.At, "saved": true}, "picture saved without privacy masks")
	writeJSON(w, 200, map[string]any{"masked": false, "audited": true})
}

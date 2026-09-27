package video

// Motion detection on the server (spec 019) for cameras without ONVIF events. The server never decodes H.264 or
// H.265, so it looks at JPEG pictures only: the frames of an MJPEG stream (the sub-stream when it is MJPEG) or the
// snapshot address of the camera polled once a second. Each picture is reduced to a grid of brightness cells and
// compared with a slowly adapting background; motion is a share of changed cells inside the zones above a threshold
// set by the sensitivity. A sudden change of almost the whole picture (lights, IR switch, auto exposure) resets the
// background instead of raising an event.

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	gridW, gridH   = 64, 36
	motionInterval = 400 * time.Millisecond // at most 2.5 pictures a second are examined
	motionHold     = 4 * time.Second        // motion ends after this long without changed pictures
	snapshotEvery  = time.Second
)

// MotionZone is a polygon in picture coordinates 0..1 where motion counts; no zones = the whole picture.
type MotionZone [][2]float64

// motionDetector keeps the background of one camera.
type motionDetector struct {
	sens    int
	active  []bool // cells inside the zones
	nActive int
	bg      []float64
	prev    []float64
	streak  int
	last    time.Time // last picture with motion
	on      bool
}

func newMotionDetector(sens int, zones []MotionZone) *motionDetector {
	d := &motionDetector{sens: max(1, min(100, sens)), active: make([]bool, gridW*gridH)}
	for y := 0; y < gridH; y++ {
		for x := 0; x < gridW; x++ {
			px, py := (float64(x)+0.5)/gridW, (float64(y)+0.5)/gridH
			in := len(zones) == 0
			for _, z := range zones {
				if len(z) >= 3 && pointIn(px, py, z) {
					in = true
					break
				}
			}
			if in {
				d.active[y*gridW+x] = true
				d.nActive++
			}
		}
	}
	return d
}

// thresholds: the brightness difference of a changed cell and the share of changed cells that is motion.
func (d *motionDetector) thresholds() (float64, float64) {
	k := float64(100-d.sens) / 100 // 0 = most sensitive
	return 10 + 30*k, 0.0005 + 0.02*k
}

// feed examines one picture; it returns the new motion state and whether it changed.
func (d *motionDetector) feed(cells []float64, now time.Time) (bool, bool) {
	if d.bg == nil {
		d.bg = append([]float64(nil), cells...)
		d.prev = append([]float64(nil), cells...)
		return d.on, false
	}
	defer copy(d.prev, cells)
	diffT, shareT := d.thresholds()
	changed, all := 0, 0
	abs := func(v float64) float64 {
		if v < 0 {
			return -v
		}
		return v
	}
	for i, v := range cells {
		fromBg := abs(v - d.bg[i])
		if fromBg > diffT {
			all++
		}
		// a cell moves when it changed since the previous picture and differs from the background now or did
		// before (the object arrives or leaves); an object that stopped (a parked car) is not motion and becomes
		// background
		if d.active[i] && abs(v-d.prev[i]) > diffT/2 && (fromBg > diffT || abs(d.prev[i]-d.bg[i]) > diffT) {
			changed++
		}
	}
	if float64(all) > 0.7*float64(len(cells)) {
		// lighting changed: take the new picture as the background
		copy(d.bg, cells)
		d.streak = 0
		return d.on, false
	}
	moving := d.nActive > 0 && float64(changed) > shareT*float64(d.nActive)
	rate := 0.08
	if moving {
		rate = 0.02 // the background does not learn the moving object
	}
	for i, v := range cells {
		d.bg[i] += (v - d.bg[i]) * rate
	}
	if moving {
		d.streak++
		if d.streak >= 3 { // a single odd picture (and its return) is not motion
			d.last = now
		}
	} else {
		d.streak = 0
	}
	was := d.on
	d.on = !d.last.IsZero() && now.Sub(d.last) < motionHold
	return d.on, d.on != was
}

// cellsOf reduces a JPEG picture to the brightness grid (the Y plane of the decoded picture, every other pixel).
func cellsOf(pic []byte) ([]float64, error) {
	img, err := jpeg.Decode(bytes.NewReader(pic))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < gridW || h < gridH {
		return nil, errors.New("picture too small")
	}
	sum := make([]float64, gridW*gridH)
	n := make([]float64, gridW*gridH)
	lum := func(x, y int) float64 {
		r, g, bl, _ := img.At(x, y).RGBA()
		return (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(bl)) / 257
	}
	if yc, ok := img.(*image.YCbCr); ok {
		lum = func(x, y int) float64 { return float64(yc.Y[yc.YOffset(x, y)]) }
	} else if gr, ok := img.(*image.Gray); ok {
		lum = func(x, y int) float64 { return float64(gr.Pix[gr.PixOffset(x, y)]) }
	}
	for y := 0; y < h; y += 2 {
		cy := y * gridH / h
		for x := 0; x < w; x += 2 {
			i := cy*gridW + x*gridW/w
			sum[i] += lum(b.Min.X+x, b.Min.Y+y)
			n[i]++
		}
	}
	for i := range sum {
		if n[i] > 0 {
			sum[i] /= n[i]
		}
	}
	return sum, nil
}

func pointIn(x, y float64, poly MotionZone) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		xi, yi, xj, yj := poly[i][0], poly[i][1], poly[j][0], poly[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}

// motionState is what the camera status reports about detection.
type motionState struct {
	mu     sync.Mutex
	status string
}

func (s *motionState) set(v string) {
	s.mu.Lock()
	s.status = v
	s.mu.Unlock()
}

func (s *motionState) get() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// startMotion runs detection for a camera that asks for it, until the camera is reloaded.
func (m *Manager) startMotion(c *cam) {
	if !c.cfg.MotionDetect {
		return
	}
	c.motion = &motionState{status: "starting"}
	ctx, cancel := context.WithCancel(context.Background())
	c.stopMotion = cancel
	go m.runMotion(ctx, c)
}

func (m *Manager) runMotion(ctx context.Context, c *cam) {
	det := newMotionDetector(c.cfg.MotionSensitivity, c.cfg.MotionZones)
	lastOn := false
	examine := func(pic []byte, now time.Time) error {
		cells, err := cellsOf(pic)
		if err != nil {
			return err
		}
		on, changed := det.feed(cells, now)
		if changed {
			m.event(c, "motion", "server", "", on, false, now, "", map[string]string{"Rule": "server"})
			lastOn = on
		}
		return nil
	}
	defer func() {
		if lastOn {
			m.event(c, "motion", "server", "", false, false, time.Now(), "", map[string]string{"Rule": "server"})
		}
	}()
	backoff := time.Second
	for ctx.Err() == nil {
		var err error
		if c.cfg.SnapshotURL != "" {
			err = m.motionSnapshots(ctx, c, examine)
		} else {
			err = m.motionStream(ctx, c, examine)
		}
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			c.motion.set(err.Error())
		}
		if lastOn { // the state is unknown while the pictures do not come
			m.event(c, "motion", "server", "", false, false, time.Now(), "", map[string]string{"Rule": "server"})
			lastOn, det.bg, det.prev, det.last, det.on = false, nil, nil, time.Time{}, false
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, time.Minute)
	}
}

// motionStream examines the frames of the MJPEG stream (sub-stream first).
func (m *Manager) motionStream(ctx context.Context, c *cam, examine func([]byte, time.Time) error) error {
	src := c.main
	if c.sub != nil {
		src = c.sub
	}
	sub := src.subscribe(true)
	defer func() { sub.Close() }()
	var last time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case p, ok := <-sub.C:
			if !ok {
				return errors.New("stream ended")
			}
			if p.Track == nil || p.Sample == nil || p.Sample.Audio != nil {
				continue
			}
			if p.Track.Codec != MJPEG {
				if c.sub != nil && src == c.sub && c.main != nil {
					// the sub-stream is H.264/H.265: try the main stream
					sub.Close()
					src = c.main
					sub = src.subscribe(true)
					continue
				}
				return errors.New("needs an MJPEG stream or a snapshot address (the server does not decode " + strings.ToUpper(p.Track.Codec) + ")")
			}
			now := time.Now()
			if now.Sub(last) < motionInterval {
				continue
			}
			last = now
			if err := examine(p.Sample.JPEG, now); err != nil {
				c.motion.set("picture: " + err.Error())
				continue
			}
			c.motion.set("watching (MJPEG)")
		}
	}
}

// motionSnapshots polls the snapshot address of the camera.
func (m *Manager) motionSnapshots(ctx context.Context, c *cam, examine func([]byte, time.Time) error) error {
	u, err := StreamURL(c.cfg.SnapshotURL, c.cfg.Username, c.cfg.Password)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	t := time.NewTicker(snapshotEvery)
	defer t.Stop()
	fails := 0
	for {
		pic, err := getSnapshot(ctx, client, u)
		if err == nil {
			err = examine(pic, time.Now())
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			fails++
			c.motion.set("snapshot: " + redact(err.Error(), u))
			if fails >= 5 {
				return fmt.Errorf("snapshot: %s", redact(err.Error(), u))
			}
		} else {
			fails = 0
			c.motion.set("watching (snapshots)")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// getSnapshot fetches one JPEG with basic or digest authentication.
func getSnapshot(ctx context.Context, client *http.Client, u *url.URL) ([]byte, error) {
	user := u.User
	clean := *u
	clean.User = nil
	do := func(auth string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, clean.String(), nil)
		if err != nil {
			return nil, err
		}
		if auth != "" {
			req.Header.Set("Authorization", auth)
		} else if user != nil {
			pw, _ := user.Password()
			req.SetBasicAuth(user.Username(), pw)
		}
		return client.Do(req)
	}
	resp, err := do("")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && user != nil && strings.HasPrefix(strings.ToLower(resp.Header.Get("WWW-Authenticate")), "digest") {
		ch := resp.Header.Get("WWW-Authenticate")
		resp.Body.Close()
		pw, _ := user.Password()
		if resp, err = do(digestAuth(ch, user.Username(), pw, clean.RequestURI())); err != nil {
			return nil, err
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

// digestAuth answers an HTTP digest challenge (RFC 7616 with MD5, qop=auth — what IP cameras use).
func digestAuth(challenge, user, pw, uri string) string {
	p := map[string]string{}
	for _, part := range strings.Split(strings.TrimSpace(challenge[len("digest"):]), ",") {
		if k, v, ok := strings.Cut(strings.TrimSpace(part), "="); ok {
			p[strings.ToLower(k)] = strings.Trim(v, `"`)
		}
	}
	h := func(s string) string { x := md5.Sum([]byte(s)); return hex.EncodeToString(x[:]) }
	ha1 := h(user + ":" + p["realm"] + ":" + pw)
	ha2 := h("GET:" + uri)
	nb := make([]byte, 8)
	_, _ = rand.Read(nb)
	cnonce := hex.EncodeToString(nb)
	out := fmt.Sprintf(`Digest username=%q, realm=%q, nonce=%q, uri=%q, algorithm=MD5`, user, p["realm"], p["nonce"], uri)
	if strings.Contains(p["qop"], "auth") {
		out += fmt.Sprintf(`, qop=auth, nc=00000001, cnonce=%q, response=%q`, cnonce, h(ha1+":"+p["nonce"]+":00000001:"+cnonce+":auth:"+ha2))
	} else {
		out += fmt.Sprintf(`, response=%q`, h(ha1+":"+p["nonce"]+":"+ha2))
	}
	if p["opaque"] != "" {
		out += fmt.Sprintf(`, opaque=%q`, p["opaque"])
	}
	return out
}

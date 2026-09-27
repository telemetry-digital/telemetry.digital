package video

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// scene draws a grey picture with a light block at x (in pixels; -1 = no block) and an overall brightness.
func scene(t *testing.T, x int, light uint8) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	for py := 0; py < 180; py++ {
		for px := 0; px < 320; px++ {
			c := light
			if x >= 0 && px >= x && px < x+40 && py >= 70 && py < 130 {
				c = 240
			}
			img.SetRGBA(px, py, color.RGBA{c, c, c, 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func run(t *testing.T, d *motionDetector, pics ...[]byte) (on bool, changes int) {
	now := time.Now()
	for i, p := range pics {
		cells, err := cellsOf(p)
		if err != nil {
			t.Fatal(err)
		}
		var ch bool
		on, ch = d.feed(cells, now.Add(time.Duration(i)*500*time.Millisecond))
		if ch {
			changes++
		}
	}
	return on, changes
}

func TestMotionDetector(t *testing.T) {
	still := scene(t, -1, 80)
	if on, _ := run(t, newMotionDetector(50, nil), still, still, still, still, still); on {
		t.Fatal("motion in a still picture")
	}
	// a block moving through the picture
	var moving [][]byte
	moving = append(moving, still, still)
	for x := 20; x < 260; x += 30 {
		moving = append(moving, scene(t, x, 80))
	}
	if on, n := run(t, newMotionDetector(50, nil), moving...); !on || n != 1 {
		t.Fatalf("moving block: on %v, changes %d", on, n)
	}
	// a single changed picture is not motion (two in a row are needed)
	if on, _ := run(t, newMotionDetector(50, nil), still, still, scene(t, 100, 80), still, still); on {
		t.Fatal("one odd picture raised motion")
	}
	// the zone covers only the right edge: the block moving on the left is ignored
	zone := []MotionZone{{{0.9, 0}, {1, 0}, {1, 1}, {0.9, 1}}}
	var left [][]byte
	left = append(left, still, still)
	for x := 10; x < 150; x += 30 {
		left = append(left, scene(t, x, 80))
	}
	if on, _ := run(t, newMotionDetector(100, zone), left...); on {
		t.Fatal("motion outside the zone")
	}
	// lights switched on: the whole picture changes, no motion
	bright := scene(t, -1, 200)
	if on, _ := run(t, newMotionDetector(50, nil), still, still, bright, bright, bright); on {
		t.Fatal("a lighting change raised motion")
	}
	// motion ends after the hold time without changes
	d := newMotionDetector(50, nil)
	run(t, d, moving...)
	cells, _ := cellsOf(scene(t, 250, 80))
	later := time.Now().Add(time.Minute)
	for i := 0; i < 30; i++ {
		d.feed(cells, later.Add(time.Duration(i)*500*time.Millisecond))
	}
	if d.on {
		t.Fatal("motion did not end")
	}
	// the threshold follows the sensitivity
	lo, hi := newMotionDetector(1, nil), newMotionDetector(100, nil)
	d1, s1 := lo.thresholds()
	d2, s2 := hi.thresholds()
	if d1 <= d2 || s1 <= s2 {
		t.Fatal("lower sensitivity must need a bigger change")
	}
}

func TestSnapshotDigest(t *testing.T) {
	pic := scene(t, -1, 80)
	const realm, nonce = "IP Camera", "abc123"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a := r.Header.Get("Authorization")
		if !strings.HasPrefix(a, "Digest ") {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Digest realm=%q, nonce=%q, qop="auth"`, realm, nonce))
			w.WriteHeader(401)
			return
		}
		p := map[string]string{}
		for _, part := range strings.Split(a[7:], ",") {
			k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
			p[k] = strings.Trim(v, `"`)
		}
		h := func(s string) string { x := md5.Sum([]byte(s)); return hex.EncodeToString(x[:]) }
		want := h(h("admin:"+realm+":secret") + ":" + nonce + ":" + p["nc"] + ":" + p["cnonce"] + ":auth:" + h("GET:"+r.URL.RequestURI()))
		if p["response"] != want {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(pic)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL + "/ISAPI/Streaming/channels/101/picture?x=1")
	u.User = url.UserPassword("admin", "secret")
	got, err := getSnapshot(context.Background(), srv.Client(), u)
	if err != nil || !bytes.Equal(got, pic) {
		t.Fatalf("digest snapshot: %v", err)
	}
	u.User = url.UserPassword("admin", "wrong")
	if _, err := getSnapshot(context.Background(), srv.Client(), u); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("wrong password: %v", err)
	}
}

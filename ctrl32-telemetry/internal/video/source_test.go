package video

import (
	"bytes"
	"net"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/video/camsim"
)

func freePort(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

func startSim(t *testing.T) string {
	addr := freePort(t)
	s := &camsim.Server{RTSPAddr: addr, FPS: 20}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return addr
}

// receive subscribes and returns the first n packets.
func receive(t *testing.T, src *Source, n int) []Packet {
	sub := src.Subscribe()
	defer sub.Close()
	var out []Packet
	timeout := time.After(15 * time.Second)
	for len(out) < n {
		select {
		case p := <-sub.C:
			out = append(out, p)
		case <-timeout:
			t.Fatalf("got %d of %d packets; status %+v", len(out), n, src.Status())
		}
	}
	return out
}

func TestRTSPH264(t *testing.T) {
	addr := startSim(t)
	u, _ := url.Parse("rtsp://" + addr + "/gate/h264/160x96")
	for _, tr := range []string{"tcp", "auto"} {
		src := &Source{URL: u, Transport: tr, Linger: time.Millisecond}
		pk := receive(t, src, 10)
		tr0 := pk[0].Track
		if tr0.Codec != H264 || tr0.Width != 160 || tr0.Height != 96 || !pk[0].Sample.Key {
			t.Fatalf("%s: track %+v key %v", tr, tr0, pk[0].Sample.Key)
		}
		if got := tr0.MIME(); got != `video/mp4; codecs="avc1.42c01e"` {
			t.Fatalf("mime %s", got)
		}
		m := &Muxer{Track: tr0}
		init, err := m.Init()
		if err != nil {
			t.Fatal(err)
		}
		var in fmp4.Init
		if err := in.Unmarshal(bytes.NewReader(init)); err != nil {
			t.Fatal(err)
		}
		var frags [][]byte
		for _, p := range pk {
			if p.Sample.DTS < pk[0].Sample.DTS {
				t.Fatal("DTS goes back")
			}
			f, err := m.Push(p.Sample)
			if err != nil {
				t.Fatal(err)
			}
			if f != nil {
				frags = append(frags, f)
			}
		}
		if len(frags) != 9 {
			t.Fatalf("fragments %d", len(frags))
		}
		var parts fmp4.Parts
		if err := parts.Unmarshal(bytes.Join(frags, nil)); err != nil {
			t.Fatal(err)
		}
		d := parts[1].Tracks[0].Samples[0].Duration
		if d < 90000/20/2 || d > 90000/20*2 {
			t.Fatalf("duration %d", d)
		}
		src.Stop()
	}
}

func TestRTSPMJPEG(t *testing.T) {
	addr := startSim(t)
	u, _ := url.Parse("rtsp://" + addr + "/door/mjpeg/320x240")
	src := &Source{URL: u, Linger: time.Millisecond}
	pk := receive(t, src, 3)
	if pk[0].Track.Codec != MJPEG || pk[0].Track.Width != 320 || len(pk[0].Sample.JPEG) < 1000 {
		t.Fatalf("track %+v", pk[0].Track)
	}
	if src.Snapshot() == nil {
		t.Fatal("no snapshot")
	}
	src.Stop()
}

func TestHTTPMJPEG(t *testing.T) {
	ts := httptest.NewServer(camsim.MJPEGHandler(20))
	defer ts.Close()
	u, _ := url.Parse(ts.URL + "/yard/320x176")
	u.User = url.UserPassword("admin", "secret")
	src := &Source{URL: u, Linger: time.Millisecond}
	pk := receive(t, src, 3)
	if pk[0].Track.Codec != MJPEG || pk[0].Track.Height != 176 {
		t.Fatalf("track %+v", pk[0].Track)
	}
	st := src.Status()
	if st.State != "online" {
		t.Fatalf("status %+v", st)
	}
	src.Stop()
}

func TestOfflineRedactsPassword(t *testing.T) {
	u, _ := url.Parse("rtsp://admin:topsecret@" + freePort(t) + "/x")
	src := &Source{URL: u, Linger: time.Millisecond}
	src.Keep(true)
	defer src.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st := src.Status(); st.State == "offline" {
			if bytes.Contains([]byte(st.Error), []byte("topsecret")) {
				t.Fatalf("password in error: %s", st.Error)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("status %+v", src.Status())
}

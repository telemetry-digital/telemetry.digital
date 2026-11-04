package video

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/video/testsrc"
)

// A connection runs for days: the time of a frame keeps growing past the 28.47 hours where ticks * time.Second overflows.
func TestTimeOfLongConnection(t *testing.T) {
	for _, d := range []time.Duration{0, time.Second, 28 * time.Hour, 29 * time.Hour, 100 * time.Hour, 24 * 400 * time.Hour} {
		ticks := int64(d/time.Second) * ClockRate
		if got := ticksToDuration(ticks); got != d {
			t.Errorf("ticksToDuration(%v) = %v", d, got)
		}
		if got := durationToTicks(d); got != ticks {
			t.Errorf("durationToTicks(%v) = %d, want %d", d, got, ticks)
		}
	}
	if got := ticksToDuration(ClockRate*100 + ClockRate/2); got != 100*time.Second+500*time.Millisecond {
		t.Errorf("fraction of a second lost: %v", got)
	}
	if got := durationToTicks(100*time.Second + 500*time.Millisecond); got != ClockRate*100+ClockRate/2 {
		t.Errorf("fraction of a second lost: %d", got)
	}
}

// longFile writes a recording of the given number of 1 s fragments, with the first picture of each at i seconds.
func longFile(t *testing.T, seconds int) (*os.File, int64, int) {
	t.Helper()
	enc := &testsrc.Encoder{W: 64, H: 48}
	m := &Muxer{Track: &Track{Codec: H264, SPS: enc.SPS(), PPS: enc.PPS()}, Batch: ClockRate}
	var buf bytes.Buffer
	init, err := m.Init()
	if err != nil {
		t.Fatal(err)
	}
	buf.Write(init)
	frame := enc.Frame()
	for i := 0; i <= seconds; i++ {
		// the picture data holds the box name, as real pictures may: the search must not take it for a fragment
		au := append([][]byte{}, frame...)
		au = append(au, []byte("xxxxmoofxxxx"))
		frag, err := m.Push(&Sample{DTS: int64(i) * ClockRate, Key: true, AU: au})
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(frag)
	}
	path := filepath.Join(t.TempDir(), "long.mp4")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f, int64(len(init)), buf.Len()
}

func TestSeekFragment(t *testing.T) {
	f, off, size := longFile(t, 300)
	perSecond := (int64(size) - off) / 300
	near := perSecond * 5
	baseOf := func(at int64) int64 {
		typ, moof, err := readBox(f, at)
		if err != nil || typ != "moof" {
			t.Fatalf("offset %d is not a fragment: %q %v", at, typ, err)
		}
		_, mdat, err := readBox(f, at+int64(len(moof)))
		if err != nil {
			t.Fatal(err)
		}
		var parts fmp4.Parts
		if err := parts.Unmarshal(append(moof, mdat...)); err != nil {
			t.Fatal(err)
		}
		return int64(parts[0].Tracks[0].BaseTime)
	}
	for _, sec := range []int64{20, 150, 299} {
		want := sec * ClockRate
		at := seekFragment(f, off, int64(size), 1, want, near)
		got := baseOf(at)
		if got > want {
			t.Errorf("%d s: landed after the requested time, at %d s", sec, got/ClockRate)
		}
		if want-got > 20*ClockRate {
			t.Errorf("%d s: landed too far before it, at %d s", sec, got/ClockRate)
		}
	}
	if at := seekFragment(f, off, int64(size), 1, -1, near); at != off {
		t.Errorf("a time before the file must stay at its start, got %d", at)
	}
}

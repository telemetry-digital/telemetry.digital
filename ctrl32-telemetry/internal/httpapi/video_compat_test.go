package httpapi

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testFFmpeg: CTRL32_TEST_FFMPEG or ffmpeg on PATH (skipped without one, or without the OpenH264 encoder).
func testFFmpeg(t *testing.T) (ff, probe string) {
	ff = os.Getenv("CTRL32_TEST_FFMPEG")
	if ff == "" {
		ff = "ffmpeg"
	}
	ff, err := exec.LookPath(ff)
	if err != nil {
		t.Skip("no ffmpeg")
	}
	out, _ := exec.Command(ff, "-hide_banner", "-encoders").Output()
	if !bytes.Contains(out, []byte("libopenh264")) {
		t.Skip("ffmpeg without libopenh264")
	}
	probe = filepath.Join(filepath.Dir(ff), strings.Replace(filepath.Base(ff), "ffmpeg", "ffprobe", 1))
	if _, err := os.Stat(probe); err != nil {
		t.Skip("no ffprobe next to ffmpeg")
	}
	return ff, probe
}

// A fragmented MP4 written into a pipe (as the recordings are) comes out as H.264 scaled to at most 1080 lines.
func TestTranscodeH264(t *testing.T) {
	ff, probe := testFFmpeg(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mp4")
	gen := exec.Command(ff, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=1920x1440:rate=10:duration=3",
		"-c:v", "libopenh264", "-movflags", "frag_keyframe+empty_moov+default_base_moof", "-f", "mp4", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("source: %v %s", err, out)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	feed := func(w io.Writer) error { _, err := w.Write(data); return err }
	if err := transcodeH264(ctx, ff, feed, &got, false); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "dst.mp4")
	if err := os.WriteFile(dst, got.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(probe, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=codec_name,width,height",
		"-of", "csv=p=0", dst).Output()
	if err != nil {
		t.Fatal(err)
	}
	if s := strings.TrimSpace(string(out)); s != "h264,1440,1080" {
		t.Fatalf("converted stream = %q, want h264,1440,1080", s)
	}
}

// A broken input is reported with ffmpeg's message.
func TestTranscodeH264Error(t *testing.T) {
	ff, _ := testFFmpeg(t)
	feed := func(w io.Writer) error { _, err := w.Write([]byte("not a video")); return err }
	err := transcodeH264(context.Background(), ff, feed, io.Discard, false)
	if err == nil || !strings.Contains(err.Error(), "ffmpeg") {
		t.Fatalf("err = %v", err)
	}
}

package httpapi

// Download in a compatible format (0.88.0): cameras recording H.265 (HEVC) give MP4 files that Windows plays only with
// a paid extension. On request the download is converted to H.264 + AAC by ffmpeg (the LGPL build installed for object
// counting, encoder OpenH264). The original stays the default: only it is the unchanged recording (evidence packages
// always carry the original).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics"
)

// CompatMaxRange: a conversion takes CPU time, so one file covers at most an hour.
const CompatMaxRange = time.Hour

// compatSlot: one conversion at a time per server (a 4K hour keeps several cores busy for minutes).
var compatSlot sync.Mutex

// compatFFmpeg returns the ffmpeg able to convert, or "" when none is installed.
func (s *Server) compatFFmpeg() string {
	ff := analytics.FFmpegPath(s.cfg.Analytics.FFmpeg)
	p, err := exec.LookPath(ff)
	if err != nil {
		return ""
	}
	return p
}

// compatArgs: fragmented MP4 on stdin → H.264 (at most 1080 lines) + AAC as fragmented MP4 on stdout.
func compatArgs(sound bool) []string {
	a := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-f", "mp4", "-i", "pipe:0", "-map", "0:v:0",
		"-vf", "scale=-2:'min(1080,ih)'", "-c:v", "libopenh264", "-b:v", "4M"}
	if sound {
		a = append(a, "-map", "0:a:0?", "-c:a", "aac", "-b:a", "96k")
	} else {
		a = append(a, "-an")
	}
	return append(a, "-movflags", "frag_keyframe+empty_moov+default_base_moof", "-f", "mp4", "pipe:1")
}

// transcodeH264 feeds the MP4 written by src through ffmpeg into w.
func transcodeH264(ctx context.Context, ff string, src func(io.Writer) error, w io.Writer, sound bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, ff, compatArgs(sound)...)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = w
	var stderr bytes.Buffer
	cmd.Stderr = &limitedBuffer{b: &stderr, n: 4096}
	if err := cmd.Start(); err != nil {
		return err
	}
	srcErr := make(chan error, 1)
	go func() {
		err := src(in)
		_ = in.Close()
		srcErr <- err
	}()
	err = cmd.Wait()
	if e := <-srcErr; e != nil && err == nil && !errors.Is(e, io.ErrClosedPipe) {
		err = e
	}
	if err != nil && ctx.Err() == nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return err
}

type limitedBuffer struct {
	b *bytes.Buffer
	n int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if room := l.n - l.b.Len(); room > 0 {
		if len(p) > room {
			l.b.Write(p[:room])
		} else {
			l.b.Write(p)
		}
	}
	return len(p), nil
}

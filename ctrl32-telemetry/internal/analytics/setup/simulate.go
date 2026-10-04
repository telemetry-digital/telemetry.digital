package setup

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics"
)

// Simulation of the downloads for development and the end-to-end tests (the server agent's [agent] vpn_simulate):
// small stand-in archives with the same layout as the real ones, pinned by their own SHA-256, served from memory.
// Nothing is downloaded and the stand-in "ffmpeg" is a text file that is read, never run.

// SimFile is one stand-in download.
type SimFile struct {
	Artifact analytics.Artifact
	Body     []byte
}

// SimulatedArtifacts builds the stand-in downloads of a platform.
func SimulatedArtifacts(platform string) []SimFile {
	win := strings.HasPrefix(platform, "windows/")
	ffVersion := []byte("ffmpeg version n8.1.2-simulation Copyright (c) the FFmpeg developers\nconfiguration: --enable-shared --disable-static\n")
	var ort, ff []byte
	format := analytics.FormatTarGz
	if win {
		format = analytics.FormatZip
		ort = zipOf(map[string][]byte{"onnxruntime-win-x64-sim/VERSION_NUMBER": []byte(analytics.ORTVersion + "\n"),
			"onnxruntime-win-x64-sim/lib/onnxruntime.dll": []byte("simulated ONNX Runtime"), "onnxruntime-win-x64-sim/LICENSE": []byte("MIT (simulation)"),
			"onnxruntime-win-x64-sim/include/onnxruntime_c_api.h": []byte("not unpacked")})
		ff = zipOf(map[string][]byte{"ffmpeg-sim/bin/ffmpeg.exe": ffVersion, "ffmpeg-sim/bin/avcodec-62.dll": []byte("simulated"),
			"ffmpeg-sim/LICENSE.txt": []byte("LGPL v3 (simulation)"), "ffmpeg-sim/doc/readme.txt": []byte("not unpacked")})
	} else {
		ort = tgzOf(map[string][]byte{"onnxruntime-linux-sim/VERSION_NUMBER": []byte(analytics.ORTVersion + "\n"),
			"onnxruntime-linux-sim/lib/libonnxruntime.so": []byte("simulated ONNX Runtime"), "onnxruntime-linux-sim/LICENSE": []byte("MIT (simulation)"),
			"onnxruntime-linux-sim/include/onnxruntime_c_api.h": []byte("not unpacked")})
		ff = tgzOf(map[string][]byte{"ffmpeg-sim/bin/ffmpeg": ffVersion, "ffmpeg-sim/lib/libavcodec.so.62": []byte("simulated"),
			"ffmpeg-sim/LICENSE.txt": []byte("LGPL v3 (simulation)"), "ffmpeg-sim/doc/readme.txt": []byte("not unpacked")})
	}
	base := "https://github.com/simulation/"
	mk := func(name, label, file, f string, body []byte) SimFile {
		s := sha256.Sum256(body)
		return SimFile{Artifact: analytics.Artifact{Name: name, Label: label, URL: base + file, SHA256: hex.EncodeToString(s[:]), Format: f}, Body: body}
	}
	return []SimFile{
		mk("onnxruntime", "ONNX Runtime "+analytics.ORTVersion+" (simulation)", "onnxruntime", format, ort),
		mk("ffmpeg", "ffmpeg (simulation)", "ffmpeg", format, ff),
		mk("model_nano", "YOLOX nano (simulation)", "yolox_nano.onnx", analytics.FormatFile, []byte("simulated yolox nano")),
		mk("model_tiny", "YOLOX tiny (simulation)", "yolox_tiny.onnx", analytics.FormatFile, []byte("simulated yolox tiny")),
		mk("model_licence", "YOLOX licence (simulation)", "LICENSE", analytics.FormatFile, []byte("Apache License 2.0 (simulation)")),
	}
}

// SimFetcher serves the stand-in downloads from memory, a little at a time, so the progress can be watched.
type SimFetcher struct {
	Files []SimFile
	Delay time.Duration // per chunk

	mu    sync.Mutex
	Count int // downloads served
}

func (s *SimFetcher) Fetch(ctx context.Context, url string, w io.Writer, progress func(n, total int64)) error {
	for _, f := range s.Files {
		if f.Artifact.URL != url {
			continue
		}
		s.mu.Lock()
		s.Count++
		s.mu.Unlock()
		total := int64(len(f.Body))
		chunk := max(1, len(f.Body)/4)
		var n int64
		for off := 0; off < len(f.Body); off += chunk {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.Delay):
			}
			end := min(off+chunk, len(f.Body))
			if _, err := w.Write(f.Body[off:end]); err != nil {
				return err
			}
			n = int64(end)
			if progress != nil {
				progress(n, total)
			}
		}
		return nil
	}
	return fmt.Errorf("HTTP 404")
}

// SimFFmpegVersion reads the stand-in ffmpeg (a text with its version) instead of running it.
func SimFFmpegVersion(_ context.Context, bin string) (string, error) {
	b, err := os.ReadFile(bin)
	return string(b), err
}

func tgzOf(files map[string][]byte) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range sortedKeys(files) {
		body := files[name]
		mode := int64(0o644)
		if strings.Contains(name, "/bin/") {
			mode = 0o755
		}
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(body)), Typeflag: tar.TypeReg, ModTime: time.Unix(1_700_000_000, 0)})
		_, _ = tw.Write(body)
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func zipOf(files map[string][]byte) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range sortedKeys(files) {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Unix(1_700_000_000, 0)}
		w, _ := zw.CreateHeader(h)
		_, _ = w.Write(files[name])
	}
	_ = zw.Close()
	return buf.Bytes()
}

func sortedKeys(m map[string][]byte) []string { return slices.Sorted(maps.Keys(m)) }

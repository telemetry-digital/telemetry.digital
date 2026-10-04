package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics/setup"
)

// fetchDetector downloads the pinned parts of the object counting detector for every platform the agent installs it
// on, unmodified, from their official releases (ADR-0022, ADR-0024), into a release directory's analytics/: the
// offline package and the distribution point then carry them for servers without the Internet. A file is kept only
// when its SHA-256 matches the pin; files already there with the right hash are kept.
//
//	relsign fetch-detector -out dist/0.76.0/analytics
func fetchDetector(args []string) error {
	fs := flag.NewFlagSet("fetch-detector", flag.ExitOnError)
	out := fs.String("out", "", "directory (analytics/ of the release directory)")
	_ = fs.Parse(args)
	if *out == "" {
		return errors.New("-out is required")
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	seen := map[string]analytics.Artifact{}
	var platforms []string
	for _, p := range analytics.InstallPlatforms {
		platforms = append(platforms, p)
		for _, a := range analytics.PinnedArtifacts(p) {
			seen[analytics.MirrorName(a)] = a
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	var src strings.Builder
	src.WriteString("Parts of the object counting detector (ADR-0022), unmodified, as published by their projects.\n" +
		"The server agent installs them only when their SHA-256 matches the pin compiled into the server.\n\n" +
		"- ONNX Runtime (MIT licence, github.com/microsoft/onnxruntime): the archive contains its LICENSE and ThirdPartyNotices.\n" +
		"- ffmpeg, LGPL build (GNU LGPL v3, github.com/BtbN/FFmpeg-Builds): the archive contains its licence; the source\n" +
		"  code of this build is available from FFmpeg (ffmpeg.org) at the version in its name and from the build project.\n" +
		"- YOLOX nano and tiny models (Apache License 2.0, github.com/Megvii-BaseDetection/YOLOX): licence in YOLOX-" + analytics.YOLOXRelease + "-LICENSE.\n\n" +
		"Platforms: " + strings.Join(platforms, ", ") + "\n\n")
	ctx := context.Background()
	f := setup.HTTPFetcher{}
	for _, n := range names {
		a := seen[n]
		dst := filepath.Join(*out, n)
		src.WriteString(n + "  sha256 " + a.SHA256 + "  " + a.URL + "\n")
		if fileHash(dst) == a.SHA256 {
			fmt.Println("kept      ", n)
			continue
		}
		tmp := dst + ".part"
		w, err := os.Create(tmp)
		if err != nil {
			return err
		}
		h := sha256.New()
		err = f.Fetch(ctx, a.URL, io.MultiWriter(w, h), nil)
		if cerr := w.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("%s: %w", a.URL, err)
		}
		if got := hex.EncodeToString(h.Sum(nil)); got != a.SHA256 {
			_ = os.Remove(tmp)
			return fmt.Errorf("%s: SHA-256 %s does not match the pin %s", a.URL, got, a.SHA256)
		}
		if err := os.Rename(tmp, dst); err != nil {
			return err
		}
		fmt.Println("downloaded", n)
	}
	return os.WriteFile(filepath.Join(*out, "SOURCES.txt"), []byte(src.String()), 0o644)
}

func fileHash(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

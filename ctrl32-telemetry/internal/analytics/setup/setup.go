// Package setup installs the detector of object counting on camera pictures (ADR-0022) the way deploy/install.sh
// --analytics and deploy/install.ps1 -Analytics do, for the server agent (button "Install the detector"): it downloads
// the pinned parts (internal/analytics/pins.go) over HTTPS from their official releases, keeps a file only when its
// SHA-256 matches the pin, unpacks into a staging directory next to the target, refuses an ffmpeg that is not an LGPL
// build and only then swaps the new directories and files in by renaming them. A failure at any point before the swap
// leaves the installed parts as they were; a failed swap is rolled back.
package setup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics"
)

// Layout is where the parts go on this host (the installers use the same places):
//
//	Linux    /opt/ctrl32-telemetry/analytics            /var/lib/ctrl32-telemetry/models
//	Windows  <install dir>\analytics (next to the .exe)  <data dir>\models (next to config.toml)
type Layout struct {
	AnalyticsDir string // ONNX Runtime and ffmpeg directories, SOURCES.txt
	ModelsDir    string // YOLOX models, their licence, SOURCES.txt
	Platform     string // os/arch, one of analytics.InstallPlatforms
}

func (l Layout) windows() bool { return strings.HasPrefix(l.Platform, "windows/") }

// ORTDir is the directory of ONNX Runtime.
func (l Layout) ORTDir() string {
	return filepath.Join(l.AnalyticsDir, "onnxruntime-"+analytics.ORTVersion)
}

// ORTLib is the ONNX Runtime library the worker loads.
func (l Layout) ORTLib() string {
	if l.windows() {
		return filepath.Join(l.ORTDir(), "lib", "onnxruntime.dll")
	}
	return filepath.Join(l.ORTDir(), "lib", "libonnxruntime.so")
}

// FFmpegDir is the directory of the LGPL ffmpeg build.
func (l Layout) FFmpegDir() string {
	return filepath.Join(l.AnalyticsDir, analytics.FFmpegBuild+"-lgpl")
}

// FFmpegBin is the ffmpeg executable.
func (l Layout) FFmpegBin() string {
	if l.windows() {
		return filepath.Join(l.FFmpegDir(), "bin", "ffmpeg.exe")
	}
	return filepath.Join(l.FFmpegDir(), "bin", "ffmpeg")
}

func (l Layout) modelFile(name string) string {
	switch name {
	case "model_nano":
		return filepath.Join(l.ModelsDir, "yolox_nano.onnx")
	case "model_tiny":
		return filepath.Join(l.ModelsDir, "yolox_tiny.onnx")
	case "model_licence":
		return filepath.Join(l.ModelsDir, "LICENSE-YOLOX.txt")
	}
	return ""
}

// Paths is what [analytics] in config.toml gets after an installation.
type Paths struct {
	FFmpeg      string `json:"ffmpeg"`
	ONNXRuntime string `json:"onnxruntime"`
	ModelNano   string `json:"model_nano"`
	ModelTiny   string `json:"model_tiny"`
	NanoSHA256  string `json:"nano_sha256"`
	TinySHA256  string `json:"tiny_sha256"`
}

// PathsOf returns the configuration of an installation on the layout.
func PathsOf(l Layout) Paths {
	return Paths{FFmpeg: l.FFmpegBin(), ONNXRuntime: l.ORTLib(), ModelNano: l.modelFile("model_nano"), ModelTiny: l.modelFile("model_tiny"),
		NanoSHA256: analytics.YOLOXNanoSHA256, TinySHA256: analytics.YOLOXTinySHA256}
}

// Progress is reported while installing.
type Progress struct {
	Step  string `json:"step"`            // prepare, download, unpack, verify, swap, done
	Item  int    `json:"item,omitempty"`  // download Item of Items
	Items int    `json:"items,omitempty"` //
	Label string `json:"label,omitempty"` // the part being downloaded
	Bytes int64  `json:"bytes,omitempty"` // downloaded of Total (Total 0 = unknown)
	Total int64  `json:"total,omitempty"`
}

// Fetcher downloads a URL into w; progress is called with the bytes so far and the total (0 = unknown).
type Fetcher interface {
	Fetch(ctx context.Context, url string, w io.Writer, progress func(n, total int64)) error
}

// Options of an installation.
type Options struct {
	Layout    Layout
	Artifacts []analytics.Artifact // analytics.PinnedArtifacts(platform); tests and the simulation pass their own
	Fetch     Fetcher
	// FFmpegVersion runs `ffmpeg -hide_banner -version` of the staged build (default: runs it); its output decides
	// whether the build is LGPL
	FFmpegVersion func(ctx context.Context, bin string) (string, error)
	// Unxz opens an xz-compressed file for reading (default: the xz program)
	Unxz func(ctx context.Context, file string) (io.ReadCloser, error)
	// FreeMB reports the free space of the file system of a directory (ok false = unknown, not checked)
	FreeMB func(dir string) (int64, bool)
	// Chown gives the models to the service user (Linux; nil = leave the owner)
	Chown func(path string) error
	Force bool   // download and replace every part even when it is installed
	By    string // who installs, for SOURCES.txt ("install.sh", "the server agent")
}

// Result of an installation.
type Result struct {
	Installed []string `json:"installed"` // parts downloaded and installed
	Kept      []string `json:"kept"`      // parts that were installed already
	Paths     Paths    `json:"paths"`
}

// Space the installation needs on each file system (downloads, unpacked parts and the previous parts while swapping).
const (
	needAnalyticsMB = 500
	needModelsMB    = 80
)

// rename moves a part into place (replaced in tests to make a swap fail).
var rename = os.Rename

// ErrNotLGPL refuses an ffmpeg build configured with --enable-gpl or --enable-nonfree.
var ErrNotLGPL = errors.New("the downloaded ffmpeg is not an LGPL build (configured with --enable-gpl or --enable-nonfree) - not installed")

// Install downloads, verifies and installs what is missing (everything with Force). Nothing installed changes unless
// every needed part downloaded, matched its pin and unpacked.
func Install(ctx context.Context, o Options, report func(Progress)) (Result, error) {
	if report == nil {
		report = func(Progress) {}
	}
	o.defaults()
	l := o.Layout
	res := Result{Paths: PathsOf(l)}
	if len(o.Artifacts) == 0 {
		return res, fmt.Errorf("the detector cannot be installed on %s (Linux amd64 or arm64, Windows amd64)", l.Platform)
	}
	report(Progress{Step: "prepare"})
	if err := os.MkdirAll(l.AnalyticsDir, 0o755); err != nil {
		return res, fmt.Errorf("cannot create %s: %w", l.AnalyticsDir, explain(err))
	}
	if err := os.MkdirAll(l.ModelsDir, 0o750); err != nil {
		return res, fmt.Errorf("cannot create %s: %w", l.ModelsDir, explain(err))
	}
	cleanLeftovers(l)
	// what is needed: a part counts as installed when it works as the installers leave it
	var need []analytics.Artifact
	for _, a := range o.Artifacts {
		if !o.Force && o.installed(ctx, a) {
			res.Kept = append(res.Kept, a.Name)
			continue
		}
		need = append(need, a)
	}
	if len(need) == 0 {
		report(Progress{Step: "done"})
		return res, nil
	}
	if free, ok := o.FreeMB(l.AnalyticsDir); ok && free < needAnalyticsMB {
		return res, fmt.Errorf("not enough free disk space: %d MB free in %s, about %d MB needed", free, l.AnalyticsDir, needAnalyticsMB)
	}
	if free, ok := o.FreeMB(l.ModelsDir); ok && free < needModelsMB {
		return res, fmt.Errorf("not enough free disk space: %d MB free in %s, about %d MB needed", free, l.ModelsDir, needModelsMB)
	}
	// staging directories next to the targets (same file system: the swap is a rename)
	tag := randomTag()
	stA, stM := filepath.Join(l.AnalyticsDir, ".install-"+tag), filepath.Join(l.ModelsDir, ".install-"+tag)
	defer os.RemoveAll(stA)
	defer os.RemoveAll(stM)
	for _, d := range []string{stA, stM} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return res, fmt.Errorf("cannot create %s: %w", d, explain(err))
		}
	}
	// 1. downloads, each checked against its pin
	files := map[string]string{}
	for i, a := range need {
		dir := stA
		if l.modelFile(a.Name) != "" {
			dir = stM
		}
		f := filepath.Join(dir, a.Name+".download")
		p := Progress{Step: "download", Item: i + 1, Items: len(need), Label: a.Label}
		report(p)
		if err := o.download(ctx, a, f, func(n, total int64) { p.Bytes, p.Total = n, total; report(p) }); err != nil {
			return res, err
		}
		files[a.Name] = f
	}
	// 2. unpack the archives into the staging directory and check them
	type swap struct{ from, to string }
	var swaps []swap
	for _, a := range need {
		f := files[a.Name]
		switch a.Name {
		case "onnxruntime", "ffmpeg":
			report(Progress{Step: "unpack", Label: a.Label})
			target := l.ORTDir()
			if a.Name == "ffmpeg" {
				target = l.FFmpegDir()
			}
			staged := filepath.Join(stA, filepath.Base(target))
			if err := unpack(ctx, o, a, f, staged, keepFunc(a.Name, l.windows())); err != nil {
				return res, fmt.Errorf("unpacking %s: %w", a.Label, explain(err))
			}
			_ = os.Remove(f)
			report(Progress{Step: "verify", Label: a.Label})
			if err := o.verify(ctx, a.Name, staged, l); err != nil {
				return res, err
			}
			swaps = append(swaps, swap{staged, target})
		default:
			swaps = append(swaps, swap{f, l.modelFile(a.Name)})
		}
	}
	// SOURCES.txt beside the parts (what was downloaded from where, and the licences)
	if err := os.WriteFile(filepath.Join(stA, "SOURCES.txt"), []byte(sourcesAnalytics(o.By, o.Artifacts)), 0o644); err != nil {
		return res, fmt.Errorf("cannot write SOURCES.txt: %w", explain(err))
	}
	if err := os.WriteFile(filepath.Join(stM, "SOURCES.txt"), []byte(sourcesModels(o.By, o.Artifacts)), 0o644); err != nil {
		return res, fmt.Errorf("cannot write SOURCES.txt: %w", explain(err))
	}
	swaps = append(swaps, swap{filepath.Join(stA, "SOURCES.txt"), filepath.Join(l.AnalyticsDir, "SOURCES.txt")},
		swap{filepath.Join(stM, "SOURCES.txt"), filepath.Join(l.ModelsDir, "SOURCES.txt")})
	// 3. swap: every new part replaces the old one by renaming; on a failure the swaps done so far are undone
	report(Progress{Step: "swap"})
	type done struct{ to, old string }
	var did []done
	rollback := func() {
		for i := len(did) - 1; i >= 0; i-- {
			_ = os.RemoveAll(did[i].to)
			if did[i].old != "" {
				_ = rename(did[i].old, did[i].to)
			}
		}
	}
	for _, s := range swaps {
		old := ""
		if _, err := os.Lstat(s.to); err == nil {
			old = s.to + ".old-" + tag
			if err := rename(s.to, old); err != nil {
				rollback()
				return res, fmt.Errorf("cannot replace %s (is it in use?): %w", s.to, explain(err))
			}
		}
		if err := rename(s.from, s.to); err != nil {
			if old != "" {
				_ = rename(old, s.to)
			}
			rollback()
			return res, fmt.Errorf("cannot move the new %s into place: %w", filepath.Base(s.to), explain(err))
		}
		did = append(did, done{s.to, old})
	}
	for _, d := range did {
		if d.old != "" {
			_ = os.RemoveAll(d.old)
		}
	}
	for _, a := range need {
		res.Installed = append(res.Installed, a.Name)
	}
	if o.Chown != nil {
		_ = filepath.Walk(l.ModelsDir, func(p string, _ os.FileInfo, err error) error {
			if err == nil {
				_ = o.Chown(p)
			}
			return nil
		})
	}
	report(Progress{Step: "done"})
	return res, nil
}

// Remove deletes the installed parts (the parts of earlier pins too) and leaves everything else.
func Remove(l Layout) error {
	var errs []error
	rm := func(p string) {
		if err := os.RemoveAll(p); err != nil {
			errs = append(errs, err)
		}
	}
	for _, g := range []string{"onnxruntime-*", "ffmpeg-*-lgpl", ".install-*", "SOURCES.txt"} {
		m, _ := filepath.Glob(filepath.Join(l.AnalyticsDir, g))
		for _, p := range m {
			rm(p)
		}
	}
	for _, n := range []string{"yolox_nano.onnx", "yolox_tiny.onnx", "LICENSE-YOLOX.txt", "SOURCES.txt"} {
		rm(filepath.Join(l.ModelsDir, n))
	}
	m, _ := filepath.Glob(filepath.Join(l.ModelsDir, ".install-*"))
	for _, p := range m {
		rm(p)
	}
	_ = os.Remove(l.AnalyticsDir) // only when empty
	return errors.Join(errs...)
}

// ---- helpers ----------------------------------------------------------------------------------------------------

func (o *Options) defaults() {
	if o.Fetch == nil {
		o.Fetch = HTTPFetcher{}
	}
	if o.FFmpegVersion == nil {
		o.FFmpegVersion = runVersion
	}
	if o.Unxz == nil {
		o.Unxz = execUnxz
	}
	if o.FreeMB == nil {
		o.FreeMB = func(string) (int64, bool) { return 0, false }
	}
	if o.By == "" {
		o.By = "the server agent"
	}
}

// installed reports whether a part works as installed (same rules as the installers).
func (o *Options) installed(ctx context.Context, a analytics.Artifact) bool {
	l := o.Layout
	switch a.Name {
	case "onnxruntime":
		v, err := os.ReadFile(filepath.Join(l.ORTDir(), "VERSION_NUMBER"))
		_, lerr := os.Stat(l.ORTLib())
		return err == nil && strings.TrimSpace(string(v)) == analytics.ORTVersion && lerr == nil
	case "ffmpeg":
		if _, err := os.Stat(l.FFmpegBin()); err != nil {
			return false
		}
		out, err := o.FFmpegVersion(ctx, l.FFmpegBin())
		return err == nil && lgpl(out) == nil
	default:
		return fileSHA256(l.modelFile(a.Name)) == a.SHA256
	}
}

func (o *Options) download(ctx context.Context, a analytics.Artifact, dest string, progress func(n, total int64)) error {
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", dest, explain(err))
	}
	h := sha256.New()
	err = o.Fetch.Fetch(ctx, a.URL, io.MultiWriter(f, h), progress)
	if cerr := f.Close(); err == nil && cerr != nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("download of %s failed (%s): %w", a.Label, a.URL, explain(err))
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != a.SHA256 {
		_ = os.Remove(dest)
		return fmt.Errorf("SHA-256 of %s does not match the pin (got %s, pinned %s) - nothing was installed", a.URL, got, a.SHA256)
	}
	return nil
}

func (o *Options) verify(ctx context.Context, name, staged string, l Layout) error {
	switch name {
	case "onnxruntime":
		lib := filepath.Join(staged, "lib", filepath.Base(l.ORTLib()))
		if _, err := os.Stat(lib); err != nil {
			return fmt.Errorf("the ONNX Runtime archive has no %s - not installed", filepath.Base(lib))
		}
		v, err := os.ReadFile(filepath.Join(staged, "VERSION_NUMBER"))
		if err != nil || strings.TrimSpace(string(v)) != analytics.ORTVersion {
			return fmt.Errorf("the ONNX Runtime archive is not version %s - not installed", analytics.ORTVersion)
		}
	case "ffmpeg":
		bin := filepath.Join(staged, "bin", filepath.Base(l.FFmpegBin()))
		if _, err := os.Stat(bin); err != nil {
			return fmt.Errorf("the ffmpeg archive has no %s - not installed", filepath.Base(bin))
		}
		out, err := o.FFmpegVersion(ctx, bin)
		if err != nil {
			return fmt.Errorf("the downloaded ffmpeg does not run on this server: %v - not installed", err)
		}
		if err := lgpl(out); err != nil {
			return err
		}
	}
	return nil
}

// lgpl checks the `ffmpeg -version` output: an ffmpeg, configured without --enable-gpl and --enable-nonfree.
func lgpl(versionOutput string) error {
	if !strings.Contains(versionOutput, "ffmpeg version") {
		return errors.New("the downloaded program does not report an ffmpeg version - not installed")
	}
	if strings.Contains(versionOutput, "--enable-gpl") || strings.Contains(versionOutput, "--enable-nonfree") {
		return ErrNotLGPL
	}
	return nil
}

func runVersion(ctx context.Context, bin string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "-hide_banner", "-version").CombinedOutput()
	return string(out), err
}

func fileSHA256(path string) string {
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

// cleanLeftovers removes staging directories and parked parts of an interrupted earlier run.
func cleanLeftovers(l Layout) {
	for _, d := range []string{l.AnalyticsDir, l.ModelsDir} {
		for _, g := range []string{".install-*", "*.old-*"} {
			m, _ := filepath.Glob(filepath.Join(d, g))
			for _, p := range m {
				_ = os.RemoveAll(p)
			}
		}
	}
}

func randomTag() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// explain turns the errors people meet into plain words.
func explain(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, syscall.ENOSPC) || strings.Contains(err.Error(), "not enough space on the disk") || strings.Contains(err.Error(), "no space left"):
		return fmt.Errorf("the disk is full (%w)", err)
	case errors.Is(err, os.ErrPermission):
		return fmt.Errorf("permission denied (%w)", err)
	}
	return err
}

func sourcesAnalytics(by string, as []analytics.Artifact) string {
	var b strings.Builder
	b.WriteString("Third-party programs for object counting (installed by " + by + " on " + time.Now().UTC().Format(time.RFC3339) + "; not part of ctrl32 telemetry)\n")
	for _, a := range as {
		if a.Name == "onnxruntime" || a.Name == "ffmpeg" {
			b.WriteString(a.Label + "  sha256 " + a.SHA256 + "  " + a.URL + "\n")
		}
	}
	b.WriteString("ONNX Runtime " + analytics.ORTVersion + " (MIT): licence in onnxruntime-" + analytics.ORTVersion + "/LICENSE\n")
	b.WriteString("ffmpeg " + strings.TrimPrefix(analytics.FFmpegBuild, "ffmpeg-") + " LGPL v3 build from https://github.com/BtbN/FFmpeg-Builds/releases/tag/" + analytics.FFmpegRelease +
		" - licence in " + analytics.FFmpegBuild + "-lgpl/LICENSE.txt; source: https://github.com/FFmpeg/FFmpeg (release/" + analytics.FFmpegBranch + ") and https://github.com/BtbN/FFmpeg-Builds\n")
	b.WriteString("ffmpeg runs as a separate, unmodified program; it is not linked into ctrl32-telemetry.\n")
	return b.String()
}

func sourcesModels(by string, as []analytics.Artifact) string {
	var b strings.Builder
	b.WriteString("Object counting models (installed by " + by + "; not part of ctrl32 telemetry)\n")
	for _, a := range as {
		if a.Name == "model_nano" || a.Name == "model_tiny" {
			b.WriteString(filepath.Base(a.URL) + "  sha256 " + a.SHA256 + "  " + a.URL + "\n")
		}
	}
	b.WriteString("Licence: Apache-2.0 (LICENSE-YOLOX.txt), Megvii-BaseDetection/YOLOX " + analytics.YOLOXRelease + "\n")
	return b.String()
}

// ---- downloads --------------------------------------------------------------------------------------------------

// HTTPFetcher downloads over HTTPS only: the address must be on one of the official hosts and every redirect must stay
// on HTTPS (GitHub serves release files from its object storage). The pin, not the host, decides what is kept.
type HTTPFetcher struct {
	Client *http.Client
	// Hosts overrides analytics.OfficialHosts (tests)
	Hosts []string
	// AllowHTTP permits plain HTTP (tests against a local server only)
	AllowHTTP bool
}

// maxDownload caps one download (the largest part is about 40 MB).
const maxDownload = 512 << 20

func (h HTTPFetcher) Fetch(ctx context.Context, raw string, w io.Writer, progress func(n, total int64)) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	hosts := h.Hosts
	if hosts == nil {
		hosts = analytics.OfficialHosts
	}
	if (u.Scheme != "https" && !(h.AllowHTTP && u.Scheme == "http")) || !slices.Contains(hosts, u.Hostname()) {
		return fmt.Errorf("refused: %s is not an HTTPS address of an official source", raw)
	}
	c := h.Client
	if c == nil {
		c = &http.Client{Timeout: 15 * time.Minute}
	}
	cc := *c
	cc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "https" && !(h.AllowHTTP && req.URL.Scheme == "http") {
			return fmt.Errorf("refused a redirect to %s (not HTTPS)", req.URL.Redacted())
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ctrl32-telemetry-agent")
	resp, err := cc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	total := resp.ContentLength
	if total < 0 {
		total = 0
	}
	var n int64
	buf := make([]byte, 256<<10)
	last := time.Time{}
	for {
		k, rerr := resp.Body.Read(buf)
		if k > 0 {
			if n+int64(k) > maxDownload {
				return errors.New("the file is larger than expected")
			}
			if _, err := w.Write(buf[:k]); err != nil {
				return err
			}
			n += int64(k)
			if progress != nil && time.Since(last) > 200*time.Millisecond {
				progress(n, total)
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if total > 0 && n != total {
		return fmt.Errorf("the download stopped after %d of %d bytes", n, total)
	}
	if progress != nil {
		progress(n, total)
	}
	return nil
}

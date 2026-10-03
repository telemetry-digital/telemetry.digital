package analytics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics/ort"
)

// CheckPaths are the [analytics] settings the self-check examines (the same names as in config.toml).
type CheckPaths struct {
	FFmpeg      string `json:"ffmpeg"`
	ONNXRuntime string `json:"onnxruntime"`
	ModelNano   string `json:"model_nano"`
	ModelTiny   string `json:"model_tiny"`
	NanoSHA256  string `json:"nano_sha256,omitempty"`
	TinySHA256  string `json:"tiny_sha256,omitempty"`
}

// Component states of the self-check.
const (
	StateOK            = "ok"             // found and working
	StateWarning       = "warning"        // works, but something should be fixed (e.g. a GPL build of ffmpeg)
	StateError         = "error"          // configured but not working
	StateMissing       = "missing"        // not installed / not configured
	StateNotConfigured = "not_configured" // optional part left out (one of the two models)
)

// Hint codes tell the user interface which advice to show (translated there); Detail carries the technical reason.
const (
	HintInstall     = "install"      // run the installer with --analytics / -Analytics
	HintGPL         = "gpl_build"    // ffmpeg is a GPL or non-free build; ADR-0022 asks for an LGPL build
	HintSHA         = "sha_mismatch" // the model file is not the pinned one
	HintPlatform    = "platform"     // 32-bit ARM or another platform without ONNX Runtime
	HintORTVersion  = "ort_version"  // ONNX Runtime older than 1.17
	HintModelBroken = "model_broken" // the model does not load or run
)

// Component is one part of the detector installation.
type Component struct {
	Name    string `json:"name"` // ffmpeg | onnxruntime | model_nano | model_tiny
	State   string `json:"state"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	Licence string `json:"licence,omitempty"` // ffmpeg: LGPL | GPL | non-free
	Detail  string `json:"detail,omitempty"`  // English technical reason
	Hint    string `json:"hint,omitempty"`    // advice code (see Hint…)
	Millis  int64  `json:"ms,omitempty"`      // models: one inference on an empty picture
}

// CheckResult is the self-check of the detector installation ("Analytics: ffmpeg found (version), ONNX Runtime found
// (version), model found").
type CheckResult struct {
	Ready      bool        `json:"ready"` // cameras can count: ffmpeg, ONNX Runtime and at least one model work
	Platform   string      `json:"platform"`
	Components []Component `json:"components"`
	Checked    time.Time   `json:"checked"`
}

var (
	reFFVersion = regexp.MustCompile(`(?m)^ffmpeg version (\S+)`)
	reFFConfig  = regexp.MustCompile(`(?m)^\s*configuration:(.*)$`)
)

// FFmpegLicence tells the licence variant of an ffmpeg build from the "configuration:" line of `ffmpeg -version`.
func FFmpegLicence(versionOutput string) string {
	cfg := ""
	if m := reFFConfig.FindStringSubmatch(versionOutput); m != nil {
		cfg = m[1]
	}
	switch {
	case strings.Contains(cfg, "--enable-nonfree"):
		return "non-free"
	case strings.Contains(cfg, "--enable-gpl"):
		return "GPL"
	}
	return "LGPL"
}

// SelfCheck examines the configured ffmpeg, ONNX Runtime and models. It loads ONNX Runtime into the calling process
// and runs each model once, so the server calls it in a separate process (`analytics-worker -selfcheck`).
func SelfCheck(ctx context.Context, p CheckPaths) CheckResult {
	res := CheckResult{Platform: runtime.GOOS + "/" + runtime.GOARCH, Checked: time.Now().UTC()}
	ff := checkFFmpeg(ctx, p.FFmpeg)
	rt := Component{Name: "onnxruntime", Path: p.ONNXRuntime}
	ortOK := false
	switch {
	case !ort.Supported:
		rt.State, rt.Hint, rt.Detail = StateError, HintPlatform, "ONNX Runtime is not available for "+res.Platform+" (a 64-bit Linux, Windows or macOS server is needed)"
	case p.ONNXRuntime == "":
		rt.State, rt.Hint, rt.Detail = StateMissing, HintInstall, "analytics.onnxruntime is not set in config.toml"
	case !fileExists(p.ONNXRuntime):
		rt.State, rt.Hint, rt.Detail = StateMissing, HintInstall, "file not found"
	default:
		v, err := ort.Load(p.ONNXRuntime)
		rt.Version = v
		if err != nil {
			rt.State, rt.Detail, rt.Hint = StateError, err.Error(), HintInstall
			if strings.Contains(err.Error(), "C API version") {
				rt.Hint = HintORTVersion
			}
		} else {
			rt.State, ortOK = StateOK, true
		}
	}
	nano := checkModel(Component{Name: "model_nano", Path: p.ModelNano}, p.NanoSHA256, ortOK)
	tiny := checkModel(Component{Name: "model_tiny", Path: p.ModelTiny}, p.TinySHA256, ortOK)
	if nano.State == StateNotConfigured && tiny.State == StateNotConfigured {
		nano.State, nano.Hint, nano.Detail = StateMissing, HintInstall, "no model configured (analytics.model_nano or analytics.model_tiny)"
	}
	res.Components = []Component{ff, rt, nano, tiny}
	ffOK := ff.State == StateOK || ff.State == StateWarning
	res.Ready = ffOK && ortOK && (nano.State == StateOK || tiny.State == StateOK)
	return res
}

func checkFFmpeg(ctx context.Context, configured string) Component {
	c := Component{Name: "ffmpeg"}
	exe := ffmpegPath(configured)
	path, err := exec.LookPath(exe)
	if err != nil {
		c.Path, c.State, c.Hint = exe, StateMissing, HintInstall
		c.Detail = "ffmpeg not found (analytics.ffmpeg in config.toml, next to the binary or on PATH)"
		return c
	}
	c.Path = path
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, path, "-hide_banner", "-version").CombinedOutput()
	if err != nil {
		// -hide_banner hides the version line on some builds; ask again without it
		out, err = exec.CommandContext(cctx, path, "-version").CombinedOutput()
	}
	if err != nil {
		c.State, c.Hint, c.Detail = StateError, HintInstall, "ffmpeg does not run: "+firstLine(string(out), err)
		return c
	}
	s := string(out)
	if !reFFVersion.MatchString(s) {
		if o2, e2 := exec.CommandContext(cctx, path, "-version").CombinedOutput(); e2 == nil {
			s = string(o2)
		}
	}
	if m := reFFVersion.FindStringSubmatch(s); m != nil {
		c.Version = m[1]
	}
	c.Licence = FFmpegLicence(s)
	c.State = StateOK
	if c.Licence != "LGPL" {
		c.State, c.Hint = StateWarning, HintGPL
		c.Detail = "a " + c.Licence + " build of ffmpeg; ADR-0022 asks for an LGPL build (an unmodified separate program)"
	}
	return c
}

func checkModel(c Component, pin string, ortOK bool) Component {
	if c.Path == "" {
		c.State = StateNotConfigured
		return c
	}
	f, err := os.Open(c.Path)
	if err != nil {
		c.State, c.Hint, c.Detail = StateMissing, HintInstall, "file not found"
		return c
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	f.Close()
	if err != nil {
		c.State, c.Detail = StateError, err.Error()
		return c
	}
	sum := hex.EncodeToString(h.Sum(nil))
	c.Version = "sha256 " + sum[:12]
	if pin != "" && !strings.EqualFold(pin, sum) {
		c.State, c.Hint, c.Detail = StateError, HintSHA, "SHA-256 differs from the configured pin (a different or damaged file)"
		return c
	}
	if !ortOK {
		// found; whether it runs is unknown without ONNX Runtime
		c.State, c.Detail = StateWarning, "found; not tried without ONNX Runtime"
		return c
	}
	t0 := time.Now()
	if err := tryModel(c.Path); err != nil {
		c.State, c.Hint, c.Detail = StateError, HintModelBroken, err.Error()
		return c
	}
	c.Millis = time.Since(t0).Milliseconds()
	c.State = StateOK
	return c
}

// tryModel loads the model and runs it once on an empty picture (checks the whole inference path).
func tryModel(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s, err := ort.NewSession(b, 1)
	if err != nil {
		return err
	}
	defer s.Close()
	out, shape, err := s.Run(make([]float32, 3*InputSize*InputSize), []int64{1, 3, InputSize, InputSize})
	if err != nil {
		return err
	}
	if len(out) == 0 || len(shape) != 3 || shape[2] < int64(5+len(Classes)) {
		return fmt.Errorf("unexpected model output %v (a YOLOX COCO model is needed)", shape)
	}
	return nil
}

func firstLine(s string, err error) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if s == "" && err != nil {
		return err.Error()
	}
	return s
}

// ErrNotReady is returned by the command line self-check when cameras cannot count.
var ErrNotReady = errors.New("object counting on cameras is not ready")

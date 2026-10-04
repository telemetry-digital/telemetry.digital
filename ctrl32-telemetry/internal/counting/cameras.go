package counting

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/video"
)

// CameraConfig is the object counting of a camera as stored (core.counting_camera.config): the analytics
// configuration plus where each line counts and where each speed section stores its values.
type CameraConfig struct {
	Lines    []CameraLine    `json:"lines"`
	Sections []CameraSection `json:"sections,omitempty"`
	Anchor   string          `json:"anchor,omitempty"`
	Margin   float64         `json:"margin,omitempty"`
}

// CameraLine is a counting line with the entrance it counts for (empty = only a speed line).
type CameraLine struct {
	analytics.Line
	EntranceID string `json:"entrance_id,omitempty"`
}

// CameraSection is a speed section with its asset (created by the API) and the speed limit.
type CameraSection struct {
	analytics.Section
	AreaID   string  `json:"area_id"`
	AssetID  string  `json:"asset_id,omitempty"`
	LimitKmh float64 `json:"limit_kmh,omitempty"`
}

// Analytics is the configuration the worker needs.
func (c CameraConfig) Analytics() analytics.Config {
	a := analytics.Config{Anchor: c.Anchor, Margin: c.Margin}
	for _, l := range c.Lines {
		a.Lines = append(a.Lines, l.Line)
	}
	for _, s := range c.Sections {
		a.Sections = append(a.Sections, s.Section)
	}
	return a
}

// CameraStatus is what the user interface shows about a camera's counting.
type CameraStatus struct {
	State   string           `json:"state"` // off, not_installed, waiting, starting, running, error, limit
	Error   string           `json:"error,omitempty"`
	Since   time.Time        `json:"since,omitzero"`
	Runtime string           `json:"runtime,omitempty"`
	Model   string           `json:"model,omitempty"`
	FPS     float64          `json:"fps,omitempty"`
	Stream  string           `json:"stream,omitempty"` // codec and size analysed
	Stats   *analytics.Stats `json:"stats,omitempty"`
	Updated time.Time        `json:"updated,omitzero"`
}

type camSpec struct {
	ID, OrgID, Name, Model string
	FPS, MinScore          float64
	Cfg                    CameraConfig
	hash                   string
}

type camWorker struct {
	spec   camSpec
	cancel context.CancelFunc
}

// Status of a camera's object counting.
func (e *Engine) Status(cameraID string) CameraStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	if st := e.status[cameraID]; st != nil {
		return *st
	}
	return CameraStatus{State: "off"}
}

// Installed reports whether the detector can run (ONNX Runtime and a model configured).
func (e *Engine) Installed() (bool, string) {
	cfg := e.conf()
	if e.installing.Load() {
		return false, "the detector is being installed on this server"
	}
	if cfg.ONNXRuntime == "" {
		return false, "object counting on cameras is not installed on this server (analytics.onnxruntime in config.toml)"
	}
	if cfg.ModelNano == "" && cfg.ModelTiny == "" {
		return false, "no detector model configured (analytics.model_nano or analytics.model_tiny in config.toml)"
	}
	return true, ""
}

// conf is the current [analytics] configuration (SetAnalytics replaces it after an installation from the user interface).
func (e *Engine) conf() config.AnalyticsConfig {
	e.cfgMu.RLock()
	defer e.cfgMu.RUnlock()
	return e.cfg
}

// SetInstalling stops the camera workers while the server agent replaces the detector parts (a running ffmpeg or a
// loaded ONNX Runtime would keep the old files busy on Windows); SetAnalytics ends it.
func (e *Engine) SetInstalling(on bool) {
	e.installing.Store(on)
	if on {
		e.stopAll()
	}
	e.kick()
}

// SetAnalytics applies new detector paths written by the server agent ([analytics] in config.toml): the paths and pins
// change, threads and max_cameras stay. The workers restart with the new parts and the cached self-check is dropped.
func (e *Engine) SetAnalytics(ffmpeg, ort, nano, tiny, nanoSum, tinySum string) {
	e.cfgMu.Lock()
	e.cfg.FFmpeg, e.cfg.ONNXRuntime, e.cfg.ModelNano, e.cfg.ModelTiny, e.cfg.NanoSHA256, e.cfg.TinySHA256 = ffmpeg, ort, nano, tiny, nanoSum, tinySum
	e.cfgMu.Unlock()
	e.checkMu.Lock()
	e.check = nil
	e.checkMu.Unlock()
	e.installing.Store(false)
	e.stopAll()
	e.kick()
}

func (e *Engine) kick() {
	select {
	case e.reload <- struct{}{}:
	default:
	}
}

// selfCheckTTL is how long a self-check result is reused (the pages ask on every load).
const selfCheckTTL = 5 * time.Minute

// SelfCheck examines the detector installation in a separate process (`analytics-worker -selfcheck`), so ONNX Runtime
// is never loaded into the server; the result is cached for selfCheckTTL unless refresh is set.
func (e *Engine) SelfCheck(ctx context.Context, refresh bool) analytics.CheckResult {
	e.checkMu.Lock()
	defer e.checkMu.Unlock()
	if !refresh && e.check != nil && time.Since(e.checkAt) < selfCheckTTL {
		return *e.check
	}
	cfg := e.conf()
	p := analytics.CheckPaths{FFmpeg: cfg.FFmpeg, ONNXRuntime: cfg.ONNXRuntime, ModelNano: cfg.ModelNano, ModelTiny: cfg.ModelTiny,
		NanoSHA256: cfg.NanoSHA256, TinySHA256: cfg.TinySHA256}
	res, err := runSelfCheck(ctx, p)
	if err != nil {
		res = analytics.CheckResult{Checked: time.Now().UTC(), Components: []analytics.Component{{Name: "worker", State: analytics.StateError, Detail: err.Error()}}}
	}
	e.check, e.checkAt = &res, time.Now()
	return res
}

func runSelfCheck(ctx context.Context, p analytics.CheckPaths) (analytics.CheckResult, error) {
	var res analytics.CheckResult
	exe, err := os.Executable()
	if err != nil {
		return res, err
	}
	raw, _ := json.Marshal(p)
	cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, exe, "analytics-worker", "-selfcheck", "-json", "-paths", string(raw))
	var stderr strings.Builder
	cmd.Stderr = &capped{b: &stderr, n: 4096}
	out, err := cmd.Output() // exit code 1 = not ready: still a valid answer on stdout
	if jerr := json.Unmarshal(out, &res); jerr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" && err != nil {
			msg = err.Error()
		} else if msg == "" {
			msg = jerr.Error()
		}
		return res, fmt.Errorf("analytics self-check: %s", msg)
	}
	return res, nil
}

func (e *Engine) setStatus(id string, f func(*CameraStatus)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.status[id]
	if st == nil {
		st = &CameraStatus{}
		e.status[id] = st
	}
	f(st)
	st.Updated = time.Now()
}

func (e *Engine) maxCameras() int {
	if n := e.conf().MaxCameras; n > 0 {
		return n
	}
	return 4
}

// reconcile starts and stops camera workers to match the configuration.
func (e *Engine) reconcile(ctx context.Context) {
	if e.vid == nil {
		return
	}
	rows, err := e.pool.Query(ctx, `SELECT cc.camera_id::text, cc.org_id::text, c.name, cc.model, cc.fps::float8, cc.min_score::float8, cc.config::text
		FROM core.counting_camera cc JOIN core.camera c ON c.id = cc.camera_id
		WHERE cc.enabled AND c.enabled ORDER BY cc.updated_at`)
	if err != nil {
		return
	}
	want := map[string]camSpec{}
	var order []string
	installing := e.installing.Load()
	for rows.Next() {
		var s camSpec
		var raw string
		if rows.Scan(&s.ID, &s.OrgID, &s.Name, &s.Model, &s.FPS, &s.MinScore, &raw) != nil {
			continue
		}
		if json.Unmarshal([]byte(raw), &s.Cfg) != nil {
			continue
		}
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%v|%v|%s|%s", s.Model, s.FPS, s.MinScore, raw, s.Name)))
		s.hash = hex.EncodeToString(sum[:8])
		if installing {
			continue // the parts are being replaced: no worker until the installation ends (SetAnalytics)
		}
		want[s.ID] = s
		order = append(order, s.ID)
	}
	rows.Close()
	e.mu.Lock()
	for id, w := range e.cams {
		if s, ok := want[id]; !ok || s.hash != w.spec.hash {
			w.cancel()
			delete(e.cams, id)
			if !ok {
				delete(e.status, id)
			}
		}
	}
	running := len(e.cams)
	var start []camSpec
	for _, id := range order {
		if _, ok := e.cams[id]; ok {
			continue
		}
		if running >= e.maxCameras() {
			st := e.status[id]
			if st == nil {
				st = &CameraStatus{}
				e.status[id] = st
			}
			st.State, st.Error = "limit", fmt.Sprintf("at most %d cameras are analysed at the same time on this server (analytics.max_cameras)", e.maxCameras())
			continue
		}
		running++
		start = append(start, want[id])
	}
	for _, s := range start {
		c, cancel := context.WithCancel(ctx)
		e.cams[s.ID] = &camWorker{spec: s, cancel: cancel}
		go e.superviseCamera(c, s)
	}
	e.mu.Unlock()
}

func (e *Engine) stopAll() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, w := range e.cams {
		w.cancel()
		delete(e.cams, id)
	}
}

func (e *Engine) superviseCamera(ctx context.Context, s camSpec) {
	backoff := 5 * time.Second
	for ctx.Err() == nil {
		began := time.Now()
		err := e.runCamera(ctx, s)
		if ctx.Err() != nil {
			return
		}
		state := "error"
		if errors.Is(err, errNotInstalled) {
			state = "not_installed"
		}
		msg := "stopped"
		if err != nil {
			msg = err.Error()
		}
		e.setStatus(s.ID, func(st *CameraStatus) { st.State, st.Error = state, msg })
		if time.Since(began) > 2*time.Minute {
			backoff = 5 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 2*time.Minute)
	}
}

var errNotInstalled = errors.New("not installed")

func (e *Engine) modelOf(name string) (string, string) {
	cfg := e.conf()
	if name == "tiny" && cfg.ModelTiny != "" {
		return cfg.ModelTiny, cfg.TinySHA256
	}
	if cfg.ModelNano != "" {
		return cfg.ModelNano, cfg.NanoSHA256
	}
	return cfg.ModelTiny, cfg.TinySHA256
}

// runCamera feeds one worker process until it or the stream fails.
func (e *Engine) runCamera(ctx context.Context, s camSpec) error {
	if ok, why := e.Installed(); !ok {
		return fmt.Errorf("%w: %s", errNotInstalled, why)
	}
	model, sum := e.modelOf(s.Model)
	e.setStatus(s.ID, func(st *CameraStatus) {
		st.State, st.Error, st.Model, st.FPS, st.Stats = "waiting", "waiting for the camera picture", s.Model, s.FPS, nil
	})
	sub, err := e.vid.AnalyticsFeed(s.ID)
	if err != nil {
		return err
	}
	defer sub.Close()
	// the first picture tells codec and size
	var first video.Packet
	timeout := time.NewTimer(60 * time.Second)
	defer timeout.Stop()
	for first.Track == nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return errors.New("no picture from the camera within 60 s")
		case p, ok := <-sub.C:
			if !ok {
				return errors.New("the camera stream ended")
			}
			if p.Track != nil && p.Sample != nil && p.Sample.Audio == nil && (p.Sample.Key || p.Track.Codec == video.MJPEG) {
				first = p
			}
		}
	}
	tr := first.Track
	codec := tr.Codec
	if codec != video.H264 && codec != video.H265 && codec != video.MJPEG {
		return fmt.Errorf("unsupported codec %s", codec)
	}
	cfg := e.conf()
	wc := analytics.WorkerConfig{Camera: s.Name, Codec: codec, Width: tr.Width, Height: tr.Height, FPS: s.FPS, FFmpeg: cfg.FFmpeg,
		ORTLib: cfg.ONNXRuntime, Model: model, ModelSHA256: sum, Threads: cfg.Threads, MinScore: s.MinScore, Count: s.Cfg.Analytics(), Boxes: true, StatsEvery: 10}
	raw, _ := json.Marshal(wc)
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(cctx, exe, "analytics-worker", "-config", string(raw))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &capped{b: &stderr, n: 4096}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("analytics worker: %w", err)
	}
	e.setStatus(s.ID, func(st *CameraStatus) {
		st.State, st.Error, st.Since = "starting", "", time.Now()
		st.Stream = fmt.Sprintf("%s %d×%d", strings.ToUpper(codec), tr.Width, tr.Height)
	})
	workerErr := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		var last string
		for sc.Scan() {
			var ev analytics.Event
			if json.Unmarshal(sc.Bytes(), &ev) != nil {
				continue
			}
			if ev.Type == "error" {
				last = ev.Message
			}
			e.handleEvent(s, ev)
		}
		err := cmd.Wait()
		if last != "" {
			err = errors.New(last)
		} else if err != nil && stderr.Len() > 0 {
			err = fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
		}
		if err == nil {
			err = errors.New("the analytics worker ended")
		}
		workerErr <- err
	}()
	// wall-clock time of each frame from its decoding timestamps (steady even when packets arrive in bursts)
	var baseT time.Time
	var baseDTS int64
	write := func(p video.Packet) error {
		smp := p.Sample
		if baseT.IsZero() {
			baseT, baseDTS = smp.Time, smp.DTS
			if baseT.IsZero() {
				baseT = time.Now()
			}
		}
		at := baseT.Add(time.Duration(float64(smp.DTS-baseDTS) / video.ClockRate * float64(time.Second)))
		if d := time.Since(at); d > 10*time.Second || d < -10*time.Second {
			baseT, baseDTS, at = time.Now(), smp.DTS, time.Now() // the camera's clock jumped: start over
		}
		return analytics.WriteFrame(stdin, at, video.AnnexB(p.Track, smp))
	}
	if err := write(first); err != nil {
		return <-workerErr
	}
	for {
		select {
		case <-ctx.Done():
			_ = stdin.Close()
			return ctx.Err()
		case err := <-workerErr:
			return err
		case p, ok := <-sub.C:
			if !ok {
				_ = stdin.Close()
				return errors.New("the camera stream ended")
			}
			if p.Track == nil || p.Sample == nil || p.Sample.Audio != nil {
				continue
			}
			if p.Track != tr && !p.Track.SameParams(tr) {
				_ = stdin.Close()
				return errors.New("the camera stream changed (codec or size): restarting")
			}
			if err := write(p); err != nil {
				return <-workerErr
			}
		}
	}
}

func (e *Engine) handleEvent(s camSpec, ev analytics.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	switch ev.Type {
	case "ready":
		e.setStatus(s.ID, func(st *CameraStatus) { st.State, st.Error, st.Runtime = "running", "", ev.Version })
	case "stats":
		e.setStatus(s.ID, func(st *CameraStatus) { st.State, st.Error, st.Stats = "running", "", ev.Stats })
	case "error":
		e.setStatus(s.ID, func(st *CameraStatus) { st.State, st.Error = "error", ev.Message })
	case "cross":
		c := ev.Cross
		if c == nil {
			return
		}
		for _, l := range s.Cfg.Lines {
			if l.ID == c.Line && l.EntranceID != "" {
				if err := e.Record(ctx, l.EntranceID, GroupsOf(c.Class), c.Dir, 1, c.At); err != nil {
					e.log.Warn("crossing not recorded", "camera", s.Name, "line", l.Name, "err", err)
				}
			}
		}
	case "speed":
		sp := ev.Speed
		if sp == nil {
			return
		}
		for _, sec := range s.Cfg.Sections {
			if sec.ID == sp.Section && sec.AssetID != "" {
				if err := e.RecordSpeed(ctx, s.OrgID, sec.AssetID, sp.Kmh, sp.Class, sp.Dir, sp.At, sec.LimitKmh); err != nil {
					e.log.Warn("speed not recorded", "camera", s.Name, "section", sec.Name, "err", err)
				}
			}
		}
	}
}

type capped struct {
	b *strings.Builder
	n int
}

func (c *capped) Write(p []byte) (int, error) {
	if c.n > 0 {
		k := min(len(p), c.n)
		c.b.Write(p[:k])
		c.n -= k
	}
	return len(p), nil
}

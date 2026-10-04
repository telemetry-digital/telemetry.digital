package agent

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics/setup"
)

// Detector of object counting (ADR-0022, 0.75.0): the button "Install the detector" of the user interface. The agent
// downloads exactly the parts the installers do (internal/analytics/pins.go: ONNX Runtime, an LGPL build of ffmpeg,
// the YOLOX models), from their official releases over HTTPS, keeps a file only when its SHA-256 matches the pin,
// refuses a GPL build of ffmpeg, swaps the parts in atomically (internal/analytics/setup) and writes the paths into
// [analytics] of config.toml (threads, max_cameras and everything else stay). The server then uses the new
// configuration without a restart and checks the installation again (analytics-worker -selfcheck).
//
//	GET  /analytics           state: platform, last installation, running or last job
//	POST /analytics/install   {force}: start an installation (202); force downloads every part again (repair)
//	POST /analytics/remove    remove the parts and their paths in [analytics]
//
// The request carries no address, path or hash: everything comes from the pins compiled into this binary.

// AnalyticsJob is one installation or removal.
type AnalyticsJob struct {
	ID         string         `json:"id"`
	Action     string         `json:"action"` // install, remove
	Force      bool           `json:"force,omitempty"`
	State      string         `json:"state"` // running, done, failed
	Progress   setup.Progress `json:"progress"`
	Error      string         `json:"error,omitempty"`
	Installed  []string       `json:"installed,omitempty"` // parts downloaded
	Kept       []string       `json:"kept,omitempty"`      // parts that were installed already
	Config     *setup.Paths   `json:"config,omitempty"`    // [analytics] after the job (nil after a removal)
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt time.Time      `json:"finished_at,omitzero"`
}

// AnalyticsInstalled is the last installation done by the agent (analytics.json).
type AnalyticsInstalled struct {
	At            time.Time   `json:"at"`
	ORTVersion    string      `json:"ort_version"`
	FFmpegBuild   string      `json:"ffmpeg_build"`
	FFmpegRelease string      `json:"ffmpeg_release"`
	YOLOXRelease  string      `json:"yolox_release"`
	Paths         setup.Paths `json:"paths"`
}

// AnalyticsState is what GET /analytics answers.
type AnalyticsState struct {
	Supported bool                `json:"supported"`
	Platform  string              `json:"platform"`
	Simulated bool                `json:"simulated,omitempty"`
	Pins      map[string]string   `json:"pins"`
	Installed *AnalyticsInstalled `json:"installed,omitempty"`
	Job       *AnalyticsJob       `json:"job,omitempty"`
}

// AnalyticsRequest starts an installation.
type AnalyticsRequest struct {
	Force bool `json:"force"`
}

type analyticsRunner struct {
	mu  sync.Mutex
	job *AnalyticsJob
	// sim: the stand-in downloads of the simulation (nil = the pinned parts from the Internet)
	sim *setup.SimFetcher
}

func (a *Server) analyticsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /analytics", a.hAnalytics)
	mux.HandleFunc("POST /analytics/install", a.hAnalyticsInstall)
	mux.HandleFunc("POST /analytics/remove", a.hAnalyticsRemove)
}

func analyticsPlatform() string { return runtime.GOOS + "/" + runtime.GOARCH }

// analyticsLayout: the places the installers use (setup.Layout); the simulation keeps everything in its directory.
func (a *Server) analyticsLayout() setup.Layout {
	l := setup.Layout{Platform: analyticsPlatform()}
	switch {
	case a.simulate:
		l.AnalyticsDir, l.ModelsDir = filepath.Join(optDir, "analytics"), filepath.Join(optDir, "models")
	case isWindows():
		// install.ps1: <install dir>\analytics next to the executable, <data dir>\models next to config.toml
		l.AnalyticsDir, l.ModelsDir = filepath.Join(filepath.Dir(binPath), "analytics"), filepath.Join(filepath.Dir(a.cfgPath), "models")
	default:
		l.AnalyticsDir, l.ModelsDir = filepath.Join(optDir, "analytics"), filepath.Join(libDir, "models")
	}
	return l
}

// analyticsStateFile records the last installation, next to config.toml (the simulation: in its directory).
func (a *Server) analyticsStateFile() string {
	if a.simulate {
		return filepath.Join(etcDir, "analytics.json")
	}
	return filepath.Join(filepath.Dir(a.cfgPath), "analytics.json")
}

func (a *Server) analyticsArtifacts() []analytics.Artifact {
	if a.an.sim != nil {
		var out []analytics.Artifact
		for _, f := range a.an.sim.Files {
			out = append(out, f.Artifact)
		}
		return out
	}
	return analytics.PinnedArtifacts(analyticsPlatform())
}

func (a *Server) analyticsState() AnalyticsState {
	st := AnalyticsState{Platform: analyticsPlatform(), Simulated: a.an.sim != nil, Supported: len(a.analyticsArtifacts()) > 0,
		Pins: map[string]string{"onnxruntime": analytics.ORTVersion, "ffmpeg": analytics.FFmpegBuild, "ffmpeg_release": analytics.FFmpegRelease, "yolox": analytics.YOLOXRelease}}
	var in AnalyticsInstalled
	if loadJSON(a.analyticsStateFile(), &in) == nil {
		st.Installed = &in
	}
	a.an.mu.Lock()
	if a.an.job != nil {
		j := *a.an.job
		st.Job = &j
	}
	a.an.mu.Unlock()
	return st
}

func (a *Server) hAnalytics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, a.analyticsState())
}

func (a *Server) hAnalyticsInstall(w http.ResponseWriter, r *http.Request) {
	var in AnalyticsRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request: "+err.Error())
		return
	}
	a.startAnalyticsJob(w, "install", in.Force)
}

func (a *Server) hAnalyticsRemove(w http.ResponseWriter, r *http.Request) {
	a.startAnalyticsJob(w, "remove", false)
}

func (a *Server) startAnalyticsJob(w http.ResponseWriter, action string, force bool) {
	if action == "install" && len(a.analyticsArtifacts()) == 0 {
		writeErr(w, 400, "the detector cannot be installed on "+analyticsPlatform()+" (Linux amd64 or arm64, Windows amd64)")
		return
	}
	a.an.mu.Lock()
	if a.an.job != nil && a.an.job.State == "running" {
		a.an.mu.Unlock()
		writeJSON(w, 409, map[string]string{"error": "an installation or removal of the detector is already running", "code": "busy"})
		return
	}
	job := &AnalyticsJob{ID: randomID()[:12], Action: action, Force: force, State: "running", StartedAt: time.Now().UTC(), Progress: setup.Progress{Step: "prepare"}}
	a.an.job = job
	a.an.mu.Unlock()
	go a.runAnalyticsJob(job)
	writeJSON(w, 202, a.analyticsState())
}

// update changes the running job under the lock (the state handler copies it).
func (a *Server) analyticsUpdate(f func(j *AnalyticsJob)) {
	a.an.mu.Lock()
	defer a.an.mu.Unlock()
	if a.an.job != nil {
		f(a.an.job)
	}
}

func (a *Server) runAnalyticsJob(job *AnalyticsJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	l := a.analyticsLayout()
	var err error
	var cfg *setup.Paths
	var res setup.Result
	if job.Action == "remove" {
		err = setup.Remove(l)
		if err == nil {
			err = a.writeAnalyticsConfig(nil)
			_ = os.Remove(a.analyticsStateFile())
		}
	} else {
		err = a.ensureXZ(ctx)
		if err == nil {
			res, err = setup.Install(ctx, a.analyticsOptions(l, job.Force), func(p setup.Progress) {
				a.analyticsUpdate(func(j *AnalyticsJob) { j.Progress = p })
			})
		}
		if err == nil {
			cfg = &res.Paths
			err = a.writeAnalyticsConfig(cfg)
		}
		if err == nil {
			_ = saveJSON(a.analyticsStateFile(), AnalyticsInstalled{At: time.Now().UTC(), ORTVersion: analytics.ORTVersion, FFmpegBuild: analytics.FFmpegBuild,
				FFmpegRelease: analytics.FFmpegRelease, YOLOXRelease: analytics.YOLOXRelease, Paths: res.Paths}, 0o644)
		}
	}
	a.analyticsUpdate(func(j *AnalyticsJob) {
		j.FinishedAt = time.Now().UTC()
		if err != nil {
			j.State, j.Error = "failed", err.Error()
			return
		}
		j.State, j.Installed, j.Kept, j.Config = "done", res.Installed, res.Kept, cfg
		j.Progress = setup.Progress{Step: "done"}
	})
	if err != nil {
		a.log.Error("detector "+job.Action+" failed", "err", err)
	} else {
		a.log.Info("detector "+job.Action+" done", "installed", res.Installed, "kept", res.Kept, "simulated", a.simulate)
	}
}

func (a *Server) analyticsOptions(l setup.Layout, force bool) setup.Options {
	o := setup.Options{Layout: l, Artifacts: a.analyticsArtifacts(), Force: force, By: "the server agent (Install the detector)",
		FreeMB: func(dir string) (int64, bool) {
			d, ok := diskInfo(dir)
			return int64(d.FreeGB * 1000), ok
		}}
	if a.an.sim != nil {
		o.Fetch, o.FFmpegVersion = a.an.sim, setup.SimFFmpegVersion
		return o
	}
	if !isWindows() {
		// the models belong to the service user (as install.sh leaves them)
		if u, err := user.Lookup(appGroup); err == nil {
			uid, _ := strconv.Atoi(u.Uid)
			gid, _ := strconv.Atoi(u.Gid)
			o.Chown = func(p string) error { return os.Lchown(p, uid, gid) }
		}
	}
	return o
}

// ensureXZ installs xz-utils on Linux when the xz program is missing (slim images): the ffmpeg build is a .tar.xz.
func (a *Server) ensureXZ(ctx context.Context) error {
	if a.an.sim != nil || isWindows() {
		return nil
	}
	if _, err := exec.LookPath("xz"); err == nil {
		return nil
	}
	if _, err := exec.LookPath("apt-get"); err != nil {
		return errors.New("the xz program is needed to unpack ffmpeg: install the package xz-utils and try again")
	}
	a.log.Info("installing", "packages", "xz-utils")
	if out, err := a.run(ctx, 2*time.Minute, "apt-get", "update", "-qq"); err != nil {
		return errors.New("xz-utils could not be installed (apt-get update): " + firstLine(out))
	}
	if out, err := a.run(ctx, 4*time.Minute, "apt-get", "install", "-y", "-qq", "--no-install-recommends", "xz-utils"); err != nil {
		return errors.New("xz-utils could not be installed: " + firstLine(out))
	}
	return nil
}

// writeAnalyticsConfig puts the paths into [analytics] of config.toml (nil removes them), under the agent's lock.
func (a *Server) writeAnalyticsConfig(p *setup.Paths) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, err := os.ReadFile(a.cfgPath)
	if err != nil {
		return errors.New("cannot read config.toml: " + err.Error())
	}
	note := "Object counting on camera pictures - written by the server agent (Install the detector) on " + time.Now().UTC().Format(time.RFC3339) + " (docs/object-counting.md)."
	out := setup.UpdateConfig(b, p, note)
	if string(out) == string(b) {
		return nil
	}
	if err := a.writeConfig(out); err != nil {
		return errors.New("cannot write config.toml: " + err.Error())
	}
	return nil
}

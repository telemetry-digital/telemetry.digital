package agent

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
)

// The simulation installs the stand-in detector into its directory, writes [analytics] of config.toml (keeping the
// rest), runs only one job at a time, a second run downloads nothing, and a removal takes the paths out again.
func TestAnalyticsInstallSimulation(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte("profile = \"intranet\"\n\n[analytics]\nthreads = 2\nmax_cameras = 6\n\n[http]\nlisten = \":8080\"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	a := NewServer(filepath.Join(dir, "a.sock"), cfg, "test", "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	a.SetSimulation(dir)
	a.an.sim.Delay = 5 * time.Millisecond
	h := a.routes()
	call := func(method, path, body string) (int, AnalyticsState) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		var st AnalyticsState
		_ = json.Unmarshal(w.Body.Bytes(), &st)
		return w.Code, st
	}
	wait := func() AnalyticsState {
		t.Helper()
		for range 400 {
			_, st := call("GET", "/analytics", "")
			if st.Job != nil && st.Job.State != "running" {
				return st
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("job did not finish")
		return AnalyticsState{}
	}
	code, st := call("POST", "/analytics/install", `{"force":false}`)
	if code != 202 || st.Job == nil || st.Job.State != "running" || !st.Simulated || !st.Supported {
		t.Fatalf("start: %d %+v", code, st)
	}
	if code, _ := call("POST", "/analytics/install", `{}`); code != 409 {
		t.Errorf("second job while running: %d", code)
	}
	st = wait()
	if st.Job.State != "done" || len(st.Job.Installed) != 5 || st.Job.Config == nil || st.Installed == nil {
		t.Fatalf("install: %+v", st.Job)
	}
	var c struct {
		Analytics config.AnalyticsConfig `toml:"analytics"`
		HTTP      config.HTTPConfig      `toml:"http"`
	}
	if _, err := toml.DecodeFile(cfg, &c); err != nil {
		t.Fatal(err)
	}
	if c.Analytics.Threads != 2 || c.Analytics.MaxCameras != 6 || c.HTTP.Listen != ":8080" || !strings.HasPrefix(c.Analytics.ONNXRuntime, filepath.ToSlash(dir)) {
		t.Fatalf("config: %+v %+v", c.Analytics, c.HTTP)
	}
	if _, err := os.Stat(filepath.FromSlash(c.Analytics.FFmpeg)); err != nil {
		t.Errorf("ffmpeg not where [analytics] points: %v", err)
	}
	// idempotent re-run
	if code, _ := call("POST", "/analytics/install", `{}`); code != 202 {
		t.Fatal(code)
	}
	if st = wait(); st.Job.State != "done" || len(st.Job.Installed) != 0 || len(st.Job.Kept) != 5 || a.an.sim.Count != 5 {
		t.Fatalf("re-run: %+v downloads %d", st.Job, a.an.sim.Count)
	}
	// a field the agent does not know is refused (the request carries no paths or addresses)
	if code, _ := call("POST", "/analytics/install", `{"url":"https://example.com/x"}`); code != 400 {
		t.Errorf("unknown field: %d", code)
	}
	// removal
	if code, _ := call("POST", "/analytics/remove", ``); code != 202 {
		t.Fatal(code)
	}
	if st = wait(); st.Job.State != "done" || st.Job.Config != nil || st.Installed != nil {
		t.Fatalf("remove: %+v", st.Job)
	}
	c.Analytics = config.AnalyticsConfig{}
	if _, err := toml.DecodeFile(cfg, &c); err != nil {
		t.Fatal(err)
	}
	if c.Analytics.FFmpeg != "" || c.Analytics.ONNXRuntime != "" || c.Analytics.Threads != 2 {
		t.Errorf("config after removal: %+v", c.Analytics)
	}
	if _, err := os.Stat(filepath.Join(dir, "analytics")); err == nil {
		t.Error("parts left after removal")
	}
}

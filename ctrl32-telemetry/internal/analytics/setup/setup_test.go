package setup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
)

func testOptions(t *testing.T, platform string) (Options, *SimFetcher) {
	t.Helper()
	dir := t.TempDir()
	files := SimulatedArtifacts(platform)
	f := &SimFetcher{Files: files}
	var as []analytics.Artifact
	for _, s := range files {
		as = append(as, s.Artifact)
	}
	return Options{Layout: Layout{AnalyticsDir: filepath.Join(dir, "opt", "analytics"), ModelsDir: filepath.Join(dir, "lib", "models"), Platform: platform},
		Artifacts: as, Fetch: f, FFmpegVersion: SimFFmpegVersion}, f
}

func mustExist(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}

func leftovers(t *testing.T, l Layout) []string {
	t.Helper()
	var out []string
	for _, d := range []string{l.AnalyticsDir, l.ModelsDir} {
		for _, g := range []string{".install-*", "*.old-*", "*.download"} {
			m, _ := filepath.Glob(filepath.Join(d, g))
			out = append(out, m...)
		}
	}
	return out
}

// A fresh installation puts every part where [analytics] points, unpacks only what the installers keep, and a second
// run downloads nothing.
func TestInstallAndRerun(t *testing.T) {
	for _, platform := range []string{"linux/amd64", "windows/amd64"} {
		t.Run(platform, func(t *testing.T) {
			o, f := testOptions(t, platform)
			var steps []string
			res, err := Install(t.Context(), o, func(p Progress) {
				if len(steps) == 0 || steps[len(steps)-1] != p.Step {
					steps = append(steps, p.Step)
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Installed) != 5 || len(res.Kept) != 0 || f.Count != 5 {
				t.Fatalf("installed %v kept %v downloads %d", res.Installed, res.Kept, f.Count)
			}
			if got := strings.Join(steps, ","); got != "prepare,download,unpack,verify,unpack,verify,swap,done" {
				t.Errorf("steps %s", got)
			}
			p := res.Paths
			mustExist(t, p.FFmpeg, p.ONNXRuntime, p.ModelNano, p.ModelTiny, filepath.Join(o.Layout.ModelsDir, "LICENSE-YOLOX.txt"),
				filepath.Join(o.Layout.ModelsDir, "SOURCES.txt"), filepath.Join(o.Layout.AnalyticsDir, "SOURCES.txt"))
			if _, err := os.Stat(filepath.Join(o.Layout.ORTDir(), "include")); err == nil {
				t.Error("headers of ONNX Runtime were unpacked")
			}
			if _, err := os.Stat(filepath.Join(o.Layout.FFmpegDir(), "doc")); err == nil {
				t.Error("documentation of ffmpeg was unpacked")
			}
			if l := leftovers(t, o.Layout); len(l) != 0 {
				t.Errorf("leftovers %v", l)
			}
			if p.NanoSHA256 != analytics.YOLOXNanoSHA256 || !strings.HasSuffix(filepath.ToSlash(p.ONNXRuntime), "onnxruntime-"+analytics.ORTVersion+"/lib/"+filepath.Base(p.ONNXRuntime)) {
				t.Errorf("paths %+v", p)
			}
			// idempotent: everything is installed, nothing is downloaded
			res, err = Install(t.Context(), o, nil)
			if err != nil || len(res.Installed) != 0 || len(res.Kept) != 5 || f.Count != 5 {
				t.Fatalf("re-run: %v installed %v kept %v downloads %d", err, res.Installed, res.Kept, f.Count)
			}
			// a damaged model is downloaded again, the rest is kept
			if err := os.WriteFile(p.ModelTiny, []byte("damaged"), 0o644); err != nil {
				t.Fatal(err)
			}
			res, err = Install(t.Context(), o, nil)
			if err != nil || strings.Join(res.Installed, ",") != "model_tiny" || f.Count != 6 {
				t.Fatalf("repair: %v installed %v downloads %d", err, res.Installed, f.Count)
			}
			// force: everything again
			o.Force = true
			if res, err = Install(t.Context(), o, nil); err != nil || len(res.Installed) != 5 || f.Count != 11 {
				t.Fatalf("force: %v %v %d", err, res.Installed, f.Count)
			}
			if err := Remove(o.Layout); err != nil {
				t.Fatal(err)
			}
			for _, x := range []string{p.FFmpeg, p.ONNXRuntime, p.ModelNano, o.Layout.AnalyticsDir} {
				if _, err := os.Stat(x); err == nil {
					t.Errorf("%s left after Remove", x)
				}
			}
		})
	}
}

// installedState snapshots what is installed, to show a failed run changed nothing.
func installedState(t *testing.T, l Layout) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.Walk(l.AnalyticsDir, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() && filepath.Base(p) != "SOURCES.txt" {
			fmt.Fprintf(&b, "%s %s\n", p, fileSHA256(p))
		}
		return nil
	})
	for _, n := range []string{"yolox_nano.onnx", "yolox_tiny.onnx"} {
		fmt.Fprintf(&b, "%s %s\n", n, fileSHA256(filepath.Join(l.ModelsDir, n)))
	}
	return b.String()
}

type failingFetcher struct {
	inner  Fetcher
	failAt string
	err    error
}

func (f failingFetcher) Fetch(ctx context.Context, url string, w io.Writer, p func(n, total int64)) error {
	if strings.HasSuffix(url, f.failAt) {
		_, _ = w.Write([]byte("partial"))
		return f.err
	}
	return f.inner.Fetch(ctx, url, w, p)
}

// Network failure, a wrong hash, a full disk, a GPL build and a failed swap leave the installed parts as they were.
func TestInstallFailuresChangeNothing(t *testing.T) {
	o, f := testOptions(t, "linux/amd64")
	if _, err := Install(t.Context(), o, nil); err != nil {
		t.Fatal(err)
	}
	before := installedState(t, o.Layout)
	o.Force = true
	check := func(name string, o Options, want string) {
		t.Helper()
		_, err := Install(t.Context(), o, nil)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error %v, want %q", name, err, want)
		}
		if after := installedState(t, o.Layout); after != before {
			t.Errorf("%s: the installed parts changed:\n%s\nwas\n%s", name, after, before)
		}
		if l := leftovers(t, o.Layout); len(l) != 0 {
			t.Errorf("%s: leftovers %v", name, l)
		}
	}
	// network
	n := o
	n.Fetch = failingFetcher{inner: f, failAt: "yolox_tiny.onnx", err: errors.New("connection reset by peer")}
	check("network", n, "download of YOLOX tiny (simulation) failed")
	// a wrong pin
	w := o
	w.Artifacts = append([]analytics.Artifact{}, o.Artifacts...)
	w.Artifacts[3].SHA256 = strings.Repeat("0", 64)
	check("hash", w, "does not match the pin")
	// disk full while writing, and too little free space up front
	d := o
	d.Fetch = failingFetcher{inner: f, failAt: "ffmpeg", err: &os.PathError{Op: "write", Path: "x", Err: syscall.ENOSPC}}
	check("disk full", d, "the disk is full")
	s := o
	s.FreeMB = func(string) (int64, bool) { return 12, true }
	check("free space", s, "not enough free disk space: 12 MB free")
	// a GPL build of ffmpeg
	g := o
	g.FFmpegVersion = func(context.Context, string) (string, error) {
		return "ffmpeg version n8.1 configuration: --enable-gpl --enable-libx264", nil
	}
	check("gpl", g, "not an LGPL build")
	// the third rename of the swap fails: the two parts already swapped go back
	calls := 0
	rename = func(a, b string) error {
		calls++
		if calls == 3 {
			return &os.LinkError{Op: "rename", Old: a, New: b, Err: errors.New("in use")}
		}
		return os.Rename(a, b)
	}
	defer func() { rename = os.Rename }()
	check("swap", o, "is it in use?")
}

// Archives cannot write outside the target: paths with .. and links out of the directory are refused.
func TestUnsafeArchives(t *testing.T) {
	mk := func(h *tar.Header, body string) string {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		h.Size = int64(len(body))
		_ = tw.WriteHeader(h)
		_, _ = tw.Write([]byte(body))
		_ = tw.Close()
		_ = gz.Close()
		f := filepath.Join(t.TempDir(), "a.tgz")
		_ = os.WriteFile(f, buf.Bytes(), 0o644)
		return f
	}
	keep := func(string) bool { return true }
	a := analytics.Artifact{Format: analytics.FormatTarGz}
	for _, c := range []*tar.Header{
		{Name: "top/../../evil", Typeflag: tar.TypeReg, Mode: 0o644},
		{Name: "top/lib/libx.so", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
		{Name: "top/lib/libx.so", Typeflag: tar.TypeSymlink, Linkname: "../../x"},
	} {
		err := unpack(t.Context(), Options{}, a, mk(c, "x"), t.TempDir(), keep)
		if err == nil || !strings.Contains(err.Error(), "unsafe") {
			t.Errorf("%s -> %s: %v", c.Name, c.Linkname, err)
		}
	}
	// setuid bits and group/other write never survive
	dest := t.TempDir()
	if err := unpack(t.Context(), Options{}, a, mk(&tar.Header{Name: "top/bin/ffmpeg", Typeflag: tar.TypeReg, Mode: 0o4777}, "x"), dest, keep); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(dest, "bin", "ffmpeg")); err != nil || (runtime.GOOS != "windows" && st.Mode()&(os.ModeSetuid|0o022) != 0) {
		t.Errorf("mode %v %v", st.Mode(), err)
	}
}

func TestLGPLCheck(t *testing.T) {
	for out, ok := range map[string]bool{
		"ffmpeg version n8.1.2 configuration: --enable-shared --enable-version3": true,
		"ffmpeg version n8.1 configuration: --enable-gpl":                        false,
		"ffmpeg version n8.1 configuration: --enable-nonfree":                    false,
		"something else": false,
	} {
		if err := lgpl(out); (err == nil) != ok {
			t.Errorf("%q: %v", out, err)
		}
	}
}

// [analytics] gets the paths; threads, max_cameras, comments, other sections, CRLF and a BOM stay.
func TestUpdateConfig(t *testing.T) {
	p := &Paths{FFmpeg: `C:\Program Files\ctrl32-telemetry\analytics\ffmpeg-x-lgpl\bin\ffmpeg.exe`, ONNXRuntime: "/opt/a/lib/libonnxruntime.so",
		ModelNano: "/var/lib/m/yolox_nano.onnx", ModelTiny: "/var/lib/m/yolox_tiny.onnx", NanoSHA256: analytics.YOLOXNanoSHA256, TinySHA256: analytics.YOLOXTinySHA256}
	decode := func(b []byte) config.AnalyticsConfig {
		t.Helper()
		var c struct {
			Profile   string                 `toml:"profile"`
			Analytics config.AnalyticsConfig `toml:"analytics"`
		}
		if _, err := toml.Decode(string(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))), &c); err != nil {
			t.Fatalf("%v\n%s", err, b)
		}
		if c.Profile != "intranet" {
			t.Errorf("profile lost:\n%s", b)
		}
		return c.Analytics
	}
	// no section yet: appended with the defaults of the installers
	src := []byte("profile = \"intranet\"\n\n[http]\nlisten = \":8080\"\n")
	out := UpdateConfig(src, p, "written by the server agent")
	a := decode(out)
	// Windows writes forward slashes; elsewhere a backslash is an ordinary character of a name and stays
	if a.FFmpeg != filepath.ToSlash(p.FFmpeg) || a.ModelTiny != p.ModelTiny || a.Threads != 1 || a.MaxCameras != 4 || a.NanoSHA256 != p.NanoSHA256 {
		t.Errorf("new section: %+v\n%s", a, out)
	}
	if !strings.Contains(string(out), "listen = \":8080\"") {
		t.Error("other section lost")
	}
	// an existing section (CRLF, BOM, a comment, custom threads and max_cameras, a key of an older install)
	src = []byte("\xef\xbb\xbfprofile = \"intranet\"\r\n[analytics]\r\n# my note\r\nffmpeg = \"/usr/bin/ffmpeg\"\r\nthreads = 3\r\nmax_cameras = 9 # mine\r\n\r\n[video]\r\nmax_disk_gb = 7\r\n")
	out = UpdateConfig(src, p, "x")
	a = decode(out)
	if a.Threads != 3 || a.MaxCameras != 9 || a.ONNXRuntime != p.ONNXRuntime || a.FFmpeg == "/usr/bin/ffmpeg" {
		t.Errorf("existing section: %+v\n%s", a, out)
	}
	s := string(out)
	if !strings.HasPrefix(s, "\xef\xbb\xbf") || strings.Contains(strings.ReplaceAll(s, "\r\n", ""), "\n") || !strings.Contains(s, "# my note") || !strings.Contains(s, "max_disk_gb = 7") ||
		strings.Count(s, "ffmpeg =") != 1 {
		t.Errorf("format not kept:\n%q", s)
	}
	// idempotent
	if again := UpdateConfig(out, p, "x"); !bytes.Equal(again, out) {
		t.Errorf("second update changed the file:\n%q\n%q", out, again)
	}
	// removal: the paths go, threads and max_cameras stay
	out = UpdateConfig(out, nil, "")
	a = decode(out)
	if a.FFmpeg != "" || a.ONNXRuntime != "" || a.ModelNano != "" || a.NanoSHA256 != "" || a.Threads != 3 || a.MaxCameras != 9 {
		t.Errorf("removal: %+v\n%s", a, out)
	}
	if got := UpdateConfig([]byte("profile = \"intranet\"\n"), nil, ""); string(got) != "profile = \"intranet\"\n" {
		t.Errorf("removal without a section changed the file: %q", got)
	}
}

// Downloads go over HTTPS from the official hosts only, never follow a redirect to plain HTTP and fail on a short body.
func TestHTTPFetcher(t *testing.T) {
	body := []byte(strings.Repeat("model", 1000))
	var plain *httptest.Server
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write(body)
		case "/to-http":
			http.Redirect(w, r, plain.URL+"/ok", http.StatusFound)
		case "/short":
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("only a bit"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	plain = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
	defer plain.Close()
	f := HTTPFetcher{Client: srv.Client(), Hosts: []string{"127.0.0.1"}}
	var buf bytes.Buffer
	var last int64
	if err := f.Fetch(t.Context(), srv.URL+"/ok", &buf, func(n, _ int64) { last = n }); err != nil || !bytes.Equal(buf.Bytes(), body) || last != int64(len(body)) {
		t.Fatalf("ok: %v %d", err, last)
	}
	for path, want := range map[string]string{"/to-http": "not HTTPS", "/short": "", "/missing": "HTTP 404"} {
		err := f.Fetch(t.Context(), srv.URL+path, io.Discard, nil)
		if err == nil || (want != "" && !strings.Contains(err.Error(), want)) {
			t.Errorf("%s: %v", path, err)
		}
	}
	if err := f.Fetch(t.Context(), plain.URL+"/ok", io.Discard, nil); err == nil || !strings.Contains(err.Error(), "official source") {
		t.Errorf("plain HTTP: %v", err)
	}
	if err := (HTTPFetcher{}).Fetch(t.Context(), "https://example.com/yolox_nano.onnx", io.Discard, nil); err == nil || !strings.Contains(err.Error(), "official source") {
		t.Errorf("other host: %v", err)
	}
}

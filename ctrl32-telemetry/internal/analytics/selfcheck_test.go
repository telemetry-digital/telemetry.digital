package analytics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFFmpegLicence(t *testing.T) {
	cases := map[string]string{
		"ffmpeg version 7.1.5-0+deb13u1\nconfiguration: --prefix=/usr --enable-gpl --enable-libx264 --enable-shared\n": "GPL",
		"ffmpeg version n8.1.2\n  configuration: --enable-version3 --enable-shared --disable-libx264\n":                "LGPL",
		"ffmpeg version 6\nconfiguration: --enable-gpl --enable-nonfree --enable-libfdk-aac\n":                         "non-free",
		"ffmpeg version 6\n": "LGPL", // no configuration line: nothing says GPL
	}
	for out, want := range cases {
		if got := FFmpegLicence(out); got != want {
			t.Errorf("FFmpegLicence(%q) = %s, want %s", out, got, want)
		}
	}
}

func comp(r CheckResult, name string) Component {
	for _, c := range r.Components {
		if c.Name == name {
			return c
		}
	}
	return Component{}
}

func TestSelfCheckNothingInstalled(t *testing.T) {
	dir := t.TempDir()
	r := SelfCheck(context.Background(), CheckPaths{FFmpeg: filepath.Join(dir, "no-ffmpeg")})
	if r.Ready {
		t.Fatal("ready without anything installed")
	}
	if c := comp(r, "ffmpeg"); c.State != StateMissing || c.Hint != HintInstall {
		t.Errorf("ffmpeg: %+v", c)
	}
	if c := comp(r, "onnxruntime"); c.State == StateOK || c.Hint == "" {
		t.Errorf("onnxruntime: %+v", c)
	}
	// neither model configured: the first is reported missing, the second stays "not configured"
	if c := comp(r, "model_nano"); c.State != StateMissing || c.Hint != HintInstall {
		t.Errorf("model_nano: %+v", c)
	}
	if c := comp(r, "model_tiny"); c.State != StateNotConfigured {
		t.Errorf("model_tiny: %+v", c)
	}
}

func TestSelfCheckModelPin(t *testing.T) {
	dir := t.TempDir()
	m := filepath.Join(dir, "yolox_nano.onnx")
	if err := os.WriteFile(m, []byte("not a model"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := SelfCheck(context.Background(), CheckPaths{ModelNano: m, NanoSHA256: "c789161ed43c8269fcd4e67c67eeeb4e80c622da2eb296a20bc6007bd18a0b7d", ModelTiny: filepath.Join(dir, "missing.onnx")})
	if c := comp(r, "model_nano"); c.State != StateError || c.Hint != HintSHA {
		t.Errorf("a file that is not the pinned one: %+v", c)
	}
	if c := comp(r, "model_tiny"); c.State != StateMissing {
		t.Errorf("a configured model that does not exist: %+v", c)
	}
	// without a pin the file is found; it is not tried without ONNX Runtime
	r = SelfCheck(context.Background(), CheckPaths{ModelNano: m})
	if c := comp(r, "model_nano"); c.State != StateWarning || c.Version == "" {
		t.Errorf("unpinned model without ONNX Runtime: %+v", c)
	}
}

// TestSelfCheckInstalled runs the full check against a real installation:
//
//	CTRL32_ORT_LIB=…/onnxruntime.dll CTRL32_ORT_MODEL=…/yolox_nano.onnx CTRL32_FFMPEG=…/ffmpeg.exe go test ./internal/analytics -run SelfCheckInstalled
func TestSelfCheckInstalled(t *testing.T) {
	lib, model, ff := os.Getenv("CTRL32_ORT_LIB"), os.Getenv("CTRL32_ORT_MODEL"), os.Getenv("CTRL32_FFMPEG")
	if lib == "" || model == "" || ff == "" {
		t.Skip("CTRL32_ORT_LIB, CTRL32_ORT_MODEL and CTRL32_FFMPEG not set")
	}
	r := SelfCheck(context.Background(), CheckPaths{FFmpeg: ff, ONNXRuntime: lib, ModelNano: model})
	for _, c := range r.Components {
		t.Logf("%-12s %-15s %s %s %s", c.Name, c.State, c.Version, c.Licence, c.Detail)
	}
	if !r.Ready {
		t.Fatal("not ready")
	}
}

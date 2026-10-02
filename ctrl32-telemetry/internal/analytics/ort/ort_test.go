package ort

import (
	"math"
	"os"
	"testing"
)

// TestRunModel runs a real model when the library and a model are available:
//
//	CTRL32_ORT_LIB=.dev/analytics/ort/lib/onnxruntime.dll CTRL32_ORT_MODEL=.dev/analytics/yolox_nano.onnx go test ./internal/analytics/ort
func TestRunModel(t *testing.T) {
	lib, model := os.Getenv("CTRL32_ORT_LIB"), os.Getenv("CTRL32_ORT_MODEL")
	if lib == "" || model == "" || !Supported {
		t.Skip("CTRL32_ORT_LIB and CTRL32_ORT_MODEL not set")
	}
	v, err := Load(lib)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("onnxruntime %s", v)
	b, err := os.ReadFile(model)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSession(b, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.InputName != "images" || s.OutputName != "output" {
		t.Fatalf("names %q %q", s.InputName, s.OutputName)
	}
	in := make([]float32, 3*416*416)
	for i := range in {
		in[i] = 114
	}
	out, dims, err := s.Run(in, []int64{1, 3, 416, 416})
	if err != nil {
		t.Fatal(err)
	}
	if len(dims) != 3 || dims[1] != 3549 || dims[2] != 85 || len(out) != 3549*85 {
		t.Fatalf("dims %v len %d", dims, len(out))
	}
	for _, v := range out {
		if math.IsNaN(float64(v)) {
			t.Fatal("NaN in output")
		}
	}
	if _, _, err := s.Run(in[:10], []int64{1, 3, 416, 416}); err == nil {
		t.Fatal("wrong input size accepted")
	}
}

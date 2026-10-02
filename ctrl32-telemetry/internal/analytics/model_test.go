package analytics

import (
	"os"
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics/ort"
)

// TestDetectorOnFrame runs the real detector on a letterboxed BGR frame when everything is available:
//
//	CTRL32_ORT_LIB=… CTRL32_ORT_MODEL=….onnx CTRL32_TEST_FRAME=frame.bgr CTRL32_TEST_FRAME_SIZE=1920x1080 go test -run Detector -v ./internal/analytics
func TestDetectorOnFrame(t *testing.T) {
	lib, model, frame := os.Getenv("CTRL32_ORT_LIB"), os.Getenv("CTRL32_ORT_MODEL"), os.Getenv("CTRL32_TEST_FRAME")
	if lib == "" || model == "" || frame == "" {
		t.Skip("CTRL32_ORT_LIB, CTRL32_ORT_MODEL and CTRL32_TEST_FRAME not set")
	}
	if _, err := ort.Load(lib); err != nil {
		t.Fatal(err)
	}
	mb, _ := os.ReadFile(model)
	s, err := ort.NewSession(mb, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	bgr, err := os.ReadFile(frame)
	if err != nil {
		t.Fatal(err)
	}
	in := make([]float32, 3*InputSize*InputSize)
	Tensor(bgr, in)
	var out []float32
	t0 := time.Now()
	const runs = 10
	for range runs {
		if out, _, err = s.Run(in, []int64{1, 3, InputSize, InputSize}); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("inference %.1f ms per frame (1 thread)", float64(time.Since(t0).Milliseconds())/runs)
	var sum float64
	for _, v := range out {
		if v < 0 {
			sum -= float64(v)
		} else {
			sum += float64(v)
		}
	}
	t.Logf("raw sum %.1f", sum)
	lb := NewLetterbox(1920, 1080)
	boxes := NMS(Decode(out, lb, 0.3), 0.45)
	for _, b := range boxes {
		t.Logf("%s %.3f  %.0f %.0f %.0f %.0f (416 px)", b.Class, b.Score, b.X1*float64(lb.SW), b.Y1*float64(lb.SH), b.X2*float64(lb.SW), b.Y2*float64(lb.SH))
	}
	if len(boxes) == 0 {
		t.Fatal("nothing detected")
	}
}

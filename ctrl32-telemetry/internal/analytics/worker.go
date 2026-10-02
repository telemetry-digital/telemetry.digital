package analytics

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics/ort"
)

// WorkerConfig is what the server tells a worker (no secrets: the video arrives on stdin).
type WorkerConfig struct {
	Camera      string  `json:"camera"`      // name, for messages
	Codec       string  `json:"codec"`       // h264 | h265 | mjpeg (the frames on stdin)
	Width       int     `json:"width"`       // picture size of the stream
	Height      int     `json:"height"`      //
	FPS         float64 `json:"fps"`         // frames analysed per second (default 5)
	FFmpeg      string  `json:"ffmpeg"`      // ffmpeg executable (LGPL build, unmodified)
	ORTLib      string  `json:"onnxruntime"` // ONNX Runtime shared library
	Model       string  `json:"model"`       // YOLOX ONNX model file
	ModelSHA256 string  `json:"model_sha256,omitempty"`
	Threads     int     `json:"threads,omitempty"`   // inference threads (default 1)
	MinScore    float64 `json:"min_score,omitempty"` // detection threshold (default 0.35)
	Count       Config  `json:"count"`
	Boxes       bool    `json:"boxes,omitempty"` // report the boxes of the last analysed frame with the statistics (line editor preview)
	StatsEvery  float64 `json:"stats_every,omitempty"`
	// offline evaluation (bench): a video file decoded directly; timestamps from its frame rate
	Input    string  `json:"input,omitempty"`
	InputFPS float64 `json:"input_fps,omitempty"`
}

// Event is one line the worker writes to stdout.
type Event struct {
	Type    string    `json:"type"` // ready | cross | speed | stats | error | summary
	Cross   *Crossing `json:"cross,omitempty"`
	Speed   *Speed    `json:"speed,omitempty"`
	Stats   *Stats    `json:"stats,omitempty"`
	Message string    `json:"message,omitempty"`
	Version string    `json:"version,omitempty"`
}

// Stats describe the work of the last interval.
type Stats struct {
	FramesIn     int        `json:"frames_in"`       // decoded frames
	Analysed     int        `json:"analysed"`        // frames run through the detector
	FPS          float64    `json:"fps"`             // analysed per second
	InferMs      float64    `json:"infer_ms"`        // average inference time
	CPUPercent   float64    `json:"cpu_percent"`     // worker + ffmpeg, % of one CPU core
	WorkerCPU    float64    `json:"worker_cpu"`      // worker alone
	DecoderCPU   float64    `json:"decoder_cpu"`     // ffmpeg alone
	Tracks       int        `json:"tracks"`          // objects followed now
	Detections   int        `json:"detections"`      // in the last analysed frame
	Boxes        [][]any    `json:"boxes,omitempty"` // [class, x1, y1, x2, y2, track] of the last analysed frame
	Counts       countTally `json:"counts"`          // since the start of the worker
	IntervalSecs float64    `json:"interval_s"`
}

type countTally map[string]map[string]int // class → in/out → n

const frameBytes = InputSize * InputSize * 3

// writer serialises events.
type writer struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func (w *writer) emit(e Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.enc.Encode(e)
}

// Main is the analytics-worker command: `ctrl32-telemetry analytics-worker -config '<json>'` (started by the server, the
// video on stdin), or `-bench -input clip.mp4 -config file.json` to evaluate a recorded clip.
func Main(args []string) int {
	fs := flag.NewFlagSet("analytics-worker", flag.ContinueOnError)
	cfgArg := fs.String("config", "", "configuration as JSON, or @file")
	input := fs.String("input", "", "bench: a video file instead of stdin")
	fps := fs.Float64("fps", 0, "bench: frames analysed per second (overrides the configuration)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	raw := *cfgArg
	if strings.HasPrefix(raw, "@") {
		b, err := os.ReadFile(raw[1:])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		raw = string(b)
	}
	var cfg WorkerConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 2
	}
	if *input != "" {
		cfg.Input = *input
	}
	if *fps > 0 {
		cfg.FPS = *fps
	}
	w := &writer{enc: json.NewEncoder(os.Stdout)}
	if err := Run(context.Background(), cfg, os.Stdin, w); err != nil {
		w.emit(Event{Type: "error", Message: err.Error()})
		return 1
	}
	return 0
}

// Run analyses frames until stdin (or the input file) ends.
func Run(ctx context.Context, cfg WorkerConfig, stdin io.Reader, w *writer) error {
	if err := cfg.Count.Validate(); err != nil {
		return err
	}
	if cfg.FPS <= 0 {
		cfg.FPS = 5
	}
	cfg.FPS = min(cfg.FPS, 30)
	if cfg.MinScore <= 0 {
		cfg.MinScore = 0.35
	}
	if cfg.StatsEvery <= 0 {
		cfg.StatsEvery = 10
	}
	version, err := ort.Load(cfg.ORTLib)
	if err != nil {
		return err
	}
	model, err := os.ReadFile(cfg.Model)
	if err != nil {
		return fmt.Errorf("model: %w", err)
	}
	if cfg.ModelSHA256 != "" {
		sum := sha256.Sum256(model)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), cfg.ModelSHA256) {
			return errors.New("model: SHA-256 does not match the configured one (a different or damaged file)")
		}
	}
	sess, err := ort.NewSession(model, max(1, cfg.Threads))
	if err != nil {
		return err
	}
	defer sess.Close()

	if cfg.Input != "" && (cfg.Width == 0 || cfg.Height == 0) {
		cfg.Width, cfg.Height = probeSize(cfg.FFmpeg, cfg.Input)
		if cfg.InputFPS <= 0 {
			cfg.InputFPS = probeFPS(cfg.FFmpeg, cfg.Input)
		}
	}
	lb := NewLetterbox(cfg.Width, cfg.Height)
	ff := ffmpegPath(cfg.FFmpeg)
	argv := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	if cfg.Input != "" {
		argv = append(argv, "-i", cfg.Input)
	} else {
		dem := map[string]string{"h264": "h264", "h265": "hevc", "mjpeg": "mjpeg"}[cfg.Codec]
		if dem == "" {
			return fmt.Errorf("codec %q: h264, h265 or mjpeg", cfg.Codec)
		}
		argv = []string{"-hide_banner", "-loglevel", "error", "-fflags", "nobuffer", "-flags", "low_delay", "-f", dem, "-i", "pipe:0"}
	}
	argv = append(argv, "-an", "-sn", "-fps_mode", "passthrough",
		"-vf", fmt.Sprintf("scale=%d:%d:flags=bilinear,pad=%d:%d:0:0:color=0x727272", lb.SW, lb.SH, InputSize, InputSize),
		"-pix_fmt", "bgr24", "-f", "rawvideo", "pipe:1")
	cmd := exec.CommandContext(ctx, ff, argv...)
	var stderr strings.Builder
	cmd.Stderr = &limitWriter{w: &stderr, n: 4096}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var in io.WriteCloser
	if cfg.Input == "" {
		if in, err = cmd.StdinPipe(); err != nil {
			return err
		}
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg (%s): %w", ff, err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	stamps := make(chan time.Time, 4096)
	if in != nil {
		go feed(stdin, in, stamps)
	}
	w.emit(Event{Type: "ready", Version: "onnxruntime " + version})

	cnt := &Counter{Cfg: cfg.Count}
	tensor := make([]float32, 3*InputSize*InputSize)
	buf := make([]byte, frameBytes)
	rd := bufio.NewReaderSize(out, frameBytes)
	period := time.Duration(float64(time.Second) / cfg.FPS)
	var last, start time.Time
	tally := countTally{}
	st := &Stats{}
	var inferSum time.Duration
	statT := time.Now()
	cpu0 := cpuTimes(os.Getpid())
	ffcpu0 := cpuTimes(cmd.Process.Pid)
	var lastBoxes []Box
	var lastTracks []*Track
	trace := os.Getenv("CTRL32_ANALYTICS_TRACE") != "" // per-frame tracks on stderr (diagnostics of a bench run)
	n := 0
	for {
		if _, err := io.ReadFull(rd, buf); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return err
		}
		var at time.Time
		if cfg.Input != "" {
			if start.IsZero() {
				start = time.Unix(0, 0).UTC()
			}
			at = start.Add(time.Duration(float64(n) / max(cfg.InputFPS, 1) * float64(time.Second)))
		} else {
			select {
			case at = <-stamps:
			default:
				at = time.Now()
			}
		}
		n++
		st.FramesIn++
		if !last.IsZero() && at.Sub(last) < period-period/10 {
			continue
		}
		last = at
		Tensor(buf, tensor)
		t0 := time.Now()
		res, _, err := sess.Run(tensor, []int64{1, 3, InputSize, InputSize})
		if err != nil {
			return err
		}
		inferSum += time.Since(t0)
		st.Analysed++
		boxes := NMS(Decode(res, lb, cfg.MinScore), 0.45)
		cs, sp := cnt.Frame(boxes, at)
		for i := range cs {
			c := cs[i]
			if tally[c.Class] == nil {
				tally[c.Class] = map[string]int{}
			}
			tally[c.Class][c.Dir]++
			w.emit(Event{Type: "cross", Cross: &c})
		}
		for i := range sp {
			s := sp[i]
			w.emit(Event{Type: "speed", Speed: &s})
		}
		lastBoxes, lastTracks = boxes, cnt.Tr.Tracks()
		if trace {
			for _, t := range lastTracks {
				if t.Last.Equal(at) {
					fmt.Fprintf(os.Stderr, "%8.3f track %3d %-7s hits %3d box %.3f %.3f %.3f %.3f score %.2f\n", at.Sub(time.Unix(0, 0)).Seconds(), t.ID, t.Class(), t.Hits, t.Box.X1, t.Box.Y1, t.Box.X2, t.Box.Y2, t.Box.Score)
				}
			}
		}
		if d := time.Since(statT); d.Seconds() >= cfg.StatsEvery {
			emitStats(w, st, d, inferSum, cpu0, ffcpu0, cmd.Process.Pid, tally, lastBoxes, lastTracks, cfg.Boxes)
			st, inferSum, statT = &Stats{}, 0, time.Now()
			cpu0, ffcpu0 = cpuTimes(os.Getpid()), cpuTimes(cmd.Process.Pid)
		}
	}
	if err := cmd.Wait(); err != nil && cfg.Input != "" {
		return fmt.Errorf("ffmpeg: %v %s", err, strings.TrimSpace(stderr.String()))
	}
	if cfg.Input != "" || in == nil {
		d := time.Since(statT)
		emitStats(w, st, d, inferSum, cpu0, ffcpu0, 0, tally, nil, nil, false)
		w.emit(Event{Type: "summary", Message: fmt.Sprintf("%d frames", n)})
	}
	if s := strings.TrimSpace(stderr.String()); s != "" && st.FramesIn == 0 {
		return errors.New("ffmpeg: " + s)
	}
	return nil
}

func emitStats(w *writer, st *Stats, d time.Duration, inferSum, cpu0, ffcpu0 time.Duration, ffpid int, tally countTally, boxes []Box, tracks []*Track, withBoxes bool) {
	secs := d.Seconds()
	st.IntervalSecs = round1(secs)
	if st.Analysed > 0 {
		st.InferMs = round1(float64(inferSum.Microseconds()) / 1000 / float64(st.Analysed))
	}
	if secs > 0 {
		st.FPS = round1(float64(st.Analysed) / secs)
		st.WorkerCPU = round1(float64(cpuTimes(os.Getpid())-cpu0) / float64(d) * 100)
		if ffpid > 0 {
			st.DecoderCPU = round1(float64(cpuTimes(ffpid)-ffcpu0) / float64(d) * 100)
		}
		st.CPUPercent = round1(st.WorkerCPU + st.DecoderCPU)
	}
	st.Tracks = len(tracks)
	st.Detections = len(boxes)
	st.Counts = tally
	if withBoxes {
		for _, b := range boxes {
			id := 0
			for _, t := range tracks {
				if t.Box == b {
					id = t.ID
				}
			}
			st.Boxes = append(st.Boxes, []any{b.Class, round3(b.X1), round3(b.Y1), round3(b.X2), round3(b.Y2), id})
		}
	}
	w.emit(Event{Type: "stats", Stats: st})
}

func round1(v float64) float64 { return float64(int64(v*10+0.5)) / 10 }
func round3(v float64) float64 { return float64(int64(v*1000+0.5)) / 1000 }

// feed copies framed samples from the server to ffmpeg: 8 bytes wall-clock time (Unix nanoseconds), 4 bytes length,
// the sample (an H.264/H.265 access unit in Annex B form, or one JPEG picture). Every sample yields one decoded frame,
// so the times queue up in the same order.
func feed(r io.Reader, ff io.WriteCloser, stamps chan<- time.Time) {
	defer ff.Close()
	br := bufio.NewReaderSize(r, 1<<20)
	hdr := make([]byte, 12)
	var buf []byte
	for {
		if _, err := io.ReadFull(br, hdr); err != nil {
			return
		}
		ns := int64(binary.BigEndian.Uint64(hdr[:8]))
		n := binary.BigEndian.Uint32(hdr[8:])
		if n > 32<<20 {
			return
		}
		if cap(buf) < int(n) {
			buf = make([]byte, n)
		}
		buf = buf[:n]
		if _, err := io.ReadFull(br, buf); err != nil {
			return
		}
		if n == 0 {
			continue
		}
		select {
		case stamps <- time.Unix(0, ns):
		default: // the analysis is behind: the frame gets the time it is read
		}
		if _, err := ff.Write(buf); err != nil {
			return
		}
	}
}

// WriteFrame writes one framed sample for a worker's stdin.
func WriteFrame(w io.Writer, at time.Time, payload []byte) error {
	hdr := make([]byte, 12)
	binary.BigEndian.PutUint64(hdr[:8], uint64(at.UnixNano()))
	binary.BigEndian.PutUint32(hdr[8:], uint32(len(payload)))
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func ffmpegPath(p string) string {
	if p != "" {
		return p
	}
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name = "ffmpeg.exe"
	}
	if exe, err := os.Executable(); err == nil {
		if c := filepath.Join(filepath.Dir(exe), name); fileExists(c) {
			return c
		}
	}
	return name
}

func fileExists(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() }

func probe(ff, input, entries string) string {
	pr := ffmpegPath(ff)
	pr = filepath.Join(filepath.Dir(pr), strings.Replace(filepath.Base(pr), "ffmpeg", "ffprobe", 1))
	out, err := exec.Command(pr, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream="+entries, "-of", "csv=p=0", input).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func probeSize(ff, input string) (int, int) {
	parts := strings.Split(probe(ff, input, "width,height"), ",")
	if len(parts) < 2 {
		return 0, 0
	}
	w, _ := strconv.Atoi(parts[0])
	h, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	return w, h
}

func probeFPS(ff, input string) float64 {
	s := probe(ff, input, "r_frame_rate")
	a, b, ok := strings.Cut(s, "/")
	if !ok {
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}
	x, _ := strconv.ParseFloat(a, 64)
	y, _ := strconv.ParseFloat(b, 64)
	if y == 0 {
		return 0
	}
	return x / y
}

type limitWriter struct {
	w io.Writer
	n int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		_, _ = l.w.Write(p[:k])
		l.n -= k
	}
	return len(p), nil
}

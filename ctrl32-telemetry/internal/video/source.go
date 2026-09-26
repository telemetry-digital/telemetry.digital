package video

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Packet is what a subscriber receives: a sample and the track it belongs to.
type Packet struct {
	Track  *Track
	Sample *Sample
}

// Status of a source.
type Status struct {
	State     string    `json:"state"` // idle, connecting, online, offline
	Error     string    `json:"error,omitempty"`
	Codec     string    `json:"codec,omitempty"`
	Width     int       `json:"width,omitempty"`
	Height    int       `json:"height,omitempty"`
	FPS       float64   `json:"fps"`
	Kbps      float64   `json:"kbps"`
	Since     time.Time `json:"since,omitzero"`
	Viewers   int       `json:"viewers"`
	Recording bool      `json:"recording"`
}

// Source is one camera stream (main or sub). It connects while it is kept (recording) or watched, reconnects with a
// back-off, and fans frames out to subscribers. A new subscriber first gets the frames since the last key frame, so
// the picture appears at once.
type Source struct {
	URL       *url.URL // with credentials
	Transport string   // auto, tcp, udp
	Log       *slog.Logger
	Linger    time.Duration // how long an unwatched on-demand stream stays connected

	mu     sync.Mutex
	subs   map[*Sub]struct{}
	keep   bool
	cancel context.CancelFunc
	track  *Track
	gop    []*Sample
	gopLen int
	status Status
	idleT  *time.Timer
	last   *Sample // last MJPEG picture (snapshot)
	push   chan *pushConn

	// statistics over the last few seconds
	winStart time.Time
	winN     int
	winBytes int
	lastFPS  float64 // over the last complete window
	lastKbps float64
}

// Sub receives packets of a source. Close it when done.
type Sub struct {
	C       chan Packet
	src     *Source
	lagging bool
}

const subQueue = 256

// Subscribe starts the stream if needed and returns a subscriber primed with the current GOP.
func (s *Source) Subscribe() *Sub {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub := &Sub{C: make(chan Packet, subQueue+len(s.gop)), src: s}
	if s.subs == nil {
		s.subs = map[*Sub]struct{}{}
	}
	s.subs[sub] = struct{}{}
	for _, smp := range s.gop {
		sub.C <- Packet{Track: s.track, Sample: smp}
	}
	if s.idleT != nil {
		s.idleT.Stop()
		s.idleT = nil
	}
	s.startLocked()
	return sub
}

// Close unsubscribes; an on-demand stream disconnects after Linger without viewers.
func (sub *Sub) Close() {
	s := sub.src
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subs[sub]; !ok {
		return
	}
	delete(s.subs, sub)
	close(sub.C)
	s.maybeStopLocked()
}

// Keep holds the stream connected (recording) or releases it.
func (s *Source) Keep(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keep = on
	if on {
		s.startLocked()
	} else {
		s.maybeStopLocked()
	}
}

// Stop disconnects regardless of subscribers and closes them.
func (s *Source) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keep = false
	for sub := range s.subs {
		delete(s.subs, sub)
		close(sub.C)
	}
	s.stopLocked()
}

// Status reports the connection state and statistics.
func (s *Source) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.status
	if st.State == "" {
		st.State = "idle"
	}
	st.Viewers = len(s.subs)
	if s.lastFPS > 0 {
		st.FPS, st.Kbps = s.lastFPS, s.lastKbps
	} else if !s.winStart.IsZero() {
		if d := time.Since(s.winStart).Seconds(); d > 0.5 {
			st.FPS = float64(int(float64(s.winN)/d*10)) / 10
			st.Kbps = float64(int(float64(s.winBytes) * 8 / 1000 / d))
		}
	}
	return st
}

// Snapshot returns the last MJPEG picture, if the stream is MJPEG and running.
func (s *Source) Snapshot() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil {
		return nil
	}
	return s.last.JPEG
}

func (s *Source) startLocked() {
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.status = Status{State: "connecting"}
	go s.run(ctx)
}

func (s *Source) maybeStopLocked() {
	if s.keep || len(s.subs) > 0 || s.cancel == nil {
		return
	}
	if s.idleT != nil {
		return
	}
	linger := s.Linger
	s.idleT = time.AfterFunc(linger, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.idleT = nil
		if !s.keep && len(s.subs) == 0 {
			s.stopLocked()
		}
	})
}

func (s *Source) stopLocked() {
	if s.idleT != nil {
		s.idleT.Stop()
		s.idleT = nil
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.status = Status{State: "idle"}
	s.gop, s.gopLen, s.track, s.last = nil, 0, nil, nil
}

// run connects and reconnects until the context ends.
func (s *Source) run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		began := time.Now()
		err := s.connect(ctx)
		if ctx.Err() != nil {
			return
		}
		msg := "stream ended"
		if err != nil {
			msg = err.Error()
		}
		s.mu.Lock()
		if s.cancel != nil {
			s.status = Status{State: "offline", Error: redact(msg, s.URL), Since: time.Now()}
			s.gop, s.gopLen, s.last = nil, 0, nil
		}
		s.mu.Unlock()
		if s.Log != nil {
			s.Log.Warn("camera stream down", "url", Redacted(s.URL), "err", redact(msg, s.URL))
		}
		if time.Since(began) > time.Minute {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
		if s.URL.Scheme == "push" {
			backoff = 100 * time.Millisecond // the camera connects to us; be ready at once
		}
	}
}

func (s *Source) connect(ctx context.Context) error {
	switch s.URL.Scheme {
	case "rtsp", "rtsps":
		return s.rtsp(ctx)
	case "http", "https":
		return s.httpMJPEG(ctx)
	case "push":
		return s.waitPush(ctx)
	}
	return errors.New("unsupported URL scheme " + s.URL.Scheme)
}

// publish hands a frame to the subscribers. The first frame of a connection must be a key frame.
func (s *Source) publish(ctx context.Context, t *Track, smp *Sample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel == nil || ctx.Err() != nil {
		return
	}
	if s.track != t {
		if !smp.Key {
			return
		}
		s.track = t
		s.gop, s.gopLen = nil, 0
		s.status = Status{State: "online", Codec: t.Codec, Width: t.Width, Height: t.Height, Since: time.Now()}
		s.winStart, s.winN, s.winBytes, s.lastFPS, s.lastKbps = time.Now(), 0, 0, 0, 0
	}
	if t.Codec == MJPEG {
		s.last = smp
	}
	// the GOP cache: MJPEG needs only the last picture; a very long GOP is capped
	if smp.Key || t.Codec == MJPEG {
		s.gop, s.gopLen = s.gop[:0:0], 0
	}
	if s.gopLen < 64<<20 && len(s.gop) < 1000 {
		s.gop = append(s.gop, smp)
		s.gopLen += smp.Size()
	}
	if d := time.Since(s.winStart); d > 5*time.Second {
		s.lastFPS = float64(int(float64(s.winN)/d.Seconds()*10)) / 10
		s.lastKbps = float64(int(float64(s.winBytes) * 8 / 1000 / d.Seconds()))
		s.winStart, s.winN, s.winBytes = time.Now(), 0, 0
	}
	s.winN++
	s.winBytes += smp.Size()
	for sub := range s.subs {
		if sub.lagging {
			if !smp.Key {
				continue
			}
			sub.lagging = false
		}
		select {
		case sub.C <- Packet{Track: t, Sample: smp}:
		default:
			sub.lagging = true // a slow viewer skips to the next key frame instead of slowing the others
		}
	}
}

// Redacted is the URL without the password, for logs and status.
func Redacted(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.Redacted()
}

func redact(msg string, u *url.URL) string {
	if u == nil || u.User == nil {
		return msg
	}
	if p, ok := u.User.Password(); ok && p != "" {
		return strings.ReplaceAll(msg, p, "xxxxx")
	}
	return msg
}

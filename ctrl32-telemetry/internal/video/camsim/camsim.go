// Package camsim simulates IP cameras for tests and demos: an RTSP server whose path chooses the camera, the codec
// and the picture size (rtsp://host:port/<label>/<h264|mjpeg>/<W>x<H>), and an HTTP MJPEG server
// (http://host:port/<label>/<W>x<H>). The H.264 stream uses raw (I_PCM) macroblocks, so no encoder library is needed.
package camsim

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/video/testsrc"
)

// Server is a simulated camera farm.
type Server struct {
	RTSPAddr string // e.g. ":8554"
	FPS      int    // default 10
	Loop     int    // > 0: encode this many frames once and repeat them (load tests measure the server, not the simulator)
	Log      *slog.Logger
	ONVIF    *ONVIF // PTZ positions shape the picture

	srv      *gortsplib.Server
	mu       sync.Mutex
	streams  map[string]*stream
	sessions map[*gortsplib.ServerSession]*stream
	ctx      context.Context
	cancel   context.CancelFunc
}

type spec struct {
	label string
	codec string
	w, h  int
}

// parse reads "<label>/<codec>/<W>x<H>"; missing parts default to h264 and 320x240.
func parse(path string) spec {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	sp := spec{label: parts[0], codec: "h264", w: 320, h: 240}
	if sp.label == "" {
		sp.label = "cam"
	}
	for _, p := range parts[1:] {
		switch {
		case p == "h264" || p == "mjpeg":
			sp.codec = p
		case strings.Contains(p, "x"):
			a, b, _ := strings.Cut(p, "x")
			w, e1 := strconv.Atoi(a)
			h, e2 := strconv.Atoi(b)
			if e1 == nil && e2 == nil && w >= 16 && h >= 16 && w <= 1920 && h <= 1088 {
				sp.w, sp.h = w/16*16, h/16*16
			}
		}
	}
	return sp
}

type stream struct {
	st      *gortsplib.ServerStream
	desc    *description.Session
	readers atomic.Int32 // frames are drawn and encoded only while someone plays the stream
}

// Start listens for RTSP.
func (s *Server) Start() error {
	if s.FPS <= 0 {
		s.FPS = 10
	}
	s.streams = map[string]*stream{}
	s.sessions = map[*gortsplib.ServerSession]*stream{}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.srv = &gortsplib.Server{Handler: s, RTSPAddress: s.RTSPAddr, WriteQueueSize: 4096} // I_PCM frames are large
	return s.srv.Start()
}

// Close stops the server and all streams.
func (s *Server) Close() {
	s.cancel()
	s.srv.Close()
}

func (s *Server) get(path string) (*stream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.Trim(path, "/")
	if st, ok := s.streams[key]; ok {
		return st, nil
	}
	sp := parse(key)
	var f format.Format
	if sp.codec == "mjpeg" {
		f = &format.MJPEG{}
	} else {
		enc := &testsrc.Encoder{W: sp.w, H: sp.h}
		f = &format.H264{PayloadTyp: 96, PacketizationMode: 1, SPS: enc.SPS(), PPS: enc.PPS()}
	}
	desc := &description.Session{Medias: []*description.Media{{Type: description.MediaTypeVideo, Formats: []format.Format{f}}}}
	st := &gortsplib.ServerStream{Server: s.srv, Desc: desc}
	if err := st.Initialize(); err != nil {
		return nil, err
	}
	out := &stream{st: st, desc: desc}
	s.streams[key] = out
	go s.produce(out, sp)
	return out, nil
}

// produce writes frames at the configured rate for as long as the server runs.
func (s *Server) produce(out *stream, sp spec) {
	medi := out.desc.Medias[0]
	period := time.Second / time.Duration(s.FPS)
	tick := time.NewTicker(period)
	defer tick.Stop()
	start := time.Now()
	var cache [][][]byte
	cached := func(n int, make func() [][]byte) [][]byte {
		if s.Loop <= 0 {
			return make()
		}
		if len(cache) < s.Loop {
			cache = append(cache, make())
			return cache[len(cache)-1]
		}
		return cache[n%s.Loop]
	}
	var h264Enc *testsrc.Encoder
	var send func(pts uint32, n int, now time.Time) error
	switch f := medi.Formats[0].(type) {
	case *format.H264:
		h264Enc = &testsrc.Encoder{W: sp.w, H: sp.h}
		rtpEnc, err := f.CreateEncoder()
		if err != nil {
			return
		}
		send = func(pts uint32, n int, now time.Time) error {
			au := cached(n, func() [][]byte { return h264Enc.Encode(toYCbCr(s.picture(sp, now, n))) })
			pkts, err := rtpEnc.Encode(au)
			if err != nil {
				return err
			}
			for _, p := range pkts {
				p.Timestamp += pts
				if err := out.st.WritePacketRTP(medi, p); err != nil {
					return err
				}
			}
			return nil
		}
	case *format.MJPEG:
		rtpEnc, err := f.CreateEncoder()
		if err != nil {
			return
		}
		send = func(pts uint32, n int, now time.Time) error {
			img := cached(n, func() [][]byte {
				var b bytes.Buffer
				_ = jpeg.Encode(&b, s.picture(sp, now, n), &jpeg.Options{Quality: 75})
				return [][]byte{b.Bytes()}
			})
			pkts, err := rtpEnc.Encode(img[0])
			if err != nil {
				return err
			}
			for _, p := range pkts {
				p.Timestamp += pts
				if err := out.st.WritePacketRTP(medi, p); err != nil {
					return err
				}
			}
			return nil
		}
	}
	for n := 0; ; n++ {
		select {
		case <-s.ctx.Done():
			out.st.Close()
			return
		case now := <-tick.C:
			if out.readers.Load() == 0 {
				continue
			}
			pts := uint32(int64(now.Sub(start)) * 90000 / int64(time.Second))
			if err := send(pts, n, now); err != nil && s.Log != nil {
				s.Log.Warn("camsim frame", "err", err)
			}
		}
	}
}

func (s *Server) picture(sp spec, now time.Time, n int) *image.RGBA {
	pan, tilt, zoom := s.ONVIF.View(sp.label)
	return PictureView(sp.label, sp.w, sp.h, now, n, pan, tilt, zoom)
}

// OnDescribe implements gortsplib.ServerHandlerOnDescribe.
func (s *Server) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
	st, err := s.get(ctx.Path)
	if err != nil {
		return &base.Response{StatusCode: base.StatusInternalServerError}, nil, err
	}
	return &base.Response{StatusCode: base.StatusOK}, st.st, nil
}

// OnSetup implements gortsplib.ServerHandlerOnSetup.
func (s *Server) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	st, err := s.get(ctx.Path)
	if err != nil {
		return &base.Response{StatusCode: base.StatusInternalServerError}, nil, err
	}
	return &base.Response{StatusCode: base.StatusOK}, st.st, nil
}

// OnPlay implements gortsplib.ServerHandlerOnPlay.
func (s *Server) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	st, err := s.get(ctx.Path)
	if err != nil {
		return &base.Response{StatusCode: base.StatusInternalServerError}, err
	}
	s.mu.Lock()
	if _, ok := s.sessions[ctx.Session]; !ok {
		s.sessions[ctx.Session] = st
		st.readers.Add(1)
	}
	s.mu.Unlock()
	return &base.Response{StatusCode: base.StatusOK}, nil
}

// OnSessionClose implements gortsplib.ServerHandlerOnSessionClose.
func (s *Server) OnSessionClose(ctx *gortsplib.ServerHandlerOnSessionCloseCtx) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.sessions[ctx.Session]; ok {
		st.readers.Add(-1)
		delete(s.sessions, ctx.Session)
	}
}

// MJPEGHandler serves /<label>/<W>x<H> as an HTTP MJPEG stream (multipart/x-mixed-replace).
func MJPEGHandler(fps int) http.Handler {
	if fps <= 0 {
		fps = 10
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sp := parse(r.URL.Path)
		mw := multipart.NewWriter(w)
		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+mw.Boundary())
		tick := time.NewTicker(time.Second / time.Duration(fps))
		defer tick.Stop()
		for n := 0; ; n++ {
			var b bytes.Buffer
			if err := jpeg.Encode(&b, Picture(sp.label, sp.w, sp.h, time.Now(), n), &jpeg.Options{Quality: 75}); err != nil {
				return
			}
			pw, err := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {"image/jpeg"}, "Content-Length": {fmt.Sprint(b.Len())}})
			if err != nil {
				return
			}
			if _, err := pw.Write(b.Bytes()); err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			select {
			case <-r.Context().Done():
				return
			case <-tick.C:
			}
		}
	})
}

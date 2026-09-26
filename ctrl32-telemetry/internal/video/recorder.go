package video

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SegmentLength is the target length of one recorded file; a file always starts with a key frame, so the real
// length is rounded up to the camera's key frame interval.
var SegmentLength = 60 * time.Second

// recorder writes the main stream of one camera into segment files and indexes them in core.video_segment.
type recorder struct {
	m      *Manager
	camID  string
	src    *Source
	cancel context.CancelFunc
	done   chan struct{}
}

type segment struct {
	id      int64
	path    string // relative to the storage directory
	f       *os.File
	mux     *Muxer
	track   *Track
	first   time.Time
	last    time.Time
	bytes   int64
	flushed time.Time
}

func (m *Manager) startRecorder(camID string, src *Source) *recorder {
	ctx, cancel := context.WithCancel(context.Background())
	r := &recorder{m: m, camID: camID, src: src, cancel: cancel, done: make(chan struct{})}
	go r.run(ctx)
	return r
}

func (r *recorder) stop() {
	r.cancel()
	<-r.done
}

func (r *recorder) run(ctx context.Context) {
	defer close(r.done)
	sub := r.src.Subscribe()
	defer sub.Close()
	var seg *segment
	defer func() { r.close(seg) }()
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if seg != nil {
				r.index(seg)
			}
		case p, ok := <-sub.C:
			if !ok {
				return
			}
			s := p.Sample
			if seg != nil && (p.Track != seg.track || (s.Key && s.Time.Sub(seg.first) >= SegmentLength) || s.Time.Sub(seg.last) > 10*time.Second) {
				r.close(seg)
				seg = nil
			}
			if seg == nil {
				if !s.Key {
					continue
				}
				var err error
				if seg, err = r.open(p.Track, s); err != nil {
					r.m.log.Error("recording not started", "camera", r.camID, "err", err)
					continue
				}
			}
			frag, err := seg.mux.Push(s)
			if err == nil && frag != nil {
				err = seg.write(frag)
			}
			if err != nil {
				r.m.log.Error("recording write failed", "camera", r.camID, "err", err)
				r.close(seg)
				seg = nil
				continue
			}
			seg.last = s.Time
		}
	}
}

func (r *recorder) open(t *Track, s *Sample) (*segment, error) {
	utc := s.Time.UTC()
	rel := filepath.Join(r.camID, utc.Format("2006-01-02"), fmt.Sprintf("%s-%03d.mp4", utc.Format("150405"), utc.Nanosecond()/1e6))
	full := filepath.Join(r.m.dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(full, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return nil, err
	}
	seg := &segment{path: filepath.ToSlash(rel), f: f, mux: &Muxer{Track: t, Batch: ClockRate}, track: t, first: s.Time, last: s.Time, flushed: time.Now()}
	init, err := seg.mux.Init()
	if err == nil {
		err = seg.write(init)
	}
	if err == nil {
		err = r.m.pool.QueryRow(context.Background(), `INSERT INTO core.video_segment (camera_id, started_at, ended_at, path, bytes, codec, width, height)
			VALUES ($1::uuid, $2, $2, $3, $4, $5, $6, $7) RETURNING id`, r.camID, seg.first, seg.path, seg.bytes, t.Codec, t.Width, t.Height).Scan(&seg.id)
	}
	if err != nil {
		f.Close()
		os.Remove(full)
		return nil, err
	}
	return seg, nil
}

func (s *segment) write(b []byte) error {
	n, err := s.f.Write(b)
	s.bytes += int64(n)
	return err
}

// index updates the segment's end and size, so the recording up to now is visible and playable.
func (r *recorder) index(seg *segment) {
	end := seg.last
	if seg.mux.lastDur > 0 {
		end = end.Add(time.Duration(seg.mux.lastDur) * time.Second / ClockRate)
	}
	_, err := r.m.pool.Exec(context.Background(), `UPDATE core.video_segment SET ended_at = $2, bytes = $3 WHERE id = $1`, seg.id, end, seg.bytes)
	if err != nil {
		r.m.log.Warn("recording index", "camera", r.camID, "err", err)
	}
}

func (r *recorder) close(seg *segment) {
	if seg == nil {
		return
	}
	if frag, err := seg.mux.Flush(); err == nil && frag != nil {
		_ = seg.write(frag)
	}
	_ = seg.f.Close()
	r.index(seg)
}

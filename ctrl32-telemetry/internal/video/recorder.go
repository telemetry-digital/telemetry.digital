package video

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// maxSegment is the longest segment the settings allow; playback looks back this far for a file covering a time.
const maxSegment = 600 * time.Second

// recorder writes the main stream of one camera into segment files and indexes them in core.video_segment.
type recorder struct {
	m      *Manager
	camID  string
	src    *Source
	cancel context.CancelFunc
	done   chan struct{}

	// event recording: files are written only while armed; the last pre seconds are kept in memory so a file
	// starts before the event
	events bool
	pre    time.Duration
	mu     sync.Mutex
	holds  map[string]bool
	until  time.Time

	started time.Time
	written atomic.Int64 // unix nanoseconds of the last fragment written (health, spec 014)
}

// hold arms the recorder while an event lasts.
func (r *recorder) hold(key string, on bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if on {
		r.holds[key] = true
	} else {
		delete(r.holds, key)
	}
}

// extend arms the recorder at least until t.
func (r *recorder) extend(t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.After(r.until) {
		r.until = t
	}
}

func (r *recorder) armed(now time.Time) bool {
	if !r.events {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.holds) > 0 || now.Before(r.until)
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
	hash    hash.Hash // SHA-256 of everything written; stored when the file is closed (evidence, spec 015)
}

func (m *Manager) startRecorder(camID string, src *Source, events bool, pre time.Duration) *recorder {
	ctx, cancel := context.WithCancel(context.Background())
	r := &recorder{m: m, camID: camID, src: src, cancel: cancel, done: make(chan struct{}), events: events, pre: pre, holds: map[string]bool{}, started: time.Now()}
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
	var buf []Packet
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
			if r.events {
				armed := r.armed(time.Now())
				if seg == nil {
					buf = r.preroll(buf, p)
					if !armed || len(buf) == 0 {
						continue
					}
					// start the file with the buffered frames (from a key frame up to pre before now)
					for _, q := range buf[:len(buf)-1] {
						seg = r.write(seg, q)
					}
					buf = buf[:0]
				} else if !armed {
					r.close(seg)
					seg = nil
					buf = r.preroll(buf[:0], p)
					continue
				}
			}
			seg = r.write(seg, p)
		}
	}
}

// preroll keeps the frames of the last pre seconds, starting at a key frame.
func (r *recorder) preroll(buf []Packet, p Packet) []Packet {
	if len(buf) > 0 && buf[len(buf)-1].Track != p.Track {
		buf = buf[:0]
	}
	if len(buf) == 0 && !p.Sample.Key {
		return buf
	}
	buf = append(buf, p)
	cut := p.Sample.Time.Add(-r.pre)
	// drop GOPs that end before the cut: keep the last key frame at or before it
	last := 0
	for i, q := range buf {
		if q.Sample.Key && !q.Sample.Time.After(cut) {
			last = i
		}
	}
	if last > 0 {
		buf = append(buf[:0], buf[last:]...)
	}
	if len(buf) > 20000 { // safety net for streams without key frames
		buf = buf[len(buf)-20000:]
	}
	return buf
}

// write adds one frame, opening and rotating files as needed; it returns the current segment.
func (r *recorder) write(seg *segment, p Packet) *segment {
	s := p.Sample
	if seg != nil && (p.Track != seg.track || (s.Key && s.Time.Sub(seg.first) >= r.m.segmentLength()) || s.Time.Sub(seg.last) > 10*time.Second) {
		r.close(seg)
		seg = nil
	}
	if seg == nil {
		if !s.Key {
			return nil
		}
		var err error
		if seg, err = r.open(p.Track, s); err != nil {
			r.m.log.Error("recording not started", "camera", r.camID, "err", err)
			return nil
		}
	}
	frag, err := seg.mux.Push(s)
	if err == nil && frag != nil {
		if err = seg.write(frag); err == nil {
			r.written.Store(time.Now().UnixNano())
		}
	}
	if err != nil {
		r.m.log.Error("recording write failed", "camera", r.camID, "err", err)
		r.close(seg)
		return nil
	}
	seg.last = s.Time
	return seg
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
	seg := &segment{path: filepath.ToSlash(rel), f: f, mux: &Muxer{Track: t, Batch: ClockRate}, track: t, first: s.Time, last: s.Time, flushed: time.Now(), hash: sha256.New()}
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
	if s.hash != nil {
		s.hash.Write(b[:n])
	}
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
	// the fingerprint of the finished file: an evidence export later shows the file is unchanged since recording
	if _, err := r.m.pool.Exec(context.Background(), `UPDATE core.video_segment SET sha256 = $2 WHERE id = $1`, seg.id, hex.EncodeToString(seg.hash.Sum(nil))); err != nil {
		r.m.log.Warn("recording fingerprint", "camera", r.camID, "err", err)
	}
}

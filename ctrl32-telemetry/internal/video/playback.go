package video

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
)

// Playback reads recorded frames of one camera from a point in time on. Output timestamps run on without gaps across
// files (the time jumps over periods without recording); each Sample.Time is the real recording time.
type Playback struct {
	m     *Manager
	camID string
	from  time.Time
	to    time.Time // zero = up to the newest recording

	queue   []segRow
	after   time.Time
	afterID int64
	done    bool
	follow  bool      // no end time: wait for the recording to grow (timeshift up to the live edge)
	idle    time.Time // since when following found no new data

	fh      *os.File
	off     int64 // read position in the current file
	seg     segRow
	track   *Track
	outBase int64 // output DTS of the current file's start
	outEnd  int64 // output DTS after the last emitted sample
	pending []Packet
	started bool // reached `from`
	gop     []Packet
}

type segRow struct {
	id         int64
	start, end time.Time
	path       string
}

// Playback opens recorded video of a camera from `from` (to `to`, when not zero).
func (m *Manager) Playback(camID string, from, to time.Time) *Playback {
	return &Playback{m: m, camID: camID, from: from, to: to, after: from.Add(-2 * SegmentLength), follow: to.IsZero()}
}

// Close releases the open file.
func (p *Playback) Close() {
	if p.fh != nil {
		p.fh.Close()
		p.fh = nil
	}
}

// FollowLimit ends a following playback when the recording stops growing for this long.
var FollowLimit = 20 * time.Second

var errPartial = errors.New("the file is still being written")

// Next returns the next frame, or io.EOF after the last one. Without an end time it follows the recording as it is
// written: playback and recording are independent, and a paused or delayed viewer catches up to the live edge.
func (p *Playback) Next(ctx context.Context) (Packet, error) {
	for {
		if len(p.pending) > 0 {
			pk := p.pending[0]
			p.pending = p.pending[1:]
			if !p.to.IsZero() && pk.Sample.Time.After(p.to) {
				p.done, p.queue = true, nil
				p.Close()
				return Packet{}, io.EOF
			}
			p.idle = time.Time{}
			return pk, nil
		}
		if p.fh == nil {
			err := p.openNext(ctx)
			if errors.Is(err, io.EOF) && p.follow && p.waitMore(ctx) {
				p.done = false
				continue
			}
			if err != nil {
				return Packet{}, err
			}
			continue
		}
		err := p.readFragment()
		if err == nil {
			continue
		}
		if errors.Is(err, errPartial) || errors.Is(err, io.EOF) {
			// the end of this file so far: a newer file means it is complete, otherwise it may still grow
			if len(p.queue) == 0 && !p.done {
				if ferr := p.fetch(ctx); ferr != nil {
					return Packet{}, ferr
				}
			}
			if len(p.queue) > 0 || !p.follow {
				p.Close()
				continue
			}
			if !p.waitMore(ctx) {
				p.Close()
				return Packet{}, io.EOF
			}
			continue
		}
		p.m.log.Warn("recording unreadable", "camera", p.camID, "path", p.seg.path, "err", err)
		p.Close()
	}
}

// waitMore pauses while following; false when nothing new came for FollowLimit or ctx ended.
func (p *Playback) waitMore(ctx context.Context) bool {
	if p.idle.IsZero() {
		p.idle = time.Now()
	}
	if time.Since(p.idle) > FollowLimit {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(250 * time.Millisecond):
		return true
	}
}

func (p *Playback) openNext(ctx context.Context) error {
	for {
		if len(p.queue) == 0 {
			if p.done {
				return io.EOF
			}
			if err := p.fetch(ctx); err != nil {
				return err
			}
			if len(p.queue) == 0 {
				p.done = true
				return io.EOF
			}
		}
		p.seg = p.queue[0]
		p.queue = p.queue[1:]
		if p.seg.end.Before(p.from.Add(-15 * time.Second)) { // the index of the file being written lags up to 10 s
			continue
		}
		f, err := os.Open(filepath.Join(p.m.dir, filepath.FromSlash(p.seg.path)))
		if err != nil {
			continue // deleted by retention meanwhile
		}
		p.fh, p.off = f, 0
		t, err := p.readInit()
		if err != nil {
			p.Close()
			continue
		}
		if !t.SameParams(p.track) {
			p.track = t
		}
		p.outBase = p.outEnd
		return nil
	}
}

func (p *Playback) fetch(ctx context.Context) error {
	q := `SELECT id, started_at, ended_at, path FROM core.video_segment
		WHERE camera_id = $1::uuid AND (started_at, id) > ($2, $3) ORDER BY started_at, id LIMIT 30`
	rows, err := p.m.pool.Query(ctx, q, p.camID, p.after, p.afterID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var s segRow
		if err := rows.Scan(&s.id, &s.start, &s.end, &s.path); err != nil {
			return err
		}
		p.after, p.afterID = s.start, s.id
		if !p.to.IsZero() && s.start.After(p.to) {
			p.done = true
			break
		}
		p.queue = append(p.queue, s)
	}
	return rows.Err()
}

// readBox reads one complete top-level MP4 box at off; errPartial when the file does not hold all of it yet (the
// position does not move, so a later call reads it once the recorder has written it).
func readBox(f *os.File, off int64) (string, []byte, error) {
	var h [16]byte
	n, err := f.ReadAt(h[:8], off)
	if n < 8 {
		if err == nil || errors.Is(err, io.EOF) {
			return "", nil, errPartial
		}
		return "", nil, err
	}
	size := uint64(binary.BigEndian.Uint32(h[:4]))
	typ := string(h[4:8])
	hdrLen := 8
	if size == 1 {
		if n, _ := f.ReadAt(h[8:16], off+8); n < 8 {
			return "", nil, errPartial
		}
		size, hdrLen = binary.BigEndian.Uint64(h[8:16]), 16
	}
	if size < uint64(hdrLen) || size > 256<<20 {
		return "", nil, errors.New("bad box size")
	}
	buf := make([]byte, size)
	if n, err := f.ReadAt(buf, off); uint64(n) < size {
		if err == nil || errors.Is(err, io.EOF) {
			return "", nil, errPartial
		}
		return "", nil, err
	}
	return typ, buf, nil
}

func (p *Playback) readInit() (*Track, error) {
	var init []byte
	for {
		typ, b, err := readBox(p.fh, p.off)
		if err != nil {
			return nil, err
		}
		p.off += int64(len(b))
		init = append(init, b...)
		if typ == "moov" {
			break
		}
	}
	var in fmp4.Init
	if err := in.Unmarshal(bytes.NewReader(init)); err != nil {
		return nil, err
	}
	if len(in.Tracks) == 0 {
		return nil, errors.New("no track")
	}
	return trackFromMP4(in.Tracks[0].Codec)
}

// readFragment reads one moof + mdat pair and queues its frames.
func (p *Playback) readFragment() error {
	var frag []byte
	off := p.off
	for {
		typ, b, err := readBox(p.fh, off)
		if err != nil {
			return err // on errPartial p.off stays at the fragment's start
		}
		off += int64(len(b))
		if typ == "moof" {
			frag = b
			continue
		}
		if typ == "mdat" && frag != nil {
			frag = append(frag, b...)
			break
		}
	}
	p.off = off
	var parts fmp4.Parts
	if err := parts.Unmarshal(frag); err != nil {
		return err
	}
	for _, part := range parts {
		for _, pt := range part.Tracks {
			dts := int64(pt.BaseTime)
			for _, s := range pt.Samples {
				smp := &Sample{
					Time:      p.seg.start.Add(time.Duration(dts) * time.Second / ClockRate),
					DTS:       p.outBase + dts,
					PTSOffset: s.PTSOffset,
					Key:       !s.IsNonSyncSample,
				}
				if p.track.Codec == MJPEG {
					smp.JPEG = s.Payload
				} else {
					var err error
					if p.track.Codec == H264 {
						smp.AU, err = s.GetH264()
					} else {
						smp.AU, err = s.GetH265()
					}
					if err != nil {
						return err
					}
				}
				dts += int64(s.Duration)
				p.outEnd = p.outBase + dts
				p.emit(Packet{Track: p.track, Sample: smp})
			}
		}
	}
	return nil
}

// emit queues a frame; before `from` it keeps only the frames since the last key frame, so playback starts with a
// decodable picture just before the requested time.
func (p *Playback) emit(pk Packet) {
	if p.started {
		p.pending = append(p.pending, pk)
		return
	}
	if pk.Sample.Key {
		p.gop = p.gop[:0]
	}
	p.gop = append(p.gop, pk)
	if !pk.Sample.Time.Before(p.from) {
		p.started = true
		p.pending = append(p.pending, p.gop...)
		p.gop = nil
	}
}

package video

import (
	"bytes"
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
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
	vid     int // MP4 track ids of the picture and the sound (0 = none) in the current file
	aid     int
	outBase int64 // output DTS of the current file's start
	outEnd  int64 // output DTS after the last emitted sample
	pending []Packet
	started bool // reached `from`
	gop     []Packet
	looked  bool // searched for a long file covering `from`
}

// A file above seekMinSize is entered by bisection, to seekNear before the requested time.
const (
	seekMinSize = 1 << 30
	seekNear    = 64 << 20
)

type segRow struct {
	id         int64
	start, end time.Time
	path       string
}

// Playback opens recorded video of a camera from `from` (to `to`, when not zero).
func (m *Manager) Playback(camID string, from, to time.Time) *Playback {
	return &Playback{m: m, camID: camID, from: from, to: to, after: from.Add(-2 * maxSegment), follow: to.IsZero()}
}

// Close releases the open file.
func (p *Playback) Close() {
	if p.fh != nil {
		p.fh.Close()
		p.fh = nil
	}
}

// FollowLimit, when set (tests), overrides the timeshift setting: a following playback ends when the recording stops
// growing for this long.
var FollowLimit time.Duration

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

// waitMore pauses while following; false when nothing new came for the follow limit or ctx ended.
func (p *Playback) waitMore(ctx context.Context) bool {
	if p.idle.IsZero() {
		p.idle = time.Now()
	}
	if time.Since(p.idle) > p.m.followLimit() {
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
		p.seekLong()
		p.outBase = p.outEnd
		return nil
	}
}

// seekLong moves the read position of a very long file close before `from`: walking days of fragments from the start
// would take about an hour on a disk. Files of normal length are read from their start.
func (p *Playback) seekLong() {
	fi, err := p.fh.Stat()
	if err != nil || fi.Size() < seekMinSize {
		return
	}
	p.off = seekFragment(p.fh, p.off, fi.Size(), p.vid, durationToTicks(p.from.Sub(p.seg.start)), seekNear)
}

// seekFragment bisects a recorded file between off (the end of its initialization) and size for a fragment whose
// picture starts at or before the DTS want, at most about near bytes before the one that passes it. The result is a
// fragment's offset, or off when nothing better is known; readFragment continues from there.
func seekFragment(f *os.File, off, size int64, vid int, want, near int64) int64 {
	lo, hi := off, size
	for hi-lo > near {
		mid := lo + (hi-lo)/2
		at, dts, ok := fragmentAfter(f, mid, hi, vid)
		switch {
		case !ok || dts > want:
			hi = mid
		default:
			lo = at
		}
	}
	return lo
}

// fragmentAfter finds the first complete fragment with a picture at or after off, below end; it returns its offset and
// the DTS of its first picture. A file holds no index, so it looks for a moof box followed by an mdat box.
func fragmentAfter(f *os.File, off, end int64, vid int) (at, dts int64, ok bool) {
	buf := make([]byte, 4<<20)
	for off < end {
		n, _ := f.ReadAt(buf, off)
		if n < 8 {
			return 0, 0, false
		}
		for i := 0; ; {
			j := bytes.Index(buf[i:n], []byte("moof"))
			if j < 0 {
				break
			}
			cand := off + int64(i+j) - 4
			i += j + 4
			if cand < off || cand >= end {
				continue
			}
			typ, moof, err := readBox(f, cand)
			if err != nil || typ != "moof" {
				continue
			}
			typ, mdat, err := readBox(f, cand+int64(len(moof)))
			if err != nil || typ != "mdat" {
				continue
			}
			var parts fmp4.Parts
			if parts.Unmarshal(append(moof, mdat...)) != nil {
				continue
			}
			for _, part := range parts {
				for _, pt := range part.Tracks {
					if pt.ID == vid {
						return cand, int64(pt.BaseTime), true
					}
				}
			}
		}
		off += int64(n) - 8 // a box name split between two reads is found in the next one
	}
	return 0, 0, false
}

// findLong moves the start of the search back to a file that began long before `from` and still covers it.
func (p *Playback) findLong(ctx context.Context) error {
	p.looked = true
	var oldest *time.Time
	err := p.m.pool.QueryRow(ctx, `SELECT min(started_at) FROM core.video_segment WHERE camera_id = $1::uuid AND started_at <= $2 AND ended_at >= $3`,
		p.camID, p.after, p.from.Add(-15*time.Second)).Scan(&oldest)
	if err != nil {
		return err
	}
	if oldest != nil {
		p.after = oldest.Add(-time.Microsecond)
	}
	return nil
}

func (p *Playback) fetch(ctx context.Context) error {
	if !p.looked {
		if err := p.findLong(ctx); err != nil {
			return err
		}
	}
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
	t, vid, aid, err := trackFromInit(in.Tracks)
	if err != nil {
		return nil, err
	}
	p.vid, p.aid = vid, aid
	return t, nil
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
	var out []Packet
	for _, part := range parts {
		for _, pt := range part.Tracks {
			if pt.ID == p.aid && p.aid != 0 {
				// sound: its own time scale; the time line is the picture's (both start at the file's first picture)
				sr := int64(p.track.Audio.SampleRate)
				at := int64(pt.BaseTime)
				for _, s := range pt.Samples {
					dts := at * ClockRate / sr
					out = append(out, Packet{Track: p.track, Sample: &Sample{
						Time: p.seg.start.Add(ticksToDuration(dts)), DTS: p.outBase + dts, Audio: s.Payload, ADur: s.Duration}})
					at += int64(s.Duration)
				}
				continue
			}
			if pt.ID != p.vid {
				continue
			}
			dts := int64(pt.BaseTime)
			for _, s := range pt.Samples {
				smp := &Sample{
					Time:      p.seg.start.Add(ticksToDuration(dts)),
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
				p.outEnd = max(p.outEnd, p.outBase+dts)
				out = append(out, Packet{Track: p.track, Sample: smp})
			}
		}
	}
	// pictures and sound in time order (a fragment holds each track as one block); a key frame goes before sound
	// of the same instant, so a player starts with a picture
	slices.SortStableFunc(out, func(a, b Packet) int {
		if a.Sample.DTS != b.Sample.DTS {
			return cmp.Compare(a.Sample.DTS, b.Sample.DTS)
		}
		return cmp.Compare(btoi(a.Sample.Audio != nil), btoi(b.Sample.Audio != nil))
	})
	for _, pk := range out {
		if pk.Sample.Audio != nil && !p.started && len(p.gop) == 0 {
			continue // sound before the first picture
		}
		p.emit(pk)
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
	} else if len(p.gop) == 0 {
		return // after a seek: nothing can be decoded before the first key frame
	}
	p.gop = append(p.gop, pk)
	if !pk.Sample.Time.Before(p.from) {
		p.started = true
		p.pending = append(p.pending, p.gop...)
		p.gop = nil
	}
}

package video

import (
	"bufio"
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

	file    *bufio.Reader
	fh      *os.File
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
	return &Playback{m: m, camID: camID, from: from, to: to, after: from.Add(-2 * SegmentLength)}
}

// Close releases the open file.
func (p *Playback) Close() {
	if p.fh != nil {
		p.fh.Close()
		p.fh = nil
	}
}

// Next returns the next frame, or io.EOF after the last one.
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
			return pk, nil
		}
		if p.file == nil {
			if err := p.openNext(ctx); err != nil {
				return Packet{}, err
			}
			continue
		}
		if err := p.readFragment(); err != nil {
			p.Close()
			p.file = nil
			if !errors.Is(err, io.EOF) {
				p.m.log.Warn("recording unreadable", "camera", p.camID, "path", p.seg.path, "err", err)
			}
		}
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
		if p.seg.end.Before(p.from) {
			continue
		}
		f, err := os.Open(filepath.Join(p.m.dir, filepath.FromSlash(p.seg.path)))
		if err != nil {
			continue // deleted by retention meanwhile
		}
		p.fh, p.file = f, bufio.NewReaderSize(f, 256<<10)
		t, err := p.readInit()
		if err != nil {
			p.Close()
			p.file = nil
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

// readBox reads one top-level MP4 box.
func readBox(r io.Reader) (string, []byte, error) {
	var h [8]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return "", nil, err
	}
	size := uint64(binary.BigEndian.Uint32(h[:4]))
	typ := string(h[4:8])
	hdr := h[:]
	if size == 1 {
		var l [8]byte
		if _, err := io.ReadFull(r, l[:]); err != nil {
			return "", nil, err
		}
		size = binary.BigEndian.Uint64(l[:])
		hdr = append(hdr, l[:]...)
	}
	if size < uint64(len(hdr)) || size > 256<<20 {
		return "", nil, errors.New("bad box size")
	}
	buf := make([]byte, size)
	copy(buf, hdr)
	if _, err := io.ReadFull(r, buf[len(hdr):]); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return "", nil, io.EOF // the file is still being written
		}
		return "", nil, err
	}
	return typ, buf, nil
}

func (p *Playback) readInit() (*Track, error) {
	var init []byte
	for {
		typ, b, err := readBox(p.file)
		if err != nil {
			return nil, err
		}
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
	for {
		typ, b, err := readBox(p.file)
		if err != nil {
			return err
		}
		if typ == "moof" {
			frag = b
			continue
		}
		if typ == "mdat" && frag != nil {
			frag = append(frag, b...)
			break
		}
	}
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

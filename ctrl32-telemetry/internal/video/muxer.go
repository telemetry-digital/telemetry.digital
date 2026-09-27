package video

import (
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
)

// Muxer packs samples of one Track into fragmented MP4. A sample's duration is known only when the next one arrives,
// so output lags one frame behind. With Batch 0 every frame is its own fragment (live view); otherwise a fragment
// collects frames for about Batch ticks and always starts with a key frame (recordings, seeking).
//
// The sound of the track (spec 020) is the second MP4 track of recordings and exports. Players get it as a separate
// stream (AudioOnly) next to the picture (NoAudio), so a gap in the sound never stalls the picture.
type Muxer struct {
	Track     *Track
	Batch     int64 // in ClockRate ticks
	NoAudio   bool  // the picture only
	AudioOnly bool  // the sound only

	seq      uint32
	base     int64
	started  bool
	prev     *Sample
	lastDur  uint32
	pending  []*fmp4.Sample
	pendBase int64
	pendDur  int64

	apending []*fmp4.Sample
	apBase   int64 // audio time (samples) of the first pending audio frame
	anext    int64 // audio time of the next audio frame
	astarted bool
}

func (m *Muxer) withAudio() bool { return m.Track.Audio != nil && !m.NoAudio }

func (m *Muxer) audioID() int {
	if m.AudioOnly {
		return 1
	}
	return 2
}

// Init returns the MP4 initialization (ftyp + moov).
func (m *Muxer) Init() ([]byte, error) {
	var buf seekablebuffer.Buffer
	var init fmp4.Init
	if !m.AudioOnly {
		init.Tracks = append(init.Tracks, &fmp4.InitTrack{ID: 1, TimeScale: ClockRate, Codec: m.Track.mp4Codec()})
	}
	if m.withAudio() {
		init.Tracks = append(init.Tracks, &fmp4.InitTrack{ID: m.audioID(), TimeScale: uint32(m.Track.Audio.SampleRate), Codec: m.Track.Audio.mp4Codec()})
	}
	if err := init.Marshal(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Push adds a sample and returns a finished fragment (moof + mdat), or nil when nothing is complete yet.
func (m *Muxer) Push(s *Sample) ([]byte, error) {
	if s.Audio != nil {
		return m.pushAudio(s)
	}
	if m.AudioOnly {
		return nil, nil
	}
	if !m.started {
		m.started, m.base = true, s.DTS
	}
	var out []byte
	if m.prev != nil {
		d := s.DTS - m.prev.DTS
		if d <= 0 {
			d = 1
		}
		if d > 10*ClockRate { // a stall: do not stretch one frame over it
			d = int64(m.lastDur)
			if d == 0 {
				d = ClockRate / 10
			}
		}
		m.lastDur = uint32(d)
		var err error
		if out, err = m.add(m.prev, uint32(d), s.Key); err != nil {
			return nil, err
		}
	}
	m.prev = s
	return out, nil
}

// pushAudio queues a sound frame on the audio track's own clock; the sound follows the picture's time line (the
// first picture is time 0), and a gap of more than 0.2 s (lost packets) moves it when nothing is queued.
func (m *Muxer) pushAudio(s *Sample) ([]byte, error) {
	if !m.withAudio() || len(s.Audio) == 0 {
		return nil, nil
	}
	if !m.started {
		return nil, nil // sound before the first picture has no place on the time line
	}
	sr := int64(m.Track.Audio.SampleRate)
	at := (s.DTS - m.base) * sr / ClockRate
	if at < 0 {
		return nil, nil
	}
	switch {
	case !m.astarted:
		m.anext, m.astarted = at, true
	case len(m.apending) == 0 && at-m.anext > sr/5:
		m.anext = at // a gap in the sound; a late frame never moves the time back
	}
	if len(m.apending) == 0 {
		m.apBase = m.anext
	}
	dur := s.ADur
	if dur == 0 {
		dur = 1024
	}
	m.apending = append(m.apending, &fmp4.Sample{Duration: dur, Payload: s.Audio})
	m.anext += int64(dur)
	// alone, the sound goes out in small fragments (about 0.1 s) so a player hears it at once
	if m.AudioOnly && m.anext-m.apBase >= sr/10 {
		return m.fragment()
	}
	return nil, nil
}

// Start sets time 0 of a sound-only muxer to a picture's time, so the sound and the picture share one time line.
func (m *Muxer) Start(dts int64) {
	if !m.started {
		m.started, m.base = true, dts
	}
}

// Flush returns the last sample (with the previous duration) and any collected frames.
func (m *Muxer) Flush() ([]byte, error) {
	if m.prev == nil {
		return m.fragment()
	}
	d := m.lastDur
	if d == 0 {
		d = ClockRate / 10
	}
	p := m.prev
	m.prev = nil
	if _, err := m.add(p, d, false); err != nil {
		return nil, err
	}
	return m.fragment()
}

// Duration of the samples already packed, in ClockRate ticks from the first sample.
func (m *Muxer) Duration() int64 {
	if m.prev == nil {
		return 0
	}
	return m.prev.DTS - m.base
}

// Pos is where a sample will show in the player's timeline, in seconds.
func (m *Muxer) Pos(s *Sample) float64 {
	if !m.started {
		return float64(s.PTSOffset) / ClockRate
	}
	return float64(s.DTS-m.base+int64(s.PTSOffset)) / ClockRate
}

func (m *Muxer) add(s *Sample, dur uint32, nextKey bool) ([]byte, error) {
	fs := &fmp4.Sample{Duration: dur, PTSOffset: s.PTSOffset, IsNonSyncSample: !s.Key}
	var err error
	switch m.Track.Codec {
	case H264:
		err = fs.FillH264(s.PTSOffset, s.AU)
	case H265:
		err = fs.FillH265(s.PTSOffset, s.AU)
	default:
		fs.Payload = s.JPEG
	}
	if err != nil {
		return nil, err
	}
	if len(m.pending) == 0 {
		m.pendBase, m.pendDur = s.DTS-m.base, 0
	}
	m.pending = append(m.pending, fs)
	m.pendDur += int64(dur)
	// a batch ends before a key frame, so a recorded fragment starts with one; very long GOPs are cut at 4 × Batch
	if m.Batch == 0 || (nextKey && m.pendDur >= m.Batch) || m.pendDur >= 4*m.Batch {
		return m.fragment()
	}
	return nil, nil
}

func (m *Muxer) fragment() ([]byte, error) {
	if len(m.pending) == 0 && len(m.apending) == 0 {
		return nil, nil
	}
	m.seq++
	part := fmp4.Part{SequenceNumber: m.seq}
	if len(m.pending) > 0 {
		part.Tracks = append(part.Tracks, &fmp4.PartTrack{ID: 1, BaseTime: uint64(m.pendBase), Samples: m.pending})
	}
	if len(m.apending) > 0 {
		part.Tracks = append(part.Tracks, &fmp4.PartTrack{ID: m.audioID(), BaseTime: uint64(m.apBase), Samples: m.apending})
	}
	m.pending, m.apending = nil, nil
	var buf seekablebuffer.Buffer
	if err := part.Marshal(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

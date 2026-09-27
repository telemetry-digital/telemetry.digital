package video

import (
	"bytes"
	"testing"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/video/testsrc"
)

// A recording with sound holds both tracks; the sound's time line starts at the first picture; players get the picture
// and the sound as separate streams on the same time line.
func TestMuxerAudio(t *testing.T) {
	enc := &testsrc.Encoder{W: 64, H: 48}
	tr := &Track{Codec: H264, SPS: enc.SPS(), PPS: enc.PPS(), Audio: &AudioTrack{Codec: Opus, SampleRate: 48000, Channels: 1}}
	frame := enc.Frame()
	var samples []*Sample
	// sound starts 0.1 s before the first picture: that part has no place on the time line
	for i := 0; i < 60; i++ { // 1.2 s of 20 ms Opus frames
		samples = append(samples, &Sample{DTS: int64(i)*1800 - 9000, Audio: []byte{0xf8, 0xff, 0xfe}, ADur: 960})
	}
	for i := 0; i < 10; i++ { // 10 pictures, 0.1 s apart
		samples = append(samples, &Sample{DTS: int64(i) * 9000, Key: i == 0 || i == 5, AU: frame})
	}
	// in time order, pictures first at the same instant
	sortSamples(samples)

	rec := &Muxer{Track: tr, Batch: ClockRate / 2}
	var buf bytes.Buffer
	init, err := rec.Init()
	if err != nil {
		t.Fatal(err)
	}
	buf.Write(init)
	for _, s := range samples {
		frag, err := rec.Push(s)
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(frag)
	}
	frag, _ := rec.Flush()
	buf.Write(frag)

	var in fmp4.Init
	if err := in.Unmarshal(bytes.NewReader(init)); err != nil {
		t.Fatal(err)
	}
	back, vid, aid, err := trackFromInit(in.Tracks)
	if err != nil || vid != 1 || aid != 2 || !back.Audio.Same(tr.Audio) {
		t.Fatalf("init: %v vid %d aid %d audio %+v", err, vid, aid, back.Audio)
	}
	var parts fmp4.Parts
	if err := parts.Unmarshal(buf.Bytes()[len(init):]); err != nil {
		t.Fatal(err)
	}
	var nv, na int
	firstAudio := int64(-1)
	for _, p := range parts {
		for _, pt := range p.Tracks {
			if pt.ID == 1 {
				nv += len(pt.Samples)
			} else {
				if firstAudio < 0 {
					firstAudio = int64(pt.BaseTime)
				}
				na += len(pt.Samples)
			}
		}
	}
	if nv != 10 {
		t.Fatalf("pictures: %d", nv)
	}
	if na != 55 || firstAudio != 0 { // 5 frames before the first picture dropped; the sound starts at 0
		t.Fatalf("sound frames %d, first at %d", na, firstAudio)
	}

	// the player's streams: the picture alone and the sound alone
	pic := &Muxer{Track: tr, NoAudio: true}
	snd := &Muxer{Track: tr, AudioOnly: true}
	pi, _ := pic.Init()
	si, _ := snd.Init()
	var pin, sin fmp4.Init
	_ = pin.Unmarshal(bytes.NewReader(pi))
	_ = sin.Unmarshal(bytes.NewReader(si))
	if len(pin.Tracks) != 1 || !pin.Tracks[0].Codec.IsVideo() || len(sin.Tracks) != 1 || sin.Tracks[0].Codec.IsVideo() || sin.Tracks[0].TimeScale != 48000 {
		t.Fatalf("player inits: %d %d", len(pin.Tracks), len(sin.Tracks))
	}
	var sfr [][]byte
	for _, s := range samples {
		if s.Audio == nil {
			snd.Start(s.DTS)
			if f, _ := pic.Push(s); f != nil {
				if bytes.Contains(f, []byte{0xf8, 0xff, 0xfe}) {
					t.Fatal("sound in the picture stream")
				}
			}
			continue
		}
		if f, _ := snd.Push(s); f != nil {
			sfr = append(sfr, f)
		}
	}
	if len(sfr) < 5 {
		t.Fatalf("sound fragments: %d", len(sfr))
	}
	var sp fmp4.Parts
	_ = sp.Unmarshal(sfr[0])
	if sp[0].Tracks[0].ID != 1 || sp[0].Tracks[0].BaseTime != 0 {
		t.Fatalf("sound stream starts at %d", sp[0].Tracks[0].BaseTime)
	}
	if (&AudioTrack{Codec: AAC, Config: []byte{0x12, 0x10}, SampleRate: 44100, Channels: 2}).MIME() != `audio/mp4; codecs="mp4a.40.2"` {
		t.Fatal("AAC MIME")
	}
}

// G.711 becomes a 16-bit PCM track (ipcm) that reads back as PCM.
func TestPCMTrack(t *testing.T) {
	enc := &testsrc.Encoder{W: 64, H: 48}
	tr := &Track{Codec: H264, SPS: enc.SPS(), PPS: enc.PPS(), Audio: &AudioTrack{Codec: PCM, SampleRate: 8000, Channels: 1}}
	m := &Muxer{Track: tr, Batch: ClockRate}
	init, err := m.Init()
	if err != nil {
		t.Fatal(err)
	}
	var in fmp4.Init
	if err := in.Unmarshal(bytes.NewReader(init)); err != nil {
		t.Fatal(err)
	}
	back, _, aid, err := trackFromInit(in.Tracks)
	if err != nil || aid != 2 || !back.Audio.Same(tr.Audio) {
		t.Fatalf("PCM track: %v %d %+v", err, aid, back.Audio)
	}
	if tr.Audio.MIME() != "" {
		t.Fatal("PCM is not for Media Source Extensions")
	}
	m.Push(&Sample{DTS: 0, Key: true, AU: enc.Frame()})
	m.Push(&Sample{DTS: 0, Audio: make([]byte, 320), ADur: 160})
	m.Push(&Sample{DTS: 9000, Key: true, AU: enc.Frame()})
	frag, err := m.Flush()
	var parts fmp4.Parts
	if err != nil || parts.Unmarshal(frag) != nil {
		t.Fatalf("fragment: %v", err)
	}
	found := false
	for _, pt := range parts[0].Tracks {
		if pt.ID == 2 && len(pt.Samples) == 1 && pt.Samples[0].Duration == 160 && len(pt.Samples[0].Payload) == 320 {
			found = true
		}
	}
	if !found {
		t.Fatal("PCM samples not in the fragment")
	}
}

func sortSamples(s []*Sample) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && (s[j].DTS < s[j-1].DTS || (s[j].DTS == s[j-1].DTS && s[j].Audio == nil && s[j-1].Audio != nil)); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

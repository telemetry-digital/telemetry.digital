package video

// The sound of a camera (spec 020). AAC and Opus go unchanged into the recordings and to the browser, which plays
// both. G.711 (A-law, µ-law — TP-Link Tapo and many other cameras cannot send anything else) is expanded to 16-bit PCM
// with a table lookup (no encoder involved): it is recorded as an uncompressed track (ipcm) and the browser plays it
// through WebAudio. Other codecs (G.726, LATM-framed AAC) are named in the status.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	g711codec "github.com/bluenviron/mediacommon/v2/pkg/codecs/g711"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/opus"
	"github.com/pion/rtp"
)

// audioDecoder picks the sound of a session. It returns no media and a note when there is none or it cannot be
// played; the picture works either way.
func (s *Source) audioDecoder(ctx context.Context, desc *description.Session, ptsOf func(*description.Media, *rtp.Packet) (int64, bool)) (*description.Media, format.Format, func(*rtp.Packet), string) {
	var (
		aac  *format.MPEG4Audio
		op   *format.Opus
		g711 *format.G711
	)
	switch {
	case desc.FindFormat(&aac) != nil && aac.Config != nil:
		medi := desc.FindFormat(&aac)
		dec, err := aac.CreateDecoder()
		if err != nil {
			return nil, nil, nil, "sound: " + err.Error()
		}
		cfg, err := aac.Config.Marshal()
		if err != nil {
			return nil, nil, nil, "sound: " + err.Error()
		}
		sr := aac.Config.SampleRate
		if sr <= 0 {
			return nil, nil, nil, "sound: unknown sample rate"
		}
		s.setAudio(&AudioTrack{Codec: AAC, Config: cfg, SampleRate: sr, Channels: aac.Config.ChannelCount})
		return medi, aac, func(pkt *rtp.Packet) {
			pts, ok := ptsOf(medi, pkt)
			if !ok {
				return
			}
			aus, err := dec.Decode(pkt)
			if err != nil {
				return
			}
			for i, au := range aus {
				t := pts + int64(i)*1024
				s.publishAudio(ctx, &Sample{Time: time.Now(), DTS: t * ClockRate / int64(sr), Audio: au, ADur: 1024})
			}
		}, fmt.Sprintf("AAC %d Hz", sr)
	case desc.FindFormat(&op) != nil:
		medi := desc.FindFormat(&op)
		dec, err := op.CreateDecoder()
		if err != nil {
			return nil, nil, nil, "sound: " + err.Error()
		}
		ch := max(1, op.ChannelCount)
		s.setAudio(&AudioTrack{Codec: Opus, SampleRate: 48000, Channels: ch})
		return medi, op, func(pkt *rtp.Packet) {
			pts, ok := ptsOf(medi, pkt)
			if !ok {
				return
			}
			p, err := dec.Decode(pkt)
			if err != nil || len(p) == 0 {
				return
			}
			d := opus.PacketDuration2(p)
			if d <= 0 {
				d = 960
			}
			s.publishAudio(ctx, &Sample{Time: time.Now(), DTS: pts * ClockRate / 48000, Audio: p, ADur: uint32(d)})
		}, "Opus"
	case desc.FindFormat(&g711) != nil && g711.SampleRate > 0:
		medi := desc.FindFormat(&g711)
		dec, err := g711.CreateDecoder()
		if err != nil {
			return nil, nil, nil, "sound: " + err.Error()
		}
		sr, ch := g711.SampleRate, max(1, g711.ChannelCount)
		s.setAudio(&AudioTrack{Codec: PCM, SampleRate: sr, Channels: ch})
		mu := g711.MULaw
		return medi, g711, func(pkt *rtp.Packet) {
			pts, ok := ptsOf(medi, pkt)
			if !ok {
				return
			}
			enc, err := dec.Decode(pkt)
			if err != nil || len(enc) == 0 {
				return
			}
			var pcm []byte // 16-bit big-endian samples
			if mu {
				var m g711codec.Mulaw
				m.Unmarshal(enc)
				pcm = m
			} else {
				var a g711codec.Alaw
				a.Unmarshal(enc)
				pcm = a
			}
			s.publishAudio(ctx, &Sample{Time: time.Now(), DTS: pts * ClockRate / int64(sr), Audio: pcm, ADur: uint32(len(enc) / ch)})
		}, fmt.Sprintf("G.711 %d Hz (as PCM)", sr)
	}
	for _, m := range desc.Medias {
		if m.Type != description.MediaTypeAudio {
			continue
		}
		for _, f := range m.Formats {
			return nil, nil, nil, "sound " + strings.ToUpper(f.Codec()) + " is not supported: choose AAC or G.711 in the camera's audio settings"
		}
	}
	return nil, nil, nil, "the camera sends no sound"
}

// withSound prepares the sound of a new connection when the camera should deliver it.
func (s *Source) withSound(ctx context.Context, desc *description.Session, ptsOf func(*description.Media, *rtp.Packet) (int64, bool)) (*description.Media, format.Format, func(*rtp.Packet)) {
	s.mu.Lock()
	s.audio, s.aInfo = nil, ""
	want := s.Audio
	s.mu.Unlock()
	if !want {
		return nil, nil, nil
	}
	m, f, on, info := s.audioDecoder(ctx, desc, ptsOf)
	s.soundNote(info)
	if m == nil {
		s.setAudio(nil)
	}
	return m, f, on
}

func (s *Source) soundNote(info string) {
	s.mu.Lock()
	s.aInfo = info
	s.status.Audio = info
	s.mu.Unlock()
}

// setAudio remembers the sound of the current connection; the next picture track carries it.
func (s *Source) setAudio(a *AudioTrack) {
	s.mu.Lock()
	s.audio = a
	s.mu.Unlock()
}

// publishAudio hands a sound frame to the subscribers of the current picture track.
func (s *Source) publishAudio(ctx context.Context, smp *Sample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel == nil || ctx.Err() != nil || s.track == nil || s.track.Audio == nil {
		return
	}
	// new viewers get the sound since the last key frame together with the pictures
	if len(s.gop) > 0 && s.gopLen < 64<<20 && len(s.gop) < 3000 {
		s.gop = append(s.gop, smp)
		s.gopLen += smp.Size()
	}
	for sub := range s.subs {
		if sub.lagging {
			continue
		}
		select {
		case sub.C <- Packet{Track: s.track, Sample: smp}:
		default:
			sub.lagging = true
		}
	}
}

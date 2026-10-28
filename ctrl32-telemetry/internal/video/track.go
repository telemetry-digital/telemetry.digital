// Package video receives camera streams (RTSP with H.264, H.265 or MJPEG; HTTP MJPEG), records them into one-minute
// fragmented MP4 files and serves live and recorded video to browsers (spec 010). No external encoder is needed: the
// server never decodes or re-encodes video, it only repackages what the camera sends.
package video

import (
	"bytes"
	"fmt"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
)

// Codec names.
const (
	H264  = "h264"
	H265  = "h265"
	MJPEG = "mjpeg"
)

// ClockRate of all timestamps (the RTP video clock).
const ClockRate = 90000

// Track describes the video of one connection or one recorded file. A new connection always yields a new Track, so a
// consumer knows it must start over (new MP4 initialization, new recording file).
type Track struct {
	Codec         string
	VPS, SPS, PPS []byte
	Width, Height int
	Audio         *AudioTrack // the camera's sound, when it is enabled and supported (spec 020)
}

// Audio codec names.
const (
	AAC  = "aac"
	Opus = "opus"
	PCM  = "pcm" // 16-bit big-endian samples (from G.711); browsers play it through WebAudio, not MSE
)

// AudioTrack describes the sound of a connection: AAC (MPEG-4 Audio) or Opus, both playable in browsers and stored
// unchanged as the second track of the MP4 files.
type AudioTrack struct {
	Codec      string
	Config     []byte // AAC AudioSpecificConfig
	SampleRate int    // also the MP4 timescale of the track
	Channels   int
}

// Same reports whether two audio descriptions can continue one MP4 stream.
func (a *AudioTrack) Same(o *AudioTrack) bool {
	if a == nil || o == nil {
		return a == o
	}
	return a.Codec == o.Codec && bytes.Equal(a.Config, o.Config) && a.SampleRate == o.SampleRate && a.Channels == o.Channels
}

// MIME is the Media Source Extensions type of the sound alone, e.g. `audio/mp4; codecs="mp4a.40.2"`.
func (a *AudioTrack) MIME() string {
	if a == nil {
		return ""
	}
	switch a.Codec {
	case Opus:
		return `audio/mp4; codecs="opus"`
	case PCM:
		return ""
	}
	var c mpeg4audio.AudioSpecificConfig
	ot := 2
	if c.Unmarshal(a.Config) == nil {
		ot = int(c.Type)
	}
	return fmt.Sprintf(`audio/mp4; codecs="mp4a.40.%d"`, ot)
}

func (a *AudioTrack) mp4Codec() codecs.Codec {
	switch a.Codec {
	case Opus:
		return &codecs.Opus{ChannelCount: a.Channels}
	case PCM:
		return &codecs.LPCM{BitDepth: 16, SampleRate: a.SampleRate, ChannelCount: a.Channels}
	}
	var c mpeg4audio.AudioSpecificConfig
	_ = c.Unmarshal(a.Config)
	return &codecs.MPEG4Audio{Config: c}
}

// Sample is one video frame, or one audio frame when Audio is set. DTS runs at ClockRate and grows monotonically within one Track.
type Sample struct {
	Time      time.Time // wall clock of the frame
	DTS       int64
	PTSOffset int32
	Key       bool
	AU        [][]byte // H.264/H.265 NAL units
	JPEG      []byte   // MJPEG picture
	Audio     []byte   // one AAC access unit or Opus packet; Key is false, DTS at ClockRate
	ADur      uint32   // duration of an audio frame in samples of the audio track
}

// Size is the payload size in bytes.
func (s *Sample) Size() int {
	n := len(s.JPEG) + len(s.Audio)
	for _, u := range s.AU {
		n += len(u)
	}
	return n
}

// SameParams reports whether two tracks can continue one MP4 stream.
func (t *Track) SameParams(o *Track) bool {
	return t != nil && o != nil && t.Codec == o.Codec && bytes.Equal(t.SPS, o.SPS) && bytes.Equal(t.PPS, o.PPS) &&
		bytes.Equal(t.VPS, o.VPS) && t.Width == o.Width && t.Height == o.Height && t.Audio.Same(o.Audio)
}

// Continues reports whether the new parameter sets o of the same connection can go on under t without the consumers
// starting over: same codec, picture size and MSE type. Cameras rewrite their SPS/PPS now and then (frame rate and
// timing in an idle-scene mode, 0.85.0); the new sets travel in front of every key frame (withParams), so live
// players, recordings and the detector decode them in-band while the stream (and its MP4 initialization) stays.
func (t *Track) Continues(o *Track) bool {
	return t != nil && o != nil && len(t.SPS) > 0 && t.Codec == o.Codec && (t.Codec == H264 || t.Codec == H265) &&
		t.Width == o.Width && t.Height == o.Height && t.Width > 0 && t.MIME() == o.MIME()
}

// Browser reports whether browsers can play the track through Media Source Extensions (MJPEG is shown as pictures).
func (t *Track) Browser() bool { return t.Codec == H264 || t.Codec == H265 }

// MIME is the Media Source Extensions type of the track, e.g. `video/mp4; codecs="avc1.42c01e"`.
func (t *Track) MIME() string {
	switch t.Codec {
	case H264:
		if len(t.SPS) >= 4 {
			return fmt.Sprintf(`video/mp4; codecs="avc1.%02x%02x%02x"`, t.SPS[1], t.SPS[2], t.SPS[3])
		}
	case H265:
		var sps h265.SPS
		if sps.Unmarshal(t.SPS) == nil {
			p := sps.ProfileTierLevel
			var compat uint32
			for i, f := range p.GeneralProfileCompatibilityFlag {
				if f {
					compat |= 1 << uint(i) // reversed bit order, as RFC 6381 wants it
				}
			}
			tier := "L"
			if p.GeneralTierFlag == 1 {
				tier = "H"
			}
			return fmt.Sprintf(`video/mp4; codecs="hvc1.%d.%X.%s%d.B0"`, p.GeneralProfileIdc, compat, tier, p.GeneralLevelIdc)
		}
		return `video/mp4; codecs="hvc1.1.6.L93.B0"`
	}
	return ""
}

// size fills Width and Height from the parameter sets.
func (t *Track) size() {
	switch t.Codec {
	case H264:
		var sps h264.SPS
		if sps.Unmarshal(t.SPS) == nil {
			t.Width, t.Height = sps.Width(), sps.Height()
		}
	case H265:
		var sps h265.SPS
		if sps.Unmarshal(t.SPS) == nil {
			t.Width, t.Height = sps.Width(), sps.Height()
		}
	}
}

func (t *Track) mp4Codec() codecs.Codec {
	switch t.Codec {
	case H264:
		return &codecs.H264{SPS: t.SPS, PPS: t.PPS}
	case H265:
		return &codecs.H265{VPS: t.VPS, SPS: t.SPS, PPS: t.PPS}
	default:
		return &codecs.MJPEG{Width: t.Width, Height: t.Height}
	}
}

// trackFromInit rebuilds a Track from a recorded file's initialization: the video track and, if present, the sound.
func trackFromInit(tracks []*fmp4.InitTrack) (*Track, int, int, error) {
	vid, aid := 0, 0
	var t *Track
	for _, it := range tracks {
		if it.Codec.IsVideo() && t == nil {
			nt, err := trackFromMP4(it.Codec)
			if err != nil {
				return nil, 0, 0, err
			}
			t, vid = nt, it.ID
		}
	}
	if t == nil {
		return nil, 0, 0, fmt.Errorf("no video track")
	}
	for _, it := range tracks {
		switch c := it.Codec.(type) {
		case *codecs.MPEG4Audio:
			cfg, err := c.Config.Marshal()
			if err == nil {
				t.Audio, aid = &AudioTrack{Codec: AAC, Config: cfg, SampleRate: int(it.TimeScale), Channels: c.Config.ChannelCount}, it.ID
			}
		case *codecs.Opus:
			t.Audio, aid = &AudioTrack{Codec: Opus, SampleRate: int(it.TimeScale), Channels: c.ChannelCount}, it.ID
		case *codecs.LPCM:
			if c.BitDepth == 16 && !c.LittleEndian {
				t.Audio, aid = &AudioTrack{Codec: PCM, SampleRate: int(it.TimeScale), Channels: c.ChannelCount}, it.ID
			}
		}
		if aid != 0 {
			break
		}
	}
	return t, vid, aid, nil
}

// trackFromMP4 rebuilds a video Track from a recorded file's initialization.
func trackFromMP4(c codecs.Codec) (*Track, error) {
	switch c := c.(type) {
	case *codecs.H264:
		t := &Track{Codec: H264, SPS: c.SPS, PPS: c.PPS}
		t.size()
		return t, nil
	case *codecs.H265:
		t := &Track{Codec: H265, VPS: c.VPS, SPS: c.SPS, PPS: c.PPS}
		t.size()
		return t, nil
	case *codecs.MJPEG:
		return &Track{Codec: MJPEG, Width: c.Width, Height: c.Height}, nil
	}
	return nil, fmt.Errorf("unsupported codec %T", c)
}

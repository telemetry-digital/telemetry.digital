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
}

// Sample is one video frame. DTS runs at ClockRate and grows monotonically within one Track.
type Sample struct {
	Time      time.Time // wall clock of the frame
	DTS       int64
	PTSOffset int32
	Key       bool
	AU        [][]byte // H.264/H.265 NAL units
	JPEG      []byte   // MJPEG picture
}

// Size is the payload size in bytes.
func (s *Sample) Size() int {
	n := len(s.JPEG)
	for _, u := range s.AU {
		n += len(u)
	}
	return n
}

// SameParams reports whether two tracks can continue one MP4 stream.
func (t *Track) SameParams(o *Track) bool {
	return t != nil && o != nil && t.Codec == o.Codec && bytes.Equal(t.SPS, o.SPS) && bytes.Equal(t.PPS, o.PPS) &&
		bytes.Equal(t.VPS, o.VPS) && t.Width == o.Width && t.Height == o.Height
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

// trackFromMP4 rebuilds a Track from a recorded file's initialization.
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

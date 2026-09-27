package video

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/pion/rtp"
)

// rtsp plays the camera's first video track until the connection fails or the context ends.
func (s *Source) rtsp(ctx context.Context) error {
	u, err := base.ParseURL(s.URL.String())
	if err != nil {
		return err
	}
	c := &gortsplib.Client{Scheme: u.Scheme, Host: u.Host, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, UDPReadBufferSize: 4 << 20}
	switch s.Transport {
	case "tcp":
		p := gortsplib.ProtocolTCP
		c.Protocol = &p
	case "udp":
		p := gortsplib.ProtocolUDP
		c.Protocol = &p
	}
	if err := c.Start(); err != nil {
		return err
	}
	defer c.Close()
	stop := context.AfterFunc(ctx, c.Close)
	defer stop()

	desc, _, err := c.Describe(u)
	if err != nil {
		return err
	}
	medi, forma, onPacket, err := s.decoder(ctx, desc, c.PacketPTS)
	if err != nil {
		return err
	}
	if _, err := c.Setup(desc.BaseURL, medi, 0, 0); err != nil {
		return err
	}
	c.OnPacketRTP(medi, forma, onPacket)
	if am, af, aon := s.withSound(ctx, desc, c.PacketPTS); am != nil {
		if _, err := c.Setup(desc.BaseURL, am, 0, 0); err == nil {
			c.OnPacketRTP(am, af, aon)
		} else {
			s.setAudio(nil)
			s.soundNote("sound: " + err.Error())
		}
	}
	if _, err := c.Play(nil); err != nil {
		return err
	}
	return c.Wait()
}

// decoder picks the first video track of a session (H.264, H.265, then MJPEG) and returns the RTP handler that turns
// its packets into samples. ptsOf gives the presentation time of a packet (client or server session).
func (s *Source) decoder(ctx context.Context, desc *description.Session, ptsOf func(*description.Media, *rtp.Packet) (int64, bool)) (*description.Media, format.Format, func(*rtp.Packet), error) {
	var (
		f264  *format.H264
		f265  *format.H265
		fjpeg *format.MJPEG
	)
	var medi *description.Media
	var forma format.Format
	var onPacket func(pkt *rtp.Packet)
	started := time.Now()
	wall := func(dts int64) time.Time { return started.Add(time.Duration(dts) * time.Second / ClockRate) }

	switch {
	case desc.FindFormat(&f264) != nil:
		medi, forma = desc.FindFormat(&f264), f264
		dec, err := f264.CreateDecoder()
		if err != nil {
			return nil, nil, nil, err
		}
		t := &Track{Codec: H264, SPS: f264.SPS, PPS: f264.PPS}
		t.size()
		var dtsx *h264.DTSExtractor
		onPacket = func(pkt *rtp.Packet) {
			pts, ok := ptsOf(medi, pkt)
			if !ok {
				return
			}
			au, err := dec.Decode(pkt)
			if err != nil {
				return
			}
			nt, out := h264Params(t, au)
			if nt != t {
				t, dtsx = nt, nil
			}
			if len(t.SPS) == 0 || len(t.PPS) == 0 || len(out) == 0 {
				return
			}
			key := h264.IsRandomAccess(out)
			if dtsx == nil {
				if !key {
					return
				}
				dtsx = h264.NewDTSExtractor()
			}
			out = withParams(key, t, out)
			dts, err := dtsx.Extract(out, pts)
			if err != nil {
				dtsx = nil
				return
			}
			s.publish(ctx, t, &Sample{Time: wall(dts), DTS: dts, PTSOffset: int32(pts - dts), Key: key, AU: out})
		}
	case desc.FindFormat(&f265) != nil:
		medi, forma = desc.FindFormat(&f265), f265
		dec, err := f265.CreateDecoder()
		if err != nil {
			return nil, nil, nil, err
		}
		t := &Track{Codec: H265, VPS: f265.VPS, SPS: f265.SPS, PPS: f265.PPS}
		t.size()
		var dtsx *h265.DTSExtractor
		onPacket = func(pkt *rtp.Packet) {
			pts, ok := ptsOf(medi, pkt)
			if !ok {
				return
			}
			au, err := dec.Decode(pkt)
			if err != nil {
				return
			}
			nt, out := h265Params(t, au)
			if nt != t {
				t, dtsx = nt, nil
			}
			if len(t.VPS) == 0 || len(t.SPS) == 0 || len(t.PPS) == 0 || len(out) == 0 {
				return
			}
			key := h265.IsRandomAccess(out)
			if dtsx == nil {
				if !key {
					return
				}
				dtsx = h265.NewDTSExtractor()
			}
			out = withParams(key, t, out)
			dts, err := dtsx.Extract(out, pts)
			if err != nil {
				dtsx = nil
				return
			}
			s.publish(ctx, t, &Sample{Time: wall(dts), DTS: dts, PTSOffset: int32(pts - dts), Key: key, AU: out})
		}
	case desc.FindFormat(&fjpeg) != nil:
		medi, forma = desc.FindFormat(&fjpeg), fjpeg
		dec, err := fjpeg.CreateDecoder()
		if err != nil {
			return nil, nil, nil, err
		}
		var t *Track
		onPacket = func(pkt *rtp.Packet) {
			pts, ok := ptsOf(medi, pkt)
			if !ok {
				return
			}
			img, err := dec.Decode(pkt)
			if err != nil {
				return
			}
			if t == nil {
				t = jpegTrack(img)
				if t == nil {
					return
				}
			}
			s.publish(ctx, t, &Sample{Time: wall(pts), DTS: pts, Key: true, JPEG: img})
		}
	default:
		return nil, nil, nil, errors.New("the camera offers no H.264, H.265 or MJPEG video")
	}
	return medi, forma, onPacket, nil
}

func jpegTrack(img []byte) *Track {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(img))
	if err != nil {
		return nil
	}
	return &Track{Codec: MJPEG, Width: cfg.Width, Height: cfg.Height}
}

// h264Params takes parameter sets out of an access unit; a change yields a new Track. Delimiters are dropped.
func h264Params(t *Track, au [][]byte) (*Track, [][]byte) {
	out := au[:0:0]
	sps, pps := t.SPS, t.PPS
	for _, n := range au {
		if len(n) == 0 {
			continue
		}
		switch h264.NALUType(n[0] & 0x1f) {
		case h264.NALUTypeSPS:
			sps = n
		case h264.NALUTypePPS:
			pps = n
		case h264.NALUTypeAccessUnitDelimiter:
		default:
			out = append(out, n)
		}
	}
	if !bytes.Equal(sps, t.SPS) || !bytes.Equal(pps, t.PPS) {
		nt := &Track{Codec: H264, SPS: bytes.Clone(sps), PPS: bytes.Clone(pps)}
		nt.size()
		if t.SameParams(nt) {
			return t, out
		}
		return nt, out
	}
	return t, out
}

func h265Params(t *Track, au [][]byte) (*Track, [][]byte) {
	out := au[:0:0]
	vps, sps, pps := t.VPS, t.SPS, t.PPS
	for _, n := range au {
		if len(n) < 2 {
			continue
		}
		switch h265.NALUType((n[0] >> 1) & 0x3f) {
		case h265.NALUType_VPS_NUT:
			vps = n
		case h265.NALUType_SPS_NUT:
			sps = n
		case h265.NALUType_PPS_NUT:
			pps = n
		case h265.NALUType_AUD_NUT:
		default:
			out = append(out, n)
		}
	}
	if !bytes.Equal(vps, t.VPS) || !bytes.Equal(sps, t.SPS) || !bytes.Equal(pps, t.PPS) {
		nt := &Track{Codec: H265, VPS: bytes.Clone(vps), SPS: bytes.Clone(sps), PPS: bytes.Clone(pps)}
		nt.size()
		return nt, out
	}
	return t, out
}

// withParams puts the parameter sets in front of key frames, so every recorded fragment decodes on its own.
func withParams(key bool, t *Track, au [][]byte) [][]byte {
	if !key {
		return au
	}
	switch t.Codec {
	case H264:
		return append([][]byte{t.SPS, t.PPS}, au...)
	case H265:
		return append([][]byte{t.VPS, t.SPS, t.PPS}, au...)
	}
	return au
}

var errNotMultipart = fmt.Errorf("not a multipart MJPEG stream")

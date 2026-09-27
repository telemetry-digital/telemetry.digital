package video

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
)

// RelayConfig is the configuration of `ctrl32-telemetry camera-relay` at a remote site (relay.toml).
type RelayConfig struct {
	Server  string        `toml:"server"`  // rtsps://telemetry.example.com:8322 (rtsp:// only inside a trusted network)
	CAFile  string        `toml:"ca_file"` // PEM of a private CA; empty = the system's trusted CAs
	Cameras []RelayCamera `toml:"camera"`
}

// RelayCamera is one camera the relay forwards.
type RelayCamera struct {
	ID        string `toml:"id"`         // camera id on the server
	Secret    string `toml:"secret"`     // the camera's push secret (shown once when the camera is created)
	Source    string `toml:"source"`     // rtsp://user:pw@192.168.1.64/… main stream of the local camera
	SubSource string `toml:"sub_source"` // optional smaller stream for tiles
	Transport string `toml:"transport"`  // auto, tcp, udp (towards the local camera)
	Audio     bool   `toml:"audio"`      // forward the camera's sound too (AAC or Opus; main stream only)
}

// Validate checks the configuration before the relay starts.
func (c *RelayConfig) Validate() error {
	u, err := url.Parse(c.Server)
	if err != nil || (u.Scheme != "rtsps" && u.Scheme != "rtsp") || u.Host == "" {
		return errors.New("server must be rtsps://host:port (or rtsp:// inside a trusted network)")
	}
	if len(c.Cameras) == 0 {
		return errors.New("no [[camera]] configured")
	}
	for i, cam := range c.Cameras {
		if !pushPath.MatchString("/"+cam.ID) || strings.Contains(cam.ID, "/") {
			return fmt.Errorf("camera %d: id must be the camera id shown on the server", i+1)
		}
		if len(cam.Secret) < 16 {
			return fmt.Errorf("camera %d: secret missing", i+1)
		}
		for _, s := range []string{cam.Source, cam.SubSource} {
			if s == "" {
				continue
			}
			if su, err := url.Parse(s); err != nil || (su.Scheme != "rtsp" && su.Scheme != "rtsps") {
				return fmt.Errorf("camera %d: source must be an rtsp:// address of the local camera", i+1)
			}
		}
		if cam.Source == "" {
			return fmt.Errorf("camera %d: source missing", i+1)
		}
	}
	return nil
}

// RunRelay forwards every configured stream until ctx ends, reconnecting each on its own.
func RunRelay(ctx context.Context, cfg RelayConfig, log *slog.Logger) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	tlsCfg, err := relayTLS(cfg)
	if err != nil {
		return err
	}
	if tlsCfg == nil {
		log.Warn("relay without TLS: use rtsps:// outside a trusted network")
	}
	var wg sync.WaitGroup
	for _, cam := range cfg.Cameras {
		for _, sub := range []bool{false, true} {
			src := cam.Source
			if sub {
				src = cam.SubSource
			}
			if src == "" {
				continue
			}
			target := strings.TrimRight(cfg.Server, "/") + "/" + cam.ID
			if sub {
				target += "/sub"
			}
			wg.Add(1)
			go func(cam RelayCamera, src, target string, sub bool) {
				defer wg.Done()
				l := log.With("camera", cam.ID, "sub", sub)
				backoff := time.Second
				for ctx.Err() == nil {
					began := time.Now()
					err := relayOnce(ctx, src, cam.Transport, target, cam.ID, cam.Secret, tlsCfg, cam.Audio && !sub, l)
					if ctx.Err() != nil {
						return
					}
					l.Warn("relay stream down", "err", redactAll(err, src, cam.Secret))
					if time.Since(began) > time.Minute {
						backoff = time.Second
					}
					select {
					case <-ctx.Done():
						return
					case <-time.After(backoff):
					}
					backoff = min(backoff*2, 30*time.Second)
				}
			}(cam, src, target, sub)
		}
	}
	wg.Wait()
	return nil
}

// relayTLS is the TLS configuration towards the server (nil for rtsp:// inside a trusted network).
func relayTLS(cfg RelayConfig) (*tls.Config, error) {
	var tlsCfg *tls.Config
	if strings.HasPrefix(cfg.Server, "rtsps://") {
		tlsCfg = &tls.Config{MinVersion: tls.VersionTLS12}
		if cfg.CAFile != "" {
			pem, err := os.ReadFile(cfg.CAFile)
			if err != nil {
				return nil, err
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil, errors.New("ca_file holds no certificate")
			}
			tlsCfg.RootCAs = pool
		}
	}
	return tlsCfg, nil
}

// relayAudio finds the sound the server can use (AAC or Opus).
func relayAudio(desc *description.Session) *description.Media {
	var aac *format.MPEG4Audio
	if m := desc.FindFormat(&aac); m != nil && aac.Config != nil {
		return m
	}
	var op *format.Opus
	return desc.FindFormat(&op)
}

func redactAll(err error, src, secret string) string {
	if err == nil {
		return "stream ended"
	}
	msg := strings.ReplaceAll(err.Error(), secret, "xxxxx")
	if u, e := url.Parse(src); e == nil {
		msg = redact(msg, u)
	}
	return msg
}

// relayOnce reads the local camera and publishes its video track unchanged (RTP packets are forwarded as they are).
func relayOnce(ctx context.Context, src, transport, target, id, secret string, tlsCfg *tls.Config, audio bool, log *slog.Logger) error {
	su, err := base.ParseURL(src)
	if err != nil {
		return err
	}
	in := &gortsplib.Client{Scheme: su.Scheme, Host: su.Host, ReadTimeout: 10 * time.Second, UDPReadBufferSize: 4 << 20}
	switch transport {
	case "tcp":
		p := gortsplib.ProtocolTCP
		in.Protocol = &p
	case "udp":
		p := gortsplib.ProtocolUDP
		in.Protocol = &p
	}
	if err := in.Start(); err != nil {
		return err
	}
	defer in.Close()
	desc, _, err := in.Describe(su)
	if err != nil {
		return err
	}
	var medi *description.Media
	for _, m := range desc.Medias {
		if m.Type != description.MediaTypeVideo {
			continue
		}
		for _, f := range m.Formats {
			switch f.(type) {
			case *format.H264, *format.H265, *format.MJPEG:
				medi = m
			}
		}
		if medi != nil {
			break
		}
	}
	if medi == nil {
		return errors.New("the camera offers no H.264, H.265 or MJPEG video")
	}
	if _, err := in.Setup(desc.BaseURL, medi, 0, 0); err != nil {
		return err
	}
	var amedi *description.Media
	if audio {
		amedi = relayAudio(desc)
		if amedi != nil {
			if _, err := in.Setup(desc.BaseURL, amedi, 0, 0); err != nil {
				log.Warn("relay: sound not available", "err", err)
				amedi = nil
			}
		} else {
			log.Warn("relay: the camera sends no AAC or Opus sound")
		}
	}

	tu, err := url.Parse(target)
	if err != nil {
		return err
	}
	tu.User = url.UserPassword(id, secret)
	out := &gortsplib.Client{TLSConfig: tlsCfg, ReadTimeout: 15 * time.Second, WriteTimeout: 10 * time.Second, WriteQueueSize: 2048}
	pm := &description.Media{Type: description.MediaTypeVideo, Formats: medi.Formats}
	medias := []*description.Media{pm}
	var am *description.Media
	if amedi != nil {
		am = &description.Media{Type: description.MediaTypeAudio, Formats: amedi.Formats}
		medias = append(medias, am)
	}
	if err := out.StartRecording(tu.String(), &description.Session{Medias: medias}); err != nil {
		return err
	}
	defer out.Close()
	stopIn := context.AfterFunc(ctx, in.Close)
	defer stopIn()
	stopOut := context.AfterFunc(ctx, out.Close)
	defer stopOut()

	in.OnPacketRTPAny(func(m *description.Media, _ format.Format, pkt *rtp.Packet) {
		switch {
		case m == medi:
			_ = out.WritePacketRTP(pm, pkt)
		case am != nil && m == amedi:
			_ = out.WritePacketRTP(am, pkt)
		}
	})
	if _, err := in.Play(nil); err != nil {
		return err
	}
	log.Info("relay forwarding", "target", strings.Split(target, "@")[0])
	errc := make(chan error, 2)
	go func() { errc <- in.Wait() }()
	go func() { errc <- out.Wait() }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		return nil
	}
}

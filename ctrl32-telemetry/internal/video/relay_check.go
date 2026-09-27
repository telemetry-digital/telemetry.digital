package video

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/video/onvif"
)

// RelayCheck is the result of one step of `camera-relay -check`.
type RelayCheck struct {
	What string
	OK   bool
	Info string
}

// CheckRelay tests a relay configuration without forwarding video: the server is reachable (and its TLS certificate
// trusted), every local camera answers with a supported video format, and the server accepts each camera's secret
// (a publish session is opened and closed at once). Passwords and secrets never appear in the results.
func CheckRelay(ctx context.Context, cfg RelayConfig) []RelayCheck {
	var out []RelayCheck
	add := func(what string, err error, info string) {
		if err != nil {
			out = append(out, RelayCheck{What: what, Info: err.Error()})
			return
		}
		out = append(out, RelayCheck{What: what, OK: true, Info: info})
	}
	if err := cfg.Validate(); err != nil {
		add("configuration", err, "")
		return out
	}
	add("configuration", nil, fmt.Sprintf("%d camera(s)", len(cfg.Cameras)))
	tlsCfg, err := relayTLS(cfg)
	if err != nil {
		add("server certificate authority", err, "")
		return out
	}
	su, _ := url.Parse(cfg.Server)
	host := su.Host
	if su.Port() == "" {
		host = net.JoinHostPort(su.Hostname(), "8322")
	}
	d := &net.Dialer{Timeout: 5 * time.Second}
	if tlsCfg != nil {
		c := tlsCfg.Clone()
		c.ServerName = su.Hostname()
		conn, err := tls.DialWithDialer(d, "tcp", host, c)
		if err == nil {
			conn.Close()
		}
		add("server "+host+" (TLS)", err, "reachable, certificate trusted")
	} else {
		conn, err := d.DialContext(ctx, "tcp", host)
		if err == nil {
			conn.Close()
		}
		add("server "+host+" (no TLS)", err, "reachable")
	}
	for i, cam := range cfg.Cameras {
		name := fmt.Sprintf("camera %d (%s)", i+1, cam.ID)
		medi, desc, info, err := probeSource(cam.Source, cam.Transport)
		if err != nil {
			add(name+": local camera", errors.New(redactAll(err, cam.Source, cam.Secret)), "")
			continue
		}
		add(name+": local camera", nil, info)
		if cam.Audio {
			if _, codec := relayAudio(desc); codec != "" {
				add(name+": sound", nil, codec)
			} else {
				add(name+": sound", errors.New("the camera sends no AAC, Opus or G.711 sound (switch its microphone on, or set audio = false)"), "")
			}
		}
		if cam.SubSource != "" {
			_, _, sinfo, err := probeSource(cam.SubSource, cam.Transport)
			if err != nil {
				err = errors.New(redactAll(err, cam.SubSource, cam.Secret))
			}
			add(name+": local sub stream", err, sinfo)
		}
		tu, _ := url.Parse(strings.TrimRight(cfg.Server, "/") + "/" + cam.ID)
		tu.User = url.UserPassword(cam.ID, cam.Secret)
		pub := &gortsplib.Client{TLSConfig: tlsCfg, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
		pm := &description.Media{Type: description.MediaTypeVideo, Formats: medi.Formats}
		err = pub.StartRecording(tu.String(), &description.Session{Medias: []*description.Media{pm}})
		if err == nil {
			pub.Close()
		} else if strings.Contains(err.Error(), "401") {
			err = errors.New("the server refused the secret (wrong, or replaced by a new one: take the new relay.toml)")
		} else {
			err = errors.New(redactAll(err, cam.Source, cam.Secret))
		}
		add(name+": server accepts the secret", err, "accepted")
		if cam.ONVIF != "" {
			checkONVIF(ctx, cfg, cam, name, add)
		}
	}
	return out
}

// checkONVIF tests camera control: the camera's ONVIF service answers (GetSystemDateAndTime needs no password) and the
// server's control endpoint accepts the secret.
func checkONVIF(ctx context.Context, cfg RelayConfig, cam RelayCamera, name string, add func(string, error, string)) {
	origin, _ := relayOrigin(cam.ONVIF)
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cl := &onvif.Client{Device: relayDevice(cam.ONVIF), HTTP: cameraClient()}
	if t, err := cl.DateTime(cctx); err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		add(name+": local ONVIF "+origin, err, "")
	} else {
		add(name+": local ONVIF "+origin, nil, "answers (camera time "+t.Format("2006-01-02 15:04:05")+" UTC)")
	}
	base, err := cfg.webBase()
	if err != nil {
		add(name+": camera control through the server", err, "")
		return
	}
	web, err := webClient(cfg, 15*time.Second)
	if err != nil {
		add(name+": camera control through the server", err, "")
		return
	}
	_, _, err = relayPollWait(cctx, web, base+"/api/v1/video/relay/"+cam.ID+"/onvif", cam.Secret, 0)
	if err != nil {
		err = errors.New(strings.ReplaceAll(err.Error(), cam.Secret, "xxxxx"))
	}
	add(name+": camera control through the server "+base, err, "secret accepted")
}

// probeSource asks a local camera for its streams and returns the video media the relay would forward.
func probeSource(src, transport string) (*description.Media, *description.Session, string, error) {
	su, err := base.ParseURL(src)
	if err != nil {
		return nil, nil, "", err
	}
	c := &gortsplib.Client{Scheme: su.Scheme, Host: su.Host, ReadTimeout: 8 * time.Second}
	if transport == "tcp" {
		p := gortsplib.ProtocolTCP
		c.Protocol = &p
	}
	if err := c.Start(); err != nil {
		return nil, nil, "", err
	}
	defer c.Close()
	desc, _, err := c.Describe(su)
	if err != nil {
		return nil, nil, "", err
	}
	for _, m := range desc.Medias {
		if m.Type != description.MediaTypeVideo {
			continue
		}
		for _, f := range m.Formats {
			switch f.(type) {
			case *format.H264, *format.H265, *format.MJPEG:
				return m, desc, f.Codec(), nil
			}
		}
	}
	return nil, nil, "", errors.New("the camera offers no H.264, H.265 or MJPEG video")
}

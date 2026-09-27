package video

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
)

const pushCam = "01a0dfc5-3db0-7441-8ed4-0b512090e72c"

// selfSigned writes a certificate for 127.0.0.1 and returns cert, key and CA (the same file) paths.
func selfSigned(t *testing.T) (string, string) {
	dir := t.TempDir()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := x509.MarshalECPrivateKey(key)
	cp, kp := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	_ = os.WriteFile(cp, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
	_ = os.WriteFile(kp, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600)
	return cp, kp
}

func pushManager(secret string) (*Manager, *cam) {
	m := &Manager{log: slog.New(slog.NewTextHandler(io.Discard, nil)), cams: map[string]*cam{}, set: DefaultSettings(1)}
	c := &cam{cfg: Camera{ID: pushCam, Push: true, PushSecret: secret}}
	c.main = &Source{URL: &url.URL{Scheme: "push"}, Linger: time.Millisecond}
	c.sub = &Source{URL: &url.URL{Scheme: "push"}, Linger: time.Millisecond}
	c.main.Keep(true)
	c.sub.Keep(true)
	m.cams[pushCam] = c
	return m, c
}

func TestRelayPushOverTLS(t *testing.T) {
	sim := startSim(t)
	cert, key := selfSigned(t)
	secret := NewPushSecret()
	m, c := pushManager(secret)
	defer c.main.Stop()
	defer c.sub.Stop()
	addr := freePort(t)
	ps, err := m.StartPush(PushConfig{Listen: addr, TLSCert: cert, TLSKey: key})
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := RelayConfig{Server: "rtsps://" + addr, CAFile: cert, Cameras: []RelayCamera{{ID: pushCam, Secret: secret,
		Source: "rtsp://" + sim + "/yard/h264/160x96", SubSource: "rtsp://" + sim + "/yard/mjpeg/160x96", Transport: "tcp"}}}
	go func() { _ = RunRelay(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil))) }()

	pk := receive(t, c.main, 5)
	if pk[0].Track.Codec != H264 || pk[0].Track.Width != 160 {
		t.Fatalf("main %+v", pk[0].Track)
	}
	pk = receive(t, c.sub, 3)
	if pk[0].Track.Codec != MJPEG {
		t.Fatalf("sub %+v", pk[0].Track)
	}
	if m.Source(pushCam, true) != c.sub {
		t.Fatal("tiles should use the pushed sub-stream")
	}
}

// TestRelayForwardsG711 pushes a camera with G.711 sound (as TP-Link Tapo cameras send it) through the relay: the
// server takes it like the sound of a pulled camera (PCM for the browser).
func TestRelayForwardsG711(t *testing.T) {
	sim := startSim(t)
	secret := NewPushSecret()
	m, c := pushManager(secret)
	c.main.Stop()
	c.main = &Source{URL: &url.URL{Scheme: "push"}, Linger: time.Millisecond, Audio: true}
	c.main.Keep(true)
	defer c.main.Stop()
	defer c.sub.Stop()
	addr := freePort(t)
	ps, err := m.StartPush(PushConfig{Listen: addr})
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()
	src := "rtsp://" + sim + "/tapo/h264/160x96/g711"
	_, desc, _, err := probeSource(src, "tcp")
	if err != nil {
		t.Fatal(err)
	}
	if _, codec := relayAudio(desc); codec != "G.711 µ-law 8000 Hz" {
		t.Fatalf("relay sees sound %q", codec)
	}
	// Tapo cameras send A-law (PCMA)
	alaw := &description.Session{Medias: []*description.Media{{Type: description.MediaTypeAudio, Formats: []format.Format{&format.G711{PayloadTyp: 8, SampleRate: 8000, ChannelCount: 1}}}}}
	if _, codec := relayAudio(alaw); codec != "G.711 A-law 8000 Hz" {
		t.Fatalf("A-law seen as %q", codec)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := RelayConfig{Server: "rtsp://" + addr, Cameras: []RelayCamera{{ID: pushCam, Secret: secret, Source: src, Transport: "tcp", Audio: true}}}
	go func() { _ = RunRelay(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	sub := c.main.Subscribe()
	defer sub.Close()
	timeout := time.After(15 * time.Second)
	for {
		select {
		case p := <-sub.C:
			if p.Sample.Audio != nil {
				if p.Track.Audio == nil || p.Track.Audio.Codec != PCM || p.Track.Audio.SampleRate != 8000 {
					t.Fatalf("sound track %+v", p.Track.Audio)
				}
				if st := c.main.Status(); !strings.HasPrefix(st.Audio, "G.711") {
					t.Fatalf("status %q", st.Audio)
				}
				return
			}
		case <-timeout:
			t.Fatalf("no sound from the relay; status %+v", c.main.Status())
		}
	}
}

func TestPushRefusesWrongSecretAndBlocks(t *testing.T) {
	sim := startSim(t)
	m, c := pushManager(NewPushSecret())
	defer c.main.Stop()
	defer c.sub.Stop()
	addr := freePort(t)
	ps, err := m.StartPush(PushConfig{Listen: addr}) // plain: digest authentication
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for i := 0; i < m.Settings().PushMaxFails; i++ {
		err := relayOnce(context.Background(), "rtsp://"+sim+"/x/h264/160x96", "tcp", "rtsp://"+addr+"/"+pushCam, pushCam, "wrong-secret-0123456789", nil, false, log)
		if err == nil {
			t.Fatal("wrong secret accepted")
		}
	}
	if !ps.blocked("127.0.0.1") {
		t.Fatal("address not blocked after repeated failures")
	}
	// even the right secret is refused while blocked
	err = relayOnce(context.Background(), "rtsp://"+sim+"/x/h264/160x96", "tcp", "rtsp://"+addr+"/"+pushCam, pushCam, c.cfg.PushSecret, nil, false, log)
	if err == nil {
		t.Fatal("blocked address accepted")
	}
	st := c.main.Status()
	if st.State == "online" {
		t.Fatal("stream from a refused publisher")
	}
}

func TestRelayConfigValidate(t *testing.T) {
	bad := []RelayConfig{
		{Server: "http://x", Cameras: []RelayCamera{{ID: pushCam, Secret: "0123456789abcdef", Source: "rtsp://a/b"}}},
		{Server: "rtsps://x:8322"},
		{Server: "rtsps://x:8322", Cameras: []RelayCamera{{ID: "../etc", Secret: "0123456789abcdef", Source: "rtsp://a/b"}}},
		{Server: "rtsps://x:8322", Cameras: []RelayCamera{{ID: pushCam, Secret: "short", Source: "rtsp://a/b"}}},
		{Server: "rtsps://x:8322", Cameras: []RelayCamera{{ID: pushCam, Secret: "0123456789abcdef", Source: "file:///etc/passwd"}}},
	}
	for i, c := range bad {
		if c.Validate() == nil {
			t.Errorf("config %d accepted", i)
		}
	}
}

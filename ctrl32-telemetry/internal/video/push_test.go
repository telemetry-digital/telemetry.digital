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
	"testing"
	"time"
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
	m := &Manager{log: slog.New(slog.NewTextHandler(io.Discard, nil)), cams: map[string]*cam{}}
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
	for i := 0; i < pushMaxFails; i++ {
		err := relayOnce(context.Background(), "rtsp://"+sim+"/x/h264/160x96", "tcp", "rtsp://"+addr+"/"+pushCam, pushCam, "wrong-secret-0123456789", nil, log)
		if err == nil {
			t.Fatal("wrong secret accepted")
		}
	}
	if !ps.blocked("127.0.0.1") {
		t.Fatal("address not blocked after repeated failures")
	}
	// even the right secret is refused while blocked
	err = relayOnce(context.Background(), "rtsp://"+sim+"/x/h264/160x96", "tcp", "rtsp://"+addr+"/"+pushCam, pushCam, c.cfg.PushSecret, nil, log)
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

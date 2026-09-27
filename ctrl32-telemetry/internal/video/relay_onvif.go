package video

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

// The relay's side of camera control (tunnel.go describes the protocol). The relay polls the server for ONVIF requests
// of its cameras and performs them against the camera's ONVIF origin from relay.toml — never anywhere else: the
// server sends only a path under /onvif/, only POST (SOAP) and GET are allowed, redirects are not followed, bodies are
// limited to 1 MiB each way and nothing of them is logged (they carry the camera's WS-Security digests).

const relaySecretHeader = "X-Camera-Secret"

// relayOrigin normalises the onvif value of relay.toml (an origin or the device_service URL) to scheme://host:port.
func relayOrigin(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", errors.New("must be the camera's address in this network, e.g. http://192.168.0.21:2020")
	}
	if u.User != nil {
		return "", errors.New("no user name or password in the address (the server keeps the camera's ONVIF account)")
	}
	if u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/" && !strings.HasPrefix(u.Path, "/onvif/")) {
		return "", errors.New("give the camera's address (http://IP:port) or its /onvif/device_service address")
	}
	return strings.ToLower(u.Scheme + "://" + u.Host), nil
}

// relayDevice is the camera's device service address for the self-check.
func relayDevice(raw string) string {
	u, _ := url.Parse(strings.TrimSpace(raw))
	if u == nil || !strings.HasPrefix(u.Path, "/onvif/") {
		o, _ := relayOrigin(raw)
		return o + "/onvif/device_service"
	}
	return u.String()
}

// webBase is the server's web address for camera control.
func (c *RelayConfig) webBase() (string, error) {
	if c.Web == "" {
		su, err := url.Parse(c.Server)
		if err != nil || su.Scheme != "rtsps" {
			return "", errors.New("web = \"https://…\" (the server's web address) is needed for onvif when server is not rtsps://")
		}
		return "https://" + su.Hostname(), nil
	}
	u, err := url.Parse(strings.TrimRight(c.Web, "/"))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("web must be the server's address, e.g. https://telemetry.example.com")
	}
	switch u.Scheme {
	case "https":
	case "http":
		// the secret travels in a header: plain HTTP only where the video goes without TLS as well
		if strings.HasPrefix(c.Server, "rtsps://") {
			return "", errors.New("web must be https:// when server is rtsps://")
		}
	default:
		return "", errors.New("web must be https://host (http:// only inside a trusted network)")
	}
	return u.Scheme + "://" + u.Host, nil
}

// webClient talks to the server's web port (the private CA of ca_file is trusted too).
func webClient(cfg RelayConfig, timeout time.Duration) (*http.Client, error) {
	tc := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("ca_file holds no certificate")
		}
		tc.RootCAs = pool
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = tc
	return &http.Client{Transport: tr, Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

// cameraClient reaches the local camera: no proxy, no redirects, 10 s.
func cameraClient() *http.Client {
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		ResponseHeaderTimeout: 10 * time.Second, MaxIdleConnsPerHost: 2}
	return &http.Client{Transport: tr, Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// relayTarget checks a request of the server and builds the camera address from the configured origin.
func relayTarget(origin string, rq *TunnelRequest) (string, error) {
	if rq.Method != http.MethodPost && rq.Method != http.MethodGet {
		return "", errors.New("method not allowed")
	}
	if len(rq.Body) > tunnelMaxBody {
		return "", errors.New("request larger than 1 MiB")
	}
	p := rq.Path
	if len(p) > 2048 || !strings.HasPrefix(p, "/onvif/") || strings.ContainsAny(p, "\\#@\r\n\t ") || strings.Contains(p, "//") {
		return "", errors.New("path not allowed")
	}
	u, err := url.Parse(p)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || path.Clean(u.Path) != u.Path || !strings.HasPrefix(u.Path, "/onvif/") {
		return "", errors.New("path not allowed")
	}
	if dec, err := url.PathUnescape(u.EscapedPath()); err != nil || strings.Contains(dec, "..") {
		return "", errors.New("path not allowed")
	}
	full := origin + p
	fu, err := url.Parse(full)
	if err != nil || strings.ToLower(fu.Scheme+"://"+fu.Host) != origin {
		return "", errors.New("path not allowed")
	}
	return full, nil
}

// relayPerform sends one request to the camera and returns its answer (or why there is none).
func relayPerform(ctx context.Context, hc *http.Client, origin string, rq *TunnelRequest) TunnelResponse {
	target, err := relayTarget(origin, rq)
	if err != nil {
		return TunnelResponse{Error: "refused by the relay: " + err.Error()}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, rq.Method, target, bytes.NewReader(rq.Body))
	if err != nil {
		return TunnelResponse{Error: "invalid request"}
	}
	for _, k := range tunnelHeaders {
		if v := rq.Header[k]; v != "" && !strings.ContainsAny(v, "\r\n") {
			req.Header.Set(k, v)
		}
	}
	resp, err := hc.Do(req)
	if err != nil {
		// the camera's address is the site's business: the server learns only that it did not answer
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return TunnelResponse{Error: "the camera did not answer: " + err.Error()}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, tunnelMaxBody+1))
	if err != nil {
		return TunnelResponse{Error: "the camera's answer broke off"}
	}
	if len(body) > tunnelMaxBody {
		return TunnelResponse{Error: "the camera's answer is larger than 1 MiB"}
	}
	return TunnelResponse{Status: resp.StatusCode, Header: pickHeaders(resp.Header), Body: body}
}

// relayONVIF serves camera control for one camera until ctx ends: long polls, each request performed on its own (a
// held events pull does not delay a PTZ move), reconnecting with a back-off.
func relayONVIF(ctx context.Context, cfg RelayConfig, cam RelayCamera, log *slog.Logger) {
	origin, _ := relayOrigin(cam.ONVIF)
	base, err := cfg.webBase()
	if err != nil {
		log.Error("camera control off", "err", err)
		return
	}
	web, err := webClient(cfg, tunnelPoll+15*time.Second)
	if err != nil {
		log.Error("camera control off", "err", err)
		return
	}
	camc := cameraClient()
	base += "/api/v1/video/relay/" + cam.ID + "/onvif"
	sem := make(chan struct{}, 4)
	backoff, up := time.Second, false
	for ctx.Err() == nil {
		rq, status, err := relayPoll(ctx, web, base, cam.Secret)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			if up || backoff == time.Second {
				log.Warn("camera control: server not reached", "err", strings.ReplaceAll(err.Error(), cam.Secret, "xxxxx"), "status", status)
			}
			up = false
			wait := backoff
			if status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusTooManyRequests {
				// a refused secret (camera removed, secret replaced) is retried rarely, so it never gets the site's
				// address blocked for its other cameras
				wait = 5 * time.Minute
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			backoff = min(backoff*2, time.Minute)
			continue
		}
		if !up {
			log.Info("camera control connected", "camera_onvif", origin)
			up = true
		}
		backoff = time.Second
		if rq == nil {
			continue
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return
		}
		go func(rq *TunnelRequest) {
			defer func() { <-sem }()
			ans := relayPerform(ctx, camc, origin, rq)
			if ans.Error != "" {
				log.Warn("camera control request failed", "err", ans.Error, "method", rq.Method, "path", strings.SplitN(rq.Path, "?", 2)[0])
			}
			if err := relayAnswer(ctx, web, base+"/"+url.PathEscape(rq.ID), cam.Secret, ans); err != nil && ctx.Err() == nil {
				log.Warn("camera control answer not delivered", "err", strings.ReplaceAll(err.Error(), cam.Secret, "xxxxx"))
			}
		}(rq)
	}
}

// relayPoll asks the server for the next request; nil without one. wait=0 only tests the secret.
func relayPoll(ctx context.Context, web *http.Client, base, secret string) (*TunnelRequest, int, error) {
	return relayPollWait(ctx, web, base, secret, tunnelPoll)
}

func relayPollWait(ctx context.Context, web *http.Client, base, secret string, wait time.Duration) (*TunnelRequest, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/next?wait=%d", base, int(wait.Seconds())), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set(relaySecretHeader, secret)
	resp, err := web.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil, resp.StatusCode, nil
	case http.StatusOK:
		var rq TunnelRequest
		if err := json.NewDecoder(io.LimitReader(resp.Body, 2*tunnelMaxBody)).Decode(&rq); err != nil {
			return nil, resp.StatusCode, fmt.Errorf("invalid request from the server: %w", err)
		}
		return &rq, resp.StatusCode, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, resp.StatusCode, errors.New("the server refused the secret (wrong, or replaced by a new one: take the new relay.toml)")
	case http.StatusTooManyRequests:
		return nil, resp.StatusCode, errors.New("the server blocks this address after failed attempts; wait and check the secret")
	case http.StatusNotFound:
		return nil, resp.StatusCode, errors.New("the server does not offer camera control (update the server to 0.58.0 or newer)")
	}
	return nil, resp.StatusCode, fmt.Errorf("server answered HTTP %d", resp.StatusCode)
}

func relayAnswer(ctx context.Context, web *http.Client, u, secret string, ans TunnelResponse) error {
	b, err := json.Marshal(ans)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set(relaySecretHeader, secret)
	req.Header.Set("Content-Type", "application/json")
	resp, err := web.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusGone {
		return fmt.Errorf("server answered HTTP %d", resp.StatusCode)
	}
	return nil
}

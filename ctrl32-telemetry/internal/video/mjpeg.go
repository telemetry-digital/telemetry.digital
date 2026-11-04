package video

import (
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// httpMJPEG reads a multipart/x-mixed-replace stream of JPEG pictures (the MJPEG mode of many cheap cameras).
func (s *Source) httpMJPEG(ctx context.Context) error {
	u := *s.URL
	u.User = nil
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	if s.URL.User != nil {
		pw, _ := s.URL.User.Password()
		req.SetBasicAuth(s.URL.User.Username(), pw)
	}
	client := &http.Client{Timeout: 0, Transport: &http.Transport{ResponseHeaderTimeout: 10 * time.Second, Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("camera answered HTTP %d", resp.StatusCode)
	}
	mt, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mt, "multipart/") || params["boundary"] == "" {
		return errNotMultipart
	}
	// a stalled camera: no picture for 10 s closes the connection
	body := &idleReader{r: resp.Body, cancel: func() { resp.Body.Close() }, max: 10 * time.Second}
	defer body.stop()
	mr := multipart.NewReader(body, strings.TrimPrefix(params["boundary"], "--"))
	started := time.Now()
	var t *Track
	for {
		part, err := mr.NextPart()
		if err != nil {
			return err
		}
		img, err := io.ReadAll(io.LimitReader(part, 16<<20))
		if err != nil {
			return err
		}
		if len(img) < 4 || img[0] != 0xFF || img[1] != 0xD8 {
			continue
		}
		if t == nil {
			if t = jpegTrack(img); t == nil {
				continue
			}
		}
		now := time.Now()
		s.publish(ctx, t, &Sample{Time: now, DTS: durationToTicks(now.Sub(started)), Key: true, JPEG: img})
	}
}

// idleReader closes the stream when no data arrives for max.
type idleReader struct {
	r      io.Reader
	cancel func()
	max    time.Duration
	t      *time.Timer
}

func (i *idleReader) Read(p []byte) (int, error) {
	if i.t == nil {
		i.t = time.AfterFunc(i.max, i.cancel)
	} else {
		i.t.Reset(i.max)
	}
	return i.r.Read(p)
}

func (i *idleReader) stop() {
	if i.t != nil {
		i.t.Stop()
	}
}

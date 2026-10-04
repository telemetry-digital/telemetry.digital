package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics/setup"
)

// The detector installation without the Internet (ADR-0024): its pinned parts are taken, in this order, from the staged
// offline package (analytics/), from a mirror of the distribution point ([agent] update_url other than the portal:
// <update_url>/analytics/<name>), and only then from their official releases. Every source is checked against the same
// pin (setup.Install keeps a file only when its SHA-256 matches), so a mirror cannot change what is installed.

const portalURL = "https://portal.telemetry.digital/dl"

type mirrorSource struct {
	names  map[string]string // official URL -> mirror name (analytics.MirrorName)
	dirs   []string          // local directories with the parts
	base   string            // mirror base URL ("" = none)
	next   setup.Fetcher     // the official source
	client *http.Client
}

func (a *Server) mirrorFetcher(next setup.Fetcher) setup.Fetcher {
	m := mirrorSource{names: map[string]string{}, dirs: []string{filepath.Join(stagedDir(), "analytics")}, next: next}
	for _, art := range a.analyticsArtifacts() {
		m.names[art.URL] = analytics.MirrorName(art)
	}
	if u := strings.TrimRight(a.updateURL, "/"); u != "" && u != portalURL {
		m.base = u
	}
	return m
}

func (m mirrorSource) Fetch(ctx context.Context, url string, w io.Writer, progress func(n, total int64)) error {
	name := m.names[url]
	if name == "" {
		return m.next.Fetch(ctx, url, w, progress)
	}
	for _, d := range m.dirs {
		f, err := os.Open(filepath.Join(d, name))
		if err != nil {
			continue
		}
		defer f.Close()
		var total int64
		if st, err := f.Stat(); err == nil {
			total = st.Size()
		}
		return copyProgress(w, f, total, progress)
	}
	if m.base != "" {
		err := m.fromMirror(ctx, m.base+"/analytics/"+name, w, progress)
		if err == nil || !errors.Is(err, errTryNext) {
			return err
		}
	}
	return m.next.Fetch(ctx, url, w, progress)
}

var errTryNext = errors.New("not on the mirror")

// fromMirror downloads from the mirror; a missing file or an unreachable mirror lets the official source try
// (errTryNext), a broken transfer does not (the bytes are already written).
func (m mirrorSource) fromMirror(ctx context.Context, u string, w io.Writer, progress func(n, total int64)) error {
	c := m.client
	if c == nil {
		c = &http.Client{Timeout: 30 * time.Minute}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return errTryNext
	}
	req.Header.Set("User-Agent", "ctrl32-telemetry-agent")
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", errTryNext, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%w: HTTP %d", errTryNext, resp.StatusCode)
	}
	return copyProgress(w, resp.Body, resp.ContentLength, progress)
}

func copyProgress(w io.Writer, r io.Reader, total int64, progress func(n, total int64)) error {
	if total < 0 {
		total = 0
	}
	buf := make([]byte, 256<<10)
	var n int64
	for {
		k, err := r.Read(buf)
		if k > 0 {
			if _, werr := w.Write(buf[:k]); werr != nil {
				return werr
			}
			n += int64(k)
			if progress != nil {
				progress(n, total)
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
)

// Framing stays forbidden everywhere except the camera page embedded by a video wall of the same origin.
func TestSecureHeadersFraming(t *testing.T) {
	s := &Server{cfg: &config.Config{}}
	h := s.secureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	for _, c := range []struct {
		method, url, xfo, ancestors string
	}{
		{"GET", "/video/cameras/abc?embed=1", "SAMEORIGIN", "frame-ancestors 'self'"},
		{"GET", "/video/cameras/abc", "DENY", "frame-ancestors 'none'"},
		{"GET", "/video/cameras/abc?embed=yes", "DENY", "frame-ancestors 'none'"},
		{"GET", "/video/walls/abc?embed=1", "DENY", "frame-ancestors 'none'"},
		{"GET", "/settings?embed=1", "DENY", "frame-ancestors 'none'"},
		{"POST", "/video/cameras/abc?embed=1", "DENY", "frame-ancestors 'none'"},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(c.method, c.url, nil))
		if got := rec.Header().Get("X-Frame-Options"); got != c.xfo {
			t.Errorf("%s %s: X-Frame-Options %q, want %q", c.method, c.url, got, c.xfo)
		}
		if csp := rec.Header().Get("Content-Security-Policy"); !strings.HasSuffix(csp, c.ancestors) {
			t.Errorf("%s %s: CSP %q, want it to end with %q", c.method, c.url, csp, c.ancestors)
		}
	}
}

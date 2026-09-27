package httpapi

import (
	"net/http"
	"strings"
)

// serviceWorker serves sw.js with the server version baked into the cache name, so every release invalidates the
// PWA shell cache, and with no-cache headers so browsers always check for a new worker.
func (s *Server) serviceWorker(w http.ResponseWriter, r *http.Request) {
	b, err := static.ReadFile("static/sw.js")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	body := strings.ReplaceAll(string(b), "__VERSION__", s.version)
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Service-Worker-Allowed", "/")
	_, _ = w.Write([]byte(body))
}

// noStaleShell makes browsers revalidate the shell files on every load (the service worker fetches them with
// cache: 'no-cache' too), so a deployed fix is visible on the next navigation.
func noStaleShell(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app.js", "/app.css", "/manifest.webmanifest", "/app-install.js", "/settings-apps.js":
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

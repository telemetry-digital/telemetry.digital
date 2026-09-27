package httpapi

import (
	"html"
	"net/http"

	ctrl32telemetry "github.com/telemetry-digital/ctrl32-telemetry"
)

// licensePage shows the license the binary ships under. It is linked from the fixed footer of every page.
func (s *Server) licensePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("format") == "txt" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(ctrl32telemetry.License))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>License — ctrl32 telemetry</title>
<meta name="viewport" content="width=device-width, initial-scale=1"><link rel="stylesheet" href="/app.css"><link rel="manifest" href="/manifest.webmanifest"><link rel="icon" href="/favicon.svg" type="image/svg+xml"></head>
<body><header class="topbar"></header>
<main class="container no-rebrand"><h1>License</h1><p class="muted">ctrl32 telemetry is free of charge, forever. It is licensed under the Apache License 2.0 with the Commons Clause: you may use, copy, modify and redistribute it, but you may not sell it. Optional paid features (white labeling) are licensed separately by the licensor. <a href="/license?format=txt">Plain text</a></p>
<pre class="license">` + html.EscapeString(ctrl32telemetry.License) + `</pre></main>
<script src="/app.js"></script><script>ctrl32.ready();</script></body></html>`))
}

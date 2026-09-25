// Package logging configures the structured system log (ADR-0007, layer 2): JSON lines, levels, no secrets,
// with a "security" attribute for security-relevant events so a SIEM can filter them.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Setup returns a JSON logger writing to stdout, optionally to a file and optionally to a syslog collector
// (syslogAddr "udp://host:514" or "tcp://host:514", URS I-07).
func Setup(level, file, syslogAddr string) (*slog.Logger, func(), error) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	writers := []io.Writer{os.Stdout}
	closeFn := func() {}
	if file != "" {
		f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
		if err != nil {
			return nil, nil, err
		}
		writers = append(writers, f)
		closeFn = func() { _ = f.Close() }
	}
	var h slog.Handler = slog.NewJSONHandler(io.MultiWriter(writers...), &slog.HandlerOptions{Level: lvl})
	if syslogAddr != "" {
		sh, err := newSyslog(syslogAddr, lvl)
		if err != nil {
			closeFn()
			return nil, nil, err
		}
		h = multiHandler{h, sh}
	}
	logger := slog.New(h).With("app", "ctrl32-telemetry")
	slog.SetDefault(logger)
	return logger, closeFn, nil
}

// Security logs a security-relevant event (failed logins, revocations, chain violations, ...) with security=true.
func Security(ctx context.Context, logger *slog.Logger, msg string, attrs ...any) {
	attrs = append([]any{"security", true}, attrs...)
	logger.WarnContext(ctx, msg, attrs...)
}

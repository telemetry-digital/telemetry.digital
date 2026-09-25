package logging

import (
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSyslogHandler(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	h, err := newSyslog("udp://"+pc.LocalAddr().String(), slog.LevelInfo)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(multiHandler{h}).With("app", "test")
	logger.Debug("hidden")
	logger.Warn("disk almost full", "security", true, "free_mb", 12)
	_ = pc.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 2048)
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	got := string(buf[:n])
	if !strings.HasPrefix(got, "<132>1 ") { // local0 (16*8) + warning (4)
		t.Errorf("priority/version: %q", got)
	}
	for _, want := range []string{" ctrl32-telemetry ", `"msg":"disk almost full"`, `"security":true`, `"free_mb":12`, `"app":"test"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "hidden") {
		t.Errorf("debug record must be filtered by level")
	}
	if _, err := newSyslog("http://x", slog.LevelInfo); err == nil {
		t.Errorf("scheme must be validated")
	}
}

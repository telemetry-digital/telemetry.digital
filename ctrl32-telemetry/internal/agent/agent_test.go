package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestParseSc(t *testing.T) {
	if s := parseScState("SERVICE_NAME: x\n        STATE              : 4  RUNNING\n"); s != "active" {
		t.Errorf("running → %s", s)
	}
	if s := parseScState("        STATE              : 1  STOPPED"); s != "inactive" {
		t.Errorf("stopped → %s", s)
	}
	if s := parseScState("[SC] EnumQueryServicesStatus:OpenService FAILED 1060:"); s != "not-installed" {
		t.Errorf("missing → %s", s)
	}
	if s := parseScStart("        START_TYPE         : 2   AUTO_START"); s != "enabled" {
		t.Errorf("auto → %s", s)
	}
	if s := parseScStart("        START_TYPE         : 3   DEMAND_START"); s != "manual" {
		t.Errorf("demand → %s", s)
	}
}

// The agent serves status, the backup schedule and backups on a Unix socket on every platform (Windows 10+ has
// AF_UNIX). The database dump needs pg_dump or the PostgreSQL container; without them the backup records its error.
func TestAgentOverSocket(t *testing.T) {
	dir := t.TempDir()
	etcDir, optDir = dir, dir
	BackupDir = filepath.Join(dir, "backups")
	derivePaths()
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte("profile = \"intranet\"\n[db]\nurl = \"postgres://x@127.0.0.1:1/x\"\nadmin_url = \"postgres://x@127.0.0.1:1/x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(os.TempDir(), fmt.Sprintf("c32a%d.sock", os.Getpid()))
	defer os.Remove(sock)
	a := NewServer(sock, cfg, "test", "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if c := os.Getenv("CTRL32_TEST_PG_CONTAINER"); c != "" {
		a.SetDBTools("", c)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = a.Serve(ctx) }()
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", sock)
	}}}
	var res *http.Response
	var err error
	for i := 0; i < 40; i++ {
		res, err = client.Get("http://agent/status")
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("agent not reachable on %s: %v", sock, err)
	}
	var st Status
	if err := json.NewDecoder(res.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if st.Agent.OS != runtime.GOOS || len(st.Services) == 0 || st.Host.Hostname == "" {
		t.Errorf("status: os=%s services=%d host=%s", st.Agent.OS, len(st.Services), st.Host.Hostname)
	}
	// schedule
	body, _ := json.Marshal(map[string]any{"enabled": true, "hour": 3, "minute": 15, "keep": 5})
	req, _ := http.NewRequest(http.MethodPut, "http://agent/backups/schedule", bytes.NewReader(body))
	res, err = client.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("schedule: %v %v", err, res)
	}
	res.Body.Close()
	var saved backupStored
	if b, err := os.ReadFile(backupStateFile); err != nil || json.Unmarshal(b, &saved) != nil || saved.Hour != 3 || saved.Keep != 5 {
		t.Errorf("schedule not persisted in %s: %s", backupStateFile, b)
	}
	// backup (background)
	res, err = client.Post("http://agent/backups", "application/json", bytes.NewReader([]byte("{}")))
	if err != nil || res.StatusCode != 202 {
		t.Fatalf("backup start: %v %v", err, res)
	}
	res.Body.Close()
	var bs BackupsState
	for i := 0; i < 600; i++ {
		res, err = client.Get("http://agent/backups")
		if err != nil {
			t.Fatal(err)
		}
		_ = json.NewDecoder(res.Body).Decode(&bs)
		res.Body.Close()
		if !bs.Running && (!bs.Schedule.LastRun.IsZero()) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if bs.DBMode == "unknown" {
		if bs.Schedule.LastError == "" {
			t.Errorf("without PostgreSQL the backup should record an error")
		}
		t.Logf("no PostgreSQL tooling on this host: %s", bs.Schedule.LastError)
		return
	}
	if len(bs.Files) != 1 || bs.Schedule.LastError != "" {
		t.Fatalf("backup: files=%d err=%q", len(bs.Files), bs.Schedule.LastError)
	}
	if st, err := os.Stat(filepath.Join(BackupDir, bs.Files[0].Name)); err != nil || st.Size() < 1000 {
		t.Errorf("bundle missing or tiny: %v", err)
	}
	t.Logf("backup %s (%d bytes, mode %s)", bs.Files[0].Name, bs.Files[0].SizeBytes, bs.DBMode)
}

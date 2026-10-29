package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseAptSimulation(t *testing.T) {
	out := `NOTE: This is only a simulation!
Reading package lists...
Inst libssl3t64 [3.0.13-0ubuntu3.4] (3.0.13-0ubuntu3.5 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Inst linux-image-7.0.0-42-generic (7.0.0-42.42 Ubuntu:26.04/resolute-updates [amd64])
Inst tzdata [2026a-0ubuntu0.24.04] (2026b-0ubuntu0.24.04 Ubuntu:24.04/noble-updates [all])
Conf libssl3t64 (3.0.13-0ubuntu3.5 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Inst openssl [3.0.11-1~deb12u2] (3.0.15-1~deb12u1 Debian-Security:12/stable-security [amd64])
`
	l := parseAptSimulation(out)
	if len(l) != 4 {
		t.Fatalf("got %d upgrades: %+v", len(l), l)
	}
	if l[0].Name != "libssl3t64" || l[0].From != "3.0.13-0ubuntu3.4" || l[0].To != "3.0.13-0ubuntu3.5" || !l[0].Security || strings.Contains(l[0].Origin, "[amd64]") {
		t.Errorf("first: %+v", l[0])
	}
	if !l[1].New || l[1].From != "" || l[1].Security {
		t.Errorf("new kernel: %+v", l[1])
	}
	if l[2].Security {
		t.Errorf("tzdata is not a security update: %+v", l[2])
	}
	if !l[3].Security {
		t.Errorf("Debian security: %+v", l[3])
	}
	if len(parseAptSimulation("")) != 0 {
		t.Error("empty output")
	}
}

func TestParseMdstat(t *testing.T) {
	ok := parseMdstat(simMdstat)
	if len(ok) != 1 || ok[0].Name != "md0" || ok[0].Level != "raid0" || ok[0].Health != "ok" || len(ok[0].Devices) != 2 {
		t.Fatalf("raid0: %+v", ok)
	}
	degraded := parseMdstat(`Personalities : [raid1]
md1 : active raid1 sdb1[1] sda1[0](F)
      976630464 blocks super 1.2 [2/1] [U_]

md2 : active raid1 sdd1[2] sdc1[0]
      976630464 blocks super 1.2 [2/1] [U_]
      [=>...................]  recovery =  7.5% (73654336/976630464) finish=95.3min speed=157828K/sec

md127 : inactive sde[1](S)
      3906886488 blocks super 1.2

unused devices: <none>
`)
	if len(degraded) != 3 {
		t.Fatalf("got %+v", degraded)
	}
	if degraded[0].Health != "degraded" || degraded[0].Status != "[2/1] [U_]" {
		t.Errorf("md1: %+v", degraded[0])
	}
	if degraded[1].Health != "rebuilding" || !strings.HasPrefix(degraded[1].Progress, "recovery 7.5%") || !strings.Contains(degraded[1].Progress, "95.3min") {
		t.Errorf("md2: %+v", degraded[1])
	}
	if degraded[2].Health != "failed" || degraded[2].State != "inactive" || len(degraded[2].Devices) != 1 {
		t.Errorf("md127: %+v", degraded[2])
	}
	if len(parseMdstat("")) != 0 {
		t.Error("no mdstat")
	}
}

func TestParseRebootAndSmart(t *testing.T) {
	if p := parseRebootPackages("linux-image-7.0.0-42-generic\nlibc6\nlibc6\n\n"); len(p) != 2 || p[0] != "linux-image-7.0.0-42-generic" {
		t.Errorf("packages: %v", p)
	}
	d := parseSmart("/dev/sda", `=== START OF INFORMATION SECTION ===
Device Model:     HGST HUS726T4TALA6L4
Serial Number:    K4KJDYXL
SMART overall-health self-assessment test result: PASSED
194 Temperature_Celsius     0x0002   171   171   000    Old_age   Always       -       35 (Min/Max 20/45)
`)
	if d.Model != "HGST HUS726T4TALA6L4" || d.Serial != "K4KJDYXL" || d.Health != "PASSED" || d.Temperature != 35 {
		t.Errorf("sata: %+v", d)
	}
	n := parseSmart("/dev/nvme0", "Model Number:                       Samsung SSD 980\nSMART overall-health self-assessment test result: FAILED!\nTemperature:                        41 Celsius\n")
	if n.Health != "FAILED!" && n.Health != "FAILED" {
		t.Errorf("nvme health: %+v", n)
	}
	if n.Temperature != 41 {
		t.Errorf("nvme temperature: %+v", n)
	}
	if parseSmart("/dev/sdb", "").Health != "unknown" {
		t.Error("unknown health")
	}
}

func TestNetLinks(t *testing.T) {
	root := t.TempDir()
	mk := func(name, state, speed string, physical bool) {
		d := filepath.Join(root, name)
		_ = os.MkdirAll(d, 0o755)
		_ = os.WriteFile(filepath.Join(d, "operstate"), []byte(state+"\n"), 0o644)
		_ = os.WriteFile(filepath.Join(d, "speed"), []byte(speed+"\n"), 0o644)
		_ = os.WriteFile(filepath.Join(d, "address"), []byte("aa:bb:cc:dd:ee:ff\n"), 0o644)
		if physical {
			_ = os.MkdirAll(filepath.Join(d, "device"), 0o755)
		}
	}
	mk("enp7s0", "up", "1000", true)
	mk("eno1", "down", "-1", true)
	mk("lo", "unknown", "", false)
	mk("docker0", "up", "", false)
	l := netLinks(root)
	if len(l) != 2 {
		t.Fatalf("physical links only: %+v", l)
	}
	for _, x := range l {
		switch x.Name {
		case "enp7s0":
			if x.State != "up" || x.SpeedMbps != 1000 {
				t.Errorf("enp7s0: %+v", x)
			}
		case "eno1":
			if x.State != "down" || x.SpeedMbps != 0 {
				t.Errorf("eno1: %+v", x)
			}
		}
	}
}

// The simulation stands in for apt and systemd: a check, an upgrade with its log, the refused reboot while a job
// runs, a scheduled and a cancelled reboot, the acknowledgement for the audit trail and the closed list of logs.
func TestOSOperationsInTheSimulation(t *testing.T) {
	dir := t.TempDir()
	saved := []string{optDir, etcDir, libDir, binPath, BackupDir, wgConfFile}
	t.Cleanup(func() {
		optDir, etcDir, libDir, binPath, BackupDir, wgConfFile = saved[0], saved[1], saved[2], saved[3], saved[4], saved[5]
		derivePaths()
	})
	a := NewServer(filepath.Join(dir, "a.sock"), filepath.Join(dir, "config.toml"), "0.86.0", "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	a.SetSimulation(dir)
	h := a.routes()
	call := func(method, path, body string) (int, map[string]any) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	code, st := call("GET", "/os", "")
	if code != 200 || st["supported"] != true || st["upgrade_count"].(float64) != 2 || st["security_count"].(float64) != 1 || st["reboot_required"] != false {
		t.Fatalf("state: %d %v", code, st)
	}
	if code, _ := call("POST", "/os/check", `{"actor":"admin","reason":"e2e"}`); code != 202 {
		t.Fatalf("check: %d", code)
	}
	waitPhase := func(kind string) map[string]any {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			_, st := call("GET", "/os", "")
			if st["job"] == nil && st["checking"] == false {
				for _, j := range st["history"].([]any) {
					if j.(map[string]any)["kind"] == kind {
						return st
					}
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("job %s did not finish", kind)
		return nil
	}
	waitPhase("check")
	code, j := call("POST", "/os/upgrade", `{"actor":"admin","reason":"monthly updates"}`)
	if code != 202 || j["phase"] != "running" {
		t.Fatalf("upgrade: %d %v", code, j)
	}
	if code, e := call("POST", "/os/reboot", `{"actor":"admin","reason":"x"}`); code != 409 || e["code"] != "busy" {
		t.Errorf("reboot during the upgrade: %d %v", code, e)
	}
	if code, _ := call("POST", "/os/upgrade", `{"actor":"admin","reason":"again"}`); code != 409 {
		t.Errorf("second upgrade: %d", code)
	}
	st = waitPhase("upgrade")
	last := st["last"].(map[string]any)
	if last["phase"] != "done" || last["upgrades"].(float64) != 2 || last["reboot_required"] != true || !strings.Contains(last["log"].(string), "2 upgraded") {
		t.Errorf("finished upgrade: %v", last)
	}
	if st["reboot_required"] != true || len(st["reboot_packages"].([]any)) != 1 || st["upgrade_count"].(float64) != 0 {
		t.Errorf("after the upgrade: %v", st)
	}
	if code, e := call("POST", "/os/reboot", `{"actor":"admin","reason":"kernel","delay_s":5}`); code != 400 {
		t.Errorf("delay below 30 s: %d %v", code, e)
	}
	code, rb := call("POST", "/os/reboot", `{"actor":"admin","reason":"kernel"}`)
	if code != 202 || rb["actor"] != "admin" {
		t.Fatalf("reboot: %d %v", code, rb)
	}
	if _, st := call("GET", "/os", ""); st["reboot"] == nil {
		t.Error("the scheduled reboot is in the state")
	}
	if code, _ := call("POST", "/os/reboot", `{"actor":"admin","reason":"again"}`); code != 409 {
		t.Errorf("second reboot: %d", code)
	}
	if code, _ := call("POST", "/os/reboot/cancel", `{"actor":"admin","reason":"not now"}`); code != 200 {
		t.Errorf("cancel: %d", code)
	}
	if code, _ := call("POST", "/os/reboot/cancel", `{"actor":"admin","reason":"not now"}`); code != 404 {
		t.Errorf("cancel without a reboot: %d", code)
	}
	ids := []string{}
	_, st = call("GET", "/os?jobs=1", "")
	for _, x := range st["history"].([]any) {
		ids = append(ids, x.(map[string]any)["id"].(string))
	}
	b, _ := json.Marshal(AckRequest{IDs: ids})
	if code, _ := call("POST", "/os/ack", string(b)); code != 200 {
		t.Errorf("ack: %d", code)
	}
	_, st = call("GET", "/os?jobs=1", "")
	for _, x := range st["history"].([]any) {
		if x.(map[string]any)["audited"] != true {
			t.Errorf("not acknowledged: %v", x)
		}
	}
	if code, _ := call("GET", "/logs?unit=../../etc/shadow", ""); code != 400 {
		t.Errorf("a log outside the list: %d", code)
	}
	if code, lr := call("GET", "/logs?unit=app&lines=50", ""); code != 200 || len(lr["lines"].([]any)) == 0 {
		t.Errorf("log of the app: %d %v", code, lr)
	}
	// the status carries the health (simulated RAID and link)
	req := httptest.NewRequest("GET", "/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var s Status
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	if s.Health == nil || len(s.Health.RAID) != 1 || len(s.Health.Links) != 1 || !s.Health.RebootRequired {
		t.Errorf("health: %+v", s.Health)
	}
}

func TestRunOSUpgradeRefusesABadID(t *testing.T) {
	if err := RunOSUpgrade(context.Background(), t.TempDir(), "../x", nil); err == nil {
		t.Error("a job id that is not the agent's")
	}
}

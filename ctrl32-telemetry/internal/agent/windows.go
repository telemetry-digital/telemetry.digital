package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Windows variant of the host operations (0.32.0): services through the Service Control Manager (sc.exe /
// Restart-Service), host facts through CIM, backups with pg_dump.exe of the bundled PostgreSQL. Caddy and
// WireGuard are Linux-only; on Windows TLS is configured directly in config.toml (http.tls_cert/tls_key).

const pgUnit = "ctrl32-telemetry-postgres"

func isWindows() bool { return runtime.GOOS == "windows" }

// parseScState maps the STATE line of `sc query` to the agent's active states.
func parseScState(out string) string {
	switch {
	case strings.Contains(out, "RUNNING"):
		return "active"
	case strings.Contains(out, "STOPPED"):
		return "inactive"
	case strings.Contains(out, "PENDING"):
		return "activating"
	case strings.Contains(out, "1060"):
		return "not-installed"
	}
	return "unknown"
}

// parseScStart maps the START_TYPE line of `sc qc` to enabled/manual/disabled.
func parseScStart(out string) string {
	switch {
	case strings.Contains(out, "AUTO_START"):
		return "enabled"
	case strings.Contains(out, "DEMAND_START"):
		return "manual"
	case strings.Contains(out, "DISABLED"):
		return "disabled"
	}
	return "unknown"
}

// svcState reports a Windows service: (active, enabled) like unitActive on Linux.
func svcState(ctx context.Context, name string) (string, string) {
	out, _ := run(ctx, 10*time.Second, "sc", "query", name)
	qc, _ := run(ctx, 10*time.Second, "sc", "qc", name)
	return parseScState(out), parseScStart(qc)
}

// restartServiceLater restarts a Windows service from a detached PowerShell after a delay, so the agent can
// answer the request first — and restart itself.
func restartServiceLater(name string, delay time.Duration) {
	script := fmt.Sprintf("Start-Sleep -Milliseconds %d; Restart-Service -Name '%s' -Force", delay.Milliseconds(), strings.ReplaceAll(name, "'", ""))
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	_ = cmd.Start()
}

// psJSON runs a PowerShell expression that ends with ConvertTo-Json and decodes the object.
func psJSON(ctx context.Context, script string) (map[string]any, error) {
	out, err := run(ctx, 20*time.Second, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &m); err != nil {
		return nil, err
	}
	return m, nil
}

func hostInfoWindows() HostInfo {
	h := HostInfo{Arch: runtime.GOARCH, CPUs: runtime.NumCPU(), Time: time.Now()}
	h.Hostname, _ = os.Hostname()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if m, err := psJSON(ctx, `$o = Get-CimInstance Win32_OperatingSystem; [pscustomobject]@{caption=$o.Caption; version=$o.Version; total=[int64]$o.TotalVisibleMemorySize; free=[int64]$o.FreePhysicalMemory; uptime=[int64]([Environment]::TickCount64/1000); tz=(Get-TimeZone).Id} | ConvertTo-Json -Compress`); err == nil {
		h.OS, _ = m["caption"].(string)
		h.Kernel, _ = m["version"].(string)
		if v, ok := m["total"].(float64); ok {
			h.MemTotalMB = int64(v) / 1024
		}
		if v, ok := m["free"].(float64); ok {
			h.MemAvailMB = int64(v) / 1024
		}
		if v, ok := m["uptime"].(float64); ok {
			h.UptimeSeconds = int64(v)
		}
		h.Timezone, _ = m["tz"].(string)
	} else {
		h.OS = "Windows"
	}
	seen := map[string]bool{}
	for _, p := range []string{os.Getenv("SystemDrive") + `\`, filepath.VolumeName(etcDir) + `\`} {
		if p == `\` || seen[p] {
			continue
		}
		seen[p] = true
		if d, ok := diskInfo(p); ok {
			h.Disks = append(h.Disks, d)
		}
	}
	return h
}

// findPgDump locates pg_dump.exe: the configured path, the bundled PostgreSQL next to the binary, the standard
// EDB installation directories, then PATH.
func findPgDump(hint string) string {
	var candidates []string
	if hint != "" {
		candidates = append(candidates, hint)
	}
	candidates = append(candidates, filepath.Join(filepath.Dir(binPath), "pgsql", "bin", "pg_dump.exe"))
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	candidates = append(candidates, filepath.Join(pf, "ctrl32-telemetry", "pgsql", "bin", "pg_dump.exe"))
	if m, _ := filepath.Glob(filepath.Join(pf, "PostgreSQL", "*", "bin", "pg_dump.exe")); len(m) > 0 {
		candidates = append(candidates, m[len(m)-1])
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	if p, err := exec.LookPath("pg_dump"); err == nil {
		return p
	}
	return ""
}

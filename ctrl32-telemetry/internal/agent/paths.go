package agent

import (
	"os"
	"path/filepath"
	"runtime"
)

// Paths are fixed by the installers (deploy/install.sh on Linux, deploy/install.ps1 on Windows); the agent never
// takes them from a request. On Windows everything lives under %ProgramData%\ctrl32-telemetry and the binary is
// the running executable (the app service and the agent service share it).
var (
	optDir  = "/opt/ctrl32-telemetry"
	etcDir  = "/etc/ctrl32-telemetry"
	binPath = "/usr/local/bin/ctrl32-telemetry"
	// BackupDir is exported: the web service streams downloads from here.
	BackupDir  = "/var/lib/ctrl32-telemetry/backups"
	wgConfFile = "/etc/wireguard/wg0.conf"

	caddyStateFile, caddyEnvFile, caddyFile, caddyCompose, caddyDocker, certSyncScript, tlsChain string
	// caddySites holds additional sites (*.caddy) the Caddyfile imports, e.g. remote management; kept when the
	// Caddyfile is regenerated from the UI
	caddySites                                                  string
	wgDir, wgStateFile, wgPeersFile, wgKeyFile, backupStateFile string
)

func init() {
	if runtime.GOOS == "windows" {
		pd := os.Getenv("ProgramData")
		if pd == "" {
			pd = `C:\ProgramData`
		}
		etcDir = filepath.Join(pd, "ctrl32-telemetry")
		optDir = etcDir
		if exe, err := os.Executable(); err == nil {
			binPath = exe
		}
		BackupDir = filepath.Join(etcDir, "backups")
		wgConfFile = filepath.Join(etcDir, "wireguard", "wg0.conf")
	}
	derivePaths()
}

// derivePaths recomputes the files that hang off optDir/etcDir (tests override the roots and call it again).
func derivePaths() {
	caddyStateFile = filepath.Join(optDir, "caddy.json")
	caddyEnvFile = filepath.Join(optDir, "caddy.env")
	caddyFile = filepath.Join(optDir, "Caddyfile")
	caddyCompose = filepath.Join(optDir, "docker-compose.caddy.yml")
	caddySites = filepath.Join(optDir, "caddy-sites")
	caddyDocker = filepath.Join(optDir, "caddy-cloudflare.Dockerfile")
	certSyncScript = filepath.Join(optDir, "mqtt-certs-sync.sh")
	tlsChain = filepath.Join(etcDir, "tls", "fullchain.pem")
	wgDir = filepath.Join(etcDir, "wireguard")
	wgStateFile = filepath.Join(wgDir, "state.json")
	wgPeersFile = filepath.Join(wgDir, "peers.json")
	wgKeyFile = filepath.Join(wgDir, "server.key")
	backupStateFile = filepath.Join(etcDir, "backup.json")
}

package agent

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/release"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/uptunnel"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/vpn"
)

// Unit and container names; paths are in paths.go (per operating system).
const (
	appUnit            = "ctrl32-telemetry"
	agentUnit          = "ctrl32-telemetry-agent"
	wgUnit             = "wg-quick@wg0"
	caddyContainer     = "ctrl32-telemetry-caddy"
	pgContainerDefault = "ctrl32-telemetry-postgres"
	caddyImage         = "caddy:2-alpine"
	caddyCFImage       = "ctrl32-telemetry/caddy-cloudflare:latest"
	appGroup           = "ctrl32"
)

var (
	domainRe  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)
	emailRe   = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	versionRe = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
)

// Server is the root-side process.
type Server struct {
	socket      string
	cfgPath     string
	version     string
	updateURL   string
	log         *slog.Logger
	started     time.Time
	pgDump      string     // Windows: path to pg_dump.exe (optional; found next to the binary or in Program Files)
	pgContainer string     // Docker container of the bundled PostgreSQL
	mu          sync.Mutex // one mutating operation at a time
	// host access of the VPN operations (vpn.go): replaced by fakes in tests and by the simulation
	exec          func(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error)
	simulate      bool
	vpnHost       bool         // the host can run the VPN with kernel WireGuard and nftables (Linux); tests set it on any platform
	hub           userspaceHub // Windows: WireGuard and the access rules inside the agent (ADR-0020); nil elsewhere
	localNets     func() []netip.Prefix
	setForwarding func(on bool) error
	// uplink to a hub (uplink.go): the userspace device starter (Windows; nil elsewhere), the running device and the
	// HTTPS transport, and per-interface forwarding (replaced by fakes in tests and by the simulation)
	startUplink     uplinkStartFunc
	upDev           uplinkDevice
	tunnel          *uptunnel.Client
	tunnelURL       string
	tunnelKey       string
	tunnelWatchStop context.CancelFunc
	// tunnelWatchEvery: how often the agent puts WireGuard back on the HTTPS transport (0 = never: tests, simulation)
	tunnelWatchEvery time.Duration
	ifForwarding     func(iface string) (bool, error)
	setIfForwarding  func(iface string, on bool) error
	// installation of the object counting detector (analytics.go)
	an analyticsRunner
	// updates (update.go): [agent] max_package_mb and allow_unsigned_updates (development builds only)
	maxPackageMB  int
	allowUnsigned bool
	direct        bool // the command line installs without the agent service (update.go)
	// testKeys replaces the trusted release keys in unit tests (nil = release.TrustedKeys)
	testKeys map[string]ed25519.PublicKey
}

func NewServer(socket, cfgPath, version, updateURL string, log *slog.Logger) *Server {
	if updateURL == "" {
		updateURL = "https://portal.telemetry.digital/dl"
	}
	return &Server{socket: socket, cfgPath: cfgPath, version: version, updateURL: strings.TrimRight(updateURL, "/"), log: log, started: time.Now(), pgContainer: pgContainerDefault,
		exec: run, localNets: vpn.LocalNets, setForwarding: setWGForwarding, vpnHost: !isWindows(), hub: newUserspaceHub(log),
		startUplink: newUserspaceUplink(), ifForwarding: readForwarding, setIfForwarding: writeForwarding, tunnelWatchEvery: 5 * time.Second}
}

// SetDBTools configures where the database tooling is (config [agent] pg_dump / pg_container).
func (a *Server) SetDBTools(pgDump, pgContainer string) {
	a.pgDump = pgDump
	if pgContainer != "" {
		a.pgContainer = pgContainer
	}
}

// Serve listens on the Unix socket until ctx is done (Windows 10+ supports AF_UNIX sockets as well).
func (a *Server) Serve(ctx context.Context) error {
	_ = os.MkdirAll(filepath.Dir(a.socket), 0o755)
	_ = os.Remove(a.socket)
	ln, err := net.Listen("unix", a.socket)
	if err != nil {
		return err
	}
	if !isWindows() {
		if g, err := user.LookupGroup(appGroup); err == nil {
			gid, _ := strconv.Atoi(g.Gid)
			_ = os.Chown(a.socket, 0, gid)
		}
		_ = os.Chmod(a.socket, 0o660)
	}
	srv := &http.Server{Handler: a.routes(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
		_ = os.Remove(a.socket)
		if a.vpnMode() == ModeUserspace {
			a.hub.Stop()
		}
		a.mu.Lock()
		if a.upDev != nil {
			a.upDev.Stop()
		}
		a.stopTunnel()
		a.mu.Unlock()
	}()
	a.startHubFromFiles()
	a.startUplinkFromFiles()
	a.log.Info("agent listening", "socket", a.socket, "version", a.version)
	if !a.simulate {
		go a.runScheduler(ctx)
	}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// backupRoutes: backups, the offsite target and the restore test (backup*.go).
func (a *Server) backupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /backups", a.hBackups)
	mux.HandleFunc("POST /backups", a.hCreateBackup)
	mux.HandleFunc("DELETE /backups/{name}", a.hDeleteBackup)
	mux.HandleFunc("PUT /backups/schedule", a.hBackupSchedule)
	mux.HandleFunc("PUT /backups/offsite", a.hSetOffsite)
	mux.HandleFunc("POST /backups/offsite/test", a.hOffsiteTest)
	mux.HandleFunc("POST /backups/offsite/sync", a.hOffsiteSync)
	mux.HandleFunc("PUT /backups/restore-test", a.hRestoreTestSchedule)
	mux.HandleFunc("POST /backups/restore-test", a.hRunRestoreTest)
	mux.HandleFunc("POST /backups/ack", a.hBackupAck)
}

// routes is the closed list of operations (ADR-0012). The VPN simulation of development and tests (vpn_simulate)
// serves only the VPN operations and refuses everything else, so it can never touch services, Caddy or updates.
func (a *Server) routes() http.Handler {
	mux := http.NewServeMux()
	if a.simulate {
		mux.HandleFunc("GET /status", a.hStatusSimulated)
		mux.HandleFunc("GET /wireguard", a.hWireGuard)
		mux.HandleFunc("PUT /wireguard", a.hSetWireGuard)
		mux.HandleFunc("PUT /vpn", a.hApplyVPN)
		a.uplinkRoutes(mux)
		a.analyticsRoutes(mux) // stand-in downloads into the simulation directory (setup.SimulatedArtifacts)
		a.updateRoutes(mux)    // installs into the simulation directory; no restart, no database restore
		a.backupRoutes(mux)    // backups in the simulation directory with a stand-in dump; no database restore
		a.osRoutes(mux)        // package upgrades, jobs and reboots are simulated (simUpgrades, simulateUpgrade)
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			writeErr(w, 501, "not available in the VPN simulation of the agent (vpn_simulate)")
		})
		return mux
	}
	mux.HandleFunc("GET /status", a.hStatus)
	mux.HandleFunc("POST /restart", a.hRestart)
	mux.HandleFunc("PUT /caddy", a.hCaddy)
	mux.HandleFunc("GET /wireguard", a.hWireGuard)
	mux.HandleFunc("PUT /wireguard", a.hSetWireGuard)
	mux.HandleFunc("PUT /vpn", a.hApplyVPN)
	a.uplinkRoutes(mux)
	a.analyticsRoutes(mux)
	a.updateRoutes(mux)
	a.backupRoutes(mux)
	a.osRoutes(mux)
	mux.HandleFunc("POST /replica/promote", a.hReplicaPromote)
	return mux
}

// ---- helpers --------------------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// run executes a program without a shell; the output is trimmed and returned with the error.
func run(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		if s != "" {
			return s, fmt.Errorf("%s: %s", name, firstLine(s))
		}
		return s, fmt.Errorf("%s: %w", name, err)
	}
	return s, nil
}

func runInput(ctx context.Context, input string, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %s", name, firstLine(strings.TrimSpace(string(out))))
	}
	return strings.TrimSpace(string(out)), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func loadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func saveJSON(path string, v any, mode os.FileMode) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o750)
	return writeFileAtomic(path, b, mode)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	_ = os.Chmod(tmp, mode)
	return os.Rename(tmp, path)
}

func unitActive(ctx context.Context, unit string) (string, string) {
	if isWindows() {
		return svcState(ctx, unit)
	}
	act, _ := run(ctx, 5*time.Second, "systemctl", "is-active", unit)
	en, _ := run(ctx, 5*time.Second, "systemctl", "is-enabled", unit)
	if act == "" {
		act = "unknown"
	}
	if en == "" {
		en = "unknown"
	}
	return firstLine(act), firstLine(en)
}

// setConfigValue edits one `key = value` line inside a [section] of the TOML config, keeping everything else.
// An empty value removes the line. The file stays root:ctrl32 0640.
func (a *Server) setConfigValue(section, key, value string) (bool, error) {
	b, err := os.ReadFile(a.cfgPath)
	if err != nil {
		return false, err
	}
	lines := strings.Split(string(b), "\n")
	header := "[" + section + "]"
	secStart, secEnd := -1, len(lines)
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if secStart < 0 {
			if t == header {
				secStart = i
			}
			continue
		}
		if strings.HasPrefix(t, "[") {
			secEnd = i
			break
		}
	}
	newLine := key + " = " + strconv.Quote(value)
	changed := false
	if secStart < 0 {
		if value == "" {
			return false, nil
		}
		lines = append(lines, "", header, newLine)
		changed = true
	} else {
		found := false
		for i := secStart + 1; i < secEnd; i++ {
			t := strings.TrimSpace(lines[i])
			if strings.HasPrefix(t, key+" ") || strings.HasPrefix(t, key+"=") {
				found = true
				if value == "" {
					lines = append(lines[:i], lines[i+1:]...)
					changed = true
				} else if lines[i] != newLine {
					lines[i] = newLine
					changed = true
				}
				break
			}
		}
		if !found && value != "" {
			lines = append(lines[:secStart+1], append([]string{newLine}, lines[secStart+1:]...)...)
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	if err := a.writeConfig([]byte(strings.Join(lines, "\n"))); err != nil {
		return false, err
	}
	return true, nil
}

// writeConfig replaces config.toml atomically and keeps its mode and group (root:ctrl32 0640 on Linux).
func (a *Server) writeConfig(data []byte) error {
	st, _ := os.Stat(a.cfgPath)
	mode := os.FileMode(0o640)
	if st != nil {
		mode = st.Mode().Perm()
	}
	if err := writeFileAtomic(a.cfgPath, data, mode); err != nil {
		return err
	}
	if st != nil && !isWindows() {
		if sys, ok := st.Sys().(interface{ Gid() uint32 }); ok {
			_ = os.Chown(a.cfgPath, 0, int(sys.Gid()))
		} else if g, err := user.LookupGroup(appGroup); err == nil {
			gid, _ := strconv.Atoi(g.Gid)
			_ = os.Chown(a.cfgPath, 0, gid)
		}
	}
	return nil
}

func (a *Server) restartAppLater() {
	if isWindows() {
		restartServiceLater(appUnit, 800*time.Millisecond)
		return
	}
	go func() {
		time.Sleep(1500 * time.Millisecond)
		if _, err := run(context.Background(), 60*time.Second, "systemctl", "restart", appUnit); err != nil {
			a.log.Error("app restart failed", "err", err)
		}
	}()
}

// ---- status ---------------------------------------------------------------------------------------

func (a *Server) hStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st := Status{Agent: AgentInfo{Version: a.version, StartedAt: a.started, Socket: a.socket, OS: runtime.GOOS}, Versions: map[string]string{}}
	st.Host = hostInfo()
	units := []struct{ unit, label string }{{appUnit, "ctrl32 telemetry"}, {agentUnit, "server agent"}, {"docker", "Docker"}, {wgUnit, "WireGuard (wg0)"}}
	if isWindows() {
		units = []struct{ unit, label string }{{appUnit, "ctrl32 telemetry"}, {agentUnit, "server agent"}, {pgUnit, "PostgreSQL"}}
	}
	for _, u := range units {
		act, en := unitActive(ctx, u.unit)
		st.Services = append(st.Services, ServiceState{Unit: u.unit, Label: u.label, Active: act, Enabled: en})
	}
	if out, err := run(ctx, 10*time.Second, "docker", "ps", "-a", "--filter", "name=ctrl32-telemetry", "--format", "{{.Names}}|{{.Image}}|{{.Status}}"); err == nil {
		for _, l := range strings.Split(out, "\n") {
			p := strings.SplitN(l, "|", 3)
			if len(p) == 3 {
				st.Containers = append(st.Containers, ContainerInfo{Name: p[0], Image: p[1], Status: p[2]})
			}
		}
	}
	st.Versions["ctrl32-telemetry"] = a.version
	if out, err := run(ctx, 5*time.Second, "typst", "--version"); err == nil {
		st.Versions["typst"] = out
	}
	if out, err := run(ctx, 10*time.Second, "docker", "exec", caddyContainer, "caddy", "version"); err == nil {
		st.Versions["caddy"] = firstLine(out)
	}
	if out, err := run(ctx, 10*time.Second, "docker", "exec", a.pgContainer, "postgres", "--version"); err == nil {
		st.Versions["postgresql"] = out
	}
	if out, err := run(ctx, 5*time.Second, "wg", "--version"); err == nil {
		st.Versions["wireguard"] = out
	}
	if out, err := run(ctx, 5*time.Second, "docker", "--version"); err == nil {
		st.Versions["docker"] = out
	}
	if isWindows() {
		if pd := findPgDump(a.pgDump); pd != "" {
			if out, err := run(ctx, 10*time.Second, pd, "--version"); err == nil {
				st.Versions["postgres"] = firstLine(out)
			}
		}
	}
	st.Caddy = a.caddyState()
	st.WireGuard = a.wgState(ctx)
	st.Health = a.hostHealth(ctx)
	writeJSON(w, 200, st)
}

func hostInfo() HostInfo {
	if isWindows() {
		return hostInfoWindows()
	}
	h := HostInfo{Arch: runtime.GOARCH, CPUs: runtime.NumCPU(), Time: time.Now()}
	h.Hostname, _ = os.Hostname()
	for _, l := range strings.Split(readFile("/etc/os-release"), "\n") {
		if strings.HasPrefix(l, "PRETTY_NAME=") {
			h.OS = strings.Trim(strings.TrimPrefix(l, "PRETTY_NAME="), `"`)
		}
	}
	h.Kernel = strings.TrimSpace(readFile("/proc/sys/kernel/osrelease"))
	if f := strings.Fields(readFile("/proc/uptime")); len(f) > 0 {
		if v, err := strconv.ParseFloat(f[0], 64); err == nil {
			h.UptimeSeconds = int64(v)
		}
	}
	if f := strings.Fields(readFile("/proc/loadavg")); len(f) >= 3 {
		for _, x := range f[:3] {
			v, _ := strconv.ParseFloat(x, 64)
			h.Load = append(h.Load, v)
		}
	}
	for _, l := range strings.Split(readFile("/proc/meminfo"), "\n") {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		kb, _ := strconv.ParseInt(f[1], 10, 64)
		switch f[0] {
		case "MemTotal:":
			h.MemTotalMB = kb / 1024
		case "MemAvailable:":
			h.MemAvailMB = kb / 1024
		}
	}
	// the recordings often have their own disk (or RAID); a path on the same file system as the one before is skipped
	for _, p := range []string{"/", "/var/lib/ctrl32-telemetry", "/var/lib/ctrl32-telemetry/video"} {
		if d, ok := diskInfo(p); ok && !sameDisk(h.Disks, d) {
			h.Disks = append(h.Disks, d)
		}
	}
	if tz := strings.TrimSpace(readFile("/etc/timezone")); tz != "" {
		h.Timezone = tz
	} else if l, err := os.Readlink("/etc/localtime"); err == nil {
		if i := strings.Index(l, "zoneinfo/"); i >= 0 {
			h.Timezone = l[i+len("zoneinfo/"):]
		}
	}
	return h
}

// sameDisk: the file system of d is already listed.
func sameDisk(list []DiskInfo, d DiskInfo) bool {
	for _, x := range list {
		if d.dev != 0 && x.dev == d.dev {
			return true
		}
	}
	return false
}

// ---- Caddy ----------------------------------------------------------------------------------------

type caddyStored struct {
	Domain       string    `json:"domain"`
	ExtraDomains []string  `json:"extra_domains"`
	Email        string    `json:"email"`
	DNSProvider  string    `json:"dns_provider"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (a *Server) caddyState() CaddyState {
	var cs caddyStored
	st := CaddyState{ExtraDomains: []string{}}
	if loadJSON(caddyStateFile, &cs) == nil {
		st.Managed, st.Domain, st.ExtraDomains, st.Email, st.DNSProvider, st.UpdatedAt = true, cs.Domain, cs.ExtraDomains, cs.Email, cs.DNSProvider, cs.UpdatedAt
		if st.ExtraDomains == nil {
			st.ExtraDomains = []string{}
		}
	} else {
		// installer default: "domain, fallback.sslip.io {"
		for _, l := range strings.Split(readFile(caddyFile), "\n") {
			t := strings.TrimSpace(l)
			if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "{") {
				continue
			}
			if strings.HasSuffix(t, "{") {
				names := strings.Split(strings.TrimSpace(strings.TrimSuffix(t, "{")), ",")
				for i, n := range names {
					n = strings.TrimSpace(n)
					if i == 0 {
						st.Domain = n
					} else if n != "" {
						st.ExtraDomains = append(st.ExtraDomains, n)
					}
				}
				break
			}
		}
	}
	st.HasDNSToken = strings.Contains(readFile(caddyEnvFile), "CF_API_TOKEN=")
	st.Caddyfile = readFile(caddyFile)
	if block, _ := pem.Decode([]byte(readFile(tlsChain))); block != nil {
		if c, err := x509.ParseCertificate(block.Bytes); err == nil {
			st.CertSubject, st.CertIssuer, st.CertExpires, st.CertDomains = c.Subject.CommonName, c.Issuer.CommonName, c.NotAfter, c.DNSNames
		}
	}
	return st
}

func caddyfileFor(cs caddyStored) string {
	var b strings.Builder
	b.WriteString("# ctrl32 telemetry — generated by the server agent; edit in the UI under System → Domain and TLS\n")
	if cs.Email != "" {
		b.WriteString("{\n\temail " + cs.Email + "\n}\n\n")
	}
	names := append([]string{cs.Domain}, cs.ExtraDomains...)
	b.WriteString(strings.Join(names, ", ") + " {\n")
	b.WriteString("\tencode gzip\n\theader {\n\t\tStrict-Transport-Security \"max-age=31536000\"\n\t\tX-Content-Type-Options nosniff\n\t\tReferrer-Policy strict-origin-when-cross-origin\n\t\t-Server\n\t}\n")
	if cs.DNSProvider == "cloudflare" {
		b.WriteString("\ttls {\n\t\tdns cloudflare {env.CF_API_TOKEN}\n\t}\n")
	}
	b.WriteString("\thandle /install.sh {\n\t\troot * /srv/ctrl32\n\t\tfile_server\n\t}\n\thandle /install.ps1 {\n\t\troot * /srv/ctrl32\n\t\tfile_server\n\t}\n\thandle_path /dl/* {\n\t\troot * /srv/ctrl32/dl\n\t\tfile_server browse\n\t}\n\treverse_proxy 127.0.0.1:8080\n}\n")
	// additional sites of the host (e.g. remote management, docs/remote-management.md) survive a regeneration
	b.WriteString("\n# additional sites: " + caddySites + "/*.caddy\nimport sites/*.caddy\n")
	return b.String()
}

func composeFor(image string, withEnv bool) string {
	env := ""
	if withEnv {
		env = "    env_file:\n      - " + caddyEnvFile + "\n"
	}
	return "name: ctrl32-telemetry-caddy\nservices:\n  caddy:\n    image: " + image + "\n    container_name: " + caddyContainer + "\n    restart: unless-stopped\n    network_mode: host\n" + env +
		"    volumes:\n      - " + caddyFile + ":/etc/caddy/Caddyfile:ro\n      - " + caddySites + ":/etc/caddy/sites:ro\n      - /var/www/ctrl32:/srv/ctrl32:ro\n      - ctrl32-telemetry-caddy-data:/data\n      - ctrl32-telemetry-caddy-config:/config\nvolumes:\n  ctrl32-telemetry-caddy-data:\n    name: ctrl32-telemetry-caddy-data\n  ctrl32-telemetry-caddy-config:\n    name: ctrl32-telemetry-caddy-config\n"
}

const cloudflareDockerfile = "FROM caddy:2-builder-alpine AS builder\nRUN xcaddy build --with github.com/caddy-dns/cloudflare\nFROM caddy:2-alpine\nCOPY --from=builder /usr/bin/caddy /usr/bin/caddy\n"

func certSyncFor(names []string) string {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\n# generated by the server agent — copies the Caddy certificate for the MQTT TLS listener\nset -euo pipefail\nSRC=$(docker volume inspect ctrl32-telemetry-caddy-data -f '{{.Mountpoint}}')/caddy/certificates\nfor NAME in")
	for _, n := range names {
		b.WriteString(" " + strconv.Quote(n))
	}
	b.WriteString("; do\n  CRT=$(find \"$SRC\" -path \"*/$NAME/$NAME.crt\" 2>/dev/null | head -1 || true)\n  KEY=$(find \"$SRC\" -path \"*/$NAME/$NAME.key\" 2>/dev/null | head -1 || true)\n  [[ -n \"$CRT\" && -n \"$KEY\" ]] && break\ndone\n[[ -n \"$CRT\" && -n \"$KEY\" ]] || exit 0\nif ! cmp -s \"$CRT\" " + tlsChain + " 2>/dev/null; then\n  install -m 0640 -o root -g ctrl32 \"$CRT\" " + tlsChain + "\n  install -m 0640 -o root -g ctrl32 \"$KEY\" " + etcDir + "/tls/privkey.pem\n  systemctl restart ctrl32-telemetry\n  logger -t ctrl32-telemetry \"MQTT TLS certificate updated from Caddy\"\nfi\n")
	return b.String()
}

func (a *Server) hCaddy(w http.ResponseWriter, r *http.Request) {
	if isWindows() {
		writeErr(w, 501, "the domain and TLS proxy (Caddy) is not managed on Windows; set http.tls_cert and http.tls_key in config.toml")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var in CaddyRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request: "+err.Error())
		return
	}
	in.Domain = strings.ToLower(strings.TrimSpace(in.Domain))
	in.Email = strings.TrimSpace(in.Email)
	if !domainRe.MatchString(in.Domain) {
		writeErr(w, 400, "invalid domain")
		return
	}
	extras := []string{}
	for _, d := range in.ExtraDomains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		if !domainRe.MatchString(d) {
			writeErr(w, 400, "invalid extra domain: "+d)
			return
		}
		extras = append(extras, d)
	}
	if in.Email != "" && !emailRe.MatchString(in.Email) {
		writeErr(w, 400, "invalid e-mail")
		return
	}
	if in.DNSProvider != "" && in.DNSProvider != "cloudflare" {
		writeErr(w, 400, "unsupported DNS provider")
		return
	}
	ctx := r.Context()
	if _, err := run(ctx, 10*time.Second, "docker", "--version"); err != nil {
		writeErr(w, 400, "Docker is required for the reverse proxy")
		return
	}
	cs := caddyStored{Domain: in.Domain, ExtraDomains: extras, Email: in.Email, DNSProvider: in.DNSProvider, UpdatedAt: time.Now()}
	image := caddyImage
	if in.DNSProvider == "cloudflare" {
		token := strings.TrimSpace(in.CloudflareToken)
		if token == "" {
			for _, l := range strings.Split(readFile(caddyEnvFile), "\n") {
				if strings.HasPrefix(l, "CF_API_TOKEN=") {
					token = strings.TrimPrefix(l, "CF_API_TOKEN=")
				}
			}
		}
		if token == "" || strings.ContainsAny(token, "\n\r\"'`$") {
			writeErr(w, 400, "a Cloudflare API token (Zone:DNS:Edit) is required for the DNS challenge")
			return
		}
		if err := writeFileAtomic(caddyEnvFile, []byte("CF_API_TOKEN="+token+"\n"), 0o600); err != nil {
			writeErr(w, 500, "cannot store the token: "+err.Error())
			return
		}
		image = caddyCFImage
		if _, err := run(ctx, 10*time.Second, "docker", "image", "inspect", caddyCFImage); err != nil {
			a.log.Info("building Caddy image with the Cloudflare DNS module")
			if err := writeFileAtomic(caddyDocker, []byte(cloudflareDockerfile), 0o644); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			if out, err := run(ctx, 12*time.Minute, "docker", "build", "-t", caddyCFImage, "-f", caddyDocker, optDir); err != nil {
				a.log.Error("caddy image build failed", "out", out)
				writeErr(w, 500, "building the Caddy image with the Cloudflare module failed: "+err.Error())
				return
			}
		}
	} else {
		_ = os.Remove(caddyEnvFile)
	}
	// validate the new Caddyfile before touching the running proxy
	if err := os.MkdirAll(caddySites, 0o755); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	tmp := caddyFile + ".new"
	if err := writeFileAtomic(tmp, []byte(caddyfileFor(cs)), 0o644); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if out, err := run(ctx, 2*time.Minute, "docker", "run", "--rm", "-e", "CF_API_TOKEN=validation", "-v", tmp+":/etc/caddy/Caddyfile:ro", "-v", caddySites+":/etc/caddy/sites:ro", image, "caddy", "validate", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"); err != nil {
		_ = os.Remove(tmp)
		writeErr(w, 400, "Caddy rejected the configuration: "+firstLine(out))
		return
	}
	// Write in place: the container bind-mounts this file, so a rename would leave it looking at the old inode.
	data, err := os.ReadFile(tmp)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := os.WriteFile(caddyFile, data, 0o644); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = os.Remove(tmp)
	if err := writeFileAtomic(caddyCompose, []byte(composeFor(image, in.DNSProvider == "cloudflare")), 0o644); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if out, err := run(ctx, 5*time.Minute, "docker", "compose", "-f", caddyCompose, "up", "-d", "caddy"); err != nil {
		writeErr(w, 500, "starting Caddy failed: "+firstLine(out))
		return
	}
	_, _ = run(ctx, time.Minute, "docker", "exec", caddyContainer, "caddy", "reload", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile")
	_ = writeFileAtomic(certSyncScript, []byte(certSyncFor(append([]string{cs.Domain}, cs.ExtraDomains...))), 0o755)
	if err := saveJSON(caddyStateFile, cs, 0o644); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	changed, err := a.setConfigValue("http", "base_url", "https://"+cs.Domain)
	if err != nil {
		writeErr(w, 500, "config update failed: "+err.Error())
		return
	}
	msg := "reverse proxy reloaded"
	if changed {
		a.restartAppLater()
		msg = "reverse proxy reloaded; the service restarts with the new base URL"
	}
	a.log.Info("caddy configured", "domain", cs.Domain, "extra", cs.ExtraDomains, "dns", cs.DNSProvider)
	writeJSON(w, 200, Result{OK: true, Message: msg})
}

// ---- restart --------------------------------------------------------------------------------------

func (a *Server) hRestart(w http.ResponseWriter, r *http.Request) {
	var in RestartRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request")
		return
	}
	ctx := r.Context()
	switch in.Unit {
	case "app":
		a.restartAppLater()
		writeJSON(w, 200, Result{OK: true, Message: "service restarting"})
	case "agent":
		if isWindows() {
			restartServiceLater(agentUnit, time.Second)
			writeJSON(w, 200, Result{OK: true, Message: "agent restarting"})
			return
		}
		go func() {
			time.Sleep(time.Second)
			_, _ = run(context.Background(), 30*time.Second, "systemctl", "restart", agentUnit)
		}()
		writeJSON(w, 200, Result{OK: true, Message: "agent restarting"})
	case "caddy", "wireguard":
		if isWindows() {
			writeErr(w, 501, in.Unit+" is not managed on Windows")
			return
		}
		if in.Unit == "wireguard" {
			if out, err := run(ctx, time.Minute, "systemctl", "restart", wgUnit); err != nil {
				writeErr(w, 500, firstLine(out))
				return
			}
			writeJSON(w, 200, Result{OK: true, Message: "WireGuard restarted"})
			break
		}
		if out, err := run(ctx, time.Minute, "docker", "restart", caddyContainer); err != nil {
			writeErr(w, 500, firstLine(out))
			return
		}
		writeJSON(w, 200, Result{OK: true, Message: "Caddy restarted"})
	default:
		writeErr(w, 400, "unknown unit")
	}
	a.log.Info("restart requested", "unit", in.Unit)
}

// ---- update ---------------------------------------------------------------------------------------

// updateEntitled checks a downloaded binary against the licence's updates_until (YYYY-MM-DD, empty = no limit) using
// the release date the binary reports in `version` (internal/release). It returns "" when it may be installed. The
// running system is never touched here: a refused update leaves the current binary as it is.
func updateEntitled(updatesUntil, versionOut string, now time.Time) string {
	if updatesUntil == "" {
		return ""
	}
	until, err := time.Parse(release.Layout, updatesUntil)
	if err != nil {
		return "invalid updates_until"
	}
	released, ok := release.FromVersionOutput(versionOut)
	if !ok {
		// a binary without a release date (before 0.66.0, or a build without it): only while the updates have not
		// ended — anything published up to now is then within them
		if now.UTC().After(until.Add(24*time.Hour - time.Nanosecond)) {
			return "the release date of this version is unknown and the updates included in the licence ended on " + updatesUntil + "; renew the licence to install it"
		}
		return ""
	}
	if !release.Covered(released, until) {
		return "this version was released on " + released.Format(release.Layout) + ", after the updates included in the licence ended (" + updatesUntil +
			"); the system keeps running — renew the licence to install newer versions"
	}
	return ""
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

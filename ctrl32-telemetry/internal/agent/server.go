package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	nameRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,62}$`)
	keyRe     = regexp.MustCompile(`^[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]=$`)
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
}

func NewServer(socket, cfgPath, version, updateURL string, log *slog.Logger) *Server {
	if updateURL == "" {
		updateURL = "https://telemetry.digital/dl"
	}
	return &Server{socket: socket, cfgPath: cfgPath, version: version, updateURL: strings.TrimRight(updateURL, "/"), log: log, started: time.Now(), pgContainer: pgContainerDefault}
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
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", a.hStatus)
	mux.HandleFunc("POST /restart", a.hRestart)
	mux.HandleFunc("PUT /caddy", a.hCaddy)
	mux.HandleFunc("GET /wireguard", a.hWireGuard)
	mux.HandleFunc("PUT /wireguard", a.hSetWireGuard)
	mux.HandleFunc("POST /wireguard/peers", a.hAddPeer)
	mux.HandleFunc("DELETE /wireguard/peers/{id}", a.hRemovePeer)
	mux.HandleFunc("POST /update", a.hUpdate)
	mux.HandleFunc("GET /backups", a.hBackups)
	mux.HandleFunc("POST /backups", a.hCreateBackup)
	mux.HandleFunc("DELETE /backups/{name}", a.hDeleteBackup)
	mux.HandleFunc("PUT /backups/schedule", a.hBackupSchedule)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { <-ctx.Done(); _ = srv.Close(); _ = os.Remove(a.socket) }()
	a.log.Info("agent listening", "socket", a.socket, "version", a.version)
	go a.runScheduler(ctx)
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
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
	st, _ := os.Stat(a.cfgPath)
	mode := os.FileMode(0o640)
	if st != nil {
		mode = st.Mode().Perm()
	}
	if err := writeFileAtomic(a.cfgPath, []byte(strings.Join(lines, "\n")), mode); err != nil {
		return false, err
	}
	if st != nil {
		if sys, ok := st.Sys().(interface{ Gid() uint32 }); ok {
			_ = os.Chown(a.cfgPath, 0, int(sys.Gid()))
		} else if g, err := user.LookupGroup(appGroup); err == nil {
			gid, _ := strconv.Atoi(g.Gid)
			_ = os.Chown(a.cfgPath, 0, gid)
		}
	}
	return true, nil
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
	for _, p := range []string{"/", "/var/lib/ctrl32-telemetry"} {
		if d, ok := diskInfo(p); ok {
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
	return b.String()
}

func composeFor(image string, withEnv bool) string {
	env := ""
	if withEnv {
		env = "    env_file:\n      - " + caddyEnvFile + "\n"
	}
	return "name: ctrl32-telemetry-caddy\nservices:\n  caddy:\n    image: " + image + "\n    container_name: " + caddyContainer + "\n    restart: unless-stopped\n    network_mode: host\n" + env +
		"    volumes:\n      - " + caddyFile + ":/etc/caddy/Caddyfile:ro\n      - /var/www/ctrl32:/srv/ctrl32:ro\n      - ctrl32-telemetry-caddy-data:/data\n      - ctrl32-telemetry-caddy-config:/config\nvolumes:\n  ctrl32-telemetry-caddy-data:\n    name: ctrl32-telemetry-caddy-data\n  ctrl32-telemetry-caddy-config:\n    name: ctrl32-telemetry-caddy-config\n"
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
	tmp := caddyFile + ".new"
	if err := writeFileAtomic(tmp, []byte(caddyfileFor(cs)), 0o644); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if out, err := run(ctx, 2*time.Minute, "docker", "run", "--rm", "-e", "CF_API_TOKEN=validation", "-v", tmp+":/etc/caddy/Caddyfile:ro", image, "caddy", "validate", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"); err != nil {
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

// ---- WireGuard ------------------------------------------------------------------------------------

type wgStored struct {
	Enabled    bool      `json:"enabled"`
	ListenPort int       `json:"listen_port"`
	Address    string    `json:"address"`
	Endpoint   string    `json:"endpoint"`
	DNS        string    `json:"dns"`
	PublicKey  string    `json:"public_key"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type wgPeerStored struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	PublicKey    string    `json:"public_key"`
	PresharedKey string    `json:"preshared_key"`
	Address      string    `json:"address"`
	CreatedAt    time.Time `json:"created_at"`
	CreatedBy    string    `json:"created_by"`
}

func wgInstalled(ctx context.Context) bool {
	_, err := run(ctx, 5*time.Second, "wg", "--version")
	return err == nil
}

func (a *Server) wgLoad() (wgStored, []wgPeerStored) {
	var st wgStored
	_ = loadJSON(wgStateFile, &st)
	peers := []wgPeerStored{}
	_ = loadJSON(wgPeersFile, &peers)
	return st, peers
}

func (a *Server) wgState(ctx context.Context) WireGuardState {
	st, peers := a.wgLoad()
	out := WireGuardState{Installed: wgInstalled(ctx), Enabled: st.Enabled, ListenPort: st.ListenPort, Address: st.Address, Endpoint: st.Endpoint, DNS: st.DNS, PublicKey: st.PublicKey, UpdatedAt: st.UpdatedAt, Peers: []WireGuardPeer{}}
	if ip, _, err := net.ParseCIDR(st.Address); err == nil {
		out.MQTT = ip.String() + ":1883"
	}
	live := map[string][]string{}
	if out.Installed {
		if dump, err := run(ctx, 5*time.Second, "wg", "show", "wg0", "dump"); err == nil {
			out.Running = true
			for i, l := range strings.Split(dump, "\n") {
				f := strings.Split(l, "\t")
				if i == 0 || len(f) < 8 {
					continue
				}
				live[f[0]] = f
			}
		}
	}
	for _, p := range peers {
		wp := WireGuardPeer{ID: p.ID, Name: p.Name, PublicKey: p.PublicKey, Address: p.Address, CreatedAt: p.CreatedAt, CreatedBy: p.CreatedBy}
		if f, ok := live[p.PublicKey]; ok {
			if f[2] != "(none)" {
				wp.Endpoint = f[2]
			}
			if hs, _ := strconv.ParseInt(f[4], 10, 64); hs > 0 {
				wp.LatestHandshake = time.Unix(hs, 0)
				wp.Online = time.Since(wp.LatestHandshake) < 3*time.Minute
			}
			wp.RxBytes, _ = strconv.ParseInt(f[5], 10, 64)
			wp.TxBytes, _ = strconv.ParseInt(f[6], 10, 64)
		}
		out.Peers = append(out.Peers, wp)
	}
	return out
}

func (a *Server) hWireGuard(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, a.wgState(r.Context()))
}

func wgConfFor(st wgStored, privateKey string, peers []wgPeerStored) string {
	var b strings.Builder
	b.WriteString("# ctrl32 telemetry — generated by the server agent; edit in the UI under System → WireGuard\n[Interface]\n")
	b.WriteString("Address = " + st.Address + "\nListenPort = " + strconv.Itoa(st.ListenPort) + "\nPrivateKey = " + privateKey + "\n")
	for _, p := range peers {
		ip := p.Address
		if !strings.Contains(ip, "/") {
			ip += "/32"
		}
		b.WriteString("\n# " + strings.ReplaceAll(p.Name, "\n", " ") + "\n[Peer]\nPublicKey = " + p.PublicKey + "\nPresharedKey = " + p.PresharedKey + "\nAllowedIPs = " + ip + "\n")
	}
	return b.String()
}

func (a *Server) wgWriteConf(st wgStored, peers []wgPeerStored) error {
	key := strings.TrimSpace(readFile(wgKeyFile))
	if key == "" {
		return errors.New("server key missing")
	}
	_ = os.MkdirAll(filepath.Dir(wgConfFile), 0o700)
	return writeFileAtomic(wgConfFile, []byte(wgConfFor(st, key, peers)), 0o600)
}

// wgSync applies peer changes to the live interface without dropping tunnels.
func (a *Server) wgSync(ctx context.Context) error {
	stripped, err := run(ctx, 20*time.Second, "wg-quick", "strip", "wg0")
	if err != nil {
		return err
	}
	// Ubuntu confines wg with AppArmor: it may read files only under /etc/wireguard, never /tmp.
	syncFile := filepath.Dir(wgConfFile) + "/wg0.sync.conf"
	if err := writeFileAtomic(syncFile, []byte(stripped+"\n"), 0o600); err != nil {
		return err
	}
	defer os.Remove(syncFile)
	_, err = run(ctx, 20*time.Second, "wg", "syncconf", "wg0", syncFile)
	return err
}

func (a *Server) hSetWireGuard(w http.ResponseWriter, r *http.Request) {
	if isWindows() {
		writeErr(w, 501, "WireGuard is not managed on Windows; set http.tls_cert and http.tls_key in config.toml")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var in WireGuardRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request: "+err.Error())
		return
	}
	ctx := r.Context()
	if in.ListenPort == 0 {
		in.ListenPort = 51820
	}
	if in.ListenPort < 1 || in.ListenPort > 65535 {
		writeErr(w, 400, "invalid port")
		return
	}
	if in.Address == "" {
		in.Address = "10.66.0.1/24"
	}
	ip, ipnet, err := net.ParseCIDR(strings.TrimSpace(in.Address))
	if err != nil || ip.To4() == nil {
		writeErr(w, 400, "address must be an IPv4 address with prefix, e.g. 10.66.0.1/24")
		return
	}
	if ones, _ := ipnet.Mask.Size(); ones > 30 || ones < 8 {
		writeErr(w, 400, "prefix must be between /8 and /30")
		return
	}
	in.Endpoint = strings.TrimSpace(in.Endpoint)
	if in.Endpoint != "" {
		host, port, err := net.SplitHostPort(in.Endpoint)
		if err != nil || host == "" {
			writeErr(w, 400, "endpoint must be host:port")
			return
		}
		if _, err := strconv.Atoi(port); err != nil {
			writeErr(w, 400, "endpoint must be host:port")
			return
		}
	} else {
		cs := a.caddyState()
		if cs.Domain != "" {
			in.Endpoint = cs.Domain + ":" + strconv.Itoa(in.ListenPort)
		}
	}
	in.DNS = strings.TrimSpace(in.DNS)
	if in.DNS != "" && net.ParseIP(in.DNS) == nil {
		writeErr(w, 400, "DNS must be an IP address")
		return
	}
	if in.Enabled && !wgInstalled(ctx) {
		a.log.Info("installing wireguard-tools")
		if out, err := run(ctx, 4*time.Minute, "apt-get", "install", "-y", "-qq", "wireguard-tools"); err != nil {
			writeErr(w, 500, "wireguard-tools could not be installed: "+firstLine(out))
			return
		}
	}
	st, peers := a.wgLoad()
	if in.Enabled || st.PublicKey != "" {
		if strings.TrimSpace(readFile(wgKeyFile)) == "" {
			priv, err := run(ctx, 10*time.Second, "wg", "genkey")
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			_ = os.MkdirAll(wgDir, 0o700)
			if err := writeFileAtomic(wgKeyFile, []byte(priv+"\n"), 0o600); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		}
		pub, err := runInput(ctx, strings.TrimSpace(readFile(wgKeyFile))+"\n", "wg", "pubkey")
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		st.PublicKey = pub
	}
	st.Enabled, st.ListenPort, st.Address, st.Endpoint, st.DNS, st.UpdatedAt = in.Enabled, in.ListenPort, in.Address, in.Endpoint, in.DNS, time.Now()
	if err := saveJSON(wgStateFile, st, 0o600); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if st.PublicKey != "" {
		if err := a.wgWriteConf(st, peers); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	if in.Enabled {
		if out, err := run(ctx, time.Minute, "systemctl", "enable", "--now", wgUnit); err != nil {
			writeErr(w, 500, "WireGuard could not be started: "+firstLine(out))
			return
		}
		if out, err := run(ctx, time.Minute, "systemctl", "restart", wgUnit); err != nil {
			writeErr(w, 500, "WireGuard could not be restarted: "+firstLine(out))
			return
		}
		if out, err := run(ctx, 5*time.Second, "ufw", "status"); err == nil && strings.Contains(out, "Status: active") {
			_, _ = run(ctx, 10*time.Second, "ufw", "allow", strconv.Itoa(in.ListenPort)+"/udp", "comment", "ctrl32 WireGuard")
		}
	} else {
		_, _ = run(ctx, time.Minute, "systemctl", "disable", "--now", wgUnit)
	}
	// the broker listens inside the VPN so devices reach MQTT without TLS/public exposure
	vpnListen := ""
	if in.Enabled {
		vpnListen = ip.String() + ":1883"
	}
	if changed, err := a.setConfigValue("mqtt", "vpn_listen", vpnListen); err != nil {
		writeErr(w, 500, "config update failed: "+err.Error())
		return
	} else if changed {
		a.restartAppLater()
	}
	a.log.Info("wireguard configured", "enabled", in.Enabled, "port", in.ListenPort, "address", in.Address, "endpoint", in.Endpoint)
	writeJSON(w, 200, a.wgState(ctx))
}

func (a *Server) hAddPeer(w http.ResponseWriter, r *http.Request) {
	if isWindows() {
		writeErr(w, 501, "WireGuard is not managed on Windows; set http.tls_cert and http.tls_key in config.toml")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var in PeerRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !nameRe.MatchString(in.Name) {
		writeErr(w, 400, "name: letters, digits, space, dot, dash or underscore (max 63)")
		return
	}
	ctx := r.Context()
	st, peers := a.wgLoad()
	if st.PublicKey == "" || !wgInstalled(ctx) {
		writeErr(w, 400, "configure and enable WireGuard first")
		return
	}
	for _, p := range peers {
		if strings.EqualFold(p.Name, in.Name) {
			writeErr(w, 400, "a peer with that name exists")
			return
		}
	}
	serverIP, ipnet, err := net.ParseCIDR(st.Address)
	if err != nil {
		writeErr(w, 500, "server address invalid")
		return
	}
	used := map[string]bool{serverIP.String(): true}
	for _, p := range peers {
		used[strings.TrimSuffix(p.Address, "/32")] = true
	}
	var addr string
	first := ipnet.IP.To4()
	ones, bits := ipnet.Mask.Size()
	total := 1 << (bits - ones)
	for i := 2; i < total-1; i++ {
		c := net.IPv4(first[0], first[1], first[2], first[3]).To4()
		n := (int(c[0])<<24 | int(c[1])<<16 | int(c[2])<<8 | int(c[3])) + i
		cand := net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n)).String()
		if !used[cand] {
			addr = cand
			break
		}
	}
	if addr == "" {
		writeErr(w, 400, "no free address in "+st.Address)
		return
	}
	priv, err := run(ctx, 10*time.Second, "wg", "genkey")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	pub, err := runInput(ctx, priv+"\n", "wg", "pubkey")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	psk, err := run(ctx, 10*time.Second, "wg", "genpsk")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	p := wgPeerStored{ID: randomID(), Name: in.Name, PublicKey: pub, PresharedKey: psk, Address: addr + "/32", CreatedAt: time.Now(), CreatedBy: in.CreatedBy}
	peers = append(peers, p)
	if err := saveJSON(wgPeersFile, peers, 0o600); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := a.wgWriteConf(st, peers); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if st.Enabled {
		if err := a.wgSync(ctx); err != nil {
			a.log.Warn("wg syncconf failed", "err", err)
		}
	}
	var cfg strings.Builder
	cfg.WriteString("[Interface]\nPrivateKey = " + priv + "\nAddress = " + addr + "/32\n")
	if st.DNS != "" {
		cfg.WriteString("DNS = " + st.DNS + "\n")
	}
	cfg.WriteString("\n[Peer]\nPublicKey = " + st.PublicKey + "\nPresharedKey = " + psk + "\nAllowedIPs = " + ipnet.String() + "\nPersistentKeepalive = 25\n")
	if st.Endpoint != "" {
		cfg.WriteString("Endpoint = " + st.Endpoint + "\n")
	}
	a.log.Info("wireguard peer added", "name", in.Name, "address", addr)
	writeJSON(w, 200, PeerResult{Peer: WireGuardPeer{ID: p.ID, Name: p.Name, PublicKey: p.PublicKey, Address: p.Address, CreatedAt: p.CreatedAt, CreatedBy: p.CreatedBy}, Config: cfg.String()})
}

func (a *Server) hRemovePeer(w http.ResponseWriter, r *http.Request) {
	if isWindows() {
		writeErr(w, 501, "WireGuard is not managed on Windows; set http.tls_cert and http.tls_key in config.toml")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	id := r.PathValue("id")
	st, peers := a.wgLoad()
	kept := peers[:0]
	removed := ""
	for _, p := range peers {
		if p.ID == id {
			removed = p.Name
			continue
		}
		kept = append(kept, p)
	}
	if removed == "" {
		writeErr(w, 404, "peer not found")
		return
	}
	if err := saveJSON(wgPeersFile, kept, 0o600); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if st.PublicKey != "" {
		if err := a.wgWriteConf(st, kept); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if st.Enabled {
			if err := a.wgSync(r.Context()); err != nil {
				a.log.Warn("wg syncconf failed", "err", err)
			}
		}
	}
	a.log.Info("wireguard peer removed", "name", removed)
	writeJSON(w, 200, Result{OK: true, Message: "peer removed"})
}

// ---- update ---------------------------------------------------------------------------------------

func (a *Server) hUpdate(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var in UpdateRequest
	if err := decode(r, &in); err != nil || !versionRe.MatchString(in.Version) {
		writeErr(w, 400, "invalid version")
		return
	}
	ctx := r.Context()
	name := "ctrl32-telemetry-" + runtime.GOOS + "-" + runtime.GOARCH
	if isWindows() {
		name += ".exe"
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	fetch := func(u string) ([]byte, error) {
		req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	}
	sums, err := fetch(a.updateURL + "/SHA256SUMS")
	if err != nil {
		writeErr(w, 502, "checksums unavailable: "+err.Error())
		return
	}
	want := ""
	for _, l := range strings.Split(string(sums), "\n") {
		f := strings.Fields(l)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			want = f[0]
		}
	}
	if want == "" {
		writeErr(w, 502, "no checksum published for "+name)
		return
	}
	bin, err := fetch(a.updateURL + "/" + name)
	if err != nil {
		writeErr(w, 502, "download failed: "+err.Error())
		return
	}
	sum := sha256.Sum256(bin)
	got := hex.EncodeToString(sum[:])
	if got != want {
		writeErr(w, 502, "checksum mismatch — update refused")
		return
	}
	tmp := binPath + ".new"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if out, err := run(ctx, 20*time.Second, tmp, "version"); err != nil || !bytes.Contains([]byte(out), []byte(in.Version)) {
		_ = os.Remove(tmp)
		writeErr(w, 502, "downloaded binary does not report version "+in.Version)
		return
	}
	if isWindows() {
		// a running executable can be renamed but not overwritten: park the old one, then move the new one in
		old := binPath + ".old"
		_ = os.Remove(old)
		if err := os.Rename(binPath, old); err != nil {
			writeErr(w, 500, "cannot park the current binary: "+err.Error())
			return
		}
		if err := os.Rename(tmp, binPath); err != nil {
			_ = os.Rename(old, binPath)
			writeErr(w, 500, err.Error())
			return
		}
	} else if err := os.Rename(tmp, binPath); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if out, err := run(ctx, 5*time.Minute, binPath, "migrate", "-config", a.cfgPath); err != nil {
		writeErr(w, 500, "migration failed: "+firstLine(out))
		return
	}
	a.log.Info("updated binary", "version", in.Version, "sha256", got)
	a.restartAppLater()
	if isWindows() {
		restartServiceLater(agentUnit, 4*time.Second)
	} else {
		go func() {
			time.Sleep(4 * time.Second)
			_, _ = run(context.Background(), 30*time.Second, "systemctl", "restart", agentUnit)
		}()
	}
	writeJSON(w, 200, UpdateResult{Version: in.Version, SHA256: got, Message: "installed; service and agent are restarting"})
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

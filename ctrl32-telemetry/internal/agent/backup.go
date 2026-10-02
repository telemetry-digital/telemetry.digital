package agent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
)

var (
	backupRunning atomic.Bool
	backupMu      sync.Mutex // one backup at a time (manual or scheduled)
)

// Backups (URS K-14): a bundle of the database dump (pg_dump custom format), the configuration and the
// agent-managed files (Caddy state, WireGuard keys and peers). The bundle contains secrets, so it can be
// encrypted with a passphrase (openssl, AES-256-CBC, PBKDF2). Files are readable by the ctrl32 group so the
// web service can stream downloads; scheduled backups run inside the agent once a day with a retention count.

const (
	dbName  = "ctrl32"
	dbAdmin = "ctrl32"
)

// adminURL is the administrative database URL from the configuration (Windows: pg_dump connects with it).
func (a *Server) adminURL() string {
	cfg, err := config.Load(a.cfgPath)
	if err != nil {
		return ""
	}
	return cfg.DB.AdminURL
}

var backupNameRe = regexp.MustCompile(`^ctrl32-\d{8}-\d{6}\.tar\.gz(\.enc)?$`)

type backupStored struct {
	Enabled    bool      `json:"enabled"`
	Hour       int       `json:"hour"`
	Minute     int       `json:"minute"`
	Keep       int       `json:"keep"`
	Passphrase string    `json:"passphrase,omitempty"`
	LastRun    time.Time `json:"last_run,omitempty"`
	LastError  string    `json:"last_error,omitempty"`
	LastFile   string    `json:"last_file,omitempty"`
}

func (a *Server) backupLoad() backupStored {
	st := backupStored{Hour: 2, Minute: 30, Keep: 14}
	_ = loadJSON(backupStateFile, &st)
	if st.Keep <= 0 {
		st.Keep = 14
	}
	return st
}

func (a *Server) backupSave(st backupStored) error { return saveJSON(backupStateFile, st, 0o600) }

func (a *Server) dbMode(ctx context.Context) string {
	if out, err := run(ctx, 10*time.Second, "docker", "inspect", "-f", "{{.State.Running}}", a.pgContainer); err == nil && strings.TrimSpace(out) == "true" {
		return "docker"
	}
	if isWindows() {
		if findPgDump(a.pgDump) != "" {
			return "native"
		}
		return "unknown"
	}
	if _, err := exec.LookPath("pg_dump"); err == nil {
		return "native"
	}
	return "unknown"
}

func (a *Server) backupState(ctx context.Context) BackupsState {
	st := a.backupLoad()
	out := BackupsState{Dir: BackupDir, DBMode: a.dbMode(ctx), Running: backupRunning.Load(), Files: []BackupInfo{},
		Schedule: BackupSchedule{Enabled: st.Enabled, Hour: st.Hour, Minute: st.Minute, Keep: st.Keep, HasPassphrase: st.Passphrase != "", LastRun: st.LastRun, LastError: st.LastError, LastFile: st.LastFile}}
	entries, _ := os.ReadDir(BackupDir)
	for _, e := range entries {
		if e.IsDir() || !backupNameRe.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out.Files = append(out.Files, BackupInfo{Name: e.Name(), SizeBytes: info.Size(), CreatedAt: info.ModTime(), Encrypted: strings.HasSuffix(e.Name(), ".enc")})
	}
	sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].CreatedAt.After(out.Files[j].CreatedAt) })
	return out
}

func (a *Server) hBackups(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, a.backupState(r.Context()))
}

// createBackup writes ctrl32-<timestamp>.tar.gz[.enc] and returns its name.
func (a *Server) createBackup(ctx context.Context, passphrase string) (BackupInfo, error) {
	gid := 0
	if !isWindows() {
		if g, err := user.LookupGroup(appGroup); err == nil {
			gid, _ = strconv.Atoi(g.Gid)
		}
	}
	if err := os.MkdirAll(BackupDir, 0o750); err != nil {
		return BackupInfo{}, err
	}
	if !isWindows() {
		_ = os.Chown(BackupDir, 0, gid)
		_ = os.Chmod(BackupDir, 0o750)
	}
	ts := time.Now().Format("20060102-150405")
	work, err := os.MkdirTemp(BackupDir, ".work-")
	if err != nil {
		return BackupInfo{}, err
	}
	defer os.RemoveAll(work)

	// 1. database dump
	dump := filepath.Join(work, "db.dump")
	f, err := os.OpenFile(dump, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return BackupInfo{}, err
	}
	dctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	var cmd *exec.Cmd
	switch a.dbMode(ctx) {
	case "docker":
		cmd = exec.CommandContext(dctx, "docker", "exec", a.pgContainer, "pg_dump", "-U", dbAdmin, "-Fc", "--no-owner", dbName)
	case "native":
		if isWindows() {
			url := a.adminURL()
			if url == "" {
				f.Close()
				return BackupInfo{}, errors.New("db.admin_url missing in the configuration")
			}
			cmd = exec.CommandContext(dctx, findPgDump(a.pgDump), "-Fc", "--no-owner", "--dbname", url)
		} else {
			cmd = exec.CommandContext(dctx, "runuser", "-u", "postgres", "--", "pg_dump", "-Fc", "--no-owner", dbName)
		}
	default:
		f.Close()
		return BackupInfo{}, errors.New("no PostgreSQL found (neither the Docker container nor pg_dump)")
	}
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = f, &stderr
	err = cmd.Run()
	f.Close()
	if err != nil {
		return BackupInfo{}, fmt.Errorf("pg_dump failed: %s", firstLine(strings.TrimSpace(stderr.String())))
	}
	if st, err := os.Stat(dump); err != nil || st.Size() < 100 {
		return BackupInfo{}, errors.New("pg_dump produced an empty file")
	}

	// 2. bundle: dump + configuration + agent-managed state
	name := "ctrl32-" + ts + ".tar.gz"
	bundle := filepath.Join(work, name)
	if err := writeBundle(bundle, map[string]string{
		"db.dump": dump, "config.toml": a.cfgPath, "caddy.json": caddyStateFile, "caddy.env": caddyEnvFile, "Caddyfile": caddyFile,
		"wireguard/state.json": wgStateFile, "wireguard/peers.json": wgPeersFile, "wireguard/server.key": wgKeyFile, "wireguard/vpn.json": wgVPNFile, "backup.json": backupStateFile,
	}, "RESTORE.txt", restoreNotes(ts)); err != nil {
		return BackupInfo{}, err
	}
	final := filepath.Join(BackupDir, name)
	if passphrase != "" {
		if _, err := exec.LookPath("openssl"); err != nil {
			return BackupInfo{}, errors.New("encrypted backups need openssl on this host; install OpenSSL or leave the passphrase empty")
		}
		enc := bundle + ".enc"
		c := exec.CommandContext(dctx, "openssl", "enc", "-aes-256-cbc", "-pbkdf2", "-iter", "200000", "-salt", "-in", bundle, "-out", enc, "-pass", "env:CTRL32_BACKUP_PASS")
		c.Env = append(os.Environ(), "CTRL32_BACKUP_PASS="+passphrase)
		if out, err := c.CombinedOutput(); err != nil {
			return BackupInfo{}, fmt.Errorf("encryption failed: %s", firstLine(strings.TrimSpace(string(out))))
		}
		_ = os.Remove(bundle)
		bundle, name = enc, name+".enc"
		final = filepath.Join(BackupDir, name)
	}
	if err := os.Rename(bundle, final); err != nil {
		return BackupInfo{}, err
	}
	if !isWindows() {
		_ = os.Chown(final, 0, gid)
		_ = os.Chmod(final, 0o640)
	}
	st, _ := os.Stat(final)
	a.log.Info("backup created", "file", name, "bytes", st.Size(), "encrypted", passphrase != "")
	return BackupInfo{Name: name, SizeBytes: st.Size(), CreatedAt: st.ModTime(), Encrypted: passphrase != ""}, nil
}

func writeBundle(path string, files map[string]string, noteName, note string) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		src := files[n]
		st, err := os.Stat(src)
		if err != nil || !st.Mode().IsRegular() {
			continue // optional parts (no WireGuard yet, no Caddy token…)
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{Name: n, Mode: 0o600, Size: int64(len(data)), ModTime: st.ModTime()}); err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
	}
	if err := tw.WriteHeader(&tar.Header{Name: noteName, Mode: 0o644, Size: int64(len(note)), ModTime: time.Now()}); err != nil {
		return err
	}
	if _, err := io.WriteString(tw, note); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func restoreNotes(ts string) string {
	return "ctrl32 telemetry backup " + ts + "\n\n" +
		"Contents: db.dump (pg_dump custom format), config.toml (contains the password pepper and secret key — the\n" +
		"database is useless without them), Caddyfile/caddy.json/caddy.env, wireguard/ (server key, peers).\n\n" +
		"Restore on a fresh install (same version or newer):\n" +
		"  1. curl -fsSL https://portal.telemetry.digital/install.sh | sudo bash -s -- --domain <domain> --no-start\n" +
		"  2. copy config.toml to /etc/ctrl32-telemetry/config.toml (root:ctrl32 0640); optionally caddy.*, wireguard/ to their places\n" +
		"  3. docker exec -i ctrl32-telemetry-postgres pg_restore -U ctrl32 -d ctrl32 --clean --if-exists --no-owner < db.dump\n" +
		"     (native PostgreSQL: runuser -u postgres -- pg_restore -d ctrl32 --clean --if-exists --no-owner db.dump)\n" +
		"  4. ctrl32-telemetry migrate -config /etc/ctrl32-telemetry/config.toml && systemctl start ctrl32-telemetry ctrl32-telemetry-agent\n\n" +
		"Encrypted bundles (.enc): openssl enc -d -aes-256-cbc -pbkdf2 -iter 200000 -in <file>.enc -out <file>\n\n" +
		"Windows (install.ps1): copy config.toml to %ProgramData%\\ctrl32-telemetry\\config.toml, then\n" +
		"  \"C:\\Program Files\\ctrl32-telemetry\\pgsql\\bin\\pg_restore.exe\" --dbname <db.admin_url from config.toml> --clean --if-exists --no-owner db.dump\n" +
		"  ctrl32-telemetry.exe migrate -config %ProgramData%\\ctrl32-telemetry\\config.toml; Start-Service ctrl32-telemetry, ctrl32-telemetry-agent\n"
}

func (a *Server) hCreateBackup(w http.ResponseWriter, r *http.Request) {
	var in BackupRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request")
		return
	}
	if strings.ContainsAny(in.Passphrase, "\n\r") {
		writeErr(w, 400, "invalid passphrase")
		return
	}
	if !backupRunning.CompareAndSwap(false, true) {
		writeErr(w, 409, "a backup is already running")
		return
	}
	// Large databases take minutes: run in the background, the UI polls the state.
	go func(pass string) {
		defer backupRunning.Store(false)
		backupMu.Lock()
		defer backupMu.Unlock()
		info, err := a.createBackup(context.Background(), pass)
		st := a.backupLoad()
		st.LastRun = time.Now()
		if err != nil {
			st.LastError = err.Error()
			a.log.Error("backup failed", "err", err)
		} else {
			st.LastError, st.LastFile = "", info.Name
		}
		if err := a.backupSave(st); err != nil {
			a.log.Error("backup state save failed", "err", err)
		}
	}(in.Passphrase)
	writeJSON(w, 202, a.backupState(r.Context()))
}

func (a *Server) hDeleteBackup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !backupNameRe.MatchString(name) {
		writeErr(w, 400, "invalid backup name")
		return
	}
	if err := os.Remove(filepath.Join(BackupDir, name)); err != nil {
		writeErr(w, 404, "backup not found")
		return
	}
	a.log.Info("backup deleted", "file", name)
	writeJSON(w, 200, Result{OK: true, Message: "backup deleted"})
}

func (a *Server) hBackupSchedule(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var in ScheduleRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request: "+err.Error())
		return
	}
	if in.Hour < 0 || in.Hour > 23 || in.Minute < 0 || in.Minute > 59 {
		writeErr(w, 400, "time must be HH:MM")
		return
	}
	if in.Keep < 1 || in.Keep > 365 {
		writeErr(w, 400, "keep must be between 1 and 365")
		return
	}
	if strings.ContainsAny(in.Passphrase, "\n\r") {
		writeErr(w, 400, "invalid passphrase")
		return
	}
	st := a.backupLoad()
	st.Enabled, st.Hour, st.Minute, st.Keep = in.Enabled, in.Hour, in.Minute, in.Keep
	if in.ClearPassphrase {
		st.Passphrase = ""
	} else if in.Passphrase != "" {
		st.Passphrase = in.Passphrase
	}
	if err := a.backupSave(st); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.log.Info("backup schedule saved", "enabled", st.Enabled, "at", fmt.Sprintf("%02d:%02d", st.Hour, st.Minute), "keep", st.Keep, "encrypted", st.Passphrase != "")
	writeJSON(w, 200, a.backupState(r.Context()))
}

// runScheduler performs the daily backup and prunes old files; it runs in the agent process.
func (a *Server) runScheduler(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			st := a.backupLoad()
			if !st.Enabled || now.Hour() != st.Hour || now.Minute() != st.Minute || (st.LastRun.YearDay() == now.YearDay() && st.LastRun.Year() == now.Year()) {
				continue
			}
			if !backupRunning.CompareAndSwap(false, true) {
				continue
			}
			backupMu.Lock()
			info, err := a.createBackup(ctx, st.Passphrase)
			st = a.backupLoad()
			st.LastRun = now
			if err != nil {
				st.LastError = err.Error()
				a.log.Error("scheduled backup failed", "err", err)
			} else {
				st.LastError, st.LastFile = "", info.Name
				a.prune(st.Keep)
			}
			_ = a.backupSave(st)
			backupMu.Unlock()
			backupRunning.Store(false)
		}
	}
}

func (a *Server) prune(keep int) {
	files := a.backupState(context.Background()).Files // newest first
	for i, f := range files {
		if i >= keep {
			if err := os.Remove(filepath.Join(BackupDir, f.Name)); err == nil {
				a.log.Info("backup pruned", "file", f.Name)
			}
		}
	}
}

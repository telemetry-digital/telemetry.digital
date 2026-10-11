package agent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	"github.com/telemetry-digital/ctrl32-telemetry/internal/offsite"
)

var (
	backupRunning  atomic.Bool
	restoreRunning atomic.Bool
	backupMu       sync.Mutex // one backup, synchronisation or restore test at a time (manual or scheduled)
	backupStateMu  sync.Mutex // read-modify-write of backup.json
)

// Backups (URS K-01..K-04, K-14): a bundle of the database dump (pg_dump custom format), the configuration and the
// agent-managed files (Caddy state, WireGuard keys and peers) with a manifest (application version, migrations, row
// counts and the SHA-256 of every member). The bundle contains secrets, so it can be encrypted with a passphrase (the
// openssl enc format, backup_crypto.go); a checksum file <name>.sha256 lies next to it. Files are readable by the
// ctrl32 group so the web service can stream downloads. Scheduled backups run inside the agent once a day, are kept by
// a grandfather–father–son rule (backup_gfs.go), copied encrypted to an offsite target and read back (backup_offsite.go),
// and a restore test restores the newest one into a temporary database every month (backup_restoretest.go). Every run
// is recorded with its steps (BackupRun); the web service audits the results and opens an incident on a failure.

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
	Enabled     bool      `json:"enabled"`
	Hour        int       `json:"hour"`
	Minute      int       `json:"minute"`
	Keep        int       `json:"keep"`
	KeepWeekly  int       `json:"keep_weekly"`
	KeepMonthly int       `json:"keep_monthly"`
	Passphrase  string    `json:"passphrase,omitempty"`
	LastRun     time.Time `json:"last_run,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
	LastFile    string    `json:"last_file,omitempty"`
	LastOK      time.Time `json:"last_ok,omitempty"`
	// offsite target (secrets included: this file is root-only)
	Offsite offsite.Config `json:"offsite"`
	// Uploaded: names this server put at the target; only these are ever deleted there (a shared folder is safe)
	Uploaded         []string            `json:"uploaded,omitempty"`
	OffsiteFiles     []OffsiteObject     `json:"offsite_files,omitempty"`
	OffsiteListedAt  time.Time           `json:"offsite_listed_at,omitempty"`
	OffsiteListError string              `json:"offsite_list_error,omitempty"`
	RestoreTest      RestoreTestSchedule `json:"restore_test"`
}

func (a *Server) backupLoad() backupStored {
	backupStateMu.Lock()
	defer backupStateMu.Unlock()
	return backupLoadLocked()
}

func backupLoadLocked() backupStored {
	// defaults; a file of an older version keeps them for the fields it does not have
	st := backupStored{Hour: 2, Minute: 30, Keep: 14, KeepWeekly: 4, KeepMonthly: 12,
		RestoreTest: RestoreTestSchedule{Enabled: true, Day: 1, Hour: 4, Minute: 30}}
	_ = loadJSON(backupStateFile, &st)
	if st.Keep <= 0 {
		st.Keep = 14
	}
	if st.RestoreTest.Day < 1 || st.RestoreTest.Day > 28 {
		st.RestoreTest.Day = 1
	}
	return st
}

// backupUpdate changes backup.json under the lock.
func (a *Server) backupUpdate(fn func(*backupStored)) error {
	backupStateMu.Lock()
	defer backupStateMu.Unlock()
	st := backupLoadLocked()
	fn(&st)
	return saveJSON(backupStateFile, st, 0o600)
}

func (a *Server) dbMode(ctx context.Context) string {
	if a.simulate {
		return "simulated"
	}
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
	off := OffsiteState{Config: map[string]any{"enabled": false}, Files: st.OffsiteFiles, ListedAt: st.OffsiteListedAt, ListError: st.OffsiteListError}
	if st.Offsite.Kind != "" {
		off.Config, off.Describe = st.Offsite.Redacted(), st.Offsite.Describe()
	}
	if off.Files == nil {
		off.Files = []OffsiteObject{}
	}
	remote := map[string]bool{}
	for _, f := range st.OffsiteFiles {
		remote[f.Name] = true
	}
	out := BackupsState{Dir: BackupDir, DBMode: a.dbMode(ctx), Running: backupRunning.Load(), RestoreRunning: restoreRunning.Load(), Files: a.localBackups(remote),
		Schedule: BackupSchedule{Enabled: st.Enabled, Hour: st.Hour, Minute: st.Minute, Keep: st.Keep, KeepWeekly: st.KeepWeekly, KeepMonthly: st.KeepMonthly,
			HasPassphrase: st.Passphrase != "", LastRun: st.LastRun, LastError: st.LastError, LastFile: st.LastFile, LastOK: st.LastOK},
		Offsite: off, RestoreTest: st.RestoreTest, History: loadBackupHistory(30)}
	return out
}

// localBackups lists the bundles in BackupDir, newest first.
func (a *Server) localBackups(remote map[string]bool) []BackupInfo {
	files := []BackupInfo{}
	entries, _ := os.ReadDir(BackupDir)
	for _, e := range entries {
		if e.IsDir() || !backupNameRe.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		created := info.ModTime()
		if t, ok := backupTime(e.Name()); ok {
			created = t
		}
		files = append(files, BackupInfo{Name: e.Name(), SizeBytes: info.Size(), CreatedAt: created, Encrypted: strings.HasSuffix(e.Name(), ".enc"),
			SHA256: readChecksum(filepath.Join(BackupDir, e.Name()+".sha256")), Offsite: remote[e.Name()]})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name > files[j].Name })
	return files
}

func (a *Server) hBackups(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, a.backupState(r.Context()))
}

// ---- the bundle --------------------------------------------------------------------------------------------------

// backupManifest is manifest.json inside every bundle since 0.77.0 (URS K-04): what the bundle holds and what the
// database looked like, so a restore test can tell a complete restore from a partial one.
type backupManifest struct {
	Format     int                     `json:"format"`
	AppVersion string                  `json:"app_version"`
	CreatedAt  time.Time               `json:"created_at"`
	Host       string                  `json:"host"`
	DB         map[string]string       `json:"db"` // migrations, last_migration, tables, organizations, users, audit_entries
	Files      map[string]manifestFile `json:"files"`
}

type manifestFile struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// dbCheckQuery returns migrations|last migration|tables|organizations|users|audit entries.
const dbCheckQuery = `SELECT (SELECT count(*) FROM public.schema_migration) || '|' || (SELECT coalesce(max(name), '') FROM public.schema_migration) || '|' || ` +
	`(SELECT count(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog', 'information_schema')) || '|' || ` +
	`(SELECT count(*) FROM core.organization) || '|' || (SELECT count(*) FROM core.user_account) || '|' || (SELECT count(*) FROM core.audit_trail)`

// dbSchemaQuery tells whether the tables of the figures exist.
const dbSchemaQuery = `SELECT to_regclass('public.schema_migration') IS NOT NULL AND to_regclass('core.organization') IS NOT NULL AND ` +
	`to_regclass('core.user_account') IS NOT NULL AND to_regclass('core.audit_trail') IS NOT NULL`

var dbCheckKeys = []string{"migrations", "last_migration", "tables", "organizations", "users", "audit_entries"}

// dbChecks reads the figures of the manifest from a database.
func (a *Server) dbChecks(ctx context.Context, db string) (map[string]string, error) {
	if a.simulate {
		return map[string]string{"migrations": "54", "last_migration": "0054_server_settings.sql", "tables": "180", "organizations": "1", "users": "1", "audit_entries": "10"}, nil
	}
	// a database without the schema (a fresh or a foreign one) has nothing to count: its backup is still valid
	have, err := a.psql(ctx, db, dbSchemaQuery)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(have) != "t" {
		tables, err := a.psql(ctx, db, `SELECT count(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog', 'information_schema')`)
		if err != nil {
			return nil, err
		}
		return map[string]string{"migrations": "0", "last_migration": "", "tables": strings.TrimSpace(tables), "organizations": "0", "users": "0", "audit_entries": "0"}, nil
	}
	out, err := a.psql(ctx, db, dbCheckQuery)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(strings.TrimSpace(out), "|")
	if len(parts) != len(dbCheckKeys) {
		return nil, fmt.Errorf("unexpected answer of the database: %q", firstLine(out))
	}
	m := map[string]string{}
	for i, k := range dbCheckKeys {
		m[k] = parts[i]
	}
	return m, nil
}

// liveDB is the database of the installation.
func (a *Server) liveDB() (string, error) {
	if !isWindows() || a.simulate {
		return dbName, nil
	}
	u, err := url.Parse(a.adminURL())
	if err != nil || a.adminURL() == "" {
		return "", errors.New("db.admin_url missing in the configuration")
	}
	return strings.TrimPrefix(u.Path, "/"), nil
}

// psql runs one statement in a database and returns the unaligned output.
func (a *Server) psql(ctx context.Context, db, statement string) (string, error) {
	var cmd *exec.Cmd
	switch mode := a.dbMode(ctx); {
	case mode == "docker":
		cmd = exec.CommandContext(ctx, "docker", "exec", "-i", a.pgContainer, "psql", "-U", dbAdmin, "-d", db, "-tA", "-v", "ON_ERROR_STOP=1", "-c", statement)
	case mode == "native" && isWindows():
		cmd = exec.CommandContext(ctx, strings.Replace(findPgDump(a.pgDump), "pg_dump.exe", "psql.exe", 1), "--dbname", withDB(a.adminURL(), db), "-tA", "-v", "ON_ERROR_STOP=1", "-c", statement)
	case mode == "native":
		cmd = exec.CommandContext(ctx, "runuser", "-u", "postgres", "--", "psql", "-d", db, "-tA", "-v", "ON_ERROR_STOP=1", "-c", statement)
	default:
		return "", errors.New("no PostgreSQL found (neither the Docker container nor psql)")
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %s", firstLine(statement), firstLine(strings.TrimSpace(stderr.String())))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// createBackup writes ctrl32-<timestamp>.tar.gz[.enc] with its checksum file and returns it. rec (may be nil)
// records the steps.
func (a *Server) createBackup(ctx context.Context, passphrase string, rec *runRec) (BackupInfo, error) {
	gid := 0
	if !isWindows() {
		if g, err := user.LookupGroup(appGroup); err == nil {
			gid, _ = strconv.Atoi(g.Gid)
		}
	}
	if err := os.MkdirAll(BackupDir, 0o750); err != nil {
		return BackupInfo{}, err
	}
	if !isWindows() && !a.simulate {
		_ = os.Chown(BackupDir, 0, gid)
		_ = os.Chmod(BackupDir, 0o750)
	}
	ts := time.Now().Format("20060102-150405")
	work, err := os.MkdirTemp(BackupDir, ".work-")
	if err != nil {
		return BackupInfo{}, err
	}
	defer os.RemoveAll(work)
	dctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	// 1. database dump, and the figures of the manifest just before it
	dump := filepath.Join(work, "db.dump")
	man := &backupManifest{Format: 1, AppVersion: a.version, CreatedAt: time.Now().UTC(), Files: map[string]manifestFile{}}
	man.Host, _ = os.Hostname()
	if err := rec.step("dump", func() (string, error) {
		db, err := a.liveDB()
		if err != nil {
			return "", err
		}
		if man.DB, err = a.dbChecks(dctx, db); err != nil {
			return "", fmt.Errorf("reading the database figures failed: %w", err)
		}
		if err := a.pgDump2(dctx, dump); err != nil {
			return "", err
		}
		st, err := os.Stat(dump)
		if err != nil || st.Size() < 100 {
			return "", errors.New("pg_dump produced an empty file")
		}
		return fmt.Sprintf("%s, %s migrations, %s tables", humanBytes(st.Size()), man.DB["migrations"], man.DB["tables"]), nil
	}); err != nil {
		return BackupInfo{}, err
	}

	// 2. bundle: dump + configuration + agent-managed state + manifest
	name := "ctrl32-" + ts + ".tar.gz"
	bundle := filepath.Join(work, name)
	if err := rec.step("bundle", func() (string, error) {
		if err := writeBundle(bundle, map[string]string{
			"db.dump": dump, "config.toml": a.cfgPath, "caddy.json": caddyStateFile, "caddy.env": caddyEnvFile, "Caddyfile": caddyFile,
			"wireguard/state.json": wgStateFile, "wireguard/peers.json": wgPeersFile, "wireguard/server.key": wgKeyFile, "wireguard/vpn.json": wgVPNFile, "backup.json": backupStateFile,
		}, "RESTORE.txt", restoreNotes(ts), man); err != nil {
			return "", err
		}
		st, _ := os.Stat(bundle)
		return fmt.Sprintf("%d files, %s", len(man.Files), humanBytes(st.Size())), nil
	}); err != nil {
		return BackupInfo{}, err
	}

	// 3. encryption
	if passphrase != "" {
		enc := bundle + ".enc"
		if err := rec.step("encrypt", func() (string, error) {
			if err := encryptFile(bundle, enc, passphrase); err != nil {
				return "", fmt.Errorf("encryption failed: %w", err)
			}
			return "AES-256-CBC, PBKDF2-SHA256 200000 iterations", nil
		}); err != nil {
			return BackupInfo{}, err
		}
		_ = os.Remove(bundle)
		bundle, name = enc, name+".enc"
	}

	// 4. checksum file, then both files into place (the checksum first: a listed bundle always has one)
	final := filepath.Join(BackupDir, name)
	var sum string
	if err := rec.step("checksum", func() (string, error) {
		var err error
		if sum, err = fileSHA256(bundle); err != nil {
			return "", err
		}
		if err := os.WriteFile(final+".sha256", []byte(sum+"  "+name+"\n"), 0o640); err != nil {
			return "", err
		}
		if err := os.Rename(bundle, final); err != nil {
			os.Remove(final + ".sha256")
			return "", err
		}
		return "SHA-256 " + sum[:16] + "…", nil
	}); err != nil {
		return BackupInfo{}, err
	}
	if !isWindows() && !a.simulate {
		for _, f := range []string{final, final + ".sha256"} {
			_ = os.Chown(f, 0, gid)
			_ = os.Chmod(f, 0o640)
		}
	}
	st, _ := os.Stat(final)
	a.log.Info("backup created", "file", name, "bytes", st.Size(), "encrypted", passphrase != "")
	t, _ := backupTime(name)
	return BackupInfo{Name: name, SizeBytes: st.Size(), CreatedAt: t, Encrypted: passphrase != "", SHA256: sum}, nil
}

// pgDump2 writes the custom-format dump of the installation's database (the simulation writes a stand-in).
func (a *Server) pgDump2(ctx context.Context, dump string) error {
	if a.simulate {
		return os.WriteFile(dump, bytes.Repeat([]byte("simulated database dump of the agent simulation\n"), 64), 0o600)
	}
	f, err := os.OpenFile(dump, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	db, err := a.liveDB()
	if err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch a.dbMode(ctx) {
	case "docker":
		cmd = exec.CommandContext(ctx, "docker", "exec", a.pgContainer, "pg_dump", "-U", dbAdmin, "-Fc", "--no-owner", db)
	case "native":
		if isWindows() {
			u := a.adminURL()
			if u == "" {
				return errors.New("db.admin_url missing in the configuration")
			}
			cmd = exec.CommandContext(ctx, findPgDump(a.pgDump), "-Fc", "--no-owner", "--dbname", u)
		} else {
			cmd = exec.CommandContext(ctx, "runuser", "-u", "postgres", "--", "pg_dump", "-Fc", "--no-owner", dbName)
		}
	default:
		return errors.New("no PostgreSQL found (neither the Docker container nor pg_dump)")
	}
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = f, &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_dump failed: %s", firstLine(strings.TrimSpace(stderr.String())))
	}
	return nil
}

// writeBundle streams the files into a .tar.gz (missing optional files are skipped), then the note and the manifest
// with the size and SHA-256 of every member.
func writeBundle(path string, files map[string]string, noteName, note string, man *backupManifest) error {
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
	add := func(n string, mode int64, size int64, mod time.Time, r io.Reader) error {
		if err := tw.WriteHeader(&tar.Header{Name: n, Mode: mode, Size: size, ModTime: mod}); err != nil {
			return err
		}
		h := sha256.New()
		written, err := io.Copy(io.MultiWriter(tw, h), r)
		if err != nil {
			return err
		}
		if written != size {
			return fmt.Errorf("%s changed while it was copied", n)
		}
		if man != nil {
			man.Files[n] = manifestFile{Size: size, SHA256: hex.EncodeToString(h.Sum(nil))}
		}
		return nil
	}
	for _, n := range names {
		src := files[n]
		st, err := os.Stat(src)
		if err != nil || !st.Mode().IsRegular() {
			continue // optional parts (no WireGuard yet, no Caddy token…)
		}
		f, err := os.Open(src)
		if err != nil {
			return err
		}
		err = add(n, 0o600, st.Size(), st.ModTime(), f)
		f.Close()
		if err != nil {
			return err
		}
	}
	if err := add(noteName, 0o644, int64(len(note)), time.Now(), strings.NewReader(note)); err != nil {
		return err
	}
	if man != nil {
		mj, _ := json.MarshalIndent(man, "", "  ")
		if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len(mj)), ModTime: time.Now()}); err != nil {
			return err
		}
		if _, err := tw.Write(mj); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func restoreNotes(ts string) string {
	return "ctrl32 telemetry backup " + ts + "\n\n" +
		"Contents: db.dump (pg_dump custom format), config.toml (contains the password pepper and secret key — the\n" +
		"database is useless without them), Caddyfile/caddy.json/caddy.env, wireguard/ (server key, peers), backup.json\n" +
		"(backup settings, also the offsite target and its credentials), manifest.json (version, migrations, row counts,\n" +
		"SHA-256 of every member).\n\n" +
		"Restore on a fresh install (same version or newer):\n" +
		"  1. curl -fsSL https://portal.telemetry.digital/install.sh | sudo bash -s -- --domain <domain> --no-start\n" +
		"  2. copy config.toml to /etc/ctrl32-telemetry/config.toml (root:ctrl32 0640); optionally caddy.*, wireguard/ to their places\n" +
		"  3. docker exec -i ctrl32-telemetry-postgres pg_restore -U ctrl32 -d ctrl32 --clean --if-exists --no-owner < db.dump\n" +
		"     (native PostgreSQL: runuser -u postgres -- pg_restore -d ctrl32 --clean --if-exists --no-owner db.dump)\n" +
		"  4. ctrl32-telemetry migrate -config /etc/ctrl32-telemetry/config.toml && systemctl start ctrl32-telemetry ctrl32-telemetry-agent\n\n" +
		"Encrypted bundles (.enc): openssl enc -d -aes-256-cbc -pbkdf2 -iter 200000 -in <file>.enc -out <file>\n" +
		"Checksum: sha256sum -c <file>.sha256\n\n" +
		"Windows (install.ps1): copy config.toml to %ProgramData%\\ctrl32-telemetry\\config.toml, then\n" +
		"  \"C:\\Program Files\\ctrl32-telemetry\\pgsql\\bin\\pg_restore.exe\" --dbname <db.admin_url from config.toml> --clean --if-exists --no-owner db.dump\n" +
		"  ctrl32-telemetry.exe migrate -config %ProgramData%\\ctrl32-telemetry\\config.toml; Start-Service ctrl32-telemetry, ctrl32-telemetry-agent\n"
}

// ---- backup runs -------------------------------------------------------------------------------------------------

// runBackup takes a backup, copies it to the offsite target and applies the retention: the whole run with its steps
// goes into the history.
func (a *Server) runBackup(ctx context.Context, trigger, actor, passphrase string) BackupRun {
	rec := a.newRun("backup", trigger, actor)
	info, err := a.createBackup(ctx, passphrase, rec)
	if err == nil {
		rec.run.File, rec.run.SizeBytes = info.Name, info.SizeBytes
		st := a.backupLoad()
		if st.Offsite.Enabled {
			err = a.backupOffsite(ctx, rec, st, info)
		}
		if trigger == "schedule" {
			_ = rec.step("prune", func() (string, error) { return a.prune(st.Keep, st.KeepWeekly, st.KeepMonthly) })
		}
	}
	_ = a.backupUpdate(func(s *backupStored) {
		s.LastRun = rec.run.StartedAt
		if info.Name != "" {
			s.LastFile = info.Name // the local backup exists even when its offsite copy failed
		}
		if err != nil {
			s.LastError = err.Error()
		} else {
			s.LastError, s.LastFile, s.LastOK = "", info.Name, time.Now()
		}
	})
	if err != nil {
		a.log.Error("backup failed", "trigger", trigger, "err", err)
	}
	return rec.finish(err)
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
	if in.Passphrase == "" && in.UseStored {
		in.Passphrase = a.backupLoad().Passphrase
	}
	if !backupRunning.CompareAndSwap(false, true) {
		writeErr(w, 409, "a backup is already running")
		return
	}
	// Large databases take minutes: run in the background, the UI polls the state.
	go func(pass, actor string) {
		defer backupRunning.Store(false)
		backupMu.Lock()
		defer backupMu.Unlock()
		a.runBackup(context.Background(), "manual", actor, pass)
	}(in.Passphrase, in.Actor)
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
	_ = os.Remove(filepath.Join(BackupDir, name+".sha256"))
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
	if in.KeepWeekly < 0 || in.KeepWeekly > 104 || in.KeepMonthly < 0 || in.KeepMonthly > 120 {
		writeErr(w, 400, "weekly backups 0–104, monthly backups 0–120")
		return
	}
	if strings.ContainsAny(in.Passphrase, "\n\r") {
		writeErr(w, 400, "invalid passphrase")
		return
	}
	st := a.backupLoad()
	if in.ClearPassphrase && st.Offsite.Enabled {
		writeErr(w, 409, "the offsite target receives only encrypted backups: turn it off before removing the passphrase")
		return
	}
	err := a.backupUpdate(func(s *backupStored) {
		s.Enabled, s.Hour, s.Minute, s.Keep, s.KeepWeekly, s.KeepMonthly = in.Enabled, in.Hour, in.Minute, in.Keep, in.KeepWeekly, in.KeepMonthly
		if in.ClearPassphrase {
			s.Passphrase = ""
		} else if in.Passphrase != "" {
			s.Passphrase = in.Passphrase
		}
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.log.Info("backup schedule saved", "enabled", in.Enabled, "at", fmt.Sprintf("%02d:%02d", in.Hour, in.Minute), "keep", in.Keep, "weekly", in.KeepWeekly, "monthly", in.KeepMonthly)
	writeJSON(w, 200, a.backupState(r.Context()))
}

// runScheduler performs the daily backup and the monthly restore test; it runs in the agent process.
func (a *Server) runScheduler(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			a.schedulerTick(ctx, now)
		}
	}
}

func sameDay(a, b time.Time) bool { return a.YearDay() == b.YearDay() && a.Year() == b.Year() }

func (a *Server) schedulerTick(ctx context.Context, now time.Time) {
	st := a.backupLoad()
	if st.Enabled && now.Hour() == st.Hour && now.Minute() == st.Minute && !sameDay(st.LastRun, now) && backupRunning.CompareAndSwap(false, true) {
		backupMu.Lock()
		a.runBackup(ctx, "schedule", "", st.Passphrase)
		backupMu.Unlock()
		backupRunning.Store(false)
	}
	rt := st.RestoreTest
	if rt.Enabled && now.Day() == rt.Day && now.Hour() == rt.Hour && now.Minute() == rt.Minute && !sameDay(rt.LastRun, now) && restoreRunning.CompareAndSwap(false, true) {
		backupMu.Lock()
		a.runRestoreTest(ctx, "schedule", "", rt.Source)
		backupMu.Unlock()
		restoreRunning.Store(false)
	}
}

// prune keeps the backups of the retention rule and deletes the others (with their checksum files).
func (a *Server) prune(keep, weekly, monthly int) (string, error) {
	files := a.localBackups(nil)
	times := make([]time.Time, len(files))
	for i, f := range files {
		times[i] = f.CreatedAt
	}
	k := gfsKeep(times, keep, weekly, monthly)
	deleted := 0
	for i, f := range files {
		if k[i] {
			continue
		}
		if err := os.Remove(filepath.Join(BackupDir, f.Name)); err == nil {
			_ = os.Remove(filepath.Join(BackupDir, f.Name+".sha256"))
			a.log.Info("backup pruned", "file", f.Name)
			deleted++
		}
	}
	return fmt.Sprintf("%d kept, %d deleted (daily %d, weekly %d, monthly %d)", len(files)-deleted, deleted, keep, weekly, monthly), nil
}

// ---- restore of the live database (rollback of an update) ----------------------------------------------------------

// restoreDatabase restores the database from a backup of this agent: the rollback of an update whose migrations
// failed (update.go). The dump goes into a new database first; only when that succeeded is it swapped in for the old
// one while the service is stopped (pg_restore --clean on the live database fails on partitioned tables, and a failed
// restore must never leave the server without a database).
func (a *Server) restoreDatabase(ctx context.Context, name string) error {
	if a.simulate {
		a.log.Info("simulation: the database restore is not run", "backup", name)
		return nil
	}
	if !backupNameRe.MatchString(name) {
		return errors.New("invalid backup name")
	}
	work, err := os.MkdirTemp(BackupDir, ".restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	rctx, cancel := context.WithTimeout(ctx, 60*time.Minute)
	defer cancel()
	dump, _, err := a.openBundle(filepath.Join(BackupDir, name), work, a.backupLoad().Passphrase)
	if err != nil {
		return err
	}
	db, err := a.liveDB()
	if err != nil {
		return err
	}
	tmpDB, oldDB := db+"_restore", db+"_before_restore"
	if err := a.restoreInto(rctx, dump, tmpDB); err != nil {
		return err
	}
	sql := func(statement string) error { _, err := a.psql(rctx, "postgres", statement); return err }
	stopApp(rctx)
	defer startApp(rctx)
	if err := sql(`DROP DATABASE IF EXISTS "` + oldDB + `" WITH (FORCE)`); err != nil {
		return err
	}
	if err := sql(`ALTER DATABASE "` + db + `" RENAME TO "` + oldDB + `"`); err != nil {
		// sessions still open (a replica, a console): close them and try once more
		_ = sql(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '` + db + `' AND pid <> pg_backend_pid()`)
		if err := sql(`ALTER DATABASE "` + db + `" RENAME TO "` + oldDB + `"`); err != nil {
			return err
		}
	}
	if err := sql(`ALTER DATABASE "` + tmpDB + `" RENAME TO "` + db + `"`); err != nil {
		_ = sql(`ALTER DATABASE "` + oldDB + `" RENAME TO "` + db + `"`)
		return err
	}
	_ = sql(`DROP DATABASE IF EXISTS "` + oldDB + `" WITH (FORCE)`)
	a.log.Info("database restored", "backup", name)
	return nil
}

// openBundle decrypts a bundle when needed and extracts its database dump into work; it returns the dump and the
// manifest (nil for bundles made before 0.77.0) after checking every member against the manifest.
func (a *Server) openBundle(file, work, passphrase string) (dump string, man *backupManifest, err error) {
	bundle := file
	if strings.HasSuffix(file, ".enc") {
		if passphrase == "" {
			return "", nil, errors.New("the backup is encrypted and no passphrase is stored")
		}
		bundle = filepath.Join(work, "bundle.tar.gz")
		if err := decryptFile(file, bundle, passphrase); err != nil {
			return "", nil, err
		}
	}
	dump = filepath.Join(work, "db.dump")
	man, err = extractBundle(bundle, dump)
	return dump, man, err
}

// extractBundle reads a bundle: it writes db.dump to dumpPath, hashes every member and checks them against the
// manifest when there is one.
func extractBundle(bundle, dumpPath string) (*backupManifest, error) {
	f, err := os.Open(bundle)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("the bundle is damaged: %w", err)
	}
	tr := tar.NewReader(gz)
	sums := map[string]manifestFile{}
	var man *backupManifest
	haveDump := false
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("the bundle is damaged: %w", err)
		}
		if h.Name == "manifest.json" {
			b, err := io.ReadAll(io.LimitReader(tr, 1<<20))
			if err != nil {
				return nil, err
			}
			man = &backupManifest{}
			if err := json.Unmarshal(b, man); err != nil {
				return nil, fmt.Errorf("manifest.json is damaged: %w", err)
			}
			continue
		}
		hs := sha256.New()
		var w io.Writer = hs
		var out *os.File
		if h.Name == "db.dump" {
			if out, err = os.OpenFile(dumpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err != nil {
				return nil, err
			}
			w = io.MultiWriter(hs, out)
			haveDump = true
		}
		n, err := io.Copy(w, tr)
		if out != nil {
			if cerr := out.Close(); err == nil {
				err = cerr
			}
		}
		if err != nil {
			return nil, fmt.Errorf("the bundle is damaged (%s): %w", h.Name, err)
		}
		sums[h.Name] = manifestFile{Size: n, SHA256: hex.EncodeToString(hs.Sum(nil))}
	}
	if !haveDump {
		return nil, errors.New("db.dump is not in the backup")
	}
	if man != nil {
		for n, want := range man.Files {
			got, ok := sums[n]
			if !ok {
				return man, fmt.Errorf("%s is listed in the manifest but missing in the bundle", n)
			}
			if got != want {
				return man, fmt.Errorf("%s differs from the manifest (damaged bundle)", n)
			}
		}
	}
	return man, nil
}

// restoreInto restores a dump into a new database target (dropped first if it exists; dropped again on failure).
func (a *Server) restoreInto(ctx context.Context, dump, target string) error {
	sql := func(statement string) error { _, err := a.psql(ctx, "postgres", statement); return err }
	if err := sql(`DROP DATABASE IF EXISTS "` + target + `" WITH (FORCE)`); err != nil {
		return err
	}
	if err := sql(`CREATE DATABASE "` + target + `" OWNER "` + dbAdmin + `"`); err != nil {
		return err
	}
	var cmd *exec.Cmd
	args := []string{"--no-owner", "--exit-on-error", "--single-transaction"}
	switch mode := a.dbMode(ctx); {
	case mode == "docker":
		cmd = exec.CommandContext(ctx, "docker", append([]string{"exec", "-i", a.pgContainer, "pg_restore", "-U", dbAdmin, "-d", target}, args...)...)
	case isWindows():
		cmd = exec.CommandContext(ctx, strings.Replace(findPgDump(a.pgDump), "pg_dump.exe", "pg_restore.exe", 1), append([]string{"--dbname", withDB(a.adminURL(), target)}, args...)...)
	default:
		// the objects belong to the database administrator of the installation, as the migrations created them
		cmd = exec.CommandContext(ctx, "runuser", append([]string{"-u", "postgres", "--", "pg_restore", "-d", target, "--role=" + dbAdmin}, args...)...)
	}
	in, err := os.Open(dump)
	if err != nil {
		return err
	}
	defer in.Close()
	var stderr bytes.Buffer
	cmd.Stdin, cmd.Stderr = in, &stderr
	if err := cmd.Run(); err != nil {
		_ = sql(`DROP DATABASE IF EXISTS "` + target + `" WITH (FORCE)`)
		return fmt.Errorf("pg_restore failed (the database was not changed): %s", firstLine(strings.TrimSpace(stderr.String())))
	}
	return nil
}

// withDB replaces the database of a PostgreSQL URL.
func withDB(raw, db string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Path = "/" + db
	return u.String()
}

// ---- small helpers ---------------------------------------------------------------------------------------------

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// readChecksum returns the hash of a checksum file ("<hex>  <name>"), or "".
func readChecksum(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	f := strings.Fields(string(b))
	if len(f) == 0 || len(f[0]) != 64 {
		return ""
	}
	return f[0]
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f kB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

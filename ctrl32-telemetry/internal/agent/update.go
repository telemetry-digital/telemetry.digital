package agent

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/release"
)

// Updates (ADR-0024, 0.76.0). The agent installs a release only when its manifest (release.json) carries a valid
// signature of a trusted key and every file matches the size and SHA-256 the manifest lists. A release comes from the
// distribution point (update_url: the portal or a mirror in the customer's network) or from an uploaded offline
// package staged on this host. The installation:
//
//  1. verifies the manifest, downloads the binary of this platform and checks it against the manifest, runs it with
//     `version` and checks the licence (updates_until against the signed release date) — nothing has changed yet;
//  2. optionally (always in the automatic mode) takes a backup: database, configuration, agent files;
//  3. keeps the running binary as <binary>.prev, puts the new one in place and runs the schema migrations;
//     when the migrations fail, the previous binary returns at once and the database is restored from the backup of
//     step 2 (migrations are forward-only: an older binary is not guaranteed to work on a newer schema);
//  4. restarts the service and the agent; a guard process of the PREVIOUS binary (a systemd transient unit, on Windows
//     a detached process) waits until the new version answers /healthz with its version three times in a row; when it
//     does not within the rollback time, the guard puts the previous binary back and restarts the services. The
//     database is not restored then: the migrations succeeded, the schema stays (see ADR-0024, "rollback").
//
// Every step is recorded in update.json (the history); the server writes the results into its audit trail.
//
//	POST /update            UpdateRequest: install from the distribution point or the staged package
//	POST /update/package    body = offline package (.zip); ?install=1&backup=1&mode=&updates_until=&rollback_minutes=
//	GET  /update/state      history, staged package, trusted keys, signature policy
//	POST /update/ack        AckRequest: results written into the audit trail

const (
	defaultRollbackMinutes = 10
	defaultMaxPackageMB    = 2048
	healthyStreak          = 3
	maxHistory             = 20
)

var (
	updateStateFile string // derived from etcDir (paths.go)
	updateFileMu    sync.Mutex
)

// updatesDir holds uploads and the staged offline package (Linux: /var/lib/ctrl32-telemetry/updates).
func updatesDir() string {
	if isWindows() {
		return filepath.Join(etcDir, "updates")
	}
	return filepath.Join(libDir, "updates")
}

// stagedDir is the verified content of the last uploaded package.
func stagedDir() string { return filepath.Join(updatesDir(), "release") }

// SetUpdatePolicy applies [agent] max_package_mb and allow_unsigned_updates (the latter only in development builds).
func (a *Server) SetUpdatePolicy(maxPackageMB int, allowUnsigned bool) {
	a.maxPackageMB = maxPackageMB
	a.allowUnsigned = allowUnsigned
	if allowUnsigned && !release.DevBuild() {
		a.log.Warn("agent.allow_unsigned_updates is ignored: this is a release build, which installs only signed releases")
	}
}

// SetDirect marks a Server used by the command line in place of the agent service (restarts happen synchronously).
func (a *Server) SetDirect() { a.direct = true }

func (a *Server) packageLimit() int64 {
	mb := a.maxPackageMB
	if mb <= 0 {
		mb = defaultMaxPackageMB
	}
	return int64(mb) << 20
}

// requireSignature: release builds always; development builds unless allow_unsigned_updates.
func (a *Server) requireSignature() bool { return !(release.DevBuild() && a.allowUnsigned) }

func (a *Server) updateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /update", a.hUpdate)
	mux.HandleFunc("POST /update/package", a.hUpdatePackage)
	mux.HandleFunc("GET /update/state", a.hUpdateState)
	mux.HandleFunc("POST /update/ack", a.hUpdateAck)
}

// ---- history ------------------------------------------------------------------------------------------------------

type updateStore struct {
	Current *UpdateRecord  `json:"current,omitempty"`
	History []UpdateRecord `json:"history"`
}

func loadUpdateStore() updateStore {
	var st updateStore
	_ = loadJSON(updateStateFile, &st)
	return st
}

// saveRecord makes rec the current record and keeps it in the history (newest first).
func saveRecord(rec UpdateRecord) {
	updateFileMu.Lock()
	defer updateFileMu.Unlock()
	st := loadUpdateStore()
	r := rec
	st.Current = &r
	st.History = slices.DeleteFunc(st.History, func(h UpdateRecord) bool { return h.ID == rec.ID })
	st.History = append([]UpdateRecord{rec}, st.History...)
	if len(st.History) > maxHistory {
		st.History = st.History[:maxHistory]
	}
	_ = saveJSON(updateStateFile, st, 0o640)
}

func loadRecord(id string) (UpdateRecord, bool) {
	updateFileMu.Lock()
	defer updateFileMu.Unlock()
	st := loadUpdateStore()
	for _, h := range st.History {
		if h.ID == id {
			return h, true
		}
	}
	return UpdateRecord{}, false
}

// busy reports an installation that is still running (or waiting for the new version to answer).
func busy(now time.Time) *UpdateRecord {
	st := loadUpdateStore()
	c := st.Current
	if c == nil {
		return nil
	}
	switch c.Phase {
	case "installing", "rolling_back":
		if now.Sub(c.StartedAt) < 2*time.Hour {
			return c
		}
	case "restarting":
		if !c.Deadline.IsZero() && now.Before(c.Deadline.Add(2*time.Minute)) {
			return c
		}
	}
	return nil
}

// ---- sources ------------------------------------------------------------------------------------------------------

var errNotFound = errors.New("not found")

// releaseSource provides the files of a release: the distribution point over HTTP(S), or a staged package.
type releaseSource interface {
	open(ctx context.Context, name string) (io.ReadCloser, error)
	String() string
}

type httpSource struct {
	base   string
	client *http.Client
}

func (h httpSource) String() string { return h.base }

func (h httpSource) open(ctx context.Context, name string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", h.base+"/"+name, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ctrl32-telemetry-agent")
	c := h.client
	if c == nil {
		c = &http.Client{Timeout: 30 * time.Minute}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 404 {
		resp.Body.Close()
		return nil, errNotFound
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("%s/%s: HTTP %d", h.base, name, resp.StatusCode)
	}
	return resp.Body, nil
}

type dirSource struct{ dir string }

func (d dirSource) String() string { return "the uploaded package" }

func (d dirSource) open(_ context.Context, name string) (io.ReadCloser, error) {
	f, err := os.Open(filepath.Join(d.dir, filepath.FromSlash(name)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, errNotFound
	}
	return f, err
}

func readSmall(ctx context.Context, src releaseSource, name string, max int64) ([]byte, error) {
	rc, err := src.open(ctx, name)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("%s is too large", name)
	}
	return b, nil
}

// fetchTo copies a file of the release to dst and returns its SHA-256 and size (at most max bytes).
func fetchTo(ctx context.Context, src releaseSource, name, dst string, max int64) (string, int64, error) {
	rc, err := src.open(ctx, name)
	if err != nil {
		return "", 0, err
	}
	defer rc.Close()
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(rc, max+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", n, err
	}
	if n > max {
		return "", n, fmt.Errorf("%s is larger than its manifest says", name)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// verifyManifest checks the signature (required unless a development build allows unsigned releases) and the manifest.
func (a *Server) verifyManifest(mb, sb []byte) (*release.Manifest, string, error) {
	keys := release.TrustedKeys()
	if a.testKeys != nil {
		keys = a.testKeys
	}
	keyID, err := release.VerifySignature(mb, sb, keys)
	if err == nil {
		m, err := release.ParseManifest(mb)
		return m, keyID, err
	}
	if a.requireSignature() || !errors.Is(err, release.ErrUnsigned) {
		return nil, "", err
	}
	a.log.Warn("installing an UNSIGNED release (development build with agent.allow_unsigned_updates)")
	m, err := release.ParseManifest(mb)
	return m, "", err
}

// ---- installation -------------------------------------------------------------------------------------------------

// updateErr is a refusal with an HTTP status and an optional machine-readable code.
type updateErr struct {
	status int
	code   string
	msg    string
}

func (e *updateErr) Error() string { return e.msg }

func uerr(status int, code, format string, args ...any) *updateErr {
	return &updateErr{status: status, code: code, msg: fmt.Sprintf(format, args...)}
}

// prepared is a verified binary waiting in binPath.new.
type prepared struct {
	version, released, keyID, sha string
	tmp                           string
}

// prepare verifies the release and stages its binary; nothing on the system changes.
func (a *Server) prepare(ctx context.Context, src releaseSource, want, updatesUntil string) (*prepared, *updateErr) {
	name := release.BinaryName(runtime.GOOS, runtime.GOARCH)
	tmp := binPath + ".new"
	_ = os.MkdirAll(filepath.Dir(tmp), 0o755)
	mb, err := readSmall(ctx, src, release.ManifestFile, release.MaxManifestBytes)
	p := &prepared{tmp: tmp}
	switch {
	case errors.Is(err, errNotFound) && a.requireSignature():
		return nil, uerr(502, "unsigned", "%s offers no signed release manifest (%s); this server installs only signed releases — update refused", src, release.ManifestFile)
	case errors.Is(err, errNotFound):
		// development build with allow_unsigned_updates: the checksums of SHA256SUMS as before 0.76.0
		a.log.Warn("installing an UNSIGNED release from SHA256SUMS (development build with agent.allow_unsigned_updates)")
		sums, err := readSmall(ctx, src, "SHA256SUMS", 1<<20)
		if err != nil {
			return nil, uerr(502, "", "checksums unavailable: %v", err)
		}
		want256 := ""
		for _, l := range strings.Split(string(sums), "\n") {
			f := strings.Fields(l)
			if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
				want256 = f[0]
			}
		}
		if want256 == "" {
			return nil, uerr(502, "", "no checksum published for %s", name)
		}
		got, _, err := fetchTo(ctx, src, name, tmp, 512<<20)
		if err != nil {
			_ = os.Remove(tmp)
			return nil, uerr(502, "", "download failed: %v", err)
		}
		if got != want256 {
			_ = os.Remove(tmp)
			return nil, uerr(502, "checksum", "checksum mismatch — update refused")
		}
		p.version, p.sha = want, got
	case err != nil:
		return nil, uerr(502, "", "release manifest unavailable from %s: %v", src, err)
	default:
		sb, err := readSmall(ctx, src, release.SignatureFile, 64<<10)
		if err != nil && !errors.Is(err, errNotFound) {
			return nil, uerr(502, "", "release signature unavailable: %v", err)
		}
		m, keyID, verr := a.verifyManifest(mb, sb)
		if verr != nil {
			a.log.Warn("release refused", "source", src.String(), "err", verr)
			return nil, uerr(502, "signature", "%v", verr)
		}
		if want != "" && m.Version != want {
			return nil, uerr(409, "version", "%s offers version %s, not %s", src, m.Version, want)
		}
		f, ok := m.File(name)
		if !ok {
			return nil, uerr(502, "", "release %s has no binary for this platform (%s)", m.Version, name)
		}
		got, n, err := fetchTo(ctx, src, name, tmp, f.Size)
		if err != nil {
			_ = os.Remove(tmp)
			return nil, uerr(502, "", "download failed: %v", err)
		}
		if n != f.Size || got != f.SHA256 {
			_ = os.Remove(tmp)
			return nil, uerr(502, "checksum", "%s does not match the signed manifest (size or SHA-256) — update refused", name)
		}
		p.version, p.released, p.keyID, p.sha = m.Version, m.Released, keyID, got
	}
	out, err := run(ctx, 20*time.Second, tmp, "version")
	if err != nil || !strings.Contains(out, " "+p.version) {
		_ = os.Remove(tmp)
		return nil, uerr(502, "", "the new binary does not report version %s", p.version)
	}
	if d, ok := release.FromVersionOutput(out); ok && p.released != "" && d.Format(release.Layout) != p.released {
		_ = os.Remove(tmp)
		return nil, uerr(502, "", "the new binary reports release date %s, the signed manifest %s — update refused", d.Format(release.Layout), p.released)
	}
	check := out
	if p.released != "" {
		check = "ctrl32-telemetry " + p.version + " released " + p.released // the signed date decides
	}
	if msg := updateEntitled(updatesUntil, check, time.Now()); msg != "" {
		_ = os.Remove(tmp)
		a.log.Warn("update refused: the licence does not include it", "version", p.version, "updates_until", updatesUntil)
		return nil, &updateErr{status: 402, code: "updates_ended", msg: msg}
	}
	return p, nil
}

// install runs the whole installation (see the package comment of this file).
func (a *Server) install(ctx context.Context, in UpdateRequest, src releaseSource) (*UpdateResult, *updateErr) {
	if c := busy(time.Now()); c != nil {
		return nil, uerr(409, "busy", "an update to %s is in progress (%s)", c.To, c.Phase)
	}
	if a.simulate {
		a.seedSimulatedBinary()
	}
	p, e := a.prepare(ctx, src, in.Version, in.UpdatesUntil)
	// from here on a closed connection of the caller must not interrupt the backup, the migrations or a rollback
	ctx = context.WithoutCancel(ctx)
	if e != nil {
		return nil, e
	}
	mode := in.Mode
	if mode == "" {
		mode = "manual"
	}
	source := "online"
	if in.Source == "package" {
		source = "package"
	}
	rec := UpdateRecord{ID: randomID()[:12], From: a.version, To: p.version, Mode: mode, Source: source, KeyID: p.keyID, SHA256: p.sha,
		Actor: in.Actor, Reason: in.Reason, Phase: "installing", StartedAt: time.Now().UTC()}
	saveRecord(rec)
	fail := func(status int, msg string) (*UpdateResult, *updateErr) {
		rec.Phase, rec.Error, rec.FinishedAt = "failed", msg, time.Now().UTC()
		saveRecord(rec)
		_ = os.Remove(p.tmp)
		return nil, uerr(status, "", "%s", msg)
	}
	if in.Backup {
		name, err := a.preUpdateBackup(ctx)
		if err != nil {
			return fail(500, "the backup before the update failed, nothing was changed: "+err.Error())
		}
		rec.Backup = name
		saveRecord(rec)
	}
	prev := binPath + ".prev"
	if err := parkBinary(prev); err != nil {
		return fail(500, "cannot keep the running binary: "+err.Error())
	}
	if err := os.Rename(p.tmp, binPath); err != nil {
		_ = restoreBinary(prev)
		return fail(500, "cannot put the new binary in place: "+err.Error())
	}
	if out, err := run(ctx, 10*time.Minute, binPath, "migrate", "-config", a.cfgPath); err != nil {
		msg := "migration failed: " + firstLine(out)
		if out == "" {
			msg = "migration failed: " + err.Error()
		}
		if a.rollbackNow(ctx, &rec, msg) {
			return nil, uerr(500, "rolled_back", "%s; the previous version is back, the database is being restored from %s and the service restarts — see the history", msg, rec.Backup)
		}
		return nil, uerr(500, "rolled_back", "%s", rec.Error)
	}
	rec.Phase = "restarting"
	minutes := in.RollbackMinutes
	if minutes <= 0 {
		minutes = defaultRollbackMinutes
	}
	rec.Deadline = time.Now().UTC().Add(time.Duration(minutes) * time.Minute)
	saveRecord(rec)
	a.log.Info("update installed", "id", rec.ID, "from", rec.From, "to", rec.To, "mode", rec.Mode, "source", rec.Source, "key", rec.KeyID, "sha256", p.sha, "backup", rec.Backup)
	a.startGuard(rec)
	switch {
	case a.simulate:
	case a.direct:
		// `ctrl32-telemetry update --file` without the agent service: this process ends, so it restarts now
		if isWindows() {
			_, _ = run(ctx, 2*time.Minute, "powershell", "-NoProfile", "-NonInteractive", "-Command", "Restart-Service -Name '"+appUnit+"','"+agentUnit+"' -Force")
		} else {
			_, _ = run(ctx, 2*time.Minute, "systemctl", "restart", appUnit, agentUnit)
		}
	default:
		a.restartAppLater()
		if isWindows() {
			restartServiceLater(agentUnit, 4*time.Second)
		} else {
			go func() {
				time.Sleep(4 * time.Second)
				_, _ = run(context.Background(), 30*time.Second, "systemctl", "restart", agentUnit)
			}()
		}
	}
	msg := "installed; the service and the agent restart, the previous version returns if the new one does not answer healthy within " + fmt.Sprint(minutes) + " minutes"
	return &UpdateResult{ID: rec.ID, Version: p.version, SHA256: p.sha, KeyID: p.keyID, Backup: rec.Backup, Message: msg}, nil
}

// parkBinary keeps the running binary as prev (Linux: a copy; Windows: the running executable is renamed, it cannot
// be overwritten).
func parkBinary(prev string) error {
	if _, err := os.Stat(binPath); err != nil {
		return err
	}
	if isWindows() {
		_ = os.Remove(prev)
		return os.Rename(binPath, prev)
	}
	if err := copyFile(binPath, prev+".tmp", 0o755); err != nil {
		return err
	}
	return os.Rename(prev+".tmp", prev)
}

// restoreBinary puts prev back as the binary (a copy, so prev stays for a later manual rollback).
func restoreBinary(prev string) error {
	if _, err := os.Stat(prev); err != nil {
		return fmt.Errorf("the previous binary is missing: %w", err)
	}
	tmp := binPath + ".rollback"
	if err := copyFile(prev, tmp, 0o755); err != nil {
		return err
	}
	if isWindows() {
		_ = os.Remove(binPath + ".failed")
		_ = os.Rename(binPath, binPath+".failed")
	}
	return os.Rename(tmp, binPath)
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

// rollbackPlan decides what a rollback restores: always the binary; the database only when the migrations failed
// and a backup was taken before the update (after successful migrations the schema stays: they are forward-only and
// a restore would lose the data written since).
func rollbackPlan(migrationsFailed bool, backup string) (binary, database bool) {
	return true, migrationsFailed && backup != ""
}

// rollbackNow undoes an installation whose migrations failed: the previous binary at once, and the database from the
// backup. The database restore stops the web service — the one that is waiting for this answer — so it runs in the
// background (returns true) unless this process is the command line, which waits for it.
func (a *Server) rollbackNow(ctx context.Context, rec *UpdateRecord, why string) bool {
	ctx = context.WithoutCancel(ctx)
	_, restoreDB := rollbackPlan(true, rec.Backup)
	rec.Error = why
	err := restoreBinary(binPath + ".prev")
	rec.RolledBack = "binary"
	if err != nil {
		rec.Error += "; restoring the previous binary failed: " + err.Error()
	}
	finish := func() {
		if restoreDB {
			if derr := a.restoreDatabase(ctx, rec.Backup); derr != nil {
				rec.Error += "; restoring the database from " + rec.Backup + " failed: " + derr.Error()
			} else {
				rec.RolledBack = "binary and database"
				rec.Error += "; the database was restored from " + rec.Backup
			}
		} else {
			rec.Error += "; no backup was taken before the update: the database may hold a part of the new schema — restore a backup if the service does not start"
		}
		rec.Phase, rec.FinishedAt = "rolled_back", time.Now().UTC()
		saveRecord(*rec)
		a.log.Error("update rolled back", "id", rec.ID, "to", rec.To, "err", rec.Error)
	}
	if restoreDB && !a.direct && !a.simulate {
		rec.Phase = "rolling_back"
		saveRecord(*rec)
		go func() {
			a.mu.Lock() // no other operation of the agent while the database is replaced
			defer a.mu.Unlock()
			finish()
		}()
		return true
	}
	finish()
	return false
}

// preUpdateBackup takes a backup with the passphrase of the schedule (the simulation writes a stand-in).
func (a *Server) preUpdateBackup(ctx context.Context) (string, error) {
	if a.simulate {
		_ = os.MkdirAll(BackupDir, 0o750)
		name := "ctrl32-" + time.Now().Format("20060102-150405") + ".tar.gz"
		return name, os.WriteFile(filepath.Join(BackupDir, name), []byte("simulated backup before an update\n"), 0o640)
	}
	if !backupRunning.CompareAndSwap(false, true) {
		return "", errors.New("a backup is already running; try again when it has finished")
	}
	defer backupRunning.Store(false)
	backupMu.Lock()
	defer backupMu.Unlock()
	info, err := a.createBackup(ctx, a.backupLoad().Passphrase)
	if err != nil {
		return "", err
	}
	return info.Name, nil
}

// seedSimulatedBinary gives the simulation a "running binary" (a copy of this executable) on its first update.
func (a *Server) seedSimulatedBinary() {
	if _, err := os.Stat(binPath); err == nil {
		return
	}
	if exe, err := os.Executable(); err == nil {
		_ = os.MkdirAll(filepath.Dir(binPath), 0o755)
		_ = copyFile(exe, binPath, 0o755)
	}
}

// ---- the guard: the new version must answer healthy ------------------------------------------------------------

// guardDecision: "done" after healthyStreak healthy answers in a row, "rollback" when the deadline passed, else "wait".
func guardDecision(now, deadline time.Time, streak int) string {
	switch {
	case streak >= healthyStreak:
		return "done"
	case now.After(deadline):
		return "rollback"
	}
	return "wait"
}

// healthURL is the address of /healthz of the local service (config [http] listen).
func healthURL(cfgPath string) string {
	listen, scheme := ":8080", "http"
	var cfg struct {
		HTTP config.HTTPConfig `toml:"http"`
	}
	if _, err := toml.DecodeFile(cfgPath, &cfg); err == nil {
		if cfg.HTTP.Listen != "" {
			listen = cfg.HTTP.Listen
		}
		if cfg.HTTP.TLSCert != "" {
			scheme = "https"
		}
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		host, port = "", "8080"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return scheme + "://" + net.JoinHostPort(host, port) + "/healthz"
}

// probeHealth reports whether the service answers healthy with the expected version.
func probeHealth(ctx context.Context, url, version string) bool {
	c := &http.Client{Timeout: 4 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}} // loopback only
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var h struct{ Status, DB, Version string }
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&h) != nil {
		return false
	}
	return h.Status == "ok" && h.Version == version
}

// startGuard watches the new version from a process of the previous binary, which survives the restarts.
func (a *Server) startGuard(rec UpdateRecord) {
	if a.simulate {
		go a.guardLoop(context.Background(), rec.ID, healthURL(a.cfgPath), 2*time.Second)
		return
	}
	prev := binPath + ".prev"
	args := []string{"update", "guard", "-config", a.cfgPath, "-state", updateStateFile, "-id", rec.ID, "-bin", binPath}
	if !isWindows() {
		unit := "ctrl32-telemetry-update-guard-" + rec.ID
		if out, err := run(context.Background(), 20*time.Second, "systemd-run", append([]string{"--unit=" + unit, "--collect", "--quiet", prev}, args...)...); err == nil {
			return
		} else {
			a.log.Warn("systemd-run of the update guard failed, starting it as a detached process", "out", out, "err", err)
		}
	}
	if err := spawnDetached(prev, args...); err != nil {
		a.log.Error("the update guard could not be started: no automatic rollback", "err", err)
	}
}

// Guard is `ctrl32-telemetry update guard`: it runs from the previous binary and waits for the new version. bin is the
// installed binary (on Windows the guard itself runs from <bin>.prev).
func Guard(ctx context.Context, cfgPath, stateFile, id, bin string, log *slog.Logger) error {
	updateStateFile = stateFile
	if bin != "" {
		binPath = bin
	}
	a := &Server{cfgPath: cfgPath, log: log}
	a.guardLoop(ctx, id, healthURL(cfgPath), 5*time.Second)
	return nil
}

func (a *Server) guardLoop(ctx context.Context, id, url string, every time.Duration) {
	rec, ok := loadRecord(id)
	if !ok || rec.Phase != "restarting" {
		a.log.Warn("update guard: nothing to watch", "id", id)
		return
	}
	a.log.Info("update guard: waiting for the new version", "id", id, "version", rec.To, "url", url, "deadline", rec.Deadline)
	streak := 0
	for {
		if probeHealth(ctx, url, rec.To) {
			streak++
		} else {
			streak = 0
		}
		switch guardDecision(time.Now(), rec.Deadline, streak) {
		case "done":
			rec.Phase, rec.FinishedAt = "done", time.Now().UTC()
			saveRecord(rec)
			a.log.Info("update guard: the new version is healthy", "id", id, "version", rec.To)
			return
		case "rollback":
			a.rollbackAfterRestart(ctx, &rec)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

// rollbackAfterRestart puts the previous binary back after the new version did not become healthy in time.
func (a *Server) rollbackAfterRestart(ctx context.Context, rec *UpdateRecord) {
	rec.Error = fmt.Sprintf("version %s did not answer healthy until %s; the previous version %s was restored (the database keeps the new schema: the migrations had succeeded)",
		rec.To, rec.Deadline.Format(time.RFC3339), rec.From)
	if !a.simulate {
		stopApp(ctx)
	}
	if err := restoreBinary(binPath + ".prev"); err != nil {
		rec.Error += "; restoring the previous binary failed: " + err.Error()
	}
	rec.RolledBack = "binary"
	rec.Phase, rec.FinishedAt = "rolled_back", time.Now().UTC()
	saveRecord(*rec)
	a.log.Error("update guard: rolled back", "id", rec.ID, "to", rec.To, "from", rec.From)
	if !a.simulate {
		startApp(ctx)
		if isWindows() {
			restartServiceLater(agentUnit, time.Second)
		} else {
			_, _ = run(ctx, time.Minute, "systemctl", "restart", agentUnit)
		}
	}
}

func stopApp(ctx context.Context) {
	if isWindows() {
		_, _ = run(ctx, 2*time.Minute, "powershell", "-NoProfile", "-NonInteractive", "-Command", "Stop-Service -Name '"+appUnit+"' -Force")
		return
	}
	_, _ = run(ctx, 2*time.Minute, "systemctl", "stop", appUnit)
}

func startApp(ctx context.Context) {
	if isWindows() {
		_, _ = run(ctx, 2*time.Minute, "powershell", "-NoProfile", "-NonInteractive", "-Command", "Start-Service -Name '"+appUnit+"'")
		return
	}
	_, _ = run(ctx, 2*time.Minute, "systemctl", "start", appUnit)
}

// ---- handlers ----------------------------------------------------------------------------------------------------

func writeUpdateErr(w http.ResponseWriter, e *updateErr) {
	out := map[string]string{"error": e.msg}
	if e.code != "" {
		out["code"] = e.code
	}
	writeJSON(w, e.status, out)
}

func (a *Server) hUpdate(w http.ResponseWriter, r *http.Request) {
	var in UpdateRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request: "+err.Error())
		return
	}
	if in.Version != "" && !versionRe.MatchString(in.Version) || in.Version == "" && in.Source != "package" {
		writeErr(w, 400, "invalid version")
		return
	}
	if in.RollbackMinutes < 0 || in.RollbackMinutes > 120 {
		writeErr(w, 400, "rollback_minutes must be between 1 and 120")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var src releaseSource = httpSource{base: a.updateURL}
	if in.Source == "package" {
		if _, err := os.Stat(filepath.Join(stagedDir(), release.ManifestFile)); err != nil {
			writeErr(w, 404, "no verified package is staged on this server; upload it first")
			return
		}
		src = dirSource{dir: stagedDir()}
	}
	res, e := a.install(r.Context(), in, src)
	if e != nil {
		writeUpdateErr(w, e)
		return
	}
	writeJSON(w, 200, res)
}

func (a *Server) hUpdateState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, a.updateState())
}

func (a *Server) updateState() UpdateState {
	updateFileMu.Lock()
	st := loadUpdateStore()
	updateFileMu.Unlock()
	out := UpdateState{Current: st.Current, History: st.History, RequireSignature: a.requireSignature(), TestBuild: release.TestBuild(),
		UpdateURL: a.updateURL, MaxPackageMB: int(a.packageLimit() >> 20), Simulated: a.simulate, TrustedKeys: []string{}}
	if out.History == nil {
		out.History = []UpdateRecord{}
	}
	for id := range release.TrustedKeys() {
		out.TrustedKeys = append(out.TrustedKeys, id)
	}
	slices.Sort(out.TrustedKeys)
	var p PackageInfo
	if loadJSON(filepath.Join(updatesDir(), "staged.json"), &p) == nil {
		out.Staged = &p
	}
	return out
}

func (a *Server) hUpdateAck(w http.ResponseWriter, r *http.Request) {
	var in AckRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request")
		return
	}
	updateFileMu.Lock()
	st := loadUpdateStore()
	for i := range st.History {
		if slices.Contains(in.IDs, st.History[i].ID) {
			st.History[i].Audited = true
		}
	}
	if st.Current != nil && slices.Contains(in.IDs, st.Current.ID) {
		st.Current.Audited = true
	}
	err := saveJSON(updateStateFile, st, 0o640)
	updateFileMu.Unlock()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, Result{OK: true})
}

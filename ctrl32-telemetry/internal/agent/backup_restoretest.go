package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/offsite"
)

// The restore test (URS K-02): the newest backup — from the offsite target when one is set, so the copy that would
// be used after losing the server is the one tested — is fetched, checked against its checksum file, decrypted,
// checked against its manifest, restored into a temporary database and compared with the figures the manifest
// recorded; the temporary database is then dropped. The live database is never touched. Every step is in the
// protocol (the history); a failure raises an incident in the web service (K-03).

const restoreTestDB = "ctrl32_restoretest"

func (a *Server) runRestoreTest(ctx context.Context, trigger, actor, source string) BackupRun {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Hour)
	defer cancel()
	st := a.backupLoad()
	if source == "" {
		source = st.RestoreTest.Source
	}
	if source == "" || source == "offsite" && !st.Offsite.Enabled {
		source = map[bool]string{true: "offsite", false: "local"}[st.Offsite.Enabled]
	}
	rec := a.newRun("restore_test", trigger, actor)
	rec.run.Source = source
	if source == "offsite" {
		rec.run.Target = st.Offsite.Describe()
	}
	err := a.restoreTest(ctx, rec, st, source)
	_ = a.backupUpdate(func(s *backupStored) {
		s.RestoreTest.LastRun = rec.run.StartedAt
		if err == nil {
			s.RestoreTest.LastOK = time.Now()
		}
	})
	if err != nil {
		a.log.Error("restore test failed", "security", true, "err", err)
	} else {
		a.log.Info("restore test passed", "file", rec.run.File, "source", source)
	}
	return rec.finish(err)
}

func (a *Server) restoreTest(ctx context.Context, rec *runRec, st backupStored, source string) error {
	if err := os.MkdirAll(BackupDir, 0o750); err != nil {
		return err
	}
	work, err := os.MkdirTemp(BackupDir, ".restoretest-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	// 1. the newest backup at the source
	var tg offsite.Target
	var name string
	var size int64
	if err := rec.step("select", func() (string, error) {
		if source == "offsite" {
			var err error
			if tg, err = offsite.Open(ctx, st.Offsite); err != nil {
				return "", err
			}
			list, err := tg.List(ctx)
			if err != nil {
				return "", err
			}
			sort.Slice(list, func(i, j int) bool { return list[i].Name > list[j].Name })
			for _, o := range list {
				if backupNameRe.MatchString(o.Name) {
					name, size = o.Name, o.Size
					break
				}
			}
		} else {
			if files := a.localBackups(nil); len(files) > 0 {
				name, size = files[0].Name, files[0].SizeBytes
			}
		}
		if name == "" {
			return "", fmt.Errorf("no backup at the source (%s)", source)
		}
		t, _ := backupTime(name)
		return fmt.Sprintf("%s (%s, %s, %s old)", name, source, humanBytes(size), roundAge(time.Since(t))), nil
	}); err != nil {
		return err
	}
	if tg != nil {
		defer tg.Close()
	}
	rec.run.File, rec.run.SizeBytes = name, size

	// 2. the file and its checksum file
	file := filepath.Join(BackupDir, name)
	sumFile := file + ".sha256"
	if source == "offsite" {
		file, sumFile = filepath.Join(work, name), filepath.Join(work, name+".sha256")
		if err := rec.step("fetch", func() (string, error) {
			t0 := time.Now()
			if err := download(ctx, tg, name, file); err != nil {
				return "", err
			}
			if err := download(ctx, tg, name+".sha256", sumFile); err != nil {
				return "", fmt.Errorf("the checksum file is missing at the target: %w", err)
			}
			return fmt.Sprintf("downloaded in %.0f s", time.Since(t0).Seconds()), nil
		}); err != nil {
			return err
		}
	}
	if err := rec.step("checksum", func() (string, error) {
		want := readChecksum(sumFile)
		if want == "" {
			if source == "offsite" {
				return "", errors.New("no checksum file")
			}
			return "no checksum file (backup made before 0.77.0): checked by the manifest only", nil
		}
		got, err := fileSHA256(file)
		if err != nil {
			return "", err
		}
		if got != want {
			return "", fmt.Errorf("the file is damaged: SHA-256 %s… instead of %s…", got[:12], want[:12])
		}
		return "SHA-256 matches", nil
	}); err != nil {
		return err
	}

	// 3. decryption and the manifest
	var dump string
	var man *backupManifest
	if err := rec.step("manifest", func() (string, error) {
		pass := ""
		if strings.HasSuffix(name, ".enc") {
			pass = st.Passphrase
			if pass == "" {
				return "", errors.New("the backup is encrypted and no passphrase is stored on this server")
			}
		}
		var err error
		if dump, man, err = a.openBundle(file, work, pass); err != nil {
			return "", err
		}
		enc := map[bool]string{true: "decrypted, ", false: ""}[pass != ""]
		if man == nil {
			return enc + "no manifest (backup made before 0.77.0)", nil
		}
		return fmt.Sprintf("%s%d members match the manifest (version %s)", enc, len(man.Files), man.AppVersion), nil
	}); err != nil {
		return err
	}

	// 4. restore into a temporary database, compare, drop it
	if a.simulate {
		rec.note("restore", "simulation: the database restore is not run")
		rec.note("checks", "simulation")
		return nil
	}
	if err := rec.step("restore", func() (string, error) {
		t0 := time.Now()
		if err := a.restoreInto(ctx, dump, restoreTestDB); err != nil {
			return "", err
		}
		return fmt.Sprintf("into the temporary database %s in %.0f s", restoreTestDB, time.Since(t0).Seconds()), nil
	}); err != nil {
		return err
	}
	defer func() {
		_ = rec.step("cleanup", func() (string, error) {
			_, err := a.psql(context.Background(), "postgres", `DROP DATABASE IF EXISTS "`+restoreTestDB+`" WITH (FORCE)`)
			return "temporary database dropped", err
		})
	}()
	return rec.step("checks", func() (string, error) {
		got, err := a.dbChecks(ctx, restoreTestDB)
		if err != nil {
			return "", err
		}
		return compareChecks(man, got)
	})
}

// compareChecks compares the restored database with the manifest: the schema (migrations, tables) must be equal;
// the row counts are reported, and an empty table that had rows is a failure.
func compareChecks(man *backupManifest, got map[string]string) (string, error) {
	summary := fmt.Sprintf("%s migrations (last %s), %s tables, %s organizations, %s users, %s audit entries",
		got["migrations"], got["last_migration"], got["tables"], got["organizations"], got["users"], got["audit_entries"])
	if man == nil || man.DB == nil {
		if n, _ := strconv.Atoi(got["migrations"]); n == 0 {
			return "", errors.New("the restored database has no schema: " + summary)
		}
		return summary, nil
	}
	var bad []string
	for _, k := range []string{"migrations", "last_migration", "tables"} {
		if got[k] != man.DB[k] {
			bad = append(bad, fmt.Sprintf("%s %s instead of %s", k, got[k], man.DB[k]))
		}
	}
	for _, k := range []string{"organizations", "users", "audit_entries"} {
		want, _ := strconv.Atoi(man.DB[k])
		have, _ := strconv.Atoi(got[k])
		if want > 0 && have == 0 {
			bad = append(bad, fmt.Sprintf("%s empty (the backup recorded %d)", k, want))
		}
	}
	if len(bad) > 0 {
		return "", errors.New("the restored database differs from the backup: " + strings.Join(bad, "; "))
	}
	return summary + " — as recorded in the backup", nil
}

func download(ctx context.Context, tg offsite.Target, name, dst string) error {
	rc, err := tg.Get(ctx, name)
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func roundAge(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h", int(d.Hours()))
	}
	return fmt.Sprintf("%d days", int(d.Hours()/24))
}

// ---- handlers ----------------------------------------------------------------------------------------------------

func (a *Server) hRestoreTestSchedule(w http.ResponseWriter, r *http.Request) {
	var in RestoreTestRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request")
		return
	}
	if in.Day < 1 || in.Day > 28 || in.Hour < 0 || in.Hour > 23 || in.Minute < 0 || in.Minute > 59 {
		writeErr(w, 400, "day 1–28, time HH:MM")
		return
	}
	if in.Source != "" && in.Source != "offsite" && in.Source != "local" {
		writeErr(w, 400, "source: offsite or local")
		return
	}
	if err := a.backupUpdate(func(s *backupStored) {
		s.RestoreTest.Enabled, s.RestoreTest.Day, s.RestoreTest.Hour, s.RestoreTest.Minute, s.RestoreTest.Source = in.Enabled, in.Day, in.Hour, in.Minute, in.Source
	}); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, a.backupState(r.Context()))
}

// hRunRestoreTest starts a restore test now (in the background).
func (a *Server) hRunRestoreTest(w http.ResponseWriter, r *http.Request) {
	var in RunRequest
	_ = decode(r, &in)
	if in.Source != "" && in.Source != "offsite" && in.Source != "local" {
		writeErr(w, 400, "source: offsite or local")
		return
	}
	if in.Source == "offsite" && !a.backupLoad().Offsite.Enabled {
		writeErr(w, 409, "no offsite target is enabled")
		return
	}
	if !restoreRunning.CompareAndSwap(false, true) {
		writeErr(w, 409, "a restore test is already running")
		return
	}
	go func() {
		defer restoreRunning.Store(false)
		backupMu.Lock()
		defer backupMu.Unlock()
		a.runRestoreTest(context.Background(), "manual", in.Actor, in.Source)
	}()
	writeJSON(w, 202, a.backupState(r.Context()))
}

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/offsite"
)

// Offsite copies (URS K-01): after every backup the encrypted bundle and its checksum file go to the target and the
// bundle is read back and hashed — a copy counts only when it matches. Unencrypted bundles never leave the server.
// The retention rule applies at the target too, but only to files this server uploaded (Uploaded), so several
// servers may share a folder or a bucket prefix without deleting each other's backups.

const offsiteTimeout = 6 * time.Hour

// backupOffsite copies the new backup, then any local encrypted backup the target misses, and prunes the target.
func (a *Server) backupOffsite(ctx context.Context, rec *runRec, st backupStored, info BackupInfo) error {
	ctx, cancel := context.WithTimeout(ctx, offsiteTimeout)
	defer cancel()
	rec.run.Target = st.Offsite.Describe()
	if !info.Encrypted {
		return rec.step("upload", func() (string, error) {
			return "", errors.New("not sent: only encrypted backups leave the server — set a passphrase in the schedule")
		})
	}
	var tg offsite.Target
	if err := rec.step("upload", func() (string, error) {
		var err error
		if tg, err = offsite.Open(ctx, st.Offsite); err != nil {
			return "", err
		}
		return a.offsitePut(ctx, tg, info.Name)
	}); err != nil {
		a.offsiteListing(ctx, nil, err)
		return err
	}
	defer tg.Close()
	if err := rec.step("verify", func() (string, error) { return offsiteVerify(ctx, tg, info.Name, info.SHA256) }); err != nil {
		a.offsiteListing(ctx, tg, nil)
		return err
	}
	_ = a.backupUpdate(func(s *backupStored) { s.Uploaded = addName(s.Uploaded, info.Name) })
	return a.offsiteCatchUp(ctx, rec, tg)
}

// offsiteCatchUp uploads the local encrypted backups the target misses (earlier failures), prunes the target and
// refreshes the listing.
func (a *Server) offsiteCatchUp(ctx context.Context, rec *runRec, tg offsite.Target) error {
	remote, err := tg.List(ctx)
	if err != nil {
		a.offsiteListing(ctx, nil, err)
		return rec.step("sync", func() (string, error) { return "", err })
	}
	have := map[string]bool{}
	for _, o := range remote {
		have[o.Name] = true
	}
	var missing []BackupInfo
	for _, f := range a.localBackups(nil) {
		if f.Encrypted && f.SHA256 != "" && !have[f.Name] {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		if err := rec.step("sync", func() (string, error) {
			for _, f := range missing {
				if _, err := a.offsitePut(ctx, tg, f.Name); err != nil {
					return "", fmt.Errorf("%s: %w", f.Name, err)
				}
				if _, err := offsiteVerify(ctx, tg, f.Name, f.SHA256); err != nil {
					return "", fmt.Errorf("%s: %w", f.Name, err)
				}
				_ = a.backupUpdate(func(s *backupStored) { s.Uploaded = addName(s.Uploaded, f.Name) })
			}
			return fmt.Sprintf("%d earlier backups copied", len(missing)), nil
		}); err != nil {
			a.offsiteListing(ctx, tg, nil)
			return err
		}
	}
	err = rec.step("prune_offsite", func() (string, error) { return a.offsitePrune(ctx, tg) })
	a.offsiteListing(ctx, tg, nil)
	return err
}

// offsitePut uploads a local bundle and its checksum file.
func (a *Server) offsitePut(ctx context.Context, tg offsite.Target, name string) (string, error) {
	local := filepath.Join(BackupDir, name)
	st, err := os.Stat(local)
	if err != nil {
		return "", err
	}
	t0 := time.Now()
	if err := tg.Put(ctx, name, local); err != nil {
		return "", err
	}
	if _, err := os.Stat(local + ".sha256"); err == nil {
		if err := tg.Put(ctx, name+".sha256", local+".sha256"); err != nil {
			return "", err
		}
	}
	secs := time.Since(t0).Seconds()
	return fmt.Sprintf("%s in %.0f s", humanBytes(st.Size()), secs), nil
}

// offsiteVerify reads the file back from the target and compares its SHA-256.
func offsiteVerify(ctx context.Context, tg offsite.Target, name, want string) (string, error) {
	if want == "" {
		return "", errors.New("no checksum to compare with")
	}
	rc, err := tg.Get(ctx, name)
	if err != nil {
		return "", fmt.Errorf("reading the copy back failed: %w", err)
	}
	defer rc.Close()
	h := sha256.New()
	if _, err := io.Copy(h, rc); err != nil {
		return "", fmt.Errorf("reading the copy back failed: %w", err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return "", fmt.Errorf("the copy differs from the backup (SHA-256 %s… instead of %s…)", got[:12], want[:12])
	}
	return "read back, SHA-256 matches", nil
}

// offsitePrune applies the retention rule to the bundles this server uploaded.
func (a *Server) offsitePrune(ctx context.Context, tg offsite.Target) (string, error) {
	st := a.backupLoad()
	remote, err := tg.List(ctx)
	if err != nil {
		return "", err
	}
	var names []string
	present := map[string]bool{}
	for _, o := range remote {
		present[o.Name] = true
		if slices.Contains(st.Uploaded, o.Name) && !strings.HasSuffix(o.Name, ".sha256") {
			names = append(names, o.Name)
		}
	}
	sort.Strings(names)
	times := make([]time.Time, len(names))
	for i, n := range names {
		times[i], _ = backupTime(n)
	}
	keep := gfsKeep(times, st.Keep, st.KeepWeekly, st.KeepMonthly)
	deleted := 0
	for i, n := range names {
		if keep[i] {
			continue
		}
		if err := tg.Delete(ctx, n); err != nil {
			return "", fmt.Errorf("deleting %s at the target failed: %w", n, err)
		}
		_ = tg.Delete(ctx, n+".sha256")
		delete(present, n)
		deleted++
	}
	_ = a.backupUpdate(func(s *backupStored) {
		s.Uploaded = slices.DeleteFunc(s.Uploaded, func(n string) bool { return !present[n] })
	})
	return fmt.Sprintf("%d kept, %d deleted at the target (only backups of this server)", len(names)-deleted, deleted), nil
}

// offsiteListing stores the target's listing for the Backups page (tg nil: only the error).
func (a *Server) offsiteListing(ctx context.Context, tg offsite.Target, listErr error) {
	var files []OffsiteObject
	if tg != nil && listErr == nil {
		remote, err := tg.List(ctx)
		listErr = err
		for _, o := range remote {
			if !strings.HasSuffix(o.Name, ".sha256") {
				files = append(files, OffsiteObject{Name: o.Name, Size: o.Size, Modified: o.Modified})
			}
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Name > files[j].Name })
	}
	_ = a.backupUpdate(func(s *backupStored) {
		s.OffsiteListedAt = time.Now()
		if listErr != nil {
			s.OffsiteListError = listErr.Error()
			return
		}
		s.OffsiteListError, s.OffsiteFiles = "", files
	})
}

func addName(list []string, n string) []string {
	if slices.Contains(list, n) {
		return list
	}
	return append(list, n)
}

// ---- handlers ----------------------------------------------------------------------------------------------------

// testTarget writes a probe file, reads it back and deletes it.
func testTarget(ctx context.Context, c offsite.Config) OffsiteTestResult {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	tg, err := offsite.Open(ctx, c)
	if err != nil {
		var hk *offsite.ErrHostKey
		if errors.As(err, &hk) {
			return OffsiteTestResult{Detail: err.Error(), HostKey: hk.Seen}
		}
		return OffsiteTestResult{Detail: err.Error()}
	}
	defer tg.Close()
	const probe = "ctrl32-19700101-000000.tar.gz"
	tmp, err := os.CreateTemp("", "ctrl32-probe-")
	if err != nil {
		return OffsiteTestResult{Detail: err.Error()}
	}
	defer os.Remove(tmp.Name())
	payload := "ctrl32 telemetry: test of the offsite target " + time.Now().UTC().Format(time.RFC3339) + "\n"
	_, _ = tmp.WriteString(payload)
	tmp.Close()
	if err := tg.Put(ctx, probe, tmp.Name()); err != nil {
		return OffsiteTestResult{Detail: "writing failed: " + err.Error()}
	}
	defer tg.Delete(context.Background(), probe)
	rc, err := tg.Get(ctx, probe)
	if err != nil {
		return OffsiteTestResult{Detail: "reading back failed: " + err.Error()}
	}
	back, _ := io.ReadAll(io.LimitReader(rc, 4096))
	rc.Close()
	if string(back) != payload {
		return OffsiteTestResult{Detail: "the file read back differs from the one written"}
	}
	if err := tg.Delete(ctx, probe); err != nil {
		return OffsiteTestResult{Detail: "deleting the test file failed (the retention needs it): " + err.Error()}
	}
	return OffsiteTestResult{OK: true, Detail: "written, read back and deleted: " + c.Describe()}
}

func (a *Server) hOffsiteTest(w http.ResponseWriter, r *http.Request) {
	var in OffsiteRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request")
		return
	}
	c := in.Config.Merge(a.backupLoad().Offsite)
	if err := c.Validate(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, testTarget(r.Context(), c))
}

func (a *Server) hSetOffsite(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var in OffsiteRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad request")
		return
	}
	st := a.backupLoad()
	c := in.Config
	if c.Kind == "" { // remove the target
		err := a.backupUpdate(func(s *backupStored) {
			s.Offsite, s.Uploaded, s.OffsiteFiles, s.OffsiteListError = offsite.Config{}, nil, nil, ""
		})
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		a.log.Info("offsite target removed")
		writeJSON(w, 200, a.backupState(r.Context()))
		return
	}
	c = c.Merge(st.Offsite)
	if err := c.Validate(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if c.Enabled {
		if st.Passphrase == "" {
			writeErr(w, 409, "only encrypted backups leave the server: set a passphrase in the schedule first")
			return
		}
		if c.Kind == "sftp" && c.HostKey == "" {
			writeErr(w, 400, "sftp: pin the server's host key first (Test shows it)")
			return
		}
		if res := testTarget(r.Context(), c); !res.OK {
			writeErr(w, 400, "the target does not work: "+res.Detail)
			return
		}
	}
	if err := a.backupUpdate(func(s *backupStored) {
		if s.Offsite.Kind != c.Kind || s.Offsite.Describe() != c.Describe() {
			s.Uploaded, s.OffsiteFiles = nil, nil // another place: nothing there is ours yet
		}
		s.Offsite = c
	}); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a.log.Info("offsite target saved", "target", c.Describe(), "enabled", c.Enabled)
	writeJSON(w, 200, a.backupState(r.Context()))
}

// hOffsiteSync copies the missing backups to the target and applies the retention there (in the background).
func (a *Server) hOffsiteSync(w http.ResponseWriter, r *http.Request) {
	var in RunRequest
	_ = decode(r, &in)
	st := a.backupLoad()
	if !st.Offsite.Enabled {
		writeErr(w, 409, "no offsite target is enabled")
		return
	}
	if !backupRunning.CompareAndSwap(false, true) {
		writeErr(w, 409, "a backup is already running")
		return
	}
	go func(actor string) {
		defer backupRunning.Store(false)
		backupMu.Lock()
		defer backupMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), offsiteTimeout)
		defer cancel()
		rec := a.newRun("offsite_sync", "manual", actor)
		rec.run.Target = st.Offsite.Describe()
		var tg offsite.Target
		err := rec.step("connect", func() (string, error) {
			var err error
			tg, err = offsite.Open(ctx, st.Offsite)
			return st.Offsite.Describe(), err
		})
		if err == nil {
			err = a.offsiteCatchUp(ctx, rec, tg)
			tg.Close()
		} else {
			a.offsiteListing(ctx, nil, err)
		}
		rec.finish(err)
	}(in.Actor)
	writeJSON(w, 202, a.backupState(r.Context()))
}

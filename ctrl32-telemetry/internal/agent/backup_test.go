package agent

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/offsite"
)

func stepNames(r BackupRun) string {
	var n []string
	for _, s := range r.Steps {
		mark := ""
		if !s.OK {
			mark = "!"
		}
		n = append(n, s.Name+mark)
	}
	return strings.Join(n, ",")
}

// The whole flow in the agent's simulation (stand-in dump, no database restore): backup with manifest, encryption
// and checksum, offsite copy read back, retention at the target limited to this server's files, restore test from
// the target and locally, and the detection of a damaged copy.
func TestBackupOffsiteRestoreTestFlow(t *testing.T) {
	dir := t.TempDir()
	a := NewServer(filepath.Join(dir, "agent.sock"), filepath.Join(dir, "config.toml"), "0.77.0-test", "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	a.SetSimulation(dir)
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[http]\nlisten = \":8080\"\n"), 0o600)
	target := filepath.Join(dir, "nas")
	os.MkdirAll(target, 0o755)
	// a backup of another server in the same folder: never deleted by our retention
	foreign := filepath.Join(target, "ctrl32-20200101-031500.tar.gz.enc")
	os.WriteFile(foreign, []byte("someone else's backup"), 0o600)

	ctx := context.Background()
	if err := a.backupUpdate(func(s *backupStored) {
		s.Passphrase = "correct horse"
		s.Keep, s.KeepWeekly, s.KeepMonthly = 2, 0, 0
		s.Offsite = offsite.Config{Enabled: true, Kind: "folder", Path: target}
	}); err != nil {
		t.Fatal(err)
	}

	var last BackupRun
	for i := 0; i < 3; i++ {
		last = a.runBackup(ctx, "schedule", "", "correct horse")
		if !last.OK {
			t.Fatalf("backup %d failed: %s (%s)", i, last.Error, stepNames(last))
		}
		time.Sleep(1100 * time.Millisecond) // names have one-second resolution
	}
	if got := stepNames(last); got != "dump,bundle,encrypt,checksum,upload,verify,prune_offsite,prune" {
		t.Fatalf("steps: %s", got)
	}
	st := a.backupState(ctx)
	if len(st.Files) != 2 {
		t.Fatalf("local retention keeps 2: %d", len(st.Files))
	}
	for _, f := range st.Files {
		if !f.Encrypted || f.SHA256 == "" || !f.Offsite {
			t.Fatalf("file %+v: encrypted, with checksum and offsite expected", f)
		}
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal("the other server's backup must stay at the target")
	}
	remote, _ := os.ReadDir(target)
	bundles := 0
	for _, e := range remote {
		if strings.HasSuffix(e.Name(), ".enc") && e.Name() != filepath.Base(foreign) {
			bundles++
		}
	}
	if bundles != 2 {
		t.Fatalf("retention at the target keeps 2 of ours: %d", bundles)
	}

	// restore test from the target and locally
	rt := a.runRestoreTest(ctx, "manual", "tester", "offsite")
	if !rt.OK || rt.Source != "offsite" {
		t.Fatalf("restore test failed: %s (%s)", rt.Error, stepNames(rt))
	}
	if got := stepNames(rt); got != "select,fetch,checksum,manifest,restore,checks" {
		t.Fatalf("restore test steps: %s", got)
	}
	if rt := a.runRestoreTest(ctx, "manual", "", "local"); !rt.OK {
		t.Fatalf("local restore test failed: %s", rt.Error)
	}

	// a damaged copy at the target is detected
	newest := filepath.Join(target, last.File)
	b, _ := os.ReadFile(newest)
	b[len(b)/2] ^= 0xff
	os.WriteFile(newest, b, 0o600)
	rt = a.runRestoreTest(ctx, "schedule", "", "offsite")
	if rt.OK || !strings.Contains(rt.Error, "damaged") || stepNames(rt) != "select,fetch,checksum!" {
		t.Fatalf("damage must fail at the checksum: %s (%s)", rt.Error, stepNames(rt))
	}

	// a wrong passphrase fails at the manifest step (locally the file is intact)
	_ = a.backupUpdate(func(s *backupStored) { s.Passphrase = "wrong" })
	if rt := a.runRestoreTest(ctx, "manual", "", "local"); rt.OK || !strings.Contains(stepNames(rt), "manifest!") {
		t.Fatalf("wrong passphrase: %s (%s)", rt.Error, stepNames(rt))
	}
	_ = a.backupUpdate(func(s *backupStored) { s.Passphrase = "correct horse" })

	// the next backup repairs the target: the damaged file stays (same name), a new one is added and verified
	if r := a.runBackup(ctx, "manual", "admin", "correct horse"); !r.OK {
		t.Fatalf("backup after damage: %s", r.Error)
	}

	// an unencrypted backup never leaves the server
	r := a.runBackup(ctx, "manual", "admin", "")
	if r.OK || !strings.Contains(r.Error, "only encrypted backups") {
		t.Fatalf("unencrypted backup must not be sent: %s (%s)", r.Error, stepNames(r))
	}

	// the history holds every run, newest first, not yet audited
	h := loadBackupHistory(0)
	if len(h) < 9 || h[0].ID != r.ID || h[0].Audited {
		t.Fatalf("history: %d runs, first %s", len(h), h[0].ID)
	}
}

// A bundle damaged inside (a member differs from the manifest) is refused even with a matching checksum file.
func TestExtractBundleManifest(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "db.dump")
	cfg := filepath.Join(dir, "config.toml")
	os.WriteFile(dump, []byte(strings.Repeat("dump ", 100)), 0o600)
	os.WriteFile(cfg, []byte("[db]\n"), 0o600)
	man := &backupManifest{Format: 1, Files: map[string]manifestFile{}}
	bundle := filepath.Join(dir, "b.tar.gz")
	if err := writeBundle(bundle, map[string]string{"db.dump": dump, "config.toml": cfg, "missing.json": filepath.Join(dir, "none")}, "RESTORE.txt", "note", man); err != nil {
		t.Fatal(err)
	}
	if len(man.Files) != 3 { // db.dump, config.toml, RESTORE.txt (missing optional file skipped)
		t.Fatalf("manifest files: %v", man.Files)
	}
	got, err := extractBundle(bundle, filepath.Join(dir, "out.dump"))
	if err != nil || got == nil || len(got.Files) != 3 {
		t.Fatalf("extract: %v %v", got, err)
	}
	// a manifest that lists a member the bundle does not have
	bad := filepath.Join(dir, "bad.tar.gz")
	m2 := &backupManifest{Format: 1, Files: map[string]manifestFile{"config.toml": {Size: 1, SHA256: "x"}}}
	if err := writeBundleWithManifest(bad, dump, m2); err != nil {
		t.Fatal(err)
	}
	if _, err := extractBundle(bad, filepath.Join(dir, "o3")); err == nil || !strings.Contains(err.Error(), "config.toml") {
		t.Fatalf("a member listed in the manifest but missing must be reported: %v", err)
	}
}

// writeBundleWithManifest writes a bundle whose manifest also lists the entries of man that are not in the bundle
// (writeBundle fills in the real members and keeps the rest): a tampered or incomplete bundle.
func writeBundleWithManifest(path, dump string, man *backupManifest) error {
	return writeBundle(path, map[string]string{"db.dump": dump}, "RESTORE.txt", "note", man)
}

func TestCompareChecks(t *testing.T) {
	man := &backupManifest{DB: map[string]string{"migrations": "54", "last_migration": "0054_x.sql", "tables": "100", "organizations": "2", "users": "5", "audit_entries": "900"}}
	ok := map[string]string{"migrations": "54", "last_migration": "0054_x.sql", "tables": "100", "organizations": "2", "users": "4", "audit_entries": "905"}
	if _, err := compareChecks(man, ok); err != nil {
		t.Fatal(err)
	}
	bad := map[string]string{"migrations": "53", "last_migration": "0053_y.sql", "tables": "100", "organizations": "2", "users": "5", "audit_entries": "0"}
	_, err := compareChecks(man, bad)
	if err == nil || !strings.Contains(err.Error(), "migrations 53") || !strings.Contains(err.Error(), "audit_entries empty") {
		t.Fatalf("differences must be reported: %v", err)
	}
	if _, err := compareChecks(nil, map[string]string{"migrations": "0"}); err == nil {
		t.Fatal("a database without schema fails even without a manifest")
	}
}

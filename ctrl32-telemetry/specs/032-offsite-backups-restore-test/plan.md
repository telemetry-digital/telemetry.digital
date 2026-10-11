# Implementation Plan: Offsite backups, retention and the automatic restore test

**Spec**: `spec.md` · **Branch**: `feature/0.77` · **Version**: 0.77.0

## Design

- `internal/offsite` — the targets behind one interface (`Put` from a local file, `Get`, `List`, `Delete`, `Close`),
  accepting only backup file names: `folder.go` (temporary name, fsync, rename), `sftp.go` (`golang.org/x/crypto/ssh`
  + `github.com/pkg/sftp`, host key callback comparing the SHA-256 fingerprint before authentication, POSIX rename),
  `s3.go` (SigV4 signer, ListObjectsV2 with continuation, error bodies shown). `Config` with `Validate`, `Redacted`,
  `Merge` (secrets kept only for the same place) and `Describe`.
- `internal/agent`:
  - `backup.go` — bundle written as a stream with the manifest, `createBackup` with recorded steps, `runBackup`
    (backup → offsite → prune), scheduler for the daily backup and the monthly restore test, `restoreInto` shared by
    the rollback of updates and the restore test, `psql`/`dbChecks` per database mode (Docker, native, Windows).
  - `backup_crypto.go` — the openssl `enc` format in Go (streaming CBC with PKCS#7, PBKDF2-SHA256).
  - `backup_gfs.go` — the retention rule; `backup_offsite.go` — upload, read-back, catch-up, prune of own files,
    listing, handlers; `backup_restoretest.go` — the restore test and its handlers; `backup_history.go` — runs with
    steps and the acknowledgement of audited runs.
  - The simulation serves backups with a stand-in dump and records the restore step as simulated.
- `internal/httpapi/backups.go` — permission, session and step-up for the target, reasons, audit; `auditBackupResults`
  (tick and page load, mutex) writes results and opens or resolves incidents in the first organization.
- `server.html` — the Backups section: state, schedule with GFS, target form per kind with test and pinning, restore
  test, stored backups with checksum and offsite columns, protocol with a dialog of steps.

## Files

New: `internal/offsite/*`, `internal/agent/backup_{crypto,gfs,offsite,restoretest,history}.go` and tests,
`internal/httpapi/backups.go`, `tests/e2e/backups.sh`, `tests/headless/backups.js`, this spec.
Changed: `internal/agent/{backup,types,client,server,update,vpn}.go`, `internal/httpapi/{server,serveradmin,updates,openapi}.go`,
`internal/httpapi/static/server.html`, the 19 catalogs, `tests/run-all.sh`, `go.mod`, URS, threat model, README, JOURNAL.

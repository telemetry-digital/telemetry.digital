# Feature Specification: Offsite backups, retention and the automatic restore test

**Feature Branch**: `feature/0.77`

**Created**: 2026-10-10

**Status**: Implemented (0.77.0)

**Input**: The open mandatory requirements of the URS for backups: K-01 (backup module: schedule with
grandfather–father–son retention, a target away from the server — SMB/NFS/S3/SFTP —, encryption, integrity check),
K-02 (automated restore test into a temporary database by schedule, by default monthly, with a protocol visible in the
UI), K-03 (a failed backup or restore test raises an alarm to the administrator and a security event) and K-04 (the
backup carries the configuration and the application version: a restore gives a consistent state). Until 0.76 the
agent made a daily local bundle (database dump, configuration, proxy and VPN state), optionally encrypted with
openssl, kept the newest N files and nothing else.

**Related**: ADR-0012 (server agent), ADR-0024 (signed releases; the backup before an update), spec 031, threat model,
URS K-01..K-04, K-14.

## Clarifications (defaults)

- Targets: a **folder** (an absolute path: a second disk, a USB disk, an SMB/NFS share the operating system mounted,
  a UNC path on Windows — the agent does not mount shares itself), **SFTP** (password or private key; the host key is
  pinned by its SHA-256 fingerprint and required: without it, or with another key, nothing is sent), **S3-compatible
  storage** (AWS, Wasabi, Backblaze B2, Hetzner, Cloudflare R2, MinIO, Versity …; Signature Version 4 without an SDK;
  path-style or virtual-hosted addresses; https, plain http only to this host or the local network; one PUT up to
  5 GiB).
- **Only encrypted bundles leave the server.** Enabling a target needs a stored passphrase; removing the passphrase
  while a target is enabled is refused; a manual backup without a passphrase stays local and its run reports why.
- Encryption keeps the format of `openssl enc -aes-256-cbc -pbkdf2 -iter 200000 -salt` (bundles stay readable with the
  standard tool and old bundles stay readable), but is done in Go: Windows servers no longer need OpenSSL.
- Integrity: `manifest.json` inside every bundle (format, application version, host, creation time, the database
  figures — number and last name of applied migrations, tables, organizations, users, audit entries — and the size and
  SHA-256 of every member); `<bundle>.sha256` (sha256sum format) next to every bundle, locally and at the target. An
  offsite copy counts only after it was read back from the target and its SHA-256 matched.
- Retention (GFS): the newest *daily* N backups (as before 0.77, "keep N"), plus the newest backup of each of the last
  *weekly* N ISO weeks and *monthly* N calendar months; the newest backup is always kept. Defaults 14 / 4 / 12 (also for
  existing installations, which therefore keep up to 16 more files). The same rule applies at the target, but **only
  to files this server uploaded** (several servers may share a folder or a prefix).
- After a failed copy the next backup copies the missing encrypted bundles again ("sync"); *Send missing backups now*
  does it on demand.
- Restore test: by default **enabled, monthly on day 1 at 04:30**, from the offsite target when one is enabled (the copy
  that would be used after losing the server), otherwise from this server. Steps: newest backup, download, checksum,
  decryption and manifest, `pg_restore` into the temporary database `ctrl32_restoretest` of the same PostgreSQL,
  comparison with the manifest (migrations and tables equal; a table that had rows must not be empty), drop of the
  temporary database. The live database is never touched. Needs free disk space for one more copy of the database.
- Every backup, offsite synchronisation and restore test is a *run* with its steps (the protocol) in
  `backup-history.json` next to `backup.json` (newest 200). The web service writes every finished run into the audit
  trail once (`server.backup_result`, `server.backup_offsite_sync_result`, `server.restore_test_result`).
- K-03: a failed run opens an incident of kind `backup_failed` (*Backup failed*, *Offsite copy of the backup failed*,
  *Restore test failed*) in the first organization of the installation, where the server administrators read the
  server's own audit trail; the incident notifies the administrators; the next successful run of the same kind
  resolves it. No completed backup for more than 26 hours while the schedule is on opens *No completed backup since …*.
  Failures are logged with `security: true`.
- Setting or removing the target sends the server's data elsewhere: only a person in the browser (no API token, no AI
  assistant), with a reason and the password (and authenticator code) again. Secrets are write-only: the state shows
  `has_password` / `has_private_key` / `has_secret_key`; a blank secret keeps the stored one only for the same host and
  user (SFTP) or the same endpoint and access key (S3). The connection test writes, reads back and deletes a probe file
  and is audited.

## User Scenarios & Testing

### User Story 1 — The backups survive the loss of the server (P1)

An administrator sets an SFTP server, an S3 bucket or a mounted share as the target. Every scheduled backup is
encrypted, copied there, read back and compared. The Backups page shows each stored backup with its checksum and
whether it is at the target.

**Acceptance**: (1) a backup with a passphrase ends with the steps dump, bundle, encrypt, checksum, upload, verify,
prune_offsite (and prune when scheduled); (2) the bundle and its `.sha256` are at the target; (3) an unencrypted backup
is not sent and the run says why; (4) an SFTP server with an unpinned or different host key receives nothing and the
test shows its fingerprint to pin; (5) a wrong S3 secret is reported with the server's message.

### User Story 2 — We know the backups can be restored (P1)

Once a month the newest offsite copy is restored into a temporary database and compared with what it recorded; the
protocol shows every step.

**Acceptance**: (1) the run has the steps select, fetch, checksum, manifest, restore, checks, cleanup; (2) a damaged
copy fails at the checksum and opens an incident; (3) the next good restore test resolves it; (4) the live database is
unchanged; (5) the result is in the audit trail.

### User Story 3 — Retention without surprises (P2)

Daily backups for two weeks, weekly for a month, monthly for a year — on the server and at the target — without
deleting another server's files in a shared folder.

### Edge cases

- The target is unreachable during a backup: the local backup exists, the run fails at *upload*, an incident opens,
  the next backup copies the missing ones.
- The passphrase is changed: older bundles at the target are still encrypted with the old one; the restore test uses
  the stored (new) passphrase and fails on an old bundle until a new backup exists — the protocol says *wrong passphrase
  or damaged file*. Keep the old passphrase with the bundles it encrypted.
- Bundles made before 0.77.0 have no manifest and no checksum file: the restore test reports that and checks the
  database only by its schema.

## Requirements

- **FR-001** Targets folder, SFTP (pinned host key) and S3 (SigV4) with validation, write-only secrets and a connection
  test that writes, reads back and deletes a probe file.
- **FR-002** Only encrypted bundles at the target; the copy read back and compared by SHA-256.
- **FR-003** Manifest and checksum file for every bundle; encryption compatible with `openssl enc`.
- **FR-004** GFS retention locally and at the target (only own files at the target).
- **FR-005** Scheduled and manual restore test into a temporary database with a protocol.
- **FR-006** History of runs with steps; audit trail of every result; incidents on failures and on missing backups.
- **FR-007** Target changes only in a browser session with reason and password (and code); audited.
- **FR-008** The Backups page: state, schedule with retention, target, restore test, stored backups, protocol; 19
  languages.

## Success Criteria

- Unit tests: AWS's published SigV4 example, every target against a real server in the test process (S3 with signature
  check, SFTP with host key pinning), encryption both ways against the openssl binary, GFS, the manifest, the whole flow
  in the agent's simulation (backup → offsite → restore test → damage detected → wrong passphrase → unencrypted refused).
- e2e `backups` (agent simulation) and headless `backups` pass.
- Real check: Debian 13, PostgreSQL 18, the agent not simulated, an SFTP server and an S3 server in Docker: backup,
  copy, restore test into PostgreSQL, damage detected.

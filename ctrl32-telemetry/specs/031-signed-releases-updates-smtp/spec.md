# Feature Specification: Signed releases, update modes, offline updates and outgoing e-mail in the UI

**Feature Branch**: `feature/0.76`

**Created**: 2026-10-04

**Status**: Implemented (0.76.0)

**Input**: The owner (4. 10. 2026): signed releases (Ed25519 signature of the release manifest, public keys in the
binary with key rotation, the private key never in the repository, unsigned or badly signed releases refused, a
development-only escape hatch); update modes Manual / Notify (default) / Automatic in a maintenance window with a
backup and an automatic rollback; offline updates (package upload, command line, internal mirror, the detector from
the offline package); the outgoing e-mail (SMTP) set in the user interface.

**Related**: ADR-0024 (decision), ADR-0012 (server agent), ADR-0016 (licence, `updates_until`), ADR-0022 (detector),
spec 026 (licence model), threat model.

## Clarifications (defaults)

- The manifest lists every published file (binaries, SHA256SUMS, VERSION, SBOM, installers, `analytics/*`); the
  signature covers the exact bytes of `release.json`. Several signature lines are allowed (key rotation).
- `SHA256SUMS` stays published for servers before 0.76.0.
- Mode *notify* is the default for new installations and for servers updated to 0.76.0 (no row = defaults).
- The maintenance window is one hour long, weekly, in an IANA time zone (default: the server's local time). Automatic
  installation: patch versions; minor versions only with `include_minor`; major versions never; a version that
  failed is not retried automatically; always with a backup.
- Rollback time default 10 minutes (2–60). "Healthy" = `/healthz` answers `status: ok` with the new version three
  times in a row (5 s apart).
- Rollback after failed migrations: binary + database (from the backup of the same installation). After successful
  migrations: binary only (forward-only migrations).
- Offline package limit 2048 MB (`[agent] max_package_mb`); one staged release at a time.
- SMTP: security `starttls` / `tls` / `none` (only localhost, private or link-local addresses, names without a dot or
  with a local suffix); UI settings win over config.toml unless `[smtp] override = true`.

## User Scenarios & Testing

### User Story 1 — Only our releases are installed (P1)

A system administrator installs an update; a compromised distribution point or mirror cannot make the server install
anything we did not sign.

**Acceptance**: (1) a release signed by a trusted key installs; (2) unsigned, signed by an unknown key, a tampered
manifest, a file with another size or hash, another version than asked for — refused with a clear message, nothing
changed; (3) a release signed by an old and a new key installs on servers trusting either.

### User Story 2 — The server tells me about updates (P1)

In *notify* mode a banner shows a newer version to system administrators and the server administrators get one
e-mail per version.

### User Story 3 — Patch versions install themselves at night (P2)

In *automatic* mode a patch version is installed in the weekly window with a backup; when the new version does not
come up healthy, the previous one returns and the administrators get an e-mail; everything is in the audit trail.

### User Story 4 — A server without the Internet updates (P1)

(a) The administrator uploads the offline package in System → Updates (password again, reason); (b) or runs
`ctrl32-telemetry update --file package.zip` on the server; (c) or points `update_url` at a mirror made with
`ctrl32-telemetry update mirror --to DIR`. The detector installation then works without the Internet.

### User Story 5 — E-mail without editing config.toml (P2)

The system administrator enters the SMTP server, port, security, user, password (write-only), sender and reply-to,
sends a test e-mail and sees the SMTP conversation when it fails.

### Edge Cases

- The new binary does not even start: the guard runs from the previous binary in its own unit and rolls back.
- The agent is down: `update --file` installs in its own process (root).
- A package without the binary of this platform: verified and staged, its detector parts are used; nothing installed.
- The licence's updates ended before the release date: refused (402) before anything changes; automatic mode does not
  try.
- An unencrypted SMTP connection to a public host: refused.

## Requirements

- **FR-001** Signed manifest and trusted keys with rotation; private key outside the repository; publish refuses
  without it.
- **FR-002** The agent verifies signature, version, sizes, hashes and the binary's reported version/date before any
  change; `allow_unsigned_updates` only in development builds.
- **FR-003** Update modes with time-zone-aware window, daily jittered check, banner, e-mail, licence respected.
- **FR-004** Backup before install (always in automatic mode), migration failure → binary + database rollback, health
  guard → binary rollback; history and audit of every result.
- **FR-005** Offline package upload (size limit, step-up, reason, audit including refusals, not via MCP / API tokens),
  CLI install, mirror command, detector parts from package/mirror.
- **FR-006** SMTP settings in the UI, write-only encrypted password, security rules, test with transcript, delivery
  log, audit, precedence over config.toml.

## Success Criteria

- Unit, e2e (agent simulation) and headless tests pass; a real Linux test shows: 0.75 → 0.76 online, signed online
  update, rollback of a release that does not start, rollback of a release whose migrations fail (database restored),
  offline upload, CLI update.

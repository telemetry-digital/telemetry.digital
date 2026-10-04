# Implementation Plan: Signed releases, update modes, offline updates, SMTP in the UI

**Branch**: `feature/0.76` | **Spec**: spec.md | **ADR**: ADR-0024

## Technical context

Go server + privileged agent (ADR-0012), PostgreSQL, plain-JS UI. No new dependencies (Ed25519 from the standard
library, zip from the standard library, go-mail already used).

## Structure

| Part | Where |
|---|---|
| Manifest, signatures, trusted keys, version comparison | `internal/release/manifest.go`, `internal/release/trusted/*.pub` |
| Key generation, signing, packaging, detector parts for the package | `tools/relsign` |
| Installation, sources (HTTP, staged package), backup, migrations, rollback, guard | `internal/agent/update.go`, `backup.go` (restore) |
| Offline package verification and staging, CLI direct install | `internal/agent/package.go` |
| Detector parts from package → mirror → official | `internal/agent/mirror.go`, `internal/analytics/pins.go` (MirrorName) |
| CLI: `update --file`, `update verify`, `update mirror`, `update guard` | `cmd/ctrl32-telemetry/update.go` |
| Modes, scheduler, banner, upload endpoint, result audit | `internal/httpapi/updates.go` |
| SMTP settings, encryption, test with transcript | `internal/httpapi/smtp_settings.go`, `internal/notify/email.go` |
| Storage | migration `0054_server_settings.sql` (`core.server_setting`, notification event `system`) |
| UI | `static/server.html` (sections updates, email), `static/app.js` (banner, menu) |
| Publish | `deploy/publish.sh` |

## Decisions in detail

- Signature line format `ed25519 <key id> <base64>`; key id = file name of the `.pub`.
- Guard: `systemd-run --unit=ctrl32-telemetry-update-guard-<id> --collect <binary>.prev update guard …` (fallback:
  detached process); on Windows a detached process; in the simulation a goroutine.
- The simulation of the agent (`vpn_simulate`) installs into its directory (binary, backups), runs `version` and
  `migrate` of the new binary, never restarts services or restores a database.
- System audit entries go to the first organization (where the server administrators read the trail).

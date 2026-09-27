# Feature Specification: Camera relay as a service

**Feature Branch**: `assistant/jolly-sagan-nzolwc`

**Created**: 2026-09-27

**Status**: Implemented (0.52.0)

**Input**: From the list of what a professional system lacks: the relay (spec 011) ran only by hand; the user: "áno,
poupravuj všetko".

**Related**: spec 011; URS P-23; constitution I (security), II.

## Requirements
- FR-001 `camera-relay -check`: configuration valid, server reachable (TLS certificate trusted), every local camera
  answers with a supported format, the server accepts each secret; exit code 0 only when all pass; secrets and
  camera passwords never printed.
- FR-002 The relay runs under the Windows Service Control Manager and systemd; `service install-relay|start-relay|…`
  registers it (the configuration is validated before installing).
- FR-003 Installers `deploy/install-relay.sh` (systemd, own user, sandbox) and `deploy/install-relay.ps1` (Windows
  service, automatic start, restart on failure): download checked against SHA256SUMS, configuration readable only by
  the service (Linux 0640 root:ctrl32relay; Windows SYSTEM + Administrators), test before the service starts
  (`--force` to override), `--check`, `--update`, `--uninstall [--purge]`.
- FR-004 The dialog with the new camera's secret offers *Download relay.toml* and the installer commands.

## Success Criteria
- SC-001 e2e `video`: self-check passes with the right secret and reports a wrong one without printing it.
- SC-002 The systemd unit passes `systemd-analyze verify`; windows/arm64 builds succeed.

# Feature Specification: Remote management inside ctrl32 telemetry

**Feature Branch**: `feature/0.86.0-remote-management`

**Created**: 2026-10-29

**Status**: Implemented (0.86.0)

**Input**: The owner asked whether the functions of MeshCentral, which we use next to ctrl32 telemetry, can be built
into telemetry "so there need not be two programs", and chose: the main things in telemetry, MeshCentral only as the
emergency remote desktop behind a button in our UI. What we really did through MeshCentral at the BB site: OS upgrades
and reboots of the Ubuntu server, ctrl32 updates (cut by Cloudflare after 100 s when a backup came first), reading
journals, and setting up and looking after the Windows PC behind the camera wall (kiosk user, autologon, one Chrome
kiosk per monitor, screenshots, restarts).

**Related**: ADR-0025 (this decision), ADR-0012 (server agent), ADR-0024 (updates), spec 014 (displays), spec 016
(privacy masks), docs/remote-management.md, docs/displays.md, threat model.

## Clarifications (defaults)

- No shell, no arbitrary command, anywhere. Closed lists of operations; inputs are ids the server generated, numbers in
  bounds and fixed names.
- Operating system: Debian/Ubuntu with apt and systemd. Windows servers: "Linux servers only".
- The OS upgrade runs as its own systemd unit (`systemd-run`, the agent's binary `update os-upgrade`), non-interactive,
  keeping changed configuration files; one job at a time; the log stays (last 12 jobs).
- Reboot: 60 s later by default (30–3600), cancellable; refused while an OS job, a ctrl32 update, a backup or a restore
  test runs, or another reboot is scheduled; the page reconnects by itself.
- Health is read-only: md RAID, physical links, SMART (smartctl when installed, cached 10 min), reboot required.
- Updates: 202 once verified; `wait: true` = the old synchronous answer.
- Display health every 30 s, at most every 10 s per display; shown when younger than 3 minutes.
- Kiosk PC: Windows 10/11; setup script generated per PC with its credential (shown once; the server keeps the hash);
  scripts in `C:\ProgramData\ctrl32-kiosk` (SYSTEM/Administrators write, users read; `user\` for the kiosk user);
  agent polls every 15 s; operations wait 10 minutes; results stored with the PC.
- Screenshots: off per PC; enabling needs `display.screenshot` + reason; each one `display.screenshot` + reason,
  audited, in memory only, fetched once by the requester, gone after 3 minutes.
- MeshCentral: https address and node of the server as a server setting; node per kiosk PC; opening audited with a
  reason; link `/?gotonode=<node>&viewmode=11&hide=16`.

## User Scenarios & Testing

### Story 1 — Patch the server without a shell (P1)

A system administrator opens System → Operating system, sees 12 updates (3 security), clicks *Install updates*, gives a
reason and watches the log. The page then shows *reboot required (linux-image-…)*; *Reboot server* with a reason and a
confirmation schedules the reboot; the page waits and reloads when the server is back.

**Acceptance**: the list matches `apt-get -s full-upgrade`; a second job or a reboot during the job is refused (409
busy); the job finishes when the agent restarts; requests and results are in the audit trail.

### Story 2 — Update through the tunnel (P1)

An update with a backup behind Cloudflare finishes although the backup takes several minutes: the answer comes after
the verification, the page follows the installation in the history.

### Story 3 — Is the wall all right? (P1)

The operator sees on Displays "39/39 playing" for each monitor of the wall, or "38/39 playing · Reštaurácia (picture
stands)".

### Story 4 — Set up and look after the wall PC (P1)

An engineer registers a kiosk PC with 2 monitors, downloads its setup script, runs it on the PC, pairs the two monitors
and links them. Later the PC reports Windows, uptime, load and pending Windows updates; the engineer restarts its
browsers or reboots it with a reason. After a server update *scripts outdated* → *Update scripts*.

### Story 5 — See the wall's screen (P2, sensitive)

An administrator allows screenshots for the PC (reason), takes one (reason), sees it once; it is not stored. Without
`display.screenshot` or with screenshots off the request is refused.

### Story 6 — Emergency (P2)

The server does not respond properly; the administrator opens *Emergency remote desktop* (reason): MeshCentral opens
at the server's node and asks for its own sign-in.

## Requirements

- **FR-001** Agent `GET /os`, `POST /os/check|upgrade|reboot|reboot/cancel|ack`, `GET /logs` as in ADR-0025; API
  `/api/v1/server/os…`, `/api/v1/server/logs`, `system.admin`, reasons, audit.
- **FR-002** Health in the agent status (raid, links, disks, smart, reboot_required, problems).
- **FR-003** Asynchronous update and package installation (202), `wait` for the synchronous answer.
- **FR-004** `POST /api/v1/display/health`; health summary in `GET /api/v1/displays` with names only for cameras the
  user may see.
- **FR-005** Kiosk PCs: CRUD, credential (hash, shown once, replaceable), setup script, scripts download, report,
  closed operations, results, links of displays to monitors (one display per monitor), removal.
- **FR-006** Screenshots under FR-005 with the rules above; permission `display.screenshot` (org_admin).
- **FR-007** MeshCentral settings and audited links for the server and for kiosk PCs.
- **FR-008** UI: System → Operating system, System → Logs, Remote management card, async update progress; Displays:
  health, Kiosk PCs section and dialogs. 18 languages.

## Success Criteria

- The routine operations done through MeshCentral at the BB site in October 2026 are possible in the UI with reasons
  and audit; MeshCentral is opened only through the audited emergency button.
- e2e `remote.sh`, `updates.sh` (async), unit tests of the parsers and the simulated OS flow, a headless probe of the
  new screens.

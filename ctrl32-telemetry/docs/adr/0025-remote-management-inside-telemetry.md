# ADR-0025: Remote management inside ctrl32 telemetry, MeshCentral as the emergency path

**Status:** accepted 29 Oct 2026 (owner: "Mesh, which we use — implement all its functions in our telemetry so there
need not be two programs?"; of the options offered the owner chose "the main things in telemetry, MeshCentral only as
the emergency remote desktop behind a button in our UI"). Extends ADR-0012 (server agent: a closed list of operations,
no shell) and ADR-0024 (updates).

## Context

Since 0.63 the documentation said: ctrl32 telemetry manages its own settings, remote desktop, terminal, files and
operating-system updates are the job of MeshCentral next to it (docs/remote-management.md). In practice, at the BB
camera site, everything routine went through MeshCentral's remote shell: `apt full-upgrade` and reboots of the server,
ctrl32 updates that a Cloudflare tunnel cut after 100 seconds, reading journals, and — for the Windows PC behind the
camera wall — writing PowerShell scripts for the kiosk user, the launcher, screenshots and restarts. Two programs, two
sign-ins, no reasons and no audit trail of ours for any of it, and a shell where a closed operation would do.

Replacing MeshCentral completely (own remote desktop, terminal, file transfer, agent self-update for Windows and
Linux) was rejected: months of work, a large attack surface, an unsigned remote-control binary that antivirus products
flag, and against ADR-0012 and the threat model ("no shell to the server").

## Decision

1. **The routine work moves into ctrl32 telemetry as closed operations.** Nothing takes a command line from a request;
   every operation needs a permission and a reason and is written to the audit trail when it is asked for, and its
   result when it is done (the ack mechanism of updates and backups).
2. **Server agent (Linux, apt), new operations** — `GET /os`, `POST /os/check` (apt-get update + list),
   `POST /os/upgrade`, `POST /os/reboot`, `POST /os/reboot/cancel`, `POST /os/ack`, `GET /logs`:
   - the upgrade runs `apt-get update` and `apt-get full-upgrade` non-interactively (`--force-confdef`,
     `--force-confold`, `NEEDRESTART_MODE=a`, a dpkg lock timeout) in **its own systemd unit** started with
     `systemd-run` (the agent's own binary, `update os-upgrade -dir … -id …`), so it survives restarts of services and
     of the agent; its output is the job's log, its result a file next to it; one job at a time;
   - the reboot is a systemd timer 60 s ahead (30–3600 s), cancellable, refused while a job, an update, a backup or a
     restore test runs;
   - the logs are a closed list (app, agent, caddy, postgresql, wireguard, cloudflared, kernel), at most 2000 lines;
   - the status carries read-only **health**: /proc/mdstat (degraded, rebuilding), physical network links
     (operstate, speed), SMART health when smartctl is installed (cached 10 min), reboot required;
   - Windows servers: the pages say "Linux servers only" (Windows Update maintains Windows).
   - Permission `system.admin`; audit `server.os_check`, `server.os_upgrade`, `server.reboot`, `server.reboot_cancel`,
     results `server.os_job_result`.
3. **Updates no longer depend on the length of a request.** `POST /api/v1/server/update` and the package upload answer
   once the release is verified and staged (202, the installation id); backup, migrations and restart continue in the
   agent. The UI follows the record in the history across the restart. `wait: true` keeps the previous behaviour.
4. **Displays report what they show**: the page posts every 30 s the state of each camera tile (playing, stalled,
   connecting, offline, unsupported), the screen and its version (`POST /api/v1/display/health`, display cookie, only
   cameras the display may show). The Displays list shows "39/39 playing" and names the cameras that are not.
5. **Kiosk PCs** (`core.kiosk_pc`, `display.manage`): a Windows PC behind displays gets a generated setup script with
   its own credential (random token, only its hash stored, shown once) that prepares the PC and installs two
   **readable PowerShell scripts** — no binary to sign: the launcher (one Chrome kiosk per monitor, own profile =
   own display, watchdog, hidden taskbar) at the kiosk user's logon, and the agent as a SYSTEM task. The agent polls
   the server (`POST /api/v1/kiosk/report`, header `X-Kiosk-Token`); the server never connects to the PC. Closed list:
   report, restart the browsers, reboot, update the scripts (from the server's own version), screenshot. The scripts
   directory is writable only by SYSTEM and administrators; the credential file is readable only by them.
6. **Screenshots are sensitive** (they can show camera pictures without privacy masks): off per PC by default; turning
   them on needs `display.screenshot` and a reason; each screenshot needs `display.screenshot` and a reason, is
   audited, is accepted only for the request it answers, and is kept in memory only until the requester fetches it
   once (at most 3 minutes). New permission `display.screenshot`, granted to `org_admin` only.
   **Supplemented 29. 10. 2026 (0.86.5):** the picture comes from each kiosk browser (Chrome DevTools protocol,
   `Page.captureScreenshot`, one per monitor) on a debugging port that Chrome opens on 127.0.0.1 only. The first
   implementation grabbed the desktop in PowerShell, which Microsoft Defender blocks as
   `HackTool:PowerShell/EmpireGetScreenshot.C`; rewording such code to slip past the signature or adding an antivirus
   exclusion on a customer's PC was rejected. Consequence: any local process on the kiosk PC can drive the kiosk browser
   through that port — accepted for a single-purpose PC whose only user is the kiosk account (threat model).
7. **MeshCentral stays the emergency path.** A server setting holds its address and the node of this server; a kiosk PC
   may carry its own node. *Emergency remote desktop* returns a `gotonode=…&viewmode=11` link after a reason and audits
   `remote.mesh_open`; MeshCentral keeps its own sign-in with two factors. Still MeshCentral's: a host that does not boot
   to its services, network repair on the host, BIOS, remote installation on a new PC, anything outside the lists.

## Consequences

- Routine maintenance has one sign-in, one audit trail and reasons; MeshCentral sessions become rare and visible.
- The agent grows by a handful of operations, all deterministic, all refusing to overlap with an update or a backup.
- `apt full-upgrade` can restart Docker (PostgreSQL, Caddy) and services through needrestart; the upgrade runs outside
  the agent, so it completes; the UI warns about short restarts.
- The kiosk agent is a script on a PC the customer owns: a local administrator can change it — like any software on
  that PC. Its credential only lets it report and fetch its own scripts and the operations asked for it.
- Linux kiosks report their tiles as displays; a kiosk agent for Linux is a next step.
- Considered and rejected: a full MeshCentral replacement (above); a generic "run command" operation for the kiosk
  agent (a shell by another name); keeping screenshots on the server (unnecessary copies of possibly unmasked video).

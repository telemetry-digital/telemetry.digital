# Remote management of servers and kiosk PCs

Since 0.86.0 (ADR-0025, spec 034) the everyday remote work happens **inside ctrl32 telemetry**: the web UI and the API
maintain the server (updates of ctrl32 telemetry and of the operating system, reboot, logs, health) and the Windows
PCs behind the displays (kiosk PCs). Every operation needs a permission and a reason and is in the audit trail. There
is still **no shell** and no arbitrary command anywhere: the server agent and the kiosk agent perform closed lists of
operations (ADR-0012, ADR-0025).

**MeshCentral** (open source, Apache-2.0, self-hosted on our server) stays as the **emergency path** — a remote
desktop or terminal when something is broken that the closed lists cannot fix. It is opened from our UI (System →
Server → Remote management, or a kiosk PC on the Displays page); every opening is audited with its reason, and
MeshCentral asks for its own sign-in with two factors.

## What is in ctrl32 telemetry now

| Task | Where | Permission |
|---|---|---|
| Update ctrl32 telemetry (signed releases, backup first, rollback) | System → Updates | `system.admin` |
| List the operating system updates, refresh the package lists | System → Operating system | `system.admin` |
| Install the operating system updates (apt full-upgrade, with its live log) | System → Operating system | `system.admin` |
| Reboot the server (in one minute, cancellable; the page reconnects by itself) | System → Operating system | `system.admin` |
| Health: software RAID, network links, disks (SMART), reboot required | System → Server, System → Operating system | `system.admin` |
| Logs of the service, the agent, Caddy, PostgreSQL, WireGuard, Cloudflare Tunnel, the kernel | System → Logs | `system.admin` |
| What each display shows: camera tiles playing or not, screen, page version | Displays | `display.control` |
| Kiosk PCs: setup script, state, restart the browsers, reboot, update the scripts | Displays → Kiosk PCs | `display.manage` |
| Screenshot of a kiosk PC, one picture per monitor (off per PC until allowed; fetched once, never stored) | Displays → Kiosk PCs | `display.screenshot` |
| Emergency remote desktop (MeshCentral) of the server or a kiosk PC | System → Server, Displays → Kiosk PCs | `system.admin` / `display.manage` |

The operating system pages need a Linux server with apt (Debian, Ubuntu). On a Windows server they say so: install the
Windows updates with Windows Update.

### Updates through a proxy that cuts long requests

Behind Cloudflare a request ends after 100 seconds. Since 0.86.0 `POST /api/v1/server/update` (and the offline
package upload) answers as soon as the release is verified (202 with the installation id); the backup, the migrations
and the restart run in the background and the page follows the record in the history, also across the restart.
`"wait": true` keeps the earlier behaviour for scripts that want the final answer.

### Operating system jobs

- *Check for updates* runs `apt-get update` and lists what a full upgrade would install (security updates marked).
- *Install updates* runs `apt-get update` and `apt-get full-upgrade` without questions — changed configuration files
  are kept (`--force-confold`) — in its own systemd unit (`ctrl32-os-upgrade-<id>`), so it finishes when services, the
  server agent included, restart. The page shows its log; the result goes to the audit trail
  (`server.os_job_result`) and a failure is mailed to the server administrators.
- *Reboot server* schedules a reboot in 60 seconds (a systemd timer, cancellable). It is refused while a job, an update
  of ctrl32 telemetry, a backup or a restore test runs.

## Kiosk PCs

A kiosk PC is the Windows 10/11 computer behind one or more displays (one Chrome kiosk per monitor, each paired as its
own display). On the Displays page:

1. *Add kiosk PC*: name, number of monitors and the address of this server as the PC reaches it (often the local one,
   e.g. `http://10.48.8.30:8080`). The credential of the PC is shown once and is inside the setup script.
2. *Download setup script* and run it on the PC in an elevated PowerShell:
   `powershell -ExecutionPolicy Bypass -File .\setup-kiosk-<name>.ps1` (parameters: `-KioskUser`, `-ResetPassword`,
   `-ProfileDir` to keep the Chrome profiles of an older setup, `-ReplaceTask` to switch an older launcher off).
3. Restart the PC; pair each monitor (*Pair display*) and link the displays to the monitors (*Edit* of the PC).

The setup script prepares the PC: a standard kiosk user with automatic logon (its password is generated on the PC, kept
as an LSA secret and never sent anywhere), power settings (never sleep, no hibernation, no USB/PCIe power saving), no
lock screen, notifications or tips, Windows Update installing and restarting at 03:00, Chrome and Edge policies for
unattended video. It installs two readable PowerShell scripts in `C:\ProgramData\ctrl32-kiosk` (writable only by
SYSTEM and administrators; the kiosk user writes only into `user\`):

- **kiosk-launcher.ps1** (task *ctrl32 kiosk launcher*, at the kiosk user's logon): one Chrome kiosk per monitor with
  its own profile (`profile-0` = the top left monitor), started again within 30 s when it closes; the taskbar is kept
  hidden. Each kiosk gets a DevTools debugging port (`--remote-debugging-port`, 9222 for monitor 1, 9223 for monitor 2,
  …; `cdp_port` in `launcher.json`) for the screenshot. Chrome opens it on 127.0.0.1 only, so nothing outside the PC
  reaches it; any program running on the PC itself could drive the kiosk browser through it, which is accepted for a
  PC that does nothing but show the displays. A kiosk started by older scripts without the port is restarted once.
- **kiosk-agent.ps1** (task *ctrl32 kiosk agent*, SYSTEM): every 15 s it reports the PC (Windows, uptime, CPU, video
  decoding, memory, disk, browser version, kiosk windows, pending Windows updates, a pending restart, the launcher's
  monitors and its log) and performs what an operator asked for — *report now*, *restart the browsers*, *reboot*,
  *update the scripts* (the versions of this server), *screenshot* (since 0.86.5 it asks each kiosk browser for a
  picture of its page over the DevTools port — one picture per monitor, the display page only; a desktop grab in
  PowerShell is blocked by Microsoft Defender as a hacking tool). Nothing else; it never runs text from the server.
  Its credential is in `agent.json`, readable by SYSTEM and administrators only.

The PC connects to the server; nothing connects to the PC. *Replace credential* stops the old one at once; *Remove*
locks the agent out (the displays stay paired).

Linux kiosks (a Raspberry Pi or a mini PC with Chromium) are paired as displays and report their tiles like any
display; a kiosk agent for Linux is a next step.

## MeshCentral (the emergency path)

- server: `https://mesh.telemetry.digital/` (sign-in with two factors; no self-registration)
- on each managed PC: the small MeshCentral agent (a Windows service or a Linux daemon) that connects **out** to the
  server over HTTPS — no open port and no VPN at the customer's site
- remote desktop in the browser, terminal, files; every session is in MeshCentral's event log

In ctrl32 telemetry: System → Server → *Remote management*: the MeshCentral address and the node of this server (from
the device's details in MeshCentral, or `gotonode=` in its address bar). A kiosk PC has its own node (*Edit* of the
PC). *Emergency remote desktop* opens `…/?gotonode=<node>&viewmode=11` in a new tab after asking for a reason
(`remote.mesh_open` in the audit trail).

Still needs MeshCentral: a server or PC that does not boot to its services, network repair on the PC itself, BIOS or
hardware questions, installing ctrl32 telemetry or the kiosk setup on a new PC remotely, and anything outside the
closed lists above.

### Setting up the MeshCentral server (once)

1. DNS: A record `mesh.telemetry.digital` → `2.29.5.17`, on Cloudflare **DNS only** (grey cloud: agents check the
   certificate of the server and must see the server's own one, not Cloudflare's).
2. On the server: `sudo bash deploy/remote-management/meshcentral.sh mesh.telemetry.digital` (needs ctrl32 telemetry
   0.63.1+, whose Caddyfile imports `/opt/ctrl32-telemetry/caddy-sites/*.caddy`).
3. Sign in with `admin` and the password from `/root/meshcentral-admin.txt`, set up two-factor sign-in, and delete the
   file.

### Adding a customer's PC to MeshCentral

1. MeshCentral → *My Devices* → *Add Device Group* (e.g. "Zákazník s.r.o."), type *Manage using a software agent*.
2. *Add Agent* → Windows (or Linux) → download the installer (it carries the server address and the group) → run it on
   the customer's PC as administrator → *Install*.
3. The PC appears in the group; *Desktop*, *Terminal*, *Files* work at once. Set the group's *User Consent* if the
   customer wants to confirm every remote session. Copy the node id into ctrl32 telemetry (server or kiosk PC).

### Security of MeshCentral

- Only accounts created by an administrator; two-factor sign-in required; 10 failed sign-ins in 10 minutes lock the
  address for 30 minutes.
- MeshCentral listens on `127.0.0.1:4430` only; Caddy terminates HTTPS on 443 and MeshCentral announces that
  certificate to the agents (`certUrl`).
- Every remote session is in MeshCentral's event log (who, which PC, when); the opening from ctrl32 telemetry is also in
  its audit trail. Remove a PC from its group when the contract ends; the agent then has no access.
- Updating MeshCentral: `cd /opt/meshcentral && docker compose pull && docker compose up -d`.

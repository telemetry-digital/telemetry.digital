# Remote management of PCs at customers' sites

ctrl32 telemetry manages its own settings remotely (the web UI, the API, the server pages: updates, backups,
restarts), but it **does not control the operating system** of the PC it runs on — its privileged agent performs only
a closed list of operations (ADR-0012). Remote desktop, a terminal, files and operating-system updates are the job of a
separate tool running next to it. We use **MeshCentral** (open source, Apache-2.0), self-hosted on our server:

- server: `https://mesh.telemetry.digital/` (sign-in with two factors; no self-registration)
- on each managed PC: the small MeshCentral agent (a Windows service or a Linux daemon) that connects **out** to the
  server over HTTPS — no open port and no VPN at the customer's site
- remote desktop in the browser, terminal (PowerShell, cmd, bash), file transfer, processes, services, event log,
  Wake-on-LAN; every session is in the server's event log

## Setting up the server (once)

1. DNS: A record `mesh.telemetry.digital` → `2.29.5.17`, on Cloudflare **DNS only** (grey cloud: agents check the
   certificate of the server and must see the server's own one, not Cloudflare's).
2. On the server: `sudo bash deploy/remote-management/meshcentral.sh mesh.telemetry.digital` (needs ctrl32 telemetry
   0.63.1+, whose Caddyfile imports `/opt/ctrl32-telemetry/caddy-sites/*.caddy`).
3. Sign in with `admin` and the password from `/root/meshcentral-admin.txt`, set up two-factor sign-in, and delete the
   file.

## Adding a customer's PC

1. MeshCentral → *My Devices* → *Add Device Group* (e.g. "Zákazník s.r.o."), type *Manage using a software agent*.
2. *Add Agent* → Windows (or Linux) → download the installer (it carries the server address and the group) → run it on
   the customer's PC as administrator → *Install*.
3. The PC appears in the group; *Desktop*, *Terminal*, *Files* work at once. Set the group's *User Consent* if the
   customer wants to confirm every remote session.

## Security

- Only accounts created by an administrator; two-factor sign-in required; 10 failed sign-ins in 10 minutes lock the
  address for 30 minutes.
- MeshCentral listens on `127.0.0.1:4430` only; Caddy terminates HTTPS on 443 and MeshCentral announces that
  certificate to the agents (`certUrl`).
- Every remote session is in the server's event log (who, which PC, when). Remove a PC from its group when the
  contract ends; the agent then has no access.
- Updating MeshCentral: `cd /opt/meshcentral && docker compose pull && docker compose up -d`.

# Installing at a customer's site with a Cloudflare Tunnel

The system runs on a PC at the customer's site (next to the cameras, recordings stay there) and is reachable from
anywhere — the phone app (PWA) included — at an address such as `https://kamery.zakaznik.sk`, **without opening a port,
without a public IP address and without a certificate**. `cloudflared` connects out to Cloudflare; Cloudflare
terminates HTTPS and forwards to the server on `127.0.0.1`. Verified on 29 Sep 2026: sign-in (secure cookie) and live
camera video (WebSocket) on a phone through a tunnel.

Alternative without a server at the site: install only the camera relay (`docs/video.md`, relay) and let the video go
to a central server such as telemetry.digital.

## 1. In Cloudflare (once per customer)

1. The customer's domain is on Cloudflare (free plan is enough), e.g. `zakaznik.sk`.
2. *Zero Trust → Networks → Tunnels → Create a tunnel → Cloudflared*, name e.g. `zakaznik-kamery`.
3. Copy the **token** from the install command Cloudflare shows (the long string after `service install`).
4. *Public hostname*: subdomain `kamery`, domain `zakaznik.sk`, service **HTTP** `localhost:8080`.
5. Optional: *Access → Applications* to put an extra Cloudflare sign-in in front (e.g. only e-mails of the customer).

## 2. On the customer's PC

Windows (elevated PowerShell):

```powershell
Set-ExecutionPolicy -Scope Process Bypass -Force
irm https://telemetry.digital/install.ps1 -OutFile install.ps1
.\install.ps1 -Domain kamery.zakaznik.sk -CloudflareToken <token> -OrgName "Zákazník s.r.o."
```

Linux (Debian, Ubuntu, Raspberry Pi OS 64-bit):

```bash
curl -fsSL https://telemetry.digital/install.sh | sudo bash -s -- --domain kamery.zakaznik.sk --cloudflare-token <token>
```

The installer sets profile `internet` (MFA for every user), binds the web server to `127.0.0.1:8080`, opens **no**
firewall port and installs `cloudflared` as a service (Windows service *Cloudflared*, systemd unit `cloudflared`). The
one-time password of `admin` is in `admin-bootstrap.txt` in the data directory (`/etc/ctrl32-telemetry` on Linux).

## 3. Cameras and the phone app

1. Sign in at `https://kamery.zakaznik.sk`, set up two-factor sign-in, add the cameras (*Cameras → Camera management*;
   *Find cameras* scans the local network).
2. *Settings → Apps → New app → Cameras* creates a phone app `https://kamery.zakaznik.sk/app/kamery` (locked to the
   cameras, one camera per row). Open it on the phone and install it (Android: Install, iPhone: Share → Add to Home
   Screen), or scan the QR code on the install page.
3. Users of the customer: *Users*, each with their own account and, if needed, camera groups.

## Notes

- MQTT for devices listens on `127.0.0.1` only (profile `internet` allows plain MQTT only on loopback). Devices in the
  customer's LAN that send over MQTT need `mqtt.tls_listen` with a certificate, or an installation in profile
  `intranet` without a tunnel.
- Video through Cloudflare: Cloudflare's terms have limited serving large amounts of video through the free plan in the
  past; check the current terms for the expected number of viewers. A few people watching live cameras is typical use;
  recordings are exported as MP4 downloads.
- Changing the address later: Cloudflare public hostname, then `http.base_url` in `config.toml` and a restart.
- An existing installation is switched to a tunnel by re-running the installer with `-CloudflareToken`; it keeps the
  existing `config.toml` and prints the lines to change (`profile`, `[http] listen`, `base_url`).
- Remote administration of the PC itself (desktop, files, updates of the operating system) is not part of ctrl32
  telemetry; see `docs/remote-management.md`.

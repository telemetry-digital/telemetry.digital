# ctrl32 telemetry

A universal platform for collecting, storing, evaluating and reporting data from devices — "ThingsBoard for one company", growing step by step into a SCADA. One static binary for Linux and Windows servers plus PostgreSQL 18. **Free, and free forever** — only the optional white labeling is paid (99,- EUR one-time, perpetual; ADR-0016). The source code is published once version 1.0 is fully functional (ADR-0002).

Developer: telemetry.digital (Palo Krnáč). Revenue comes from implementation services, not from the system. Whoever deploys the system takes responsibility for the deployment (see `docs/adr/0003-nasadenie.md`).

## Status

Version **0.64.0** (2 Oct 2026; production runs 0.64.0). Development on a PC: `deploy/dev/README.md`. Phases 1–2 of the roadmap (`docs/02-navrh-systemu.md`, chapter 13) are functionally covered; the `Zdroj` column of the URS names the version and the test that fulfil each requirement. The production instance runs at `https://telemetry.digital/` (installed with the public installer, updated from the Server page). Every release has an entry in `JOURNAL.md`.

## What it does

- **Collection:** MQTT broker in the process (TLS, WebSocket, per-device ACL, ThingsBoard-compatible topics and payloads), a device HTTP API (telemetry, status, attributes, commands by long polling), LoRaWAN through ChirpStack, OPC UA client with write, Modbus TCP and RTU-over-TCP with versioned register profiles, device provisioning with tokens, device positions.
- **SCADA:** process pictures drawn in a WinCC-style editor (68 industrial symbols, pipes with flow, dynamics, faceplates, commands with reason, object library with SVG import), dashboards of widgets, or both on one screen.
- **Redundant database:** attach a second PC with a one-time code — one-time copy with a verified safe detach, continuous offsite replica, or hot standby with manual failover and fencing (`docs/runbook-replication.md`).
- **White labeling (paid, 99,- EUR one-time, forever):** your own product name, footer, icon and app name on every page, in e-mails, authenticator apps and PDF reports, unlocked by the ctrl32 licence bought on ctrl32.com (one licence: either the ctrl32 editor or white labeling on one server) and activated with the order number under System → License (30-day licenses during the refund period, renewed automatically, then a perpetual one), or with a license key verified offline; everything else stays free and without a license the fixed ctrl32 footer stays (`docs/whitelabel.md`).
- **Smart home and MQTT provisioning:** zigbee2mqtt, Tasmota, ESPHome, Shelly and any device that announces itself over MQTT are recognized automatically with rooms and entities, are recorded and controlled from a Home page (switches, dimming, blinds, thermostats, locks) with audited commands confirmed by the device; MQTT accounts with private topic namespaces, bridges to existing brokers, values published back to the broker for other systems; Shelly Gen2+/Gen3 and Tasmota also with their own native protocols; colour pickers on light cards; scenes; an energy overview with consumption, production and cost per hour, day and month; MQTT in/out flow nodes; an incident when a bridge stays disconnected; fleets provision themselves with a key and secret, ThingsBoard firmware works unchanged (`docs/smarthome.md`).
- **Cameras:** IP cameras over RTSP (H.264, H.265, MJPEG) or HTTP MJPEG recorded continuously into one-minute MP4 files with retention per camera and a disk limit, live and recorded video in the browser without plug-ins or ffmpeg, a timeline with playback at up to 16× and MP4 export (audited), video walls over several monitors such as 2 × 20 cameras, a camera widget for dashboards, a player with zoom and timeshift, remote sites through an encrypted relay (with PTZ, events and G.711 sound through it), live stream information (codec, size, fps, kbit/s), motion and other ONVIF events without decoding video, recording on events with a pre-roll, PTZ, discovery, camera events in flows, camera groups per user, incidents for cameras that stop recording, tested with 128 cameras (`docs/video.md`).
- **Displays (kiosks):** a PC or TV at a wall paired once with a code shows dashboards, process screens and video wall screens in turn without anyone signed in, enlarges cameras with events and follows operators' commands; it reads only its own content and can be disconnected at any time (`docs/displays.md`).
- **Flows:** Node-RED-style automation built into the server — a closed library of 28 trigger, logic and action nodes wired on a canvas, typed expressions instead of code, test messages with dry run, a debug panel, versions with deploy and four eyes, every action audited with the flow as origin; automation rules convert into flows (`docs/flows.md`).
- **Dashboard widgets:** 47 widget types (values, gauges, charts, tables, controls, clock, Markdown, QR…), each with ThingsBoard-level settings — appearance, per-series options, two axes, time window and aggregation, legend values, conditional formatting, click actions, full screen, and filters the viewer can switch on the widget.
- **Records:** append-only observations with two timestamps, gaps and quality, hash-chained audit trail with `audit verify`, versioned objects (assets, datastreams, screens, rules, profiles) with a reason on every change, CSV exports of audit and access logs, cursor pagination.
- **Evaluation:** alarm rules with escalation policies and delivery records (e-mail, HMAC-signed webhooks, HTTP SMS and voice gateways, Web Push), automation rules "condition → action" with an expression language, commands to devices with four-eyes approval, change requests for limits and assignments, an incident log with impact, cause and CAPA (opened automatically for undelivered alarms and a broken audit chain).
- **Presentation:** dashboards as a versioned screen format (20 widget types, live values over SSE, public read-only links with QR), ISA-101 process mimics on a free canvas, maps with the organization's tiles or a self-hosted PMTiles archive, PDF reports and exports rendered with Typst from block-designed templates.
- **Devices:** FOTA with author-signed images and staged campaigns, remote console as a channel into the firmware, C client library with Arduino and ESP-IDF ports (`ctrl32-telemetry-c`).
- **Operation:** organizations as tenants, roles and permissions, TOTP MFA, self-service registration with approval, security policies, 19 language catalogs editable in the UI, branding and themes, PWA with own installable apps per organization (e.g. a phone app with only the cameras, `docs/pwa.md`). The Server page manages the host through a privileged agent: restart, Caddy with automatic HTTPS, the VPN (WireGuard with access rules), encrypted backups with schedule and restore notes, updates verified by SHA-256 (Linux and Windows).
- **API first:** everything the UI does is `/api/v1` with an OpenAPI 3.1 description (`/api/docs`), a generated TypeScript client and a Bruno collection (`clients/`).
- **At a customer's site:** the installers connect the server to a Cloudflare Tunnel (`-CloudflareToken`, `--cloudflare-token`): reachable at the customer's address without open ports (`docs/cloudflare-tunnel.md`); remote management of the PCs with MeshCentral next to it (`docs/remote-management.md`).
- **Period tables:** a dashboard widget with values per local day, week or month (daily temperature logs, monthly consumption of meters) and export of the same table to Excel, CSV and a PDF report (`docs/period-tables.md`).
- **AI assistants (MCP):** AI assistants (MCP clients) work in the system through `/mcp` — every API operation as tools, the user's permissions, reasons and the audit trail; OAuth 2.1 for custom connectors or an API token; off until an administrator switches it on; reading free, changes through assistants with the paid license (the white-labeling one) (`docs/mcp.md`, ADR-0017).
- **VPN for sites, technicians and devices:** the server as a WireGuard hub (Linux) — technicians reach PLCs and HMIs at sites, a router or Raspberry Pi connects a site's LAN (site-to-site, router texts for Linux, OpenWrt and MikroTik), devices send data through the tunnel; access rules with default deny enforced by an nftables table, configurations shown once behind a password check, time-limited access, key rotation, every change audited; up to 5 peers free, sites, time-limited access and more peers with the license (`docs/vpn.md`, ADR-0018).
- **Operations evidence:** `/healthz`, Prometheus `/metrics` (data completeness, alarm latency, queues, NTP, audit chain), syslog forwarding, an NTP clock check, `security.txt`, and a CycloneDX SBOM with a `govulncheck` result published with every release.

## Install

Linux (Debian/Ubuntu with systemd; PostgreSQL 18 and Caddy run in Docker by default):

```bash
curl -fsSL https://telemetry.digital/install.sh | sudo bash -s -- --domain telemetry.example.com --demo
```

Windows Server / Windows 10+ (elevated PowerShell; bundled PostgreSQL 18, Typst, two services):

```powershell
Set-ExecutionPolicy -Scope Process Bypass -Force
irm https://telemetry.digital/install.ps1 -OutFile install.ps1; .\install.ps1
```

Both installers are idempotent. Binaries and `SHA256SUMS` are at `https://telemetry.digital/dl/`; the Server page offers updates from there. Without `--domain` the server runs in the `intranet` profile (TLS optional); with a public hostname it runs in `internet` (TLS everywhere, MFA for everyone).

## Configuration

One file, `config.toml` (`/etc/ctrl32-telemetry/config.toml` on Linux, `%ProgramData%\ctrl32-telemetry\config.toml` on Windows), documented in `config.example.toml`: `profile`, `[http]`, `[mqtt]`, `[db]`, `[smtp]`, `[log]`, `[gap]`, `[security]` (pepper and secret key that never enter the database), `[report]` (Typst), `[fota]`, `[agent]`, `[map]` (self-hosted PMTiles), `[time]` (NTP check). Every key can be overridden by an environment variable `CTRL32_<SECTION>_<KEY>`.

The CLI: `serve`, `migrate`, `audit verify`, `service install|uninstall|start|stop|restart` (`-agent` for the agent service), `agent`, `dev seed`, `admin bootstrap|reset-password|seed-dashboard|purge` (purge = the audited deletion of a site, device, dashboard or reports), `version`.

## Development

Go 1.27, PostgreSQL 18 (`docker compose -f deploy/dev/docker-compose.yml up -d`, port 5433), Node 18+ for the e2e helpers, Typst for PDF tests, Chrome for the headless probes.

```bash
cp config.example.toml config.toml            # then set db.url / admin_url for the dev database
go run ./cmd/ctrl32-telemetry migrate -config config.toml
go run ./cmd/ctrl32-telemetry dev seed -config config.toml   # demo organization, site, datastreams, a device token
go run ./cmd/ctrl32-telemetry admin bootstrap -config config.toml   # first org_admin "admin" with a one-time password
go run ./cmd/ctrl32-telemetry admin reset-password -username admin -password admin1234 -permanent -allow-weak -config config.toml   # dev credentials the e2e suites use
# e2e suites: USER=admin in the environment (the shell's $USER wins otherwise) and no forced MFA enrolment for the dev org:
#   UPDATE core.organization SET settings = jsonb_set(coalesce(settings,'{}'), '{security}', coalesce(settings->'security','{}') || '{"mfa_optional":true}');
# tests/e2e/iam.sh is the exception: run it first on a fresh database with the one-time password from `admin bootstrap`.
go run ./cmd/ctrl32-telemetry serve -config config.toml      # http://127.0.0.1:8080
go test ./...                                  # unit and integration tests (OpenAPI completeness, generated clients, agent)
bash tests/e2e/assets.sh                       # 26 end-to-end suites against the running server, one at a time
bash tests/run-all.sh                          # everything above on a fresh database (PG_ADMIN_URL, SKIP; also run by CI)
```

Static files under `internal/httpapi/static` are embedded: rebuild before checking a UI change. New user-visible strings are added to all 19 catalogs in `internal/i18n/catalog` (English key = text). After an API change run `go run ./tools/apigen`; the test in `tools/apigen` fails while `clients/` is stale. Releases: `deploy/publish.sh <version> root@host -i key` builds Linux amd64/arm64 and Windows amd64 and publishes them with the installers.

## Documentation

| Path | Content |
|---|---|
| `JOURNAL.md` | Journal: one entry per release, decisions, next steps — the current state of the product |
| `docs/02-navrh-systemu.md` | System design (the original plan; chapter 15 lists which open decisions were resolved and how) |
| `docs/adr/` | Architecture decision records 0001–0016 |
| `docs/smarthome.md` | Smart home and MQTT provisioning: accounts, bridges, provisioning profiles, ThingsBoard API, entities, Home page |
| `docs/whitelabel.md` | Paid white labeling (99,- EUR one-time, perpetual): what changes, buying on ctrl32.com, activation with the order number (lic.ctrl32.com), offline license keys, `tools/licgen issue` and `batch` |
| `docs/flows.md` | Flows: nodes, expressions, governance, rule conversion, operation |
| `docs/video.md` | Cameras: adding cameras, recording and storage, ONVIF events, PTZ, playback and export, video walls on several monitors |
| `docs/displays.md` | Displays (kiosks) at a wall: pairing, content in turn, alarm cameras, operator commands, camera groups |
| `docs/urs/URS-v0.1.md` | User requirements with IDs, priority, GxP relevance, phase, and the version and test that fulfil them |
| `docs/threat-model.md` | Threat model |
| `docs/protocol/device-protocol-v0.1.md` | Device protocol (MQTT/HTTP), normative |
| `docs/integration/device-integration.md` | How any device connects: registration, MQTT and HTTP with tested examples, limits, FOTA, checklist |
| `clients/README.md` | Generated API clients; the reference is served at `/api/docs` |
| `docs/01-technicky-research.md` (+ folder) | Technical research: platforms, licenses, stack, hardware (verified 23 Sep 2026) |
| `docs/00-regulacny-research/` | Regulatory research (Annex 11, GDP, PIC/S, Slovak decrees) + verification log |

Language note: the design documents, the URS and the journal are in Slovak; per ADR-0009 new documents are written in English and the existing ones will be translated before publication. The UI and the API are English first with catalogs for 18 further languages.

## Principles in one paragraph

One process (a single static binary for Windows and Linux) plus PostgreSQL. Append-only records with a hash-chained audit trail, two timestamps on every measurement, versioned decoders, secure defaults, three layers of logs. Connectors for MQTT, HTTP, LoRaWAN, OPC UA and Modbus; FOTA with author-signed firmware; a remote device console that is a channel into the firmware, never a shell into the server; a message viewer; a REST API that the UI itself uses. Device libraries for Arduino, ESP-IDF, STM32 and the ctrl32 editor. English first, other languages via i18n; Tabler icons; third-party components only under permissive (MIT-class) licenses.

## Repositories

- `ctrl32-telemetry` — server (Go), web UI (plain JavaScript until 1.0, see ADR-0013), installers, documentation (this repository)
- `clients/` — generated TypeScript client and Bruno collection of the REST API (see `clients/README.md`); the API reference is served at `/api/docs`
- `ctrl32-telemetry-c` — device client libraries (C core + Arduino and ESP-IDF ports; STM32 planned) and ctrl32 editor integration
- `ctrl32-runtime` — optional real-time acquisition/control layer (C++), phase 3+ (see ADR-0001 and ADR-0008)

## License

ctrl32 telemetry is free of charge, forever. Binaries ship under the Apache License 2.0 with additional conditions: no resale of the software or paid hosting of it, and the product footer (ctrl32 · telemetry, GitHub, license) stays as it is — unless the deployment has a paid white-label license (99,- EUR, one-time, perpetual; `docs/whitelabel.md`). See `LICENSE` or `/license` on any running instance. The source code is published after version 1.0.

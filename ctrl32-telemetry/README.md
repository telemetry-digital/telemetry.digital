# ctrl32 telemetry

A universal platform for collecting, storing, evaluating and reporting data from devices — "ThingsBoard for one company", growing step by step into a SCADA. One static binary for Linux and Windows servers plus PostgreSQL 18. **Free, and free forever.** The source code is published once version 1.0 is fully functional (ADR-0002).

Developer: telemetry.digital (Palo Krnáč). Revenue comes from implementation services, not from the system. Whoever deploys the system takes responsibility for the deployment (see `docs/adr/0003-nasadenie.md`).

## Status

Version **0.37.0** (25 Sep 2026; production runs 0.36.3). Phases 1–2 of the roadmap (`docs/02-navrh-systemu.md`, chapter 13) are functionally covered; the `Zdroj` column of the URS names the version and the test that fulfil each requirement. The production instance runs at `https://telemetry.digital/` (installed with the public installer, updated from the Server page). Every release has an entry in `JOURNAL.md`.

## What it does

- **Collection:** MQTT broker in the process (TLS, WebSocket, per-device ACL, ThingsBoard-compatible topics and payloads), a device HTTP API (telemetry, status, attributes, commands by long polling), LoRaWAN through ChirpStack, OPC UA client with write, Modbus TCP and RTU-over-TCP with versioned register profiles, device provisioning with tokens, device positions.
- **SCADA:** process pictures drawn in a WinCC-style editor (68 industrial symbols, pipes with flow, dynamics, faceplates, commands with reason, object library with SVG import), dashboards of widgets, or both on one screen.
- **Records:** append-only observations with two timestamps, gaps and quality, hash-chained audit trail with `audit verify`, versioned objects (assets, datastreams, screens, rules, profiles) with a reason on every change, CSV exports of audit and access logs, cursor pagination.
- **Evaluation:** alarm rules with escalation policies and delivery records (e-mail, HMAC-signed webhooks, HTTP SMS and voice gateways, Web Push), automation rules "condition → action" with an expression language, commands to devices with four-eyes approval, change requests for limits and assignments, an incident log with impact, cause and CAPA (opened automatically for undelivered alarms and a broken audit chain).
- **Presentation:** dashboards as a versioned screen format (20 widget types, live values over SSE, public read-only links with QR), ISA-101 process mimics on a free canvas, maps with the organization's tiles or a self-hosted PMTiles archive, PDF reports and exports rendered with Typst from block-designed templates.
- **Devices:** FOTA with author-signed images and staged campaigns, remote console as a channel into the firmware, C client library with Arduino and ESP-IDF ports (`ctrl32-telemetry-c`).
- **Operation:** organizations as tenants, roles and permissions, TOTP MFA, self-service registration with approval, security policies, 19 language catalogs editable in the UI, branding and themes, PWA. The Server page manages the host through a privileged agent: restart, Caddy with automatic HTTPS, WireGuard, encrypted backups with schedule and restore notes, updates verified by SHA-256 (Linux and Windows).
- **API first:** everything the UI does is `/api/v1` with an OpenAPI 3.1 description (`/api/docs`), a generated TypeScript client and a Bruno collection (`clients/`).
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
| `docs/adr/` | Architecture decision records 0001–0014 |
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

ctrl32 telemetry is free of charge, forever. Binaries ship under the Apache License 2.0 with additional conditions: no resale of the software or paid hosting of it, and the product footer (ctrl32 · telemetry, GitHub, license) stays as it is. See `LICENSE` or `/license` on any running instance. The source code is published after version 1.0.

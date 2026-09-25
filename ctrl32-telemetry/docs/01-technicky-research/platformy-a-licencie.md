# Platform building-block research for telemetry.digital — verified 23 Sep 2026

> Rešeršný report (EN) vypracovaný agentom na zadanie z 23. 9. 2026. Každý fakt bol overený živým načítaním zdroja v ten deň; položky, ktoré sa nepodarilo overiť, sú označené **UNVERIFIED**. Slovenské zhrnutie a závery sú v `../01-technicky-research.md`.

Method: every fact below was checked via live web fetch/search on 23 Sep 2026 (GitHub/npm registry APIs for versions and SPDX licenses, vendor docs for terms). Items I could not confirm are marked **UNVERIFIED**.

---

## 1. ThingsBoard

| Item | Finding (source) |
|---|---|
| CE version / license | v4.3.1.5, published 2026-09-11 (GitHub API); v4.3.x is the current line, no v4.4/v5 ([release table](https://thingsboard.io/docs/releases/releases-table/v4-3-x/)). Repo license Apache-2.0, Java backend + Angular/TypeScript UI ([repo](https://github.com/thingsboard/thingsboard)). |
| PE pricing (self-managed) | Maker $10/mo (10 devices) → Prototype $39 → Pilot $99 → Startup $299 → Business $499/mo (1,000 devices); extra devices $0.10/device/mo; perpetual from $4,999 one-time; Cloud free (5 devices) → $749/mo ([pricing](https://thingsboard.io/pricing/)). |
| PE license mechanics | "A subscription covers a single ThingsBoard PE instance (server process)"; HA needs ≥2 licenses; perpetual fallback = 1 year of updates ([subscription](https://thingsboard.io/docs/license-server/subscription/), [perpetual](https://thingsboard.io/docs/license-server/perpetual/)). Whether PE must reach license.thingsboard.io / works air-gapped: **UNVERIFIED**. |
| PE EULA | Non-sublicensable, non-transferable; forbids providing "installations of the Software … as a service"; no OEM/reseller/white-label resale clause ([EULA](https://thingsboard.io/products/thingsboard-pe/eula/)). Reselling a white-labelled PE-based product would need a bespoke agreement. |
| Architecture | tb-node (REST, WebSocket, Rule Engine, Actor system), separate transports MQTT/HTTP/CoAP/LwM2M/SNMP, JS executors, Node.js web UI ([MSA reference](https://thingsboard.io/docs/reference/msa/)). Queue options: In-Memory (dev only), Kafka, Confluent Cloud ([Ubuntu guide](https://thingsboard.io/docs/user-guide/install/ubuntu/)). DB: PostgreSQL-only (≤5k msg/s) or PostgreSQL + Cassandra hybrid; PE docker compose defaults to `postgres:18` ([PE docker](https://thingsboard.io/docs/user-guide/install/pe/docker/)). TimescaleDB support status **UNVERIFIED**. |
| Core concepts (CE) | Tenant → Customer → Device/Asset; Entity Views in CE ([entity views](https://thingsboard.io/docs/user-guide/entity-views/)); Attributes = latest value only, Time-series = full history ([telemetry](https://thingsboard.io/docs/user-guide/telemetry/)); rule chains/nodes, dashboards/widgets, alarms, device profiles, OTA all in CE ([OTA](https://thingsboard.io/docs/user-guide/ota-updates/)). ThingsBoard Edge is a separate product ([edge releases](https://github.com/thingsboard/thingsboard-edge/releases)). |
| PE-only | White-labeling, advanced RBAC, entity groups, SSO/OAuth2, secrets storage, LPWAN integrations (LoRaWAN/Sigfox), system integrations (AWS IoT, Azure IoT, Pub/Sub, Kafka), 400+ payload converters, Scheduler, Reporting, CSV/XLS export ([PE page](https://thingsboard.io/products/thingsboard-pe/)). |
| Audit log | Records user actions on entities (create/update/delete, login); **telemetry writes are not audited** ([audit log](https://thingsboard.io/docs/user-guide/audit-log/)). Stored in PostgreSQL; `SQL_TTL_AUDIT_LOGS_SECS` default 0 = keep forever ([thingsboard.yml](https://github.com/thingsboard/thingsboard/blob/master/application/src/main/resources/thingsboard.yml)). No hash chain / signing; DB-level deletion is not prevented. |
| Telemetry mutability | Time-series TTL default 0; when enabled, a cleanup job physically deletes rows ([TTL recipe](https://thingsboard.io/docs/pe/recipes/configure-telemetry-ttl/)). REST API can "delete specific ranges" of history ([telemetry](https://thingsboard.io/docs/user-guide/telemetry/)); a second write with the same (entity,key,ts) overwrites — "last wins" ([issue #330](https://github.com/thingsboard/thingsboard/issues/330)). Storage is not append-only. |
| E-signatures / GxP | No e-signature or reason-for-change feature found; no GxP / 21 CFR Part 11 / GDP positioning ([use cases](https://thingsboard.io/use-cases/)). **UNVERIFIED negative**. |

**What this means for telemetry.digital:** ThingsBoard is a good reference model for the domain (tenant/customer/device/asset, attributes vs telemetry, rule chains, device profiles), but its data layer is the opposite of what GxP needs: telemetry is overwritable and deletable, the audit log covers entity CRUD not measurements, and nothing is tamper-evident. The features you would actually need for pharmacies (white-label, reporting, scheduler, LoRaWAN integration) are all PE, and the PE EULA blocks reselling it as your own product. Conclusion: borrow the concept model, do not build on the codebase.

---

## 2. Open-source alternatives (status Sep 2026)

| Platform | Language / License | Latest | Fit |
|---|---|---|---|
| Magistrala (ex-Mainflux) | Go / Apache-2.0 | v1.0.0, 2026-09-04 ([repo](https://github.com/absmach/magistrala)) | Messaging + identity framework; rules engine and audit logs are Enterprise Edition; no dashboards. Low fit as a base; good reference for Go multi-tenant auth. |
| OpenRemote | Java / AGPL-3.0 | 1.30.1, 2026-09-22 ([API](https://api.github.com/repos/openremote/openremote/releases/latest)) | Complete, with dashboards; AGPL forces source disclosure for hosted SaaS — poor fit for a proprietary product. |
| Kaa | Java / Apache-2.0 (0.x only) | 0.10 frozen; Kaa 1.x "Enterprise" is commercial ([kaaiot](https://www.kaaiot.com/kaa-open-source)) | Dead as OSS. Skip. |
| Thinger.io | server not published | Public repos are client libs, mostly archived ([org](https://github.com/thinger-io)) | Backend effectively closed. Skip. |
| Eclipse Ditto | Java / EPL-2.0 | 3.9.7, 2026-09-01 ([Eclipse](https://projects.eclipse.org/projects/iot.ditto)) | Digital-twin API layer only; over-engineered for loggers. |
| Eclipse Hono | Java (Quarkus) / EPL-2.0 | 2.7.0, 2026-02-16 ([releases](https://github.com/eclipse-hono/hono/releases)) | Needs Kafka/MongoDB; too heavy for single-binary on-prem. |
| FROST-Server | Java / LGPL-3.0-or-later | v2.8.1, 2026-09-06 ([repo](https://github.com/FraunhoferIOSB/FROST-Server)) | OGC SensorThings reference impl; interesting as a *standard data model* (Thing/Datastream/Observation) but PostGIS/Tomcat stack. |
| Node-RED | JS / Apache-2.0 | 5.0.7 ([npm](https://registry.npmjs.org/node-red/latest)) | Fine as a bundled integration engine, not validatable as GxP logic (user-editable flows). |
| EdgeX Foundry | Go / Apache-2.0 | 4.0 "Odesa" LTS | Industrial edge (Modbus/OPC UA/BACnet); not for LoRa loggers. |

**What this means:** none of these gives you an append-only, signed, validatable data core; all would need the same GxP layer rebuilt on top, and the strongest complete ones (OpenRemote) are AGPL. This supports building a purpose-built core and reusing only libraries (brokers, LNS, DB, PDF) rather than a platform.

---

## 3. LoRaWAN network servers

| Item | ChirpStack v4 | The Things Stack (TTS) |
|---|---|---|
| License / language | MIT, Rust ([GitHub API](https://api.github.com/repos/chirpstack/chirpstack)) | Apache-2.0, Go ([GitHub API](https://api.github.com/repos/TheThingsNetwork/lorawan-stack)) |
| Latest | v4.19.2, 2026-09-19 ([releases](https://github.com/chirpstack/chirpstack/releases)); v4.20 in test | v3.36.2, 2026-08-17 |
| Architecture | NS + AS merged into one binary; Gateway Bridge (UDP/Basics Station → MQTT), Concentratord, MQTT Forwarder ([architecture](https://www.chirpstack.io/docs/architecture.html)). Requires MQTT v5 broker with shared subscriptions, PostgreSQL ≥13 (pg_trgm) **or SQLite via the `chirpstack-sqlite` binary**, Redis ≥6.2 ([requirements](https://www.chirpstack.io/docs/chirpstack/requirements.html)) | Microservice stack needing Redis + SQL DB |
| Multi-tenancy | Tenants isolate gateways/devices; per-tenant users ([forum](https://forum.chirpstack.io/t/is-multi-tenant-supported-by-the-architecture/970)) | Full multi-tenancy is Enterprise (**UNVERIFIED** detail) |
| Integrations / API | gRPC API + REST via grpc-gateway, API tokens, event streams ([API](https://www.chirpstack.io/docs/chirpstack/api/index.html)); integrations: MQTT, HTTP, InfluxDB, ThingsBoard, AWS SNS, Azure, GCP, … ([integrations](https://www.chirpstack.io/docs/chirpstack/integrations/index.html)) | Webhooks, MQTT, Storage Integration (paid tiers) |
| Codec | Custom JS decode/encode in embedded QuickJS (ES2020) per device profile ([device profiles](https://www.chirpstack.io/docs/chirpstack/use/device-profiles.html)) | JS payload formatters + device repository |
| Editions | Single edition, no paid tier | Open Source: "limited in features and is not suitable for production" ([TTI concepts](https://www.thethingsindustries.com/docs/concepts/)) |
| Embedded on gateway | ChirpStack Gateway OS (OpenWrt) "full" image runs a full NS+AS on the gateway ([Gateway OS](https://www.chirpstack.io/gateway-os/)); repo license **UNVERIFIED** | Not offered |

**What this means:** ChirpStack is the clear choice: MIT, single Rust binary, SQLite mode, no commercial tier, tenant model that maps 1:1 onto pharmacy-per-tenant. For the warehouse appliance you can bundle ChirpStack (SQLite) + an MQTT broker + Redis next to your own binary (Redis is the one extra process you cannot avoid today). Consume uplinks via its MQTT integration or gRPC event stream, keep decoders in your own code (not in QuickJS) so decoding logic is versioned and validated with the product. TTS open-source is explicitly "not suitable for production".

---

## 4. MQTT brokers and licensing

| Broker | Lang / License | Latest | Embed in commercial on-prem product? |
|---|---|---|---|
| EMQX | Erlang / **BSL 1.1 from 5.9.0** (May 2025); reverts to Apache-2.0 after 4 years; 5.8.x and earlier stay Apache-2.0 | 6.3.1, 2026-09-18 ([releases](https://github.com/emqx/emqx/releases)) | No without a commercial license: single node only, "integrating it into a product or solution offered to third parties" needs a commercial license ([license FAQ](https://www.emqx.com/en/content/license-faq)) |
| Mosquitto | C / EPL-2.0 OR EDL-1.0 dual ([LICENSE](https://github.com/eclipse-mosquitto/mosquitto/blob/master/LICENSE.txt)) | v2.1.2, 2026-02-09 ([changelog](https://mosquitto.org/ChangeLog.txt)) | Yes (choose EDL). MQTT 5 + shared subscriptions → satisfies ChirpStack. |
| NanoMQ | C / MIT | 0.25.6, 2026-08-19 ([releases](https://github.com/nanomq/nanomq/releases)) | Yes; EMQ-backed but MIT. |
| HiveMQ CE | Java / Apache-2.0 | 2026.5, 2026-05-27 ([releases](https://github.com/hivemq/hivemq-community-edition/releases)) | Yes, but JVM footprint. |
| mochi-mqtt | Go / MIT | v2.7.9, 2025-03-01; repo pushed Sep 2026 ([releases](https://github.com/mochi-mqtt/server/releases)) | Yes — in-process broker, MQTT v5, hooks for auth/persistence; passes Paho interop tests. Release cadence slow. |
| rumqttd | Rust / Apache-2.0 | 0.20.0, 2025-09-29 | Yes, but lowest activity. |

**What this means:** Avoid EMQX ≥5.9 for anything you ship. Two safe patterns: (a) Go backend with mochi-mqtt embedded in the single binary (no broker process at all), or (b) Mosquitto 2.1 as a sidecar (mature, tiny, EDL).

---

## 5. Time-series storage

| Option | License | Latest | Notes |
|---|---|---|---|
| PostgreSQL 18 | PostgreSQL License | 18.6 (EOL 2030-11-14); PG 19 planned Sep 2026, still beta on 23 Sep 2026 ([versioning](https://www.postgresql.org/support/versioning/)) | PG 18 improves planning for many partitions, partitionwise joins, no locks on pruned partitions ([18.0 notes](https://www.postgresql.org/docs/release/18.0/)) |
| pg_partman | PostgreSQL License | v5.5.0, 2026-07-22, PG 18 supported ([releases](https://github.com/pgpartman/pg_partman/releases)) | Automates time-based partitions + retention (detach, not silent delete) |
| TimescaleDB | Apache-2 edition = hypertables + basic hyperfunctions; **TSL** edition = compression, continuous aggregates, retention policies, jobs ([editions](https://www.tigerdata.com/docs/get-started/choose-your-path/timescaledb-editions)) | 2.30.1, 2026-09-17, PG 16–18 | TSL permits embedding in "Value Added Products", but customers must be "prohibited … from modifying the database schema", and DBaaS exposure is prohibited ([TSL](https://www.tigerdata.com/legal/licenses)). Apache-2 edition lacks compression. |
| QuestDB | Apache-2.0 | 10.0.1, 2026-08-24 | Separate DB to operate; no advantage at your volumes |
| InfluxDB 3 Core | MIT/Apache-2 | — | Core limits queries to ~72 h of data — history needs Enterprise ([docs](https://docs.influxdata.com/influxdb3/core/get-started/query/)). Unusable for multi-year retention. |
| VictoriaMetrics | Apache-2.0 | v1.152.0, 2026-09-14 | Metrics engine; no ACID row semantics for GxP records |

**What this means:** Your volume (10^8–10^9 rows/yr ≈ 3–30 rows/s) is small for PostgreSQL. A `measurements` table range-partitioned by month (pg_partman), `(sensor_id, ts)` btree/BRIN, and no UPDATE/DELETE grants gives append-only semantics with native tooling and zero license risk. Keep TimescaleDB out of the shipped product unless you accept TSL's schema-lock clause. Skip Influx 3 Core outright.

---

## 6. Immutable audit trail + PDF reports

| Building block | License / status | Fit |
|---|---|---|
| immudb | **BSL 1.1** for v1.9DOM.2+; converts to Apache-2.0 after 4 years ([LICENSE](https://raw.githubusercontent.com/codenotary/immudb/master/LICENSE)); v1.11.2, 2026-09-03 | Embedding in a shipped appliance is arguably an "embedded basis" — legally grey; a second DB to validate. Not recommended. |
| PostgreSQL techniques | `REVOKE UPDATE, DELETE, TRUNCATE` from app role; BEFORE UPDATE/DELETE row triggers **plus a BEFORE TRUNCATE statement trigger**; per-row `hash = sha256(payload ‖ prev_hash)` in a trigger; anchor periodic head hashes externally ([AppMaster](https://appmaster.io/blog/tamper-evident-audit-trails-postgresql)) | Recommended core. Cheap, validatable, no extra components. |
| pgaudit | PostgreSQL License; 18.0 for PG 18 ([site](https://www.pgaudit.org/)) | Infrastructure-level audit (who ran what on the DB), not the business audit trail. |
| Tessera / Trillian | Apache-2.0, Go; Trillian in maintenance, Tessera successor ([Tessera](https://pkg.go.dev/github.com/transparency-dev/tessera)) | Merkle-tree log with proofs; heavier than needed. |
| Typst | Apache-2.0; 0.15.1, 2026-07-17 ([releases](https://github.com/typst/typst/releases)); `typst-pdf` exports PDF/A-1b…A-4 and PDF/UA-1 ([typst-pdf](https://crates.io/crates/typst-pdf)) | Best fit; for Go/.NET call the CLI (single static binary). PDF/A-2b for long-term retention. |
| Gotenberg | MIT; v8.37.0, 2026-09-11; Chromium + LibreOffice | Docker-only; fine hosted, too heavy for the appliance. |
| chromedp / headless Chrome | MIT, Go, active | Bundling Chromium into a validated appliance is a maintenance burden. |
| wkhtmltopdf | Archived Jan 2023, unpatched CVE-2022-35583 | Do not use. |
| QuestPDF (.NET) | Community MIT only under USD 1M annual revenue, not for public-sector ([Community License](https://www.questpdf.com/license/community.html)) | Licence trap with hospital/pharma customers. |

**What this means:** Build the audit trail *in* PostgreSQL: one append-only `audit_events` table and the `measurements` table both hash-chained by trigger, no UPDATE/DELETE/TRUNCATE for the app role, a `verify_chain()` function auditors can run, and daily externally-anchored head hashes. For reports, Typst (Apache-2.0, static binary, PDF/A) is the lowest-risk generator on-prem.

---

## 7. Frontend charting and web components

| Library | License | Latest | Fit |
|---|---|---|---|
| uPlot | MIT | 1.6.32 ([repo](https://github.com/leeoniya/uPlot)) | ~50 KB canvas, built for millions of time-series points; ideal for raw-measurement trend views. |
| Apache ECharts | Apache-2.0 | 6.1.0 | Richer dashboards/heatmaps; heavier bundle. |
| Plotly.js | MIT | 4.1.1 (4.x requires WebGL2) | Big bundle; overkill. |
| Chart.js | MIT | 4.5.1 | Simple KPI charts. |
| Lit | BSD-3-Clause | 3.3.3 ([npm](https://registry.npmjs.org/lit/latest)) | Current 3.x line; minimal breaking changes vs 2.x. |

**What this means:** uPlot + Lit 3 covers the monitoring UI at minimal bundle size and permissive licensing; add ECharts only for the few rich visualisations.

---

## Unverified / open items
- ThingsBoard PE online-license-check and air-gapped behaviour.
- Whether ThingsBoard still supports TimescaleDB as a ts backend.
- ChirpStack Gateway OS repository license.
- The Things Stack Enterprise vs Open Source feature matrix.
- immudb "embedded basis" wording as applied to an on-prem appliance.
- PostgreSQL 19 exact GA date (planned Sep 2026, still in beta on 23 Sep 2026).

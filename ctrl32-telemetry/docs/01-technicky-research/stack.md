# Tech-stack research: GxP telemetry platform (telemetry.digital) — as of 23 Sep 2026

> Rešeršný report (EN) vypracovaný agentom na zadanie z 23. 9. 2026. Každý fakt nesie URL; neoverené položky sú označené **UNVERIFIED**. Slovenské zhrnutie a závery sú v `../01-technicky-research.md`.

Scope: single self-contained binary (Linux + Windows Server), PostgreSQL only, also hosted multi-tenant; 5–10-year validated state; append-only + hash-chained audit; LoRaWAN/MQTT/HTTP ingest; Slovak PDF reports; email/SMS/push/voice.

## 0. Final recommendation (summary)

| Layer | Pick | Why (tied to requirements) |
|---|---|---|
| Backend | **Go 1.27** | Single static binary for linux/windows from one machine, Go 1 compatibility promise, reproducible toolchain, embedded MQTT v5 broker (mochi-mqtt), tiny dependency tree |
| DB | **PostgreSQL 18** (+ pg_partman, pgBackRest, RLS) | Supported to 14 Nov 2030; declarative partitioning; permissive licence allows bundling EDB zip binaries with your installer |
| Frontend | **Lit 3 + TypeScript 7 + Vite 8**, charts **uPlot** (ECharts for non-time-series) | Web-standards base, minimal API churn, MIT/Apache licences |
| PDF | **Typst 0.15** CLI shipped next to the binary; charts via cetz-plot or server-generated SVG | Apache-2.0, bundled Unicode fonts (Slovak diacritics fine), no Chromium dependency |
| Notifications | SMS: GoSMS or BulkGate; voice: Twilio; email: Postmark (deliverability) or SES (cost); push: Web Push/VAPID | EUR pricing to SK numbers, HTTP APIs, all usable from a Go binary via plain HTTPS/SMTP |

Ranking of backends: **1. Go (9/10), 2. .NET 10 LTS (8/10), 3. Rust (7/10)**; then Java/Kotlin (6), Node/Bun (5), Elixir (5).

---

## 1. Backend language / runtime comparison

### 1.1 Platform facts (verified)

| Runtime | Current version | Support policy | Single binary Linux+Windows | Notes |
|---|---|---|---|---|
| Go | 1.27.0 (2026-08-19); 1.26.0 (2026-02-10) | Each major supported until two newer majors (~1 yr); releases every Feb/Aug — https://go.dev/doc/devel/release | Yes, `GOOS=windows go build` from Linux; assets via `//go:embed` — https://pkg.go.dev/embed | Go 1 promise: "programs written to the Go 1 specification will continue to compile and run correctly, unchanged" — https://go.dev/doc/go1compat. Toolchain "perfectly reproducible" since 1.21 — https://go.dev/blog/rebuild. Windows service: https://pkg.go.dev/golang.org/x/sys/windows/svc or kardianos/service — https://github.com/kardianos/service |
| .NET | .NET 10 LTS (2025-11-11, EOS 2028-11-14); .NET 11 STS GA Nov 2026; .NET 8/9 EOS 2026-11-10 | LTS 3 yrs, STS 2 yrs — https://dotnet.microsoft.com/en-us/platform/support/policy/dotnet-core | Yes: `PublishSingleFile` + `SelfContained`; native libs extract to disk — https://learn.microsoft.com/en-us/dotnet/core/deploying/single-file/overview | Windows service via `Microsoft.Extensions.Hosting.WindowsServices` — https://learn.microsoft.com/en-us/dotnet/core/extensions/windows-service |
| Rust | 1.98.1 (2026-09-03), 6-week trains — https://blog.rust-lang.org/releases/latest/ | Editions stable; no LTS | cargo-xwin for Windows MSVC — https://github.com/rust-cross/cargo-xwin | "slow compilation was at the top of the list" of productivity limits — https://blog.rust-lang.org/2025/02/13/2024-State-Of-Rust-Survey-results/ |
| Node.js / Bun | Node 24 LTS (maintenance from 2026-10-20); Node 26 → Active LTS 2026-10-28 — https://github.com/nodejs/release | From Node 27: one major/yr — https://nodejs.org/en/blog/announcements/evolving-the-nodejs-release-schedule | Node SEA still "Experimental" — https://nodejs.org/api/single-executable-applications.html; Bun `bun build --compile` cross-compiles — https://bun.com/docs/bundler/executables | Bun acquired by vendor 2025-12-02, stays MIT — https://bun.com/blog/bun-joins-vendor |
| Java/Kotlin | Java 25 LTS (2025-09-16); Oracle JDK 21 free updates ended Sep 2026 | Spring Boot 4.1 (Jun 2026); OSS support ~13 months per minor — https://spring.io/support-policy/ | Needs bundled JRE (jlink) or GraalVM native image; Oracle detached GraalVM from the Java SE train — https://blogs.oracle.com/java/detaching-graalvm-from-the-java-ecosystem-train | |
| Elixir/Phoenix | Elixir 1.20 (2026-06-03); Phoenix 1.8.0 (2025-08-05) | Community; no LTS | Burrito 1.5 wraps app + ERTS into one exe — https://burrito.hexdocs.pm/readme.html | No first-class Windows-service story |

### 1.2 Library matrix (mature choices; licence in brackets)

| Need | Go | .NET | Rust | Node/Bun | Java/Kotlin | Elixir |
|---|---|---|---|---|---|---|
| PostgreSQL driver | pgx v5.9.2, Apr 2026 (MIT; ≥5.9.0 for CVE-2026-33816) — https://github.com/jackc/pgx | Npgsql 10 + EF Core 10 (LTS to Nov 2028) | sqlx 0.9 (May 2026) | pg / postgres.js (UNVERIFIED) | pgjdbc (UNVERIFIED) | Postgrex |
| Embedded MQTT broker | **mochi-mqtt/server** MQTT v5 + 3.1.1, hooks, TLS/WS, active Sep 2026 (MIT) — https://github.com/mochi-mqtt/server | MQTTnet 5.2.0 | rumqttd | aedes 1.1.1, MQTT 3.1.1 only | HiveMQ CE embedded 2026.5; Moquette | mqttx 0.11.2 (pre-1.0) |
| MQTT client | paho.golang v0.20, MQTT v5 — https://github.com/eclipse-paho/paho.golang | MQTTnet | rumqttc | mqtt.js | Paho Java | tortoise / emqtt |
| TOTP (RFC 6238) | pquerna/otp — https://github.com/pquerna/otp | Otp.NET 1.4.1 | totp-rs 6.0.0 | otplib | java-otp 1.0.0 | NimbleTOTP |
| Argon2id | golang.org/x/crypto/argon2 | Konscious…Argon2 1.3.1 | argon2 (RustCrypto) | argon2 0.45.1 | password4j 1.8.4 | argon2_elixir |
| WebAuthn / passkeys | go-webauthn/webauthn (FIDO2 conformant, updated 2026) — https://github.com/go-webauthn/webauthn | Fido2 4.1.0 | webauthn-rs 0.5.5 | @simplewebauthn/server 14.0.2 | webauthn4j | Wax |
| PDF | go-typst (shells out to Typst CLI, MIT) — https://github.com/Dadido3/go-typst; chromedp; go-pdf/fpdf archived Mar 2025 | QuestPDF (licence limits) | printpdf 0.12.8; `typst` crate | puppeteer/playwright | OpenPDF, JasperReports 7.0.7 | ChromicPDF |
| SMTP | wneessen/go-mail (MIT) — https://github.com/wneessen/go-mail | MailKit 4.18.0 | lettre 0.11.23 | nodemailer | Jakarta Mail | Swoosh |
| Cron / scheduling | go-co-op/gocron v2 (MIT) — https://github.com/go-co-op/gocron | Quartz 4.1.1 | tokio-cron-scheduler (UNVERIFIED) | node-cron | Quartz | Oban |
| i18n | nicksnyder/go-i18n (MIT) — https://github.com/nicksnyder/go-i18n | resx / CultureInfo | fluent-rs (UNVERIFIED) | i18next | ResourceBundle | Gettext |

### 1.3 Verdicts

- **Go — 9/10.** The only runtime where "one static binary for Linux and Windows Server, built reproducibly from one CI box" is the default; no runtime extraction to `%TEMP%`, no JIT, no framework EOL every 2–3 years. Go 1 compatibility means code written today compiles unchanged in 2036. mochi-mqtt gives an in-process MQTT v5 broker, pgx is best-in-class, every security primitive has one obvious library. Weak spot: PDF (use Typst CLI).
- **.NET 10 LTS — 8/10.** Excellent Windows Server citizen, single-file publish, Npgsql/EF Core 10 LTS, MQTTnet broker. Costs: LTS every 2 years → ~3 mandatory runtime migrations over 10 years; single-file with native libs self-extracts to disk; QuestPDF licence trap for public hospitals.
- **Rust — 7/10.** Best correctness story and native Typst embedding; but sqlx 0.8→0.9 broke API in May 2026, compile times, cross-compiling needs cargo-xwin. Good for a later hot-path crate, not for the whole product.
- **Java/Kotlin — 6/10.** Ship a JRE, Spring Boot minors expire after ~13 months, GraalVM future uncertain. Memory footprint hurts small on-prem boxes.
- **TypeScript/Node/Bun — 5/10.** npm dependency churn is the opposite of "low churn"; Node SEA experimental; aedes is MQTT 3.1.1 only. Bun `--compile` is good but Bun is 1.x and changes weekly.
- **Elixir/Phoenix — 5/10.** No mature embeddable broker, Burrito only single-binary path, Windows-service deployment unusual.

---

## 2. PostgreSQL

| Major | First release | EOL (final minor) | Note |
|---|---|---|---|
| 14 | 2021-09-30 | **2026-11-12** | do not target |
| 15 | 2022-10-13 | 2027-11-11 | |
| 16 | 2023-09-14 | 2028-11-09 | |
| 17 | 2024-09-26 | 2029-11-08 | safe fallback |
| 18 | 2025-09-25 | **2030-11-14** | recommended baseline |
| 19 | beta 3 (2026-08-13); GA targeted end Oct 2026 | ~Nov 2031 | not yet |

Source: https://www.postgresql.org/support/versioning/. PG18 brings async I/O, `uuidv7()`, B-tree skip scan, OAuth — https://www.postgresql.org/about/news/postgresql-18-released-3142/.

- **Time series:** declarative range partitioning by month/week managed by **pg_partman** (PostgreSQL License) — https://pgxn.org/dist/pg_partman/. Detaching old partitions instead of `DELETE` preserves the append-only property; BRIN index on `(ts)` per partition.
- **Multi-tenancy (hosted):** RLS — https://www.postgresql.org/docs/current/ddl-rowsecurity.html. Pattern: `tenant_id NOT NULL` on every table, `POLICY ... USING (tenant_id = current_setting('app.current_tenant')::uuid)`, `FORCE ROW LEVEL SECURITY`, `SET LOCAL` per transaction — https://www.crunchydata.com/blog/row-level-security-for-tenants-in-postgres.
- **Logical replication:** built in — https://www.postgresql.org/docs/current/logical-replication.html. Use for hosted read replicas / migration.
- **Backup tooling & licences:** pgBackRest **MIT**, Barman **GPLv3**, WAL-G **Apache-2.0** — https://github.com/cybertec-postgresql/pg_hardstorage/blob/main/docs/explanation/comparison-pgbackrest-walg-barman.md. For on-prem installs, wrap `pg_basebackup` + WAL archiving in the app; recommend pgBackRest for Linux hosts.
- **Windows service:** `pg_ctl register -N name -D datadir -S auto` — https://www.postgresql.org/docs/current/app-pg-ctl.html.
- **Bundling:** PostgreSQL License allows redistribution in commercial products — https://www.postgresql.org/about/licence/. EDB publishes zip archives for Windows x64 for 14.24–18.6 — https://www.enterprisedb.com/download-postgresql-binaries.
- **Encryption at rest:** core PostgreSQL has no TDE; Percona pg_tde needs Percona Server — https://github.com/percona/pg_tde. Practical answer: OS-level full-disk encryption (LUKS / BitLocker), TLS on the wire, `pgcrypto` for secrets, encrypted backups (pgBackRest repo encryption).

---

## 3. Frontend

| Item | Version | Licence | Source |
|---|---|---|---|
| Lit | 3.3.3 (Jul 2026); "very few breaking changes from Lit 2.0" | BSD-3 | https://www.npmjs.com/package/lit |
| Vite | 8.0 (2026-03-12), Rolldown replaces esbuild+Rollup | MIT | https://vite.dev/blog/announcing-vite8 |
| TypeScript | 7.0 (2026-07-08), Go-native compiler | Apache-2.0 | https://devblogs.microsoft.com/typescript/announcing-typescript-7-0/ |
| Svelte | 5 (Oct 2024); SvelteKit 3 RC Sep 2026 | MIT | |
| SolidJS | 2.0 still beta | MIT | |

Charting: **uPlot** 1.6.32 MIT (~50 KB; 166,650 points in 25 ms) — https://github.com/leeoniya/uPlot; ECharts 6.1.0 Apache-2.0; Plotly.js 4.1.1 MIT (big bundle); Chart.js 4.5.1 MIT (KPIs only).

Recommendation: Lit 3 + TS 7 + Vite 8, uPlot for trend/alarm timelines, ECharts only for gauges/heatmaps, MQTT-over-WebSocket straight from mochi-mqtt for live values, no state-management framework. Pin exact versions, vendor the build, keep `npm audit` out of the validated release path (record it as a change-control input).

---

## 4. PDF reports in Slovak with charts

| Option | Status / licence | Verdict |
|---|---|---|
| **Typst** | 0.15.1 (2026-07-17), Apache-2.0 — https://typst.app/open-source/. CLI embeds Libertinus Serif, New Computer Modern, DejaVu Sans Mono; `--font-path` adds your own. Charts: cetz-plot 0.1.4; lilaq — https://lilaq.org/ | **Recommended.** ~30 MB single static binary, deterministic output, full Unicode. Ship the CLI next to your binary and call it via go-typst (MIT) — https://github.com/Dadido3/go-typst. Render the trend chart yourself as SVG and `#image()` it. Note: Typst 0.14.0/0.14.1 had a WASM runtime CVE fixed in 0.14.2. |
| gotenberg 8 | MIT, Docker ~418 MB with Chromium + LibreOffice | Hosted only |
| Headless Chromium | Needs Chromium per host, non-deterministic across versions | Avoid for validated reports |
| wkhtmltopdf | Archived, CVE-2022-35583 CVSS 9.8 | Do not use |
| go-pdf/fpdf | archived 2025-03-04, continues on Codeberg | Trivial labels only |
| QuestPDF (.NET) | Community licence < USD 1M revenue; public-sector ineligible | Licence risk |
| printpdf (Rust) | 0.12.8 MIT | Good if on Rust |

---

## 5. Notification channels from Slovakia

**SMS to Slovak numbers (HTTP API):**

| Provider | Price / SMS | Notes | Source |
|---|---|---|---|
| **GoSMS** (SK/CZ) | from **€0.0145** excl. VAT, no setup/monthly fee | Slovak-language API + support | https://www.gosms.eu/sk/cennik/ |
| **BulkGate** (CZ) | **€0.026** excl. VAT (Orange/Telekom), €0.0291 (4ka/O2) | HTTP, SMPP | https://www.bulkgate.com/en/pricing/sms/sk/slovakia/ |
| Bird (ex-MessageBird) | **$0.065** + carrier fees | | https://bird.com/en/pricing/sms |
| Twilio | **$0.0883** per segment | USD billing; also voice | https://www.twilio.com/en-us/sms/pricing/sk |
| SMSAPI (PL) | UNVERIFIED | | |
| Vonage | UNVERIFIED (403) | | |

**Voice-call alerts:** Twilio Programmable Voice to Slovakia: **$0.1158/min mobile, $0.0298/min landline** — https://www.twilio.com/en-us/voice/pricing/sk. Slovak TTS quality UNVERIFIED.

**Web Push (VAPID):** iOS/iPadOS 16.4+ support Web Push **only for Home-Screen web apps** — https://webkit.org/blog/13878/web-push-for-web-apps-on-ios-and-ipados/. Treat push as convenience; GxP escalation must land on SMS/voice.

**Transactional email:** Postmark $15/mo for 10,000 — https://postmarkapp.com/pricing; Resend free 3,000/mo, Pro $20/mo for 50,000; Amazon SES $0.10 per 1,000 — https://aws.amazon.com/ses/pricing/.

Design note: on-prem customers will usually point you at their own SMTP relay; keep SMTP as the primary transport and make SMS/voice/push HTTP providers pluggable with an escalation ladder stored in PostgreSQL and logged to the audit trail.

---

## 6. Security building blocks

- **TOTP:** RFC 6238 libraries per language in §1.2; store secrets encrypted (pgcrypto/AES-GCM), enforce 30 s step, ±1 window, one-time-use of each code, backup codes hashed with Argon2id.
- **Argon2id parameters (OWASP, current):** `m=19456 KiB, t=2, p=1`; alternatives `m=12288/t=3` or `m=9216/t=4`; use a pepper stored separately — https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html.
- **WebAuthn/passkeys:** go-webauthn (Go), Fido2 4.1 (.NET), webauthn-rs (Rust). For Annex 11 e-signatures, treat a passkey as "something you have" and still require a second factor or the password at signing time.
- **Sessions (OWASP):** server-side enforcement; idle timeout 2–5 min high-value / 15–30 min low-risk; absolute timeout 4–8 h; re-authentication before sensitive operations — https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html. Rotate session ID on login/privilege change; opaque IDs in `HttpOnly; Secure; SameSite=Strict` cookie stored in PostgreSQL.
- **Hash-chained / Merkle audit trail — reference designs:** immudb, Trillian/Tessera, Rekor v2 (tile-based Merkle log, signed checkpoints) — https://blog.sigstore.dev/rekor-v2-ga/. **What to reimplement in-app (~300 lines of Go):** (1) `audit_log` with gapless `seq`, `prev_hash`, `hash = SHA-256(seq || ts || actor || action || canonical-JSON payload || prev_hash)`; (2) app DB role INSERT/SELECT only + trigger raising on UPDATE/DELETE; (3) periodic **checkpoint** = Ed25519 signature over (size, head hash); (4) external anchoring: checkpoint hash into the daily/monthly PDF report, email to QA mailbox, and/or RFC 3161 TSA; (5) `telemetry audit verify` command re-walking the chain, reusable in IQ/OQ protocols.

---

## 7. Ingest notes

ChirpStack 4.18 (May 2026) merged NS+AS; integrations publish device uplinks as JSON over MQTT and gRPC/REST — https://www.chirpstack.io/docs/chirpstack/integrations/mqtt.html. With mochi-mqtt embedded you can either subscribe to ChirpStack's broker or have ChirpStack's MQTT integration point at your binary's broker, plus a plain HTTP integration endpoint. Same broker serves direct MQTT sensors and the browser (MQTT over WebSocket).

## 8. Open items marked UNVERIFIED

SMSAPI and Vonage Slovak rates; systemd hosting package name for .NET; tokio-cron-scheduler/fluent-rs versions; Slovak TTS quality for Twilio/Vonage voice; Chromium download size; exact JDBC version.

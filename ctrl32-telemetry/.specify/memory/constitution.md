<!--
Sync Impact Report
- Version change: 1.0.0 → 1.0.1 (2 Oct 2026, PATCH: Principle VI wording follows the licence model of the ADR-0016
  amendment — the product is free including up to 4 cameras; the licence of the server covers the extras)
- Earlier: template → 1.0.0 (first ratification; principles derived from ADR-0001…0014, URS v0.1, README)
- Principles added: I Record Integrity, II One Binary + PostgreSQL, III Secure by Default, IV API-First Content,
  V Tested End to End, VI Permissive Dependencies Only, VII Plain Web UI, Translated
- Sections added: Technical Constraints, Development Workflow, Governance
- Deferred: ADR-0015 (canvas editors before 1.0) is proposed with the SCADA spec; Principle VII already allows it
  subject to that ADR.
-->
# ctrl32 telemetry Constitution

## Core Principles

### I. Record Integrity (NON-NEGOTIABLE)

Everything that is a record is append-only: raw messages, observations, audit trail, alarm log, access log,
console log, signatures, reports, FOTA states. Every measurement carries `measured_at` (source) and
`received_at` (database clock). Every object that changes the meaning of data or behaviour (screens,
dashboards, rules, flows, profiles, limits, templates, replication settings) is versioned: a change is a new
version with a user and a mandatory reason; the old version stays. The audit trail is hash-chained and
verifiable (`audit verify`); no configuration switch may disable it or any security control. Gaps are visible,
never hidden. Rationale: GxP/ALCOA+ (EU GMP Annex 11, 21 CFR Part 11-style expectations, ADR-0005).

### II. One Binary + PostgreSQL

The server is one static Go binary for Linux (amd64/arm64) and Windows plus PostgreSQL 18 as the only external
dependency. No second runtime (Node.js, JVM, Python) is required to run the product; browser libraries are
vendored files served from the binary. Anything that needs host privileges goes through the closed operation
list of the server agent (ADR-0012), never through arbitrary commands. Rationale: deployable by the operator
alone, on an intranet without the Internet (ADR-0001, ADR-0003).

### III. Secure by Default

Personal accounts only, Argon2id passwords, MFA for administrators (all users in the `internet` profile),
least privilege per permission, CSRF and rate limits on every session endpoint. The platform is never in a
control loop: every write to a process (device, connector register, OPC UA node) is a command with identity,
permission, reason, audit entry and acknowledgement; safety interlocks stay in the PLC/runtime. Stored content
is inert data (validated, whitelisted, escaped when rendered) — it can never carry script or markup. Secrets are
never logged and never leave the server in clear text. Rationale: ADR-0007, threat model.

### IV. API-First Content

There is no UI-only function: every capability is an `/api/v1` endpoint described in OpenAPI 3.1 and the web
UI uses exactly those endpoints. Configuration and presentation are content (JSON documents owned by an
organization, versioned, exportable/importable, validated server-side against a closed schema). Generated
clients (`clients/`) stay current. Rationale: ADR-0010.

### V. Tested End to End

Every requirement has a URS ID and at least one automated test that names it. A change is done when
`go vet`, `go test -race`, the C host tests, the e2e suites and the headless probes pass through
`tests/run-all.sh` on a fresh database. New server logic gets unit tests; new API surface gets an e2e suite;
new UI gets a headless probe that fails on regressions (not only prints). Rationale: validation evidence for
IQ/OQ and regression safety.

### VI. Permissive Dependencies Only

Third-party code is MIT-class (MIT, BSD, ISC, zlib, Apache-2.0, PostgreSQL License); copyleft,
source-available and proprietary components are excluded unless an ADR records an exception. Dependencies are
few, pinned and listed in the SBOM. The product is free (including up to 4 cameras; the licence of a server
covers the extras of ADR-0016); no third-party component that requires a paid licence may be bundled. Rationale:
ADR-0002, ADR-0009, ADR-0016.

### VII. Plain Web UI, Translated

Until 1.0 the UI is plain JavaScript without a build step (ADR-0013). Canvas editors (HMI/SCADA screens, flow
editor) may use one vendored library with a clean licence when an ADR accepts it. Rendering is separated from
data loading, state is plain JSON, and every user-visible string goes through the i18n catalogs (English key;
Slovak and Czech maintained first). The UI works on phones and tablets without horizontal page scroll, in light
and dark themes.

## Technical Constraints

- Go 1.27, PostgreSQL 18 (`uuidv7()`, pgcrypto), Typst for PDF; plain ES2022 in the browser.
- Sizing: one server up to ~10 000 datastreams at 1-minute intervals; higher availability through PostgreSQL
  replication managed by the product, not through a different architecture.
- Profiles `intranet` and `internet`; `internet` refuses plain HTTP/MQTT.
- English is the language of code, API, logs and new documents; Slovak working originals stay until publication.

## Development Workflow

- Spec-driven: `/speckit-specify` → `/speckit-clarify` → `/speckit-plan` → `/speckit-tasks` →
  `/speckit-implement`, one folder per feature under `specs/`. A spec references the URS IDs it adds or changes;
  the URS and `JOURNAL.md` are updated with the release that fulfils it.
- A decision that changes an accepted ADR needs a new ADR that supersedes it (fully or in part).
- Commits are small and green; the branch is pushed only after `tests/run-all.sh` passes.

## Governance

This constitution summarizes the accepted ADRs; where they conflict, the newer ADR wins and this document is
amended in the same change. Amendments bump the version (MAJOR: a principle removed or redefined; MINOR: a
principle or section added; PATCH: wording). Every plan produced by `/speckit-plan` includes a Constitution
Check against principles I–VII and justifies any deviation in its Complexity Tracking table.

**Version**: 1.0.1 | **Ratified**: 2026-09-25 | **Last Amended**: 2026-10-02

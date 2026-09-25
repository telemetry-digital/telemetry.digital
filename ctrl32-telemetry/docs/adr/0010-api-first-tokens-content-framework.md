# ADR-0010: API-first through tokens, a content framework, and PDF exports with editable templates

**Status:** accepted 23 Sep 2026 (user: "exports also as PDF, if possible with a PDF editor … everything should go through tokens, the whole system something like a content management framework")

## Decision

### 1. API-first, every action through a token

- **There is no UI-only functionality.** Every capability of the system is an API endpoint (OpenAPI 3.1, `/api/v1`); the built-in web UI is one client among others and uses exactly the same endpoints.
- **Every call carries an identity token:** user sessions (cookie + CSRF header) for the browser, **API tokens with scopes** for scripts, integrations and third-party UIs, **device tokens/certificates** for devices, **claim codes** for provisioning, **HMAC-signed webhooks** for outbound integrations. Scopes are always a subset of the owner's current permissions; revoking a role shrinks every token derived from it.
- Anonymous access exists only as the explicit `public_read` demo flag, and an invalid credential is never downgraded to anonymous.

### 2. Content framework

Everything that configures behaviour or presentation is **content**: versioned, audited, exportable/importable, owned by an organization, optionally provided as a built-in default (org_id NULL) that an organization can override.

| Content type | Body | Renderer / consumer | Phase |
|---|---|---|---|
| Report template | Typst source + JSON data contract | PDF engine (Typst) | 1 (source editing + preview), 2 (block designer) |
| Dashboard | JSON layout of widgets bound to datastreams | Lit UI | 2 |
| HMI screen | JSON screen format v1 (ADR-0008) | HMI viewer/editor | 2–3 |
| Device profile + decoder | JSON/YAML/JS, hashed | ingest | 1 |
| Alarm/escalation templates | JSON | alarm engine | 1 |
| Application pack | bundle of the above + retention policies + compliance matrix | import/export | 2 |

Common pattern for every content table: `id, org_id (NULL = built-in), kind, name, version, locale, body, checksum, created_by, created_at, valid_to, reason`. A new version is a new row (append-only history, consistent with ADR-0005); "current" = highest version without `valid_to`. Export = the row as JSON; import = a new version with audit. Built-ins ship inside the binary and are seeded/upgraded by migrations.

### 3. PDF exports and the "PDF editor"

- Every export and every report is available as **PDF** in addition to CSV/JSONL, rendered by **Typst** (Apache-2.0, deterministic, PDF/A) from a report template with a documented **data contract** (the JSON the template receives).
- **Editing:** phase 1 = the template *source* is editable through the API (and later the UI) with a **preview endpoint** rendering sample data, versioning and audit; phase 2 = a **block designer** (header/footer, text with variables, table, chart, signature block, page settings) that generates the Typst source, with live preview — this is the "PDF editor". Free-form editing of arbitrary PDFs is out of scope; templates are the unit of design.
- Generated PDFs are records: `report` rows carry the template version, data hash, file hash, generator and time; the file is stored under `data/reports/` and can be re-verified. Regulatory reports print the audit checkpoint hash (ADR-0005).
- Typst runs as a separate static binary shipped next to the server (installer downloads the pinned release with checksum); no Chromium.

## Consequences

- New URS items: G-09 (no UI-only functionality), G-10 (all actors use tokens), J-11 (PDF for every export/report), J-12 (report template as versioned content with preview), J-13 (block designer, phase 2), N-10 (content pattern with import/export).
- Migration 0004 introduces `report_template` and `report`; later content types reuse the same column pattern.
- The installer gains a Typst download step; the dev environment needs the Typst CLI on PATH or `report.typst_path` in the config.

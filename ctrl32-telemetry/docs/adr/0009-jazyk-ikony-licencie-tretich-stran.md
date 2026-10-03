# ADR-0009: English first with i18n, Tabler icons and flags, permissive third-party licenses only

**Status:** **accepted 23 Sep 2026** (user: "everything in English with the option of other languages; we use Tabler icons and Tabler flags; everything third-party is open source MIT" — and, on the open point: "other licenses too, MIT was only a suggestion"). The permissive MIT-class policy below is confirmed; translation of existing Slovak documents is deferred until publication.

## Decision

### 1. Language

- **English is the primary language** of everything the product produces or contains: source code and identifiers, commit messages, API (OpenAPI, field names, error codes), UI default locale, system logs, product documentation, release notes, security advisories.
- **Other languages are translations** delivered through i18n (`go-i18n` on the server, message catalogs in the UI), never hard-coded. First translations: Slovak and Czech (needed for the GxP pack reports and pilot deployments). Regulatory report templates exist per locale (the Slovak pharmacy report is a Slovak template, not a translation).
- Date, number and time-zone formats follow the organization's locale settings, not the UI language.
- Internal design documents written in Slovak so far (`docs/02-navrh-systemu.md`, ADR-0001…0008, URS, threat model, protocol) remain the working originals; **they are translated to English before the repository is published** (ADR-0002), and every new document from now on is written in English.

### 2. Icons and flags

- **Tabler Icons** (MIT) is the only icon set; **Tabler flags** (MIT, from the Tabler UI repository) for country/language flags. No mixing with other icon sets.
- Tabler UI CSS (MIT, Bootstrap 5 based) is the candidate design system for the web UI, used in light DOM alongside Lit web components; confirmed or rejected in the phase 0 PoC.

### 3. Third-party license policy

- Every third-party component (server dependencies, frontend packages, device-library dependencies, bundled binaries such as PostgreSQL and Typst, fonts, icons) must be **open source under a permissive, MIT-class license**: MIT, BSD-2/3-Clause, ISC, zlib, Apache-2.0, PostgreSQL License, Unlicense/CC0.
- **Not allowed:** copyleft (GPL, AGPL, LGPL), source-available/business licenses (BSL, SSPL, Elastic, Commons Clause), proprietary or "free for non-commercial" terms, anything without a license file.
- **Exceptions require a written decision in an ADR.** Known candidate exception: **open62541** (OPC UA stack, MPL-2.0, file-level weak copyleft) for the optional C++ runtime in phase 3+ — acceptable only if we do not modify its files or if we publish modifications; alternative OPC UA server stacks with permissive licenses do not exist at the same maturity.
- **Enforcement:** the CI license check runs on every release (`go-licenses` for Go modules, `license-checker` for npm, manual list for bundled binaries and C libraries); the SBOM (CycloneDX) lists the license of every component; a release with an unapproved license fails. The policy is part of the supplier documentation (URS N-08).

## Resolved point

A literal "MIT only" policy would have excluded Go (BSD-3-Clause), PostgreSQL (PostgreSQL License), TypeScript and Typst (Apache-2.0) and Lit (BSD-3-Clause). The user confirmed on 23 Sep 2026 that other licenses are acceptable and MIT was only a suggestion; the policy is therefore **permissive, MIT-class** as written above, with exceptions only through an ADR.

## Consequences

- All new files, comments and docs in English; Slovak used only in user-facing translations and in regulatory templates.
- Icon and flag assets are vendored (pinned version, MIT license file kept in the repo).
- The license check is a release gate alongside `govulncheck`, SBOM and signing (ADR-0007).

## Addendum (3 Oct 2026): separate programs under LGPL

Approved by the owner on 3 Oct 2026 for object counting (ADR-0022): an LGPL program that runs as a **separate,
unmodified process** — never compiled into, linked with or shipped inside the product binary — is allowed. The first
case is `ffmpeg`, which decodes camera sub-streams for the analytics worker; the administrator installs it (or points
`[analytics]` at an existing copy). Everything compiled into the product still follows the licence list above.

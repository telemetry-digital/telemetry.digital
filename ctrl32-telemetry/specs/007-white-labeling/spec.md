# Feature Specification: Paid white labeling

**Feature Branch**: `main`

**Created**: 2026-09-26

**Status**: Implemented (first 0.43.0 on 26 Sep 2026, reverted the same day; reinstated in 0.57.0 on 27 Sep 2026 as a perpetual license, 99,- EUR one-time, ADR-0016)

**Input**: User description: "Predpríprava white labeling, bude platený" (prepare white labeling; it will be paid).

**Related**: ADR-0016 (amends ADR-0002 and ADR-0011), URS J-16/J-16a, constitution II (one binary, offline), III
(secure by default).

## Clarifications

### Session 2026-09-26

The user did not want to answer questions; these defaults were chosen and can be revisited:

- Q: What does the white label replace? → A: Everything the customer sees: product name, footer, browser icon, PWA
  name, start-page introduction, e-mails, authenticator issuer, PDF footer. Logo and colours stay free branding.
- Q: How is payment enforced? → A: An offline Ed25519-signed license key issued by the licensor (`tools/licgen`),
  installed under System → License; no online activation.
- Q: Per server or per organization? → A: Per server (all organizations), optionally restricted to the server name.
- Q: License text? → A: Condition 2 of `LICENSE` gets an exception for licensed deployments (draft for legal review).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Licensor issues a license (Priority: P1)
The licensor creates a key pair once, commits the public key, and issues a key per customer with licensee, validity,
features and optionally the server name.

**Independent Test**: `licgen keygen`, `licgen issue`, `licgen show -pub` verifies it.

### User Story 2 - Administrator installs the license (Priority: P1)
A system administrator pastes the key under System → License; the page shows licensee, features, validity and whether
it is valid on this server. Tampered keys, keys of other licensors and keys for other servers are refused.

### User Story 3 - Organization uses its own brand (Priority: P1)
In Settings → Branding and theme, the organization sets its product name, footer, links, icon and introduction; every
page, the installed app, e-mails, the authenticator app and PDF reports show its brand instead of ctrl32 telemetry.

### User Story 4 - Perpetual license, removal and optional expiry (Priority: P2)
The white-label license is sold once (99,- EUR) and never expires; System → License shows it as perpetual. When it is
removed (or a license issued with an expiry date expires), the fixed ctrl32 footer returns; the settings stay.

### Edge Cases
- Settings can be prepared without a license but not enabled (402).
- Pasted keys wrapped over lines are accepted.
- Explanatory texts about the ctrl32 license itself are not rewritten (`.no-rebrand`).
- The test key is trusted only by test builds.

## Requirements *(mandatory)*

- **FR-001**: License tokens signed with Ed25519, verified offline against public keys built into the binary; payload
  id, licensee, e-mail, issued, expires (optional; absent = perpetual, the default for white_label), features, domains (optional, `*.` wildcards), notes.
- **FR-002**: `tools/licgen` keygen (refuses to overwrite a private key), issue, show.
- **FR-003**: API to read, install (reason) and remove (reason) the server license; audited; bad tokens logged as
  security events.
- **FR-004**: White-label settings per organization with validation (plain text, link schemes http(s)/mailto/path,
  icon PNG/SVG ≤ 64 KiB without scripts); enabling needs a valid license with `white_label`.
- **FR-005**: With an active white label: product name in `GET /api/v1/branding`, own footer, browser icon, PWA
  manifest, page titles and visible product mentions, start-page introduction, e-mail subjects and texts,
  authenticator issuer, PDF footer.
- **FR-006**: Without a valid license the fixed footer of ADR-0011 applies.

## Success Criteria *(mandatory)*

- **SC-001**: A key changed by one character is refused (100 %).
- **SC-002**: Installing a license and enabling the white label takes under 2 minutes, without a restart.
- **SC-003**: No page shows "ctrl32" in its footer or title while the white label is active (except the license page).

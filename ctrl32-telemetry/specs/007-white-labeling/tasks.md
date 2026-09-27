---
description: "Tasks for paid white labeling"
---

# Tasks: Paid white labeling

**Input**: `specs/007-white-labeling/` · **Tests**: required · **Paths**: relative to `ctrl32-telemetry/`

## Phase 1: Foundation
- [x] T001 License tokens (Sign, Parse, Check, Has, TrustedKeys, DomainAllowed) with unit tests in internal/license/
- [x] T002 [P] tools/licgen keygen, issue, show
- [x] T003 Migration 0039 (first numbered 0026; renumbered because 0026 became scenes) core.product_license

## Phase 2: US1–US2 license (P1)
- [x] T004 [US2] License API (GET/PUT/DELETE, audit, security events, cache) in internal/httpapi/whitelabel.go
- [x] T005 [US2] System → License section in static/server.html
- [x] T006 Test key trusted only by test builds (tests/keys, tests/run-all.sh)

## Phase 3: US3 white label (P1)
- [x] T007 [US3] white_label in branding (validation, 402 without license, icon) in internal/httpapi/branding.go
- [x] T008 [US3] Branding response, icon, manifest, e-mails, TOTP issuer, PDF footer
- [x] T009 [US3] app.js footer and product rebranding, favicon, start-page introduction; Settings section

## Phase 4: Polish
- [x] T010 e2e tests/e2e/whitelabel.sh
- [x] T011 ADR-0016, LICENSE condition 2 draft, ADR-0002/0011 notes, URS J-16a, docs/whitelabel.md, i18n, OpenAPI, JOURNAL, README; run-all

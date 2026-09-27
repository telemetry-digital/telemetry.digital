# Implementation Plan: Paid white labeling

**Branch**: `assistant/jolly-sagan-nzolwc` | **Date**: 2026-09-26 | **Spec**: [spec.md](spec.md)

## Summary
An offline license mechanism (Ed25519 tokens, public keys embedded, `tools/licgen`) and per-organization white-label
settings applied by the server (branding API, manifest, icon, e-mails, TOTP issuer, PDF footer) and the UI (footer,
titles, product mentions, favicon).

## Technical Context
**Language**: Go 1.27 (crypto/ed25519, standard library only), plain JS · **Storage**: migration 0039
(`core.product_license`), `organization.branding.white_label` · **Testing**: unit `internal/license`, e2e `whitelabel`.

## Constitution Check
| Principle | Status |
|---|---|
| I Record integrity | License install/remove and branding changes audited with reason |
| II One binary + PostgreSQL | Offline verification, no licensing server |
| III Secure by default | Private key never in the binary; bad tokens are security events; icons sanitized; test key only in test builds |
| IV API-first | Endpoints in OpenAPI, clients regenerated |
| V Tested end to end | e2e `whitelabel` (31 checks) |
| VI Permissive dependencies | none added |
| VII Plain web UI, translated | Settings and System pages, sk/cs |

## Structure
```
internal/license/            license.go (Sign, Parse, Check, Has, TrustedKeys), trusted/*.pub
tools/licgen/                keygen, issue, show
db/migrations/0026_product_license.sql
internal/httpapi/whitelabel.go   license API, white-label helpers, icon, manifest
internal/httpapi/branding.go     white_label in branding; static app.js, settings.html, server.html, index.html
internal/report/, internal/iam/  product name in PDF footer and TOTP issuer
tests/keys/, tests/e2e/whitelabel.sh, tests/run-all.sh (test key via ldflags)
docs/whitelabel.md, docs/adr/0016-paid-white-labeling.md, LICENSE (condition 2 draft)
```

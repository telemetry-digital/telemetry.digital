# Implementation Plan: Licence model — free up to 4 cameras, updates included for a period

**Spec**: `specs/026-licence-model/spec.md` · **Version**: 0.66.0 · **Decisions**: ADR-0016 amendment of 2 Oct 2026

## Design

| Part | Where | Notes |
|---|---|---|
| Licence key | `internal/license/license.go` | `UpdatesUntil` claim, `UpdatesEnd`, `UpdatesEnded`, `NoExpiry` (was `Perpetual`), online-key limit `OnlineMaxUpdateDays` |
| Release date | `internal/release` | `-X …/internal/release.date=`, VCS time fallback, `FromVersionOutput`, `Covered` |
| CLI | `cmd/ctrl32-telemetry/main.go`, `deploy/publish.sh` | `version` prints "released YYYY-MM-DD" |
| Agent | `internal/agent/server.go`, `client.go`, `types.go` | `UpdateRequest.UpdatesUntil`, `updateEntitled`, `CodedError` (402 `updates_ended`) |
| Server | `internal/httpapi/license_model.go`, `video.go`, `serveradmin.go`, `whitelabel.go`, `license_activation.go` | `FreeCameraLimit`, `cameraSlot` (transaction + advisory lock), `cameraLimitJSON`, `updatesUntil`, `updatesJSON`, `licenseCoversRelease` |
| licgen | `tools/licgen/main.go`, `batch.go` | `-updates-until`, `queueUpdatesUntil` (`paid_at`, `updates_until`) |
| UI | `static/server.html`, `video.html`, `settings.html`, `internal/httpapi/license.go` | texts without price/"perpetual", rows Updates and Cameras, camera count badge |
| i18n | `internal/i18n/catalog/*.json` | 16 new strings, 1 changed, 4 removed, 19 catalogs |
| Dev | `deploy/dev/dev.sh`, `demo/cameras.py`, `README.md` | development build trusts the test key; demo installs a development licence; cameras.py idempotent, 4 cameras without a licence |
| Tests | `license_test.go`, `release_test.go`, `agent/update_test.go`, `licgen/batch_test.go`, `tests/e2e/video.sh`, `whitelabel.sh`, `licfake.mjs`, `tests/headless/license.js`, `docshots.js` | |

## Constitution Check

I–VII unaffected; VI wording amended (PATCH 1.0.1): the product is free including up to 4 cameras, the licence covers
the extras. No migration is needed for this feature (the claim lives in the signed token).

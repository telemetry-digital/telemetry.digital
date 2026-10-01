# Implementation Plan: SCADA process pictures and editor

**Branch**: `main` | **Date**: 2026-09-25 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/001-scada-process-pictures/spec.md`

## Summary

Process pictures become real SCADA screens: an optional **drawing layer** (`elements`) in the existing screen format
v1 (kind `mimic`, canvas layout) holds vector objects — rect, ellipse, line, polyline, polygon, pipe, text, library
symbol, imported image — with dynamics and click actions. A plain-JavaScript SVG runtime (`scada.js`) renders the
layer below the existing widgets, updates only changed objects on live values and opens faceplates/trends/screens/
commands. A desktop-class editor (`scada-editor.js`) replaces the current canvas editing in `dashboards.html` for
pictures. Customer SVG graphics and reusable composites live in a new versioned content table (object library);
imported SVG is sanitised on the server and displayed through `<image>` so it can never run code. The server
validates everything against a closed schema (already started in `screen_elements.go`).

## Technical Context

**Language/Version**: Go 1.27 (server); ES2022 plain JavaScript in the browser, no build step

**Primary Dependencies**: none new (standard library `encoding/xml` for SVG sanitising; existing pgx, widgets.js)

**Storage**: PostgreSQL 18 — `core.dashboard.spec` (jsonb) for pictures; new `core.hmi_object` for the object library

**Testing**: `go test` (schema + sanitiser unit tests), e2e `tests/e2e/mimics.sh` (extended) + new
`tests/e2e/hmi_objects.sh`, headless probes `tests/headless/scada.js` (viewer) and `scada_editor.js` (editor),
all through `tests/run-all.sh`

**Target Platform**: server Linux/Windows; browsers: last 2 versions of Chrome/Edge/Firefox/Safari (URS N-05)

**Project Type**: web application inside one Go binary (embedded static files)

**Performance Goals**: 1500 objects render < 3 s; live update visible < 2 s after arrival; editor drag at 60 fps
for ≤ 300 objects, ≥ 20 fps at 1500

**Constraints**: CSP `script-src 'self' 'unsafe-inline'` (already), no external requests; stored content inert
(constitution III); pictures ≤ 1500 elements, ≤ 60 widgets; imported SVG ≤ 512 KiB

**Scale/Scope**: ~50 built-in symbols, 8 element types (+ image), 14 dynamic kinds, 4 click actions, 1 new table,
~6 new API routes

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Check | Status |
|---|---|---|
| I Record Integrity | pictures and library objects are versioned (new row, `valid_to`, reason, checksum, audit); commands from pictures go through the existing command path (audit, ack) | PASS |
| II One Binary + PostgreSQL | no new runtime or dependency; static files embedded | PASS |
| III Secure by Default | closed schema validation of elements; colours hex only; text escaped at render; no URL/script actions; SVG import sanitised by whitelist and shown only through `<image>` (no script, no external loads) + served with `CSP: sandbox`; commands need permission + reason, four-eyes applies; public links read only bound tags | PASS |
| IV API-First Content | object library and SVG import get `/api/v1` routes in OpenAPI; clients regenerated; the editor uses only these routes | PASS |
| V Tested End to End | unit tests (schema, sanitiser), e2e (pictures, library), headless probes that fail on regressions; run-all | PASS |
| VI Permissive Dependencies | none added | PASS |
| VII Plain Web UI, Translated | plain JS modules (ADR-0015), rendering separated from loading, strings via catalogs (sk, cs added) | PASS (with ADR-0015 accepted) |

Post-design re-check (after Phase 1): unchanged — PASS.

## Project Structure

### Documentation (this feature)

```text
specs/001-scada-process-pictures/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── screen-elements.md      # drawing layer schema (part of screen format v1)
│   └── hmi-objects-api.md      # object library + SVG import routes
└── tasks.md                    # /speckit-tasks
```

### Source Code (repository root = ctrl32-telemetry/)

```text
db/migrations/0021_hmi_objects.sql          # core.hmi_object (versioned content)
internal/httpapi/
├── screen_elements.go (+_test.go)          # drawing layer validation (exists; + image, group)
├── hmi_objects.go (+_test.go)              # library CRUD, SVG import, SVG serving, public access
├── svgsanitize.go (+_test.go)              # whitelist sanitiser
├── dashboards.go                            # spec: elements, bg, grid, scheme (done)
├── dashboard_starter.go                     # new demo mixing plant picture with elements
├── public_dashboard.go                      # bound tags + referenced library images readable
├── openapi.go, server.go                    # routes
└── static/
    ├── scada.js                             # runtime: symbols, dynamics, events, faceplate (exists, WIP)
    ├── scada-editor.js                      # editor
    ├── widgets.js                           # mounts the drawing layer, live updates for element tags
    ├── dashboards.html                      # screen kinds, editor host for pictures
    ├── public.html                          # loads scada.js
    └── app.css                              # sc-* styles, animations
internal/i18n/catalog/{sk,cs,…}.json         # new strings
tests/e2e/mimics.sh, tests/e2e/hmi_objects.sh
tests/headless/scada.js, tests/headless/scada_editor.js
docs/urs/URS-v0.1.md, JOURNAL.md, docs/adr/0015-*.md
```

**Structure Decision**: existing single Go module with embedded UI; no new top-level directories.

## Complexity Tracking

No violations.

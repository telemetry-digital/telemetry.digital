---
description: "Tasks for SCADA process pictures and editor"
---

# Tasks: SCADA process pictures and editor

**Input**: Design documents from `specs/001-scada-process-pictures/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: required by the constitution (principle V) — unit, e2e and headless tasks are included per story.

**Paths**: relative to `ctrl32-telemetry/`.

## Phase 1: Setup

- [x] T001 Record ADR-0015 (canvas editors before 1.0) in docs/adr/0015-canvas-editors-before-1-0.md
- [ ] T002 [P] Add sc-* styles and keyframes (sc-spin, sc-agit, sc-flow using `--sc-per`, sc-blink, sc-flame, sc-click cursor, faceplate dialog layout, `.sc-sym *{vector-effect:non-scaling-stroke}`) in internal/httpapi/static/app.css

---

## Phase 2: Foundational

- [x] T003 Validate the drawing layer (types rect/ellipse/line/polyline/polygon/pipe/text/symbol, ≤ 1500 elements, ≤ 200 points, ≤ 8 dyn, ≤ 4 on, hex colours, closed style keys) in internal/httpapi/screen_elements.go with tests in screen_elements_test.go
- [x] T004 Accept `layout.bg` (hex), `layout.grid` (1–100), `scheme` (classic|isa101) and "widgets or elements non-empty" in internal/httpapi/dashboards.go
- [x] T005 Public links read tags of elements and dynamics in internal/httpapi/public_dashboard.go
- [ ] T006 Extend the schema with element type `image` (`src` = library slug `^[a-z0-9][a-z0-9-]{0,62}$`) and field `group` (`^[A-Za-z0-9_-]{1,32}$`) plus tests in internal/httpapi/screen_elements.go and screen_elements_test.go
- [ ] T007 Mount the drawing layer in canvas screens (SVG below widgets), add element tags to the live subscription, refresh the layer on observations (debounced 400 ms), export `isPublic()` in internal/httpapi/static/widgets.js
- [ ] T008 Load scada.js before widgets.js in internal/httpapi/static/dashboards.html and internal/httpapi/static/public.html

**Checkpoint**: an element-only picture stored through the API renders in view mode.

---

## Phase 3: User Story 1 — live process picture (P1) 🎯 MVP

**Goal**: operators see levels, states, flow, alarms and values update live.
**Independent test**: store a picture via API, publish values, see changes without reload.

- [ ] T009 [US1] Finish the runtime: renderer for all element types, symbol library ≥ 50 symbols in 8 categories, dynamics evaluation (all 14 kinds, first-match mappings, clamped linear ranges), keyed diff redraw in internal/httpapi/static/scada.js
- [ ] T010 [US1] Show "—" and no state for tags without a value; quality marker in tooltip in internal/httpapi/static/scada.js
- [ ] T011 [P] [US1] Headless probe that loads a stored picture, publishes values via the device API and asserts level height, fill colours, flow class and text content, failing on mismatch, in tests/headless/scada.js
- [ ] T012 [P] [US1] Extend tests/e2e/mimics.sh: store a picture with elements, read it back canonical, reject 5 invalid pictures (bad colour, markup-free text kept literally, unknown type, empty lin range, foreign screen slug)

**Checkpoint**: US1 demonstrable with an API-made picture.

---

## Phase 4: User Story 2 — professional editor (P1)

**Goal**: engineers draw pictures like in a desktop SCADA editor.
**Independent test**: draw tank + pump + valve + 2 pipes + 3 texts, undo/redo, align, copy/paste, save, reload identical.

- [ ] T013 [US2] Editor core: layout (toolbar, library panel, canvas with zoom 25–400 % / fit and pan, property panel, layer list), selection (click, shift, rubber band), move, 8 resize handles, rotate handle, snap to `layout.grid` in internal/httpapi/static/scada-editor.js
- [ ] T014 [US2] Drawing tools: rect, ellipse, line, polyline, polygon, pipe (click points, finish on double-click/Enter, Esc cancels), text; point editing (drag, add on segment double-click, delete with Alt-click) in internal/httpapi/static/scada-editor.js
- [ ] T015 [US2] Commands: undo/redo (100 snapshots), copy/cut/paste/duplicate (offset one grid step, new ids), delete, lock/unlock, hide, align ×6, distribute ×2, same size, z-order ×4, group/ungroup, keyboard shortcuts (Ctrl+Z/Y/C/X/V/D/A/G, Delete, arrows ±1/±grid) in internal/httpapi/static/scada-editor.js
- [ ] T016 [US2] Symbol library panel with categories, search, previews, click and drag to place in internal/httpapi/static/scada-editor.js
- [ ] T017 [US2] Host the editor for canvas screens in the edit mode of internal/httpapi/static/dashboards.html (existing widget placement keeps working on the same canvas; unsaved-changes guard; save with reason)
- [ ] T018 [P] [US2] Headless editor probe: place symbols, draw a pipe, align, undo/redo, copy/paste, save, reload and compare the stored spec, in tests/headless/scada_editor.js

**Checkpoint**: pictures can be drawn and saved; US1 shows them live.

---

## Phase 5: User Story 3 — bindings and dynamics (P1)

**Goal**: engineers bind tags and configure dynamics without code.
**Independent test**: each dynamic kind configured through the panel produces the expected picture for hit/miss values.

- [ ] T019 [US3] Property panel sections Geometry, Appearance, Text, Binding (datastream picker with filter, device + key), Dynamics (row editor per kind: mapping rows op/value/colour|state|text, condition op/value (+colour/speed), linear in0/in1/out0/out1, optional tag) in internal/httpapi/static/scada-editor.js
- [ ] T020 [US3] Show server validation errors with the element highlighted in internal/httpapi/static/scada-editor.js
- [ ] T021 [P] [US3] Extend screen_elements_test.go with one valid and one invalid case per dynamic kind in internal/httpapi/screen_elements_test.go

---

## Phase 6: User Story 4 — faceplates, trends, screens, commands (P2)

**Goal**: click actions with permission, reason, four-eyes, audit.
**Independent test**: one object per action, check dialog/navigation/audited command/permissions.

- [ ] T022 [US4] Events editor (faceplate, trend, screen with slug picker, command key/value/method/confirm) in internal/httpapi/static/scada-editor.js
- [ ] T023 [US4] Runtime actions: faceplate (value, quality, time, 1h/6h/24h/7d trend, Start/Stop/setpoint with reason), trend window, screen change (disabled in public mode), command dialog with reason/confirm and live status via the command stream in internal/httpapi/static/scada.js
- [ ] T024 [P] [US4] e2e: command from a picture's bound device/key produces an audited command with the reason; public link cannot send in tests/e2e/mimics.sh

---

## Phase 7: User Story 5 — SCADA, widgets, or both (P2)

- [ ] T025 [US5] "New screen" dialog offers Dashboard / Process picture / Process picture with widgets / demo mixing plant in internal/httpapi/static/dashboards.html
- [ ] T026 [US5] Demo mixing plant starter (≥ 40 elements, vessels, pumps, valves, pipes, heat exchanger, instruments, texts bound to the demo datastreams) in internal/httpapi/dashboard_starter.go
- [ ] T027 [P] [US5] Headless: widget + drawing on one picture both update in tests/headless/scada.js

---

## Phase 8: User Story 6 — object library, SVG import, schemes (P3)

- [ ] T028 [US6] Migration `core.hmi_object` (id, org_id, slug `^[a-z0-9][a-z0-9-]{0,62}$`, kind `svg|composite`, name ≤ 128, category ≤ 64, version, body jsonb, checksum, created_by, created_at, valid_to, reason) with grants SELECT/INSERT/UPDATE to ctrl32_app in db/migrations/0021_hmi_objects.sql
- [ ] T029 [US6] SVG sanitiser (element/attribute whitelist, href only `#…`, style url only `url(#…)`, no on*, no foreignObject/script/image/a/animate/set, ≤ 512 KiB, ≤ 20 000 elements, returns removed items) with unit tests incl. XSS vectors in internal/httpapi/svgsanitize.go and svgsanitize_test.go
- [ ] T030 [US6] Library API per contracts/hmi-objects-api.md (list, get, create, update, retire, import-svg, svg serving with CSP sandbox, public svg route checked against the picture) with audit in internal/httpapi/hmi_objects.go, routes in server.go, OpenAPI in openapi.go, regenerate clients (`go run ./tools/apigen`)
- [ ] T031 [US6] Composite placement (tag placeholder re-binding dialog), store selection as composite, import SVG dialog, image element rendering via `<image href>` in internal/httpapi/static/scada-editor.js and scada.js
- [ ] T032 [US6] Picture settings: background, grid, scheme (classic/isa101) in internal/httpapi/static/dashboards.html
- [ ] T033 [P] [US6] e2e library suite (create, version, retire, import with script stripped, serving headers, public access only when referenced) in tests/e2e/hmi_objects.sh

---

## Phase 9: Polish & cross-cutting

- [ ] T034 [P] Slovak and Czech translations for all new strings in internal/i18n/catalog/sk.json and cs.json (English keys)
- [ ] T035 [P] Update URS J-03/J-04/J-05 and add J-20…J-27 in docs/urs/URS-v0.1.md; JOURNAL entry and version bump in JOURNAL.md and README.md
- [ ] T036 Performance check: 1500-element picture renders < 3 s and editor drag stays responsive (headless timing) in tests/headless/scada.js
- [ ] T037 Run `bash tests/run-all.sh` green, commit and push

## Dependencies

Setup → Foundational (T006–T008) → US1 → US2 → US3 → US4, US5 (parallel) → US6 → Polish.
US2 needs the US1 renderer; US3 needs the US2 panel; US6 needs US2 (editor) and T006 (image element).

## Parallel opportunities

T002 ∥ T006; within US1 T011 ∥ T012; T018, T021, T024, T027, T033, T034, T035 are independent of the files their
stories touch.

## Implementation strategy

MVP = Phases 1–3 (live pictures made through the API) → then the editor (US2–US3) which makes the MVP usable by
engineers → interaction (US4) and screen kinds (US5) → library (US6). Commit after each phase with run-all green.

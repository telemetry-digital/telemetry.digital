---
description: "Tasks for the visual flow editor"
---

# Tasks: Visual flow editor

**Input**: `specs/004-flow-editor/` · **Tests**: required · **Paths**: relative to `ctrl32-telemetry/`

## Phase 1: Foundation
- [x] T001 Expression language (typed, limits, functions of research R3) with unit tests incl. limits and malformed input in internal/flows/expr.go
- [x] T002 Message model and node catalogue with settings schema in internal/flows/catalog.go
- [x] T003 Validation (closed schema, wires, cycles without delay, expressions, references, allow-list, sizes) with a regression set of invalid flows in internal/flows/validate.go

## Phase 2: US1 engine (P1) 🎯 MVP
- [x] T004 Engine: per-flow goroutine, bounded queue, timer heap, node interface, counters, debug ring in internal/flows/engine.go
- [x] T005 Trigger nodes: datastream, alarm, device, schedule, webhook, inject
- [x] T006 Logic nodes: filter, switch, change, math, delay, debounce, rate limit, deadband, aggregate, join, template, counter, latch, time window, stable-for
- [x] T007 Action nodes: derive, command, attribute, notify, incident, http out (allow-list), debug; audit origin
- [x] T008 Migration 0023 (core.flow, core.flow_state) and store (versions, state persistence); webhook tokens are derived with HMAC from the server secret, so no core.flow_hook table is needed

## Phase 3: US4 governance + API
- [x] T009 API: catalog, CRUD, deploy (four-eyes), disable, versions, restore, delete, validate, export/import, OpenAPI in internal/httpapi/flows.go

## Phase 4: US1/US2 editor
- [x] T010 Flows list page and editor (palette, canvas, wires, selection, copy/paste, undo/redo, zoom/pan, properties) in static/flows.html, static/flow-editor.js
- [x] T011 Debug panel, test injection, dry run, node status badges

## Phase 5: US3/US4 extras
- [x] T012 Rule → flow conversion (internal/flows/fromrule.go, GET /api/v1/rules/{id}/as-flow); metrics (Prometheus); state persistence across restarts; throughput test (SC-002)

## Phase 6: Tests and polish
- [x] T013 e2e: build/deploy the US1 flow via API, publish values, verify notification/command with audit origin; dry run; invalid flows refused; versions/restore; four-eyes in tests/e2e/flows.sh
- [x] T014 Headless: build a flow with the mouse, wire it, deploy, inject, read the debug panel in tests/headless/flows.js
- [x] T015 i18n sk/cs, URS, JOURNAL, README, docs/flows.md
- [x] T016 run-all green, commit, push

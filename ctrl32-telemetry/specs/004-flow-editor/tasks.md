---
description: "Tasks for the visual flow editor"
---

# Tasks: Visual flow editor

**Input**: `specs/004-flow-editor/` · **Tests**: required · **Paths**: relative to `ctrl32-telemetry/`

## Phase 1: Foundation
- [ ] T001 Expression language (typed, limits, functions of research R3) with unit tests incl. limits and malformed input in internal/flows/expr.go
- [ ] T002 Message model (≤ 16 KB) and node catalogue with settings schema in internal/flows/message.go, catalog.go
- [ ] T003 Validation (closed schema, wires, cycles without delay, expressions, references, allow-list, sizes) with a regression set of invalid flows in internal/flows/validate.go

## Phase 2: US1 engine (P1) 🎯 MVP
- [ ] T004 Engine: per-flow goroutine, bounded queue, timer heap, node interface, counters, debug ring in internal/flows/engine.go
- [ ] T005 Trigger nodes: datastream, alarm, device, schedule, webhook, inject
- [ ] T006 Logic nodes: filter, switch, change, math, delay, debounce, rate limit, deadband, aggregate, join, template, counter, latch, time window, stable-for
- [ ] T007 Action nodes: derive, command, attribute, notify, incident, http out (allow-list), debug; audit origin
- [ ] T008 Migration 0023 (core.flow, core.flow_state, core.flow_hook) and store (versions, state persistence)

## Phase 3: US4 governance + API
- [ ] T009 API: catalog, CRUD, deploy (four-eyes), disable, versions, restore, delete, validate, export/import, OpenAPI in internal/httpapi/flows.go

## Phase 4: US1/US2 editor
- [ ] T010 Flows list page and editor (palette, canvas, wires, selection, copy/paste, undo/redo, zoom/pan, properties) in static/flows.html, static/flow-editor.js
- [ ] T011 Debug panel, test injection, dry run, node status badges

## Phase 5: US3/US4 extras
- [ ] T012 Rule → flow conversion; metrics (Prometheus); state persistence across restarts

## Phase 6: Tests and polish
- [ ] T013 e2e: build/deploy the US1 flow via API, publish values, verify notification/command with audit origin; dry run; invalid flows refused; versions/restore; four-eyes in tests/e2e/flows.sh
- [ ] T014 Headless: build a flow with the mouse, wire it, deploy, inject, read the debug panel in tests/headless/flows.js
- [ ] T015 i18n sk/cs, URS, JOURNAL, README, docs/flows.md
- [ ] T016 run-all green, commit, push

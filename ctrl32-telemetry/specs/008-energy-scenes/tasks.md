---
description: "Tasks for energy overview and scenes"
---

# Tasks: Energy overview and scenes

**Input**: `specs/008-energy-scenes/` · **Tests**: required · **Paths**: relative to `ctrl32-telemetry/`

## Phase 1: Foundation
- [x] T001 Migration 0026 core.scene

## Phase 2: US1–US2 energy
- [x] T002 [US1] Meter discovery, roles, SQL aggregation per bucket, cost, power in internal/httpapi/energy.go
- [x] T003 [US1] Energy settings API with audit
- [x] T004 [US1] static/energy.html (tiles, chart, table, settings; organization time zone), menu entry

## Phase 3: US3 scenes
- [x] T005 [US3] Scenes CRUD, capture, activation through entity commands in internal/httpapi/scenes.go
- [x] T006 [US3] Scenes on the Home page (activate, create by capture, edit, delete)
- [x] T007 [US3] Flow node `scene` with unit test; editor picker
- [x] T008 color_mode inference for lights without it (capture picks the right colour)

## Phase 4: Polish
- [x] T009 e2e tests/e2e/energy_scenes.sh; OpenAPI and clients; sk/cs; docs/smarthome.md; URS A-30/A-31; JOURNAL, README; run-all

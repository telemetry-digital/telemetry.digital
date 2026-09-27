---
description: "Tasks for smart home phase 2"
---

# Tasks: Smart home phase 2

**Input**: `specs/006-smarthome-phase2/` · **Tests**: required · **Paths**: relative to `ctrl32-telemetry/`

## Phase 1: Foundation
- [x] T001 Migration 0025: entity.name_locked, incident kind mqtt_source in db/migrations/0025_smarthome_native.sql
- [x] T002 [P] Template methods, slices, join, int base with unit tests in internal/smarthome/template.go
- [x] T003 [P] Raw subscribe/publish per source in internal/smarthome/raw.go; bridges resubscribe raw filters

## Phase 2: US1–US2 native devices (P1) 🎯
- [x] T004 [US1] Shelly adapter (identification, RPC requests, status merge, synthesized configs) in internal/smarthome/native.go
- [x] T005 [US2] Tasmota native adapter (config, sensors, removal, STATE query) in internal/smarthome/native.go
- [x] T006 [US1][US2] Unit tests through ParseConfig/Entity in internal/smarthome/native_test.go
- [x] T007 [US1][US2] Simulators `mqttsim shelly`, `mqttsim tasmota -native` in tools/mqttsim/main.go

## Phase 3: US3 colour (P2)
- [x] T008 [US3] Entity.Color, kelvin conversion, RGB/CT templates in internal/smarthome/entity.go; `ui.color` in internal/httpapi/smarthome.go
- [x] T009 [US3] Colour picker and colour-temperature slider in static/home.html, app.css

## Phase 4: US4 MQTT nodes (P2)
- [x] T010 [US4] mqtt_in, mqtt_out in internal/flows (catalog, validate, engine, nodes) with tests
- [x] T011 [US4] flowHost.MQTTSubscribe/MQTTPublish, flowRefs.MQTTSource, editor source picker (flow-editor.js, flows.html)

## Phase 5: US5 outage incidents (P2)
- [x] T012 [US5] checkOutages in internal/smarthome/manager.go; SourceDown/SourceRecovered in internal/incident; setting in API and source dialog

## Phase 6: Tests and polish
- [x] T013 Renamed entities keep their names (name_locked) in internal/smarthome/manager.go, internal/httpapi/smarthome.go
- [x] T014 e2e tests/e2e/smarthome_native.sh; headless colour checks in tests/headless/home.js
- [x] T015 i18n sk/cs, OpenAPI and clients, docs/smarthome.md, URS, JOURNAL, README; run-all green, commit, push

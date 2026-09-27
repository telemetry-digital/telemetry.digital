---
description: "Tasks for MQTT provisioning and smart-home devices"
---

# Tasks: MQTT provisioning and smart-home devices

**Input**: `specs/005-mqtt-provisioning-smarthome/` · **Tests**: required · **Paths**: relative to `ctrl32-telemetry/`

## Phase 1: Foundation
- [x] T001 Migration 0024: provision_profile, entity, discovery; device transport `mqtt_discovery`, auto_datastreams, approval; connector kinds `mqtt_account`, `mqtt_bridge` and status; token hash index in db/migrations/0024_smarthome.sql
- [x] T002 [P] Jinja subset templates with unit tests (zigbee2mqtt, Tasmota, ESPHome samples, limits, errors) in internal/smarthome/template.go
- [x] T003 [P] Discovery normalization (abbreviations, `~`, device info, availability, device-based discovery) with unit tests in internal/smarthome/abbrev.go, discovery.go
- [x] T004 [P] Entity state parsing and command payloads per component with unit tests in internal/smarthome/entity.go

## Phase 2: US1 provisioning (P1) 🎯
- [x] T005 Provisioning service (profiles, strategies, approval, placement, rate limit, audit) in internal/provision/
- [x] T006 Broker: token-only login, `provision` sessions with direct replies, ThingsBoard alias rewriting, approval check in internal/broker/auth.go, namespaces.go
- [x] T007 Ingest: auto datastreams, TB attribute requests, RPC responses; commands to `d/{ext}/tb/rpc/request/{n}` in internal/ingest/, internal/httpapi/commands_approval.go
- [x] T008 API: provision profiles, approvals, HTTP provisioning with key/secret; OpenAPI in internal/httpapi/provisioning.go

## Phase 3: US2 sources and discovery (P1)
- [x] T009 Broker namespaces for MQTT accounts (OnPublish/OnSubscribe/OnWill/OnPacketEncode, ACL) in internal/broker/namespaces.go
- [x] T010 Smart-home manager: sources, discovery intake, inbox/adoption, devices, rooms, entities, datastreams, states, availability, attributes in internal/smarthome/manager.go
- [x] T011 Bridges with autopaho (reconnect, status, sealed password) in internal/smarthome/bridge.go
- [x] T012 API: MQTT sources, discovery inbox, entities, home in internal/httpapi/smarthome.go

## Phase 4: US3 control and Home page (P1)
- [x] T013 Entity commands through device_command (dispatch for `mqtt_discovery`, ack on state match) in internal/smarthome/manager.go, internal/httpapi/commands_approval.go
- [x] T014 Home page (rooms, cards, controls, live, history) in static/home.html; live event `entity`

## Phase 5: US4–US6
- [x] T015 Onboarding: sources page with inbox, accounts, bridges, provisioning profiles, approvals and per-ecosystem instructions in static/smarthome.html; `GET /api/v1/onboarding`
- [x] T016 Flow nodes entity, entity_action, command_result in internal/flows/
- [x] T017 Publishing selected datastreams to a bridge with discovery in internal/smarthome/export.go

## Phase 6: Tests and polish
- [x] T018 tools/mqttsim (provision, tb, z2m, tasmota, broker) and e2e tests/e2e/provisioning.sh, tests/e2e/smarthome.sh (isolation, bridge)
- [x] T019 Headless tests/headless/home.js
- [x] T020 i18n sk/cs, docs/smarthome.md, URS, JOURNAL, README; run-all green, commit, push

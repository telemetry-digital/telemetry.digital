# Implementation Plan: Smart home phase 2

**Branch**: `assistant/jolly-sagan-nzolwc` | **Date**: 2026-09-26 | **Spec**: [spec.md](spec.md)

## Summary
Native adapters that synthesize Home Assistant discovery configs (Shelly Gen2+ over RPC, Tasmota native discovery),
colour controls on light cards, raw MQTT flow nodes on MQTT sources, and an incident for sources that stay
disconnected. Also fixes renamed entities being renamed back by the device.

## Technical Context
**Language**: Go 1.27, plain JS UI · **Storage**: PostgreSQL 18 (migration 0025) · **MQTT**: embedded mochi-mqtt,
autopaho bridges · **Testing**: Go unit tests, `tests/e2e/smarthome_native.sh` with `tools/mqttsim` (new `shelly` and
`tasmota -native` modes), headless `home.js`.

## Constitution Check
| Principle | Status |
|---|---|
| I Record integrity | Adapter states are entity states → observations; commands audited |
| II One binary + PostgreSQL | Adapters inside the smart-home manager; no add-ons |
| III Secure by default | MQTT out only inside the organization's own sources; commanding flows need four eyes where required |
| IV API-first | `ui.color`, `outage_incident_min`, incident kind documented in OpenAPI |
| V Tested end to end | e2e `smarthome_native` (36 checks), headless `home` (colour) |
| VI Permissive dependencies | none added |
| VII Plain web UI, translated | colour controls, source dialog field, incident kind labels in sk/cs |

## Structure
```
db/migrations/0025_smarthome_native.sql   entity.name_locked, incident kind mqtt_source
internal/smarthome/native.go              Shelly and Tasmota adapters (ShellyConfigs, TasmotaConfigs)
internal/smarthome/raw.go                 Subscribe / PublishRaw for flows
internal/smarthome/entity.go              Color(), kelvin conversion, RGB templates
internal/smarthome/template.go            methods, slices, join, int base
internal/smarthome/manager.go             dispatch, checkOutages, Incidents interface
internal/incident/incident.go             SourceDown / SourceRecovered
internal/flows/                           mqtt_in, mqtt_out
internal/httpapi/                         flowHost MQTT, flowRefs.MQTTSource, ui.color, outage setting; home.html
tools/mqttsim/                            shelly, tasmota -native
```

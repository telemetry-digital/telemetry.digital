# Implementation Plan: MQTT provisioning and smart-home devices

**Branch**: `main` | **Date**: 2026-09-26 | **Spec**: [spec.md](spec.md)

## Summary
Provisioning profiles with ThingsBoard-compatible provisioning over MQTT and HTTP, the ThingsBoard device API on the
embedded broker, automatic datastreams, MQTT accounts with isolated namespaces, bridges to foreign brokers, a Home
Assistant discovery engine with entities and commands, a Home page, onboarding pages, flow nodes, and optional
publishing to Home Assistant.

## Technical Context
**Language**: Go 1.27, plain JS UI · **Storage**: PostgreSQL 18 (migration 0024) · **MQTT**: mochi-mqtt v2.7.9
(embedded, hooks for rewriting), paho.golang autopaho (bridges) · **Testing**: Go unit tests, bash e2e with a Go MQTT
simulator (`tools/mqttsim`), headless Chrome probe · **Platforms**: Linux, Windows · **Constraints**: one binary, no
second broker, organizations isolated.

## Constitution Check
| Principle | Status |
|---|---|
| I Record integrity | Entity states → append-only observations; current state in a mutable row like `device.status`; every control action is a command with audit |
| II One binary + PostgreSQL | Broker namespaces instead of extra brokers; bridge client in process |
| III Secure by default | Hashed secrets (profile, account), sealed bridge passwords, rate limits, security events, namespace isolation, direct provisioning replies |
| IV API-first | Every page on documented endpoints (OpenAPI, generated clients) |
| V Tested end to end | e2e `provisioning`, `smarthome`, headless `home` |
| VI Permissive dependencies | paho.golang (EDL-1.0 = BSD-3) already in go.mod |
| VII Plain web UI, translated | `/home`, `/smarthome` in plain JS, sk/cs catalogs |

## Structure
```
db/migrations/0024_smarthome.sql
internal/broker/        namespaces.go (mount points), tb.go (ThingsBoard aliases, token login), provision hook
internal/provision/     profiles, Provision(ctx, req) shared by MQTT and HTTP
internal/ingest/        auto datastreams, TB attribute request / RPC response
internal/smarthome/     abbrev.go, discovery.go (normalize), template.go (Jinja subset), entity.go (state parse,
                        command payloads), manager.go (sources, adoption, states, commands), bridge.go, export.go
internal/flows/         entity, entity_action, command_result nodes
internal/httpapi/       provisioning.go, smarthome.go, static/home.html, static/smarthome.html
tools/mqttsim/          provisioning client, ThingsBoard device, zigbee2mqtt and Tasmota simulators
tests/e2e/provisioning.sh, tests/e2e/smarthome.sh, tests/headless/home.js
docs/smarthome.md
```

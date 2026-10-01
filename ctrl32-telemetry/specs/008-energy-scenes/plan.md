# Implementation Plan: Energy overview and scenes

**Branch**: `main` | **Date**: 2026-09-26 | **Spec**: [spec.md](spec.md)

## Summary
A SQL aggregation of meter increases per bucket in the organization's time zone with roles and prices, an Energy page,
scenes stored per organization and activated through the entity command path, a Home row and a flow node.

## Technical Context
Go 1.27, PostgreSQL 18 (migration 0026 `core.scene`; `organization.settings.energy`), plain JS · Tests: unit
`internal/flows` (scene node), e2e `energy_scenes`.

## Constitution Check
| Principle | Status |
|---|---|
| I Record integrity | Energy computed from append-only observations; scene activations are audited commands |
| II One binary + PostgreSQL | window functions and date_trunc in PostgreSQL, no extra service |
| III Secure by default | scene actions validated against the entity's services; permissions device.manage / device.command |
| IV API-first | OpenAPI, clients regenerated |
| V Tested end to end | e2e `energy_scenes` (37 checks) |
| VII Plain web UI, translated | energy.html, Home scenes, sk/cs |

## Structure
```
db/migrations/0026_scenes.sql
internal/httpapi/energy.go, scenes.go, static/energy.html, static/home.html (scenes), app.js (menu)
internal/flows/ scene node (catalog, validate, nodes, engine Host.SceneActivate)
internal/smarthome/entity.go  color_mode inference
tests/e2e/energy_scenes.sh
```

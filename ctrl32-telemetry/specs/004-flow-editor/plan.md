# Implementation Plan: Visual flow editor

**Branch**: `assistant/jolly-sagan-nzolwc` | **Spec**: [spec.md](spec.md)

## Summary
An in-process flow engine (`internal/flows`) with a closed node library and a typed expression language, flows as
versioned content with validation, deploy/four-eyes/restore/export, test injection, dry run and debug; a plain-JS
SVG editor (palette, canvas, wires, properties, debug panel). See research.md R1–R10.

## Technical Context
Go 1.27, PostgreSQL 18, pgx; plain JS (ADR-0013/0015); migration 0023; reuses ingest sink, live hub, rules actions
(deriver, notification channels, alarms), the server's Commander and four-eyes change requests.

## Constitution Check
I integrity (audited actions with origin, versions) ✅ · II no second runtime ✅ · III no user code (expressions
only, clarified) ✅ · IV API first ✅ · V tests (unit per node, expression fuzz/limits, e2e, headless editor) ✅ ·
VI plain JS ✅ · VII docs ✅

## Structure
```
db/migrations/0023_flows.sql
internal/flows/{expr.go, message.go, catalog.go, nodes_*.go, validate.go, engine.go, store.go, *_test.go}
internal/httpapi/flows.go (+ OpenAPI), static/flows.html, static/flow-editor.js
tests/e2e/flows.sh, tests/headless/flows.js
```

## Phases
1 expression language + message + catalogue + validation · 2 engine + triggers/logic/actions · 3 store, API,
deploy/versions/four-eyes · 4 editor + debug · 5 rule→flow, import/export, metrics · 6 docs, i18n, tests.

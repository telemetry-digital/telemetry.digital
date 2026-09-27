# Implementation Plan: Dashboard widgets with rich settings

**Branch**: `assistant/jolly-sagan-nzolwc` | **Date**: 2026-09-25 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/002-dashboard-widgets/spec.md`

## Summary

The widget catalogue (served by `GET /api/v1/dashboards/catalog`, validated by `validateParams`) gains new parameter
kinds (colour, icon, series list, range list, date-time) and a common **Appearance** group appended to every widget
type. The renderer applies appearance to the card, charts get per-series settings, a right axis, legend statistics,
aggregation functions and fixed time windows, and 24 new widget types are added in a second renderer module. The
property panel groups settings in tabs. Existing specs keep rendering identically because every new setting has a
default equal to today's look.

## Technical Context

**Language/Version**: Go 1.27; plain ES2022 JavaScript

**Primary Dependencies**: none new in Go; Tabler Icons subset (MIT, vendored per ADR-0009) for title and card icons

**Storage**: unchanged — widget params inside `core.dashboard.spec`

**Testing**: unit tests for the new parameter kinds; e2e `tests/e2e/dashboards.sh` extended; headless
`tests/headless/widgets.js` (every type renders, appearance applied, old dashboards unchanged by screenshot hash)

**Target Platform**: as the product

**Project Type**: web application in one Go binary

**Performance Goals**: showcase with every type (≥ 44 widgets) loads < 3 s

**Constraints**: no charting library (in-house SVG renderer); settings validated server-side (constitution III, IV)

**Scale/Scope**: ≈ 15 common settings, 24 new types, ≈ 300 new strings (sk, cs)

## Constitution Check

| Principle | Check | Status |
|---|---|---|
| I Record Integrity | dashboards stay versioned with reason and audit | PASS |
| II One Binary + PostgreSQL | no new runtime; icons vendored | PASS |
| III Secure by Default | every setting validated (hex colours, closed icon list, bounded text, closed enums); no user HTML/JS (markdown card renders a safe subset from escaped text) | PASS |
| IV API-First Content | catalog endpoint describes every setting; OpenAPI unchanged routes, schema text updated | PASS |
| V Tested End to End | unit + e2e + headless with screenshot comparison of existing dashboards | PASS |
| VI Permissive Dependencies | Tabler Icons MIT (already the chosen icon set, ADR-0009) | PASS |
| VII Plain Web UI, Translated | plain JS, strings through catalogs | PASS |

## Project Structure

```text
specs/002-dashboard-widgets/{plan.md, research.md, data-model.md, contracts/widget-settings.md, quickstart.md, tasks.md}
internal/httpapi/dashboards.go            # new param kinds, common appearance group, new widget defs
internal/httpapi/widget_catalog_more.go   # definitions of the 24 new types
internal/httpapi/dashboard_starter.go     # showcase with every type
internal/httpapi/static/widgets.js        # appearance, series, legend stats, aggregation, time window, actions
internal/httpapi/static/widgets-more.js   # renderers of the new types (registered into widgets.js)
internal/httpapi/static/vendor/tabler-icons.js + TABLER-LICENSE
internal/httpapi/static/dashboards.html   # tabbed property panel, new editors (series, ranges, icon, colour)
tests/e2e/dashboards.sh, tests/headless/widgets.js
```

**Structure Decision**: existing layout; the new renderer module keeps `widgets.js` readable.

## Complexity Tracking

No violations.

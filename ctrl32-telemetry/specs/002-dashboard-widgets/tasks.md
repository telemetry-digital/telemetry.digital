---
description: "Tasks for dashboard widgets with rich settings"
---

# Tasks: Dashboard widgets with rich settings

**Input**: `specs/002-dashboard-widgets/` · **Tests**: required (constitution V) · **Paths**: relative to `ctrl32-telemetry/`

## Phase 1: Setup

- [x] T001 Vendor ≈ 60 Tabler Icons (outline, pinned tag, MIT licence file) as a name → path map in internal/httpapi/static/vendor/tabler-icons.js and vendor/TABLER-LICENSE; the same name list in Go for validation (internal/httpapi/icons.go)

## Phase 2: Foundational

- [x] T002 Extend `ScreenParam` with `Group` and kinds color, icon, series, ranges, datetime; validation for each (hex colour, closed icon list, series ≤ 24 entries with label ≤ 64 / color / unit ≤ 16 / decimals 0–6 / style / axis / flags, ranges ≤ 8 with op incl. between, datetime RFC 3339) in internal/httpapi/dashboards.go with unit tests in dashboards_params_test.go
- [x] T003 Unknown params are dropped and returned as `warnings` in the save response in internal/httpapi/dashboards.go (FR-006)
- [x] T004 `withCommon`: append the common appearance params (data-model table) to every widget definition in internal/httpapi/dashboards.go
- [x] T005 `ctrl32Widgets.register(type, fn)` and apply appearance (show_title, title_icon, sizes, colours, border, radius, padding, shadow, transparent, tooltip, fullscreen button) to every card in internal/httpapi/static/widgets.js

## Phase 3: US1 — appearance for every widget (P1) 🎯 MVP

- [x] T006 [US1] Tabbed property panel (Data, Appearance, Series, Time, Actions) with colour and icon pickers in internal/httpapi/static/dashboards.html
- [x] T007 [US1] Full-screen view of one widget (Escape returns) in internal/httpapi/static/widgets.js
- [x] T008 [P] [US1] Headless probe: every appearance setting on one widget of each existing type, and the existing showcase unchanged (card geometry and computed styles) in tests/headless/widgets.js

## Phase 4: US2 — series, axes, legend, time window, aggregation (P1)

- [x] T009 [US2] Series editor (per bound datastream) in internal/httpapi/static/dashboards.html
- [x] T010 [US2] Chart: per-series label/colour/unit/decimals/style/points/fill/hidden, right axis, legend position with min/max/avg/total/latest, time_mode fixed from/to, agg none|avg|min|max|sum|count with interval in internal/httpapi/static/widgets.js
- [x] T011 [P] [US2] e2e: store a chart with every series/time setting, read back canonical; invalid colour/axis/agg refused; unknown param reported as warning in tests/e2e/dashboards.sh

## Phase 5: US3 — 24+ new widget types (P2)

- [x] T012 [US3] Catalogue definitions of the new types in internal/httpapi/widget_catalog_more.go
- [x] T013 [US3] Renderers value_card, agg_card, progress, level_card, thermometer, battery, signal, compass, radial_gauge, linear_gauge, led_grid in internal/httpapi/static/widgets-more.js
- [x] T014 [US3] Renderers state_timeline, bar_time, pie, polar, radar, ts_table, entity_table, alarm_count, clock, markdown (escaped safe subset), image (library graphic), qr, nav_card in internal/httpapi/static/widgets-more.js
- [x] T015 [US3] Control renderers knob, slider, round_switch (commands with reason, confirm, permission) in internal/httpapi/static/widgets-more.js
- [x] T016 [US3] Showcase starter with every type in internal/httpapi/dashboard_starter.go; thumbnails for the add dialog in dashboards.html
- [x] T017 [P] [US3] Headless: every type renders without error with demo data in tests/headless/widgets.js

## Phase 6: US4 — conditional formatting and click actions (P3)

- [x] T018 [US4] Ranges editor and `condFormat` applied to value colour/background/icon; click actions screen/trend/faceplate in widgets.js and dashboards.html
- [x] T019 [P] [US4] Headless: ranges switch the background, click opens the trend in tests/headless/widgets.js

- [x] T023 [US2] Viewer filters (FR-011): `filters` setting (range, agg, interval, series) and the filter bar on the widget in internal/httpapi/static/widgets.js; headless check in tests/headless/widgets.js

## Phase 7: Polish

- [x] T020 [P] sk/cs translations in internal/i18n/catalog
- [x] T021 [P] URS J-02 and J-28…J-31, JOURNAL, README
- [x] T022 run-all green, commit, push

## Dependencies & strategy

Setup → Foundational → US1 (MVP) → US2 → US3 → US4 → Polish. US3 renderers depend only on T005 (register).

# Implementation Plan: Entity aliases, key filters and dashboard states

**Spec**: `specs/030-aliases-key-filters-states/spec.md` · **Version**: 0.73.0 · **ADR**: ADR-0023

## Design

| Part | Where | Notes |
|---|---|---|
| Format 1.2 validation | `internal/httpapi/dashboard_aliases.go`, `dashboards.go` | `validateAlias`, `validateKeyFilter`/`validateRule`, `validateStates`, `validateWidgetAlias`, `validateActions`, `validateDashModel`; widget fields `state`, `alias`, `actions`; parameter kind `keyfilter`; `value` filters in `dashboard_filters.go` |
| Resolution | `internal/httpapi/entities.go` | `aliasCandidates` (SQL per kind, recursive CTE for hierarchies), `convert` (between entity types through `dsOwners`), `loadEntities`, `loadDS` (datastreams by key, units), `latestOf`, `alarmsOf`, `evalFilter`/`evalRule` (pure, unit-tested), `resolve` (filters, key filters, answers as entities, datastreams via `queryDS`/`enrichDS`, or devices) |
| Public scope | `resolveAliases` (called by `shareOf`), `publicEntitiesQuery`, `publicCurrent` | `shareInfo.Ents`; map/devices widgets of an alias no longer open all devices |
| Organization attributes | `getOrgAttributes`, `putOrgAttributes` | `core.organization.settings.attributes`; no migration |
| Renderer | `static/widgets.js` | `aliasParams`/`aliasQuery`, `resolve` with aliases, `runAction`/`passEntity`, states in `render` (cards keep their spec index), chart pick, map markers, device rows, buttons and widget clicks |
| Widgets | `static/widgets-more.js` | `entities_table`, `entity_selector`, overview table highlighting and row actions |
| Navigation | `static/dashfilters.js` | `ctrl32DashStates.mount` (stack in the address, breadcrumb, popstate); value filters in the bar |
| Pages | `dashboards.html`, `index.html`, `public.html` | nav passed to `render`; editor: settings tabs, alias/key-filter/state editors with presets and previews, state tabs, preview entity, Data and Actions tabs |
| Showcase | `dashboard_starter.go` | alias *All devices*, entities table and entity selector |
| OpenAPI | `openapi.go` | 4 operations; `go run ./tools/apigen` |

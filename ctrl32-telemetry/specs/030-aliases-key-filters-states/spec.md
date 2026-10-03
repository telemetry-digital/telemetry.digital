# Feature Specification: Entity aliases, key filters and dashboard states (drill-down)

**Feature Branch**: `feature/0.73`

**Created**: 2026-10-03

**Status**: Implemented (0.73.0)

**Input**: The owner (3. 10. 2026): "filters like ThingsBoard" — named entity aliases shared by widgets (one entity, a
list, by type, by name, an asset hierarchy N levels deep, the current entity of the dashboard state), key filters on
latest values and attributes with dynamic values, dashboard states with drill-down from tables, maps and charts, an
entity selector and an entities table; editors understandable for non-programmers; public links never read outside
their scope.

**Related**: spec 002 (widgets), 0.68.0 (dashboard filters, query sources, overview table — no spec), ADR-0023,
ADR-0008 (screens), ADR-0010 (API first).

## Clarifications (defaults)

- Additive screen format 1.2: `spec.aliases`, `spec.key_filters`, `spec.states`, per widget `alias`, `state`,
  `actions`; a dashboard without them is stored byte for byte as before, the 0.68 query source becomes the alias kind
  *query* (both stay valid).
- Entities are devices, assets, datastreams and sites of the caller's organization (permission `data.read`; there is
  no finer scope than the organization). A device's *type* is its device profile.
- Resolution on the server: at most 2000 candidates per alias (sorted by name), 500 rows per answer, 10 levels of a
  hierarchy. Widgets convert entities to what they show: their datastreams (devices: the assigned ones, assets and
  sites: their own), their devices, or the entities themselves.
- A key filter compares the latest value of a datastream key of the entity (a datastream: its own value), an attribute
  (the entity's, else its asset's, else its site's), the quality, staleness (older than 1.5 × the expected interval,
  or one hour without one; no value counts as stale) and the alarm state. Dynamic values: a *value* filter of the
  dashboard (a number the viewer types), an attribute of the entity itself, of the current entity or of the
  organization; when it is missing the constant applies, without one the condition does not match. Strings compare
  case-insensitively; `between` takes the bounds in any order.
- The current entity and the state stack live in the address (`st=<state>.<type>.<id>.<name>` per level), so a link
  opens the same view and the browser's Back goes up one level.
- Public links: the scope of a shared dashboard grows by everything its aliases select without key filters (the
  entities, all their datastreams and devices); the current entity of a viewer is accepted only within that scope,
  aliases, filters and columns are always taken from the stored spec; actions other than *go to a state* are hidden.

## User Scenarios

1. A pharmacy chain: alias *All fridges* (assets of type fridge), key filter *Above 8 °C*; an overview table bound to
   the alias with the filter shows only the warm fridges, new fridges appear on their own.
2. An entities table of the fridges with columns name, temperature (`t:temp`), limit (`a:max_temp`), alarm; rows above
   their own limit (key filter `temp > attribute max_temp`) are highlighted; a row click opens the state *Fridge
   detail* with the fridge's logger; its trend, logger and alarms follow; Back returns.
3. An entity selector of the fridges switches every widget bound to the *current entity* alias.
4. *Devices of the selected asset* (hierarchy under the current entity, devices) on a detail state; *All sensors of a
   site* (hierarchy under a site, datastreams) on an overview.

## Requirements

- FR-001 Validation of aliases (7 kinds, ≤ 20), key filters (≤ 20, ≤ 12 conditions, one level of groups), states
  (≤ 10, unique ids), widget alias bindings (not on control widgets, not together with a query source), actions
  (≤ 6; events row, row_button, marker, chart, click, button; actions state, device, dashboard; pass self, device,
  asset; buttons need a label), the `value` filter kind and the `keyfilter` parameter kind; references to missing
  aliases, key filters or states are refused; the root state is not stored on widgets; states need the grid layout.
- FR-002 `GET /api/v1/entities/query` (`data.read`): alias and key filters as JSON parameters, `current`, `want`
  (entities, datastreams, devices), `quantity`/`key`, `values`/`attrs` (columns), `f_value`, `follow` + `f_…`,
  `latest`, `from`/`to`, `limit`; entities with latest values, attributes, alarm and `match` (highlight).
- FR-003 `GET /api/v1/public/{token}/entities/query?widget=`: within the dashboard's scope only; `GET/PUT
  /api/v1/organization/attributes` (`data.read` / `config.write`, reason, audit).
- FR-004 Widgets: *Entities table* (columns `name`, `type`, `site`, `asset`, `asset_type`, `device`, `profile`,
  `alarm`, `last_seen`, `t:<key>`, `a:<name>` with labels after `=`; search, sorting, pages, highlighting, row
  actions), *Entity selector* (drop-down or searchable list, chooses the first entity), the overview table with
  highlighting and row actions; every datastream, device and alarm widget can take an alias.
- FR-005 States in the viewer, on the home page and on public links: breadcrumb, Back, address; actions run on row,
  marker, chart and widget clicks and on buttons.
- FR-006 Editor: Dashboard settings with tabs General, Filters, Entity aliases, Key filters, States, Public link;
  presets (*All sensors of a site*, *Devices of the selected asset*, *The selected entity*, *Assets of a type*, *All
  devices*; *Values above a limit*, *Above the entity's own limit*, *Above a limit the viewer types*, *In alarm*, *No
  recent value*); previews of the matches; attributes of the organization; state tabs and a preview entity above the
  grid; Data tab with the alias, key filter, quantities and keys; Actions tab.
- FR-007 OpenAPI and clients (apigen), MCP (`data_read` offers the query automatically), translations in 19 catalogs.
- FR-008 Tests: unit (validation, canonical form, old dashboards unchanged, key filter evaluation), DB (isolation of
  organizations, depth), e2e `aliases`, `states`, headless `aliases_states`; the documentation demo and pages.

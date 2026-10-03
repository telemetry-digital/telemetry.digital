# ADR-0023: Entity aliases and key filters are resolved on the server; dashboard states live in the address

**Status:** accepted 3 Oct 2026 (owner: "filters like ThingsBoard"). Extends ADR-0008 (screens) and the overviews of
0.68.0 (screen format 1.1); spec 030.

## Context

Dashboards of 0.68.0 select datastreams by a query and narrow them with a filter bar. Customers with many similar
objects (fridges, pumps, meters) need more: named sets of entities shared by many widgets, conditions on latest values
and attributes ("fridges above their own limit"), and a detail view that opens with the clicked entity. Two ways were
possible: resolve everything in the browser from the lists the UI already loads (all devices, assets, datastreams,
latest values), or on the server.

## Decision

1. **Resolution on the server.** An alias (one entity, chosen entities, by type, by name, an asset hierarchy, the
   current entity, a query) and a key filter are evaluated by `GET /api/v1/entities/query` within the caller's
   organization, with limits (2000 candidates, 500 rows, 10 levels). The browser never loads all entities of an
   organization to filter them, and public links can be held to their scope.
2. **The spec carries definitions, the request carries only the context.** Signed-in users send the alias and filter
   definitions with the request (the editor previews unsaved changes); a public link sends only the widget id, the
   current entity and the values of public filters — definitions, columns and the highlight come from the stored
   spec.
3. **Public scope.** The scope of a shared dashboard grows by what its aliases select without key filters and without
   a current entity: those entities, all their datastreams and devices. A viewer's current entity is accepted only
   within it; aliases of the current entity therefore never widen the scope. Actions that leave the dashboard (device
   page, another dashboard) are hidden on public links.
4. **States in the address.** The stack of states with their current entities is part of the URL (`st=` per level),
   so a link reproduces the view, Back works, and no server state is needed.
5. **Additive format.** Screen format 1.2 adds `aliases`, `key_filters`, `states` and per widget `alias`, `state`,
   `actions`; older dashboards keep their stored form; the query source of 0.68.0 stays valid next to the alias kind
   *query*.

## Consequences

- One more request per widget with an alias (cached 30 s per card, like query sources); live updates stay per
  datastream.
- Key filters see only the latest values (no history conditions); attributes come from the entity, its asset and its
  site, plus the attributes of the organization (`settings.attributes`, no migration).
- Organization is the only permission scope for entities (there are no per-site permissions); if they come, the
  resolution is the one place to apply them.

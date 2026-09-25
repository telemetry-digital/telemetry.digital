# Research: Dashboard widgets with rich settings

## R1 — Where settings live
- **Decision**: extend the existing catalogue model (`ScreenParam` with `Kind`, `Default`, `Options`) with a `Group`
  (data, appearance, series, time, actions) and new kinds: `color`, `icon`, `series`, `ranges`, `datetime`.
- **Rationale**: one source of truth for the editor, the validator and the API documentation (constitution IV).
- **Alternative**: free-form JSON per widget (ThingsBoard style) — rejected: cannot be validated or documented.

## R2 — Common appearance
- **Decision**: a list of ≈ 15 appearance params appended to every definition at start-up (`withCommon`), defaults equal
  to today's rendering (title shown, card border, no background override).
- **Rationale**: every widget gets the settings without per-type code; old specs validate to the same look.

## R3 — Aggregation
- **Decision**: the observations endpoint already returns avg/min/max/count per bucket; `sum` is computed as avg × count
  on the client; aggregation `none` uses raw points up to 10 000 and otherwise buckets.
- **Rationale**: no server change needed; exact.

## R4 — Icons
- **Decision**: vendor a subset (≈ 60) of Tabler Icons outline SVG paths as a JS map with the MIT licence file
  (fetched from the Tabler GitHub repository at a pinned tag); `icon` params accept only names from that map.
- **Rationale**: ADR-0009 names Tabler as the only icon set; a closed list is validatable server-side.

## R5 — New widget renderers
- **Decision**: a second module `widgets-more.js` registers renderers through `ctrl32Widgets.register(type, fn)`;
  all use the existing data helpers (`datastreams`, `latest`, `series`, `hourly`) and SVG drawing style.

## R6 — Regression safety
- **Decision**: headless probe renders the existing showcase before/after and compares DOM geometry and computed styles
  of every card (screenshots vary with fonts; geometry/styles are stable).

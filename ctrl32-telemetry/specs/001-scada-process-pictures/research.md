# Research: SCADA process pictures and editor

## R1 — Rendering technology

- **Decision**: SVG built as strings, one `<g data-eid>` per element, keyed diff (replace only changed elements).
- **Rationale**: crisp at any zoom, native hit testing, CSS animations for spin/flow/blink, no dependency; replacing
  only changed groups keeps running animations (FR-004). 1500 groups render in well under a second in current
  browsers.
- **Alternatives**: Canvas 2D (Konva, Fabric.js) — faster for tens of thousands of objects but needs own hit
  testing, text layout and a dependency; WinCC-class pictures are hundreds to ~1500 objects. Kept as a later option
  (ADR-0015).

## R2 — Editor engine

- **Decision**: own editor on the same SVG; an overlay SVG draws selection frames and handles; model = the spec JSON.
- **Rationale**: the object model is small; owning the editor gives exact WinCC-like behaviour (handles, point
  editing, align/distribute, z-order, lock) and zero dependencies.
- **Alternatives**: maxGraph (Apache-2.0) — diagram-oriented (vertices/edges), awkward for free drawing and symbol
  dynamics; Fabric.js (MIT) — canvas objects, would duplicate the renderer.

## R3 — Undo/redo

- **Decision**: snapshot stack of the serialised drawing layer + widgets (JSON strings), 100 steps, pushed on
  commit (pointer up, property apply), not on every pointer move.
- **Rationale**: simple and exact; a 1500-element picture serialises to ≈ 300–600 KB, 100 steps ≈ 60 MB worst case —
  acceptable; compress by storing only when changed.

## R4 — Live values

- **Decision**: reuse the existing SSE stream (`/api/v1/stream`) through `widgets.js` `connectLive`; add element tags
  to the subscription; on an observation for an element tag, refresh the latest values of the layer (debounced
  400 ms) and redraw changed elements.
- **Alternatives**: MQTT-over-WS as ADR-0008 planned — the SSE stream already exists, is permission-checked and
  works in public links; MQTT-over-WS stays for the future runtime.

## R5 — SVG import safety

- **Decision**: parse with `encoding/xml`, keep a whitelist of SVG elements (svg, g, defs, path, rect, circle,
  ellipse, line, polyline, polygon, text, tspan, title, desc, linearGradient, radialGradient, stop, clipPath, mask,
  pattern, symbol, use) and presentation attributes; drop `on*`, `style` values with `url(` other than `url(#…)`,
  `@import`, `expression`, `javascript:`; `href`/`xlink:href` only `#fragment`; no `foreignObject`, `script`,
  `image`, `a`, `iframe`, `animate*`, `set`; re-serialise; limit 512 KiB and 20 000 elements. Display via
  `<image href>` (browsers treat SVG in `<image>` as a static document: no scripts, no external resources) and
  serve with `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; sandbox` and
  `X-Content-Type-Options: nosniff`.
- **Rationale**: two independent layers; either alone would stop script execution.
- **Consequence**: imported graphics support geometry, opacity, visibility, blink frame, rotation and movement, but
  not colour mapping of their inner parts (documented).

## R6 — Object library storage

- **Decision**: new table `core.hmi_object` following the ADR-0010 content pattern (slug identity, versions,
  `valid_to`, reason, checksum), kinds `svg` (sanitised SVG text) and `composite` (list of elements with tag
  placeholders). Composites are **copied** into a picture on placement (template semantics), grouped by a `group`
  key; imported SVG is **referenced** (`image` element with the object's slug, latest version).
- **Rationale**: copying keeps pictures self-contained and validated as one document (a picture never changes because
  someone edited a library item); images are referenced to keep pictures small.

## R7 — Symbol state colours

- **Decision**: two schemes — `classic` (WinCC-like) and `isa101` (high-performance HMI); per picture (`spec.scheme`).

## R8 — Existing mimics

- **Decision**: the five widget symbols (tank, pump, valve, pipe, instrument) stay as widget types; pictures without
  `elements` render exactly as today (FR-021). The new starter uses elements.

# ADR-0015: Canvas editors (SCADA pictures, flows) before 1.0, in plain JavaScript

**Status:** proposed 25 Sep 2026 — supersedes ADR-0013 decision 4 in part; ADR-0008 stage S3 moves forward
**Context:** The user asked for a professional SCADA editor (WinCC / VisiWin.NET class: drawing, symbol library,
dynamics, events), for many more dashboard widget settings (ThingsBoard class) and for a Node-RED style flow
editor, with a choice per screen between SCADA drawing, widgets, or both (specs 001, 002, 004). ADR-0013 decision 4
deferred "a rule-chain editor and similar canvases" until the Lit component UI after 1.0; ADR-0008 placed the HMI
editor in stage S3 with an engine under a clean licence (Konva, Fabric.js, maxGraph; tldraw excluded).

## Options

| Option | For | Against |
|---|---|---|
| A. Wait for 1.0 and Lit (ADR-0013 as is) | no second migration of the editors | the most requested functions wait for months |
| B. Migrate to Lit now, then build the editors | editors born as components | weeks of rewrite with no visible gain, blocks features (ADR-0013 rationale) |
| **C. Build the editors now in plain JavaScript** | ships now; same rules as the rest of the UI; screen and flow documents are plain JSON and survive any later port | editors are the largest plain-JS modules; they must be written in the component-ready style of ADR-0013 §2 |

## Decision (proposed)

**Option C.**

1. The SCADA picture editor and viewer (spec 001) are built in plain JavaScript on SVG without a third-party engine:
   the drawing model is small (8 object types), SVG gives crisp scaling and native hit testing, and it keeps the
   dependency count at zero. Konva/Fabric.js remain allowed candidates if a later need (thousands of objects with
   canvas-level performance) appears.
2. The flow editor (spec 004) may use **one** vendored library with a permissive licence for the node canvas.
   Preferred: **Drawflow** (MIT, dependency-free, plain JS); alternatives maxGraph (Apache-2.0), Rete.js (MIT). The
   flow *engine* is Go inside the server (constitution II); Node-RED is not embedded (needs Node.js), paid products
   cannot be bundled (ADR-0002).
3. Editors keep the ADR-0013 §2 discipline: model = plain JSON validated by the server, renderer separate from data
   loading and from the editor, no page-level globals beyond one namespace per module, all strings through i18n.
4. A screen offers the choice the user asked for: dashboard (widgets on a grid), process picture (drawing), or
   process picture with widgets (drawing below, widgets above) — one screen format v1 with an optional drawing layer.

## Consequences

- ADR-0013 decision 4 no longer applies to HMI screens and flows; vector maps (MapLibre) still wait for 1.0.
- The Lit migration after 1.0 will port the viewer first (as Web Components) and the editors last; the JSON
  formats do not change.
- Third-party licence list grows by at most one entry (the flow canvas library), recorded in the SBOM.

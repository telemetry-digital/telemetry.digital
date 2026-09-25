# ADR-0013: Frontend stays plain JavaScript until 1.0; Lit + Tabler is a post-1.0 migration

**Status:** accepted 25 Sep 2026
**Context:** The design (R4, ADR-0001) planned the UI in TypeScript with Lit 3 and the HMI viewer as Web Components. The
product was built page by page in plain JavaScript instead: one `app.js` (navigation, session, i18n by walking text
nodes, toasts, dialogs), one `widgets.js` renderer shared by dashboards, mimics and public links, and about twenty
pages under `internal/httpapi/static`, embedded in the binary. At 0.33.0 the UI covers the whole URS scope for
phases 1–2 with 19 language catalogs and end-to-end and headless tests. A rewrite to Lit + Tabler now would take
weeks, change nothing a user can see, and block feature work; the operators asking for the product ask for
functions (maps without the Internet, Modbus profiles, four-eyes approvals), not for a framework.

## Decision

1. **No framework before 1.0.** Pages remain plain JavaScript modules served from the binary. No build step, no
   bundler, no npm dependency in the server repository; third-party browser libraries stay vendored and pinned
   (Leaflet 1.9.4, Tabler icons) under the licence rules of ADR-0009.
2. **Write for the migration anyway.** New UI code keeps the state that a future component would need as plain JSON
   (screen specs, rule specs, profile specs are already server-side records), keeps rendering separate from data
   loading (`widgets.js` is the model: `render(el, props, ctx)`), avoids page-specific globals, and takes every
   user-visible string through the catalog so a component port needs no re-translation.
3. **Lit + Tabler after 1.0, incrementally.** The migration starts with the widget renderer (the HMI viewer as Web
   Components, which the SCADA path in ADR-0008 needs for embedding screens elsewhere), then the editor, then the
   remaining pages one by one behind the same URLs. Each step must keep the e2e suites and the headless probes green.
4. **Vector maps and other browser-heavy features wait for the same point.** MapLibre for vector PMTiles, a
   rule-chain editor and similar canvases are not added to the plain pages; they arrive with the component UI.

## Consequences

- The server repository stays a single Go module with embedded static files; a release is one binary.
- UI features ship at the current pace; the cost is paid later as a controlled migration with tests already in place.
- Web Components as an integration surface (embedding a screen in a third-party portal) are not available before
  1.0; a public dashboard link (`/p/<token>`) in an iframe covers that need meanwhile.
- Revisit when 1.0 is released or when a feature genuinely cannot be built without a component model.

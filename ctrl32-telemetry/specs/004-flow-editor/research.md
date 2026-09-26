# Research: Visual flow editor

## R1 — Engine
- **Decision**: in-process Go engine `internal/flows`: one goroutine per deployed flow with a bounded queue (1 000
  messages; the oldest are dropped and counted, alarm at the limit), nodes implemented as Go types behind one
  interface (`Process(ctx, msg) → outputs`), a closed registry of node types with a settings schema served to the
  editor (like the widget catalogue). Timers (delay, debounce, windows, schedules) run inside the flow's goroutine
  through a timer heap, so a flow's state is touched by one goroutine only.
- **Rationale**: constitution II (no second runtime), I (audited actions), performance (SC-002: 10 nodes < 50 ms).
- **Alternatives**: embedding Node-RED (Node.js runtime, against II); goja/JS sandbox (user code, clarified "no");
  a generic workflow engine (Temporal etc. — a separate service).

## R2 — Editor canvas
- **Decision**: own plain-JS SVG editor `static/flow-editor.js` in the style of the SCADA editor (ADR-0015): palette,
  nodes with ports, Bézier wires, pan/zoom/grid, rubber-band multi-select, copy/paste, undo/redo, property panel
  generated from the node catalogue, debug panel. No vendored library.
- **Alternatives**: Drawflow (MIT, small, but its DOM model and styling fight the product's i18n and theming),
  Rete.js (MIT, framework-oriented), maxGraph (Apache-2.0, large). The SCADA editor already solved the hard parts.

## R3 — Expression language (FR-004, clarified: expressions only)
- **Decision**: a new typed expression language in `internal/flows/expr.go`: numbers, strings, booleans, null;
  `+ - * / % ^`, comparisons, `&& || !`, `?:`; identifiers `value topic ts quality source` and `msg.<field>`,
  `flow.<name>` (flow memory); functions abs min max round(x[,d]) floor ceil sqrt pow clamp log exp, len lower upper
  trim contains startsWith endsWith replace substr str num bool, now hour minute weekday day month year
  fmtTime(t, layout), if(c, a, b), coalesce(a, b). Limits: source ≤ 1 000 characters, ≤ 300 AST nodes, ≤ 10 000
  evaluation steps, string results ≤ 4 KB. Parsed and checked at deploy (unknown names/functions refused).
- **Alternatives**: expr-lang/expr (MIT, powerful, but a larger surface and reflection over Go values); CEL (Apache,
  heavy dependency); extending the numeric rule language (no strings/time/booleans).

## R4 — Message
`{topic, value (number|string|bool|null), ts, quality, source {kind, id, name}, fields {name → value}}`, at most 16 KB
serialized; `msg.<field>` reads `fields`.

## R5 — Triggers
Datastream values (the ingest sink the rules engine uses), alarms and device connection (the live hub's events),
schedules (interval, daily times on weekdays), incoming webhook (`POST /api/v1/flow-hooks/{token}` per trigger node,
token stored hashed, optional HMAC), manual inject (API, editor). MQTT from a foreign broker: phase 2.

## R6 — Actions
Derived datastream value (the rules engine's deriver), device command (the server's Commander: permissions of the
flow's owner, four-eyes for commands, audit reason "flow <name> v<version> node <id>: <trigger>"), shared attribute,
notification through configured channels, incident open/resolve, HTTP request to allow-listed hosts only
(organization setting), debug. Every action is audited with flow, version, node and triggering message (FR-005).

## R7 — Governance (FR-006, US4)
`core.flow` is versioned content like the HMI library (a deploy is a new row, `valid_to` on the old one, reason,
author); restore = deploy of an old version. Deploying a flow that contains command or connector-write nodes goes
through the existing four-eyes change requests when the organization's `four_eyes_commands` policy is on.
Export/import JSON (validated like a save). An automation rule can be converted to an equivalent flow (read, then
saved as a new flow; the rule remains until the user retires it).

## R8 — Dry run, debug, metrics (FR-008, FR-009)
Per flow a ring buffer of the last 500 debug entries (debug nodes, errors, dry-run actions) in memory, served by
`GET /api/v1/flows/{id}/debug?after=`; test injection runs a message through the deployed or the edited (not yet
deployed) flow; dry run records actions instead of executing them. Per node counters (in, out, errors, last run)
and queue length at `GET /api/v1/flows/{id}/status`, plus Prometheus totals.

## R9 — Validation (SC-004)
Closed schema per node type (settings, ports), wires only output→input between existing nodes, cycles only through a
delay/debounce/rate-limit node, expressions parsed, referenced datastreams/devices/channels exist in the
organization, HTTP hosts on the allow-list, ≤ 200 nodes per flow, ≤ 500 deployed flows per organization.

## R10 — State across restarts
Latches, counters and flow memory are saved in `core.flow_state` (per flow version and node) every 30 s and at
shutdown, and loaded at start; timers (delays, windows) start empty after a restart (documented).

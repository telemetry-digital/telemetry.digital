# Feature Specification: SCADA process pictures and editor

**Feature Branch**: `assistant/jolly-sagan-nzolwc`

**Created**: 2026-09-25

**Status**: Draft

**Input**: User description: "SCADA — the example has a basic process picture but it is really poor. A SCADA editor:
drawing, instruments, events — everything like the most professional SCADA in the world, it should look like WinCC
and VisiWin.NET. Plus more widgets on dashboards with many settings, at least like ThingsBoard. The user must be able
to choose: SCADA, widgets, or both."

**Related**: URS J-03, J-04, J-05 (changed), new J-20…J-27; ADR-0008 (stage S2/S3), ADR-0013 (superseded in part by
ADR-0015, proposed with this spec). Widget catalogue and widget settings are specified separately in
`specs/002-dashboard-widgets`.

## Clarifications

### Session 2026-09-25

- Q: Import of customer SVG graphics (logos, special machines) in this feature, or later? → A: In this feature, with server-side sanitising.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Operator watches the process on a live process picture (Priority: P1)

An operator opens a process picture (for example "Mixer C4") that shows the plant as drawn by an engineer: vessels
with their current level, pumps and motors coloured by state, valves open or closed, pipes with animated flow while
the medium moves, instrument bubbles (TT-101, PT-102) with live values and units. When something is abnormal the
object changes colour and blinks until the value returns to normal. Values update without reloading the page.

**Why this priority**: Watching the process is what a SCADA is for; without it the editor has no purpose. It also
replaces the current poor picture made of rectangular widgets.

**Independent Test**: Load a stored picture that binds datastreams, publish new values for them, and verify that
levels, colours, animations and texts change within the live-update delay, with no page reload.

**Acceptance Scenarios**:

1. **Given** a picture with a tank bound to a level datastream (0–100 %), **When** a value of 65 arrives,
   **Then** the tank shows a liquid level at 65 % of its height and the tooltip shows "65.0 %".
2. **Given** a pump whose state datastream maps 1 = running, 0 = stopped, 2 = fault, **When** the value changes from
   0 to 1 to 2, **Then** the pump is shown stopped, then running (and its rotating parts turn), then in the fault
   colour.
3. **Given** a pipe whose flow condition is "flow > 0.5", **When** the flow is 3.2, **Then** the pipe shows moving
   flow marks in the direction it was drawn; **When** the flow is 0, **Then** the marks stop.
4. **Given** an object with a blink condition "temperature > 80" and an alarm colour, **When** the temperature
   rises to 82, **Then** the object is framed in the alarm colour and blinks; **When** it falls to 78, blinking stops.
5. **Given** a datastream with no value yet, **When** the picture opens, **Then** bound texts show "—" and no
   object pretends to have a state.

---

### User Story 2 - Engineer draws a process picture in a professional editor (Priority: P1)

An engineer with the content-editing permission creates a new process picture and draws it: picks symbols from a
categorised library (vessels, pumps and drives, valves, heat transfer, process equipment, instruments,
electrical), draws pipes as polylines with corners, adds lines, rectangles, ellipses, polygons and texts, and places
them precisely. The editor behaves like a desktop SCADA editor: select, multi-select with a rubber band,
move, resize with handles, rotate, snap to a grid, align and distribute, bring to front / send to back,
copy/paste/duplicate, undo/redo, zoom and pan, lock objects, nudge with the arrow keys, delete. Every save is a
new version with a reason.

**Why this priority**: Without a proper editor no customer picture can be built; this is the core of the request
("editor scady … kreslenie").

**Independent Test**: With the editor alone, draw a small picture (one tank, one pump, one valve, two pipes, three
texts), use undo/redo, align and copy/paste, save with a reason, reload, and verify the picture is identical and
its version increased.

**Acceptance Scenarios**:

1. **Given** an empty picture, **When** the engineer drags a "centrifugal pump" from the library onto the canvas,
   **Then** the pump appears at the drop point with its default size and is selected.
2. **Given** the pipe tool, **When** the engineer clicks four points and finishes with a double-click or Enter,
   **Then** a pipe with three segments exists and each corner can be moved later.
3. **Given** three selected objects, **When** the engineer chooses "align left", **Then** their left edges share
   the same x coordinate; **When** the engineer presses undo, **Then** they return to their previous positions.
4. **Given** a selection, **When** the engineer copies and pastes, **Then** duplicates appear offset by one grid
   step with new identities and the same bindings.
5. **Given** unsaved changes, **When** the engineer tries to leave the editor, **Then** the browser asks for
   confirmation.
6. **Given** a save with an empty reason, **Then** the save is refused; with a reason, **Then** a new version is
   stored and the audit trail shows who saved it, when and why.

---

### User Story 3 - Engineer makes objects dynamic and binds them to data (Priority: P1)

For a selected object the engineer binds a datastream (tag) and configures dynamics in a property panel, without
writing code: colour mapping (value ranges → colours), state mapping (value → running/stopped/fault/manual/
open/closed/transit), level (value range → fill 0–100 %), visibility, blinking, rotation of moving parts, flow
animation, text (value with decimals and unit, or value → text mapping), opacity, angle and movement. Each dynamic
may use the object's tag or another tag.

**Why this priority**: A picture without dynamics is a static drawing; dynamics are what makes it SCADA.

**Independent Test**: Configure each dynamic type on a test picture, publish values that hit and miss each
condition, and compare what is shown with the expected result table.

**Acceptance Scenarios**:

1. **Given** a rectangle with the colour mapping "< 10 → blue, 10–30 → green, ≥ 30 → red", **When** values 5, 20 and
   40 arrive, **Then** its fill is blue, green, red.
2. **Given** a text "{value} {unit}" bound to a temperature with 1 decimal, **When** 21.456 °C arrives, **Then** it
   reads "21.5 °C".
3. **Given** a visibility condition "valve position > 0", **When** the position is 0, **Then** the object is hidden
   in operation but still visible (dimmed) in the editor.
4. **Given** an invalid dynamic (for example an empty input range for a level), **When** saving, **Then** the
   server refuses the picture and names the element and the problem.

---

### User Story 4 - Operator interacts: faceplates, trends, screen changes, commands (Priority: P2)

Clicking an object opens what the engineer configured: a faceplate (name, value, unit, quality, time, a trend over
1 h / 6 h / 24 h / 7 d, and — when a device is bound and the user may send commands — Start/Stop and setpoint
controls), a trend window, another process picture, or a predefined command (for example "reset pump P-101").
Every command asks for a reason, may ask for confirmation, respects the four-eyes policy and appears in the audit
trail. Objects that react to clicks show a pointer cursor and a tooltip.

**Why this priority**: Operation from the picture is expected from a SCADA, but watching (US1) already delivers
value without it.

**Independent Test**: On a picture with one object per action type, click each object and verify the dialog,
navigation, command request (with reason, audit entry and acknowledgement) and permission handling.

**Acceptance Scenarios**:

1. **Given** a pump bound to a state tag and a device with key "run", **When** an operator with command
   permission clicks it, enters a reason and presses Start, **Then** a command `run = true` is created with that
   reason and the faceplate shows its status (queued, acked, pending approval, failed).
2. **Given** the same pump, **When** a user without command permission clicks it, **Then** the faceplate shows
   values and trend but no controls, and says why.
3. **Given** an object with "go to screen boiler-house", **When** it is clicked, **Then** the "boiler-house"
   picture opens.
4. **Given** a picture opened through a public read-only link, **When** any object is clicked, **Then** values and
   trends may be shown but no command can be sent and no screen outside the link opens.

---

### User Story 5 - Choose SCADA, widgets, or both on one screen (Priority: P2)

When creating a screen, the user chooses its kind: a dashboard of widgets on a grid, a process picture (SCADA
drawing), or a process picture that also contains dashboard widgets (for example a trend and an alarm list next to
the drawing). Both kinds live in the same list, share the versioning, public links and default-screen setting,
and a process picture can be the home screen.

**Why this priority**: Explicitly requested ("aby bolo na výber: scada alebo widgety alebo oboje"); it builds on US1–US2.

**Independent Test**: Create one screen of each kind, set each as default in turn, open each through a public link.

**Acceptance Scenarios**:

1. **Given** the "New screen" dialog, **When** the user picks "Process picture with widgets", **Then** the editor
   offers both the drawing tools and the widget catalogue on one canvas.
2. **Given** a process picture containing a trend widget and drawn objects, **When** it is viewed, **Then** the
   widget and the drawing both update live and the drawing stays behind the widgets.

---

### User Story 6 - Engineer reuses work: object library, templates, colour schemes (Priority: P3)

The engineer groups objects into a reusable composite ("pump station" = pump + two valves + pressure bubble), stores
it in the organization's object library, and places it on other pictures, re-binding its tags in one step. The
picture has a colour scheme — "classic" (WinCC-like: green running, grey stopped, red fault) or "ISA-101
high-performance" (grey process, colour only for abnormal states) — and a background colour.

**Why this priority**: Productivity for larger plants; pictures can be built without it.

**Independent Test**: Create a composite, place it twice with different tags, switch the scheme, verify colours.

**Acceptance Scenarios**:

1. **Given** a stored composite with three tags, **When** it is placed and the engineer maps the three tags to new
   datastreams, **Then** all inner dynamics use the new datastreams.
2. **Given** the "ISA-101" scheme, **When** a pump runs normally, **Then** it is drawn in grey tones; a fault is still red.

---

### Edge Cases

- A tag bound to an object is retired or its datastream deleted: the object shows "—", the editor marks the
  broken binding, saving is still allowed.
- Values with bad quality (sensor error, stale): the value is shown with a quality marker; dynamics use the value
  but the faceplate names the quality.
- A picture larger than the screen (up to 4000 × 3000 px): it scales to the width in view mode; the editor zooms
  and scrolls.
- Up to 1500 drawn objects and 60 widgets per picture; more is refused with a clear message.
- Two engineers edit the same picture: the second save creates a newer version; no silent overwrite of history.
- An object references a symbol that a later release no longer contains: it is drawn as a red placeholder with its
  name, never dropped.
- Stored pictures are data only: text containing markup is shown literally; colours must be colour values;
  navigation targets must be screen names; there is no free URL or script action.
- Phones and tablets: pictures can be viewed and operated (tap = click); editing requires a desktop-size screen.
- Live updates interrupted: the picture keeps the last values, shows that live updates are disconnected and
  reconnects automatically.

## Requirements *(mandatory)*

### Functional Requirements

**Viewing (US1)**

- **FR-001**: The system MUST render process pictures as vector graphics that scale without loss to the available
  width, keeping the picture's aspect ratio.
- **FR-002**: The system MUST provide drawn object types: rectangle, ellipse, line, polyline, polygon, pipe, text and
  library symbol; each with position, size, rotation, stroke, fill, line style, opacity, corner radius, shading
  (metallic gradient) and drop shadow where applicable.
- **FR-003**: The system MUST ship a library of at least 50 industrial symbols in at least 8 categories (vessels,
  pumps and drives, valves, heat transfer, process equipment, instruments, electrical, other), drawn consistently
  and supporting state colour, level, rotating parts and labels where meaningful.
- **FR-004**: The system MUST update bound objects when new values arrive, re-drawing only objects whose
  appearance changed, so running animations are not restarted.
- **FR-005**: Pipes MUST show animated flow in drawing direction when their flow condition holds.

**Editing (US2)**

- **FR-006**: Users with content-editing permission MUST be able to create, edit, save (new version with reason)
  and retire process pictures; all other users MUST only view them.
- **FR-007**: The editor MUST offer: select, multi-select (shift-click and rubber band), move, resize with 8
  handles, rotate, point editing for lines/polylines/polygons/pipes, snap to a configurable grid, zoom (fit,
  25–400 %) and pan, align (left/centre/right/top/middle/bottom), distribute (horizontal/vertical), same size,
  z-order (front/back/forward/backward), copy/cut/paste/duplicate, delete, lock/unlock, hide in operation,
  undo/redo (at least 100 steps) and keyboard shortcuts for all of these.
- **FR-008**: The editor MUST show a layer list (all objects in z-order with name and type) where objects can be
  selected, renamed, locked and hidden.
- **FR-009**: The editor MUST offer a symbol library panel with categories, search and previews, placing symbols
  by click or drag.
- **FR-010**: The editor MUST offer a property panel for geometry, appearance, text, binding, dynamics and events
  of the selection; changes MUST be visible immediately on the canvas with live values.
- **FR-011**: The editor MUST warn before leaving with unsaved changes.
- **FR-012**: Picture settings MUST include name, canvas size (600–4000 × 300–3000 px), background colour, grid size,
  colour scheme, default time range and refresh interval, default-screen flag and public link.

**Dynamics (US3)**

- **FR-013**: Each object MUST support up to 8 dynamics of these kinds: fill/stroke/text colour mapping, state
  mapping, text (formatted value or value→text mapping), visibility, blinking (optionally with an alarm-colour
  frame), rotation of moving parts (speed), flow, level (linear range), opacity, angle and movement in x/y
  (linear range). Conditions use the operators >, ≥, <, ≤, =, ≠; mappings are ordered, first match wins.
- **FR-014**: Each dynamic MUST use the object's tag unless it names another tag.
- **FR-015**: Texts MUST support placeholders for value, unit, object name and value time; value formatting MUST
  honour the configured number of decimals and unit (datastream unit by default).
- **FR-016**: The server MUST validate every stored picture against a closed schema (object types, property names,
  numeric ranges, colour format, identifiers, action types) and refuse invalid pictures with a message naming the
  element and field.

**Interaction (US4)**

- **FR-017**: Objects MUST support click actions: open faceplate, open trend, go to another screen, send a
  predefined command. Objects with a tag and no configured action MUST open the faceplate.
- **FR-018**: Every command from a picture MUST require a reason, respect the command permission and the four-eyes
  policy, be recorded in the audit trail and show its status until acknowledged, failed or expired.
- **FR-019**: In public read-only links, pictures MUST NOT send commands and MUST only read the datastreams the
  picture binds.

**Screen kinds (US5)**

- **FR-020**: Users MUST be able to create a dashboard (widgets on a grid), a process picture (drawing), or a
  process picture with widgets; widgets on a process picture sit above the drawing.
- **FR-021**: Existing dashboards and existing process pictures MUST keep working unchanged after the upgrade.

**Reuse (US6)**

- **FR-022**: Users MUST be able to group objects, store a group in the organization's object library (versioned,
  with reason), place it on any picture and re-bind all its tags in one dialog.
- **FR-023**: Pictures MUST support the colour schemes "classic" and "ISA-101"; symbol state colours follow the scheme.
- **FR-024**: Users MUST be able to import their own vector graphics into the object library, now: imported SVG is sanitised on the server (no script, event attributes, foreign or external references,
  embedded HTML), limited in size, stored as versioned organization content and usable like a built-in symbol.

**Quality**

- **FR-025**: Every user-visible string of the viewer and editor MUST be translatable; Slovak and Czech MUST be
  provided with the feature.
- **FR-026**: The feature MUST ship with a demonstration process picture (a mixing plant with vessels, pumps,
  valves, pipes, heat exchanger and instruments) bound to the demo data.

### Key Entities

- **Screen**: a versioned document owned by an organization — name, kind (dashboard / process picture), layout,
  settings (range, refresh, background, grid, scheme), widgets and drawn objects; each version has author, time,
  reason and checksum.
- **Drawn object**: identity, type, geometry, appearance, optional symbol, optional tag (datastream) and device
  binding, dynamics, click actions, name, lock and hidden flags.
- **Dynamic**: property kind, optional tag, condition or mapping or linear range, options (colour, speed).
- **Click action**: faceplate, trend, screen change (target screen), command (key, value, method, confirmation).
- **Library composite**: a named, versioned group of drawn objects with a list of tag placeholders.
- **Symbol**: a built-in vector drawing with a category, default size and the dynamic parts it supports.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An engineer who knows a desktop SCADA editor draws the demo mixing picture (≈ 40 objects, 15 bindings)
  in under 30 minutes without documentation.
- **SC-002**: A new value is visible on an open picture within 2 seconds of arriving at the server, for pictures with
  up to 1500 objects.
- **SC-003**: A picture with 1500 objects opens in under 3 seconds on a current office laptop and scrolls/zooms in
  the editor without visible stutter.
- **SC-004**: 100 % of invalid pictures in the regression set (unknown types, bad colours, markup, foreign
  navigation targets, empty ranges) are refused by the server; 0 % of valid pictures are refused.
- **SC-005**: Every command sent from a picture appears in the audit trail with user, time, reason and result
  (100 % in the e2e suite).
- **SC-006**: All existing dashboards and pictures of the demo instance render identically before and after the upgrade.

## Assumptions

- Engineers use a desktop browser (≥ 1280 px wide) to edit; operators may use desktops, tablets or phones.
- Symbols are drawn by the project (original artwork, no third-party symbol packs), so no licence issue arises.
- The picture engine is built without a framework (constitution VII); a vendored editing library is not required
  for the scope above. If one is adopted later it follows ADR-0015.
- Alarm-driven appearance uses the object's own conditions; linking objects to alarm rules (alarm state as a
  source) is a later refinement.
- Real-time requirements below one second and redundancy of the process connection are out of scope (ADR-0008 S4).
- Multi-language texts inside pictures (per-language labels) are out of scope; UI chrome is translated.

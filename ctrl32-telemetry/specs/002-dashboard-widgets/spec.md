# Feature Specification: Dashboard widgets with rich settings

**Feature Branch**: `assistant/jolly-sagan-nzolwc`

**Created**: 2026-09-25

**Status**: Draft

**Input**: User description: "…plus in dashboards: add more widgets and give them many settings, at least as many as
ThingsBoard has."

**Related**: URS J-02 (extended), new J-28…J-31; spec 001 (widgets can be placed on process pictures too).

## Clarifications

### Session 2026-09-25

- Q: Refuse unknown widget settings (FR-006)? → A: No — refuse invalid values, drop unknown names with a warning
  in the response; legacy dashboards stay saveable (found while planning).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Every widget can be styled and titled like in ThingsBoard (Priority: P1)

A dashboard author opens any widget's properties and, beside its data settings, finds a common "Appearance" section:
show/hide title, title text, title icon, title and value font size, text and background colour, padding, border
(colour, width, radius), drop shadow, transparent card, tooltip text, and "open full screen" on the widget header.
The same section exists for every widget type and applies immediately.

**Why this priority**: The request names settings first ("aby mali veľa nastavení"); a common section multiplies
the value of all 20 existing widget types at once.

**Independent Test**: Change each appearance setting on one widget of every type and verify the rendered card.

**Acceptance Scenarios**:

1. **Given** a gauge widget, **When** the author sets background #0f172a, text colour #f8fafc, hides the title and
   removes the border, **Then** the widget renders as a dark borderless tile without a header.
2. **Given** any widget with "full screen" enabled, **When** the viewer clicks the full-screen icon, **Then** the
   widget fills the screen and returns on Escape.

---

### User Story 2 - Per-series data settings and time window per widget (Priority: P1)

For widgets showing several datastreams the author sets, per series: label, colour, unit override, decimals,
line/bar style, axis (left/right), show points, fill, and hidden-by-default. Each time-based widget can override the
dashboard time range (last 1 h … 90 d, or a fixed from–to), choose aggregation (none, average, min, max, sum, count)
and the aggregation interval. The legend can be placed top/bottom/right and show min, max, average, total and latest.

**Why this priority**: Without per-series control, multi-signal trends (the main use) cannot be made readable.

**Independent Test**: Configure a trend with three series and two axes; compare labels, colours, axes, legend
values and aggregation with the stored data.

**Acceptance Scenarios**:

1. **Given** a trend with temperature (°C, left axis) and humidity (%, right axis), **When** rendered, **Then** two
   y-axes with their own scales and units are shown and each series uses its configured colour and label.
2. **Given** aggregation "max" per 1 h over 7 days, **When** rendered, **Then** each point equals the maximum of
   the raw values in that hour.
3. **Given** legend values min/max/avg, **When** rendered, **Then** the legend shows these numbers for the visible
   range with the configured decimals.

---

### User Story 3 - More widget types (Priority: P2)

The catalogue grows with the widget types operators know from ThingsBoard and SCADA dashboards: value card with
icon and trend arrow, aggregated value card (min/max/avg/sum over the range), progress bar (horizontal/vertical),
liquid-level card, thermometer, battery level, signal strength, compass / wind direction, radial gauge with ticks
and coloured bands, linear gauge, LED/indicator grid, state timeline (value → coloured bands over time), bar chart
over time (grouped/stacked), pie and polar-area chart, radar chart, time-series table (rows per timestamp),
entity table (latest values of many datastreams with sort and search), alarm count card, clock, markdown/text
card with value placeholders, image card, QR code, navigation card (link to another screen), knob / slider /
round switch control widgets (commands with reason).

**Why this priority**: Breadth matters for adoption, but styling and data settings (US1–US2) matter first.

**Independent Test**: Place one widget of every new type on a showcase dashboard bound to demo data; each renders
without error and responds to live values.

**Acceptance Scenarios**:

1. **Given** a state timeline bound to a pump state (0/1/2), **When** rendered for 24 h, **Then** coloured bands
   show when the pump was stopped, running and in fault, with a legend.
2. **Given** a slider control bound to a device key with min 0, max 100, step 5, **When** the operator moves it to
   40 and confirms with a reason, **Then** a command `key = 40` is sent and audited.

---

### User Story 4 - Conditional formatting and click actions (Priority: P3)

The author defines conditional formatting per widget (value ranges → colour of value, background or icon) and a
click action (open another screen, open the datastream trend, open a faceplate).

**Why this priority**: Adds operator guidance; builds on US1–US3.

**Independent Test**: Configure ranges and an action on a value card, publish values across ranges, click it.

**Acceptance Scenarios**:

1. **Given** ranges "< 2 → blue, 2–8 → green, > 8 → red background", **When** 9.1 arrives, **Then** the card
   background is red.

---

### Edge Cases

- Settings stored by an older version stay valid; new settings have defaults, so old dashboards look the same.
- A colour or text setting with unsafe content (markup, CSS expressions) is refused by the server.
- Right-axis series without a unit: axis labelled by the series label.
- Aggregation interval too small for the range (e.g. 1 min over 90 d): the interval is raised to keep at most
  ~2000 points and the widget says so.
- Control widgets in public links or without permission: rendered read-only with an explanation.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Every widget type MUST support the common appearance settings of US1.
- **FR-002**: Multi-series widgets MUST support per-series label, colour, unit, decimals, style, axis, points,
  fill and hidden-by-default.
- **FR-003**: Time-based widgets MUST support their own time window (relative or fixed), aggregation function and
  interval, and legend position with min/max/avg/total/latest.
- **FR-004**: The catalogue MUST add at least the 24 widget types listed in US3.
- **FR-005**: Widgets MUST support conditional formatting (up to 8 ranges) and one click action.
- **FR-006**: The server MUST validate every widget setting against the catalogue (type, range, allowed values,
  colour format, text length); invalid values are refused, and unknown settings are dropped and reported back as
  warnings in the save response (so dashboards stored by older versions can still be saved).
- **FR-007**: Existing dashboards MUST render unchanged after the upgrade (defaults equal the current look).
- **FR-008**: The property panel MUST group settings into tabs (Data, Appearance, Series, Time, Actions) and offer
  a live preview; every label MUST be translatable (Slovak and Czech provided).
- **FR-009**: Control widgets MUST send commands with reason, confirmation, permission and audit like the existing
  setpoint/switch/button widgets.
- **FR-010**: The showcase starter MUST contain every widget type.

### Key Entities

- **Widget setting definition**: name, kind (text, number, colour, boolean, choice, list, series list, range
  list), default, allowed values; part of the widget catalogue served by the server.
- **Series settings**: per bound datastream — label, colour, unit, decimals, style, axis, flags.
- **Conditional format rule**: condition (operator, value or range) → target (value colour, background, icon) and colour.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The widget catalogue offers at least 44 widget types (20 existing + 24 new).
- **SC-002**: Every widget type exposes at least 15 settings (common appearance included).
- **SC-003**: A showcase dashboard with one widget of every type loads in under 3 seconds with demo data.
- **SC-004**: 100 % of demo dashboards look identical before and after the upgrade (headless screenshot comparison).

## Assumptions

- Charts are drawn by the existing in-house SVG renderer, extended; no charting library is added.
- "Like ThingsBoard" means comparable breadth of settings, not ThingsBoard's JSON format or its JavaScript
  post-processing functions (user scripts are excluded by the constitution, principle III).
- Icons come from the Tabler icon set already allowed by ADR-0009.

# Feature Specification: Energy overview and scenes

**Feature Branch**: `assistant/jolly-sagan-nzolwc`

**Created**: 2026-09-26

**Status**: Implemented (0.43.0)

**Input**: User description: "Pokračujeme" (continue) — the next items of the phase-2 list in the status report: an
energy overview (consumption per day from energy entities) and scenes (groups of entity actions).

**Related**: specs 005/006 (smart home), 004 (flows), URS A-30, A-31; constitution I (records), III, VII.

## Clarifications

### Session 2026-09-26
Defaults chosen (the user asked to continue without questions):
- Q: Which meters? → A: Every smart-home entity with device_class energy and every datastream in Wh, kWh or MWh;
  roles consumption, production or not counted; a device with several energy sensors counts the one named "total".
- Q: How is consumption computed from cumulative meters? → A: Sum of increases between consecutive readings; a drop
  below half of the previous reading is a meter reset (counts the new reading), a smaller drop counts nothing.
- Q: Scenes and four eyes? → A: Activating a scene issues ordinary entity commands of the user; the organization's
  four-eyes policy applies to each.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See how much energy the home or plant uses (Priority: P1)
The user opens Energy and sees today's consumption per hour, the cost at their price, the current power, and which
meter used how much; switches to week, month or year and browses back.

**Independent Test**: a kWh meter fed with 49 readings +0.5 kWh every 30 min → 24 kWh over the last day.

### User Story 2 - Production (Priority: P2)
A solar inverter's meter marked as production appears as a second series; produced energy can be credited at a
feed-in price.

### User Story 3 - Scenes (Priority: P1)
The user sets the hall light to 30 % warm and the plug on, chooses "+ Scene", selects both and saves "Evening"; later
one click on "Evening" restores them. Scenes can also be written as explicit actions and activated by flows.

### Edge Cases
- Meters without readings in the period show 0.
- Periods follow the organization's time zone (days start at local midnight, DST included).
- A light without color_mode reported: the mode is inferred from the single colour field of the message.
- Deleting a scene used by a flow: the flow's node fails with "scene not found".

## Requirements *(mandatory)*
- **FR-001**: `GET /api/v1/energy` with period day/week/month/year and a date: buckets (hour/day/month), totals,
  cost, per-meter kWh and share, current power (sum of power entities).
- **FR-002**: Energy settings per organization (price, feed-in price, currency, meter roles), audited.
- **FR-003**: Energy page with period switcher, navigation, tiles, bar chart, table, settings dialog.
- **FR-004**: Scenes: create (explicit actions or capture of current states), change, delete (reason, audit), list,
  activate (one entity command per action, results per action).
- **FR-005**: Scenes on the Home page (activate, create, edit, delete) and a flow node "Scene".

## Success Criteria *(mandatory)*
- **SC-001**: The energy of a day is exact to 0.001 kWh for a meter with regular readings.
- **SC-002**: A scene with 5 actions is activated with one click; every action is an audited command.

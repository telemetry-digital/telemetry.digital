# Feature Specification: Smart home phase 2 — native devices, MQTT flow nodes, colour, outage incidents

**Feature Branch**: `assistant/jolly-sagan-nzolwc`

**Created**: 2026-09-26

**Status**: Implemented (0.42.0)

**Input**: User description: "Pokračuj" (continue) after spec 005, with the phase-2 list proposed in the status report:
MQTT in/out flow nodes, Shelly Gen2/Gen3 native support, the Tasmota native discovery format, a colour and colour
temperature picker on light cards, and an incident when a bridge stays disconnected.

**Related**: spec 005 (smart home), spec 004 (flows), URS A-20…A-26, C-07 (incidents); constitution I–VII.

## Clarifications

### Session 2026-09-26

The user asked to proceed without further questions; these defaults were chosen:

- Q: How are native devices modelled? → A: The adapters synthesize Home Assistant discovery configs, so native devices
  become ordinary devices and entities (adoption, inbox, commands, history, flows) without a second entity model.
- Q: Which Shelly generations? → A: Gen2 and later (Plus, Pro, Mini, Gen3, Gen4), which speak JSON-RPC over MQTT.
  Gen1 (`shellies/…`) is out of scope (it has its own HA discovery add-ons and is end of life).
- Q: Default outage limit? → A: 10 minutes, per source, 0 disables it. Accounts count only once they have entities
  (a new account without devices is not an outage).
- Q: May MQTT out publish anywhere? → A: Only inside the chosen source (the account's namespace or the bridge's
  broker), governed like device commands (flows with it count as commanding flows: four eyes, audit).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Shelly devices appear by themselves (Priority: P1)

A Shelly Plus 1PM configured with the account's MQTT settings appears within seconds as a device with a switch, power,
energy, voltage, current and temperature, and an input; switching it on the Home page switches the relay.

**Independent Test**: `mqttsim shelly` against a new account → entities; `turn_on` → `Switch.Set` RPC, state `on`.

### User Story 2 - Tasmota without extra settings (Priority: P1)

A Tasmota device with default settings (`SetOption19 0`, native discovery) appears with its relays, lights and
sensors; no console command is needed.

**Independent Test**: `mqttsim tasmota -native` → switch and sensors; `turn_on` → `cmnd/…/POWER ON`, state from
`stat/…/RESULT`.

### User Story 3 - Colour on light cards (Priority: P2)

A colour light shows a colour picker and a colour-temperature slider on its card; changes are commands like any other
and the card shows the colour the light reports.

### User Story 4 - Raw MQTT in flows (Priority: P2)

A flow starts on MQTT messages of a topic filter of an MQTT account or bridge (with JSON fields as message fields) and
publishes messages there (for devices without discovery, or scenes over plain MQTT).

### User Story 5 - Know when a bridge is down (Priority: P2)

When a bridge (or an account with devices) stays disconnected longer than its limit, an incident opens and the
administrators are notified; it resolves itself when the connection is back.

### Edge Cases

- Shelly reports partial `NotifyStatus` updates → merged with the last full status before the entities see them.
- Shelly button inputs (state `null`) → no binary sensor (they report events, not states).
- Tasmota `stat/…/RESULT` for other commands (e.g. `{"Dimmer":20}`) → does not change a relay's state.
- Tasmota colours in hex or, with `SetOption17 1`, decimal.
- Lights using kelvin (`color_temp_kelvin`) → converted to and from mireds.
- A user renames an entity and the device announces its config again → the user's name stays.
- A flow's MQTT source is deleted or not yet running → the subscription waits; bridges resubscribe after reconnecting.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Recognize Shelly Gen2+ devices by `<prefix>/events/rpc` notifications from `shelly…` or by
  `<prefix>/status/<component>:<n>` topics; ask `Shelly.GetDeviceInfo` and `Shelly.GetStatus` over MQTT RPC.
- **FR-002**: Synthesize entities for Shelly components switch, cover, light, rgb, rgbw, input (with state), pm1, em1,
  temperature, humidity, devicepower, illuminance, voltmeter; commands as RPC calls (`Switch.Set`, `Cover.Open/Close/
  Stop/GoToPosition`, `Light.Set`, `RGB(W).Set`).
- **FR-003**: Recognize Tasmota native discovery (`tasmota/discovery/<MAC>/config`, `…/sensors`); relays (switch, or
  light with `SetOption30`), lights (dimmer, CT, RGB), numeric sensor fields with units; removal by an empty config.
- **FR-004**: Light entities report their colour controls (RGB, colour temperature and its range); the Home page shows a
  colour picker and a colour-temperature slider; template-schema lights read red/green/blue templates.
- **FR-005**: Templates support string methods (`split`, `strip`, `startswith`, `endswith`, `lower`, `upper`,
  `replace`), slices, `join` and `int(default, base)`.
- **FR-006**: Flow nodes MQTT in (trigger; topic filter with `+`/`#`; JSON objects become message fields, nested
  objects `a_b`) and MQTT out (topic and payload templates, retain), validated against the organization's sources.
- **FR-007**: An incident of kind `mqtt_source` opens when a bridge, or an account with entities, is disconnected longer
  than `outage_incident_min` (default 10, 0 = never); it resolves when the source is connected again.
- **FR-008**: A renamed entity keeps its name when its device announces the config again.

### Key Entities

- **Synthesized config**: a discovery config generated by an adapter, stored with the discovered device under
  `<prefix>/<component>/ctrl32native_<device>/<object>/config`, never published.
- **Outage watch**: per source, in memory: since when disconnected, whether an incident is open.

## Success Criteria *(mandatory)*

- **SC-001**: A Shelly Gen2 or a default Tasmota device shows its entities within 10 seconds of connecting.
- **SC-002**: 100 % of adapter commands are audited device commands acknowledged by the reported state.
- **SC-003**: An outage longer than the limit is an incident within the limit + 10 seconds; it resolves within
  10 seconds after reconnection.

## Assumptions

- Shelly Gen2 API as documented in 2026 (RPC over MQTT enabled, the default when MQTT is on).
- Tasmota 12+ native discovery (`ver` 1).

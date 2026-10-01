# Feature Specification: MQTT provisioning and smart-home devices (Home Assistant style)

**Feature Branch**: `main`

**Created**: 2026-09-26

**Status**: Draft

**Input**: User description: "pokračuj a rob a chcelo by to aj provisioning na mqtt a všetko okolo toho aby sa dalo
používať ako home assistant" (continue, and it needs provisioning over MQTT and everything around it, so that it can be
used like Home Assistant).

**Related**: URS A-02…A-04 (broker, ACL), A-12 (provisioning), A-14…A-16 (MQTT out, webhooks), C-08 (commands),
C-12…C-17 (flows), J (dashboards); protocol v0.1 §3.7 (claim codes); constitution I (records), II (one binary),
III (secure by default).

## Clarifications

### Session 2026-09-26

The user asked to proceed without further questions; these defaults were chosen and can be revisited:

- Q: Which device ecosystems? → A: Everything that speaks **Home Assistant MQTT discovery** (zigbee2mqtt, Tasmota with
  `SetOption19 1`, ESPHome with MQTT, Z-Wave JS UI, OpenMQTTGateway, Frigate, many DIY devices) plus devices built on
  the **ThingsBoard device API** (provisioning, telemetry, attributes, RPC). Shelly Gen2 native RPC and the Tasmota
  native discovery format: phase 2.
- Q: Where do such devices connect? → A: Either to the ctrl32 broker with their own MQTT account (isolated namespace
  per account), or ctrl32 connects as a client to an existing broker (e.g. the Mosquitto of an existing zigbee2mqtt or
  Home Assistant installation) — a "bridge".
- Q: Are discovered devices added automatically? → A: Configurable per account/bridge; default **automatic** for
  accounts on the ctrl32 broker (the owner connected them on purpose) and **inbox** (adopt/ignore) for bridges.
- Q: Does ctrl32 also publish its own data to Home Assistant? → A: Yes, optional per bridge (P3).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Devices provision themselves over MQTT or HTTP (Priority: P1)

An installer flashes 50 identical sensors with one **provisioning key and secret**. On first start each sensor
connects to the broker as `provision`, publishes its name to `/provision/request` and receives its own access token on
`/provision/response`; it reconnects with the token and starts sending telemetry. The organization sees 50 new devices
in the chosen site/room, each with datastreams created from the keys it sends. Devices built for ThingsBoard work
unchanged (same topics, token as MQTT user name, RPC and attributes).

**Why this priority**: Fleet onboarding without typing a claim code per device; the core of "provisioning over MQTT".

**Independent Test**: Create a provisioning profile, run a ThingsBoard-style client: provision, reconnect with the
token, publish telemetry, receive an RPC command and answer it; verify device, datastreams, values, command status and
audit.

**Acceptance Scenarios**:

1. **Given** a profile with strategy "create new devices", **When** an unknown device provisions with the right key
   and secret, **Then** a device is created with a token, the response is `{"status":"SUCCESS","credentialsType":
   "ACCESS_TOKEN","credentialsValue":"…"}` and the event is audited.
2. **Given** a wrong secret, **Then** the response is `{"status":"FAILURE"}`, no device is created, a security event is
   logged, and repeated attempts are rate-limited.
3. **Given** strategy "pre-provisioned only", **When** a name that does not exist provisions, **Then** it fails; a
   pre-registered name succeeds once and a second attempt for the same device fails.
4. **Given** "approval required", **Then** the device is created disabled and appears in the approval list; after
   approval its token works.
5. **Given** two provisioning clients connected at the same time, **Then** each receives only its own response.
6. **Given** a provisioned device publishes `{"temp":21.5,"door":true,"mode":"eco"}` with auto datastreams on,
   **Then** datastreams `temp` (gauge), `door` (state) and `mode` (state, text) exist and hold the values.
7. **Given** an RPC command to a ThingsBoard-style device, **Then** it arrives on `v1/devices/me/rpc/request/{n}` and
   the answer on `v1/devices/me/rpc/response/{n}` marks the command acknowledged.

---

### User Story 2 - Smart-home devices appear by themselves (Priority: P1)

A home owner creates an **MQTT account** in ctrl32 and enters its host, user name and password into zigbee2mqtt (or
Tasmota, ESPHome). The devices announce themselves with Home Assistant discovery messages; ctrl32 creates the devices
with their entities (temperature, humidity, a relay, a dimmable light, a blind) in the room they suggest, and starts
recording states. Alternatively the owner adds a **bridge** to an existing broker, and discovered devices wait in an
inbox until adopted.

**Why this priority**: "Use it like Home Assistant" starts with devices showing up without manual modelling.

**Independent Test**: Publish discovery configs and states the way zigbee2mqtt and Tasmota do; verify devices,
entities, rooms and recorded states, both through an account on the ctrl32 broker and through a bridge.

**Acceptance Scenarios**:

1. **Given** an MQTT account, **When** a client publishes `homeassistant/sensor/0x00158d/temperature/config` with
   `state_topic`, `value_template` `{{ value_json.temperature }}`, unit and device info, **Then** a device with a
   temperature entity exists, and a state `{"temperature":21.4}` is recorded as 21.4 °C.
2. **Given** abbreviated keys (`stat_t`, `cmd_t`, `val_tpl`, `~`, `dev.ids`), **Then** they are understood.
3. **Given** two accounts (two organizations) using the same topics (`zigbee2mqtt/…`), **Then** neither sees the
   other's messages.
4. **Given** an empty retained config, **Then** the entity is removed (marked removed; its history stays).
5. **Given** an availability topic reporting `offline`, **Then** the entity shows unavailable.
6. **Given** a bridge with the inbox mode, **Then** new devices wait until adopted or ignored.

---

### User Story 3 - Control devices from a Home page (Priority: P1)

The owner opens **Home**: rooms with cards — values with units and a small trend, switches, dimmable lights with a
brightness slider and colour, blinds with open/stop/close and position, thermostats with the target temperature,
locks, buttons, selects and numbers. Tapping a switch turns the relay on; the card follows the device's reported state
live. Every control action is an audited command (four eyes when the organization's policy requires it).

**Why this priority**: The daily use of a Home-Assistant-like system.

**Independent Test**: Toggle a switch, dim a light, move a blind from the page; verify the MQTT payloads on the
command topics, the command records, and the live state after the device reports back.

**Acceptance Scenarios**:

1. **Given** a Tasmota relay (`cmd_t` `cmnd/x/POWER`, `pl_on` `ON`), **When** toggled on, **Then** `ON` is published
   to `cmnd/x/POWER` in the account's namespace and the command is acknowledged when the state reports `ON`.
2. **Given** a zigbee2mqtt light with `schema: json`, **When** brightness 40 % is set, **Then**
   `{"state":"ON","brightness":102}` is published to its command topic.
3. **Given** a user without `device.command`, **Then** controls are read-only.
4. **Given** an unavailable entity, **Then** its controls are disabled.

---

### User Story 4 - Onboarding help for every path (Priority: P2)

"Add device" offers: a ctrl32 device (claim code), a fleet (provisioning key), a smart-home device on the ctrl32
broker (Tasmota / zigbee2mqtt / ESPHome / generic — with ready-to-paste settings), a bridge to an existing broker, and
connectors (OPC UA, Modbus, LoRaWAN). Every page shows the actual host, ports and credentials to use.

**Independent Test**: For each path the shown settings, used verbatim by the simulator, connect successfully.

---

### User Story 5 - Automations with entities (Priority: P2)

Flows gain an **Entity** trigger (state changed, optionally from/to), an **Entity action** node (turn on/off, toggle,
set value, open/close/stop, press) and a **Command result** trigger (acknowledged / failed / expired).

**Independent Test**: A flow "door opened → turn on the hall light" acts within one second of the door state.

---

### User Story 6 - Share ctrl32 values with Home Assistant (Priority: P3)

On a bridge, the owner selects datastreams to publish to the foreign broker with Home Assistant discovery, so an
existing Home Assistant shows ctrl32 values as sensors.

**Independent Test**: With publishing on, the bridge's broker receives a discovery config and state messages.

### Edge Cases

- Discovery config with an unsupported component or template → the entity is kept as "unsupported" with the reason;
  raw payloads are still stored for the message browser.
- Templates are a safe subset of Jinja (values, JSON paths, arithmetic, comparisons, inline if, common filters); no
  loops, no access to anything but the payload.
- A device announces 200 entities → limit per device (100) and per account (2 000); excess refused with a warning.
- The same device (same identifiers) seen through two sources → the first source owns it; the second is reported.
- Command while the source is disconnected → command fails immediately with the reason.
- Provisioning key leaked → key rotation; devices already provisioned keep their tokens.
- Very frequent states (power meters every second) → stored like telemetry; the Home page throttles redraws.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Provisioning profiles per organization (name, key, hashed secret, strategy create-new or pre-provisioned,
  approval required, target site and room, auto datastreams, device name pattern), managed with reason and audit.
- **FR-002**: Provisioning over MQTT (`/provision/request` → `/provision/response`, user name `provision`, ThingsBoard
  payloads) and HTTP (`POST /api/v1/provision` with key and secret) with rate limits and security events.
- **FR-003**: ThingsBoard device API on the ctrl32 broker: token as user name, `v1/devices/me/telemetry`,
  `…/attributes` (publish, subscribe, request/response), `…/rpc/request/+` and `…/rpc/response/{n}`, delivered only to
  the owning device.
- **FR-004**: Automatic datastreams from telemetry keys when enabled for the device (number → gauge, boolean → state,
  text → state with text values), limited per device.
- **FR-005**: MQTT accounts on the ctrl32 broker with user name and password and an isolated topic namespace; free
  topic choice inside the namespace; `$SYS` and other namespaces unreachable.
- **FR-006**: Bridges to foreign brokers (host, port, TLS, user, password, client id, discovery prefix), reconnecting
  with backoff, status and last error visible.
- **FR-007**: Home Assistant MQTT discovery: components sensor, binary_sensor, switch, light (default and json schema),
  cover, fan, lock, climate, number, select, button, text, device_tracker (position), event, scene; abbreviations,
  `~` base topic, device info, availability, `json_attributes_topic`, removal by empty config.
- **FR-008**: Entities with current state, attributes, availability and room; numeric and text states recorded as
  observations of a datastream per entity (history, charts, reports, alarms, flows work unchanged).
- **FR-009**: Entity commands through the existing command system (permissions, four eyes, audit, status), turned
  into MQTT payloads per component; acknowledged when the device reports the requested state (or immediately for
  optimistic entities).
- **FR-010**: Home page with rooms and entity cards (read and control), live updates, history dialog; discovery inbox.
- **FR-011**: Onboarding pages with ready-to-use settings for each path (US4).
- **FR-012**: Flow nodes: Entity trigger, Entity action, Command result trigger (US5).
- **FR-013**: Optional publishing of selected datastreams to a bridge with discovery (US6).
- **FR-014**: Everything inside the ctrl32 server (constitution II); no Home Assistant or Mosquitto needed.

### Key Entities

- **Provisioning profile**: key, secret hash, strategy, approval, placement, options; versioned by audit.
- **MQTT source**: account on the ctrl32 broker (namespace) or bridge to a foreign broker; discovery prefix, adoption
  mode, status.
- **Discovered device / entity**: identity from discovery (identifiers), component, normalized config, state,
  availability, room, datastream.

## Success Criteria *(mandatory)*

- **SC-001**: A zigbee2mqtt or Tasmota installation pointed at a new MQTT account shows its devices within 10 seconds,
  without any manual modelling.
- **SC-002**: A switch toggled on the Home page changes the relay in under 1 second on a LAN, and the card shows the
  confirmed state.
- **SC-003**: 100 % of control actions are audited commands with user, reason (implicit "Home page") and result.
- **SC-004**: A ThingsBoard device firmware provisions and runs without modification.
- **SC-005**: No MQTT message crosses from one organization's namespace to another (e2e test with two accounts).

## Assumptions

- Home Assistant's discovery format as documented in 2026 (MQTT integration); zigbee2mqtt 2.x and Tasmota 14+.
- Colour, effects and advanced climate modes are shown and settable where the discovery config declares them;
  camera/image streams are out of scope.

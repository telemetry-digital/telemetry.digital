# Smart home and MQTT provisioning

ctrl32 can be used like Home Assistant for devices that speak MQTT: they register themselves, appear with their
rooms and entities, are recorded and controlled from the **Home** page, and take part in flows. Specification:
`specs/005-mqtt-provisioning-smarthome/`; URS A-20…A-26.

Everything runs inside the ctrl32 server: the MQTT broker, the discovery engine, the bridges. No Home Assistant,
Mosquitto or Node-RED is needed (an existing installation can keep running next to it).

## Ways to connect a device

**Devices and connections → Add a device** shows each path with the actual host, ports and credentials.

| Device | Path |
|---|---|
| zigbee2mqtt, Tasmota, ESPHome, Z-Wave JS UI, OpenMQTTGateway, any device with Home Assistant MQTT discovery | an **MQTT account** on the ctrl32 broker |
| Devices already on your own broker (Mosquitto of zigbee2mqtt or Home Assistant) | a **bridge** |
| Many identical devices, ThingsBoard firmware | a **provisioning profile** (key and secret) |
| A single ctrl32 device | a claim code (Devices) |
| OPC UA, Modbus TCP, LoRaWAN (ChirpStack) | a connector (Settings → Connectors) |

### MQTT accounts

An account has a generated user name (`hs_…`) and password (shown once; *New password* issues another and disconnects
the account's clients). Its devices use any topics they like — `zigbee2mqtt/…`, `cmnd/…`, `homeassistant/…` — inside
a private namespace: two accounts, even of two organizations, never see each other's messages, retained messages or
last wills. `$SYS` is out of reach.

Ready-to-paste settings:

- **zigbee2mqtt** (`configuration.yaml`): `mqtt.server`, `mqtt.user`, `mqtt.password`, `homeassistant.enabled: true`.
- **Tasmota** (console): `Backlog MqttHost …; MqttPort 1883; MqttUser …; MqttPassword …; SetOption19 1`
  (`SetOption19 1` switches Tasmota to Home Assistant discovery).
- **ESPHome** (YAML): `mqtt:` with `broker`, `username`, `password`, `discovery: true`.

New devices are added automatically (setting *New devices: add automatically*); set *ask first* to collect them in the
inbox instead.

### Bridges

ctrl32 connects to your broker as a client (host, port, TLS, optionally accepting a self-signed certificate, user,
password — stored encrypted with the server's secret key). It subscribes to the discovery prefix and the entities'
topics and publishes commands there. Discovered devices wait in **Discovered devices** until you adopt or ignore them.
The connection is re-established with backoff; its state and last error are shown on the sources page.

**Sharing values with Home Assistant**: select datastreams in the bridge dialog; ctrl32 publishes a retained Home
Assistant discovery config (`homeassistant/sensor/ctrl32_…/config`) and every new value to
`ctrl32/<bridge>/<datastream>/state`, so an existing Home Assistant shows them as sensors.

### Provisioning profiles (fleets, ThingsBoard firmware)

A profile has a key (`pk_…`) and a secret (shown once; *New secret* replaces it, provisioned devices keep their tokens).
Options: strategy *create new devices* or *only devices registered beforehand*, approval, a device-name pattern, a
device limit, the site and room of new devices, automatic datastreams.

- **MQTT**: connect with the user name `provision`, publish `/provision/request`
  `{"deviceName":"sensor-001","provisionDeviceKey":"pk_…","provisionDeviceSecret":"…"}`; the answer arrives on
  `/provision/response` `{"status":"SUCCESS","credentialsType":"ACCESS_TOKEN","credentialsValue":"<token>"}` — only on
  the requesting connection. Then connect with the token as user name (no password).
- **HTTP**: `POST /api/v1/provision` with the same body.
- Failed attempts are limited to 10 per minute per address and per key and logged as security events; successful ones
  are not limited (a fleet behind one NAT address registers at once).
- With approval, the device exists but its token is refused until someone approves it (Provisioning → Devices waiting
  for approval).

### ThingsBoard device API

Devices with ThingsBoard firmware work unchanged: `v1/devices/me/telemetry`, `v1/devices/me/attributes` (publish and
shared-attribute updates), `v1/devices/me/attributes/request/{n}` → `…/response/{n}`, and RPC — a command to the device
arrives on `v1/devices/me/rpc/request/{n}` as `{"method","params"}` and the device's answer on
`v1/devices/me/rpc/response/{n}` acknowledges it (`{"error":…}` marks it failed). Each device receives only its own
messages although all use the same topic names.

### Automatic datastreams

For devices of a profile with *automatic datastreams*, every new telemetry key creates a datastream under the device's
asset (in the profile's site and room): numbers → gauge, booleans → state (1/0), texts and objects → state with the
text. Keys are normalized (`Battery Level` → `battery_level`), at most 64 per device, audited as the device.

## Entities and the Home page

A discovered device becomes a device (transport `mqtt_discovery`) in the room of its suggested area, with one entity per
discovery config: sensor, binary sensor, switch, light (default, JSON and template schema), cover, valve, fan, lock,
climate, number, select, text, button, scene, siren, humidifier, event, device tracker, update. Abbreviated keys, `~`,
availability (topics, templates, payloads), JSON attributes and device-based discovery are understood; an empty config
removes the entity (its history stays).

Templates (`value_template` and friends) are a safe Jinja subset: `value`, `value_json` with members and indexes,
arithmetic, comparisons, `and/or/not`, `x if c else y`, `{% if %}`, filters `float int round default lower upper string
trim replace length min max tojson iif` and tests `is defined`, `is number`. A config with a template ctrl32 cannot
evaluate is kept as *unsupported* with the reason.

Every state is recorded as an observation of the entity's datastream — numbers every report, other states when they
change — so charts, reports, alarm rules and flows work with them like with any telemetry.

**Home** shows the rooms and cards: values with units and a 24-hour trend, switches, brightness, blinds with
open/stop/close and position, locks, thermostats with target temperature and mode, numbers, selects, texts, buttons.
Changes appear live. A card's chart button opens the history (24 h, 7 or 30 days).

Every control action is a device command: the same permission (`device.command`), four eyes when the organization
requires it, audit (reason "Home: turn on Hall light"), and a status — *sent* until the device reports the requested
state, then *acknowledged*; *failed* when it does not within 15 s, when the entity is unavailable or no device of the
account is connected.

## Flows

- **Entity** trigger: a state change of an entity, optionally only from/to given states (`msg.from` is the previous
  state; attributes such as `msg.battery` come along).
- **Entity action**: turn on/off, toggle, open/close/stop, position, lock/unlock, press, set value, select, temperature,
  HVAC mode, fan percentage — an audited command with the flow as origin.
- **Command result** trigger: a command ended as acknowledged, failed, expired or rejected.

Example: *door opened → hall light on at 60 %* is an Entity trigger (to `on`) wired to an Entity action (`turn_on`,
value `60`).

## Not yet (phase 2)

Shelly Gen2 native RPC and the Tasmota native discovery format (Tasmota with `SetOption19 1` works), camera and image
entities, colour pickers on the cards (colour can be set through the API), an incident when a bridge stays
disconnected, X.509 and MQTT-basic credentials in provisioning.

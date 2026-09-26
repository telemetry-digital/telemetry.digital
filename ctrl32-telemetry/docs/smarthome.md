# Smart home and MQTT provisioning

ctrl32 runs the smart home itself for devices that speak MQTT: they announce themselves and are recognized
automatically (auto-discovery), appear with their rooms and entities, are recorded and controlled from the **Home** page, and take part in flows. Specification:
`specs/005-mqtt-provisioning-smarthome/` and `specs/006-smarthome-phase2/`, `specs/008-energy-scenes/`; URS A-20…A-31.

Everything runs inside the ctrl32 server: the MQTT broker, the automatic recognition, the bridges. No other home
automation software, Mosquitto or Node-RED is needed (an existing installation can keep running next to it).

*Compatibility note*: automatic recognition understands the common MQTT discovery format that zigbee2mqtt, Tasmota,
ESPHome and many other devices publish (introduced by Home Assistant; its default topic prefix is
`homeassistant/`). ctrl32 does not use Home Assistant itself.

## Ways to connect a device

**Devices and connections → Add a device** shows each path with the actual host, ports and credentials.

| Device | Path |
|---|---|
| zigbee2mqtt, Tasmota, ESPHome, Z-Wave JS UI, OpenMQTTGateway, any device that announces itself over MQTT | an **MQTT account** on the ctrl32 broker |
| Shelly Gen2+/Gen3 (Plus, Pro, Mini, Gen3, Gen4), Tasmota with default settings | an **MQTT account** (native support, see below) |
| Devices already on your own broker (for example the Mosquitto of zigbee2mqtt) | a **bridge** |
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
  (`SetOption19 1` switches Tasmota to the common MQTT discovery format; without it the native support below applies).
- **ESPHome** (YAML): `mqtt:` with `broker`, `username`, `password`, `discovery: true`.

New devices are added automatically (setting *New devices: add automatically*); set *ask first* to collect them in the
inbox instead.

### Shelly and Tasmota natively

- **Shelly Gen2 and later**: in the device's web page, *Settings → MQTT*: enable, server `host:1883`, the account's user
  and password, keep *RPC status notifications* on. ctrl32 recognizes the device by its notifications, asks it for its
  device info and status over MQTT RPC and adds switches, covers, lights (also RGB/RGBW), inputs and the power, energy,
  voltage, current, temperature, humidity and battery values it reports. Control actions are RPC calls
  (`Switch.Set`, `Cover.GoToPosition`, `Light.Set`, …) on `<device>/rpc`; the answer comes to `ctrl32-…/rpc`.
- **Tasmota with default settings** (`SetOption19 0`, Tasmota's own discovery on `tasmota/discovery/…`): relays
  (as lights with `SetOption30 1`), lights with dimmer, colour temperature and colour, and every numeric value of
  `tele/<topic>/SENSOR` (temperature, humidity, power, energy, …) with units. After a device appears ctrl32 asks it for
  its state (`cmnd/<topic>/STATE`). `SetOption19 1` (the common discovery format) keeps working as before.

Both appear like any other device (automatically or in the inbox) with entities, history, control and flows.

### Bridges

ctrl32 connects to your broker as a client (host, port, TLS, optionally accepting a self-signed certificate, user,
password — stored encrypted with the server's secret key). It subscribes to the discovery prefix and the entities'
topics and publishes commands there. Discovered devices wait in **Discovered devices** until you adopt or ignore them.
The connection is re-established with backoff; its state and last error are shown on the sources page.

**Outages**: when a bridge — or an account whose devices have entities — stays disconnected longer than *Incident when
disconnected longer than* (default 10 minutes, 0 = never), an incident *MQTT connection* opens and the administrators
are notified; it is resolved automatically when the connection is back.

**Publishing values to other systems**: select datastreams in the bridge dialog; ctrl32 publishes a retained
discovery config (`homeassistant/sensor/ctrl32_…/config`) and every new value to
`ctrl32/<bridge>/<datastream>/state`, so other systems on that broker (for example an existing Home Assistant) show them as sensors.

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

Templates (`value_template` and friends) are a safe Jinja subset: `value`, `value_json` with members, indexes and
slices (`value[0:2]`), arithmetic, comparisons, `and/or/not`, `x if c else y`, `{% if %}`, filters `float int round
default lower upper string trim replace length min max join tojson iif` (`int(0, 16)` parses hexadecimal), string
methods `split strip lower upper replace startswith endswith` and tests `is defined`, `is number`. A config with a template ctrl32 cannot
evaluate is kept as *unsupported* with the reason.

Every state is recorded as an observation of the entity's datastream — numbers every report, other states when they
change — so charts, reports, alarm rules and flows work with them like with any telemetry.

**Home** shows the rooms and cards: values with units and a 24-hour trend, switches, brightness, colour and colour
temperature (lights that announce them), blinds with
open/stop/close and position, locks, thermostats with target temperature and mode, numbers, selects, texts, buttons.
Changes appear live. A card's chart button opens the history (24 h, 7 or 30 days).

Every control action is a device command: the same permission (`device.command`), four eyes when the organization
requires it, audit (reason "Home: turn on Hall light"), and a status — *sent* until the device reports the requested
state, then *acknowledged*; *failed* when it does not within 15 s, when the entity is unavailable or no device of the
account is connected.

## Scenes

A scene sets several entities at once — *Evening*: hall light at 30 % warm white, plug on, boiler off. On **Home**,
*+ Scene* lists the controllable devices: set them the way you want, select them and save; the scene remembers their
states (on/off, brightness, colour or colour temperature, position, target temperature and mode, value). The scene
buttons above the rooms activate it with one click; *⋯* renames it, captures the states again or deletes it.

Activating a scene sends one ordinary command per device — your permission, four eyes when the organization requires
them, the audit trail with the reason *Scene: Evening*, and the device's confirmation. Scenes can also be written as
explicit actions through the API (`POST /api/v1/scenes`) and activated by flows (node **Scene**).

## Energy

**Energy** shows the consumption of all energy meters — the energy sensors of Shelly, Tasmota, zigbee2mqtt and other
devices and every datastream in Wh, kWh or MWh — per hour (day), per day (week, month) or per month (year), in the
organization's time zone, with the cost at your price per kWh, the current power (sum of all power sensors) and each
meter's share. Meters are cumulative: consumption is the sum of increases between readings; a meter that drops below
half its previous reading was reset (the new reading counts).

*Settings*: price per kWh and currency, a price paid for produced energy, and each meter's role — consumption,
production (solar panels; shown in green) or not counted. A device with several energy sensors (Tasmota: Total,
Today, Yesterday) counts only the one named *total* unless you choose otherwise.

## Flows

- **Entity** trigger: a state change of an entity, optionally only from/to given states (`msg.from` is the previous
  state; attributes such as `msg.battery` come along).
- **Entity action**: turn on/off, toggle, open/close/stop, position, lock/unlock, press, set value, select, temperature,
  HVAC mode, fan percentage — an audited command with the flow as origin.
- **Command result** trigger: a command ended as acknowledged, failed, expired or rejected.

Example: *door opened → hall light on at 60 %* is an Entity trigger (to `on`) wired to an Entity action (`turn_on`,
value `60`).

- **MQTT in** trigger: every message on a topic filter (`+` and `#` allowed) of an MQTT account or bridge. A JSON
  number, text or boolean is the value; the members of a JSON object become message fields, nested objects joined with
  `_` (`{"ENERGY":{"Power":5}}` → `msg.ENERGY_Power`).
- **Scene**: activates a scene (every action an audited command with the flow as origin; `msg.ok`, `msg.failed`).
- **MQTT out**: publishes a message (topic and payload templates, retain) on an MQTT account or bridge — for devices
  without discovery or plain MQTT scenes. Flows with it are commanding flows (four eyes where required).

Renaming an entity keeps the new name even when the device announces its configuration again.

## Not yet

Shelly Gen1 (`shellies/…`), camera and image entities, effects on the cards, X.509 and MQTT-basic credentials in
provisioning.

# Research: MQTT provisioning and smart-home devices

## R1 — Isolation of free-topic MQTT clients on one broker
- **Decision**: "Mount points" in the embedded mochi-mqtt broker. A client that logs in with an MQTT account gets the
  namespace `h/{12 hex}/`: its publish topics are prefixed in `OnPublish`, its subscription filters in `OnSubscribe`,
  its will topic in `OnWill`, and the prefix is stripped in `OnPacketEncode` before bytes go to the client. The ACL
  check sees the raw topic on publish (refuse `$…`) and the prefixed filter on subscribe (must be inside the
  namespace). The server reads everything under `h/+/#` with its inline client.
- **Rationale**: devices keep their native topics (`zigbee2mqtt/…`, `cmnd/…`, `homeassistant/…`) and two
  organizations can use the same topics safely; no second broker (constitution II).
- **Alternatives**: a Mosquitto per organization (second runtime); forcing a topic prefix in device settings (not all
  firmwares allow it, error-prone).

## R2 — ThingsBoard device API
- **Decision**: token-only login (user name = token, empty password) looked up by token hash; `v1/devices/me/…`
  aliases for devices: publish telemetry/attributes/attribute requests/RPC responses; subscriptions to
  `v1/devices/me/attributes`, `…/attributes/response/+`, `…/rpc/request/+` are rewritten to the device's own
  `d/{ext}/tb/…` filters and rewritten back on the way out, so a message reaches only its device. Commands to MQTT
  devices go to `d/{ext}/cmd` (ctrl32 protocol) and `d/{ext}/tb/rpc/request/{n}` (ThingsBoard) — the device receives
  the one it subscribed to. `n` is a per-server counter mapped to the command id in memory (a restart loses pending
  mappings; the command then expires, which is visible).
- **Provisioning**: user name `provision` (any password) may only publish `/provision/request` (and
  `provision/request`) and subscribe `/provision/response`; the response is written directly to that client
  (`Client.WritePacket`), never published, so concurrent provisioning clients cannot see each other's tokens. The
  connection is closed 5 s after the response.

## R3 — Provisioning profiles
- Key `pk_` + 20 hex (public identifier), secret 32 random bytes shown once, stored as SHA-256. Strategies
  `create` (unknown names create devices) and `preprovisioned` (only devices that exist in the organization and have
  no token yet). Options: approval required (device created with `status.approval = pending` and a token that is
  refused by the broker until approved), site/room placement, auto datastreams, name pattern (regex), transport of the
  created device (mqtt / http). Payload: ThingsBoard `{deviceName, provisionDeviceKey, provisionDeviceSecret,
  credentialsType?}`; only `ACCESS_TOKEN` credentials are issued (MQTT_BASIC and X509: refused with a message).
- Rate limit per IP and per key (10/min), failures are security events.

## R4 — Automatic datastreams
- A device with `auto_datastreams` (set by the profile or on the device page) creates a datastream for an unknown key
  under the device's asset (created as `<device name>` in the profile's site/room if the device has none): number →
  `gauge`, boolean → `state` (1/0), text/object → `state` stored in `value_json`. Keys are normalized to
  `^[a-z][a-z0-9_]{0,31}$`; at most 64 automatic datastreams per device. Audited as the device (`actor_type=device`).

## R5 — Bridges
- `github.com/eclipse/paho.golang` (EPL-2.0/EDL-1.0 — EDL is BSD-3; already a dependency of the tools) `autopaho`
  connection manager: reconnect with backoff, clean session false, subscriptions to `{prefix}/#` and to every state /
  availability / attributes topic of adopted entities. Password sealed with the server's SecretBox.
- **Alternative**: a hand-written MQTT client (no).

## R6 — Home Assistant discovery
- Topic `{prefix}/{component}/[{node_id}/]{object_id}/config`, default prefix `homeassistant`. Payload JSON with the
  documented abbreviations (full table in `internal/smarthome/abbrev.go`), `~` replaced at the start/end of topic
  values, `device` (`dev`) abbreviations (`ids`, `cns`, `mf`, `mdl`, `sw`, `hw`, `sa`, `via_device`), `availability`
  list or `availability_topic`, `availability_mode` (all/any/latest). Device-based discovery (`{prefix}/device/…/config`
  with `cmps`) is also accepted (2024.12+).
- Unique id: `unique_id`, else `{component}.{node_id}.{object_id}`. Device identity: first identifier, else the
  entity's unique id.

## R7 — Templates
- **Decision**: a safe Jinja subset evaluated in Go: `{{ … }}` with literals, `value`, `value_json` with `.a`, `['a']`,
  `[0]`, arithmetic, comparisons, `and/or/not`, `x if c else y`, filters `float int round(n) default(x) lower upper
  string abs trim replace(a,b) is defined` and `{% if %}…{% elif %}…{% else %}…{% endif %}`. Limits: 2 000 characters,
  10 000 steps. Unsupported syntax → the entity is marked with the error and the raw payload is used.
- **Rationale**: covers the templates zigbee2mqtt, Tasmota, ESPHome and OpenMQTTGateway actually send.

## R8 — Entity model and commands
- `core.entity` holds the current state (mutable, like `device.status`); every state change of a recorded entity is
  also an observation of the entity's datastream (append-only history, constitution I). Numeric states → `value`;
  on/off → 1/0 plus the text in `value_json`; other text → `value_json`.
- Commands use `core.device_command` (method = service, args = `{entity, …}`) so permissions, four eyes, audit and the
  command list work unchanged. The payload per component: switch/fan/lock/button/scene/select/number/text by
  `payload_*` / `command_template`; light default schema (`command_topic`, `brightness_command_topic` with scale,
  `rgb_command_topic`, `color_temp_command_topic`) and json schema; cover (`payload_open/close/stop`,
  `set_position_topic`); climate (`temperature_command_topic`, `mode_command_topic`). A command is `sent` when
  published and `acked` when the state matches within the expiry, or at once for optimistic entities.

## R9 — Home page
- Plain JS page `/home` (constitution VII): rooms (assets of type `room`, ordered) with cards per entity; controls by
  component; live via the existing SSE stream (new event type `entity`); history dialog uses the existing datastream
  chart API.

## R10 — Flows
- `entity` trigger (hub `entity` events, filters from/to state), `entity_action` action (builds an entity command as
  the flow, same governance as commands), `command_result` trigger (hub `command` events with a final status).

## R11 — Publishing to Home Assistant (US6)
- For selected datastreams, a bridge publishes retained `{prefix}/sensor/ctrl32_{id}/config` with `state_topic`
  `ctrl32/{org short}/{key}/state` and the unit, and each new value to the state topic; removed with an empty config
  when deselected.

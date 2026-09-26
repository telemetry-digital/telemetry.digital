# Research: Smart home phase 2

## R1 Native devices as synthesized discovery configs
- **Decision**: Adapters produce Home Assistant discovery configs and pass them through the normal discovery path.
- **Rationale**: One entity model; adoption, inbox, commands with acknowledgement, history, flows and export work
  unchanged; unit tests exercise the same parser and entity code as real configs.
- **Alternatives**: A second entity model per ecosystem (duplicated command and state logic).

## R2 Shelly Gen2 state and commands
- **Decision**: Identify by `events/rpc` (source `shelly…`) or `status/<component>:<n>`; ask `Shelly.GetDeviceInfo` and
  `Shelly.GetStatus` with our own reply prefix `ctrl32-<12 hex>`; merge `NotifyStatus` deltas into a per-component
  cache and deliver the merged object as `<prefix>/status/<component>`; commands via RPC on `<prefix>/rpc`.
- **Rationale**: `NotifyStatus` is on by default (RPC notifications), status topics are optional; RPC commands are the
  documented API for all components (`command/…` topics exist only for some).
- **Alternatives**: Require "generic status update over MQTT" (extra setting on every device); HTTP polling (not
  reachable through NAT, needs device IPs).

## R3 Tasmota native discovery
- **Decision**: Parse `config` (`t`, `ft`, `tp`, `rl`, `lt_st`, `so`, `state`, `fn`) and `sensors` (`sn`); relay and
  light state from `stat/<t>/RESULT` with `| default('')` so other results do not change it; ask `STATE` once after
  (re)discovery for the current state; sensors from `tele/<t>/SENSOR`.
- **Rationale**: RESULT carries POWER, Dimmer, Color and CT after every change and after `STATE`.
- **Alternatives**: `stat/<t>/POWER` (no dimmer/colour), `tele/<t>/STATE` (only every TelePeriod).

## R4 Colour
- **Decision**: `ui.color {rgb, color_temp, min_mireds, max_mireds}` derived from the config for every light schema;
  colour temperature kept in mireds, converted for `color_temp_kelvin` lights; `<input type=color>` and a reversed
  range slider (warm left).

## R5 Outage incidents
- **Decision**: The manager checks every 6 s in memory; the database is touched only on transitions (open once with a
  dedup key `mqtt_source:<id>`, resolve on reconnect).
- **Alternatives**: A column with the disconnect time (writes on every flap).

## R6 Renamed entities
- **Decision**: `entity.name_locked` set by a rename; discovery updates the name only when not locked.

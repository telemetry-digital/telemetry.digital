# Data model: MQTT provisioning and smart-home devices (migration 0024)

## core.provision_profile
| Column | Type | Notes |
|---|---|---|
| id | uuid | |
| org_id | uuid | |
| name | text | unique per organization |
| key | text | `pk_` + 20 hex, unique globally (identifies the organization) |
| secret_hash | text | SHA-256 of the secret |
| strategy | text | `create` \| `preprovisioned` |
| approval | bool | created devices wait for approval |
| site_id, room_id | uuid null | placement of created devices (room = asset of type `room`) |
| auto_datastreams | bool | |
| name_pattern | text | regex the device name must match ('' = any) |
| transport | text | `mqtt` \| `http` |
| max_devices | int | 0 = unlimited |
| created_at, revoked_at | timestamptz | revoked profiles refuse provisioning |

Changes audited (`provision_profile.create|update|rotate|revoke`); a device created through a profile gets
`status.provisioned_by = profile id` and is audited as `device.provision`.

## core.device (changed)
- `transport` also allows `mqtt_discovery` (devices of an MQTT source).
- `auto_datastreams boolean NOT NULL DEFAULT false`.
- `approval text` NULL | `pending` | `approved` | `rejected` (NULL for devices created by people).
- Index on `core.device_credential (secret_hash) WHERE kind = 'token' AND revoked_at IS NULL` (token-only login).

## core.connector (changed)
- `kind` also allows `mqtt_account` (settings: `username`, `namespace`, `discovery_prefix`, `adopt`, `site_id`;
  password as `secret_hash`) and `mqtt_bridge` (settings: `host`, `port`, `tls`, `insecure_skip_verify`, `username`,
  `password_sealed`, `client_id`, `discovery_prefix`, `adopt`, `site_id`, `publish_datastreams[]`).
- `status jsonb` (connected, last_error, last_message_at, counts).

## core.entity
| Column | Type | Notes |
|---|---|---|
| id | uuid | |
| org_id | uuid | |
| connector_id | uuid | source |
| device_id | uuid | device of kind `mqtt_discovery` |
| unique_id | text | unique per connector |
| component | text | sensor, binary_sensor, switch, light, cover, fan, lock, climate, number, select, button, text, device_tracker, event, scene |
| name | text | |
| config | jsonb | normalized discovery config (full keys, `~` expanded) |
| config_topic | text | |
| datastream_id | uuid null | history of the state |
| room_id | uuid null | asset of type room (from the device's suggested area, changeable) |
| state | text null | last state as text (`on`, `21.4`, `open`, …) |
| attributes | jsonb | brightness, position, temperature, json attributes |
| available | bool | |
| last_changed, last_updated | timestamptz | |
| status | text | `active` \| `unsupported` \| `removed` |
| error | text | template or config problem |
| hidden | bool | not shown on the Home page |

Unique (connector_id, unique_id). State and attributes change in place (current value); history is in observations.

## core.discovery
| Column | Type | Notes |
|---|---|---|
| id | uuid | |
| org_id, connector_id | uuid | |
| device_key | text | identifier of the device (unique per connector) |
| name, manufacturer, model | text | |
| configs | jsonb | config topic → payload (latest) |
| status | text | `pending` \| `adopted` \| `ignored` |
| device_id | uuid null | after adoption |
| first_seen, last_seen | timestamptz | |

## Events and permissions
- Live hub: new event type `entity` (id, state, attributes, available).
- Permissions: provisioning profiles and MQTT sources `device.manage`; adopting/ignoring `device.manage`; controlling
  entities `device.command`; Home page `data.read`.
- A disconnected bridge shows its state and last error on the sources page; an incident for a long outage is phase 2.

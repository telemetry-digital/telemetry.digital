# Data model: Smart home phase 2

Migration `0025_smarthome_native.sql`:

| Change | Purpose |
|---|---|
| `core.entity.name_locked boolean NOT NULL DEFAULT false` | set by a rename; discovery keeps the user's name |
| `core.incident` kind `mqtt_source` | outage incidents (dedup key `mqtt_source:<connector id>`) |

Source settings (`core.connector.settings`, kinds `mqtt_account`, `mqtt_bridge`):

| Key | Type | Default | Meaning |
|---|---|---|---|
| `outage_incident_min` | number | 10 | minutes disconnected before an incident; 0 = never; 0.1…10080 |

Synthesized discovery configs are stored in `core.discovery.configs` like published ones, keyed by
`<prefix>/<component>/ctrl32native_<device>/<object>/config`. In memory per source: the Shelly device cache (device
info, merged component status, request ids), the Tasmota config and sensors per MAC, the last synthesized payloads, and
the outage watch.

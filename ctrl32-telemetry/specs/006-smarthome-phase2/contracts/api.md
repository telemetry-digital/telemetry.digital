# API contract (changes)

| Method | Path | Change |
|---|---|---|
| POST, PUT | /api/v1/mqtt-sources[/{id}] | `outage_incident_min` (number, 0 or 0.1…10080) |
| GET | /api/v1/entities, /entities/{id}, /home | `ui.color` for lights: `{rgb, color_temp, min_mireds, max_mireds}` |
| POST | /api/v1/entities/{id}/command | `rgb: [r,g,b]`, `color_temp` (mireds) — already accepted, now used by the Home page |
| GET | /api/v1/incidents | kind `mqtt_source` |
| GET | /api/v1/flows/catalog | nodes `mqtt_in` (trigger: `source`, `topic` filter), `mqtt_out` (action: `source`, `topic`, `payload`, `retain`) |

Devices (MQTT):

| Device | Topics used by ctrl32 |
|---|---|
| Shelly Gen2+ | reads `<prefix>/events/rpc`, `<prefix>/status/<comp>:<n>`, `<prefix>/online`; writes `<prefix>/rpc` `{"id","src":"ctrl32-…","method","params"}`; replies on `ctrl32-…/rpc` |
| Tasmota (native) | reads `tasmota/discovery/<MAC>/config` and `/sensors`, `stat/<t>/RESULT`, `tele/<t>/SENSOR`, `tele/<t>/LWT`; writes `cmnd/<t>/POWER[n]`, `Dimmer`, `CT`, `Color2`, `STATE` |

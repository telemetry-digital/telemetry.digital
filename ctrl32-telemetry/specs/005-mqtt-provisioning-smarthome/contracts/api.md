# API contract (additions)

## Device side
| Channel | Message |
|---|---|
| MQTT connect user `provision` | publish `/provision/request` `{"deviceName","provisionDeviceKey","provisionDeviceSecret"}` → receive `/provision/response` `{"status":"SUCCESS","credentialsType":"ACCESS_TOKEN","credentialsValue":"<token>"}` or `{"status":"FAILURE","errorMsg":"…"}` |
| `POST /api/v1/provision` | body as above (ThingsBoard) → same response; the existing claim-code body `{claim, hw, fw}` keeps working |
| MQTT user = token, empty password | ThingsBoard device API: `v1/devices/me/telemetry`, `v1/devices/me/attributes` (pub/sub), `v1/devices/me/attributes/request/{n}` → `…/attributes/response/{n}`, `v1/devices/me/rpc/request/+` ← `{"method","params"}`, `v1/devices/me/rpc/response/{n}` |
| MQTT account (user/password) | any topic inside its namespace; discovery under `{prefix}/…/config` |

## Management (all changes need `reason`)
| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | /api/v1/provision-profiles | device.manage | list (no secrets) |
| POST | /api/v1/provision-profiles | device.manage | → profile + `secret` (shown once) |
| PUT | /api/v1/provision-profiles/{id} | device.manage | settings |
| POST | /api/v1/provision-profiles/{id}/rotate | device.manage | new secret |
| DELETE | /api/v1/provision-profiles/{id} | device.manage | revoke |
| GET | /api/v1/device-approvals | device.manage | devices waiting for approval |
| POST | /api/v1/devices/{id}/approve, /reject | device.manage | |
| GET | /api/v1/mqtt-sources | device.manage | accounts and bridges with status |
| POST | /api/v1/mqtt-sources | device.manage | `{kind: account|bridge, name, …}` → account: `username`, `password` (shown once) |
| PUT | /api/v1/mqtt-sources/{id} | device.manage | settings; bridge password optional |
| POST | /api/v1/mqtt-sources/{id}/password | device.manage | new account password |
| DELETE | /api/v1/mqtt-sources/{id} | device.manage | entities marked removed |
| GET | /api/v1/discovery | device.manage | inbox |
| POST | /api/v1/discovery/{id}/adopt, /ignore | device.manage | |
| GET | /api/v1/entities | data.read | `?room=&device=&component=` with state |
| GET | /api/v1/entities/{id} | data.read | with config |
| PATCH | /api/v1/entities/{id} | device.manage | name, room, hidden |
| POST | /api/v1/entities/{id}/command | device.command | `{service, value?, brightness?, color?, position?, temperature?, mode?, reason?}` → command (202 when pending four eyes) |
| GET | /api/v1/home | data.read | rooms with entities (one call for the Home page) |
| GET | /api/v1/onboarding | device.manage | hosts, ports, TLS, settings snippets per path |

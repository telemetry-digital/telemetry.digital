# API contract

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | /api/v1/energy?period=&date= | data.read | buckets, total, sources, power_w, tz |
| GET | /api/v1/energy/settings | data.read | price, currency, meters with roles |
| PUT | /api/v1/energy/settings | content.write | reason required |
| GET | /api/v1/scenes | data.read | |
| POST | /api/v1/scenes | device.manage | `{name, icon, room_id, actions | capture, reason}` → 201 |
| PUT | /api/v1/scenes/{id} | device.manage | |
| DELETE | /api/v1/scenes/{id}?reason= | device.manage | |
| POST | /api/v1/scenes/{id}/activate | device.command | results per action; 409 when all failed |
| GET | /api/v1/home | data.read | adds `scenes` |
| — | flow node `scene` | — | action, commanding; msg.ok, msg.failed |

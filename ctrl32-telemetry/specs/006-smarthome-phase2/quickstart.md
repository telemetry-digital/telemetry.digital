# Quickstart: validate spec 006

Prerequisites: dev server, an MQTT account (`POST /api/v1/mqtt-sources {kind:"account"}`), `go build ./tools/mqttsim`.

1. **Shelly (US1)**: `mqttsim shelly -user … -pass … -id a8032ab12345` → device "Boiler" with switch, input and
   sensors; `POST /api/v1/entities/{switch}/command {service:"turn_on"}` → the simulator prints `rpc Switch.Set`.
2. **Tasmota native (US2)**: `mqttsim tasmota -native -user … -pass … -id D4E5F6` → "Workshop plug" with relay and
   sensors; `turn_on` → `cmnd/tasmota_D4E5F6/POWER ON`.
3. **Colour (US3)**: `mqttsim z2m …`; the Hall light card shows the colour picker; choose a colour → the simulator
   receives `"color":{"r":…}`.
4. **MQTT nodes (US4)**: a flow MQTT in `tele/+/SENSOR` → Debug shows `msg.AM2301_Temperature`; Inject → MQTT out
   `cmnd/tasmota_D4E5F6/POWER` `OFF`.
5. **Outage (US5)**: a bridge to a closed port with `outage_incident_min` 0.1 → incident within ~15 s; start
   `mqttsim broker -listen` on that port → resolved.

Automated: `tests/e2e/smarthome_native.sh`, `tests/headless/home.js`, `go test ./internal/smarthome ./internal/flows`.

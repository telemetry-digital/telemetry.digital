# Quickstart: validate spec 005

Prerequisites: dev server (`bash deploy/dev/dev.sh up` or `tests/run-all.sh`), `go build ./tools/mqttsim`.

1. **Fleet provisioning (US1)**: `POST /api/v1/provision-profiles` → key and secret;
   `mqttsim tb -broker tcp://127.0.0.1:1883 -key … -secret … -name sensor-1` provisions, reconnects with the token,
   sends telemetry and answers RPC. Expect device `sensor-1`, datastreams from the keys, an acknowledged command.
2. **Smart-home account (US2)**: `POST /api/v1/mqtt-sources {kind:"account"}` → user and password;
   `mqttsim z2m -user … -pass …` announces a temperature sensor, a light and a switch; `mqttsim tasmota …` a relay.
   Expect devices and entities in `GET /api/v1/home`, states recorded.
3. **Control (US3)**: `POST /api/v1/entities/{id}/command {service:"turn_on"}` → the simulator prints the payload
   it received and reports the state; the command becomes `acked`.
4. **Isolation (SC-005)**: two accounts in two organizations publish the same topics; neither simulator sees the
   other's messages.
5. **Bridge (US2/US6)**: run a second, foreign broker (`mqttsim broker -listen :1885`), add a bridge, publish a
   discovery config on it, adopt it from the inbox, toggle it.
6. Open `/home` and toggle the switch; the card shows the confirmed state.

Automated: `tests/e2e/provisioning.sh`, `tests/e2e/smarthome.sh`, `tests/headless/home.js`.

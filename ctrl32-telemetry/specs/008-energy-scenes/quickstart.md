# Quickstart: validate spec 008

1. A kWh datastream fed by `tools/devfill -start 100 -step 0.5 -n 49 -every 1800` → Energy shows 24 kWh over the last
   day; set a price → cost; mark the meter as production → green series.
2. `mqttsim z2m` on an MQTT account; set the light and plug; Home → "+ Scene" → select both → save; change them;
   click the scene → the simulator receives the commands and the states return.
3. A flow Inject → Scene → Debug shows `2/0`.

Automated: `tests/e2e/energy_scenes.sh`, `go test ./internal/flows`.

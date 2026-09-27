#!/usr/bin/env bash
# End-to-end checks for smart-home devices (spec 005 US2–US6): an MQTT account with its own topic namespace, devices
# announced by Home Assistant discovery (zigbee2mqtt and Tasmota simulators) adopted with rooms and entities, states
# recorded, control through commands confirmed by the device, availability, removal, isolation between accounts, a
# bridge to a foreign broker with the inbox, and values shared with Home Assistant. Needs Go (tools/mqttsim).
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 USER=admin PASS=admin1234 bash tests/e2e/smarthome.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
FOREIGN=${FOREIGN:-127.0.0.1:18883}
TMP=$(mktemp -d); trap 'kill $(jobs -p) 2>/dev/null; rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""
pass=0; fail=0
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
waitfor() { for i in $(seq 40); do eval "$1" && return 0; sleep 0.25; done; return 1; }
SUF=$RANDOM

echo "== setup"
go build -o "$TMP/mqttsim" ./tools/mqttsim && go build -o "$TMP/mqttpub" ./tools/mqttpub || { echo "cannot build the MQTT tools"; exit 1; }
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)

echo "== MQTT accounts"
req POST /api/v1/mqtt-sources "{\"kind\":\"account\",\"name\":\"Flat $SUF\",\"reason\":\"e2e\"}"; check "account created" "$(st)" 201
SA=$(jx j.id); UA=$(jx j.username); PA=$(jx j.password)
[[ "$UA" == hs_* && ${#PA} -ge 20 ]] && ok "user name and password issued" || bad "credentials: $UA"
req POST /api/v1/mqtt-sources "{\"kind\":\"account\",\"name\":\"Other $SUF\",\"reason\":\"e2e\"}"; SB=$(jx j.id); UB=$(jx j.username); PB=$(jx j.password)
req GET /api/v1/mqtt-sources; [[ "$(cat "$TMP/b")" != *"$PA"* ]] && ok "password not listed" || bad "password listed"
"$TMP/mqttpub" -addr "$MQTT" -user "$UB" -token "$PB" -client "spy-$SUF" -sub '#' -wait 12 > "$TMP/spy.log" 2>&1 &
"$TMP/mqttpub" -addr "$MQTT" -user "$UA" -token wrong -topic x -payload x -wait 0 > "$TMP/wrong.log" 2>&1 && bad "wrong password accepted" || ok "wrong password refused"

echo "== discovery (zigbee2mqtt and Tasmota)"
"$TMP/mqttsim" z2m -addr "$MQTT" -user "$UA" -pass "$PA" -wait 40 > "$TMP/z2m.log" 2>&1 &
"$TMP/mqttsim" tasmota -addr "$MQTT" -user "$UA" -pass "$PA" -id "T$SUF" -wait 40 > "$TMP/tas.log" 2>&1 & TAS=$!
sleep 4
req GET /api/v1/home; check "home" "$(st)" 200
check "rooms from the suggested areas" "$(jx 'j.rooms.map(r => r.name).filter(Boolean).sort().join(",")')" "Hall,Kitchen,Living room"
req GET "/api/v1/entities?source=$SA"
check "entities of the account" "$(jx "j.map(e => e.component + ':' + e.device).sort().join(',')")" "binary_sensor:Front door,light:Hall light,sensor:Garage,sensor:Living room sensor,sensor:Living room sensor,switch:Garage,switch:Kitchen plug"
TEMP=$(jx "(j.find(e => e.name === 'Temperature' && e.device === 'Living room sensor' && e.available) || {}).id")
LIGHT=$(jx "(j.find(e => e.device === 'Hall light' && e.room === 'Hall' && e.available) || {}).id")
RELAY=$(jx "(j.find(e => e.name === 'Relay' && e.device === 'Garage' && e.available) || {}).id")
[[ -n "$TEMP" && "$TEMP" != null && -n "$LIGHT" && "$LIGHT" != null && -n "$RELAY" && "$RELAY" != null ]] && ok "entities adopted automatically" || bad "entities: $(jx 'j.map(e => e.name + "/" + e.device).join(",")')"
req GET "/api/v1/entities/$TEMP"; check "temperature state and unit" "$(jx 'isFinite(Number(j.state)) + " " + j.unit')" "true °C"
DS=$(jx j.datastream_id)
req GET "/api/v1/observations?datastream_id=$DS&limit=10"; [[ "$(jx 'j.length')" -ge 1 ]] && ok "states recorded as observations" || bad "no observations"
req GET "/api/v1/entities/$LIGHT"; check "light services" "$(jx 'j.services.join(",")')" "turn_on,turn_off,toggle"

echo "== control"
req POST "/api/v1/entities/$LIGHT/command" '{"service":"turn_on","brightness":40}'; check "light command accepted" "$(st)" 201
CID=$(jx j.id); [[ "$(jx j.status)" == sent || "$(jx j.status)" == acked ]] && ok "command sent" || bad "status $(jx j.status)"
waitfor 'grep -q "Hall light/set {\"brightness\":102,\"state\":\"ON\"}" "$TMP/z2m.log"' && ok "JSON light payload on the command topic" || bad "payload: $(grep command "$TMP/z2m.log")"
DEV=$(curl -s -b "$JAR" "$BASE/api/v1/entities/$LIGHT" | node -pe 'JSON.parse(require("fs").readFileSync(0)).device_id')
waitfor 'curl -s -b "$JAR" "$BASE/api/v1/devices/$DEV/commands" | grep -q "\"id\":\"$CID\"[^}]*\"status\":\"acked\"\|\"status\":\"acked\"[^}]*\"id\":\"$CID\""' ; req GET "/api/v1/devices/$DEV/commands"
check "confirmed by the reported state" "$(jx "(j.find(c => c.id === '$CID') || {}).status")" acked
check "implicit reason" "$(jx "(j.find(c => c.id === '$CID') || {}).reason")" "Home: turn on Hall light"
req GET "/api/v1/entities/$LIGHT"; check "light state" "$(jx 'j.state + " " + j.attributes.brightness')" "on 102"
req POST "/api/v1/entities/$RELAY/command" '{"service":"toggle"}'; check "relay toggle" "$(st)" 201
waitfor 'grep -q "cmnd/tasmota_T$SUF/POWER ON" "$TMP/tas.log"' && ok "Tasmota command in the account's namespace" || bad "tasmota: $(grep command "$TMP/tas.log")"
req POST "/api/v1/entities/$RELAY/command" '{"service":"set_position","position":5}'; check "unsupported action refused" "$(st)" 400
req POST "/api/v1/entities/$TEMP/command" '{"service":"turn_on"}'; check "sensor is read-only" "$(st)" 400
req GET "/api/v1/export/audit?format=csv"; grep -q "Home: turn on Hall light" "$TMP/b" && ok "control audited" || bad "audit"

echo "== entity settings"
req PATCH "/api/v1/entities/$TEMP" '{"name":"Living temperature","hidden":true,"reason":"e2e"}'; check "rename and hide" "$(st)" 200
req GET /api/v1/home; check "hidden entity not on the Home page" "$(jx "j.rooms.some(r => r.entities.some(e => e.id === '$TEMP'))")" false
req PATCH "/api/v1/entities/$TEMP" '{"hidden":false,"reason":"e2e"}'

echo "== availability and removal"
kill $TAS 2>/dev/null; wait $TAS 2>/dev/null
waitfor '[[ "$(curl -s -b "$JAR" "$BASE/api/v1/entities/$RELAY" | node -pe "JSON.parse(require(\"fs\").readFileSync(0)).available")" == false ]]' && ok "unavailable after the device's will (LWT)" || bad "relay still available"
req POST "/api/v1/entities/$RELAY/command" '{"service":"turn_on"}'; check "no command to an unavailable entity" "$(jx 'j.status + " " + j.error')" "failed entity is unavailable"
"$TMP/mqttpub" -addr "$MQTT" -user "$UA" -token "$PA" -client "rm-$SUF" -topic "homeassistant/sensor/T${SUF}_energy_power/config" -payload '' -wait 0 > /dev/null 2>&1
waitfor '[[ "$(curl -s -b "$JAR" "$BASE/api/v1/entities?source=$SA" | node -pe "JSON.parse(require(\"fs\").readFileSync(0)).some(e => e.name === \"Power\")")" == false ]]' && ok "empty config removes the entity" || bad "entity not removed"

echo "== isolation between accounts"
sleep 1
grep -q '^<<' "$TMP/spy.log" && bad "another account saw messages: $(grep '^<<' "$TMP/spy.log" | head -2)" || ok "other account sees none of the messages"
grep -q 'subscribed' "$TMP/spy.log" && ok "other account was connected and subscribed to #" || bad "spy: $(head -3 "$TMP/spy.log")"

echo "== bridge to a foreign broker"
"$TMP/mqttsim" broker -listen "$FOREIGN" -user mosq -pass "s$SUF" -wait 60 > "$TMP/fb.log" 2>&1 &
sleep 0.5
"$TMP/mqttsim" z2m -addr "$FOREIGN" -user mosq -pass "s$SUF" -base z2m -wait 50 > "$TMP/z2mb.log" 2>&1 &
sleep 1
req GET /api/v1/datastreams; EXP=$(jx "j.find(d => d.key === 'temperature').id")
"$TMP/mqttpub" -addr "$FOREIGN" -user mosq -token "s$SUF" -client "ha-$SUF" -sub 'homeassistant/sensor/ctrl32_#' -wait 12 > "$TMP/ha.log" 2>&1 &
sleep 0.5
H=${FOREIGN%:*}; P=${FOREIGN#*:}
req POST /api/v1/mqtt-sources "{\"kind\":\"bridge\",\"name\":\"Mosquitto $SUF\",\"host\":\"$H\",\"port\":$P,\"username\":\"mosq\",\"password\":\"s$SUF\",\"publish_datastreams\":[\"$EXP\"],\"reason\":\"e2e\"}"
check "bridge created" "$(st)" 201; BR=$(jx j.id)
waitfor '[[ "$(curl -s -b "$JAR" "$BASE/api/v1/mqtt-sources" | node -pe "(JSON.parse(require(\"fs\").readFileSync(0)).find(s => s.id === \"$BR\") || {}).pending")" -ge 4 ]]'
req GET /api/v1/mqtt-sources; check "bridge connected" "$(jx "(j.find(s => s.id === '$BR') || {}).status.connected")" true
req GET "/api/v1/discovery?status=pending"; check "discovered devices wait in the inbox" "$(jx "j.filter(d => d.source_id === '$BR').map(d => d.name).sort().join(',')")" "Front door,Hall light,Kitchen plug,Living room sensor"
PLUG=$(jx "j.find(d => d.source_id === '$BR' && d.name === 'Kitchen plug').id"); DOOR=$(jx "j.find(d => d.source_id === '$BR' && d.name === 'Front door').id")
req POST "/api/v1/discovery/$PLUG/adopt" '{"reason":"e2e adopt"}'; check "adopt" "$(jx j.status)" adopted
req POST "/api/v1/discovery/$DOOR/ignore" '{"reason":"e2e ignore"}'; check "ignore" "$(jx j.status)" ignored
req GET "/api/v1/entities?device=$(curl -s -b "$JAR" "$BASE/api/v1/discovery" | node -pe "JSON.parse(require('fs').readFileSync(0)).find(d => d.id === '$PLUG').device_id")"
BPLUG=$(jx 'j[0] && j[0].id')
req POST "/api/v1/entities/$BPLUG/command" '{"service":"turn_on"}'; check "command through the bridge" "$(st)" 201
waitfor 'grep -q "z2m/Kitchen plug/set ON" "$TMP/z2mb.log"' && ok "published on the foreign broker" || bad "bridge publish: $(grep command "$TMP/z2mb.log")"
grep -q "homeassistant/sensor/ctrl32_" "$TMP/ha.log" && ok "ctrl32 value announced to Home Assistant" || bad "export: $(tail -2 "$TMP/ha.log")"

echo "== cleanup"
for s in "$SA" "$SB" "$BR"; do req DELETE "/api/v1/mqtt-sources/$s?reason=e2e"; done; check "sources deleted" "$(st)" 200
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

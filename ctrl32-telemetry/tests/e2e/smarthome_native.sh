#!/usr/bin/env bash
# End-to-end checks for smart home phase 2 (spec 006): Shelly Gen2 (RPC over MQTT) and Tasmota's native discovery
# added without Home Assistant discovery, control through them, colour on lights, a renamed entity keeping its name,
# MQTT in/out flow nodes, and an incident for a bridge that stays disconnected. Needs Go (tools/mqttsim).
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 USER=admin PASS=admin1234 bash tests/e2e/smarthome_native.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
DEAD=${DEAD:-127.0.0.1:18884}
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
waitfor() { local n=${2:-40}; for i in $(seq "$n"); do eval "$1" && return 0; sleep 0.25; done; return 1; }
ents() { curl -s -b "$JAR" "$BASE/api/v1/entities?source=$SA" > "$TMP/e"; node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); console.log(eval(process.argv[2]))' "$TMP/e" "$1" 2>/dev/null; }
SUF=$RANDOM
SID=$(printf 'e2e%09d' "$SUF")     # Shelly id suffix
TID=$(printf 'N%05d' "$SUF")        # Tasmota id

echo "== setup"
go build -o "$TMP/mqttsim" ./tools/mqttsim || { echo "cannot build tools/mqttsim"; exit 1; }
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req POST /api/v1/mqtt-sources "{\"kind\":\"account\",\"name\":\"Native $SUF\",\"reason\":\"e2e\"}"; check "account created" "$(st)" 201
SA=$(jx j.id); UA=$(jx j.username); PA=$(jx j.password)

echo "== Shelly Gen2 (RPC over MQTT)"
"$TMP/mqttsim" shelly -addr "$MQTT" -user "$UA" -pass "$PA" -id "$SID" -wait 60 > "$TMP/shelly.log" 2>&1 &
waitfor '[[ "$(ents "j.filter(e => e.device === \"Boiler\").length")" -ge 7 ]]' 60 && ok "Shelly recognized and adopted" || bad "Shelly entities: $(ents 'j.map(e => e.name + "/" + e.device).join(",")')"
grep -q "rpc Shelly.GetDeviceInfo" "$TMP/shelly.log" && grep -q "rpc Shelly.GetStatus" "$TMP/shelly.log" && ok "device info and status asked over RPC" || bad "rpc: $(head -4 "$TMP/shelly.log")"
check "Shelly components" "$(ents "[...new Set(j.filter(e => e.device === 'Boiler').map(e => e.component))].sort().join(',')")" "binary_sensor,sensor,switch"
check "energy sensor with unit" "$(ents "(j.find(e => e.device === 'Boiler' && e.name === 'Energy') || {}).unit")" "Wh"
SW=$(ents "(j.find(e => e.device === 'Boiler' && e.component === 'switch') || {}).id")
check "switch available and off" "$(ents "(j.find(e => e.id === '$SW') || {}).available + ' ' + (j.find(e => e.id === '$SW') || {}).state")" "true off"
req POST "/api/v1/entities/$SW/command" '{"service":"turn_on"}'; check "switch command" "$(st)" 201
waitfor 'grep -q "rpc Switch.Set.*\"on\":true" "$TMP/shelly.log"' && ok "Switch.Set called on the device" || bad "rpc: $(grep Switch "$TMP/shelly.log")"
waitfor '[[ "$(ents "(j.find(e => e.id === \"$SW\") || {}).state")" == on ]]' && ok "state confirmed by NotifyStatus" || bad "state $(ents "(j.find(e => e.id === '$SW') || {}).state")"
waitfor '[[ "$(ents "Number((j.find(e => e.device === \"Boiler\" && e.name === \"Power\") || {}).state) > 0")" == true ]]' && ok "power follows the partial updates" || bad "power $(ents "(j.find(e => e.device === 'Boiler' && e.name === 'Power') || {}).state")"

echo "== Tasmota native discovery"
"$TMP/mqttsim" tasmota -native -addr "$MQTT" -user "$UA" -pass "$PA" -id "$TID" -wait 60 > "$TMP/tas.log" 2>&1 &
waitfor '[[ "$(ents "j.filter(e => e.device === \"Workshop plug\").length")" -ge 8 ]]' 60 && ok "Tasmota recognized and adopted" || bad "Tasmota entities: $(ents 'j.map(e => e.name + "/" + e.device).join(",")')"
check "temperature from tele/SENSOR" "$(ents "(j.find(e => e.device === 'Workshop plug' && e.name === 'AM2301 Temperature') || {}).unit")" "°C"
check "energy total" "$(ents "(j.find(e => e.device === 'Workshop plug' && e.name === 'Energy total') || {}).unit")" "kWh"
RL=$(ents "(j.find(e => e.device === 'Workshop plug' && e.component === 'switch') || {}).id")
req POST "/api/v1/entities/$RL/command" '{"service":"turn_on"}'; check "relay command" "$(st)" 201
waitfor "grep -q \"command cmnd/tasmota_$TID/POWER ON\" \"\$TMP/tas.log\"" && ok "POWER ON published" || bad "tasmota: $(grep command "$TMP/tas.log")"
waitfor '[[ "$(ents "(j.find(e => e.id === \"$RL\") || {}).state")" == on ]]' && ok "relay state from stat/RESULT" || bad "relay state"

echo "== colour and names (zigbee2mqtt)"
"$TMP/mqttsim" z2m -addr "$MQTT" -user "$UA" -pass "$PA" -wait 20 > "$TMP/z2m.log" 2>&1 & Z=$!
waitfor '[[ -n "$(ents "(j.find(e => e.device === \"Hall light\" && e.available) || {}).id || \"\"")" ]]' 60
LIGHT=$(ents "(j.find(e => e.device === 'Hall light') || {}).id")
req GET "/api/v1/entities/$LIGHT"; check "colour capabilities" "$(jx 'JSON.stringify(j.ui.color)')" '{"rgb":true,"color_temp":true,"min_mireds":153,"max_mireds":454}'
req POST "/api/v1/entities/$LIGHT/command" '{"service":"turn_on","rgb":[255,0,0]}'; check "colour command" "$(st)" 201
waitfor 'grep -q "\"color\":{\"b\":0,\"g\":0,\"r\":255}" "$TMP/z2m.log"' && ok "colour published" || bad "z2m: $(grep command "$TMP/z2m.log" | tail -1)"
waitfor '[[ "$(ents "JSON.stringify((j.find(e => e.id === \"$LIGHT\") || {}).attributes.rgb_color)")" == "[255,0,0]" ]]' && ok "colour reported back" || bad "rgb $(ents "JSON.stringify((j.find(e => e.id === '$LIGHT') || {}).attributes)")"
req POST "/api/v1/entities/$LIGHT/command" '{"service":"turn_on","color_temp":370}'
waitfor 'grep -q "\"color_temp\":370" "$TMP/z2m.log"' && ok "colour temperature published" || bad "ct: $(grep command "$TMP/z2m.log" | tail -1)"
req PATCH "/api/v1/entities/$LIGHT" '{"name":"Ceiling '$SUF'","reason":"e2e rename"}'; check "rename" "$(st)" 200
kill $Z 2>/dev/null; wait $Z 2>/dev/null
"$TMP/mqttsim" z2m -addr "$MQTT" -user "$UA" -pass "$PA" -wait 10 > "$TMP/z2m2.log" 2>&1 &
sleep 3
check "name kept when the device announces itself again" "$(ents "(j.find(e => e.id === '$LIGHT') || {}).name")" "Ceiling $SUF"

echo "== MQTT in/out flow nodes"
SPEC="{\"nodes\":[
 {\"id\":\"in\",\"type\":\"mqtt_in\",\"x\":40,\"y\":40,\"settings\":{\"source\":\"$SA\",\"topic\":\"tele/+/SENSOR\"}},
 {\"id\":\"f\",\"type\":\"filter\",\"x\":260,\"y\":40,\"settings\":{\"condition\":\"msg.AM2301_Temperature > 0\"}},
 {\"id\":\"d\",\"type\":\"debug\",\"x\":480,\"y\":40,\"settings\":{\"expr\":\"topic + ' ' + msg.AM2301_Temperature\"}},
 {\"id\":\"inj\",\"type\":\"inject\",\"x\":40,\"y\":160,\"settings\":{}},
 {\"id\":\"out\",\"type\":\"mqtt_out\",\"x\":260,\"y\":160,\"settings\":{\"source\":\"$SA\",\"topic\":\"cmnd/tasmota_$TID/POWER\",\"payload\":\"OFF\"}}],
 \"wires\":[{\"from\":\"in\",\"port\":0,\"to\":\"f\"},{\"from\":\"f\",\"port\":0,\"to\":\"d\"},{\"from\":\"inj\",\"port\":0,\"to\":\"out\"}]}"
req POST /api/v1/flows "{\"name\":\"MQTT nodes $SUF\",\"reason\":\"e2e\",\"spec\":$SPEC}"; check "create flow" "$(st)" 201
FL=$(jx j.flow.slug); check "no problems" "$(jx 'j.problems.length')" 0
req POST "/api/v1/flows/$FL/deploy" '{"reason":"e2e"}'; check "deploy" "$(st)" 200
waitfor '[[ "$(curl -s -b "$JAR" "$BASE/api/v1/flows/$FL/debug" | node -pe "JSON.parse(require(\"fs\").readFileSync(0)).filter(d => d.node === \"d\" && d.text.startsWith(\"tele/tasmota_$TID/SENSOR \")).length")" -ge 1 ]]' 40 && ok "MQTT in delivers topics and nested JSON fields" || bad "debug: $(curl -s -b "$JAR" "$BASE/api/v1/flows/$FL/debug" | head -c 400)"
req POST "/api/v1/flows/$FL/inject" '{"node":"inj","value":1}'; check "inject" "$(st)" 202
waitfor "grep -q \"command cmnd/tasmota_$TID/POWER OFF\" \"\$TMP/tas.log\"" && ok "MQTT out published" || bad "mqtt out: $(grep command "$TMP/tas.log" | tail -2)"
req POST "/api/v1/mqtt-sources" "{\"kind\":\"account\",\"name\":\"Other $SUF\",\"reason\":\"e2e\"}"; SB=$(jx j.id)
BADSPEC="{\"nodes\":[{\"id\":\"in\",\"type\":\"mqtt_in\",\"x\":40,\"y\":40,\"settings\":{\"source\":\"00000000-0000-0000-0000-000000000000\",\"topic\":\"a/#/b\"}}],\"wires\":[]}"
req POST /api/v1/flows "{\"name\":\"MQTT bad $SUF\",\"reason\":\"e2e\",\"spec\":$BADSPEC}"; BAD=$(jx j.flow.slug)
[[ "$(jx 'j.problems.length')" -ge 1 ]] && ok "unknown source is a problem" || bad "problems: $(jx j.problems)"

echo "== incident for a disconnected bridge"
H=${DEAD%:*}; P=${DEAD#*:}
req POST /api/v1/mqtt-sources "{\"kind\":\"bridge\",\"name\":\"Down $SUF\",\"host\":\"$H\",\"port\":$P,\"outage_incident_min\":0.05,\"reason\":\"e2e\"}"
check "invalid limit refused" "$(st)" 400
req POST /api/v1/mqtt-sources "{\"kind\":\"bridge\",\"name\":\"Down $SUF\",\"host\":\"$H\",\"port\":$P,\"outage_incident_min\":0.1,\"reason\":\"e2e\"}"; check "bridge created" "$(st)" 201
BR=$(jx j.id)
openinc() { curl -s -b "$JAR" "$BASE/api/v1/incidents?status=$1" | node -pe "JSON.parse(require('fs').readFileSync(0)).filter(i => i.kind === 'mqtt_source' && i.title === 'MQTT bridge Down $SUF disconnected').length"; }
waitfor '[[ "$(openinc open)" == 1 ]]' 120 && ok "incident opened after the limit" || bad "no incident: $(curl -s -b "$JAR" "$BASE/api/v1/incidents?status=open" | head -c 300)"
"$TMP/mqttsim" broker -listen "$DEAD" -wait 60 > "$TMP/dead.log" 2>&1 &
waitfor '[[ "$(openinc resolved)" == 1 ]]' 160 && ok "incident resolved when the bridge reconnected" || bad "still open: $(openinc open)"

echo "== cleanup"
for s in "$FL" "$BAD"; do req DELETE "/api/v1/flows/$s?reason=e2e"; done
for s in "$SA" "$SB" "$BR"; do req DELETE "/api/v1/mqtt-sources/$s?reason=e2e"; done; check "sources deleted" "$(st)" 200
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

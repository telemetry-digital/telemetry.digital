#!/usr/bin/env bash
# End-to-end checks for spec 008: the energy overview (consumption from a cumulative meter fed with backdated readings,
# roles, price and cost, settings with audit) and scenes (captured from current states, activated as commands the
# device receives, from the API and from a flow). Needs Go (tools/devfill, tools/mqttsim).
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 USER=admin PASS=admin1234 bash tests/e2e/energy_scenes.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
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
SUF=$RANDOM

echo "== setup"
go build -o "$TMP/devfill" ./tools/devfill && go build -o "$TMP/mqttsim" ./tools/mqttsim || { echo "cannot build the tools"; exit 1; }
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
for p in /energy /home; do check "page $p" "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR" "$BASE$p")" 200; done

echo "== energy meter"
req POST /api/v1/sites "{\"name\":\"Energy site $SUF\",\"reason\":\"e2e\"}"; SITE=$(jx j.id)
req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"machine\",\"name\":\"Main meter $SUF\",\"reason\":\"e2e\"}"; ASSET=$(jx j.id)
req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"energy\",\"quantity\":\"energy_active_import\",\"unit\":\"kWh\",\"kind\":\"counter\",\"reason\":\"e2e\"}"; DS=$(jx j.id)
req POST /api/v1/devices "{\"external_id\":\"meter-$SUF\",\"name\":\"meter $SUF\",\"reason\":\"e2e\"}"; DEV=$(jx j.id)
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$DS\",\"reason\":\"e2e\"}"
req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}'; TOK=$(jx j.token)
[[ -n "$DS" && -n "$TOK" ]] && ok "meter datastream in kWh" || bad "setup: $(head -c 200 "$TMP/b")"
# 49 readings every 30 min over the last 24 h, +0.5 kWh each
"$TMP/devfill" -addr "$MQTT" -user "meter-$SUF" -token "$TOK" -key energy -start 100 -step 0.5 -n 49 -every 1800 > "$TMP/fill.log" 2>&1 && ok "backdated readings published" || bad "devfill: $(tail -2 "$TMP/fill.log")"
sumdays() { local tot=0; for d in "$(date -u -d 'yesterday' +%F)" "$(date -u +%F)" "$(date -u -d 'tomorrow' +%F)"; do
  req GET "/api/v1/energy?period=day&date=$d"; tot=$(node -e "console.log(($tot + ($(jx "(j.sources.find(s => s.datastream_id === '$DS') || {kwh: 0}).kwh"))).toFixed(3))"); done; echo "$tot"; }
waitfor '[[ "$(sumdays)" == "24.000" ]]' 40 && ok "24 kWh over the last day (48 increases of 0.5 kWh)" || bad "sum $(sumdays)"
req GET "/api/v1/energy?period=day"; check "hourly buckets" "$(jx 'j.bucket + " " + (j.buckets.length >= 23)')" "hour true"
req GET "/api/v1/energy?period=week"; check "week in days" "$(jx 'j.bucket + " " + j.buckets.length')" "day 7"
req GET "/api/v1/energy?period=year"; check "year in months" "$(jx 'j.bucket + " " + j.buckets.length')" "month 12"
req GET "/api/v1/energy?period=decade"; check "bad period" "$(st)" 400

echo "== settings, price and roles"
req PUT /api/v1/energy/settings '{"price_per_kwh":0.2,"currency":"EUR"}'; check "reason required" "$(st)" 400
req PUT /api/v1/energy/settings '{"price_per_kwh":-1,"reason":"e2e"}'; check "negative price refused" "$(st)" 400
req PUT /api/v1/energy/settings "{\"price_per_kwh\":0.2,\"currency\":\"EUR\",\"sources\":[{\"datastream_id\":\"$DS\",\"role\":\"consumption\"}],\"reason\":\"e2e price\"}"; check "price set" "$(st)" 200
req GET "/api/v1/energy?period=week"
check "cost = consumption × price" "$(jx 'Math.abs(j.total.cost - j.total.consumption * 0.2) < 0.01 && j.currency === "EUR"')" true
req GET /api/v1/energy/settings; check "meter listed with its role" "$(jx "(j.meters.find(m => m.datastream_id === '$DS') || {}).role")" consumption
req PUT /api/v1/energy/settings "{\"price_per_kwh\":0.2,\"currency\":\"EUR\",\"sources\":[{\"datastream_id\":\"$DS\",\"role\":\"production\"}],\"reason\":\"e2e production\"}"
req GET "/api/v1/energy?period=week"; check "meter as production" "$(jx "j.total.production >= 24 && (j.sources.find(s => s.datastream_id === '$DS') || {}).role === 'production'")" true
req PUT /api/v1/energy/settings "{\"price_per_kwh\":0,\"sources\":[{\"datastream_id\":\"$DS\",\"role\":\"off\"}],\"reason\":\"e2e off\"}"
req GET "/api/v1/energy?period=week"; check "meter not counted" "$(jx "(j.sources.find(s => s.datastream_id === '$DS') || {}).kwh")" 0

echo "== scenes"
req POST /api/v1/mqtt-sources "{\"kind\":\"account\",\"name\":\"Scenes $SUF\",\"reason\":\"e2e\"}"; SA=$(jx j.id); UA=$(jx j.username); PA=$(jx j.password)
"$TMP/mqttsim" z2m -addr "$MQTT" -user "$UA" -pass "$PA" -base "sc$SUF" -wait 60 > "$TMP/z2m.log" 2>&1 &
ents() { curl -s -b "$JAR" "$BASE/api/v1/entities?source=$SA" > "$TMP/e"; node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); console.log(eval(process.argv[2]))' "$TMP/e" "$1" 2>/dev/null; }
waitfor '[[ -n "$(ents "(j.find(e => e.device === \"Hall light\" && e.available) || {}).id || \"\"")" && -n "$(ents "(j.find(e => e.device === \"Kitchen plug\" && e.available) || {}).id || \"\"")" ]]' 60
LIGHT=$(ents "(j.find(e => e.device === 'Hall light') || {}).id"); PLUG=$(ents "(j.find(e => e.device === 'Kitchen plug') || {}).id")
req POST "/api/v1/entities/$LIGHT/command" '{"service":"turn_on","brightness":40}'
req POST "/api/v1/entities/$PLUG/command" '{"service":"turn_on"}'
waitfor '[[ "$(ents "(j.find(e => e.id === \"$LIGHT\") || {}).state + (j.find(e => e.id === \"$PLUG\") || {}).state")" == onon ]]' && ok "devices set" || bad "states"
req POST /api/v1/scenes "{\"name\":\"Evening $SUF\",\"icon\":\"moon\",\"capture\":[\"$LIGHT\",\"$PLUG\"]}"; check "reason required" "$(st)" 400
req POST /api/v1/scenes "{\"name\":\"Bad $SUF\",\"actions\":[{\"entity_id\":\"$LIGHT\",\"service\":\"fly\"}],\"reason\":\"e2e\"}"; check "unknown action refused" "$(st)" 400
req POST /api/v1/scenes "{\"name\":\"Evening $SUF\",\"icon\":\"moon\",\"capture\":[\"$LIGHT\",\"$PLUG\"],\"reason\":\"e2e\"}"; check "scene captured" "$(st)" 201
SC=$(jx j.id)
check "captured actions" "$(jx 'j.actions.map(a => a.service + (a.params.brightness ? "@" + a.params.brightness : "")).sort().join(",")')" "turn_on,turn_on@40"
req POST "/api/v1/entities/$LIGHT/command" '{"service":"turn_off"}'
req POST "/api/v1/entities/$PLUG/command" '{"service":"turn_off"}'
waitfor '[[ "$(ents "(j.find(e => e.id === \"$LIGHT\") || {}).state + (j.find(e => e.id === \"$PLUG\") || {}).state")" == offoff ]]' && ok "devices off" || bad "off"
: > "$TMP/z2m.mark"; N=$(wc -l < "$TMP/z2m.log")
req POST "/api/v1/scenes/$SC/activate" '{}'; check "scene activated" "$(st)" 200
check "two commands" "$(jx 'j.results.length + " " + j.failed')" "2 0"
waitfor '[[ "$(ents "(j.find(e => e.id === \"$LIGHT\") || {}).state + (j.find(e => e.id === \"$PLUG\") || {}).state")" == onon ]]' && ok "devices back as in the scene" || bad "scene states"
tail -n +"$((N+1))" "$TMP/z2m.log" | grep -q '"brightness":102' && ok "light received the brightness of the scene" || bad "z2m: $(tail -3 "$TMP/z2m.log")"
req GET /api/v1/home; check "scenes on the Home page" "$(jx "j.scenes.some(s => s.id === '$SC')")" true
req PUT "/api/v1/scenes/$SC" '{"name":"Night '$SUF'","reason":"e2e rename"}'; check "rename" "$(jx j.name)" "Night $SUF"

echo "== scene in a flow"
req POST "/api/v1/entities/$PLUG/command" '{"service":"turn_off"}'
SPEC="{\"nodes\":[{\"id\":\"i\",\"type\":\"inject\",\"x\":40,\"y\":40,\"settings\":{}},{\"id\":\"s\",\"type\":\"scene\",\"x\":260,\"y\":40,\"settings\":{\"scene\":\"$SC\"}},
 {\"id\":\"d\",\"type\":\"debug\",\"x\":480,\"y\":40,\"settings\":{\"expr\":\"msg.ok + '/' + msg.failed\"}}],\"wires\":[{\"from\":\"i\",\"port\":0,\"to\":\"s\"},{\"from\":\"s\",\"port\":0,\"to\":\"d\"}]}"
req POST /api/v1/flows "{\"name\":\"Scene flow $SUF\",\"reason\":\"e2e\",\"spec\":$SPEC}"; check "flow with a scene node" "$(st)" 201
FL=$(jx j.flow.slug); check "no problems" "$(jx 'j.problems.length')" 0
req POST "/api/v1/flows/$FL/deploy" '{"reason":"e2e"}'; check "deploy" "$(st)" 200
req POST "/api/v1/flows/$FL/inject" '{"node":"i","value":1}'
waitfor '[[ "$(curl -s -b "$JAR" "$BASE/api/v1/flows/$FL/debug" | node -pe "JSON.parse(require(\"fs\").readFileSync(0)).some(d => d.node === \"d\" && d.text === \"2/0\")")" == true ]]' 40 && ok "flow activated the scene" || bad "debug: $(curl -s -b "$JAR" "$BASE/api/v1/flows/$FL/debug" | head -c 300)"
waitfor '[[ "$(ents "(j.find(e => e.id === \"$PLUG\") || {}).state")" == on ]]' && ok "plug switched by the flow's scene" || bad "plug"

echo "== cleanup"
req DELETE "/api/v1/flows/$FL?reason=e2e"
req DELETE "/api/v1/scenes/$SC"; check "delete needs a reason" "$(st)" 400
req DELETE "/api/v1/scenes/$SC?reason=e2e"; check "scene deleted" "$(st)" 200
req POST "/api/v1/scenes/$SC/activate" '{}'; check "deleted scene gone" "$(st)" 404
req DELETE "/api/v1/mqtt-sources/$SA?reason=e2e"
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

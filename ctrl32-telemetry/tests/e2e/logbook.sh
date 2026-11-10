#!/usr/bin/env bash
# End-to-end checks of the logbook (0.87.0): commands (who, where, key, value, reason, status) and alarm events
# (raised, acknowledged with who and why) in one list, filters by devices, keys (prefix*) and kinds, input checks,
# and the click action "popup" of process pictures. Needs Go (tools/devfill) and Node.
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 USER=admin PASS=admin1234 bash tests/e2e/logbook.sh
set -u
cd "$(dirname "$0")/../.."
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
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
go build -o "$TMP/devfill" ./tools/devfill || { echo "devfill build failed"; exit 1; }
SUF=$RANDOM
pub() { "$TMP/devfill" -addr "$MQTT" -user "lb-$SUF" -token "$TOK" -key "$1" -start "$2" -step 0 -n 1 -every 1 > "$TMP/pub.log" 2>&1 || bad "publish $1=$2: $(tail -1 "$TMP/pub.log")"; }

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req POST /api/v1/sites "{\"name\":\"Logbook site $SUF\",\"reason\":\"e2e\"}"; SITE=$(jx j.id)
req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"mixer\",\"name\":\"Logbook mixer $SUF\",\"reason\":\"e2e\"}"; ASSET=$(jx j.id)
req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"temp\",\"quantity\":\"temperature\",\"unit\":\"C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}"; TEMP=$(jx j.id)
req POST /api/v1/devices "{\"external_id\":\"lb-$SUF\",\"name\":\"Logbook mixer $SUF\",\"reason\":\"e2e\"}"; DEV=$(jx j.id)
req POST /api/v1/devices "{\"external_id\":\"lb2-$SUF\",\"name\":\"Other mixer $SUF\",\"reason\":\"e2e\"}"; DEV2=$(jx j.id)
req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}'; TOK=$(jx j.token)
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$TEMP\",\"reason\":\"e2e\"}"; check "assigned" "$(st)" 201
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$TEMP\",\"type\":\"high\",\"warning\":30,\"action\":40,\"reason\":\"e2e\"}"; check "alarm rule" "$(st)" 201

echo "== commands: who, where, what, value, reason"
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"sp_temp","value":52},"reason":"e2e recipe: higher temperature"}'; check "setpoint command" "$(st)" 201
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"agi_auto","value":0},"reason":"e2e manual mode"}'; check "mode command" "$(st)" 201
req POST "/api/v1/devices/$DEV2/commands" '{"method":"write","args":{"key":"sp_temp","value":40},"reason":"e2e other mixer"}'; check "command to another device" "$(st)" 201
req GET "/api/v1/logbook?devices=$DEV&kinds=commands"
check "two commands of the device" "$(jx j.items.length)" 2
check "newest first: mode" "$(jx 'j.items[0].key+" "+j.items[0].value+" "+j.items[0].user')" "agi_auto 0 $USER"
check "setpoint entry" "$(jx 'j.items[1].kind+" "+j.items[1].event+" "+j.items[1].key+" "+j.items[1].value+" "+j.items[1].device+" | "+j.items[1].reason')" "command write sp_temp 52 Logbook mixer $SUF | e2e recipe: higher temperature"
check "status of the command is kept" "$(jx '["queued","sent"].includes(j.items[1].status)')" "true"
req GET "/api/v1/logbook?devices=$DEV&keys=sp_*"; check "key prefix filter" "$(jx 'j.items.map(e=>e.key).join(",")')" "sp_temp"
req GET "/api/v1/logbook?devices=$DEV&keys=agi_auto,temp"; check "exact key filter" "$(jx 'j.items.map(e=>e.key).join(",")')" "agi_auto"
req GET "/api/v1/logbook?devices=$DEV,$DEV2&kinds=commands&limit=2"; check "limit" "$(jx j.items.length)" 2

echo "== alarm events: raised, acknowledged by whom and why"
pub temp 45
for i in $(seq 1 20); do req GET "/api/v1/alarms?datastream_id=$TEMP"; [[ "$(jx 'j.length')" == "1" ]] && break; sleep 0.5; done
ALARM=$(jx 'j[0].id'); [[ -n "$ALARM" && "$ALARM" != "null" ]] && ok "alarm raised" || bad "no alarm"
req POST "/api/v1/alarms/$ALARM/ack" '{"reason":"e2e operator checked the jacket"}'; check "acknowledged" "$(st)" 200
req GET "/api/v1/logbook?devices=$DEV&kinds=alarms"
check "raised and acknowledged" "$(jx 'j.items.map(e=>e.event).join(",")')" "acknowledged,raised"
check "acknowledged by whom and why" "$(jx 'j.items[0].user+" | "+j.items[0].reason+" | "+j.items[0].key+" | "+j.items[0].device')" "$USER | e2e operator checked the jacket | temp | Logbook mixer $SUF"
check "raised with severity and value" "$(jx 'j.items[1].severity+" "+j.items[1].value')" "action 45"
req GET "/api/v1/logbook?devices=$DEV"; check "both kinds merged" "$(jx 'j.items.map(e=>e.kind).join(",")')" "alarm,alarm,command,command"
req GET "/api/v1/logbook?devices=$DEV2"; check "other device: its command only" "$(jx 'j.items.map(e=>e.kind+":"+e.key).join(",")')" "command:sp_temp"

echo "== input checks"
req GET "/api/v1/logbook?devices=not-an-id"; check "bad device id" "$(st)" 400
req GET "/api/v1/logbook?keys=a%20b"; check "bad key" "$(st)" 400
req GET "/api/v1/logbook?before=yesterday"; check "bad time" "$(st)" 400
req GET "/api/v1/logbook?before=2000-01-01T00:00:00Z"; check "before: nothing that old" "$(jx j.items.length)" 0
curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/logbook" > "$TMP/s"; check "anonymous refused" "$([[ $(st) == 401 || $(st) == 403 ]] && echo refused || st)" "refused"

echo "== click action popup"
SPEC='{"kind":"mimic","layout":{"type":"canvas","width":800,"height":600},"elements":[{"type":"symbol","symbol":"motor","x":10,"y":10,"w":100,"h":80,"on":[{"act":"popup","target":"lb-motor-'$SUF'"}]}],"widgets":[{"id":"lb","type":"logbook","geometry":{"x":200,"y":10,"w":500,"h":300},"bindings":{"devices":["'$DEV'"]},"params":{"title":"Logbook","limit":10,"keys":"sp_*,agi_auto","labels":"sp_temp=Setpoint"}}]}'
req POST /api/v1/dashboards "{\"slug\":\"lb-pic-$SUF\",\"name\":\"Logbook picture $SUF\",\"spec\":$SPEC,\"reason\":\"e2e\"}"; check "picture with a popup action and a logbook widget" "$(st)" 201
req GET "/api/v1/dashboards/lb-pic-$SUF"; check "popup target kept" "$(jx 'j.spec.elements[0].on[0].act+" "+j.spec.elements[0].on[0].target')" "popup lb-motor-$SUF"
check "logbook params kept" "$(jx 'j.spec.widgets[0].params.keys')" "sp_*,agi_auto"
BAD='{"kind":"mimic","layout":{"type":"canvas","width":800,"height":600},"elements":[{"type":"rect","on":[{"act":"popup"}]}]}'
req POST /api/v1/dashboards "{\"slug\":\"lb-bad-$SUF\",\"name\":\"bad\",\"spec\":$BAD,\"reason\":\"e2e\"}"; check "popup without a target refused" "$(st)" 400
req DELETE "/api/v1/dashboards/lb-pic-$SUF?reason=e2e"

echo
echo "logbook: $pass passed, $fail failed"
[[ $fail -eq 0 ]]

#!/usr/bin/env bash
# End-to-end checks of alarm rule types and delays (URS C-01, C-03): every type that can be saved is evaluated
# (low_battery, power_loss, door_open, comm_loss; sensor_fault and ignored limits are refused), the delay holds an alarm
# back until the condition has lasted, acknowledging needs a reason and fires the flows' Alarm trigger "acknowledged",
# Incident nodes pair open and resolve by an incident key, and an escalation policy used by an automation rule cannot
# be revoked. Needs Go (tools/devfill) and Node.
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 USER=admin PASS=admin1234 bash tests/e2e/alarms.sh
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
pub() { "$TMP/devfill" -addr "$MQTT" -user "al-$SUF" -token "$TOK" -key "$1" -start "$2" -step 0 -n 1 -every 1 > "$TMP/pub.log" 2>&1 || bad "publish $1=$2: $(tail -1 "$TMP/pub.log")"; }
# the newest alarm of a datastream as "state severity cleared?"
alarm() { req GET "/api/v1/alarms?datastream_id=$1"; jx 'j.length ? j[0].state + " " + j[0].severity + " " + (j[0].cleared_at ? "cleared" : "open") : "none"'; }
SUF=$RANDOM

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req POST /api/v1/sites "{\"name\":\"Alarm site $SUF\",\"reason\":\"e2e\"}"; SITE=$(jx j.id)
req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"room\",\"name\":\"Alarm room $SUF\",\"reason\":\"e2e\"}"; ASSET=$(jx j.id)
mkds() { req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"$1\",\"quantity\":\"$2\",\"unit\":\"$3\",\"kind\":\"$4\",\"reason\":\"e2e\"}"; jx j.id; }
DOOR=$(mkds door state "" state); BAT=$(mkds bat battery % gauge); PWR=$(mkds pwr state "" state); LINK=$(mkds link temperature C gauge)
[[ -n "$DOOR" && -n "$BAT" && -n "$PWR" && -n "$LINK" ]] && ok "four datastreams" || bad "datastreams"
req POST /api/v1/devices "{\"external_id\":\"al-$SUF\",\"name\":\"alarm test\",\"reason\":\"e2e\"}"; DEV=$(jx j.id)
req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}'; TOK=$(jx j.token)
for d in $DOOR $BAT $PWR $LINK; do req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$d\",\"reason\":\"e2e\"}"; done; check "assigned" "$(st)" 201

echo "== rules that cannot be evaluated are refused"
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$LINK\",\"type\":\"sensor_fault\",\"reason\":\"e2e\"}"; check "sensor_fault refused" "$(st) $(jx 'j.error.includes("cannot be evaluated")')" "400 true"
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$PWR\",\"type\":\"power_loss\",\"action\":5,\"reason\":\"e2e\"}"; check "power_loss with a limit refused" "$(st)" 400
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$LINK\",\"type\":\"comm_loss\",\"reason\":\"e2e\"}"; check "comm_loss without a limit or an expected interval refused" "$(st)" 400
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$BAT\",\"type\":\"low_battery\",\"reason\":\"e2e\"}"; check "low_battery without limits refused" "$(st)" 400

echo "== rules of every type"
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$DOOR\",\"type\":\"door_open\",\"delay_seconds\":4,\"reason\":\"e2e door may stay open 4 s\"}"; check "door_open with a delay" "$(st)" 201
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$BAT\",\"type\":\"low_battery\",\"warning\":20,\"action\":10,\"reason\":\"e2e\"}"; check "low_battery" "$(st)" 201
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$PWR\",\"type\":\"power_loss\",\"reason\":\"e2e\"}"; check "power_loss" "$(st)" 201
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$LINK\",\"type\":\"comm_loss\",\"warning\":6,\"reason\":\"e2e\"}"; check "comm_loss after 6 s" "$(st)" 201
pub link 21; pub pwr 1; pub door 0
sleep 1

echo "== delay: the door may stay open 4 s"
pub door 1; sleep 2
check "open 2 s: no alarm yet" "$(alarm "$DOOR")" "none"
sleep 4
check "open over 4 s: warning raised" "$(alarm "$DOOR")" "active warning open"
pub door 0; sleep 2
check "closed: cleared" "$(alarm "$DOOR")" "cleared warning cleared"
pub door 1; sleep 1; pub door 0; sleep 5
check "a short opening raises nothing" "$(req GET "/api/v1/alarms?datastream_id=$DOOR"; jx 'j.length')" 1

echo "== low battery, power loss"
pub bat 15; sleep 2; check "battery 15 %: warning" "$(alarm "$BAT")" "active warning open"
pub bat 8; sleep 2; check "battery 8 %: action" "$(alarm "$BAT")" "active action open"
pub pwr 0; sleep 2; check "power 0: action" "$(alarm "$PWR")" "active action open"
pub pwr 1; sleep 2; check "power back: cleared" "$(alarm "$PWR")" "cleared action cleared"

echo "== communication loss"
pub link 21; sleep 12
check "no value for 6 s: warning" "$(alarm "$LINK")" "active warning open"
req GET "/api/v1/alarms?datastream_id=$LINK"; check "the value is the silence in seconds" "$(jx 'j[0].value >= 6')" true
pub link 22; sleep 2
check "a new value clears it" "$(alarm "$LINK")" "cleared warning cleared"

echo "== acknowledge fires the flow trigger"
req POST /api/v1/flows "{\"name\":\"Ack $SUF\",\"reason\":\"e2e\",\"spec\":{\"nodes\":[{\"id\":\"a\",\"type\":\"alarm_event\",\"x\":0,\"y\":0,\"settings\":{\"events\":[\"acknowledged\"],\"datastream\":\"$BAT\"}},{\"id\":\"d\",\"type\":\"debug\",\"x\":200,\"y\":0,\"settings\":{\"expr\":\"msg.event + ' ' + msg.severity\"}}],\"wires\":[{\"from\":\"a\",\"port\":0,\"to\":\"d\"}]}}"; AFLOW=$(jx j.flow.slug)
req POST "/api/v1/flows/$AFLOW/deploy" '{"reason":"e2e"}'; check "flow with the Alarm trigger deployed" "$(st)" 200
req GET "/api/v1/alarms?datastream_id=$BAT"; ALID=$(jx 'j[0].id')
req POST "/api/v1/alarms/$ALID/ack" '{"reason":""}'; check "acknowledge needs a reason" "$(st)" 400
req POST "/api/v1/alarms/$ALID/ack" '{"reason":"e2e: battery replaced"}'; check "acknowledged" "$(st)" 200
sleep 1
req GET "/api/v1/flows/$AFLOW/debug"; check "the flow's Alarm trigger fired on acknowledged" "$(jx 'j.some(d => d.text === "acknowledged action")')" true
check "state acknowledged" "$(alarm "$BAT")" "acknowledged action open"
pub bat 80; sleep 2; check "battery full: cleared" "$(alarm "$BAT")" "cleared action cleared"
req POST "/api/v1/flows/$AFLOW/disable" '{"reason":"e2e cleanup"}'

echo "== incident key pairs open and resolve nodes"
ISPEC="{\"nodes\":[{\"id\":\"i1\",\"type\":\"inject\",\"x\":0,\"y\":0,\"settings\":{\"value\":\"1\"}},{\"id\":\"i2\",\"type\":\"inject\",\"x\":0,\"y\":100,\"settings\":{\"value\":\"0\"}},{\"id\":\"o\",\"type\":\"incident\",\"x\":200,\"y\":0,\"settings\":{\"action\":\"open\",\"severity\":\"minor\",\"title\":\"Pump $SUF stopped\",\"key\":\"pump\"}},{\"id\":\"r\",\"type\":\"incident\",\"x\":200,\"y\":100,\"settings\":{\"action\":\"resolve\",\"severity\":\"minor\",\"title\":\"Pump $SUF runs\",\"key\":\"pump\"}}],\"wires\":[{\"from\":\"i1\",\"port\":0,\"to\":\"o\"},{\"from\":\"i2\",\"port\":0,\"to\":\"r\"}]}"
req POST /api/v1/flows "{\"name\":\"Incident $SUF\",\"reason\":\"e2e\",\"spec\":$ISPEC}"; IFLOW=$(jx j.flow.slug)
req POST "/api/v1/flows/$IFLOW/deploy" '{"reason":"e2e"}'; check "incident flow deployed" "$(st)" 200
req POST "/api/v1/flows/$IFLOW/inject" '{"node":"i1","value":1}'; check "open" "$(st)" 202
sleep 1
req GET "/api/v1/incidents?status=all"; check "incident opened" "$(jx "(j.find(i => i.title === 'Pump $SUF stopped') || {}).status")" open
req POST "/api/v1/flows/$IFLOW/inject" '{"node":"i2","value":0}'; check "resolve" "$(st)" 202
sleep 1
req GET "/api/v1/incidents?status=all"; check "the other node resolved it" "$(jx "(j.find(i => i.title === 'Pump $SUF stopped') || {}).status")" resolved
req POST "/api/v1/flows/$IFLOW/disable" '{"reason":"e2e cleanup"}'

echo "== an escalation policy used by an automation rule cannot be revoked"
req POST /api/v1/notification-channels "{\"name\":\"al-mail-$SUF\",\"kind\":\"email\",\"settings\":{\"recipients\":[\"ops@example.com\"]},\"reason\":\"e2e\"}"; CH=$(jx j.id)
req POST /api/v1/escalation-policies "{\"name\":\"al-pol-$SUF\",\"steps\":[{\"after_s\":0,\"channel_id\":\"$CH\"}],\"reason\":\"e2e\"}"; POL=$(jx j.id); check "policy" "$(st)" 201
req POST /api/v1/rules "{\"name\":\"al-rule-$SUF\",\"spec\":{\"inputs\":[{\"name\":\"b\",\"datastream_id\":\"$BAT\"}],\"conditions\":[{\"input\":\"b\",\"op\":\"<\",\"value\":5}],\"actions\":[{\"type\":\"alarm\",\"severity\":\"action\",\"escalation_policy_id\":\"$POL\"}]},\"reason\":\"e2e\"}"; RULE=$(jx j.id); check "automation rule with the policy" "$(st)" 201
req DELETE "/api/v1/escalation-policies/$POL?reason=e2e"; check "revoking the policy refused" "$(st) $(jx 'j.error.includes("automation rule")')" "409 true"
req DELETE "/api/v1/rules/$RULE?reason=e2e%20cleanup"; check "rule revoked" "$(st)" 200
req DELETE "/api/v1/escalation-policies/$POL?reason=e2e"; check "then the policy can be revoked" "$(st)" 200
req DELETE "/api/v1/notification-channels/$CH?reason=e2e"

echo "== cleanup"
req GET "/api/v1/alarm-rules"; for id in $(jx "j.filter(r => ['$DOOR','$BAT','$PWR','$LINK'].includes(r.datastream_id)).map(r => r.id).join(' ')"); do req DELETE "/api/v1/alarm-rules/$id?reason=e2e%20cleanup"; done
req GET "/api/v1/alarm-rules"; check "rules disabled" "$(jx "j.filter(r => ['$DOOR','$BAT','$PWR','$LINK'].includes(r.datastream_id)).length")" 0
req POST "/api/v1/devices/$DEV/revoke" '{"reason":"e2e cleanup"}'; check "device revoked" "$(st)" 200
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

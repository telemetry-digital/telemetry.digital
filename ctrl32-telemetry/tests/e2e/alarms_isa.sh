#!/usr/bin/env bash
# End-to-end checks of alarm management after ISA-18.2 (spec 035): alarm classes, the ISA states (an alarm that returned
# to normal waits for its acknowledgement), acknowledging several at once, a class without acknowledgement, shelving
# (recorded but not annunciated, a new alarm of the rule too), the summary for the alarm bar, history, statistics and
# permissions. Needs Go (tools/devfill) and Node.
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 USER=admin PASS=admin1234 bash tests/e2e/alarms_isa.sh
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
pub() { "$TMP/devfill" -addr "$MQTT" -user "isa-$SUF" -token "$TOK" -key "$1" -start "$2" -step 0 -n 1 -every 1 > "$TMP/pub.log" 2>&1 || bad "publish $1=$2: $(tail -1 "$TMP/pub.log")"; }
SUF=$RANDOM
# the newest alarm of a datastream: an expression over it (a)
al() { req GET "/api/v1/alarms?datastream_id=$1"; jx "(a => a ? ($2) : 'none')(j[0])"; }
cur() { req GET "/api/v1/alarms?view=current"; jx "j.filter(a => a.datastream_id === '$1').map(a => a.isa_state).join(',')"; }

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req POST /api/v1/sites "{\"name\":\"ISA site $SUF\",\"reason\":\"e2e\"}"; SITE=$(jx j.id)
req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"room\",\"name\":\"ISA tank $SUF\",\"reason\":\"e2e\"}"; ASSET=$(jx j.id)
mkds() { req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"$1\",\"quantity\":\"$2\",\"unit\":\"$3\",\"kind\":\"gauge\",\"reason\":\"e2e\"}"; jx j.id; }
TEMP=$(mkds temp temperature C); LVL=$(mkds lvl level m)
[[ -n "$TEMP" && -n "$LVL" ]] && ok "two datastreams" || bad "datastreams"
req POST /api/v1/devices "{\"external_id\":\"isa-$SUF\",\"name\":\"isa test\",\"reason\":\"e2e\"}"; DEV=$(jx j.id)
req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}'; TOK=$(jx j.token)
for d in $TEMP $LVL; do req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$d\",\"reason\":\"e2e\"}"; done; check "assigned" "$(st)" 201

echo "== alarm classes"
req GET /api/v1/alarm-classes; check "three built-in classes" "$(jx "j.filter(c => c.builtin).map(c => c.code + ':' + c.priority + ':' + c.ack_required + ':' + c.sound).join(' ')")" "alarm:1:true:siren warning:2:true:beep message:4:false:none"
ALARMCLS=$(jx "j.find(c => c.code === 'alarm').id")
req POST /api/v1/alarm-classes "{\"code\":\"info$SUF\",\"name\":\"Info $SUF\",\"priority\":3,\"color\":\"#2fb344\",\"ack_required\":false,\"sound\":\"none\"}"; check "class without a reason refused" "$(st)" 400
req POST /api/v1/alarm-classes "{\"code\":\"info$SUF\",\"name\":\"Info $SUF\",\"priority\":3,\"color\":\"#2fb344\",\"ack_required\":false,\"sound\":\"none\",\"reason\":\"e2e\"}"; check "class created" "$(st)" 201; INFO=$(jx j.id)
req POST /api/v1/alarm-classes "{\"code\":\"info$SUF\",\"name\":\"Again\",\"priority\":3,\"color\":\"#2fb344\",\"reason\":\"e2e\"}"; check "same code refused" "$(st)" 409
req POST /api/v1/alarm-classes "{\"code\":\"x$SUF\",\"name\":\"X\",\"priority\":7,\"color\":\"#2fb344\",\"reason\":\"e2e\"}"; check "priority outside 1..4 refused" "$(st)" 400
req POST /api/v1/alarm-classes "{\"code\":\"x$SUF\",\"name\":\"X\",\"priority\":2,\"color\":\"green\",\"reason\":\"e2e\"}"; check "colour must be #rrggbb" "$(st)" 400
req PUT "/api/v1/alarm-classes/$INFO" "{\"name\":\"Info $SUF b\",\"priority\":4,\"color\":\"#2fb344\",\"sound\":\"beep\",\"reason\":\"e2e\"}"; check "class changed" "$(st)" 200
req GET /api/v1/alarm-classes; check "change stored, acknowledgement kept" "$(jx "(c => c.name + ' ' + c.priority + ' ' + c.sound + ' ' + c.ack_required)(j.find(c => c.id === '$INFO'))")" "Info $SUF b 4 beep false"
req DELETE "/api/v1/alarm-classes/$ALARMCLS?reason=e2e"; check "built-in class cannot be deleted" "$(st)" 409

echo "== rules with a class"
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$TEMP\",\"type\":\"high\",\"action\":30,\"hysteresis\":1,\"reason\":\"e2e\"}"; check "rule without a class" "$(st)" 201
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$LVL\",\"type\":\"high\",\"action\":5,\"class_id\":\"$SITE\",\"reason\":\"e2e\"}"; check "unknown class refused" "$(st)" 400
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$LVL\",\"type\":\"high\",\"action\":5,\"class_id\":\"$INFO\",\"reason\":\"e2e\"}"; check "rule with a class" "$(st)" 201
req GET "/api/v1/alarm-rules?datastream_id=$LVL"; check "the rule names its class" "$(jx j[0].class_id)" "$INFO"
req DELETE "/api/v1/alarm-classes/$INFO?reason=e2e"; check "a class in use cannot be deleted" "$(st)" 409
pub temp 20; pub lvl 1; sleep 1

echo "== ISA states: active, returned to normal unacknowledged, acknowledged"
pub temp 35; sleep 1
check "raised: unacknowledged, active, class Alarm" "$(al "$TEMP" "a.isa_state + ' ' + a.state + ' ' + a.class.code + ' ' + a.limit + ' ' + a.source.asset")" "unack_active active alarm 30 ISA tank $SUF"
A1=$(al "$TEMP" a.id)
check "in the current list" "$(cur "$TEMP")" "unack_active"
req GET /api/v1/alarms/summary; check "summary counts it" "$(jx "(j.unack >= 1) + ' ' + (j.active >= 1) + ' ' + (j.top !== null)")" "true true true"
pub temp 20; sleep 1
check "returned to normal, still unacknowledged" "$(al "$TEMP" "a.isa_state + ' ' + a.state")" "unack_rtn cleared"
check "stays in the current list" "$(cur "$TEMP")" "unack_rtn"
req POST "/api/v1/alarms/$A1/ack" '{"reason":""}'; check "acknowledging needs a reason" "$(st)" 400
req POST "/api/v1/alarms/$A1/ack" '{"reason":"seen, e2e"}'; check "an alarm that returned to normal can be acknowledged" "$(st)" 200
check "then it is normal" "$(al "$TEMP" "a.isa_state + ' ' + a.state + ' ' + a.acked_by + ' ' + (a.acked_at !== null)")" "normal cleared $USER true"
check "and leaves the current list" "$(cur "$TEMP")" ""
req POST "/api/v1/alarms/$A1/ack" '{"reason":"again"}'; check "a second acknowledgement finds nothing" "$(st)" 404

echo "== a class without acknowledgement"
pub lvl 9; sleep 1
check "active without waiting for anyone" "$(al "$LVL" "a.isa_state + ' ' + a.class.code + ' ' + a.class.ack_required")" "ack_active info$SUF false"
pub lvl 1; sleep 1
check "normal as soon as it returns" "$(al "$LVL" a.isa_state)" "normal"
check "not in the current list" "$(cur "$LVL")" ""

echo "== acknowledging several at once"
pub temp 36; sleep 1; A2=$(al "$TEMP" a.id)
req POST /api/v1/alarms/ack '{"ids":[],"reason":"e2e"}'; check "no alarms refused" "$(st)" 400
req POST /api/v1/alarms/ack "{\"ids\":[\"$A2\",\"$A1\"],\"reason\":\"e2e shift handover\"}"; check "bulk acknowledgement counts only the unacknowledged" "$(st) $(jx j.acknowledged)" "200 1"
check "acknowledged and active" "$(al "$TEMP" "a.isa_state + ' ' + a.state")" "ack_active acknowledged"

echo "== shelving"
req POST "/api/v1/alarms/$A2/shelve" '{"minutes":5,"reason":"e2e sensor being replaced"}'; check "shorter than 15 minutes refused" "$(st)" 400
req POST "/api/v1/alarms/$A2/shelve" '{"minutes":2000,"reason":"e2e"}'; check "longer than 24 hours refused" "$(st)" 400
req POST "/api/v1/alarms/$A2/shelve" '{"minutes":30,"reason":""}'; check "shelving needs a reason" "$(st)" 400
req POST "/api/v1/alarms/$A2/shelve" '{"minutes":30,"reason":"e2e sensor being replaced"}'; check "shelved" "$(st) $(jx j.alarms)" "200 1"
check "the alarm is shelved" "$(al "$TEMP" "a.isa_state + ' ' + a.state + ' ' + (a.shelved_until !== null) + ' ' + a.shelf.by + ' ' + a.shelf.reason")" "shelved suppressed true $USER e2e sensor being replaced"
check "not in the current list" "$(cur "$TEMP")" ""
req GET "/api/v1/alarms?view=shelved"; check "in the shelved list" "$(jx "j.some(a => a.id === '$A2')")" "true"
req POST "/api/v1/alarms/$A2/shelve" '{"minutes":30,"reason":"e2e again"}'; check "already shelved" "$(st)" 409
pub temp 20; sleep 1; pub temp 37; sleep 1
A3=$(al "$TEMP" a.id)
check "a new alarm of the shelved rule is recorded shelved" "$([[ "$A3" != "$A2" ]] && echo new) $(al "$TEMP" "a.isa_state + ' ' + a.state")" "new shelved suppressed"
req GET /api/v1/alarms/summary; check "summary counts shelved apart" "$(jx "j.shelved >= 1")" "true"
req GET /api/v1/alarms/summary; check "the bar does not show a shelved alarm" "$(jx "j.top === null || j.top.id !== '$A3'")" "true"
req POST "/api/v1/alarms/$A3/unshelve" '{"reason":""}'; check "unshelving needs a reason" "$(st)" 400
req POST "/api/v1/alarms/$A3/unshelve" '{"reason":"e2e sensor replaced"}'; check "unshelved" "$(st) $(jx j.alarms)" "200 1"
check "back, unacknowledged and active" "$(al "$TEMP" "a.isa_state + ' ' + a.state + ' ' + a.shelved_until")" "unack_active active null"
check "in the current list again" "$(cur "$TEMP")" "unack_active"
req POST "/api/v1/alarms/$A3/unshelve" '{"reason":"e2e again"}'; check "not shelved any more" "$(st)" 409
req GET "/api/v1/export/audit?format=csv"; check "shelving audited" "$(grep -c 'alarm.suppress\|alarm.unsuppress' "$TMP/b" | awk '{print ($1 >= 2)}')" 1

echo "== history and statistics"
req GET "/api/v1/alarms?view=history&q=ISA%20tank%20$SUF"; check "history finds the alarms of the asset" "$(jx "j.length")" 4
req GET "/api/v1/alarms?view=history&q=ISA%20tank%20$SUF&class=info$SUF"; check "history by class" "$(jx "j.length + ' ' + j[0].key")" "1 lvl"
req GET "/api/v1/alarms?view=nonsense"; check "unknown view refused" "$(st)" 400
req GET "/api/v1/alarms/stats"; check "statistics" "$(st) $(jx "(j.total >= 3) + ' ' + (j.days.length >= 1) + ' ' + (j.top.length >= 1) + ' ' + (j.per_hour > 0) + ' ' + (j.mean_ack_s > 0) + ' ' + j.target_per_hour")" "200 true true true true true 6"
req GET "/api/v1/alarms/stats"; check "the most frequent alarms carry a share" "$(jx "j.top.every(t => t.count >= 1 && t.share > 0 && t.share <= 1)")" "true"
req GET "/api/v1/alarms/stats?from=2026-01-01T00:00:00Z&to=2026-12-01T00:00:00Z"; check "range limited to 92 days" "$(st)" 400

echo "== permissions"
req POST /api/v1/users "{\"username\":\"isaview_$SUF\",\"full_name\":\"Viewer\",\"email\":\"isaview$SUF@example.com\",\"roles\":[\"viewer\"],\"reason\":\"e2e\"}"; check "viewer created" "$(st)" 201
VU=$(jx j.id); VPW=$(jx j.initial_password); NPW="Isa-Viewer-Passw0rd-$SUF"
J2="$TMP/jar2"; C2=""
v() { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then curl -s -b "$J2" -c "$J2" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $C2" -o "$TMP/b" -w '%{http_code}' --data-binary "$d" "$BASE$p" > "$TMP/s"
  else curl -s -b "$J2" -c "$J2" -X "$m" -H "X-CSRF-Token: $C2" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
v POST /api/v1/auth/login "{\"username\":\"isaview_$SUF\",\"password\":\"$VPW\"}"; v GET /api/v1/auth/me; C2=$(jx j.csrf_token)
v POST /api/v1/auth/password "{\"current\":\"$VPW\",\"new\":\"$NPW\"}"
v POST /api/v1/auth/login "{\"username\":\"isaview_$SUF\",\"password\":\"$NPW\"}"; v GET /api/v1/auth/me; C2=$(jx j.csrf_token)
v GET /api/v1/alarms/summary; check "a viewer reads the summary" "$(st)" 200
v POST "/api/v1/alarms/$A3/ack" '{"reason":"viewer"}'; check "a viewer cannot acknowledge" "$(st)" 403
v POST "/api/v1/alarms/$A3/shelve" '{"minutes":30,"reason":"viewer"}'; check "a viewer cannot shelve" "$(st)" 403
v POST /api/v1/alarm-classes "{\"code\":\"v$SUF\",\"name\":\"V\",\"priority\":2,\"color\":\"#2fb344\",\"reason\":\"viewer\"}"; check "a viewer cannot change classes" "$(st)" 403

echo "== cleanup"
pub temp 20; sleep 1
req POST "/api/v1/alarms/$A3/ack" '{"reason":"e2e cleanup"}'; check "last alarm acknowledged" "$(st)" 200
req GET "/api/v1/alarm-rules"; for id in $(jx "j.filter(r => ['$TEMP','$LVL'].includes(r.datastream_id)).map(r => r.id).join(' ')"); do req DELETE "/api/v1/alarm-rules/$id?reason=e2e%20cleanup"; done
req GET "/api/v1/alarm-rules"; check "rules disabled" "$(jx "j.filter(r => ['$TEMP','$LVL'].includes(r.datastream_id)).length")" 0
req POST "/api/v1/devices/$DEV/revoke" '{"reason":"e2e cleanup"}'; check "device revoked" "$(st)" 200
[[ -n "${VU:-}" ]] && req DELETE "/api/v1/users/$VU?reason=e2e%20cleanup"
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

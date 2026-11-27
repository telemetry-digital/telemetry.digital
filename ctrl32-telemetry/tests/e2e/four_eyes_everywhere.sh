#!/usr/bin/env bash
# End-to-end check of four eyes everywhere (spec 040, 0.94.0, ADR-0026 step 2): every switch is off by default; a
# control marked "needs a second person" holds its command; alarms and device attributes are held as change requests;
# cameras and displays need an approval that lasts some minutes on one station; everything can be approved from the
# list by a different user or on the spot with the second person's name and password, and never by the one who asked.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/four_eyes_everywhere.sh
set -u
cd "$(dirname "$0")/../.."
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""; JAR2="$TMP/jar2"; CSRF2=""; JAR3="$TMP/jar3"; CSRF3=""
pass=0; fail=0
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
call() { local jar=$1 csrf=$2 m=$3 p=$4 d=${5:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$jar" -c "$jar" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $csrf" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$jar" -c "$jar" -X "$m" -H "X-CSRF-Token: $csrf" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
req()  { call "$JAR" "$CSRF" "$@"; }
req2() { call "$JAR2" "$CSRF2" "$@"; }
req3() { call "$JAR3" "$CSRF3" "$@"; }
go build -o "$TMP/devfill" ./tools/devfill || { echo "devfill build failed"; exit 1; }
SUF=$RANDOM
pub() { "$TMP/devfill" -addr "$MQTT" -user "fe4-$SUF" -token "$TOK" -key "$1" -start "$2" -step 0 -n 1 -every 1 > "$TMP/pub.log" 2>&1 || bad "publish $1=$2: $(tail -1 "$TMP/pub.log")"; }
al() { req GET "/api/v1/alarms?datastream_id=$1"; jx "(a => a ? ($2) : 'none')(j[0])"; }
NOCAM=00000000-0000-7000-8000-000000000000
ALLOFF='{"four_eyes_commands":false,"four_eyes_alarms":false,"four_eyes_attributes":false,"four_eyes_cameras":false,"four_eyes_displays":false,"four_eyes_minutes":15,"reason":"e2e reset"}'

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req PUT /api/v1/settings/policy "$ALLOFF"; check "everything off" "$(st)" 200
SECOND="fe4_$SUF"; NEWPW="Second-Person-Passw0rd-$SUF"
req POST /api/v1/users "{\"username\":\"$SECOND\",\"full_name\":\"Second Person\",\"email\":\"fe4$SUF@example.com\",\"roles\":[\"engineer\"],\"reason\":\"e2e\"}"; check "a second user" "$(st)" 201
PW=$(jx j.initial_password)
req2 POST /api/v1/auth/login "{\"username\":\"$SECOND\",\"password\":\"$PW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
req2 POST /api/v1/auth/password "{\"current\":\"$PW\",\"new\":\"$NEWPW\"}"
req2 POST /api/v1/auth/login "{\"username\":\"$SECOND\",\"password\":\"$NEWPW\"}"; check "the second user signs in" "$(st)" 200
req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
req POST /api/v1/sites "{\"name\":\"FE4 site $SUF\",\"reason\":\"e2e\"}"; SITE=$(jx j.id)
req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"room\",\"name\":\"FE4 tank $SUF\",\"reason\":\"e2e\"}"; ASSET=$(jx j.id)
req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"temp\",\"quantity\":\"temperature\",\"unit\":\"C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}"; TEMP=$(jx j.id)
req POST /api/v1/devices "{\"external_id\":\"fe4-$SUF\",\"name\":\"Motor M$SUF\",\"reason\":\"e2e\"}"; DEV=$(jx j.id); check "device" "$(st)" 201
req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}'; TOK=$(jx j.token)
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$TEMP\",\"reason\":\"e2e\"}"; check "assigned" "$(st)" 201
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$TEMP\",\"type\":\"high\",\"action\":30,\"hysteresis\":1,\"reason\":\"e2e\"}"; check "alarm rule" "$(st)" 201
pub temp 20; sleep 1; pub temp 35; sleep 1
A1=$(al "$TEMP" a.id); check "an alarm is raised" "$(al "$TEMP" a.isa_state)" "unack_active"

echo "== off by default"
req GET /api/v1/settings/policy; check "the new switches are off, 15 minutes" "$(jx '[j.four_eyes_alarms, j.four_eyes_attributes, j.four_eyes_cameras, j.four_eyes_displays, j.four_eyes_minutes].join(" ")')" "false false false false 15"
req GET /api/v1/auth/me; check "the pages are told so" "$(jx '[j.four_eyes.commands, j.four_eyes.alarms, j.four_eyes.cameras, j.four_eyes.displays, j.four_eyes_minutes].join(" ")')" "false false false false 15"
req POST "/api/v1/video/cameras/$NOCAM/ptz" '{"action":"stop"}'; check "a camera move is not held back" "$(jx 'j.code || "no"')" "no"
req POST /api/v1/second-person/grants '{"area":"cameras","reason":"x"}'; check "no approval to ask for while the area is off" "$(st)" 409
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"run","value":true},"reason":"plain"}'; check "a command runs at once" "$(st)" 201

echo "== a control marked: needs a second person"
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"run","value":true},"reason":"start the motor","second_person":true}'
check "the command is held" "$(st) $(jx 'j.status + " " + j.second_person.kind')" "202 pending command"; C1=$(jx j.id)
req GET /api/v1/second-person/waiting; check "it waits in the list; who asked cannot approve" "$(jx "(w => w ? w.kind + ' ' + w.own + ' ' + w.can_approve + ' ' + w.requested_by : 'none')(j.waiting.find(w => w.id === '$C1'))")" "command true false $USER"
req2 GET /api/v1/second-person/waiting; check "the second user may approve it" "$(jx "(w => w ? w.can_approve + ' ' + w.reason : 'none')(j.waiting.find(w => w.id === '$C1'))")" "true start the motor"
spot() { req POST /api/v1/second-person/approve "{\"kind\":\"$1\",\"id\":\"$2\",\"device_id\":\"${3:-}\",\"username\":\"${4:-$SECOND}\",\"password\":\"${5:-$NEWPW}\",\"reason\":\"${6:-}\"}"; }
spot command "$C1" "$DEV" "$SECOND" "wrong-password"; check "a wrong password of the second person" "$(st) $(jx j.code)" "403 second_person_failed"
spot command "$C1" "$DEV" "$USER" "$PASS"; check "one cannot be one's own second person" "$(st)" 403
spot delete "$C1" "$DEV"; check "only approvals can be run this way" "$(st)" 400
spot command "$C1" "$DEV"; check "approved on the spot" "$(st) $(jx 'j.approved_by + " " + (["queued","sent","acked"].includes(j.result.status))')" "200 $SECOND true"
req GET "/api/v1/devices/$DEV/commands"; check "the command carries both names" "$(jx "(c => c.issued_by + ' ' + c.approved_by + ' ' + (c.status !== 'pending'))(j.find(c => c.id === '$C1'))")" "$USER $SECOND true"
req GET "/api/v1/audit?limit=30"; check "the audit trail says it was on the spot, and where" "$(jx "j.entries.some(e => e.action === 'command.approve' && e.actor === '$SECOND' && e.reason.includes('on the spot, at the station of $USER'))")" "true"
spot command "$C1" "$DEV"; check "a second approval finds nothing waiting" "$(st)" 409

echo "== alarms"
req2 PUT /api/v1/settings/policy '{"four_eyes_alarms":true,"reason":"x"}'; check "an engineer cannot switch it on" "$(st)" 403
req PUT /api/v1/settings/policy '{"four_eyes_alarms":true}'; check "a reason is needed" "$(st)" 400
req PUT /api/v1/settings/policy '{"four_eyes_alarms":true,"reason":"e2e alarms"}'; check "switched on" "$(st) $(jx j.four_eyes_alarms)" "200 true"
req POST "/api/v1/alarms/$A1/ack" '{"reason":"seen"}'; check "the acknowledgement is held" "$(st) $(jx 'j.status + " " + j.second_person.kind')" "202 pending change"; CR1=$(jx j.second_person.id)
check "the summary names the alarm" "$(jx 'j.second_person.summary.startsWith("acknowledge alarm high")')" "true"
check "the alarm is still unacknowledged" "$(al "$TEMP" a.isa_state)" "unack_active"
req POST "/api/v1/change-requests/$CR1/approve" '{"reason":"me again"}'; check "who asked cannot approve from the list" "$(st)" 403
req2 POST "/api/v1/change-requests/$CR1/approve" '{"reason":"confirmed"}'; check "the second user approves from the list" "$(st) $(jx j.status)" "200 approved"
check "the alarm is acknowledged" "$(al "$TEMP" a.isa_state)" "ack_active"
req POST "/api/v1/alarms/$A1/shelve" '{"minutes":30,"reason":"sensor being replaced"}'; check "the shelf is held" "$(st) $(jx j.status)" "202 pending"; CR2=$(jx j.second_person.id)
check "the summary says for how long" "$(jx 'j.second_person.summary.includes("for 30 minutes")')" "true"
spot change "$CR2"; check "approved on the spot" "$(st) $(jx 'j.result.status')" "200 approved"
check "the alarm is shelved" "$(al "$TEMP" a.isa_state)" "shelved"
req PUT /api/v1/settings/policy '{"four_eyes_alarms":false,"reason":"e2e"}'
req POST "/api/v1/alarms/$A1/unshelve" '{"reason":"sensor replaced"}'; check "with the switch off the shelf ends at once" "$(st)" 200

echo "== device attributes"
req PUT /api/v1/settings/policy '{"four_eyes_attributes":true,"reason":"e2e attributes"}'; check "switched on" "$(st)" 200
req PUT "/api/v1/devices/$DEV/attributes" '{"attributes":{"interval":30},"reason":"slower"}'; check "the change is held" "$(st) $(jx j.second_person.kind)" "202 change"; CR3=$(jx j.second_person.id)
req2 POST "/api/v1/change-requests/$CR3/reject" '{"reason":"not now"}'; check "rejected" "$(st) $(jx j.status)" "200 rejected"
req GET "/api/v1/devices/$DEV/attributes"; check "nothing changed" "$(jx 'j.shared.interval === undefined')" "true"
req PUT "/api/v1/devices/$DEV/attributes" '{"attributes":{"interval":60},"reason":"slower"}'; CR4=$(jx j.second_person.id)
spot change "$CR4" "" "$SECOND" "$NEWPW" "agreed with the shift leader"; check "approved on the spot" "$(st) $(jx j.result.status)" "200 approved"
req GET "/api/v1/devices/$DEV/attributes"; check "the attribute is set" "$(jx j.shared.interval)" "60"
req PUT /api/v1/settings/policy '{"four_eyes_attributes":false,"reason":"e2e"}'

echo "== cameras: an approval for a limited time on one station"
req PUT /api/v1/settings/policy '{"four_eyes_minutes":0,"reason":"e2e"}'; check "0 minutes refused" "$(st)" 400
req PUT /api/v1/settings/policy '{"four_eyes_minutes":481,"reason":"e2e"}'; check "more than 8 hours refused" "$(st)" 400
req PUT /api/v1/settings/policy '{"four_eyes_cameras":true,"four_eyes_minutes":2,"reason":"e2e cameras"}'; check "switched on, 2 minutes" "$(st) $(jx 'j.four_eyes_cameras + " " + j.four_eyes_minutes')" "200 true 2"
ptz() { "$1" POST "/api/v1/video/cameras/$NOCAM/ptz" '{"action":"stop"}'; echo "$(st) $(jx 'j.code || "passed"')"; }
check "a camera move is refused" "$(ptz req) $(jx j.area) $(jx j.minutes)" "403 second_person_required cameras 2"
req GET "/api/v1/video/cameras/$NOCAM/playback?from=2026-01-01T00:00:00Z"; check "so is a recording" "$(st) $(jx j.code)" "403 second_person_required"
req GET "/api/v1/video/cameras/$NOCAM/export?from=2026-01-01T00:00:00Z&to=2026-01-01T00:01:00Z"; check "and an export" "$(st) $(jx j.code)" "403 second_person_required"
req GET "/api/v1/video/cameras"; check "watching live pictures is not held back" "$(st)" 200
req POST /api/v1/second-person/grants '{"area":"cameras","reason":""}'; check "asking needs a reason" "$(st)" 400
req POST /api/v1/second-person/grants '{"area":"doors","reason":"x"}'; check "unknown area" "$(st)" 400
req POST /api/v1/second-person/grants '{"area":"cameras","reason":"incident at gate 2"}'; check "asked" "$(st) $(jx 'j.status + " " + j.minutes + " " + j.second_person.kind')" "201 pending 2 grant"; G1=$(jx j.id)
req POST /api/v1/second-person/grants '{"area":"cameras","reason":"again"}'; check "asking again returns the same request" "$(st) $(jx j.id)" "200 $G1"
req POST "/api/v1/second-person/grants/$G1/approve" '{"reason":"me"}'; check "who asked cannot approve" "$(st)" 403
check "still refused while it waits" "$(ptz req)" "403 second_person_required"
req2 GET /api/v1/second-person/waiting; check "the second user sees the request" "$(jx "(w => w ? w.kind + ' ' + w.area + ' ' + w.can_approve : 'none')(j.waiting.find(w => w.id === '$G1'))")" "grant cameras true"
req2 POST "/api/v1/second-person/grants/$G1/approve" '{"reason":"go on"}'; check "approved from the list" "$(st) $(jx 'j.status + " " + j.minutes')" "200 active 2"
check "now the camera can be moved (no such camera here)" "$(ptz req)" "404 passed"
req GET "/api/v1/second-person/grants?mine=1&status=active"; check "the station sees its running approval" "$(jx "j.length + ' ' + j[0].decided_by + ' ' + j[0].this_station + ' ' + j[0].on_spot")" "1 $SECOND true false"
req3 POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; req3 GET /api/v1/auth/me; CSRF3=$(jx j.csrf_token)
check "the same user at another station is still refused" "$(ptz req3)" "403 second_person_required"
check "the second user has no approval of their own" "$(ptz req2)" "403 second_person_required"
req DELETE "/api/v1/second-person/grants/$G1"; check "ended early" "$(st) $(jx j.status)" "200 ended"
check "refused again" "$(ptz req)" "403 second_person_required"
req POST /api/v1/second-person/grants '{"area":"cameras","reason":"incident at gate 2, again"}'; G2=$(jx j.id); check "asked again" "$(st)" 201
spot grant "$G2"; check "approved on the spot" "$(st) $(jx 'j.result.status')" "200 active"
check "the camera can be moved" "$(ptz req)" "404 passed"
req GET "/api/v1/second-person/grants?mine=1&status=active"; check "recorded as given on the spot" "$(jx "j[0].on_spot + ' ' + j[0].decided_by")" "true $SECOND"
req PUT /api/v1/settings/policy '{"four_eyes_cameras":false,"reason":"e2e"}'
check "with the switch off nothing is held back" "$(ptz req3)" "404 passed"

echo "== displays"
req PUT /api/v1/settings/policy '{"four_eyes_displays":true,"reason":"e2e displays"}'; check "switched on" "$(st)" 200
req POST /api/v1/displays/command '{"ids":[],"command":"reload","reason":"x"}'; check "a display command is refused" "$(st) $(jx 'j.code + " " + j.area')" "403 second_person_required displays"
req POST "/api/v1/kiosks/$NOCAM/command" '{"command":"reload","reason":"x"}'; check "so is a command to a kiosk PC" "$(st) $(jx j.code)" "403 second_person_required"
check "the approval for cameras does not cover displays" "$(ptz req)" "404 passed"
req POST /api/v1/second-person/grants '{"area":"displays","reason":"change the wall"}'; G3=$(jx j.id)
req2 POST "/api/v1/second-person/grants/$G3/reject" '{"reason":"not now"}'; check "rejected" "$(st) $(jx j.status)" "200 rejected"
req POST /api/v1/displays/command '{"ids":[],"command":"reload","reason":"x"}'; check "still refused" "$(st)" 403
req POST /api/v1/second-person/grants '{"area":"displays","reason":"change the wall"}'; G4=$(jx j.id); spot grant "$G4"
req POST /api/v1/displays/command '{"ids":[],"command":"reload","reason":"x"}'; check "approved: the command reaches the handler" "$(jx 'j.code || "passed"')" "passed"

echo "== cleanup"
req PUT /api/v1/settings/policy "$ALLOFF"; check "everything off again" "$(st) $(jx '[j.four_eyes_alarms, j.four_eyes_attributes, j.four_eyes_cameras, j.four_eyes_displays].join(" ")')" "200 false false false false"

echo; echo "passed: $pass, failed: $fail"
[[ $fail -eq 0 ]]

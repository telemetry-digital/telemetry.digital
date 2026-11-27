#!/usr/bin/env bash
# End-to-end check of the batch record (spec 043, 0.97.0): units; batches begun and ended by a tag of the controller
# (a run bit, a batch number) with the time of the sample; by a person; one batch per unit at a time; the statistics of
# the curves kept with a finished batch; permissions, reasons and the audit trail. Batches begun by a recipe load are
# checked in tests/e2e/recipes.sh (they need a connector).
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/batches.sh
set -u
cd "$(dirname "$0")/../.."
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""; JAR2="$TMP/jar2"; CSRF2=""
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
go build -o "$TMP/devfill" ./tools/devfill || { echo "devfill build failed"; exit 1; }
SUF=$RANDOM
pub() { "$TMP/devfill" -addr "$MQTT" -user "bat-$SUF" -token "$TOK" -key "$1" -start "$2" -step 0 -n 1 -every 1 > "$TMP/pub.log" 2>&1 || bad "publish $1=$2: $(tail -1 "$TMP/pub.log")"; }
# wait until the batches of a unit answer the expression (the tags are looked at every five seconds)
waitfor() { local unit=$1 expr=$2 want=$3 got=""; for i in $(seq 1 24); do req GET "/api/v1/batches?unit=$unit"; got=$(jx "$expr"); [[ "$got" == "$want" ]] && break; sleep 1; done; echo "$got"; }

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req POST /api/v1/sites "{\"name\":\"Batch site $SUF\",\"reason\":\"e2e\"}"; SITE=$(jx j.id)
req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"machine\",\"name\":\"Reactor $SUF\",\"reason\":\"e2e\"}"; ASSET=$(jx j.id)
ds() { req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"$1\",\"quantity\":\"$2\",\"unit\":\"$3\",\"kind\":\"gauge\",\"reason\":\"e2e\"}"; jx j.id; }
RUN=$(ds run state ""); NUM=$(ds batchno count ""); TEMP=$(ds temp temperature C); check "datastreams" "$(st)" 201
req POST /api/v1/devices "{\"external_id\":\"bat-$SUF\",\"name\":\"Reactor PLC $SUF\",\"reason\":\"e2e\"}"; DEV=$(jx j.id)
req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}'; TOK=$(jx j.token)
for d in $RUN $NUM $TEMP; do req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$d\",\"reason\":\"e2e\"}"; done; check "assigned" "$(st)" 201
VIEWER="batv_$SUF"; NEWPW="Batch-Viewer-Passw0rd-$SUF"
req POST /api/v1/users "{\"username\":\"$VIEWER\",\"full_name\":\"Viewer\",\"email\":\"batv$SUF@example.com\",\"roles\":[\"viewer\"],\"reason\":\"e2e\"}"; PW=$(jx j.initial_password)
req2 POST /api/v1/auth/login "{\"username\":\"$VIEWER\",\"password\":\"$PW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
req2 POST /api/v1/auth/password "{\"current\":\"$PW\",\"new\":\"$NEWPW\"}"
req2 POST /api/v1/auth/login "{\"username\":\"$VIEWER\",\"password\":\"$NEWPW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token); check "a viewer" "$(st)" 200
pub run 0; pub batchno 0

echo "== units"
unit() { req POST /api/v1/batch-units "$1"; }
unit "{\"name\":\"R1 $SUF\",\"mode\":\"tag\",\"tag_datastream\":\"$RUN\",\"curves\":[\"$TEMP\"]}"; check "a unit needs a reason" "$(st)" 400
unit "{\"name\":\"R1 $SUF\",\"mode\":\"valve\",\"reason\":\"e2e\"}"; check "the way batches begin must be recipe or tag" "$(st)" 400
unit "{\"name\":\"R1 $SUF\",\"mode\":\"tag\",\"reason\":\"e2e\"}"; check "a tag unit needs its tag" "$(st)" 400
unit "{\"name\":\"R1 $SUF\",\"mode\":\"recipe\",\"reason\":\"e2e\"}"; check "a recipe unit needs its device" "$(st)" 400
unit "{\"name\":\"R1 $SUF\",\"mode\":\"tag\",\"tag_datastream\":\"$RUN\",\"tag_kind\":\"pulse\",\"reason\":\"e2e\"}"; check "the kind of the tag must be run or number" "$(st)" 400
unit "{\"name\":\"R1 $SUF\",\"mode\":\"tag\",\"tag_datastream\":\"$RUN\",\"curves\":[\"00000000-0000-7000-8000-000000000000\"],\"reason\":\"e2e\"}"; check "a curve must exist" "$(st)" 400
req2 POST /api/v1/batch-units "{\"name\":\"X $SUF\",\"mode\":\"tag\",\"tag_datastream\":\"$RUN\",\"reason\":\"e2e\"}"; check "a viewer cannot set units up" "$(st)" 403
unit "{\"name\":\"R1 $SUF\",\"mode\":\"tag\",\"tag_datastream\":\"$RUN\",\"tag_kind\":\"run\",\"curves\":[\"$TEMP\"],\"reason\":\"e2e\"}"; check "a unit that follows a run bit" "$(st) $(jx 'j.mode + " " + j.tag_kind + " " + j.curves.length')" "201 tag run 1"; U1=$(jx j.id)
unit "{\"name\":\"R1 $SUF\",\"mode\":\"tag\",\"tag_datastream\":\"$RUN\",\"reason\":\"e2e\"}"; check "the name is unique" "$(st)" 409
unit "{\"name\":\"R2 $SUF\",\"mode\":\"tag\",\"tag_datastream\":\"$NUM\",\"tag_kind\":\"number\",\"curves\":[\"$TEMP\"],\"reason\":\"e2e\"}"; check "a unit that follows a batch number" "$(st)" 201; U2=$(jx j.id)
unit "{\"name\":\"Hand $SUF\",\"mode\":\"recipe\",\"device_id\":\"$DEV\",\"prefix\":\"m1_\",\"curves\":[\"$TEMP\"],\"reason\":\"e2e\"}"; check "a unit that follows recipe loads" "$(st) $(jx 'j.mode + " " + j.prefix')" "201 recipe m1_"; U3=$(jx j.id)
req2 GET /api/v1/batch-units; check "everybody reads the units" "$(st) $(jx "j.filter(u => ['$U1','$U2','$U3'].includes(u.id)).length")" "200 3"
req GET "/api/v1/audit?limit=10"; check "setting a unit up is in the audit trail" "$(jx "String(j.entries.filter(e => e.action === 'batch_unit.create').length >= 3)")" "true"
echo "   waiting for the first look at the tags"; sleep 8

echo "== a run bit begins and ends a batch"
pub temp 20; sleep 2; pub run 1
check "the bit going to 1 begins batch 1" "$(waitfor "$U1" '(b => b ? b.number + " " + b.status + " " + b.start_source : "none")(j[0])' "1 running tag")" "1 running tag"
B1=$(jx 'j[0].id'); T0=$(jx 'j[0].started_at')
pub temp 30; sleep 1; pub temp 40; pub run 1
sleep 6; req GET "/api/v1/batches?unit=$U1"; check "the same value again begins nothing" "$(jx j.length)" 1
req GET "/api/v1/batches/$B1"; check "a running batch shows the statistics of its curve so far" "$(jx "(s => s ? [s.n >= 2, s.max].join(' ') : 'none')(j.stats['$TEMP'])")" "true 40"
pub run 0
check "the bit going to 0 ends it" "$(waitfor "$U1" 'j[0].status + " " + j[0].end_source' "finished tag")" "finished tag"
req GET "/api/v1/batches/$B1"; check "the finished batch keeps its curves and their statistics" "$(jx "[j.curves.length, j.stats['$TEMP'].min, j.stats['$TEMP'].max, j.stats['$TEMP'].first, j.stats['$TEMP'].last, j.seconds >= 1, j.started_by].join(' ')")" "1 30 40 30 40 true "
req PUT "/api/v1/batch-units/$U1" "{\"name\":\"R1 $SUF\",\"mode\":\"tag\",\"tag_datastream\":\"$RUN\",\"tag_kind\":\"run\",\"curves\":[],\"reason\":\"no curves any more\"}"; check "the unit is changed" "$(st)" 200
req GET "/api/v1/batches/$B1"; check "the record of the finished batch stays as it was" "$(jx "j.curves.length + ' ' + j.stats['$TEMP'].max")" "1 40"

echo "== a batch number"
pub batchno 101
check "a number begins a batch that carries it" "$(waitfor "$U2" '(b => b ? b.number + " " + b.code + " " + b.status : "none")(j[0])' "1 101 running")" "1 101 running"
pub batchno 102
check "the next number ends it and begins the next batch" "$(waitfor "$U2" 'j.map(b => b.code + ":" + b.status).join(" ")' "102:running 101:finished")" "102:running 101:finished"
pub batchno 0
check "zero ends the batch and begins none" "$(waitfor "$U2" 'j.map(b => b.code + ":" + b.status).join(" ")' "102:finished 101:finished")" "102:finished 101:finished"
req GET "/api/v1/batches?unit=$U2&q=101"; check "batches are found by their code" "$(jx 'j.length + " " + j[0].code')" "1 101"
req GET "/api/v1/batches?unit=$U2&status=running"; check "and by their state" "$(jx j.length)" 0

echo "== by hand"
req2 POST /api/v1/batches "{\"unit_id\":\"$U3\",\"code\":\"X\",\"reason\":\"e2e\"}"; check "a viewer cannot begin a batch" "$(st)" 403
req POST /api/v1/batches "{\"unit_id\":\"$U3\",\"code\":\"LOT-7\"}"; check "beginning needs a reason" "$(st)" 400
req POST /api/v1/batches "{\"unit_id\":\"00000000-0000-7000-8000-000000000000\",\"reason\":\"e2e\"}"; check "an unknown unit" "$(st)" 404
req POST /api/v1/batches "{\"unit_id\":\"$U3\",\"code\":\"LOT-7\",\"reason\":\"trial run\"}"; check "a person begins a batch" "$(st) $(jx 'j.number + " " + j.code + " " + j.status')" "201 1 LOT-7 running"; H1=$(jx j.id)
req POST /api/v1/batches "{\"unit_id\":\"$U3\",\"code\":\"LOT-8\",\"reason\":\"next lot\"}"; check "the next one" "$(st) $(jx j.number)" "201 2"; H2=$(jx j.id)
req GET "/api/v1/batches/$H1"; check "ends the one that ran: one batch per unit at a time" "$(jx '[j.status, j.end_source, j.started_by, j.reason, j.next.length > 30].join(" ")')" "finished manual $USER trial run true"
req2 POST "/api/v1/batches/$H2/end" '{"reason":"e2e"}'; check "a viewer cannot end a batch" "$(st)" 403
req POST "/api/v1/batches/$H2/end" '{}'; check "ending needs a reason" "$(st)" 400
req POST "/api/v1/batches/$H2/end" '{"reason":"lot complete"}'; check "ended by hand" "$(st) $(jx j.status)" "200 finished"
req POST "/api/v1/batches/$H2/end" '{"reason":"again"}'; check "a batch ends once" "$(st)" 409
req GET "/api/v1/batches/$H2"; check "who ended it and why" "$(jx '[j.ended_by, j.end_reason, j.previous.length > 30].join(" ")')" "$USER lot complete true"
req GET "/api/v1/audit?limit=10"; check "beginning and ending by hand are in the audit trail" "$(jx "['batch.start','batch.end'].map(a => j.entries.some(e => e.action === a)).join(' ')")" "true true"
req GET "/api/v1/batches?limit=500"; check "the list of all batches: newest first" "$(jx "String(j.length >= 5 && j.every((b, i) => !i || b.started_at <= j[i-1].started_at))")" "true"
req2 GET "/api/v1/batches/$H2"; check "a viewer reads a batch" "$(st)" 200

echo "== removing a unit"
req POST /api/v1/batches "{\"unit_id\":\"$U3\",\"reason\":\"left running\"}"; H3=$(jx j.id)
req DELETE "/api/v1/batch-units/$U3"; check "removing needs a reason" "$(st)" 400
req DELETE "/api/v1/batch-units/$U3?reason=e2e"; check "removed" "$(st)" 200
req GET "/api/v1/batches/$H3"; check "its running batch is ended, the batches stay" "$(st) $(jx j.status)" "200 finished"
req GET /api/v1/batch-units; check "the unit is gone from the list" "$(jx "j.filter(u => u.id === '$U3').length")" 0
req DELETE "/api/v1/batch-units/$U1?reason=e2e"; req DELETE "/api/v1/batch-units/$U2?reason=e2e"; check "cleanup" "$(st)" 200
req GET /batches; check "the page is served" "$(st)" 200

echo
echo "passed: $pass, failed: $fail"
[[ $fail -eq 0 ]]

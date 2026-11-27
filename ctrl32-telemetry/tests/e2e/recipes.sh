#!/usr/bin/env bash
# End-to-end check of recipes (spec 038, 0.92.0) against the bundled S7 simulator: a recipe with two units behind one
# controller (key prefixes), data records, the check before a load, the load with read back, the values in the unit,
# a load that the connector refuses (nothing written), a load the controller refuses in the middle (the rest is not
# written), four eyes, permissions, CSV and the load log.
# Needs Go (builds tools/s7sim) and Node (JSON extraction) — a developer machine, not the demo server.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/recipes.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
PORT=${SIM_PORT:-10122}
command -v go >/dev/null 2>&1 || export PATH="/d/tools/go/bin:$PATH"
TMP=$(mktemp -d); trap 'rm -rf "$TMP"; [[ -n "${SIM:-}" ]] && kill $SIM 2>/dev/null; true' EXIT
JAR="$TMP/jar"; CSRF=""; JAR2="$TMP/jar2"; CSRF2=""
pass=0; fail=0
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jget() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
call() { local jar=$1 csrf=$2 m=$3 p=$4 d=${5:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$jar" -c "$jar" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $csrf" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$jar" -c "$jar" -X "$m" -H "X-CSRF-Token: $csrf" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
req()  { call "$JAR" "$CSRF" "$@"; }
req2() { call "$JAR2" "$CSRF2" "$@"; }
SUF=$RANDOM; EXT="s7-recipe-$SUF"

echo "== simulator and users"
EXE=bin/s7sim; [[ "$(uname -s)" == MINGW* || "$(uname -s)" == MSYS* ]] && EXE=bin/s7sim.exe
go build -o "$EXE" ./tools/s7sim && ok "simulator built" || { bad "simulator build"; exit 1; }
"$EXE" -port "$PORT" > "$TMP/sim.log" 2>&1 & SIM=$!
for i in $(seq 1 20); do grep -q "listening" "$TMP/sim.log" && break; sleep 0.5; done
grep -q "listening" "$TMP/sim.log" && ok "simulator listening on $PORT" || { bad "simulator: $(cat "$TMP/sim.log")"; exit 1; }
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jget csrf_token < "$TMP/b"); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"
req POST /api/v1/users "{\"username\":\"rop_$SUF\",\"full_name\":\"Recipe Operator\",\"email\":\"rop$SUF@example.com\",\"roles\":[\"operator\"],\"reason\":\"e2e\"}"; check "operator created" "$(st)" 201
PW=$(jget initial_password < "$TMP/b"); NEWPW="Recipe-Operator-Passw0rd-$SUF"
req2 POST /api/v1/auth/login "{\"username\":\"rop_$SUF\",\"password\":\"$PW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jget csrf_token < "$TMP/b")
req2 POST /api/v1/auth/password "{\"current\":\"$PW\",\"new\":\"$NEWPW\"}"
req2 POST /api/v1/auth/login "{\"username\":\"rop_$SUF\",\"password\":\"$NEWPW\"}"; check "operator login" "$(st)" 200
req2 GET /api/v1/auth/me; CSRF2=$(jget csrf_token < "$TMP/b")
[[ "$(jx 'j.permissions.includes("device.command") && !j.permissions.includes("recipe.edit")')" == "true" ]] && ok "operator loads but does not edit recipe data" || bad "operator permissions: $(jx j.permissions)"
req GET /api/v1/auth/me; [[ "$(jx 'j.permissions.includes("recipe.edit") || j.is_admin === true')" == "true" ]] && ok "administrator has recipe.edit" || bad "admin permissions"

echo "== connector: two mixers behind one controller"
unit() { local p=$1 db=$2
  printf '{"key":"%sspeed","address":"DB%s.DBW0","type":"int","writable":true,"min":0,"max":1500},{"key":"%stemp","address":"DB%s.DBD2","writable":true},{"key":"%sheat","address":"DB%s.DBX6.0","writable":true},{"key":"%sname","address":"DB%s.DBS8.10","writable":true},{"key":"%slevel","address":"DB%s.DBW30","type":"int"},{"key":"%sgone","address":"DB99.DBW0","type":"int","writable":true}' "$p" "$db" "$p" "$db" "$p" "$db" "$p" "$db" "$p" "$db" "$p"; }
TAGS="[$(unit m1_ 2),$(unit m2_ 3)]"
req POST /api/v1/connectors "{\"kind\":\"s7\",\"name\":\"recipe-plc-$SUF\",\"settings\":{\"host\":\"127.0.0.1\",\"port\":$PORT,\"plc\":\"s7-1200\",\"interval_s\":1,\"device_external_id\":\"$EXT\",\"tags\":$TAGS},\"reason\":\"e2e\"}"; check "connector" "$(st)" 201
CID=$(jget id < "$TMP/b")
for i in $(seq 1 20); do sleep 1; req GET /api/v1/devices; DEV=$(jx "(j.find(d=>d.external_id==='$EXT')||{}).id"); [[ "$DEV" != "null" && -n "$DEV" ]] && break; done
[[ "$DEV" != "null" && -n "$DEV" ]] && ok "device of the connector" || { bad "device"; exit 1; }
req POST /api/v1/devices "{\"external_id\":\"mqtt-$SUF\",\"name\":\"mqtt $SUF\",\"reason\":\"e2e\"}"; MQ=$(jget id < "$TMP/b")

echo "== recipe"
EL='[{"key":"speed","label":"Speed","unit":"rpm","min":0,"max":3000},{"key":"temp","label":"Temperature","unit":"°C","decimals":1,"min":10,"max":90},{"key":"heat","label":"Heating","type":"bool"},{"key":"name","label":"Product","type":"text"}]'
TG="[{\"name\":\"Mixer 1\",\"device_id\":\"$DEV\",\"prefix\":\"m1_\"},{\"name\":\"Mixer 2\",\"device_id\":\"$DEV\",\"prefix\":\"m2_\"}]"
req POST /api/v1/recipes "{\"name\":\"Mixing $SUF\",\"elements\":$EL,\"targets\":$TG}"; check "reason required" "$(st)" 400
req POST /api/v1/recipes "{\"name\":\"Mixing $SUF\",\"elements\":[],\"targets\":$TG,\"reason\":\"e2e\"}"; check "elements required" "$(st)" 400
req POST /api/v1/recipes "{\"name\":\"Mixing $SUF\",\"elements\":$EL,\"targets\":[{\"name\":\"x\",\"device_id\":\"0198a000-0000-7000-8000-000000000001\"}],\"reason\":\"e2e\"}"; check "unknown device refused" "$(st)" 400
if [[ -n "$MQ" ]]; then req POST /api/v1/recipes "{\"name\":\"Mixing $SUF\",\"elements\":$EL,\"targets\":[{\"name\":\"x\",\"device_id\":\"$MQ\"}],\"reason\":\"e2e\"}"; check "a device that is not behind a connector is refused" "$(st)" 400; else bad "mqtt device"; fi
req2 POST /api/v1/recipes "{\"name\":\"Mixing $SUF\",\"elements\":$EL,\"targets\":$TG,\"reason\":\"e2e\"}"; check "operator cannot create a recipe" "$(st)" 403
req POST /api/v1/recipes "{\"name\":\"Mixing $SUF\",\"description\":\"e2e\",\"elements\":$EL,\"targets\":$TG,\"reason\":\"e2e\"}"; check "created" "$(st)" 201
RC=$(jget id < "$TMP/b")
req POST /api/v1/recipes "{\"name\":\"mixing $SUF\",\"elements\":$EL,\"targets\":$TG,\"reason\":\"e2e\"}"; check "same name refused" "$(st)" 409
req GET "/api/v1/recipes/$RC"; check "read" "$(st)" 200
check "units with ids and the device" "$(jx 'j.targets.map(t => t.id + ":" + t.prefix + ":" + t.transport + ":" + t.available).join(" ")')" "u1:m1_:s7:true u2:m2_:s7:true"
check "defaults (type number)" "$(jx 'j.elements[0].type + " " + j.elements[2].type')" "number bool"

echo "== records"
V1='{"speed":1200,"temp":45.26,"heat":true,"name":"cream"}'
req2 POST "/api/v1/recipes/$RC/records" "{\"name\":\"Cream\",\"values\":$V1,\"reason\":\"e2e\"}"; check "operator cannot create a record" "$(st)" 403
req POST "/api/v1/recipes/$RC/records" "{\"name\":\"Cream\",\"values\":{\"speed\":1200},\"reason\":\"e2e\"}"; check "a value for every element" "$(st)" 400
req POST "/api/v1/recipes/$RC/records" "{\"name\":\"Cream\",\"values\":{\"speed\":1200,\"temp\":95,\"heat\":true,\"name\":\"x\"},\"reason\":\"e2e\"}"; check "limit of the element" "$(st)" 400
grep -q "above the maximum 90" "$TMP/b" && ok "the message names the limit" || bad "message: $(cat "$TMP/b")"
req POST "/api/v1/recipes/$RC/records" "{\"name\":\"Cream\",\"values\":$V1,\"note\":\"first\",\"reason\":\"e2e\"}"; check "record created" "$(st):$(jx j.number)" "201:1"
R1=$(jget id < "$TMP/b")
req POST "/api/v1/recipes/$RC/records" "{\"number\":1,\"name\":\"Other\",\"values\":$V1,\"reason\":\"e2e\"}"; check "number used" "$(st)" 409
req POST "/api/v1/recipes/$RC/records" "{\"name\":\"Too fast\",\"values\":{\"speed\":2000,\"temp\":20,\"heat\":false,\"name\":\"fast\"},\"reason\":\"e2e\"}"; check "second record" "$(st):$(jx j.number)" "201:2"
R2=$(jget id < "$TMP/b")
req GET "/api/v1/recipes/$RC"; check "values rounded to the decimals" "$(jx 'j.records[0].values.temp + " " + j.records[0].complete + " " + j.records.length')" "45.3 true 2"

echo "== check and load"
req GET "/api/v1/recipes/$RC/compare?target=u1&record=$R1"; check "values in the unit before the load" "$(st):$(jx 'j.differ > 0')" "200:true"
req POST /api/v1/batch-units "{\"name\":\"Mixer 1 $SUF\",\"mode\":\"recipe\",\"device_id\":\"$DEV\",\"prefix\":\"m1_\",\"reason\":\"e2e\"}"; BU=$(jget id < "$TMP/b"); check "a batch unit that follows the loads into Mixer 1 (spec 043)" "$(st)" 201
req POST "/api/v1/recipes/$RC/records/$R1/download" '{"target":"u1","dry_run":true}'; check "check only" "$(st):$(jx 'j.ok + " " + j.problems.length + " " + j.four_eyes')" "200:true 0 false"
req POST "/api/v1/recipes/$RC/records/$R1/download" '{"target":"u1"}'; check "a load needs a reason" "$(st)" 400
req POST "/api/v1/recipes/$RC/records/$R1/download" '{"target":"u9","reason":"e2e"}'; check "unknown unit" "$(st)" 400
req POST "/api/v1/recipes/$RC/records/$R1/download" '{"target":"u1","reason":"batch 1"}'; check "loaded" "$(st):$(jx j.status)" "201:done"
check "every value written" "$(jx 'j.results.map(r => r.key + "=" + r.status).join(" ")')" "speed=written temp=written heat=written name=written"
check "read back from the controller" "$(jx 'j.results[0].readback + " " + j.results[2].readback + " " + j.results[3].readback + " " + (Math.abs(j.results[1].readback - 45.3) < 0.001)')" "1200 true cream true"
for i in $(seq 1 10); do sleep 1; req GET "/api/v1/recipes/$RC/compare?target=u1&record=$R1"; [[ "$(jx 'j.differ + j.missing')" == "0" ]] && break; done
check "the unit holds the record" "$(jx 'j.differ + " " + j.missing + " " + j.items.every(i => i.same)')" "0 0 true"
req GET "/api/v1/recipes/$RC/compare?target=u2&record=$R1"; check "the other unit was not touched" "$(jx 'j.differ > 0')" "true"
req GET "/api/v1/devices/$DEV/commands?limit=10"; check "every write is in the command log with the recipe" "$(jx 'j.filter(c => /^recipe Mixing .*, record 1 Cream: batch 1$/.test(c.reason) && c.status === "acked").length')" 4
req GET "/api/v1/recipe-downloads?recipe=$RC"; check "load log" "$(jx 'j.length + " " + j[0].status + " " + j[0].record + " " + j[0].unit + " " + j[0].requested_by + " " + j[0].items.length + " " + j[0].reason')" "1 done Cream Mixer 1 $USER 4 batch 1"
req2 POST "/api/v1/recipes/$RC/records/$R1/download" '{"target":"u2","reason":"operator loads"}'; check "operator loads a record" "$(st):$(jx j.status)" "201:done"

echo "== batch record: a load begins a batch on the unit that follows it"
req GET "/api/v1/batches?unit=$BU"; check "the load into Mixer 1 began batch 1; the check and the load into Mixer 2 did not" "$(jx 'j.length + " " + j[0].number + " " + j[0].status + " " + j[0].start_source + " " + j[0].record + " " + (j[0].recipe_download_id.length > 30)')" "1 1 running recipe $(jx 'j[0].record') true"
B1=$(jx 'j[0].id')
req POST "/api/v1/recipes/$RC/records/$R1/download" '{"target":"u1","reason":"batch 2"}'; check "the next load" "$(st):$(jx j.status)" "201:done"
req GET "/api/v1/batches?unit=$BU"; check "ends batch 1 and begins batch 2" "$(jx 'j.map(b => b.number + ":" + b.status + ":" + b.end_source).join(" ")')" "2:running: 1:finished:recipe"
req GET "/api/v1/batches/$B1"; check "the finished batch names its recipe and who loaded it" "$(jx '[j.recipe.length > 3, j.started_by, j.end_reason, j.next.length > 30].join(" ")')" "true $USER the next batch began true"

echo "== a value the connector refuses: nothing is written"
req POST "/api/v1/recipes/$RC/records/$R2/download" '{"target":"u1","dry_run":true}'; check "check names the problem" "$(jx 'j.ok + " " + j.problems.length + " " + j.problems[0].key')" "false 1 speed"
req POST "/api/v1/recipes/$RC/records/$R2/download" '{"target":"u1","reason":"too fast"}'; check "load refused" "$(st)" 409
grep -q "nothing was written" "$TMP/b" && ok "the message says nothing was written" || bad "message: $(cat "$TMP/b")"
sleep 2; req GET "/api/v1/recipes/$RC/compare?target=u1&record=$R1"; check "the unit still holds the first record" "$(jx 'j.differ')" 0
req GET "/api/v1/recipe-downloads?recipe=$RC"; check "a refused load is not in the log" "$(jx 'j.length')" 3

echo "== a value the controller refuses in the middle: the rest is not written"
EL2='[{"key":"speed","label":"Speed"},{"key":"gone","label":"Missing block"},{"key":"temp","label":"Temperature","decimals":1}]'
req POST /api/v1/recipes "{\"name\":\"Broken $SUF\",\"elements\":$EL2,\"targets\":[{\"name\":\"Mixer 1\",\"device_id\":\"$DEV\",\"prefix\":\"m1_\"}],\"reason\":\"e2e\"}"; RB=$(jget id < "$TMP/b"); check "recipe with a key of a missing block" "$(st)" 201
req POST "/api/v1/recipes/$RB/records" '{"name":"B","values":{"speed":700,"gone":1,"temp":33},"reason":"e2e"}'; RBR=$(jget id < "$TMP/b")
req POST "/api/v1/recipes/$RB/records/$RBR/download" '{"target":"u1","reason":"will fail"}'; check "load failed" "$(st):$(jx j.status)" "201:failed"
check "written, failed, not written" "$(jx 'j.results.map(r => r.status).join(" ")')" "written failed not_written"
[[ "$(jx 'j.error.startsWith("Missing block: ")')" == "true" ]] && ok "the error names the element" || bad "error: $(jx j.error)"
sleep 2; req GET "/api/v1/recipes/$RB/compare?target=u1&record=$RBR"; check "the unit: first value written, last one not" "$(jx 'j.items[0].same + " " + j.items[2].same')" "true false"

req GET "/api/v1/batches?unit=$BU"; check "a load that failed begins no batch" "$(jx 'j.length')" 2
echo "== a record that does not fit the changed recipe"
EL3='[{"key":"speed","label":"Speed","unit":"rpm","min":0,"max":1000},{"key":"temp","label":"Temperature","unit":"°C","decimals":1,"min":10,"max":90},{"key":"heat","label":"Heating","type":"bool"},{"key":"name","label":"Product","type":"text"}]'
req PUT "/api/v1/recipes/$RC" "{\"name\":\"Mixing $SUF\",\"elements\":$EL3,\"targets\":[{\"id\":\"u1\",\"name\":\"Mixer 1\",\"device_id\":\"$DEV\",\"prefix\":\"m1_\"},{\"id\":\"u2\",\"name\":\"Mixer 2\",\"device_id\":\"$DEV\",\"prefix\":\"m2_\"}],\"reason\":\"lower limit\"}"; check "recipe changed" "$(st)" 200
req GET "/api/v1/recipes/$RC"; check "the record is marked" "$(jx 'j.records[0].complete + " " + /above the maximum 1000/.test(j.records[0].problem)')" "false true"
req POST "/api/v1/recipes/$RC/records/$R1/download" '{"target":"u1","reason":"x"}'; check "and cannot be loaded" "$(st)" 409
req PUT "/api/v1/recipes/$RC/records/$R1" '{"name":"Cream","values":{"speed":900,"temp":45.3,"heat":true,"name":"cream"},"reason":"fits again"}'; check "record corrected" "$(st)" 200
req GET "/api/v1/recipes/$RC"; check "version grows" "$(jx 'j.records[0].version + " " + j.records[0].complete')" "2 true"

echo "== four eyes"
req PUT /api/v1/settings/policy '{"four_eyes_commands":true,"reason":"e2e on"}'; check "policy on" "$(st)" 200
req POST "/api/v1/recipes/$RC/records/$R1/download" '{"target":"u1","reason":"batch 2"}'; check "the load waits" "$(st):$(jx j.status)" "202:pending"
D1=$(jget id < "$TMP/b")
req PUT "/api/v1/recipes/$RC/records/$R1" '{"name":"Cream","values":{"speed":100,"temp":45.3,"heat":true,"name":"cream"},"reason":"changed while waiting"}'
sleep 2; req GET "/api/v1/recipes/$RC/compare?target=u1"; check "nothing written while waiting" "$(jx 'j.items[0].current')" 700
req POST "/api/v1/recipe-downloads/$D1/approve" '{"reason":"ok"}'; check "the user who asked cannot approve" "$(st)" 403
req2 POST "/api/v1/recipe-downloads/$D1/approve" '{}'; check "approval needs a reason" "$(st)" 400
req2 POST "/api/v1/recipe-downloads/$D1/approve" '{"reason":"checked"}'; check "another user approves" "$(st):$(jx j.status)" "200:done"
check "the values asked for are written, not the changed record" "$(jx 'j.results[0].readback')" 900
req2 POST "/api/v1/recipe-downloads/$D1/approve" '{"reason":"again"}'; check "not twice" "$(st)" 409
req POST "/api/v1/recipes/$RC/records/$R1/download" '{"target":"u1","reason":"batch 3"}'; D2=$(jget id < "$TMP/b")
req2 POST "/api/v1/recipe-downloads/$D2/reject" '{"reason":"wrong unit"}'; check "rejected" "$(st):$(jx j.status)" "200:rejected"
req GET "/api/v1/recipe-downloads?recipe=$RC"; check "log with the decisions" "$(jx 'j[0].status + " " + j[0].decided_by + " " + j[0].decide_reason + " | " + j[1].status + " " + j[1].decided_by')" "rejected rop_$SUF wrong unit | done rop_$SUF"
req PUT /api/v1/settings/policy '{"four_eyes_commands":false,"reason":"e2e off"}'; check "policy off" "$(st)" 200

echo "== CSV"
curl -s -b "$JAR" -o "$TMP/out.csv" -w '%{http_code}' "$BASE/api/v1/recipes/$RC/export" > "$TMP/s"; check "export" "$(st)" 200
check "header with the keys" "$(head -1 "$TMP/out.csv" | sed 's/^\xef\xbb\xbf//' | tr -d '\r')" "number,name,note,speed,temp,heat,name"
node -e 'const fs=require("fs"); fs.writeFileSync(process.argv[1], JSON.stringify({csv: "number;name;note;speed;temp;heat;product\n1;Cream;changed;950;44,5;true;cream\n7;Gel;;300;20;false;gel\n", reason: "e2e import"}))' "$TMP/imp.json"
curl -s -b "$JAR" -X POST -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/imp.json" "$BASE/api/v1/recipes/$RC/import" > "$TMP/s"; check "a column that is not an element is refused" "$(st)" 400
req PUT "/api/v1/recipes/$RC" "{\"name\":\"Mixing $SUF\",\"elements\":$(echo "$EL3" | sed 's/"key":"name"/"key":"product"/'),\"targets\":[{\"id\":\"u1\",\"name\":\"Mixer 1\",\"device_id\":\"$DEV\",\"prefix\":\"m1_\"}],\"reason\":\"key renamed\"}"; check "element renamed" "$(st)" 200
curl -s -b "$JAR" -X POST -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/imp.json" "$BASE/api/v1/recipes/$RC/import" > "$TMP/s"; check "import" "$(st):$(jx 'j.created + " " + j.updated')" "200:1 1"
req GET "/api/v1/recipes/$RC"; check "records after the import" "$(jx 'j.records.map(r => r.number + ":" + r.name + ":" + r.values.speed + ":" + r.values.temp + ":" + r.complete).join(" ")')" "1:Cream:950:44.5:true 2:Too fast:2000:20:false 7:Gel:300:20:true"
req2 POST "/api/v1/recipes/$RC/import" '{"csv":"number,name\n1,a\n","reason":"x"}'; check "operator cannot import" "$(st)" 403

echo "== deleting"
req DELETE "/api/v1/recipes/$RC/records/$R2"; check "reason required" "$(st)" 400
req2 DELETE "/api/v1/recipes/$RC/records/$R2?reason=x"; check "operator cannot delete a record" "$(st)" 403
req DELETE "/api/v1/recipes/$RC/records/$R2?reason=e2e"; check "record deleted" "$(st)" 200
req POST "/api/v1/recipes/$RC/records" '{"number":2,"name":"Again","values":{"speed":1,"temp":20,"heat":false,"product":"x"},"reason":"e2e"}'; check "its number is free again" "$(st)" 201
req DELETE "/api/v1/recipes/$RC?reason=e2e"; check "recipe deleted" "$(st)" 200
req GET "/api/v1/recipes/$RC"; check "gone" "$(st)" 404
req GET "/api/v1/recipe-downloads?recipe=$RC"; check "its loads stay in the log" "$(jx 'j.length >= 4')" true
req GET /api/v1/recipes; check "not listed" "$(jx "j.filter(r => r.id === '$RC').length")" 0
req DELETE "/api/v1/recipes/$RB?reason=e2e"
req GET "/api/v1/audit?limit=500"; check "audit trail" "$(jx '["recipe.create","recipe_record.create","recipe.download","recipe.download_approve","recipe.download_reject","recipe.import","recipe.delete"].every(a => j.entries.some(e => e.action === a))')" true
req DELETE "/api/v1/connectors/$CID?reason=e2e"

echo; echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

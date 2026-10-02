#!/usr/bin/env bash
# End-to-end checks of the Telemetry tab of the device page (0.67.0) and of sensor faults:
#   - every key a device sends: assigned datastreams, keys without a datastream (from the raw messages) and status keys,
#     with the latest value, quality, statistics and a 24-hour sparkline;
#   - the chart of one key (raw and aggregated), the values newest first with paging, CSV and Excel export (audited);
#   - "create datastream and assign" for a key without a datastream (device.manage), refused for a viewer;
#   - live updates of keys without a datastream and of status keys (device_data events of /api/v1/stream);
#   - sensor faults: "fault" flags in the telemetry and values outside the physical range of the datastream are stored
#     with quality sensor_fault, a sensor_fault alarm rule raises and clears, value rules skip faulty values;
#   - annotations of observations (data.annotate): shown with the chart, in the table and in the observation export;
#     retracted by a new record; refused without the permission;
#   - organization scope: another organization's user sees nothing of the device.
# Devices talk over the device HTTP API (POST /api/v1/d/telemetry, /api/v1/d/status). Needs curl and Node.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/device_telemetry.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"; [[ -n "${SSE_PID:-}" ]] && kill "$SSE_PID" 2>/dev/null' EXIT
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
# a device message over the device HTTP API
dev()  { printf '%s' "$2" > "$TMP/dreq"; curl -s -X POST -H "Authorization: Bearer $TOK" -H "Content-Type: application/json" -o "$TMP/db" -w '%{http_code}' --data-binary @"$TMP/dreq" "$BASE/api/v1/d/$1" > "$TMP/ds"; }
# sign in as another user (one-time password changed at once): leaves JAR/CSRF on that user
signin() { # jar user password [new password]
  JAR="$1"; CSRF=""
  req POST /api/v1/auth/login "{\"username\":\"$2\",\"password\":\"$3\"}"
  req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
  if [[ -n "${4:-}" ]]; then req POST /api/v1/auth/password "{\"current\":\"$3\",\"new\":\"$4\"}"; req POST /api/v1/auth/login "{\"username\":\"$2\",\"password\":\"$4\"}"; req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token); fi
}
admin() { JAR="$TMP/jar"; CSRF=$ADMIN_CSRF; }
SUF=$RANDOM
NOW=$(date -u +%s)
iso() { date -u -d "@$1" +%Y-%m-%dT%H:%M:%SZ; }
FROM=$(iso $((NOW - 7200))); TO=$(iso $((NOW + 600)))

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token); ADMIN_CSRF=$CSRF
req POST /api/v1/sites "{\"name\":\"Telemetry site $SUF\",\"reason\":\"e2e\"}"; SITE=$(jx j.id)
req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"fridge\",\"name\":\"Fridge $SUF\",\"reason\":\"e2e\"}"; ASSET=$(jx j.id)
req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"temp\",\"quantity\":\"temperature\",\"unit\":\"°C\",\"kind\":\"gauge\",\"range\":{\"min\":-55,\"max\":125},\"reason\":\"e2e\"}"; check "datastream with a physical range" "$(st) $(jx 'j.range_min+","+j.range_max')" "201 -55,125"
TEMP=$(jx j.id)
req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"bad\",\"quantity\":\"x\",\"range\":{\"min\":5,\"max\":1},\"reason\":\"e2e\"}"; check "range with min above max refused" "$(st)" 400
req POST /api/v1/devices "{\"external_id\":\"tel-$SUF\",\"name\":\"telemetry test\",\"transport\":\"http\",\"reason\":\"e2e\"}"; DEV=$(jx j.id)
req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}'; TOK=$(jx j.token)
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$TEMP\",\"reason\":\"e2e\"}"; check "temp assigned" "$(st)" 201
# alarm rules before the first value (the alarm engine reads the rules of a datastream again after 30 s)
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$TEMP\",\"type\":\"sensor_fault\",\"warning\":1,\"reason\":\"e2e\"}"; check "sensor_fault with a limit refused" "$(st)" 400
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$TEMP\",\"type\":\"sensor_fault\",\"reason\":\"e2e probe check\"}"; check "sensor_fault rule saved" "$(st)" 201
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$TEMP\",\"type\":\"low\",\"action\":0,\"reason\":\"e2e\"}"; check "low rule saved" "$(st)" 201

echo "== the device sends"
for i in 6 5 4 3 2 1; do dev telemetry "{\"ts\":$((NOW - i * 600)),\"v\":{\"temp\":$((i + 2)).5,\"rh\":$((50 + i)),\"co2\":$((700 + i * 10))}}"; done
check "telemetry with unknown keys accepted" "$(cat "$TMP/ds")" 202
dev status '{"bat":3.61,"bat_pct":78,"rssi":-71,"up_s":86400,"fw":"1.4.2"}'; check "status accepted" "$(cat "$TMP/ds")" 202
dev telemetry "{\"ts\":$((NOW - 30)),\"v\":{\"temp\":4.0,\"mode\":\"eco\"}}"

echo "== every key of the device"
req GET "/api/v1/devices/$DEV/telemetry?from=$FROM&to=$TO"; check "summary" "$(st)" 200
check "sources" "$(jx 'j.keys.map(k=>k.source+":"+k.key).filter(x=>!x.startsWith("status:")).join(",")')" "datastream:temp,unassigned:co2,unassigned:mode,unassigned:rh"
check "status keys" "$(jx 'j.keys.filter(k=>k.source==="status").map(k=>k.key).join(",")')" "bat,bat_pct,fw,rssi,up_s"
check "latest temp" "$(jx 'const k=j.keys.find(k=>k.key==="temp"); k.last.value+" "+k.last.quality')" "4 ok"
check "temp stats over the range" "$(jx 'const k=j.keys.find(k=>k.key==="temp"); [k.stats.min,k.stats.max,k.stats.n].join(",")')" "3.5,8.5,7"
check "unassigned rh: latest and stats" "$(jx 'const k=j.keys.find(k=>k.key==="rh"); [k.last.value,k.stats.min,k.stats.max,k.stats.n].join(",")')" "51,51,56,6"
check "unassigned text value" "$(jx 'const k=j.keys.find(k=>k.key==="mode"); k.last.value_json')" 'eco'
check "status value and unit-less" "$(jx 'const k=j.keys.find(k=>k.key==="bat"); k.last.value')" "3.61"
check "status text (firmware)" "$(jx 'const k=j.keys.find(k=>k.key==="fw"); k.last.value_json')" '1.4.2'
check "sparkline of temp" "$(jx 'j.keys.find(k=>k.key==="temp").spark.length > 0')" "true"
check "sparkline of rh" "$(jx 'j.keys.find(k=>k.key==="rh").spark.length > 0')" "true"
req GET "/api/v1/devices/$DEV/telemetry?from=$TO&to=$FROM"; check "to before from refused" "$(st)" 400

echo "== chart of one key"
req GET "/api/v1/devices/$DEV/telemetry/series?key=temp&from=$FROM&to=$TO"; check "temp series" "$(st) $(jx 'j.bucket_s+" "+j.points.length+" "+j.stats.avg.toFixed(2)')" "200 0 7 5.71"
req GET "/api/v1/devices/$DEV/telemetry/series?key=temp&from=$FROM&to=$TO&agg=max&bucket=900"; check "aggregated (max per 15 minutes)" "$(st) $(jx 'j.bucket_s+" "+(j.points.length>=2)+" "+j.points.every(p=>p.v===p.max)')" "200 900 true true"
req GET "/api/v1/devices/$DEV/telemetry/series?key=rh&source=unassigned&from=$FROM&to=$TO"; check "unassigned series" "$(st) $(jx 'j.points.length+" "+j.points[0].v')" "200 6 56"
req GET "/api/v1/devices/$DEV/telemetry/series?key=bat&source=status&from=$FROM&to=$TO"; check "status series" "$(st) $(jx 'j.points.length+" "+j.points[0].v')" "200 1 3.61"
req GET "/api/v1/devices/$DEV/telemetry/series?key=nothere&from=$FROM&to=$TO"; check "unknown datastream key" "$(st)" 404
req GET "/api/v1/devices/$DEV/telemetry/series?key=rh&source=bogus"; check "bad source" "$(st)" 400

echo "== values, paging and exports"
req GET "/api/v1/devices/$DEV/telemetry/values?key=temp&from=$FROM&to=$TO&limit=4"; check "first page" "$(st) $(jx 'j.rows.length+" "+j.rows[0].value+" "+(j.next_cursor!=="")')" "200 4 4 true"
CUR=$(jx j.next_cursor)
req GET "/api/v1/devices/$DEV/telemetry/values?key=temp&from=$FROM&to=$TO&limit=4&cursor=$CUR"; check "second page" "$(st) $(jx 'j.rows.length+" "+j.rows[2].value+" "+j.next_cursor')" "200 3 8.5 "
req GET "/api/v1/devices/$DEV/telemetry/values?key=co2&source=unassigned&from=$FROM&to=$TO&limit=4"; check "unassigned first page" "$(st) $(jx 'j.rows.length+" "+j.rows[0].value')" "200 4 710"
CUR=$(jx j.next_cursor)
req GET "/api/v1/devices/$DEV/telemetry/values?key=co2&source=unassigned&from=$FROM&to=$TO&limit=4&cursor=$CUR"; check "unassigned second page" "$(st) $(jx 'j.rows.map(r=>r.value).join(",")')" "200 750,760"
curl -s -b "$JAR" -o "$TMP/x.csv" -D "$TMP/h" "$BASE/api/v1/devices/$DEV/telemetry/export?key=temp&from=$FROM&to=$TO&format=csv"
check "CSV export" "$(head -1 "$TMP/x.csv" | tr -d '\r')" "measured_at,received_at,value,value_json,unit,quality,time_source,annotation"
check "CSV rows oldest first" "$(sed -n 2p "$TMP/x.csv" | cut -d, -f3)" "8.5"
curl -s -b "$JAR" -o "$TMP/x.xlsx" -D "$TMP/h" "$BASE/api/v1/devices/$DEV/telemetry/export?key=rh&source=unassigned&from=$FROM&to=$TO&format=xlsx"
grep -qi "spreadsheetml" "$TMP/h" && ok "Excel export content type" || bad "xlsx content type: $(grep -i content-type "$TMP/h")"
check "Excel export is a zip" "$(head -c 2 "$TMP/x.xlsx")" "PK"
req GET "/api/v1/export/audit?from=$(iso $((NOW - 60)))&format=jsonl"; grep -q 'device_telemetry' "$TMP/b" && ok "exports audited" || bad "export not in the audit trail"

echo "== sensor faults"
dev telemetry '{"v":{"temp":-127}}'
sleep 1
req GET "/api/v1/observations?datastream_id=$TEMP&from=$(iso $((NOW - 60)))&limit=10"; check "outside the physical range: sensor_fault" "$(jx 'j[j.length-1].value+" "+j[j.length-1].quality')" "-127 sensor_fault"
req GET "/api/v1/alarms?datastream_id=$TEMP"; check "sensor fault alarm raised, no low alarm" "$(jx 'j.filter(a=>!a.cleared_at).length+" "+j[0].severity')" "1 action"
dev telemetry '{"v":{"temp":4.4}}'; sleep 1
req GET "/api/v1/alarms?datastream_id=$TEMP"; check "a good value clears it" "$(jx 'j.filter(a=>!a.cleared_at).length')" "0"
dev telemetry '{"v":{"temp":5.1,"rh":40},"fault":["temp"]}'; sleep 1
req GET "/api/v1/observations?datastream_id=$TEMP&from=$(iso $((NOW - 60)))&limit=10"; check "flagged by the device: sensor_fault" "$(jx 'j[j.length-1].value+" "+j[j.length-1].quality')" "5.1 sensor_fault"
req GET "/api/v1/alarms?datastream_id=$TEMP"; check "raised again" "$(jx 'j.filter(a=>!a.cleared_at).length')" "1"
dev telemetry '{"v":{"temp":5.2},"fault":{"temp":false}}'; sleep 1
req GET "/api/v1/alarms?datastream_id=$TEMP"; check "cleared by a value without the flag" "$(jx 'j.filter(a=>!a.cleared_at).length')" "0"
req GET "/api/v1/devices/$DEV/telemetry?from=$(iso $((NOW - 60)))&to=$TO"; check "faults counted, not averaged" "$(jx 'const k=j.keys.find(k=>k.key==="temp"); [k.stats.faults,k.stats.min,k.stats.max].join(",")')" "2,4,5.2"
req PATCH "/api/v1/datastreams/$TEMP" '{"range":{"min":null,"max":null},"reason":"e2e"}'; check "range removed" "$(st) $(jx 'j.range_min')" "200 null"
dev telemetry '{"v":{"temp":-127}}'; sleep 1
req GET "/api/v1/observations?datastream_id=$TEMP&from=$(iso $((NOW - 60)))&limit=20"; check "without a range: an ordinary value" "$(jx 'j[j.length-1].quality')" "ok"
req PATCH "/api/v1/datastreams/$TEMP" '{"range":{"min":-55,"max":125},"reason":"e2e"}'; check "range set again" "$(st)" 200

echo "== live: keys without a datastream and status"
curl -s -N -b "$JAR" --max-time 6 "$BASE/api/v1/stream?devices=$DEV&device_data=1" > "$TMP/sse" & SSE_PID=$!
sleep 1.5
dev telemetry '{"v":{"rh":61.5,"temp":4.1}}'
dev status '{"bat":3.55,"rssi":-69}'
wait $SSE_PID 2>/dev/null; SSE_PID=""
grep -q '^event: device_data' "$TMP/sse" && ok "device_data events" || bad "no device_data event: $(head -c 300 "$TMP/sse")"
grep -q '"rh":61.5' "$TMP/sse" && ok "unassigned value live" || bad "rh not live"
grep -q '"bat":3.55' "$TMP/sse" && ok "status value live" || bad "status not live"
grep -q '"temp":4.1' "$TMP/sse" && bad "an assigned key came as device_data" || ok "assigned keys come as observations only"

echo "== annotations"
OBS_AT=$(iso $((NOW - 1800)))
req POST /api/v1/annotations "{\"datastream_id\":\"$TEMP\",\"from\":\"$(iso $((NOW - 2400)))\",\"to\":\"$OBS_AT\",\"category\":\"expected_event\",\"text\":\"Door open for loading\"}"; check "annotate a period" "$(st)" 201
ANN=$(jx j.id)
req POST /api/v1/annotations "{\"datastream_id\":\"$TEMP\",\"from\":\"$OBS_AT\",\"text\":\"Probe checked\",\"category\":\"maintenance\"}"; check "annotate one observation" "$(st)" 201
ANN2=$(jx j.id)
req POST /api/v1/annotations "{\"datastream_id\":\"$TEMP\",\"from\":\"$OBS_AT\",\"text\":\"\"}"; check "empty text refused" "$(st)" 400
req POST /api/v1/annotations "{\"datastream_id\":\"$TEMP\",\"from\":\"$OBS_AT\",\"text\":\"x\",\"category\":\"gossip\"}"; check "unknown category refused" "$(st)" 400
req GET "/api/v1/devices/$DEV/telemetry/series?key=temp&from=$FROM&to=$TO"; check "annotations with the chart" "$(jx 'j.annotations.length')" "2"
req GET "/api/v1/devices/$DEV/telemetry/values?key=temp&from=$FROM&to=$TO&limit=100"; check "annotation in the table" "$(jx 'j.rows.find(r=>r.annotation).annotation')" "Door open for loading | Probe checked"
curl -s -b "$JAR" -o "$TMP/o.csv" "$BASE/api/v1/export/observations?datastream_id=$TEMP&from=$FROM&to=$TO"
check "annotation column in the observation export" "$(head -1 "$TMP/o.csv" | tr -d '\r' | awk -F, '{print $NF}')" "annotation"
grep -q "Door open for loading | Probe checked" "$TMP/o.csv" && ok "annotation text exported" || bad "annotation not exported"
req POST "/api/v1/annotations/$ANN2/retract" '{"reason":"wrong probe"}'; check "retract" "$(st)" 200
req POST "/api/v1/annotations/$ANN2/retract" '{"reason":"again"}'; check "retract twice refused" "$(st)" 409
req GET "/api/v1/annotations?datastream_id=$TEMP&from=$FROM&to=$TO"; check "retracted one hidden" "$(jx 'j.map(a=>a.text).join(",")')" "Door open for loading"
req GET "/api/v1/annotations?datastream_id=$TEMP&from=$FROM&to=$TO&include_retracted=1"; check "…kept as a record" "$(jx 'j.find(a=>a.id==="'"$ANN2"'").retract_reason')" "wrong probe"
req GET "/api/v1/export/audit?from=$(iso $((NOW - 120)))&format=jsonl"; check "annotations audited (two created, one retracted)" "$(grep -e "$ANN" -e "$ANN2" "$TMP/b" | grep -c 'annotation\.\(create\|retract\)')" "3"

echo "== create a datastream for a key and assign it"
req POST "/api/v1/devices/$DEV/telemetry/assign" "{\"key\":\"co2\",\"asset_id\":\"$ASSET\",\"quantity\":\"co2\",\"unit\":\"ppm\",\"reason\":\"e2e\"}"; check "created and assigned" "$(st)" 201
CO2=$(jx j.datastream_id)
req POST "/api/v1/devices/$DEV/telemetry/assign" "{\"key\":\"co2\",\"asset_id\":\"$ASSET\",\"reason\":\"e2e\"}"; check "the same key again refused" "$(st)" 409
req POST "/api/v1/devices/$DEV/telemetry/assign" "{\"key\":\"rh\",\"asset_id\":\"$ASSET\"}"; check "reason required" "$(st)" 400
dev telemetry '{"v":{"co2":812}}'; sleep 1
req GET "/api/v1/devices/$DEV/telemetry?from=$FROM&to=$TO"; check "co2 is a datastream now" "$(jx 'j.keys.find(k=>k.key==="co2").source+" "+j.keys.find(k=>k.key==="co2").last.value+" "+j.keys.find(k=>k.key==="co2").unit')" "datastream 812 ppm"

echo "== permissions and organization scope"
req POST /api/v1/users "{\"username\":\"telview$SUF\",\"full_name\":\"Viewer\",\"roles\":[\"viewer\"],\"reason\":\"e2e\"}"; VPW=$(jx j.initial_password)
signin "$TMP/jarv" "telview$SUF" "$VPW" "Viewer-e2e-$SUF-pass!"
req GET "/api/v1/devices/$DEV/telemetry"; check "viewer reads the telemetry" "$(st)" 200
req GET "/api/v1/devices/$DEV/telemetry/export?key=temp"; check "viewer cannot export (data.export)" "$(st)" 403
req POST "/api/v1/devices/$DEV/telemetry/assign" "{\"key\":\"rh\",\"asset_id\":\"$ASSET\",\"reason\":\"e2e\"}"; check "viewer cannot assign (device.manage)" "$(st)" 403
req POST /api/v1/annotations "{\"datastream_id\":\"$TEMP\",\"from\":\"$OBS_AT\",\"text\":\"viewer note\"}"; check "viewer cannot annotate (data.annotate)" "$(st)" 403
admin
req POST /api/v1/organizations "{\"slug\":\"tel-$SUF\",\"name\":\"Other $SUF\",\"admin_username\":\"oadm$SUF\",\"admin_full_name\":\"Other admin\",\"reason\":\"e2e\"}"
if [[ "$(st)" == "201" ]]; then
  OPW=$(jx j.initial_password)
  signin "$TMP/jaro" "oadm$SUF" "$OPW" "Other-e2e-$SUF-pass!"
  # the new organization requires two-factor sign-in for its administrator: enrol and sign in with a code
  req POST /api/v1/auth/totp/setup '{}'; SECRET=$(jx j.secret)
  ( cd "$(dirname "$0")/../.." && go build -o "$TMP/totpcode" ./tools/totpcode )
  req POST /api/v1/auth/totp/confirm "{\"code\":\"$("$TMP/totpcode" -secret "$SECRET")\"}"
  req POST /api/v1/auth/login "{\"username\":\"oadm$SUF\",\"password\":\"Other-e2e-$SUF-pass!\"}"; PRE=$(jx j.pre_auth_token)
  sleep $((30 - $(date +%s) % 30 + 1))
  req POST /api/v1/auth/mfa "{\"pre_auth_token\":\"$PRE\",\"code\":\"$("$TMP/totpcode" -secret "$SECRET")\"}"; check "other organization's administrator signed in" "$(st)" 200
  req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
  req GET "/api/v1/devices/$DEV/telemetry"; check "another organization: device not found" "$(st)" 404
  req GET "/api/v1/devices/$DEV/telemetry/values?key=temp"; check "another organization: no values" "$(st)" 404
  req GET "/api/v1/annotations?datastream_id=$TEMP"; check "another organization: no annotations" "$(jx 'j.length')" "0"
  curl -s -N -b "$JAR" --max-time 3 "$BASE/api/v1/stream?devices=$DEV&device_data=1" > "$TMP/sse2" & SSE_PID=$!
  sleep 1; dev status '{"bat":3.5}'; wait $SSE_PID 2>/dev/null; SSE_PID=""
  grep -q 'device_data' "$TMP/sse2" && bad "another organization got live data" || ok "another organization: no live data"
  admin
else bad "organization not created ($(st)): scope not checked"; fi

admin
req POST "/api/v1/devices/$DEV/revoke" '{"reason":"e2e cleanup"}'
echo
echo "device_telemetry: $pass passed, $fail failed"
[[ $fail -eq 0 ]]

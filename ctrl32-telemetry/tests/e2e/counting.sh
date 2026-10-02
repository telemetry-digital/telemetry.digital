#!/usr/bin/env bash
# End-to-end checks for object counting (spec 029): areas with entrances and their assets and datastreams, sensors
# through the device HTTP API (a value per passage, a running total with a restart), the occupancy (never below zero,
# flagged when clamped, corrected with a reason, reset daily), the summary of a day with shares of the entrances,
# period tables (sum) and an alarm rule on the occupancy, camera counting configuration (lines, speed sections,
# validation, the worker state), permissions and the audit trail.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/counting.sh
#   WAIT_RESET=1 also waits for a daily reset (up to two minutes)
set -u
BASE=${BASE:-http://127.0.0.1:8080}
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
tok()  { curl -s -X "$1" -H "Authorization: Bearer $2" -H "Content-Type: application/json" -o "$TMP/b" -w '%{http_code}' ${4:+--data-binary "$4"} "$BASE$3" > "$TMP/s"; }
dev()  { curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -H "Authorization: Bearer $1" -d "$2" "$BASE/api/v1/d/telemetry"; }
waitfor() { local n=${2:-40}; for i in $(seq "$n"); do eval "$1" && return 0; sleep 0.25; done; return 1; }
SUF=$RANDOM

echo "== areas and entrances"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req POST /api/v1/sites "{\"name\":\"Nitra $SUF\",\"reason\":\"e2e\"}"; SITE=$(jx j.id)
req POST /api/v1/counting/areas "{\"site_id\":\"$SITE\",\"name\":\"Main building $SUF\",\"groups\":[\"people\"],\"limits\":{\"people\":10},\"entrances\":[\"Main entrance\",\"Staff entrance\",\"Loading dock\"],\"reason\":\"e2e\"}"
check "area with three entrances" "$(st)" 201; AREA=$(jx j.id); AASSET=$(jx j.asset_id)
req POST /api/v1/counting/areas "{\"site_id\":\"$SITE\",\"name\":\"Car park $SUF\",\"kind\":\"car_park\",\"groups\":[\"vehicles\",\"cars\",\"trucks\"],\"entrances\":[\"Gate\"],\"reason\":\"e2e\"}"
check "car park counting vehicles, cars and trucks" "$(st)" 201; PARK=$(jx j.id)
req POST /api/v1/counting/areas "{\"site_id\":\"$SITE\",\"name\":\"X $SUF\",\"groups\":[\"dogs\"],\"reason\":\"e2e\"}"; check "unknown group refused" "$(st)" 400
req POST /api/v1/counting/areas "{\"site_id\":\"$SITE\",\"name\":\"X $SUF\",\"reset_time\":\"25:00\",\"reason\":\"e2e\"}"; check "bad reset time refused" "$(st)" 400
req POST /api/v1/counting/areas "{\"site_id\":\"$SITE\",\"name\":\"X $SUF\"}"; check "reason required" "$(st)" 400
req POST /api/v1/counting/areas "{\"name\":\"X $SUF\",\"reason\":\"e2e\"}"; check "site required" "$(st)" 400
req POST /api/v1/counting/areas "{\"site_id\":\"$SITE\",\"name\":\"Main building $SUF\",\"reason\":\"e2e\"}"; check "duplicate name refused" "$(st)" 409
req GET /api/v1/counting/areas
check "area listed with entrances and occupancy 0" "$(jx "(a => a.entrances.map(e => e.name).join() + ' ' + a.occupancy.people + ' ' + a.limits.people)(j.areas.find(a => a.id === '$AREA'))")" "Main entrance,Staff entrance,Loading dock 0 10"
check "groups and detector classes offered" "$(jx 'j.groups.length + " " + j.classes.join()')" "7 person,bicycle,car,motorcycle,bus,truck"
MAIN=$(jx "j.areas.find(a => a.id === '$AREA').entrances[0].id"); STAFF=$(jx "j.areas.find(a => a.id === '$AREA').entrances[1].id"); DOCK=$(jx "j.areas.find(a => a.id === '$AREA').entrances[2].id")
GATE=$(jx "j.areas.find(a => a.id === '$PARK').entrances[0].id"); MAINASSET=$(jx "j.areas.find(a => a.id === '$AREA').entrances[0].asset_id")
req GET /api/v1/datastreams
OCC=$(jx "j.find(d => d.asset_id === '$AASSET' && d.key === 'people_occupancy').id")
check "datastreams of the area (in, out, occupancy) and of each entrance (in, out)" "$(jx "j.filter(d => d.asset_id === '$AASSET').map(d => d.key).sort().join() + ' ' + j.filter(d => d.asset_id === '$MAINASSET').map(d => d.key).sort().join()")" "people_in,people_occupancy,people_out people_in,people_out"
check "car park: per group datastreams" "$(jx "j.filter(d => d.asset === 'Car park $SUF').map(d => d.key).sort().join()")" "cars_in,cars_occupancy,cars_out,trucks_in,trucks_occupancy,trucks_out,vehicles_in,vehicles_occupancy,vehicles_out"
req GET /api/v1/assets
check "assets: the area (counting_area) and its entrances under it" "$(jx "j.filter(a => a.name === 'Main building $SUF' || a.parent_id === '$AASSET').map(a => a.type).sort().join()")" "counting_area,entrance,entrance,entrance"

req POST "/api/v1/alarm-rules" "{\"datastream_id\":\"$OCC\",\"type\":\"high\",\"warning\":8,\"action\":10,\"reason\":\"e2e capacity of the building\"}"; check "alarm rule on the occupancy (the existing alarm rules)" "$(st)" 201

echo "== sensors through the device protocol"
req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"sensor\",\"name\":\"Door sensors $SUF\",\"reason\":\"e2e\"}"; SA=$(jx j.id)
for k in beam_in beam_out turn_total park_in; do
  req POST /api/v1/datastreams "{\"asset_id\":\"$SA\",\"key\":\"$k\",\"quantity\":\"count\",\"unit\":\"\",\"kind\":\"gauge\",\"reason\":\"e2e\"}"; eval "DS_$k=$(jx j.id)"
done
req POST /api/v1/devices "{\"external_id\":\"door-$SUF\",\"name\":\"Door counter $SUF\",\"reason\":\"e2e\"}"; DEV=$(jx j.id)
for d in $DS_beam_in $DS_beam_out $DS_turn_total $DS_park_in; do req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$d\",\"reason\":\"e2e\"}"; done
req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}'; DTOK=$(jx j.token)
[[ -n "$DTOK" ]] && ok "a door counter device with four datastreams" || bad "device: $(cat "$TMP/b")"
req POST /api/v1/counting/inputs "{\"entrance_id\":\"$MAIN\",\"name\":\"Light barrier\",\"in_datastream_id\":\"$DS_beam_in\",\"out_datastream_id\":\"$DS_beam_out\",\"reason\":\"e2e\"}"; check "input: a value per passage" "$(st)" 201
req POST /api/v1/counting/inputs "{\"entrance_id\":\"$STAFF\",\"name\":\"Turnstile\",\"in_datastream_id\":\"$DS_turn_total\",\"mode\":\"total\",\"reason\":\"e2e\"}"; check "input: a running total" "$(st)" 201
req POST /api/v1/counting/inputs "{\"entrance_id\":\"$GATE\",\"name\":\"Gate loop\",\"group\":\"trucks\",\"in_datastream_id\":\"$DS_park_in\",\"reason\":\"e2e\"}"; check "input: trucks at the gate" "$(st)" 201
req POST /api/v1/counting/inputs "{\"entrance_id\":\"$MAIN\",\"name\":\"bad\",\"in_datastream_id\":\"$DS_beam_in\",\"mode\":\"sometimes\",\"reason\":\"e2e\"}"; check "unknown mode refused" "$(st)" 400
req GET /api/v1/datastreams; OWN=$(jx "j.find(d => d.asset_id === '$AASSET' && d.key === 'people_in').id")
req POST /api/v1/counting/inputs "{\"entrance_id\":\"$MAIN\",\"name\":\"loop\",\"in_datastream_id\":\"$OWN\",\"reason\":\"e2e\"}"; check "a counting result cannot be a sensor (no loop)" "$(st)" 400
req GET /api/v1/counting/inputs; check "inputs listed" "$(jx "j.filter(i => i.area === 'Main building $SUF').length")" 2
SUMMARY="/api/v1/counting/summary?area=$AREA"
check "passages 3 in" "$(dev "$DTOK" '{"beam_in":3}')" 202
waitfor "req GET '$SUMMARY'; [[ \"\$(jx 'j.totals.people && j.totals.people.in')\" == 3 ]]" 40 && ok "3 people in through the main entrance" || bad "summary: $(head -c 300 "$TMP/b")"
dev "$DTOK" '{"beam_out":1}' >/dev/null
dev "$DTOK" '{"turn_total":100}' >/dev/null   # baseline of the running total
dev "$DTOK" '{"turn_total":105}' >/dev/null   # +5
dev "$DTOK" '{"turn_total":2}' >/dev/null     # the turnstile restarted: +2
dev "$DTOK" '{"park_in":2}' >/dev/null
waitfor "req GET '$SUMMARY'; [[ \"\$(jx 'j.totals.people.in')\" == 10 ]]" 40 && ok "the running total adds 5 and, after a restart, 2" || bad "totals: $(jx 'j.totals')"
check "in 10, out 1, occupancy 9" "$(jx 'j.totals.people.in + " " + j.totals.people.out + " " + j.area.occupancy.people')" "10 1 9"
check "entrances: main 3/1, staff 7/0, dock 0/0" "$(jx 'j.entrances.map(e => (e.counts.people||{in:0}).in + "/" + ((e.counts.people||{}).out||0)).join()')" "3/1,7/0,0/0"
check "shares of the entrances (in)" "$(jx 'j.entrances.map(e => e.share.people.in).join()')" "30,70,0"
check "a row per hour, the passages in the current hour" "$(jx 'j.hours.length >= 23 && j.hours.reduce((a, h) => a + (h.in.people||0), 0)')" 10
check "days for comparison (7) end today" "$(jx 'j.days.length + " " + (j.days[6].date === j.date) + " " + j.days[6].in.people')" "7 true 10"
check "peak occupancy of the day" "$(jx 'j.peak.people')" 9
req GET "/api/v1/counting/summary?area=$PARK"
check "trucks count as vehicles too" "$(jx 'j.totals.trucks.in + " " + j.totals.vehicles.in + " " + (j.totals.cars ? j.totals.cars.in : 0)')" "2 2 0"
req GET "/api/v1/counting/summary?datastream=$OWN"; check "summary found by a datastream of the area (widgets)" "$(st) $(jx 'j.area.id')" "200 $AREA"

echo "== occupancy: never below zero, corrections, alarms"
dev "$DTOK" '{"beam_out":50}' >/dev/null
waitfor "req GET '$SUMMARY'; [[ \"\$(jx 'j.totals.people.out')\" == 51 ]]" 40 && ok "50 people out" || bad "out: $(jx 'j.totals')"
check "occupancy clamped at 0, clamp counted" "$(jx 'j.area.occupancy.people + " " + j.area.clamped.people')" "0 1"
req GET "/api/v1/counting/log?area=$AREA"; check "clamp in the counting log" "$(jx 'j[0].kind + " " + j[0].before + " " + j[0].after')" "clamp -41 0"
req GET "/api/v1/observations?datastream_id=$OCC"
check "the clamped value is flagged (quality out_of_range)" "$(jx 'j.filter(o => o.quality === "out_of_range").map(o => o.value).join()')" "0"
req POST "/api/v1/counting/areas/$AREA/occupancy" "{\"group\":\"people\",\"value\":12}"; check "correction needs a reason" "$(st)" 400
req POST "/api/v1/counting/areas/$AREA/occupancy" "{\"group\":\"cars\",\"value\":3,\"reason\":\"e2e\"}"; check "correction of a group the area does not count refused" "$(st)" 400
req POST "/api/v1/counting/areas/$AREA/occupancy" "{\"group\":\"people\",\"value\":12,\"reason\":\"e2e counted by hand at the reception\"}"
check "correction 0 → 12" "$(st) $(jx 'j.before + "→" + j.occupancy')" "200 0→12"
waitfor "req GET '/api/v1/alarms?datastream_id=$OCC'; [[ \"\$(jx 'j.length > 0 && j[0].state')\" == active ]]" 40 && ok "occupancy over the capacity raises the alarm" || bad "alarm: $(head -c 300 "$TMP/b")"
req GET "/api/v1/counting/log?area=$AREA"; check "correction with its reason and actor" "$(jx 'j[0].kind + " " + j[0].after + " " + j[0].reason + " " + j[0].actor')" "correction 12 e2e counted by hand at the reception $USER"
dev "$DTOK" '{"beam_in":2}' >/dev/null
waitfor "req GET '$SUMMARY'; [[ \"\$(jx 'j.area.occupancy.people')\" == 14 ]]" 40 && ok "counting continues from the corrected value" || bad "occupancy $(jx 'j.area.occupancy')"

echo "== period tables and exports on the counts"
PT="/api/v1/period-table?datastreams=$(jx 'j.datastreams.people_in'),$(jx 'j.datastreams.people_out'),$(jx 'j.datastreams.people_occupancy')&interval=day&values=sum,max&period=today&total=1"
req GET "$PT"; check "period table: sum of passages, largest single reading, max of occupancy" "$(st) $(jx 'j.columns.map(c => (c.label.endsWith("occupancy") ? "" : j.totals[c.datastream_id].sum) + "/" + j.totals[c.datastream_id].max).join()')" "200 12/5,51/50,/14"
req GET "${PT/interval=day/interval=hour}"; check "per hour" "$(st) $(jx 'j.periods.length >= 23')" "200 true"
curl -s -b "$JAR" -o "$TMP/f" -w '%{http_code}' "$BASE${PT/period-table/period-table/export}&format=xlsx" > "$TMP/s"; check "export to Excel" "$(st) $(head -c 2 "$TMP/f")" "200 PK"

echo "== cameras: lines and speed sections"
req POST /api/v1/video/cameras "{\"name\":\"Entrance cam $SUF\",\"url\":\"rtsp://127.0.0.1:18554/gate/h264/640x360\",\"record\":\"off\",\"reason\":\"e2e\"}"; CAM=$(jx j.id)
[[ -n "$CAM" && "$CAM" != null ]] && ok "a camera" || bad "camera: $(cat "$TMP/b")"
LINE="{\"id\":\"l1\",\"name\":\"Door line\",\"a\":{\"x\":0.1,\"y\":0.6},\"b\":{\"x\":0.9,\"y\":0.6},\"in\":\"right\",\"classes\":[\"person\"],\"entrance_id\":\"$MAIN\"}"
req PUT "/api/v1/counting/cameras/$CAM" "{\"enabled\":true,\"config\":{\"lines\":[{\"id\":\"l1\",\"name\":\"x\",\"a\":{\"x\":0.1,\"y\":0.6},\"b\":{\"x\":1.4,\"y\":0.6},\"in\":\"right\"}]},\"reason\":\"e2e\"}"; check "a line outside the picture refused" "$(st)" 400
req PUT "/api/v1/counting/cameras/$CAM" "{\"enabled\":true,\"config\":{\"lines\":[{\"id\":\"l1\",\"name\":\"x\",\"a\":{\"x\":0.1,\"y\":0.6},\"b\":{\"x\":0.9,\"y\":0.6},\"in\":\"up\"}]},\"reason\":\"e2e\"}"; check "a side other than left/right refused" "$(st)" 400
req PUT "/api/v1/counting/cameras/$CAM" "{\"enabled\":true,\"config\":{\"lines\":[{\"id\":\"l1\",\"name\":\"x\",\"a\":{\"x\":0.1,\"y\":0.6},\"b\":{\"x\":0.9,\"y\":0.6},\"in\":\"right\",\"entrance_id\":\"01a0e90b-0000-7000-8000-000000000000\"}]},\"reason\":\"e2e\"}"; check "an unknown entrance refused" "$(st)" 400
req PUT "/api/v1/counting/cameras/$CAM" "{\"enabled\":true,\"fps\":40,\"config\":{\"lines\":[$LINE]},\"reason\":\"e2e\"}"; check "more than 15 analysed pictures per second refused" "$(st)" 400
req PUT "/api/v1/counting/cameras/$CAM" "{\"enabled\":true,\"config\":{\"lines\":[$LINE]}}"; check "reason required" "$(st)" 400
SPEED="{\"id\":\"s1\",\"name\":\"Plant road $SUF\",\"line_a\":\"a\",\"line_b\":\"b\",\"distance_m\":20,\"limit_kmh\":30,\"area_id\":\"$PARK\"}"
LA="{\"id\":\"a\",\"name\":\"A\",\"a\":{\"x\":0,\"y\":0.3},\"b\":{\"x\":1,\"y\":0.3},\"in\":\"right\",\"classes\":[\"car\",\"truck\"],\"entrance_id\":\"$GATE\"}"
LB="{\"id\":\"b\",\"name\":\"B\",\"a\":{\"x\":0,\"y\":0.7},\"b\":{\"x\":1,\"y\":0.7},\"in\":\"right\",\"classes\":[\"car\",\"truck\"]}"
req PUT "/api/v1/counting/cameras/$CAM" "{\"enabled\":true,\"config\":{\"lines\":[$LINE,$LA,$LB],\"sections\":[{\"id\":\"s1\",\"name\":\"x\",\"line_a\":\"a\",\"line_b\":\"b\",\"distance_m\":0.5,\"area_id\":\"$PARK\"}]},\"reason\":\"e2e\"}"; check "a speed section shorter than 1 m refused" "$(st)" 400
req PUT "/api/v1/counting/cameras/$CAM" "{\"enabled\":true,\"fps\":8,\"model\":\"nano\",\"config\":{\"lines\":[$LINE,$LA,$LB],\"sections\":[$SPEED]},\"reason\":\"e2e count people and vehicles\"}"
check "lines and a speed section saved" "$(st) $(jx 'j.config.sections[0].asset_id.length')" "200 36"
SECASSET=$(jx 'j.config.sections[0].asset_id')
req GET /api/v1/datastreams; check "speed datastreams on the section's asset" "$(jx "j.filter(d => d.asset_id === '$SECASSET').map(d => d.key + ':' + d.unit).sort().join()")" "over_limit:vehicles,vehicle_speed:km/h"
req GET "/api/v1/counting/cameras/$CAM"; check "configuration read back" "$(jx 'j.enabled + " " + j.fps + " " + j.config.lines.length + " " + j.config.lines[0].entrance_id')" "true 8 3 $MAIN"
STATE=$(jx 'j.status.state'); [[ " waiting starting running not_installed error " == *" $STATE "* ]] && ok "worker state reported ($STATE)" || bad "state: $STATE"
req GET /api/v1/counting/cameras; check "camera in the list with its state" "$(jx "(c => c.enabled + ' ' + (c.status.state !== undefined))(j.find(c => c.camera_id === '$CAM'))")" "true true"
req PUT "/api/v1/counting/cameras/$CAM" "{\"enabled\":false,\"config\":{\"lines\":[$LINE]},\"reason\":\"e2e off\"}"; check "switched off" "$(st)" 200
req GET "/api/v1/counting/cameras/$CAM"; check "off: state off" "$(jx 'j.enabled + " " + j.status.state')" "false off"

echo "== daily reset"
if [[ "${WAIT_RESET:-}" == 1 ]]; then
  NEXT=$(node -e 'const tz=process.argv[1]; const d=new Date(Date.now()+60000); console.log(d.toLocaleTimeString("en-GB",{timeZone:tz,hour:"2-digit",minute:"2-digit",hour12:false}))' "$(curl -s -b "$JAR" "$BASE$SUMMARY" | node -pe 'JSON.parse(require("fs").readFileSync(0)).tz')")
  req PUT "/api/v1/counting/areas/$AREA" "{\"name\":\"Main building $SUF\",\"groups\":[\"people\"],\"reset_time\":\"$NEXT\",\"limits\":{\"people\":10},\"reason\":\"e2e reset in a minute\"}"; check "reset time set to $NEXT" "$(st)" 200
  waitfor "req GET '$SUMMARY'; [[ \"\$(jx 'j.area.occupancy.people')\" == 0 ]]" 480 && ok "reset at $NEXT: occupancy 0" || bad "no reset: $(jx 'j.area')"
  req GET "/api/v1/counting/log?area=$AREA"; check "reset in the counting log" "$(jx 'j[0].kind + " " + j[0].before + " " + j[0].actor')" "reset 14 system"
else
  echo "  skip waiting for the daily reset (WAIT_RESET=1)"
fi

echo "== permissions and audit"
req POST /api/v1/tokens '{"name":"reader","scopes":["data.read"]}'; RT=$(jx j.token)
tok GET "$RT" /api/v1/counting/areas; check "a reader sees the areas" "$(st)" 200
tok GET "$RT" "$SUMMARY"; check "a reader sees the summary" "$(st)" 200
tok POST "$RT" /api/v1/counting/areas "{\"site_id\":\"$SITE\",\"name\":\"Y\",\"reason\":\"e2e\"}"; check "a reader cannot create an area" "$(st)" 403
tok POST "$RT" "/api/v1/counting/areas/$AREA/occupancy" '{"group":"people","value":1,"reason":"e2e"}'; check "a reader cannot correct the occupancy" "$(st)" 403
tok PUT "$RT" "/api/v1/counting/cameras/$CAM" '{"enabled":true,"config":{"lines":[]},"reason":"e2e"}'; check "a reader cannot configure a camera" "$(st)" 403
req GET "/api/v1/export/audit?format=csv"
for a in counting_area.create counting_input.create counting_occupancy.update counting_camera.update; do grep -q "$a" "$TMP/b" && ok "audited: $a" || bad "not audited: $a"; done
req DELETE "/api/v1/counting/entrances/$DOCK?reason=e2e"; check "entrance archived" "$(st)" 200
req GET /api/v1/counting/areas; check "two entrances left" "$(jx "j.areas.find(a => a.id === '$AREA').entrances.length")" 2
req DELETE "/api/v1/counting/areas/$PARK?reason=e2e"; check "area archived" "$(st)" 200
req GET /api/v1/counting/areas; check "archived area not listed" "$(jx "j.areas.some(a => a.id === '$PARK')")" false
req GET /api/v1/openapi.json; check "documented in OpenAPI" "$(jx 'Object.keys(j.paths).filter(p => p.startsWith("/api/v1/counting")).length')" 11
req DELETE "/api/v1/video/cameras/$CAM?reason=e2e"

echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

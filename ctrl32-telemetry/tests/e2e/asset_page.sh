#!/usr/bin/env bash
# End-to-end checks for the asset and site pages (0.74.0): the enriched assets and sites lists, GET /api/v1/assets/{id}
# (ancestors, children, datastreams with the latest value, devices, active alarms, position), the asset history from
# the audit trail, the telemetry of an asset (with and without the assets below it), its chart, values and exports,
# moving an asset in the hierarchy (no cycles, same site), positions of assets and sites, GET /api/v1/sites/{id},
# alarm rules by asset, the assets a device feeds, permissions and the pages.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/asset_page.sh
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
jget() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  cat "$TMP/b"; }
tok()  { curl -s -o "$TMP/b" -w '%{http_code}' -H "Authorization: Bearer $1" "$BASE$2" > "$TMP/s"; cat "$TMP/b"; }
dev()  { printf '%s' "$2" > "$TMP/dreq"; curl -s -X POST -H "Authorization: Bearer $1" -H "Content-Type: application/json" -o "$TMP/db" -w '%{http_code}' --data-binary @"$TMP/dreq" "$BASE/api/v1/d/telemetry" > "$TMP/ds"; }
SUF=$RANDOM
NOW=$(date +%s); FROM=$(date -u -d "@$((NOW - 7200))" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || node -e "console.log(new Date(($NOW-7200)*1000).toISOString().replace(/\.\d+Z/,'Z'))")
TO=$(date -u -d "@$((NOW + 60))" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || node -e "console.log(new Date(($NOW+60)*1000).toISOString().replace(/\.\d+Z/,'Z'))")

echo "== login"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

echo "== a site with a room, a fridge in the room and a meter"
SITE=$(req POST /api/v1/sites "{\"name\":\"AP site $SUF\",\"address\":\"1 Asset Street\",\"reason\":\"e2e\"}" | jget id)
SITE2=$(req POST /api/v1/sites "{\"name\":\"AP other site $SUF\",\"reason\":\"e2e\"}" | jget id)
ROOM=$(req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"room\",\"name\":\"Room $SUF\",\"attributes\":{\"floor\":\"ground\"},\"reason\":\"e2e\"}" | jget id)
FRIDGE=$(req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"parent_id\":\"$ROOM\",\"type\":\"fridge\",\"name\":\"Fridge $SUF\",\"reason\":\"e2e\"}" | jget id)
SHELF=$(req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"parent_id\":\"$FRIDGE\",\"type\":\"shelf\",\"name\":\"Shelf $SUF\",\"reason\":\"e2e\"}" | jget id)
METER=$(req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"meter\",\"name\":\"Meter $SUF\",\"reason\":\"e2e\"}" | jget id)
FAR=$(req POST /api/v1/assets "{\"site_id\":\"$SITE2\",\"type\":\"room\",\"name\":\"Far room $SUF\",\"reason\":\"e2e\"}" | jget id)
RT=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ROOM\",\"key\":\"temp\",\"quantity\":\"temperature\",\"unit\":\"C\",\"expected_interval\":\"10 minutes\",\"reason\":\"e2e\"}" | jget id)
FT=$(req POST /api/v1/datastreams "{\"asset_id\":\"$FRIDGE\",\"key\":\"ftemp\",\"quantity\":\"temperature\",\"unit\":\"C\",\"reason\":\"e2e\"}" | jget id)
MT=$(req POST /api/v1/datastreams "{\"asset_id\":\"$METER\",\"key\":\"energy\",\"quantity\":\"energy\",\"unit\":\"kWh\",\"reason\":\"e2e\"}" | jget id)
for v in SITE SITE2 ROOM FRIDGE SHELF METER FAR RT FT MT; do [[ -n "${!v}" ]] || bad "created $v"; done; ok "sites, assets and datastreams created"
DEV=$(req POST /api/v1/devices "{\"external_id\":\"ap-e2e-$SUF\",\"name\":\"AP logger $SUF\",\"transport\":\"http\",\"reason\":\"e2e\"}" | jget id)
TOK=$(req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}' | jget token); [[ -n "$TOK" ]] && ok "device and token" || bad "device token"
for d in $RT $FT; do req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$d\",\"reason\":\"e2e\"}" >/dev/null; done; check "assigned" "$(st)" 201
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$FT\",\"type\":\"high\",\"warning\":7,\"action\":8,\"reason\":\"e2e\"}" >/dev/null; check "alarm rule on the fridge" "$(st)" 201
S="["; for i in 6 5 4 3 2 1; do S="$S{\"ts\":$((NOW - i * 600)),\"v\":{\"temp\":2$i.5,\"ftemp\":4.$i}},"; done; S="$S{\"ts\":$((NOW - 30)),\"v\":{\"temp\":21.0,\"ftemp\":9.5}}]"
dev "$TOK" "$S"; check "history sent" "$(cat "$TMP/ds")" 202
sleep 1

echo "== assets list"
req GET "/api/v1/assets?site_id=$SITE" >/dev/null; check "list of the site" "$(st)" 200
check "four assets" "$(jx 'j.length')" 4
check "room: datastreams, devices, children" "$(jx "(a => [a.datastreams, a.devices, a.children].join())(j.find(a => a.id === '$ROOM'))")" "1,1,1"
check "fridge: parent and alarm" "$(jx "(a => a.parent_id + ' ' + (a.alarm && a.alarm.severity) + ' ' + a.alarm.count)(j.find(a => a.id === '$FRIDGE'))")" "$ROOM action 1"
check "room: last value time" "$(jx "!!j.find(a => a.id === '$ROOM').last_at")" true
check "meter: no device, no value, no alarm" "$(jx "(a => [a.devices, a.last_at, a.alarm].join())(j.find(a => a.id === '$METER'))")" "0,,"
req GET "/api/v1/assets?site_id=nonsense" >/dev/null; check "invalid site_id refused" "$(st)" 400
req GET /api/v1/sites >/dev/null; check "sites list" "$(st)" 200
check "site: assets, datastreams, devices, alarm" "$(jx "(s => [s.assets, s.datastreams, s.devices, s.alarm.severity].join())(j.find(s => s.id === '$SITE'))")" "4,3,1,action"

echo "== asset page"
req GET "/api/v1/assets/$SHELF" >/dev/null; check "shelf" "$(st)" 200
check "ancestors: room, then fridge" "$(jx 'j.path.map(p => p.id).join()')" "$ROOM,$FRIDGE"
check "site of the shelf" "$(jx 'j.site.name')" "AP site $SUF"
req GET "/api/v1/assets/$ROOM" >/dev/null; check "room" "$(st)" 200
check "no ancestors" "$(jx 'j.path.length')" 0
check "child: the fridge" "$(jx 'j.children.map(c => c.id).join()')" "$FRIDGE"
check "datastream with latest value and device" "$(jx "(d => [d.key, d.last.value, d.device_id, d.expected_interval].join())(j.datastreams[0])")" "temp,21,$DEV,00:10:00"
check "device feeding it, with the keys of this asset" "$(jx "(d => d.id + ' ' + d.keys.join('+'))(j.devices[0])")" "$DEV temp"
check "no alarms on the room" "$(jx 'j.alarms.length')" 0
check "no position yet" "$(jx 'j.location')" null
req GET "/api/v1/assets/$FRIDGE" >/dev/null
check "active alarm of the fridge" "$(jx "(a => [a.severity, a.key, a.datastream_id, a.asset_id].join())(j.alarms[0])")" "action,ftemp,$FT,$FRIDGE"
check "alarm rule counted" "$(jx 'j.datastreams[0].alarm_rules')" 1
req GET "/api/v1/assets/00000000-0000-7000-8000-000000000000" >/dev/null; check "unknown asset 404" "$(st)" 404
req GET "/api/v1/assets/nonsense" >/dev/null; check "invalid id 404" "$(st)" 404

echo "== positions"
req PATCH "/api/v1/sites/$SITE" '{"location":{"lat":48.3076,"lon":18.0858},"reason":"e2e"}' >/dev/null; check "site position" "$(st)" 200
req PATCH "/api/v1/sites/$SITE" '{"location":{"lat":91,"lon":0},"reason":"e2e"}' >/dev/null; check "position out of range refused" "$(st)" 400
req GET "/api/v1/assets/$ROOM" >/dev/null; check "room takes the site's position" "$(jx "[j.location.lat, j.location.source].join()")" "48.3076,site"
req PATCH "/api/v1/assets/$ROOM" '{"location":{"lat":48.31,"lon":18.09},"reason":"e2e"}' >/dev/null; check "asset position" "$(st)" 200
req GET "/api/v1/assets/$ROOM" >/dev/null; check "own position" "$(jx "[j.location.lat, j.location.source, j.asset.attributes.floor].join()")" "48.31,asset,ground"
req PATCH "/api/v1/assets/$ROOM" '{"attributes":{"floor":"first","area_m2":24},"reason":"e2e"}' >/dev/null; check "attributes replaced" "$(st)" 200
req GET "/api/v1/assets/$ROOM" >/dev/null; check "attributes replaced as a whole (the page keeps lat and lon itself)" "$(jx "[j.asset.attributes.floor, j.asset.attributes.area_m2, j.asset.attributes.lat].join()")" "first,24,"
req PATCH "/api/v1/assets/$ROOM" '{"attributes":[1,2],"reason":"e2e"}' >/dev/null; check "attributes must be an object" "$(st)" 400
req PATCH "/api/v1/assets/$ROOM" '{"location":{"lat":48.31,"lon":18.09},"reason":"e2e"}' >/dev/null
req PATCH "/api/v1/assets/$ROOM" '{"clear_location":true,"reason":"e2e"}' >/dev/null; check "clear position" "$(st)" 200
req GET "/api/v1/assets/$ROOM" >/dev/null; check "back to the site's position" "$(jx 'j.location.source')" "site"
req PATCH "/api/v1/sites/$SITE" '{"location":{"lat":91},"reason":"e2e"}' >/dev/null; check "site position: lat out of range" "$(st)" 400

echo "== moving in the hierarchy"
req PATCH "/api/v1/assets/$ROOM" "{\"parent_id\":\"$SHELF\",\"reason\":\"e2e\"}" >/dev/null; check "under its own descendant refused (cycle)" "$(st)" 400
req PATCH "/api/v1/assets/$ROOM" "{\"parent_id\":\"$ROOM\",\"reason\":\"e2e\"}" >/dev/null; check "under itself refused" "$(st)" 400
req PATCH "/api/v1/assets/$METER" "{\"parent_id\":\"$FAR\",\"reason\":\"e2e\"}" >/dev/null; check "under an asset of another site refused" "$(st)" 400
req PATCH "/api/v1/assets/$METER" "{\"parent_id\":\"$ROOM\",\"reason\":\"e2e move\"}" >/dev/null; check "meter moved under the room" "$(st)" 200
req GET "/api/v1/assets/$ROOM" >/dev/null; check "room has two children" "$(jx 'j.children.length')" 2
req PATCH "/api/v1/assets/$METER" '{"parent_id":"","reason":"e2e back"}' >/dev/null; check "meter back to the top level" "$(st)" 200
req GET "/api/v1/assets/$METER" >/dev/null; check "no parent" "$(jx 'j.asset.parent_id + j.path.length')" "0"

echo "== telemetry of an asset"
req GET "/api/v1/assets/$ROOM/telemetry?from=$FROM&to=$TO" >/dev/null; check "summary" "$(st)" 200
check "only the room's datastream" "$(jx 'j.keys.map(k => k.key).join()')" "temp"
check "latest, stats, device, asset" "$(jx "(k => [k.last.value, k.stats.n, k.stats.max, k.device_id === '$DEV', k.asset_id === '$ROOM'].join())(j.keys[0])")" "21,7,26.5,true,true"
req GET "/api/v1/assets/$ROOM/telemetry?from=$FROM&to=$TO&children=1" >/dev/null; check "with the assets below" "$(jx '[j.assets, j.keys.map(k => k.key).sort().join("+")].join()')" "3,ftemp+temp"
check "sensor fault of the fridge? no: 9.5 ok, quality ok" "$(jx "j.keys.find(k => k.key === 'ftemp').last.quality")" "ok"
req GET "/api/v1/assets/$ROOM/telemetry/series?datastream_id=$FT&from=$FROM&to=$TO" >/dev/null; check "series of a child's datastream" "$(st)" 200
check "points" "$(jx 'j.points.length + " " + j.unit')" "7 C"
req GET "/api/v1/assets/$FRIDGE/telemetry/series?datastream_id=$RT&from=$FROM&to=$TO" >/dev/null; check "datastream of the parent refused" "$(st)" 404
req GET "/api/v1/assets/$ROOM/telemetry/series?datastream_id=$MT&from=$FROM&to=$TO" >/dev/null; check "datastream of a sibling refused" "$(st)" 404
req GET "/api/v1/assets/$ROOM/telemetry/series?from=$FROM&to=$TO" >/dev/null; check "datastream_id required" "$(st)" 400
req GET "/api/v1/assets/$ROOM/telemetry/values?datastream_id=$RT&from=$FROM&to=$TO&limit=5" >/dev/null; check "values" "$(st)" 200
check "five, newest first, a next page" "$(jx '[j.rows.length, j.rows[0].value, !!j.next_cursor].join()')" "5,21,true"
req GET "/api/v1/assets/$ROOM/telemetry/values?datastream_id=$RT&from=$FROM&to=$TO&limit=5&cursor=$(jx 'j.next_cursor')" >/dev/null; check "second page" "$(jx 'j.rows.length')" 2
req GET "/api/v1/assets/$ROOM/telemetry/export?datastream_id=$RT&from=$FROM&to=$TO&format=csv" >/dev/null; check "CSV export" "$(st)" 200
check "header and 7 rows" "$(wc -l < "$TMP/b" | tr -d ' ')" 8
head -1 "$TMP/b" | grep -q '^measured_at,received_at,value' && ok "CSV columns" || bad "CSV columns: $(head -1 "$TMP/b")"
curl -s -b "$JAR" -D "$TMP/h" -o "$TMP/x.xlsx" "$BASE/api/v1/assets/$ROOM/telemetry/export?datastream_id=$RT&from=$FROM&to=$TO&format=xlsx"
grep -qi 'spreadsheetml' "$TMP/h" && grep -qi "filename=\"Room_$SUF" "$TMP/h" && ok "XLSX export named after the asset" || bad "XLSX export: $(grep -i 'content-' "$TMP/h" | tr '\n' ' ')"
req GET "/api/v1/assets/00000000-0000-7000-8000-000000000000/telemetry" >/dev/null; check "telemetry of an unknown asset 404" "$(st)" 404

echo "== history"
req GET "/api/v1/assets/$FRIDGE/history" >/dev/null; check "history" "$(st)" 200
check "asset, datastream, alarm rule and assignment entries" "$(jx "['asset.create','datastream.create','alarm_rule.create','assignment.create'].every(a => j.entries.some(e => e.action === a))")" true
req GET "/api/v1/assets/$ROOM/history" >/dev/null; check "the export is in the room's history" "$(jx "j.entries.some(e => e.action === 'export.read' && e.new_value.export === 'asset_telemetry' && e.new_value.datastream_id === '$RT')")" true
check "moves of the meter are not in the room's history" "$(jx "j.entries.every(e => e.entity_id !== '$METER')")" true
req GET "/api/v1/assets/$METER/history?limit=2" >/dev/null; check "paged" "$(jx '[j.entries.length, !!j.next_before].join()')" "2,true"
check "newest first: the move back" "$(jx 'j.entries[0].reason')" "e2e back"
req GET "/api/v1/assets/$METER/history?limit=2&before=$(jx 'j.next_before')" >/dev/null; check "older page" "$(jx 'j.entries[0].reason')" "e2e"
req DELETE "/api/v1/devices/$DEV/datastreams/$FT?reason=e2e%20unassign" >/dev/null; check "unassign" "$(st)" 200
req GET "/api/v1/assets/$FRIDGE/history" >/dev/null; check "the closed assignment is in the history" "$(jx "j.entries.some(e => e.action === 'assignment.close' && e.reason === 'e2e unassign')")" true
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$FT\",\"reason\":\"e2e\"}" >/dev/null

echo "== site page"
req GET "/api/v1/sites/$SITE" >/dev/null; check "site" "$(st)" 200
check "site with position, assets, devices, alarms" "$(jx "[j.site.location.lat, j.assets.length, j.datastreams, j.devices.length, j.alarms.length, j.alarms[0].asset_id === '$FRIDGE'].join()")" "48.3076,4,3,1,1,true"
req GET "/api/v1/sites/00000000-0000-7000-8000-000000000000" >/dev/null; check "unknown site 404" "$(st)" 404

echo "== alarm rules by asset, assets of a device"
req GET "/api/v1/alarm-rules?asset_id=$FRIDGE" >/dev/null; check "rules of the fridge" "$(jx "[j.length, j[0].asset_id].join()")" "1,$FRIDGE"
req GET "/api/v1/alarm-rules?asset_id=$ROOM" >/dev/null; check "none on the room" "$(jx 'j.length')" 0
req GET "/api/v1/alarm-rules?asset_id=bad" >/dev/null; check "invalid asset_id refused" "$(st)" 400
req GET /api/v1/devices >/dev/null; check "device lists the assets it feeds" "$(jx "j.find(d => d.id === '$DEV').assets.map(a => a.id).sort().join()")" "$(printf '%s\n' "$ROOM" "$FRIDGE" | sort | tr '\n' ',' | sed 's/,$//')"
req GET "/api/v1/devices/$DEV" >/dev/null; check "device detail: asset ids of its datastreams" "$(jx "j.datastreams.every(d => d.asset_id)")" true

echo "== permissions"
TK=$(req POST /api/v1/tokens '{"name":"asset page e2e","scopes":["data.read"]}' | jget token); check "token with data.read only" "$(st)" 201
tok "$TK" "/api/v1/assets/$ROOM" >/dev/null; check "data.read reads the asset" "$(st)" 200
tok "$TK" "/api/v1/assets/$ROOM/telemetry" >/dev/null; check "data.read reads its telemetry" "$(st)" 200
tok "$TK" "/api/v1/assets/$ROOM/history" >/dev/null; check "history needs audit.read" "$(st)" 403
tok "$TK" "/api/v1/assets/$ROOM/telemetry/export?datastream_id=$RT" >/dev/null; check "export needs data.export" "$(st)" 403
curl -s -o /dev/null -w '%{http_code}' -X PATCH -H "Authorization: Bearer $TK" -H "Content-Type: application/json" -d '{"location":{"lat":1,"lon":1},"reason":"x"}' "$BASE/api/v1/assets/$ROOM" > "$TMP/s"; check "a position needs config.write" "$(st)" 403
req GET /api/v1/tokens >/dev/null; TID=$(jx "j.find(t => t.name === 'asset page e2e' && !t.revoked_at).id"); req DELETE "/api/v1/tokens/$TID" >/dev/null
req PATCH "/api/v1/assets/$ROOM" '{"location":{"lat":1,"lon":1}}' >/dev/null; check "reason required" "$(st)" 400

echo "== pages"
for p in "/assets" "/assets/sites" "/assets/rules" "/assets/automation" "/assets/$ROOM" "/assets/$ROOM/telemetry" "/assets/$ROOM/history" "/assets/sites/$SITE"; do
  req GET "$p" >/dev/null; check "page $p" "$(st)" 200; done
req GET "/assets/$ROOM" >/dev/null; grep -q 'telemetry.js' "$TMP/b" && grep -q 'id="assetView"' "$TMP/b" && ok "asset page served" || bad "asset page content"
req GET "/assets/sites" >/dev/null; grep -q 'id="siteTable"' "$TMP/b" && ok "sites list served by the assets screen" || bad "sites list content"

echo "== audit entries"
req GET "/api/v1/export/audit?format=csv" >/dev/null
grep -q "site.update" "$TMP/b" && grep -q '"location"' "$TMP/b" && ok "audit: site position" || bad "audit site position"
grep -q '"parent_id"' "$TMP/b" && ok "audit: asset moved (parent_id before and after)" || bad "audit parent_id"

echo; echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

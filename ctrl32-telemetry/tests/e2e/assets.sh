#!/usr/bin/env bash
# End-to-end checks for the data model configuration: sites, assets, datastreams, device assignments, alarm rules.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/assets.sh
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
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  cat "$TMP/b"; }
SUF=$RANDOM

echo "== pages"
for p in /devices /assets; do req GET $p >/dev/null; check "page $p" "$(st)" 200; done
req GET /api/v1/sites >/dev/null; [[ "$(st)" == "200" || "$(st)" == "401" ]] && ok "sites list reachable ($(st))" || bad "sites list"

echo "== login"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

echo "== sites and assets"
S=$(req POST /api/v1/sites "{\"name\":\"Site $SUF\",\"tz\":\"Europe/Bratislava\",\"address\":\"Main 1\",\"reason\":\"e2e\"}"); check "create site" "$(st)" 201
SID=$(echo "$S" | jget id); [[ -n "$SID" ]] && ok "site id" || bad "site: $S"
req POST /api/v1/sites "{\"name\":\"Site $SUF\",\"reason\":\"e2e\"}" >/dev/null; check "duplicate site rejected" "$(st)" 409
req POST /api/v1/sites "{\"name\":\"Site x$SUF\"}" >/dev/null; check "reason required" "$(st)" 400
req PATCH "/api/v1/sites/$SID" '{"address":"Main 2","reason":"e2e"}' >/dev/null; check "patch site" "$(st)" 200
grep -q '"address":"Main 2"' "$TMP/b" && ok "address updated" || bad "address"
A=$(req POST /api/v1/assets "{\"site_id\":\"$SID\",\"type\":\"Fridge\",\"name\":\"Fridge $SUF\",\"reason\":\"e2e\"}"); check "create asset" "$(st)" 201
AID=$(echo "$A" | jget id); [[ -n "$AID" ]] && ok "asset id" || bad "asset: $A"
grep -q '"type":"fridge"' "$TMP/b" && ok "type normalised to lowercase" || bad "type"
req GET "/api/v1/assets?site_id=$SID" >/dev/null; grep -q "$AID" "$TMP/b" && ok "asset listed for site" || bad "asset list"
req PATCH "/api/v1/assets/$AID" '{"attributes":{"storage_class":"2-8"},"reason":"e2e"}' >/dev/null; check "patch asset attributes" "$(st)" 200

echo "== datastreams"
D=$(req POST /api/v1/datastreams "{\"asset_id\":\"$AID\",\"key\":\"temp\",\"quantity\":\"temperature\",\"unit\":\"\u00b0C\",\"kind\":\"gauge\",\"expected_interval\":\"5 minutes\",\"accuracy\":0.5,\"reason\":\"e2e\"}"); check "create datastream" "$(st)" 201
printf '{"asset_id":"%s","key":"bad","quantity":"temperature","unit":"\xb0C","kind":"gauge","reason":"e2e"}' "$AID" | curl -s -b "$JAR" -c "$JAR" -X POST -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @- "$BASE/api/v1/datastreams" > "$TMP/s"; check "invalid UTF-8 body rejected (records keep their characters)" "$(st)" 400
DSID=$(echo "$D" | jget id); [[ -n "$DSID" ]] && ok "datastream id" || bad "ds: $D"
req POST /api/v1/datastreams "{\"asset_id\":\"$AID\",\"key\":\"Temp Bad\",\"quantity\":\"temperature\",\"reason\":\"e2e\"}" >/dev/null; check "bad key rejected" "$(st)" 400
req POST /api/v1/datastreams "{\"asset_id\":\"$AID\",\"key\":\"temp\",\"quantity\":\"temperature\",\"reason\":\"e2e\"}" >/dev/null; check "duplicate key rejected" "$(st)" 409
req POST /api/v1/datastreams "{\"asset_id\":\"$AID\",\"key\":\"rh\",\"quantity\":\"humidity\",\"unit\":\"%\",\"expected_interval\":\"not an interval\",\"reason\":\"e2e\"}" >/dev/null; check "invalid interval rejected" "$(st)" 409
req GET "/api/v1/datastreams/$DSID" >/dev/null; check "datastream detail" "$(st)" 200
grep -q '"expected_interval":"00:05:00"' "$TMP/b" && ok "interval stored" || bad "interval: $(cat "$TMP/b" | head -c 300)"
req PATCH "/api/v1/datastreams/$DSID" '{"unit":"K","expected_interval":"10 minutes","reason":"e2e"}' >/dev/null; check "patch datastream" "$(st)" 200
grep -q '"unit":"K"' "$TMP/b" && ok "unit updated" || bad "unit"
req GET /api/v1/datastreams >/dev/null; grep -q "\"asset_id\":\"$AID\"" "$TMP/b" && ok "list carries asset_id" || bad "asset_id missing in list"

echo "== devices and assignments"
DV1=$(req POST /api/v1/devices "{\"external_id\":\"e2e-dev-a-$SUF\",\"name\":\"A\",\"reason\":\"e2e\"}" | jget id); [[ -n "$DV1" ]] && ok "device A" || bad "device A"
DV2=$(req POST /api/v1/devices "{\"external_id\":\"e2e-dev-b-$SUF\",\"name\":\"B\",\"reason\":\"e2e\"}" | jget id); [[ -n "$DV2" ]] && ok "device B" || bad "device B"
req POST "/api/v1/devices/$DV1/datastreams" "{\"datastream_id\":\"$DSID\",\"reason\":\"e2e\"}" >/dev/null; check "assign to A" "$(st)" 201
grep -q '"closed_previous":0' "$TMP/b" && ok "nothing closed on first assignment" || bad "closed_previous: $(cat "$TMP/b")"
req GET "/api/v1/devices/$DV1" >/dev/null; check "device detail" "$(st)" 200
grep -q '"key":"temp"' "$TMP/b" && ok "assignment visible on device" || bad "assignment missing"
req GET "/api/v1/datastreams/$DSID" >/dev/null; grep -q "e2e-dev-a-$SUF" "$TMP/b" && ok "source device visible on datastream" || bad "source device"
req POST "/api/v1/devices/$DV2/datastreams" "{\"datastream_id\":\"$DSID\",\"reason\":\"moved sensor\"}" >/dev/null; check "reassign to B" "$(st)" 201
grep -q '"closed_previous":1' "$TMP/b" && ok "previous source closed" || bad "closed_previous: $(cat "$TMP/b")"
req GET "/api/v1/devices/$DV1" >/dev/null; grep -q '"key":"temp"' "$TMP/b" && bad "A still has the datastream" || ok "A no longer feeds it"
req DELETE "/api/v1/devices/$DV2/datastreams/$DSID?reason=e2e" >/dev/null; check "unassign" "$(st)" 200
req DELETE "/api/v1/devices/$DV2/datastreams/$DSID?reason=e2e" >/dev/null; check "unassign twice -> 404" "$(st)" 404
req PATCH "/api/v1/devices/$DV1" '{"name":"A renamed","reason":"e2e"}' >/dev/null; check "rename device" "$(st)" 200
req POST "/api/v1/devices/$DV1/revoke" '{"reason":"e2e cleanup"}' >/dev/null; check "revoke A" "$(st)" 200
req POST "/api/v1/devices/$DV1/datastreams" "{\"datastream_id\":\"$DSID\",\"reason\":\"e2e\"}" >/dev/null; check "revoked device cannot be assigned" "$(st)" 404
req POST "/api/v1/devices/$DV2/revoke" '{"reason":"e2e cleanup"}' >/dev/null; check "revoke B" "$(st)" 200

echo "== alarm rules"
R1=$(req POST /api/v1/alarm-rules "{\"datastream_id\":\"$DSID\",\"type\":\"high\",\"warning\":7.5,\"action\":8,\"delay_seconds\":60,\"hysteresis\":0.2,\"reason\":\"e2e v1\"}"); check "rule v1" "$(st)" 201
echo "$R1" | grep -q '"version":1' && ok "version 1" || bad "v1: $R1"
R1ID=$(echo "$R1" | jget id)
R2=$(req POST /api/v1/alarm-rules "{\"datastream_id\":\"$DSID\",\"type\":\"high\",\"warning\":7,\"action\":8,\"reason\":\"e2e v2\"}"); check "rule v2" "$(st)" 201
echo "$R2" | grep -q '"version":2' && ok "version 2" || bad "v2: $R2"
echo "$R2" | grep -q "\"previous\":\"$R1ID\"" && ok "v1 closed by v2" || bad "previous: $R2"
req GET "/api/v1/alarm-rules?datastream_id=$DSID" >/dev/null; check "list rules" "$(st)" 200
[[ "$(grep -o '"type":"high"' "$TMP/b" | wc -l)" == "1" ]] && ok "only the current version is listed" || bad "rule list: $(cat "$TMP/b")"
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$DSID\",\"type\":\"high\",\"reason\":\"e2e\"}" >/dev/null; check "high without limits rejected" "$(st)" 400
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$DSID\",\"type\":\"weird\",\"reason\":\"e2e\"}" >/dev/null; check "unknown type rejected" "$(st)" 400
R2ID=$(echo "$R2" | jget id)
req DELETE "/api/v1/alarm-rules/$R2ID?reason=e2e" >/dev/null; check "disable rule" "$(st)" 200
req DELETE "/api/v1/alarm-rules/$R2ID?reason=e2e" >/dev/null; check "disable twice -> 404" "$(st)" 404
req GET "/api/v1/alarm-rules?datastream_id=$DSID" >/dev/null; grep -q '"type":"high"' "$TMP/b" && bad "rule still listed" || ok "no current rule after disable"

echo "== list rows and the hierarchy (0.74.0)"
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
CH=$(req POST /api/v1/assets "{\"site_id\":\"$SID\",\"parent_id\":\"$AID\",\"type\":\"shelf\",\"name\":\"Shelf $SUF\",\"reason\":\"e2e\"}" | jget id); check "child asset" "$(st)" 201
req GET "/api/v1/assets?site_id=$SID" >/dev/null; check "list of the site" "$(st)" 200
check "fridge row: datastreams, devices, children, alarm" "$(jx "(a => [a.datastreams, a.devices, a.children, a.alarm].join())(j.find(a => a.id === '$AID'))")" "1,0,1,"
check "child row names its parent" "$(jx "j.find(a => a.id === '$CH').parent_id")" "$AID"
req GET "/api/v1/sites" >/dev/null; check "site row: assets, datastreams, devices" "$(jx "(s => [s.assets, s.datastreams, s.devices].join())(j.find(s => s.id === '$SID'))")" "2,1,0"
req GET "/api/v1/assets/$CH" >/dev/null; check "asset page data" "$(st)" 200
check "its ancestor" "$(jx 'j.path.map(p => p.id).join()')" "$AID"
req PATCH "/api/v1/assets/$AID" "{\"parent_id\":\"$CH\",\"reason\":\"e2e\"}" >/dev/null; check "no cycles" "$(st)" 400
req PATCH "/api/v1/assets/$CH" '{"parent_id":"","reason":"e2e top level"}' >/dev/null; check "child moved to the top level" "$(st)" 200
req GET "/api/v1/sites/$SID" >/dev/null; check "site page data" "$(st)" 200
check "site page lists its assets" "$(jx 'j.assets.length')" 2

echo "== audit entries"
req GET "/api/v1/export/audit?format=csv" >/dev/null
for a in site.create site.update asset.create asset.update datastream.create datastream.update assignment.create assignment.close alarm_rule.create alarm_rule.close device.update; do grep -q "$a" "$TMP/b" && ok "audit $a" || bad "audit $a missing"; done

echo; echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

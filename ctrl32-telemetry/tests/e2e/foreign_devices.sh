#!/usr/bin/env bash
# Check of the paths a foreign (third-party) device uses: the device HTTP API with a Bearer device token (telemetry in
# the ctrl32, ThingsBoard and flat shapes, key normalisation, unknown keys, status, attributes, shared attributes,
# commands with long polling and acknowledgement) and plain MQTT with tools/mqttpub (flat payload, ThingsBoard alias
# topic, command delivery, refused credentials). Needs Go (tools/mqttpub) and the local broker on 127.0.0.1:1883.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/foreign_devices.sh
set -u
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
jget() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$1" "$2"; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  cat "$TMP/b"; }
# device call: dev METHOD PATH [BODY] [TOKEN]
dev()  { local m=$1 p=$2 d=${3:-} t=${4:-$TOK}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/dreq"; curl -s -X "$m" -H "Authorization: Bearer $t" -H "Content-Type: application/json" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/dreq" "$BASE$p" > "$TMP/s"
  else curl -s -X "$m" -H "Authorization: Bearer $t" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  cat "$TMP/b"; }
obs_count() { req GET "/api/v1/observations?datastream_id=$1&limit=50" >/dev/null; jx "$TMP/b" '(Array.isArray(j)?j:(j.items||j.observations||[])).length'; }
SUF=$RANDOM; NOW=$(date +%s)

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token)
SITE=$(req POST /api/v1/sites "{\"name\":\"Foreign site $SUF\",\"reason\":\"e2e\"}" | jget id)
ASSET=$(req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"gateway\",\"name\":\"Foreign gateway $SUF\",\"reason\":\"e2e\"}" | jget id); [[ -n "$ASSET" ]] && ok "asset" || bad "asset"
DS_T=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"temp\",\"quantity\":\"temperature\",\"unit\":\"\\u00b0C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" | jget id)
DS_RH=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"rh\",\"quantity\":\"humidity\",\"unit\":\"%\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" | jget id)
DS_D=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"door\",\"quantity\":\"state\",\"unit\":\"\",\"kind\":\"state\",\"reason\":\"e2e\"}" | jget id)
DS_T1=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"temp_1\",\"quantity\":\"temperature\",\"unit\":\"\\u00b0C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" | jget id)
[[ -n "$DS_T" && -n "$DS_RH" && -n "$DS_D" && -n "$DS_T1" ]] && ok "datastreams temp, rh, door, temp_1" || bad "datastreams"
DEV=$(req POST /api/v1/devices "{\"external_id\":\"http-$SUF\",\"name\":\"HTTP device $SUF\",\"transport\":\"http\",\"reason\":\"e2e\"}" | jget id); check "http device" "$(st)" 201
TOK=$(req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}' | jget token); [[ -n "$TOK" ]] && ok "token" || bad "token"
for D in $DS_T $DS_RH $DS_D $DS_T1; do req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$D\",\"reason\":\"e2e\"}" >/dev/null; done; check "assignments" "$(st)" 201

echo "== HTTP telemetry: authentication"
curl -s -o "$TMP/b" -w '%{http_code}' -X POST -d '{"temp":1}' "$BASE/api/v1/d/telemetry" > "$TMP/s"; check "no token → 401" "$(st)" 401
dev POST /api/v1/d/telemetry '{"temp":1}' "wrong-token" >/dev/null; check "wrong token → 401" "$(st)" 401

echo "== HTTP telemetry: accepted shapes"
dev POST /api/v1/d/telemetry "{\"ts\":$NOW,\"v\":{\"temp\":4.2,\"rh\":61}}" >/dev/null; check "ctrl32 shape → 202" "$(st)" 202
check "accepted 2" "$(jx "$TMP/b" 'j.accepted+" "+j.status')" "2 ok"
dev POST /api/v1/d/telemetry '{"temp":4.3,"rh":60,"door":true}' >/dev/null; check "flat shape" "$(jx "$TMP/b" 'j.accepted')" 3
dev POST /api/v1/d/telemetry "{\"ts\":${NOW}123,\"values\":{\"temp\":4.4}}" >/dev/null; check "ThingsBoard shape (ts in ms)" "$(jx "$TMP/b" 'j.accepted')" 1
dev POST /api/v1/d/telemetry '{"Temp-1":7.5}' >/dev/null; check "key normalised Temp-1 → temp_1" "$(jx "$TMP/b" 'j.accepted')" 1
dev POST /api/v1/d/telemetry "[{\"ts\":$((NOW-120)),\"temp\":4.0,\"bf\":1},{\"ts\":$((NOW-60)),\"temp\":4.1,\"bf\":1}]" >/dev/null; check "batch with backfill" "$(jx "$TMP/b" 'j.accepted')" 2
dev POST /api/v1/d/telemetry '{"temp":4.5,"pressure":1000}' >/dev/null; check "unknown key → 202 partial" "$(st)" 202
check "unknown key reported" "$(jx "$TMP/b" 'j.status+" "+j.unknown_keys.join(",")+" "+j.accepted')" "unknown_key pressure 1"
dev POST /api/v1/d/telemetry '{temp' >/dev/null; check "invalid JSON → 400" "$(st)" 400
dev POST /api/v1/d/telemetry '{"1bad":1}' >/dev/null; check "unnormalisable key → 400" "$(st)" 400
sleep 1
N=$(obs_count "$DS_T"); [[ "$N" -ge 6 ]] && ok "temp observations stored ($N)" || bad "temp observations $N"
check "temp_1 observation" "$(obs_count "$DS_T1")" 1
req GET "/api/v1/observations?datastream_id=$DS_T&limit=50" >/dev/null; check "backfilled quality kept" "$(jx "$TMP/b" '(Array.isArray(j)?j:(j.items||[])).some(o=>o.quality==="backfilled")')" true

echo "== HTTP status, attributes, shared attributes"
dev POST /api/v1/d/status '{"online":true,"rssi":-60,"bat":3.7,"fw":"2.0.0"}' >/dev/null; check "status → 202" "$(st)" 202
req GET "/api/v1/devices/$DEV" >/dev/null; check "status merged into the device" "$(jx "$TMP/b" 'j.device.status.rssi')" -60
dev POST /api/v1/d/attributes '{"fw":"2.0.0","model":"gw-1"}' >/dev/null; check "attributes → 202" "$(st)" 202
dev POST /api/v1/d/attributes '[1,2]' >/dev/null; check "attributes must be an object" "$(st)" 400
req GET "/api/v1/devices/$DEV/attributes" >/dev/null; check "client attribute stored" "$(jx "$TMP/b" 'j.client.model')" gw-1
req PUT "/api/v1/devices/$DEV/attributes" '{"attributes":{"interval_s":60,"temp_hi":8},"reason":"e2e"}' >/dev/null; check "shared attributes set by admin" "$(st)" 200
dev GET /api/v1/d/attributes/shared >/dev/null; check "device reads shared attributes" "$(jx "$TMP/b" 'j.interval_s+" "+j.temp_hi')" "60 8"

echo "== HTTP commands: long polling and acknowledgement"
T0=$(date +%s); dev GET "/api/v1/d/cmd?wait=2" >/dev/null; T1=$(date +%s); check "no command → []" "$(jx "$TMP/b" 'j.length')" 0
[[ $((T1-T0)) -ge 2 ]] && ok "long poll waited $((T1-T0)) s" || bad "long poll returned early"
CID=$(req POST "/api/v1/devices/$DEV/commands" '{"method":"set_interval","args":{"s":30},"ttl_seconds":300,"reason":"e2e"}' | jget id); check "command created" "$(st)" 201
check "http device: queued, not published" "$(jx "$TMP/b" 'j.status')" queued
dev GET "/api/v1/d/cmd?wait=5" >/dev/null; check "device fetched it" "$(jx "$TMP/b" 'j.length+" "+j[0].m+" "+j[0].a.s+" "+(j[0].exp>0)')" "1 set_interval 30 true"
dev GET "/api/v1/d/cmd?wait=0" >/dev/null; check "fetched once only" "$(jx "$TMP/b" 'j.length')" 0
req GET "/api/v1/devices/$DEV/commands" >/dev/null; check "admin sees sent" "$(jx "$TMP/b" "(j.find(c=>c.id==='$CID')||{}).status")" sent
dev POST "/api/v1/d/cmd/$CID/ack" '{"ok":true,"r":{"s":30}}' >/dev/null; check "ack" "$(jx "$TMP/b" 'j.status')" acked
req GET "/api/v1/devices/$DEV/commands" >/dev/null; check "admin sees acked with result" "$(jx "$TMP/b" "(j.find(c=>c.id==='$CID')||{}).status+' '+((j.find(c=>c.id==='$CID')||{}).result||{}).s")" "acked 30"
dev POST "/api/v1/d/cmd/$CID/ack" '{"ok":true}' >/dev/null; check "second ack → 404" "$(st)" 404
dev POST "/api/v1/d/cmd/$CID/ack" '{"nope":1}' >/dev/null; check "bad ack body → 400" "$(st)" 400
CID2=$(req POST "/api/v1/devices/$DEV/commands" '{"method":"reboot","args":{},"ttl_seconds":300,"reason":"e2e"}' | jget id)
dev GET "/api/v1/d/cmd?wait=1" >/dev/null; dev POST "/api/v1/d/cmd/$CID2/ack" '{"ok":false,"err":"busy"}' >/dev/null; check "negative ack → failed" "$(jx "$TMP/b" 'j.status')" failed
req GET "/api/v1/devices/$DEV/messages?limit=50" >/dev/null; check "raw messages carry connector http" "$(jx "$TMP/b" '(Array.isArray(j)?j:(j.items||[])).some(m=>m.connector==="http")')" true

echo "== MQTT with a plain client (tools/mqttpub)"
DEV2=$(req POST /api/v1/devices "{\"external_id\":\"mq-$SUF\",\"name\":\"MQTT device $SUF\",\"reason\":\"e2e\"}" | jget id)
TOK2=$(req POST "/api/v1/devices/$DEV2/token" '{"reason":"e2e"}' | jget token)
for D in $DS_T $DS_RH; do req POST "/api/v1/devices/$DEV2/datastreams" "{\"datastream_id\":\"$D\",\"reason\":\"e2e\"}" >/dev/null; done
go build -o "$TMP/mqttpub.exe" ./tools/mqttpub 2>"$TMP/build.log" && ok "mqttpub built" || bad "mqttpub build: $(tail -n 2 "$TMP/build.log")"
"$TMP/mqttpub.exe" -addr "$MQTT" -user "mq-$SUF" -token "wrong" -topic "d/mq-$SUF/telemetry" -payload '{"temp":1}' >/dev/null 2>&1 && bad "wrong token accepted" || ok "wrong token refused"
B0=$(obs_count "$DS_T")
"$TMP/mqttpub.exe" -addr "$MQTT" -user "mq-$SUF" -token "$TOK2" -topic "d/mq-$SUF/telemetry" -payload '{"temp":5.5,"rh":50}' > "$TMP/p1.log" 2>&1 && ok "flat payload published" || bad "publish: $(cat "$TMP/p1.log")"
"$TMP/mqttpub.exe" -addr "$MQTT" -user "mq-$SUF" -token "$TOK2" -topic "v1/devices/me/telemetry" -payload "{\"ts\":${NOW}000,\"values\":{\"temp\":5.6}}" > "$TMP/p2.log" 2>&1 && ok "ThingsBoard alias topic published" || bad "alias: $(cat "$TMP/p2.log")"
"$TMP/mqttpub.exe" -addr "$MQTT" -user "mq-$SUF" -token "$TOK2" -topic "d/other-$SUF/telemetry" -payload '{"temp":9}' >/dev/null 2>&1; ok "foreign topic attempt did not crash the client"
sleep 1; B1=$(obs_count "$DS_T"); [[ $((B1-B0)) -eq 2 ]] && ok "two MQTT observations stored" || bad "observations $B0 → $B1"
req GET "/api/v1/devices/$DEV2/messages?limit=20" >/dev/null; check "acl-denied topic not stored as this device's message" "$(jx "$TMP/b" '(Array.isArray(j)?j:(j.items||[])).some(m=>(m.topic||"").includes("other-"))')" false
"$TMP/mqttpub.exe" -addr "$MQTT" -user "mq-$SUF" -token "$TOK2" -sub "d/mq-$SUF/cmd" -wait 6 > "$TMP/sub.log" 2>&1 &
SUBPID=$!; sleep 2
CID3=$(req POST "/api/v1/devices/$DEV2/commands" '{"method":"set_interval","args":{"s":45},"ttl_seconds":300,"reason":"e2e"}' | jget id); check "mqtt device: command sent" "$(jx "$TMP/b" 'j.status')" sent
wait $SUBPID; grep -q "\"m\":\"set_interval\"" "$TMP/sub.log" && ok "command delivered on d/{id}/cmd" || bad "command not seen: $(cat "$TMP/sub.log" | head -c 300)"
"$TMP/mqttpub.exe" -addr "$MQTT" -user "mq-$SUF" -token "$TOK2" -topic "d/mq-$SUF/cmd/ack" -payload "{\"id\":\"$CID3\",\"ok\":true,\"r\":{\"s\":45}}" >/dev/null 2>&1; sleep 1
req GET "/api/v1/devices/$DEV2/commands" >/dev/null; check "mqtt ack recorded" "$(jx "$TMP/b" "(j.find(c=>c.id==='$CID3')||{}).status")" acked

echo "== metrics"
curl -s -o "$TMP/b" "$BASE/metrics"; grep -q '^ctrl32_ingest_messages_total{status="unknown_key"} [1-9]' "$TMP/b" && ok "unknown_key counted in metrics" || bad "metrics unknown_key"

echo
echo "foreign_devices: $pass passed, $fail failed"
[[ $fail -eq 0 ]]

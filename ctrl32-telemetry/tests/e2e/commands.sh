#!/usr/bin/env bash
# End-to-end checks for device commands with acknowledgement, shared attributes, the ChirpStack connector and the
# organization export. Needs Go for the ack simulator (tools/devcmd) and a running server.
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 USER=admin PASS=admin1234 bash tests/e2e/commands.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"; [[ -n "${SIM:-}" ]] && kill $SIM 2>/dev/null' EXIT
JAR="$TMP/jar"; CSRF=""
pass=0; fail=0
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jget() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -D "$TMP/h" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -D "$TMP/h" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  cat "$TMP/b"; }
SUF=$RANDOM

echo "== login"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

echo "== device with token and ack simulator"
DEV=$(req POST /api/v1/devices "{\"external_id\":\"cmd-$SUF\",\"name\":\"cmd test\",\"reason\":\"e2e\"}" | jget id); [[ -n "$DEV" ]] && ok "device" || bad "device"
TOK=$(req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}' | jget token); [[ -n "$TOK" ]] && ok "token" || bad "token"
go run ./tools/devcmd -addr "$MQTT" -user "cmd-$SUF" -token "$TOK" -duration 25s > "$TMP/sim.log" 2>&1 &
SIM=$!
sleep 4
grep -q "subscribed" "$TMP/sim.log" && ok "simulator connected" || bad "simulator: $(cat "$TMP/sim.log")"

echo "== commands"
C=$(req POST "/api/v1/devices/$DEV/commands" '{"method":"set_interval","args":{"s":60},"ttl_seconds":60,"reason":"e2e"}'); check "send command" "$(st)" 201
echo "$C" | grep -q '"status":"sent"' && ok "status sent (device online)" || bad "status: $C"
CID=$(echo "$C" | jget id)
sleep 3
req GET "/api/v1/devices/$DEV/commands" >/dev/null; check "list commands" "$(st)" 200
grep -q "\"id\":\"$CID\"[^}]*\"status\":\"acked\"" "$TMP/b" || grep -q '"status":"acked"' "$TMP/b" && ok "command acked by device" || bad "not acked: $(head -c 400 "$TMP/b")"
grep -q '"result":{"s":60}' "$TMP/b" && ok "ack result stored" || bad "result: $(head -c 300 "$TMP/b")"
req POST "/api/v1/devices/$DEV/commands" '{"method":"fail_me","args":{},"ttl_seconds":60,"reason":"e2e"}' >/dev/null; check "send failing command" "$(st)" 201
sleep 3
req GET "/api/v1/devices/$DEV/commands" >/dev/null; grep -q '"status":"failed"' "$TMP/b" && ok "failed ack recorded" || bad "failed status missing"
grep -q '"error":"unsupported"' "$TMP/b" && ok "error text stored" || bad "error text"
req POST "/api/v1/devices/$DEV/commands" '{"method":"Bad Method","reason":"e2e"}' >/dev/null; check "invalid method rejected" "$(st)" 400
req POST "/api/v1/devices/$DEV/commands" '{"method":"ping"}' >/dev/null; check "reason required" "$(st)" 400
E=$(req POST "/api/v1/devices/$DEV/commands" '{"method":"noack","args":{},"ttl_seconds":1,"reason":"e2e expiry"}'); EID=$(echo "$E" | jget id)
sleep 3
req GET "/api/v1/devices/$DEV/commands?limit=1" >/dev/null; grep -q "\"id\":\"$EID\"" "$TMP/b" && grep -q '"status":"expired"' "$TMP/b" && ok "unacked command expires" || bad "expiry: $(head -c 300 "$TMP/b")"

echo "== shared attributes"
req PUT "/api/v1/devices/$DEV/attributes" '{"attributes":{"interval_s":60,"temp_hi":8},"reason":"e2e"}' >/dev/null; check "set attributes" "$(st)" 200
grep -q '"published":true' "$TMP/b" && ok "published retained" || bad "published: $(cat "$TMP/b")"
req GET "/api/v1/devices/$DEV/attributes" >/dev/null; grep -q '"temp_hi":8' "$TMP/b" && ok "shared attributes stored" || bad "shared: $(cat "$TMP/b")"
sleep 2
grep -q 'shared' "$TMP/sim.log" && ok "device received shared attributes" || bad "device did not receive attributes: $(tail -3 "$TMP/sim.log")"
grep -q 'response' "$TMP/sim.log" && ok "attributes/request answered" || bad "no attributes response: $(tail -3 "$TMP/sim.log")"
req PUT "/api/v1/devices/$DEV/attributes" '{"remove":["temp_hi"],"reason":"e2e"}' >/dev/null; check "remove attribute" "$(st)" 200
grep -q 'temp_hi' "$TMP/b" && bad "attribute not removed" || ok "attribute removed"
req PUT "/api/v1/devices/$DEV/attributes" '{"attributes":{"a":1}}' >/dev/null; check "reason required for attributes" "$(st)" 400

echo "== chirpstack connector"
CN=$(req POST /api/v1/connectors "{\"kind\":\"chirpstack_http\",\"name\":\"cs-$SUF\",\"auto_create_devices\":true,\"reason\":\"e2e\"}"); check "create connector" "$(st)" 201
CNID=$(echo "$CN" | jget id); SECRET=$(echo "$CN" | jget secret); [[ -n "$CNID" && "$SECRET" == c32c_* ]] && ok "connector secret issued" || bad "connector: $CN"
EUI="70b3d5$(printf '%06x' $SUF)aa"
UP="{\"deduplicationId\":\"x\",\"time\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"deviceInfo\":{\"devEui\":\"$EUI\",\"deviceName\":\"lht65-$SUF\"},\"fCnt\":7,\"fPort\":2,\"data\":\"AQID\",\"object\":{\"temp\":21.5,\"rh\":48},\"rxInfo\":[{\"gatewayId\":\"gw1\",\"rssi\":-80,\"snr\":7.5}]}"
curl -s -o "$TMP/b" -w '%{http_code}' -H "Content-Type: application/json" -H "Authorization: Bearer $SECRET" --data "$UP" "$BASE/api/v1/ingest/chirpstack/$CNID?event=up" > "$TMP/s"; check "uplink accepted" "$(st)" 202
grep -q '"decoded_keys":2' "$TMP/b" && ok "object decoded (2 keys)" || bad "decoded: $(cat "$TMP/b")"
LDEV=$(req GET /api/v1/devices | grep -o "\"external_id\":\"$EUI\",\"id\":\"[^\"]*\"" | cut -d'"' -f8); [[ -n "$LDEV" ]] && ok "device auto-created by DevEUI" || bad "lorawan device missing"
SITE=$(req GET /api/v1/sites | jget id)
AID=$(req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"room\",\"name\":\"LoRa room $SUF\",\"reason\":\"e2e\"}" | jget id)
DSID=$(req POST /api/v1/datastreams "{\"asset_id\":\"$AID\",\"key\":\"temp\",\"quantity\":\"temperature\",\"unit\":\"\u00b0C\",\"reason\":\"e2e\"}" | jget id)
req POST "/api/v1/devices/$LDEV/datastreams" "{\"datastream_id\":\"$DSID\",\"reason\":\"e2e\"}" >/dev/null; check "assign lorawan datastream" "$(st)" 201
sleep 1
curl -s -o "$TMP/b" -w '%{http_code}' -H "Content-Type: application/json" -H "Authorization: Bearer $SECRET" --data "$UP" "$BASE/api/v1/ingest/chirpstack/$CNID?event=up" > "$TMP/s"; check "second uplink" "$(st)" 202
sleep 1
req GET "/api/v1/observations?datastream_id=$DSID&from=$(date -u -d '-1 hour' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u +%Y-%m-%dT00:00:00Z)" >/dev/null; grep -q '"value":21.5' "$TMP/b" && ok "observation stored from LoRaWAN uplink" || bad "observation: $(head -c 300 "$TMP/b")"
req GET "/api/v1/devices/$LDEV/messages?limit=5" >/dev/null; grep -q '"connector":"lorawan.chirpstack"' "$TMP/b" && ok "raw message tagged with connector" || bad "raw connector"
grep -q '"rssi":-80' "$TMP/b" && ok "rssi in meta" || bad "meta: $(head -c 300 "$TMP/b")"
curl -s -o "$TMP/b" -w '%{http_code}' -H "Content-Type: application/json" -H "Authorization: Bearer c32c_wrong" --data "$UP" "$BASE/api/v1/ingest/chirpstack/$CNID?event=up" > "$TMP/s"; check "wrong secret rejected" "$(st)" 401
curl -s -o "$TMP/b" -w '%{http_code}' -H "Content-Type: application/json" -H "Authorization: Bearer $SECRET" --data '{"deviceInfo":{"devEui":"'"$EUI"'"},"batteryLevel":87.5}' "$BASE/api/v1/ingest/chirpstack/$CNID?event=status" > "$TMP/s"; check "status event" "$(st)" 202
req GET "/api/v1/devices/$LDEV" >/dev/null; grep -q '"battery":87.5' "$TMP/b" && ok "battery in device status" || bad "status: $(head -c 300 "$TMP/b")"
req GET /api/v1/connectors >/dev/null; grep -q "\"id\":\"$CNID\"" "$TMP/b" && ok "connector listed" || bad "connector list"
grep -q "$SECRET" "$TMP/b" && bad "secret leaked in list" || ok "secret not in list"
req DELETE "/api/v1/connectors/$CNID?reason=e2e" >/dev/null; check "revoke connector" "$(st)" 200
curl -s -o "$TMP/b" -w '%{http_code}' -H "Content-Type: application/json" -H "Authorization: Bearer $SECRET" --data "$UP" "$BASE/api/v1/ingest/chirpstack/$CNID?event=up" > "$TMP/s"; check "revoked connector rejected" "$(st)" 401

echo "== organization export"
req GET "/api/v1/export/organization" >/dev/null; check "export zip" "$(st)" 200
head -c 2 "$TMP/b" | grep -q "PK" && ok "zip signature" || bad "not a zip"
grep -qi "application/zip" "$TMP/h" && ok "content type zip" || bad "content type"
[[ $(stat -c %s "$TMP/b") -gt 2000 ]] && ok "export has content ($(stat -c %s "$TMP/b") bytes)" || bad "export too small"
req GET "/api/v1/export/audit?format=csv" >/dev/null
for a in device.command device.attributes connector.create connector.revoke export.organization; do grep -q "$a" "$TMP/b" && ok "audit $a" || bad "audit $a missing"; done

echo "== cleanup"
echo "== commands over the device HTTP API are handed out once"
HDEV=$(req POST /api/v1/devices "{\"external_id\":\"cmdhttp-$SUF\",\"name\":\"cmd http\",\"transport\":\"http\",\"reason\":\"e2e\"}" | jget id)
HTOK=$(req POST "/api/v1/devices/$HDEV/token" '{"reason":"e2e"}' | jget token); [[ -n "$HTOK" ]] && ok "http device with token" || bad "http device"
for i in 1 2 3 4 5 6; do req POST "/api/v1/devices/$HDEV/commands" "{\"method\":\"step_$i\",\"args\":{\"n\":$i},\"ttl_seconds\":120,\"reason\":\"e2e\"}" >/dev/null; done
req GET "/api/v1/devices/$HDEV/commands" >/dev/null; check "six commands queued" "$(grep -o '"status":"queued"' "$TMP/b" | wc -l | tr -d ' ')" 6
for i in $(seq 12); do curl -s -H "Authorization: Bearer $HTOK" "$BASE/api/v1/d/cmd" > "$TMP/poll$i" & done; wait
cat "$TMP"/poll* | grep -o '"id":"[^"]*"' > "$TMP/handed"
check "twelve polls at once hand out six commands in total" "$(wc -l < "$TMP/handed" | tr -d ' ')" 6
check "no command is handed out twice" "$(sort -u "$TMP/handed" | wc -l | tr -d ' ')" 6
FIRST=$(for i in $(seq 12); do [[ $(grep -o '"id"' "$TMP/poll$i" | wc -l) -gt 1 ]] && { grep -o '"m":"[^"]*"' "$TMP/poll$i" | tr -d '\n'; break; }; done)
[[ -z "$FIRST" || "$FIRST" == $(echo "$FIRST" | grep -o '"m":"[^"]*"' | sort | tr -d '\n') ]] && ok "one poll gets its commands in the order they were issued" || bad "order: $FIRST"
check "nothing is left for the next poll" "$(curl -s -H "Authorization: Bearer $HTOK" "$BASE/api/v1/d/cmd")" "[]"
req GET "/api/v1/devices/$HDEV/commands" >/dev/null; check "all six are marked sent" "$(grep -o '"status":"sent"' "$TMP/b" | wc -l | tr -d ' ')" 6
req POST "/api/v1/devices/$HDEV/revoke" '{"reason":"e2e cleanup"}' >/dev/null

req POST "/api/v1/devices/$DEV/revoke" '{"reason":"e2e cleanup"}' >/dev/null; check "revoke test device" "$(st)" 200
req POST "/api/v1/devices/$LDEV/revoke" '{"reason":"e2e cleanup"}' >/dev/null; check "revoke lorawan device" "$(st)" 200

echo; echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

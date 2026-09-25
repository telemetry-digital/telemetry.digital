#!/usr/bin/env bash
# End-to-end checks for the remote console (protocol §3.5) and FOTA (§3.6) with the device simulator.
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 USER=admin PASS=admin1234 bash tests/e2e/console_fota.sh
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

echo "== login + device + simulator"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"
req GET /fota >/dev/null; check "fota page" "$(st)" 200
DEV=$(req POST /api/v1/devices "{\"external_id\":\"fota-$SUF\",\"name\":\"fota test\",\"reason\":\"e2e\"}" | jget id); [[ -n "$DEV" ]] && ok "device" || bad "device"
TOK=$(req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}' | jget token); [[ -n "$TOK" ]] && ok "token" || bad "token"
go run ./tools/devcmd -addr "$MQTT" -user "fota-$SUF" -token "$TOK" -duration 40s > "$TMP/sim.log" 2>&1 &
SIM=$!
sleep 4
grep -q "subscribed" "$TMP/sim.log" && ok "simulator connected" || bad "simulator: $(cat "$TMP/sim.log")"

echo "== console"
req GET "/api/v1/devices/$DEV/console" >/dev/null; check "console status" "$(st)" 200
grep -q '"open":null' "$TMP/b" && ok "no open session" || bad "status: $(cat "$TMP/b")"
S=$(req POST "/api/v1/devices/$DEV/console" '{}'); check "open session" "$(st)" 201
SID=$(echo "$S" | jget id); [[ -n "$SID" ]] && ok "session id" || bad "session: $S"
sleep 1
grep -q '"console.enabled": *true' "$TMP/sim.log" && ok "device received console.enabled=true" || bad "console.enabled missing: $(grep shared "$TMP/sim.log" | tail -2)"
req POST "/api/v1/devices/$DEV/console/$SID/in" '{"line":"wifi status"}' >/dev/null; check "send line" "$(st)" 200
sleep 2
req GET "/api/v1/devices/$DEV/console/$SID" >/dev/null; check "read lines" "$(st)" 200
grep -q '"direction":"in"' "$TMP/b" && grep -q '"line":"wifi status"' "$TMP/b" && ok "input recorded" || bad "input: $(cat "$TMP/b")"
grep -q '"line":"echo: wifi status"' "$TMP/b" && ok "device output received and recorded" || bad "output: $(head -c 400 "$TMP/b")"
LAST=$(sed 's/"session".*//' "$TMP/b" | grep -o '"id":"[^"]*"' | tail -1 | cut -d'"' -f4)
req GET "/api/v1/devices/$DEV/console/$SID?after=$LAST" >/dev/null; grep -q '"lines":\[\]' "$TMP/b" && ok "cursor works (no new lines)" || bad "cursor: $(cat "$TMP/b")"
req POST "/api/v1/devices/$DEV/console/$SID/in" '{"line":"bad\u0007line"}' >/dev/null; check "control chars stripped, still accepted" "$(st)" 200
grep -q '"line":"badline"' "$TMP/b" && ok "sanitized" || bad "sanitize: $(cat "$TMP/b")"
LIM=0; for i in 1 2 3 4 5 6 7; do req POST "/api/v1/devices/$DEV/console/$SID/in" "{\"line\":\"burst $i\"}" >/dev/null; [[ "$(st)" == "429" ]] && LIM=1; done; [[ $LIM == 1 ]] && ok "rate limit 5/s enforced" || bad "no rate limit"
S2=$(req POST "/api/v1/devices/$DEV/console" '{}'); check "reopen by same user resumes" "$(st)" 201
echo "$S2" | grep -q "\"id\":\"$SID\"" && ok "same session returned" || bad "different session"
req DELETE "/api/v1/devices/$DEV/console/$SID?reason=e2e" >/dev/null; check "close session" "$(st)" 200
sleep 1
grep -q '"console.enabled": *false' "$TMP/sim.log" && ok "device received console.enabled=false" || bad "console.enabled=false missing"
req DELETE "/api/v1/devices/$DEV/console/$SID?reason=e2e" >/dev/null; check "close twice -> 409" "$(st)" 409
req POST "/api/v1/devices/$DEV/console/$SID/in" '{"line":"x"}' >/dev/null; check "send after close refused" "$(st)" 409
req GET "/api/v1/devices/$DEV/console" >/dev/null; grep -q '"close_reason":"e2e"' "$TMP/b" && ok "session history" || bad "history: $(cat "$TMP/b")"

echo "== firmware"
head -c 120000 /dev/urandom > "$TMP/fw.bin"; SHA=$(sha256sum "$TMP/fw.bin" | cut -d' ' -f1)
curl -s -b "$JAR" -c "$JAR" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' -F "file=@$TMP/fw.bin" -F "name=sim-node" -F "version=1.$SUF.0" -F "hw=sim" -F "reason=e2e" "$BASE/api/v1/firmware" > "$TMP/s"; check "upload firmware" "$(st)" 201
FW=$(cat "$TMP/b" | jget id); grep -q "\"sha256\":\"$SHA\"" "$TMP/b" && ok "sha256 recorded" || bad "sha: $(cat "$TMP/b")"
curl -s -b "$JAR" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' -F "file=@$TMP/fw.bin" -F "name=sim-node" -F "version=1.$SUF.0" -F "hw=sim" -F "reason=e2e" "$BASE/api/v1/firmware" > "$TMP/s"; check "duplicate version rejected" "$(st)" 409
curl -s -b "$JAR" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' -F "file=@$TMP/fw.bin" -F "name=x" -F "version=bad version" -F "hw=sim" -F "reason=e2e" "$BASE/api/v1/firmware" > "$TMP/s"; check "bad version rejected" "$(st)" 400
req GET /api/v1/firmware >/dev/null; grep -q "\"id\":\"$FW\"" "$TMP/b" && ok "firmware listed" || bad "list"
curl -s -o "$TMP/b" -w '%{http_code}' "$BASE/api/v1/d/fota/$FW/blob" > "$TMP/s"; check "blob without token -> 401" "$(st)" 401
curl -s -o "$TMP/b" -w '%{http_code}' -H "Authorization: Bearer $TOK" "$BASE/api/v1/d/fota/$FW/blob" > "$TMP/s"; check "blob before campaign -> 401" "$(st)" 401

echo "== campaign"
C=$(req POST /api/v1/fota/campaigns "{\"firmware_id\":\"$FW\",\"device_ids\":[\"$DEV\"],\"reason\":\"e2e\"}"); check "create campaign" "$(st)" 201
CID=$(echo "$C" | jget id); echo "$C" | grep -q '"published":1' && ok "target published to device" || bad "campaign: $C"
sleep 6
req GET "/api/v1/fota/campaigns/$CID" >/dev/null; check "campaign detail" "$(st)" 200
grep -q '"status":"updated"' "$TMP/b" && ok "device reported updated" || bad "targets: $(head -c 500 "$TMP/b"); sim: $(tail -5 "$TMP/sim.log")"
grep -q "\"version\":\"1.$SUF.0\"" "$TMP/b" && ok "reported version recorded" || bad "version"
grep -q "\"device_fw\":\"1.$SUF.0\"" "$TMP/b" && ok "device status fw updated" || bad "device fw"
curl -s -o "$TMP/dl.bin" -w '%{http_code}' -H "Authorization: Bearer $TOK" "$BASE/api/v1/d/fota/$FW/blob" > "$TMP/s"; check "blob with token" "$(st)" 200
check "blob content matches" "$(sha256sum "$TMP/dl.bin" | cut -d' ' -f1)" "$SHA"
curl -s -o "$TMP/part.bin" -w '%{http_code}' -H "Authorization: Bearer $TOK" -H "Range: bytes=0-999" "$BASE/api/v1/d/fota/$FW/blob" > "$TMP/s"; check "range request -> 206" "$(st)" 206
[[ $(stat -c %s "$TMP/part.bin") == 1000 ]] && ok "range length 1000" || bad "range length $(stat -c %s "$TMP/part.bin")"
req GET /api/v1/fota/campaigns >/dev/null; grep -q '"updated":1' "$TMP/b" && ok "campaign counts" || bad "counts: $(head -c 300 "$TMP/b")"
C2=$(req POST /api/v1/fota/campaigns "{\"firmware_id\":\"$FW\",\"device_ids\":[\"$DEV\"],\"name\":\"cancel me\",\"reason\":\"e2e\"}"); CID2=$(echo "$C2" | jget id)
req DELETE "/api/v1/fota/campaigns/$CID2?reason=e2e" >/dev/null; check "cancel campaign" "$(st)" 200
req GET "/api/v1/fota/campaigns/$CID2" >/dev/null; grep -q '"status":"cancelled"\|"status":"updated"' "$TMP/b" && ok "targets closed" || bad "cancel targets: $(head -c 300 "$TMP/b")"
sleep 1
grep -q "fota target cleared" "$TMP/sim.log" && ok "retained target cleared on device" || bad "target not cleared"
req DELETE "/api/v1/fota/campaigns/$CID2?reason=e2e" >/dev/null; check "cancel twice -> 404" "$(st)" 404

echo "== audit"
req GET "/api/v1/export/audit?format=csv" >/dev/null
for a in console.open console.close firmware.upload fota.campaign_create fota.campaign_cancel; do grep -q "$a" "$TMP/b" && ok "audit $a" || bad "audit $a missing"; done
req POST "/api/v1/devices/$DEV/revoke" '{"reason":"e2e cleanup"}' >/dev/null; check "cleanup" "$(st)" 200

echo; echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

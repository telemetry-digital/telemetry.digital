#!/usr/bin/env bash
# End-to-end checks for live data over SSE (URS G-07): an observation published over MQTT arrives on
# GET /api/v1/stream for the session and on the public dashboard stream; device connect/disconnect events;
# unauthenticated and invalid requests are refused. Needs Go (tools/devfill) and Node — a developer machine.
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 USER=admin PASS=admin1234 bash tests/e2e/live.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
command -v go >/dev/null 2>&1 || export PATH="/d/tools/go/bin:$PATH"
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
  STATUS=$(cat "$TMP/s"); cat "$TMP/b"; }
SUF=$RANDOM

echo "== login and data"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"
SITE=$(req POST /api/v1/sites "{\"name\":\"Live site $SUF\",\"reason\":\"e2e\"}" | jget id); [[ -n "$SITE" ]] && ok "site" || bad "site"
ASSET=$(req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"machine\",\"name\":\"Live asset $SUF\",\"reason\":\"e2e\"}" | jget id); [[ -n "$ASSET" ]] && ok "asset" || bad "asset"
DS=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"t_c\",\"quantity\":\"temperature\",\"unit\":\"C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" | jget id); [[ -n "$DS" ]] && ok "datastream" || bad "datastream: $(head -c 160 "$TMP/b")"
DEV=$(req POST /api/v1/devices "{\"external_id\":\"live-$SUF\",\"name\":\"live test\",\"transport\":\"mqtt\",\"reason\":\"e2e\"}" | jget id); [[ -n "$DEV" ]] && ok "device" || bad "device"
TOK=$(req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}' | jget token); [[ -n "$TOK" ]] && ok "token" || bad "token"
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$DS\",\"reason\":\"e2e\"}" >/dev/null; check "assign t_c" "$(st)" 201

echo "== stream (session)"
curl -s -o "$TMP/anon" -w '%{http_code}' "$BASE/api/v1/stream?datastreams=$DS" > "$TMP/s"; [[ "$(st)" == "401" || "$(st)" == "403" ]] && ok "stream needs auth ($(st))" || bad "stream needs auth (got $(st))"
req GET "/api/v1/stream?datastreams=nonsense" >/dev/null; check "ids validated" "$(st)" 400
timeout 12 curl -s -N -b "$JAR" "$BASE/api/v1/stream?datastreams=$DS&devices=all&alarms=1" > "$TMP/stream" 2>/dev/null &
SP=$!
sleep 1.5
go run ./tools/devfill -addr "$MQTT" -user "live-$SUF" -token "$TOK" -key t_c -start 21 -step 0 -n 1 -every 1 > "$TMP/pub.log" 2>&1 && ok "published a sample" || bad "publish: $(tail -2 "$TMP/pub.log")"
sleep 3
kill $SP 2>/dev/null; wait $SP 2>/dev/null
grep -q "^event: hello" "$TMP/stream" && ok "hello event" || bad "hello: $(head -c 200 "$TMP/stream")"
grep -q "^event: observation" "$TMP/stream" && grep -q "\"datastream_id\":\"$DS\"" "$TMP/stream" && ok "observation event for the datastream" || bad "observation: $(head -c 300 "$TMP/stream")"
grep -q "\"value\":21" "$TMP/stream" && ok "value in event" || bad "value: $(grep observation -A1 "$TMP/stream" | head -3)"
grep -q "^event: device" "$TMP/stream" && grep -q "\"device_id\":\"$DEV\"" "$TMP/stream" && ok "device connect/disconnect events" || bad "device events: $(grep -c device "$TMP/stream")"
grep -q '"connected":true' "$TMP/stream" && grep -q '"connected":false' "$TMP/stream" && ok "connected true then false" || bad "connected flags"

echo "== stream (public dashboard)"
SPEC="{\"version\":1,\"kind\":\"dashboard\",\"layout\":{\"type\":\"grid\",\"row_height\":96},\"range\":\"6h\",\"refresh_s\":30,\"widgets\":[{\"id\":\"a\",\"type\":\"value\",\"geometry\":{\"x\":0,\"y\":0,\"w\":3,\"h\":1},\"bindings\":{\"datastreams\":[\"$DS\"]},\"params\":{\"title\":\"T\"}}]}"
SLUG=$(req POST /api/v1/dashboards "{\"name\":\"Live $SUF\",\"spec\":$SPEC,\"reason\":\"e2e\"}" | jget slug); [[ -n "$SLUG" ]] && ok "dashboard" || bad "dashboard: $(head -c 120 "$TMP/b")"
PTOKEN=$(req POST "/api/v1/dashboards/$SLUG/share" '{"reason":"publish"}' | jget token); [[ -n "$PTOKEN" ]] && ok "public link" || bad "public link"
timeout 10 curl -s -N "$BASE/api/v1/public/$PTOKEN/stream" > "$TMP/pstream" 2>/dev/null &
PP=$!
sleep 1.5
go run ./tools/devfill -addr "$MQTT" -user "live-$SUF" -token "$TOK" -key t_c -start 22 -step 0 -n 1 -every 1 > "$TMP/pub2.log" 2>&1
sleep 3
kill $PP 2>/dev/null; wait $PP 2>/dev/null
grep -q "^event: observation" "$TMP/pstream" && grep -q "\"value\":22" "$TMP/pstream" && ok "public stream delivers the bound datastream" || bad "public stream: $(head -c 300 "$TMP/pstream")"
grep -q "^event: device" "$TMP/pstream" && bad "public stream leaks device events (no device widget)" || ok "public stream has no device events"
curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/public/${PTOKEN}x/stream" > "$TMP/s"; check "bad public token 404" "$(st)" 404
req DELETE "/api/v1/dashboards/$SLUG/share?reason=e2e" >/dev/null; check "public link disabled" "$(st)" 200
req DELETE "/api/v1/dashboards/$SLUG?reason=e2e" >/dev/null; check "dashboard retired" "$(st)" 200
req POST "/api/v1/devices/$DEV/revoke" '{"reason":"e2e cleanup"}' >/dev/null; check "device revoked" "$(st)" 200

echo
echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

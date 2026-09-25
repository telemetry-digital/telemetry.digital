#!/usr/bin/env bash
# Check of `admin purge` (URS B-11: deletion only as an authorised, logged operation): a throwaway site with an
# asset, datastreams, a device with data, a dashboard and the purge of each — dry run first, then -yes; the API no
# longer finds them and the audit trail has data.purge entries. Runs the local binary against the local database.
#   BASE=http://127.0.0.1:8080 BIN=bin/ctrl32-telemetry.exe CONFIG=config.toml bash tests/e2e/purge.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
BIN=${BIN:-bin/ctrl32-telemetry.exe}
CONFIG=${CONFIG:-config.toml}
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
SUF=$RANDOM

echo "== throwaway data"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token)
SITE=$(req POST /api/v1/sites "{\"name\":\"Purge site $SUF\",\"reason\":\"e2e\"}" | jget id); check "site" "$(st)" 201
ASSET=$(req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"fridge\",\"name\":\"Purge fridge $SUF\",\"reason\":\"e2e\"}" | jget id)
DS=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"t_c\",\"quantity\":\"temperature\",\"unit\":\"C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" | jget id)
DEV=$(req POST /api/v1/devices "{\"external_id\":\"purge-$SUF\",\"name\":\"purge test\",\"reason\":\"e2e\"}" | jget id)
TOK=$(req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}' | jget token)
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$DS\",\"reason\":\"e2e\"}" >/dev/null; check "assign" "$(st)" 201
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$DS\",\"type\":\"high\",\"action\":30,\"reason\":\"e2e\"}" >/dev/null; check "alarm rule" "$(st)" 201
go run ./tools/devfill -addr "$MQTT" -user "purge-$SUF" -token "$TOK" -key t_c -start 35 -step 1 -n 5 -every 60 > "$TMP/pub.log" 2>&1 && ok "5 samples published (one above the limit)" || bad "publish: $(tail -n 2 "$TMP/pub.log")"
sleep 2
req GET "/api/v1/observations?datastream_id=$DS&limit=50" >/dev/null; check "observations stored" "$(jx "$TMP/b" '(Array.isArray(j)?j:(j.items||[])).length')" 5
req GET "/api/v1/dashboards/starter?kind=overview" >/dev/null; check "starter" "$(st)" 200
node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); process.stdout.write(JSON.stringify({name:"Purge dash "+process.argv[2], spec:j.spec, reason:"e2e purge"}))' "$TMP/b" "$SUF" > "$TMP/dash.json"
DASH=$(req POST /api/v1/dashboards "$(cat "$TMP/dash.json")" | jget slug); [[ -n "$DASH" ]] && ok "dashboard $DASH" || bad "dashboard create: $(head -c 200 "$TMP/b")"

echo "== dry run"
"$BIN" admin purge -config "$CONFIG" -site "Purge site $SUF" -device "purge-$SUF" -dashboard "$DASH" -reason "e2e dry run" > "$TMP/dry.log" 2>&1; check "dry run exits 0" "$?" 0
grep -q "dry run" "$TMP/dry.log" && ok "dry run announced" || bad "dry: $(cat "$TMP/dry.log")"
grep -q "observations=5" "$TMP/dry.log" && ok "counts shown (observations=5)" || bad "counts: $(cat "$TMP/dry.log")"
grep -q "alarm_rules=1" "$TMP/dry.log" && ok "alarm rule counted" || bad "alarm rule count: $(cat "$TMP/dry.log")"
grep -q "raw_messages=1" "$TMP/dry.log" && ok "raw message counted" || bad "raw messages: $(cat "$TMP/dry.log")"
req GET "/api/v1/observations?datastream_id=$DS&limit=50" >/dev/null; check "nothing deleted by the dry run" "$(jx "$TMP/b" '(Array.isArray(j)?j:(j.items||[])).length')" 5
"$BIN" admin purge -config "$CONFIG" -site "Purge site $SUF" -yes > "$TMP/r.log" 2>&1; check "reason required" "$?" 1

echo "== purge"
"$BIN" admin purge -config "$CONFIG" -site "Purge site $SUF" -device "purge-$SUF" -dashboard "$DASH" -reason "e2e purge of throwaway data" -yes > "$TMP/purge.log" 2>&1; check "purge exits 0" "$?" 0
grep -q "audit trail entries written" "$TMP/purge.log" && ok "audit entries written" || bad "purge: $(cat "$TMP/purge.log")"
req GET "/api/v1/observations?datastream_id=$DS&limit=50" >/dev/null; check "observations gone" "$(jx "$TMP/b" '(Array.isArray(j)?j:(j.items||[])).length')" 0
req GET "/api/v1/devices/$DEV" >/dev/null; check "device gone" "$(st)" 404
req GET /api/v1/sites >/dev/null; check "site gone" "$(jx "$TMP/b" "j.some(s=>s.id==='$SITE')")" false
req GET "/api/v1/dashboards/$DASH" >/dev/null; check "dashboard gone" "$(st)" 404
req GET /api/v1/export/audit >/dev/null; N=$(grep -c ",data.purge," "$TMP/b"); [[ "$N" -ge 3 ]] && ok "audit trail: $N data.purge entries" || bad "audit purge entries $N"
req GET /api/v1/audit/verify >/dev/null; check "audit chain still verifies" "$(jx "$TMP/b" 'j.ok')" true
"$BIN" admin purge -config "$CONFIG" -device "purge-$SUF" -reason "again" -yes > "$TMP/again.log" 2>&1; check "unknown device → error" "$?" 1

echo
echo "purge: $pass passed, $fail failed"
[[ $fail -eq 0 ]]

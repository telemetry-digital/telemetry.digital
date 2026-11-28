#!/usr/bin/env bash
# The delay of an alarm survives a restart of the server (0.98.1, audit). It stops and starts the development server,
# so it is not part of tests/run-all.sh. Needs Go (tools/devfill), Node and psql.
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 bash tests/restart/alarm_restart.sh
#   STOP="bash deploy/dev/dev.sh stop" START="bash deploy/dev/dev.sh restart" DB_ADMIN=postgres://…
set -u
cd "$(dirname "$0")/../.."
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
USER=${USER_NAME:-admin}
PASS=${PASS:-admin1234}
STOP=${STOP:-bash deploy/dev/dev.sh stop}
START=${START:-bash deploy/dev/dev.sh restart}
DB=${DB_ADMIN:-postgres://ctrl32:ctrl32-dev-only@127.0.0.1:5433/ctrl32_dev}
DELAY=${DELAY:-45}
SYSTMP=${TMP:-}; TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""
pass=0; fail=0
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1"; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
sql()  { psql -v ON_ERROR_STOP=1 -Atq "$DB" -c "$1" 2>&1; }
login(){ req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token); }
go build -o "$TMP/devfill" ./tools/devfill || { echo "devfill build failed"; exit 1; }
pub() { "$TMP/devfill" -addr "$MQTT" -user "ar-$SUF" -token "$TOK" -key "$1" -start "$2" -step 0 -n 1 -every 1 > "$TMP/pub.log" 2>&1 || bad "publish $1=$2: $(tail -1 "$TMP/pub.log")"; }
alarm() { req GET "/api/v1/alarms?datastream_id=$1"; jx 'j.length ? j[0].state + " " + j[0].severity + " " + (j[0].cleared_at ? "cleared" : "open") : "none"'; }
SUF=$RANDOM

echo "== setup"
login; check "login" "$(st)" 200
req POST /api/v1/sites "{\"name\":\"Restart site $SUF\",\"reason\":\"e2e\"}"; SITE=$(jx j.id)
req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"room\",\"name\":\"Restart room $SUF\",\"reason\":\"e2e\"}"; ASSET=$(jx j.id)
mkds() { req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"$1\",\"quantity\":\"state\",\"unit\":\"\",\"kind\":\"state\",\"reason\":\"e2e\"}"; jx j.id; }
STAYS=$(mkds stays); ENDS=$(mkds ends)
req POST /api/v1/devices "{\"external_id\":\"ar-$SUF\",\"name\":\"alarm restart\",\"reason\":\"e2e\"}"; DEV=$(jx j.id)
req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}'; TOK=$(jx j.token)
for d in $STAYS $ENDS; do req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$d\",\"reason\":\"e2e\"}"; done
for d in $STAYS $ENDS; do req POST /api/v1/alarm-rules "{\"datastream_id\":\"$d\",\"type\":\"door_open\",\"delay_seconds\":$DELAY,\"reason\":\"e2e restart\"}"; done; check "two rules with a delay of $DELAY s" "$(st)" 201
RULE=$(sql "SELECT id FROM core.alarm_rule WHERE datastream_id = '$STAYS' AND valid_to IS NULL")

echo "== the conditions begin"
pub stays 0; pub ends 0; sleep 1
T0=$(date +%s); pub stays 1; pub ends 1; sleep 2
check "no alarm yet" "$(alarm "$STAYS") $(alarm "$ENDS")" "none none"
check "both delays are stored" "$(sql "SELECT count(*) FROM core.alarm_pending WHERE datastream_id IN ('$STAYS', '$ENDS')")" "2"

echo "== the server stops; one condition ends meanwhile"
$STOP > "$TMP/stop.log" 2>&1
ORG=$(sql "SELECT org_id FROM core.datastream WHERE id = '$ENDS'")
sql "INSERT INTO core.observation (org_id, datastream_id, measured_at, value) VALUES ('$ORG', '$ENDS', now(), 0)" > /dev/null
# the server must not inherit the folder of this test as its temporary folder
TMP="$SYSTMP" $START > "$TMP/start.log" 2>&1
for _ in $(seq 60); do curl -s -m 2 "$BASE/healthz" | grep -q '"status":"ok"' && break; sleep 1; done
UP=$(date +%s); login; check "the server is back" "$(st)" 200
[[ $((UP - T0)) -lt $DELAY ]] && ok "back $((UP - T0)) s after the conditions began, before the delay of $DELAY s ran out" || bad "the restart took $((UP - T0)) s, longer than the delay: raise DELAY"
check "the delay whose condition ended was dropped" "$(sql "SELECT count(*) FROM core.alarm_pending WHERE datastream_id = '$ENDS'")" "0"
check "still no alarm" "$(alarm "$STAYS")" "none"

echo "== the delay runs out by the first clock, with no new value"
while [[ $(( $(date +%s) - T0 )) -lt $((DELAY + 4)) ]]; do sleep 1; done
check "the alarm of the lasting condition is raised" "$(alarm "$STAYS")" "active warning open"
check "the ended condition raised nothing" "$(alarm "$ENDS")" "none"
check "nothing is left waiting" "$(sql "SELECT count(*) FROM core.alarm_pending WHERE datastream_id IN ('$STAYS', '$ENDS')")" "0"

echo "== one open alarm per rule"
R=$(sql "INSERT INTO core.alarm (org_id, rule_id, datastream_id, severity) VALUES ('$ORG', '$RULE', '$STAYS', 'warning')")
[[ "$R" == *alarm_one_open_per_rule_idx* ]] && ok "the database refuses a second open alarm of the rule" || bad "a second open alarm was stored: $R"

echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

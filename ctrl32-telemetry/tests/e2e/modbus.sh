#!/usr/bin/env bash
# End-to-end check of the Modbus TCP connector (URS A-11) and writes as commands (URS A-12): starts the bundled
# simulator, validates settings, tests, creates a connector, waits for polls, writes a setpoint with read-back,
# refuses writes outside the limits and to read-only registers, then updates and revokes the connector.
# Needs Go (builds tools/modbussim) and Node (JSON extraction) — a developer machine, not the demo server.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/modbus.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
PORT=${SIM_PORT:-5020}
command -v go >/dev/null 2>&1 || export PATH="/d/tools/go/bin:$PATH"
TMP=$(mktemp -d); trap 'tail -3 "$TMP/sim.log" 2>/dev/null; rm -rf "$TMP"; taskkill //F //IM modbussim.exe >/dev/null 2>&1; pkill -f modbussim >/dev/null 2>&1; true' EXIT
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
  STATUS=$(cat "$TMP/s"); cat "$TMP/b"; }
SUF=$RANDOM

echo "== simulator"
EXE=bin/modbussim; [[ "$(uname -s)" == MINGW* || "$(uname -s)" == MSYS* ]] && EXE=bin/modbussim.exe
go build -o "$EXE" ./tools/modbussim && ok "simulator built" || { bad "simulator build"; exit 1; }
"$EXE" -port "$PORT" > "$TMP/sim.log" 2>&1 &
sleep 1.5
grep -q "listening" "$TMP/sim.log" && ok "simulator listening on $PORT" || { bad "simulator: $(cat "$TMP/sim.log")"; exit 1; }

echo "== login"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

echo "== validation"
req POST /api/v1/connectors "{\"kind\":\"modbus_tcp\",\"name\":\"bad-$SUF\",\"settings\":{\"host\":\"\",\"registers\":[]},\"reason\":\"e2e\"}" >/dev/null; check "host validated" "$(st)" 400
req POST /api/v1/connectors "{\"kind\":\"modbus_tcp\",\"name\":\"bad-$SUF\",\"settings\":{\"host\":\"127.0.0.1\",\"registers\":[{\"key\":\"Bad Key\",\"table\":\"holding\",\"address\":0}]},\"reason\":\"e2e\"}" >/dev/null; check "key validated" "$(st)" 400
req POST /api/v1/connectors "{\"kind\":\"modbus_tcp\",\"name\":\"bad-$SUF\",\"settings\":{\"host\":\"127.0.0.1\",\"registers\":[{\"key\":\"a\",\"table\":\"input\",\"address\":0,\"writable\":true}]},\"reason\":\"e2e\"}" >/dev/null; check "input registers cannot be writable" "$(st)" 400
req POST /api/v1/connectors "{\"kind\":\"modbus_tcp\",\"name\":\"bad-$SUF\",\"settings\":{\"host\":\"127.0.0.1\",\"registers\":[{\"key\":\"a\",\"table\":\"coil\",\"address\":0,\"type\":\"int16\"}]},\"reason\":\"e2e\"}" >/dev/null; check "coil type validated" "$(st)" 400
req POST /api/v1/connectors/test "{\"kind\":\"modbus_tcp\",\"settings\":{\"host\":\"127.0.0.1\",\"port\":1,\"registers\":[{\"key\":\"a\",\"table\":\"holding\",\"address\":0}]}}" >/dev/null; check "test against a dead port fails cleanly" "$(st)" 502

echo "== connector"
REGS='[{"key":"temp","table":"holding","address":0,"type":"int16","scale":0.1},{"key":"setpoint","table":"holding","address":1,"type":"int16","scale":0.1,"writable":true,"min":10,"max":40},{"key":"flow","table":"holding","address":2,"type":"float32"},{"key":"counter","table":"holding","address":4,"type":"uint16"},{"key":"state","table":"holding","address":5,"type":"int16"},{"key":"sinus","table":"input","address":0,"type":"int16","scale":0.01},{"key":"pump","table":"coil","address":0,"writable":true},{"key":"alarm","table":"coil","address":1},{"key":"door","table":"discrete","address":0}]'
SETTINGS="{\"host\":\"127.0.0.1\",\"port\":$PORT,\"unit_id\":1,\"interval_s\":2,\"registers\":$REGS}"
req POST /api/v1/connectors/test "{\"kind\":\"modbus_tcp\",\"settings\":$SETTINGS}" >/dev/null; check "test" "$(st)" 200
[[ "$(jx "$TMP/b" "j.values.setpoint")" == "25" ]] && ok "test reads setpoint 25.0 (scaled)" || bad "test values: $(head -c 200 "$TMP/b")"
[[ "$(jx "$TMP/b" "typeof j.values.flow")" == "number" && "$(jx "$TMP/b" "j.values.pump")" == "false" ]] && ok "float32 and coil decoded" || bad "decode: $(head -c 200 "$TMP/b")"
NAME="mb-$SUF"
CID=$(req POST /api/v1/connectors "{\"kind\":\"modbus_tcp\",\"name\":\"$NAME\",\"settings\":$SETTINGS,\"reason\":\"e2e\"}" | jget id); check "create" "$(st)" 201
sleep 5
req GET /api/v1/connectors >/dev/null
[[ "$(jx "$TMP/b" "(j.find(c=>c.id==='$CID')||{}).status.state")" == "connected" ]] && ok "status connected" || bad "status: $(jx "$TMP/b" "(j.find(c=>c.id==='$CID')||{}).status")"
[[ "$(jx "$TMP/b" "(j.find(c=>c.id==='$CID')||{}).status.polls >= 2")" == "true" ]] && ok "polls counted" || bad "polls"
req GET /api/v1/devices >/dev/null
DEV=$(jx "$TMP/b" "(j.find(d=>d.external_id==='modbus-$NAME')||{}).id"); [[ "$DEV" != "null" && -n "$DEV" ]] && ok "virtual device modbus-$NAME" || bad "virtual device"
[[ "$(jx "$TMP/b" "(j.find(d=>d.id==='$DEV')||{}).transport")" == "modbus" ]] && ok "transport modbus" || bad "transport"
req GET "/api/v1/devices/$DEV/messages?limit=5" >/dev/null; check "messages" "$(st)" 200
grep -q '"setpoint":25' "$TMP/b" && ok "poll payload stored (setpoint 25)" || bad "messages: $(head -c 200 "$TMP/b")"

echo "== writes as commands"
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"setpoint","value":31.5},"reason":"e2e setpoint"}' >/dev/null; check "write setpoint" "$(st)" 201
[[ "$(jx "$TMP/b" "j.status")" == "acked" ]] && ok "command acked" || bad "command: $(head -c 200 "$TMP/b")"
[[ "$(jx "$TMP/b" "j.result.readback")" == "31.5" ]] && ok "read back 31.5" || bad "readback: $(head -c 200 "$TMP/b")"
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"setpoint","value":99},"reason":"e2e"}' >/dev/null; check "value above max refused" "$(st)" 400
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"temp","value":1},"reason":"e2e"}' >/dev/null; check "read-only key refused" "$(st)" 400
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"nope","value":1},"reason":"e2e"}' >/dev/null; check "unknown key refused" "$(st)" 400
req POST "/api/v1/devices/$DEV/commands" '{"method":"reboot","args":{},"reason":"e2e"}' >/dev/null; check "other methods refused" "$(st)" 400
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"pump","value":true},"reason":"e2e pump on"}' >/dev/null; check "write coil" "$(st)" 201
[[ "$(jx "$TMP/b" "j.result.readback")" == "true" ]] && ok "coil read back true" || bad "coil: $(head -c 200 "$TMP/b")"
sleep 3
req GET /api/v1/connectors >/dev/null
[[ "$(jx "$TMP/b" "(j.find(c=>c.id==='$CID')||{}).status.last_values.setpoint")" == "31.5" ]] && ok "next poll sees the written setpoint" || bad "poll after write: $(jx "$TMP/b" "(j.find(c=>c.id==='$CID')||{}).status.last_values")"
[[ "$(jx "$TMP/b" "(j.find(c=>c.id==='$CID')||{}).status.writes")" == "2" ]] && ok "writes counted" || bad "writes"
req GET "/api/v1/devices/$DEV/commands" >/dev/null; check "command list" "$(st)" 200
grep -q '"readback":31.5' "$TMP/b" && ok "command result stored" || bad "command list: $(head -c 200 "$TMP/b")"
req GET "/api/v1/export/audit" >/dev/null; grep -q "device.command" "$TMP/b" && ok "write in the audit trail" || bad "audit"

echo "== update and revoke"
req PUT "/api/v1/connectors/$CID" "{\"settings\":$(echo "$SETTINGS" | sed 's/"interval_s":2/"interval_s":3/'),\"reason\":\"e2e\"}" >/dev/null; check "update" "$(st)" 200
req DELETE "/api/v1/connectors/$CID?reason=e2e" >/dev/null; check "revoke" "$(st)" 200
sleep 1
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"setpoint","value":20},"reason":"e2e"}' >/dev/null; check "write refused after revoke" "$(st)" 400

echo
echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

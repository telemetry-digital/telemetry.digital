#!/usr/bin/env bash
# End-to-end check of the Siemens S7 connector (0.83.0): starts the bundled S7 simulator (tools/s7sim), tests the
# settings, creates a connector, waits for polls, verifies the virtual devices (own + a routed one), writes as
# commands with limits and read back, the PUT/GET hint, update and revoke.
# Needs Go (builds tools/s7sim) and Node (JSON extraction) — a developer machine, not the demo server.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/s7.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
PORT=${SIM_PORT:-10102}
PORT2=$((PORT + 1))
command -v go >/dev/null 2>&1 || export PATH="/d/tools/go/bin:$PATH"
TMP=$(mktemp -d); trap 'tail -5 "$TMP/sim.log" 2>/dev/null; rm -rf "$TMP"; taskkill //F //IM s7sim.exe >/dev/null 2>&1; pkill -f s7sim >/dev/null 2>&1; true' EXIT
JAR="$TMP/jar"; CSRF=""
pass=0; fail=0
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jget() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$1" "$2" 2>/dev/null; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  STATUS=$(cat "$TMP/s"); cat "$TMP/b"; }
SUF=$RANDOM; NAME="plc-$SUF"; EXT="s7-sim-$SUF"; LINE="s7-line-$SUF"

echo "== simulator"
EXE=bin/s7sim; [[ "$(uname -s)" == MINGW* || "$(uname -s)" == MSYS* ]] && EXE=bin/s7sim.exe
go build -o "$EXE" ./tools/s7sim && ok "simulator built" || { bad "simulator build"; exit 1; }
"$EXE" -port "$PORT" -pdu 240 > "$TMP/sim.log" 2>&1 &
"$EXE" -port "$PORT2" -put-get-off > "$TMP/sim2.log" 2>&1 &
for i in $(seq 1 20); do grep -q "listening" "$TMP/sim.log" && grep -q "listening" "$TMP/sim2.log" && break; sleep 0.5; done
grep -q "listening" "$TMP/sim.log" && ok "simulator listening on $PORT (PDU 240)" || { bad "simulator: $(cat "$TMP/sim.log")"; exit 1; }

echo "== login"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

echo "== validation"
req POST /api/v1/connectors "{\"kind\":\"s7\",\"name\":\"bad-$SUF\",\"settings\":{\"host\":\"\",\"tags\":[]},\"reason\":\"e2e\"}" >/dev/null; check "host validated" "$(st)" 400
req POST /api/v1/connectors "{\"kind\":\"s7\",\"name\":\"bad-$SUF\",\"settings\":{\"host\":\"127.0.0.1\",\"tags\":[{\"key\":\"a\",\"address\":\"DB1.DBQ0\"}]},\"reason\":\"e2e\"}" >/dev/null; check "address validated" "$(st)" 400
grep -q "tag 1 (a)" "$TMP/b" && ok "error names the tag" || bad "error: $(head -c 200 "$TMP/b")"
req POST /api/v1/connectors "{\"kind\":\"s7\",\"name\":\"bad-$SUF\",\"settings\":{\"host\":\"127.0.0.1\",\"tags\":[{\"key\":\"a\",\"address\":\"DB1.DBW0\",\"type\":\"real\"}]},\"reason\":\"e2e\"}" >/dev/null; check "type of the wrong size refused" "$(st)" 400
req POST /api/v1/connectors "{\"kind\":\"s7\",\"name\":\"bad-$SUF\",\"settings\":{\"host\":\"127.0.0.1\",\"tags\":[{\"key\":\"a\",\"address\":\"I0.0\",\"writable\":true}]},\"reason\":\"e2e\"}" >/dev/null; check "inputs cannot be writable" "$(st)" 400
req POST /api/v1/connectors "{\"kind\":\"s7\",\"name\":\"bad-$SUF\",\"settings\":{\"host\":\"127.0.0.1\",\"plc\":\"s5\",\"tags\":[{\"key\":\"a\",\"address\":\"MW0\"}]},\"reason\":\"e2e\"}" >/dev/null; check "plc family validated" "$(st)" 400
req POST /api/v1/connectors/test "{\"kind\":\"s7\",\"settings\":{\"host\":\"127.0.0.1\",\"port\":1,\"timeout_ms\":500,\"tags\":[{\"key\":\"a\",\"address\":\"MW0\"}]}}" >/dev/null; check "test against a dead port fails cleanly" "$(st)" 502
req POST /api/v1/connectors/test "{\"kind\":\"s7\",\"settings\":{\"host\":\"127.0.0.1\",\"port\":$PORT2,\"tags\":[{\"key\":\"a\",\"address\":\"MW0\"}]}}" >/dev/null; check "PUT/GET disabled answers 502" "$(st)" 502
grep -q "PUT/GET" "$TMP/b" && ok "PUT/GET hint for TIA Portal" || bad "hint: $(head -c 200 "$TMP/b")"

echo "== test connection"
TAGS="[{\"key\":\"sinus\",\"address\":\"DB1.DBD0\"},{\"key\":\"counter\",\"address\":\"DB1,INT4\"},{\"key\":\"run\",\"address\":\"DB1.DBX6.0\",\"writable\":true},{\"key\":\"setpoint\",\"address\":\"DB1.DBD8\",\"writable\":true,\"min\":0,\"max\":100},{\"key\":\"txt\",\"address\":\"DB1.DBS20.20\",\"writable\":true},{\"key\":\"level\",\"address\":\"MW10\",\"scale\":0.1,\"device\":\"$LINE\"},{\"key\":\"small\",\"address\":\"MW12\",\"type\":\"int\",\"writable\":true,\"device\":\"$LINE\"},{\"key\":\"gone\",\"address\":\"DB99.DBW0\"}]"
SETTINGS="{\"host\":\"127.0.0.1\",\"port\":$PORT,\"plc\":\"s7-1200\",\"interval_s\":2,\"device_external_id\":\"$EXT\",\"tags\":$TAGS}"
req POST /api/v1/connectors/test "{\"kind\":\"s7\",\"settings\":$SETTINGS}" >/dev/null; check "test" "$(st)" 200
[[ "$(jx "$TMP/b" 'j.pdu')" == "240" ]] && ok "negotiated PDU 240" || bad "pdu: $(jx "$TMP/b" 'j.pdu')"
[[ "$(jx "$TMP/b" 'j.values.setpoint')" == "21.5" ]] && ok "real read (setpoint 21.5)" || bad "values: $(jx "$TMP/b" 'j.values')"
[[ "$(jx "$TMP/b" "j.values['$LINE/level']")" == "123.4" ]] && ok "scaled int on the routed device (MW10 1234 × 0.1)" || bad "level: $(jx "$TMP/b" 'j.values')"
[[ "$(jx "$TMP/b" 'j.values.txt')" == "hello" ]] && ok "string read" || bad "txt"
[[ "$(jx "$TMP/b" 'typeof j.values.run')" == "boolean" ]] && ok "bool read" || bad "run"
[[ "$(jx "$TMP/b" 'j.bad_nodes.length')" == "1" ]] && ok "missing DB reported, not fatal" || bad "bad_nodes: $(jx "$TMP/b" 'j.bad_nodes')"
[[ "$(jx "$TMP/b" '!!(j.cpu && (j.cpu.order_code || j.cpu.module_type))')" == "true" ]] && ok "CPU identified ($(jx "$TMP/b" 'j.cpu.module_type || j.cpu.order_code'))" || bad "cpu: $(jx "$TMP/b" 'j.cpu')"

echo "== create and poll"
CID=$(req POST /api/v1/connectors "{\"kind\":\"s7\",\"name\":\"$NAME\",\"settings\":$SETTINGS,\"reason\":\"e2e\"}" | jget id); check "create" "$(st)" 201
[[ "$(jx "$TMP/b" 'j.settings.slot')" == "1" && "$(jx "$TMP/b" 'j.settings.tags[0].type')" == "real" ]] && ok "defaults stored (slot 1, DBD → real)" || bad "settings: $(head -c 300 "$TMP/b")"
for i in $(seq 1 20); do sleep 1; req GET /api/v1/connectors >/dev/null; [[ "$(jx "$TMP/b" "((j.find(c=>c.id==='$CID')||{}).status||{}).polls >= 2")" == "true" ]] && break; done
[[ "$(jx "$TMP/b" "((j.find(c=>c.id==='$CID')||{}).status||{}).state")" == "connected" ]] && ok "status connected" || bad "status: $(jx "$TMP/b" "(j.find(c=>c.id==='$CID')||{}).status")"
[[ "$(jx "$TMP/b" "((j.find(c=>c.id==='$CID')||{}).status||{}).polls >= 2")" == "true" ]] && ok "polls counted" || bad "polls"
[[ "$(jx "$TMP/b" "((j.find(c=>c.id==='$CID')||{}).status||{}).pdu")" == "240" ]] && ok "PDU in status" || bad "pdu in status"
req GET /api/v1/devices >/dev/null
DEV=$(jx "$TMP/b" "(j.find(d=>d.external_id==='$EXT')||{}).id"); [[ "$DEV" != "null" && -n "$DEV" ]] && ok "virtual device $EXT" || bad "virtual device"
DEV2=$(jx "$TMP/b" "(j.find(d=>d.external_id==='$LINE')||{}).id"); [[ "$DEV2" != "null" && -n "$DEV2" ]] && ok "routed device $LINE" || bad "routed device"
[[ "$(jx "$TMP/b" "(j.find(d=>d.id==='$DEV')||{}).transport")" == "s7" ]] && ok "transport s7" || bad "transport"
req GET "/api/v1/devices/$DEV/messages?limit=5" >/dev/null; check "messages" "$(st)" 200
grep -q "s7/$NAME" "$TMP/b" && grep -q '"setpoint":21.5' "$TMP/b" && ok "poll payload stored" || bad "messages: $(head -c 300 "$TMP/b")"
req GET "/api/v1/devices/$DEV2/messages?limit=5" >/dev/null
grep -q '"level":123.4' "$TMP/b" && ! grep -q '"setpoint"' "$TMP/b" && ok "routed device gets only its own tags" || bad "routed messages: $(head -c 300 "$TMP/b")"

echo "== writes as commands"
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"setpoint","value":55.5},"reason":"e2e setpoint"}' >/dev/null; check "write real" "$(st)" 201
[[ "$(jx "$TMP/b" "j.status")" == "acked" && "$(jx "$TMP/b" "j.result.readback")" == "55.5" ]] && ok "acked, read back 55.5" || bad "command: $(head -c 300 "$TMP/b")"
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"run","value":true},"reason":"e2e start"}' >/dev/null; check "write bool" "$(st)" 201
[[ "$(jx "$TMP/b" "j.result.readback")" == "true" ]] && ok "bool read back" || bad "bool: $(head -c 200 "$TMP/b")"
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"txt","value":"ahoj"},"reason":"e2e text"}' >/dev/null; check "write string" "$(st)" 201
[[ "$(jx "$TMP/b" "j.result.readback")" == "ahoj" ]] && ok "string read back" || bad "string: $(head -c 200 "$TMP/b")"
req POST "/api/v1/devices/$DEV2/commands" '{"method":"write","args":{"key":"small","value":-7},"reason":"e2e routed"}' >/dev/null; check "write on the routed device" "$(st)" 201
[[ "$(jx "$TMP/b" "j.result.readback")" == "-7" ]] && ok "int read back on the routed device" || bad "routed write: $(head -c 200 "$TMP/b")"
req POST "/api/v1/devices/$DEV2/commands" '{"method":"write","args":{"key":"small","value":40000},"reason":"e2e"}' >/dev/null; check "int out of range refused" "$(st)" 400
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"setpoint","value":150},"reason":"e2e"}' >/dev/null; check "value above max refused" "$(st)" 400
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"sinus","value":1},"reason":"e2e"}' >/dev/null; check "read-only tag refused" "$(st)" 400
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"txt","value":12},"reason":"e2e"}' >/dev/null; check "number into a string refused" "$(st)" 400
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"small","value":1},"reason":"e2e"}' >/dev/null; check "key of another device refused" "$(st)" 400
req POST "/api/v1/devices/$DEV/commands" '{"method":"reboot","args":{},"reason":"e2e"}' >/dev/null; check "other methods refused" "$(st)" 400
sleep 3; req GET /api/v1/connectors >/dev/null
[[ "$(jx "$TMP/b" "((j.find(c=>c.id==='$CID')||{}).status||{}).last_values.setpoint")" == "55.5" ]] && ok "next poll sees the written setpoint" || bad "poll after write: $(jx "$TMP/b" "((j.find(c=>c.id==='$CID')||{}).status||{}).last_values")"
[[ "$(jx "$TMP/b" "((j.find(c=>c.id==='$CID')||{}).status||{}).writes")" == "4" ]] && ok "writes counted" || bad "writes: $(jx "$TMP/b" "((j.find(c=>c.id==='$CID')||{}).status||{}).writes")"
req GET "/api/v1/export/audit" >/dev/null; grep -q "device.command" "$TMP/b" && ok "writes in the audit trail" || bad "audit"

echo "== update and revoke"
req PUT "/api/v1/connectors/$CID" "{\"settings\":$(echo "$SETTINGS" | sed 's/"interval_s":2/"interval_s":3/'),\"reason\":\"e2e\"}" >/dev/null; check "update" "$(st)" 200
[[ "$(jx "$TMP/b" 'j.settings.interval_s')" == "3" ]] && ok "interval updated" || bad "interval"
req PUT "/api/v1/connectors/$CID" '{"settings":{"host":"127.0.0.1","tags":[{"key":"a","address":"nonsense"}]},"reason":"e2e"}' >/dev/null; check "update validated" "$(st)" 400
req GET "/api/v1/export/audit" >/dev/null; grep -q "connector.update" "$TMP/b" && ok "update audited" || bad "audit update"
req DELETE "/api/v1/connectors/$CID?reason=e2e" >/dev/null; check "revoke" "$(st)" 200
sleep 1
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"setpoint","value":20},"reason":"e2e"}' >/dev/null; check "write refused after revoke" "$(st)" 400

echo
echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

#!/usr/bin/env bash
# End-to-end check of the OPC UA client connector (URS E-*): starts the bundled simulator, creates a connector,
# waits for polls, verifies the virtual device, its messages and the test/update/revoke paths; endpoint discovery,
# browsing and a subscription connector feeding several devices (0.83.0).
# Needs Go (builds tools/opcuasim) and Node (JSON extraction) — a developer machine, not the demo server.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/opcua.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
PORT=${SIM_PORT:-4850}
command -v go >/dev/null 2>&1 || export PATH="/d/tools/go/bin:$PATH"
TMP=$(mktemp -d); trap 'tail -5 "$TMP/sim.log" 2>/dev/null; rm -rf "$TMP"; taskkill //F //IM opcuasim.exe >/dev/null 2>&1; pkill -f opcuasim >/dev/null 2>&1; true' EXIT
JAR="$TMP/jar"; CSRF=""
pass=0; fail=0
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jget() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }
# jx FILE EXPR — evaluate a JS expression over the parsed JSON in FILE (bound to j); prints the result as JSON
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$1" "$2" 2>/dev/null; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  STATUS=$(cat "$TMP/s"); cat "$TMP/b"; }
SUF=$RANDOM; NAME="sim-$SUF"; EXT="opcua-sim-$SUF"

echo "== simulator"
EXE=bin/opcuasim; [[ "$(uname -s)" == MINGW* || "$(uname -s)" == MSYS* ]] && EXE=bin/opcuasim.exe
go build -o "$EXE" ./tools/opcuasim && ok "simulator built" || { bad "simulator build"; exit 1; }
"$EXE" -port "$PORT" > "$TMP/sim.log" 2>&1 &
for i in $(seq 1 20); do grep -q "listening" "$TMP/sim.log" && break; sleep 0.5; done
grep -q "listening" "$TMP/sim.log" && ok "simulator listening on $PORT" || { bad "simulator start: $(cat "$TMP/sim.log")"; exit 1; }
NS=$(grep -o "namespace [0-9]*" "$TMP/sim.log" | grep -o "[0-9]*"); [[ -n "$NS" ]] && ok "namespace index $NS" || bad "namespace index"

echo "== login"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
ME=$(req GET /api/v1/auth/me); CSRF=$(echo "$ME" | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

SETTINGS="{\"endpoint\":\"opc.tcp://127.0.0.1:$PORT\",\"interval_s\":2,\"device_external_id\":\"$EXT\",\"nodes\":[{\"node_id\":\"ns=$NS;s=Setpoint\",\"key\":\"setpoint\",\"writable\":true,\"min\":0,\"max\":100},{\"node_id\":\"ns=$NS;s=Sinusoid\",\"key\":\"sinus\"},{\"node_id\":\"ns=$NS;s=Counter\",\"key\":\"counter\"},{\"node_id\":\"ns=$NS;s=Temperature\",\"key\":\"t_c\",\"scale\":1,\"offset\":0.5},{\"node_id\":\"ns=$NS;s=Boolean\",\"key\":\"run\"},{\"node_id\":\"ns=$NS;s=Text\",\"key\":\"txt\"},{\"node_id\":\"ns=$NS;s=Missing\",\"key\":\"missing\"}]}"

echo "== validation"
req POST /api/v1/connectors "{\"kind\":\"opcua\",\"name\":\"bad-$SUF\",\"settings\":{\"endpoint\":\"http://x\",\"nodes\":[]},\"reason\":\"e2e\"}" >/dev/null; check "endpoint validated" "$(st)" 400
req POST /api/v1/connectors "{\"kind\":\"opcua\",\"name\":\"bad-$SUF\",\"settings\":{\"endpoint\":\"opc.tcp://h:4840\",\"nodes\":[{\"node_id\":\"nonsense\",\"key\":\"a\"}]},\"reason\":\"e2e\"}" >/dev/null; check "node id validated" "$(st)" 400
req POST /api/v1/connectors "{\"kind\":\"opcua\",\"name\":\"bad-$SUF\",\"settings\":{\"endpoint\":\"opc.tcp://h:4840\",\"nodes\":[{\"node_id\":\"ns=2;i=1\",\"key\":\"Bad Key\"}]},\"reason\":\"e2e\"}" >/dev/null; check "key validated" "$(st)" 400
req POST /api/v1/connectors "{\"kind\":\"opcua\",\"name\":\"bad-$SUF\",\"settings\":{\"endpoint\":\"opc.tcp://h:4840\",\"security_policy\":\"Basic256Sha256\",\"security_mode\":\"None\",\"nodes\":[{\"node_id\":\"ns=2;i=1\",\"key\":\"a\"}]},\"reason\":\"e2e\"}" >/dev/null; check "policy/mode consistency" "$(st)" 400
req POST /api/v1/connectors/test "{\"settings\":{\"endpoint\":\"opc.tcp://127.0.0.1:1\",\"nodes\":[{\"node_id\":\"ns=2;i=1\",\"key\":\"a\"}]}}" >/dev/null; check "test against a dead endpoint fails cleanly" "$(st)" 502

echo "== test connection"
req POST /api/v1/connectors/test "{\"settings\":$SETTINGS}" >/dev/null; check "test" "$(st)" 200
[[ "$(jx "$TMP/b" 'typeof j.values.sinus')" == "number" ]] && ok "sinusoid read" || bad "sinus: $(jx "$TMP/b" 'j.values')"
[[ "$(jx "$TMP/b" 'j.values.txt')" == tick* ]] && ok "string value read" || bad "txt"
[[ "$(jx "$TMP/b" 'typeof j.values.run')" == "boolean" ]] && ok "boolean read" || bad "boolean"
[[ "$(jx "$TMP/b" 'typeof j.values.counter')" == "number" ]] && ok "integer read" || bad "counter"
[[ "$(jx "$TMP/b" 'j.bad_nodes.length')" == "1" ]] && ok "missing node reported, not fatal" || bad "bad_nodes: $(jx "$TMP/b" 'j.bad_nodes')"

echo "== create and poll"
req POST /api/v1/connectors "{\"kind\":\"opcua\",\"name\":\"$NAME\",\"settings\":$SETTINGS,\"reason\":\"e2e\"}" >/dev/null; check "create" "$(st)" 201
CID=$(jget id < "$TMP/b"); [[ -n "$CID" ]] && ok "id" || bad "id"
grep -q '"secret"' "$TMP/b" && bad "no secret expected" || ok "no secret for outbound connector"
STATE=""; for i in $(seq 1 30); do sleep 1; req GET /api/v1/connectors >/dev/null; STATE=$(jx "$TMP/b" "(j.find(c=>c.name==='$NAME')||{}).status"); [[ "$(jx "$TMP/b" "((j.find(c=>c.name==='$NAME')||{}).status||{}).polls > 0 && ((j.find(c=>c.name==='$NAME')||{}).status||{}).state")" == "connected" ]] && break; done
[[ "$(jx "$TMP/b" "((j.find(c=>c.name==='$NAME')||{}).status||{}).state")" == "connected" ]] && ok "poller connected" || bad "poller state: $STATE"
[[ "$(jx "$TMP/b" "typeof ((j.find(c=>c.name==='$NAME')||{}).status||{}).last_values.counter")" == "number" ]] && ok "values polled" || bad "values: $STATE"
[[ "$(jx "$TMP/b" "(((j.find(c=>c.name==='$NAME')||{}).status||{}).bad_nodes||[]).length")" == "1" ]] && ok "bad node tracked" || bad "bad_nodes"
grep -q '"password"' "$TMP/b" && bad "password field leaked in list" || ok "list without password"
req GET /api/v1/devices >/dev/null; DID=$(jx "$TMP/b" "(j.find(d=>d.external_id==='$EXT')||{}).id"); [[ -n "$DID" && "$DID" != "null" ]] && ok "virtual device created" || bad "virtual device"
[[ "$(jx "$TMP/b" "(j.find(d=>d.external_id==='$EXT')||{}).transport")" == "opcua" ]] && ok "transport opcua" || bad "transport"
sleep 3
req GET "/api/v1/devices/$DID/messages?limit=5" >/dev/null; check "messages" "$(st)" 200
grep -q "opcua/$NAME" "$TMP/b" && ok "message topic" || bad "topic: $(head -c 200 "$TMP/b")"
grep -q 'counter' "$TMP/b" && ok "payload carries counter" || bad "payload"

echo "== writes as commands"
req POST "/api/v1/devices/$DID/commands" '{"method":"write","args":{"key":"setpoint","value":42.5},"reason":"e2e setpoint"}' >/dev/null; check "write setpoint" "$(st)" 201
[[ "$(jx "$TMP/b" "j.status")" == "acked" ]] && ok "command acked" || bad "command: $(head -c 200 "$TMP/b")"
[[ "$(jx "$TMP/b" "j.result.readback")" == "42.5" ]] && ok "read back 42.5" || bad "readback: $(head -c 200 "$TMP/b")"
req POST "/api/v1/devices/$DID/commands" '{"method":"write","args":{"key":"setpoint","value":500},"reason":"e2e"}' >/dev/null; check "value above max refused" "$(st)" 400
req POST "/api/v1/devices/$DID/commands" '{"method":"write","args":{"key":"sinus","value":1},"reason":"e2e"}' >/dev/null; check "read-only node refused" "$(st)" 400
req POST "/api/v1/devices/$DID/commands" '{"method":"write","args":{"key":"nope","value":1},"reason":"e2e"}' >/dev/null; check "unknown key refused" "$(st)" 400
sleep 3; req GET /api/v1/connectors >/dev/null
[[ "$(jx "$TMP/b" "((j.find(c=>c.name==='$NAME')||{}).status||{}).last_values.setpoint")" == "42.5" ]] && ok "next poll sees the written setpoint" || bad "poll after write: $(jx "$TMP/b" "((j.find(c=>c.name==='$NAME')||{}).status||{}).last_values")"

echo "== update and revoke"
req PUT "/api/v1/connectors/$CID" "{\"settings\":$(echo "$SETTINGS" | sed 's/"interval_s":2/"interval_s":3/'),\"reason\":\"e2e\"}" >/dev/null; check "update" "$(st)" 200
[[ "$(jx "$TMP/b" 'j.settings.interval_s')" == "3" ]] && ok "interval updated" || bad "interval"
req PUT "/api/v1/connectors/$CID" '{"settings":{"endpoint":"x"},"reason":"e2e"}' >/dev/null; check "update validated" "$(st)" 400
req GET "/api/v1/connectors/opcua/certificate" >/dev/null; check "client certificate" "$(st)" 200
grep -q "BEGIN CERTIFICATE" "$TMP/b" && ok "PEM" || bad "PEM"
req DELETE "/api/v1/connectors/$CID?reason=e2e" >/dev/null; check "revoke" "$(st)" 200
sleep 2; req GET /api/v1/connectors >/dev/null; [[ "$(jx "$TMP/b" "!!(j.find(c=>c.name==='$NAME')||{}).revoked_at")" == "true" ]] && ok "revoked" || bad "revoked"
req GET "/api/v1/export/audit" >/dev/null; grep -q "connector.update" "$TMP/b" && ok "audited" || bad "audit"

echo "== endpoints and browsing (0.83.0)"
req POST /api/v1/connectors/opcua/endpoints "{\"endpoint\":\"opc.tcp://127.0.0.1:$PORT\"}" >/dev/null; check "endpoints" "$(st)" 200
[[ "$(jx "$TMP/b" 'j.endpoints[0].security_policy + "/" + j.endpoints[0].user_tokens[0]')" == "None/anonymous" ]] && ok "None / anonymous offered" || bad "endpoints: $(head -c 300 "$TMP/b")"
req POST /api/v1/connectors/opcua/endpoints '{"endpoint":"http://x"}' >/dev/null; check "endpoint validated" "$(st)" 400
CONN="{\"endpoint\":\"opc.tcp://127.0.0.1:$PORT\"}"
req POST /api/v1/connectors/opcua/browse "{\"settings\":$CONN}" >/dev/null; check "browse Objects" "$(st)" 200
SIMID=$(jx "$TMP/b" '(j.items.find(i=>i.display_name==="Simulation")||{}).node_id'); [[ -n "$SIMID" && "$SIMID" != "null" ]] && ok "Simulation folder found ($SIMID)" || bad "browse: $(head -c 300 "$TMP/b")"
req POST /api/v1/connectors/opcua/browse "{\"settings\":$CONN,\"node_id\":\"$SIMID\"}" >/dev/null; check "browse Simulation" "$(st)" 200
[[ "$(jx "$TMP/b" 'j.items[0].node_class')" == "Object" ]] && ok "objects listed first" || bad "order"
[[ "$(jx "$TMP/b" '(j.items.find(i=>i.display_name==="Setpoint")||{}).writable')" == "true" ]] && ok "Setpoint marked writable" || bad "setpoint access"
[[ "$(jx "$TMP/b" '(j.items.find(i=>i.display_name==="Sinusoid")||{}).data_type')" == "Double" ]] && ok "data type named" || bad "data type"
req POST /api/v1/connectors/opcua/browse "{\"settings\":$CONN,\"node_id\":\"ns=$NS;s=Line1\"}" >/dev/null; check "browse Line1" "$(st)" 200
[[ "$(jx "$TMP/b" 'j.items.map(i=>i.display_name).sort().join(",")')" == "Count,Name,Running,Speed" ]] && ok "Line1 variables" || bad "line1: $(jx "$TMP/b" 'j.items.map(i=>i.display_name)')"
req POST /api/v1/connectors/opcua/browse "{\"settings\":$CONN,\"node_id\":\"nonsense\"}" >/dev/null; check "invalid node id refused" "$(st)" 502
req POST /api/v1/connectors/opcua/browse '{"settings":{"endpoint":"opc.tcp://127.0.0.1:1"}}' >/dev/null; check "dead server fails cleanly" "$(st)" 502

echo "== subscription with several devices (0.83.0)"
SNAME="sub-$SUF"; L1="opcua-line1-$SUF"; L2="opcua-line2-$SUF"
SUBSET="{\"endpoint\":\"opc.tcp://127.0.0.1:$PORT\",\"mode\":\"subscribe\",\"publishing_ms\":500,\"device_external_id\":\"opcua-sub-$SUF\",\"nodes\":[{\"node_id\":\"ns=$NS;s=Counter\",\"key\":\"counter\"},{\"node_id\":\"ns=$NS;s=Line1.Speed\",\"key\":\"speed\",\"device\":\"$L1\",\"deadband\":0.1},{\"node_id\":\"ns=$NS;s=Line1.Running\",\"key\":\"running\",\"device\":\"$L1\",\"writable\":true},{\"node_id\":\"ns=$NS;s=Line2.Speed\",\"key\":\"speed\",\"device\":\"$L2\"},{\"node_id\":\"ns=$NS;s=Line2.Count\",\"key\":\"count\",\"device\":\"$L2\"}]}"
req POST /api/v1/connectors "{\"kind\":\"opcua\",\"name\":\"$SNAME\",\"settings\":{\"endpoint\":\"opc.tcp://127.0.0.1:$PORT\",\"mode\":\"subscribe\",\"publishing_ms\":10,\"nodes\":[{\"node_id\":\"i=1\",\"key\":\"a\"}]},\"reason\":\"e2e\"}" >/dev/null; check "publishing interval validated" "$(st)" 400
SID=$(req POST /api/v1/connectors "{\"kind\":\"opcua\",\"name\":\"$SNAME\",\"settings\":$SUBSET,\"reason\":\"e2e\"}" | jget id); check "create subscription connector" "$(st)" 201
for i in $(seq 1 20); do sleep 1; req GET /api/v1/connectors >/dev/null; [[ "$(jx "$TMP/b" "((j.find(c=>c.id==='$SID')||{}).status||{}).notifications >= 3")" == "true" ]] && break; done
[[ "$(jx "$TMP/b" "((j.find(c=>c.id==='$SID')||{}).status||{}).mode")" == "subscribe" ]] && ok "mode subscribe" || bad "mode: $(jx "$TMP/b" "(j.find(c=>c.id==='$SID')||{}).status")"
[[ "$(jx "$TMP/b" "((j.find(c=>c.id==='$SID')||{}).status||{}).notifications >= 3")" == "true" ]] && ok "data change notifications arrive" || bad "notifications: $(jx "$TMP/b" "(j.find(c=>c.id==='$SID')||{}).status")"
[[ "$(jx "$TMP/b" "typeof ((j.find(c=>c.id==='$SID')||{}).status||{}).last_values['$L2/count']")" == "number" ]] && ok "values keyed by device" || bad "last_values: $(jx "$TMP/b" "((j.find(c=>c.id==='$SID')||{}).status||{}).last_values")"
req GET /api/v1/devices >/dev/null
D1=$(jx "$TMP/b" "(j.find(d=>d.external_id==='$L1')||{}).id"); D2=$(jx "$TMP/b" "(j.find(d=>d.external_id==='$L2')||{}).id"); D0=$(jx "$TMP/b" "(j.find(d=>d.external_id==='opcua-sub-$SUF')||{}).id")
[[ -n "$D1" && "$D1" != "null" && -n "$D2" && "$D2" != "null" && -n "$D0" && "$D0" != "null" ]] && ok "three virtual devices from one server" || bad "devices: $D0 $D1 $D2"
req GET "/api/v1/devices/$D2/messages?limit=5" >/dev/null
grep -q '"count"' "$TMP/b" && ! grep -q '"counter"' "$TMP/b" && ok "line2 gets only its own nodes" || bad "line2 messages: $(head -c 300 "$TMP/b")"
req POST "/api/v1/devices/$D1/commands" '{"method":"write","args":{"key":"running","value":false},"reason":"e2e stop line 1"}' >/dev/null; check "write on a routed device" "$(st)" 201
[[ "$(jx "$TMP/b" "j.status")" == "acked" && "$(jx "$TMP/b" "j.result.readback")" == "false" ]] && ok "acked over the live session, read back false" || bad "write: $(head -c 300 "$TMP/b")"
req POST "/api/v1/devices/$D2/commands" '{"method":"write","args":{"key":"running","value":true},"reason":"e2e"}' >/dev/null; check "key of another device refused" "$(st)" 400
req POST "/api/v1/devices/$D1/commands" '{"method":"write","args":{"key":"speed","value":1},"reason":"e2e"}' >/dev/null; check "read-only node refused" "$(st)" 400
req PUT "/api/v1/connectors/$SID" "{\"settings\":$(echo "$SUBSET" | sed 's/"publishing_ms":500/"publishing_ms":1000/'),\"reason\":\"e2e\"}" >/dev/null; check "update subscription" "$(st)" 200
req GET "/api/v1/export/audit" >/dev/null; grep -q '"publishing_ms"' "$TMP/b" && ok "mode and publishing interval audited" || bad "audit publishing_ms"
req DELETE "/api/v1/connectors/$SID?reason=e2e" >/dev/null; check "revoke subscription connector" "$(st)" 200

echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

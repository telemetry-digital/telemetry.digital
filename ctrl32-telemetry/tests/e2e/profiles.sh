#!/usr/bin/env bash
# End-to-end check of versioned register profiles (URS A-11): a profile is created and validated, two Modbus
# connectors take their registers from it (snapshot + reference), a new version is saved without touching the
# connectors, applying it moves both connectors to the new registers (audited, pollers restarted, new key polled),
# retiring is refused while the profile is in use and works afterwards. Needs Go (tools/modbussim) and Node.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/profiles.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
PORT=${SIM_PORT:-5023}
command -v go >/dev/null 2>&1 || export PATH="/d/tools/go/bin:$PATH"
TMP=$(mktemp -d); trap 'kill $SIM 2>/dev/null; rm -rf "$TMP"' EXIT
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

echo "== simulator"
EXE=bin/modbussim; [[ "$(uname -s)" == MINGW* || "$(uname -s)" == MSYS* ]] && EXE=bin/modbussim.exe
go build -o "$EXE" ./tools/modbussim && ok "simulator built" || { bad "simulator build"; exit 1; }
"$EXE" -port "$PORT" > "$TMP/sim.log" 2>&1 &
SIM=$!
sleep 1.5; grep -q "listening" "$TMP/sim.log" && ok "simulator on $PORT" || { bad "simulator"; exit 1; }

echo "== login and profile validation"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token)
req POST /api/v1/register-profiles "{\"name\":\"bad-$SUF\",\"spec\":{\"registers\":[]},\"reason\":\"e2e\"}" >/dev/null; check "empty registers refused" "$(st)" 400
req POST /api/v1/register-profiles "{\"name\":\"bad-$SUF\",\"spec\":{\"registers\":[{\"key\":\"Bad Key\",\"table\":\"holding\",\"address\":0}]},\"reason\":\"e2e\"}" >/dev/null; check "key validated" "$(st)" 400
req POST /api/v1/register-profiles "{\"name\":\"bad-$SUF\",\"spec\":{\"byte_order\":\"weird\",\"registers\":[{\"key\":\"a\",\"table\":\"holding\",\"address\":0}]},\"reason\":\"e2e\"}" >/dev/null; check "byte order validated" "$(st)" 400
req POST /api/v1/register-profiles "{\"name\":\"bad-$SUF\",\"spec\":{\"registers\":[{\"key\":\"a\",\"table\":\"holding\",\"address\":0}]}}" >/dev/null; check "reason required" "$(st)" 400
SPEC1='{"byte_order":"big","registers":[{"key":"temp","table":"holding","address":0,"type":"int16","scale":0.1},{"key":"setpoint","table":"holding","address":1,"type":"int16","scale":0.1,"writable":true,"min":10,"max":40}]}'
PROF=$(req POST /api/v1/register-profiles "{\"name\":\"erc-$SUF\",\"description\":\"cold room controller\",\"spec\":$SPEC1,\"reason\":\"e2e\"}" | jget id); check "profile created" "$(st)" 201
[[ "$(jx "$TMP/b" 'j.version')" == "1" && "$(jx "$TMP/b" 'j.registers')" == "2" ]] && ok "version 1 with 2 registers" || bad "create: $(cat "$TMP/b")"
req POST /api/v1/register-profiles "{\"name\":\"erc-$SUF\",\"spec\":$SPEC1,\"reason\":\"e2e\"}" >/dev/null; check "duplicate name refused" "$(st)" 409
req GET /api/v1/register-profiles >/dev/null; [[ "$(jx "$TMP/b" "(j.find(p=>p.id==='$PROF')||{}).writable")" == "1" && "$(jx "$TMP/b" "(j.find(p=>p.id==='$PROF')||{}).connectors")" == "0" ]] && ok "listed: 1 writable, no connectors" || bad "list: $(head -c 300 "$TMP/b")"

echo "== connectors from the profile"
req POST /api/v1/connectors "{\"kind\":\"modbus_tcp\",\"name\":\"plc-a-$SUF\",\"settings\":{\"host\":\"127.0.0.1\",\"port\":$PORT,\"unit_id\":1,\"interval_s\":2,\"profile_id\":\"00000000-0000-0000-0000-000000000000\"},\"reason\":\"e2e\"}" >/dev/null; check "unknown profile refused" "$(st)" 400
CA=$(req POST /api/v1/connectors "{\"kind\":\"modbus_tcp\",\"name\":\"plc-a-$SUF\",\"settings\":{\"host\":\"127.0.0.1\",\"port\":$PORT,\"unit_id\":1,\"interval_s\":2,\"profile_id\":\"$PROF\"},\"reason\":\"e2e\"}" | jget id); check "connector A from the profile" "$(st)" 201
[[ "$(jx "$TMP/b" 'j.settings.registers.length')" == "2" && "$(jx "$TMP/b" 'j.settings.profile_version')" == "1" && "$(jx "$TMP/b" 'j.settings.profile_name')" == "erc-$SUF" ]] && ok "registers snapshot + profile reference in the settings" || bad "settings: $(head -c 300 "$TMP/b")"
CB=$(req POST /api/v1/connectors "{\"kind\":\"modbus_tcp\",\"name\":\"plc-b-$SUF\",\"settings\":{\"host\":\"127.0.0.1\",\"port\":$PORT,\"unit_id\":1,\"interval_s\":2,\"profile_id\":\"$PROF\"},\"reason\":\"e2e\"}" | jget id); check "connector B from the profile" "$(st)" 201
sleep 5
req GET /api/v1/connectors >/dev/null; [[ "$(jx "$TMP/b" "(j.find(c=>c.id==='$CA')||{}).status.last_values.temp")" != "null" ]] && ok "A polls temp through the profile registers" || bad "poll: $(jx "$TMP/b" "(j.find(c=>c.id==='$CA')||{}).status")"
req GET /api/v1/register-profiles >/dev/null; [[ "$(jx "$TMP/b" "(j.find(p=>p.id==='$PROF')||{}).connectors")" == "2" ]] && ok "profile lists 2 connectors" || bad "connectors count"
req DELETE "/api/v1/register-profiles/$PROF?reason=e2e" >/dev/null; check "retire refused while in use" "$(st)" 409

echo "== new version"
SPEC2='{"byte_order":"big","registers":[{"key":"temp","table":"holding","address":0,"type":"int16","scale":0.1},{"key":"setpoint","table":"holding","address":1,"type":"int16","scale":0.1,"writable":true,"min":5,"max":45},{"key":"mode","table":"holding","address":10,"type":"int16","writable":true,"min":0,"max":3}]}'
req PUT "/api/v1/register-profiles/$PROF" "{\"spec\":$SPEC2,\"changelog\":\"mode register, wider setpoint\",\"reason\":\"e2e v2\"}" >/dev/null; check "version 2 saved" "$(st)" 201
PROF2=$(jx "$TMP/b" 'j.id'); [[ "$(jx "$TMP/b" 'j.version')" == "2" && "$PROF2" != "$PROF" ]] && ok "new version row $PROF2" || bad "v2: $(cat "$TMP/b")"
req PUT "/api/v1/register-profiles/$PROF" "{\"spec\":$SPEC2,\"reason\":\"e2e\"}" >/dev/null; check "old version cannot be changed" "$(st)" 409
req GET "/api/v1/register-profiles/$PROF2" >/dev/null; [[ "$(jx "$TMP/b" 'j.versions.length')" == "2" && "$(jx "$TMP/b" 'j.versions[1].connectors')" == "2" && "$(jx "$TMP/b" 'j.versions[0].changelog')" == "mode register, wider setpoint" ]] && ok "history: v1 has 2 connectors, v2 has the changelog" || bad "detail: $(head -c 400 "$TMP/b")"
req GET /api/v1/register-profiles >/dev/null; [[ "$(jx "$TMP/b" "j.filter(p=>p.name==='erc-$SUF').length")" == "1" && "$(jx "$TMP/b" "(j.find(p=>p.id==='$PROF2')||{}).outdated")" == "2" ]] && ok "list shows only the current version, 2 connectors outdated" || bad "list 2: $(head -c 300 "$TMP/b")"
req GET /api/v1/connectors >/dev/null; [[ "$(jx "$TMP/b" "(j.find(c=>c.id==='$CA')||{}).settings.profile_version")" == "1" ]] && ok "connectors still on v1" || bad "connector version"

echo "== apply"
req POST "/api/v1/register-profiles/$PROF/apply" '{"reason":"e2e"}' >/dev/null; check "old version cannot be applied" "$(st)" 409
req POST "/api/v1/register-profiles/$PROF2/apply" '{"reason":"roll out v2"}' >/dev/null; check "apply v2" "$(st)" 200
[[ "$(jx "$TMP/b" 'j.applied')" == "2" ]] && ok "applied to both connectors" || bad "apply: $(cat "$TMP/b")"
req GET /api/v1/connectors >/dev/null; [[ "$(jx "$TMP/b" "(j.find(c=>c.id==='$CA')||{}).settings.profile_version")" == "2" && "$(jx "$TMP/b" "(j.find(c=>c.id==='$CB')||{}).settings.registers.length")" == "3" ]] && ok "connectors on v2 with 3 registers" || bad "after apply: $(jx "$TMP/b" "(j.find(c=>c.id==='$CA')||{}).settings")"
sleep 5
req GET /api/v1/connectors >/dev/null; [[ "$(jx "$TMP/b" "(j.find(c=>c.id==='$CB')||{}).status.last_values.mode")" != "null" ]] && ok "B polls the new mode register" || bad "mode poll: $(jx "$TMP/b" "(j.find(c=>c.id==='$CB')||{}).status.last_values")"
req POST "/api/v1/register-profiles/$PROF2/apply" '{"reason":"again"}' >/dev/null; [[ "$(jx "$TMP/b" 'j.applied')" == "0" ]] && ok "nothing left to apply" || bad "apply again: $(cat "$TMP/b")"
DEV=$(jx "$TMP/b" 'null'); req GET /api/v1/devices >/dev/null; DEV=$(jx "$TMP/b" "(j.find(d=>d.external_id==='modbus-plc-b-$SUF')||{}).id")
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"setpoint","value":44},"reason":"e2e"}' >/dev/null; check "write within the v2 limit (44 ≤ 45)" "$(st)" 201
req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"setpoint","value":46},"reason":"e2e"}' >/dev/null; check "write above the v2 limit refused" "$(st)" 400

echo "== retire after use ends"
req PUT "/api/v1/connectors/$CA" "{\"settings\":{\"host\":\"127.0.0.1\",\"port\":$PORT,\"unit_id\":1,\"interval_s\":2,\"registers\":[{\"key\":\"temp\",\"table\":\"holding\",\"address\":0,\"type\":\"int16\",\"scale\":0.1}]},\"reason\":\"e2e detach\"}" >/dev/null; check "A detached from the profile" "$(st)" 200
[[ "$(jx "$TMP/b" 'j.settings.profile_id')" == "null" ]] && ok "profile reference dropped" || bad "detach: $(head -c 300 "$TMP/b")"
req DELETE "/api/v1/connectors/$CB?reason=e2e" >/dev/null; check "B revoked" "$(st)" 200
req DELETE "/api/v1/register-profiles/$PROF2?reason=e2e" >/dev/null; check "profile retired" "$(st)" 200
req GET /api/v1/register-profiles >/dev/null; [[ "$(jx "$TMP/b" "(j.find(p=>p.id==='$PROF2')||{}).retired_at")" != "null" ]] && ok "listed as retired" || bad "retired list"
req POST /api/v1/register-profiles "{\"name\":\"erc-$SUF\",\"spec\":$SPEC1,\"reason\":\"e2e\"}" >/dev/null; check "name reusable after retirement" "$(st)" 201
NEWP=$(jget id < "$TMP/b"); req DELETE "/api/v1/register-profiles/$NEWP?reason=e2e" >/dev/null
req DELETE "/api/v1/connectors/$CA?reason=e2e" >/dev/null; check "A revoked" "$(st)" 200

echo "== audit"
req GET "/api/v1/export/audit" >/dev/null
for a in register_profile.create register_profile.update register_profile.apply register_profile.retire; do grep -q "$a" "$TMP/b" && ok "audit $a" || bad "audit $a missing"; done
grep -q "profile erc-$SUF v2 applied" "$TMP/b" && ok "connector updates audited with the profile version" || bad "audit connector update"
echo
echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

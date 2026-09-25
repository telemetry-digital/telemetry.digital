#!/usr/bin/env bash
# End-to-end check of four eyes for configuration changes (URS H-08): with the policies four_eyes_limits and
# four_eyes_assignments an alarm rule change and a datastream assignment are recorded as change requests (202),
# the requester cannot approve them, a second user approves (the original call is replayed and audited with both
# names) or rejects, expired/decided requests are refused, and turning the policies off restores direct execution.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/fouryes2.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; JAR2="$TMP/jar2"; CSRF=""; CSRF2=""
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
req2() { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR2" -c "$JAR2" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF2" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR2" -c "$JAR2" -X "$m" -H "X-CSRF-Token: $CSRF2" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  cat "$TMP/b"; }
SUF=$RANDOM

echo "== users"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "admin login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token)
U=$(req POST /api/v1/users "{\"username\":\"fe2_$SUF\",\"full_name\":\"Second Eyes\",\"email\":\"fe2$SUF@example.com\",\"roles\":[\"engineer\"],\"reason\":\"e2e\"}"); check "second user (engineer)" "$(st)" 201
UID2=$(echo "$U" | grep -o '"user":{[^}]*' | jget id); PW=$(echo "$U" | jget initial_password)
req2 POST /api/v1/auth/login "{\"username\":\"fe2_$SUF\",\"password\":\"$PW\"}" >/dev/null; CSRF2=$(req2 GET /api/v1/auth/me | jget csrf_token)
NEWPW="Second-Eyes-Passw0rd-$SUF"
req2 POST /api/v1/auth/password "{\"current\":\"$PW\",\"new\":\"$NEWPW\"}" >/dev/null; req2 POST /api/v1/auth/login "{\"username\":\"fe2_$SUF\",\"password\":\"$NEWPW\"}" >/dev/null; CSRF2=$(req2 GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF2" ]] && ok "second user signed in" || bad "second user"

echo "== fixtures"
SITE=$(req POST /api/v1/sites "{\"name\":\"4E site $SUF\",\"reason\":\"e2e\"}" | jget id)
ASSET=$(req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"fridge\",\"name\":\"4E fridge $SUF\",\"reason\":\"e2e\"}" | jget id)
DS=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"t_c\",\"quantity\":\"temperature\",\"unit\":\"C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" | jget id)
DEV=$(req POST /api/v1/devices "{\"external_id\":\"fe2-$SUF\",\"name\":\"four eyes 2\",\"reason\":\"e2e\"}" | jget id); [[ -n "$DS" && -n "$DEV" ]] && ok "site, asset, datastream, device" || bad "fixtures"

echo "== policies"
req GET /api/v1/settings/policy >/dev/null; [[ "$(jx "$TMP/b" 'j.four_eyes_limits')" == "false" && "$(jx "$TMP/b" 'j.four_eyes_assignments')" == "false" ]] && ok "policies off by default" || bad "policy: $(cat "$TMP/b")"
req PUT /api/v1/settings/policy '{"reason":"e2e"}' >/dev/null; check "at least one field required" "$(st)" 400
req PUT /api/v1/settings/policy '{"four_eyes_limits":true,"four_eyes_assignments":true,"reason":"e2e on"}' >/dev/null; check "policies on" "$(st)" 200
req GET /api/v1/settings/policy >/dev/null; [[ "$(jx "$TMP/b" 'j.four_eyes_limits && j.four_eyes_assignments && !j.four_eyes_commands')" == "true" ]] && ok "limits and assignments on, commands untouched" || bad "policy: $(cat "$TMP/b")"

echo "== alarm rule change becomes a change request"
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$DS\",\"type\":\"high\",\"action\":30,\"reason\":\"e2e limit\"}" >/dev/null; check "202 pending" "$(st)" 202
CR1=$(jx "$TMP/b" 'j.change_request.id'); [[ "$(jx "$TMP/b" 'j.status')" == "pending" && -n "$CR1" ]] && ok "change request $CR1" || bad "body: $(cat "$TMP/b")"
jx "$TMP/b" 'j.change_request.summary' | grep -q "alarm rule high on 4E fridge $SUF / t_c: action 30" && ok "summary in words" || bad "summary: $(jx "$TMP/b" 'j.change_request.summary')"
req GET "/api/v1/alarm-rules?datastream_id=$DS" >/dev/null; check "rule not created yet" "$(jx "$TMP/b" 'j.length')" 0
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$DS\",\"type\":\"high\",\"action\":30}" >/dev/null; check "reason still required" "$(st)" 400
req GET /api/v1/change-requests >/dev/null; [[ "$(jx "$TMP/b" "(j.find(c=>c.id==='$CR1')||{}).can_approve")" == "false" ]] && ok "requester sees it but cannot approve" || bad "list: $(head -c 300 "$TMP/b")"
req POST "/api/v1/change-requests/$CR1/approve" '{"reason":"self"}' >/dev/null; check "requester cannot approve" "$(st)" 403
req2 GET /api/v1/change-requests >/dev/null; [[ "$(jx "$TMP/b" "(j.find(c=>c.id==='$CR1')||{}).can_approve")" == "true" ]] && ok "second user may approve" || bad "list 2: $(head -c 300 "$TMP/b")"
req2 POST "/api/v1/change-requests/$CR1/approve" '{"reason":"limit checked against the SOP"}' >/dev/null; check "second user approves" "$(st)" 200
[[ "$(jx "$TMP/b" 'j.status')" == "approved" && "$(jx "$TMP/b" 'j.code')" == "201" && "$(jx "$TMP/b" 'j.result.version')" == "1" ]] && ok "original call replayed: rule version 1 created" || bad "approve: $(cat "$TMP/b")"
req GET "/api/v1/alarm-rules?datastream_id=$DS" >/dev/null; check "rule exists now" "$(jx "$TMP/b" 'j.length')" 1
RULE=$(jx "$TMP/b" 'j[0].id')
req2 POST "/api/v1/change-requests/$CR1/approve" '{"reason":"again"}' >/dev/null; check "cannot approve twice" "$(st)" 409

echo "== disable rule → request → reject"
req DELETE "/api/v1/alarm-rules/$RULE?reason=e2e%20disable" >/dev/null; check "disable becomes a request" "$(st)" 202
CR2=$(jx "$TMP/b" 'j.change_request.id'); jx "$TMP/b" 'j.change_request.summary' | grep -q "disable alarm rule high" && ok "disable summary" || bad "summary 2"
req2 POST "/api/v1/change-requests/$CR2/reject" '{"reason":"keep the limit"}' >/dev/null; check "rejected" "$(st)" 200
req GET "/api/v1/alarm-rules?datastream_id=$DS" >/dev/null; check "rule still active" "$(jx "$TMP/b" 'j.length')" 1
req2 POST "/api/v1/change-requests/$CR2/approve" '{"reason":"late"}' >/dev/null; check "rejected request cannot be approved" "$(st)" 409

echo "== assignment becomes a change request"
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$DS\",\"reason\":\"e2e assign\"}" >/dev/null; check "202 pending" "$(st)" 202
CR3=$(jx "$TMP/b" 'j.change_request.id'); jx "$TMP/b" 'j.change_request.summary' | grep -q "assign 4E fridge $SUF / t_c to device four eyes 2" && ok "assignment summary" || bad "summary 3: $(jx "$TMP/b" 'j.change_request.summary')"
req GET "/api/v1/devices/$DEV" >/dev/null; check "not assigned yet" "$(jx "$TMP/b" 'j.datastreams.length')" 0
req2 POST "/api/v1/change-requests/$CR3/approve" '{"reason":"correct sensor"}' >/dev/null; check "approved" "$(st)" 200
[[ "$(jx "$TMP/b" 'j.code')" == "201" ]] && ok "assignment call replayed (201)" || bad "replay: $(cat "$TMP/b")"
req GET "/api/v1/devices/$DEV" >/dev/null; check "assigned now" "$(jx "$TMP/b" 'j.datastreams.length')" 1
req DELETE "/api/v1/devices/$DEV/datastreams/$DS?reason=e2e%20unassign" >/dev/null; check "unassign becomes a request" "$(st)" 202
CR4=$(jx "$TMP/b" 'j.change_request.id')
req2 POST "/api/v1/change-requests/$CR4/approve" '{"reason":"ok"}' >/dev/null; check "unassign approved" "$(st)" 200
req GET "/api/v1/devices/$DEV" >/dev/null; check "unassigned" "$(jx "$TMP/b" 'j.datastreams.length')" 0

echo "== second user's own request needs the admin"
req2 POST /api/v1/alarm-rules "{\"datastream_id\":\"$DS\",\"type\":\"low\",\"warning\":2,\"reason\":\"e2e low\"}" >/dev/null; check "engineer's change pending" "$(st)" 202
CR5=$(jx "$TMP/b" 'j.change_request.id')
req2 POST "/api/v1/change-requests/$CR5/approve" '{"reason":"self"}' >/dev/null; check "engineer cannot approve own" "$(st)" 403
req POST "/api/v1/change-requests/$CR5/approve" '{"reason":"admin approves"}' >/dev/null; check "admin approves" "$(st)" 200
req GET "/api/v1/change-requests?status=all" >/dev/null; [[ "$(jx "$TMP/b" "j.filter(c=>['$CR1','$CR2','$CR3','$CR4','$CR5'].includes(c.id)).map(c=>c.status).sort().join(',')")" == "approved,approved,approved,approved,rejected" ]] && ok "history lists all five" || bad "history: $(jx "$TMP/b" "j.filter(c=>['$CR1','$CR2','$CR3','$CR4','$CR5'].includes(c.id)).map(c=>c.status)")"

echo "== policies off → direct execution"
req PUT /api/v1/settings/policy '{"four_eyes_limits":false,"four_eyes_assignments":false,"reason":"e2e off"}' >/dev/null; check "policies off" "$(st)" 200
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$DS\",\"reason\":\"e2e direct\"}" >/dev/null; check "assignment direct again (201)" "$(st)" 201
req DELETE "/api/v1/alarm-rules/$RULE?reason=e2e" >/dev/null; check "rule disabled directly" "$(st)" 200

echo "== audit"
req GET "/api/v1/export/audit" >/dev/null
for a in change_request.create change_request.approve change_request.reject; do grep -q "$a" "$TMP/b" && ok "audit $a" || bad "audit $a missing"; done
grep -q "approved change $CR1 requested by $USER" "$TMP/b" && ok "replayed change audited with both names" || bad "audit reason"

echo "== cleanup"
req POST "/api/v1/devices/$DEV/revoke" '{"reason":"e2e cleanup"}' >/dev/null; check "device revoked" "$(st)" 200
req PATCH "/api/v1/users/$UID2" '{"disabled":true,"reason":"e2e cleanup"}' >/dev/null; check "second user disabled" "$(st)" 200
echo
echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

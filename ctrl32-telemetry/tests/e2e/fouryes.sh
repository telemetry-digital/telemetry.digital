#!/usr/bin/env bash
# End-to-end check of four eyes for commands (URS H-08, A-12): with the organization policy on, a command is
# recorded as pending, the issuer cannot approve it, a different user with device.command approves (executed) or
# rejects it; with the policy off commands run directly. Uses an MQTT device that is not connected (queued).
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/fouryes.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""; JAR2="$TMP/jar2"; CSRF2=""
pass=0; fail=0
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jget() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }
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
CSRF=$(req GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"
U=$(req POST /api/v1/users "{\"username\":\"fe_$SUF\",\"full_name\":\"Four Eyes\",\"email\":\"fe$SUF@example.com\",\"roles\":[\"engineer\"],\"reason\":\"e2e\"}"); check "second user (engineer)" "$(st)" 201
UID2=$(echo "$U" | grep -o '"user":{[^}]*' | jget id); PW=$(echo "$U" | jget initial_password); [[ -n "$UID2" && -n "$PW" ]] && ok "one-time password" || bad "user: $U"
req2 POST /api/v1/auth/login "{\"username\":\"fe_$SUF\",\"password\":\"$PW\"}" >/dev/null; check "second user login" "$(st)" 200
CSRF2=$(req2 GET /api/v1/auth/me | jget csrf_token)
NEWPW="Four-Eyes-Passw0rd-$SUF"
req2 POST /api/v1/auth/password "{\"current\":\"$PW\",\"new\":\"$NEWPW\"}" >/dev/null; check "password changed" "$(st)" 200
req2 POST /api/v1/auth/login "{\"username\":\"fe_$SUF\",\"password\":\"$NEWPW\"}" >/dev/null; check "second user re-login" "$(st)" 200
CSRF2=$(req2 GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF2" ]] && ok "csrf 2" || bad "csrf 2"

echo "== policy"
req GET /api/v1/settings/policy >/dev/null; check "policy read" "$(st)" 200
req PUT /api/v1/settings/policy '{"four_eyes_commands":true,"reason":"e2e on"}' >/dev/null; check "policy on" "$(st)" 200
req2 PUT /api/v1/settings/policy '{"four_eyes_commands":false,"reason":"x"}' >/dev/null; check "engineer cannot change policy" "$(st)" 403
DEV=$(req POST /api/v1/devices "{\"external_id\":\"fe-$SUF\",\"name\":\"four eyes\",\"reason\":\"e2e\"}" | jget id); [[ -n "$DEV" ]] && ok "device" || bad "device"

echo "== pending, approve"
C1=$(req POST "/api/v1/devices/$DEV/commands" '{"method":"set_output","args":{"key":"relay","value":1},"reason":"e2e"}' | jget id); check "command recorded as pending" "$(st)" 202
grep -q '"status":"pending"' "$TMP/b" && ok "status pending" || bad "status: $(head -c 160 "$TMP/b")"
req POST "/api/v1/devices/$DEV/commands/$C1/approve" '{"reason":"self"}' >/dev/null; check "issuer cannot approve" "$(st)" 403
req GET "/api/v1/devices/$DEV/commands" >/dev/null; grep -q "\"id\":\"$C1\"[^}]*\"status\":\"pending\"" "$TMP/b" && ok "listed as pending" || bad "list: $(head -c 200 "$TMP/b")"
req2 POST "/api/v1/devices/$DEV/commands/$C1/approve" '{"reason":"checked the value"}' >/dev/null; check "other user approves" "$(st)" 200
grep -q '"status":"queued"\|"status":"sent"' "$TMP/b" && ok "executed after approval (queued: device offline)" || bad "approve result: $(head -c 160 "$TMP/b")"
req2 POST "/api/v1/devices/$DEV/commands/$C1/approve" '{"reason":"again"}' >/dev/null; check "cannot approve twice" "$(st)" 409
req GET "/api/v1/devices/$DEV/commands" >/dev/null; grep -q "\"approved_by\":\"fe_$SUF\"" "$TMP/b" && ok "approver recorded" || bad "approver: $(head -c 200 "$TMP/b")"

echo "== pending, reject"
C2=$(req POST "/api/v1/devices/$DEV/commands" '{"method":"set_output","args":{"key":"relay","value":0},"reason":"e2e"}' | jget id); check "second command pending" "$(st)" 202
req2 POST "/api/v1/devices/$DEV/commands/$C2/reject" '{}' >/dev/null; check "reason required to reject" "$(st)" 400
req2 POST "/api/v1/devices/$DEV/commands/$C2/reject" '{"reason":"wrong value"}' >/dev/null; check "other user rejects" "$(st)" 200
req GET "/api/v1/devices/$DEV/commands" >/dev/null; grep -q "\"id\":\"$C2\"[^}]*\"status\":\"rejected\"" "$TMP/b" && ok "listed as rejected" || bad "list: $(head -c 200 "$TMP/b")"
req2 POST "/api/v1/devices/$DEV/commands/$C2/approve" '{"reason":"late"}' >/dev/null; check "rejected command cannot be approved" "$(st)" 409

echo "== other direction and policy off"
C3=$(req2 POST "/api/v1/devices/$DEV/commands" '{"method":"reboot","args":{},"reason":"e2e"}' | jget id); check "engineer command pending" "$(st)" 202
req POST "/api/v1/devices/$DEV/commands/$C3/approve" '{"reason":"admin approves"}' >/dev/null; check "admin approves engineer's command" "$(st)" 200
req PUT /api/v1/settings/policy '{"four_eyes_commands":false,"reason":"e2e off"}' >/dev/null; check "policy off" "$(st)" 200
req POST "/api/v1/devices/$DEV/commands" '{"method":"ping","args":{},"reason":"e2e"}' >/dev/null; check "direct command without four eyes" "$(st)" 201
grep -qE '"status":"(queued|sent)"' "$TMP/b" && ok "queued or sent directly" || bad "status: $(head -c 160 "$TMP/b")"

echo "== audit"
req GET "/api/v1/export/audit" >/dev/null
for a in organization.policy command.approve command.reject; do grep -q "$a" "$TMP/b" && ok "audit $a" || bad "audit $a missing"; done

echo "== cleanup"
req POST "/api/v1/devices/$DEV/revoke" '{"reason":"e2e cleanup"}' >/dev/null; check "device revoked" "$(st)" 200
req PATCH "/api/v1/users/$UID2" '{"disabled":true,"reason":"e2e cleanup"}' >/dev/null; check "second user disabled" "$(st)" 200

echo
echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

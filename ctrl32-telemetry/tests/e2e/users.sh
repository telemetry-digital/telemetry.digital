#!/usr/bin/env bash
# End-to-end checks for user, role and permission administration (URS H-04, H-05, I-01).
# Requires a running server with an admin without MFA (demo policy mfa_optional).
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/users.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
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
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  STATUS=$(cat "$TMP/s"); cat "$TMP/b"; }
SUF=$RANDOM

echo "== pages"
for p in /users /account /settings; do req GET $p >/dev/null; check "page $p served" "$(st)" 200; done
grep -q 'id="signout"' "$TMP/b" && bad "static page must not contain sign-out markup" || ok "no sign-out markup before session known"
req GET / >/dev/null; grep -q 'id="signinBtn"' "$TMP/b" && ok "single sign-in button on landing" || bad "sign-in button missing"
req GET /api/v1/roles >/dev/null; check "roles need auth" "$(st)" 401

echo "== login"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
ME=$(req GET /api/v1/auth/me); CSRF=$(echo "$ME" | jget csrf_token); MYID=$(echo "$ME" | jget user_id); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

echo "== catalog"
req GET /api/v1/permissions >/dev/null; check "permissions" "$(st)" 200
grep -q '"content.write"' "$TMP/b" && ok "catalog has content.write" || bad "catalog"
ROLES=$(req GET /api/v1/roles); check "roles" "$(st)" 200
for r in viewer operator qa engineer org_admin; do echo "$ROLES" | grep -q "\"name\":\"$r\"" && ok "built-in role $r" || bad "role $r missing"; done
echo "$ROLES" | grep -q '"builtin":true' && ok "builtin flag" || bad "builtin flag"
ADMIN_ROLE_ID=$(echo "$ROLES" | grep -o '{[^{}]*"name":"org_admin"[^{}]*}' | jget id)

echo "== organization roles"
ROLE=$(req POST /api/v1/roles "{\"name\":\"auditor_$SUF\",\"permissions\":[\"data.read\",\"audit.read\"],\"reason\":\"e2e\"}"); check "create role" "$(st)" 201
RID=$(echo "$ROLE" | jget id); [[ -n "$RID" ]] && ok "role id" || bad "role id: $ROLE"
req POST /api/v1/roles "{\"name\":\"auditor_$SUF\",\"permissions\":[],\"reason\":\"e2e\"}" >/dev/null; check "duplicate role rejected" "$(st)" 400
req POST /api/v1/roles "{\"name\":\"bad_$SUF\",\"permissions\":[\"nope\"],\"reason\":\"e2e\"}" >/dev/null; check "unknown permission rejected" "$(st)" 400
req POST /api/v1/roles "{\"name\":\"viewer\",\"permissions\":[],\"reason\":\"e2e\"}" >/dev/null; check "shadowing built-in rejected" "$(st)" 400
req PUT "/api/v1/roles/$RID" '{"permissions":["data.read","audit.read","data.export"],"reason":"e2e"}' >/dev/null; check "update role" "$(st)" 200
grep -q '"data.export"' "$TMP/b" && ok "permissions updated" || bad "permissions: $(cat "$TMP/b")"
req PUT "/api/v1/roles/$ADMIN_ROLE_ID" '{"permissions":[],"reason":"e2e"}' >/dev/null; check "built-in role immutable" "$(st)" 403
req PUT "/api/v1/roles/$RID" '{"permissions":[]}' >/dev/null; check "reason required on role update" "$(st)" 400

echo "== users"
U=$(req POST /api/v1/users "{\"username\":\"e2e_$SUF\",\"full_name\":\"E2E User\",\"email\":\"e2e$SUF@example.com\",\"roles\":[\"auditor_$SUF\"],\"reason\":\"e2e\"}"); check "create user" "$(st)" 201
UID_=$(echo "$U" | grep -o '"user":{[^}]*' | jget id); PW=$(echo "$U" | jget initial_password); [[ -n "$UID_" && -n "$PW" ]] && ok "user id + one-time password" || bad "user: $U"
D=$(req GET "/api/v1/users/$UID_"); check "user detail" "$(st)" 200
echo "$D" | grep -q '"audit.read"' && ok "effective permissions include audit.read" || bad "permissions: $D"
echo "$D" | grep -q '"role_history"' && ok "role history present" || bad "history"
echo "$D" | grep -q "\"event\":\"granted\"" && ok "grant recorded" || bad "grant"
req PATCH "/api/v1/users/$UID_" "{\"roles\":[\"viewer\"],\"full_name\":\"E2E Renamed\",\"reason\":\"e2e\"}" >/dev/null; check "patch roles + name" "$(st)" 200
grep -q '"E2E Renamed"' "$TMP/b" && ok "name updated" || bad "name"
D=$(req GET "/api/v1/users/$UID_"); echo "$D" | grep -q '"event":"revoked"' && ok "revoke recorded in history" || bad "revoke history"
echo "$D" | grep -q '"audit.read"' && bad "audit.read should be gone" || ok "permissions follow roles"
req PATCH "/api/v1/users/$MYID" '{"roles":["viewer"],"reason":"e2e"}' >/dev/null; check "cannot drop own org_admin" "$(st)" 400
req POST "/api/v1/users/$UID_/reset-password" '{"reason":"e2e"}' >/dev/null; check "admin password reset" "$(st)" 200
PW2=$(cat "$TMP/b" | jget initial_password); [[ -n "$PW2" ]] && ok "new one-time password" || bad "no password"
req POST "/api/v1/users/$UID_/mfa-reset" '{"reason":"e2e"}' >/dev/null; check "mfa reset" "$(st)" 200
req POST "/api/v1/users/$UID_/sessions/revoke" '{"reason":"e2e"}' >/dev/null; check "sessions revoke" "$(st)" 200
req POST "/api/v1/users/$UID_/unlock" '{"reason":"e2e"}' >/dev/null; check "unlock" "$(st)" 200
req POST "/api/v1/users/$UID_/unlock" '{}' >/dev/null; check "reason required on action" "$(st)" 400
req POST "/api/v1/users/00000000-0000-0000-0000-000000000000/unlock" '{"reason":"x"}' >/dev/null; check "unknown user 404" "$(st)" 404

echo "== disabled user cannot sign in"
req PATCH "/api/v1/users/$UID_" '{"disabled":true,"reason":"e2e"}' >/dev/null; check "disable" "$(st)" 200
JAR2="$TMP/jar2"
curl -s -c "$JAR2" -o "$TMP/b2" -w '%{http_code}' -H "Content-Type: application/json" --data "{\"username\":\"e2e_$SUF\",\"password\":\"$PW2\"}" "$BASE/api/v1/auth/login" > "$TMP/s2"
[[ "$(cat "$TMP/s2")" != "200" ]] && ok "disabled login refused ($(cat "$TMP/s2"))" || bad "disabled user logged in"
req PATCH "/api/v1/users/$UID_" '{"disabled":false,"reason":"e2e"}' >/dev/null; check "enable" "$(st)" 200
curl -s -c "$JAR2" -o "$TMP/b2" -w '%{http_code}' -H "Content-Type: application/json" --data "{\"username\":\"e2e_$SUF\",\"password\":\"$PW2\"}" "$BASE/api/v1/auth/login" > "$TMP/s2"
check "enabled user logs in with one-time password" "$(cat "$TMP/s2")" 200
grep -q '"must_change_password":true' "$TMP/b2" && ok "must change password" || bad "must_change: $(cat "$TMP/b2")"
curl -s -b "$JAR2" -o /dev/null -w '%{http_code}' "$BASE/api/v1/roles" > "$TMP/s2"; check "gated until password change" "$(cat "$TMP/s2")" 403
req PATCH "/api/v1/users/$UID_" '{"disabled":true,"reason":"e2e cleanup"}' >/dev/null; check "cleanup: disable test user" "$(st)" 200

echo "== audit entries"
req GET "/api/v1/export/audit?format=csv" >/dev/null
for a in role.create role.update user.create user.update user.password_reset user.mfa_reset user.sessions_revoke user.unlock; do grep -q "$a" "$TMP/b" && ok "audit $a" || bad "audit $a missing"; done

echo; echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

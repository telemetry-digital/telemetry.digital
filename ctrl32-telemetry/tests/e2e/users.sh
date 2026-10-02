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

echo "== server administration is separate from organization administration (ADR-0019)"
echo "$ROLES" | grep -q '"name":"server_admin"' && ok "built-in role server_admin" || bad "server_admin missing"
SA_ROLE=$(echo "$ROLES" | grep -o '{[^{}]*"name":"server_admin"[^{}]*}'); OA_ROLE=$(echo "$ROLES" | grep -o '{[^{}]*"name":"org_admin"[^{}]*}')
echo "$SA_ROLE" | grep -q '"system.admin"' && ok "server_admin holds system.admin" || bad "server_admin: $SA_ROLE"
echo "$OA_ROLE" | grep -q '"system.admin"' && bad "org_admin still holds system.admin" || ok "org_admin without system.admin"
echo "$OA_ROLE" | grep -q '"vpn.manage"' && ok "org_admin manages the organization's VPN" || bad "org_admin: $OA_ROLE"
echo "$ME" | grep -q '"system.admin"' && ok "the administrator keeps server access (server_admin)" || bad "admin lost system.admin"
JAR3="$TMP/jar3"; CSRF3=""
req3() { local m=$1 p=$2 d=${3:-}; printf '%s' "$d" > "$TMP/req3"
  curl -s -b "$JAR3" -c "$JAR3" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF3" -o "$TMP/b" -w '%{http_code}' ${d:+--data-binary @"$TMP/req3"} "$BASE$p" > "$TMP/s"; cat "$TMP/b"; }
OA=$(req POST /api/v1/users "{\"username\":\"oa_$SUF\",\"full_name\":\"Org Admin\",\"email\":\"oa$SUF@example.com\",\"roles\":[\"org_admin\"],\"reason\":\"e2e\"}"); check "organization administrator created" "$(st)" 201
OAID=$(echo "$OA" | grep -o '"user":{[^}]*' | jget id); OAPW=$(echo "$OA" | jget initial_password); OANEW="Users-e2e-$SUF-pass!"
req3 POST /api/v1/auth/login "{\"username\":\"oa_$SUF\",\"password\":\"$OAPW\"}" >/dev/null; CSRF3=$(req3 GET /api/v1/auth/me | jget csrf_token)
req3 POST /api/v1/auth/password "{\"current\":\"$OAPW\",\"new\":\"$OANEW\"}" >/dev/null
req3 POST /api/v1/auth/login "{\"username\":\"oa_$SUF\",\"password\":\"$OANEW\"}" >/dev/null; check "org admin signs in" "$(st)" 200
CSRF3=$(req3 GET /api/v1/auth/me | jget csrf_token)
req3 GET /api/v1/organizations >/dev/null; check "org admin: organizations refused" "$(st)" 403
req3 GET /api/v1/server >/dev/null; check "org admin: server refused" "$(st)" 403
req3 GET /api/v1/server/backups >/dev/null; check "org admin: backups refused" "$(st)" 403
req3 GET "/api/v1/export/organization?from=2026-01-01T00:00:00Z&to=2026-01-01T00:00:01Z" >/dev/null; check "org admin: organization export allowed" "$(st)" 200
req3 POST /api/v1/users "{\"username\":\"sa_$SUF\",\"full_name\":\"x\",\"email\":\"sa$SUF@example.com\",\"roles\":[\"server_admin\"],\"reason\":\"e2e\"}" >/dev/null; check "org admin cannot grant server_admin" "$(st)" 403
grep -q server_admin_required "$TMP/b" && ok "clear code" || bad "code: $(cat "$TMP/b")"
req3 POST /api/v1/roles "{\"name\":\"srv_$SUF\",\"permissions\":[\"system.admin\"],\"reason\":\"e2e\"}" >/dev/null; check "org admin cannot create a role with system.admin" "$(st)" 403
req3 POST "/api/v1/users/$MYID/reset-password" '{"reason":"e2e"}' >/dev/null; check "org admin cannot reset a server administrator's password" "$(st)" 403
req3 POST "/api/v1/users/$MYID/mfa-reset" '{"reason":"e2e"}' >/dev/null; check "org admin cannot reset a server administrator's MFA" "$(st)" 403
req3 PATCH "/api/v1/users/$MYID" '{"email":"takeover@example.com","reason":"e2e"}' >/dev/null; check "org admin cannot change a server administrator's account" "$(st)" 403
req3 PATCH "/api/v1/users/$OAID" '{"roles":["viewer"],"reason":"e2e"}' >/dev/null; check "server-side: own org_admin cannot be dropped" "$(st)" 400
req3 PATCH "/api/v1/users/$OAID" '{"disabled":true,"reason":"e2e"}' >/dev/null; check "server-side: cannot disable yourself" "$(st)" 400
req PATCH "/api/v1/users/$OAID" '{"roles":["org_admin","server_admin"]}' >/dev/null; check "granting server_admin needs a reason" "$(st)" 400
req PATCH "/api/v1/users/$OAID" '{"roles":["org_admin","server_admin"],"reason":"e2e second server administrator"}' >/dev/null; check "server administrator grants server_admin" "$(st)" 200
req3 GET /api/v1/organizations >/dev/null; check "now the organizations are reachable" "$(st)" 200
D=$(req GET "/api/v1/users/$OAID"); echo "$D" | grep -q '"role":"server_admin"' && echo "$D" | grep -q 'e2e second server administrator' && ok "grant with reason in the role history" || bad "history: $D"
req3 PATCH "/api/v1/users/$OAID" '{"roles":["org_admin"],"reason":"e2e"}' >/dev/null; check "nobody drops their own server_admin" "$(st)" 400
req PATCH "/api/v1/users/$OAID" '{"roles":["org_admin"],"reason":"e2e revoke"}' >/dev/null; check "server administrator revokes server_admin" "$(st)" 200
req3 GET /api/v1/organizations >/dev/null; check "organizations refused again" "$(st)" 403
req PATCH "/api/v1/users/$MYID" '{"roles":["org_admin"],"reason":"e2e"}' >/dev/null; check "own server_admin cannot be dropped" "$(st)" 400
req PATCH "/api/v1/users/$OAID" '{"disabled":true,"reason":"e2e cleanup"}' >/dev/null; check "cleanup: disable org admin" "$(st)" 200

echo "== audit entries"
req GET "/api/v1/export/audit?format=csv" >/dev/null
for a in role.create role.update user.create user.update user.password_reset user.mfa_reset user.sessions_revoke user.unlock; do grep -q "$a" "$TMP/b" && ok "audit $a" || bad "audit $a missing"; done

echo; echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

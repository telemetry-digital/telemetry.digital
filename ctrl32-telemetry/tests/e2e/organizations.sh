#!/usr/bin/env bash
# End-to-end checks for organizations as tenants (URS O-01..O-05).
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/organizations.sh
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
SUF=$RANDOM; SLUG="acme$SUF"; JAR2="$TMP/jar2"; CSRF2=""
req2() { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR2" -c "$JAR2" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF2" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR2" -c "$JAR2" -X "$m" -H "X-CSRF-Token: $CSRF2" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  cat "$TMP/b"; }

echo "== access"
req GET /organizations >/dev/null; check "page served" "$(st)" 200
req GET /api/v1/organizations >/dev/null; check "list needs auth" "$(st)" 401
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
ME=$(req GET /api/v1/auth/me); CSRF=$(echo "$ME" | jget csrf_token); MYORG=$(echo "$ME" | jget org_id); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

echo "== list"
L=$(req GET /api/v1/organizations); check "list" "$(st)" 200
echo "$L" | grep -q '"current":true' && ok "own organization flagged" || bad "current flag"
echo "$L" | grep -q '"admin_username":"' && ok "administrator resolved" || bad "admin_username"
echo "$L" | grep -q '"users":' && ok "user counts" || bad "users count"

echo "== create"
req POST /api/v1/organizations '{"slug":"Bad Slug","name":"x","reason":"e2e"}' >/dev/null; check "slug validated" "$(st)" 400
req POST /api/v1/organizations "{\"slug\":\"$SLUG\",\"name\":\"Acme $SUF\",\"tz\":\"Mars/Olympus\",\"reason\":\"e2e\"}" >/dev/null; check "time zone validated" "$(st)" 400
req POST /api/v1/organizations "{\"slug\":\"$SLUG\",\"name\":\"Acme $SUF\",\"admin_username\":\"$USER\",\"reason\":\"e2e\"}" >/dev/null; check "admin username must be free server-wide" "$(st)" 400
R=$(req POST /api/v1/organizations "{\"slug\":\"$SLUG\",\"name\":\"Acme $SUF\",\"tz\":\"Europe/Prague\",\"locale\":\"cs\",\"admin_full_name\":\"Acme Admin\",\"admin_email\":\"admin@$SLUG.example\",\"reason\":\"e2e\"}"); check "create organization" "$(st)" 201
OID=$(echo "$R" | grep -o '"organization":{[^}]*}' | jget id); APW=$(echo "$R" | jget initial_password); AUSER=$(echo "$R" | grep -o '"admin":{[^}]*}' | jget username)
[[ -n "$OID" && -n "$APW" ]] && ok "id and initial password returned" || bad "create response ($R)"
check "default admin username" "$AUSER" "$SLUG-admin"
req POST /api/v1/organizations "{\"slug\":\"$SLUG\",\"name\":\"Again\",\"admin_username\":\"other$SUF\",\"reason\":\"e2e\"}" >/dev/null; check "duplicate slug rejected" "$(st)" 400
req GET /api/v1/organizations >/dev/null; grep -q "\"slug\":\"$SLUG\"" "$TMP/b" && ok "new organization listed" || bad "listed"
grep -o "{[^{}]*\"slug\":\"$SLUG\"[^{}]*}" "$TMP/b" | grep -q "\"admin_username\":\"$AUSER\"" && ok "its administrator resolved" || bad "new admin resolved"

echo "== tenant isolation"
req2 POST /api/v1/auth/login "{\"username\":\"$AUSER\",\"password\":\"$APW\"}" >/dev/null; check "new admin signs in" "$(st)" 200
ME2=$(req2 GET /api/v1/auth/me); CSRF2=$(echo "$ME2" | jget csrf_token); ORG2=$(echo "$ME2" | jget org_id)
check "new admin lives in the new organization" "$ORG2" "$OID"
echo "$ME2" | grep -q '"must_change_password":true' && ok "must change the one-time password" || bad "must_change"
req2 POST /api/v1/auth/password "{\"current\":\"$APW\",\"new\":\"Acme-Strong-Passw0rd-$SUF\"}" >/dev/null; check "password changed" "$(st)" 200
req2 POST /api/v1/auth/login "{\"username\":\"$AUSER\",\"password\":\"Acme-Strong-Passw0rd-$SUF\"}" >/dev/null; check "re-login" "$(st)" 200
ME2=$(req2 GET /api/v1/auth/me); CSRF2=$(echo "$ME2" | jget csrf_token)
if echo "$ME2" | grep -q '"mfa_required":true'; then
  S=$(req2 POST /api/v1/auth/totp/setup '{}'); echo "$S" | grep -q '"qr":"data:image/png' && ok "new tenant admin gets the MFA QR" || bad "mfa qr"
  echo "  (tenant admin must finish MFA setup in a browser; data checks that need a full session are skipped)"
else
  req2 GET /api/v1/sites >/dev/null; check "new tenant sees its own (empty) sites" "$(st)" 200
  grep -q '^\[\]$' "$TMP/b" && ok "no sites leak from the demo organization" || bad "sites leaked: $(head -c 80 "$TMP/b")"
  req2 GET /api/v1/devices >/dev/null; grep -q '^\[\]$' "$TMP/b" && ok "no devices leak" || bad "devices leaked"
  req2 GET /api/v1/users >/dev/null; grep -q "\"username\":\"$USER\"" "$TMP/b" && bad "demo users visible to the tenant" || ok "users isolated"
fi

echo "== edit and disable"
req PATCH /api/v1/organizations/$OID '{"name":"Acme renamed","tz":"Europe/Vienna","locale":"de","reason":"e2e"}' >/dev/null; check "rename" "$(st)" 200
grep -q '"name":"Acme renamed"' "$TMP/b" && ok "renamed" || bad "renamed"
req PATCH /api/v1/organizations/$MYORG '{"disabled":true,"reason":"e2e"}' >/dev/null; check "own organization cannot be disabled" "$(st)" 400
req PATCH /api/v1/organizations/$OID '{"disabled":true,"reason":"e2e closed"}' >/dev/null; check "disable" "$(st)" 200
req2 GET /api/v1/auth/me >/dev/null; check "tenant sessions revoked" "$(st)" 401
req2 POST /api/v1/auth/login "{\"username\":\"$AUSER\",\"password\":\"Acme-Strong-Passw0rd-$SUF\"}" >/dev/null; check "tenant login refused while disabled" "$(st)" 401
req GET /api/v1/organizations >/dev/null; grep -o "{[^{}]*\"slug\":\"$SLUG\"[^{}]*}" "$TMP/b" | grep -q '"disabled_reason":"e2e closed"' && ok "disabled with reason" || bad "disabled_reason"
req PATCH /api/v1/organizations/$OID '{"disabled":false,"reason":"e2e"}' >/dev/null; check "enable" "$(st)" 200
req2 POST /api/v1/auth/login "{\"username\":\"$AUSER\",\"password\":\"Acme-Strong-Passw0rd-$SUF\"}" >/dev/null; check "tenant login works again" "$(st)" 200

echo "== reset administrator"
req POST /api/v1/organizations/$OID/reset-admin '{"username":"nobody","reason":"e2e"}' >/dev/null; check "unknown user 404" "$(st)" 404
R=$(req POST /api/v1/organizations/$OID/reset-admin "{\"username\":\"$AUSER\",\"reason\":\"e2e\"}"); check "reset" "$(st)" 200
NPW=$(echo "$R" | jget initial_password); [[ -n "$NPW" ]] && ok "new one-time password" || bad "password"
req2 POST /api/v1/auth/login "{\"username\":\"$AUSER\",\"password\":\"$NPW\"}" >/dev/null; check "login with the new password" "$(st)" 200
req2 GET /api/v1/auth/me >/dev/null; grep -q '"must_change_password":true' "$TMP/b" && ok "must change again" || bad "must_change after reset"
req GET "/api/v1/export/audit" >/dev/null; grep -q "organization.create" "$TMP/b" && ok "creation audited" || bad "audit"

echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

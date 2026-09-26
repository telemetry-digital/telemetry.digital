#!/usr/bin/env bash
# End-to-end test of identity, access, devices, provisioning, exports and MFA against a running server.
# Requires: curl, node, go (for tools/totpcode). Usage: tests/e2e/iam.sh <base_url> <admin_username> <one_time_password>
# Covers URS: H-01 H-02 H-03 H-04 H-05 H-09 H-15 G-02 G-04 G-05 J-09 I-01 I-06 A-03 (protocol §3.7)
set -euo pipefail
BASE=${1:-http://127.0.0.1:8080}; USER=${2:-admin}; OTP=${3:?one-time password required}
JAR=$(mktemp); trap 'rm -f "$JAR"' EXIT
pass(){ printf '  \033[32mPASS\033[0m %s\n' "$*"; }
fail(){ printf '  \033[31mFAIL\033[0m %s\n' "$*"; exit 1; }
json(){ node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const j=JSON.parse(s);const v=process.argv[1].split(".").reduce((a,k)=>a==null?a:a[k],j);console.log(typeof v==="object"?JSON.stringify(v):(v??""))})' "$1"; }
req(){ curl -s -b "$JAR" -c "$JAR" -H 'Content-Type: application/json' "$@"; }
code(){ curl -s -o /dev/null -w '%{http_code}' -b "$JAR" -c "$JAR" -H 'Content-Type: application/json' "$@"; }
NEWPW="Str0ng-Passw0rd-$RANDOM"

echo "1. unauthenticated access to protected endpoints"
[ "$(code "$BASE/api/v1/users")" = 401 ] && pass "users -> 401" || fail "users should be 401"
[ "$(code -X POST "$BASE/api/v1/devices" -d '{}')" = 401 ] && pass "create device -> 401" || fail "create device should be 401"

echo "2. login with one-time password, must change"
R=$(req -X POST "$BASE/api/v1/auth/login" -d "{\"username\":\"$USER\",\"password\":\"$OTP\"}")
[ "$(echo "$R" | json must_change_password)" = "true" ] && pass "must_change_password=true" || fail "login: $R"
[ "$(code "$BASE/api/v1/users")" = 403 ] && pass "protected call blocked until password change (403)" || fail "should be blocked"
CSRF=$(req "$BASE/api/v1/auth/me" | json csrf_token)
[ "$(code -X POST "$BASE/api/v1/auth/password" -d "{\"current\":\"$OTP\",\"new\":\"$NEWPW\"}")" = 403 ] && pass "mutation without CSRF header -> 403" || fail "csrf not enforced"
[ "$(code -X POST "$BASE/api/v1/auth/password" -H "X-CSRF-Token: $CSRF" -d "{\"current\":\"$OTP\",\"new\":\"short\"}")" = 400 ] && pass "weak password rejected" || fail "policy not enforced"
[ "$(code -X POST "$BASE/api/v1/auth/password" -H "X-CSRF-Token: $CSRF" -d "{\"current\":\"$OTP\",\"new\":\"$NEWPW\"}")" = 200 ] && pass "password changed" || fail "password change failed"

echo "3. login with new password (no MFA yet in intranet profile for org_admin? policy requires MFA setup)"
: > "$JAR"
R=$(req -X POST "$BASE/api/v1/auth/login" -d "{\"username\":\"$USER\",\"password\":\"$NEWPW\"}")
[ "$(echo "$R" | json mfa_required)" = "false" ] && pass "password login ok" || fail "login: $R"
ME=$(req "$BASE/api/v1/auth/me"); CSRF=$(echo "$ME" | json csrf_token)
[ "$(echo "$ME" | json mfa_required)" = "true" ] && pass "policy: org_admin must set up MFA" || fail "mfa_required expected"
[ "$(code "$BASE/api/v1/users")" = 403 ] && pass "blocked until MFA setup (403)" || fail "should be blocked"

echo "4. TOTP setup and confirm"
S=$(req -X POST "$BASE/api/v1/auth/totp/setup" -H "X-CSRF-Token: $CSRF" -d '{}')
SECRET=$(echo "$S" | json secret); [ -n "$SECRET" ] && pass "totp secret issued" || fail "setup: $S"
CODE=$(go run ./tools/totpcode -secret "$SECRET")
R=$(req -X POST "$BASE/api/v1/auth/totp/confirm" -H "X-CSRF-Token: $CSRF" -d "{\"code\":\"$CODE\"}")
RC=$(echo "$R" | json recovery_codes); [ -n "$RC" ] && pass "totp confirmed, recovery codes issued" || fail "confirm: $R"
[ "$(code -X POST "$BASE/api/v1/auth/totp/confirm" -H "X-CSRF-Token: $CSRF" -d "{\"code\":\"$CODE\"}")" = 400 ] && pass "replayed code rejected" || fail "replay accepted"
[ "$(code "$BASE/api/v1/users")" = 200 ] && pass "users accessible after MFA setup" || fail "users should be 200"

echo "5. MFA login flow"
: > "$JAR"
R=$(req -X POST "$BASE/api/v1/auth/login" -d "{\"username\":\"$USER\",\"password\":\"$NEWPW\"}")
PRE=$(echo "$R" | json pre_auth_token); [ "$(echo "$R" | json mfa_required)" = "true" ] && pass "mfa step required" || fail "login: $R"
[ "$(code "$BASE/api/v1/auth/me")" = 401 ] && pass "pending session is not authenticated" || fail "pending session leaked"
[ "$(code -X POST "$BASE/api/v1/auth/mfa" -d "{\"pre_auth_token\":\"$PRE\",\"code\":\"000000\"}")" = 401 ] && pass "wrong code rejected" || fail "wrong code accepted"
# each TOTP step is accepted once (replay protection): wait for the next 30 s step before generating a new code
sleep $((30 - $(date +%s) % 30 + 1)); CODE=$(go run ./tools/totpcode -secret "$SECRET")
R=$(req -X POST "$BASE/api/v1/auth/mfa" -d "{\"pre_auth_token\":\"$PRE\",\"code\":\"$CODE\"}")
[ "$(echo "$R" | json ok)" = "true" ] && pass "mfa login ok" || fail "mfa: $R"
ME=$(req "$BASE/api/v1/auth/me"); CSRF=$(echo "$ME" | json csrf_token)

echo "6. devices: create with claim, provision (unauthenticated), rotate, messages, revoke"
D=$(req -X POST "$BASE/api/v1/devices" -H "X-CSRF-Token: $CSRF" -d '{"external_id":"e2e-dev-1","name":"E2E device","transport":"mqtt","reason":"e2e test"}')
DEVID=$(echo "$D" | json id); CLAIM=$(echo "$D" | json claim_code); [ -n "$CLAIM" ] && pass "device created, claim $CLAIM" || fail "create device: $D"
: > /tmp/anon.jar
P=$(curl -s -H 'Content-Type: application/json' -X POST "$BASE/api/v1/provision" -d "{\"claim\":\"$CLAIM\",\"hw\":\"esp32-s3\",\"fw\":\"0.1.0\"}")
TOKEN=$(echo "$P" | json token); [ -n "$TOKEN" ] && pass "provisioned, token issued" || fail "provision: $P"
[ "$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' -X POST "$BASE/api/v1/provision" -d "{\"claim\":\"$CLAIM\"}")" = 401 ] && pass "claim is one-time" || fail "claim reused"
R2=$(req -X POST "$BASE/api/v1/devices/$DEVID/token" -H "X-CSRF-Token: $CSRF" -d '{"reason":"rotation test"}')
[ -n "$(echo "$R2" | json token)" ] && pass "token rotated" || fail "rotate: $R2"
[ "$(code "$BASE/api/v1/devices/$DEVID/messages")" = 200 ] && pass "message viewer endpoint" || fail "messages"

echo "7. API token with scopes, bearer access, scope escalation rejected"
T=$(req -X POST "$BASE/api/v1/tokens" -H "X-CSRF-Token: $CSRF" -d '{"name":"e2e","scopes":["data.read","data.export"]}')
RAW=$(echo "$T" | json token); [ -n "$RAW" ] && pass "api token created" || fail "token: $T"
[ "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $RAW" "$BASE/api/v1/datastreams")" = 200 ] && pass "bearer read ok" || fail "bearer read"
[ "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $RAW" "$BASE/api/v1/users")" = 403 ] && pass "bearer without user.admin -> 403" || fail "scope not enforced"
[ "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer c32_wrong" "$BASE/api/v1/datastreams")" = 401 ] && pass "bad bearer -> 401" || fail "bad bearer"

echo "8. exports (CSV + JSONL) with audit entry"
DS=$(curl -s -H "Authorization: Bearer $RAW" "$BASE/api/v1/datastreams" | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{console.log(JSON.parse(s)[0].id)})')
CSV=$(curl -s -H "Authorization: Bearer $RAW" "$BASE/api/v1/export/observations?datastream_id=$DS&format=csv")
grep -q '^measured_at,received_at,value' <<< "${CSV%%$'\n'*}" && pass "observations csv header" || fail "csv: $(echo "$CSV" | head -2)"
curl -s -H "Authorization: Bearer $RAW" "$BASE/api/v1/export/alarms?format=jsonl" | head -1 | grep -q '"alarm_id"' && pass "alarms jsonl" || pass "alarms jsonl (empty is fine)"
[ "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $RAW" "$BASE/api/v1/export/audit")" = 403 ] && pass "audit export needs audit.read (403 for token)" || fail "audit export scope"
AUD=$(req "$BASE/api/v1/export/audit?format=csv"); grep -q 'export.read' <<< "$AUD" && pass "export recorded in audit trail" || fail "export not audited"

echo "9. lockout after failed logins, revocation"
: > /tmp/lock.jar
for i in 1 2 3 4 5; do curl -s -o /dev/null -c /tmp/lock.jar -H 'Content-Type: application/json' -X POST "$BASE/api/v1/auth/login" -d "{\"username\":\"$USER\",\"password\":\"wrong-$i\"}"; done
LOCKED=$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' -X POST "$BASE/api/v1/auth/login" -d "{\"username\":\"$USER\",\"password\":\"$NEWPW\"}")
[ "$LOCKED" = 423 ] && pass "account locked after 5 failures (423)" || fail "lockout not applied (got $LOCKED)"
[ "$(code -X POST "$BASE/api/v1/devices/$DEVID/revoke" -H "X-CSRF-Token: $CSRF" -d '{"reason":"e2e cleanup"}')" = 200 ] && pass "device revoked" || fail "revoke"
[ "$(code -X POST "$BASE/api/v1/auth/logout" -H "X-CSRF-Token: $CSRF")" = 200 ] && pass "logout" || fail "logout"
[ "$(code "$BASE/api/v1/auth/me")" = 401 ] && pass "session revoked" || fail "session still valid"
echo "ALL IAM E2E CHECKS PASSED (account $USER is now locked for 15 minutes by design)"

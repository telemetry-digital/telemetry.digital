#!/usr/bin/env bash
# End-to-end checks for languages/translations (URS N-13) and self-service auth flows (forgot, reset, account requests, TOTP QR).
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/i18n_auth.sh
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

echo "== public pages and catalogs"
for p in /login /forgot /reset /register /settings/languages /settings/translations; do req GET $p >/dev/null; check "page $p served" "$(st)" 200; done
req GET /login >/dev/null; grep -q 'href="/forgot"' "$TMP/b" && ok "login links to forgot" || bad "forgot link"
grep -q 'href="/register"' "$TMP/b" && ok "login links to register" || bad "register link"
grep -q 'name="email"' "$TMP/b" && bad "login must not contain registration fields" || ok "login has only credentials"
L=$(req GET /api/v1/i18n/languages); check "languages public" "$(st)" 200
echo "$L" | grep -q '"code":"sk"' && ok "supported has sk" || bad "sk missing"
echo "$L" | grep -q '"code":"nl"' && ok "supported has nl" || bad "nl missing"
req GET /api/v1/i18n/sk >/dev/null; check "sk catalog" "$(st)" 200
grep -q '"Sign in":"' "$TMP/b" && ok "sk translates Sign in" || bad "sk catalog content"
req GET /api/v1/i18n/xx >/dev/null; check "unknown locale rejected" "$(st)" 404
req PUT /api/v1/i18n/languages '{"enabled":["en","sk"],"default":"en","reason":"x"}' >/dev/null; check "languages write needs auth" "$(st)" 401

echo "== self-service before login"
req POST /api/v1/auth/forgot '{"login":"nobody-'$SUF'"}' >/dev/null; check "forgot never reveals accounts" "$(st)" 200
grep -q '"mail_configured":' "$TMP/b" && ok "forgot reports mail availability" || bad "mail_configured"
req POST /api/v1/auth/reset '{"token":"bogus","password":"Whatever-Password-1"}' >/dev/null; check "bogus reset token rejected" "$(st)" 400
req POST /api/v1/auth/register-request '{"full_name":"","email":"x"}' >/dev/null; check "register validates" "$(st)" 400
req POST /api/v1/auth/register-request "{\"full_name\":\"Req $SUF\",\"email\":\"req$SUF@example.com\",\"message\":\"please\"}" >/dev/null; check "register request stored" "$(st)" 201
req GET /api/v1/account-requests >/dev/null; check "requests need auth" "$(st)" 401

echo "== login"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
ME=$(req GET /api/v1/auth/me); CSRF=$(echo "$ME" | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"
echo "$ME" | grep -q '"locale":' && ok "me carries locale" || bad "locale in me"

echo "== languages"
req PUT /api/v1/i18n/languages '{"enabled":["en","sk","cs","xx"],"default":"en","reason":"test"}' >/dev/null; check "unknown language rejected" "$(st)" 400
req PUT /api/v1/i18n/languages '{"enabled":["sk"],"default":"de","reason":"test"}' >/dev/null; check "default must be enabled" "$(st)" 400
req PUT /api/v1/i18n/languages '{"enabled":["en","sk","cs","de"],"default":"en","reason":"test"}' >/dev/null; check "enable languages" "$(st)" 200
L=$(req GET /api/v1/i18n/languages); echo "$L" | grep -q '"enabled":\["en","sk","cs","de"\]' && ok "enabled list stored" || bad "enabled list ($L)"
req PUT /api/v1/auth/locale '{"locale":"sk"}' >/dev/null; check "user locale sk" "$(st)" 200
req PUT /api/v1/auth/locale '{"locale":"fi"}' >/dev/null; check "user locale must be enabled" "$(st)" 400
req GET /api/v1/auth/me >/dev/null; grep -q '"locale":"sk"' "$TMP/b" && ok "me.locale updated" || bad "me.locale"
req PUT /api/v1/auth/locale '{"locale":"en"}' >/dev/null

echo "== translations"
E=$(req GET /api/v1/i18n/de/edit); check "edit view" "$(st)" 200
echo "$E" | grep -q '"key":"Sign in"' && ok "edit view lists keys" || bad "edit view keys"
req PUT /api/v1/i18n/de "{\"entries\":{\"Sign in\":\"Anmelden $SUF\",\"Custom key $SUF\":\"Eigener Text\"},\"reason\":\"test\"}" >/dev/null; check "override saved" "$(st)" 200
grep -q '"set":2' "$TMP/b" && ok "two entries set" || bad "set count"
req GET /api/v1/i18n/de >/dev/null; grep -q "\"Sign in\":\"Anmelden $SUF\"" "$TMP/b" && ok "override served" || bad "override served"
grep -q "\"Custom key $SUF\":\"Eigener Text\"" "$TMP/b" && ok "org-added key served" || bad "org key"
req PUT /api/v1/i18n/de "{\"entries\":{\"Sign in\":\"\",\"Custom key $SUF\":\"\"},\"reason\":\"cleanup\"}" >/dev/null; check "override removed" "$(st)" 200
grep -q '"removed":2' "$TMP/b" && ok "two entries removed" || bad "removed count"
req GET /api/v1/i18n/de >/dev/null; grep -q "Anmelden $SUF" "$TMP/b" && bad "override still served" || ok "fallback after removal"
req PUT /api/v1/i18n/de '{"entries":{"x":"y"}}' >/dev/null; check "reason required" "$(st)" 400
req GET "/api/v1/export/audit" >/dev/null; grep -q "translation.update" "$TMP/b" && ok "translation audited" || bad "audit"

echo "== account requests"
R=$(req GET /api/v1/account-requests); check "list requests" "$(st)" 200
RID=$(echo "$R" | grep -o "{[^{}]*req$SUF@example.com[^{}]*}" | jget id); [[ -n "$RID" ]] && ok "request listed" || bad "request listed"
req POST /api/v1/account-requests/$RID/dismiss '{"reason":"test"}' >/dev/null; check "dismiss" "$(st)" 200
req POST /api/v1/account-requests/$RID/dismiss '{"reason":"test"}' >/dev/null; check "dismiss twice is 404" "$(st)" 404
req GET /api/v1/account-requests >/dev/null; grep -q "req$SUF@example.com" "$TMP/b" && bad "dismissed still listed" || ok "dismissed hidden"

echo "== totp QR"
req POST /api/v1/auth/totp/setup '{}' >/dev/null; check "totp setup" "$(st)" 200
grep -q '"qr":"data:image/png;base64,' "$TMP/b" && ok "setup returns QR image" || bad "qr"
grep -q '"otpauth_url":"otpauth://totp/' "$TMP/b" && ok "setup returns otpauth url" || bad "otpauth"

echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

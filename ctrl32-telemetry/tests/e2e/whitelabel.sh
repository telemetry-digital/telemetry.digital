#!/usr/bin/env bash
# End-to-end checks for paid white labeling (spec 007): license keys verified offline (signature, server name),
# install/remove with audit, white-label settings refused without a license, product name, footer, icon and PWA
# manifest applied with it, and the fixed ctrl32 footer back when the license is removed. The server must trust the
# test key (tests/run-all.sh builds it with -X …/internal/license.extraKey=<tests/keys/license-test.pub>).
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/whitelabel.sh
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
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
pub()  { curl -s -o "$TMP/b" -w '%{http_code}' "$BASE$1" > "$TMP/s"; }
SUF=$RANDOM

echo "== setup"
go build -o "$TMP/licgen" ./tools/licgen || { echo "cannot build tools/licgen"; exit 1; }
issue() { "$TMP/licgen" issue -key "$1" -licensee "E2E $SUF" "${@:2}"; }
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req DELETE "/api/v1/server/license?reason=e2e"
req GET /api/v1/server/license; check "no license" "$(jx 'j.installed + " " + j.white_label')" "false false"
[[ "$(jx j.trusted_keys)" -ge 1 ]] && ok "test build trusts a licensor key" || bad "no trusted key — build the server with the test key (tests/keys/README.md)"

echo "== without a license"
req PUT /api/v1/branding '{"white_label":{"enabled":true,"product_name":"Acme"},"reason":"e2e"}'; check "enabling refused" "$(st)" 402
req PUT /api/v1/branding "{\"white_label\":{\"enabled\":false,\"product_name\":\"Acme $SUF\",\"short_name\":\"Acme\",\"footer_text\":\"© Acme $SUF\",\"footer_links\":[{\"label\":\"Support\",\"url\":\"mailto:help@acme.example\"}]},\"reason\":\"e2e prepare\"}"
check "settings can be prepared" "$(st)" 200
pub /api/v1/branding; check "fixed product footer" "$(jx 'j.product + " | " + j.footer.text + " | " + j.footer.fixed + " | " + j.white_label.licensed')" "ctrl32 telemetry | ctrl32 · telemetry | true | false"

echo "== license keys"
req PUT /api/v1/server/license '{"token":"c32l1.garbage.x","reason":"e2e"}'; check "garbage refused" "$(st)" 400
"$TMP/licgen" keygen -out "$TMP/k" -name rogue > /dev/null
req PUT /api/v1/server/license "{\"token\":\"$(issue "$TMP/k/rogue.key")\",\"reason\":\"e2e\"}"; check "foreign signature refused" "$(st)" 400
[[ "$(jx j.error)" == *signature* ]] && ok "reason: signature" || bad "error $(jx j.error)"
req PUT /api/v1/server/license "{\"token\":\"$(issue tests/keys/license-test.key -domains other.example)\",\"reason\":\"e2e\"}"; check "other server name refused" "$(st)" 400
TOK=$(issue tests/keys/license-test.key -days 30 -email e2e@example.com)
req PUT /api/v1/server/license "{\"token\":\"$TOK\"}"; check "reason required" "$(st)" 400
req PUT /api/v1/server/license "{\"token\":\"${TOK:0:30}\\n${TOK:30}\",\"reason\":\"e2e install\"}"; check "license installed (wrapped key)" "$(st)" 200
check "license valid with white label" "$(jx 'j.valid + " " + j.white_label + " " + j.licensee')" "true true E2E $SUF"

echo "== white label active"
req PUT /api/v1/branding '{"white_label":{"enabled":true,"product_name":"x","footer_links":[{"label":"x","url":"javascript:alert(1)"}]},"reason":"e2e"}'; check "script link refused" "$(st)" 400
req PUT /api/v1/branding '{"white_label":{"enabled":true,"product_name":"<b>x</b>"},"reason":"e2e"}'; check "markup refused" "$(st)" 400
BADSVG=$(printf '<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>' | base64 -w0)
req PUT /api/v1/branding "{\"white_label\":{\"enabled\":true,\"product_name\":\"Acme $SUF\",\"icon\":{\"mime\":\"image/svg+xml\",\"data\":\"$BADSVG\"}},\"reason\":\"e2e\"}"; check "icon with script refused" "$(st)" 400
ICON=$(printf '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><rect width="8" height="8" fill="#0f766e"/></svg>' | base64 -w0)
req PUT /api/v1/branding "{\"white_label\":{\"enabled\":true,\"product_name\":\"Acme $SUF\",\"short_name\":\"Acme\",\"footer_text\":\"© Acme $SUF\",\"intro\":\"Hello from Acme\",\"footer_links\":[{\"label\":\"Support\",\"url\":\"mailto:help@acme.example\"}],\"icon\":{\"mime\":\"image/svg+xml\",\"data\":\"$ICON\"}},\"reason\":\"e2e enable\"}"
check "white label enabled" "$(st)" 200
pub /api/v1/branding
check "product name" "$(jx j.product)" "Acme $SUF"
check "own footer" "$(jx 'j.footer.text + " | " + j.footer.white_label + " | " + j.footer.links.map(l => l.label).join() + " | " + j.footer.github + " | " + j.footer.intro')" "© Acme $SUF | true | Support |  | Hello from Acme"
[[ "$(jx j.icon_url)" == /api/v1/branding/icon* ]] && ok "icon URL" || bad "icon $(jx j.icon_url)"
check "icon served as SVG" "$(curl -s -o /dev/null -w '%{content_type}' "$BASE/api/v1/branding/icon")" "image/svg+xml"
curl -s "$BASE/manifest.webmanifest" > "$TMP/b"; check "PWA manifest" "$(jx 'j.name + " / " + j.short_name')" "Acme $SUF / Acme"
req GET /api/v1/branding/white-label; check "settings page state" "$(jx 'j.enabled + " " + j.licensed + " " + j.active + " " + j.icon')" "true true true true"
req PUT /api/v1/branding '{"white_label":{"enabled":true,"product_name":"Acme '$SUF'","short_name":"Acme","footer_text":"© Acme '$SUF'"},"reason":"e2e keep icon"}'
req GET /api/v1/branding/white-label; check "icon kept when not sent" "$(jx 'j.icon + " " + j.footer_links.length')" "true 0"

echo "== license removed"
req DELETE /api/v1/server/license; check "removal needs a reason" "$(st)" 400
req DELETE "/api/v1/server/license?reason=e2e%20remove"; check "license removed" "$(st)" 200
pub /api/v1/branding; check "fixed footer back" "$(jx 'j.product + " | " + j.footer.fixed + " | " + j.footer.white_label')" "ctrl32 telemetry | true | false"
curl -s "$BASE/manifest.webmanifest" > "$TMP/b"; check "manifest back" "$(jx j.name)" "ctrl32 telemetry"
check "icon falls back" "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/branding/icon")" 302
req GET /api/v1/branding/white-label; check "settings kept, not active" "$(jx 'j.enabled + " " + j.active')" "true false"

echo "== cleanup"
req PUT /api/v1/branding '{"white_label":{"enabled":false,"product_name":"","clear_icon":true},"reason":"e2e cleanup"}'; check "white label cleared" "$(st)" 200
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

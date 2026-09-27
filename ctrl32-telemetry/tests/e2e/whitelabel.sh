#!/usr/bin/env bash
# End-to-end checks for paid white labeling (spec 007): license keys verified offline (signature, server name,
# perpetual by default), install/remove with audit, white-label settings refused without a license, product name, footer, icon and PWA
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
issue() { "$TMP/licgen" issue -key "$1" -licensee "E2E $SUF" "${@:2}" 2>/dev/null; }
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
check "limited license has an expiry" "$(jx 'j.perpetual + " " + (j.expires ? "expires" : "none")')" "false expires"
req PUT /api/v1/server/license "{\"token\":\"$(issue tests/keys/license-test.key -email e2e@example.com)\",\"reason\":\"e2e perpetual\"}"; check "perpetual license installed (licgen default)" "$(st)" 200
check "perpetual: valid, no expiry" "$(jx 'j.valid + " " + j.perpetual + " " + (j.expires ? "expires" : "none")')" "true true none"

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

echo "== online activation (order number)"
# A local stand-in for lic.ctrl32.com (licfake.mjs); tests/run-all.sh points license.activation_url at it. Against a
# server configured otherwise the section is skipped (the fake then receives no request).
LICFAKE_PORT=${LICFAKE_PORT:-18790}; FAKE=http://127.0.0.1:$LICFAKE_PORT
LICFAKE_PORT=$LICFAKE_PORT node tests/e2e/licfake.mjs & FAKE_PID=$!
trap 'kill $FAKE_PID 2>/dev/null; rm -rf "$TMP"' EXIT
for _ in $(seq 40); do curl -s -o /dev/null "$FAKE/requests" && break; sleep 0.1; done
mode() { curl -s -o /dev/null -X POST -H 'Content-Type: application/json' --data "{\"order\":\"$1\",\"mode\":\"$2\",\"days\":${3:-30}}" "$FAKE/control"; }
fakereq() { curl -s "$FAKE/requests" > "$TMP/b"; jx "$1"; }
ORDER="E2E-ORDER-$SUF"
req DELETE "/api/v1/server/license?reason=e2e"
req POST /api/v1/server/license/activate "{\"order\":\"$ORDER\"}"; check "activation needs a reason" "$(st)" 400
req POST /api/v1/server/license/activate '{"order":"","reason":"e2e"}'; check "empty order refused" "$(st)" 400
mode "$ORDER" starter 10
req POST /api/v1/server/license/activate "{\"order\":\"$ORDER\",\"reason\":\"e2e activate\"}"; ACT=$(st)
if [[ "$(fakereq 'j.length')" == "0" ]]; then
  echo "  skipped: the server does not use the local activation stand-in (license.activation_url = $FAKE/telemetry/activate)"
else
  check "activated with the order number" "$ACT" 200
  check "request carried order and host" "$(fakereq 'j[j.length-1].order + " " + j[j.length-1].host')" "$ORDER localhost"
  req GET /api/v1/server/license
  check "30-day online license installed" "$(jx 'j.valid + " " + j.white_label + " " + j.online + " " + j.perpetual')" "true true true false"
  check "order stored, renewing" "$(jx 'j.activation.order + " " + j.activation.kind + " " + j.auto_renew + " " + j.activation.refused')" "$ORDER starter true false"
  check "buy link" "$(jx j.buy_url)" "https://ctrl32.com/pricing/"
  EXP1=$(jx j.expires)
  req PUT /api/v1/branding "{\"white_label\":{\"enabled\":true,\"product_name\":\"Online $SUF\"},\"reason\":\"e2e\"}"; check "white labeling unlocked by the activation" "$(st)" 200

  echo "== renewal"
  req POST /api/v1/server/license/renew '{}'; check "renew needs a reason" "$(st)" 400
  mode "$ORDER" starter 30
  req POST /api/v1/server/license/renew '{"reason":"e2e renew"}'; check "renewed (longer license)" "$(st) $(jx j.renewed)" "200 true"
  [[ "$(jx j.expires)" > "$EXP1" ]] && ok "expiry moved forward" || bad "expiry $(jx j.expires) not after $EXP1"
  mode "$ORDER" starter 20
  req POST /api/v1/server/license/renew '{"reason":"e2e renew shorter"}'; check "a shorter license is not installed" "$(st) $(jx j.renewed)" "200 false"
  mode "$ORDER" rogue
  req POST /api/v1/server/license/renew '{"reason":"e2e rogue"}'; check "perpetual license from the ONLINE key refused" "$(st)" 502
  [[ "$(jx j.error)" == *"online activation key"* ]] && ok "reason: online key limit" || bad "error $(jx j.error)"
  req GET /api/v1/server/license; check "still the 30-day license" "$(jx 'j.online + " " + j.perpetual + " " + j.activation.refused')" "true false false"
  mode "$ORDER" pending
  req POST /api/v1/server/license/renew '{"reason":"e2e pending"}'; check "service busy: temporary error" "$(st)" 502
  req GET /api/v1/server/license; check "temporary error keeps renewing" "$(jx 'j.auto_renew + " " + j.activation.refused + " " + (j.activation.last_error ? "error" : "none")')" "true false error"
  mode "$ORDER" permanent
  req POST /api/v1/server/license/renew '{"reason":"e2e permanent"}'; check "perpetual license from the master key installed" "$(st) $(jx j.renewed) $(jx j.perpetual)" "200 true true"
  check "no renewal needed any more" "$(jx 'j.auto_renew + " " + j.activation.kind + " " + (j.activation.last_error ? "error" : "none")')" "false permanent none"
  mode "$ORDER" starter 30
  req POST /api/v1/server/license/activate "{\"order\":\"$ORDER\",\"reason\":\"e2e downgrade\"}"; check "a starter does not replace a perpetual license" "$(st)" 409

  echo "== refusals"
  req DELETE "/api/v1/server/license?reason=e2e%20refusals"
  for m in refunded claimed-by-editor host-cap unknown-order; do
    mode "E2E-$m-$SUF" "$m"
    req POST /api/v1/server/license/activate "{\"order\":\"E2E-$m-$SUF\",\"reason\":\"e2e\"}"; check "refused: $m" "$(st)" 409
  done
  [[ "$(jx j.error)" == *"order number"* ]] && ok "unknown order explained" || bad "error $(jx j.error)"
  mode "E2E-claimed-by-editor-$SUF" claimed-by-editor
  req POST /api/v1/server/license/activate "{\"order\":\"E2E-claimed-by-editor-$SUF\",\"reason\":\"e2e\"}"
  [[ "$(jx j.error)" == *"editor"* ]] && ok "used for the editor explained" || bad "error $(jx j.error)"
  req GET /api/v1/server/license; check "nothing installed after refusals" "$(jx 'j.installed + " " + (j.activation ? "stored" : "none")')" "false none"
  mode "$ORDER-2" rogue
  req POST /api/v1/server/license/activate "{\"order\":\"$ORDER-2\",\"reason\":\"e2e rogue\"}"; check "rogue perpetual online license not installed" "$(st)" 502
  req GET /api/v1/server/license; check "still nothing installed" "$(jx j.installed)" "false"

  echo "== refusal stops the automatic renewal"
  mode "$ORDER-3" starter 15
  req POST /api/v1/server/license/activate "{\"order\":\"$ORDER-3\",\"reason\":\"e2e\"}"; check "activated" "$(st)" 200
  mode "$ORDER-3" refunded
  req POST /api/v1/server/license/renew '{"reason":"e2e refund"}'; check "renewal refused" "$(st)" 409
  req GET /api/v1/server/license
  check "refusal shown, renewal stopped, license kept until it expires" "$(jx 'j.activation.refused + " " + j.auto_renew + " " + j.valid')" "true false true"
  [[ "$(jx j.activation.last_error)" == *refunded* ]] && ok "refusal reason stored" || bad "last_error $(jx j.activation.last_error)"
  req GET "/api/v1/export/audit?format=csv"
  for a in license.activate license.renew; do grep -q "$a" "$TMP/b" && ok "audit $a" || bad "audit $a missing"; done
  req DELETE "/api/v1/server/license?reason=e2e%20remove%20activated"; check "removed" "$(st)" 200
  check "order number forgotten with the license" "$(jx 'j.installed + " " + (j.activation ? "stored" : "none")')" "false none"
  req POST /api/v1/server/license/renew '{"reason":"e2e"}'; check "renew without an order" "$(st)" 409
fi

echo "== cleanup"
req PUT /api/v1/branding '{"white_label":{"enabled":false,"product_name":"","clear_icon":true},"reason":"e2e cleanup"}'; check "white label cleared" "$(st)" 200
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

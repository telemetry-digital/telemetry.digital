#!/usr/bin/env bash
# End-to-end checks for installable app profiles (spec 021): create, update (new version), disable, delete with a
# reason and audit; strict validation (address, start page of this application only, known menu sections); the
# public install page, manifest and launch info (no data, 404 for unknown or disabled apps); the main manifest
# unchanged; permissions (content.write to manage); white-label naming when the server trusts the test license key;
# options of 0.61.0: locked app (no way into the full application), page zoom, camera layout — defaults, validation,
# kept on an update that leaves them out, in the launch info, install page of a locked app without the way out.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/pwa_apps.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; J2="$TMP/jar2"; CSRF=""; CSRF2=""
pass=0; fail=0
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
call() { local jar=$1 csrf=$2 m=$3 p=$4 d=${5:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$jar" -c "$jar" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $csrf" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$jar" -c "$jar" -X "$m" -H "X-CSRF-Token: $csrf" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
req()  { call "$JAR" "$CSRF" "$@"; }
req2() { call "$J2" "$CSRF2" "$@"; }
pub()  { curl -s -o "$TMP/b" -w '%{http_code}' "$BASE$1" > "$TMP/s"; }
SUF=$RANDOM
S="cams-$SUF"

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req POST /api/v1/video/walls "{\"name\":\"Gate wall $SUF\",\"reason\":\"e2e\",\"screens\":[{\"cols\":1,\"rows\":1,\"tiles\":[null]}]}"; WALL=$(jx j.id)
[[ -n "$WALL" && "$WALL" != null ]] && ok "video wall for the start page" || bad "no wall: $(head -c 200 "$TMP/b")"

echo "== options"
req GET /api/v1/pwa-apps/options; check "options" "$(st)" 200
check "menu sections" "$(jx 'j.sections.map(s => s.key).join()')" "dashboards,home,energy,counting,cameras,displays,flows,devices,incidents,assets,firmware,users,audit,settings,system"
check "the wall is a start page" "$(jx "j.start_pages.some(p => p.url === '/video/walls/$WALL' && p.group === 'Video walls')")" true
check "live view is a start page" "$(jx "j.start_pages.some(p => p.url === '/video')")" true

echo "== validation"
base() { printf '{"slug":"%s","name":"%s","short_name":"Cams","start_url":"%s","sections":%s,"reason":"%s"}' "$1" "$2" "$3" "$4" "${5-e2e}"; }
req POST /api/v1/pwa-apps "$(base "$S" Cameras /video '["cameras"]' '')"; check "reason required" "$(st)" 400
req POST /api/v1/pwa-apps "$(base "Bad Slug" Cameras /video '["cameras"]')"; check "bad address refused" "$(st)" 400
req POST /api/v1/pwa-apps "$(base options Cameras /video '["cameras"]')"; check "reserved address refused" "$(st)" 400
for u in 'https://evil.example/' '//evil.example/x' 'javascript:alert(1)' '/api/v1/devices' "/app/$S" '/video/../api/v1/users' '/video#x'; do
  req POST /api/v1/pwa-apps "$(base "$S" Cameras "$u" '["cameras"]')"; check "start page $u refused" "$(st)" 400
done
req POST /api/v1/pwa-apps "$(base "$S" Cameras /video '[]')"; check "no section refused" "$(st)" 400
req POST /api/v1/pwa-apps "$(base "$S" Cameras /video '["account"]')"; check "unknown section refused" "$(st)" 400
req POST /api/v1/pwa-apps "$(base "$S" '<b>x</b>' /video '["cameras"]')"; check "markup in the name refused" "$(st)" 400
req POST /api/v1/pwa-apps "{\"slug\":\"$S\",\"name\":\"Cameras\",\"start_url\":\"/video\",\"sections\":[\"cameras\"],\"theme_color\":\"red\",\"reason\":\"e2e\"}"; check "bad colour refused" "$(st)" 400

echo "== create (Cameras preset)"
req POST /api/v1/pwa-apps "{\"slug\":\"$S\",\"name\":\"Cameras $SUF\",\"short_name\":\"Cameras\",\"start_url\":\"/video\",\"sections\":[\"cameras\"],\"reason\":\"e2e create\"}"
check "created" "$(st)" 201
check "stored" "$(jx 'j.version + " " + j.enabled + " " + j.install_url + " " + j.warnings.length')" "1 true /app/$S 0"
check "options default: not locked, page zoom, automatic layout" "$(jx 'j.locked + " " + j.page_zoom + " " + j.camera_layout')" "false true auto"
req POST /api/v1/pwa-apps "{\"slug\":\"$S\",\"name\":\"Again\",\"start_url\":\"/video\",\"sections\":[\"cameras\"],\"reason\":\"e2e\"}"; check "same address twice refused" "$(st)" 409
req GET /api/v1/pwa-apps; check "listed" "$(jx "j.filter(a => a.slug === '$S').map(a => a.name).join()")" "Cameras $SUF"
req GET "/api/v1/pwa-apps/$S"; check "get" "$(st) $(jx j.start_url)" "200 /video"

echo "== public install page, manifest and launch"
pub "/app/$S"; check "install page" "$(st)" 200
grep -q "<link rel=\"manifest\" href=\"/app/$S/manifest.webmanifest\">" "$TMP/b" && ok "page links its own manifest" || bad "no manifest link"
grep -q '<meta name="apple-mobile-web-app-title" content="Cameras">' "$TMP/b" && ok "iPhone home-screen name" || bad "no apple-mobile-web-app-title"
grep -q "Cameras $SUF" "$TMP/b" && ok "app name shown" || bad "no app name"
grep -q 'vendor/qrcode.js\|app-install.js' "$TMP/b" && ok "install script (QR code)" || bad "no install script"
pub "/app/$S/manifest.webmanifest"; check "manifest" "$(st)" 200
check "manifest content type" "$(curl -s -o /dev/null -w '%{content_type}' "$BASE/app/$S/manifest.webmanifest")" "application/manifest+json"
check "manifest identity" "$(jx 'j.id + " " + j.start_url + " " + j.scope + " " + j.display')" "/app/$S /app/$S / standalone"
check "manifest names (no white label: product in the full name)" "$(jx 'j.name + " | " + j.short_name')" "Cameras $SUF · ctrl32 telemetry | Cameras"
check "shortcut to the section" "$(jx 'j.shortcuts.map(s => s.url).join()')" "/video"
check "product icons" "$(jx 'j.icons.length + " " + j.icons[0].src')" "3 /icon.svg"
pub "/api/v1/pwa-apps/$S/launch"; check "launch info without sign-in" "$(st)" 200
check "launch info" "$(jx 'j.start_url + " " + j.sections.join() + " " + Object.keys(j).sort().join()')" "/video cameras camera_layout,locked,name,page_zoom,sections,short_name,slug,start_url,theme_color"
pub "/app/$S"; grep -q 'href="/?full=1"' "$TMP/b" && ok "unlocked app: install page links the full application" || bad "unlocked app: no link to the full application"
pub "/app/nope-$SUF"; check "unknown app: 404 page" "$(st)" 404
pub "/app/nope-$SUF/manifest.webmanifest"; check "unknown app: no manifest" "$(st)" 404
pub "/api/v1/pwa-apps/nope-$SUF/launch"; check "unknown app: no launch info" "$(st)" 404
pub "/app/..%2f..%2fapi"; check "path tricks: 404" "$(st)" 404
pub /manifest.webmanifest; check "main manifest unchanged" "$(jx 'j.start_url + " " + j.scope + " " + (j.id === undefined) + " " + j.shortcuts.map(s => s.url).join()')" "/ / true /video,/video/walls,/home"
curl -s "$BASE/sw.js" > "$TMP/b"; grep -q "'/app/" "$TMP/b" && bad "service worker caches app pages" || ok "service worker never caches app pages"
check "manifest not cached stale" "$(curl -s -D - -o /dev/null "$BASE/app/$S/manifest.webmanifest" | grep -i '^cache-control' | tr -d '\r' | cut -d' ' -f2)" "no-cache"
check "install page not cached" "$(curl -s -D - -o /dev/null "$BASE/app/$S" | grep -i '^cache-control' | tr -d '\r' | cut -d' ' -f2)" "no-store"

echo "== locked phone app (Cameras preset: locked, no page zoom, one camera per row)"
K="kamery-$SUF"
req POST /api/v1/pwa-apps "{\"slug\":\"$K\",\"name\":\"Kamery $SUF\",\"start_url\":\"/video\",\"sections\":[\"cameras\"],\"locked\":true,\"page_zoom\":false,\"camera_layout\":\"list\",\"reason\":\"e2e\"}"; check "unknown camera layout refused" "$(st)" 400
req POST /api/v1/pwa-apps "{\"slug\":\"$K\",\"name\":\"Kamery $SUF\",\"start_url\":\"/video\",\"sections\":[\"cameras\"],\"locked\":\"yes\",\"reason\":\"e2e\"}"; check "locked must be a boolean" "$(st)" 400
req POST /api/v1/pwa-apps "{\"slug\":\"$K\",\"name\":\"Kamery $SUF\",\"short_name\":\"Kamery\",\"start_url\":\"/video\",\"sections\":[\"cameras\"],\"locked\":true,\"page_zoom\":false,\"camera_layout\":\"single\",\"reason\":\"e2e locked\"}"
check "locked app created" "$(st) $(jx 'j.locked + " " + j.page_zoom + " " + j.camera_layout + " " + j.warnings.length')" "201 true false single 0"
pub "/api/v1/pwa-apps/$K/launch"; check "launch info carries the options" "$(st) $(jx 'j.locked + " " + j.page_zoom + " " + j.camera_layout')" "200 true false single"
pub "/app/$K"; check "install page of the locked app" "$(st)" 200
grep -q '/?full=1' "$TMP/b" && bad "locked app: install page links the full application" || ok "locked app: install page has no way into the full application"
req PUT "/api/v1/pwa-apps/$K" "{\"name\":\"Kamery $SUF\",\"short_name\":\"Kamery\",\"start_url\":\"/video\",\"sections\":[\"cameras\"],\"reason\":\"e2e keep options\"}"
check "update without the options keeps them" "$(st) $(jx 'j.version + " " + j.locked + " " + j.page_zoom + " " + j.camera_layout')" "200 2 true false single"
req PUT "/api/v1/pwa-apps/$K" "{\"name\":\"Kamery $SUF\",\"start_url\":\"/video/walls/$WALL\",\"sections\":[\"cameras\"],\"camera_layout\":\"grid\",\"reason\":\"e2e wall start\"}"
check "wall start page is in the cameras section (no warning)" "$(st) $(jx 'j.camera_layout + " " + j.warnings.length')" "200 grid 0"
req PUT "/api/v1/pwa-apps/$K" "{\"name\":\"Kamery $SUF\",\"start_url\":\"/incidents\",\"sections\":[\"cameras\"],\"reason\":\"e2e outside\"}"
check "locked app with a start page outside its sections: warned" "$(st) $(jx 'j.warnings.length')" "200 1"
req PUT "/api/v1/pwa-apps/$K" "{\"name\":\"Kamery $SUF\",\"start_url\":\"/video\",\"sections\":[\"cameras\"],\"locked\":false,\"page_zoom\":true,\"camera_layout\":\"\",\"reason\":\"e2e unlock\"}"
check "unlocked, zoom back, empty layout keeps it" "$(st) $(jx 'j.locked + " " + j.page_zoom + " " + j.camera_layout')" "200 false true grid"
pub "/app/$K"; grep -q '/?full=1' "$TMP/b" && ok "unlocked again: the way out is back" || bad "unlocked again: no link to the full application"
req GET /api/v1/pwa-apps; check "list shows the options" "$(jx "j.filter(a => a.slug === '$K').map(a => a.locked + ' ' + a.page_zoom + ' ' + a.camera_layout).join()")" "false true grid"
req GET "/api/v1/export/audit?format=csv"; grep "pwa_app.update" "$TMP/b" | grep "e2e unlock" | grep -q "locked" && ok "options in the audit trail" || bad "options missing in the audit trail"
req DELETE "/api/v1/pwa-apps/$K?reason=e2e%20cleanup"; check "locked app deleted" "$(st)" 200

echo "== update (Video wall)"
req PUT "/api/v1/pwa-apps/$S" "{\"name\":\"Gate $SUF\",\"short_name\":\"Gate wall at entrance\",\"start_url\":\"/video/walls/$WALL\",\"sections\":[\"home\",\"cameras\",\"dashboards\",\"home\"],\"theme_color\":\"#0F766E\",\"reason\":\"e2e update\"}"
check "updated" "$(st)" 200
check "new version, sections in menu order" "$(jx 'j.version + " " + j.sections.join() + " " + j.theme_color + " " + j.enabled')" "2 dashboards,home,cameras #0F766E true"
check "long home-screen name warned" "$(jx 'j.warnings.length')" 1
pub "/app/$S/manifest.webmanifest"; check "manifest follows" "$(jx 'j.short_name + " | " + j.theme_color + " | " + j.shortcuts.map(s => s.url).join()')" "Gate wall at entrance | #0F766E | /dashboards,/home,/video"
req PUT "/api/v1/pwa-apps/$S" '{"name":"x","start_url":"/video","sections":["cameras"]}'; check "update needs a reason" "$(st)" 400
req PUT "/api/v1/pwa-apps/nope-$SUF" '{"name":"x","start_url":"/video","sections":["cameras"],"reason":"e2e"}'; check "update of an unknown app" "$(st)" 404

echo "== permissions"
U=$(req POST /api/v1/users "{\"username\":\"app_$SUF\",\"full_name\":\"App operator\",\"email\":\"app$SUF@example.com\",\"roles\":[\"operator\"],\"reason\":\"e2e\"}"; cat "$TMP/b")
UID2=$(echo "$U" | node -pe 'JSON.parse(require("fs").readFileSync(0)).user.id'); PW=$(echo "$U" | node -pe 'JSON.parse(require("fs").readFileSync(0)).initial_password')
req2 POST /api/v1/auth/login "{\"username\":\"app_$SUF\",\"password\":\"$PW\"}"
req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
NEWPW="App-Operator-Passw0rd-$SUF"
req2 POST /api/v1/auth/password "{\"current\":\"$PW\",\"new\":\"$NEWPW\"}"
req2 POST /api/v1/auth/login "{\"username\":\"app_$SUF\",\"password\":\"$NEWPW\"}"; check "operator signs in" "$(st)" 200
req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
req2 GET /api/v1/pwa-apps; check "operator cannot list apps" "$(st)" 403
req2 POST /api/v1/pwa-apps "{\"slug\":\"op-$SUF\",\"name\":\"x\",\"start_url\":\"/video\",\"sections\":[\"cameras\"],\"reason\":\"e2e\"}"; check "operator cannot create" "$(st)" 403
req2 PUT "/api/v1/pwa-apps/$S" '{"name":"x","start_url":"/video","sections":["cameras"],"reason":"e2e"}'; check "operator cannot change" "$(st)" 403
req2 DELETE "/api/v1/pwa-apps/$S?reason=e2e"; check "operator cannot delete" "$(st)" 403
req2 GET "/api/v1/pwa-apps/$S/launch"; check "operator's app window gets the launch info" "$(st) $(jx j.name)" "200 Gate $SUF"
curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d '{"slug":"anon","name":"x","start_url":"/video","sections":["cameras"],"reason":"x"}' "$BASE/api/v1/pwa-apps" > "$TMP/s"; check "anonymous cannot create" "$(st)" 401
req PATCH "/api/v1/users/$UID2" '{"disabled":true,"reason":"e2e cleanup"}'

echo "== white-label naming"
req GET /api/v1/server/license
if [[ "$(jx j.trusted_keys)" -ge 1 && "$(jx j.installed)" == false ]]; then
  go build -o "$TMP/licgen" ./tools/licgen && TOK=$("$TMP/licgen" issue -key tests/keys/license-test.key -licensee "E2E apps $SUF" 2>/dev/null)
  req PUT /api/v1/server/license "{\"token\":\"$TOK\",\"reason\":\"e2e apps\"}"; check "test license installed" "$(st)" 200
  ICON=$(printf '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><rect width="8" height="8" fill="#0f766e"/></svg>' | base64 -w0)
  req PUT /api/v1/branding "{\"white_label\":{\"enabled\":true,\"product_name\":\"Acme $SUF\",\"short_name\":\"Acme\",\"icon\":{\"mime\":\"image/svg+xml\",\"data\":\"$ICON\"}},\"reason\":\"e2e apps\"}"; check "white label on" "$(st)" 200
  pub "/app/$S/manifest.webmanifest"
  check "white label: the app's own name, the organization's icon" "$(jx 'j.name + " | " + j.icons.map(i => i.src).join() + " | " + j.description')" "Gate $SUF | /api/v1/branding/icon | Gate $SUF — Acme $SUF"
  pub "/app/$S"; grep -q "Acme $SUF" "$TMP/b" && ok "install page names the white-label product" || bad "install page without the product name"
  req PUT /api/v1/branding '{"white_label":{"enabled":false,"product_name":"","clear_icon":true},"reason":"e2e apps cleanup"}'
  req DELETE "/api/v1/server/license?reason=e2e%20apps%20cleanup"; check "test license removed" "$(st)" 200
  pub "/app/$S/manifest.webmanifest"; check "product back in the name" "$(jx j.name)" "Gate $SUF · ctrl32 telemetry"
else
  echo "  skip white-label naming (server does not trust the test key, or a license is installed)"
fi

echo "== disable and delete"
req PUT "/api/v1/pwa-apps/$S" "{\"name\":\"Gate $SUF\",\"start_url\":\"/video/walls/$WALL\",\"sections\":[\"cameras\"],\"enabled\":false,\"reason\":\"e2e disable\"}"; check "disabled" "$(st) $(jx j.enabled)" "200 false"
pub "/app/$S"; check "disabled app: 404 page" "$(st)" 404
pub "/api/v1/pwa-apps/$S/launch"; check "disabled app: no launch info" "$(st)" 404
req PUT "/api/v1/pwa-apps/$S" "{\"name\":\"Gate $SUF\",\"start_url\":\"/video/walls/$WALL\",\"sections\":[\"cameras\"],\"reason\":\"e2e keep state\"}"; check "enabled omitted keeps the state" "$(jx j.enabled)" false
req DELETE "/api/v1/pwa-apps/$S"; check "delete needs a reason" "$(st)" 400
req DELETE "/api/v1/pwa-apps/$S?reason=e2e%20delete"; check "deleted" "$(st)" 200
req GET "/api/v1/pwa-apps/$S"; check "gone" "$(st)" 404
req DELETE "/api/v1/pwa-apps/$S?reason=e2e"; check "second delete" "$(st)" 404
req POST /api/v1/pwa-apps "{\"slug\":\"$S\",\"name\":\"Cameras again\",\"start_url\":\"/video\",\"sections\":[\"cameras\"],\"reason\":\"e2e reuse\"}"; check "address free again" "$(st) $(jx j.version)" "201 1"
req DELETE "/api/v1/pwa-apps/$S?reason=e2e%20cleanup"

echo "== audit"
req GET "/api/v1/export/audit?format=csv"
check "create, update, retire audited with reasons" "$(grep -c "pwa_app.create\|pwa_app.update\|pwa_app.retire" "$TMP/b" | awk '{print ($1 >= 7)}')" 1
grep "pwa_app.update" "$TMP/b" | grep -q "e2e update" && ok "reason in the trail" || bad "reason missing in the trail"

echo "== cleanup"
req DELETE "/api/v1/video/walls/$WALL?reason=e2e"; check "wall removed" "$(st)" 200
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

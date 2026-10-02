#!/usr/bin/env bash
# End-to-end checks for the global search (0.68.0): GET /api/v1/search finds devices, datastreams, assets, sites,
# dashboards, flows, incidents and users of the organization by name — each kind only with the permission that shows
# it, limited per kind — and the pages it links to open the object (deep links).
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/search.sh
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
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  cat "$TMP/b"; }
tok()  { curl -s -o "$TMP/b" -w '%{http_code}' -H "Authorization: Bearer $1" "$BASE$2" > "$TMP/s"; cat "$TMP/b"; }
SUF=$RANDOM; Q="zq$SUF"

echo "== access"
req GET "/api/v1/search?q=x" >/dev/null; [[ "$(st)" == "401" || "$(st)" == "403" ]] && ok "search needs a session ($(st))" || bad "search needs a session (got $(st))"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

echo "== objects named $Q"
SITE=$(req POST /api/v1/sites "{\"name\":\"Site $Q\",\"address\":\"1 Search Street\",\"reason\":\"e2e\"}" | jget id)
ASSET=$(req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"fridge\",\"name\":\"Fridge $Q\",\"reason\":\"e2e\"}" | jget id)
DS=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"temp\",\"quantity\":\"temperature\",\"unit\":\"C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" | jget id)
DEV=$(req POST /api/v1/devices "{\"external_id\":\"ext-$Q\",\"name\":\"Logger $Q\",\"transport\":\"http\",\"reason\":\"e2e\"}" | jget id)
DASH=$(req POST /api/v1/dashboards "{\"name\":\"Board $Q\",\"spec\":{\"version\":1,\"widgets\":[]},\"reason\":\"e2e\"}" | jget slug)
INC=$(req POST /api/v1/incidents "{\"title\":\"Incident $Q\",\"severity\":\"minor\",\"reason\":\"e2e\"}" | jget id)
USR=$(req POST /api/v1/users "{\"username\":\"user$Q\",\"full_name\":\"Search Person $Q\",\"roles\":[\"viewer\"],\"reason\":\"e2e\"}" | grep -o '"user":{[^}]*}' | jget id)
for v in SITE ASSET DS DEV DASH INC USR; do [[ -n "${!v}" ]] || bad "created $v"; done; ok "site, asset, datastream, device, dashboard, incident and user created"
for i in 1 2 3 4 5 6 7; do req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"room\",\"name\":\"Room $i $Q\",\"reason\":\"e2e\"}" >/dev/null; done

echo "== search"
req GET "/api/v1/search?q=$Q" >/dev/null; check "search" "$(st)" 200
check "every kind found" "$(jx '["site","asset","datastream","device","dashboard","incident","user"].every(k=>j.results.some(r=>r.kind===k))')" true
check "device links its page" "$(jx "j.results.find(r=>r.kind==='device').url")" "/devices/$DEV"
check "datastream links the asset page" "$(jx "j.results.find(r=>r.kind==='datastream').url")" "/assets?site=$SITE&ds=$DS"
check "asset links its card" "$(jx "j.results.find(r=>r.kind==='asset' && r.id==='$ASSET').url")" "/assets?site=$SITE&asset=$ASSET"
check "dashboard, incident and user links" "$(jx "['dashboard','incident','user'].map(k=>j.results.find(r=>r.kind===k).url).join(' ')")" "/dashboards/$DASH /incidents?id=$INC /users?id=$USR"
check "at most 5 per kind by default, more announced" "$(jx "[j.results.filter(r=>r.kind==='asset').length, j.more.includes('asset')].join()")" "5,true"
check "hits of a kind ordered by name" "$(jx "j.results.filter(r=>r.kind==='asset')[0].title")" "Fridge $Q"
req GET "/api/v1/search?q=ext-$Q" >/dev/null; check "device by external id" "$(jx "j.results.map(r=>r.kind).join()")" "device"
req GET "/api/v1/search?q=Search%20Street" >/dev/null; check "site by address" "$(jx "j.results.some(r=>r.kind==='site' && r.id==='$SITE')")" true
req GET "/api/v1/search?q=$Q&kinds=asset&limit=20" >/dev/null; check "kinds and limit" "$(jx "[j.results.length, j.results.every(r=>r.kind==='asset'), j.more.length].join()")" "8,true,0"
req GET "/api/v1/search?q=50%25_$Q" >/dev/null; check "LIKE characters are text, not wildcards" "$(jx 'j.results.length')" 0
req GET "/api/v1/search?q=" >/dev/null; check "empty text refused" "$(st)" 400
req GET "/api/v1/search?q=x&limit=21" >/dev/null; check "limit 21 refused" "$(st)" 400

echo "== permissions"
TOK=$(req POST /api/v1/tokens '{"name":"search e2e","scopes":["data.read"]}' | jget token); check "token with data.read only" "$(st)" 201
tok "$TOK" "/api/v1/search?q=$Q" >/dev/null; check "data.read searches" "$(st)" 200
check "no users without user.admin" "$(jx "j.results.some(r=>r.kind==='user')")" false
check "data still found" "$(jx "j.results.some(r=>r.kind==='device')")" true
TOK2=$(req POST /api/v1/tokens '{"name":"search e2e 2","scopes":["token.manage"]}' | jget token)
tok "$TOK2" "/api/v1/search?q=$Q" >/dev/null; check "a token without data.read finds nothing" "$(jx 'j.results.length')" 0
req GET /api/v1/tokens >/dev/null; for n in "search e2e" "search e2e 2"; do TID=$(jx "j.find(t => t.name === '$n' && !t.revoked_at).id"); req DELETE "/api/v1/tokens/$TID" >/dev/null; done

echo "== deep links open the object"
for p in "/assets?site=$SITE&ds=$DS" "/incidents?id=$INC" "/users?id=$USR"; do req GET "$p" >/dev/null; check "page $p served" "$(st)" 200; done
req GET /app.js >/dev/null; grep -q "openSearch" "$TMP/b" && grep -q "/api/v1/search" "$TMP/b" && ok "Ctrl+K search in the page runtime" || bad "search runtime"
req DELETE "/api/v1/dashboards/$DASH?reason=e2e" >/dev/null; check "dashboard retired" "$(st)" 200

echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

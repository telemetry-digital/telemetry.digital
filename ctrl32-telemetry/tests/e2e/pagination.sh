#!/usr/bin/env bash
# Check of cursor pagination and range limits on observations (URS G-03) and of the generated clients (URS G-08):
# 25 backdated samples are read in pages of 10 through X-Next-Cursor, page boundaries are stable, bad cursors and
# oversized ranges are refused, the TypeScript client parses and the Bruno collection is complete.
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 USER=admin PASS=admin1234 bash tests/e2e/pagination.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
command -v go >/dev/null 2>&1 || export PATH="/d/tools/go/bin:$PATH"
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""
pass=0; fail=0
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jget() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$1" "$2"; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -D "$TMP/h" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -D "$TMP/h" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  cat "$TMP/b"; }
hdr()  { grep -i "^$1:" "$TMP/h" | head -1 | cut -d' ' -f2- | tr -d '\r'; }
SUF=$RANDOM

echo "== fixtures: 25 samples one minute apart"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token)
SITE=$(req POST /api/v1/sites "{\"name\":\"Page site $SUF\",\"reason\":\"e2e\"}" | jget id)
ASSET=$(req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"room\",\"name\":\"Page room $SUF\",\"reason\":\"e2e\"}" | jget id)
DS=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"t_c\",\"quantity\":\"temperature\",\"unit\":\"C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" | jget id)
DEV=$(req POST /api/v1/devices "{\"external_id\":\"pg-$SUF\",\"name\":\"paging\",\"reason\":\"e2e\"}" | jget id)
TOK=$(req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}' | jget token)
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$DS\",\"reason\":\"e2e\"}" >/dev/null; check "fixtures" "$(st)" 201
go run ./tools/devfill -addr "$MQTT" -user "pg-$SUF" -token "$TOK" -key t_c -start 10 -step 1 -n 25 -every 60 > "$TMP/pub.log" 2>&1 && ok "published 25 samples" || bad "publish: $(tail -2 "$TMP/pub.log")"
sleep 2
FROM=$(node -e 'console.log(new Date(Date.now()-3600e3).toISOString())'); TO=$(node -e 'console.log(new Date(Date.now()+60e3).toISOString())')

echo "== pages of 10"
req GET "/api/v1/observations?datastream_id=$DS&from=$FROM&to=$TO&limit=10" >/dev/null; check "page 1" "$(st)" 200
check "10 rows" "$(jx "$TMP/b" 'j.length')" 10
C1=$(hdr X-Next-Cursor); [[ -n "$C1" ]] && ok "X-Next-Cursor present" || bad "no cursor header: $(cat "$TMP/h")"
hdr Link | grep -q 'rel="next"' && ok "Link rel=next" || bad "link: $(hdr Link)"
V1=$(jx "$TMP/b" 'j.map(r=>r.value).join(",")')
req GET "/api/v1/observations?datastream_id=$DS&from=$FROM&to=$TO&limit=10&cursor=$C1" >/dev/null; check "page 2" "$(st)" 200
check "10 rows" "$(jx "$TMP/b" 'j.length')" 10
V2=$(jx "$TMP/b" 'j.map(r=>r.value).join(",")'); C2=$(hdr X-Next-Cursor); [[ -n "$C2" && "$C2" != "$C1" ]] && ok "second cursor" || bad "cursor 2"
req GET "/api/v1/observations?datastream_id=$DS&from=$FROM&to=$TO&limit=10&cursor=$C2" >/dev/null; check "page 3" "$(st)" 200
check "5 rows on the last page" "$(jx "$TMP/b" 'j.length')" 5
[[ -z "$(hdr X-Next-Cursor)" ]] && ok "no cursor after the last page" || bad "cursor on last page"
V3=$(jx "$TMP/b" 'j.map(r=>r.value).join(",")')
ALL="$V1,$V2,$V3"; N=$(echo "$ALL" | tr ',' '\n' | sort -u | wc -l | tr -d ' '); check "25 distinct values across the pages" "$N" 25
[[ "$(echo "$ALL" | cut -d, -f1)" == "10" && "$(echo "$ALL" | awk -F, '{print $NF}')" == "34" ]] && ok "ordered oldest to newest (10 … 34)" || bad "order: $ALL"

echo "== validation"
req GET "/api/v1/observations?datastream_id=$DS&cursor=not-a-cursor" >/dev/null; check "bad cursor refused" "$(st)" 400
req GET "/api/v1/observations?datastream_id=$DS&from=2020-01-01T00:00:00Z&to=2026-01-01T00:00:00Z" >/dev/null; check "raw range over 366 days refused" "$(st)" 400
req GET "/api/v1/observations?datastream_id=$DS&from=2020-01-01T00:00:00Z&to=2026-01-01T00:00:00Z&bucket=60" >/dev/null; check "too many buckets refused" "$(st)" 400
req GET "/api/v1/observations?datastream_id=$DS&from=2020-01-01T00:00:00Z&to=2026-01-01T00:00:00Z&bucket=86400" >/dev/null; check "6 years of daily buckets accepted" "$(st)" 200
req GET "/api/v1/observations?datastream_id=$DS&from=$TO&to=$FROM" >/dev/null; check "to before from refused" "$(st)" 400
req GET "/api/v1/observations?datastream_id=$DS&from=$FROM&to=$TO&bucket=600" >/dev/null; check "buckets" "$(st)" 200
[[ "$(jx "$TMP/b" 'j.points.reduce((n,p)=>n+p.n,0)')" == "25" ]] && ok "buckets cover all 25 rows" || bad "buckets: $(head -c 200 "$TMP/b")"

echo "== generated clients (G-08)"
node --check clients/typescript/ctrl32-api.ts && ok "TypeScript client parses" || bad "TypeScript client syntax"
grep -c "Promise<" clients/typescript/ctrl32-api.ts | awk '{ if ($1 > 160) exit 0; exit 1 }' && ok "client has a method per operation" || bad "client methods"
grep -q "getObservations(query: { datastream_id: string" clients/typescript/ctrl32-api.ts && ok "typed query parameters" || bad "typed query"
node -e 'JSON.parse(require("fs").readFileSync("clients/bruno/bruno.json","utf8"))' && ok "bruno.json valid" || bad "bruno.json"
NB=$(find clients/bruno -name '*.bru' | wc -l | tr -d ' '); [[ "$NB" -ge 165 ]] && ok "$NB Bruno requests" || bad "bruno files: $NB"
grep -q 'url: {{baseUrl}}/api/v1/devices/:id/commands' "clients/bruno/Commands/"*"Send a command"*.bru && ok "path parameters as :id" || bad "bruno path params"
go run ./tools/apigen -check > "$TMP/gen.log" 2>&1 && ok "clients are current" || bad "clients stale: $(cat "$TMP/gen.log")"

echo "== cleanup"
req POST "/api/v1/devices/$DEV/revoke" '{"reason":"e2e cleanup"}' >/dev/null; check "device revoked" "$(st)" 200
echo
echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

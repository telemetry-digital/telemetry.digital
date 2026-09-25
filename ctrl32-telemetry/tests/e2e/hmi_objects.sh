#!/usr/bin/env bash
# End-to-end checks for the HMI object library (spec 001, US6): SVG import with sanitising, serving headers,
# composites with tag placeholders, versions and retirement with reasons, audit trail, and public access only
# through a picture that uses the graphic.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/hmi_objects.sh
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
jx()   { node -e "const j=JSON.parse(require('fs').readFileSync('$1','utf8'));console.log($2)"; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  STATUS=$(cat "$TMP/s"); cat "$TMP/b"; }
SUF=$RANDOM

echo "== access"
code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/hmi/objects"); [[ "$code" == 401 || "$code" == 403 || "$code" == 200 ]] && ok "library list without session: $code (public_read decides)" || bad "anonymous list: $code"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token)

echo "== SVG import"
SVG='<svg xmlns="http://www.w3.org/2000/svg" width="120" height="60" onload="alert(1)"><script>alert(2)</script><rect width="120" height="60" fill="#1e3a8a" onclick="alert(3)"/><image href="https://evil.example/x.png"/><text x="10" y="35" fill="#fff">ACME</text></svg>'
BODY=$(node -e "console.log(JSON.stringify({name:'ACME logo $SUF',category:'Logos',svg:process.argv[1]}))" "$SVG")
req POST /api/v1/hmi/objects/import-svg "$BODY" >/dev/null; check "import needs a reason" "$(st)" 400
BODY=$(node -e "console.log(JSON.stringify({name:'ACME logo $SUF',category:'Logos',svg:process.argv[1],reason:'e2e'}))" "$SVG")
req POST /api/v1/hmi/objects/import-svg "$BODY" >/dev/null; check "import" "$(st)" 201
LOGO=$(jget slug < "$TMP/b")
check "size taken from the SVG" "$(jx "$TMP/b" 'j.body.width + "x" + j.body.height')" "120x60"
check "removed items reported" "$(jx "$TMP/b" 'j.removed.length >= 4')" "true"
check "no active content kept" "$(jx "$TMP/b" '/script|onload|onclick|evil|<image/i.test(j.body.svg) + ""')" "false"
code=$(curl -s -b "$JAR" -D "$TMP/h" -o "$TMP/svg" -w '%{http_code}' "$BASE/api/v1/hmi/objects/$LOGO/svg"); check "SVG served" "$code" 200
grep -qi '^content-type: image/svg+xml' "$TMP/h" && ok "content type image/svg+xml" || bad "content type"
grep -qi "^content-security-policy:.*sandbox" "$TMP/h" && ok "sandbox CSP" || bad "CSP header"
grep -qi '^x-content-type-options: nosniff' "$TMP/h" && ok "nosniff" || bad "nosniff"
req POST /api/v1/hmi/objects/import-svg "{\"name\":\"broken\",\"svg\":\"<svg><rect></svg>\",\"reason\":\"e2e\"}" >/dev/null; check "malformed SVG refused" "$(st)" 400
req POST /api/v1/hmi/objects/import-svg "{\"name\":\"html\",\"svg\":\"<html><svg/></html>\",\"reason\":\"e2e\"}" >/dev/null; check "non-SVG root refused" "$(st)" 400
req POST /api/v1/hmi/objects/import-svg "$BODY" >/dev/null; check "duplicate slug refused" "$(st)" 409

echo "== composites"
COMP="{\"name\":\"Pump station $SUF\",\"kind\":\"composite\",\"category\":\"Stations\",\"reason\":\"e2e\",\"body\":{\"width\":300,\"height\":200,\"tags\":[\"run\"],\"elements\":[{\"type\":\"symbol\",\"symbol\":\"pump_centrifugal\",\"x\":0,\"y\":0,\"w\":90,\"h\":84,\"tag\":\"\$run\",\"dyn\":[{\"prop\":\"state\",\"map\":[{\"op\":\"==\",\"v\":1,\"to\":\"run\"}]}]},{\"type\":\"symbol\",\"symbol\":\"valve_gate\",\"x\":120,\"y\":20,\"w\":80,\"h\":70}]}}"
req POST /api/v1/hmi/objects "$COMP" >/dev/null; check "composite created" "$(st)" 201
CSLUG=$(jget slug < "$TMP/b")
check "placeholder kept" "$(jx "$TMP/b" 'j.body.elements[0].tag')" '$run'
req POST /api/v1/hmi/objects "{\"name\":\"bad $SUF\",\"kind\":\"composite\",\"reason\":\"e2e\",\"body\":{\"width\":100,\"height\":100,\"tags\":[],\"elements\":[{\"type\":\"rect\",\"tag\":\"\$x\"}]}}" >/dev/null; check "unlisted placeholder refused" "$(st)" 400
req PUT "/api/v1/hmi/objects/$CSLUG" "{\"body\":{\"width\":300,\"height\":200,\"tags\":[],\"elements\":[{\"type\":\"rect\",\"x\":0,\"y\":0,\"w\":10,\"h\":10}]},\"reason\":\"e2e v2\"}" >/dev/null; check "new version" "$(st)" 200
check "version 2" "$(jx "$TMP/b" 'j.version')" 2
req GET "/api/v1/hmi/objects?kind=composite" >/dev/null; check "list by kind" "$(jx "$TMP/b" 'j.some(o => o.slug === "'$CSLUG'" && o.version === 2) + ""')" "true"

echo "== public access through a picture only"
SPEC="{\"kind\":\"mimic\",\"widgets\":[],\"elements\":[{\"type\":\"image\",\"src\":\"$LOGO\",\"x\":10,\"y\":10,\"w\":120,\"h\":60}]}"
PSLUG=$(req POST /api/v1/dashboards "{\"name\":\"Logo picture $SUF\",\"spec\":$SPEC,\"reason\":\"e2e\"}" | jget slug); check "picture with the logo" "$(st)" 201
req POST "/api/v1/dashboards/$PSLUG/share" '{"reason":"e2e"}' >/dev/null; PURL=$(jget url < "$TMP/b"); PTOK=${PURL##*/}
code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/public/$PTOK/hmi/objects/$LOGO/svg"); check "public link serves the used graphic" "$code" 200
code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/public/$PTOK/hmi/objects/$CSLUG/svg"); check "other library objects stay private" "$code" 404
req DELETE "/api/v1/dashboards/$PSLUG?reason=e2e" >/dev/null

echo "== retire and audit"
req DELETE "/api/v1/hmi/objects/$LOGO" >/dev/null; check "retire needs a reason" "$(st)" 400
req DELETE "/api/v1/hmi/objects/$LOGO?reason=e2e" >/dev/null; check "retired" "$(st)" 200
req GET "/api/v1/hmi/objects/$LOGO" >/dev/null; check "gone from the library" "$(st)" 404
req DELETE "/api/v1/hmi/objects/$CSLUG?reason=e2e" >/dev/null
req GET "/api/v1/export/audit?format=csv" >/dev/null
grep -q "hmi_object.create" "$TMP/b" && grep -q "hmi_object.update" "$TMP/b" && grep -q "hmi_object.retire" "$TMP/b" && ok "audit trail has create, update, retire" || bad "audit entries"

echo
echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

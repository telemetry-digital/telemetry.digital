#!/usr/bin/env bash
# Check of the OpenAPI description (URS G-01): the document is served without sign-in, is OpenAPI 3.1, describes
# every /api/v1 route with a summary, tag and permission, all $ref targets exist, and the reference page renders.
#   BASE=http://127.0.0.1:8080 bash tests/e2e/openapi.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
pass=0; fail=0
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$1" "$2"; }

echo "== document"
code=$(curl -s -o "$TMP/spec.json" -w '%{http_code}' "$BASE/api/v1/openapi.json"); check "served without sign-in" "$code" 200
check "OpenAPI 3.1" "$(jx "$TMP/spec.json" 'j.openapi')" "3.1.0"
[[ "$(jx "$TMP/spec.json" 'j.info.title')" == "ctrl32 telemetry API" ]] && ok "title" || bad "title"
N=$(jx "$TMP/spec.json" 'Object.values(j.paths).reduce((n,p)=>n+Object.keys(p).length,0)'); [[ "$N" -ge 160 ]] && ok "$N operations described" || bad "only $N operations"
check "every operation has a summary" "$(jx "$TMP/spec.json" 'Object.values(j.paths).flatMap(p=>Object.values(p)).filter(o=>!o.summary).length')" 0
check "every operation has a tag" "$(jx "$TMP/spec.json" 'Object.values(j.paths).flatMap(p=>Object.values(p)).filter(o=>!o.tags||!o.tags.length).length')" 0
check "dangling \$ref" "$(jx "$TMP/spec.json" 'const s=JSON.stringify(j); [...s.matchAll(/"\$ref":"#\/components\/schemas\/([A-Za-z]+)"/g)].filter(m=>!j.components.schemas[m[1]]).length')" 0
check "security schemes" "$(jx "$TMP/spec.json" 'Object.keys(j.components.securitySchemes).sort().join(",")')" "bearerToken,cameraSecret,connectorSecret,deviceToken,sessionCookie"
check "permission on a protected operation" "$(jx "$TMP/spec.json" 'j.paths["/api/v1/rules"].post["x-permission"]')" "config.write"
check "public operation has empty security" "$(jx "$TMP/spec.json" 'JSON.stringify(j.paths["/api/v1/auth/login"].post.security)')" "[]"
check "path parameter derived" "$(jx "$TMP/spec.json" 'j.paths["/api/v1/devices/{id}/commands/{cid}/approve"].post.parameters.map(p=>p.name).join(",")')" "id,cid"
check "request body documented" "$(jx "$TMP/spec.json" 'j.paths["/api/v1/devices/{id}/commands"].post.requestBody.content["application/json"].schema.required.join(",")')" "method,reason"
check "rule spec schema" "$(jx "$TMP/spec.json" 'j.components.schemas.RuleAction.properties.type.enum.join(",")')" "notify,webhook,command,alarm,derive"
check "SSE documented" "$(jx "$TMP/spec.json" 'Object.keys(j.paths["/api/v1/stream"].get.responses["200"].content)[0]')" "text/event-stream"
check "operationId unique" "$(jx "$TMP/spec.json" 'const ids=Object.values(j.paths).flatMap(p=>Object.values(p)).map(o=>o.operationId); new Set(ids).size===ids.length')" true

echo "== reference page"
code=$(curl -s -o "$TMP/docs.html" -w '%{http_code}' "$BASE/api/docs"); check "page served" "$code" 200
grep -q 'openapi.json' "$TMP/docs.html" && ok "page loads the document" || bad "page content"

echo
echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

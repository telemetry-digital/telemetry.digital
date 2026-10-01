#!/usr/bin/env bash
# End-to-end checks for AI assistants over MCP (spec 022): discovery from the 401 of /mcp (RFC 9728, RFC 8414), dynamic
# client registration, consent in the browser session, PKCE S256, single-use codes, access tokens valid only at /mcp,
# rotating refresh tokens, revocation; the MCP protocol (initialize, tools/list, tools/call, resources); every change
# through the API's own handlers with a reason and "via MCP" in the audit trail; read-only approval; a user's
# permissions limit the tools; switching AI access off stops connected assistants; personal API tokens work too.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/mcp.sh
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
# form POST without cookies (token endpoint)
form() { curl -s -D "$TMP/h" -o "$TMP/b" -w '%{http_code}' -X POST "$BASE$1" "${@:2}" > "$TMP/s"; }
# one JSON-RPC message to /mcp with a bearer token
ID=0
rpc()  { ID=$((ID+1)); local tok=$1 method=$2 params=${3:-'{}'}
  printf '{"jsonrpc":"2.0","id":%d,"method":"%s","params":%s}' "$ID" "$method" "$params" > "$TMP/req"
  curl -s -D "$TMP/h" -H "Authorization: Bearer $tok" -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" \
    -H "MCP-Protocol-Version: 2025-06-18" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE/mcp" > "$TMP/s"; }
tool() { rpc "$1" tools/call "{\"name\":\"$2\",\"arguments\":$3}"; }
text() { jx 'j.result.content[0].text'; }
SUF=$RANDOM
REDIR="http://127.0.0.1:$((20000 + SUF % 10000))/callback"
pkce() { VER=$(node -e 'console.log(require("crypto").randomBytes(48).toString("base64url"))'); CH=$(V="$VER" node -e 'console.log(require("crypto").createHash("sha256").update(process.env.V).digest("base64url"))'); }
qs()   { A1="$1" A2="$2" A3="$3" A4="$4" A5="$5" node -e 'const e=process.env; console.log(new URLSearchParams({response_type:"code",client_id:e.A1,redirect_uri:e.A2,code_challenge:e.A3,code_challenge_method:"S256",state:e.A4,scope:e.A5,resource:e.BASE+"/mcp"}).toString())'; }
param(){ U="$1" P="$2" node -e 'console.log(new URL(process.env.U).searchParams.get(process.env.P) || "")'; }
export BASE

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req PUT /api/v1/mcp/settings '{"enabled":false,"reason":"e2e mcp: start switched off"}'; check "AI access off (known state)" "$(st) $(jx j.enabled)" "200 false"

echo "== discovery"
curl -s -D "$TMP/h" -o "$TMP/b" -w '%{http_code}' -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' "$BASE/mcp" > "$TMP/s"
check "/mcp without a token: 401" "$(st)" 401
grep -qi 'www-authenticate: Bearer resource_metadata=".*/.well-known/oauth-protected-resource/mcp"' "$TMP/h" && ok "401 names the resource metadata" || bad "WWW-Authenticate: $(grep -i www-auth "$TMP/h")"
curl -s -o "$TMP/b" -w '%{http_code}' "$BASE/.well-known/oauth-protected-resource/mcp" > "$TMP/s"
check "resource metadata" "$(st) $(jx 'j.resource === j.authorization_servers[0] + String.fromCharCode(47) + "mcp" && j.authorization_servers.length === 1')" "200 true"
AS=$(jx 'j.authorization_servers[0]'); export AS
curl -s -o "$TMP/b" -w '%{http_code}' "$BASE/.well-known/oauth-authorization-server" > "$TMP/s"
check "authorization server metadata" "$(st) $(jx 'j.issuer === process.env.AS && j.code_challenge_methods_supported.join() === "S256" && j.token_endpoint_auth_methods_supported.join() === "none"')" "200 true"
check "endpoints" "$(jx '["authorization_endpoint","token_endpoint","registration_endpoint","revocation_endpoint"].every(k => j[k].startsWith(j.issuer + "/oauth/"))')" true

echo "== registration"
form /oauth/register -H "Content-Type: application/json" --data "{\"client_name\":\"Evil\",\"redirect_uris\":[\"http://evil.example/cb\"]}"
check "http redirect outside loopback refused" "$(st) $(jx j.error)" "400 invalid_redirect_uri"
form /oauth/register -H "Content-Type: application/json" --data "{\"client_name\":\"Evil\",\"redirect_uris\":[\"https://x.example/cb\"],\"token_endpoint_auth_method\":\"client_secret_basic\"}"
check "confidential clients refused" "$(st) $(jx j.error)" "400 invalid_client_metadata"
form /oauth/register -H "Content-Type: application/json" --data "{\"client_name\":\"E2E Assistant $SUF\",\"redirect_uris\":[\"$REDIR\"],\"grant_types\":[\"authorization_code\",\"refresh_token\"],\"response_types\":[\"code\"],\"token_endpoint_auth_method\":\"none\",\"client_uri\":\"https://assistant.example\"}"
check "client registered" "$(st) $(jx 'j.client_id.startsWith("c32c_") && j.token_endpoint_auth_method')" "201 none"
CID=$(jx j.client_id)

echo "== consent while AI access is off"
pkce; Q=$(qs "$CID" "$REDIR" "$CH" "st-$SUF" mcp)
curl -s -o "$TMP/b" -w '%{http_code}' "$BASE/oauth/authorize?$Q" > "$TMP/s"; check "consent page served" "$(st) $(grep -c '<h1>Connect an AI assistant</h1>' "$TMP/b")" "200 1"
req GET "/api/v1/mcp/authorize?$Q"; check "consent info: off, the host the code goes to" "$(st) $(jx 'j.client_name + " | " + j.enabled + " | " + j.redirect_host')" "200 E2E Assistant $SUF | false | 127.0.0.1:$((20000 + SUF % 10000))"
DEC() { printf '{"response_type":"code","client_id":"%s","redirect_uri":"%s","code_challenge":"%s","code_challenge_method":"S256","state":"%s","scope":"%s","resource":"%s/mcp","approve":%s,"read_only":%s}' "$CID" "$REDIR" "$1" "$2" "$3" "$BASE" "$4" "$5"; }
req POST /api/v1/mcp/authorize "$(DEC "$CH" "st-$SUF" mcp true false)"; check "approval refused while off" "$(st)" 403
req PUT /api/v1/mcp/settings '{"enabled":true}'; check "switching on needs a reason" "$(st)" 400
req PUT /api/v1/mcp/settings '{"enabled":true,"reason":"e2e mcp: allow assistants"}'; check "AI access on" "$(st) $(jx 'j.enabled + " " + j.url.endsWith("/mcp") + " " + (j.operations > 280)')" "200 true true true"

echo "== authorization request validation"
req GET "/api/v1/mcp/authorize?$(qs "$CID" "http://127.0.0.1:1/other" "$CH" s mcp)"; check "unregistered redirect path: shown on the page" "$(st)" 400
req GET "/api/v1/mcp/authorize?$(qs c32c_unknownunknownunknown "$REDIR" "$CH" s mcp)"; check "unknown client" "$(st)" 400
req GET "/api/v1/mcp/authorize?$(qs "$CID" "$REDIR" "short" s mcp)"; check "PKCE required: error goes back to the client" "$(st) $(jx j.error) $(param "$(jx j.redirect)" error)" "200 invalid_request invalid_request"
req GET "/api/v1/mcp/authorize?$(qs "$CID" "$REDIR" "$CH" s admin)"; check "unknown scope" "$(jx j.error)" "invalid_scope"
curl -s -H "Authorization: Bearer c32_x" -o "$TMP/b" -w '%{http_code}' "$BASE/api/v1/mcp/authorize?$Q" > "$TMP/s"; check "consent info needs a browser session" "$(st)" 401

echo "== deny"
req POST /api/v1/mcp/authorize "$(DEC "$CH" "deny-$SUF" mcp false false)"
R=$(jx j.redirect); check "deny → access_denied with state and iss" "$(param "$R" error) $(param "$R" state) $([[ -n "$(param "$R" iss)" ]] && echo iss)" "access_denied deny-$SUF iss"

echo "== approve and exchange the code"
req POST /api/v1/mcp/authorize "$(DEC "$CH" "st-$SUF" mcp true false)"; check "approved" "$(st)" 200
R=$(jx j.redirect); CODE=$(param "$R" code)
check "redirect carries code, state and issuer" "$([[ ${#CODE} -gt 30 ]] && echo code) $(param "$R" state) $(param "$R" iss)" "code st-$SUF $AS"
form /oauth/token -d grant_type=authorization_code -d "code=$CODE" -d "client_id=$CID" -d "redirect_uri=$REDIR" -d "code_verifier=wrong-verifier-wrong-verifier-wrong-verifier-000"
check "wrong verifier" "$(st) $(jx j.error)" "400 invalid_grant"
form /oauth/token -d grant_type=authorization_code -d "code=$CODE" -d "client_id=c32c_otherotherotherother" -d "redirect_uri=$REDIR" -d "code_verifier=$VER"
check "other client" "$(st) $(jx j.error)" "400 invalid_grant"
form /oauth/token -d grant_type=authorization_code -d "code=$CODE" -d "client_id=$CID" -d "redirect_uri=$REDIR" -d "code_verifier=$VER" -d "resource=$BASE/mcp"
check "tokens issued" "$(st) $(jx 'j.token_type + " " + j.expires_in + " " + j.scope + " " + j.access_token.startsWith("c32_") + " " + j.refresh_token.startsWith("c32r_")')" "200 Bearer 3600 mcp true true"
AT=$(jx j.access_token); RT=$(jx j.refresh_token); export AT
grep -qi "cache-control: no-store" "$TMP/h" && ok "token answers are not cached" || bad "cache-control of the token endpoint"

echo "== reading is free, changes need the license"
req GET "/api/v1/mcp/authorize?$Q"; check "consent says whether changes are licensed" "$(jx 'typeof j.write_licensed')" boolean
req GET /api/v1/server/license; LIC_OURS=""
if [[ "$(jx 'j.white_label && j.valid')" != true ]]; then
  rpc "$AT" tools/list; check "without the license: no change tools" "$(jx 'j.result.tools.some(t => t.name.endsWith("_change"))') $(jx 'j.result.tools.some(t => t.name === "sites_read")')" "false true"
  tool "$AT" sites_change '{"operation":"postSites","body":{"name":"x","reason":"e2e"}}'; check "without the license: a change is refused with the reason" "$(jx 'j.result.isError') $(text | grep -c 'license of this server')" "true 1"
  rpc "$AT" resources/read '{"uri":"ctrl32://me"}'; check "resource says changes are not licensed" "$(jx 'JSON.parse(j.result.contents[0].text).changes_licensed')" false
  req GET /api/v1/server/license
  if [[ "$(jx j.trusted_keys)" -ge 1 && "$(jx j.installed)" == false ]]; then
    go build -o "$TMP/licgen" ./tools/licgen && LTOK=$("$TMP/licgen" issue -key tests/keys/license-test.key -licensee "E2E mcp $SUF" 2>/dev/null)
    req PUT /api/v1/server/license "{\"token\":\"$LTOK\",\"reason\":\"e2e mcp\"}"; check "test license installed" "$(st)" 200; LIC_OURS=1
  else
    bad "changes through MCP need a license: run against a server that trusts the test license key (tests/run-all.sh builds it)"
  fi
fi
req GET /api/v1/mcp/settings; check "changes licensed" "$(jx j.write_licensed)" true

echo "== MCP protocol"
rpc "$AT" initialize '{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"e2e","version":"1"}}'
check "initialize" "$(st) $(jx 'j.result.protocolVersion + " " + j.result.serverInfo.name + " " + (j.result.instructions.includes("reason")) + " " + !!j.result.capabilities.tools')" "200 2025-06-18 ctrl32-telemetry true true"
curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $AT" -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"notifications/initialized"}' "$BASE/mcp" > "$TMP/s"; check "notification accepted" "$(st)" 202
rpc "$AT" tools/list
check "tools of an administrator" "$(jx '[j.result.tools.length > 40, j.result.tools.some(t => t.name === "cameras_read"), j.result.tools.some(t => t.name === "devices_change"), j.result.tools.some(t => t.name === "api_reference")].join()')" "true,true,true,true"
check "read tools are marked read-only, change tools destructive" "$(jx 'j.result.tools.every(t => t.name.endsWith("_read") ? t.annotations.readOnlyHint && !t.annotations.destructiveHint : (t.name.endsWith("_change") ? t.annotations.destructiveHint && !t.annotations.readOnlyHint : true))')" true
check "no tool manages AI access or API tokens" "$(jx 'JSON.stringify(j.result.tools).includes("/api/v1/mcp/") || JSON.stringify(j.result.tools).includes("/api/v1/tokens")')" false
rpc "$AT" resources/read '{"uri":"ctrl32://me"}'
check "resource: who I act as" "$(jx 'JSON.parse(j.result.contents[0].text).user + " " + JSON.parse(j.result.contents[0].text).ai_client')" "$USER E2E Assistant $SUF"
tool "$AT" api_reference '{"operations":["postSites"]}'
check "api_reference: body schema with reason" "$(text | node -e 'const a=JSON.parse(require("fs").readFileSync(0)); console.log(a[0].tool + " " + !!a[0].body.properties.reason)')" "sites_change true"

echo "== tool calls through the API"
tool "$AT" devices_read '{"operation":"getDevices","query":{"limit":5}}'
check "read devices" "$(jx '!j.result.isError') $(text | head -c 1)" "true ["
tool "$AT" devices_read '{"operation":"postSites","body":{}}'; check "operation of another tool" "$(jx 'j.result.isError') $(text)" "true operation postSites belongs to the tool sites_change"
tool "$AT" sites_change '{"operation":"postSites","body":{"name":"MCP site '"$SUF"'"}}'; check "change without a reason is refused by the API" "$(jx 'j.result.isError') $(text)" "true HTTP 400: reason required"
tool "$AT" sites_change '{"operation":"postSites","body":{"name":"MCP site '"$SUF"'","reason":"e2e: the user asked the assistant for a site"}}'
check "site created through MCP" "$(jx '!j.result.isError') $(text | node -e 'console.log(JSON.parse(require("fs").readFileSync(0)).name)')" "true MCP site $SUF"
SITE=$(text | node -e 'console.log(JSON.parse(require("fs").readFileSync(0)).id)')
tool "$AT" nope_read '{"operation":"getDevices"}'; check "unknown tool: protocol error" "$(jx j.error.code)" "-32602"
tool "$AT" devices_read '{"operation":"getDevices","extra":1}'; check "unknown argument: protocol error" "$(jx j.error.code)" "-32602"
tool "$AT" sites_change '{"operation":"patchSitesId","path":{},"body":{"reason":"e2e"}}'; check "missing path parameter" "$(jx 'j.result.isError') $(text)" "true path parameter id is required for patchSitesId"

echo "== the token is for /mcp only"
curl -s -H "Authorization: Bearer $AT" -o "$TMP/b" -w '%{http_code}' "$BASE/api/v1/devices" > "$TMP/s"; check "AI access token refused by the REST API" "$(st)" 401
curl -s -b "$JAR" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' "$BASE/mcp" > "$TMP/s"; check "browser session refused at /mcp" "$(st)" 401

echo "== audit"
req GET "/api/v1/export/audit?format=csv"
grep "site.create" "$TMP/b" | grep -q "via MCP" && ok "the change names the assistant (via MCP)" || bad "no 'via MCP' in the audit trail: $(grep -i 'MCP site' "$TMP/b" | head -c 300)"
grep -q "E2E Assistant $SUF via MCP" "$TMP/b" && ok "…with the client's name" || bad "client name missing in the audit"
grep -q "mcp_settings.update" "$TMP/b" && ok "switching AI access audited" || bad "mcp_settings.update missing"
grep "mcp_grant.create" "$TMP/b" | grep -q "E2E Assistant $SUF" && ok "approval audited" || bad "mcp_grant.create missing"

echo "== refresh token rotation"
form /oauth/token -d grant_type=refresh_token -d "refresh_token=$RT" -d "client_id=$CID"
check "refresh" "$(st) $(jx 'j.access_token !== process.env.AT && j.refresh_token.startsWith("c32r_")')" "200 true"
AT2=$(jx j.access_token); RT2=$(jx j.refresh_token)
form /oauth/token -d grant_type=refresh_token -d "refresh_token=$RT" -d "client_id=$CID"; check "old refresh token no longer works" "$(st) $(jx j.error)" "400 invalid_grant"
rpc "$AT2" tools/list; check "new access token works" "$(st)" 200

echo "== code used twice ends the grant"
pkce; req POST /api/v1/mcp/authorize "$(DEC "$CH" "twice-$SUF" mcp true false)"; C2=$(param "$(jx j.redirect)" code)
form /oauth/token -d grant_type=authorization_code -d "code=$C2" -d "client_id=$CID" -d "redirect_uri=$REDIR" -d "code_verifier=$VER"; AT3=$(jx j.access_token)
form /oauth/token -d grant_type=authorization_code -d "code=$C2" -d "client_id=$CID" -d "redirect_uri=$REDIR" -d "code_verifier=$VER"; check "second use refused" "$(st) $(jx j.error)" "400 invalid_grant"
rpc "$AT3" tools/list; check "…and its tokens stop" "$(st)" 401

echo "== a new approval replaces the earlier one"
req GET /api/v1/mcp/grants; check "the earlier approval of this assistant was replaced (none left after the reused code)" "$(jx "j.grants.filter(g => !g.revoked_at && g.client_name === 'E2E Assistant $SUF').length")" 0
echo "== reading only"
pkce; req POST /api/v1/mcp/authorize "$(DEC "$CH" "ro-$SUF" "mcp:read" true true)"; C3=$(param "$(jx j.redirect)" code)
form /oauth/token -d grant_type=authorization_code -d "code=$C3" -d "client_id=$CID" -d "redirect_uri=${REDIR/:$((20000 + SUF % 10000))/:5999}" -d "code_verifier=$VER"; check "loopback redirect must be the same as in the request" "$(st)" 400
form /oauth/token -d grant_type=authorization_code -d "code=$C3" -d "client_id=$CID" -d "redirect_uri=$REDIR" -d "code_verifier=$VER"
check "read-only grant" "$(st) $(jx j.scope)" "200 mcp:read"
ATR=$(jx j.access_token); RTR=$(jx j.refresh_token)
rpc "$ATR" tools/list; check "no change tools" "$(jx 'j.result.tools.some(t => t.name.endsWith("_change"))') $(jx 'j.result.tools.some(t => t.name === "devices_read")')" "false true"
tool "$ATR" sites_change '{"operation":"postSites","body":{"name":"x","reason":"e2e"}}'; check "change refused" "$(jx 'j.result.isError') $(text)" "true this AI access was approved for reading only"

echo "== a user's permissions limit the tools"
U=$(req POST /api/v1/users "{\"username\":\"ai_$SUF\",\"full_name\":\"AI operator\",\"email\":\"ai$SUF@example.com\",\"roles\":[\"operator\"],\"reason\":\"e2e mcp\"}"; cat "$TMP/b")
UID2=$(echo "$U" | node -pe 'JSON.parse(require("fs").readFileSync(0)).user.id'); PW=$(echo "$U" | node -pe 'JSON.parse(require("fs").readFileSync(0)).initial_password')
req2 POST /api/v1/auth/login "{\"username\":\"ai_$SUF\",\"password\":\"$PW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
NEWPW="Ai-Operator-Passw0rd-$SUF"; req2 POST /api/v1/auth/password "{\"current\":\"$PW\",\"new\":\"$NEWPW\"}"
req2 POST /api/v1/auth/login "{\"username\":\"ai_$SUF\",\"password\":\"$NEWPW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
pkce; req2 POST /api/v1/mcp/authorize "$(DEC "$CH" "op-$SUF" mcp true false)"; check "operator approves an assistant" "$(st)" 200
C4=$(param "$(jx j.redirect)" code)
form /oauth/token -d grant_type=authorization_code -d "code=$C4" -d "client_id=$CID" -d "redirect_uri=$REDIR" -d "code_verifier=$VER"; ATO=$(jx j.access_token)
rpc "$ATO" tools/list
check "operator: no server or user administration" "$(jx 'j.result.tools.some(t => t.name === "server_change" || t.name === "users_change")')" false
tool "$ATO" sites_change '{"operation":"postSites","body":{"name":"x","reason":"e2e"}}'; check "operator: permission of the API applies" "$(jx 'j.result.isError')" true
req2 GET /api/v1/mcp/grants; check "operator lists own assistants" "$(st) $(jx 'j.grants.length')" "200 1"
req2 GET "/api/v1/mcp/grants?all=1"; check "…not the organization's" "$(st)" 403
req2 PUT /api/v1/mcp/settings '{"enabled":false,"reason":"x"}'; check "operator cannot switch AI access" "$(st)" 403

echo "== disconnect"
req GET "/api/v1/mcp/grants?all=1"; G=$(jx "j.grants.find(g => g.user === 'ai_$SUF' && !g.revoked_at).id")
check "administrator sees the operator's assistant" "$([[ -n "$G" && "$G" != null ]] && echo yes)" yes
req DELETE "/api/v1/mcp/grants/$G"; check "disconnect needs a reason" "$(st)" 400
req DELETE "/api/v1/mcp/grants/$G?reason=e2e%20disconnect"; check "disconnected" "$(st)" 200
rpc "$ATO" tools/list; check "its token stops at once" "$(st)" 401
req DELETE "/api/v1/mcp/grants/$G?reason=e2e"; check "second disconnect" "$(st)" 409
form /oauth/revoke -d "token=$RT2"; check "revocation endpoint" "$(st)" 200
rpc "$AT2" tools/list; check "revoked refresh token ends its grant" "$(st)" 401

echo "== personal API token"
req POST /api/v1/tokens '{"name":"mcp e2e","scopes":["data.read","device.manage","config.write"]}'; PT=$(jx j.token)
rpc "$PT" tools/list; check "API token works at /mcp" "$(st) $(jx 'j.result.tools.some(t => t.name === "sites_change")')" "200 true"
tool "$PT" sites_change "{\"operation\":\"patchSitesId\",\"path\":{\"id\":\"$SITE\"},\"body\":{\"name\":\"MCP site $SUF renamed\",\"reason\":\"e2e rename through MCP\"}}"
check "change with a path parameter through MCP" "$(jx '!j.result.isError')" true
tool "$PT" sites_read '{"operation":"getSites"}'; check "…visible when read back" "$(text | grep -c "MCP site $SUF renamed")" 1

echo "== switched off"
req PUT /api/v1/mcp/settings '{"enabled":false,"reason":"e2e mcp: switched off again"}'
rpc "$PT" tools/list; check "API token refused while off" "$(st)" 403
rpc "$ATR" tools/list; check "connected assistant refused while off" "$(st)" 403
form /oauth/token -d grant_type=refresh_token -d "refresh_token=$RTR" -d "client_id=$CID"; check "refresh still works while off (the approval stays)" "$(st)" 200
ATR2=$(jx j.access_token); RTR=$(jx j.refresh_token)
rpc "$ATR2" tools/list; check "…but the new token is refused at /mcp while off" "$(st)" 403
pkce; req POST /api/v1/mcp/authorize "$(DEC "$CH" "off-$SUF" mcp true false)"; check "no new approval while off" "$(st)" 403
req PUT /api/v1/mcp/settings '{"enabled":true,"reason":"e2e mcp: on again"}'
rpc "$ATR2" tools/list; check "switched on again: the assistant continues without reconnecting" "$(st)" 200
req PUT /api/v1/mcp/settings '{"enabled":false,"reason":"e2e mcp: off at the end"}'

echo "== cleanup"
[[ -n "$LIC_OURS" ]] && { req DELETE "/api/v1/server/license?reason=e2e%20mcp%20cleanup"; check "test license removed" "$(st)" 200; }
req GET /api/v1/mcp/grants; for g in $(jx "j.grants.filter(g => !g.revoked_at && g.client_name === 'E2E Assistant $SUF').map(g => g.id).join(' ')"); do req DELETE "/api/v1/mcp/grants/$g?reason=e2e%20cleanup"; done
req GET /api/v1/mcp/grants; check "no assistant of this run left connected" "$(jx "j.grants.filter(g => !g.revoked_at && g.client_name === 'E2E Assistant $SUF').length")" 0
req PATCH "/api/v1/users/$UID2" '{"disabled":true,"reason":"e2e cleanup"}'
req GET /api/v1/tokens; TID=$(jx "j.find(t => t.name === 'mcp e2e' && !t.revoked_at).id"); req DELETE "/api/v1/tokens/$TID"
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

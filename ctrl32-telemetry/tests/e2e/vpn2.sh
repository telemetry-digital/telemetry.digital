#!/usr/bin/env bash
# End-to-end checks for VPN phase 2 (0.65.0, ADR-0018 amendments, ADR-0019): management per organization (an
# organization's VPN manager sees and changes only its own peers and rules, server settings stay with the server
# administrator), time-limited access grants, four eyes for technicians' access through Settings → Approvals, the
# audited end of a grant, and the router/app templates. Needs the server agent's VPN simulation (or Linux) and a build
# that trusts the test licence key (tests/keys), like vpn.sh; tests/run-all.sh starts both.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/vpn2.sh
# SLOW=0 skips the wait for a grant to end (about two minutes).
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
SLOW=${SLOW:-1}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
pass=0; fail=0
JAR="$TMP/jar"; CSRF=""
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
skipped() { echo "  skip $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
# as <jar> <csrf> runs the following req as another user
as()   { local j=$JAR c=$CSRF; JAR=$1; CSRF=$2; shift 2; "$@"; JAR=$j; CSRF=$c; }
login(){ # jar user password -> prints csrf
  local j=$JAR; JAR=$1; CSRF=""
  req POST /api/v1/auth/login "{\"username\":\"$2\",\"password\":\"$3\"}"; local code=$(st)
  req GET /api/v1/auth/me; local c; c=$(jx j.csrf_token); JAR=$j; [[ $code == 200 ]] && echo "$c"; }
SUF=$RANDOM
NEWPW="Vpn2-e2e-$SUF-pass!"

echo "== setup"
CSRF=$(login "$JAR" "$USER" "$PASS"); [[ -n "$CSRF" ]] && ok "server administrator signed in" || { bad "login"; echo; echo "passed $pass, failed $fail"; exit 1; }
req GET /api/v1/auth/me; check "administrator holds server_admin" "$(jx 'j.roles.includes("server_admin") && j.permissions.includes("system.admin")')" true
MYORG=$(jx j.org_id)
req GET /api/v1/vpn; AGENT=$(jx '!!(j.agent.connected && j.supported)')
if [[ "$AGENT" != "true" ]]; then
  skipped "everything (no agent with VPN support; run with [agent] vpn_simulate or on Linux)"
  echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]; exit
fi
check "server scope" "$(jx j.scope)" server
req PUT /api/v1/server/wireguard '{"enabled":true,"listen_port":51820,"address":"10.66.0.1/24","endpoint":"vpn.example.com:51820","dns":"","reason":"e2e"}'; check "VPN server configured" "$(st)" 200
LICOK=false
if command -v go >/dev/null && go build -o "$TMP/licgen" ./tools/licgen 2>/dev/null; then
  req PUT /api/v1/server/license "{\"token\":\"$("$TMP/licgen" issue -key tests/keys/license-test.key -licensee "E2E VPN2 $SUF" 2>/dev/null)\",\"reason\":\"e2e\"}"
  [[ "$(st)" == 200 ]] && LICOK=true
fi
if [[ $LICOK != true ]]; then
  skipped "the server does not trust the test key (tests/keys/README.md): grants and sites need the licence"
  echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]; exit
fi
ok "test license installed"
req POST /api/v1/organizations "{\"slug\":\"vpn2-$SUF\",\"name\":\"VPN2 other $SUF\",\"tz\":\"Europe/Bratislava\",\"locale\":\"en\",\"admin_username\":\"vpn2adm_$SUF\",\"admin_full_name\":\"Other admin\",\"admin_email\":\"vpn2adm$SUF@example.com\",\"reason\":\"e2e\"}"
check "second organization" "$(st)" 201
OTHER=$(jx 'j.id || (j.organization && j.organization.id)')
for u in orgadm second; do
  req POST /api/v1/users "{\"username\":\"${u}_$SUF\",\"full_name\":\"VPN $u\",\"email\":\"$u$SUF@example.com\",\"roles\":[\"org_admin\"],\"reason\":\"e2e\"}"
  check "organization administrator $u" "$(st)" 201
  eval "PW_$u=\$(jx j.initial_password)"; eval "ID_$u=\$(jx j.user.id)"
done
for u in orgadm second; do
  J="$TMP/jar_$u"; eval "pw=\$PW_$u"
  c=$(login "$J" "${u}_$SUF" "$pw")
  as "$J" "$c" req POST /api/v1/auth/password "{\"current\":\"$pw\",\"new\":\"$NEWPW\"}"
  c=$(login "$J" "${u}_$SUF" "$NEWPW"); eval "C_$u=\$c"
  [[ -n "$c" ]] && ok "$u signed in with a new password" || bad "$u login"
done
JO="$TMP/jar_orgadm"; JS="$TMP/jar_second"
as "$JO" "$C_orgadm" req GET /api/v1/auth/me
check "org_admin has vpn.manage, not system.admin" "$(jx '[j.permissions.includes("vpn.manage"), j.permissions.includes("system.admin")].join(" ")')" "true false"

echo "== peers of other organizations and of the whole server"
peer() { # name kind [extra json] (as the current jar; password $2 of the caller)
  req POST /api/v1/vpn/peers "{\"name\":\"$1\",\"kind\":\"$2\",\"reason\":\"e2e\",\"password\":\"$PW\"${3:+,$3}}"; }
PW=$PASS
peer "srv-dev-$SUF" device; check "server-wide device" "$(st)" 201; SRVDEV=$(jx j.peer.id)
peer "other-site-$SUF" site "\"org_id\":\"$OTHER\",\"subnets\":[\"172.31.$((SUF % 200)).0/24\"]"; check "site of the other organization" "$(st)" 201; OTHERSITE=$(jx j.peer.id)
peer "my-site-$SUF" site "\"org_id\":\"$MYORG\",\"subnets\":[\"172.30.$((SUF % 200)).0/24\"]"; check "site of this organization" "$(st)" 201; MYSITE=$(jx j.peer.id)
check "router templates for more brands" "$(jx '["wg_quick","openwrt","mikrotik","rutos","edgerouter","pfsense","opnsense"].every(k => j.template_keys.includes(k) && j.templates[k].length > 50)')" true
check "templates name the routed LAN" "$(jx "['rutos','edgerouter','pfsense','opnsense'].every(k => j.templates[k].includes('172.30.$((SUF % 200)).0/24'))")" true
check "Windows app file with the key" "$(jx 'j.client_templates.windows.includes("[Interface]") && j.client_templates.windows.includes("PrivateKey = ")')" true
check "phone steps without the key" "$(jx '!j.client_templates.mobile.includes("PrivateKey") && j.client_templates.mobile.length > 20')" true

echo "== an organization's VPN manager"
PW=$NEWPW
as "$JO" "$C_orgadm" req GET /api/v1/vpn; check "VPN state for the organization" "$(st) $(jx j.scope)" "200 organization"
check "only own peers" "$(jx "[j.peers.some(p => p.id === '$MYSITE'), j.peers.some(p => p.id === '$SRVDEV'), j.peers.some(p => p.id === '$OTHERSITE')].join(' ')")" "true false false"
check "no generated table" "$(jx '!(j.interface.firewall.ruleset)')" true
check "no SSH service" "$(jx '!j.services.some(s => s.key === "ssh")')" true
as "$JO" "$C_orgadm" req PUT /api/v1/server/wireguard '{"enabled":true,"reason":"e2e"}'; check "server settings refused" "$(st)" 403
as "$JO" "$C_orgadm" req POST /api/v1/vpn/apply '{"reason":"e2e"}'; check "apply refused" "$(st)" 403
as "$JO" "$C_orgadm" peer "foreign-$SUF" device "\"org_id\":\"$OTHER\""; check "peer of another organization refused" "$(st)" 403
as "$JO" "$C_orgadm" peer "tech-$SUF" user; check "technician of the own organization" "$(st)" 201
TECH=$(jx j.peer.id)
as "$JO" "$C_orgadm" req GET /api/v1/vpn; check "the peer belongs to the organization" "$(jx "j.peers.find(p => p.id === '$TECH').org_id")" "$MYORG"
as "$JO" "$C_orgadm" req PATCH "/api/v1/vpn/peers/$SRVDEV" '{"name":"x","reason":"e2e"}'; check "server-wide peer invisible" "$(st)" 404
as "$JO" "$C_orgadm" req POST "/api/v1/vpn/peers/$OTHERSITE/remove" '{"reason":"e2e"}'; check "other organization's site invisible" "$(st)" 404
as "$JO" "$C_orgadm" req POST "/api/v1/vpn/peers/$SRVDEV/rotate" "{\"reason\":\"e2e\",\"password\":\"$NEWPW\"}"; check "rotating a foreign key refused" "$(st)" 404
as "$JO" "$C_orgadm" req PATCH "/api/v1/vpn/peers/$TECH" "{\"org_id\":\"$OTHER\",\"reason\":\"e2e\"}"; check "moving a peer to another organization refused" "$(st)" 403
R="\"reason\":\"e2e\""
as "$JO" "$C_orgadm" req POST /api/v1/vpn/rules "{\"name\":\"all\",\"sources\":[{\"type\":\"all\"}],\"dest_type\":\"site\",\"dest_peer_id\":\"$MYSITE\",\"protocol\":\"any\",$R}"; check "all peers as a source refused" "$(st)" 403
as "$JO" "$C_orgadm" req POST /api/v1/vpn/rules "{\"name\":\"kind\",\"sources\":[{\"type\":\"kind\",\"kind\":\"user\"}],\"dest_type\":\"site\",\"dest_peer_id\":\"$MYSITE\",\"protocol\":\"any\",$R}"; check "a kind as a source refused" "$(st)" 403
as "$JO" "$C_orgadm" req POST /api/v1/vpn/rules "{\"name\":\"x\",\"sources\":[{\"type\":\"peer\",\"id\":\"$TECH\"}],\"dest_type\":\"site\",\"dest_peer_id\":\"$OTHERSITE\",\"protocol\":\"any\",$R}"; check "another organization's site as a destination refused" "$(st)" 404
as "$JO" "$C_orgadm" req POST /api/v1/vpn/rules "{\"name\":\"x\",\"sources\":[{\"type\":\"org\",\"id\":\"$OTHER\"}],\"dest_type\":\"site\",\"dest_peer_id\":\"$MYSITE\",\"protocol\":\"any\",$R}"; check "another organization as a source refused" "$(st)" 403
as "$JO" "$C_orgadm" req POST /api/v1/vpn/rules "{\"name\":\"ssh\",\"sources\":[{\"type\":\"peer\",\"id\":\"$TECH\"}],\"dest_type\":\"server\",\"protocol\":\"tcp\",\"ports\":\"22\",$R}"; check "SSH of the host refused" "$(st)" 403
as "$JO" "$C_orgadm" req POST /api/v1/vpn/rules "{\"name\":\"MQTT org $SUF\",\"sources\":[{\"type\":\"org\",\"id\":\"$MYORG\"}],\"dest_type\":\"server\",\"protocol\":\"tcp\",\"ports\":\"1883\",$R}"; check "own organization → MQTT" "$(st)" 201
ORGRULE=$(jx j.id)
as "$JO" "$C_orgadm" req POST /api/v1/vpn/rules "{\"name\":\"PLC $SUF\",\"sources\":[{\"type\":\"peer\",\"id\":\"$TECH\"}],\"dest_type\":\"site\",\"dest_peer_id\":\"$MYSITE\",\"dest_cidr\":\"172.30.$((SUF % 200)).20\",\"protocol\":\"tcp\",\"ports\":\"102, 502\",$R}"; check "technician → own PLC" "$(st)" 201
PLCRULE=$(jx j.id)
req GET /api/v1/vpn; check "the server administrator sees the organization's rule" "$(jx "j.rules.find(r => r.id === '$ORGRULE').org_id")" "$MYORG"
req POST /api/v1/vpn/rules "{\"name\":\"server rule $SUF\",\"sources\":[{\"type\":\"kind\",\"kind\":\"device\"}],\"dest_type\":\"server\",\"protocol\":\"tcp\",\"ports\":\"1883\",$R}"; SRVRULE=$(jx j.id)
as "$JO" "$C_orgadm" req PUT "/api/v1/vpn/rules/$SRVRULE" "{\"name\":\"x\",\"sources\":[{\"type\":\"org\",\"id\":\"$MYORG\"}],\"dest_type\":\"server\",\"protocol\":\"tcp\",\"ports\":\"1883\",$R}"; check "server-wide rule cannot be changed" "$(st)" 404
as "$JO" "$C_orgadm" req POST "/api/v1/vpn/rules/$SRVRULE/remove" "{$R}"; check "server-wide rule cannot be removed" "$(st)" 404

echo "== access grants and four eyes"
PW=$NEWPW
as "$JO" "$C_orgadm" req POST "/api/v1/vpn/peers/$TECH/grant" "{\"minutes\":5,$R}"; check "grant shorter than 15 minutes refused" "$(st)" 400
as "$JO" "$C_orgadm" req POST "/api/v1/vpn/peers/$SRVDEV/grant" "{\"minutes\":120,$R}"; check "grant for a foreign peer refused" "$(st)" 404
as "$JO" "$C_orgadm" req POST "/api/v1/vpn/peers/$TECH/grant" "{\"minutes\":120}"; check "grant needs a reason" "$(st)" 400
as "$JO" "$C_orgadm" req POST "/api/v1/vpn/peers/$TECH/grant" "{\"minutes\":120,\"reason\":\"service call $SUF\"}"; check "grant without four eyes" "$(st)" 201
check "two hours window" "$(jx 'Math.round((new Date(j.grant.ends_at) - new Date(j.grant.starts_at)) / 60000)')" 120
as "$JO" "$C_orgadm" req GET /api/v1/vpn; check "peer active with the grant" "$(jx "[j.peers.find(p => p.id === '$TECH').active, !!j.peers.find(p => p.id === '$TECH').grant].join(' ')")" "true true"
req PUT /api/v1/settings/policy '{"four_eyes_vpn":true,"reason":"e2e"}'; check "policy four eyes for VPN access" "$(st) $(jx j.four_eyes_vpn)" "200 true"
req GET /api/v1/settings/policy; check "policy stored" "$(jx j.four_eyes_vpn)" true
as "$JO" "$C_orgadm" peer "tech2-$SUF" user; check "technician added under four eyes" "$(st) $(jx j.approval_required) $(jx j.peer.enabled)" "201 true false"
TECH2=$(jx j.peer.id)
EXP=$(date -u -d '+3 hours' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v+3H +%Y-%m-%dT%H:%M:%SZ)
as "$JO" "$C_orgadm" peer "tech3-$SUF" user "\"expires_at\":\"$EXP\""; check "expiry at creation refused under four eyes" "$(st) $(jx j.code)" "409 approval_required"
as "$JO" "$C_orgadm" req PATCH "/api/v1/vpn/peers/$TECH2" "{\"enabled\":true,$R}"; check "enabling directly refused" "$(st) $(jx j.code)" "409 approval_required"
as "$JO" "$C_orgadm" req PATCH "/api/v1/vpn/peers/$TECH2" "{\"expires_at\":\"$EXP\",$R}"; check "setting the expiry directly refused" "$(st) $(jx j.code)" "409 approval_required"
as "$JO" "$C_orgadm" req POST "/api/v1/vpn/peers/$TECH2/grant" "{\"minutes\":480,\"reason\":\"night shift $SUF\"}"; check "grant held for approval" "$(st) $(jx j.status)" "202 pending"
CR=$(jx j.change_request.id)
as "$JO" "$C_orgadm" req GET /api/v1/vpn; check "request shown at the peer" "$(jx "[j.peers.find(p => p.id === '$TECH2').pending_grant.change_request_id === '$CR', j.peers.find(p => p.id === '$TECH2').enabled].join(' ')")" "true false"
as "$JO" "$C_orgadm" req GET /api/v1/change-requests; check "own request cannot be approved by the requester" "$(jx "j.find(c => c.id === '$CR').can_approve")" false
check "kind vpn with a summary" "$(jx "j.find(c => c.id === '$CR').kind + ' ' + j.find(c => c.id === '$CR').summary.includes('8 hours')")" "vpn true"
as "$JO" "$C_orgadm" req POST "/api/v1/change-requests/$CR/approve" '{"reason":"self"}'; check "requester cannot approve" "$(st)" 403
as "$JS" "$C_second" req GET /api/v1/change-requests; check "a second administrator may approve" "$(jx "j.find(c => c.id === '$CR').can_approve")" true
as "$JS" "$C_second" req POST "/api/v1/change-requests/$CR/approve" '{"reason":"approved by the shift lead"}'; check "approved by a second person" "$(st) $(jx j.status)" "200 approved"
as "$JO" "$C_orgadm" req GET /api/v1/vpn
check "technician active after the approval" "$(jx "j.peers.find(p => p.id === '$TECH2').active")" true
check "grant names both people" "$(jx "j.peers.find(p => p.id === '$TECH2').grant.requested_by + ' ' + j.peers.find(p => p.id === '$TECH2').grant.approved_by")" "orgadm_$SUF second_$SUF"
check "window of 8 hours from the approval" "$(jx "Math.round((new Date(j.peers.find(p => p.id === '$TECH2').expires_at) - Date.now()) / 3600000)")" 8
as "$JO" "$C_orgadm" req POST "/api/v1/vpn/peers/$TECH2/grant" "{\"minutes\":60,\"reason\":\"another $SUF\"}"; CR2=$(jx j.change_request.id)
as "$JS" "$C_second" req POST "/api/v1/change-requests/$CR2/reject" '{"reason":"not needed"}'; check "a request can be rejected" "$(st) $(jx j.status)" "200 rejected"
as "$JO" "$C_orgadm" req PATCH "/api/v1/vpn/peers/$TECH2" "{\"enabled\":false,$R}"; check "disabling stays direct" "$(st)" 200
as "$JO" "$C_orgadm" req GET /api/v1/vpn; check "disabling ends the grant" "$(jx "!j.peers.find(p => p.id === '$TECH2').grant")" true
as "$JO" "$C_orgadm" req POST "/api/v1/vpn/peers/$SRVDEV/grant" "{\"minutes\":60,$R}"; check "devices are outside four eyes but still scoped" "$(st)" 404
req POST "/api/v1/vpn/peers/$SRVDEV/grant" "{\"minutes\":60,$R}"; check "devices are granted directly" "$(st)" 201
req DELETE "/api/v1/server/license?reason=e2e"
req POST "/api/v1/vpn/peers/$SRVDEV/grant" "{\"minutes\":60,$R}"; check "grants need the license" "$(st) $(jx j.code)" "402 license_required"
req PUT /api/v1/server/license "{\"token\":\"$("$TMP/licgen" issue -key tests/keys/license-test.key -licensee "E2E VPN2 $SUF" 2>/dev/null)\",\"reason\":\"e2e\"}"

echo "== the end of a grant is audited"
if [[ "$SLOW" == 1 ]]; then
  req PUT /api/v1/settings/policy '{"four_eyes_vpn":false,"reason":"e2e"}'
  EXP=$(date -u -d '+65 seconds' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v+65S +%Y-%m-%dT%H:%M:%SZ)
  req PATCH "/api/v1/vpn/peers/$TECH" "{\"expires_at\":\"$EXP\",$R}"; check "grant shortened to a minute" "$(st)" 200
  ended=false
  for _ in $(seq 24); do sleep 5; req GET /api/v1/vpn; [[ "$(jx "j.peers.find(p => p.id === '$TECH').enabled")" == false ]] && { ended=true; break; }; done
  check "peer disabled when the window ended" "$ended" true
  check "grant closed" "$(jx "!j.peers.find(p => p.id === '$TECH').grant")" true
  req GET "/api/v1/export/audit?format=csv"; grep "vpn.peer_expire" "$TMP/b" | grep -q "tech-$SUF" && ok "audit vpn.peer_expire" || bad "expiry not audited"
else
  skipped "wait for the end of a grant (SLOW=0)"
fi

echo "== audit and clean-up"
req GET "/api/v1/export/audit?format=csv"
for a in vpn.grant change_request.create change_request.approve change_request.reject organization.policy; do grep -q "$a" "$TMP/b" && ok "audit $a" || bad "audit $a missing"; done
grep "vpn.grant" "$TMP/b" | grep -q "second_$SUF" && ok "grant audited with the approver" || bad "approver not in the audit trail"
req PUT /api/v1/settings/policy '{"four_eyes_vpn":false,"reason":"e2e cleanup"}'
for id in $ORGRULE $PLCRULE $SRVRULE; do req POST "/api/v1/vpn/rules/$id/remove" '{"reason":"e2e cleanup"}'; done
for id in $TECH $TECH2 $SRVDEV $OTHERSITE $MYSITE; do req POST "/api/v1/vpn/peers/$id/remove" '{"reason":"e2e cleanup"}'; done
req GET /api/v1/vpn; check "clean-up" "$(jx "j.peers.filter(p => p.name.endsWith('-$SUF')).length")" 0
for u in $ID_orgadm $ID_second; do req PATCH "/api/v1/users/$u" '{"disabled":true,"reason":"e2e cleanup"}'; done
req PATCH "/api/v1/organizations/$OTHER" '{"disabled":true,"reason":"e2e cleanup"}'
req DELETE "/api/v1/server/license?reason=e2e"

echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

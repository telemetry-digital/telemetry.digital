#!/usr/bin/env bash
# End-to-end checks for the own WireGuard VPN (spec 024, ADR-0018): peers of three kinds, the configuration shown once
# behind a password check, the free limit and the licensed capabilities, access rules with default deny compiled into
# the nftables table, disable/enable, key rotation, removal with rule clean-up, and the audit trail.
# The server needs a server agent that manages the VPN: on Linux the real agent, elsewhere the agent's simulation
# ([agent] vpn_simulate; tests/run-all.sh starts one for this suite). Without it only the checks that need no agent
# run and the rest is reported as skipped. Licensed checks need a build that trusts the test key (tests/keys).
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/vpn.sh
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
skipped() { echo "  skip $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
SUF=$RANDOM
peer() { # name kind [extra json]: creates a peer with the password, leaves the answer in $TMP/b
  req POST /api/v1/vpn/peers "{\"name\":\"$1\",\"kind\":\"$2\",\"reason\":\"e2e\",\"password\":\"$PASS\"${3:+,$3}}"; }
ruleset() { req GET /api/v1/vpn; jx 'j.interface && j.interface.firewall ? j.interface.firewall.ruleset : ""' > "$TMP/nft"; }

echo "== pages and access"
curl -s -o /dev/null -w '%{http_code}' "$BASE/server/vpn" > "$TMP/s"; check "page /server/vpn" "$(st)" 200
curl -s -o /dev/null -w '%{http_code} %{redirect_url}' "$BASE/server/wireguard" > "$TMP/s"; [[ "$(st)" == 301*"/server/vpn" ]] && ok "old WireGuard page redirects" || bad "redirect ($(st))"
req GET /api/v1/vpn; check "needs authentication" "$(st)" 401
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"
MFA=$(jx j.mfa_enabled)
req GET /api/v1/vpn; check "state" "$(st)" 200
check "free limit published" "$(jx j.free_limit)" 5
AGENT=$(jx '!!(j.agent.connected && j.supported)')

echo "== validation before anything is applied"
req POST /api/v1/vpn/peers '{"name":"x","kind":"user"}'; check "peer needs a reason" "$(st)" 400
req POST /api/v1/vpn/peers '{"name":"x","kind":"robot","reason":"e2e","password":"x"}'; check "kind validated" "$(st)" 400
req POST /api/v1/vpn/peers '{"name":"bad\nname","kind":"user","reason":"e2e","password":"x"}'; check "name validated" "$(st)" 400
req POST /api/v1/vpn/peers '{"name":"tech","kind":"user","reason":"e2e"}'; check "configuration needs the password" "$(st) $(jx j.code)" "403 step_up_required"
req POST /api/v1/vpn/peers '{"name":"tech","kind":"user","reason":"e2e","password":"wrong-password"}'; check "wrong password refused" "$(st) $(jx j.code)" "403 step_up_failed"
req POST /api/v1/vpn/peers/0192a000-0000-7000-8000-000000000001/rotate '{"reason":"e2e"}'; check "rotation needs the password" "$(st)" 403
req POST /api/v1/vpn/rules '{"name":"x","sources":[{"type":"all"}],"dest_type":"server","protocol":"tcp"}'; check "rule needs a reason" "$(st)" 400
req POST /api/v1/vpn/peers/not-an-id/remove '{"reason":"e2e"}'; check "peer id validated" "$(st)" 400
req POST /api/v1/vpn/apply '{}'; check "apply needs a reason" "$(st)" 400

if [[ "$AGENT" != "true" ]]; then
  echo "== without an agent that manages the VPN"
  req POST /api/v1/vpn/apply '{"reason":"e2e"}'
  [[ "$(st)" == 503 || "$(st)" == 501 || "$(st)" == 502 ]] && ok "changes refused without the agent ($(st))" || bad "changes without agent ($(st))"
  skipped "peers, rules and firewall (no agent with VPN support; run with [agent] vpn_simulate or on Linux)"
  echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]; exit
fi
req GET /api/v1/vpn; SIM=$(jx '!!j.simulated'); echo "  (agent connected, simulated: $SIM)"

echo "== server"
req PUT /api/v1/server/wireguard '{"enabled":true,"listen_port":51820,"address":"10.66.0.1/24","endpoint":"vpn.example.com:51820","dns":"","reason":"e2e"}'; check "VPN server configured" "$(st)" 200
check "server public key" "$(jx '!!j.public_key')" true
req PUT /api/v1/server/wireguard '{"enabled":true,"address":"10.66.0.1/31","reason":"e2e"}'; check "agent validates the range" "$(st)" 502
# start from a clean state: remove what an earlier run left
req GET /api/v1/vpn
for id in $(jx 'j.rules.map(r => r.id).join(" ")'); do req POST "/api/v1/vpn/rules/$id/remove" '{"reason":"e2e cleanup"}'; done
req GET /api/v1/vpn
for id in $(jx 'j.peers.map(p => p.id).join(" ")'); do req POST "/api/v1/vpn/peers/$id/remove" '{"reason":"e2e cleanup"}'; done
req DELETE "/api/v1/server/license?reason=e2e"
req GET /api/v1/vpn; check "clean start" "$(jx 'j.peers.length + " " + j.rules.length + " " + j.licensed')" "0 0 false"
ruleset; grep -q 'counter drop comment "default deny"' "$TMP/nft" && ok "default deny without rules" || bad "default deny missing"
grep -q 'comment "rule' "$TMP/nft" && bad "rules without access rules" || ok "nothing allowed without rules"

echo "== peers without the license"
peer "tech-$SUF" user; check "technician added" "$(st)" 201
TECH=$(jx j.peer.id); CONF=$(jx j.config)
echo "$CONF" | grep -q '^PrivateKey = ' && ok "configuration with the private key" || bad "config: $CONF"
echo "$CONF" | grep -q '^Address = 10.66.0.2/32' && ok "first free address" || bad "address"
echo "$CONF" | grep -q '^Endpoint = vpn.example.com:51820' && ok "endpoint" || bad "endpoint"
echo "$CONF" | grep -q '^PresharedKey = ' && ok "preshared key" || bad "psk"
check "QR code" "$(jx 'j.qr.startsWith("data:image/png;base64,")')" true
PRIV=$(echo "$CONF" | grep '^PrivateKey' | cut -d' ' -f3)
req GET /api/v1/vpn; grep -q "$PRIV" "$TMP/b" && bad "private key readable later" || ok "private key not stored or shown again"
check "peer listed" "$(jx "j.peers.find(p => p.id === '$TECH').kind + ' ' + j.peers_used")" "user 1"
peer "plant-$SUF" site '"subnets":["192.168.77.0/24"]'; check "site needs the license" "$(st) $(jx j.code)" "402 license_required"
peer "contractor-$SUF" user "\"expires_at\":\"$(date -u -d '+2 hours' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v+2H +%Y-%m-%dT%H:%M:%SZ)\""; check "expiry needs the license" "$(st) $(jx j.code)" "402 license_required"
peer "tech-$SUF" device; check "duplicate name refused" "$(st)" 400
peer "gw-$SUF" device '"subnets":["10.1.0.0/24"]'; check "subnets only for sites" "$(st)" 400
for i in 1 2 3 4; do peer "dev$i-$SUF" device; [[ "$(st)" == 201 ]] || bad "device $i ($(st) $(cat "$TMP/b"))"; [[ $i == 1 ]] && DEV1=$(jx j.peer.id); done
ok "five free peers"
peer "dev5-$SUF" device; check "sixth peer needs the license" "$(st) $(jx j.code)" "402 license_required"
grep -q "Existing peers keep working" "$TMP/b" && ok "clear message" || bad "message: $(cat "$TMP/b")"

echo "== access rules"
req POST /api/v1/vpn/rules '{"name":"MQTT for devices","sources":[{"type":"kind","kind":"device"}],"dest_type":"server","protocol":"tcp","ports":"1883","reason":"e2e"}'; check "rule devices → MQTT" "$(st)" 201
MQTT_RULE=$(jx j.id)
req POST /api/v1/vpn/rules "{\"name\":\"bad ports\",\"sources\":[{\"type\":\"all\"}],\"dest_type\":\"server\",\"protocol\":\"tcp\",\"ports\":\"1883; drop\",\"reason\":\"e2e\"}"; check "ports validated" "$(st)" 400
req POST /api/v1/vpn/rules "{\"name\":\"x\",\"sources\":[{\"type\":\"peer\",\"id\":\"0192a000-0000-7000-8000-00000000dead\"}],\"dest_type\":\"server\",\"protocol\":\"any\",\"reason\":\"e2e\"}"; check "unknown source refused" "$(st)" 400
req POST /api/v1/vpn/rules "{\"name\":\"tech to gateway\",\"sources\":[{\"type\":\"peer\",\"id\":\"$TECH\"}],\"dest_type\":\"peer\",\"dest_peer_id\":\"$DEV1\",\"protocol\":\"tcp\",\"ports\":\"80, 443\",\"reason\":\"e2e\"}"; check "rule technician → device" "$(st)" 201
ruleset
grep -q 'ip saddr { 10.66.0.3/32, 10.66.0.4/32, 10.66.0.5/32, 10.66.0.6/32 } ip daddr 10.66.0.1/32 tcp dport 1883 counter accept' "$TMP/nft" && ok "MQTT rule in the input chain" || bad "MQTT rule: $(grep 1883 "$TMP/nft")"
grep -q 'iifname "wg0" oifname "wg0" ip saddr 10.66.0.2/32 ip daddr 10.66.0.3/32 tcp dport { 80, 443 } counter accept' "$TMP/nft" && ok "forward rule technician → device" || bad "forward rule: $(grep 10.66.0.2 "$TMP/nft")"
req GET /api/v1/vpn; check "forwarding on with a forward rule" "$(jx j.interface.firewall.forward)" true

echo "== disable, enable, rotate"
req PATCH "/api/v1/vpn/peers/$TECH" '{"enabled":false}'; check "disable needs a reason" "$(st)" 400
req PATCH "/api/v1/vpn/peers/$TECH" '{"enabled":false,"reason":"e2e"}'; check "technician disabled" "$(st)" 200
ruleset; grep -q '10.66.0.2/32' "$TMP/nft" && bad "disabled peer still in the rules" || ok "disabled peer out of the firewall"
req GET /api/v1/vpn; check "disabled is not active" "$(jx "j.peers.find(p => p.id === '$TECH').active")" false
req PATCH "/api/v1/vpn/peers/$TECH" '{"enabled":true,"reason":"e2e"}'; check "enabled again" "$(st)" 200
req GET /api/v1/vpn; OLDKEY=$(jx "j.peers.find(p => p.id === '$TECH').public_key")
req POST "/api/v1/vpn/peers/$TECH/rotate" "{\"reason\":\"e2e rotate\",\"password\":\"$PASS\"}"; check "key rotated" "$(st)" 200
NEWKEY=$(jx j.peer.public_key); [[ -n "$NEWKEY" && "$NEWKEY" != "$OLDKEY" ]] && ok "new public key" || bad "key unchanged"
echo "$(jx j.config)" | grep -q '^Address = 10.66.0.2/32' && ok "same address after rotation" || bad "address after rotation"
req PATCH "/api/v1/vpn/peers/$TECH" '{"expires_at":"2020-01-01T00:00:00Z","reason":"e2e"}'; check "expiry in the past refused" "$(st)" 400

echo "== licensed"
LICOK=false
if command -v go >/dev/null && go build -o "$TMP/licgen" ./tools/licgen 2>/dev/null; then
  req PUT /api/v1/server/license "{\"token\":\"$("$TMP/licgen" issue -key tests/keys/license-test.key -licensee "E2E VPN $SUF" 2>/dev/null)\",\"reason\":\"e2e\"}"
  [[ "$(st)" == 200 ]] && LICOK=true
fi
if [[ $LICOK == true ]]; then
  ok "test license installed"
  peer "dev5-$SUF" device; check "sixth peer with the license" "$(st)" 201
  peer "plant-$SUF" site '"subnets":["192.168.77.5/24"]'; check "subnet must be a network address" "$(st)" 400
  peer "plant-$SUF" site '"subnets":["10.66.0.0/25"]'; check "subnet must not overlap the VPN" "$(st)" 400
  peer "plant-$SUF" site '"subnets":["192.168.77.0/24","192.168.78.0/24"]'; check "site with routed subnets" "$(st)" 201
  SITE=$(jx j.peer.id)
  check "router templates" "$(jx '["wg_quick","openwrt","mikrotik"].every(k => j.templates[k].includes("PrivateKey") || j.templates[k].includes("private_key") || j.templates[k].includes("private-key"))')" true
  peer "plant2-$SUF" site '"subnets":["192.168.77.128/25"]'; check "overlapping site subnet refused" "$(st)" 400
  req POST /api/v1/vpn/rules "{\"name\":\"PLC at plant\",\"sources\":[{\"type\":\"kind\",\"kind\":\"user\"}],\"dest_type\":\"site\",\"dest_peer_id\":\"$SITE\",\"dest_cidr\":\"192.168.77.20\",\"protocol\":\"tcp\",\"ports\":\"102, 502\",\"reason\":\"e2e\"}"; check "rule technicians → PLC" "$(st)" 201
  PLC_RULE=$(jx j.id)
  req POST /api/v1/vpn/rules "{\"name\":\"outside\",\"sources\":[{\"type\":\"all\"}],\"dest_type\":\"site\",\"dest_peer_id\":\"$SITE\",\"dest_cidr\":\"192.168.99.1\",\"protocol\":\"any\",\"reason\":\"e2e\"}"; check "destination outside the site refused" "$(st)" 400
  ruleset
  grep -q 'ip saddr 10.66.0.2/32 ip daddr 192.168.77.20/32 tcp dport { 102, 502 } counter accept' "$TMP/nft" && ok "PLC rule in the forward chain" || bad "PLC rule: $(grep 192.168.77 "$TMP/nft")"
  grep -q 'ip saddr { 10.66.0.3/32, 10.66.0.4/32, 10.66.0.5/32, 10.66.0.6/32, 10.66.0.7/32 }' "$TMP/nft" && ok "sixth device in the MQTT rule" || bad "sixth device"
  EXP=$(date -u -d '+3 hours' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v+3H +%Y-%m-%dT%H:%M:%SZ)
  peer "contractor-$SUF" user "\"expires_at\":\"$EXP\""; check "time-limited technician" "$(st)" 201
  CONTR=$(jx j.peer.id)
  echo "$(jx j.config)" | grep -q "192.168.77.0/24, 192.168.78.0/24" && ok "technician routes the site networks" || bad "client AllowedIPs: $(jx j.config | grep AllowedIPs)"
  req GET /api/v1/vpn; check "expiry stored" "$(jx "!!j.peers.find(p => p.id === '$CONTR').expires_at")" true
  req PATCH "/api/v1/vpn/peers/$SITE" '{"subnets":["192.168.77.0/24"],"reason":"e2e"}'; check "site subnets changed" "$(st)" 200
  req DELETE "/api/v1/server/license?reason=e2e"
  req GET /api/v1/vpn; check "without the license the peers stay" "$(jx 'j.licensed + " " + j.peers.filter(p => p.active).length')" "false 8"
  req PATCH "/api/v1/vpn/peers/$SITE" '{"subnets":["192.168.77.0/24","192.168.79.0/24"],"reason":"e2e"}'; check "new subnets need the license again" "$(st) $(jx j.code)" "402 license_required"
  req PATCH "/api/v1/vpn/peers/$SITE" '{"name":"plant-renamed-'$SUF'","reason":"e2e"}'; check "renaming stays free" "$(st)" 200
  req PATCH "/api/v1/vpn/peers/$CONTR" '{"expires_at":"","reason":"e2e"}'; check "clearing the expiry stays free" "$(st)" 200
  req POST "/api/v1/vpn/peers/$SITE/remove" '{"reason":"e2e"}'; check "site removed" "$(st) $(jx j.rules_removed)" "200 1"
  req GET /api/v1/vpn; check "its rule went with it" "$(jx "j.rules.some(r => r.id === '$PLC_RULE')")" false
else
  skipped "licensed checks (the server does not trust the test key; tests/keys/README.md)"
fi

echo "== removal and audit"
req POST "/api/v1/vpn/peers/$DEV1/remove" '{"reason":"e2e remove"}'; check "device removed" "$(st)" 200
check "rule towards it removed" "$(jx j.rules_removed)" 1
ruleset; grep -q '10.66.0.3/32' "$TMP/nft" && bad "removed peer still in the firewall" || ok "removed peer out of the firewall"
req POST "/api/v1/vpn/rules/$MQTT_RULE/remove" '{"reason":"e2e"}'; check "rule removed" "$(st)" 200
req POST /api/v1/vpn/apply '{"reason":"e2e apply"}'; check "apply again" "$(st)" 200
req GET "/api/v1/export/audit?format=csv"
for a in vpn.peer_add vpn.peer_update vpn.peer_rotate vpn.peer_remove vpn.rule_add vpn.rule_remove vpn.apply server.wireguard; do
  grep -q "$a" "$TMP/b" && ok "audit $a" || bad "audit $a missing"
done
grep -q "$PRIV" "$TMP/b" && bad "private key in the audit trail" || ok "no private key in the audit trail"

echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

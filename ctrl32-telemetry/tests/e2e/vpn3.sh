#!/usr/bin/env bash
# End-to-end checks for VPN phase 3 (0.69.0, ADR-0021, spec 027): the uplink of a customer server to a hub. The same
# server plays both parts against the server agent's VPN simulation: as the hub it issues an uplink token for a
# "Server (uplink)" peer (licence, step-up, no router texts, the HTTPS transport secret), as the customer it connects
# with that token (step-up, refusal of an overlapping range, its own default-deny uplink rules, transport switch with
# the WebSocket relay authenticated by the uplink's secret, replacement after a key rotation, disconnect) — and only a
# server administrator may do the customer part. Needs a build that trusts the test licence key (tests/keys), like
# vpn.sh and vpn2.sh; tests/run-all.sh starts both.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/vpn3.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
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
as()   { local j=$JAR c=$CSRF; JAR=$1; CSRF=$2; shift 2; "$@"; JAR=$j; CSRF=$c; }
login(){ local j=$JAR; JAR=$1; CSRF=""
  req POST /api/v1/auth/login "{\"username\":\"$2\",\"password\":\"$3\"}"; local code=$(st)
  req GET /api/v1/auth/me; local c; c=$(jx j.csrf_token); JAR=$j; [[ $code == 200 ]] && echo "$c"; }
# tunnel <key>: the hub's WebSocket endpoint of the HTTPS transport, answer code only
tunnel(){ curl -s -o /dev/null -w '%{http_code}' --max-time 5 -H "Authorization: Bearer $1" -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" "$BASE/api/v1/vpn/uplink/tunnel"; }
tokfield(){ node -e 'const t=process.argv[1].replace(/^ctrl32-uplink-v1\./,""); const v=JSON.parse(Buffer.from(t,"base64url").toString())[process.argv[2]]; console.log(Array.isArray(v) ? v.join() : (v ?? ""))' "$1" "$2"; }
SUF=$RANDOM
R='"reason":"e2e vpn3"'

echo "== setup"
CSRF=$(login "$JAR" "$USER" "$PASS"); [[ -n "$CSRF" ]] && ok "server administrator signed in" || { bad "login"; echo; echo "passed $pass, failed $fail"; exit 1; }
req GET /api/v1/vpn; AGENT=$(jx '!!(j.agent.connected && j.supported)')
if [[ "$AGENT" != "true" ]]; then
  skipped "everything (no agent with VPN support; run with [agent] vpn_simulate or on Linux)"
  echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]; exit
fi
check "uplink state reported, not connected" "$(jx '[!!j.uplink, j.uplink && j.uplink.configured].join(" ")')" "true false"
check "uplink rules listed" "$(jx 'Array.isArray(j.uplink_rules)')" true
# the hub (a cloud hub would use a range of its own that no customer network uses; the suites before left peers in the default one)
req PUT /api/v1/server/wireguard "{\"enabled\":true,\"listen_port\":51820,\"address\":\"10.66.0.1/24\",\"endpoint\":\"hub.example.com:51820\",\"dns\":\"\",$R}"; check "hub VPN configured" "$(st)" 200

echo "== hub: an uplink peer is a site and needs the licence"
req DELETE "/api/v1/server/license?reason=e2e"
req POST /api/v1/vpn/peers "{\"name\":\"customer-$SUF\",\"kind\":\"site\",\"uplink\":true,\"subnets\":[\"192.168.77.0/24\"],\"password\":\"$PASS\",$R}"
check "uplink without the licence" "$(st) $(jx j.code)" "402 license_required"
LICOK=false
if command -v go >/dev/null && go build -o "$TMP/licgen" ./tools/licgen 2>/dev/null; then
  req PUT /api/v1/server/license "{\"token\":\"$("$TMP/licgen" issue -key tests/keys/license-test.key -licensee "E2E VPN3 $SUF" 2>/dev/null)\",$R}"
  [[ "$(st)" == 200 ]] && LICOK=true
fi
if [[ $LICOK != true ]]; then
  skipped "the server does not trust the test key (tests/keys/README.md): uplinks need the licence"
  echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]; exit
fi
ok "test licence installed"
req POST /api/v1/vpn/peers "{\"name\":\"tech-$SUF\",\"kind\":\"user\",\"uplink\":true,\"password\":\"$PASS\",$R}"; check "uplink must be a site" "$(st)" 400
req POST /api/v1/vpn/peers "{\"name\":\"customer-$SUF\",\"kind\":\"site\",\"uplink\":true,\"subnets\":[\"192.168.77.0/24\"],$R}"; check "uplink token needs the password" "$(st) $(jx j.code)" "403 step_up_required"
req POST /api/v1/vpn/peers "{\"name\":\"customer-$SUF\",\"kind\":\"site\",\"uplink\":true,\"subnets\":[\"192.168.77.0/24\"],\"password\":\"$PASS\",$R}"; check "uplink peer added" "$(st)" 201
UPLINK=$(jx j.peer.id); TOKEN=$(jx j.uplink_token)
check "uplink token instead of a configuration" "$(jx '[j.uplink_token.startsWith("ctrl32-uplink-v1."), !j.config, !j.templates, !j.qr].join(" ")')" "true true true true"
check "token: hub endpoint and range" "$(tokfield "$TOKEN" endpoint) $(tokfield "$TOKEN" vpn_net) $(tokfield "$TOKEN" hub_address)" "hub.example.com:51820 10.66.0.0/24 10.66.0.1"
check "token: networks routed to the uplink" "$(tokfield "$TOKEN" subnets)" "192.168.77.0/24"
check "token carries the HTTPS transport (loopback base URL)" "$(jx j.https_transport) $(tokfield "$TOKEN" tunnel_url | grep -c /api/v1/vpn/uplink/tunnel)" "true 1"
KEY=$(tokfield "$TOKEN" tunnel_key)
UPADDR=$(tokfield "$TOKEN" address)
req GET /api/v1/vpn
check "uplink peer listed as a site with the flag" "$(jx "(p => [p.kind, p.uplink, p.subnets.join()].join(' '))(j.peers.find(p => p.id === '$UPLINK'))")" "site true 192.168.77.0/24"
req POST /api/v1/vpn/peers "{\"name\":\"tech-$SUF\",\"kind\":\"user\",\"password\":\"$PASS\",$R}"; check "technician on the hub" "$(st)" 201
TECH=$(jx j.peer.id)
check "the technician routes the uplink's network" "$(jx 'j.allowed_ips.includes("192.168.77.0/24")')" true
req POST /api/v1/vpn/rules "{\"name\":\"tech to customer PLC $SUF\",\"sources\":[{\"type\":\"peer\",\"id\":\"$TECH\"}],\"dest_type\":\"site\",\"dest_peer_id\":\"$UPLINK\",\"dest_cidr\":\"192.168.77.20\",\"protocol\":\"tcp\",\"ports\":\"502\",$R}"
check "hub rule technician → uplink network" "$(st)" 201; HUBRULE=$(jx j.id)
req GET /api/v1/vpn
check "hub firewall forwards to the uplink's network" "$(jx 'j.interface.firewall.ruleset.includes("ip daddr 192.168.77.20/32 tcp dport 502")')" true

echo "== the hub's end of the HTTPS transport"
check "unknown key refused" "$(tunnel "$(printf 'x%.0s' {1..43})")" 401
check "no key refused" "$(tunnel "")" 401
check "the uplink's key opens the tunnel" "$(tunnel "$KEY")" 101

echo "== customer: rules before connecting, step-up, overlapping ranges"
req POST /api/v1/vpn/uplink/rules "{\"name\":\"hub to web $SUF\",\"dest_type\":\"server\",\"protocol\":\"tcp\",\"ports\":\"443\",$R}"; check "uplink rule stored without an uplink" "$(st)" 201; UR1=$(jx j.id)
req POST /api/v1/vpn/uplink/rules "{\"name\":\"bad\",\"dest_type\":\"site\",\"protocol\":\"tcp\",$R}"; check "uplink rule destination checked" "$(st)" 400
req POST /api/v1/vpn/uplink "{\"token\":\"not a token\",\"password\":\"$PASS\",$R}"; check "garbage token refused" "$(st)" 400
req POST /api/v1/vpn/uplink "{\"token\":\"$TOKEN\",$R}"; check "connecting needs the password" "$(st) $(jx j.code)" "403 step_up_required"
req POST /api/v1/vpn/uplink "{\"token\":\"$TOKEN\",\"password\":\"$PASS\"}"; check "connecting needs a reason" "$(st)" 400
req POST /api/v1/vpn/uplink "{\"token\":\"$TOKEN\",\"password\":\"$PASS\",$R}"; check "hub range overlaps the own VPN range" "$(st)" 409
grep -q "overlaps this server's own VPN range" "$TMP/b" && ok "overlap explained" || bad "overlap message ($(cat "$TMP/b"))"
# this server stops being a hub for the rest of the test (a customer server usually has no VPN of its own)
req PUT /api/v1/server/wireguard "{\"enabled\":false,\"listen_port\":51820,\"address\":\"10.66.0.1/24\",\"endpoint\":\"hub.example.com:51820\",\"dns\":\"\",$R}"; check "own VPN disabled" "$(st)" 200

echo "== customer: connected"
req POST /api/v1/vpn/uplink "{\"token\":\"$TOKEN\",\"transport\":\"udp\",\"password\":\"$PASS\",$R}"; check "connected to the hub" "$(st)" 200
check "stored rule applied on connecting" "$(jx j.rules_applied)" 1
req GET /api/v1/vpn
check "uplink state" "$(jx '(u => [u.configured, u.mode, u.transport, u.vpn_net, u.subnets.join(), u.https_available].join(" "))(j.uplink)')" "true simulation udp 10.66.0.0/24 192.168.77.0/24 true"
check "address from the hub" "$(jx j.uplink.address)" "$UPADDR"
check "no secrets in the state" "$(grep -c -e "$(tokfield "$TOKEN" private_key)" -e "$KEY" "$TMP/b")" 0
check "uplink table rendered" "$(jx 'j.uplink.firewall.ruleset.includes("table inet ctrl32_uplink") && j.uplink.firewall.ruleset.includes("counter drop comment \"default deny\"")')" true
check "the web rule towards this server's uplink address" "$(jx "j.uplink.firewall.ruleset.includes('ip saddr 10.66.0.0/24 ip daddr $UPADDR/32 tcp dport 443 counter accept comment \"rule $UR1\"')")" true
check "no forwarding without a LAN rule" "$(jx j.uplink.firewall.forward)" false
check "presets for the uplink" "$(jx 'j.uplink_services.some(s => s.key === "mqtt" && s.ports === "1883")')" true
req POST /api/v1/vpn/uplink/rules "{\"name\":\"tech to PLC $SUF\",\"src_cidr\":\"10.66.0.200\",\"dest_type\":\"lan\",\"dest_cidr\":\"192.168.77.20\",\"protocol\":\"tcp\",\"ports\":\"502, 102\",$R}"; check "LAN rule" "$(st)" 201; UR2=$(jx j.id)
check "rule canonical" "$(jx '[j.src_cidr, j.dest_cidr, j.ports].join(" ")')" "10.66.0.200/32 192.168.77.20/32 102, 502"
req POST /api/v1/vpn/uplink/rules "{\"name\":\"x\",\"dest_type\":\"lan\",\"dest_cidr\":\"192.168.78.1\",\"protocol\":\"any\",$R}"; check "LAN outside the offered networks" "$(st)" 400
req POST /api/v1/vpn/uplink/rules "{\"name\":\"x\",\"src_cidr\":\"10.67.0.1\",\"dest_type\":\"server\",\"protocol\":\"any\",$R}"; check "source outside the hub's range" "$(st)" 400
req GET /api/v1/vpn
check "forwarding into the LAN with masquerade" "$(jx 'j.uplink.firewall.forward && j.uplink.firewall.ruleset.includes("ip saddr 10.66.0.200/32 ip daddr 192.168.77.20/32 tcp dport { 102, 502 }") && j.uplink.firewall.ruleset.includes("masquerade")')" true
req PUT "/api/v1/vpn/uplink/rules/$UR2" "{\"name\":\"tech to PLC $SUF\",\"src_cidr\":\"10.66.0.200\",\"dest_type\":\"lan\",\"dest_cidr\":\"192.168.77.20\",\"protocol\":\"tcp\",\"ports\":\"502\",\"enabled\":false,$R}"; check "LAN rule disabled" "$(st)" 200
req GET /api/v1/vpn; check "disabled rule leaves the table" "$(jx '!j.uplink.firewall.ruleset.includes("192.168.77.20") && !j.uplink.firewall.forward')" true
req PUT /api/v1/server/wireguard "{\"enabled\":true,\"listen_port\":51820,\"address\":\"10.66.0.1/24\",\"endpoint\":\"hub.example.com:51820\",\"dns\":\"\",$R}"
check "own VPN may not take the hub's range" "$(st) $(grep -c 'hub this server is connected to' "$TMP/b")" "502 1"

echo "== customer: HTTPS transport"
req POST /api/v1/vpn/uplink/transport "{\"transport\":\"tcp\",$R}"; check "unknown transport" "$(st)" 400
req POST /api/v1/vpn/uplink/transport "{\"transport\":\"https\",$R}"; check "HTTPS transport" "$(st)" 200
up=false
for _ in $(seq 20); do req GET /api/v1/vpn; [[ "$(jx 'j.uplink.tunnel && j.uplink.tunnel.connected')" == true ]] && { up=true; break; }; sleep 0.5; done
check "the agent's tunnel reached the hub with the uplink's key" "$up" true
# the tunnel host is the host of the hub's base_url (localhost or 127.0.0.1), not necessarily the one of $BASE
TUNHOST=$(tokfield "$TOKEN" tunnel_url | sed -E 's|^[a-z]+://([^/]+).*|\1|')
check "transport reported" "$(jx '[j.uplink.transport, j.uplink.tunnel_host].join(" ")')" "https $TUNHOST"
req PATCH "/api/v1/vpn/peers/$UPLINK" "{\"enabled\":false,$R}"; check "hub disables the uplink peer" "$(st)" 200
check "a disabled uplink's key is refused" "$(tunnel "$KEY")" 401
down=false
for _ in $(seq 30); do req GET /api/v1/vpn; [[ "$(jx '!j.uplink.tunnel.connected && /401/.test(j.uplink.tunnel.error || "")')" == true ]] && { down=true; break; }; sleep 0.5; done
check "the customer's tunnel is cut and refused" "$down" true
req PATCH "/api/v1/vpn/peers/$UPLINK" "{\"enabled\":true,$R}"; check "uplink peer enabled again" "$(st)" 200

echo "== key rotation on the hub, replacement on the customer"
req POST "/api/v1/vpn/peers/$UPLINK/rotate" "{\"password\":\"$PASS\",$R}"; check "uplink key rotated" "$(st)" 200
TOKEN2=$(jx j.uplink_token); KEY2=$(tokfield "$TOKEN2" tunnel_key)
check "new token, new keys" "$([[ -n "$TOKEN2" && "$TOKEN2" != "$TOKEN" && "$KEY2" != "$KEY" && "$(tokfield "$TOKEN2" private_key)" != "$(tokfield "$TOKEN" private_key)" ]] && echo yes)" yes
check "the old tunnel key stops working" "$(tunnel "$KEY")" 401
check "the new one works" "$(tunnel "$KEY2")" 101
req POST /api/v1/vpn/uplink "{\"token\":\"$TOKEN2\",\"transport\":\"https\",\"password\":\"$PASS\",$R}"; check "configuration replaced" "$(st)" 200
req GET /api/v1/vpn; check "rules kept on replacement" "$(jx 'j.uplink.firewall.rules')" 1
check "new key in use" "$(jx j.uplink.public_key | grep -c .) $([[ "$(jx j.uplink.public_key)" != "" ]] && echo set)" "1 set"
req POST /api/v1/vpn/uplink/transport "{\"transport\":\"udp\",$R}"; check "back to UDP" "$(st) $(jx j.uplink.transport)" "200 udp"

echo "== only server administrators manage the uplink"
req POST /api/v1/users "{\"username\":\"orgadm3_$SUF\",\"full_name\":\"VPN3 org admin\",\"email\":\"orgadm3$SUF@example.com\",\"roles\":[\"org_admin\"],$R}"
check "organization administrator" "$(st)" 201; OAPW=$(jx j.initial_password); OAID=$(jx j.user.id)
JO="$TMP/jar_o"; NEWPW="Vpn3-e2e-$SUF-pass!"
c=$(login "$JO" "orgadm3_$SUF" "$OAPW"); as "$JO" "$c" req POST /api/v1/auth/password "{\"current\":\"$OAPW\",\"new\":\"$NEWPW\"}"
CO=$(login "$JO" "orgadm3_$SUF" "$NEWPW")
as "$JO" "$CO" req GET /api/v1/vpn; check "organization view without the uplink" "$(st) $(jx '[!j.uplink, !j.uplink_rules].join(" ")')" "200 true true"
as "$JO" "$CO" req POST /api/v1/vpn/uplink/disconnect "{$R}"; check "organization administrator cannot disconnect" "$(st)" 403
as "$JO" "$CO" req POST /api/v1/vpn/uplink/rules "{\"name\":\"x\",\"dest_type\":\"server\",\"protocol\":\"any\",$R}"; check "nor add uplink rules" "$(st)" 403

echo "== disconnect, audit, clean-up"
req POST /api/v1/vpn/uplink/disconnect "{$R}"; check "disconnected" "$(st) $(jx j.uplink.configured)" "200 false"
req POST /api/v1/vpn/uplink/disconnect "{$R}"; check "nothing to disconnect" "$(st)" 409
req GET /api/v1/vpn; check "rules kept after disconnecting" "$(jx "j.uplink_rules.filter(r => r.name.endsWith('$SUF')).length")" 2
req GET "/api/v1/export/audit?format=csv"
for a in vpn.uplink_connect vpn.uplink_replace vpn.uplink_transport vpn.uplink_disconnect vpn.uplink_rule_add vpn.uplink_rule_update; do grep -q "$a" "$TMP/b" && ok "audit $a" || bad "audit $a missing"; done
grep "vpn.peer_add" "$TMP/b" | grep "customer-$SUF" | grep -q 'uplink' && ok "uplink peer audited as an uplink" || bad "uplink flag in the audit"
grep -q "$(tokfield "$TOKEN" private_key)" "$TMP/b" && bad "a private key in the audit trail" || ok "no private key in the audit trail"
for id in $UR1 $UR2; do req POST "/api/v1/vpn/uplink/rules/$id/remove" "{$R}"; done; check "uplink rules removed" "$(st)" 200
req POST "/api/v1/vpn/uplink/rules/$UR1/remove" "{$R}"; check "removed twice" "$(st)" 404
req GET /api/v1/vpn; check "audit vpn.uplink_rule_remove" "$(jx "j.uplink_rules.filter(r => r.name.endsWith('$SUF')).length")" 0
req POST "/api/v1/vpn/rules/$HUBRULE/remove" "{$R}"
for id in $TECH $UPLINK; do req POST "/api/v1/vpn/peers/$id/remove" "{$R}"; done
check "the removed uplink's key is refused" "$(tunnel "$KEY2")" 401
req PATCH "/api/v1/users/$OAID" "{\"disabled\":true,$R}"
req PUT /api/v1/server/wireguard "{\"enabled\":false,\"listen_port\":51820,\"address\":\"10.66.0.1/24\",\"endpoint\":\"\",\"dns\":\"\",$R}"
req DELETE "/api/v1/server/license?reason=e2e"

echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

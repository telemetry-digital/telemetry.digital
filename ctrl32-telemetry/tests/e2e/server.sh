#!/usr/bin/env bash
# End-to-end checks for the server screen API (URS K-06..K-09) on a host without the privileged agent:
# read-only information works, host operations are refused with 503 and every mutation needs a reason.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/server.sh
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
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  STATUS=$(cat "$TMP/s"); cat "$TMP/b"; }

echo "== pages and access"
for p in /server /server/domain /server/vpn /server/updates; do req GET $p >/dev/null; check "page $p served" "$(st)" 200; done
req GET /api/v1/server >/dev/null; check "server info needs auth" "$(st)" 401
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
ME=$(req GET /api/v1/auth/me); CSRF=$(echo "$ME" | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

echo "== information"
I=$(req GET /api/v1/server); check "server info" "$(st)" 200
echo "$I" | grep -q '"version":"' && ok "version" || bad "version"
echo "$I" | grep -q '"config":{' && ok "config summary" || bad "config"
echo "$I" | grep -q '"password\|"pepper\|"secret_key' && bad "secrets leaked" || ok "no secrets in config summary"
echo "$I" | grep -q '"database":{' && ok "database facts" || bad "database"
echo "$I" | grep -q '"organizations":' && ok "organization count" || bad "org count"
echo "$I" | grep -q '"broker":{"connected_clients":' && ok "broker facts" || bad "broker"
echo "$I" | grep -q '"agent":{' && ok "agent state" || bad "agent"
echo "$I" | grep -q '"update":{' && ok "update check" || bad "update"
AGENT=$(echo "$I" | grep -o '"agent":{[^}]*"connected":[a-z]*' | grep -o '"connected":[a-z]*' | cut -d: -f2)

echo "== validation"
req POST /api/v1/server/restart '{"unit":"app"}' >/dev/null; check "restart needs reason" "$(st)" 400
req POST /api/v1/server/restart '{"unit":"shell","reason":"x"}' >/dev/null; check "restart unit allow-list" "$(st)" 400
req PUT /api/v1/server/caddy '{"domain":"a.example.com"}' >/dev/null; check "caddy needs reason" "$(st)" 400
req PUT /api/v1/server/wireguard '{"enabled":true}' >/dev/null; check "wireguard needs reason" "$(st)" 400
req POST /api/v1/vpn/peers '{"name":"x","kind":"user"}' >/dev/null; check "peer needs reason" "$(st)" 400
req POST /api/v1/server/update '{"version":"../x","reason":"x"}' >/dev/null; check "update version validated" "$(st)" 400
req POST /api/v1/server/update '{"reason":"x"}' >/dev/null; check "update needs version" "$(st)" 400

if [[ "$AGENT" == "true" ]]; then
  echo "== agent (connected)"
  req GET /api/v1/server/wireguard >/dev/null; check "wireguard state" "$(st)" 200
  grep -q '"installed":' "$TMP/b" && ok "wireguard fields" || bad "wireguard fields"
  echo "$I" | grep -q '"host":{' && ok "host facts from agent" || bad "host"
  echo "$I" | grep -q '"services":\[' && ok "systemd units" || bad "services"
  echo "$I" | grep -q '"caddy":{' && ok "caddy state" || bad "caddy"
  req PUT /api/v1/server/caddy '{"domain":"not a domain","reason":"e2e"}' >/dev/null; check "agent validates domain" "$(st)" 502
  req PUT /api/v1/server/wireguard '{"enabled":false,"address":"nonsense","reason":"e2e"}' >/dev/null; check "agent validates address" "$(st)" 502
  req POST /api/v1/server/update '{"version":"99.99.99","reason":"e2e"}' >/dev/null; check "unknown release refused" "$(st)" 502
else
  echo "== without agent"
  for c in "POST /api/v1/server/restart {\"unit\":\"app\",\"reason\":\"e2e\"}" "GET /api/v1/server/wireguard" "PUT /api/v1/server/caddy {\"domain\":\"a.example.com\",\"reason\":\"e2e\"}" "PUT /api/v1/server/wireguard {\"enabled\":false,\"reason\":\"e2e\"}" "POST /api/v1/vpn/apply {\"reason\":\"e2e\"}" "POST /api/v1/vpn/rules/0192a000-0000-7000-8000-000000000001/remove {\"reason\":\"e2e\"}" "POST /api/v1/server/update {\"version\":\"0.1.0\",\"reason\":\"e2e\"}"; do
    set -- $c; req "$1" "$2" "${3:-}" >/dev/null; check "$1 $2 refused without agent" "$(st)" 503
  done
  grep -q "agent is not configured" "$TMP/b" && ok "clear message" || bad "message"
fi

echo "== backups validation"
req GET /server/backups >/dev/null; check "backups page served" "$(st)" 200
req POST /api/v1/server/backups '{"passphrase":""}' >/dev/null; check "backup needs reason" "$(st)" 400
req PUT /api/v1/server/backups/schedule '{"enabled":true,"hour":2,"minute":30,"keep":14}' >/dev/null; check "schedule needs reason" "$(st)" 400
req POST "/api/v1/server/backups/../etc/passwd/delete" '{"reason":"x"}' >/dev/null; [[ "$(st)" == "400" || "$(st)" == "404" || "$(st)" == "405" ]] && ok "backup name validated" || bad "backup name validated (got $(st))"
req GET "/api/v1/server/backups/evil.tar.gz" >/dev/null; check "download name validated" "$(st)" 400
if [[ "$AGENT" == "true" ]]; then
  echo "== backups (agent)"
  B=$(req GET /api/v1/server/backups); check "backups state" "$(st)" 200
  echo "$B" | grep -q '"db_mode":"' && ok "database mode detected" || bad "db_mode"
  req PUT /api/v1/server/backups/schedule '{"enabled":false,"hour":2,"minute":30,"keep":0,"reason":"e2e"}' >/dev/null; check "agent validates keep" "$(st)" 502
  req POST /api/v1/server/backups '{"passphrase":"e2e-pass","reason":"e2e backup"}' >/dev/null; check "backup started" "$(st)" 202
  for i in $(seq 1 40); do sleep 3; B=$(req GET /api/v1/server/backups); echo "$B" | grep -q '"running":false' && break; done
  echo "$B" | grep -q '"running":false' && ok "backup finished" || bad "backup still running"
  echo "$B" | grep -q '"last_error":""\|"last_file":"ctrl32-' && ok "no error" || bad "backup error: $(echo "$B" | grep -o '"last_error":"[^"]*"')"
  NAME=$(echo "$B" | grep -o '"name":"ctrl32-[0-9]*-[0-9]*\.tar\.gz\.enc"' | head -1 | cut -d'"' -f4)
  [[ -n "$NAME" ]] && ok "encrypted bundle listed ($NAME)" || bad "bundle listed"
  curl -s -b "$JAR" -o "$TMP/bk" -w '%{http_code}' "$BASE/api/v1/server/backups/$NAME" > "$TMP/s"; check "download" "$(st)" 200
  SZ=$(stat -c %s "$TMP/bk" 2>/dev/null || wc -c < "$TMP/bk"); [[ "$SZ" -gt 1000 ]] && ok "downloaded $SZ bytes" || bad "download size $SZ"
  head -c 8 "$TMP/bk" | grep -q "Salted__" && ok "bundle is openssl-encrypted" || bad "not encrypted"
  req POST "/api/v1/server/backups/$NAME/delete" '{"reason":"e2e cleanup"}' >/dev/null; check "delete" "$(st)" 200
  req GET /api/v1/server/backups >/dev/null; grep -q "\"name\":\"$NAME\"" "$TMP/b" && bad "still listed" || ok "deleted"
else
  for c in "GET /api/v1/server/backups" "POST /api/v1/server/backups {\"reason\":\"e2e\"}" "PUT /api/v1/server/backups/schedule {\"enabled\":false,\"hour\":2,\"minute\":30,\"keep\":14,\"reason\":\"e2e\"}" "POST /api/v1/server/backups/ctrl32-20260101-000000.tar.gz/delete {\"reason\":\"e2e\"}" "GET /api/v1/server/backups/ctrl32-20260101-000000.tar.gz"; do
    set -- $c; req "$1" "$2" "${3:-}" >/dev/null; check "$1 $2 refused without agent" "$(st)" 503
  done
fi


echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

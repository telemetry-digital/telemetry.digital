#!/usr/bin/env bash
# Check of the operational endpoints (0.34.0): security.txt (RFC 9116), Prometheus metrics, the NTP clock check on
# the Server page and in /healthz. Runs against a server with public_read on (the metrics are then public).
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/ops.sh
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
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$1" "$2"; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  cat "$TMP/b"; }
has()  { grep -q -- "$2" "$TMP/b" && ok "$1" || bad "$1 (missing: $2)"; }

echo "== security.txt"
curl -s -D "$TMP/h" -o "$TMP/b" -w '%{http_code}' "$BASE/.well-known/security.txt" > "$TMP/s"; check "security.txt" "$(st)" 200
grep -qi "^content-type: text/plain" "$TMP/h" && ok "text/plain" || bad "content type"
has "product security contact" "^Contact: mailto:info@telemetry.digital"
has "expires within a year" "^Expires: $(date -u +%Y)\|^Expires: $(( $(date -u +%Y) + 1 ))"
has "canonical" "^Canonical: .*/.well-known/security.txt"
has "policy served by the installation" "^Policy: http.*/security-policy"
POLICY=$(grep -o '^Policy: .*' "$TMP/b" | cut -d' ' -f2 | tr -d '\r')
curl -s -D "$TMP/h" -o "$TMP/p" -w '%{http_code}' "$POLICY" > "$TMP/s"; check "policy reachable" "$(st)" 200
grep -qi "^content-type: text/plain" "$TMP/h" && ok "policy is text" || bad "policy content type"
grep -q "^## Reporting a vulnerability" "$TMP/p" && grep -q "info@telemetry.digital" "$TMP/p" && ok "policy says how to report" || bad "policy text"
has "languages" "^Preferred-Languages: en"
curl -s -o /dev/null -D "$TMP/h" -w '%{http_code}' "$BASE/security.txt" > "$TMP/s"; check "/security.txt redirects" "$(st)" 301
grep -qi "^location: /.well-known/security.txt" "$TMP/h" && ok "redirect target" || bad "redirect location"

echo "== metrics"
curl -s -D "$TMP/h" -o "$TMP/b" -w '%{http_code}' "$BASE/metrics" > "$TMP/s"; check "metrics (public read)" "$(st)" 200
grep -qi "^content-type: text/plain; version=0.0.4" "$TMP/h" && ok "exposition format 0.0.4" || bad "content type: $(grep -i content-type "$TMP/h")"
for m in ctrl32_info ctrl32_process_uptime_seconds ctrl32_ingest_messages_total ctrl32_observations_total ctrl32_http_requests_total ctrl32_alarms_active ctrl32_gaps_open ctrl32_notifications_pending ctrl32_commands_pending ctrl32_db_pool_total_conns ctrl32_mqtt_clients ctrl32_ntp_synced ctrl32_ntp_offset_seconds ctrl32_audit_chain_ok; do
  has "metric $m" "^# TYPE $m "
done
has "version label" "^ctrl32_info{.*version=\""
has "audit chain verified" "^ctrl32_audit_chain_ok 1"
A=$(grep '^ctrl32_http_requests_total{class="2xx"}' "$TMP/b" | awk '{print $2}')
curl -s -o "$TMP/b" "$BASE/metrics"; B=$(grep '^ctrl32_http_requests_total{class="2xx"}' "$TMP/b" | awk '{print $2}')
[[ -n "$A" && -n "$B" && "$B" -gt "$A" ]] && ok "request counter grows ($A → $B)" || bad "request counter $A → $B"
grep -c "^# HELP" "$TMP/b" > "$TMP/n"; [[ $(cat "$TMP/n") -gt 20 ]] && ok "$(cat "$TMP/n") metrics documented" || bad "few metrics"

echo "== clock check on the Server page and in healthz"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token)
# the first NTP query runs in the background after start; give it up to 60 s before judging the clock
for _ in $(seq 1 30); do req GET /api/v1/server >/dev/null; [[ "$(jx "$TMP/b" 'j.time.checked_at')" > "2026" ]] && break; sleep 2; done
check "server info" "$(st)" 200
check "ntp enabled" "$(jx "$TMP/b" 'j.time.enabled')" true
check "ntp server" "$(jx "$TMP/b" 'j.time.server')" "pool.ntp.org"
[[ "$(jx "$TMP/b" 'j.time.checked_at')" > "2026" ]] && ok "checked at $(jx "$TMP/b" 'j.time.checked_at')" || bad "not checked yet"
check "clock ok (Internet reachable on the dev machine)" "$(jx "$TMP/b" 'j.time.ok')" true
[[ "$(jx "$TMP/b" 'Math.abs(j.time.offset_ms) < 1000')" == true ]] && ok "offset $(jx "$TMP/b" 'j.time.offset_ms') ms" || bad "offset $(jx "$TMP/b" 'j.time.offset_ms')"
check "os sync reported" "$(jx "$TMP/b" '["yes","no","unknown"].includes(j.time.os_sync)')" true
check "metrics and security links" "$(jx "$TMP/b" 'j.links.metrics + " " + j.links.security_txt')" "/metrics /.well-known/security.txt"
curl -s -o "$TMP/b" "$BASE/healthz"; check "healthz ntp field" "$(jx "$TMP/b" 'j.ntp')" ok

echo
echo "ops: $pass passed, $fail failed"
[[ $fail -eq 0 ]]

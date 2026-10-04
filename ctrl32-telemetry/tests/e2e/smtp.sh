#!/usr/bin/env bash
# End-to-end checks of the outgoing e-mail in the user interface (0.76.0, spec 031): System → Server → E-mail (SMTP).
# An SMTP sink (tools/smtpsink, no TLS, no authentication) stands in for the mail server. Covers the permission, the
# reason, the validation (no encryption only to this host or the local network), the write-only password (never
# returned), the test e-mail with the SMTP conversation (delivered and failing), the delivery log, the audit trail,
# that the saved settings are used at once (a password reset link goes through them) and the return to config.toml.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/smtp.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d)
SINK_PID=""
trap '[[ -n "$SINK_PID" ]] && kill $SINK_PID 2>/dev/null; rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""
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
SUF=$RANDOM
SINK=127.0.0.1:2526
go build -o "$TMP/smtpsink" ./tools/smtpsink || { echo "cannot build tools/smtpsink"; exit 2; }
"$TMP/smtpsink" -listen "$SINK" -dir "$TMP/mail" > "$TMP/sink.log" 2>&1 &
SINK_PID=$!; sleep 0.5

echo "== access"
req GET /api/v1/server/smtp; check "needs sign-in" "$(st)" 401
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req GET /api/v1/server/smtp; SRC0=$(jx j.source)
check "state" "$(st) $(jx '["none","config"].includes(j.source) + " " + j.override')" "200 true false"
req POST /api/v1/users "{\"username\":\"smtp_$SUF\",\"full_name\":\"Engineer\",\"email\":\"smtp$SUF@example.com\",\"roles\":[\"engineer\"],\"reason\":\"e2e\"}"; PW=$(jx j.initial_password)
J2="$TMP/jar2"; CSRF2=""; req2() { call "$J2" "$CSRF2" "$@"; }
req2 POST /api/v1/auth/login "{\"username\":\"smtp_$SUF\",\"password\":\"$PW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
req2 GET /api/v1/server/smtp; check "an engineer cannot see it" "$(st)" 403
req2 POST /api/v1/server/smtp/test '{"to":"x@example.com"}'; check "an engineer cannot send a test" "$(st)" 403

echo "== validation"
req PUT /api/v1/server/smtp '{"host":"127.0.0.1","port":2526,"security":"none","from":"telemetry@example.com"}'; check "reason required" "$(st)" 400
req PUT /api/v1/server/smtp '{"host":"smtp.gmail.com","port":25,"security":"none","from":"telemetry@example.com","reason":"e2e"}'
check "no encryption to a public server refused" "$(st) $(jx 'j.error.includes("local network")')" "400 true"
req PUT /api/v1/server/smtp '{"host":"smtp.example.com","security":"ssl3","from":"telemetry@example.com","reason":"e2e"}'; check "unknown security refused" "$(st)" 400
req PUT /api/v1/server/smtp '{"host":"smtp.example.com","security":"tls","from":"Name <a@example.com>","reason":"e2e"}'; check "sender must be an address" "$(st)" 400
req PUT /api/v1/server/smtp '{"host":"smtp.example.com","security":"tls","from":"a@example.com","from_name":"x\r\nBcc: y","reason":"e2e"}'; check "header injection refused" "$(st)" 400

echo "== save, the password is write-only"
req PUT /api/v1/server/smtp '{"host":"smtp.example.com","port":587,"security":"starttls","user":"mailer","password":"Sup3r-Secret-'$SUF'","from":"telemetry@example.com","from_name":"Telemetry","reply_to":"ops@example.com","to":["ops@example.com"],"reason":"e2e smtp starttls"}'
check "saved" "$(st) $(jx j.ok)" "200 true"
req GET /api/v1/server/smtp
check "source ui, password stored" "$(jx 'j.source + " " + j.has_password + " " + j.settings.security + " " + j.settings.from_name')" "ui true starttls Telemetry"
grep -q "Sup3r-Secret" "$TMP/b" && bad "the password is returned" || ok "the password is never returned"
req PUT /api/v1/server/smtp '{"host":"smtp.example.com","port":587,"security":"starttls","user":"mailer","from":"telemetry@example.com","reason":"e2e keep password"}'
req GET /api/v1/server/smtp; check "an empty password keeps the stored one" "$(jx j.has_password)" true
req GET "/api/v1/audit?action=server.smtp&limit=5"
check "audited without the password" "$(jx 'j.entries.some(e=>e.reason==="e2e smtp starttls" && e.new_value && e.new_value.password_changed===true)') $(grep -c Sup3r-Secret "$TMP/b")" "true 0"

echo "== the relay of this host (no encryption, no login)"
req PUT /api/v1/server/smtp "{\"host\":\"127.0.0.1\",\"port\":2526,\"security\":\"none\",\"from\":\"telemetry@example.com\",\"from_name\":\"Telemetry e2e\",\"reply_to\":\"ops@example.com\",\"reason\":\"e2e local relay\"}"
check "saved with a warning" "$(st) $(jx '(j.warnings||[]).length > 0')" "200 true"
req GET /api/v1/server/smtp; check "the user cleared the password" "$(jx j.has_password)" false
req POST /api/v1/server/smtp/test '{"to":"dest@example.com"}'
check "test e-mail delivered" "$(st) $(jx j.ok)" "200 true"
check "the conversation is returned" "$(jx 'j.transcript.includes("250") && j.transcript.toUpperCase().includes("DATA")')" true
sleep 0.5
M=$(ls "$TMP/mail" 2>/dev/null | head -1)
if [[ -n "$M" ]] && grep -qi "dest@example.com" "$TMP/mail/$M" && grep -qiE "Reply-To:.*ops@example.com" "$TMP/mail/$M" && grep -qi "Telemetry e2e" "$TMP/mail/$M"; then ok "the sink received it (to, sender name, reply-to)"; else bad "message in the sink ($M)"; fi
req POST /api/v1/server/smtp/test '{"to":"dest@example.com","settings":{"host":"127.0.0.1","port":2599,"security":"none","from":"telemetry@example.com"}}'
check "unsaved form values tested: failure with the conversation" "$(st) $(jx 'j.ok + " " + j.transcript.includes("failed") + " " + (j.error.length > 0)')" "200 false true true"
req GET "/api/v1/notifications?limit=10"
check "delivery log: the test messages" "$(jx '(l => l.includes("sent") + " " + l.includes("failed"))(j.filter(n=>n.event==="test" && (n.subject||"").includes("test e-mail")).map(n=>n.status))')" "true true"
req GET "/api/v1/audit?action=server.smtp_test&limit=5"; check "tests audited" "$(jx 'j.entries.length >= 2')" true

echo "== used at once: a password reset link goes through the saved settings"
N0=$(ls "$TMP/mail" | wc -l)
curl -s -o /dev/null -X POST -H "Content-Type: application/json" -d "{\"login\":\"smtp_$SUF\"}" "$BASE/api/v1/auth/forgot"
sleep 1
N1=$(ls "$TMP/mail" | wc -l)
check "reset e-mail sent through the sink" "$((N1 > N0))" 1

echo "== back to config.toml"
req DELETE /api/v1/server/smtp '{"reason":"e2e reset"}'; check "removed" "$(st)" 200
req GET /api/v1/server/smtp; check "source again" "$(jx j.source)" "$SRC0"
req GET "/api/v1/audit?action=server.smtp_reset&limit=5"; check "reset audited" "$(jx 'j.entries.length >= 1')" true

echo
echo "smtp: $pass passed, $fail failed"
[[ $fail == 0 ]]

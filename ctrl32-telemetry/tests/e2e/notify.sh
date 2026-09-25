#!/usr/bin/env bash
# End-to-end check of notification channels and escalation (URS C-06): a webhook channel (HMAC-signed) receives
# a test message, an escalation policy with two steps is bound to an alarm rule, a limit violation over MQTT raises
# the alarm and delivers step 0 at once and step 1 after its delay; clearing notifies; acknowledging cancels the
# pending step; the delivery log and audit trail record everything. Needs Go (tools/devfill) and Node (webhook sink).
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 USER=admin PASS=admin1234 bash tests/e2e/notify.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
SINK_PORT=${SINK_PORT:-5090}
command -v go >/dev/null 2>&1 || export PATH="/d/tools/go/bin:$PATH"
TMP=$(mktemp -d); trap 'kill $SINK 2>/dev/null; rm -rf "$TMP"' EXIT
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
sink_count() { local c; c=$(grep -c "\"ev\":\"$1\"" "$TMP/sink.log" 2>/dev/null); echo "${c:-0}"; }
SUF=$RANDOM
SECRET="s3cret-$SUF"

echo "== webhook sink"
node -e 'const http=require("http"),fs=require("fs");http.createServer((q,s)=>{let b="";q.on("data",c=>b+=c);q.on("end",()=>{fs.appendFileSync(process.argv[1],JSON.stringify({sig:q.headers["x-ctrl32-signature"]||"",ev:q.headers["x-ctrl32-event"]||"",body:b})+"\n");s.end("ok")})}).listen(process.argv[2])' "$TMP/sink.log" "$SINK_PORT" &
SINK=$!
sleep 1; : > "$TMP/sink.log"; ok "sink on $SINK_PORT"

echo "== login"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

echo "== channels"
req POST /api/v1/notification-channels "{\"name\":\"bad-$SUF\",\"kind\":\"webhook\",\"settings\":{\"url\":\"ftp://x\"},\"reason\":\"e2e\"}" >/dev/null; check "webhook url validated" "$(st)" 400
req POST /api/v1/notification-channels "{\"name\":\"bad-$SUF\",\"kind\":\"email\",\"settings\":{\"recipients\":[]},\"reason\":\"e2e\"}" >/dev/null; check "email recipients required" "$(st)" 400
req POST /api/v1/notification-channels "{\"name\":\"bad-$SUF\",\"kind\":\"pigeon\",\"settings\":{},\"reason\":\"e2e\"}" >/dev/null; check "kind validated" "$(st)" 400
CH=$(req POST /api/v1/notification-channels "{\"name\":\"hook-$SUF\",\"kind\":\"webhook\",\"settings\":{\"url\":\"http://127.0.0.1:$SINK_PORT/hook\",\"secret\":\"$SECRET\"},\"reason\":\"e2e\"}" | jget id); check "webhook channel" "$(st)" 201
req GET /api/v1/notification-channels >/dev/null; [[ "$(jx "$TMP/b" "(j.find(c=>c.id==='$CH')||{}).has_secret")" == "true" && "$(jx "$TMP/b" "(j.find(c=>c.id==='$CH')||{}).settings.secret")" != "$SECRET" ]] && ok "secret stored, redacted in list" || bad "list: $(head -c 200 "$TMP/b")"
CH2=$(req POST /api/v1/notification-channels "{\"name\":\"mail-$SUF\",\"kind\":\"email\",\"settings\":{\"recipients\":[\"ops@example.com\"]},\"reason\":\"e2e\"}" | jget id); check "email channel" "$(st)" 201
req POST "/api/v1/notification-channels/$CH/test" '{}' >/dev/null; check "test message delivered" "$(st)" 200
sleep 0.5
[[ "$(sink_count test)" == "1" ]] && ok "sink received the test" || bad "sink: $(cat "$TMP/sink.log")"
SIGOK=$(node -e 'const c=require("crypto");const l=require("fs").readFileSync(process.argv[1],"utf8").trim().split("\n").pop();const r=JSON.parse(l);console.log(r.sig==="sha256="+c.createHmac("sha256",process.argv[2]).update(r.body).digest("hex"))' "$TMP/sink.log" "$SECRET"); [[ "$SIGOK" == "true" ]] && ok "HMAC signature valid" || bad "signature"

echo "== escalation policy"
req POST /api/v1/escalation-policies "{\"name\":\"pol-$SUF\",\"steps\":[],\"reason\":\"e2e\"}" >/dev/null; check "steps required" "$(st)" 400
req POST /api/v1/escalation-policies "{\"name\":\"pol-$SUF\",\"steps\":[{\"after_s\":0,\"channel_id\":\"00000000-0000-0000-0000-000000000000\"}],\"reason\":\"e2e\"}" >/dev/null; check "unknown channel refused" "$(st)" 400
POL=$(req POST /api/v1/escalation-policies "{\"name\":\"pol-$SUF\",\"steps\":[{\"after_s\":0,\"channel_id\":\"$CH\"},{\"after_s\":3,\"channel_id\":\"$CH\"}],\"reason\":\"e2e\"}" | jget id); check "policy" "$(st)" 201
req GET /api/v1/escalation-policies >/dev/null; [[ "$(jx "$TMP/b" "(j.find(p=>p.id==='$POL')||{}).steps.length")" == "2" && "$(jx "$TMP/b" "(j.find(p=>p.id==='$POL')||{}).steps[0].channel_name")" == "hook-$SUF" ]] && ok "policy listed with channel names" || bad "policies: $(head -c 200 "$TMP/b")"

echo "== data and rule"
SITE=$(req POST /api/v1/sites "{\"name\":\"Notify site $SUF\",\"reason\":\"e2e\"}" | jget id); [[ -n "$SITE" ]] && ok "site" || bad "site"
ASSET=$(req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"fridge\",\"name\":\"Notify fridge $SUF\",\"reason\":\"e2e\"}" | jget id); [[ -n "$ASSET" ]] && ok "asset" || bad "asset"
DS=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"t_c\",\"quantity\":\"temperature\",\"unit\":\"C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" | jget id); [[ -n "$DS" ]] && ok "datastream" || bad "datastream"
DEV=$(req POST /api/v1/devices "{\"external_id\":\"nt-$SUF\",\"name\":\"notify test\",\"reason\":\"e2e\"}" | jget id); [[ -n "$DEV" ]] && ok "device" || bad "device"
TOK=$(req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}' | jget token); [[ -n "$TOK" ]] && ok "token" || bad "token"
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$DS\",\"reason\":\"e2e\"}" >/dev/null; check "assign" "$(st)" 201
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$DS\",\"type\":\"high\",\"action\":30,\"escalation_policy_id\":\"00000000-0000-0000-0000-000000000000\",\"reason\":\"e2e\"}" >/dev/null; check "unknown policy refused on rule" "$(st)" 400
RULE=$(req POST /api/v1/alarm-rules "{\"datastream_id\":\"$DS\",\"type\":\"high\",\"action\":30,\"escalation_policy_id\":\"$POL\",\"reason\":\"e2e\"}" | jget id); check "rule with policy" "$(st)" 201
req GET "/api/v1/alarm-rules?datastream_id=$DS" >/dev/null; grep -q "\"escalation_policy_id\":\"$POL\"" "$TMP/b" && ok "rule lists its policy" || bad "rule: $(head -c 200 "$TMP/b")"
req DELETE "/api/v1/escalation-policies/$POL?reason=e2e" >/dev/null; check "policy in use cannot be revoked" "$(st)" 409

echo "== alarm → step 0 at once, step 1 after 3 s"
: > "$TMP/sink.log"
go run ./tools/devfill -addr "$MQTT" -user "nt-$SUF" -token "$TOK" -key t_c -start 35 -step 0 -n 1 -every 1 > "$TMP/pub.log" 2>&1 && ok "published 35 (above 30)" || bad "publish: $(tail -2 "$TMP/pub.log")"
sleep 2
[[ "$(sink_count raised)" == "1" ]] && ok "raised delivered immediately" || bad "raised: $(cat "$TMP/sink.log" | head -c 300)"
req GET /api/v1/alarms >/dev/null; ALARM=$(jx "$TMP/b" "(j.find(a=>a.key==='t_c' && !a.cleared_at)||{}).id"); [[ -n "$ALARM" && "$ALARM" != "null" ]] && ok "alarm active" || bad "alarm: $(head -c 200 "$TMP/b")"
sleep 8
[[ "$(sink_count escalated)" == "1" ]] && ok "escalated after the delay (alarm still active)" || bad "escalated: $(grep -c . "$TMP/sink.log") lines: $(cat "$TMP/sink.log" | head -c 300)"
req GET "/api/v1/notifications?limit=20" >/dev/null; [[ "$(jx "$TMP/b" "j.filter(n=>n.status==='sent' && n.alarm_id==='$ALARM').length")" == "2" ]] && ok "delivery log: two sent" || bad "log: $(head -c 300 "$TMP/b")"

echo "== clear"
go run ./tools/devfill -addr "$MQTT" -user "nt-$SUF" -token "$TOK" -key t_c -start 20 -step 0 -n 1 -every 1 > "$TMP/pub2.log" 2>&1; sleep 2
[[ "$(sink_count cleared)" == "1" ]] && ok "cleared notice delivered" || bad "cleared: $(cat "$TMP/sink.log" | head -c 300)"

echo "== acknowledge cancels the pending step"
: > "$TMP/sink.log"
go run ./tools/devfill -addr "$MQTT" -user "nt-$SUF" -token "$TOK" -key t_c -start 36 -step 0 -n 1 -every 1 > "$TMP/pub3.log" 2>&1; sleep 2
req GET /api/v1/alarms >/dev/null; ALARM2=$(jx "$TMP/b" "(j.find(a=>a.key==='t_c' && !a.cleared_at)||{}).id"); [[ -n "$ALARM2" && "$ALARM2" != "null" && "$ALARM2" != "$ALARM" ]] && ok "second alarm" || bad "second alarm"
req POST "/api/v1/alarms/$ALARM2/ack" '{"reason":"e2e ack"}' >/dev/null; check "acknowledged" "$(st)" 200
sleep 8
[[ "$(sink_count escalated)" == "0" ]] && ok "no escalation after acknowledgement" || bad "escalated anyway"
req GET "/api/v1/notifications?limit=20" >/dev/null; [[ "$(jx "$TMP/b" "j.filter(n=>n.status==='cancelled' && n.alarm_id==='$ALARM2').length")" == "1" ]] && ok "pending step cancelled in the log" || bad "log: $(head -c 300 "$TMP/b")"
go run ./tools/devfill -addr "$MQTT" -user "nt-$SUF" -token "$TOK" -key t_c -start 20 -step 0 -n 1 -every 1 > "$TMP/pub4.log" 2>&1; sleep 1

echo "== cleanup and audit"
req DELETE "/api/v1/alarm-rules/$RULE?reason=e2e" >/dev/null; check "rule disabled" "$(st)" 200
req DELETE "/api/v1/escalation-policies/$POL?reason=e2e" >/dev/null; check "policy revoked" "$(st)" 200
req DELETE "/api/v1/notification-channels/$CH?reason=e2e" >/dev/null; check "channel revoked" "$(st)" 200
req DELETE "/api/v1/notification-channels/$CH2?reason=e2e" >/dev/null; check "email channel revoked" "$(st)" 200
req POST "/api/v1/devices/$DEV/revoke" '{"reason":"e2e cleanup"}' >/dev/null; check "device revoked" "$(st)" 200
req GET "/api/v1/export/audit" >/dev/null
for a in notification_channel.create notification_channel.test escalation_policy.create escalation_policy.revoke notification_channel.revoke; do grep -q "$a" "$TMP/b" && ok "audit $a" || bad "audit $a missing"; done

echo
echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

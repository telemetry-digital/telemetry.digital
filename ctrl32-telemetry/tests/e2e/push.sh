#!/usr/bin/env bash
# End-to-end check of Web Push and the voice channel (URS C-06): the server publishes its VAPID key, a browser
# subscription (keys generated here, endpoint = a local sink) is registered, a test push and a web_push channel
# deliver encrypted, VAPID-signed messages to the sink, an alarm through an escalation policy reaches the browser,
# a push service answering 410 retires the subscription, and a voice_http channel calls the provider URL.
# Needs Go (tools/devfill) and Node (sink, key generation) — a developer machine, not the demo server.
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 USER=admin PASS=admin1234 bash tests/e2e/push.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
SINK_PORT=${SINK_PORT:-5094}
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
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -H "User-Agent: e2e-browser/1.0" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -H "User-Agent: e2e-browser/1.0" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  cat "$TMP/b"; }
sink_count() { local c; c=$(grep -c "\"path\":\"$1\"" "$TMP/sink.log" 2>/dev/null); echo "${c:-0}"; }
SUF=$RANDOM

echo "== sink (push service + voice provider)"
node -e 'const http=require("http"),fs=require("fs");http.createServer((q,s)=>{const b=[];q.on("data",c=>b.push(c));q.on("end",()=>{const body=Buffer.concat(b);fs.appendFileSync(process.argv[1],JSON.stringify({path:q.url.split("?")[0],query:q.url.split("?")[1]||"",auth:q.headers["authorization"]||"",enc:q.headers["content-encoding"]||"",ttl:q.headers["ttl"]||"",len:body.length})+"\n");if(q.url.startsWith("/gone")){s.statusCode=410;s.end("gone");}else{s.statusCode=201;s.end("ok");}})}).listen(process.argv[2])' "$TMP/sink.log" "$SINK_PORT" &
SINK=$!
sleep 1; : > "$TMP/sink.log"; ok "sink on $SINK_PORT"
# browser-side keys: an ECDH P-256 key pair (p256dh = uncompressed public point) and a 16-byte auth secret
node -e 'const c=require("crypto");const e=c.createECDH("prime256v1");e.generateKeys();const b=x=>x.toString("base64").replace(/\+/g,"-").replace(/\//g,"_").replace(/=+$/,"");console.log(b(e.getPublicKey())+" "+b(c.randomBytes(16)))' > "$TMP/keys"
P256=$(cut -d' ' -f1 "$TMP/keys"); AUTH=$(cut -d' ' -f2 "$TMP/keys"); [[ ${#P256} -gt 80 && ${#AUTH} -gt 20 ]] && ok "browser keys generated" || bad "keys: $(cat "$TMP/keys")"

echo "== login and VAPID key"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"
KEY=$(req GET /api/v1/push/key | jget public_key); check "public key served" "$(st)" 200
[[ ${#KEY} -gt 80 ]] && ok "VAPID public key (${#KEY} chars)" || bad "key: $KEY"
KEY2=$(req GET /api/v1/push/key | jget public_key); [[ "$KEY" == "$KEY2" ]] && ok "key is stable" || bad "key changed"

echo "== subscribe"
req POST /api/v1/push/subscriptions "{\"endpoint\":\"ftp://x\",\"keys\":{\"p256dh\":\"$P256\",\"auth\":\"$AUTH\"}}" >/dev/null; check "endpoint validated" "$(st)" 400
req POST /api/v1/push/subscriptions "{\"endpoint\":\"http://127.0.0.1:$SINK_PORT/push/$SUF\",\"keys\":{\"p256dh\":\"nope\",\"auth\":\"$AUTH\"}}" >/dev/null; check "keys validated" "$(st)" 400
SUB=$(req POST /api/v1/push/subscriptions "{\"endpoint\":\"http://127.0.0.1:$SINK_PORT/push/$SUF\",\"keys\":{\"p256dh\":\"$P256\",\"auth\":\"$AUTH\"},\"user_agent\":\"e2e browser\"}" | jget id); check "subscription registered" "$(st)" 201
req POST /api/v1/push/subscriptions "{\"endpoint\":\"http://127.0.0.1:$SINK_PORT/push/$SUF\",\"keys\":{\"p256dh\":\"$P256\",\"auth\":\"$AUTH\"}}" >/dev/null; check "same endpoint again is idempotent" "$(st)" 200
req GET /api/v1/push/subscriptions >/dev/null; [[ "$(jx "$TMP/b" "j.filter(s=>s.id==='$SUB').length")" == "1" && "$(jx "$TMP/b" "(j.find(s=>s.id==='$SUB')||{}).user_agent")" == "e2e browser" ]] && ok "listed with user agent" || bad "list: $(head -c 300 "$TMP/b")"
grep -q "p256dh\|\"auth\"" "$TMP/b" && bad "keys leaked in the list" || ok "keys not returned"

echo "== test push"
req POST /api/v1/push/test '{}' >/dev/null; check "test push" "$(st)" 200
[[ "$(jx "$TMP/b" "j.sent")" == "1" ]] && ok "sent to one browser" || bad "test result: $(cat "$TMP/b")"
sleep 0.5
[[ "$(sink_count "/push/$SUF")" == "1" ]] && ok "push service received the message" || bad "sink: $(cat "$TMP/sink.log" | head -c 300)"
L=$(grep "/push/$SUF" "$TMP/sink.log" | tail -n 1)
echo "$L" | grep -q '"auth":"vapid t=' && ok "VAPID authorization header" || bad "auth: $L"
echo "$L" | grep -q '"enc":"aes128gcm"' && ok "payload encrypted (aes128gcm)" || bad "encoding: $L"
[[ "$(echo "$L" | grep -o '"len":[0-9]*' | cut -d: -f2)" -gt 100 ]] && ok "encrypted body present" || bad "body: $L"
[[ "$(echo "$L" | grep -o '"ttl":"[0-9]*"' | cut -d'"' -f4)" -gt 0 ]] && ok "TTL set" || bad "ttl: $L"

echo "== web_push channel and an alarm through an escalation policy"
req POST /api/v1/notification-channels "{\"name\":\"push-bad-$SUF\",\"kind\":\"web_push\",\"settings\":{\"users\":[]},\"reason\":\"e2e\"}" >/dev/null; check "users required" "$(st)" 400
CH=$(req POST /api/v1/notification-channels "{\"name\":\"push-$SUF\",\"kind\":\"web_push\",\"settings\":{\"users\":[\"$USER\"]},\"reason\":\"e2e\"}" | jget id); check "web_push channel" "$(st)" 201
CHALL=$(req POST /api/v1/notification-channels "{\"name\":\"push-all-$SUF\",\"kind\":\"web_push\",\"settings\":{\"users\":[\"all\"]},\"reason\":\"e2e\"}" | jget id); check "web_push channel for everyone" "$(st)" 201
: > "$TMP/sink.log"
req POST "/api/v1/notification-channels/$CHALL/test" '{}' >/dev/null; check "channel test (all users)" "$(st)" 200
sleep 0.5; [[ "$(sink_count "/push/$SUF")" == "1" ]] && ok "channel test reached the browser" || bad "channel test sink: $(cat "$TMP/sink.log" | head -c 200)"
POL=$(req POST /api/v1/escalation-policies "{\"name\":\"push-pol-$SUF\",\"steps\":[{\"after_s\":0,\"channel_id\":\"$CH\"}],\"reason\":\"e2e\"}" | jget id); check "policy" "$(st)" 201
SITE=$(req POST /api/v1/sites "{\"name\":\"Push site $SUF\",\"reason\":\"e2e\"}" | jget id)
ASSET=$(req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"fridge\",\"name\":\"Push fridge $SUF\",\"reason\":\"e2e\"}" | jget id)
DS=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"t_c\",\"quantity\":\"temperature\",\"unit\":\"C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" | jget id)
DEV=$(req POST /api/v1/devices "{\"external_id\":\"pu-$SUF\",\"name\":\"push test\",\"reason\":\"e2e\"}" | jget id)
TOK=$(req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}' | jget token)
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$DS\",\"reason\":\"e2e\"}" >/dev/null; check "fixtures" "$(st)" 201
RULE=$(req POST /api/v1/alarm-rules "{\"datastream_id\":\"$DS\",\"type\":\"high\",\"action\":30,\"escalation_policy_id\":\"$POL\",\"reason\":\"e2e\"}" | jget id); check "alarm rule with the push policy" "$(st)" 201
: > "$TMP/sink.log"
go run ./tools/devfill -addr "$MQTT" -user "pu-$SUF" -token "$TOK" -key t_c -start 35 -step 0 -n 1 -every 1 > "$TMP/pub.log" 2>&1 || bad "publish"
sleep 2
[[ "$(sink_count "/push/$SUF")" == "1" ]] && ok "alarm pushed to the browser" || bad "alarm push: $(cat "$TMP/sink.log" | head -c 200)"
req GET "/api/v1/notifications?limit=10" >/dev/null; [[ "$(jx "$TMP/b" "j.filter(n=>n.kind==='web_push' && n.status==='sent' && n.event==='raised').length")" -ge 1 ]] && ok "delivery log: web_push sent" || bad "log: $(head -c 300 "$TMP/b")"
req GET /api/v1/push/subscriptions >/dev/null; [[ "$(jx "$TMP/b" "(j.find(s=>s.id==='$SUB')||{}).last_used_at")" != "null" ]] && ok "last_used_at recorded" || bad "last_used_at"
go run ./tools/devfill -addr "$MQTT" -user "pu-$SUF" -token "$TOK" -key t_c -start 20 -step 0 -n 1 -every 1 > "$TMP/pub2.log" 2>&1; sleep 1

echo "== a push service answering 410 retires the subscription"
SUB2=$(req POST /api/v1/push/subscriptions "{\"endpoint\":\"http://127.0.0.1:$SINK_PORT/gone/$SUF\",\"keys\":{\"p256dh\":\"$P256\",\"auth\":\"$AUTH\"},\"user_agent\":\"old phone\"}" | jget id); check "second subscription" "$(st)" 201
req POST /api/v1/push/test '{}' >/dev/null; [[ "$(jx "$TMP/b" "j.sent")" == "1" && "$(jx "$TMP/b" "j.failed")" == "1" ]] && ok "one sent, one failed" || bad "test 2: $(cat "$TMP/b")"
req GET /api/v1/push/subscriptions >/dev/null; [[ "$(jx "$TMP/b" "(j.find(s=>s.id==='$SUB2')||{}).revoked_at")" != "null" ]] && ok "gone subscription revoked" || bad "gone: $(head -c 300 "$TMP/b")"

echo "== voice_http channel"
req POST /api/v1/notification-channels "{\"name\":\"voice-$SUF\",\"kind\":\"voice_http\",\"settings\":{\"url\":\"http://127.0.0.1:$SINK_PORT/voice?to={to}&say={text}\",\"recipients\":[\"+421900000000\"]},\"reason\":\"e2e\"}" >/dev/null; check "voice channel" "$(st)" 201
VC=$(jget id < "$TMP/b"); : > "$TMP/sink.log"
req POST "/api/v1/notification-channels/$VC/test" '{}' >/dev/null; check "voice test" "$(st)" 200
sleep 0.5; grep -q '"path":"/voice"' "$TMP/sink.log" && grep -q 'to=%2B421900000000' "$TMP/sink.log" && ok "provider called with the number and text" || bad "voice sink: $(cat "$TMP/sink.log" | head -c 300)"

echo "== unsubscribe and cleanup"
req DELETE "/api/v1/push/subscriptions/$SUB" >/dev/null; check "unsubscribe" "$(st)" 200
req POST "/api/v1/notification-channels/$CH/test" '{}' >/dev/null; check "channel test without browsers fails cleanly" "$(st)" 502
req DELETE "/api/v1/alarm-rules/$RULE?reason=e2e" >/dev/null
req DELETE "/api/v1/escalation-policies/$POL?reason=e2e" >/dev/null; check "policy revoked" "$(st)" 200
for c in $CH $CHALL $VC; do req DELETE "/api/v1/notification-channels/$c?reason=e2e" >/dev/null; done; ok "channels revoked"
req POST "/api/v1/devices/$DEV/revoke" '{"reason":"e2e cleanup"}' >/dev/null; check "device revoked" "$(st)" 200
req GET "/api/v1/export/audit" >/dev/null; grep -q "push.subscribe" "$TMP/b" && ok "audit push.subscribe" || bad "audit push.subscribe missing"

echo
echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

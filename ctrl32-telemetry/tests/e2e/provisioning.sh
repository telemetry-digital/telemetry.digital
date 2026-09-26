#!/usr/bin/env bash
# End-to-end checks for provisioning profiles and the ThingsBoard device API (spec 005 US1): a device provisions
# itself over MQTT, reconnects with its token, sends telemetry that creates datastreams, answers an RPC command;
# HTTP provisioning, wrong secrets, pre-provisioned devices, approval, name pattern, device limit, secret rotation,
# revocation and the limit of failed attempts. Needs Go (tools/mqttsim) and a running server.
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 USER=admin PASS=admin1234 bash tests/e2e/provisioning.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d); trap 'kill $(jobs -p) 2>/dev/null; rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""
pass=0; fail=0
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
prov() { curl -s -o "$TMP/b" -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d "$1" "$BASE/api/v1/provision" > "$TMP/s"; }
SUF=$RANDOM

echo "== setup"
go build -o "$TMP/mqttsim" ./tools/mqttsim || { echo "cannot build tools/mqttsim"; exit 1; }
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)

echo "== profile"
req POST /api/v1/provision-profiles "{\"name\":\"Fleet $SUF\",\"reason\":\"e2e\"}"; check "profile created" "$(st)" 201
PID=$(jx j.id); KEY=$(jx j.key); SEC=$(jx j.secret)
[[ "$KEY" == pk_* && ${#SEC} -ge 32 ]] && ok "key and secret issued" || bad "key/secret: $KEY"
req GET /api/v1/provision-profiles; [[ "$(jx "JSON.stringify(j.find(p => p.id === '$PID'))")" != *"$SEC"* ]] && ok "secret not listed" || bad "secret listed"

echo "== MQTT provisioning, ThingsBoard device API"
"$TMP/mqttsim" tb -addr "$MQTT" -key "$KEY" -secret "$SEC" -name "tb-$SUF" -wait 14 > "$TMP/tb.log" 2>&1 &
for i in $(seq 40); do grep -q 'EVENT connected' "$TMP/tb.log" && break; sleep 0.25; done
grep -q 'EVENT provisioned' "$TMP/tb.log" && ok "provisioned over MQTT" || bad "provisioning: $(head -3 "$TMP/tb.log")"
grep -q 'EVENT connected' "$TMP/tb.log" && ok "reconnected with the token as user name" || bad "token login"
sleep 3
req GET /api/v1/devices; DEV=$(jx "(j.find(d => d.external_id === 'tb-$SUF') || {}).id")
[[ -n "$DEV" && "$DEV" != null ]] && ok "device created" || bad "device missing"
req POST "/api/v1/devices/$DEV/commands" '{"method":"set_led","args":{"on":true},"reason":"e2e rpc"}'; CID=$(jx j.id)
sleep 2
grep -q 'EVENT rpc .*"method":"set_led","params":{"on":true}' "$TMP/tb.log" && ok "RPC request delivered on v1/devices/me/rpc/request/{n}" || bad "rpc: $(grep rpc "$TMP/tb.log")"
req GET "/api/v1/devices/$DEV/commands"; check "RPC response acknowledges the command" "$(jx "(j.find(c => c.id === '$CID') || {}).status")" acked
grep -q 'EVENT attributes_response {"shared":{}}' "$TMP/tb.log" && ok "attribute request answered" || bad "attribute response"
req PUT "/api/v1/devices/$DEV/attributes" '{"attributes":{"interval":30},"reason":"e2e"}'; sleep 1
grep -q 'EVENT attributes {"interval":30}' "$TMP/tb.log" && ok "shared attribute update on v1/devices/me/attributes" || bad "attribute update: $(grep attributes "$TMP/tb.log")"
req GET "/api/v1/datastreams"
check "automatic datastreams" "$(jx "j.filter(d => d.asset === 'tb-$SUF').map(d => d.key + ':' + d.kind).sort().join(',')")" "battery_level:gauge,door:state,mode:state,temp:gauge"
MODE=$(jx "(j.find(d => d.asset === 'tb-$SUF' && d.key === 'mode') || {}).id")
req GET "/api/v1/observations?datastream_id=$MODE&limit=5"; check "text value stored" "$(jx 'j[0] && j[0].value_json')" eco
req GET "/api/v1/export/audit?format=csv"; grep -q "device.provision" "$TMP/b" && grep -q "datastream.auto_create" "$TMP/b" && ok "provisioning and datastreams audited" || bad "audit"

echo "== HTTP provisioning"
prov "{\"deviceName\":\"http-$SUF\",\"provisionDeviceKey\":\"$KEY\",\"provisionDeviceSecret\":\"$SEC\"}"
check "HTTP provisioning" "$(st):$(jx j.status):$(jx j.credentialsType)" "200:SUCCESS:ACCESS_TOKEN"
prov "{\"deviceName\":\"http-$SUF\",\"provisionDeviceKey\":\"$KEY\",\"provisionDeviceSecret\":\"$SEC\"}"
check "second provisioning of the same device refused" "$(jx j.status)" FAILURE
prov "{\"deviceName\":\"x-$SUF\",\"provisionDeviceKey\":\"$KEY\",\"provisionDeviceSecret\":\"wrong\"}"
check "wrong secret refused" "$(st):$(jx j.status)" "400:FAILURE"
req GET /api/v1/devices; check "no device for a wrong secret" "$(jx "j.some(d => d.external_id === 'x-$SUF')")" false
prov "{\"deviceName\":\"bad name\",\"provisionDeviceKey\":\"$KEY\",\"provisionDeviceSecret\":\"$SEC\"}"; check "invalid device name refused" "$(jx j.status)" FAILURE

echo "== strategies and options"
req POST /api/v1/provision-profiles "{\"name\":\"Pre $SUF\",\"strategy\":\"preprovisioned\",\"name_pattern\":\"^pre-\",\"reason\":\"e2e\"}"; KEY2=$(jx j.key); SEC2=$(jx j.secret)
prov "{\"deviceName\":\"pre-unknown-$SUF\",\"provisionDeviceKey\":\"$KEY2\",\"provisionDeviceSecret\":\"$SEC2\"}"; check "unregistered device refused" "$(jx j.status)" FAILURE
req POST /api/v1/devices "{\"external_id\":\"pre-$SUF\",\"name\":\"pre\",\"transport\":\"mqtt\",\"reason\":\"e2e\"}"
prov "{\"deviceName\":\"pre-$SUF\",\"provisionDeviceKey\":\"$KEY2\",\"provisionDeviceSecret\":\"$SEC2\"}"; check "pre-registered device provisioned" "$(jx j.status)" SUCCESS
req POST /api/v1/devices "{\"external_id\":\"other-$SUF\",\"name\":\"o\",\"transport\":\"mqtt\",\"reason\":\"e2e\"}"
prov "{\"deviceName\":\"other-$SUF\",\"provisionDeviceKey\":\"$KEY2\",\"provisionDeviceSecret\":\"$SEC2\"}"; check "name pattern enforced" "$(jx j.status)" FAILURE

req POST /api/v1/provision-profiles "{\"name\":\"Approve $SUF\",\"approval\":true,\"max_devices\":1,\"reason\":\"e2e\"}"; PID3=$(jx j.id); KEY3=$(jx j.key); SEC3=$(jx j.secret)
prov "{\"deviceName\":\"appr-$SUF\",\"provisionDeviceKey\":\"$KEY3\",\"provisionDeviceSecret\":\"$SEC3\"}"; TOK3=$(jx j.credentialsValue)
check "approval pending" "$(jx j.status):$(jx j.approvalPending)" "SUCCESS:true"
"$TMP/mqttsim" tb -addr "$MQTT" -token "$TOK3" -name "appr-$SUF" -wait 8 > "$TMP/appr.log" 2>&1 &
sleep 1.5
grep -q 'EVENT connect_refused' "$TMP/appr.log" && ok "token refused until approved" || bad "pending device connected: $(head -2 "$TMP/appr.log")"
req GET /api/v1/device-approvals; ADEV=$(jx "(j.find(d => d.external_id === 'appr-$SUF') || {}).id")
[[ -n "$ADEV" && "$ADEV" != null ]] && ok "device in the approval list" || bad "approval list"
req POST "/api/v1/devices/$ADEV/approve" '{"reason":"e2e approve"}'; check "approved" "$(jx j.approval)" approved
for i in $(seq 20); do grep -q 'EVENT connected' "$TMP/appr.log" && break; sleep 0.5; done
grep -q 'EVENT connected' "$TMP/appr.log" && ok "approved device connects" || bad "approved device: $(tail -2 "$TMP/appr.log")"
prov "{\"deviceName\":\"appr2-$SUF\",\"provisionDeviceKey\":\"$KEY3\",\"provisionDeviceSecret\":\"$SEC3\"}"; check "device limit of the profile" "$(jx j.status)" FAILURE

req POST "/api/v1/provision-profiles/$PID/rotate" '{"reason":"e2e rotate"}'; NEW=$(jx j.secret)
prov "{\"deviceName\":\"rot-$SUF\",\"provisionDeviceKey\":\"$KEY\",\"provisionDeviceSecret\":\"$SEC\"}"; check "old secret refused after rotation" "$(jx j.status)" FAILURE
prov "{\"deviceName\":\"rot-$SUF\",\"provisionDeviceKey\":\"$KEY\",\"provisionDeviceSecret\":\"$NEW\"}"; check "new secret works" "$(jx j.status)" SUCCESS
req DELETE "/api/v1/provision-profiles/$PID?reason=e2e"; check "profile revoked" "$(st)" 200
prov "{\"deviceName\":\"rev-$SUF\",\"provisionDeviceKey\":\"$KEY\",\"provisionDeviceSecret\":\"$NEW\"}"; check "revoked profile refuses" "$(jx j.status)" FAILURE

echo "== limit of failed attempts"
for i in $(seq 12); do prov "{\"deviceName\":\"lim-$SUF\",\"provisionDeviceKey\":\"$KEY3\",\"provisionDeviceSecret\":\"nope\"}"; done
[[ "$(jx j.errorMsg)" == *"too many"* ]] && ok "failed attempts limited" || bad "no limit: $(jx j.errorMsg)"

for p in $(curl -s -b "$JAR" "$BASE/api/v1/provision-profiles" | node -pe 'JSON.parse(require("fs").readFileSync(0)).filter(p=>p.name.endsWith(" '$SUF'")).map(p=>p.id).join(" ")'); do req DELETE "/api/v1/provision-profiles/$p?reason=e2e"; done
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

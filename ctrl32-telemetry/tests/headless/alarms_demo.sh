#!/usr/bin/env bash
# Demo alarms for screenshots of the Alarms page (spec 035): one of each ISA state on a dev server.
set -u
cd "$(dirname "$0")/../.."
BASE=${BASE:-http://127.0.0.1:8080}; MQTT=${MQTT:-127.0.0.1:1883}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""
st()   { cat "$TMP/s"; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
go build -o "$TMP/devfill" ./tools/devfill || exit 1
SUF=$RANDOM
pub() { "$TMP/devfill" -addr "$MQTT" -user "demo-$SUF" -token "$TOK" -key "$1" -start "$2" -step 0 -n 1 -every 1 > /dev/null 2>&1; }
req POST /api/v1/auth/login '{"username":"admin","password":"admin1234"}'
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req POST /api/v1/sites "{\"name\":\"Plant $SUF\",\"reason\":\"demo\"}"; SITE=$(jx j.id)
req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"room\",\"name\":\"Reactor R1\",\"reason\":\"demo\"}"; ASSET=$(jx j.id)
mkds() { req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"$1\",\"quantity\":\"$2\",\"unit\":\"$3\",\"kind\":\"gauge\",\"reason\":\"demo\"}"; jx j.id; }
T=$(mkds temperature temperature "°C"); P=$(mkds pressure pressure bar); L=$(mkds level level m); F=$(mkds flow flow "m3/h"); V=$(mkds vibration vibration "mm/s")
req POST /api/v1/devices "{\"external_id\":\"demo-$SUF\",\"name\":\"demo plc\",\"reason\":\"demo\"}"; DEV=$(jx j.id)
req POST "/api/v1/devices/$DEV/token" '{"reason":"demo"}'; TOK=$(jx j.token)
for d in $T $P $L $F $V; do req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$d\",\"reason\":\"demo\"}"; done
req GET /api/v1/alarm-classes; MSG=$(jx "j.find(c => c.code === 'message').id")
rule() { req POST /api/v1/alarm-rules "$1"; }
rule "{\"datastream_id\":\"$T\",\"type\":\"high\",\"warning\":80,\"action\":95,\"hysteresis\":1,\"reason\":\"demo\"}"
rule "{\"datastream_id\":\"$P\",\"type\":\"high\",\"warning\":6,\"reason\":\"demo\"}"
rule "{\"datastream_id\":\"$L\",\"type\":\"low\",\"action\":0.5,\"reason\":\"demo\"}"
rule "{\"datastream_id\":\"$F\",\"type\":\"low\",\"action\":2,\"reason\":\"demo\"}"
rule "{\"datastream_id\":\"$V\",\"type\":\"high\",\"action\":4,\"class_id\":\"$MSG\",\"reason\":\"demo\"}"
pub temperature 70; pub pressure 4; pub level 2; pub flow 10; pub vibration 1; sleep 1
id() { req GET "/api/v1/alarms?datastream_id=$1"; jx 'j[0].id'; }
pub pressure 6.4; sleep 1; req POST "/api/v1/alarms/$(id "$P")/ack" '{"reason":"Seen, action taken"}'       # active, acknowledged (warning)
pub level 0.3; sleep 1; pub level 1.8; sleep 1                                                            # returned to normal, unacknowledged
pub flow 1.2; sleep 1; req POST "/api/v1/alarms/$(id "$F")/shelve" '{"minutes":120,"reason":"Flow meter being replaced"}'   # shelved
pub vibration 5.1; sleep 1                                                                                 # message class: no acknowledgement
for v in 96 70 97 70 98 70 99; do pub temperature $v; sleep 1; done                                        # chattering + unacknowledged active
echo "demo alarms ready (site Plant $SUF)"

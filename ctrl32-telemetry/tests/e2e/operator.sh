#!/usr/bin/env bash
# End-to-end checks of the operator view (spec 036): the place of a screen in the picture tree (validation: parent, cycle,
# depth), the tree with group alarms following raise / acknowledge / return / shelve, faceplate types bound to another
# asset or key prefix (tags, devices, commands), the popup parameters and the pages. Needs Go (tools/devfill) and Node.
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 USER=admin PASS=admin1234 bash tests/e2e/operator.sh
set -u
cd "$(dirname "$0")/../.."
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
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
go build -o "$TMP/devfill" ./tools/devfill || { echo "devfill build failed"; exit 1; }
SUF=$RANDOM
pub() { "$TMP/devfill" -addr "$MQTT" -user "op-a-$SUF" -token "$TOK" -key "$1" -start "$2" -step 0 -n 1 -every 1 > "$TMP/pub.log" 2>&1 || bad "publish $1=$2: $(tail -1 "$TMP/pub.log")"; }
# a node of the tree: an expression over it (n)
tn() { req GET /api/v1/screens/tree; jx "(n => n ? ($2) : 'none')(j.nodes.find(n => n.slug === '$1'))"; }
# save a screen: slug, name, the rest of the spec (JSON members), create|update
scr() { local body="{\"slug\":\"$1\",\"name\":\"$2\",\"reason\":\"e2e\",\"spec\":{\"version\":1,\"kind\":\"mimic\",\"widgets\":[]${3:+,$3}}}"
  if [[ "${4:-create}" == create ]]; then req POST /api/v1/dashboards "$body"; else req PUT "/api/v1/dashboards/$1" "$body"; fi; }

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req POST /api/v1/sites "{\"name\":\"Operator site $SUF\",\"reason\":\"e2e\"}"; SITE=$(jx j.id)
mkas() { req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"machine\",\"name\":\"$1\",\"reason\":\"e2e\"}"; jx j.id; }
mkds() { req POST /api/v1/datastreams "{\"asset_id\":\"$1\",\"key\":\"$2\",\"quantity\":\"speed\",\"unit\":\"rpm\",\"kind\":\"gauge\",\"reason\":\"e2e\"}"; jx j.id; }
mkdev() { req POST /api/v1/devices "{\"external_id\":\"$1\",\"name\":\"$1\",\"reason\":\"e2e\"}"; jx j.id; }
AA=$(mkas "Motor A $SUF"); AB=$(mkas "Motor B $SUF"); AC=$(mkas "Motor C $SUF"); PLC=$(mkas "PLC $SUF")
A_SPEED=$(mkds "$AA" speed); A_RUN=$(mkds "$AA" run); B_SPEED=$(mkds "$AB" speed); B_RUN=$(mkds "$AB" run); C_SPEED=$(mkds "$AC" speed)
M1=$(mkds "$PLC" m1_cur); M2=$(mkds "$PLC" m2_cur); SUPPLY=$(mkds "$PLC" supply)
DEVA=$(mkdev "op-a-$SUF"); DEVB=$(mkdev "op-b-$SUF")
req POST "/api/v1/devices/$DEVA/token" '{"reason":"e2e"}'; TOK=$(jx j.token)
for d in $A_SPEED $A_RUN $M1 $M2 $SUPPLY; do req POST "/api/v1/devices/$DEVA/datastreams" "{\"datastream_id\":\"$d\",\"reason\":\"e2e\"}"; done
for d in $B_SPEED $B_RUN; do req POST "/api/v1/devices/$DEVB/datastreams" "{\"datastream_id\":\"$d\",\"reason\":\"e2e\"}"; done; check "datastreams assigned to two devices" "$(st)" 201
el() { echo "{\"id\":\"$1\",\"type\":\"text\",\"x\":10,\"y\":10,\"w\":120,\"h\":30,\"text\":\"{value}\",\"tag\":\"$2\"${3:+,$3}}"; }

echo "== place in the picture tree"
P=op-plant-$SUF; T=op-tank-$SUF; M=op-motors-$SUF; B1=op-blower-$SUF
scr "$P" "Plant $SUF" '"nav":{"order":1}'; check "top-level picture" "$(st)" 201
check "nav stored" "$(jx "JSON.stringify(j.spec.nav)")" '{"order":1}'
scr "$T" "Tank $SUF" "\"nav\":{\"parent\":\"$P\",\"order\":20,\"label\":\"Tank\"},\"elements\":[$(el speed "$A_SPEED")]"; check "child picture" "$(st)" 201
scr "$M" "Motors $SUF" "\"nav\":{\"parent\":\"$P\",\"order\":10},\"elements\":[$(el cur "$M1")]"; check "second child" "$(st)" 201
scr "$B1" "Blower $SUF" "\"nav\":{\"parent\":\"$T\"},\"elements\":[$(el run "$A_RUN")]"; check "grandchild" "$(st)" 201
scr "op-x-$SUF" "X" '"nav":{"parent":"no-such-screen"}'; check "unknown parent refused" "$(st)" 400
scr "op-x-$SUF" "X" "\"nav\":{\"parent\":\"op-x-$SUF\"}"; check "own parent refused" "$(st)" 400
scr "op-x-$SUF" "X" '"nav":{"order":10000}'; check "order above 9999 refused" "$(st)" 400
scr "op-x-$SUF" "X" '"nav":{"label":"a label far too long for one button"}'; check "long label refused" "$(st)" 400
scr "$P" "Plant $SUF" "\"nav\":{\"parent\":\"$B1\"}" update; check "cycle refused" "$(st)" 400
D3=op-d3-$SUF; D4=op-d4-$SUF; D5=op-d5-$SUF
scr "$D3" "D3" "\"nav\":{\"parent\":\"$B1\"}"; check "level 4" "$(st)" 201
scr "$D4" "D4" "\"nav\":{\"parent\":\"$D3\"}"; check "level 5" "$(st)" 201
scr "$D5" "D5" "\"nav\":{\"parent\":\"$D4\"}"; check "level 6 refused" "$(st)" 400
scr "op-free-$SUF" "Free $SUF" ''; check "a screen outside the tree" "$(st)" 201

echo "== tree"
req GET /api/v1/screens/tree; check "tree" "$(st)" 200
check "depth-first, siblings by order" "$(jx "j.nodes.filter(n => n.slug.endsWith('-$SUF')).map(n => n.depth + ':' + n.slug.replace('-$SUF', '').replace('op-', '')).join(' ')")" "1:plant 2:motors 2:tank 3:blower 4:d3 5:d4"
check "screens without nav are not in it" "$(jx "j.nodes.some(n => n.slug === 'op-free-$SUF')")" "false"
check "label, parent, children" "$(tn "$T" "n.label + ' ' + n.parent + ' ' + n.children + ' ' + n.kind")" "Tank $P 1 mimic"
check "default label is the name" "$(tn "$M" n.label)" "Motors $SUF"
check "home is a top-level picture" "$(jx "j.nodes.find(n => n.slug === j.home).depth")" "1"
check "quiet at the start" "$(tn "$P" "n.unack + ' ' + n.active + ' [' + n.color + '] ' + n.priority")" "0 0 [] 0"

echo "== group alarms"
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$A_SPEED\",\"type\":\"high\",\"action\":100,\"hysteresis\":1,\"reason\":\"e2e\"}"; check "alarm rule on a tag of the tank picture" "$(st)" 201
pub speed 50; sleep 1; pub speed 150; sleep 1
check "the picture of the tag" "$(tn "$T" "n.unack + ' ' + n.active + ' ' + n.color + ' ' + n.priority")" "1 1 #d63939 1"
check "its parent" "$(tn "$P" "n.unack + ' ' + n.active + ' ' + n.color")" "1 1 #d63939"
check "not the sibling" "$(tn "$M" "n.unack + ' ' + n.active")" "0 0"
check "not the child (another tag)" "$(tn "$B1" "n.unack + ' ' + n.active")" "0 0"
req GET "/api/v1/alarms?datastream_id=$A_SPEED"; AL=$(jx j[0].id)
req POST "/api/v1/alarms/$AL/ack" '{"reason":"seen, e2e"}'; check "acknowledged" "$(st)" 200
check "acknowledged and active: steady" "$(tn "$T" "n.unack + ' ' + n.active + ' ' + n.color")" "0 1 #d63939"
pub speed 50; sleep 1
check "returned to normal: quiet" "$(tn "$P" "n.unack + ' ' + n.active + ' [' + n.color + ']'")" "0 0 []"
pub speed 150; sleep 1; pub speed 50; sleep 1
check "came and went unseen: still waits" "$(tn "$T" "n.unack + ' ' + n.active")" "1 0"
req GET "/api/v1/alarms?datastream_id=$A_SPEED"; AL=$(jx j[0].id)
req POST "/api/v1/alarms/$AL/ack" '{"reason":"seen, e2e"}'
pub speed 150; sleep 1
req GET "/api/v1/alarms?datastream_id=$A_SPEED"; AL=$(jx j[0].id)
check "raised again" "$(tn "$T" "n.unack + ' ' + n.active")" "1 1"
req POST "/api/v1/alarms/$AL/shelve" '{"minutes":15,"reason":"sensor being replaced, e2e"}'; check "shelved" "$(st)" 200
check "a shelved alarm is not on the button" "$(tn "$P" "n.unack + ' ' + n.active")" "0 0"
req POST "/api/v1/alarms/$AL/unshelve" '{"reason":"done, e2e"}'
check "back after unshelving" "$(tn "$T" "n.unack + ' ' + n.active")" "1 1"
scr "$T" "Tank $SUF" "\"nav\":{\"parent\":\"$P\",\"order\":20,\"label\":\"Tank\"},\"elements\":[$(el other "$C_SPEED")]" update; check "new version without the tag" "$(st)" 200
check "the alarm left the picture" "$(tn "$T" "n.unack + ' ' + n.active")" "0 0"
check "and its parent" "$(tn "$P" "n.unack + ' ' + n.active")" "0 0"
req POST "/api/v1/alarms/$AL/ack" '{"reason":"cleanup, e2e"}'; pub speed 50

echo "== faceplate type: another asset"
FP=op-fp-$SUF
EL="$(el speed "$A_SPEED" "\"device\":\"$DEVA\",\"key\":\"speed\",\"on\":[{\"act\":\"command\",\"key\":\"speed\",\"value\":10},{\"act\":\"trend\"}]"),$(el run "$A_RUN" "\"dyn\":[{\"prop\":\"visible\",\"op\":\">\",\"v\":0},{\"prop\":\"blink\",\"tag\":\"$A_SPEED\",\"op\":\">\",\"v\":100}]"),$(el supply "$SUPPLY")"
scr "$FP" "Motor $SUF" "\"faceplate\":{\"asset\":\"$SITE\"},\"elements\":[$EL]"; check "unknown asset refused" "$(st)" 400
scr "$FP" "Motor $SUF" "\"faceplate\":{\"asset\":\"$AA\",\"prefix\":\"M1.\"},\"elements\":[$EL]"; check "bad prefix refused" "$(st)" 400
scr "$FP" "Motor $SUF" "\"faceplate\":{\"asset\":\"$AA\"},\"elements\":[$EL]"; check "faceplate type saved" "$(st)" 201
req GET "/api/v1/dashboards/$FP/instance?asset=$AB"; check "instance for another asset" "$(st)" 200
E="j.spec.elements"
check "tags re-bound by key" "$(jx "$E[0].tag + ' ' + $E[1].tag")" "$B_SPEED $B_RUN"
check "tag of a dynamic re-bound" "$(jx "$E[1].dyn[1].tag")" "$B_SPEED"
check "device and command follow" "$(jx "$E[0].device + ' ' + $E[0].on[0].key + ' ' + $E[0].on.length")" "$DEVB speed 2"
check "a tag of another asset stays" "$(jx "$E[2].tag")" "$SUPPLY"
check "instance described" "$(jx "j.instance.asset + ' ' + j.instance.name + ' ' + j.instance.tags + ' ' + j.instance.bound + ' ' + j.name")" "$AB Motor B $SUF 2 2 Motor $SUF"
req GET "/api/v1/dashboards/$FP/instance?tag=$B_RUN"; check "the asset of a tag" "$(jx "j.instance.asset + ' ' + $E[0].tag")" "$AB $B_SPEED"
req GET "/api/v1/dashboards/$FP/instance"; check "without parameters: the example" "$(jx "$E[0].tag + ' ' + $E[0].device")" "$A_SPEED $DEVA"
req GET "/api/v1/dashboards/$FP/instance?asset=$AC"; check "instance with a missing tag and no device" "$(st)" 200
check "the missing tag is unbound" "$(jx "$E[0].tag + ' ' + ($E[1].tag === undefined) + ' ' + j.instance.bound")" "$C_SPEED true 1"
check "no command reaches the example's device" "$(jx "($E[0].device === undefined) + ' ' + ($E[0].key === undefined) + ' ' + $E[0].on.map(o => o.act).join(',')")" "true true trend"
check "a dynamic of the missing tag is dropped, others re-bound" "$(jx "$E[1].dyn.length + ' ' + $E[1].dyn[1].tag")" "2 $C_SPEED"
req GET "/api/v1/dashboards/$FP"; check "the type itself is unchanged" "$(jx "$E[0].tag + ' ' + $E[1].dyn.length")" "$A_SPEED 2"
req GET "/api/v1/dashboards/$FP/instance?asset=$SITE"; check "unknown asset" "$(st)" 404
req GET "/api/v1/dashboards/$FP/instance?asset=nonsense"; check "asset must be an id" "$(st)" 400
req GET "/api/v1/dashboards/$FP/instance?prefix=M%202"; check "prefix must be simple" "$(st)" 400
req GET "/api/v1/dashboards/$P/instance"; check "an ordinary picture is no faceplate type" "$(st)" 400
req GET "/api/v1/dashboards/no-such-$SUF/instance"; check "unknown screen" "$(st)" 404

echo "== faceplate type: another key prefix"
FQ=op-fpp-$SUF
scr "$FQ" "Drive $SUF" "\"faceplate\":{\"asset\":\"$PLC\",\"prefix\":\"m1_\"},\"elements\":[$(el cur "$M1" "\"device\":\"$DEVA\",\"key\":\"m1_cur\",\"on\":[{\"act\":\"command\",\"key\":\"m1_start\",\"value\":true}]"),$(el supply "$SUPPLY")]"; check "type with a prefix" "$(st)" 201
req GET "/api/v1/dashboards/$FQ/instance?prefix=m2_"; check "instance for another prefix" "$(st)" 200
check "tag, device and keys" "$(jx "$E[0].tag + ' ' + $E[0].device + ' ' + $E[0].key + ' ' + $E[0].on[0].key")" "$M2 $DEVA m2_cur m2_start"
check "a key outside the prefix stays" "$(jx "$E[1].tag + ' ' + j.instance.prefix + ' ' + j.instance.tags")" "$SUPPLY m2_ 1"
req GET "/api/v1/dashboards/$FQ/instance?prefix=m9_"; check "a prefix without datastreams: unbound, no command" "$(jx "($E[0].tag === undefined) + ' ' + ($E[0].device === undefined) + ' ' + $E[0].on.length + ' ' + j.instance.bound")" "true true 0 0"

echo "== popup parameters"
scr "op-pop-$SUF" "Pop $SUF" "\"elements\":[$(el m "$B_RUN" "\"on\":[{\"act\":\"popup\",\"target\":\"$FP\"}]"),$(el d "$M2" "\"on\":[{\"act\":\"popup\",\"target\":\"$FQ\",\"asset\":\"$PLC\",\"prefix\":\"m2_\"}]")]"; check "popup actions saved" "$(st)" 201
check "asset and prefix kept" "$(jx "JSON.stringify(j.spec.elements[1].on[0])")" "{\"act\":\"popup\",\"asset\":\"$PLC\",\"prefix\":\"m2_\",\"target\":\"$FQ\"}"
check "a plain popup has none" "$(jx "JSON.stringify(j.spec.elements[0].on[0])")" "{\"act\":\"popup\",\"target\":\"$FP\"}"
scr "op-pop2-$SUF" "Pop2" "\"elements\":[$(el m "$B_RUN" "\"on\":[{\"act\":\"popup\",\"target\":\"$FP\",\"asset\":\"nonsense\"}]")]"; check "popup asset must be an id" "$(st)" 400

echo "== pages"
check "operator page" "$(curl -s -b "$JAR" -o "$TMP/p" -w '%{http_code}' "$BASE/operator")" 200
check "operator page of a picture" "$(curl -s -b "$JAR" -o "$TMP/p" -w '%{http_code}' "$BASE/operator/$P")" 200
grep -q 'id="opNav"' "$TMP/p" && ok "the frame is there" || bad "frame"

echo "== cleanup"
for s in "op-pop-$SUF" "$FQ" "$FP" "op-free-$SUF" "$D4" "$D3" "$B1" "$M" "$T" "$P"; do req DELETE "/api/v1/dashboards/$s?reason=e2e"; done; check "screens deleted" "$(st)" 200
req GET /api/v1/screens/tree; check "the tree no longer lists them" "$(jx "j.nodes.filter(n => n.slug.endsWith('-$SUF')).length")" "0"

echo
echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

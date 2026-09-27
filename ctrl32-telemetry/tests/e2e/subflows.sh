#!/usr/bin/env bash
# End-to-end checks for spec 009: subflows (a reusable part with its input and outputs, pinned versions, not
# deployable alone, commands inside count for the using flow), the Split node and groups on the canvas.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/subflows.sh
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
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
debug() { curl -s -b "$JAR" "$BASE/api/v1/flows/$1/debug" | node -pe "JSON.parse(require('fs').readFileSync(0)).filter(d => d.kind === 'debug').map(d => d.text).join(',')"; }
waitfor() { local n=${2:-40}; for i in $(seq "$n"); do eval "$1" && return 0; sleep 0.25; done; return 1; }
SUF=$RANDOM

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)

echo "== subflow"
SUB="{\"kind\":\"subflow\",\"nodes\":[{\"id\":\"in\",\"type\":\"sub_in\",\"x\":0,\"y\":0,\"settings\":{}},
 {\"id\":\"x\",\"type\":\"math\",\"x\":200,\"y\":0,\"settings\":{\"expr\":\"value * 2\"}},
 {\"id\":\"big\",\"type\":\"filter\",\"x\":400,\"y\":100,\"settings\":{\"condition\":\"value > 10\"}},
 {\"id\":\"o1\",\"type\":\"sub_out\",\"x\":600,\"y\":0,\"settings\":{\"port\":1}},
 {\"id\":\"o2\",\"type\":\"sub_out\",\"x\":600,\"y\":100,\"settings\":{\"port\":2}}],
 \"wires\":[{\"from\":\"in\",\"port\":0,\"to\":\"x\"},{\"from\":\"x\",\"port\":0,\"to\":\"o1\"},{\"from\":\"x\",\"port\":0,\"to\":\"big\"},{\"from\":\"big\",\"port\":0,\"to\":\"o2\"}]}"
req POST /api/v1/flows "{\"name\":\"Double $SUF\",\"reason\":\"e2e\",\"spec\":$SUB}"; check "subflow created" "$(st)" 201
SS=$(jx j.flow.slug); check "kind and outputs" "$(jx 'j.flow.kind + " " + j.flow.outputs + " " + j.problems.length')" "subflow 2 0"
req POST "/api/v1/flows/$SS/deploy" '{"reason":"e2e"}'; check "a subflow is not deployed alone" "$(st)" 400
BADSUB='{"kind":"subflow","nodes":[{"id":"t","type":"inject","x":0,"y":0,"settings":{}}],"wires":[]}'
req POST /api/v1/flows "{\"name\":\"Bad sub $SUF\",\"reason\":\"e2e\",\"spec\":$BADSUB}"; BS=$(jx j.flow.slug)
check "a subflow needs its input and no triggers" "$(jx 'j.problems.map(p => p.message).some(m => m.includes("exactly one subflow input")) && j.problems.some(p => p.message.includes("triggers belong"))')" true

echo "== flow using the subflow"
FLOW="{\"nodes\":[{\"id\":\"i\",\"type\":\"inject\",\"x\":0,\"y\":0,\"settings\":{}},
 {\"id\":\"s\",\"type\":\"subflow\",\"x\":200,\"y\":0,\"settings\":{\"flow\":\"$SS\"}},
 {\"id\":\"d1\",\"type\":\"debug\",\"x\":400,\"y\":0,\"settings\":{\"expr\":\"'o1 ' + value\"}},
 {\"id\":\"d2\",\"type\":\"debug\",\"x\":400,\"y\":100,\"settings\":{\"expr\":\"'o2 ' + value\"}},
 {\"id\":\"i2\",\"type\":\"inject\",\"x\":0,\"y\":200,\"settings\":{}},
 {\"id\":\"sp\",\"type\":\"split\",\"x\":200,\"y\":200,\"settings\":{}},
 {\"id\":\"d3\",\"type\":\"debug\",\"x\":400,\"y\":200,\"settings\":{\"expr\":\"'part ' + msg.index + '/' + msg.count + '=' + value\"}}],
 \"wires\":[{\"from\":\"i\",\"port\":0,\"to\":\"s\"},{\"from\":\"s\",\"port\":0,\"to\":\"d1\"},{\"from\":\"s\",\"port\":1,\"to\":\"d2\"},{\"from\":\"i2\",\"port\":0,\"to\":\"sp\"},{\"from\":\"sp\",\"port\":0,\"to\":\"d3\"}],
 \"groups\":[{\"id\":\"g1\",\"name\":\"Doubling\",\"color\":\"#60a5fa\",\"x\":-20,\"y\":-40,\"w\":640,\"h\":200}]}"
req POST /api/v1/flows "{\"name\":\"Uses double $SUF\",\"reason\":\"e2e\",\"spec\":$FLOW}"; check "flow created" "$(st)" 201
FS=$(jx j.flow.slug); check "no problems" "$(jx 'j.problems.map(p => p.node + ": " + p.message).join("; ")')" ""
req GET "/api/v1/flows/$FS"
check "subflow version pinned, outputs derived" "$(jx '(n => n.settings.version + " " + n.settings.outputs)(j.flow.spec.nodes.find(n => n.id === "s"))')" "1 2"
check "groups kept" "$(jx 'j.flow.spec.groups.map(g => g.name + " " + g.color).join()')" "Doubling #60a5fa"
req POST "/api/v1/flows/$FS/deploy" '{"reason":"e2e"}'; check "deploy" "$(st)" 200
req POST "/api/v1/flows/$FS/inject" '{"node":"i","value":7}'; req POST "/api/v1/flows/$FS/inject" '{"node":"i","value":3}'
waitfor '[[ "$(debug "$FS" | tr "," "\n" | grep -c "^o")" -ge 3 ]]' && ok "messages through the subflow" || bad "debug: $(debug "$FS")"
D=$(debug "$FS"); [[ "$D" == *"o1 14"* && "$D" == *"o2 14"* && "$D" == *"o1 6"* && "$D" != *"o2 6"* ]] && ok "both outputs as the subflow decides" || bad "debug: $D"
req POST "/api/v1/flows/$FS/inject" '{"node":"i2","value":"[5,6,7]"}'
waitfor '[[ "$(debug "$FS" | tr "," "\n" | grep -c "^part")" -ge 3 ]]' && ok "split into three messages" || bad "split: $(debug "$FS")"
[[ "$(debug "$FS")" == *"part 2/3=7"* ]] && ok "index and count" || bad "split: $(debug "$FS")"

echo "== versions are pinned"
SUB2=${SUB/value \* 2/value * 3}
req PUT "/api/v1/flows/$SS" "{\"spec\":$SUB2,\"reason\":\"triple\"}"; check "subflow v2" "$(jx j.flow.version)" 2
req POST "/api/v1/flows/$FS/inject" '{"node":"i","value":4}'
waitfor '[[ "$(debug "$FS")" == *"o1 8"* ]]' && ok "running flow keeps subflow v1" || bad "debug: $(debug "$FS")"
FLOW2=${FLOW/\"flow\":\"$SS\"/\"flow\":\"$SS\",\"version\":null}
req PUT "/api/v1/flows/$FS" "{\"spec\":$FLOW2,\"reason\":\"take subflow v2\"}"; check "flow saved" "$(st)" 200
req GET "/api/v1/flows/$FS"; check "pinned to v2" "$(jx 'j.flow.spec.nodes.find(n => n.id === "s").settings.version')" 2
req POST "/api/v1/flows/$FS/deploy" '{"reason":"e2e v2"}'; check "redeploy" "$(st)" 200
req POST "/api/v1/flows/$FS/inject" '{"node":"i","value":4}'
waitfor '[[ "$(debug "$FS")" == *"o1 12"* ]]' && ok "new subflow version after redeploy" || bad "debug: $(debug "$FS")"

echo "== commands inside a subflow"
req POST /api/v1/devices "{\"external_id\":\"sub-$SUF\",\"name\":\"sub device\",\"reason\":\"e2e\"}"; DEV=$(jx j.id)
CSUB="{\"kind\":\"subflow\",\"nodes\":[{\"id\":\"in\",\"type\":\"sub_in\",\"x\":0,\"y\":0,\"settings\":{}},{\"id\":\"c\",\"type\":\"command\",\"x\":200,\"y\":0,\"settings\":{\"device\":\"$DEV\",\"key\":\"x\",\"value\":\"value\"}}],\"wires\":[{\"from\":\"in\",\"port\":0,\"to\":\"c\"}]}"
req POST /api/v1/flows "{\"name\":\"Cmd sub $SUF\",\"reason\":\"e2e\",\"spec\":$CSUB}"; CS=$(jx j.flow.slug)
req POST /api/v1/flows "{\"name\":\"Uses cmd $SUF\",\"reason\":\"e2e\",\"spec\":{\"nodes\":[{\"id\":\"i\",\"type\":\"inject\",\"x\":0,\"y\":0,\"settings\":{}},{\"id\":\"s\",\"type\":\"subflow\",\"x\":200,\"y\":0,\"settings\":{\"flow\":\"$CS\"}}],\"wires\":[{\"from\":\"i\",\"port\":0,\"to\":\"s\"}]}}"
CF=$(jx j.flow.slug); check "commands through the subflow count" "$(jx j.flow.has_commands)" true

echo "== validation"
req POST /api/v1/flows "{\"name\":\"Stray $SUF\",\"reason\":\"e2e\",\"spec\":{\"nodes\":[{\"id\":\"i\",\"type\":\"inject\",\"x\":0,\"y\":0,\"settings\":{}},{\"id\":\"o\",\"type\":\"sub_out\",\"x\":0,\"y\":0,\"settings\":{}},{\"id\":\"s\",\"type\":\"subflow\",\"x\":0,\"y\":0,\"settings\":{\"flow\":\"nope-$SUF\"}}],\"wires\":[],\"groups\":[{\"id\":\"g\",\"name\":\"x\",\"color\":\"blue\"}]}}"
ST=$(jx j.flow.slug)
check "problems reported" "$(jx '["belong in a subflow", "not found", "group"].every(w => j.problems.some(p => p.message.includes(w)))')" true

echo "== cleanup"
for s in "$FS" "$CF" "$ST" "$BS" "$SS" "$CS"; do req DELETE "/api/v1/flows/$s?reason=e2e"; done; check "deleted" "$(st)" 200
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

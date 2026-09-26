#!/usr/bin/env bash
# End-to-end checks for visual automation flows (spec 004): the US1 flow (value above a limit for a while → notify and
# command) through the API with real telemetry, the audit origin of automated actions, dry run and test injection,
# invalid flows refused, versions and restore, the HTTP allow-list, incoming webhooks, export/import, four eyes for
# flows that send commands, stop and delete.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/flows.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; JAR2="$TMP/jar2"; CSRF=""; CSRF2=""
pass=0; fail=0
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
req2() { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR2" -c "$JAR2" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF2" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR2" -c "$JAR2" -X "$m" -H "X-CSRF-Token: $CSRF2" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
SUF=$RANDOM

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req GET /api/v1/flows/catalog; check "catalogue" "$(st)" 200
[[ "$(jx 'j.nodes.length >= 25 && ["trigger","logic","action"].every(g => j.nodes.some(n => n.group === g))')" == true ]] && ok "node library (triggers, logic, actions)" || bad "catalogue: $(jx 'j.nodes.length')"
req POST /api/v1/sites "{\"name\":\"Flow site $SUF\",\"reason\":\"e2e\"}"; SITE=$(jx j.id)
req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"machine\",\"name\":\"Mixer $SUF\",\"reason\":\"e2e\"}"; ASSET=$(jx j.id)
req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"temp\",\"quantity\":\"temperature\",\"unit\":\"C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}"; DS=$(jx j.id)
req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"temp_f\",\"quantity\":\"temperature\",\"unit\":\"F\",\"kind\":\"gauge\",\"reason\":\"e2e\"}"; DSF=$(jx j.id)
req POST /api/v1/devices "{\"external_id\":\"flow-$SUF\",\"name\":\"flow device\",\"transport\":\"http\",\"reason\":\"e2e\"}"; DEV=$(jx j.id)
req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}'; TOK=$(jx j.token)
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$DS\",\"reason\":\"e2e\"}"
req POST /api/v1/notification-channels "{\"name\":\"flow-hook-$SUF\",\"kind\":\"webhook\",\"settings\":{\"url\":\"http://127.0.0.1:9/hook\"},\"reason\":\"e2e\"}"; CH=$(jx j.id)
[[ -n "$DS" && -n "$DEV" && -n "$TOK" && -n "$CH" ]] && ok "datastreams, device, channel" || bad "setup"
publish() { curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -H "Authorization: Bearer $TOK" -d "{\"v\":{\"temp\":$1}}" "$BASE/api/v1/d/telemetry"; }

echo "== US1: value above a limit for a while → notify and command"
SPEC="{\"nodes\":[
 {\"id\":\"t\",\"type\":\"ds_value\",\"x\":40,\"y\":40,\"settings\":{\"datastream\":\"$DS\"}},
 {\"id\":\"hot\",\"type\":\"stable_for\",\"x\":260,\"y\":40,\"settings\":{\"condition\":\"value > 80\",\"seconds\":1}},
 {\"id\":\"mail\",\"type\":\"notify\",\"x\":480,\"y\":20,\"settings\":{\"channel\":\"$CH\",\"subject\":\"Too hot: {{ fixed(value, 1) }}\"}},
 {\"id\":\"cool\",\"type\":\"command\",\"x\":480,\"y\":100,\"settings\":{\"device\":\"$DEV\",\"key\":\"cooling\",\"value\":\"true\"}},
 {\"id\":\"f\",\"type\":\"math\",\"x\":260,\"y\":160,\"settings\":{\"expr\":\"value * 1.8 + 32\"}},
 {\"id\":\"store\",\"type\":\"derive\",\"x\":480,\"y\":160,\"settings\":{\"datastream\":\"$DSF\",\"expr\":\"round(value, 1)\"}},
 {\"id\":\"dbg\",\"type\":\"debug\",\"x\":700,\"y\":60,\"settings\":{\"expr\":\"'sent ' + msg.status\"}}],
 \"wires\":[{\"from\":\"t\",\"port\":0,\"to\":\"hot\"},{\"from\":\"hot\",\"port\":0,\"to\":\"mail\"},{\"from\":\"hot\",\"port\":0,\"to\":\"cool\"},
 {\"from\":\"t\",\"port\":0,\"to\":\"f\"},{\"from\":\"f\",\"port\":0,\"to\":\"store\"},{\"from\":\"cool\",\"port\":0,\"to\":\"dbg\"}]}"
req POST /api/v1/flows "{\"name\":\"Cooling $SUF\",\"reason\":\"e2e\",\"spec\":$SPEC}"; check "create flow" "$(st)" 201
SLUG=$(jx j.flow.slug); check "no problems" "$(jx 'j.problems.length')" 0
check "has commands" "$(jx j.flow.has_commands)" true
req POST "/api/v1/flows/$SLUG/deploy" '{}'; check "deploy needs a reason" "$(st)" 400
req POST "/api/v1/flows/$SLUG/deploy" '{"reason":"e2e go live"}'; check "deploy" "$(st)" 200
check "running" "$(jx j.status.running)" true
check "publish 85" "$(publish 85)" 202
sleep 1.4
check "publish 86 (still hot)" "$(publish 86)" 202
sleep 2
req GET "/api/v1/flows/$SLUG/debug"
check "command sent once (debug)" "$(jx 'j.filter(d => d.kind === "action" && d.text.startsWith("command")).length')" 1
check "notification attempted once" "$(jx 'j.filter(d => (d.kind === "action" || d.kind === "error") && d.node === "mail").length')" 1
req GET "/api/v1/devices/$DEV/commands"
[[ "$(jx 'j.some(c => (c.reason || "").includes("flow Cooling '$SUF' v1 node cool") && (c.issued_by_name || c.issued_by || "").includes("flow:"))')" == true ]] && ok "command carries the flow, version and node as origin" || bad "command origin: $(head -c 400 "$TMP/b")"
req GET "/api/v1/export/audit?format=csv"
grep -q "flow Cooling $SUF v1 node cool" "$TMP/b" && ok "audited with the flow origin" || bad "audit origin"
grep -q "flow.deploy" "$TMP/b" && ok "deploy audited" || bad "deploy audit"
req GET "/api/v1/datastreams/latest?ids=$DSF"
check "derived °F value stored" "$(jx "j['$DSF'] && j['$DSF'].value")" 186.8
req GET "/api/v1/flows/$SLUG/status"
[[ "$(jx 'j.nodes.t.in >= 2 && j.nodes.hot.out === 1')" == true ]] && ok "node counters" || bad "status: $(head -c 300 "$TMP/b")"

echo "== dry run and test injection"
req GET "/api/v1/devices/$DEV/commands"; N0=$(jx j.length)
publish 50 >/dev/null; sleep 0.5   # cool down first: "true for" is still active from the real values
req POST "/api/v1/flows/$SLUG/inject" '{"node":"t","value":99,"dry_run":true}'; check "dry-run injection" "$(st)" 202
sleep 1.8
req GET "/api/v1/flows/$SLUG/debug"
[[ "$(jx 'j.some(d => d.kind === "dry_action" && d.text.includes("would send write"))')" == true ]] && ok "dry run shows the command it would send" || bad "dry run debug"
req GET "/api/v1/devices/$DEV/commands"; check "nothing executed in the dry run" "$(jx j.length)" "$N0"
EDITED=$(printf '%s' "$SPEC" | sed 's/value > 80/value > 10/')
req POST "/api/v1/flows/$SLUG/inject" "{\"node\":\"t\",\"value\":20,\"spec\":$EDITED}"; check "inject into the edited (unsaved) flow" "$(st)" 202

echo "== invalid flows"
req POST /api/v1/flows/validate "{\"spec\":{\"nodes\":[{\"id\":\"a\",\"type\":\"script\",\"settings\":{}}],\"wires\":[]}}"
check "unknown node type refused" "$(jx j.ok)" false
req POST /api/v1/flows/validate "{\"spec\":{\"nodes\":[{\"id\":\"t\",\"type\":\"inject\",\"settings\":{}},{\"id\":\"a\",\"type\":\"math\",\"settings\":{}},{\"id\":\"b\",\"type\":\"math\",\"settings\":{}}],\"wires\":[{\"from\":\"t\",\"port\":0,\"to\":\"a\"},{\"from\":\"a\",\"port\":0,\"to\":\"b\"},{\"from\":\"b\",\"port\":0,\"to\":\"a\"}]}}"
[[ "$(jx 'j.problems.some(p => p.message.includes("loop"))')" == true ]] && ok "loop without delay refused" || bad "loop"
req POST /api/v1/flows/validate "{\"spec\":{\"nodes\":[{\"id\":\"t\",\"type\":\"inject\",\"settings\":{}},{\"id\":\"f\",\"type\":\"filter\",\"settings\":{\"condition\":\"os.exit(1)\"}}],\"wires\":[]}}"
check "bad expression refused" "$(jx j.ok)" false
req POST /api/v1/flows/validate "{\"spec\":{\"nodes\":[],\"wires\":[],\"script\":\"x\"}}"
check "unknown spec field refused" "$(jx j.ok)" false
req POST /api/v1/flows "{\"name\":\"Broken $SUF\",\"reason\":\"e2e\",\"spec\":{\"nodes\":[{\"id\":\"t\",\"type\":\"inject\",\"settings\":{}},{\"id\":\"c\",\"type\":\"command\",\"settings\":{}}],\"wires\":[]}}"
check "draft with problems can be saved" "$(st)" 201
[[ "$(jx 'j.problems.length > 0')" == true ]] && ok "problems reported" || bad "problems"
BROKEN=$(jx j.flow.slug)
req POST "/api/v1/flows/$BROKEN/deploy" '{"reason":"e2e"}'; check "invalid flow cannot be deployed" "$(st)" 422

echo "== versions and restore"
V2=$(printf '%s' "$SPEC" | sed 's/value > 80/value > 90/')
req PUT "/api/v1/flows/$SLUG" "{\"spec\":$V2,\"reason\":\"raise limit\"}"; check "save version 2" "$(st)" 200
check "version 2 is a draft, v1 keeps running" "$(jx 'j.flow.version + "/" + j.flow.state + "/" + j.flow.deployed_version')" "2/draft/1"
req POST "/api/v1/flows/$SLUG/restore" '{"version":1,"reason":"back to 80"}'; check "restore v1 as v3" "$(jx j.flow.version)" 3
req GET "/api/v1/flows/$SLUG/versions"; check "three versions" "$(jx j.length)" 3

echo "== HTTP allow-list and webhooks"
HSPEC="{\"nodes\":[{\"id\":\"w\",\"type\":\"webhook\",\"settings\":{}},{\"id\":\"h\",\"type\":\"http_out\",\"settings\":{\"url\":\"https://hooks.example.com/in\"}},{\"id\":\"d\",\"type\":\"debug\",\"settings\":{\"expr\":\"topic + ' ' + value + ' ' + msg.batch\"}}],\"wires\":[{\"from\":\"w\",\"port\":0,\"to\":\"d\"}]}"
req PUT /api/v1/flows/settings '{"allowed_hosts":[],"reason":"e2e reset"}'
req POST /api/v1/flows/validate "{\"spec\":$HSPEC}"
[[ "$(jx 'j.problems.some(p => p.message.includes("allow-list"))')" == true ]] && ok "host not on the allow-list refused" || bad "allow-list"
req PUT /api/v1/flows/settings '{"allowed_hosts":["hooks.example.com"],"reason":"e2e"}'; check "allow-list saved" "$(st)" 200
req POST /api/v1/flows/validate "{\"spec\":$HSPEC}"; check "allowed host accepted" "$(jx j.ok)" true
req POST /api/v1/flows "{\"name\":\"Hook $SUF\",\"reason\":\"e2e\",\"spec\":$HSPEC}"; HSLUG=$(jx j.flow.slug)
req POST "/api/v1/flows/$HSLUG/deploy" '{"reason":"e2e"}'; check "webhook flow deployed" "$(st)" 200
req GET "/api/v1/flows/$HSLUG"; HOOK=$(jx 'j.hooks.w')
[[ "$HOOK" == */api/v1/flow-hooks/fh_* ]] && ok "webhook URL" || bad "hook url: $HOOK"
HPATH=/api/v1/flow-hooks/${HOOK##*/flow-hooks/}
check "webhook accepted" "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d '{"value":5,"topic":"line1","batch":"B-7"}' "$BASE$HPATH")" 202
check "unknown webhook 404" "$(curl -s -o /dev/null -w '%{http_code}' -X POST -d '{}' "$BASE/api/v1/flow-hooks/fh_nope")" 404
sleep 1
req GET "/api/v1/flows/$HSLUG/debug"
[[ "$(jx 'j.some(d => d.text === "line1 5 B-7")')" == true ]] && ok "webhook message reached the flow" || bad "webhook debug: $(head -c 300 "$TMP/b")"

echo "== export and import"
req GET "/api/v1/flows/$SLUG/export"; check "export" "$(jx j.format)" "ctrl32-flow/1"
EXP=$(cat "$TMP/b")
req POST /api/v1/flows/import "$(node -e 'const j=JSON.parse(process.argv[1]); j.name="Imported '$SUF'"; j.slug=""; j.reason="e2e import"; console.log(JSON.stringify(j))' "$EXP")"; check "import" "$(st)" 201
check "imported content equal" "$(jx 'j.flow.nodes')" 7

echo "== four eyes for flows that send commands"
req PUT /api/v1/settings/policy '{"four_eyes_commands":true,"reason":"e2e on"}'; check "policy on" "$(st)" 200
req POST "/api/v1/flows/$SLUG/deploy" '{"reason":"deploy v3"}'; check "deploy becomes a change request" "$(st)" 202
CR=$(jx j.change_request.id)
req GET /api/v1/change-requests; [[ "$(jx "(j.find(c => c.id === '$CR') || {}).summary")" == *"deploy flow Cooling $SUF version 3"* ]] && ok "change request summary" || bad "summary: $(jx "(j.find(c => c.id === '$CR') || {}).summary")"
req POST "/api/v1/change-requests/$CR/approve" '{"reason":"self"}'; check "requester cannot approve" "$(st)" 403
NEWPW=Fl0w-e2e-$SUF-New-Passw0rd
req POST /api/v1/users "{\"username\":\"flow2_$SUF\",\"full_name\":\"Second Eyes\",\"email\":\"flow2$SUF@example.com\",\"roles\":[\"org_admin\"],\"reason\":\"e2e\"}"; PW=$(jx j.initial_password)
req2 POST /api/v1/auth/login "{\"username\":\"flow2_$SUF\",\"password\":\"$PW\"}"; CSRF2=$(req2 GET /api/v1/auth/me; jx j.csrf_token)
req2 POST /api/v1/auth/password "{\"current\":\"$PW\",\"new\":\"$NEWPW\"}"; req2 POST /api/v1/auth/login "{\"username\":\"flow2_$SUF\",\"password\":\"$NEWPW\"}"; CSRF2=$(req2 GET /api/v1/auth/me; jx j.csrf_token)
req2 POST "/api/v1/change-requests/$CR/approve" '{"reason":"checked the flow"}'
if [[ "$(st)" == 200 ]]; then ok "second user approved"; req GET "/api/v1/flows/$SLUG"; check "version 3 running" "$(jx 'j.flow.deployed_version')" 3; else bad "approve: $(st) $(head -c 200 "$TMP/b")"; fi
req PUT /api/v1/settings/policy '{"four_eyes_commands":false,"reason":"e2e off"}'

echo "== stop and delete"
req POST "/api/v1/flows/$SLUG/disable" '{"reason":"e2e stop"}'; check "stop" "$(st)" 200
req GET "/api/v1/flows/$SLUG/status"; check "not running" "$(jx j.running)" false
N1=$(req GET "/api/v1/devices/$DEV/commands"; jx j.length)
publish 95 >/dev/null; sleep 2.2; req GET "/api/v1/devices/$DEV/commands"; check "stopped flow does nothing" "$(jx j.length)" "$N1"
for s in "$SLUG" "$HSLUG" "$BROKEN"; do req DELETE "/api/v1/flows/$s?reason=e2e"; done; check "delete" "$(st)" 200
req GET /api/v1/flows; [[ "$(jx "j.some(x => x.flow.slug === '$SLUG')")" == false ]] && ok "deleted flow gone" || bad "list"

echo "== automation rule → flow"
req POST /api/v1/rules "{\"name\":\"Rule to flow $SUF\",\"spec\":{\"inputs\":[{\"name\":\"t\",\"datastream_id\":\"$DS\"}],\"conditions\":[{\"input\":\"t\",\"op\":\">\",\"value\":50},{\"input\":\"t\",\"op\":\"stale\",\"value\":600}],\"match\":\"any\",\"for_s\":0,\"actions\":[{\"type\":\"derive\",\"datastream_id\":\"$DSF\",\"expr\":\"t * 10\"}]},\"reason\":\"e2e\"}"; RULE=$(jx j.id)
req PUT "/api/v1/rules/$RULE" '{"enabled":false,"reason":"e2e: the flow takes over"}'; check "rule created and stopped" "$(st)" 200
req GET "/api/v1/rules/$RULE/as-flow"; check "rule converted" "$(st)" 200
check "converted nodes" "$(jx 'j.spec.nodes.map(n => n.type).sort().join(",")')" "change,comment,derive,ds_value,join,stable_for"
check "condition rewritten" "$(jx 'j.spec.nodes.find(n => n.type === "stable_for").settings.condition')" "msg.t > 50"
check "staleness reported" "$(jx 'j.notes.length + ":" + j.problems.length')" "1:0"
CONV=$(cat "$TMP/b")
req POST /api/v1/flows "$(node -e 'const c=JSON.parse(process.argv[1]); console.log(JSON.stringify({name: c.name, spec: c.spec, reason: "e2e convert"}))' "$CONV")"; check "flow created from the rule" "$(st)" 201
RSLUG=$(jx j.flow.slug)
req POST "/api/v1/flows/$RSLUG/deploy" '{"reason":"e2e"}'; check "converted flow deployed" "$(st)" 200
publish 61 >/dev/null; sleep 1.5
req GET "/api/v1/datastreams/latest?ids=$DSF"; check "converted flow acts like the rule" "$(jx "j['$DSF'] && j['$DSF'].value")" 610
curl -s -o "$TMP/b" "$BASE/metrics"; grep -q '^ctrl32_flows_running [1-9]' "$TMP/b" && grep -q '^ctrl32_flow_messages_total [1-9]' "$TMP/b" && ok "flow metrics" || bad "flow metrics: $(grep ctrl32_flow "$TMP/b")"
req DELETE "/api/v1/flows/$RSLUG?reason=e2e"; req DELETE "/api/v1/rules/$RULE?reason=e2e"

echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

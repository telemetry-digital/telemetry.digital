#!/usr/bin/env bash
# End-to-end check of the operator action log (spec 039, 0.93.0): off by default (nothing is written), switched on in
# the organization policy; then commands, attribute changes, sign-ins and sign-outs are logged by the server with the
# user, the target, the page and the reason, pages report pictures and windows, the list filters and pages, the export
# works, and reading needs audit.read.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/operator_actions.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""; JAR2="$TMP/jar2"; CSRF2=""
pass=0; fail=0
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jget() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
call() { local jar=$1 csrf=$2 m=$3 p=$4 d=${5:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$jar" -c "$jar" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $csrf" -H "X-Ctrl32-Screen: ${SCREEN:-}" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$jar" -c "$jar" -X "$m" -H "X-CSRF-Token: $csrf" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
req()  { call "$JAR" "$CSRF" "$@"; }
req2() { call "$JAR2" "$CSRF2" "$@"; }
SUF=$RANDOM
FROM=$(date -u -d '-2 minutes' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v-2M +%Y-%m-%dT%H:%M:%SZ)
list() { req GET "/api/v1/operator-actions?from=$FROM&$1"; }

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jget csrf_token < "$TMP/b"); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"
req PUT /api/v1/settings/policy '{"log_operator_actions":false,"reason":"e2e start"}'
req POST /api/v1/devices "{\"external_id\":\"oplog-$SUF\",\"name\":\"Pump P$SUF\",\"transport\":\"http\",\"reason\":\"e2e\"}"; DEV=$(jget id < "$TMP/b"); check "device" "$(st)" 201
req POST /api/v1/users "{\"username\":\"olop_$SUF\",\"full_name\":\"Log Operator\",\"email\":\"olop$SUF@example.com\",\"roles\":[\"operator\"],\"reason\":\"e2e\"}"; check "operator created" "$(st)" 201
PW=$(jget initial_password < "$TMP/b"); NEWPW="Log-Operator-Passw0rd-$SUF"
req2 POST /api/v1/auth/login "{\"username\":\"olop_$SUF\",\"password\":\"$PW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jget csrf_token < "$TMP/b")
req2 POST /api/v1/auth/password "{\"current\":\"$PW\",\"new\":\"$NEWPW\"}"
req2 POST /api/v1/auth/login "{\"username\":\"olop_$SUF\",\"password\":\"$NEWPW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jget csrf_token < "$TMP/b")

echo "== off by default: nothing is written"
req GET /api/v1/settings/policy; check "the policy is off" "$(jx j.log_operator_actions)" false
req GET /api/v1/auth/me; check "the pages are told not to report" "$(jx j.log_actions)" false
SCREEN=/dashboards/plant req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"run","value":true},"reason":"before the log"}'; check "command" "$(st)" 201
req POST /api/v1/operator-actions '{"kind":"navigate","target":"Plant"}'; check "a report is accepted and not stored" "$(st):$(jx j.logged)" "200:false"
list "user=$USER"; check "the list is empty and says the log is off" "$(st):$(jx 'j.enabled + " " + j.actions.filter(a => /before the log|Plant/.test(a.reason + a.target)).length')" "200:false 0"

echo "== switching it on"
req2 PUT /api/v1/settings/policy '{"log_operator_actions":true,"reason":"x"}'; check "an operator cannot switch it on" "$(st)" 403
req PUT /api/v1/settings/policy '{"log_operator_actions":true}'; check "a reason is needed" "$(st)" 400
req PUT /api/v1/settings/policy '{"log_operator_actions":true,"reason":"e2e on"}'; check "switched on" "$(st):$(jx j.log_operator_actions)" "200:true"
req GET /api/v1/auth/me; check "the pages are told to report" "$(jx j.log_actions)" true
req GET "/api/v1/audit?limit=20"; check "the change of the policy is in the audit trail" "$(jx 'j.entries.some(e => e.action === "organization.policy" && e.reason === "e2e on")')" true

echo "== actions"
SCREEN="/operator/pumps%20hall" req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"run","value":true},"reason":"start the motor"}'; check "command" "$(st)" 201
list "kind=command"; check "the command is logged with who, what, where and why" "$(jx '(a => a ? [a.user, a.target, a.detail.args.key, a.detail.args.value, a.reason, a.screen, a.station.length > 0].join("|") : "none")(j.actions.find(a => a.reason === "start the motor"))')" "$USER|Pump P$SUF|run|true|start the motor|/operator/pumps hall|true"
req2 POST /api/v1/auth/login "{\"username\":\"olop_$SUF\",\"password\":\"$NEWPW\"}"; check "operator signs in" "$(st)" 200
req2 GET /api/v1/auth/me; CSRF2=$(jget csrf_token < "$TMP/b")
SCREEN=/dashboards/plant req2 POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"speed","value":42},"reason":"slower"}'; check "operator sends a command" "$(st)" 201
req2 POST /api/v1/operator-actions '{"kind":"navigate","target":"Pumps hall","detail":{"path":"/operator/pumps"}}'; check "a picture is reported" "$(st):$(jx j.logged)" "200:true"
req2 POST /api/v1/operator-actions '{"kind":"popup","target":"Pump P1","detail":{"screen":"fp-pump"}}'; check "a window is reported" "$(st)" 200
req2 POST /api/v1/operator-actions '{"kind":"cancel","target":"Stop","detail":{"key":"run","value":false}}'; check "a cancelled press is reported" "$(st)" 200
req2 POST /api/v1/operator-actions '{"kind":"command","target":"x"}'; check "a page cannot report a command" "$(st)" 400
req2 POST /api/v1/operator-actions '{"kind":"navigate","target":""}'; check "a target is needed" "$(st)" 400
req PUT "/api/v1/devices/$DEV/attributes" '{"attributes":{"mode":"auto"},"reason":"mode"}'; check "attributes" "$(st)" 200
req2 POST /api/v1/auth/logout '{}'; check "operator signs out" "$(st)" 200

echo "== the list"
req2 GET "/api/v1/operator-actions?from=$FROM"; check "reading needs audit.read (signed out here)" "$(st)" 401
list "user=olop_$SUF"; check "the operator's actions, newest first" "$(jx 'j.actions.map(a => a.kind).join(",")')" "logout,cancel,popup,navigate,command,login"
check "sign-in and sign-out name the user" "$(jx 'j.actions[0].target + " " + j.actions[5].target')" "olop_$SUF olop_$SUF"
check "users and kinds for the filters" "$(jx 'j.users.includes("olop_'$SUF'") + " " + j.kinds.includes("ptz") + " " + j.kinds.includes("recipe_load") + " " + j.kinds.includes("alarm_ack")')" "true true true true"
list "user=olop_$SUF&kind=navigate,popup"; check "filter by kind" "$(jx 'j.actions.length')" 2
list "user=olop_$SUF&q=slower"; check "search in the reason" "$(jx 'j.actions.length + " " + j.actions[0].kind')" "1 command"
list "user=olop_$SUF&q=fp-pump"; check "search in the detail" "$(jx 'j.actions.length')" 1
list "kind=attributes"; check "the attribute change is logged" "$(jx 'j.actions.some(a => a.target === "Pump P'$SUF'" && a.reason === "mode")')" true
list "user=olop_$SUF&limit=2"; B=$(jx 'encodeURIComponent(j.actions[1].ts)'); list "user=olop_$SUF&limit=2&before=$B"; check "paging with before" "$(jx 'j.actions.map(a => a.kind).join(",")')" "popup,navigate"
list "q=%25"; check "a percent sign is searched as text" "$(st):$(jx 'j.actions.length')" "200:0"
curl -s -b "$JAR" -o "$TMP/out.csv" -w '%{http_code}' "$BASE/api/v1/export/operator-actions?from=$FROM&format=csv" > "$TMP/s"; check "export" "$(st)" 200
grep -q "start the motor" "$TMP/out.csv" && head -1 "$TMP/out.csv" | grep -q "time,user,operator,kind,target,screen,reason" && ok "the export has the columns and the command" || bad "export: $(head -2 "$TMP/out.csv")"

echo "== permissions and switching off"
req2 POST /api/v1/auth/login "{\"username\":\"olop_$SUF\",\"password\":\"$NEWPW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jget csrf_token < "$TMP/b")
req2 GET "/api/v1/operator-actions?from=$FROM"; check "an operator cannot read the log" "$(st)" 403
req2 GET "/api/v1/export/operator-actions?from=$FROM"; check "nor export it" "$(st)" 403
req PUT /api/v1/settings/policy '{"log_operator_actions":false,"reason":"e2e off"}'; check "switched off" "$(st)" 200
SCREEN=/x req POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"run","value":false},"reason":"after the log"}'
list "q=after%20the%20log"; check "nothing is written when off, the old rows stay" "$(jx 'j.enabled + " " + j.actions.length')" "false 0"
list "q=start%20the%20motor"; check "old rows are still readable" "$(jx 'j.actions.filter(a => a.target === "Pump P'$SUF'").length')" 1
req DELETE "/api/v1/devices/$DEV?reason=e2e"

echo; echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

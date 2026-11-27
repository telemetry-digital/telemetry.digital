#!/usr/bin/env bash
# End-to-end check of SCADA users (spec 041, 0.95.0, ADR-0026 step 3): people who sign in at the controls with a PIN or
# a password on top of the session of the station; groups as in WinCC; a control names the groups that may use it and
# the server checks it; the SCADA user is recorded in the commands, the audit trail and the operator log; a member of a
# group that may approve is the second person with a PIN; retention of the operator log.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/scada_users.sh
set -u
cd "$(dirname "$0")/../.."
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""; JAR2="$TMP/jar2"; CSRF2=""; JAR3="$TMP/jar3"; CSRF3=""
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
req2() { call "$JAR2" "$CSRF2" "$@"; }
req3() { call "$JAR3" "$CSRF3" "$@"; }
tok()  { local m=$1 p=$2 d=${3:-}; printf '%s' "$d" > "$TMP/req"; curl -s -X "$m" -H "Content-Type: application/json" -H "Authorization: Bearer $TK" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"; }
SUF=$RANDOM
OFF='{"four_eyes_commands":false,"scada_users_required":false,"scada_idle_minutes":10,"operator_log_days":0,"log_operator_actions":true,"reason":"e2e reset"}'
cmd()  { local who=$1 key=$2 ctl=${3:-} extra=${4:-}; local c=""; [[ -n "$ctl" ]] && c=",\"control\":{\"dashboard\":\"$DASH\",\"id\":\"$ctl\"}"
  $who POST "/api/v1/devices/$DEV/commands" "{\"method\":\"write\",\"args\":{\"key\":\"$key\",\"value\":true},\"reason\":\"e2e\"$c$extra}"; }
signin() { $1 POST /api/v1/scada/session "{\"name\":\"$2\",\"secret\":\"$3\"}"; }
J="jana$SUF"; P="peter$SUF"; V="veduci$SUF"; V2="veduca$SUF"

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req PUT /api/v1/settings/policy "$OFF"; check "policy reset" "$(st)" 200
req POST /api/v1/devices "{\"external_id\":\"scu-$SUF\",\"name\":\"Line $SUF\",\"reason\":\"e2e\"}"; DEV=$(jx j.id); check "device" "$(st)" 201
STATION="station_$SUF"; NEWPW="Station-Passw0rd-$SUF"
req POST /api/v1/users "{\"username\":\"$STATION\",\"full_name\":\"Control room PC\",\"email\":\"scu$SUF@example.com\",\"roles\":[\"operator\"],\"reason\":\"e2e\"}"; check "the account of a station (role operator)" "$(st)" 201
PW=$(jx j.initial_password)
req2 POST /api/v1/auth/login "{\"username\":\"$STATION\",\"password\":\"$PW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
req2 POST /api/v1/auth/password "{\"current\":\"$PW\",\"new\":\"$NEWPW\"}"
req2 POST /api/v1/auth/login "{\"username\":\"$STATION\",\"password\":\"$NEWPW\"}"; check "the station signs in" "$(st)" 200
req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
req3 POST /api/v1/auth/login "{\"username\":\"$STATION\",\"password\":\"$NEWPW\"}"; req3 GET /api/v1/auth/me; CSRF3=$(jx j.csrf_token)

echo "== off by default, default groups"
req GET /api/v1/settings/policy; check "nobody is required, 10 minutes, the log is kept for good" "$(jx '[j.scada_users_required, j.scada_idle_minutes, j.operator_log_days].join(" ")')" "false 10 0"
req GET /api/v1/scada/groups; check "four groups as in WinCC" "$(jx 'j.filter(g => ["operator","shift_leader","technologist","maintenance"].includes(g.key)).map(g => g.key + ":" + g.approves).join(" ")')" "operator:false shift_leader:true technologist:false maintenance:false"
req2 GET /api/v1/auth/me; check "the pages are told who is at the controls: nobody" "$(jx '[j.scada.required, j.scada.user, j.scada.idle_minutes].join(" ")')" "false  10"
req2 GET /api/v1/scada/users; check "the station cannot read the SCADA users" "$(st)" 403
req2 POST /api/v1/scada/groups '{"name":"X","reason":"e2e"}'; check "nor change the groups" "$(st)" 403

echo "== administration"
mk() { req POST /api/v1/scada/users "{\"name\":\"$1\",\"full_name\":\"$2\",\"secret_kind\":\"$3\",\"secret\":\"$4\",\"groups\":$5,\"reason\":\"e2e\"}"; }
mk "$J" "Jana Operator" pin 12 '["operator"]'; check "a PIN of two digits is refused" "$(st)" 400
mk "$J" "Jana Operator" password short '["operator"]'; check "a short password is refused" "$(st)" 400
mk "$J" "Jana Operator" pin 1234 '["nonsense"]'; check "an unknown group is refused" "$(st)" 400
mk "Jana $SUF" "Jana Operator" pin 1234 '[]'; check "a name with a space is refused" "$(st)" 400
mk "$J" "Jana Operator" pin 1234 '["operator"]'; check "a SCADA user with a PIN" "$(st) $(jx 'j.secret_kind + " " + j.groups.join(",")')" "201 pin operator"; JID=$(jx j.id)
mk "$J" "Again" pin 1234 '[]'; check "the name is unique" "$(st)" 409
mk "$P" "Peter Technologist" password "Peter-heslo-1" '["technologist"]'; check "a SCADA user with a password" "$(st) $(jx j.secret_kind)" "201 password"; PID=$(jx j.id)
mk "$V" "Vedúci zmeny" pin 4321 '["shift_leader","operator"]'; check "a shift leader" "$(st)" 201; VID=$(jx j.id)
mk "$V2" "Vedúca zmeny" pin 8765 '["shift_leader"]'; check "another shift leader" "$(st)" 201
req GET /api/v1/scada/users; check "the list never shows a secret" "$(jx "(u => u ? Object.keys(u).filter(k => /secret|hash|pin/.test(k)).join(',') : 'none')(j.find(u => u.id === '$JID'))")" "secret_kind"
req POST /api/v1/scada/groups "{\"name\":\"Laborant $SUF\",\"description\":\"lab\",\"reason\":\"e2e\"}"; check "a group of one's own; the key comes from the name" "$(st) $(jx j.key)" "201 laborant_$SUF"; GID=$(jx j.id); GKEY=$(jx j.key)
req PUT "/api/v1/scada/groups/$GID" '{"name":"Laboratory","approves":true,"reason":"e2e"}'; check "renamed; the key stays" "$(st) $(jx 'j.name + " " + j.key + " " + j.approves')" "200 Laboratory $GKEY true"
req POST /api/v1/scada/groups '{"name":"No reason"}'; check "a change needs a reason" "$(st)" 400
req GET "/api/v1/audit?limit=20"; check "the administration is in the audit trail, without the secret" "$(jx "(e => e ? String(JSON.stringify(e).includes('1234')) : 'none')(j.entries.find(e => e.action === 'scada_user.create'))")" "false"

echo "== a dashboard with controls"
SPEC="{\"version\":1,\"kind\":\"dashboard\",\"layout\":{\"type\":\"grid\",\"row_height\":96},\"range\":\"6h\",\"widgets\":[
 {\"id\":\"plain\",\"type\":\"switch\",\"geometry\":{\"x\":0,\"y\":0,\"w\":3,\"h\":2},\"bindings\":{\"devices\":[\"$DEV\"]},\"params\":{\"title\":\"Light\",\"key\":\"light\"}},
 {\"id\":\"tech\",\"type\":\"switch\",\"geometry\":{\"x\":3,\"y\":0,\"w\":3,\"h\":2},\"bindings\":{\"devices\":[\"$DEV\"]},\"params\":{\"title\":\"Dosing\",\"key\":\"dosing\",\"scada_groups\":[\"technologist\"]}},
 {\"id\":\"lab\",\"type\":\"switch\",\"geometry\":{\"x\":6,\"y\":0,\"w\":3,\"h\":2},\"bindings\":{\"devices\":[\"$DEV\"]},\"params\":{\"title\":\"Sampler\",\"key\":\"sampler\",\"scada_groups\":[\"$GKEY\"]}},
 {\"id\":\"two\",\"type\":\"switch\",\"geometry\":{\"x\":9,\"y\":0,\"w\":3,\"h\":2},\"bindings\":{\"devices\":[\"$DEV\"]},\"params\":{\"title\":\"Main motor\",\"key\":\"motor\",\"second_person\":true}},
 {\"id\":\"txt\",\"type\":\"text\",\"geometry\":{\"x\":0,\"y\":2,\"w\":3,\"h\":2},\"params\":{\"text\":\"note\"}}]}"
req POST /api/v1/dashboards "{\"name\":\"SCADA users $SUF\",\"spec\":$SPEC,\"reason\":\"e2e\"}"; check "dashboard" "$(st)" 201; DASH=$(jx j.slug)
req GET "/api/v1/dashboards/$DASH"; check "the groups of a control are stored" "$(jx 'j.spec.widgets.find(w => w.id === "tech").params.scada_groups.join(",")')" "technologist"
BADSPEC=${SPEC/\"technologist\"/\"Not A Key\"}
req POST /api/v1/dashboards "{\"name\":\"Bad $SUF\",\"spec\":$BADSPEC,\"reason\":\"e2e\"}"; check "groups must be keys" "$(st)" 400
req POST /api/v1/dashboards "{\"name\":\"Picture $SUF\",\"spec\":{\"kind\":\"mimic\",\"widgets\":[],\"elements\":[{\"id\":\"m1\",\"type\":\"rect\",\"x\":0,\"y\":0,\"w\":50,\"h\":50,\"device\":\"$DEV\",\"key\":\"valve\",\"second_person\":true,\"scada_groups\":[\"maintenance\"]}]},\"reason\":\"e2e\"}"
check "a picture element keeps its groups and its mark" "$(st) $(jx '(e => e.scada_groups.join(",") + " " + e.second_person)(j.spec.elements[0])')" "201 maintenance true"; PIC=$(jx j.slug)

echo "== the control a command names"
cmd req light; check "a command without a control runs as before" "$(st)" 201
cmd req light plain; check "a control without groups: nobody has to sign in" "$(st)" 201
cmd req dosing plain; check "the key must be the one the control sends" "$(st)" 400
cmd req light nowhere; check "an unknown control" "$(st)" 400
cmd req light txt; check "a widget that sends no commands" "$(st)" 400
cmd req2 dosing tech; check "a control with groups: sign in first" "$(st) $(jx 'j.code + " " + j.groups.join(",")')" "403 scada_user_required Technologist"

echo "== signing in at the controls"
signin req2 "$J" 0000; check "a wrong PIN" "$(st) $(jx j.code)" "403 scada_failed"
signin req2 "nobody$SUF" 1234; check "an unknown name answers the same" "$(st) $(jx j.code)" "403 scada_failed"
signin req2 "$J" 1234; check "Jana signs in with her PIN" "$(st) $(jx 'j.user.name + " " + j.user.groups.join(",")')" "200 $J operator"
req2 GET /api/v1/auth/me; check "the station knows who is at the controls" "$(jx 'j.scada.user.name + " " + (j.scada.user.seconds_left > 500)')" "$J true"
req3 GET /api/v1/scada/session; check "another session of the same account is another station" "$(jx 'String(j.user)')" "null"
cmd req2 dosing tech; check "her group may not use the control" "$(st) $(jx 'j.code + " " + j.groups.join(",")')" "403 scada_group_required Technologist"
cmd req2 light plain; check "a control without groups works" "$(st)" 201; C0=$(jx j.id)
signin req2 "$P" "Peter-heslo-1"; check "Peter signs in with his password and replaces her" "$(st) $(jx j.user.name)" "200 $P"
cmd req2 dosing tech; check "the technologist may" "$(st)" 201; C1=$(jx j.id)
req GET "/api/v1/devices/$DEV/commands"; check "the command log names the SCADA user next to the station" "$(jx "(c => c.issued_by + ' ' + c.operator)(j.find(c => c.id === '$C1'))") $(jx "j.find(c => c.id === '$C0').operator")" "$STATION $P $J"
req GET "/api/v1/audit?limit=20"; check "so does the audit trail" "$(jx "String(j.entries.some(e => e.action === 'device.command' && e.actor === '$STATION / $P'))")" "true"
req GET "/api/v1/operator-actions?limit=100"; check "and the operator log: sign-ins, the refusal, the command" "$(jx "['scada_login','scada_failed','scada_denied','command'].map(k => j.actions.some(a => a.kind === k && (k === 'scada_failed' || a.operator !== '' || k === 'scada_login'))).join(' ')")" "true true true true"
check "the command row carries the operator" "$(jx "(a => a ? a.user + ' ' + a.operator : 'none')(j.actions.find(a => a.kind === 'command' && a.operator === '$P'))")" "$STATION $P"
req2 POST /api/v1/scada/session/touch; check "activity keeps the person signed in" "$(st) $(jx 'String(j.user.seconds_left > 590)')" "200 true"

echo "== API tokens are the system, not a person at the controls"
req POST /api/v1/tokens "{\"name\":\"scada e2e $SUF\",\"scopes\":[\"device.command\",\"data.read\"]}"; TK=$(jx j.token)
tok POST "/api/v1/devices/$DEV/commands" '{"method":"write","args":{"key":"light","value":true},"reason":"integration"}'; check "a token sends a command without a SCADA user" "$(st)" 201
tok POST "/api/v1/devices/$DEV/commands" "{\"method\":\"write\",\"args\":{\"key\":\"dosing\",\"value\":true},\"reason\":\"integration\",\"control\":{\"dashboard\":\"$DASH\",\"id\":\"tech\"}}"; check "but cannot use a control with groups" "$(st) $(jx j.code)" "403 scada_group_required"
tok POST /api/v1/scada/session "{\"name\":\"$P\",\"secret\":\"Peter-heslo-1\"}"; check "and cannot sign a SCADA user in" "$(st)" 403

echo "== the mark of a control is checked by the server"
cmd req2 motor two; check "a marked control holds the command even when the page does not say so" "$(st) $(jx 'j.status + " " + j.second_person.kind')" "202 pending command"; C2=$(jx j.id)
spot() { $1 POST /api/v1/second-person/approve "{\"kind\":\"command\",\"id\":\"$2\",\"device_id\":\"$DEV\",\"username\":\"$3\",\"password\":\"$4\",\"scada\":true}"; }
req2 GET /api/v1/second-person/waiting; check "the list shows the station and the person" "$(jx "(w => w ? w.requested_by : 'none')(j.waiting.find(w => w.id === '$C2'))")" "$STATION / $P"
spot req2 "$C2" "$J" 1234; check "an operator is not in a group that approves" "$(st)" 403
spot req2 "$C2" "$V" 9999; check "a wrong PIN of the second person" "$(st) $(jx j.code)" "403 second_person_failed"
spot req2 "$C2" "$V" 4321; check "the shift leader approves with a PIN" "$(st) $(jx 'j.approved_by')" "200 $STATION / $V"
req GET "/api/v1/devices/$DEV/commands"; check "the command carries both people" "$(jx "(c => c.operator + ' | ' + c.approved_by + ' | ' + (c.status !== 'pending'))(j.find(c => c.id === '$C2'))")" "$P | $STATION / $V | true"
req GET "/api/v1/audit?limit=20"; check "the audit trail names the second person and the station" "$(jx "String(j.entries.some(e => e.action === 'command.approve' && e.actor === '$STATION / $V' && e.reason.includes('at the station of $STATION')))")" "true"
signin req2 "$V" 4321; cmd req2 motor two; C3=$(jx j.id); check "the shift leader asks" "$(st)" 202
spot req2 "$C3" "$V" 4321; check "and cannot be his own second person" "$(st)" 403
spot req2 "$C3" "$V2" 8765; check "another shift leader can" "$(st)" 200
req2 DELETE /api/v1/scada/session; check "signing out" "$(st) $(jx 'String(j.user)')" "200 null"
cmd req2 motor two; C4=$(jx j.id); check "asked with nobody signed in" "$(st)" 202
spot req2 "$C4" "$V" 4321; check "then a PIN is not enough: an account has to approve" "$(st)" 403
req POST "/api/v1/devices/$DEV/commands/$C4/approve" '{"reason":"e2e"}'; check "an account approves from the list" "$(st)" 200

echo "== the organization requires a SCADA user"
req PUT /api/v1/settings/policy '{"scada_users_required":true,"scada_idle_minutes":1,"reason":"e2e"}'; check "switched on, one minute without activity" "$(st) $(jx '[j.scada_users_required, j.scada_idle_minutes].join(" ")')" "200 true 1"
req PUT /api/v1/settings/policy '{"scada_idle_minutes":0,"reason":"e2e"}'; check "the time has limits" "$(st)" 400
cmd req2 light plain; check "without a person at the controls nothing is sent" "$(st) $(jx j.code)" "403 scada_user_required"
signin req2 "$J" 1234; cmd req2 light plain; check "signed in: the control works" "$(st)" 201
cmd req2 light; check "a station cannot walk around the controls" "$(st) $(jx j.code)" "403 scada_control_required"
cmd req light; check "an account that manages devices works under its own name outside the pictures" "$(st)" 201
cmd req light plain; check "but at a control it signs in like everybody" "$(st) $(jx j.code)" "403 scada_user_required"
echo "   waiting 65 s for the sign-out without activity"; sleep 65
req2 GET /api/v1/scada/session; check "signed out after the time without activity" "$(jx 'String(j.user)')" "null"
cmd req2 light plain; check "and the control is closed again" "$(st) $(jx j.code)" "403 scada_user_required"
req PUT /api/v1/settings/policy '{"scada_users_required":false,"scada_idle_minutes":10,"reason":"e2e"}'; check "switched off" "$(st)" 200

echo "== lockout, disabling, removing"
for i in 1 2 3 4 5; do signin req3 "$J" 0000; done
signin req2 "$J" 1234; check "five wrong PINs lock the SCADA user, also for the right PIN" "$(st) $(jx j.code)" "403 scada_failed"
signin req3 "$J" 1234; check "and the station has to wait" "$(st)" 429
req PUT "/api/v1/scada/users/$JID" '{"unlock":true,"reason":"e2e"}'; check "the administrator unlocks" "$(st)" 200
signin req2 "$J" 1234; check "she signs in again" "$(st)" 200
req PUT "/api/v1/scada/users/$JID" '{"enabled":false,"reason":"e2e"}'; check "disabled" "$(st) $(jx j.enabled)" "200 false"
req2 GET /api/v1/scada/session; check "a disabled person is signed out at once" "$(jx 'String(j.user)')" "null"
signin req2 "$J" 1234; check "and cannot sign in" "$(st)" 403
signin req2 "$P" "Peter-heslo-1"; req PUT "/api/v1/scada/users/$PID" '{"secret_kind":"pin","secret":"246810","reason":"e2e"}'; check "the administrator changes a password to a PIN" "$(st) $(jx j.secret_kind)" "200 pin"
req2 GET /api/v1/scada/session; check "a new secret signs the person out" "$(jx 'String(j.user)')" "null"
signin req2 "$P" "Peter-heslo-1"; check "the old password is gone" "$(st)" 403
signin req2 "$P" 246810; check "the PIN works" "$(st)" 200
req PUT "/api/v1/scada/users/$PID" "{\"groups\":[\"$GKEY\"],\"reason\":\"e2e\"}"; cmd req2 sampler lab; check "a group of one's own opens its control" "$(st)" 201
req DELETE "/api/v1/scada/groups/$GID?reason=e2e"; check "the group is removed" "$(st)" 200
cmd req2 sampler lab; check "a control that names a removed group is closed to everybody" "$(st) $(jx j.code)" "403 scada_group_required"
req DELETE "/api/v1/scada/users/$PID?reason=e2e"; check "the SCADA user is removed" "$(st)" 200
req2 GET /api/v1/scada/session; check "and signed out" "$(jx 'String(j.user)')" "null"
req GET "/api/v1/devices/$DEV/commands"; check "the records keep the name" "$(jx "j.find(c => c.id === '$C1').operator")" "$P"

echo "== retention of the operator log"
req PUT /api/v1/settings/policy '{"operator_log_days":10,"reason":"e2e"}'; check "less than 30 days is refused" "$(st)" 400
req PUT /api/v1/settings/policy '{"operator_log_days":90,"reason":"e2e"}'; check "90 days" "$(st) $(jx j.operator_log_days)" "200 90"
if [[ -n "${DB_ADMIN:-}" ]] && command -v psql >/dev/null 2>&1; then
  ORG=$(psql -Atq "$DB_ADMIN" -c "SELECT org_id FROM core.device WHERE id = '$DEV'")
  psql -Atq "$DB_ADMIN" -c "INSERT INTO core.operator_action (org_id, ts, user_name, kind, target) VALUES ('$ORG', now() - interval '100 days', 'old', 'press', 'old-$SUF'), ('$ORG', now() - interval '40 days', 'mid', 'press', 'mid-$SUF')" >/dev/null
  check "the application cannot delete rows itself" "$(psql -Atq "$DB_ADMIN" -c "DELETE FROM core.operator_action WHERE target = 'mid-$SUF'" 2>&1 | grep -c 'not allowed')" "1"
  check "nothing younger than 30 days, whatever is asked" "$(psql -Atq "$DB_ADMIN" -c "SELECT core.purge_operator_actions('$ORG', 5)" 2>&1 | grep -c 'at least 30 days')" "1"
  check "the retention drops what is older than 90 days" "$(psql -Atq "$DB_ADMIN" -c "SELECT core.purge_operator_actions('$ORG', 90) >= 1")" "t"
  check "and keeps the rest" "$(psql -Atq "$DB_ADMIN" -c "SELECT count(*) FROM core.operator_action WHERE target IN ('old-$SUF', 'mid-$SUF')")" "1"
else
  echo "  skip (DB_ADMIN or psql missing): the database function of the retention"
fi
req PUT /api/v1/settings/policy "$OFF"; check "policy reset" "$(st)" 200

echo
echo "passed: $pass, failed: $fail"
[[ $fail -eq 0 ]]

#!/usr/bin/env bash
# Check of the incident log (URS C-07, I-09): manual incidents with impact/cause/CAPA and the workflow (closing needs
# a cause and a CAPA), history events, audit trail, filters; an alarm whose escalation step cannot be delivered
# (webhook to a closed port) opens an incident automatically; /metrics counts open incidents; an instance-level
# incident (the server's disk) is seen by server administrators only (needs DB_ADMIN, the database URL psql uses, to
# open one; tests/run-all.sh exports it). Needs Go (tools/devfill).
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 [DB_ADMIN=postgres://…] bash tests/e2e/incidents.sh
set -u
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
jget() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$1" "$2"; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  cat "$TMP/b"; }
SUF=$RANDOM

echo "== login"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

echo "== manual incident and workflow"
req POST /api/v1/incidents '{"title":"","reason":"e2e"}' >/dev/null; check "title required" "$(st)" 400
req POST /api/v1/incidents '{"title":"x"}' >/dev/null; check "reason required" "$(st)" 400
req POST /api/v1/incidents '{"title":"Sensor calibration overdue","severity":"bogus","reason":"e2e"}' >/dev/null; check "severity validated" "$(st)" 400
ID=$(req POST /api/v1/incidents "{\"title\":\"Sensor calibration overdue $SUF\",\"severity\":\"minor\",\"impact\":\"readings of fridge 3 unverified since Monday\",\"reason\":\"e2e\"}" | jget id); check "created" "$(st)" 201
check "status open" "$(jx "$TMP/b" 'j.status')" open
check "kind manual" "$(jx "$TMP/b" 'j.kind')" manual
check "opened by" "$(jx "$TMP/b" 'j.opened_by')" "$USER"
req GET /api/v1/incidents >/dev/null; check "listed among open" "$(jx "$TMP/b" "j.some(i=>i.id==='$ID')")" true
req GET "/api/v1/incidents/$ID" >/dev/null; check "detail" "$(st)" 200
check "first event opened" "$(jx "$TMP/b" 'j.events[0].event')" opened
req PATCH "/api/v1/incidents/$ID" '{"status":"closed","reason":"e2e"}' >/dev/null; check "close needs cause and CAPA" "$(st)" 400
req PATCH "/api/v1/incidents/$ID" '{"status":"resolved","reason":"e2e"}' >/dev/null; check "resolve needs a cause" "$(st)" 400
req PATCH "/api/v1/incidents/$ID" '{"note":"probe checked","reason":"e2e"}' >/dev/null; check "note without reason? no — with reason" "$(st)" 200
req PATCH "/api/v1/incidents/$ID" '{"cause":"probe drift beyond tolerance","reason":"e2e"}' >/dev/null; check "cause recorded" "$(st)" 200
req PATCH "/api/v1/incidents/$ID" '{"cause":"probe drift beyond tolerance","reason":"e2e"}' >/dev/null; check "no change → 400" "$(st)" 400
req PATCH "/api/v1/incidents/$ID" '{"status":"investigating","reason":"e2e"}' >/dev/null; check "investigating" "$(jx "$TMP/b" 'j.status')" investigating
req PATCH "/api/v1/incidents/$ID" '{"status":"resolved","reason":"e2e"}' >/dev/null; check "resolved" "$(jx "$TMP/b" 'j.status')" resolved
check "resolved_at set" "$(jx "$TMP/b" '!!j.resolved_at')" true
req PATCH "/api/v1/incidents/$ID" '{"status":"closed","reason":"e2e"}' >/dev/null; check "close still needs CAPA" "$(st)" 400
req PATCH "/api/v1/incidents/$ID" '{"capa":"recalibrated; quarterly calibration added to the plan","status":"closed","reason":"e2e"}' >/dev/null; check "closed with CAPA" "$(jx "$TMP/b" 'j.status')" closed
check "closed_by" "$(jx "$TMP/b" 'j.closed_by')" "$USER"
req PATCH "/api/v1/incidents/$ID" '{"status":"open","reason":"e2e"}' >/dev/null; check "closed → open refused" "$(st)" 400
req PATCH "/api/v1/incidents/$ID" '{"status":"investigating","reason":"e2e"}' >/dev/null; check "reopen to investigating" "$(jx "$TMP/b" 'j.status')" investigating
check "closed_at cleared on reopen" "$(jx "$TMP/b" 'j.closed_at')" null
req PATCH "/api/v1/incidents/$ID" '{"status":"closed","reason":"e2e"}' >/dev/null; check "closed again" "$(jx "$TMP/b" 'j.status')" closed
req GET "/api/v1/incidents/$ID" >/dev/null
check "history has fields, note and status events" "$(jx "$TMP/b" '["fields","note","status"].every(e=>j.events.some(x=>x.event===e))')" true
req GET "/api/v1/incidents?status=closed" >/dev/null; check "filter closed" "$(jx "$TMP/b" "j.some(i=>i.id==='$ID')")" true
req GET "/api/v1/incidents?status=open" >/dev/null; check "not among open" "$(jx "$TMP/b" "j.some(i=>i.id==='$ID')")" false
req GET "/api/v1/incidents?status=bogus" >/dev/null; check "filter validated" "$(st)" 400
req GET "/api/v1/export/audit" >/dev/null; grep -q ",incident.open," "$TMP/b" && ok "audit: incident.open" || bad "audit open"; grep -q ",incident.update," "$TMP/b" && ok "audit: incident.update" || bad "audit update"

echo "== undelivered alarm opens an incident"
CH=$(req POST /api/v1/notification-channels "{\"name\":\"dead-$SUF\",\"kind\":\"webhook\",\"settings\":{\"url\":\"http://127.0.0.1:9/hook\",\"secret\":\"s3cret-$SUF\"},\"reason\":\"e2e\"}" | jget id); check "webhook channel to a closed port" "$(st)" 201
POL=$(req POST /api/v1/escalation-policies "{\"name\":\"dead-pol-$SUF\",\"steps\":[{\"after_s\":0,\"channel_id\":\"$CH\"}],\"reason\":\"e2e\"}" | jget id); check "policy" "$(st)" 201
SITE=$(req POST /api/v1/sites "{\"name\":\"Incident site $SUF\",\"reason\":\"e2e\"}" | jget id)
ASSET=$(req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"fridge\",\"name\":\"Incident fridge $SUF\",\"reason\":\"e2e\"}" | jget id)
DS=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"t_c\",\"quantity\":\"temperature\",\"unit\":\"C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" | jget id)
DEV=$(req POST /api/v1/devices "{\"external_id\":\"inc-$SUF\",\"name\":\"incident test\",\"reason\":\"e2e\"}" | jget id)
TOK=$(req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}' | jget token); [[ -n "$TOK" ]] && ok "device and token" || bad "token"
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$DS\",\"reason\":\"e2e\"}" >/dev/null; check "assign" "$(st)" 201
req POST /api/v1/alarm-rules "{\"datastream_id\":\"$DS\",\"type\":\"high\",\"action\":30,\"escalation_policy_id\":\"$POL\",\"reason\":\"e2e\"}" >/dev/null; check "rule with the policy" "$(st)" 201
go run ./tools/devfill -addr "$MQTT" -user "inc-$SUF" -token "$TOK" -key t_c -start 35 -step 0 -n 1 -every 1 > "$TMP/pub.log" 2>&1 && ok "published 35 (above 30)" || bad "publish: $(tail -2 "$TMP/pub.log")"
for i in 1 2 3 4 5 6 7 8; do sleep 1; req GET "/api/v1/incidents?status=open" >/dev/null; [[ "$(jx "$TMP/b" 'j.some(i=>i.kind==="alarm_undelivered" && (i.detail.alarm||{}).key==="t_c" && i.title.includes("dead-'"$SUF"'"))')" == true ]] && break; done
check "incident opened for the failed step" "$(jx "$TMP/b" 'j.some(i=>i.kind==="alarm_undelivered" && i.title.includes("dead-'"$SUF"'"))')" true
INC=$(jx "$TMP/b" 'j.find(i=>i.kind==="alarm_undelivered" && i.title.includes("dead-'"$SUF"'")).id')
req GET "/api/v1/incidents/$INC" >/dev/null
check "severity major (alarm was warning)" "$(jx "$TMP/b" 'j.incident.severity')" major
check "opened by system" "$(jx "$TMP/b" 'j.incident.opened_by')" system
check "detail carries the error" "$(jx "$TMP/b" 'new RegExp("refused|connect|dial|timeout","i").test(j.incident.detail.error)')" true
check "detail carries channel kind" "$(jx "$TMP/b" 'j.incident.detail.channel_kind')" webhook
ALARM=$(jx "$TMP/b" 'j.incident.detail.alarm_id'); [[ -n "$ALARM" && "$ALARM" != "null" ]] && ok "detail links an alarm" || bad "alarm id missing"
check "detail alarm object matches" "$(jx "$TMP/b" 'j.incident.detail.alarm.id === j.incident.detail.alarm_id')" true
req GET /api/v1/alarms >/dev/null; check "that alarm is active" "$(jx "$TMP/b" "j.some(a=>a.id==='$ALARM' && !a.cleared_at)")" true
sleep 1; req GET "/api/v1/incidents/$INC" >/dev/null; check "notified event recorded" "$(jx "$TMP/b" 'j.events.some(e=>e.event==="notified")')" true
req GET "/api/v1/notifications?limit=20" >/dev/null; check "delivery log: failed" "$(jx "$TMP/b" "j.some(n=>n.status==='failed' && n.alarm_id==='$ALARM')")" true

echo "== instance-level incidents: server administrators only"
if [[ -n "${DB_ADMIN:-}" ]] && command -v psql >/dev/null 2>&1; then
  KEY="host:disk:/e2e-$SUF:low"
  HID=$(psql -Atq "$DB_ADMIN" -c "INSERT INTO core.incident (kind, severity, title, impact, detail, dedup_key, opened_by)
    VALUES ('storage', 'major', 'Server disk is running low', 'e2e', '{}', '$KEY', 'system') RETURNING id" | head -1)
  [[ -n "$HID" ]] && ok "instance-level incident opened" || bad "insert"
  req GET /api/v1/incidents >/dev/null; check "a server administrator sees it" "$(jx "$TMP/b" "j.some(i=>i.id==='$HID')")" true
  req GET "/api/v1/incidents/$HID" >/dev/null; check "and its detail" "$(st)" 200
  J2="$TMP/jar2"
  req POST /api/v1/users "{\"username\":\"incop_$SUF\",\"full_name\":\"Operator\",\"email\":\"incop$SUF@example.com\",\"roles\":[\"operator\"],\"reason\":\"e2e\"}" >/dev/null; check "operator created" "$(st)" 201
  OPW=$(jx "$TMP/b" 'j.initial_password')
  c2() { curl -s -b "$J2" -c "$J2" -H "Content-Type: application/json" -H "X-CSRF-Token: ${C2:-}" -o "$TMP/b2" -w '%{http_code}' "$@"; }
  c2 -X POST --data-binary "{\"username\":\"incop_$SUF\",\"password\":\"$OPW\"}" "$BASE/api/v1/auth/login" >/dev/null
  c2 "$BASE/api/v1/auth/me" >/dev/null; C2=$(jget csrf_token < "$TMP/b2")
  NPW="Incident-Passw0rd-$SUF"; c2 -X POST --data-binary "{\"current\":\"$OPW\",\"new\":\"$NPW\"}" "$BASE/api/v1/auth/password" >/dev/null
  c2 -X POST --data-binary "{\"username\":\"incop_$SUF\",\"password\":\"$NPW\"}" "$BASE/api/v1/auth/login" >/dev/null
  c2 "$BASE/api/v1/auth/me" >/dev/null; C2=$(jget csrf_token < "$TMP/b2")
  check "an operator lists incidents" "$(c2 "$BASE/api/v1/incidents")" 200
  check "but not the server's" "$(jx "$TMP/b2" "j.some(i=>i.id==='$HID')")" false
  check "nor its detail" "$(c2 "$BASE/api/v1/incidents/$HID")" 404
  check "nor can change it" "$(c2 -X PATCH --data-binary '{"note":"x","reason":"e2e"}' "$BASE/api/v1/incidents/$HID")" 404
  req PATCH "/api/v1/incidents/$HID" '{"cause":"e2e","capa":"e2e","status":"closed","reason":"e2e cleanup"}' >/dev/null; check "the administrator closes it" "$(st)" 200
else
  echo "  skip (DB_ADMIN or psql missing)"
fi

echo "== metrics"
curl -s -o "$TMP/b" "$BASE/metrics"; grep -q "^ctrl32_incidents_open " "$TMP/b" && ok "ctrl32_incidents_open exposed" || bad "metric missing"
N=$(grep "^ctrl32_incidents_open " "$TMP/b" | awk '{print $2}'); [[ "${N:-0}" -ge 1 ]] && ok "open incidents counted ($N)" || bad "count $N"

echo "== cleanup"
req PATCH "/api/v1/incidents/$INC" '{"cause":"e2e: webhook to a closed port","capa":"e2e: none","status":"closed","reason":"e2e cleanup"}' >/dev/null; check "close the automatic incident" "$(st)" 200
req DELETE "/api/v1/alarm-rules/$(req GET "/api/v1/alarm-rules?datastream_id=$DS" | jx /dev/stdin 'j[0].id' 2>/dev/null || echo x)?reason=e2e" >/dev/null 2>&1 || true

echo
echo "incidents: $pass passed, $fail failed"
[[ $fail -eq 0 ]]

#!/usr/bin/env bash
# End-to-end check of automation rules "condition → action" (URS C-09): a rule over two MQTT datastreams fires on
# enter with every action type — notification channel, inline webhook (HMAC), Modbus write with read-back,
# alarm, derived datastream — and clears on exit; hold time and cooldown work through the ticker; a "stale" rule
# fires when data stops; four eyes keeps a command rule pending until a second user approves it. Needs Go
# (tools/devfill, tools/modbussim) and Node (webhook sink) — a developer machine, not the demo server.
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 USER=admin PASS=admin1234 bash tests/e2e/rules.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
SINK_PORT=${SINK_PORT:-5091}
PORT=${SIM_PORT:-5022}
command -v go >/dev/null 2>&1 || export PATH="/d/tools/go/bin:$PATH"
TMP=$(mktemp -d); trap 'kill $SINK 2>/dev/null; kill $SIM 2>/dev/null; rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; JAR2="$TMP/jar2"; CSRF=""; CSRF2=""
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
req2() { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR2" -c "$JAR2" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF2" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR2" -c "$JAR2" -X "$m" -H "X-CSRF-Token: $CSRF2" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  cat "$TMP/b"; }
sink_count() { local c; c=$(grep -c "\"ev\":\"$1\"" "$TMP/sink.log" 2>/dev/null); echo "${c:-0}"; }
pub() { go run ./tools/devfill -addr "$MQTT" -user "ar-$SUF" -token "$TOK" -key "$1" -start "$2" -step 0 -n 1 -every 1 > "$TMP/pub.log" 2>&1 || bad "publish $1=$2: $(tail -2 "$TMP/pub.log")"; }
runs() { req GET "/api/v1/rule-runs?rule_id=$1&limit=50" >/dev/null; jx "$TMP/b" "$2"; }
SUF=$RANDOM
SECRET="s3cret-$SUF"

echo "== webhook sink and Modbus simulator"
node -e 'const http=require("http"),fs=require("fs");http.createServer((q,s)=>{let b="";q.on("data",c=>b+=c);q.on("end",()=>{fs.appendFileSync(process.argv[1],JSON.stringify({sig:q.headers["x-ctrl32-signature"]||"",ev:q.headers["x-ctrl32-event"]||"",body:b})+"\n");s.end("ok")})}).listen(process.argv[2])' "$TMP/sink.log" "$SINK_PORT" &
SINK=$!
EXE=bin/modbussim; [[ "$(uname -s)" == MINGW* || "$(uname -s)" == MSYS* ]] && EXE=bin/modbussim.exe
go build -o "$EXE" ./tools/modbussim && ok "simulator built" || { bad "simulator build"; exit 1; }
"$EXE" -port "$PORT" > "$TMP/sim.log" 2>&1 &
SIM=$!
sleep 1.5; : > "$TMP/sink.log"
grep -q "listening" "$TMP/sim.log" && ok "simulator on $PORT, sink on $SINK_PORT" || { bad "simulator: $(cat "$TMP/sim.log")"; exit 1; }

echo "== login and fixtures"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"
CH=$(req POST /api/v1/notification-channels "{\"name\":\"rhook-$SUF\",\"kind\":\"webhook\",\"settings\":{\"url\":\"http://127.0.0.1:$SINK_PORT/hook\",\"secret\":\"$SECRET\"},\"reason\":\"e2e\"}" | jget id); check "webhook channel" "$(st)" 201
SITE=$(req POST /api/v1/sites "{\"name\":\"Rules site $SUF\",\"reason\":\"e2e\"}" | jget id)
ASSET=$(req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"room\",\"name\":\"Rules room $SUF\",\"reason\":\"e2e\"}" | jget id)
DS1=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"t_in\",\"quantity\":\"temperature\",\"unit\":\"C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" | jget id)
DS2=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"t_out\",\"quantity\":\"temperature\",\"unit\":\"C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" | jget id)
DS3=$(req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"t_diff\",\"quantity\":\"temperature\",\"unit\":\"K\",\"kind\":\"gauge\",\"reason\":\"e2e\"}" | jget id)
[[ -n "$DS1" && -n "$DS2" && -n "$DS3" ]] && ok "site, asset, datastreams t_in t_out t_diff" || bad "fixtures"
DEV=$(req POST /api/v1/devices "{\"external_id\":\"ar-$SUF\",\"name\":\"rules test\",\"reason\":\"e2e\"}" | jget id)
TOK=$(req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}' | jget token)
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$DS1\",\"reason\":\"e2e\"}" >/dev/null; check "assign t_in" "$(st)" 201
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$DS2\",\"reason\":\"e2e\"}" >/dev/null; check "assign t_out" "$(st)" 201
REGS='[{"key":"temp","table":"holding","address":0,"type":"int16","scale":0.1},{"key":"setpoint","table":"holding","address":1,"type":"int16","scale":0.1,"writable":true,"min":10,"max":40}]'
NAME="rsim-$SUF"
req POST /api/v1/connectors "{\"kind\":\"modbus_tcp\",\"name\":\"$NAME\",\"settings\":{\"host\":\"127.0.0.1\",\"port\":$PORT,\"unit_id\":1,\"interval_s\":2,\"registers\":$REGS},\"reason\":\"e2e\"}" >/dev/null; check "modbus connector" "$(st)" 201
CONN=$(jget id < "$TMP/b")
sleep 3
req GET /api/v1/devices >/dev/null; MB=$(jx "$TMP/b" "(j.find(d=>d.external_id==='modbus-$NAME')||{}).id"); [[ -n "$MB" && "$MB" != "null" ]] && ok "virtual Modbus device" || bad "virtual device: $(head -c 200 "$TMP/b")"

echo "== validation"
IN="[{\"name\":\"t_in\",\"datastream_id\":\"$DS1\"},{\"name\":\"t_out\",\"datastream_id\":\"$DS2\"}]"
req POST /api/v1/rules "{\"name\":\"bad-$SUF\",\"spec\":{\"inputs\":$IN,\"conditions\":[{\"input\":\"nope\",\"op\":\">\",\"value\":1}],\"actions\":[{\"type\":\"alarm\"}]},\"reason\":\"e2e\"}" >/dev/null; check "unknown input refused" "$(st)" 400
req POST /api/v1/rules "{\"name\":\"bad-$SUF\",\"spec\":{\"inputs\":$IN,\"conditions\":[{\"input\":\"t_in\",\"op\":\">\",\"value\":\"t_out +\"}],\"actions\":[{\"type\":\"alarm\"}]},\"reason\":\"e2e\"}" >/dev/null; check "bad expression refused" "$(st)" 400
req POST /api/v1/rules "{\"name\":\"bad-$SUF\",\"spec\":{\"inputs\":$IN,\"actions\":[{\"type\":\"derive\",\"datastream_id\":\"$DS1\",\"expr\":\"t_out\"}]},\"reason\":\"e2e\"}" >/dev/null; check "derive into an input refused" "$(st)" 400
req POST /api/v1/rules "{\"name\":\"bad-$SUF\",\"spec\":{\"inputs\":$IN,\"actions\":[{\"type\":\"notify\",\"channel_id\":\"00000000-0000-0000-0000-000000000000\"}]},\"reason\":\"e2e\"}" >/dev/null; check "unknown channel refused" "$(st)" 400
req POST /api/v1/rules "{\"name\":\"bad-$SUF\",\"spec\":{\"inputs\":$IN,\"conditions\":[{\"input\":\"t_in\",\"op\":\">\",\"value\":1}]},\"reason\":\"e2e\"}" >/dev/null; check "no actions refused" "$(st)" 400
req POST /api/v1/rules "{\"name\":\"bad-$SUF\",\"spec\":{\"inputs\":$IN,\"actions\":[{\"type\":\"command\",\"device_id\":\"$MB\",\"key\":\"setpoint\",\"value\":\"t_in - 10\"}]}}" >/dev/null; check "reason required" "$(st)" 400

echo "== rule with every action type"
SPEC="{\"inputs\":$IN,\"match\":\"all\",\"conditions\":[{\"input\":\"t_in\",\"op\":\">=\",\"value\":30},{\"input\":\"t_out\",\"op\":\"<\",\"value\":10}],\"actions\":[{\"type\":\"notify\",\"channel_id\":\"$CH\",\"subject\":\"hot {rule}: t_in={t_in}\"},{\"type\":\"webhook\",\"url\":\"http://127.0.0.1:$SINK_PORT/inline\",\"secret\":\"$SECRET\"},{\"type\":\"command\",\"device_id\":\"$MB\",\"key\":\"setpoint\",\"value\":\"t_in - 10\"},{\"type\":\"alarm\",\"severity\":\"action\"},{\"type\":\"derive\",\"datastream_id\":\"$DS3\",\"expr\":\"t_in - t_out\"}],\"clear_actions\":[{\"type\":\"notify\",\"channel_id\":\"$CH\"}]}"
req PUT /api/v1/flows/settings '{"allowed_hosts":[],"reason":"e2e reset"}' >/dev/null
req POST /api/v1/rules "{\"name\":\"hostless-$SUF\",\"spec\":$SPEC,\"reason\":\"e2e\"}" >/dev/null; check "webhook to a host off the allow-list refused" "$(st)" 400
req PUT /api/v1/flows/settings '{"allowed_hosts":["127.0.0.1"],"reason":"e2e: the webhook sink"}' >/dev/null; check "sink host allowed" "$(st)" 200
req POST /api/v1/rules/test "{\"spec\":$SPEC}" >/dev/null; check "dry run" "$(st)" 200
[[ "$(jx "$TMP/b" "j.condition")" == "false" && "$(jx "$TMP/b" "j.missing.length")" == "2" ]] && ok "dry run: no data yet, both inputs missing" || bad "dry run: $(head -c 200 "$TMP/b")"
R1=$(req POST /api/v1/rules "{\"name\":\"rule-a-$SUF\",\"description\":\"all actions\",\"spec\":$SPEC,\"reason\":\"e2e\"}" | jget id); check "create" "$(st)" 201
req GET /api/v1/rules >/dev/null; [[ "$(jx "$TMP/b" "(j.find(r=>r.id==='$R1')||{}).status")" == "active" && "$(jx "$TMP/b" "(j.find(r=>r.id==='$R1')||{}).state.loaded")" == "true" ]] && ok "listed active and loaded" || bad "list: $(head -c 300 "$TMP/b")"
pub t_in 35; sleep 2
[[ "$(runs "$R1" "j.length")" == "0" ]] && ok "t_in alone does not fire (t_out missing)" || bad "fired early"
req POST /api/v1/rules/test "{\"spec\":$SPEC,\"rule_id\":\"$R1\"}" >/dev/null; [[ "$(jx "$TMP/b" "j.inputs.t_in")" == "35" && "$(jx "$TMP/b" "j.missing[0]")" == "t_out" ]] && ok "dry run sees t_in=35, t_out missing" || bad "dry run 2: $(head -c 200 "$TMP/b")"
: > "$TMP/sink.log"
pub t_out 5; sleep 4
[[ "$(runs "$R1" "j.length")" == "1" && "$(runs "$R1" "j[0].event")" == "enter" ]] && ok "enter run recorded" || bad "runs: $(head -c 300 "$TMP/b")"
runs "$R1" "j[0].actions.map(a=>a.type+':'+a.status).sort().join(',')" > "$TMP/acts"; check "all five actions done" "$(cat "$TMP/acts")" "alarm:done,command:done,derive:done,notify:done,webhook:done"
[[ "$(runs "$R1" "j[0].inputs.t_in")" == "35" && "$(runs "$R1" "j[0].ok")" == "true" ]] && ok "inputs recorded, run ok" || bad "run detail: $(head -c 300 "$TMP/b")"
[[ "$(sink_count rule)" == "2" ]] && ok "sink got the channel and the inline webhook" || bad "sink: $(cat "$TMP/sink.log" | head -c 300)"
SIGOK=$(node -e 'const c=require("crypto");const ls=require("fs").readFileSync(process.argv[1],"utf8").trim().split("\n");console.log(ls.every(l=>{const r=JSON.parse(l);return r.sig==="sha256="+c.createHmac("sha256",process.argv[2]).update(r.body).digest("hex")}))' "$TMP/sink.log" "$SECRET"); [[ "$SIGOK" == "true" ]] && ok "both webhooks signed" || bad "signature"
grep -q "hot rule-a-$SUF: t_in=35" "$TMP/sink.log" && ok "subject template rendered" || bad "subject: $(head -c 300 "$TMP/sink.log")"
grep -q 'transition\\":\\"enter' "$TMP/sink.log" && ok "payload carries the rule transition" || bad "payload"
req GET "/api/v1/devices/$MB/commands" >/dev/null; [[ "$(jx "$TMP/b" "(j[0]||{}).issued_by")" == "rule:rule-a-$SUF" && "$(jx "$TMP/b" "(j[0]||{}).result.readback")" == "25" ]] && ok "Modbus setpoint written by the rule: 35 - 10 = 25 read back" || bad "command: $(head -c 300 "$TMP/b")"
req GET /api/v1/alarms >/dev/null; ALARM=$(jx "$TMP/b" "(j.find(a=>a.key==='t_in' && !a.cleared_at && a.severity==='action')||{}).id"); [[ -n "$ALARM" && "$ALARM" != "null" ]] && ok "action alarm raised by the rule" || bad "alarm: $(head -c 300 "$TMP/b")"
req GET "/api/v1/observations?datastream_id=$DS3&limit=5" >/dev/null; [[ "$(jx "$TMP/b" "(j[j.length-1]||{}).value")" == "30" ]] && ok "derived t_diff = 35 - 5 = 30" || bad "derived: $(head -c 300 "$TMP/b")"
req GET "/api/v1/notifications?limit=10" >/dev/null; [[ "$(jx "$TMP/b" "j.filter(n=>n.event==='rule' && n.status==='sent' && (n.subject||'').includes('$SUF')).length")" == "2" ]] && ok "delivery log: two rule notifications sent" || bad "log: $(head -c 300 "$TMP/b")"

echo "== stays active: derive updates, no new run"
pub t_in 36; sleep 2
[[ "$(runs "$R1" "j.length")" == "1" ]] && ok "no second enter while active" || bad "runs: $(runs "$R1" "j.length")"
req GET "/api/v1/observations?datastream_id=$DS3&limit=5" >/dev/null; [[ "$(jx "$TMP/b" "(j[j.length-1]||{}).value")" == "31" ]] && ok "derived t_diff follows: 36 - 5 = 31" || bad "derived 2: $(head -c 300 "$TMP/b")"
req GET /api/v1/rules >/dev/null; [[ "$(jx "$TMP/b" "(j.find(r=>r.id==='$R1')||{}).state.active")" == "true" ]] && ok "state: condition active" || bad "state"

echo "== exit"
: > "$TMP/sink.log"
pub t_in 20; sleep 3
[[ "$(runs "$R1" "j.length")" == "2" && "$(runs "$R1" "j[0].event")" == "exit" ]] && ok "exit run recorded" || bad "exit: $(head -c 300 "$TMP/b")"
[[ "$(sink_count rule)" == "1" ]] && ok "clear action notified" || bad "sink after exit: $(cat "$TMP/sink.log" | head -c 300)"
req GET /api/v1/alarms >/dev/null; [[ "$(jx "$TMP/b" "(j.find(a=>a.id==='$ALARM')||{}).state")" == "cleared" ]] && ok "alarm cleared on exit" || bad "alarm after exit: $(head -c 300 "$TMP/b")"
runs "$R1" "j[0].actions.map(a=>a.type+':'+a.status).sort().join(',')" > "$TMP/acts"; check "exit actions: alarm cleared, notify" "$(cat "$TMP/acts")" "alarm:done,notify:done"

echo "== hold time and cooldown"
R2=$(req POST /api/v1/rules "{\"name\":\"rule-hold-$SUF\",\"spec\":{\"inputs\":[{\"name\":\"t_in\",\"datastream_id\":\"$DS1\"}],\"conditions\":[{\"input\":\"t_in\",\"op\":\">=\",\"value\":30}],\"for_s\":3,\"cooldown_s\":60,\"actions\":[{\"type\":\"webhook\",\"url\":\"http://127.0.0.1:$SINK_PORT/hold\"}]},\"reason\":\"e2e\"}" | jget id); check "hold rule" "$(st)" 201
pub t_in 35; sleep 1
[[ "$(runs "$R2" "j.length")" == "0" ]] && ok "not fired before the hold time" || bad "fired early"
req GET /api/v1/rules >/dev/null; [[ "$(jx "$TMP/b" "(j.find(r=>r.id==='$R2')||{}).state.pending")" == "true" ]] && ok "state: pending (holding)" || bad "pending state: $(jx "$TMP/b" "(j.find(r=>r.id==='$R2')||{}).state")"
sleep 8
[[ "$(runs "$R2" "j.length")" == "1" && "$(runs "$R2" "j[0].event")" == "enter" ]] && ok "fired by the ticker after the hold time" || bad "hold: $(runs "$R2" "j.length")"
pub t_in 20; sleep 2
[[ "$(runs "$R2" "j.length")" == "2" ]] && ok "exit" || bad "exit 2"
pub t_in 35; sleep 9
[[ "$(runs "$R2" "j.length")" == "2" ]] && ok "no re-fire inside the cooldown" || bad "cooldown ignored: $(runs "$R2" "j.length")"
pub t_in 20; sleep 1

echo "== stale"
R3=$(req POST /api/v1/rules "{\"name\":\"rule-stale-$SUF\",\"spec\":{\"inputs\":[{\"name\":\"t_out\",\"datastream_id\":\"$DS2\"}],\"conditions\":[{\"input\":\"t_out\",\"op\":\"stale\",\"value\":5}],\"actions\":[{\"type\":\"webhook\",\"url\":\"http://127.0.0.1:$SINK_PORT/stale\"}]},\"reason\":\"e2e\"}" | jget id); check "stale rule" "$(st)" 201
sleep 7
[[ "$(runs "$R3" "j.length")" == "1" && "$(runs "$R3" "j[0].event")" == "enter" ]] && ok "no data for 3 s → enter" || bad "stale enter: $(runs "$R3" "j.length")"
pub t_out 5; sleep 2
[[ "$(runs "$R3" "j.length")" == "2" && "$(runs "$R3" "j[0].event")" == "exit" ]] && ok "fresh data → exit" || bad "stale exit: $(runs "$R3" "j.length")"

echo "== versions"
req PUT "/api/v1/rules/$R1" '{"description":"renamed only","reason":"e2e"}' >/dev/null; check "update without spec" "$(st)" 200
[[ "$(jx "$TMP/b" "j.version")" == "1" ]] && ok "version unchanged" || bad "version: $(cat "$TMP/b")"
req PUT "/api/v1/rules/$R1" "{\"spec\":{\"inputs\":$IN,\"conditions\":[{\"input\":\"t_in\",\"op\":\">=\",\"value\":31}],\"actions\":[{\"type\":\"alarm\"}]},\"reason\":\"e2e\"}" >/dev/null; check "update spec" "$(st)" 200
[[ "$(jx "$TMP/b" "j.version")" == "2" ]] && ok "version 2" || bad "version: $(cat "$TMP/b")"
req PUT "/api/v1/rules/$R1" '{"enabled":false,"reason":"e2e"}' >/dev/null; check "disable" "$(st)" 200
req GET /api/v1/rules >/dev/null; [[ "$(jx "$TMP/b" "(j.find(r=>r.id==='$R1')||{}).state.loaded")" == "false" ]] && ok "disabled rule is not evaluated" || bad "still loaded"

echo "== four eyes for command rules"
U=$(req POST /api/v1/users "{\"username\":\"fr_$SUF\",\"full_name\":\"Rule Approver\",\"email\":\"fr$SUF@example.com\",\"roles\":[\"engineer\"],\"reason\":\"e2e\"}"); check "second user (engineer)" "$(st)" 201
UID2=$(echo "$U" | grep -o '"user":{[^}]*' | jget id); PW=$(echo "$U" | jget initial_password)
req2 POST /api/v1/auth/login "{\"username\":\"fr_$SUF\",\"password\":\"$PW\"}" >/dev/null; CSRF2=$(req2 GET /api/v1/auth/me | jget csrf_token)
NEWPW="Rule-Approver-Passw0rd-$SUF"
req2 POST /api/v1/auth/password "{\"current\":\"$PW\",\"new\":\"$NEWPW\"}" >/dev/null; req2 POST /api/v1/auth/login "{\"username\":\"fr_$SUF\",\"password\":\"$NEWPW\"}" >/dev/null; CSRF2=$(req2 GET /api/v1/auth/me | jget csrf_token); [[ -n "$CSRF2" ]] && ok "second user signed in" || bad "second user"
req PUT /api/v1/settings/policy '{"four_eyes_commands":true,"reason":"e2e on"}' >/dev/null; check "policy on" "$(st)" 200
CMDSPEC="{\"inputs\":[{\"name\":\"t_in\",\"datastream_id\":\"$DS1\"}],\"conditions\":[{\"input\":\"t_in\",\"op\":\">=\",\"value\":30}],\"actions\":[{\"type\":\"command\",\"device_id\":\"$MB\",\"key\":\"setpoint\",\"value\":15}]}"
R4=$(req POST /api/v1/rules "{\"name\":\"rule-cmd-$SUF\",\"spec\":$CMDSPEC,\"reason\":\"e2e\"}" | jget id); check "command rule is pending (202)" "$(st)" 202
pub t_in 35; sleep 2
[[ "$(runs "$R4" "j.length")" == "0" ]] && ok "pending rule does not run" || bad "pending rule ran"
req POST "/api/v1/rules/$R4/approve" '{"reason":"self"}' >/dev/null; check "editor cannot approve" "$(st)" 403
req2 POST "/api/v1/rules/$R4/approve" '{"reason":"checked"}' >/dev/null; check "second user approves" "$(st)" 200
req2 POST "/api/v1/rules/$R4/approve" '{"reason":"again"}' >/dev/null; check "cannot approve twice" "$(st)" 409
sleep 6
[[ "$(runs "$R4" "j.length")" == "1" ]] && ok "approved rule fired on the next tick/observation" || { pub t_in 36; sleep 2; [[ "$(runs "$R4" "j.length")" == "1" ]] && ok "approved rule fired on the next observation" || bad "approved rule did not run"; }
req GET "/api/v1/devices/$MB/commands" >/dev/null; [[ "$(jx "$TMP/b" "(j[0]||{}).result.readback")" == "15" ]] && ok "setpoint 15 written after approval" || bad "command after approval: $(head -c 200 "$TMP/b")"
req GET /api/v1/rules >/dev/null; [[ "$(jx "$TMP/b" "(j.find(r=>r.id==='$R4')||{}).approved_by")" == "fr_$SUF" ]] && ok "approver recorded" || bad "approver"
req PUT "/api/v1/rules/$R4" "{\"spec\":{\"inputs\":[{\"name\":\"t_in\",\"datastream_id\":\"$DS1\"}],\"conditions\":[{\"input\":\"t_in\",\"op\":\">=\",\"value\":30}],\"actions\":[{\"type\":\"command\",\"device_id\":\"$MB\",\"key\":\"setpoint\",\"value\":16}]},\"reason\":\"e2e change\"}" >/dev/null; check "edited command rule" "$(st)" 200
[[ "$(jx "$TMP/b" "j.status")" == "pending" ]] && ok "edit puts it back to pending" || bad "status after edit: $(cat "$TMP/b")"
req2 POST "/api/v1/rules/$R4/reject" '{"reason":"too low"}' >/dev/null; check "second user rejects" "$(st)" 200
req PUT "/api/v1/rules/$R4" '{"enabled":true,"reason":"e2e"}' >/dev/null; check "rejected rule cannot be enabled as is" "$(st)" 409
req PUT /api/v1/settings/policy '{"four_eyes_commands":false,"reason":"e2e off"}' >/dev/null; check "policy off" "$(st)" 200
pub t_in 20; sleep 1

echo "== cleanup and audit"
for r in $R1 $R2 $R3 $R4; do req DELETE "/api/v1/rules/$r?reason=e2e" >/dev/null; [[ "$(st)" == 200 ]] || bad "revoke $r: $(st)"; done; ok "rules revoked"
req GET "/api/v1/rule-runs?limit=100" >/dev/null; [[ "$(jx "$TMP/b" "j.filter(r=>r.rule.endsWith('$SUF')).length")" -ge 7 ]] && ok "org-wide run log" || bad "run log: $(head -c 200 "$TMP/b")"
req DELETE "/api/v1/connectors/$CONN?reason=e2e" >/dev/null; check "connector revoked" "$(st)" 200
req DELETE "/api/v1/notification-channels/$CH?reason=e2e" >/dev/null; check "channel revoked" "$(st)" 200
req POST "/api/v1/devices/$DEV/revoke" '{"reason":"e2e cleanup"}' >/dev/null; check "device revoked" "$(st)" 200
req PATCH "/api/v1/users/$UID2" '{"disabled":true,"reason":"e2e cleanup"}' >/dev/null; check "second user disabled" "$(st)" 200
req GET "/api/v1/export/audit" >/dev/null
for a in rule.create rule.update rule.approve rule.reject rule.revoke; do grep -q "$a" "$TMP/b" && ok "audit $a" || bad "audit $a missing"; done
grep -q "rule:rule-a-$SUF" "$TMP/b" && ok "audit: command issued by the rule" || bad "audit rule command"

echo
echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

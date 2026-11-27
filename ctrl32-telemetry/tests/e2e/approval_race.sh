#!/usr/bin/env bash
# End-to-end check that a decision about something held for four eyes is taken once (0.96.1): an approval and a
# rejection sent at the same moment can never both win, a command that was rejected is never sent, and two approvals
# run a held change once. Before 0.96.1 the approval did not check that its update changed the pending row.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 ROUNDS=25 bash tests/e2e/approval_race.sh
set -u
cd "$(dirname "$0")/../.."
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
ROUNDS=${ROUNDS:-25}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""; JAR2="$TMP/jar2"; CSRF2=""
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
# two decisions of the second user fired together; each writes its HTTP status to its own file
both() { local a=$1 b=$2
  curl -s -b "$JAR2" -X POST -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF2" -o "$TMP/ra" -w '%{http_code}' -d '{"reason":"race a"}' "$BASE$a" > "$TMP/sa" &
  curl -s -b "$JAR2" -X POST -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF2" -o "$TMP/rb" -w '%{http_code}' -d '{"reason":"race b"}' "$BASE$b" > "$TMP/sb" &
  wait; }
SUF=$RANDOM

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
SECOND="race_$SUF"; NEWPW="Second-Person-Passw0rd-$SUF"
req POST /api/v1/users "{\"username\":\"$SECOND\",\"full_name\":\"Second\",\"email\":\"race$SUF@example.com\",\"roles\":[\"engineer\"],\"reason\":\"e2e\"}"; PW=$(jx j.initial_password)
req2 POST /api/v1/auth/login "{\"username\":\"$SECOND\",\"password\":\"$PW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
req2 POST /api/v1/auth/password "{\"current\":\"$PW\",\"new\":\"$NEWPW\"}"
req2 POST /api/v1/auth/login "{\"username\":\"$SECOND\",\"password\":\"$NEWPW\"}"; check "a second user" "$(st)" 200
req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
req POST /api/v1/devices "{\"external_id\":\"race-$SUF\",\"name\":\"Race $SUF\",\"reason\":\"e2e\"}"; DEV=$(jx j.id); check "device" "$(st)" 201

echo "== a command: approve and reject at the same moment, $ROUNDS times"
two=0; none=0; sentRejected=0; approved=0; rejected=0
for i in $(seq 1 "$ROUNDS"); do
  req POST "/api/v1/devices/$DEV/commands" "{\"method\":\"write\",\"args\":{\"key\":\"run\",\"value\":$i},\"reason\":\"race $i\",\"second_person\":true}"; C=$(jx j.id)
  both "/api/v1/devices/$DEV/commands/$C/approve" "/api/v1/devices/$DEV/commands/$C/reject"
  a=$(cat "$TMP/sa"); b=$(cat "$TMP/sb")
  [[ "$a" == 200 && "$b" == 200 ]] && two=$((two+1))
  [[ "$a" != 200 && "$b" != 200 ]] && none=$((none+1))
  req GET "/api/v1/devices/$DEV/commands?limit=5"; s=$(jx "(c => c ? c.status : 'missing')(j.find(c => c.id === '$C'))")
  if [[ "$b" == 200 ]]; then rejected=$((rejected+1)); [[ "$s" != rejected ]] && sentRejected=$((sentRejected+1)); fi
  [[ "$a" == 200 ]] && approved=$((approved+1))
done
echo "   approved $approved, rejected $rejected"
check "never both an approval and a rejection" "$two" 0
check "one of them always wins" "$none" 0
check "a command whose rejection was accepted is never sent" "$sentRejected" 0

echo "== a command: two approvals at the same moment"
dbl=0
for i in $(seq 1 10); do
  req POST "/api/v1/devices/$DEV/commands" "{\"method\":\"write\",\"args\":{\"key\":\"run\",\"value\":$i},\"reason\":\"twice $i\",\"second_person\":true}"; C=$(jx j.id)
  both "/api/v1/devices/$DEV/commands/$C/approve" "/api/v1/devices/$DEV/commands/$C/approve"
  [[ "$(cat "$TMP/sa")" == 200 && "$(cat "$TMP/sb")" == 200 ]] && dbl=$((dbl+1))
done
check "a command is approved, and so sent, once" "$dbl" 0

echo "== a held change: decided once"
req PUT /api/v1/settings/policy '{"four_eyes_attributes":true,"reason":"e2e race"}'; check "four eyes for device attributes on" "$(st)" 200
two=0; ran=0; dbl=0
for i in $(seq 1 10); do
  req PUT "/api/v1/devices/$DEV/attributes" "{\"attributes\":{\"race\":$i},\"reason\":\"race $i\"}"; R=$(jx j.id)
  both "/api/v1/change-requests/$R/approve" "/api/v1/change-requests/$R/reject"
  a=$(cat "$TMP/sa"); b=$(cat "$TMP/sb"); [[ "$a" == 200 && "$b" == 200 ]] && two=$((two+1))
  req GET "/api/v1/devices/$DEV/attributes"; v=$(jx 'j.shared.race')
  [[ "$b" == 200 && "$v" == "$i" ]] && ran=$((ran+1))
  req PUT "/api/v1/devices/$DEV/attributes" "{\"attributes\":{\"twice\":$i},\"reason\":\"twice $i\"}"; R=$(jx j.id)
  both "/api/v1/change-requests/$R/approve" "/api/v1/change-requests/$R/approve"
  [[ "$(cat "$TMP/sa")" == 200 && "$(cat "$TMP/sb")" == 200 ]] && dbl=$((dbl+1))
done
check "never both an approval and a rejection" "$two" 0
check "a change whose rejection was accepted is not made" "$ran" 0
check "two approvals run the change once" "$dbl" 0
req PUT /api/v1/settings/policy '{"four_eyes_attributes":false,"reason":"e2e race end"}'; check "switched off" "$(st)" 200

echo
echo "passed: $pass, failed: $fail"
[[ $fail -eq 0 ]]

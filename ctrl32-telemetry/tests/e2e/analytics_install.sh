#!/usr/bin/env bash
# End-to-end checks of the detector installation from the user interface (0.75.0): GET/POST/DELETE
# /api/v1/counting/install against the server agent's simulation ([agent] vpn_simulate: stand-in downloads with their
# own pins, served from memory; nothing is downloaded and nothing on the host changes). Covers the permission
# (system.admin), the reason, the password step-up, API tokens refused, one job at a time, the progress, the paths the
# server applies without a restart (the self-check then points at the new parts), the idempotent re-run, the repair
# (force), the removal and the audit trail. (The exclusion from AI assistants: TestMCPDetectorInstallExcluded.)
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 [CONFIG_VPN=config-vpn.toml] bash tests/e2e/analytics_install.sh
# (run-all.sh starts the server with the agent's simulation for this suite)
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
CONFIG_VPN=${CONFIG_VPN:-}
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""
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
# wait for the job: polls GET /api/v1/counting/install until the job is no longer running; prints the steps seen
wait_job() {
  local steps="" s
  for _ in $(seq 150); do
    req GET /api/v1/counting/install
    s=$(jx 'j.job && j.job.state === "running" ? j.job.progress.step : "-"')
    [[ "$s" == "-" ]] && { echo "$steps"; return; }
    [[ " $steps " == *" $s "* ]] || steps="$steps $s"
    sleep 0.2
  done
  echo "timeout"
}
SUF=$RANDOM

echo "== access"
req GET /api/v1/counting/install; check "needs sign-in" "$(st)" 401
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req GET /api/v1/counting/install
check "state" "$(st) $(jx 'j.agent.connected + " " + j.supported + " " + j.simulated')" "200 true true true"
check "pinned versions" "$(jx 'j.pins.onnxruntime + " " + j.pins.yolox')" "1.30.0 0.1.1rc0"
req POST /api/v1/users "{\"username\":\"aninst_$SUF\",\"full_name\":\"Engineer\",\"email\":\"aninst$SUF@example.com\",\"roles\":[\"engineer\"],\"reason\":\"e2e\"}"; PW=$(jx j.initial_password)
J2="$TMP/jar2"; CSRF2=""; req2() { call "$J2" "$CSRF2" "$@"; }
req2 POST /api/v1/auth/login "{\"username\":\"aninst_$SUF\",\"password\":\"$PW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
NEWPW="Detector-Passw0rd-$SUF"; req2 POST /api/v1/auth/password "{\"current\":\"$PW\",\"new\":\"$NEWPW\"}"
req2 POST /api/v1/auth/login "{\"username\":\"aninst_$SUF\",\"password\":\"$NEWPW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
req2 GET /api/v1/counting/install; check "an engineer (no system.admin) cannot see it" "$(st)" 403
req2 POST /api/v1/counting/install "{\"reason\":\"x\",\"password\":\"$NEWPW\"}"; check "an engineer cannot install" "$(st)" 403
req2 GET /api/v1/counting/selfcheck; check "the engineer still reads the self-check" "$(st)" 200

echo "== confirmation"
req POST /api/v1/counting/install '{"password":"x"}'; check "reason required" "$(st)" 400
req POST /api/v1/counting/install '{"reason":"e2e detector"}'; check "password required" "$(st) $(jx j.code)" "403 step_up_required"
req POST /api/v1/counting/install '{"reason":"e2e detector","password":"wrong-password"}'; check "wrong password" "$(st) $(jx j.code)" "403 step_up_failed"
req POST /api/v1/tokens '{"name":"detector e2e","scopes":["system.admin","data.read"]}'; TK=$(jx j.token)
if [[ -n "$TK" && "$TK" != null ]]; then
  curl -s -o "$TMP/b" -w '%{http_code}' -X POST -H "Authorization: Bearer $TK" -H "Content-Type: application/json" -d "{\"reason\":\"x\",\"password\":\"$PASS\"}" "$BASE/api/v1/counting/install" > "$TMP/s"
  check "an API token is refused" "$(st) $(jx j.code)" "403 session_required"
  curl -s -o "$TMP/b" -w '%{http_code}' -H "Authorization: Bearer $TK" "$BASE/api/v1/counting/install" > "$TMP/s"; check "an API token may read the state" "$(st)" 200
else bad "API token for the check ($(head -c 200 "$TMP/b"))"; fi
req GET /api/v1/openapi.json
check "documented in the API" "$(jx '!!(j.paths["/api/v1/counting/install"] && j.paths["/api/v1/counting/install"].post && j.paths["/api/v1/counting/install"].delete)')" true

echo "== install"
req GET "/api/v1/counting/selfcheck?refresh=1"; check "before: not ready" "$(jx j.ready)" false
req POST /api/v1/counting/install "{\"reason\":\"e2e detector\",\"password\":\"$PASS\"}"
check "started" "$(st) $(jx 'j.job.action + " " + j.job.state')" "202 install running"
req POST /api/v1/counting/install "{\"reason\":\"e2e detector\",\"password\":\"$PASS\"}"; check "one job at a time" "$(st) $(jx j.code)" "409 busy"
STEPS=$(wait_job)
[[ "$STEPS" == *download* ]] && ok "progress seen:$STEPS" || bad "progress (steps:$STEPS)"
req GET /api/v1/counting/install
check "done: five parts downloaded" "$(jx 'j.job.state + " " + (j.job.installed||[]).length + " " + (j.job.kept||[]).length')" "done 5 0"
check "the last installation is recorded" "$(jx '!!(j.installed && j.installed.paths.ffmpeg)')" true
FF=$(jx 'j.job.config.ffmpeg')
[[ "$FF" == *analytics*lgpl*ffmpeg* ]] && ok "[analytics] ffmpeg path: $FF" || bad "ffmpeg path $FF"
check "model pin written" "$(jx 'j.job.config.nano_sha256')" "c789161ed43c8269fcd4e67c67eeeb4e80c622da2eb296a20bc6007bd18a0b7d"
# the server applied the paths without a restart: the self-check now examines the installed (stand-in) parts
req GET "/api/v1/counting/selfcheck?refresh=1"
check "self-check uses the new ONNX Runtime" "$(jx 'j.components.find(c=>c.name==="onnxruntime").path.includes("onnxruntime-1.30.0")')" true
check "self-check uses the new models" "$(jx 'j.components.find(c=>c.name==="model_nano").path.endsWith("yolox_nano.onnx")')" true
if [[ -n "$CONFIG_VPN" && -f "$CONFIG_VPN" ]]; then
  grep -q '^onnxruntime = ".*onnxruntime-1.30.0' "$CONFIG_VPN" && ok "config.toml: [analytics] written" || bad "config.toml: [analytics] not written"
  grep -q '^profile' "$CONFIG_VPN" && ok "config.toml: other keys kept" || bad "config.toml: other keys lost"
fi

echo "== again, repair, audit"
req POST /api/v1/counting/install "{\"reason\":\"e2e detector again\",\"password\":\"$PASS\"}"; check "re-run started" "$(st)" 202
wait_job >/dev/null; req GET /api/v1/counting/install
check "re-run: nothing downloaded, all kept" "$(jx 'j.job.state + " " + (j.job.installed||[]).length + " " + (j.job.kept||[]).length')" "done 0 5"
req POST /api/v1/counting/install "{\"reason\":\"e2e repair\",\"password\":\"$PASS\",\"force\":true}"; check "repair started" "$(st) $(jx j.job.force)" "202 true"
wait_job >/dev/null; req GET /api/v1/counting/install
check "repair: every part again" "$(jx 'j.job.state + " " + j.job.installed.length')" "done 5"
sleep 2.5 # the watcher records the result
req GET "/api/v1/audit?action=analytics.install&limit=20"
check "audited: requests and results" "$(jx 'j.entries.filter(e=>e.action==="analytics.install").length >= 6')" true
check "audit keeps the reason" "$(jx 'j.entries.some(e=>e.reason==="e2e repair")')" true
check "audit keeps the result" "$(jx 'j.entries.some(e=>(e.reason||"").endsWith("(result)") && JSON.stringify(e).includes("done"))')" true

echo "== remove"
req DELETE /api/v1/counting/install '{"reason":"e2e remove"}'; check "removal needs the password" "$(st)" 403
req DELETE /api/v1/counting/install "{\"reason\":\"e2e remove\",\"password\":\"$PASS\"}"; check "removal started" "$(st) $(jx j.job.action)" "202 remove"
wait_job >/dev/null; req GET /api/v1/counting/install
check "removed" "$(jx 'j.job.state + " " + (j.installed === null || j.installed === undefined)')" "done true"
req GET "/api/v1/counting/selfcheck?refresh=1"
check "self-check: ONNX Runtime no longer set" "$(jx '(c => c.state + " " + c.detail)(j.components.find(c=>c.name==="onnxruntime"))')" "missing analytics.onnxruntime is not set in config.toml"
if [[ -n "$CONFIG_VPN" && -f "$CONFIG_VPN" ]]; then
  grep -q '^onnxruntime = ' "$CONFIG_VPN" && bad "config.toml: paths left after the removal" || ok "config.toml: paths removed"
fi
sleep 2.5
req GET "/api/v1/audit?action=analytics.remove&limit=5"; check "removal audited" "$(jx 'j.entries.length >= 2')" true

echo
echo "analytics_install: $pass passed, $fail failed"
[[ $fail == 0 ]]

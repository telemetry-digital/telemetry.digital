#!/usr/bin/env bash
# End-to-end checks of offsite backups, the retention rule and the restore test (0.77.0, spec 032, URS K-01..K-04)
# against the server agent's simulation ([agent] vpn_simulate: the backup is real except the database dump, which is a
# stand-in, and the restore test does not restore into PostgreSQL). The offsite target is a folder made by this test.
# Covers: the schedule with weekly and monthly retention (validation, audit), the offsite target (browser session and
# password required, API tokens refused, connection test, secrets never returned), a backup copied and read back with
# its checksum file, the restore test from the offsite copy, a damaged copy detected with an incident, the incident
# resolved by the next good restore test, the audit trail of every result, and the protection of the passphrase.
#   BASE=… bash tests/e2e/backups.sh   (run-all.sh runs it against CONFIG_VPN)
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
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
wp()   { if command -v cygpath >/dev/null 2>&1; then cygpath -m "$1"; else printf %s "$1"; fi; }
# waits until no run is in progress; prints the newest run as JSON
wait_runs() { for _ in $(seq 60); do req GET /api/v1/server/backups; [[ "$(jx 'j.running || j.restore_running || j.history.some(r => r.running)')" == "false" ]] && { jx 'j.history[0]'; return; }; sleep 1; done; echo timeout; }

NAS="$TMP/nas"; mkdir -p "$NAS"; NASW=$(wp "$NAS")

echo "== login"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token); [[ -n "$CSRF" ]] && ok "csrf" || bad "csrf"

echo "== state and defaults"
req GET /api/v1/server/backups; check "backups state" "$(st)" 200
check "simulation" "$(jx j.db_mode)" simulated
check "GFS defaults" "$(jx '[j.schedule.keep_weekly, j.schedule.keep_monthly].join()')" "4,12"
check "monthly restore test on by default" "$(jx '[j.restore_test.enabled, j.restore_test.day].join()')" "true,1"
check "no offsite target" "$(jx j.offsite.config.enabled)" false

echo "== schedule"
req PUT /api/v1/server/backups/schedule '{"enabled":true,"hour":3,"minute":15,"keep":7,"keep_weekly":200,"keep_monthly":12,"reason":"e2e"}'; check "weekly limit" "$(st)" 502
req PUT /api/v1/server/backups/schedule '{"enabled":true,"hour":3,"minute":15,"keep":7,"keep_weekly":4,"keep_monthly":12}'; check "reason required" "$(st)" 400
req PUT /api/v1/server/backups/schedule '{"enabled":true,"hour":3,"minute":15,"keep":7,"keep_weekly":4,"keep_monthly":12,"passphrase":"e2e backup passphrase","reason":"e2e schedule"}'; check "schedule saved" "$(st)" 200
check "schedule values" "$(jx '[j.schedule.keep, j.schedule.keep_weekly, j.schedule.keep_monthly, j.schedule.has_passphrase].join()')" "7,4,12,true"
grep -q "e2e backup passphrase" "$TMP/b" && bad "passphrase returned" || ok "passphrase never returned"

echo "== offsite target: who may set it"
FOLDER="{\"enabled\":true,\"kind\":\"folder\",\"path\":\"$NASW\"}"
req PUT /api/v1/server/backups/offsite "{\"config\":$FOLDER,\"reason\":\"e2e\"}"; check "password required" "$(st) $(jx j.code)" "403 step_up_required"
req PUT /api/v1/server/backups/offsite "{\"config\":$FOLDER,\"reason\":\"e2e\",\"password\":\"wrong\"}"; check "wrong password" "$(st) $(jx j.code)" "403 step_up_failed"
req PUT /api/v1/server/backups/offsite "{\"config\":$FOLDER,\"password\":\"$PASS\"}"; check "reason required" "$(st)" 400
req POST /api/v1/tokens '{"name":"backups e2e","scopes":["system.admin"]}'; TK=$(jx j.token)
printf '%s' "{\"config\":$FOLDER,\"reason\":\"e2e\",\"password\":\"$PASS\"}" > "$TMP/req"
curl -s -o "$TMP/b" -w '%{http_code}' -X PUT -H "Authorization: Bearer $TK" -H "Content-Type: application/json" --data-binary @"$TMP/req" "$BASE/api/v1/server/backups/offsite" > "$TMP/s"
check "API token refused" "$(st) $(jx j.code)" "403 session_required"

echo "== offsite target: test and save"
req POST /api/v1/server/backups/offsite/test "{\"config\":{\"kind\":\"folder\",\"path\":\"$NASW/missing\"}}"; check "test answers" "$(st)" 200
check "missing folder reported" "$(jx j.ok)" false
req POST /api/v1/server/backups/offsite/test '{"config":{"kind":"folder","path":"relative/dir"}}'; check "relative path refused" "$(st)" 502
req POST /api/v1/server/backups/offsite/test '{"config":{"kind":"s3","endpoint":"http://127.0.0.1:1","bucket":"backup","access_key":"AK","secret_key":"SK","path_style":true}}'; check "s3 test answers" "$(st) $(jx j.ok)" "200 false"
req POST /api/v1/server/backups/offsite/test '{"config":{"kind":"s3","endpoint":"http://s3.example.com","bucket":"backup","access_key":"AK","secret_key":"SK"}}'; check "s3 over plain http to the internet refused" "$(st)" 502
req POST /api/v1/server/backups/offsite/test "{\"config\":$FOLDER}"; check "folder test" "$(st) $(jx j.ok)" "200 true"
ls "$NAS" | grep -q . && bad "probe file left at the target" || ok "probe file deleted"
req PUT /api/v1/server/backups/offsite "{\"config\":{\"enabled\":true,\"kind\":\"folder\",\"path\":\"$NASW/missing\"},\"reason\":\"e2e\",\"password\":\"$PASS\"}"; check "a target that does not work is not saved" "$(st)" 502
req PUT /api/v1/server/backups/offsite "{\"config\":$FOLDER,\"reason\":\"e2e offsite\",\"password\":\"$PASS\"}"; check "target saved" "$(st)" 200
check "target state" "$(jx '[j.offsite.config.enabled, j.offsite.config.kind].join()')" "true,folder"
req PUT /api/v1/server/backups/schedule '{"enabled":true,"hour":3,"minute":15,"keep":7,"keep_weekly":4,"keep_monthly":12,"clear_passphrase":true,"reason":"e2e"}'; check "passphrase kept while a target is on" "$(st)" 502
grep -q "only encrypted" "$TMP/b" && ok "clear message" || bad "message: $(cat "$TMP/b")"

echo "== backup copied offsite"
req POST /api/v1/server/backups '{"use_stored":true,"reason":"e2e backup"}'; check "backup started" "$(st)" 202
RUN=$(wait_runs)
echo "$RUN" > "$TMP/b"
check "backup ok" "$(jx j.ok)" true
check "steps" "$(jx 'j.steps.map(s => s.name).join()')" "dump,bundle,encrypt,checksum,upload,verify,prune_offsite"
check "actor recorded" "$(jx j.actor)" "$USER"
NAME=$(jx j.file)
[[ -f "$NAS/$NAME" && -f "$NAS/$NAME.sha256" ]] && ok "bundle and checksum at the target" || bad "files at the target: $(ls "$NAS")"
req GET /api/v1/server/backups
check "listed with checksum and offsite" "$(jx "(f => [f.encrypted, f.sha256.length, f.offsite].join())(j.files.find(f => f.name === '$NAME'))")" "true,64,true"
check "target listing" "$(jx "j.offsite.files.some(f => f.name === '$NAME')")" true

echo "== restore test"
req POST /api/v1/server/backups/restore-test '{"source":"offsite"}'; check "reason required" "$(st)" 400
req POST /api/v1/server/backups/restore-test '{"source":"offsite","reason":"e2e restore test"}'; check "restore test started" "$(st)" 202
RUN=$(wait_runs); echo "$RUN" > "$TMP/b"
check "restore test ok" "$(jx '[j.kind, j.ok, j.source].join()')" "restore_test,true,offsite"
check "restore test steps" "$(jx 'j.steps.map(s => s.name).join()')" "select,fetch,checksum,manifest,restore,checks"

echo "== damaged copy: incident"
printf 'garbage' >> "$NAS/$NAME"
req POST /api/v1/server/backups/restore-test '{"source":"offsite","reason":"e2e damaged"}'; check "second restore test started" "$(st)" 202
RUN=$(wait_runs); echo "$RUN" > "$TMP/b"
check "damage detected" "$(jx '[j.ok, j.steps[j.steps.length-1].name].join()')" "false,checksum"
req GET /api/v1/server/backups >/dev/null   # writes the results into the audit trail
req GET "/api/v1/incidents?status=open"
check "incident opened" "$(jx "j.some(i => i.kind === 'backup_failed' && i.title.startsWith('Restore test failed'))")" true

echo "== repaired: incident resolved"
req POST /api/v1/server/backups '{"use_stored":true,"reason":"e2e backup 2"}'; check "second backup started" "$(st)" 202
sleep 1.2; RUN=$(wait_runs); echo "$RUN" > "$TMP/b"; check "second backup ok" "$(jx j.ok)" true
req POST /api/v1/server/backups/restore-test '{"source":"offsite","reason":"e2e repaired"}'
RUN=$(wait_runs); echo "$RUN" > "$TMP/b"; check "restore test ok again" "$(jx j.ok)" true
req GET /api/v1/server/backups >/dev/null
req GET "/api/v1/incidents?status=open"
check "incident resolved" "$(jx "j.some(i => i.kind === 'backup_failed' && i.title.startsWith('Restore test failed'))")" false

echo "== audit trail"
req GET "/api/v1/audit?limit=200"
for a in server.backup_schedule server.backup_offsite server.backup_offsite_test server.backup_create server.backup_result server.restore_test_start server.restore_test_result; do
  [[ "$(jx "JSON.stringify(j).includes('\"$a\"')")" == "true" ]] && ok "audit $a" || bad "audit $a missing"
done
grep -q "e2e backup passphrase" "$TMP/b" && bad "passphrase in the audit trail" || ok "no passphrase in the audit trail"
req GET /api/v1/server/backups
check "every finished run audited" "$(jx 'j.history.filter(r => !r.running && !r.audited).length')" 0

echo "== offsite copy now, delete, remove the target"
req POST /api/v1/server/backups/offsite/sync '{"reason":"e2e sync"}'; check "sync started" "$(st)" 202
RUN=$(wait_runs); echo "$RUN" > "$TMP/b"; check "sync ok" "$(jx '[j.kind, j.ok].join()')" "offsite_sync,true"
req POST "/api/v1/server/backups/$NAME/delete" '{"reason":"e2e cleanup"}'; check "local delete" "$(st)" 200
req GET /api/v1/server/backups; check "deleted locally" "$(jx "j.files.some(f => f.name === '$NAME')")" false
[[ -f "$NAS/$NAME" ]] && ok "the offsite copy stays" || bad "offsite copy removed with the local file"
req PUT /api/v1/server/backups/offsite "{\"config\":{\"kind\":\"\"},\"reason\":\"e2e remove\",\"password\":\"$PASS\"}"; check "target removed" "$(st) $(jx j.offsite.config.enabled)" "200 false"

echo
echo "backups: $pass passed, $fail failed"
[[ $fail -eq 0 ]]

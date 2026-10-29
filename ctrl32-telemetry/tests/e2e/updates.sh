#!/usr/bin/env bash
# End-to-end checks of signed releases, update modes and offline updates (0.76.0, ADR-0024, spec 031) against the
# server agent's simulation ([agent] vpn_simulate: a release is verified exactly like on a server and its binary goes
# into the simulation directory; no service restarts, no database restore). The test release is a copy of the server
# binary under test, signed with a key pair generated for this run (RELKEY, key id build-test, trusted by the test
# build only) — never the project's release key. Covers: the update mode (permission, reason, validation, audit),
# the check of the distribution point (signed, key, release date), the mirror command, the online installation with
# backup and the health guard, the offline package upload (password step-up, API tokens refused, unsigned, untrusted
# and tampered packages refused, audit of refusals and results), the command line (update verify, update --file) and
# the history.
#   BASE=… BIN=… CONFIG_VPN=… RELSIGN=… RELKEY=… bash tests/e2e/updates.sh   (run-all.sh sets everything)
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d)
WWW_PID=""
trap '[[ -n "$WWW_PID" ]] && kill $WWW_PID 2>/dev/null; rm -rf "$TMP"' EXIT
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
# upload: file, password, reason [extra form fields…]
upload() { local f=$1 pw=$2 reason=$3; shift 3
  curl -s -b "$JAR" -c "$JAR" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' -F "reason=$reason" -F "password=$pw" "$@" -F "file=@$(wp "$f");type=application/zip" "$BASE/api/v1/server/update/package" > "$TMP/s"; }
# waits until the last installation left "installing"/"restarting"; prints its phase
wait_done() { local p
  for _ in $(seq 60); do req GET /api/v1/server/updates; p=$(jx 'j.state && j.state.current ? j.state.current.phase : "-"')
    [[ "$p" != installing && "$p" != restarting ]] && { echo "$p"; return; }; sleep 1; done; echo timeout; }
SUF=$RANDOM
# Windows (Git Bash): curl needs native paths for -F file=@…
wp() { if command -v cygpath >/dev/null 2>&1; then cygpath -m "$1"; else printf %s "$1"; fi; }
for v in BIN RELSIGN RELKEY CONFIG_VPN; do [[ -n "${!v:-}" ]] || { echo "$v is required (run through tests/run-all.sh)"; exit 2; }; done

echo "== a signed test release"
VOUT=$("$BIN" version)
VER=$(echo "$VOUT" | awk '{print $2}')
REL=$(echo "$VOUT" | sed -n 's/.* released \([0-9-]*\).*/\1/p'); REL=${REL:-$(date -u +%Y-%m-%d)}
GOOS=$(go env GOOS); GOARCH=$(go env GOARCH); EXE=""; [[ $GOOS == windows ]] && EXE=.exe
mkdir -p "$TMP/www/dl"
cp "$BIN" "$TMP/www/dl/ctrl32-telemetry-$GOOS-$GOARCH$EXE"
printf '%s\n' "$VER" > "$TMP/www/dl/VERSION"
( cd "$TMP/www/dl" && sha256sum ctrl32-telemetry-* > SHA256SUMS )
"$RELSIGN" sign -dir "$TMP/www/dl" -version "$VER" -released "$REL" -key "$RELKEY" > /dev/null && ok "signed with the test key ($VER, $REL)" || bad "sign"
"$RELSIGN" package -dir "$TMP/www/dl" -out "$TMP/good.zip" > /dev/null && ok "offline package built" || bad "package"
# an untrusted key, a tampered binary
"$RELSIGN" keygen -dir "$TMP/otherkey" -id build-test > /dev/null
cp -r "$TMP/www/dl" "$TMP/other"; rm -f "$TMP/other"/release.json*
"$RELSIGN" sign -dir "$TMP/other" -version "$VER" -released "$REL" -key "$TMP/otherkey/release-signing.key" > /dev/null
"$RELSIGN" package -dir "$TMP/other" -out "$TMP/untrusted.zip" > /dev/null
node -e '
const fs=require("fs"), zlib=require("zlib");
// rewrite one stored byte of the binary inside a copy of the package: the zip stays valid, the hash does not
const b=fs.readFileSync(process.argv[1]); let i=b.indexOf(Buffer.from("ctrl32-telemetry-"));
for(;i>=0;i=b.indexOf(Buffer.from("ctrl32-telemetry-"),i+1)){ if(b.readUInt32LE(i-30)===0x04034b50){ const n=b.readUInt16LE(i-4), x=b.readUInt16LE(i-2); const d=i+n+x+200; b[d]^=0xff; break; } }
fs.writeFileSync(process.argv[2], b);' "$TMP/good.zip" "$TMP/tampered.zip"
# the distribution point of the simulated agent (run-all.sh: update_url = http://127.0.0.1:18791/dl)
node -e '
const http=require("http"),fs=require("fs"),path=require("path"); const root=path.resolve(process.argv[1]);
http.createServer((q,r)=>{ const f=path.join(root, decodeURIComponent(q.url.split("?")[0])); if(!f.startsWith(root)||!fs.existsSync(f)||fs.statSync(f).isDirectory()){ r.writeHead(404); return r.end(); } r.writeHead(200); fs.createReadStream(f).pipe(r); }).listen(18791,"127.0.0.1");' "$TMP/www" &
WWW_PID=$!; sleep 0.5

echo "== command line"
"$BIN" update verify --file "$TMP/good.zip" > "$TMP/o" 2>&1 && grep -q "signed by build-test" "$TMP/o" && ok "update verify: good package" || bad "update verify good ($(head -3 "$TMP/o"))"
"$BIN" update verify --file "$TMP/untrusted.zip" > "$TMP/o" 2>&1 && bad "update verify accepted an untrusted package" || ok "update verify: untrusted key refused"
"$BIN" update verify --file "$TMP/tampered.zip" > "$TMP/o" 2>&1 && bad "update verify accepted a tampered package" || ok "update verify: tampered package refused"
"$BIN" update mirror --from http://127.0.0.1:18791/dl --to "$TMP/mirror" > "$TMP/o" 2>&1 && cmp -s "$TMP/mirror/release.json" "$TMP/www/dl/release.json" && cmp -s "$TMP/mirror/ctrl32-telemetry-$GOOS-$GOARCH$EXE" "$BIN" \
  && ok "update mirror: verified copy" || bad "update mirror ($(tail -3 "$TMP/o"))"

echo "== access and the update mode"
req GET /api/v1/server/updates; check "needs sign-in" "$(st)" 401
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req GET /api/v1/server/updates
check "state" "$(st) $(jx 'j.settings.mode + " " + j.agent.connected + " " + j.state.require_signature')" "200 notify true true"
check "trusted keys: the release key and the test key" "$(jx 'j.state.trusted_keys.join(",")')" "build-test,release-2026"
req POST /api/v1/users "{\"username\":\"upd_$SUF\",\"full_name\":\"Engineer\",\"email\":\"upd$SUF@example.com\",\"roles\":[\"engineer\"],\"reason\":\"e2e\"}"; PW=$(jx j.initial_password)
J2="$TMP/jar2"; CSRF2=""; req2() { call "$J2" "$CSRF2" "$@"; }
req2 POST /api/v1/auth/login "{\"username\":\"upd_$SUF\",\"password\":\"$PW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
req2 GET /api/v1/server/updates; check "an engineer cannot see it" "$(st)" 403
req PUT /api/v1/server/updates '{"mode":"auto"}'; check "reason required" "$(st)" 400
req PUT /api/v1/server/updates '{"mode":"sometimes","reason":"e2e"}'; check "unknown mode refused" "$(st)" 400
req PUT /api/v1/server/updates '{"mode":"auto","tz":"Mars/Olympus","reason":"e2e"}'; check "unknown time zone refused" "$(st)" 400
req PUT /api/v1/server/updates '{"mode":"auto","weekday":2,"time":"04:30","tz":"Europe/Bratislava","include_minor":true,"rollback_minutes":5,"reason":"e2e automatic"}'
check "automatic mode saved" "$(st) $(jx 'j.settings.mode + " " + j.settings.weekday + " " + j.settings.time + " " + j.settings.include_minor + " " + !!j.next_window')" "200 auto 2 04:30 true true"
req PUT /api/v1/server/updates '{"mode":"notify","weekday":0,"time":"03:00","tz":"","rollback_minutes":2,"reason":"e2e back to notify"}'; check "back to notify" "$(st) $(jx j.settings.mode)" "200 notify"
req GET "/api/v1/audit?action=server.update_settings&limit=5"; check "mode changes audited" "$(jx 'j.entries.some(e=>e.reason==="e2e automatic")')" true

echo "== the distribution point"
req POST /api/v1/server/updates/check '{}'
check "available, signed, key, date" "$(jx 'j.available.version + " " + j.available.signed + " " + j.available.key_id + " " + j.available.released')" "$VER true build-test $REL"
req GET /api/v1/server/updates/notice; check "no banner: the same version" "$(st) $(jx 'Object.keys(j).length')" "200 0"
req POST /api/v1/server/update '{"version":"9.9.9","reason":"e2e"}'; check "another version than offered" "$(st) $(jx j.code)" "409 version"
req POST /api/v1/server/update "{\"version\":\"$VER\",\"backup\":true,\"reason\":\"e2e online\"}"
# 0.86.0: the answer comes once the release is verified; the backup and the installation follow in the background
check "online installation started (202 once verified)" "$(st) $(jx 'j.version + " " + j.key_id + " " + j.phase + " " + !j.backup')" "202 $VER build-test installing true"
ID=$(jx j.id)
check "health guard: done" "$(wait_done)" done
req GET /api/v1/server/updates
check "history: online, manual, backup, in the background" "$(jx "(h => h.id + ' ' + h.source + ' ' + h.mode + ' ' + h.actor + ' ' + !!h.backup + ' ' + h.async)(j.state.history[0])")" "$ID online manual $USER true true"
req GET "/api/v1/audit?action=server.update_result&limit=5"; check "result audited" "$(jx 'j.entries.some(e=>(e.reason||"").includes("online") && (e.reason||"").endsWith("done"))')" true

echo "== offline package"
upload "$TMP/good.zip" "" "e2e"; check "password required" "$(st) $(jx j.code)" "403 step_up_required"
upload "$TMP/good.zip" "wrong-password" "e2e"; check "wrong password" "$(st) $(jx j.code)" "403 step_up_failed"
upload "$TMP/good.zip" "$PASS" ""; check "reason required" "$(st)" 400
req POST /api/v1/tokens '{"name":"updates e2e","scopes":["system.admin"]}'; TK=$(jx j.token)
curl -s -o "$TMP/b" -w '%{http_code}' -H "Authorization: Bearer $TK" -F reason=x -F "password=$PASS" -F "file=@$(wp "$TMP/good.zip")" "$BASE/api/v1/server/update/package" > "$TMP/s"
check "an API token is refused" "$(st) $(jx j.code)" "403 session_required"
upload "$TMP/untrusted.zip" "$PASS" "e2e untrusted"; check "untrusted key refused" "$(st) $(jx j.code)" "400 signature"
upload "$TMP/tampered.zip" "$PASS" "e2e tampered"; check "tampered binary refused" "$(st) $(jx j.code)" "400 checksum"
printf 'not a zip' > "$TMP/junk.zip"; upload "$TMP/junk.zip" "$PASS" "e2e junk"; check "not a package" "$(st) $(jx j.code)" "400 package"
upload "$TMP/good.zip" "$PASS" "e2e stage only" -F install=0
check "verified and staged only" "$(st) $(jx 'j.package.version + " " + j.package.key_id + " " + j.package.has_binary + " " + (j.update === undefined)')" "200 $VER build-test true true"
upload "$TMP/good.zip" "$PASS" "e2e offline" -F backup=1
check "uploaded, verified, installing in the background" "$(st) $(jx 'j.update.version + " " + j.update.phase')" "200 $VER installing"
check "health guard: done" "$(wait_done)" done
req GET /api/v1/server/updates; check "history: package with a backup" "$(jx 'j.state.history[0].source + " " + j.state.staged.version + " " + !!j.state.history[0].backup')" "package $VER true"
req GET "/api/v1/audit?action=server.update_package&limit=20"
check "refusals audited" "$(jx 'j.entries.filter(e=>e.new_value && e.new_value.refused).length >= 3')" true
check "installation audited" "$(jx 'j.entries.some(e=>e.reason==="e2e offline" && e.new_value && e.new_value.installed)')" true
req POST /api/v1/server/update '{"source":"package","wait":true,"backup":true,"reason":"e2e from the staged package"}'
check "install the staged package again, waiting for it (wait: the behaviour before 0.86.0)" "$(st) $(jx 'j.version + " " + !!j.backup + " " + !j.phase')" "200 $VER true true"
check "health guard: done" "$(wait_done)" done

echo "== command line through the agent"
"$BIN" update --file "$TMP/good.zip" --yes --no-backup --reason "e2e cli" -config "$CONFIG_VPN" > "$TMP/o" 2>&1 && grep -q "Installed $VER" "$TMP/o" && ok "update --file installed through the agent" || bad "update --file ($(tail -3 "$TMP/o"))"
check "health guard: done" "$(wait_done)" done
req GET /api/v1/server/updates; check "history: cli" "$(jx 'j.state.history[0].mode + " " + j.state.history[0].source')" "cli package"
"$BIN" update --file "$TMP/tampered.zip" --yes -config "$CONFIG_VPN" > "$TMP/o" 2>&1 && bad "the command line installed a tampered package" || ok "update --file refuses a tampered package"
req GET /api/v1/openapi.json
check "documented in the API" "$(jx '!!(j.paths["/api/v1/server/update/package"] && j.paths["/api/v1/server/updates"].put)')" true

echo
echo "updates: $pass passed, $fail failed"
[[ $fail == 0 ]]

#!/usr/bin/env bash
# End-to-end checks for spec 034 (0.86.0, ADR-0025): remote management inside ctrl32 telemetry — the operating system
# of the server through the agent's simulation (updates, the upgrade job with its log, reboot and its refusals, the
# closed list of logs, the audit trail of requests and results), MeshCentral as the audited emergency path, what
# displays report about their camera tiles, and kiosk PCs (registration, setup script, the agent's report, the closed
# list of operations, screenshots off by default and fetched once, links of displays to monitors, credential
# replacement and removal).
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/remote.sh   (run-all.sh: against the agent's simulation)
set -u
cd "$(dirname "$0")/../.."
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; KJ="$TMP/kiosk"; CSRF=""
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
kiosk(){ call "$KJ" "" "$@"; }
# the kiosk PC's agent: its own credential in a header, no cookies
agent(){ local tok=$1 m=$2 p=$3 d=${4:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -X "$m" -H "X-Kiosk-Token: $tok" -H "Content-Type: application/json" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -X "$m" -H "X-Kiosk-Token: $tok" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
waitfor() { local n=${2:-40}; for i in $(seq "$n"); do eval "$1" && return 0; sleep 1; done; return 1; }
audited() { req GET "/api/v1/audit?action=$1&limit=50"; jx "j.entries.some(e => JSON.stringify(e).includes('$2'))"; }
SUF=$RANDOM

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
check "the administrator may see screenshots (display.screenshot, org_admin)" "$(jx 'j.permissions.includes("display.screenshot")')" true

echo "== operating system (the agent's simulation)"
req GET /api/v1/server/os; check "state" "$(st) $(jx 'j.supported + " " + j.package_manager + " " + j.upgrade_count + " " + j.security_count')" "200 true apt 2 1"
check "packages listed with versions and source" "$(jx 'j.upgrades[0].name + " " + j.upgrades[0].security + " " + !!j.upgrades[0].origin')" "libssl3t64 true true"
req POST /api/v1/server/os/check '{}'; check "check needs a reason" "$(st)" 400
req POST /api/v1/server/os/check '{"reason":"e2e check"}'; check "check started" "$(st) $(jx 'j.kind + " " + j.phase')" "202 check running"
waitfor '(req GET /api/v1/server/os; [[ "$(jx "!j.checking && !j.job")" == true ]])' 20 && ok "check finished" || bad "check still running"
req POST /api/v1/server/os/upgrade '{"reason":"e2e monthly updates"}'; check "upgrade started" "$(st) $(jx 'j.kind + " " + j.phase')" "202 upgrade running"
JOB=$(jx j.id)
req POST /api/v1/server/os/reboot '{"reason":"e2e"}'; check "no reboot while the upgrade runs" "$(st) $(jx j.code)" "409 busy"
req POST /api/v1/server/os/upgrade '{"reason":"e2e twice"}'; check "one job at a time" "$(st) $(jx j.code)" "409 busy"
req GET /api/v1/server/os; check "the running job with its log" "$(jx "j.job && j.job.id === '$JOB' && typeof j.job.log === 'string'")" true
waitfor '(req GET /api/v1/server/os; [[ "$(jx "!j.job && j.last && j.last.id === \"$JOB\"")" == true ]])' 30 && ok "upgrade finished" || bad "upgrade still running"
check "result: upgraded, reboot required, log kept" "$(jx 'j.last.phase + " " + j.last.upgrades + " " + j.reboot_required + " " + j.reboot_packages.join() + " " + j.last.log.includes("2 upgraded") + " " + j.upgrade_count')" "done 2 true libssl3t64 true 0"
req GET /api/v1/server; check "health in the status: RAID, link, reboot required" "$(jx 'j.agent.status.health.raid[0].health + " " + j.agent.status.health.links[0].state + " " + j.agent.status.health.reboot_required')" "ok up true"
req POST /api/v1/server/os/reboot '{"reason":"e2e new kernel","delay_s":5}'; check "reboot delay validated" "$(st)" 400
req POST /api/v1/server/os/reboot '{"reason":"e2e new kernel"}'; check "reboot scheduled in a minute" "$(st) $(jx "Math.round((Date.parse(j.at) - Date.parse(j.requested_at)) / 1000)")" "202 60"
req GET /api/v1/server/os; check "…in the state" "$(jx 'j.reboot && j.reboot.reason')" "e2e new kernel"
req POST /api/v1/server/os/upgrade '{"reason":"e2e"}'; check "no upgrade while a reboot is scheduled" "$(st)" 409
req POST /api/v1/server/os/reboot/cancel '{}'; check "cancel needs a reason" "$(st)" 400
req POST /api/v1/server/os/reboot/cancel '{"reason":"e2e not now"}'; check "reboot cancelled" "$(st)" 200
req POST /api/v1/server/os/reboot/cancel '{"reason":"e2e"}'; check "nothing left to cancel" "$(st) $(jx j.code)" "404 not_found"
req GET /api/v1/server/os >/dev/null
check "requests audited (check, upgrade, reboot, cancel)" "$(audited server.os_check 'e2e check'),$(audited server.os_upgrade 'e2e monthly updates'),$(audited server.reboot 'e2e new kernel'),$(audited server.reboot_cancel 'e2e not now')" "true,true,true,true"
check "results audited once (server.os_job_result)" "$(audited server.os_job_result "$JOB")" true
req GET "/api/v1/server/logs?unit=../../etc/shadow"; check "logs: a closed list" "$(st)" 400
req GET "/api/v1/server/logs?unit=app&lines=50"; check "logs of the service" "$(st) $(jx 'j.unit + " " + (j.lines.length > 0)')" "200 app true"

echo "== MeshCentral: the emergency path"
req POST /api/v1/server/remote/open '{"reason":"e2e"}'; check "not configured yet" "$(st) $(jx j.code)" "409 not_configured"
req PUT /api/v1/server/remote '{"mesh_url":"http://mesh.example.com","mesh_node":"","reason":"e2e"}'; check "https only" "$(st)" 400
req PUT /api/v1/server/remote '{"mesh_url":"https://mesh.example.com","mesh_node":"bad\"node","reason":"e2e"}'; check "node validated" "$(st)" 400
NODE='h3l$p4ZnbBa43Fd82gTIlqKsfGNo02d1gXFhiM78EM5DSwiOzo3jLsRJ3sNgrlnO'
req PUT /api/v1/server/remote "{\"mesh_url\":\"https://mesh.example.com/\",\"mesh_node\":\"node//$NODE\",\"reason\":\"e2e mesh\"}"; check "saved (prefix removed)" "$(st) $(jx 'j.mesh_url + " " + j.mesh_node')" "200 https://mesh.example.com $NODE"
req POST /api/v1/server/remote/open '{}'; check "opening needs a reason" "$(st)" 400
req POST /api/v1/server/remote/open '{"reason":"e2e emergency"}'; check "link to the remote desktop" "$(st) $(jx 'j.url')" "200 https://mesh.example.com/?gotonode=$NODE&viewmode=11&hide=16"
check "opening audited" "$(audited remote.mesh_open 'e2e emergency')" true

echo "== a display reports its tiles"
mk() { req POST /api/v1/video/cameras "{\"name\":\"$1 $SUF\",\"url\":\"rtsp://127.0.0.1:1/x\",\"record\":\"off\",\"enabled\":false,\"reason\":\"e2e\"}"; jx j.id; }
C1=$(mk Gate); C2=$(mk Yard); C3=$(mk Elsewhere)
req POST /api/v1/video/walls "{\"name\":\"Wall $SUF\",\"reason\":\"e2e\",\"screens\":[{\"cols\":2,\"rows\":1,\"tiles\":[\"$C1\",\"$C2\"]}]}"; WALL=$(jx j.id)
kiosk POST /api/v1/display/hello '{}'; CODE=$(jx j.code)
req POST /api/v1/displays/pair "{\"code\":\"$CODE\",\"name\":\"Top $SUF\",\"content\":{\"items\":[{\"kind\":\"wall\",\"wall\":\"$WALL\",\"screen\":0}],\"popup\":\"none\"},\"reason\":\"e2e\"}"; DID=$(jx j.id)
kiosk POST /api/v1/display/hello '{"code_known":true}'; check "display paired" "$(jx j.paired)" true
curl -s -X POST -H 'Content-Type: application/json' -d '{"tiles":[]}' -o /dev/null -w '%{http_code}' "$BASE/api/v1/display/health" > "$TMP/s"; check "health needs the display cookie" "$(st)" 401
H="{\"page_version\":\"0.86.0\",\"item\":\"Wall $SUF\",\"screen\":{\"w\":1920,\"h\":1080,\"vw\":1920,\"vh\":1080,\"dpr\":1},\"tiles\":[{\"camera\":\"$C1\",\"state\":\"playing\"},{\"camera\":\"$C2\",\"state\":\"stalled\",\"since_s\":42},{\"camera\":\"$C3\",\"state\":\"playing\"},{\"camera\":\"$C1\",\"state\":\"exploded\"}],\"kiosk\":\"KIOSK\",\"monitor\":1}"
kiosk POST /api/v1/display/health "${H/KIOSK/00000000-0000-7000-8000-000000000000}"; check "reported" "$(st)" 200
kiosk POST /api/v1/display/health "${H/KIOSK/00000000-0000-7000-8000-000000000000}"; check "at most every 10 s" "$(st)" 429
req GET /api/v1/displays
check "the list: playing of the tiles, the camera that is not, screen and page" "$(jx "(h => h.playing + '/' + h.tiles + ' ' + h.problems.map(p => p.name + ':' + p.state + ':' + p.since_s).join() + ' ' + h.screen.w + 'x' + h.screen.h + ' ' + h.page_version)(j.find(d => d.id === '$DID').health)")" "1/2 Yard $SUF:stalled:42 1920x1080 0.86.0"

echo "== kiosk PCs"
req POST /api/v1/kiosks "{\"name\":\"Wall PC $SUF\",\"monitors\":2,\"server_url\":\"http://10.0.0.5:8080\"}"; check "registration needs a reason" "$(st)" 400
req POST /api/v1/kiosks "{\"name\":\"Wall PC $SUF\",\"monitors\":2,\"server_url\":\"javascript:alert(1)\",\"reason\":\"e2e\"}"; check "server address validated" "$(st)" 400
req POST /api/v1/kiosks "{\"name\":\"Wall PC $SUF\",\"monitors\":9,\"reason\":\"e2e\"}"; check "monitors 1 to 8" "$(st)" 400
req POST /api/v1/kiosks "{\"name\":\"Wall PC $SUF\",\"monitors\":2,\"server_url\":\"http://10.0.0.5:8080/\",\"reason\":\"e2e register\"}"
check "registered, credential shown once, screenshots off" "$(st) $(jx 'j.token.startsWith("c32k_") + " " + j.kiosk.screenshots + " " + j.kiosk.server_url + " " + j.kiosk.online')" "201 true false http://10.0.0.5:8080 false"
KID=$(jx j.kiosk.id); TOK=$(jx j.token)
req GET /api/v1/kiosks; check "listed without the credential" "$(jx "!JSON.stringify(j).includes('$TOK') && j.some(k => k.id === '$KID')")" true
req POST "/api/v1/kiosks/$KID/setup" '{"token":"c32k_wrong"}'; check "setup script only with its credential" "$(st)" 400
curl -s -b "$JAR" -X POST -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -D "$TMP/h" -o "$TMP/setup.ps1" -w '%{http_code}' --data-binary "{\"token\":\"$TOK\"}" "$BASE/api/v1/kiosks/$KID/setup" > "$TMP/s"
check "setup script" "$(st) $(grep -c "\$Token = '$TOK'" "$TMP/setup.ps1") $(grep -c "\$Server = 'http://10.0.0.5:8080'" "$TMP/setup.ps1") $(grep -c '__CTRL32_' "$TMP/setup.ps1")" "200 1 1 0"
grep -qi 'attachment; filename="setup-kiosk-wall-pc' "$TMP/h" && ok "…as a download" || bad "no attachment header"
agent c32k_nope POST /api/v1/kiosk/report '{"report":{}}'; check "agent: unknown credential" "$(st)" 401
agent "$TOK" GET /api/v1/kiosk/scripts; check "agent: its scripts" "$(st) $(jx 'Object.keys(j.files).sort().join() + " " + j.version.length')" "200 capture.ps1,kiosk-agent.ps1,kiosk-launcher.ps1,launcher.json 12"
SV=$(jx j.version)
check "…the launcher settings of this PC" "$(jx "JSON.parse(j.files['launcher.json']).url + ' ' + JSON.parse(j.files['launcher.json']).monitors + ' ' + JSON.parse(j.files['launcher.json']).kiosk")" "http://10.0.0.5:8080/display 2 $KID"
REP="{\"report\":{\"hostname\":\"DESKTOP-$SUF\",\"windows\":\"Windows 11 Pro\",\"uptime_s\":3600,\"cpu_pct\":19,\"gpu_decode_pct\":43,\"ram_total_mb\":7900,\"ram_free_mb\":4200,\"browser\":\"130.0\",\"browser_windows\":2,\"scripts_version\":\"$SV\"},\"result\":null}"
agent "$TOK" POST /api/v1/kiosk/report "$REP"; check "agent reports" "$(st) $(jx 'j.poll_s + " " + j.scripts_version + " " + !j.command')" "200 15 $SV true"
req GET /api/v1/kiosks; check "online with its report, scripts current" "$(jx "(k => k.online + ' ' + k.report.hostname + ' ' + k.scripts_current)(j.find(k => k.id === '$KID'))")" "true DESKTOP-$SUF true"
req POST "/api/v1/kiosks/$KID/command" '{"action":"format_c","reason":"e2e"}'; check "a closed list of operations" "$(st)" 400
req POST "/api/v1/kiosks/$KID/command" '{"action":"reboot"}'; check "an operation needs a reason" "$(st)" 400
req POST "/api/v1/kiosks/$KID/command" '{"action":"restart_browsers","reason":"e2e frozen tile"}'; check "restart the browsers asked" "$(st) $(jx j.action)" "200 restart_browsers"
SEQ=$(jx j.seq)
agent "$TOK" POST /api/v1/kiosk/report "$REP"; check "the agent gets it" "$(jx 'j.command.action + " " + j.command.by')" "restart_browsers $USER"
NORES='"result":null'; RES="\"result\":{\"seq\":$SEQ,\"action\":\"restart_browsers\",\"ok\":true,\"message\":\"stopped 12 browser processes\"}"
agent "$TOK" POST /api/v1/kiosk/report "${REP/$NORES/$RES}"
check "…and answers; the operation is done" "$(st) $(jx '!j.command')" "200 true"
req GET /api/v1/kiosks; check "the result is shown" "$(jx "(k => k.result.ok + ' ' + k.result.message + ' ' + !k.command)(j.find(k => k.id === '$KID'))")" "true stopped 12 browser processes true"
check "operation audited" "$(audited kiosk.command 'restart_browsers')" true
req POST "/api/v1/kiosks/$KID/command" '{"action":"screenshot","reason":"e2e"}'; check "screenshots are off by default" "$(st) $(jx j.code)" "409 screenshots_off"
req PUT "/api/v1/kiosks/$KID" '{"screenshots":true}'; check "allowing them needs a reason" "$(st)" 400
req PUT "/api/v1/kiosks/$KID" '{"screenshots":true,"reason":"e2e: checking the wall remotely"}'; check "screenshots allowed" "$(st) $(jx j.screenshots)" "200 true"
check "…audited with the reason" "$(audited kiosk.update 'e2e: checking the wall remotely')" true
req POST "/api/v1/kiosks/$KID/command" '{"action":"screenshot","reason":"e2e is the wall dark?"}'; SEQ=$(jx j.seq); check "screenshot asked" "$(st)" 200
req GET "/api/v1/kiosks/$KID/screenshot"; check "…waiting for the PC" "$(st) $(jx j.waiting)" "202 true"
printf '\xff\xd8\xff\xe0fake jpeg for e2e' > "$TMP/shot.jpg"
curl -s -X POST -H "X-Kiosk-Token: $TOK" -H "Content-Type: image/jpeg" --data-binary @"$TMP/shot.jpg" -o "$TMP/b" -w '%{http_code}' "$BASE/api/v1/kiosk/screenshot?seq=1" > "$TMP/s"; check "only the picture asked for" "$(st)" 409
curl -s -X POST -H "X-Kiosk-Token: $TOK" -H "Content-Type: text/plain" --data-binary @"$TMP/shot.jpg" -o "$TMP/b" -w '%{http_code}' "$BASE/api/v1/kiosk/screenshot?seq=$SEQ" > "$TMP/s"; check "only a picture" "$(st)" 415
curl -s -X POST -H "X-Kiosk-Token: $TOK" -H "Content-Type: image/jpeg" --data-binary @"$TMP/shot.jpg" -o "$TMP/b" -w '%{http_code}' "$BASE/api/v1/kiosk/screenshot?seq=$SEQ" > "$TMP/s"; check "the agent delivers it" "$(st)" 200
curl -s -b "$JAR" -o "$TMP/got.jpg" -w '%{http_code} %{content_type}' "$BASE/api/v1/kiosks/$KID/screenshot" > "$TMP/s"; check "fetched once" "$(st) $(cmp -s "$TMP/shot.jpg" "$TMP/got.jpg" && echo same)" "200 image/jpeg same"
req GET "/api/v1/kiosks/$KID/screenshot"; check "…and gone" "$(st)" 404
check "screenshot audited with its reason" "$(audited kiosk.command 'e2e is the wall dark?')" true
req PUT "/api/v1/kiosks/$KID" "{\"displays\":[{\"id\":\"$DID\",\"monitor\":2}],\"reason\":\"e2e\"}"; check "a monitor the PC does not have" "$(st)" 400
req PUT "/api/v1/kiosks/$KID" "{\"displays\":[{\"id\":\"$DID\",\"monitor\":0},{\"id\":\"$DID\",\"monitor\":1}],\"reason\":\"e2e\"}"; check "a display on one monitor only" "$(st)" 400
req PUT "/api/v1/kiosks/$KID" "{\"displays\":[{\"id\":\"$DID\",\"monitor\":1}],\"reason\":\"e2e link\"}"; check "display linked to monitor 2" "$(st)" 200
req GET /api/v1/kiosks; check "…shown at the PC" "$(jx "(k => k.displays.map(d => d.name + ':' + d.monitor).join())(j.find(k => k.id === '$KID'))")" "Top $SUF:1"
req GET /api/v1/displays; check "…and at the display" "$(jx "(d => d.kiosk_pc + ':' + d.kiosk_monitor)(j.find(d => d.id === '$DID'))")" "$KID:1"
req POST "/api/v1/kiosks/$KID/remote" '{"reason":"e2e"}'; check "remote desktop needs the PC's node" "$(st) $(jx j.code)" "409 not_configured"
req PUT "/api/v1/kiosks/$KID" "{\"mesh_node\":\"$NODE\",\"reason\":\"e2e\"}"
req POST "/api/v1/kiosks/$KID/remote" '{"reason":"e2e wall pc"}'; check "remote desktop of the PC" "$(st) $(jx 'j.url.includes("gotonode=")')" "200 true"
req POST "/api/v1/kiosks/$KID/token" '{"reason":"e2e new credential"}'; check "credential replaced" "$(st) $(jx 'j.token.startsWith("c32k_")')" "200 true"
NEW=$(jx j.token)
agent "$TOK" POST /api/v1/kiosk/report "$REP"; check "the old credential stops at once" "$(st)" 401
agent "$NEW" POST /api/v1/kiosk/report "$REP"; check "the new one works" "$(st)" 200
req DELETE "/api/v1/kiosks/$KID"; check "removal needs a reason" "$(st)" 400
req DELETE "/api/v1/kiosks/$KID?reason=e2e"; check "removed" "$(st)" 200
agent "$NEW" POST /api/v1/kiosk/report "$REP"; check "its agent is locked out" "$(st)" 401
req GET /api/v1/displays; check "the display stays paired, unlinked" "$(jx "(d => d.paired + ' ' + d.kiosk_pc)(j.find(d => d.id === '$DID'))")" "true "
check "registration, change, credential and removal audited" "$(audited kiosk.create "Wall PC $SUF"),$(audited kiosk.token 'e2e new credential'),$(audited kiosk.remove "Wall PC $SUF")" "true,true,true"
req GET /api/v1/openapi.json; check "documented in the API" "$(jx '!!(j.paths["/api/v1/kiosks"] && j.paths["/api/v1/kiosk/report"] && j.paths["/api/v1/server/os"] && j.paths["/api/v1/display/health"])')" true

echo "== cleanup"
req PUT /api/v1/server/remote '{"mesh_url":"","mesh_node":"","reason":"e2e cleanup"}'
req DELETE "/api/v1/displays/$DID?reason=e2e"
req DELETE "/api/v1/video/walls/$WALL?reason=e2e"
for c in $C1 $C2 $C3; do req DELETE "/api/v1/video/cameras/$c?reason=e2e"; done; check "cleaned up" "$(st)" 200
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

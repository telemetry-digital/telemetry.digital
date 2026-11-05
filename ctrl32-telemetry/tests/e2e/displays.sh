#!/usr/bin/env bash
# End-to-end checks for spec 014: displays (kiosks) — pairing with a code, what a display may read (its dashboards and
# walls only, cameras only on its content), operator commands, revocation; camera groups limiting a user; health
# incidents for a recording camera that is offline and a display that stopped showing.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/displays.sh
set -u
cd "$(dirname "$0")/../.."
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; KJ="$TMP/kiosk"; J2="$TMP/jar2"; CSRF=""; CSRF2=""
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
req2() { call "$J2" "$CSRF2" "$@"; }
kiosk(){ call "$KJ" "" "$@"; }
waitfor() { local n=${2:-40}; for i in $(seq "$n"); do eval "$1" && return 0; sleep 1; done; return 1; }
SUF=$RANDOM

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
check "admin may pair and control displays" "$(jx '["display.manage","display.control"].every(p => j.permissions.includes(p))')" true
DW="{\"name\":\"Kiosk $SUF\",\"reason\":\"e2e\",\"spec\":{\"version\":1,\"kind\":\"dashboard\",\"layout\":{\"type\":\"grid\",\"row_height\":96},\"range\":\"6h\",\"refresh_s\":30,\"widgets\":[{\"id\":\"c\",\"type\":\"camera\",\"geometry\":{\"x\":0,\"y\":0,\"w\":4,\"h\":3},\"bindings\":{},\"params\":{\"camera\":\"CAMID\"}}]}}"
DP="{\"name\":\"Plan $SUF\",\"reason\":\"e2e\",\"spec\":{\"version\":1,\"kind\":\"dashboard\",\"layout\":{\"type\":\"grid\",\"row_height\":96},\"range\":\"6h\",\"refresh_s\":30,\"widgets\":[{\"id\":\"p\",\"type\":\"camera_pin\",\"geometry\":{\"x\":4,\"y\":0,\"w\":1,\"h\":1},\"bindings\":{},\"params\":{\"camera\":\"PINID\",\"direction\":180}}]}}"
mk() { req POST /api/v1/video/cameras "{\"name\":\"$1 $SUF\",\"url\":\"rtsp://127.0.0.1:1/x\",\"record\":\"$3\",\"enabled\":$4,\"group\":\"$2\",\"reason\":\"e2e\"}"; jx j.id; }
GATE=$(mk Gate "gate $SUF" off false); YARD=$(mk Yard "yard $SUF" off false); ODD=$(mk Other "" off false); PIN=$(mk Pin "" off false)
check "cameras with groups" "$(jx j.group)" ""
req GET "/api/v1/video/cameras/$GATE"; check "group stored" "$(jx j.group)" "gate $SUF"
D1=$(node -e 'const a=JSON.parse(process.argv[1]), b=JSON.parse(process.argv[2]); a.spec.widgets.push(...b.spec.widgets); console.log(JSON.stringify(a))' "${DW/CAMID/$GATE}" "${DP/PINID/$PIN}")
req POST /api/v1/dashboards "$D1"; check "dashboard with a camera and a camera on a plan" "$(st)" 201; SLUG=$(jx j.slug)
req POST /api/v1/dashboards "{\"name\":\"Other $SUF\",\"reason\":\"e2e\",\"spec\":{\"version\":1,\"kind\":\"dashboard\",\"layout\":{\"type\":\"grid\",\"row_height\":96},\"range\":\"6h\",\"refresh_s\":30,\"widgets\":[]}}"; OTHER=$(jx j.slug)
req POST /api/v1/video/walls "{\"name\":\"Wall $SUF\",\"reason\":\"e2e\",\"screens\":[{\"cols\":2,\"rows\":1,\"tiles\":[\"$YARD\"]},{\"cols\":1,\"rows\":1,\"tiles\":[\"$ODD\"]}]}"; WALL=$(jx j.id)
check "playback from the wall is off by default" "$(jx j.playback_on_click)" false
req PUT "/api/v1/video/walls/$WALL" '{"playback_on_click":true,"reason":"e2e: playback from the wall"}'; check "playback from the wall allowed" "$(st) $(jx j.playback_on_click) $(jx 'j.screens.length')" "200 true 2"
req PUT "/api/v1/video/walls/$WALL" "{\"name\":\"Wall $SUF\",\"reason\":\"e2e: rename keeps the option\"}"; check "…kept when left out" "$(jx j.playback_on_click)" true
req GET "/api/v1/audit?action=video_wall.update&limit=20"; check "…audited with the reason" "$(jx 'JSON.stringify(j).includes("e2e: playback from the wall")')" true
curl -s -D "$TMP/fh" -o /dev/null "$BASE/video/cameras/$YARD?embed=1"; check "the embedded camera page may be framed by the same origin only" "$(grep -ci 'frame-ancestors .self.' "$TMP/fh") $(grep -ci 'x-frame-options: sameorigin' "$TMP/fh")" "1 1"
curl -s -D "$TMP/fh" -o /dev/null "$BASE/video/cameras/$YARD"; check "…any other page not at all" "$(grep -ci "frame-ancestors 'none'" "$TMP/fh") $(grep -ci 'x-frame-options: deny' "$TMP/fh")" "1 1"

echo "== pairing"
kiosk POST /api/v1/display/hello '{}'; check "kiosk gets a code" "$(st) $(jx '/^[A-Z2-9]{4}-[A-Z2-9]{4}$/.test(j.code) && !j.paired')" "200 true"
CODE=$(jx j.code)
grep -q ctrl32_display_pair "$KJ" && ok "the kiosk keeps a pairing secret in a cookie" || bad "no pairing cookie"
grep -P 'ctrl32_display_pair' "$KJ" | grep -q '#HttpOnly_' && ok "pairing cookie is HttpOnly" || bad "pairing cookie not HttpOnly"
kiosk POST /api/v1/display/hello '{"code_known":true}'; check "kiosk waits (the code is not repeated)" "$(jx 'j.paired === false && !j.code')" true
CONTENT="{\"items\":[{\"kind\":\"dashboard\",\"slug\":\"$SLUG\",\"seconds\":20},{\"kind\":\"wall\",\"wall\":\"$WALL\",\"screen\":0,\"seconds\":20}],\"popup\":\"wall\",\"popup_seconds\":15,\"locale\":\"sk\"}"
req POST /api/v1/displays/pair "{\"code\":\"ZZZZ-ZZZZ\",\"name\":\"x\",\"content\":$CONTENT,\"reason\":\"e2e\"}"; check "unknown code refused" "$(st)" 404
req POST /api/v1/displays/pair "{\"code\":\"$CODE\",\"name\":\"x\",\"content\":{\"items\":[{\"kind\":\"dashboard\",\"slug\":\"no-such-$SUF\"}]},\"reason\":\"e2e\"}"; check "content validated" "$(st)" 400
req POST /api/v1/displays/pair "{\"code\":\"$CODE\",\"name\":\"x\",\"content\":{\"items\":[{\"kind\":\"wall\",\"wall\":\"$WALL\",\"screen\":5}]},\"reason\":\"e2e\"}"; check "wall screen validated" "$(st)" 400
req POST /api/v1/displays/pair "{\"code\":\"$CODE\",\"name\":\"x\",\"content\":$CONTENT}"; check "pairing needs a reason" "$(st)" 400
req POST /api/v1/displays/pair "{\"code\":\"${CODE,,}\",\"name\":\"Control room $SUF\",\"content\":$CONTENT,\"reason\":\"e2e\"}"; check "paired (code case-insensitive)" "$(st) $(jx 'j.paired + " " + (j.device_ip.length > 0)')" "201 false true"
DID=$(jx j.id)
req POST /api/v1/displays/pair "{\"code\":\"$CODE\",\"name\":\"again\",\"content\":$CONTENT,\"reason\":\"e2e\"}"; check "a code pairs only once" "$(st)" 404
curl -s -X POST -H 'Content-Type: application/json' -d '{"code_known":true}' -o /dev/null -w '%{http_code}' "$BASE/api/v1/display/hello" > "$TMP/s"
kiosk POST /api/v1/display/hello '{"code_known":true}'; check "the kiosk collects its credential" "$(jx j.paired)" true
grep -P '\tctrl32_display\t' "$KJ" | grep -q '#HttpOnly_' && ok "display cookie is HttpOnly" || bad "display cookie not HttpOnly"
grep -q 'ctrl32_display_pair' "$KJ" && [[ "$(grep -P '\tctrl32_display_pair\t' "$KJ" | awk '{print $7}')" != "" ]] && bad "pairing secret left behind" || ok "pairing secret removed"

echo "== what a display may read"
kiosk GET /api/v1/display/state; check "state" "$(st) $(jx '[j.name, j.items.length, j.locale, j.items[1].screen].join(" ")')" "200 Control room $SUF 2 sk 0"
check "state carries the server version and time (reload after an update, commands after the page loaded)" "$(jx "j.server_version === '$(curl -s "$BASE/healthz" | node -pe 'JSON.parse(require("fs").readFileSync(0)).version')' && !isNaN(Date.parse(j.server_time))")" true
check "camera names of its content only" "$(jx "Object.keys(j.cameras).sort().join(',')")" "$(echo -e "$GATE\n$PIN\n$YARD" | sort | paste -sd,)"
kiosk GET "/api/v1/public/~$SLUG"; check "its dashboard" "$(st)" 200
kiosk GET "/api/v1/public/~$OTHER"; check "another dashboard refused" "$(st)" 404
curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/public/~$SLUG" > "$TMP/s"; check "dashboard without the display cookie refused" "$(st)" 404
kiosk GET "/api/v1/display/walls/$WALL"; check "its wall with camera names" "$(st) $(jx "Object.keys(j.cameras).length")" "200 2"
kiosk GET "/api/v1/display/cameras/$ODD/live"; check "camera of another wall screen refused" "$(st)" 403
kiosk GET "/api/v1/display/cameras/$GATE/live"; [[ "$(st)" != 403 && "$(st)" != 401 ]] && ok "camera of its dashboard allowed ($(st))" || bad "camera of its dashboard: $(st)"
kiosk GET "/api/v1/display/cameras/$PIN/live"; [[ "$(st)" != 403 && "$(st)" != 401 ]] && ok "camera on a plan of its dashboard allowed ($(st))" || bad "camera on a plan: $(st)"
echo "== recordings on a display: off by default, playing only"
FROMR=$(date -u -d '-1 hour' +%Y-%m-%dT%H:%M:%SZ)
kiosk GET /api/v1/display/state; check "recordings off by default" "$(jx j.playback)" false
kiosk GET "/api/v1/display/cameras/$YARD/recordings?from=$FROMR"; check "time line refused while off" "$(st)" 403
kiosk GET "/api/v1/display/cameras/$YARD/playback?from=$FROMR"; check "playback refused while off" "$(st)" 403
req GET /api/v1/displays; DIDR=$(jx "(j.displays||j.items||j).find(d => d.name === 'Control room $SUF').id")
req PUT "/api/v1/displays/$DIDR" '{"allow_playback":true}'; check "switching on needs a reason" "$(st)" 400
req PUT "/api/v1/displays/$DIDR" '{"allow_playback":true,"reason":"e2e: watching at the wall"}'; check "switched on" "$(st) $(jx j.allow_playback)" "200 true"
kiosk GET /api/v1/display/state; check "the display knows" "$(jx j.playback)" true
kiosk GET "/api/v1/display/cameras/$YARD/recordings?from=$FROMR"; check "time line of a camera of its content" "$(st) $(jx 'Array.isArray(j.spans)')" "200 true"
kiosk GET "/api/v1/display/cameras/$YARD/playback?from=$FROMR"; [[ "$(st)" != 403 && "$(st)" != 401 ]] && ok "playback of a camera of its content allowed ($(st))" || bad "playback: $(st)"
kiosk GET "/api/v1/display/cameras/$ODD/recordings?from=$FROMR"; check "camera of another wall screen refused" "$(st)" 403
kiosk GET "/api/v1/display/cameras/$ODD/playback?from=$FROMR"; check "its playback refused" "$(st)" 403
kiosk GET "/api/v1/display/cameras/$YARD/export?from=$FROMR"; check "no download route for a display" "$(st)" 404
kiosk GET "/api/v1/video/cameras/$YARD/export?from=$FROMR"; check "the download of a user refused" "$(st)" 401
kiosk GET "/api/v1/video/cameras/$YARD/evidence?from=$FROMR&reason=x"; check "the evidence package refused" "$(st)" 401
req GET "/api/v1/audit?action=video.playback&limit=5"; check "playback audited under the display's name" "$(jx 'JSON.stringify(j).includes("display:Control room")')" true
req GET "/api/v1/audit?action=display.update&limit=5"; check "switching on audited with the reason" "$(jx 'JSON.stringify(j).includes("e2e: watching at the wall")')" true
req PUT "/api/v1/displays/$DIDR" '{"allow_playback":false,"reason":"e2e: off again"}'; check "switched off" "$(st) $(jx j.allow_playback)" "200 false"
kiosk GET "/api/v1/display/cameras/$YARD/recordings?from=$FROMR"; check "refused again" "$(st)" 403
kiosk GET /api/v1/video/cameras; check "no session API with a display cookie" "$(st)" 401
kiosk GET /api/v1/displays; check "a display cannot list displays" "$(st)" 401

echo "== commands"
req POST "/api/v1/displays/$DID/command" "{\"action\":\"camera\",\"camera\":\"$ODD\",\"seconds\":60}"; check "operator sends a camera" "$(st) $(jx j.action)" "200 camera"
kiosk GET /api/v1/display/state; check "the display sees the command" "$(jx 'j.command.action + " " + j.command.camera')" "camera $ODD"
kiosk GET "/api/v1/display/cameras/$ODD/live"; [[ "$(st)" != 403 ]] && ok "the sent camera may be played" || bad "sent camera refused"
req POST "/api/v1/displays/$DID/command" '{"action":"identify"}'; check "identify" "$(st)" 200
req POST "/api/v1/displays/$DID/command" '{"action":"format-disk"}'; check "unknown action refused" "$(st)" 400
req POST "/api/v1/displays/$DID/command" '{"action":"camera","camera":"00000000-0000-7000-8000-000000000000"}'; check "unknown camera refused" "$(st)" 404
req POST "/api/v1/displays/$DID/command" '{"action":"clear"}'
kiosk GET "/api/v1/display/cameras/$ODD/live"; check "after clear the camera is refused again" "$(st)" 403
req PUT "/api/v1/displays/$DID" "{\"name\":\"Renamed $SUF\",\"reason\":\"e2e\"}"; check "renamed" "$(st) $(jx j.name)" "200 Renamed $SUF"
req PUT "/api/v1/displays/$DID" "{\"location\":\"Hall $SUF\",\"reason\":\"e2e\"}"; check "location set" "$(jx j.location)" "Hall $SUF"
req POST /api/v1/displays/command "{\"location\":\"Hall $SUF\",\"action\":\"message\",\"text\":\"<b>Meeting</b> at 10:00\",\"level\":\"warning\",\"seconds\":600}"; check "message to all displays of a location" "$(st) $(jx j.displays)" "200 1"
kiosk GET /api/v1/display/state; check "the display shows the message as text" "$(jx 'j.message.level + " " + j.message.text')" "warning <b>Meeting</b> at 10:00"
req POST /api/v1/displays/command '{"all":true,"action":"message","text":"x","level":"panic"}'; check "message level validated" "$(st)" 400
req POST /api/v1/displays/command '{"all":true,"action":"message","text":"  "}'; check "empty message refused" "$(st)" 400
req POST /api/v1/displays/command '{"action":"identify"}'; check "a target is required" "$(st)" 400
req POST /api/v1/displays/command '{"location":"nowhere-'$SUF'","action":"identify"}'; check "no display in the location" "$(st)" 404
req POST /api/v1/displays/command "{\"ids\":[\"$DID\"],\"action\":\"clear\"}"; check "clear by id" "$(st) $(jx j.displays)" "200 1"
kiosk GET /api/v1/display/state; check "clear ends the message" "$(jx '!j.message')" true
req GET /api/v1/displays; check "listed as showing" "$(jx "j.find(d => d.id === '$DID').online")" true

echo "== camera groups"
U=$(req POST /api/v1/users "{\"username\":\"gate_$SUF\",\"full_name\":\"Gate keeper\",\"email\":\"gate$SUF@example.com\",\"roles\":[\"operator\"],\"reason\":\"e2e\"}"; cat "$TMP/b")
UID2=$(echo "$U" | node -pe 'JSON.parse(require("fs").readFileSync(0)).user.id'); PW=$(echo "$U" | node -pe 'JSON.parse(require("fs").readFileSync(0)).initial_password')
req PUT "/api/v1/users/$UID2/camera-groups" "{\"limited\":true,\"groups\":[\"gate $SUF\"],\"reason\":\"e2e\"}"; check "user limited to one group" "$(st) $(jx 'j.groups.join()')" "200 gate $SUF"
req2 POST /api/v1/auth/login "{\"username\":\"gate_$SUF\",\"password\":\"$PW\"}"
req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
NEWPW="Gate-Keeper-Passw0rd-$SUF"
req2 POST /api/v1/auth/password "{\"current\":\"$PW\",\"new\":\"$NEWPW\"}"
req2 POST /api/v1/auth/login "{\"username\":\"gate_$SUF\",\"password\":\"$NEWPW\"}"; check "limited user signs in" "$(st)" 200
req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
req2 GET /api/v1/video/cameras; check "sees only the cameras of the group" "$(jx "j.filter(c => c.name.endsWith(' $SUF')).map(c => c.id).join()")" "$GATE"
req2 GET "/api/v1/video/cameras/$YARD"; check "another camera is not found" "$(st)" 404
req2 GET "/api/v1/video/cameras/$YARD/live"; check "live of another camera refused" "$(st)" 404
req2 POST "/api/v1/video/cameras/$YARD/events" '{"label":"x"}'; check "marking another camera refused" "$(st)" 404
req2 GET /api/v1/video/camera-groups; check "only its group listed" "$(jx "j.filter(g => g.group.endsWith('$SUF')).map(g => g.group).join()")" "gate $SUF"
req2 POST "/api/v1/displays/$DID/command" "{\"action\":\"camera\",\"camera\":\"$YARD\"}"; check "operator cannot send a camera outside the groups" "$(st)" 404
req2 POST "/api/v1/displays/$DID/command" "{\"action\":\"camera\",\"camera\":\"$GATE\"}"; check "operator sends a camera of the group" "$(st)" 200
req2 POST /api/v1/displays/pair "{\"code\":\"ABCD-EFGH\",\"name\":\"x\",\"content\":$CONTENT,\"reason\":\"e2e\"}"; check "operator cannot pair displays" "$(st)" 403
# a limited engineer (may manage displays, walls, dashboards, users of their own) cannot route other cameras anywhere
J3="$TMP/jar3"; CSRF3=""; req3() { call "$J3" "$CSRF3" "$@"; }
U3=$(req POST /api/v1/users "{\"username\":\"eng_$SUF\",\"full_name\":\"Limited engineer\",\"email\":\"eng$SUF@example.com\",\"roles\":[\"engineer\",\"org_admin\"],\"reason\":\"e2e\"}"; cat "$TMP/b")
UID3=$(echo "$U3" | node -pe 'JSON.parse(require("fs").readFileSync(0)).user.id'); PW3=$(echo "$U3" | node -pe 'JSON.parse(require("fs").readFileSync(0)).initial_password')
req PUT "/api/v1/users/$UID3/camera-groups" "{\"limited\":true,\"groups\":[\"gate $SUF\"],\"reason\":\"e2e\"}"
req3 POST /api/v1/auth/login "{\"username\":\"eng_$SUF\",\"password\":\"$PW3\"}"; req3 GET /api/v1/auth/me; CSRF3=$(jx j.csrf_token)
req3 POST /api/v1/auth/password "{\"current\":\"$PW3\",\"new\":\"Limited-Engineer-Passw0rd-$SUF\"}"
req3 POST /api/v1/auth/login "{\"username\":\"eng_$SUF\",\"password\":\"Limited-Engineer-Passw0rd-$SUF\"}"; req3 GET /api/v1/auth/me; CSRF3=$(jx j.csrf_token)
req3 POST /api/v1/video/walls "{\"name\":\"Sneak $SUF\",\"reason\":\"e2e\",\"screens\":[{\"cols\":1,\"rows\":1,\"tiles\":[\"$YARD\"]}]}"; check "limited: wall with another group's camera refused" "$(st)" 400
D3="${DW/CAMID/$YARD}"; req3 POST /api/v1/dashboards "${D3/Kiosk /Sneak }"; check "limited: camera widget of another group refused" "$(st)" 403
D4="${DP/PINID/$YARD}"; req3 POST /api/v1/dashboards "${D4/Plan /Sneak plan }"; check "limited: camera on a plan of another group refused" "$(st)" 403
req3 POST /api/v1/displays/pair "{\"code\":\"ABCD-EFGH\",\"name\":\"x\",\"content\":{\"items\":[{\"kind\":\"wall\",\"wall\":\"$WALL\",\"screen\":0}],\"popup\":\"none\"},\"reason\":\"e2e\"}"; check "limited: display with another group's wall refused" "$(st) $(jx 'j.error.includes("outside your camera groups")')" "400 true"
req3 POST /api/v1/displays/pair "{\"code\":\"ABCD-EFGH\",\"name\":\"x\",\"content\":{\"items\":[{\"kind\":\"dashboard\",\"slug\":\"$OTHER\"}],\"popup\":\"all\"},\"reason\":\"e2e\"}"; check "limited: popups of all cameras refused" "$(st)" 400
req3 GET /api/v1/video/storage; check "limited: storage lists only own cameras" "$(jx "j.cameras.filter(c => c.name.endsWith(' $SUF')).map(c => c.id).join()")" "$GATE"
req3 POST /api/v1/users "{\"username\":\"sub_$SUF\",\"full_name\":\"Sub\",\"email\":\"sub$SUF@example.com\",\"roles\":[\"viewer\"],\"reason\":\"e2e\"}"; SUB=$(jx j.user.id)
req GET "/api/v1/users/$SUB/camera-groups"; check "a user created by a limited admin inherits the limit" "$(jx 'j.limited + " " + j.groups.join()')" "true gate $SUF"
for u in $UID3 $SUB; do req PATCH "/api/v1/users/$u" '{"disabled":true,"reason":"e2e cleanup"}'; done
req PUT "/api/v1/users/$UID2/camera-groups" '{"limited":false,"reason":"e2e"}'; req2 GET /api/v1/video/cameras
check "unlimited again: all cameras at once" "$(jx "j.filter(c => c.name.endsWith(' $SUF')).length")" 4
req PATCH "/api/v1/users/$UID2" '{"disabled":true,"reason":"e2e cleanup"}'

echo "== revoke"
req DELETE "/api/v1/displays/$DID"; check "revoke needs a reason" "$(st)" 400
req DELETE "/api/v1/displays/$DID?reason=e2e"; check "display disconnected" "$(st)" 200
kiosk GET /api/v1/display/state; check "the kiosk is locked out at once" "$(st)" 401
kiosk GET "/api/v1/public/~$SLUG"; check "and reads nothing" "$(st)" 404
kiosk POST /api/v1/display/hello '{}'; check "and may pair again" "$(jx '!!j.code')" true
req GET "/api/v1/export/audit?format=csv"; check "pair, command, revoke and camera groups audited" "$(for a in display.pair display.command display.revoke user.camera_groups; do grep -c "$a" "$TMP/b" | awk '{print ($1>0)}'; done | paste -sd,)" "1,1,1,1"

echo "== health incidents"
req GET /api/v1/video/settings; OLD=$(node -pe 'JSON.stringify(JSON.parse(require("fs").readFileSync(process.argv[1])).settings)' "$TMP/b")
req PUT /api/v1/video/settings '{"camera_offline_minutes":1,"display_offline_minutes":1,"reason":"e2e"}'; check "monitoring set to one minute" "$(st)" 200
DOWN=$(mk Down "" continuous true)
# a second kiosk is paired and then switched off (it never reports again)
K2="$TMP/kiosk2"; call "$K2" "" POST /api/v1/display/hello '{}'; C2=$(jx j.code)
req POST /api/v1/displays/pair "{\"code\":\"$C2\",\"name\":\"Dark $SUF\",\"content\":$CONTENT,\"reason\":\"e2e\"}"; D2=$(jx j.id)
call "$K2" "" POST /api/v1/display/hello '{"code_known":true}'; check "second display paired" "$(jx j.paired)" true
if waitfor '(req GET "/api/v1/incidents?status=open"; [[ "$(jx "j.some(i => i.kind === \"camera\" && i.title.includes(\"Down $SUF\"))")" == true ]])' 200; then ok "incident for a recording camera that is offline"; else bad "no camera incident: $(head -c 300 "$TMP/b")"; fi
if waitfor '(req GET "/api/v1/incidents?status=open"; [[ "$(jx "j.some(i => i.kind === \"display\" && i.title.includes(\"Dark $SUF\"))")" == true ]])' 120; then ok "incident for a display that stopped showing"; else bad "no display incident"; fi
req DELETE "/api/v1/displays/$D2?reason=e2e"
req DELETE "/api/v1/video/cameras/$DOWN?reason=e2e"
if waitfor '(req GET "/api/v1/incidents?status=open"; [[ "$(jx "j.some(i => i.kind === \"camera\" && i.title.includes(\"Down $SUF\"))")" == false ]])' 130; then ok "resolved when the camera is gone"; else bad "camera incident not resolved"; fi
req PUT /api/v1/video/settings "$(node -e 'const o=JSON.parse(process.argv[1]); o.reason="e2e restore"; console.log(JSON.stringify(o))' "$OLD")"; check "settings restored" "$(st)" 200

echo "== cleanup"
req DELETE "/api/v1/video/walls/$WALL?reason=e2e"
for d in $SLUG $OTHER; do req DELETE "/api/v1/dashboards/$d?reason=e2e"; done
for c in $GATE $YARD $ODD $PIN; do req DELETE "/api/v1/video/cameras/$c?reason=e2e"; done; check "cleaned up" "$(st)" 200
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

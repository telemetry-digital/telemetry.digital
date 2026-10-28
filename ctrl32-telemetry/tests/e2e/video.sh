#!/usr/bin/env bash
# End-to-end checks for spec 010: cameras (RTSP H.264 and MJPEG from the camera simulator), stream test, live video over
# a WebSocket, continuous recording, playback, MP4 export, audit of playback, video walls and validation; the camera
# limit without a license (spec 026: up to 4 enabled cameras, more refused with 402, existing ones keep working). The
# suite runs many cameras at once, so it installs a test license (tests/keys) unless one is installed, and removes it.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/video.sh
set -u
cd "$(dirname "$0")/../.."
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d)
SIM_PID=""
RELAY=""
trap '[[ -n "$SIM_PID" ]] && kill $SIM_PID 2>/dev/null; [[ -n "$RELAY" ]] && kill $RELAY 2>/dev/null; rm -rf "$TMP"' EXIT
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
waitfor() { local n=${2:-40}; for i in $(seq "$n"); do eval "$1" && return 0; sleep 0.5; done; return 1; }
iso() { date -u -d "$1" +%Y-%m-%dT%H:%M:%SZ; }
SUF=$RANDOM

# ws PATH SECONDS: a minimal WebSocket client (Node's built-in one cannot send the session cookie); prints a summary
cat > "$TMP/ws.js" <<'EOF'
const [url, jar, secs] = process.argv.slice(2);
const fs = require('fs'), http = require('http'), crypto = require('crypto');
const cookie = fs.readFileSync(jar, 'utf8').split(/\r?\n/).map(l => l.replace('#HttpOnly_', '')).filter(l => l && !l.startsWith('#')).map(l => l.split('\t')).filter(p => p.length >= 7).map(p => p[5] + '=' + p[6]).join('; ');
const u = new URL(url);
const out = { init: null, types: {}, bin: 0, bytes: 0, boxes: [], jpeg: 0, times: [] };
const req = http.request({ host: u.hostname, port: u.port, path: u.pathname + u.search, headers: { Cookie: cookie, Connection: 'Upgrade', Upgrade: 'websocket', 'Sec-WebSocket-Version': '13', 'Sec-WebSocket-Key': crypto.randomBytes(16).toString('base64') } });
req.on('response', r => { out.status = r.statusCode; });
req.on('upgrade', (res, sock, head) => {
  out.status = 101; let buf = head;
  sock.on('data', d => { buf = Buffer.concat([buf, d]); for (;;) {
    if (buf.length < 2) return;
    const op = buf[0] & 15; let len = buf[1] & 127, off = 2;
    if (len === 126) { if (buf.length < 4) return; len = buf.readUInt16BE(2); off = 4; } else if (len === 127) { if (buf.length < 10) return; len = Number(buf.readBigUInt64BE(2)); off = 10; }
    if (buf.length < off + len) return;
    const p = buf.subarray(off, off + len); buf = buf.subarray(off + len);
    if (op === 1) { const j = JSON.parse(p.toString()); out.types[j.type] = (out.types[j.type] || 0) + 1; if (j.type === 'init' && !out.init) out.init = j; if (j.type === 'audio') out.audio = j; if (j.type === 'stats') out.stats = j; if (j.type === 'time') out.times.push(j.t); }
    else if (op === 2) { if (p[0] === 0x50) { out.pbin = (out.pbin || 0) + 1; continue; } if (p[0] === 0x41) { out.abin = (out.abin || 0) + 1; if (!out.abox) out.abox = p.subarray(5, 9).toString('latin1'); continue; } out.bin++; out.bytes += p.length; if (p[0] === 0xff && p[1] === 0xd8) out.jpeg++; else if (out.boxes.length < 3) out.boxes.push(p.subarray(4, 8).toString('latin1')); }
  } });
});
req.end();
setTimeout(() => { out.times = out.times.length ? [out.times[0], out.times[out.times.length - 1]] : []; console.log(JSON.stringify(out)); process.exit(0); }, secs * 1000);
EOF
ws() { node "$TMP/ws.js" "${BASE/http/ws}$1" "$JAR" "$2" > "$TMP/b"; }

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
check "admin has the video permissions" "$(jx '["video.view","video.playback","video.manage"].every(p => j.permissions.includes(p))')" true
go build -o "$TMP/camsim" ./tools/camsim || { echo "camsim build failed"; exit 1; }
go build -o "$TMP/licgen" ./tools/licgen || { echo "licgen build failed"; exit 1; }
LIC_OURS=""
req GET /api/v1/server/license
if [[ "$(jx j.valid)" != true ]]; then
  req PUT /api/v1/server/license "{\"token\":\"$("$TMP/licgen" issue -key tests/keys/license-test.key -licensee "E2E video $SUF" 2>/dev/null)\",\"reason\":\"e2e video\"}"
  check "test license installed (many cameras at once)" "$(st)" 200; LIC_OURS=1
fi
[[ -n "${BIN:-}" ]] || go build -o "$TMP/c32" ./cmd/ctrl32-telemetry || { echo "build failed"; exit 1; }
RP=$((20000 + RANDOM % 20000))
HP=$((RP + 1)); DP=$((RP + 2))
"$TMP/camsim" -rtsp 127.0.0.1:$RP -http 127.0.0.1:$HP -fps 10 -onvif ongate -onvif-user admin -onvif-pass onvifpw -onvif-codec h264 -discovery 127.0.0.1:$DP > "$TMP/camsim.log" 2>&1 &
SIM_PID=$!
sleep 1
SIM=rtsp://127.0.0.1:$RP

echo "== stream test"
req POST /api/v1/video/cameras/test "{\"url\":\"$SIM/gate/h264/160x96\"}"
check "H.264 stream recognized" "$(jx 'j.state + " " + j.codec + " " + j.width + "x" + j.height')" "online h264 160x96"
req POST /api/v1/video/cameras/test "{\"url\":\"$SIM/gate/mjpeg/320x240\",\"transport\":\"tcp\"}"
check "MJPEG stream recognized" "$(jx 'j.codec + " " + j.width')" "mjpeg 320"
req POST /api/v1/video/cameras/test "{\"url\":\"rtsp://admin:topsecret@127.0.0.1:1/x\"}"
check "unreachable camera: offline, password not in the error" "$(jx 'j.state + " " + (j.error.length > 0) + " " + j.error.includes("topsecret")')" "offline true false"

echo "== cameras"
req POST /api/v1/video/cameras "{\"name\":\"Gate $SUF\",\"location\":\"North\",\"url\":\"rtsp://viewer:pw$SUF@127.0.0.1:$RP/gate/h264/160x96\",\"sub_url\":\"$SIM/gate/mjpeg/160x96\",\"retention_days\":3,\"reason\":\"e2e\"}"
check "camera created" "$(st)" 201
CAM=$(jx j.id)
check "credentials moved out of the URL" "$(jx 'j.url + " " + j.username + " " + j.has_password')" "$SIM/gate/h264/160x96 viewer true"
req POST /api/v1/video/cameras "{\"name\":\"x\",\"url\":\"ftp://a/b\",\"reason\":\"e2e\"}"; check "bad scheme refused" "$(st)" 400
req POST /api/v1/video/cameras "{\"name\":\"x\",\"url\":\"$SIM/a\"}"; check "reason required" "$(st)" 400
req POST /api/v1/video/cameras "{\"name\":\"x\",\"url\":\"$SIM/a\",\"retention_days\":0,\"reason\":\"e2e\"}"; check "retention validated" "$(st)" 400
req POST /api/v1/video/cameras "{\"name\":\"Door $SUF\",\"url\":\"$SIM/door/mjpeg/160x96\",\"record\":\"off\",\"reason\":\"e2e\"}"; DOOR=$(jx j.id)
waitfor '(req GET /api/v1/video/cameras/$CAM; [[ "$(jx j.status.state)" == online ]])' 30 && ok "recording camera connects" || bad "status $(cat "$TMP/b")"
check "recording" "$(jx 'j.status.recording + " " + j.status.codec')" "true h264"
req GET /api/v1/video/cameras/$DOOR; check "on-demand camera idle without viewers" "$(jx j.status.state)" idle

echo "== live"
ws "/api/v1/video/cameras/$CAM/live?quality=main" 3
check "H.264 live: init, MP4 initialization, fragments" "$(jx 'j.status + " " + j.init.codec + " " + j.init.mime + " " + j.boxes.join(",")')" '101 h264 video/mp4; codecs="avc1.42c01e" ftyp,moof,moof'
check "frames flow" "$(jx 'j.bin > 15 && j.types.time > 10')" true
ws "/api/v1/video/cameras/$CAM/live" 2
check "tiles use the MJPEG sub-stream" "$(jx 'j.init.codec + " " + (j.jpeg > 5)')" "mjpeg true"
req GET "/api/v1/video/cameras/$DOOR/snapshot"; check "snapshot of an MJPEG camera" "$(st) $(head -c 2 "$TMP/b" | od -An -tx1 | tr -d ' ')" "200 ffd8"
req GET "/api/v1/video/cameras/$CAM/snapshot?quality=main"; check "no snapshot of H.264" "$(st)" 404
curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/video/cameras/$CAM/live" > "$TMP/s"; check "live needs a session" "$(st)" 401

echo "== recordings"
FROM=$(iso '-10 min')
waitfor '(req GET "/api/v1/video/cameras/$CAM/recordings?from=$FROM"; [[ "$(jx "j.spans.length > 0 && j.bytes > 0 && new Date(j.spans[0].to) - new Date(j.spans[0].from) > 5000")" == true ]])' 40 \
  && ok "recorded span listed" || bad "recordings: $(cat "$TMP/b")"
REC_FROM=$(jx j.spans[0].from)
req GET "/api/v1/video/cameras/$CAM/recordings?from=$FROM&to=$(iso '+40 days')"; check "range limited" "$(st)" 400
ws "/api/v1/video/cameras/$CAM/playback?from=$REC_FROM&speed=4" 3
check "playback: init and fragments" "$(jx 'j.status + " " + j.init.codec + " " + j.boxes[0]')" "101 h264 ftyp"
check "playback runs faster than real time" "$(jx 'j.times.length == 2 && j.times[1] - j.times[0] > 4000')" true
check "playback starts at the requested time" "$(jx "Math.abs(j.times[0] - Date.parse('$REC_FROM')) < 2000")" true
TO=$(node -e "console.log(new Date(Date.parse('$REC_FROM') + 4000).toISOString())")
curl -s -b "$JAR" -o "$TMP/clip.mp4" -D "$TMP/h" -w '%{http_code}' "$BASE/api/v1/video/cameras/$CAM/export?from=$REC_FROM&to=$TO" > "$TMP/s"
check "export" "$(st) $(grep -i '^content-type' "$TMP/h" | tr -d '\r' | cut -d' ' -f2)" "200 video/mp4"
check "export is an MP4 with fragments" "$(node -e 'const b=require("fs").readFileSync(process.argv[1]); const t=[]; for (let o=0; o<b.length;) { const n=b.readUInt32BE(o); t.push(b.toString("latin1",o+4,o+8)); if(n<8) break; o+=n; } console.log(t.slice(0,3).join(",") + " " + (t.filter(x=>x==="moof").length >= 2))' "$TMP/clip.mp4")" "ftyp,moov,moof true"
req GET "/api/v1/video/cameras/$CAM/export?from=$FROM&to=$(iso '+1 hour')"; check "export limited to one hour" "$(st)" 400
req GET "/api/v1/export/audit?format=csv"
check "playback and export audited" "$(grep -c "video.playback\|video.export" "$TMP/b" | awk '{print ($1 >= 2)}')" 1

echo "== timeshift: playback follows the recording while it goes on"
req GET "/api/v1/video/cameras/$CAM/recordings?from=$FROM"; B0=$(jx j.bytes)
ws "/api/v1/video/cameras/$CAM/playback?from=$(iso '-4 sec')" 8
check "playback runs on past the moment it was started (no end)" "$(jx 'j.times.length == 2 && j.times[1] - j.times[0] >= 5000 && !j.types.end')" true
check "playback reaches the present" "$(jx 'Date.now() - j.times[1] < 6000')" true
waitfor '(req GET "/api/v1/video/cameras/$CAM/recordings?from=$FROM"; [[ "$(jx "j.bytes > $B0")" == true ]])' 30 && ok "recording went on during playback" || bad "recording stalled: $(cat "$TMP/b")"

echo "== camera widget"
DW="{\"name\":\"Cams $SUF\",\"reason\":\"e2e\",\"spec\":{\"version\":1,\"kind\":\"dashboard\",\"layout\":{\"type\":\"grid\",\"row_height\":96},\"range\":\"6h\",\"refresh_s\":30,\"widgets\":[{\"id\":\"c\",\"type\":\"camera\",\"geometry\":{\"x\":0,\"y\":0,\"w\":4,\"h\":3},\"bindings\":{},\"params\":{\"camera\":\"CAMID\"}}]}}"
req POST /api/v1/dashboards "${DW/CAMID/$CAM}"; check "dashboard with a camera widget" "$(st)" 201
DSLUG=$(jx j.slug)
req GET "/api/v1/dashboards/$DSLUG"; check "camera widget stored with defaults" "$(jx 'JSON.stringify(j.spec.widgets[0].params)')" "{\"camera\":\"$CAM\",\"quality\":\"sub\",\"show_name\":true,\"show_stats\":false,\"open_on_click\":true}"
req POST /api/v1/dashboards "${DW/CAMID/not-a-camera}"; check "camera id validated" "$(st)" 400
req DELETE "/api/v1/dashboards/$DSLUG?reason=e2e"

echo "== remote camera control through the relay, G.711 sound, stream information (0.58.0)"
ONVR=http://127.0.0.1:$HP/onvif/rptz/device_service
req POST /api/v1/video/cameras "{\"name\":\"x\",\"source\":\"push\",\"onvif_url\":\"http://10.0.0.1/cgi-bin/x\",\"reason\":\"e2e\"}"; check "a relayed camera's ONVIF address must be under /onvif/" "$(st)" 400
req POST /api/v1/video/cameras "{\"name\":\"Remote PTZ $SUF\",\"source\":\"push\",\"onvif_url\":\"$ONVR\",\"username\":\"admin\",\"password\":\"onvifpw\",\"record\":\"off\",\"audio\":true,\"audio_record\":false,\"reason\":\"e2e\"}"
check "relayed camera with an ONVIF address and account" "$(st) $(jx 'j.onvif + " " + j.username + " " + j.has_password + " " + j.onvif_url')" "201 true admin true $ONVR"
RPTZ=$(jx j.id); RSEC=$(jx j.push_secret); RPURL=$(jx j.push.url); PUSHON=$(jx j.push.enabled)
check "relay configuration names the web address and the camera's ONVIF address" "$(jx "j.relay_config.includes('web = \"') && j.relay_config.includes('onvif = \"$ONVR\"')")" true
req GET /api/v1/video/cameras/$RPTZ; check "camera control not connected before the relay polls" "$(jx 'j.status.onvif_relay')" false
req POST "/api/v1/video/cameras/$RPTZ/ptz" '{"action":"stop"}'; check "PTZ without the relay: a clear conflict" "$(st) $(jx 'j.error.includes("relay")')" "409 true"
rcurl() { curl -s -o "$TMP/b" -w '%{http_code}' "$@" > "$TMP/s"; }
rcurl -H "X-Camera-Secret: $RSEC" "$BASE/api/v1/video/relay/$RPTZ/onvif/next?wait=0"; check "control endpoint accepts the camera's secret" "$(st)" 204
rcurl -X POST -H "Content-Type: application/json" -H "X-Camera-Secret: $RSEC" --data '{"status":200,"body":""}' "$BASE/api/v1/video/relay/$RPTZ/onvif/unknown"; check "an answer to no request is gone" "$(st)" 410
if [[ "$PUSHON" == true ]]; then
  printf 'server = "%s"\nweb = "%s"\n[[camera]]\nid = "%s"\nsecret = "%s"\nsource = "%s"\ntransport = "tcp"\naudio = true\nonvif = "%s"\n' "${RPURL%/*}" "$BASE" "$RPTZ" "$RSEC" "$SIM/rptz/h264/160x96/g711" "$ONVR" > "$TMP/rptz.toml"
  chmod 600 "$TMP/rptz.toml"
  "${BIN:-$TMP/c32}" camera-relay -check -config "$TMP/rptz.toml" > "$TMP/check3.log" 2>&1
  check "self-check: camera, G.711 sound, secret, local ONVIF, camera control" "$? $(grep -c '^ok ' "$TMP/check3.log") $(grep -c '^ok .*sound: G.711 µ-law 8000 Hz' "$TMP/check3.log") $(grep -c '^ok .*local ONVIF' "$TMP/check3.log") $(grep -c '^ok .*camera control through the server' "$TMP/check3.log")" "0 7 1 1 1"
  grep -q "$RSEC" "$TMP/check3.log" && bad "secret in the self-check output" || ok "secret not in the self-check output"
  "${BIN:-$TMP/c32}" camera-relay -config "$TMP/rptz.toml" > "$TMP/relay3.log" 2>&1 &
  RELAY=$!
  waitfor '(req GET /api/v1/video/cameras/$RPTZ; [[ "$(jx "j.status.state + j.status.onvif_relay")" == onlinetrue ]])' 40 && ok "relay forwards the video and polls for camera control" || bad "status $(cat "$TMP/b"); relay: $(tail -3 "$TMP/relay3.log")"
  req GET "/api/v1/video/cameras/$RPTZ/ptz/presets"; check "PTZ presets through the relay" "$(st) $(jx 'j.map(p => p.name).join()')" "200 Home,Gate,Parking"
  req POST "/api/v1/video/cameras/$RPTZ/ptz" '{"action":"move","pan":0.5,"tilt":0,"zoom":0}'; check "PTZ move through the relay" "$(st)" 200
  req POST "/api/v1/video/cameras/$RPTZ/ptz" '{"action":"stop"}'; check "PTZ stop through the relay" "$(st)" 200
  req POST "/api/v1/video/cameras/$RPTZ/ptz" '{"action":"preset","preset":"2"}'; check "PTZ preset through the relay" "$(st)" 200
  check "the camera turned to the preset" "$(curl -s "http://127.0.0.1:$HP/sim/ptz/rptz" | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>console.log(JSON.parse(s).pan))')" "-0.6"
  req POST /api/v1/video/onvif/profiles "{\"url\":\"$ONVR\",\"username\":\"admin\",\"id\":\"$RPTZ\"}"; check "ONVIF profiles of a relayed camera through the relay" "$(st) $(jx 'j.profiles.length + " " + j.ptz')" "200 2 true"
  waitfor '(req GET /api/v1/video/cameras/$RPTZ; [[ "$(jx j.status.events)" == connected ]])' 30 && ok "ONVIF events through the relay" || bad "events $(jx j.status.events)"
  curl -s -X POST "http://127.0.0.1:$HP/sim/motion/rptz?on=1"
  waitfor '(req GET /api/v1/video/cameras/$RPTZ; [[ "$(jx "(j.status.active || []).join()")" == motion ]])' 20 && ok "motion of a relayed camera arrives" || bad "status $(cat "$TMP/b")"
  curl -s -X POST "http://127.0.0.1:$HP/sim/motion/rptz?on=0"
  waitfor '(req GET /api/v1/video/cameras/$RPTZ; [[ "$(jx j.status.audio)" == "G.711 8000 Hz (as PCM)" ]])' 20 && ok "the relay forwards G.711 sound" || bad "sound status: $(jx j.status.audio)"
  ws "/api/v1/video/cameras/$RPTZ/live?quality=main&audio=1" 4; check "relayed G.711 sound plays live (PCM)" "$(jx '(j.audio && j.audio.codec) + " " + (j.pbin > 5)')" "pcm true"
  check "the player gets the stream information" "$(jx 'j.stats && (j.stats.stream + " " + j.stats.codec + " " + j.stats.width + "x" + j.stats.height + " " + (j.stats.fps > 0) + " " + (j.stats.kbps > 0))')" "main h264 160x96 true true"
  req GET /api/v1/video/cameras/$RPTZ; check "status shows what arrives" "$(jx 'j.status.codec + " " + (j.status.fps > 0) + " " + (j.status.kbps > 0)')" "h264 true true"
  grep -q "onvifpw" "$TMP/relay3.log" && bad "ONVIF password in the relay log" || ok "no ONVIF request body in the relay log"
  kill $RELAY 2>/dev/null; RELAY=""
  waitfor '(req GET /api/v1/video/cameras/$RPTZ; [[ "$(jx j.status.onvif_relay)" == false ]])' 20 && ok "a stopped relay shows as not connected" || bad "still connected: $(jx j.status.onvif_relay)"
  req POST "/api/v1/video/cameras/$RPTZ/ptz" '{"action":"stop"}'; check "PTZ after the relay stopped: a clear conflict" "$(st) $(jx 'j.error.includes("relay")')" "409 true"
fi
# wrong secrets last: they count towards blocking this address (as the relay checks below do)
rcurl -H "X-Camera-Secret: wrong-secret-0123456789" "$BASE/api/v1/video/relay/$RPTZ/onvif/next?wait=0"; check "control endpoint refuses a wrong secret" "$(st)" 401
rcurl "$BASE/api/v1/video/relay/$RPTZ/onvif/next?wait=0"; check "and a missing one" "$(st)" 401
req DELETE "/api/v1/video/cameras/$RPTZ?reason=e2e"

echo "== remote camera (relay sends the video)"
req POST /api/v1/video/cameras "{\"name\":\"Remote $SUF\",\"source\":\"push\",\"url\":\"rtsp://ignored/x\",\"reason\":\"e2e\"}"
check "push camera created" "$(st) $(jx 'j.source + " " + j.url')" "201 push "
PUSH=$(jx j.id); SECRET=$(jx j.push_secret); PUSHON=$(jx j.push.enabled)
check "secret shown once with a relay configuration" "$(jx "j.push_secret.length >= 30 && j.relay_config.includes('$PUSH') && j.relay_config.includes(j.push_secret)")" true
req GET /api/v1/video/cameras/$PUSH; check "secret never shown again" "$(jx 'j.push_secret === undefined && j.relay_config === undefined && JSON.stringify(j).includes("secret") === false')" true
if [[ "$PUSHON" == true ]]; then
  PURL=$(jx j.push.url)
  printf 'server = "%s"\n[[camera]]\nid = "%s"\nsecret = "%s"\nsource = "%s"\nsub_source = "%s"\ntransport = "tcp"\n' "${PURL%/*}" "$PUSH" "$SECRET" "$SIM/remote/h264/160x96" "$SIM/remote/mjpeg/160x96" > "$TMP/relay.toml"
  chmod 600 "$TMP/relay.toml"
  "${BIN:-$TMP/c32}" camera-relay -check -config "$TMP/relay.toml" > "$TMP/check.log" 2>&1; check "relay self-check passes (server, cameras, secret)" "$? $(grep -c '^ok ' "$TMP/check.log") $(grep -c 'the relay is ready' "$TMP/check.log")" "0 5 1"
  "${BIN:-$TMP/c32}" camera-relay -config "$TMP/relay.toml" > "$TMP/relay.log" 2>&1 &
  RELAY=$!
  waitfor '(req GET /api/v1/video/cameras/$PUSH; [[ "$(jx j.status.state)" == online ]])' 30 && ok "relay connected, camera online" || bad "push status $(cat "$TMP/b"); relay: $(tail -3 "$TMP/relay.log")"
  check "remote camera records" "$(jx 'j.status.recording + " " + j.status.codec')" "true h264"
  ws "/api/v1/video/cameras/$PUSH/live" 2; check "tiles use the pushed sub-stream" "$(jx 'j.init && j.init.codec')" mjpeg
  printf 'server = "%s"\n[[camera]]\nid = "%s"\nsecret = "%s"\nsource = "%s"\n' "${PURL%/*}" "$PUSH" "wrong-secret-wrong-secret-123" "$SIM/remote/h264/160x96" > "$TMP/bad.toml"; chmod 600 "$TMP/bad.toml"
  "${BIN:-$TMP/c32}" camera-relay -check -config "$TMP/bad.toml" > "$TMP/check2.log" 2>&1; check "self-check reports a wrong secret" "$? $(grep -c '^FAIL .*server accepts the secret' "$TMP/check2.log") $(grep -c 'wrong-secret-wrong-secret-123' "$TMP/check2.log")" "1 1 0"
  "${BIN:-$TMP/c32}" camera-relay -config "$TMP/bad.toml" > "$TMP/bad.log" 2>&1 &
  BADR=$!; sleep 3; kill $BADR 2>/dev/null
  check "wrong secret refused" "$(grep -c 'relay forwarding' "$TMP/bad.log")" 0
  grep -q "wrong-secret-wrong-secret-123" "$TMP/bad.log" && bad "secret written to the relay log" || ok "secret not in the relay log"
  req POST /api/v1/video/cameras/$PUSH/push-secret '{"reason":"e2e rotate"}'; check "secret rotated" "$(st) $(jx "j.push_secret !== '$SECRET'")" "200 true"
  sleep 4; req GET /api/v1/video/cameras/$PUSH; check "relay with the old secret is cut off" "$(jx j.status.state)" offline
  kill $RELAY 2>/dev/null
else
  echo "  (video.push_listen not configured: relay checks skipped)"
fi
req POST /api/v1/video/cameras/$CAM/push-secret '{"reason":"e2e"}'; check "no secret for a pulled camera" "$(st)" 409
req DELETE "/api/v1/video/cameras/$PUSH?reason=e2e"

echo "== ONVIF: discovery, profiles, events, event recording, PTZ (spec 013)"
ONV=http://127.0.0.1:$HP/onvif/ongate/device_service
req POST /api/v1/video/discover "{\"hosts\":[\"127.0.0.1:$DP\"]}"; check "camera found by WS-Discovery" "$(jx "j.some(d => d.address === '$ONV' && d.name === 'ongate')")" true
req POST /api/v1/video/discover '{"hosts":["example.com"]}'; check "discovery probes IP addresses only" "$(st)" 400
req POST /api/v1/video/onvif/profiles "{\"url\":\"$ONV\",\"username\":\"admin\",\"password\":\"wrong\"}"; check "wrong ONVIF password" "$(st)" 401
req POST /api/v1/video/onvif/profiles "{\"url\":\"$ONV\",\"username\":\"admin\",\"password\":\"onvifpw\"}"
check "profiles with stream addresses" "$(jx 'j.profiles.length + " " + j.profiles[0].uri + " " + j.events + " " + j.ptz')" "2 rtsp://127.0.0.1:$RP/ongate/h264/640x352 true true"
req POST /api/v1/video/cameras "{\"name\":\"x\",\"url\":\"$SIM/a\",\"onvif_url\":\"http://admin:pw@127.0.0.1/onvif\",\"reason\":\"e2e\"}"; check "credentials in the ONVIF address refused" "$(st)" 400
req POST /api/v1/video/cameras "{\"name\":\"Events $SUF\",\"url\":\"$SIM/ongate/h264/160x96\",\"username\":\"admin\",\"password\":\"onvifpw\",\"onvif_url\":\"$ONV\",\"record\":\"events\",\"event_pre_s\":3,\"event_post_s\":4,\"reason\":\"e2e\"}"
check "camera recording on events" "$(st) $(jx j.record)" "201 events"; EVC=$(jx j.id)
waitfor '(req GET /api/v1/video/cameras/$EVC; [[ "$(jx "j.status.events + j.status.state")" == connectedonline ]])' 30 && ok "ONVIF events connected" || bad "status $(cat "$TMP/b")"
check "not recording without an event" "$(jx 'j.status.recording + " " + j.status.record_mode')" "false events"
sleep 4
EF=$(iso '-2 min')
req GET "/api/v1/video/cameras/$EVC/recordings?from=$EF"; check "no files without an event" "$(jx j.files)" 0
curl -s -X POST "http://127.0.0.1:$HP/sim/motion/ongate?on=1"
waitfor '(req GET /api/v1/video/cameras/$EVC; [[ "$(jx "(j.status.active || []).join() + j.status.recording")" == motiontrue ]])' 20 && ok "motion reported, recording" || bad "status $(cat "$TMP/b")"
req GET "/api/v1/video/cameras/$EVC/events?from=$EF"; check "motion event open" "$(jx 'j.length + " " + j[0].kind + " " + j[0].source + " " + (j[0].ended_at === null)')" "1 motion camera true"
MSTART=$(jx j[0].started_at)
sleep 3
curl -s -X POST "http://127.0.0.1:$HP/sim/motion/ongate?on=0"
waitfor '(req GET "/api/v1/video/cameras/$EVC/events?from=$EF"; [[ "$(jx "j[0].ended_at !== null")" == true ]])' 20 && ok "motion event closed" || bad "events $(cat "$TMP/b")"
waitfor '(req GET /api/v1/video/cameras/$EVC; [[ "$(jx j.status.recording)" == false ]])' 20 && ok "recording stops after the post-roll" || bad "still recording"
sleep 11
req GET "/api/v1/video/cameras/$EVC/recordings?from=$EF"
check "recording starts before the event (pre-roll)" "$(jx "j.spans.length === 1 && Date.parse(j.spans[0].from) <= Date.parse('$MSTART') - 1500")" true
check "and ends after it (post-roll)" "$(jx "(Date.parse(j.spans[0].to) - Date.parse(j.spans[0].from)) >= 8000")" true
req POST /api/v1/video/cameras/$EVC/events '{"label":"door opened","seconds":5}'; check "manual mark" "$(st)" 201
req GET "/api/v1/video/cameras/$EVC/events?from=$EF&kind=mark"; check "mark listed" "$(jx 'j[0].source + " " + j[0].label')" "user door opened"
req POST /api/v1/video/cameras/$EVC/events '{"label":""}'; check "mark needs a label" "$(st)" 400
req GET "/api/v1/video/cameras/$EVC/ptz/presets"; check "PTZ presets" "$(jx 'j.map(p => p.name).join()')" "Home,Gate,Parking"
req POST "/api/v1/video/cameras/$EVC/ptz" '{"action":"move","pan":0.5,"tilt":0,"zoom":0}'; check "PTZ move" "$(st)" 200
req POST "/api/v1/video/cameras/$EVC/ptz" '{"action":"stop"}'; check "PTZ stop" "$(st)" 200
req POST "/api/v1/video/cameras/$EVC/ptz" '{"action":"move","pan":5}'; check "PTZ speed validated" "$(st)" 400
req POST "/api/v1/video/cameras/$EVC/ptz" '{"action":"preset","preset":"2"}'; check "PTZ preset" "$(st)" 200
req POST "/api/v1/video/cameras/$CAM/ptz" '{"action":"stop"}'; check "no PTZ without ONVIF" "$(st)" 409

echo "== camera events in flows"
FLOW="{\"nodes\":[{\"id\":\"c\",\"type\":\"camera_event\",\"x\":0,\"y\":0,\"settings\":{\"camera\":\"$EVC\",\"kinds\":[\"motion\"]}},{\"id\":\"m\",\"type\":\"camera_mark\",\"x\":200,\"y\":0,\"settings\":{\"camera\":\"$CAM\",\"label\":\"motion at {{topic}}\",\"seconds\":10}}],\"wires\":[{\"from\":\"c\",\"port\":0,\"to\":\"m\"}]}"
req POST /api/v1/flows "{\"name\":\"Camera flow $SUF\",\"reason\":\"e2e\",\"spec\":$FLOW}"; check "flow with camera nodes" "$(st) $(jx 'j.problems.length')" "201 0"
FSLUG=$(jx j.flow.slug)
req POST "/api/v1/flows/$FSLUG/deploy" '{"reason":"e2e"}'; check "deployed" "$(st)" 200
curl -s -X POST "http://127.0.0.1:$HP/sim/motion/ongate?on=1"
waitfor '(req GET "/api/v1/video/cameras/$CAM/events?from=$EF&kind=mark"; [[ "$(jx "j.some(e => e.source === \"flow\" && e.label === \"motion at Events $SUF\")")" == true ]])' 30 && ok "motion on one camera marks another through a flow" || bad "marks $(cat "$TMP/b")"
curl -s -X POST "http://127.0.0.1:$HP/sim/motion/ongate?on=0"
req DELETE "/api/v1/flows/$FSLUG?reason=e2e"
req GET "/api/v1/export/audit?format=csv"; check "marks and PTZ audited" "$(grep -c 'camera.mark\|camera.ptz' "$TMP/b" | awk '{print ($1 >= 3)}')" 1
req DELETE "/api/v1/video/cameras/$EVC?reason=e2e"

echo "== video walls"
WALL="{\"name\":\"Control room $SUF\",\"reason\":\"e2e\",\"screens\":[{\"name\":\"Left\",\"cols\":5,\"rows\":4,\"tiles\":[\"$CAM\",null,\"$DOOR\"]},{\"name\":\"Right\",\"cols\":5,\"rows\":4,\"tiles\":[\"$DOOR\"]}]}"
req POST /api/v1/video/walls "$WALL"; check "wall created" "$(st)" 201
WID=$(jx j.id); check "screens stored" "$(jx 'j.screens.map(s => s.name + ":" + s.cols + "x" + s.rows + ":" + s.tiles.length).join(" ")')" "Left:5x4:3 Right:5x4:1"
req POST /api/v1/video/walls "{\"name\":\"x\",\"reason\":\"e2e\",\"screens\":[{\"cols\":9,\"rows\":1,\"tiles\":[]}]}"; check "grid size validated" "$(st)" 400
req POST /api/v1/video/walls "{\"name\":\"x\",\"reason\":\"e2e\",\"screens\":[{\"cols\":1,\"rows\":1,\"tiles\":[\"00000000-0000-7000-8000-000000000000\"]}]}"; check "unknown camera refused" "$(st)" 400
req POST /api/v1/video/walls "{\"name\":\"x\",\"reason\":\"e2e\",\"screens\":[{\"cols\":1,\"rows\":1,\"tiles\":[\"$CAM\",\"$DOOR\"]}]}"; check "more tiles than cells refused" "$(st)" 400
# tiles with their own place and size (0.85.0): a 3×2 tile, a tall 1×2 beside it, an empty 1×1 and one more below
RW="{\"name\":\"Rects $SUF\",\"reason\":\"e2e\",\"screens\":[{\"cols\":6,\"rows\":4,\"tiles\":[\"$CAM\",\"$DOOR\",null,\"$CAM\"],\"rects\":[{\"x\":0,\"y\":0,\"w\":3,\"h\":2},{\"x\":3,\"y\":0,\"w\":1,\"h\":2},{\"x\":4,\"y\":0,\"w\":1,\"h\":1},{\"x\":5,\"y\":3,\"w\":1,\"h\":1}]}]}"
req POST /api/v1/video/walls "$RW"; check "wall with tile rects stored" "$(st) $(jx 'j.screens[0].rects.map(r => r.x + "," + r.y + "," + r.w + "x" + r.h).join(" ")')" "201 0,0,3x2 3,0,1x2 4,0,1x1 5,3,1x1"
RWID=$(jx j.id); req DELETE "/api/v1/video/walls/$RWID?reason=e2e"
req POST /api/v1/video/walls "{\"name\":\"x\",\"reason\":\"e2e\",\"screens\":[{\"cols\":3,\"rows\":3,\"tiles\":[\"$CAM\",\"$DOOR\"],\"rects\":[{\"x\":0,\"y\":0,\"w\":2,\"h\":2},{\"x\":1,\"y\":1,\"w\":1,\"h\":1}]}]}"; check "overlapping tiles refused" "$(st) $(jx j.error)" "400 screen 1: tiles 1 and 2 overlap"
req POST /api/v1/video/walls "{\"name\":\"x\",\"reason\":\"e2e\",\"screens\":[{\"cols\":3,\"rows\":3,\"tiles\":[\"$CAM\"],\"rects\":[{\"x\":2,\"y\":0,\"w\":2,\"h\":1}]}]}"; check "tile outside the grid refused" "$(st)" 400
req POST /api/v1/video/walls "{\"name\":\"x\",\"reason\":\"e2e\",\"screens\":[{\"cols\":3,\"rows\":3,\"tiles\":[\"$CAM\",\"$DOOR\"],\"rects\":[{\"x\":0,\"y\":0,\"w\":1,\"h\":1}]}]}"; check "one rect per tile required" "$(st)" 400
req POST /api/v1/video/walls "{\"name\":\"Plain $SUF\",\"reason\":\"e2e\",\"screens\":[{\"cols\":2,\"rows\":2,\"tiles\":[\"$CAM\",\"$DOOR\"],\"rects\":[{\"x\":0,\"y\":0,\"w\":1,\"h\":1},{\"x\":1,\"y\":0,\"w\":1,\"h\":1}]}]}"; check "a plain grid is stored without rects" "$(st) $(jx 'j.screens[0].rects === undefined')" "201 true"
RWID=$(jx j.id); req DELETE "/api/v1/video/walls/$RWID?reason=e2e"
req PUT /api/v1/video/walls/$WID '{"name":"Renamed","reason":"e2e"}'; check "wall renamed, screens kept" "$(jx 'j.name + " " + j.screens.length')" "Renamed 2"
req GET /api/v1/video/walls; check "wall listed" "$(jx "j.some(w => w.id === '$WID')")" true
curl -s -o /dev/null -w '%{http_code}' "$BASE/video/walls/$WID" > "$TMP/s"; check "wall page served" "$(st)" 200

echo "== motion detection on the server (spec 019)"
mcam() { req POST /api/v1/video/cameras "$1"; jx j.id; }
mstat() { req GET "/api/v1/video/cameras/$1"; }
req POST /api/v1/video/cameras "{\"name\":\"M0 $SUF\",\"url\":\"$SIM/stillx/mjpeg/160x96\",\"motion_detect\":true,\"motion_sensitivity\":0,\"reason\":\"e2e\"}"; check "sensitivity validated" "$(st)" 400
req POST /api/v1/video/cameras "{\"name\":\"M0 $SUF\",\"url\":\"$SIM/stillx/h264/160x96\",\"motion_detect\":true,\"snapshot_url\":\"http://admin:pw@127.0.0.1:$HP/snapshot/x/160x96\",\"reason\":\"e2e\"}"; check "snapshot address without credentials" "$(st)" 400
req POST /api/v1/video/cameras "{\"name\":\"M0 $SUF\",\"source\":\"push\",\"motion_detect\":true,\"snapshot_url\":\"http://127.0.0.1:$HP/snapshot/x/160x96\",\"reason\":\"e2e\"}"; check "no snapshot address for a relayed camera" "$(st)" 400
req POST /api/v1/video/cameras "{\"name\":\"M0 $SUF\",\"url\":\"$SIM/stillx/h264/160x96\",\"motion_zones\":[[[0,0],[2,0],[1,1]]],\"reason\":\"e2e\"}"; check "zones validated" "$(st)" 400
MJ=$(mcam "{\"name\":\"Motion MJPEG $SUF\",\"url\":\"$SIM/STILL-M$SUF/h264/320x176\",\"sub_url\":\"$SIM/STILL-M$SUF/mjpeg/320x176\",\"record\":\"events\",\"motion_detect\":true,\"motion_sensitivity\":60,\"reason\":\"e2e\"}")
check "camera with motion detection" "$(jx 'j.motion_detect + " " + j.motion_sensitivity')" "true 60"
waitfor '(mstat $MJ; [[ "$(jx j.status.motion)" == "watching (MJPEG)" ]])' 20 && ok "watching the MJPEG sub-stream" || bad "motion status: $(jx j.status.motion)"
check "the detector is not counted as a viewer" "$(jx j.status.viewers)" 0
sleep 3; mstat $MJ; check "a still picture is no motion" "$(jx '(j.status.active || []).length')" 0
curl -s -X POST "http://127.0.0.1:$HP/sim/motion/STILL-M$SUF?on=1" >/dev/null
waitfor '(mstat $MJ; [[ "$(jx "(j.status.active || []).includes(\"motion\")")" == true ]])' 15 && ok "motion detected" || bad "no motion: $(head -c 300 "$TMP/b")"
check "motion starts event recording" "$(jx j.status.recording)" true
curl -s -X POST "http://127.0.0.1:$HP/sim/motion/STILL-M$SUF?on=0" >/dev/null
waitfor '(mstat $MJ; [[ "$(jx "(j.status.active || []).length")" == 0 ]])' 20 && ok "motion ended" || bad "motion did not end"
req GET "/api/v1/video/cameras/$MJ/events?from=$(iso '-2 min')"; check "event stored with source server" "$(jx 'j.filter(e => e.kind === "motion" && e.source === "server" && e.ended_at).length')" 1
# a zone where nothing moves (bottom left corner): no motion although the scene moves
ZN=$(mcam "{\"name\":\"Motion zone $SUF\",\"url\":\"$SIM/zone$SUF/mjpeg/320x176\",\"motion_detect\":true,\"motion_sensitivity\":80,\"motion_zones\":[[[0,0.85],[0.3,0.85],[0.3,1],[0,1]]],\"reason\":\"e2e\"}")
waitfor '(mstat $ZN; [[ "$(jx j.status.motion)" == "watching (MJPEG)" ]])' 20; sleep 4; mstat $ZN; check "motion outside the zones is ignored" "$(jx '(j.status.active || []).length')" 0
req PUT "/api/v1/video/cameras/$ZN" '{"motion_zones":[],"reason":"e2e"}'; check "zones removed" "$(st)" 200
waitfor '(mstat $ZN; [[ "$(jx "(j.status.active || []).includes(\"motion\")")" == true ]])' 20 && ok "the whole picture: motion of the scene detected" || bad "no motion without zones"
# an H.264 camera: snapshots from its snapshot address
SN=$(mcam "{\"name\":\"Motion snapshot $SUF\",\"url\":\"$SIM/STILL-S$SUF/h264/320x176\",\"motion_detect\":true,\"snapshot_url\":\"http://127.0.0.1:$HP/snapshot/STILL-S$SUF/320x176\",\"reason\":\"e2e\"}")
waitfor '(mstat $SN; [[ "$(jx j.status.motion)" == "watching (snapshots)" ]])' 20 && ok "watching snapshots of an H.264 camera" || bad "snapshot status: $(jx j.status.motion)"
curl -s -X POST "http://127.0.0.1:$HP/sim/motion/STILL-S$SUF?on=1" >/dev/null
waitfor '(mstat $SN; [[ "$(jx "(j.status.active || []).includes(\"motion\")")" == true ]])' 15 && ok "motion detected in snapshots" || bad "no motion in snapshots"
curl -s -X POST "http://127.0.0.1:$HP/sim/motion/STILL-S$SUF?on=0" >/dev/null
NO=$(mcam "{\"name\":\"Motion h264 $SUF\",\"url\":\"$SIM/nosnap$SUF/h264/160x96\",\"motion_detect\":true,\"reason\":\"e2e\"}")
waitfor '(mstat $NO; [[ "$(jx j.status.motion)" == needs* ]])' 20 && ok "an H.264 camera without a snapshot address is explained" || bad "status: $(jx j.status.motion)"
for c in $MJ $ZN $SN $NO; do req DELETE "/api/v1/video/cameras/$c?reason=e2e"; done; check "motion cameras removed" "$(st)" 200

echo "== sound (spec 020)"
req GET /api/v1/auth/me; check "admin may hear sound" "$(jx 'j.permissions.includes("video.audio")')" true
SND=$(mcam "{\"name\":\"Sound $SUF\",\"url\":\"$SIM/snd$SUF/h264/160x96/opus\",\"record\":\"continuous\",\"audio\":true,\"reason\":\"e2e\"}")
check "camera with sound" "$(jx j.audio)" true
waitfor '(mstat $SND; [[ "$(jx j.status.audio)" == Opus ]])' 20 && ok "the camera's Opus sound is taken" || bad "sound status: $(jx j.status.audio)"
ws "/api/v1/video/cameras/$SND/live?quality=main" 3; check "no sound unless asked for" "$(jx '(j.abin || 0) + " " + (j.types.audio || 0)')" "0 0"
ws "/api/v1/video/cameras/$SND/live?quality=main&audio=1" 3; check "sound as a separate stream next to the picture" "$(jx 'j.types.audio + " " + (j.abin > 10) + " " + j.abox + " " + (j.bin > 5)')" "1 true ftyp true"
FROMS=$(iso '-60 sec'); sleep 2
ws "/api/v1/video/cameras/$SND/playback?from=$FROMS&audio=1" 3; check "recorded sound plays back" "$(jx '(j.abin > 5) + " " + (j.bin > 5)')" "true true"
ws "/api/v1/video/cameras/$SND/playback?from=$FROMS&speed=4&audio=1" 2; check "no sound at another speed" "$(jx 'j.abin || 0')" 0
curl -s -b "$JAR" -o "$TMP/snd.mp4" "$BASE/api/v1/video/cameras/$SND/export?from=$FROMS&to=$(iso '-1 sec')"
check "export holds picture and sound" "$(python3 -c "d=open('$TMP/snd.mp4','rb').read(); print(d.count(b'trak'), b'Opus' in d[:4000])")" "2 True"
# a user without video.audio sees the picture, hears nothing and exports no sound
J2="$TMP/jar2"; OU=$(req POST /api/v1/users "{\"username\":\"ear_$SUF\",\"full_name\":\"Engineer\",\"email\":\"ear$SUF@example.com\",\"roles\":[\"engineer\"],\"reason\":\"e2e\"}"; cat "$TMP/b")
OPW=$(echo "$OU" | node -pe 'JSON.parse(require("fs").readFileSync(0)).initial_password'); OUID=$(echo "$OU" | node -pe 'JSON.parse(require("fs").readFileSync(0)).user.id')
curl -s -c "$J2" -b "$J2" -H 'Content-Type: application/json' -d "{\"username\":\"ear_$SUF\",\"password\":\"$OPW\"}" "$BASE/api/v1/auth/login" >/dev/null
C2=$(curl -s -b "$J2" "$BASE/api/v1/auth/me" | node -pe 'JSON.parse(require("fs").readFileSync(0)).csrf_token')
curl -s -c "$J2" -b "$J2" -H 'Content-Type: application/json' -H "X-CSRF-Token: $C2" -d "{\"current\":\"$OPW\",\"new\":\"Ear-Operator-Passw0rd-$SUF\"}" "$BASE/api/v1/auth/password" >/dev/null
curl -s -c "$J2" -b "$J2" -H 'Content-Type: application/json' -d "{\"username\":\"ear_$SUF\",\"password\":\"Ear-Operator-Passw0rd-$SUF\"}" "$BASE/api/v1/auth/login" >/dev/null
node "$TMP/ws.js" "${BASE/http/ws}/api/v1/video/cameras/$SND/live?quality=main&audio=1" "$J2" 3 > "$TMP/b"; check "without video.audio: picture, no sound" "$(jx '(j.bin > 5) + " " + (j.abin || 0)')" "true 0"
curl -s -b "$J2" -o "$TMP/snd2.mp4" "$BASE/api/v1/video/cameras/$SND/export?from=$FROMS&to=$(iso '-1 sec')"
check "without video.audio: export without sound" "$(python3 -c "d=open('$TMP/snd2.mp4','rb').read(); print(d.count(b'trak'), b'Opus' in d[:4000])")" "1 False"
req DELETE "/api/v1/users/$OUID?reason=e2e"
G7=$(mcam "{\"name\":\"G711 $SUF\",\"url\":\"$SIM/g7$SUF/h264/160x96/g711\",\"record\":\"continuous\",\"audio\":true,\"reason\":\"e2e\"}")
waitfor '(mstat $G7; [[ "$(jx j.status.audio)" == "G.711 8000 Hz (as PCM)" ]])' 20 && ok "G.711 (Tapo and others) taken as PCM" || bad "G.711 status: $(jx j.status.audio)"
ws "/api/v1/video/cameras/$G7/live?quality=main&audio=1" 3; check "G.711 sound as PCM frames next to the picture" "$(jx 'j.audio.codec + " " + j.audio.rate + " " + (j.pbin > 50) + " " + (j.bin > 5) + " " + (j.abin || 0)')" "pcm 8000 true true 0"
G7M=$(mcam "{\"name\":\"G711 MJPEG $SUF\",\"url\":\"$SIM/g7m$SUF/mjpeg/160x96/g711\",\"audio\":true,\"reason\":\"e2e\"}")
ws "/api/v1/video/cameras/$G7M/live?quality=main&audio=1" 3; check "G.711 sound next to an MJPEG picture" "$(jx '(j.pbin > 50) + " " + (j.jpeg > 5)')" "true true"
FROMG=$(iso '-20 sec'); sleep 3
ws "/api/v1/video/cameras/$G7/playback?from=$FROMG&audio=1" 3; check "recorded G.711 sound plays back" "$(jx '(j.pbin > 20) + " " + (j.bin > 5)')" "true true"
curl -s -b "$JAR" -o "$TMP/g7.mp4" "$BASE/api/v1/video/cameras/$G7/export?from=$FROMG&to=$(iso '-1 sec')"
check "export holds picture and PCM sound" "$(python3 -c "d=open('$TMP/g7.mp4','rb').read(); print(d.count(b'trak'), b'ipcm' in d[:4000])")" "2 True"
LV=$(mcam "{\"name\":\"Live sound $SUF\",\"url\":\"$SIM/lv$SUF/h264/160x96/g711\",\"record\":\"continuous\",\"audio\":true,\"audio_record\":false,\"reason\":\"e2e\"}")
check "sound heard live only (saves disk space)" "$(jx 'j.audio + " " + j.audio_record')" "true false"
waitfor '(mstat $LV; [[ "$(jx j.status.recording)" == true ]])' 20
ws "/api/v1/video/cameras/$LV/live?quality=main&audio=1" 3; check "live-only sound is heard" "$(jx '(j.pbin > 50)')" true
FROML=$(iso '-10 sec'); sleep 3
curl -s -b "$JAR" -o "$TMP/lv.mp4" "$BASE/api/v1/video/cameras/$LV/export?from=$FROML&to=$(iso '-1 sec')"
check "live-only sound is not recorded" "$(python3 -c "d=open('$TMP/lv.mp4','rb').read(); print(d.count(b'trak'), b'ipcm' in d[:4000])")" "1 False"
G6=$(mcam "{\"name\":\"G726 $SUF\",\"url\":\"$SIM/g6$SUF/h264/160x96/g726\",\"audio\":true,\"reason\":\"e2e\"}")
waitfor '(mstat $G6; [[ "$(jx j.status.audio)" == *"not supported"* ]])' 20 && ok "other codecs are explained (choose AAC or G.711)" || bad "G.726 status: $(jx j.status.audio)"
req GET "/api/v1/export/audit?format=csv"; check "listening audited" "$(grep -c 'video.listen' "$TMP/b" | awk '{print ($1 >= 2)}')" 1
for c in $SND $G7 $G7M $G6 $LV; do req DELETE "/api/v1/video/cameras/$c?reason=e2e"; done

echo "== video settings"
req GET /api/v1/video/settings; check "settings with defaults" "$(st) $(jx 'j.defaults.segment_seconds + " " + j.defaults.event_merge_seconds + " " + j.can_edit')" "200 60 5 true"
OLD=$(cat "$TMP/b" | node -pe 'JSON.stringify(JSON.parse(require("fs").readFileSync(0)).settings)')
req PUT /api/v1/video/settings '{"segment_seconds":30}'; check "settings need a reason" "$(st)" 400
req PUT /api/v1/video/settings '{"segment_seconds":5,"reason":"e2e"}'; check "bounds enforced" "$(st) $(jx j.error)" "400 segment_seconds: 10 to 600"
req PUT /api/v1/video/settings '{"push_max_fails":1,"reason":"e2e"}'; check "relay blocking cannot be weakened below 3" "$(st)" 400
req PUT /api/v1/video/settings '{"segment_seconds":30,"wall_sub_stream_above":9,"ptz_audit_minutes":0,"reason":"e2e"}'; check "settings saved" "$(st) $(jx 'j.segment_seconds + " " + j.wall_sub_stream_above + " " + j.event_merge_seconds')" "200 30 9 5"
req GET /api/v1/video/settings; check "settings kept" "$(jx 'j.settings.segment_seconds')" 30
req POST /api/v1/video/walls "{\"name\":\"Q $SUF\",\"reason\":\"e2e\",\"screens\":[{\"cols\":2,\"rows\":2,\"tiles\":[\"$CAM\"],\"quality\":\"main\"}]}"; check "wall screen quality stored" "$(st) $(jx 'j.screens[0].quality + " " + j.sub_stream_above')" "201 main 9"
QW=$(jx j.id); req DELETE "/api/v1/video/walls/$QW?reason=e2e"
req POST /api/v1/video/walls "{\"name\":\"x\",\"reason\":\"e2e\",\"screens\":[{\"cols\":1,\"rows\":1,\"tiles\":[],\"quality\":\"4k\"}]}"; check "wall quality validated" "$(st)" 400
req PUT /api/v1/video/settings "$(node -e 'const o=JSON.parse(process.argv[1]); o.reason="e2e restore"; console.log(JSON.stringify(o))' "$OLD")"; check "settings restored" "$(st) $(jx j.segment_seconds)" "200 60"
req GET "/api/v1/export/audit?format=csv"; check "settings change audited" "$(grep -c 'video.settings' "$TMP/b" | awk '{print ($1 >= 2)}')" 1

echo "== change and remove"
req PUT /api/v1/video/cameras/$CAM '{"record":"off","reason":"e2e"}'; check "recording switched off" "$(st) $(jx j.record)" "200 off"
req GET /api/v1/video/cameras/$CAM; check "password kept on update" "$(jx j.has_password)" true
req PUT /api/v1/video/cameras/$CAM '{"password":"","reason":"e2e"}'; check "password cleared" "$(jx j.has_password)" false
req DELETE "/api/v1/video/walls/$WID?reason=e2e"; check "wall deleted" "$(st)" 200
req DELETE "/api/v1/video/cameras/$DOOR"; check "delete needs a reason" "$(st)" 400
for c in $CAM $DOOR; do req DELETE "/api/v1/video/cameras/$c?reason=e2e"; done; check "cameras removed" "$(st)" 200
req GET /api/v1/video/cameras/$CAM; check "removed camera gone" "$(st)" 404
req GET /api/v1/video/storage; check "recordings stay after removal" "$(jx "j.cameras.some(c => c.id === '$CAM' && c.removed && c.bytes > 0)")" true

echo "== camera limit without a license (spec 026)"
if [[ -z "$LIC_OURS" ]]; then
  echo "  skipped: a license was installed before the suite (the limit needs a server without one)"
else
  lcam() { req POST /api/v1/video/cameras "{\"name\":\"$1 $SUF\",\"url\":\"$SIM/lim$1/mjpeg/160x96\",\"record\":\"off\",\"enabled\":${2:-true},\"reason\":\"e2e limit\"}"; }
  req GET /api/v1/video/storage; E0=$(jx j.camera_limit.enabled)
  LIM=()
  for i in 1 2 3 4; do lcam L$i; LIM+=("$(jx j.id)"); done
  check "with the license more cameras run" "$(st)" 201
  req GET /api/v1/video/storage; check "count over all organizations, licensed" "$(jx 'j.camera_limit.enabled + " " + j.camera_limit.free_limit + " " + j.camera_limit.licensed')" "$((E0 + 4)) 4 true"
  req DELETE "/api/v1/server/license?reason=e2e%20camera%20limit"; check "license removed" "$(st)" 200
  req GET /api/v1/server/license; check "license page: cameras of 4 without a license" "$(jx 'j.cameras.enabled + " " + j.cameras.free_limit + " " + j.cameras.licensed')" "$((E0 + 4)) 4 false"
  lcam L5; check "a fifth enabled camera is refused" "$(st) $(jx j.code)" "402 license_required"
  [[ "$(jx j.error)" == *"up to 4 cameras"* && "$(jx j.error)" == *"keep working"* ]] && ok "the refusal explains the limit" || bad "error $(jx j.error)"
  lcam L6 false; check "a disabled camera can be added" "$(st) $(jx j.enabled)" "201 false"; L6=$(jx j.id)
  req PUT "/api/v1/video/cameras/$L6" '{"enabled":true,"reason":"e2e"}'; check "enabling it is refused" "$(st) $(jx j.code)" "402 license_required"
  req PUT "/api/v1/video/cameras/${LIM[0]}" '{"location":"limit test","reason":"e2e"}'; check "existing cameras can still be changed" "$(st) $(jx 'j.enabled + " " + j.location')" "200 true limit test"
  req GET /api/v1/video/cameras; check "nothing was switched off" "$(jx "j.filter(c => c.name.endsWith(' $SUF') && c.name.startsWith('L') && c.enabled).length")" 4
  waitfor '(req GET /api/v1/video/cameras/${LIM[3]}; [[ "$(jx j.status.state)" != disabled && "$(jx j.status.state)" != "" ]])' 20 && ok "cameras beyond the limit keep running ($(jx j.status.state))" || bad "status $(jx j.status.state)"
  req GET /api/v1/video/storage; check "storage reports the limit" "$(jx 'j.camera_limit.licensed + " " + (j.camera_limit.enabled >= 4)')" "false true"
  for c in "${LIM[@]}"; do req DELETE "/api/v1/video/cameras/$c?reason=e2e"; done
  if [[ $E0 -lt 4 ]]; then
    req PUT "/api/v1/video/cameras/$L6" '{"enabled":true,"reason":"e2e"}'; check "below the limit a camera can be enabled again" "$(st) $(jx j.enabled)" "200 true"
  else
    echo "  skipped: other cameras keep $E0 enabled"
  fi
  req DELETE "/api/v1/video/cameras/$L6?reason=e2e"; check "limit test cameras removed" "$(st)" 200
  req GET "/api/v1/export/audit?format=csv"; check "license removal audited" "$(grep -c 'license.remove' "$TMP/b" | awk '{print ($1 >= 1)}')" 1
fi
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

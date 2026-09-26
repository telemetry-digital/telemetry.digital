#!/usr/bin/env bash
# End-to-end checks for spec 010: cameras (RTSP H.264 and MJPEG from the camera simulator), stream test, live video over
# a WebSocket, continuous recording, playback, MP4 export, audit of playback, video walls and validation.
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
const cookie = fs.readFileSync(jar, 'utf8').split('\n').map(l => l.replace('#HttpOnly_', '')).filter(l => l && !l.startsWith('#')).map(l => l.split('\t')).filter(p => p.length >= 7).map(p => p[5] + '=' + p[6]).join('; ');
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
    if (op === 1) { const j = JSON.parse(p.toString()); out.types[j.type] = (out.types[j.type] || 0) + 1; if (j.type === 'init' && !out.init) out.init = j; if (j.type === 'time') out.times.push(j.t); }
    else if (op === 2) { out.bin++; out.bytes += p.length; if (p[0] === 0xff && p[1] === 0xd8) out.jpeg++; else if (out.boxes.length < 3) out.boxes.push(p.subarray(4, 8).toString('latin1')); }
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
[[ -n "${BIN:-}" ]] || go build -o "$TMP/c32" ./cmd/ctrl32-telemetry || { echo "build failed"; exit 1; }
RP=$((20000 + RANDOM % 20000))
"$TMP/camsim" -rtsp 127.0.0.1:$RP -http "" -fps 10 > "$TMP/camsim.log" 2>&1 &
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
req GET "/api/v1/dashboards/$DSLUG"; check "camera widget stored with defaults" "$(jx 'JSON.stringify(j.spec.widgets[0].params)')" "{\"camera\":\"$CAM\",\"quality\":\"sub\",\"show_name\":true,\"open_on_click\":true}"
req POST /api/v1/dashboards "${DW/CAMID/not-a-camera}"; check "camera id validated" "$(st)" 400
req DELETE "/api/v1/dashboards/$DSLUG?reason=e2e"

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
  "${BIN:-$TMP/c32}" camera-relay -config "$TMP/relay.toml" > "$TMP/relay.log" 2>&1 &
  RELAY=$!
  waitfor '(req GET /api/v1/video/cameras/$PUSH; [[ "$(jx j.status.state)" == online ]])' 30 && ok "relay connected, camera online" || bad "push status $(cat "$TMP/b"); relay: $(tail -3 "$TMP/relay.log")"
  check "remote camera records" "$(jx 'j.status.recording + " " + j.status.codec')" "true h264"
  ws "/api/v1/video/cameras/$PUSH/live" 2; check "tiles use the pushed sub-stream" "$(jx 'j.init && j.init.codec')" mjpeg
  printf 'server = "%s"\n[[camera]]\nid = "%s"\nsecret = "%s"\nsource = "%s"\n' "${PURL%/*}" "$PUSH" "wrong-secret-wrong-secret-123" "$SIM/remote/h264/160x96" > "$TMP/bad.toml"; chmod 600 "$TMP/bad.toml"
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

echo "== video walls"
WALL="{\"name\":\"Control room $SUF\",\"reason\":\"e2e\",\"screens\":[{\"name\":\"Left\",\"cols\":5,\"rows\":4,\"tiles\":[\"$CAM\",null,\"$DOOR\"]},{\"name\":\"Right\",\"cols\":5,\"rows\":4,\"tiles\":[\"$DOOR\"]}]}"
req POST /api/v1/video/walls "$WALL"; check "wall created" "$(st)" 201
WID=$(jx j.id); check "screens stored" "$(jx 'j.screens.map(s => s.name + ":" + s.cols + "x" + s.rows + ":" + s.tiles.length).join(" ")')" "Left:5x4:3 Right:5x4:1"
req POST /api/v1/video/walls "{\"name\":\"x\",\"reason\":\"e2e\",\"screens\":[{\"cols\":9,\"rows\":1,\"tiles\":[]}]}"; check "grid size validated" "$(st)" 400
req POST /api/v1/video/walls "{\"name\":\"x\",\"reason\":\"e2e\",\"screens\":[{\"cols\":1,\"rows\":1,\"tiles\":[\"00000000-0000-7000-8000-000000000000\"]}]}"; check "unknown camera refused" "$(st)" 400
req POST /api/v1/video/walls "{\"name\":\"x\",\"reason\":\"e2e\",\"screens\":[{\"cols\":1,\"rows\":1,\"tiles\":[\"$CAM\",\"$DOOR\"]}]}"; check "more tiles than cells refused" "$(st)" 400
req PUT /api/v1/video/walls/$WID '{"name":"Renamed","reason":"e2e"}'; check "wall renamed, screens kept" "$(jx 'j.name + " " + j.screens.length')" "Renamed 2"
req GET /api/v1/video/walls; check "wall listed" "$(jx "j.some(w => w.id === '$WID')")" true
curl -s -o /dev/null -w '%{http_code}' "$BASE/video/walls/$WID" > "$TMP/s"; check "wall page served" "$(st)" 200

echo "== change and remove"
req PUT /api/v1/video/cameras/$CAM '{"record":"off","reason":"e2e"}'; check "recording switched off" "$(st) $(jx j.record)" "200 off"
req GET /api/v1/video/cameras/$CAM; check "password kept on update" "$(jx j.has_password)" true
req PUT /api/v1/video/cameras/$CAM '{"password":"","reason":"e2e"}'; check "password cleared" "$(jx j.has_password)" false
req DELETE "/api/v1/video/walls/$WID?reason=e2e"; check "wall deleted" "$(st)" 200
req DELETE "/api/v1/video/cameras/$DOOR"; check "delete needs a reason" "$(st)" 400
for c in $CAM $DOOR; do req DELETE "/api/v1/video/cameras/$c?reason=e2e"; done; check "cameras removed" "$(st)" 200
req GET /api/v1/video/cameras/$CAM; check "removed camera gone" "$(st)" 404
req GET /api/v1/video/storage; check "recordings stay after removal" "$(jx "j.cameras.some(c => c.id === '$CAM' && c.removed && c.bytes > 0)")" true
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

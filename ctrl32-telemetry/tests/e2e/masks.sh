#!/usr/bin/env bash
# End-to-end checks for spec 016: privacy masks — validation, masks sent before the first frame of live and recorded
# video, masked JPEG snapshots, unmasked view and export only with video.unmask (audited).
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/masks.sh
set -u
cd "$(dirname "$0")/../.."
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d)
SIM_PID=""
trap '[[ -n "$SIM_PID" ]] && kill $SIM_PID 2>/dev/null; rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; J2="$TMP/jar2"; CSRF=""; CSRF2=""
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
iso() { date -u -d "$1" +%Y-%m-%dT%H:%M:%SZ; }
SUF=$RANDOM
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
    if (op === 1) { const j = JSON.parse(p.toString()); out.types[j.type] = (out.types[j.type] || 0) + 1; if (!out.first) out.first = j.type; if (j.type === 'masks') { out.masks = j.masks; out.unmasked = j.unmasked; } if (j.type === 'init' && !out.init) out.init = j; if (j.type === 'time') out.times.push(j.t); }
    else if (op === 2) { out.bin++; out.bytes += p.length; if (p[0] === 0xff && p[1] === 0xd8) out.jpeg++; else if (out.boxes.length < 3) out.boxes.push(p.subarray(4, 8).toString('latin1')); }
  } });
});
req.end();
setTimeout(() => { out.times = out.times.length ? [out.times[0], out.times[out.times.length - 1]] : []; console.log(JSON.stringify(out)); process.exit(0); }, secs * 1000);
EOF

ws() { node "$TMP/ws.js" "${BASE/http/ws}$1" "${3:-$JAR}" "$2" > "$TMP/b"; }

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
check "admin may see unmasked pictures" "$(jx 'j.permissions.includes("video.unmask")')" true
go build -o "$TMP/camsim" ./tools/camsim || { echo "camsim build failed"; exit 1; }
RP=$((20000 + RANDOM % 20000))
"$TMP/camsim" -rtsp 127.0.0.1:$RP -http 127.0.0.1:$((RP + 1)) -fps 10 > "$TMP/camsim.log" 2>&1 &
SIM_PID=$!
sleep 1
FROM=$(iso '-3 sec')
req POST /api/v1/video/cameras "{\"name\":\"Masked $SUF\",\"url\":\"http://127.0.0.1:$((RP + 1))/mask/320x180\",\"record\":\"continuous\",\"reason\":\"e2e\"}"; check "MJPEG camera" "$(st)" 201
CAM=$(jx j.id)
check "no masks by default" "$(jx 'JSON.stringify(j.privacy_masks)')" "[]"

echo "== validation"
req PUT "/api/v1/video/cameras/$CAM" '{"privacy_masks":[[[0,0],[1.2,0],[1,1]]],"reason":"e2e"}'; check "coordinates outside the picture refused" "$(st)" 400
req PUT "/api/v1/video/cameras/$CAM" '{"privacy_masks":[[[0,0],[1,0]]],"reason":"e2e"}'; check "polygon needs 3 points" "$(st)" 400
req PUT "/api/v1/video/cameras/$CAM" '{"privacy_masks":[[[0,0],[0.5,0],[0.5,1],[0,1]]],"reason":"e2e"}'; check "left half masked" "$(st) $(jx 'j.privacy_masks.length')" "200 1"

echo "== masks travel with the picture"
sleep 2
ws "/api/v1/video/cameras/$CAM/live" 3; check "live: masks before the first frame" "$(jx 'j.first')" "masks"
check "live: the mask itself" "$(jx 'JSON.stringify(j.masks)')" "[[[0,0],[0.5,0],[0.5,1],[0,1]]]"
ws "/api/v1/video/cameras/$CAM/playback?from=$FROM" 3; check "playback: masks too" "$(jx 'j.first + " " + j.masks.length')" "masks 1"
curl -s -b "$JAR" -o "$TMP/masked.jpg" "$BASE/api/v1/video/cameras/$CAM/snapshot"
curl -s -b "$JAR" -o "$TMP/plain.jpg" "$BASE/api/v1/video/cameras/$CAM/snapshot?unmask=1"
check "snapshot is a JPEG" "$(head -c 2 "$TMP/masked.jpg" | od -An -tx1 | tr -d ' ')" "ffd8"
check "masked snapshot differs from the unmasked one" "$(cmp -s "$TMP/masked.jpg" "$TMP/plain.jpg" && echo same || echo different)" "different"
ws "/api/v1/video/cameras/$CAM/live?unmask=1" 2; check "admin unmasked: no masks, flagged" "$(jx 'j.masks.length + " " + j.unmasked')" "0 true"
req POST "/api/v1/video/cameras/$CAM/picture-saved" '{"unmasked":false}'; check "saving a masked picture: nothing to audit" "$(st) $(jx 'j.masked + " " + j.audited')" "200 true false"
req POST "/api/v1/video/cameras/$CAM/picture-saved" "{\"unmasked\":true,\"at\":\"$(iso now)\"}"; check "admin saves the unmasked picture: audited" "$(st) $(jx 'j.audited')" "200 true"

echo "== without video.unmask"
U=$(req POST /api/v1/users "{\"username\":\"eng_$SUF\",\"full_name\":\"Engineer\",\"email\":\"eng$SUF@example.com\",\"roles\":[\"engineer\"],\"reason\":\"e2e\"}"; cat "$TMP/b")
PW=$(echo "$U" | node -pe 'JSON.parse(require("fs").readFileSync(0)).initial_password'); UID2=$(echo "$U" | node -pe 'JSON.parse(require("fs").readFileSync(0)).user.id')
req2 POST /api/v1/auth/login "{\"username\":\"eng_$SUF\",\"password\":\"$PW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
req2 POST /api/v1/auth/password "{\"current\":\"$PW\",\"new\":\"Engineer-Passw0rd-$SUF\"}"
req2 POST /api/v1/auth/login "{\"username\":\"eng_$SUF\",\"password\":\"Engineer-Passw0rd-$SUF\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
check "engineer: playback yes, unmask no" "$(jx 'j.permissions.includes("video.playback") + " " + j.permissions.includes("video.unmask")')" "true false"
ws "/api/v1/video/cameras/$CAM/live?unmask=1" 2 "$J2"; check "asking for unmasked without the permission still masks" "$(jx 'j.masks.length + " " + j.unmasked')" "1 false"
curl -s -b "$J2" -o /dev/null -w '%{http_code}' "$BASE/api/v1/video/cameras/$CAM/export?from=$FROM&to=$(iso now)" > "$TMP/s"; check "export of a masked camera refused" "$(st)" 403
curl -s -b "$J2" -o "$TMP/e.jpg" "$BASE/api/v1/video/cameras/$CAM/snapshot?unmask=1"; check "snapshot stays masked" "$(cmp -s "$TMP/e.jpg" "$TMP/plain.jpg" && echo unmasked || echo masked)" masked
req2 POST "/api/v1/video/cameras/$CAM/picture-saved" '{"unmasked":true}'; check "saving an unmasked picture without video.unmask refused" "$(st)" 403
req PATCH "/api/v1/users/$UID2" '{"disabled":true,"reason":"e2e cleanup"}'

echo "== audit and cleanup"
req GET "/api/v1/export/audit?format=csv"; check "unmasked views audited" "$(grep -c 'video.unmask' "$TMP/b" | awk '{print ($1 >= 2)}')" 1
check "the saved unmasked picture audited" "$(grep -c 'picture saved without privacy masks' "$TMP/b" | awk '{print ($1 >= 1)}')" 1
req PUT "/api/v1/video/cameras/$CAM" '{"privacy_masks":[],"reason":"e2e"}'; check "masks removed" "$(jx 'j.privacy_masks.length')" 0
req DELETE "/api/v1/video/cameras/$CAM?reason=e2e"; check "camera removed" "$(st)" 200
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

#!/usr/bin/env bash
# End-to-end checks for spec 015: recordings held as evidence (never deleted until released), fingerprints computed
# while recording, signed evidence packages verified with `ctrl32-telemetry verify-evidence`, tampering detected.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/evidence.sh
set -u
cd "$(dirname "$0")/../.."
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d)
SIM_PID=""
trap '[[ -n "$SIM_PID" ]] && kill $SIM_PID 2>/dev/null; rm -rf "$TMP"' EXIT
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
waitfor() { local n=${2:-40}; for i in $(seq "$n"); do eval "$1" && return 0; sleep 1; done; return 1; }
iso() { date -u -d "$1" +%Y-%m-%dT%H:%M:%SZ; }
enc() { node -pe 'encodeURIComponent(process.argv[1])' "$1"; }
SUF=$RANDOM

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
check "admin may hold evidence" "$(jx 'j.permissions.includes("video.evidence")')" true
go build -o "$TMP/camsim" ./tools/camsim || { echo "camsim build failed"; exit 1; }
go build -o "$TMP/c32" ./cmd/ctrl32-telemetry || { echo "build failed"; exit 1; }
RP=$((20000 + RANDOM % 20000))
"$TMP/camsim" -rtsp 127.0.0.1:$RP -http 127.0.0.1:$((RP + 1)) -fps 10 > "$TMP/camsim.log" 2>&1 &
SIM_PID=$!
sleep 1
req GET /api/v1/video/settings; OLD=$(node -pe 'JSON.stringify(JSON.parse(require("fs").readFileSync(process.argv[1])).settings)' "$TMP/b")
req PUT /api/v1/video/settings '{"segment_seconds":10,"reason":"e2e: short files"}'; check "short recorded files for the test" "$(st)" 200
FROM=$(iso '-5 sec')
req POST /api/v1/video/cameras "{\"name\":\"Evidence $SUF\",\"url\":\"rtsp://127.0.0.1:$RP/ev/h264/160x96\",\"record\":\"continuous\",\"reason\":\"e2e\"}"; check "recording camera" "$(st)" 201
CAM=$(jx j.id)
waitfor '(req GET "/api/v1/video/cameras/$CAM/recordings?from=$FROM"; [[ "$(jx "j.files >= 3")" == true ]])' 60 && ok "three files recorded" || bad "recording: $(head -c 200 "$TMP/b")"
TO=$(iso 'now')

echo "== holds"
req POST /api/v1/video/holds "{\"camera_id\":\"$CAM\",\"from\":\"$FROM\",\"to\":\"$TO\",\"title\":\"Break-in at the gate\"}"; check "hold needs a reason" "$(st)" 400
req POST /api/v1/video/holds "{\"camera_id\":\"$CAM\",\"from\":\"$TO\",\"to\":\"$FROM\",\"title\":\"x\",\"reason\":\"e2e\"}"; check "range validated" "$(st)" 400
req POST /api/v1/video/holds "{\"camera_id\":\"00000000-0000-7000-8000-000000000000\",\"from\":\"$FROM\",\"to\":\"$TO\",\"title\":\"x\",\"reason\":\"e2e\"}"; check "unknown camera refused" "$(st)" 404
req POST /api/v1/video/holds "{\"camera_id\":\"$CAM\",\"from\":\"$FROM\",\"to\":\"$TO\",\"title\":\"Break-in at the gate\",\"case_ref\":\"PZ-$SUF\",\"reason\":\"police request\"}"; check "held as evidence" "$(st)" 201
HID=$(jx j.id)
req GET "/api/v1/video/holds?camera_id=$CAM"; check "hold listed with its size" "$(jx "j.length + ' ' + (j[0].bytes > 0) + ' ' + j[0].case_ref")" "1 true PZ-$SUF"

echo "== evidence package"
req GET /api/v1/video/evidence/key; check "server key" "$(st) $(jx 'j.algorithm')" "200 ed25519"
FP=$(jx j.fingerprint); PUB=$(jx j.public_key)
curl -s -b "$JAR" -o "$TMP/p.zip" -w '%{http_code}' "$BASE/api/v1/video/cameras/$CAM/evidence?from=$FROM&to=$TO" > "$TMP/s"; check "package needs a reason" "$(st)" 400
curl -s -b "$JAR" -o "$TMP/p.zip" -w '%{http_code}' "$BASE/api/v1/video/cameras/$CAM/evidence?from=$FROM&to=$TO&reason=$(enc 'police request')&case_ref=PZ-$SUF" > "$TMP/s"; check "evidence package downloaded" "$(st)" 200
check "package holds video, manifest, signature, key and instructions" "$(unzip -Z1 "$TMP/p.zip" | sort | paste -sd,)" "README.txt,manifest.json,manifest.sig,public_key.txt,video.mp4"
unzip -p "$TMP/p.zip" manifest.json > "$TMP/b"
check "manifest: camera, case, reason" "$(jx 'j.camera.name + " | " + j.case_ref + " | " + j.reason')" "Evidence $SUF | PZ-$SUF | police request"
check "recorded files fingerprinted at recording time and unchanged" "$(jx 'j.recorded_files.length > 0 && j.recorded_files.filter(f => f.sha256_now).every(f => f.unchanged || !f.sha256_recorded)')" true
check "at least one finished file has its recording-time fingerprint" "$(jx 'j.recorded_files.some(f => f.unchanged && f.held)')" true
check "video hash matches the file" "$(jx 'j.video.sha256')" "$(unzip -p "$TMP/p.zip" video.mp4 | sha256sum | cut -d' ' -f1)"
"$TMP/c32" verify-evidence -key "$PUB" "$TMP/p.zip" > "$TMP/v.log" 2>&1; check "verify-evidence with the server key: intact" "$? $(grep -c 'the package is intact' "$TMP/v.log")" "0 1"
check "fingerprint printed by the tool equals the server's" "$(grep '^key' "$TMP/v.log" | sed 's/^key *//')" "$FP"
# tampering: one byte of the video changed
mkdir -p "$TMP/t" && (cd "$TMP/t" && unzip -q "$TMP/p.zip" && printf '\x00' | dd of=video.mp4 bs=1 seek=2000 conv=notrunc 2>/dev/null && zip -q -r "$TMP/bad.zip" .)
"$TMP/c32" verify-evidence -key "$PUB" "$TMP/bad.zip" > "$TMP/v2.log" 2>&1; check "a changed video is detected" "$? $(grep -c 'video hash    INVALID' "$TMP/v2.log")" "1 1"
# tampering: the manifest's reason changed
(cd "$TMP/t" && unzip -o -q "$TMP/p.zip" && sed -i 's/police request/something else/' manifest.json && rm -f "$TMP/bad2.zip" && zip -q -r "$TMP/bad2.zip" .)
"$TMP/c32" verify-evidence -key "$PUB" "$TMP/bad2.zip" > "$TMP/v3.log" 2>&1; check "a changed manifest is detected" "$? $(grep -c 'signature     INVALID' "$TMP/v3.log")" "1 1"

echo "== release and audit"
req DELETE "/api/v1/video/holds/$HID"; check "release needs a reason" "$(st)" 400
req DELETE "/api/v1/video/holds/$HID?reason=$(enc 'case closed')"; check "released" "$(st)" 200
req GET "/api/v1/video/holds?camera_id=$CAM"; check "no longer held" "$(jx j.length)" 0
req GET "/api/v1/video/holds?camera_id=$CAM&all=1"; check "released hold kept with who and why" "$(jx 'j[0].release_reason + " " + !!j[0].released_at')" "case closed true"
req GET "/api/v1/export/audit?format=csv"; check "hold, package (with the video hash) and release audited" "$(grep -c 'video.hold\b\|video.hold_release\|video.evidence_export' "$TMP/b" | awk '{print ($1 >= 3)}') $(grep -c "$(unzip -p "$TMP/p.zip" video.mp4 | sha256sum | cut -c1-16)" "$TMP/b" | awk '{print ($1 >= 1)}')" "1 1"

echo "== cleanup"
req PUT /api/v1/video/settings "$(node -e 'const o=JSON.parse(process.argv[1]); o.reason="e2e restore"; console.log(JSON.stringify(o))' "$OLD")"; check "settings restored" "$(st)" 200
req DELETE "/api/v1/video/cameras/$CAM?reason=e2e"; check "camera removed" "$(st)" 200
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

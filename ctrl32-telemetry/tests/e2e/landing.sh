#!/usr/bin/env bash
# End-to-end checks for spec 012: the public front page built in the graphical editor — versions, publishing, images
# (SVG sanitised, content-checked, sandboxed), strict validation of links and texts, permissions and audit.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/landing.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
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
anon() { curl -s -o "$TMP/b" -D "$TMP/h" -w '%{http_code}' "$BASE$1" > "$TMP/s"; }
upload() { curl -s -b "$JAR" -H "X-CSRF-Token: $CSRF" -F "file=@$1;filename=$(basename "$1")" -F "reason=e2e" -o "$TMP/b" -w '%{http_code}' "$BASE/api/v1/landing/assets" > "$TMP/s"; }

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
anon /api/v1/landing; START=$(jx 'j.version || 0')

echo "== draft and template"
req GET /api/v1/landing/draft; check "draft available" "$(st)" 200
check "template or saved page has blocks" "$(jx 'j.spec.blocks.length > 0 && j.spec.version === 1')" true
anon /api/v1/landing/draft; check "draft needs sign-in" "$(st)" 401

echo "== images"
printf '\x89PNG\r\n\x1a\n' > "$TMP/fake.png"; printf 'not really a png' >> "$TMP/fake.png"
upload "$TMP/fake.png"; check "broken PNG refused" "$(st)" 400
printf 'GIF89a' > "$TMP/x.gif"; upload "$TMP/x.gif"; check "other formats refused" "$(st)" 415
node -e 'const z=require("zlib");const c=(t,d)=>{const b=Buffer.alloc(8+d.length+4);b.writeUInt32BE(d.length,0);b.write(t,4);d.copy(b,8);const crc=(buf)=>{let c,k,t=[];for(let n=0;n<256;n++){c=n;for(k=0;k<8;k++)c=c&1?0xedb88320^(c>>>1):c>>>1;t[n]=c>>>0}c=0xffffffff;for(const x of buf)c=t[(c^x)&255]^(c>>>8);return (c^0xffffffff)>>>0};b.writeUInt32BE(crc(Buffer.concat([Buffer.from(t),d])),8+d.length);return b};const ih=Buffer.alloc(13);ih.writeUInt32BE(2,0);ih.writeUInt32BE(2,4);ih[8]=8;ih[9]=2;const raw=Buffer.from([0,255,0,0,255,0,0,0,0,0,255,0,0,255,0]);require("fs").writeFileSync(process.argv[1],Buffer.concat([Buffer.from([137,80,78,71,13,10,26,10]),c("IHDR",ih),c("IDAT",z.deflateSync(raw)),c("IEND",Buffer.alloc(0))]))' "$TMP/ok.png"
upload "$TMP/ok.png"; check "PNG stored" "$(st) $(jx j.mime)" "201 image/png"; PNG=$(jx j.id)
printf '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10" onload="alert(1)"><script>alert(2)</script><rect width="10" height="10" fill="red"/></svg>' > "$TMP/evil.svg"
upload "$TMP/evil.svg"; check "SVG stored" "$(st) $(jx j.mime)" "201 image/svg+xml"; SVG=$(jx j.id)
anon /api/v1/landing/assets/$SVG
check "SVG sanitised (no script, no handler)" "$(grep -c 'script\|onload' "$TMP/b")" 0
check "images are sandboxed" "$(grep -ci '^content-security-policy:.*sandbox' "$TMP/h")" 1
check "images not sniffed" "$(grep -ci '^x-content-type-options: nosniff' "$TMP/h")" 1

echo "== validation (the page is public)"
page() { printf '{"reason":"e2e","spec":{"version":1,"title":"Test","theme":{},"blocks":[%s]}}' "$1"; }
for href in 'javascript:alert(1)' '//evil.example' 'data:text/html,x' 'http://plain.example'; do
  req POST /api/v1/landing/versions "$(page "{\"id\":\"a\",\"type\":\"cta\",\"title\":\"x\",\"buttons\":[{\"label\":\"x\",\"href\":\"$href\"}]}")"
  check "link $href refused" "$(st)" 400
done
req POST /api/v1/landing/versions "$(page '{"id":"a","type":"html","text":"<script>"}')"; check "unknown block type refused" "$(st)" 400
req POST /api/v1/landing/versions "$(page '{"id":"a","type":"text","onclick":"x"}')"; check "unknown field refused" "$(st)" 400
req POST /api/v1/landing/versions "$(page '{"id":"a","type":"image","image":"https://evil.example/x.png"}')"; check "foreign image refused" "$(st)" 400
req POST /api/v1/landing/versions "$(page '{"id":"a","type":"hero","background":"red;x:url(y)"}')"; check "colour injection refused" "$(st)" 400
req POST /api/v1/landing/versions '{"spec":{"version":1,"theme":{},"blocks":[]}}'; check "reason required" "$(st)" 400

echo "== save, publish, versions"
SPEC="{\"id\":\"hero\",\"type\":\"hero\",\"title\":\"Hello <b>$RANDOM</b>\",\"text\":\"<script>alert(1)</script>\",\"image\":\"$PNG\",\"buttons\":[{\"label\":\"Sign in\",\"href\":\"/login\"}]},{\"id\":\"f\",\"type\":\"features\",\"columns\":2,\"items\":[{\"icon\":\"bolt\",\"title\":\"A\",\"text\":\"B\"}]},{\"id\":\"c\",\"type\":\"contact\",\"email\":\"info@example.com\",\"phone\":\"+421 900 000 000\"}"
req POST /api/v1/landing/versions "$(page "$SPEC")"; check "draft saved" "$(st) $(jx j.published)" "201 false"; V1=$(jx j.version)
anon /api/v1/landing; check "a draft is not public" "$(jx 'j.version || 0')" "$START"
req POST /api/v1/landing/versions/$V1/publish '{"reason":"e2e"}'; check "published" "$(st)" 200
anon /api/v1/landing; check "visitors get the published page" "$(jx 'j.version + " " + j.spec.blocks.length')" "$V1 3"
check "text kept as text (rendered literally by the browser)" "$(jx 'j.spec.blocks[0].text')" "<script>alert(1)</script>"
req POST /api/v1/landing/versions "$(page '{"id":"t","type":"text","title":"Second"}' | sed 's/"reason":"e2e"/"reason":"e2e","publish":true/')"; check "save and publish at once" "$(st) $(jx j.published)" "201 true"; V2=$(jx j.version)
req GET /api/v1/landing/versions; check "versions listed, one published" "$(jx "j.filter(v => v.published).map(v => v.version).join()")" "$V2"
req GET /api/v1/landing/versions/$V1; check "an old version can be opened" "$(jx 'j.spec.blocks.length')" 3
req POST /api/v1/landing/unpublish '{"reason":"e2e"}'; check "unpublished" "$(st)" 200
anon /api/v1/landing; check "built-in page again" "$(jx 'j.spec')" null
curl -s -o "$TMP/idx" "$BASE/"; check "front page always offers sign-in" "$(grep -c 'topSignin\|/login' "$TMP/idx" | awk '{print ($1 > 0)}')" 1
curl -s -o /dev/null -w '%{http_code}' "$BASE/landing/edit" > "$TMP/s"; check "editor page served" "$(st)" 200
req GET "/api/v1/export/audit?format=csv"
check "saves and publishing audited" "$(grep -c 'landing.save\|landing.publish\|landing.unpublish\|landing.asset' "$TMP/b" | awk '{print ($1 >= 4)}')" 1
echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

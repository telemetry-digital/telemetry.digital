#!/usr/bin/env bash
# Check of self-hosted map tiles (PMTiles archive, `[map] pmtiles` in config.toml): archive status, public tile
# fetches with caching, addresses outside the archive, the branding map falling back to the local archive and a
# custom tile URL taking precedence. Requires the server started with [map] pmtiles = bin/sample.pmtiles
# (go run ./tools/pmtilesgen -o bin/sample.pmtiles -maxzoom 4) and attribution = "sample tiles (e2e)".
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/tiles.sh
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
jget() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$1" "$2"; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi
  cat "$TMP/b"; }
code() { curl -s -o /dev/null -w '%{http_code}' "$BASE$1"; }

echo "== archive status (public)"
curl -s -o "$TMP/b" -w '%{http_code}' "$BASE/api/v1/tiles" > "$TMP/s"; check "tiles info" "$(st)" 200
check "configured" "$(jx "$TMP/b" 'j.configured')" true
check "archive opened" "$(jx "$TMP/b" 'j.ok')" true
check "raster png" "$(jx "$TMP/b" 'j.tile_type')" png
check "zoom range" "$(jx "$TMP/b" 'j.min_zoom+"-"+j.max_zoom')" "0-4"
check "tile url template" "$(jx "$TMP/b" 'j.tile_url')" "/tiles/{z}/{x}/{y}.png"
check "name from metadata" "$(jx "$TMP/b" 'j.name')" "ctrl32 sample tiles"
check "attribution from config" "$(jx "$TMP/b" 'j.attribution')" "sample tiles (e2e)"
check "341 tiles addressed" "$(jx "$TMP/b" 'j.tiles')" 341

echo "== tiles"
curl -s -D "$TMP/h" -o "$TMP/t.png" -w '%{http_code} %{content_type} %{size_download}' "$BASE/tiles/2/1/1.png" > "$TMP/s"; read -r c ct size < "$TMP/s"
check "tile 2/1/1" "$c" 200; check "content type" "$ct" image/png
[[ $size -gt 500 ]] && ok "tile has data ($size B)" || bad "tile size $size"
check "png signature" "$(head -c 4 "$TMP/t.png" | od -An -c | tr -d ' \n')" "211PNG"
grep -qi "^cache-control: public, max-age=86400" "$TMP/h" && ok "cacheable for a day" || bad "cache header: $(grep -i cache "$TMP/h")"
check "deepest level" "$(code /tiles/4/15/15.png)" 200
check "extension is optional" "$(code /tiles/0/0/0)" 200
check "zoom beyond the archive → 404" "$(code /tiles/5/0/0.png)" 404
check "x beyond the level → 404" "$(code /tiles/2/9/0.png)" 404
check "garbage address → 400" "$(code /tiles/a/b/c.png)" 400
check "negative → 400" "$(code /tiles/2/-1/0.png)" 400
A=$(curl -s "$BASE/tiles/3/2/5.png" | md5sum | cut -c1-32); B=$(curl -s "$BASE/tiles/3/2/6.png" | md5sum | cut -c1-32)
[[ "$A" != "$B" ]] && ok "different tiles differ" || bad "tiles 3/2/5 and 3/2/6 identical"

echo "== branding map falls back to the archive"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token)
req GET /api/v1/branding >/dev/null; check "branding" "$(st)" 200
check "source pmtiles" "$(jx "$TMP/b" 'j.map.source')" pmtiles
check "local flag" "$(jx "$TMP/b" 'j.map.local')" true
check "builtin flag off" "$(jx "$TMP/b" 'j.map.builtin')" false
check "map tile_url" "$(jx "$TMP/b" 'j.map.tile_url')" "/tiles/{z}/{x}/{y}.png"
check "max_zoom from the archive" "$(jx "$TMP/b" 'j.map.max_zoom')" 4
check "attribution" "$(jx "$TMP/b" 'j.map.attribution')" "sample tiles (e2e)"
check "centre from the archive" "$(jx "$TMP/b" 'j.map.center_lat+","+j.map.center_lon+","+j.map.zoom')" "48.7,19.5,2"

echo "== a custom tile URL takes precedence, empty returns to the archive"
req PUT /api/v1/branding '{"reason":"e2e tiles","map":{"tile_url":"https://tiles.example.com/{z}/{x}/{y}.png","attribution":"example","max_zoom":18,"center_lat":48.1,"center_lon":17.1,"zoom":9}}' >/dev/null; check "set custom tiles" "$(st)" 200
req GET /api/v1/branding >/dev/null
check "source custom" "$(jx "$TMP/b" 'j.map.source')" custom
check "custom url" "$(jx "$TMP/b" 'j.map.tile_url')" "https://tiles.example.com/{z}/{x}/{y}.png"
check "custom max_zoom" "$(jx "$TMP/b" 'j.map.max_zoom')" 18
req PUT /api/v1/branding '{"reason":"e2e tiles reset","map":{"tile_url":"","attribution":"","max_zoom":0,"center_lat":48.1,"center_lon":17.1,"zoom":9}}' >/dev/null; check "clear the url" "$(st)" 200
req GET /api/v1/branding >/dev/null
check "back to pmtiles" "$(jx "$TMP/b" 'j.map.source')" pmtiles
check "organization centre kept" "$(jx "$TMP/b" 'j.map.center_lat+","+j.map.center_lon+","+j.map.zoom')" "48.1,17.1,9"
req PUT /api/v1/branding '{"reason":"e2e tiles restore","map":{"tile_url":"","attribution":"","max_zoom":0,"center_lat":0,"center_lon":0,"zoom":0}}' >/dev/null; check "restore defaults" "$(st)" 200
check "tile url must be http(s)" "$(req PUT /api/v1/branding '{"reason":"e2e","map":{"tile_url":"/tiles/{z}/{x}/{y}.png"}}' >/dev/null; st)" 400

echo "== openapi"
req GET /api/v1/openapi.json >/dev/null; check "openapi documents /api/v1/tiles" "$(jx "$TMP/b" 'Object.keys(j.paths).some(p=>p.endsWith("v1/tiles"))')" true
req GET '/api/v1/export/audit' >/dev/null; check "audit export lists the branding changes" "$(grep -c ',branding.update,' "$TMP/b" | awk '{print ($1>0)?"yes":"no"}')" yes

echo
echo "tiles: $pass passed, $fail failed"
[[ $fail -eq 0 ]]

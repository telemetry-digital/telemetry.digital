#!/usr/bin/env bash
# Check of process mimics (screen kind "mimic", canvas layout, ISA-101 symbols): catalog, validation of the canvas
# geometry, a mimic with every symbol type created and versioned through the API, the starter, the public link.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/mimics.sh
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
SUF=$RANDOM

echo "== login and catalog"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
CSRF=$(req GET /api/v1/auth/me | jget csrf_token)
req GET /api/v1/dashboards/catalog >/dev/null; check "catalog has the five process symbols" "$(jx "$TMP/b" 'j.widgets.filter(w=>w.group==="mimic").map(w=>w.type).sort().join(",")')" "instrument,pipe,pump,tank,valve"
req GET /api/v1/datastreams >/dev/null; DS=$(jx "$TMP/b" 'j[0].id'); [[ -n "$DS" && "$DS" != "null" ]] && ok "a datastream to bind" || bad "no datastreams"

echo "== validation"
W="{\"id\":\"t\",\"type\":\"tank\",\"geometry\":{\"x\":0,\"y\":0,\"w\":200,\"h\":300},\"bindings\":{\"datastreams\":[\"$DS\"]},\"params\":{}}"
req POST /api/v1/dashboards "{\"name\":\"bad-$SUF\",\"spec\":{\"kind\":\"mimic\",\"layout\":{\"type\":\"canvas\",\"width\":100},\"widgets\":[$W]},\"reason\":\"e2e\"}" >/dev/null; check "canvas size limits" "$(st)" 400
req POST /api/v1/dashboards "{\"name\":\"bad-$SUF\",\"spec\":{\"kind\":\"mimic\",\"widgets\":[{\"id\":\"t\",\"type\":\"tank\",\"geometry\":{\"x\":1500,\"y\":0,\"w\":200,\"h\":300},\"bindings\":{\"datastreams\":[\"$DS\"]}}]},\"reason\":\"e2e\"}" >/dev/null; check "geometry outside the canvas refused" "$(st)" 400
req POST /api/v1/dashboards "{\"name\":\"bad-$SUF\",\"spec\":{\"kind\":\"mimic\",\"widgets\":[{\"id\":\"t\",\"type\":\"tank\",\"geometry\":{\"x\":0,\"y\":0,\"w\":10,\"h\":10},\"bindings\":{\"datastreams\":[\"$DS\"]}}]},\"reason\":\"e2e\"}" >/dev/null; check "too small refused" "$(st)" 400
req POST /api/v1/dashboards "{\"name\":\"bad-$SUF\",\"spec\":{\"kind\":\"other\",\"widgets\":[$W]},\"reason\":\"e2e\"}" >/dev/null; check "kind validated" "$(st)" 400
req POST /api/v1/dashboards "{\"name\":\"bad-$SUF\",\"spec\":{\"layout\":{\"type\":\"free\"},\"widgets\":[$W]},\"reason\":\"e2e\"}" >/dev/null; check "layout type validated" "$(st)" 400

echo "== a mimic with every symbol"
SPEC="{\"kind\":\"mimic\",\"widgets\":[$W,{\"id\":\"p\",\"type\":\"pump\",\"geometry\":{\"x\":300,\"y\":100,\"w\":160,\"h\":160},\"bindings\":{\"datastreams\":[\"$DS\"]},\"params\":{\"title\":\"P-1\",\"on_value\":1}},{\"id\":\"v\",\"type\":\"valve\",\"geometry\":{\"x\":600,\"y\":100,\"w\":160,\"h\":160},\"bindings\":{\"datastreams\":[\"$DS\"]},\"params\":{\"orientation\":\"vertical\"}},{\"id\":\"l\",\"type\":\"pipe\",\"geometry\":{\"x\":460,\"y\":160,\"w\":140,\"h\":40},\"bindings\":{\"datastreams\":[\"$DS\"]},\"params\":{\"thickness\":50}},{\"id\":\"i\",\"type\":\"instrument\",\"geometry\":{\"x\":300,\"y\":400,\"w\":140,\"h\":140},\"bindings\":{\"datastreams\":[\"$DS\"]},\"params\":{\"tag\":\"TT-101\",\"warn\":30}},{\"id\":\"n\",\"type\":\"text\",\"geometry\":{\"x\":20,\"y\":800,\"w\":600,\"h\":60},\"bindings\":{},\"params\":{\"title\":\"Note\",\"text\":\"hello\",\"frame\":true}}]}"
SLUG=$(req POST /api/v1/dashboards "{\"name\":\"Mimic $SUF\",\"spec\":$SPEC,\"reason\":\"e2e\"}" | jget slug); check "mimic created" "$(st)" 201
req GET "/api/v1/dashboards/$SLUG" >/dev/null; check "kind mimic" "$(jx "$TMP/b" 'j.spec.kind')" mimic
check "canvas layout with defaults" "$(jx "$TMP/b" 'j.spec.layout.type + " " + j.spec.layout.width + "x" + j.spec.layout.height')" "canvas 1600x900"
check "six widgets, pixel geometry kept" "$(jx "$TMP/b" 'j.spec.widgets.length + " " + j.spec.widgets[1].geometry.x')" "6 300"
check "symbol defaults filled" "$(jx "$TMP/b" 'j.spec.widgets.find(w=>w.id==="p").params.off_label + " " + j.spec.widgets.find(w=>w.id==="v").params.open_value')" "STOPPED 1"
req PUT "/api/v1/dashboards/$SLUG" "{\"spec\":{\"kind\":\"mimic\",\"layout\":{\"type\":\"canvas\",\"width\":2000,\"height\":1000},\"widgets\":[$W]},\"reason\":\"e2e resize\"}" >/dev/null; check "new version with a bigger canvas" "$(st)" 200
req GET "/api/v1/dashboards/$SLUG" >/dev/null; check "version 2, 2000x1000" "$(jx "$TMP/b" 'j.version + " " + j.spec.layout.width')" "2 2000"
req GET "/dashboards/$SLUG" >/dev/null; check "view page served" "$(st)" 200

echo "== starter and public link"
req GET "/api/v1/dashboards/starter?kind=mimic" >/dev/null; check "starter mimic" "$(st)" 200
[[ "$(jx "$TMP/b" 'j.spec.kind')" == "mimic" && "$(jx "$TMP/b" 'j.spec.widgets.length')" -ge 8 && "$(jx "$TMP/b" 'j.spec.widgets.map(w=>w.type).filter(t=>["tank","pump","valve","pipe","instrument"].includes(t)).length')" -ge 6 ]] && ok "starter has the process symbols on a canvas" || bad "starter: $(head -c 300 "$TMP/b")"
req POST "/api/v1/dashboards/$SLUG/share" '{"reason":"e2e"}' >/dev/null; check "public link" "$(st)" 201
URL=$(jget url < "$TMP/b"); TOKEN=${URL##*/}
code=$(curl -s -o "$TMP/p" -w '%{http_code}' "$BASE/p/$TOKEN"); check "public page" "$code" 200
code=$(curl -s -o "$TMP/pj" -w '%{http_code}' "$BASE/api/v1/public/$TOKEN"); check "public spec" "$code" 200
check "public spec is the mimic" "$(jx "$TMP/pj" 'j.spec.kind + " " + j.spec.layout.type')" "mimic canvas"
req DELETE "/api/v1/dashboards/$SLUG/share?reason=e2e" >/dev/null; check "public link withdrawn" "$(st)" 200

echo "== a symbol on a grid dashboard still works"
req POST /api/v1/dashboards "{\"name\":\"Grid symbols $SUF\",\"spec\":{\"widgets\":[{\"id\":\"t\",\"type\":\"tank\",\"geometry\":{\"x\":0,\"y\":0,\"w\":3,\"h\":3},\"bindings\":{\"datastreams\":[\"$DS\"]}}]},\"reason\":\"e2e\"}" >/dev/null; check "tank on a grid dashboard" "$(st)" 201
GSLUG=$(jget slug < "$TMP/b")
req DELETE "/api/v1/dashboards/$GSLUG?reason=e2e" >/dev/null; req DELETE "/api/v1/dashboards/$SLUG?reason=e2e" >/dev/null; check "cleanup" "$(st)" 200
echo
echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

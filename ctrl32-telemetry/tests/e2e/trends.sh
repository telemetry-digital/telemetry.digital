#!/usr/bin/env bash
# End-to-end check of saved trend views (spec 042, 0.96.0): a personal view belongs to its owner and needs no reason;
# a view shared with the organization needs content.write and a reason and is audited; the curves are validated.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/trends.sh
set -u
cd "$(dirname "$0")/../.."
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""; JAR2="$TMP/jar2"; CSRF2=""
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
req2() { call "$JAR2" "$CSRF2" "$@"; }
SUF=$RANDOM

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req POST /api/v1/sites "{\"name\":\"Trend site $SUF\",\"reason\":\"e2e\"}"; SITE=$(jx j.id)
req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"machine\",\"name\":\"Trend tank $SUF\",\"reason\":\"e2e\"}"; ASSET=$(jx j.id)
req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"temp\",\"quantity\":\"temperature\",\"unit\":\"C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}"; D1=$(jx j.id)
req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"level\",\"quantity\":\"level\",\"unit\":\"%\",\"kind\":\"gauge\",\"reason\":\"e2e\"}"; D2=$(jx j.id); check "two datastreams" "$(st)" 201
VIEWER="trv_$SUF"; NEWPW="Trend-Viewer-Passw0rd-$SUF"
req POST /api/v1/users "{\"username\":\"$VIEWER\",\"full_name\":\"Viewer\",\"email\":\"trv$SUF@example.com\",\"roles\":[\"viewer\"],\"reason\":\"e2e\"}"; PW=$(jx j.initial_password)
req2 POST /api/v1/auth/login "{\"username\":\"$VIEWER\",\"password\":\"$PW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
req2 POST /api/v1/auth/password "{\"current\":\"$PW\",\"new\":\"$NEWPW\"}"
req2 POST /api/v1/auth/login "{\"username\":\"$VIEWER\",\"password\":\"$NEWPW\"}"; check "a viewer signs in" "$(st)" 200
req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
SPEC="{\"curves\":[{\"datastream_id\":\"$D1\",\"color\":\"#2563eb\"},{\"datastream_id\":\"$D2\",\"label\":\"Level\",\"style\":\"step\",\"axis\":\"lvl\",\"min\":0,\"max\":100}],\"range\":\"6h\",\"limits\":true}"

echo "== validation"
mk() { $1 POST /api/v1/trends "{\"name\":\"$2\",\"shared\":${4:-false},\"spec\":$3${5:+,\"reason\":\"$5\"}}"; }
mk req "Bad $SUF" '{"curves":[]}'; check "a view without curves" "$(st)" 400
mk req "Bad $SUF" "{\"curves\":[{\"datastream_id\":\"$D1\"},{\"datastream_id\":\"$D1\"}]}"; check "the same datastream twice" "$(st)" 400
mk req "Bad $SUF" '{"curves":[{"datastream_id":"00000000-0000-7000-8000-000000000000"}]}'; check "a datastream that does not exist" "$(st)" 400
mk req "Bad $SUF" "{\"curves\":[{\"datastream_id\":\"$D1\",\"color\":\"red\"}]}"; check "a colour that is not #rrggbb" "$(st)" 400
mk req "Bad $SUF" "{\"curves\":[{\"datastream_id\":\"$D1\",\"min\":5,\"max\":1}]}"; check "an axis that ends below its start" "$(st)" 400
mk req "Bad $SUF" "{\"curves\":[{\"datastream_id\":\"$D1\",\"style\":\"wave\"}]}"; check "an unknown style" "$(st)" 400
mk req "Bad $SUF" "{\"curves\":[{\"datastream_id\":\"$D1\"}],\"range\":\"3h\"}"; check "an unknown range" "$(st)" 400
mk req "Bad $SUF" "{\"curves\":[{\"datastream_id\":\"$D1\"}],\"from\":\"2026-01-02T00:00:00Z\"}"; check "from without to" "$(st)" 400
mk req "" "$SPEC"; check "a name is needed" "$(st)" 400
MANY=$(node -e 'console.log(JSON.stringify({curves: Array.from({length: 17}, (_, i) => ({datastream_id: "00000000-0000-7000-8000-" + String(i).padStart(12, "0")}))}))')
mk req "Bad $SUF" "$MANY"; check "more than 16 curves" "$(st)" 400

echo "== a personal view"
mk req "Mine $SUF" "$SPEC"; check "saved without a reason" "$(st) $(jx '[j.shared, j.mine, j.can_edit, j.spec.curves.length, j.spec.range, j.spec.curves[1].axis].join(" ")')" "201 false true true 2 6h lvl"; MINE=$(jx j.id)
mk req "Default $SUF" "{\"curves\":[{\"datastream_id\":\"$D1\"}]}"; check "a view without a time opens live for one hour" "$(jx j.spec.range)" "1h"; DEF=$(jx j.id)
req GET "/api/v1/audit?limit=5"; check "a personal view is not configuration: no audit entry" "$(jx "String(j.entries.some(e => e.action === 'trend_view.create'))")" "false"
req2 GET /api/v1/trends; check "another user does not see it" "$(jx "String(j.some(v => v.id === '$MINE'))")" "false"
req2 GET "/api/v1/trends/$MINE"; check "nor by its id" "$(st)" 404
req2 PUT "/api/v1/trends/$MINE" "{\"name\":\"Stolen\",\"spec\":$SPEC}"; check "nor change it" "$(st)" 404
req2 DELETE "/api/v1/trends/$MINE"; check "nor remove it" "$(st)" 404
mk req2 "Viewer $SUF" "$SPEC"; check "a viewer keeps views of their own" "$(st)" 201; VW=$(jx j.id)
req GET "/api/v1/trends/$VW"; check "which the administrator does not see either" "$(st)" 404

echo "== a view shared with the organization"
mk req2 "Shared $SUF" "$SPEC" true "for everybody"; check "sharing needs content.write" "$(st)" 403
mk req "Shared $SUF" "$SPEC" true; check "and a reason" "$(st)" 400
mk req "Shared $SUF" "$SPEC" true "the shift watches these together"; check "shared" "$(st) $(jx j.shared)" "201 true"; SH=$(jx j.id)
req GET "/api/v1/audit?limit=5"; check "it is in the audit trail" "$(jx "(e => e ? e.reason : 'none')(j.entries.find(e => e.action === 'trend_view.create'))")" "the shift watches these together"
req2 GET /api/v1/trends; check "everybody sees it, read-only for a viewer" "$(jx "(v => v ? v.shared + ' ' + v.can_edit + ' ' + v.mine : 'none')(j.find(v => v.id === '$SH'))")" "true false false"
req2 PUT "/api/v1/trends/$SH" "{\"name\":\"Changed\",\"shared\":true,\"spec\":$SPEC,\"reason\":\"x\"}"; check "a viewer cannot change it" "$(st)" 403
req2 DELETE "/api/v1/trends/$SH?reason=x"; check "nor remove it" "$(st)" 403
req PUT "/api/v1/trends/$SH" "{\"name\":\"Shared $SUF v2\",\"shared\":true,\"spec\":$SPEC}"; check "a change needs a reason" "$(st)" 400
req PUT "/api/v1/trends/$SH" "{\"name\":\"Shared $SUF v2\",\"shared\":true,\"spec\":{\"curves\":[{\"datastream_id\":\"$D2\"}],\"from\":\"2026-11-01T06:00:00Z\",\"to\":\"2026-11-01T14:00:00Z\"},\"reason\":\"one curve, the morning shift\"}"
check "changed: a fixed window" "$(st) $(jx '[j.name.endsWith("v2"), j.spec.curves.length, j.spec.from, j.spec.range].join(" ")')" "200 true 1 2026-11-01T06:00:00Z "
req PUT "/api/v1/trends/$SH" "{\"name\":\"Shared $SUF v2\",\"shared\":false,\"spec\":$SPEC}"; check "taking it back from the organization needs a reason too" "$(st)" 400
req PUT "/api/v1/trends/$SH" "{\"name\":\"Shared $SUF v2\",\"shared\":false,\"spec\":$SPEC,\"reason\":\"not needed any more\"}"; check "taken back: it is the caller's own now" "$(st) $(jx '[j.shared, j.mine].join(" ")')" "200 false true"
req2 GET "/api/v1/trends/$SH"; check "and the others do not see it any more" "$(st)" 404
req PUT "/api/v1/trends/$MINE" "{\"name\":\"Mine $SUF\",\"shared\":true,\"spec\":$SPEC,\"reason\":\"share it\"}"; check "a personal view can be shared later" "$(st) $(jx j.shared)" "200 true"
req DELETE "/api/v1/trends/$MINE"; check "removing a shared view needs a reason" "$(st)" 400
req DELETE "/api/v1/trends/$MINE?reason=e2e"; check "removed" "$(st)" 200
req DELETE "/api/v1/trends/$DEF"; check "a personal view is removed without a reason" "$(st)" 200
req DELETE "/api/v1/trends/$SH"; req2 DELETE "/api/v1/trends/$VW"; check "cleanup" "$(st)" 200
req GET /trends; check "the page is served" "$(st)" 200

echo
echo "passed: $pass, failed: $fail"
[[ $fail -eq 0 ]]

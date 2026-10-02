#!/usr/bin/env bash
# End-to-end checks for spec 028, the camera inventory: export to Excel, CSV and PDF without passwords; the export
# with passwords (permission video.credentials_export, password step-up, reason, refused for API tokens, rate limit,
# audit); the template; the import with its preview (create, update by id or name, ip move, unchanged, error rows,
# all or nothing, restore by id, a kept id, options off), the round trip export → import without changes, the camera
# limit without a licence (rows beyond 4 enabled cameras imported disabled), and the command line export and import.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 BIN=bin/ctrl32-telemetry.exe CONFIG=config.toml bash tests/e2e/camera_inventory.sh
set -u
cd "$(dirname "$0")/../.."
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
BIN=${BIN:-}
CONFIG=${CONFIG:-}
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""
pass=0; fail=0
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1" 2>/dev/null; }
call() { local jar=$1 csrf=$2 m=$3 p=$4 d=${5:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$jar" -c "$jar" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $csrf" -D "$TMP/h" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$jar" -c "$jar" -X "$m" -H "X-CSRF-Token: $csrf" -D "$TMP/h" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
req()  { call "$JAR" "$CSRF" "$@"; }
# imp FILE [extra curl -F args…]: POST the file to the import
imp()  { local f=$1; shift; local a=(); for x in "$@"; do a+=(-F "$x"); done
  curl -s -b "$JAR" -c "$JAR" -H "X-CSRF-Token: $CSRF" -F "file=@$f" "${a[@]}" -o "$TMP/b" -w '%{http_code}' "$BASE/api/v1/video/inventory/import" > "$TMP/s"; }
hdr()  { grep -i "^$1:" "$TMP/h" | head -1 | cut -d' ' -f2- | tr -d '\r'; }
SUF=$RANDOM

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token); ORG=$(jx j.org_id)
check "admin may export passwords (org_admin by default)" "$(jx 'j.permissions.includes("video.credentials_export") && j.permissions.includes("video.manage")')" true
req GET /api/v1/server/license; LIC=$(jx 'j.valid ? "yes" : "no"')
# two cameras of this run: a Hikvision-style camera with a password and masks, and a disabled one
req POST /api/v1/video/cameras "{\"name\":\"Inv gate $SUF\",\"location\":\"Entrance\",\"group\":\"inv $SUF\",\"url\":\"rtsp://192.168.77.64:554/Streaming/Channels/101\",\"sub_url\":\"rtsp://192.168.77.64:554/Streaming/Channels/102\",\"username\":\"admin\",\"password\":\"Secr3t-$SUF\",\"onvif_url\":\"http://192.168.77.64/onvif/device_service\",\"record\":\"events\",\"retention_days\":21,\"enabled\":false,\"privacy_masks\":[[[0.1,0.1],[0.3,0.1],[0.3,0.2]]],\"note\":\"Mast $SUF\",\"reason\":\"e2e inventory\"}"
check "camera 1 created (disabled)" "$(st)" 201; C1=$(jx j.id)
check "note stored" "$(jx j.note)" "Mast $SUF"
req POST /api/v1/video/cameras "{\"name\":\"Inv dock $SUF\",\"url\":\"rtsp://192.168.77.65/live\",\"enabled\":false,\"reason\":\"e2e inventory\"}"
check "camera 2 created (disabled)" "$(st)" 201; C2=$(jx j.id)
req GET /api/v1/video/cameras; NCAM=$(jx 'j.length')

echo "== export without passwords"
req GET "/api/v1/video/inventory?format=xlsx"; check "Excel" "$(st) $(hdr content-type)" "200 application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
check "Excel is a zip" "$(head -c 2 "$TMP/b")" "PK"
check "every camera of the organization" "$(hdr x-cameras)" "$NCAM"
hdr content-disposition | grep -q 'filename="cameras_' && ok "file name" || bad "file name: $(hdr content-disposition)"
grep -q "Secr3t-$SUF" "$TMP/b" && bad "Excel has the password" || ok "Excel without the password"
req GET "/api/v1/video/inventory?format=csv"; check "CSV" "$(st) $(hdr content-type)" "200 text/csv; charset=utf-8"
cp "$TMP/b" "$TMP/export.csv"
check "UTF-8 byte-order mark" "$(head -c 3 "$TMP/export.csv" | od -An -tx1 | tr -d ' ')" "efbbbf"
grep -q "^id;name;location;group;enabled;connection;make;ip;main_stream_url" "$TMP/export.csv" && ok "semicolon header" || bad "header"
grep -q "^$C1;Inv gate $SUF;Entrance;inv $SUF;no;pull;hikvision;192.168.77.64;rtsp://192.168.77.64:554/Streaming/Channels/101;" "$TMP/export.csv" && ok "camera row with make and ip" || bad "row: $(grep "$C1" "$TMP/export.csv")"
grep -q "Mast $SUF" "$TMP/export.csv" && ok "note exported" || bad "note"
grep -q "Secr3t-$SUF" "$TMP/export.csv" && bad "CSV has the password" || ok "CSV without the password"
grep -q ";password;" "$TMP/export.csv" && bad "password column without the permission" || ok "no password column"
req GET "/api/v1/video/inventory?format=pdf"
if [[ "$(st)" == 200 ]]; then check "PDF" "$(head -c 4 "$TMP/b")" "%PDF"; [[ -n "$(hdr x-report-id)" ]] && ok "PDF stored as a report" || bad "no report id"
else check "PDF without typst" "$(st)" 503; fi
req GET "/api/v1/video/inventory?format=doc"; check "unknown format" "$(st)" 400
req GET "/api/v1/video/inventory/template?format=csv"; check "CSV template" "$(st)" 200
grep -q "^#example;Gate;" "$TMP/b" && ok "template example row" || bad "template: $(head -c 300 "$TMP/b")"
req GET "/api/v1/video/inventory/template"; check "Excel template" "$(st) $(head -c 2 "$TMP/b")" "200 PK"
req GET "/api/v1/export/audit?format=csv"; grep -q "camera_inventory" "$TMP/b" && ok "export audited (export.read)" || bad "export audit"

echo "== export with passwords"
req POST /api/v1/video/inventory/credentials '{"format":"csv","reason":"e2e"}'; check "without the password: step-up required" "$(st) $(jx j.code)" "403 step_up_required"
req POST /api/v1/video/inventory/credentials '{"format":"csv","reason":"e2e","password":"wrong"}'; check "wrong password" "$(st) $(jx j.code)" "403 step_up_failed"
req POST /api/v1/video/inventory/credentials "{\"format\":\"csv\",\"password\":\"$PASS\"}"; check "reason required" "$(st)" 400
req POST /api/v1/video/inventory/credentials "{\"format\":\"csv\",\"reason\":\"e2e handover $SUF\",\"password\":\"$PASS\"}"
if [[ "$(st)" == 429 ]]; then echo "  (rate limit of an earlier run: waiting a minute)"; sleep 61; req POST /api/v1/video/inventory/credentials "{\"format\":\"csv\",\"reason\":\"e2e handover $SUF\",\"password\":\"$PASS\"}"; fi
check "with password and reason" "$(st)" 200
cp "$TMP/b" "$TMP/conf.csv"
hdr content-disposition | grep -q 'filename="CONFIDENTIAL_cameras_' && ok "file name marked CONFIDENTIAL" || bad "name: $(hdr content-disposition)"
grep -q "# CONFIDENTIAL — contains passwords" "$TMP/conf.csv" && ok "marked in the file" || bad "marking"
grep -q ";admin;Secr3t-$SUF;" "$TMP/conf.csv" && ok "password in the file" || bad "password missing: $(grep "$C1" "$TMP/conf.csv")"
req POST /api/v1/video/inventory/credentials "{\"format\":\"xlsx\",\"reason\":\"e2e $SUF\",\"password\":\"$PASS\"}"; check "Excel with passwords" "$(st) $(head -c 2 "$TMP/b")" "200 PK"
req POST /api/v1/video/inventory/credentials "{\"format\":\"csv\",\"reason\":\"e2e $SUF\",\"password\":\"$PASS\"}"; check "third export" "$(st)" 200
req POST /api/v1/video/inventory/credentials "{\"format\":\"csv\",\"reason\":\"e2e $SUF\",\"password\":\"$PASS\"}"; check "fourth at once: rate limited" "$(st)" 429
req GET "/api/v1/export/audit?format=csv"; check "credential exports audited" "$(grep -c "video.credentials_export" "$TMP/b" | awk '{print ($1 >= 3)}')" 1
grep "video.credentials_export" "$TMP/b" | grep -q "e2e handover $SUF" && ok "with the reason" || bad "audit reason"
# an API token of the same user is refused; an engineer has no video.credentials_export
req POST /api/v1/tokens '{"name":"inv e2e","scopes":["video.manage","video.credentials_export","video.view"]}'; TOKEN=$(jx j.token)
curl -s -o "$TMP/b" -w '%{http_code}' -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d "{\"format\":\"csv\",\"reason\":\"x\",\"password\":\"$PASS\"}" "$BASE/api/v1/video/inventory/credentials" > "$TMP/s"
check "API token refused" "$(st)" 403
curl -s -o "$TMP/b" -w '%{http_code}' -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/video/inventory?format=csv" > "$TMP/s"; check "API token exports without passwords" "$(st)" 200
req POST /api/v1/users "{\"username\":\"inv_$SUF\",\"full_name\":\"Inventory engineer\",\"email\":\"inv$SUF@example.com\",\"roles\":[\"engineer\"],\"reason\":\"e2e\"}"; PW=$(jx j.initial_password)
J2="$TMP/jar2"; CSRF2=""; req2() { call "$J2" "$CSRF2" "$@"; }
req2 POST /api/v1/auth/login "{\"username\":\"inv_$SUF\",\"password\":\"$PW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
NEWPW="Inventory-Passw0rd-$SUF"; req2 POST /api/v1/auth/password "{\"current\":\"$PW\",\"new\":\"$NEWPW\"}"
req2 POST /api/v1/auth/login "{\"username\":\"inv_$SUF\",\"password\":\"$NEWPW\"}"; req2 GET /api/v1/auth/me; CSRF2=$(jx j.csrf_token)
check "engineer: video.manage without video.credentials_export" "$(jx 'j.permissions.includes("video.manage") + " " + j.permissions.includes("video.credentials_export")')" "true false"
req2 GET "/api/v1/video/inventory?format=csv"; check "engineer exports without passwords" "$(st)" 200
req2 POST /api/v1/video/inventory/credentials "{\"format\":\"csv\",\"reason\":\"x\",\"password\":\"$NEWPW\"}"; check "engineer cannot export passwords" "$(st)" 403

echo "== import: round trip"
imp "$TMP/export.csv"; check "preview of the export" "$(st)" 200
check "every row unchanged" "$(jx 'j.counts.unchanged + " " + (j.counts.create||0) + " " + (j.counts.update||0) + " " + (j.counts.error||0)')" "$NCAM 0 0 0"
check "export-only columns ignored" "$(jx 'j.ignored.join()')" "floor_plans,state"
imp "$TMP/conf.csv"; check "the confidential export: unchanged too (same passwords)" "$(jx 'j.counts.unchanged + " " + j.passwords')" "$NCAM 0"

echo "== import: changes"
NEWID=$(node -e 'const c=require("crypto").randomUUID(); console.log("0199ffff"+c.slice(8))')
changes() { # the last row: a new camera with a free id, without (1) or with (2) its address
  printf 'id;name;ip;password;retention_days;make;location\n'
  printf '%s;Inv gate %s;10.77.0.64;N3w-%s;30;;\n' "$C1" "$SUF" "$SUF"
  printf ';Inv dock %s;;;;;-\n' "$SUF"
  printf ';Inv new %s;10.77.0.70;;;tapo;Hall B\n' "$SUF"
  if [[ $1 == 1 ]]; then printf '%s;Inv kept id %s;;;;;\n' "$NEWID" "$SUF"; else printf '%s;Inv kept id %s;10.77.0.80;;;reolink;\n' "$NEWID" "$SUF"; fi
}
changes 1 > "$TMP/change.csv"
imp "$TMP/change.csv"; check "preview" "$(st)" 200
check "actions" "$(jx 'j.rows.map(r => r.action).join()')" "update,unchanged,create,error"
check "a camera without an address" "$(jx 'j.rows[3].errors[0]')" "url is required (rtsp://… or http://… of the camera's stream)"
changes 2 > "$TMP/change.csv"
imp "$TMP/change.csv"
check "update: ip moved, password, retention" "$(jx 'j.rows[0].changes.join()')" "onvif_url,password,retention_days,sub_url,url"
check "the dash clears nothing on an empty location" "$(jx 'j.rows[1].action')" "unchanged"
check "new camera from make and ip" "$(jx 'j.rows[2].action + " " + j.rows[2].ip + " " + j.rows[2].warnings.filter(w => w.startsWith("addresses filled in for TP-Link Tapo")).length')" "create 10.77.0.70 1"
check "a free id is kept" "$(jx 'j.rows[3].action + " " + j.rows[3].id')" "create $NEWID"
check "counts" "$(jx 'j.counts.update + " " + j.counts.create + " " + j.counts.unchanged + " " + j.passwords')" "1 2 1 1"
req GET "/api/v1/video/cameras/$C1"; check "nothing changed by the preview" "$(jx 'j.url + " " + j.retention_days')" "rtsp://192.168.77.64:554/Streaming/Channels/101 21"
imp "$TMP/change.csv" apply=1; check "apply needs a reason" "$(st)" 400
# a row with an error stops the whole import
cp "$TMP/change.csv" "$TMP/bad.csv"; echo ";Inv broken $SUF;;;0;;" >> "$TMP/bad.csv"
imp "$TMP/bad.csv" apply=1 "reason=e2e"; check "an error row: nothing is applied" "$(st) $(jx j.code) $(jx 'j.plan.counts.error')" "422 rows_invalid 1"
req GET "/api/v1/video/cameras/$C1"; check "camera 1 unchanged" "$(jx 'j.retention_days')" 21
imp "$TMP/change.csv" apply=1 "reason=e2e import $SUF"; check "applied" "$(st) $(jx j.applied)" "200 true"
req GET "/api/v1/video/cameras/$C1"; check "camera 1 moved to the new ip" "$(jx 'j.url + " " + j.onvif_url + " " + j.retention_days + " " + j.note')" "rtsp://10.77.0.64:554/Streaming/Channels/101 http://10.77.0.64/onvif/device_service 30 Mast $SUF"
req GET /api/v1/video/cameras; check "new cameras" "$(jx "j.filter(c => c.name === 'Inv new $SUF' || c.name === 'Inv kept id $SUF').length")" 2
C3=$(jx "j.find(c => c.name === 'Inv new $SUF').id")
check "tapo addresses" "$(jx "j.find(c => c.name === 'Inv new $SUF').url + ' ' + j.find(c => c.name === 'Inv new $SUF').onvif_url")" "rtsp://10.77.0.70:554/stream1 http://10.77.0.70:2020/onvif/device_service"
req GET "/api/v1/video/cameras/$NEWID"; check "camera with the kept id" "$(st) $(jx j.name)" "200 Inv kept id $SUF"
req GET "/api/v1/export/audit?format=csv"
grep "video.import" "$TMP/b" | grep -q "e2e import $SUF" && ok "import audited (video.import)" || bad "video.import audit"
check "camera entries with the import reason" "$(grep -c "import: e2e import $SUF" "$TMP/b")" 3

echo "== import: options, restore, round trip"
printf 'name;location\nInv dock %s;Quay\nInv absent %s;x\n' "$SUF" "$SUF" > "$TMP/opt.csv"
imp "$TMP/opt.csv" update=0 create=0; check "both off: skipped, with the rows" "$(jx 'j.rows.map(r => r.action).join()')" "skip,error"
imp "$TMP/opt.csv" update=1 create=0; check "update only" "$(jx 'j.rows.map(r => r.action).join()')" "update,error"
req DELETE "/api/v1/video/cameras/$C3?reason=e2e"; check "camera removed" "$(st)" 200
printf 'id;name\n%s;Inv new %s\n' "$C3" "$SUF" > "$TMP/restore.csv"
imp "$TMP/restore.csv"; check "a removed camera: restore" "$(jx 'j.rows[0].action')" "restore"
imp "$TMP/restore.csv" apply=1 "reason=e2e restore"; check "restored" "$(st)" 200
req GET "/api/v1/video/cameras/$C3"; check "the camera is back with its id" "$(st) $(jx j.name)" "200 Inv new $SUF"
req GET "/api/v1/video/inventory?format=csv"; cp "$TMP/b" "$TMP/export2.csv"
imp "$TMP/export2.csv"; check "round trip after the changes" "$(jx '(j.counts.create||0) + " " + (j.counts.update||0) + " " + (j.counts.error||0)')" "0 0 0"
DUPID=$(node -e 'const c=require("crypto").randomUUID(); console.log("0199eeee"+c.slice(8))')
printf 'id;name;main_stream_url\n%s;Inv dup A %s;rtsp://10.79.0.1/a\n%s;Inv dup B %s;rtsp://10.79.0.2/b\n;Inv gate %s;\n' "$DUPID" "$SUF" "$DUPID" "$SUF" "$SUF" > "$TMP/dup.csv"
imp "$TMP/dup.csv"; check "the same camera twice in a file" "$(jx 'j.rows.map(r => r.action).join()') $(jx 'j.rows[1].errors[0]')" "create,error,unchanged the same camera as line 2"
printf 'x;y\n1;2\n' > "$TMP/no.csv"; imp "$TMP/no.csv"; check "a file without the columns" "$(st)" 400
head -c 2200000 /dev/zero > "$TMP/big.csv"; imp "$TMP/big.csv"; check "a file over 2 MB" "$(st)" 400
req2 GET /api/v1/auth/me
curl -s -b "$J2" -H "X-CSRF-Token: $CSRF2" -F "file=@$TMP/opt.csv" -o "$TMP/b" -w '%{http_code}' "$BASE/api/v1/video/inventory/import" > "$TMP/s"; check "engineer may import (video.manage)" "$(st)" 200

echo "== camera limit without a licence"
if [[ "$LIC" == yes ]]; then
  echo "  skipped: the server has a licence"
else
  req GET /api/v1/video/storage; E0=$(jx 'j.camera_limit.enabled')
  { echo "name;main_stream_url;enabled"; for i in 1 2 3 4 5; do echo "Inv limit $i $SUF;rtsp://10.78.0.$i/live;yes"; done; } > "$TMP/limit.csv"
  imp "$TMP/limit.csv"; FREE=$(( E0 >= 4 ? 0 : 4 - E0 ))
  check "rows beyond 4 enabled cameras are imported disabled" "$(jx 'j.rows.filter(r => r.enabled).length + " " + j.counts.disabled_by_licence + " " + j.enabled_after')" "$FREE $((5 - FREE)) $((E0 + FREE))"
  jx 'j.rows[4].warnings.join()' | grep -q "imported disabled" && ok "with the warning" || bad "warning"
  imp "$TMP/limit.csv" apply=1 "reason=e2e limit"; check "applied" "$(st)" 200
  req GET /api/v1/video/storage; check "enabled cameras of the server" "$(jx 'j.camera_limit.enabled')" "$((E0 + FREE))"
  req GET /api/v1/video/cameras
  for id in $(jx "j.filter(c => c.name.startsWith('Inv limit ') && c.name.endsWith(' $SUF')).map(c => c.id).join(' ')"); do req DELETE "/api/v1/video/cameras/$id?reason=e2e%20cleanup"; done
fi

echo "== command line"
if [[ -n "$BIN" && -n "$CONFIG" && -x "$BIN" ]]; then
  req GET /api/v1/organizations; SLUG=$(jx "(j.find(o => o.id === '$ORG') || {}).slug || ''")
  OA=(); [[ -n "$SLUG" ]] && OA=(-org "$SLUG")
  "$BIN" cameras export -config "$CONFIG" "${OA[@]}" -file "$TMP/cli.xlsx" > "$TMP/cli.log" 2>&1; check "export" "$?" 0
  check "Excel written" "$(head -c 2 "$TMP/cli.xlsx")" "PK"
  "$BIN" cameras export -config "$CONFIG" "${OA[@]}" -file "$TMP/cli.csv" -passwords > "$TMP/cli.log" 2>&1; check "passwords need a reason" "$?" 1
  "$BIN" cameras export -config "$CONFIG" "${OA[@]}" -file "$TMP/cli.csv" -passwords -reason "e2e cli $SUF" > "$TMP/cli.log" 2>&1; check "export with passwords" "$?" 0
  grep -q "N3w-$SUF" "$TMP/cli.csv" && ok "password in the file" || bad "cli password"
  printf 'id;name;retention_days\n%s;Inv gate %s;45\n' "$C1" "$SUF" > "$TMP/cli-in.csv"
  "$BIN" cameras import -config "$CONFIG" "${OA[@]}" -file "$TMP/cli-in.csv" -dry-run > "$TMP/cli.log" 2>&1; check "dry run" "$?" 0
  grep -q "line 2: update" "$TMP/cli.log" && ok "dry run lists the change" || bad "dry run: $(cat "$TMP/cli.log")"
  req GET "/api/v1/video/cameras/$C1"; check "dry run changes nothing" "$(jx j.retention_days)" 30
  "$BIN" cameras import -config "$CONFIG" "${OA[@]}" -file "$TMP/cli-in.csv" > "$TMP/cli.log" 2>&1; check "import needs a reason" "$?" 1
  "$BIN" cameras import -config "$CONFIG" "${OA[@]}" -file "$TMP/cli-in.csv" -reason "e2e cli $SUF" > "$TMP/cli.log" 2>&1; check "import" "$?" 0
  req GET "/api/v1/video/cameras/$C1"; check "imported on the command line" "$(jx j.retention_days)" 45
  req GET "/api/v1/export/audit?format=csv"; grep "video.credentials_export" "$TMP/b" | grep -q "cli cameras export" && ok "command line export audited" || bad "cli audit"
else
  echo "  skipped: BIN and CONFIG not set"
fi

echo "== cleanup"
for id in $C1 $C2 $C3 $NEWID; do req DELETE "/api/v1/video/cameras/$id?reason=e2e%20cleanup"; done
check "cameras removed" "$(st)" 200

echo
echo "camera inventory: $pass passed, $fail failed"
[[ $fail -eq 0 ]]

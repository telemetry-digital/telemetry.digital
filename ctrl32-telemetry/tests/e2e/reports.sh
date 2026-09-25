#!/usr/bin/env bash
# End-to-end checks for PDF reports, report templates (content framework) and branding (ADR-0010, ADR-0011).
# Requires a running server with Typst, an admin without MFA (demo policy mfa_optional) and demo data.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 bash tests/e2e/reports.sh
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
jnum() { grep -o "\"$1\":[0-9]*" | head -1 | cut -d: -f2; }
req()  { # method path [data] -> body; status in $STATUS, headers in $TMP/h
  local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then
    printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -D "$TMP/h" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else
    curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -D "$TMP/h" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"
  fi
  STATUS=$(cat "$TMP/s"); cat "$TMP/b"
}
ispdf() { head -c 4 "$1" | grep -q '%PDF'; }
sha()   { sha256sum "$1" | cut -d' ' -f1; }

echo "== public pages"
req GET / >/dev/null; check "landing page" "$(st)" 200
grep -q 'id="landing"' "$TMP/b" && ok "landing markup present" || bad "landing markup missing"
req GET /license >/dev/null; grep -q "Apache License" "$TMP/b" && ok "license page" || bad "license page"
req GET /manifest.webmanifest >/dev/null; check "manifest" "$(st)" 200
req GET /api/v1/branding >/dev/null; check "branding public" "$(st)" 200
grep -q '"fixed":true' "$TMP/b" && ok "footer marked fixed" || bad "footer flag"
req GET "/api/v1/report-templates" >/dev/null; check "templates need auth" "$(st)" 401

echo "== login"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}" >/dev/null; check "login" "$(st)" 200
ME=$(req GET /api/v1/auth/me); CSRF=$(echo "$ME" | jget csrf_token); [[ -n "$CSRF" ]] && ok "csrf token" || bad "csrf token"

echo "== templates"
LIST=$(req GET /api/v1/report-templates); check "list templates" "$(st)" 200
echo "$LIST" | grep -q '"observations"' && ok "built-in kinds listed" || bad "built-in kinds"
T=$(req GET /api/v1/report-templates/observations); check "get template" "$(st)" 200
echo "$T" | grep -q '"builtin":true' && ok "observations is built-in" || bad "built-in flag"
req GET /api/v1/report-templates/observations/contract >/dev/null; check "contract" "$(st)" 200
grep -q '"columns"' "$TMP/b" && ok "contract has columns" || bad "contract columns"
req GET /api/v1/report-templates/observations/preview >/dev/null; check "preview built-in" "$(st)" 200
ispdf "$TMP/b" && ok "preview is PDF ($(stat -c %s "$TMP/b") bytes)" || bad "preview not PDF: $(head -c 200 "$TMP/b")"

echo "== PDF exports"
DS=$(req GET /api/v1/datastreams | jget id); [[ -n "$DS" ]] && ok "datastream $DS" || bad "no datastream"
req GET "/api/v1/export/observations?datastream_id=$DS&format=pdf" >/dev/null; check "observations pdf" "$(st)" 200
ispdf "$TMP/b" && ok "observations PDF ($(stat -c %s "$TMP/b") bytes)" || bad "observations not PDF: $(head -c 300 "$TMP/b")"
RID=$(grep -i '^X-Report-Id:' "$TMP/h" | tr -d '\r' | awk '{print $2}'); [[ -n "$RID" ]] && ok "report id $RID" || bad "no X-Report-Id"
cp "$TMP/b" "$TMP/first.pdf"
META=$(req GET "/api/v1/reports/$RID"); check "report metadata" "$(st)" 200
FH=$(echo "$META" | jget file_hash); check "file hash matches served PDF" "$(sha "$TMP/first.pdf")" "$FH"
req GET "/api/v1/reports/$RID/file" >/dev/null; check "download stored report" "$(st)" 200
check "stored file identical" "$(sha "$TMP/b")" "$FH"
req GET "/api/v1/reports?limit=5" >/dev/null; grep -q "$RID" "$TMP/b" && ok "report listed" || bad "report not listed"
for e in "alarms?format=pdf" "audit?format=pdf" "access-log?format=pdf"; do
  req GET "/api/v1/export/$e" >/dev/null; check "export $e" "$(st)" 200; ispdf "$TMP/b" && ok "$e is PDF" || bad "$e not PDF: $(head -c 200 "$TMP/b")"
done
DEV=$(req GET /api/v1/devices | jget id)
req GET "/api/v1/export/messages?device_id=$DEV&format=pdf" >/dev/null; check "messages pdf" "$(st)" 200
req GET "/api/v1/export/observations?datastream_id=$DS&format=csv" >/dev/null; check "csv still works" "$(st)" 200

echo "== template versions"
BODY=$(echo "$T" | sed 's/.*"body":"//; s/","checksum".*//' | sed 's/\\n/\n/g; s/\\"/"/g')
NEW=$(printf '%s\n#text(size: 8pt)[Custom organization template v1]\n' "$BODY")
PAYLOAD=$(printf '{"locale":"en","name":"obs custom","reason":"e2e test","body":%s}' "$(printf '%s' "$NEW" | sed 's/\\/\\\\/g; s/"/\\"/g' | awk 'BEGIN{ORS="\\n"}{print}' | sed 's/^/"/; s/\\n$/"/')")
req PUT /api/v1/report-templates/observations "$PAYLOAD" >/dev/null; check "save version 1" "$(st)" 201
VER=$(grep -o '"version":[0-9]*' "$TMP/b" | head -1 | cut -d: -f2); [[ -n "$VER" ]] && ok "version $VER" || bad "version field: $(cat "$TMP/b")"
req GET /api/v1/report-templates/observations >/dev/null; grep -q '"builtin":false' "$TMP/b" && ok "override active" || bad "override not active"
req GET "/api/v1/export/observations?datastream_id=$DS&format=pdf" >/dev/null; check "export with override" "$(st)" 200
RID2=$(grep -i '^X-Report-Id:' "$TMP/h" | tr -d '\r' | awk '{print $2}')
req GET "/api/v1/reports/$RID2" >/dev/null; grep -q "\"template_ver\":$VER" "$TMP/b" && ok "report records template version $VER" || bad "template_ver: $(cat "$TMP/b")"
req PUT /api/v1/report-templates/observations '{"locale":"en","reason":"broken","body":"#import \"layout.typ\": *\n#let d = load(\n"}' >/dev/null; check "broken template rejected" "$(st)" 422
req PUT /api/v1/report-templates/observations '{"locale":"en","body":"#text[x]"}' >/dev/null; check "reason required" "$(st)" 400
req POST /api/v1/report-templates/preview '{"kind":"alarms","locale":"en","body":"#import \"layout.typ\": *\n#let d = load()\n#show: page-setup.with(d)\n#title-block(d)\n#data-table(d)\n"}' >/dev/null; check "preview source" "$(st)" 200
ispdf "$TMP/b" && ok "preview source is PDF" || bad "preview source not PDF"
req DELETE "/api/v1/report-templates/observations?locale=en&reason=e2e" >/dev/null; check "revert" "$(st)" 200
req GET /api/v1/report-templates/observations >/dev/null; grep -q '"builtin":true' "$TMP/b" && ok "built-in again" || bad "revert failed"
req DELETE "/api/v1/report-templates/observations?locale=en" >/dev/null; check "revert twice -> 404" "$(st)" 404

echo "== branding"
PNG="iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="
req PUT /api/v1/branding "{\"theme\":{\"primary\":\"#ff6600\",\"font\":\"Inter\",\"mode\":\"dark\",\"radius\":\"large\",\"font_scale\":\"large\"},\"logo\":{\"mime\":\"image/png\",\"data\":\"$PNG\"},\"reason\":\"e2e\"}" >/dev/null; check "set branding" "$(st)" 200
req GET /api/v1/branding >/dev/null; grep -q '"primary":"#ff6600"' "$TMP/b" && ok "theme stored" || bad "theme: $(cat "$TMP/b")"
grep -q '"logo_custom":true' "$TMP/b" && ok "custom logo flagged" || bad "logo flag"
req GET /api/v1/branding/logo >/dev/null; check "logo served" "$(st)" 200
grep -qi '^Content-Type: image/png' "$TMP/h" && ok "logo mime" || bad "logo mime"
grep -qi "^Content-Security-Policy: default-src 'none'" "$TMP/h" && ok "logo CSP" || bad "logo CSP"
req GET "/api/v1/export/observations?datastream_id=$DS&format=pdf" >/dev/null; check "pdf with logo" "$(st)" 200; ispdf "$TMP/b" && ok "pdf with logo is PDF" || bad "pdf with logo: $(head -c 300 "$TMP/b")"
req PUT /api/v1/branding '{"theme":{"primary":"red"},"reason":"x"}' >/dev/null; check "bad colour rejected" "$(st)" 400
req PUT /api/v1/branding '{"logo":{"mime":"image/svg+xml","data":"PHN2Zz48c2NyaXB0PmFsZXJ0KDEpPC9zY3JpcHQ+PC9zdmc+"},"reason":"x"}' >/dev/null; check "svg with script rejected" "$(st)" 400
req PUT /api/v1/branding '{"clear_logo":true,"theme":{"primary":"#e30613","font":"","mode":"auto","radius":"medium","font_scale":"normal"},"reason":"e2e cleanup"}' >/dev/null; check "reset branding" "$(st)" 200
req GET /api/v1/branding/logo >/dev/null; check "built-in logo redirect" "$(st)" 302

echo "== integrity"
FILE=$(find "${REPORTS_DIR:-reports}" -name "$RID.pdf" 2>/dev/null | head -1)
if [[ -n "$FILE" ]]; then
  printf 'x' >> "$FILE"
  req GET "/api/v1/reports/$RID/file" >/dev/null; check "tampered file refused" "$(st)" 409
  req GET "/api/v1/export/audit?format=csv" >/dev/null; grep -q "report.integrity_failure" "$TMP/b" && ok "integrity failure audited" || bad "integrity audit entry missing"
else
  bad "stored file not found under ${REPORTS_DIR:-reports}"
fi

echo "== block designer"
req GET /api/v1/report-templates/observations >/dev/null; check "template carries spec" "$(st)" 200
grep -q '"spec":{' "$TMP/b" && ok "spec present" || bad "spec missing"
grep -q '"blocks":\[{"type":"title"' "$TMP/b" && ok "block catalog" || bad "catalog"
req GET /api/v1/report-templates/observations/default-spec >/dev/null; check "default spec" "$(st)" 200
grep -q '"type":"chart"' "$TMP/b" && ok "observations default has chart" || bad "default spec"
SPEC='{"version":1,"page":{"paper":"a4","landscape":true,"font_size":9},"blocks":[{"type":"title","params":{"title":"Fridge report {{scope.asset}}","subtitle":"{{org}}","show_period":true,"show_scope":true}},{"type":"text","params":{"text":"Period {{period.from}} to {{period.to}}.\nSecond paragraph.","style":"note"}},{"type":"summary","params":{"keys":["count","min","max"]}},{"type":"chart","params":{"caption":"Temperature"}},{"type":"daily","params":{"value_key":"value","time_key":"measured_at","decimals":1}},{"type":"table","params":{"columns":["measured_at","value"],"max_rows":50,"font_size":7}},{"type":"signatures","params":{"roles":["Prepared by","Approved by"],"date":true}},{"type":"page_break"},{"type":"end"}]}'
req POST /api/v1/report-templates/compile "{\"spec\":$SPEC}" >/dev/null; check "compile spec" "$(st)" 200
grep -q 'title-block(d, title: \\"Fridge report \\" + tok(d, \\"scope.asset\\")' "$TMP/b" && ok "tokens compiled" || bad "tokens: $(head -c 300 "$TMP/b")"
grep -q 'landscape: true' "$TMP/b" && ok "page settings compiled" || bad "page"
grep -q 'daily-table(d, value-key: \\"value\\"' "$TMP/b" && ok "daily block compiled" || bad "daily"
grep -q 'signatures-block(roles: (\\"Prepared by\\", \\"Approved by\\"), date: true)' "$TMP/b" && ok "signatures compiled" || bad "signatures"
BODY=$(node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); process.stdout.write(JSON.stringify(j.body))' "$TMP/b" 2>/dev/null)
if [[ -n "$BODY" ]]; then
  req POST /api/v1/report-templates/preview "{\"kind\":\"observations\",\"locale\":\"en\",\"body\":$BODY}" >/dev/null; check "compiled source renders a PDF" "$(st)" 200
  head -c 4 "$TMP/b" | grep -q "%PDF" && ok "PDF magic" || bad "not a PDF"
fi
req POST /api/v1/report-templates/compile '{"spec":{"blocks":[{"type":"nonsense"}]}}' >/dev/null; check "unknown block rejected" "$(st)" 400
req POST /api/v1/report-templates/compile '{"spec":{"blocks":[{"type":"table","params":{"columns":["Bad Col"]}}]}}' >/dev/null; check "column key validated" "$(st)" 400
req PUT /api/v1/report-templates/observations "{\"locale\":\"en\",\"reason\":\"blocks\",\"spec\":$SPEC}" >/dev/null; check "save spec version" "$(st)" 201
grep -q '"spec":{' "$TMP/b" && ok "saved version carries spec" || bad "spec not stored"
req GET /api/v1/report-templates/observations >/dev/null; grep -q '"builtin":false' "$TMP/b" && grep -q '"landscape":true' "$TMP/b" && ok "override resolves with spec" || bad "override spec"
req GET "/api/v1/report-templates/observations/preview" >/dev/null; check "override preview renders" "$(st)" 200
req DELETE "/api/v1/report-templates/observations?reason=e2e" >/dev/null; check "revert after block test" "$(st)" 200

echo "== audit entries"
req GET "/api/v1/export/audit?format=csv" >/dev/null
for a in "export.read" "report_template.update" "report_template.revert" "branding.update" "report.download"; do grep -q "$a" "$TMP/b" && ok "audit $a" || bad "audit $a missing"; done

echo; echo "passed $pass, failed $fail"
[[ $fail -eq 0 ]]

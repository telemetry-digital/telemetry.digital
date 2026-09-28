#!/usr/bin/env bash
# End-to-end checks for period tables (spec 023): a fridge's daily log (min, max, samples outside 3–7 °C) and the
# consumption of two electricity meters per month (start, end, consumption, total), local periods in the
# organization's time zone; exports as XLSX, CSV and PDF with the audit trail; validation; the dashboard widget and a
# public link that shows only the bound datastreams. Needs Go (tools/devfill).
#   BASE=http://127.0.0.1:8080 MQTT=127.0.0.1:1883 USER=admin PASS=admin1234 bash tests/e2e/period_table.sh
set -u
BASE=${BASE:-http://127.0.0.1:8080}
MQTT=${MQTT:-127.0.0.1:1883}
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
# a download: status, content type and the file
dl()   { curl -s -b "$JAR" -D "$TMP/h" -o "$TMP/f" -w '%{http_code}' "$BASE$1" > "$TMP/s"; }
waitfor() { local n=${2:-40}; for i in $(seq "$n"); do eval "$1" && return 0; sleep 0.25; done; return 1; }
SUF=$RANDOM

echo "== setup"
go build -o "$TMP/devfill" ./tools/devfill || { echo "cannot build devfill"; exit 1; }
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req POST /api/v1/sites "{\"name\":\"Pharmacy $SUF\",\"reason\":\"e2e\"}"; SITE=$(jx j.id)
req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"fridge\",\"name\":\"Fridge $SUF\",\"reason\":\"e2e\"}"; FA=$(jx j.id)
req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"machine\",\"name\":\"Meters $SUF\",\"reason\":\"e2e\"}"; MA=$(jx j.id)
req POST /api/v1/datastreams "{\"asset_id\":\"$FA\",\"key\":\"temp\",\"quantity\":\"temperature\",\"unit\":\"\\u00b0C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}"; T=$(jx j.id)
req POST /api/v1/datastreams "{\"asset_id\":\"$MA\",\"key\":\"e_a\",\"quantity\":\"energy_active_import\",\"unit\":\"kWh\",\"kind\":\"counter\",\"reason\":\"e2e\"}"; EA=$(jx j.id)
req POST /api/v1/datastreams "{\"asset_id\":\"$MA\",\"key\":\"e_b\",\"quantity\":\"energy_active_import\",\"unit\":\"kWh\",\"kind\":\"counter\",\"reason\":\"e2e\"}"; EB=$(jx j.id)
req POST /api/v1/devices "{\"external_id\":\"pt-$SUF\",\"name\":\"pt $SUF\",\"reason\":\"e2e\"}"; DEV=$(jx j.id)
for d in $T $EA $EB; do req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$d\",\"reason\":\"e2e\"}"; done
req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}'; TOK=$(jx j.token)
[[ -n "$T" && -n "$EA" && -n "$EB" && -n "$TOK" ]] && ok "fridge and two meters" || bad "setup: $(head -c 200 "$TMP/b")"
# fridge: 13 readings every 6 h over 3 days, 2.0 … 8.0 °C; meters: 49 readings every hour, +2 and +1 kWh
"$TMP/devfill" -addr "$MQTT" -user "pt-$SUF" -token "$TOK" -key temp -start 2 -step 0.5 -n 13 -every 21600 > "$TMP/f1.log" 2>&1 && \
"$TMP/devfill" -addr "$MQTT" -user "pt-$SUF" -token "$TOK" -key e_a -start 1000 -step 2 -n 49 -every 3600 > "$TMP/f2.log" 2>&1 && \
"$TMP/devfill" -addr "$MQTT" -user "pt-$SUF" -token "$TOK" -key e_b -start 50 -step 1 -n 49 -every 3600 > "$TMP/f3.log" 2>&1 && ok "backdated readings published" || bad "devfill: $(tail -2 "$TMP"/f*.log)"
FRIDGE="/api/v1/period-table?datastreams=$T&interval=day&values=min,max,out&period=last_7d&low=3&high=7&total=1"
waitfor "req GET '$FRIDGE'; [[ \"\$(jx 'j.totals[\"$T\"].count')\" == 13 ]]" 60 && ok "all fridge readings counted" || bad "count: $(head -c 300 "$TMP/b")"

echo "== fridge: daily log"
req GET "$FRIDGE"
check "a row per day of the last 7 days" "$(jx 'j.interval + " " + j.periods.length + " " + j.values.join()')" "day 7 min,max,out"
check "totals: min, max, samples outside 3–7 °C" "$(jx "[j.totals['$T'].min, j.totals['$T'].max, j.totals['$T'].out].join()")" "2,8,4"
check "days add up" "$(jx "j.cells['$T'].reduce((a, c) => a + c.count, 0) + ' ' + j.cells['$T'].reduce((a, c) => a + c.out, 0)")" "13 4"
check "every day's min ≤ max, empty days without values" "$(jx "j.cells['$T'].every(c => c.count ? c.min <= c.max : c.min === undefined)")" true
check "days start at local midnight of the organization" "$(jx 'j.periods.every(p => new Date(p.start).toLocaleString("sv-SE", {timeZone: j.tz}).endsWith("00:00:00"))')" true
check "consecutive local days" "$(jx 'j.periods.every((p, i) => i === 0 || p.start === j.periods[i - 1].end)')" true
check "column label and unit" "$(jx "j.columns[0].label + ' [' + j.columns[0].unit + ']'")" "Fridge $SUF / temp [°C]"
# the same days as the local dates of the readings (computed here in the organization's time zone)
check "readings fall on their local days" "$(jx "j.cells['$T'].filter(c => c.count).length >= 3 && j.cells['$T'].filter(c => c.count).length <= 4")" true

echo "== meters: consumption"
METERS="/api/v1/period-table?datastreams=$EA,$EB&interval=month&values=first,last,delta&period=last_12m&orientation=datastreams&total=1&label=Main&label=Heating"
req GET "$METERS"; check "a column per month of the last 12" "$(st) $(jx 'j.periods.length + " " + j.columns.map(c => c.label).join()')" "200 12 Main,Heating"
check "consumption over the range" "$(jx "[j.totals['$EA'].delta, j.totals['$EB'].delta].join()")" "96,48"
check "start and end readings" "$(jx "[j.totals['$EA'].first, j.totals['$EA'].last].join()")" "1000,1096"
req GET "/api/v1/period-table?datastreams=$EA&interval=day&values=delta&period=last_7d"
check "daily consumption adds up to the same" "$(jx "Math.round(j.cells['$EA'].reduce((a, c) => a + (c.delta || 0), 0))")" 96
req GET "/api/v1/period-table?datastreams=$EA&interval=hour&values=delta&period=today"; check "hours of today" "$(st) $(jx 'j.periods.length >= 23 && j.periods.length <= 25')" "200 true"
req GET "/api/v1/period-table?datastreams=$EA&values=delta&period=this_month&shift=-1"; check "previous month by shift" "$(st) $(jx 'j.periods.length >= 28 && j.periods.length <= 31')" "200 true"
req GET "/api/v1/period-table?datastreams=$EA&values=delta&period=custom&from=2026-01-01&to=2026-01-31"; check "custom range" "$(st) $(jx 'j.label + " " + j.periods.length')" "200 2026-01 31"

echo "== validation"
req GET "/api/v1/period-table?datastreams=$EA&interval=minute"; check "unknown interval" "$(st)" 400
req GET "/api/v1/period-table?datastreams=$EA&values=median"; check "unknown value" "$(st)" 400
req GET "/api/v1/period-table?datastreams=$EA&interval=hour&period=last_12m"; check "too many rows refused" "$(st)" 400
req GET "/api/v1/period-table?datastreams=01a0e90b-0000-7000-8000-000000000000"; check "unknown datastream" "$(st)" 404
req GET "/api/v1/period-table?datastreams=$EA&period=custom&from=2026-02-01&to=2026-01-01"; check "custom to before from" "$(st)" 400
curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/period-table/export?datastreams=$EA" > "$TMP/s"; check "export needs a signed-in user" "$(st)" 401

echo "== exports"
Q="datastreams=$T&interval=day&values=min,max&period=last_7d&low=3&high=7&total=1&title=Teploty%20liekov%20$SUF"
dl "/api/v1/period-table/export?$Q&format=xlsx"
check "Excel file" "$(st) $(head -c 2 "$TMP/f") $(grep -ci 'spreadsheetml.sheet' "$TMP/h")" "200 PK 1"
grep -q 'filename="Teploty-liekov-' "$TMP/h" && ok "file name from the title" || bad "file name: $(grep -i disposition "$TMP/h")"
grep -aq 'xl/worksheets/sheet1.xml' "$TMP/f" && ok "workbook parts" || bad "not a workbook"
dl "/api/v1/period-table/export?$Q&format=csv"
check "CSV with BOM" "$(st) $(head -c 3 "$TMP/f" | od -An -tx1 | tr -d ' \n')" "200 efbbbf"
check "CSV header" "$(sed -n 1p "$TMP/f" | tr -d '\r' | sed 's/^\xEF\xBB\xBF//')" "Period,Fridge $SUF / temp min [°C],Fridge $SUF / temp max [°C]"
check "CSV rows: 7 days and the total" "$(tail -n +2 "$TMP/f" | wc -l | tr -d ' ') $(tail -1 "$TMP/f" | cut -d, -f1-3 | tr -d '\r')" "8 Total,2.00,8.00"
dl "/api/v1/period-table/export?$Q&format=pdf"
if [[ "$(st)" == 503 ]]; then echo "  skip PDF (reports not configured on this server)"; else check "PDF report" "$(st) $(head -c 4 "$TMP/f")" "200 %PDF"; fi
dl "/api/v1/period-table/export?$Q&format=docx"; check "unknown format" "$(st)" 400
req GET "/api/v1/export/audit?format=csv"
check "exports audited" "$(grep -c 'period_table' "$TMP/b" | awk '{print ($1 >= 2)}')" 1

echo "== dashboard widget and public link"
SPEC="{\"version\":1,\"kind\":\"dashboard\",\"layout\":{\"type\":\"grid\",\"row_height\":96},\"range\":\"24h\",\"refresh_s\":60,\"widgets\":[{\"id\":\"log\",\"type\":\"period_table\",\"geometry\":{\"x\":0,\"y\":0,\"w\":12,\"h\":5},\"bindings\":{\"datastreams\":[\"$T\"]},\"params\":{\"title\":\"Teploty\",\"interval\":\"day\",\"values\":\"min,max\",\"period\":\"last_7d\",\"low\":3,\"high\":7,\"total\":true}}]}"
req POST /api/v1/dashboards "{\"name\":\"Lieky $SUF\",\"spec\":$SPEC,\"reason\":\"e2e\"}"; check "dashboard with a period table" "$(st)" 201
SLUG=$(jx j.slug)
req POST /api/v1/dashboards "{\"name\":\"Bad $SUF\",\"spec\":{\"version\":1,\"kind\":\"dashboard\",\"layout\":{\"type\":\"grid\",\"row_height\":96},\"widgets\":[{\"id\":\"x\",\"type\":\"period_table\",\"geometry\":{\"x\":0,\"y\":0,\"w\":6,\"h\":3},\"bindings\":{\"datastreams\":[\"$T\"]},\"params\":{\"interval\":\"minute\"}}]},\"reason\":\"e2e\"}"
check "widget options validated" "$(st)" 400
req POST "/api/v1/dashboards/$SLUG/share" '{"reason":"e2e public"}'; TOKEN=$(jx j.token); check "public link" "$(st)" 201
curl -s -o "$TMP/b" -w '%{http_code}' "$BASE/api/v1/public/$TOKEN/period-table?datastreams=$T&period=last_7d&values=min,max" > "$TMP/s"
check "public link: the bound fridge" "$(st) $(jx "j.totals['$T'].count")" "200 13"
curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/public/$TOKEN/period-table?datastreams=$EA" > "$TMP/s"; check "public link: another datastream refused" "$(st)" 403
curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/public/$TOKEN/period-table/export?datastreams=$T" > "$TMP/s"; check "no export over the public link" "$(st)" 404
req DELETE "/api/v1/dashboards/$SLUG/share?reason=e2e" ; req DELETE "/api/v1/dashboards/$SLUG?reason=e2e%20cleanup"

echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

#!/usr/bin/env bash
# End-to-end checks for scheduled reports by e-mail (spec 029): the choices of the form, cron presets and the next runs
# in the organization's time zone, validation, a summary of object counting, a period table of a dashboard and a report
# template as attachments (XLSX, PDF when Typst is installed, CSV for templates), "send now" with the delivery log, a
# scheduled run, the audit trail and permissions.
#   BASE=http://127.0.0.1:8080 USER=admin PASS=admin1234 MAIL_DIR=.dev/mail bash tests/e2e/scheduled_reports.sh
# MAIL_DIR: the directory of an SMTP sink (go run ./tools/smtpsink) the server mails to; without it the e-mails are
# not checked. WAIT_SCHEDULE=1 also waits (up to two minutes) for a run on the schedule.
set -u
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER:-admin}
PASS=${PASS:-admin1234}
MAIL_DIR=${MAIL_DIR:-}
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
tok()  { curl -s -X "$1" -H "Authorization: Bearer $2" -H "Content-Type: application/json" -o "$TMP/b" -w '%{http_code}' ${4:+--data-binary "$4"} "$BASE$3" > "$TMP/s"; }
dev()  { curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -H "Authorization: Bearer $1" -d "$2" "$BASE/api/v1/d/telemetry"; }
waitfor() { local n=${2:-40}; for i in $(seq "$n"); do eval "$1" && return 0; sleep 0.25; done; return 1; }
SUF=$RANDOM
TO="reports-$SUF@example.com"

echo "== setup: an area with counts, a dashboard with a period table, an e-mail channel"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req POST /api/v1/sites "{\"name\":\"Reports $SUF\",\"reason\":\"e2e\"}"; SITE=$(jx j.id)
req POST /api/v1/counting/areas "{\"site_id\":\"$SITE\",\"name\":\"Office $SUF\",\"entrances\":[\"Front\",\"Back\"],\"reason\":\"e2e\"}"; AREA=$(jx j.id)
req GET /api/v1/counting/areas; FRONT=$(jx "j.areas.find(a => a.id === '$AREA').entrances[0].id")
req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"sensor\",\"name\":\"Counter $SUF\",\"reason\":\"e2e\"}"; SA=$(jx j.id)
req POST /api/v1/datastreams "{\"asset_id\":\"$SA\",\"key\":\"pin\",\"quantity\":\"count\",\"unit\":\"\",\"kind\":\"gauge\",\"reason\":\"e2e\"}"; PIN=$(jx j.id)
req POST /api/v1/devices "{\"external_id\":\"cnt-$SUF\",\"name\":\"cnt $SUF\",\"reason\":\"e2e\"}"; DEV=$(jx j.id)
req POST "/api/v1/devices/$DEV/datastreams" "{\"datastream_id\":\"$PIN\",\"reason\":\"e2e\"}"
req POST "/api/v1/devices/$DEV/token" '{"reason":"e2e"}'; DTOK=$(jx j.token)
req POST /api/v1/counting/inputs "{\"entrance_id\":\"$FRONT\",\"name\":\"Barrier\",\"in_datastream_id\":\"$PIN\",\"reason\":\"e2e\"}"
dev "$DTOK" '{"pin":4}' >/dev/null
waitfor "req GET '/api/v1/counting/summary?area=$AREA'; [[ \"\$(jx 'j.totals.people && j.totals.people.in')\" == 4 ]]" 40 && ok "4 people counted today" || bad "counts: $(head -c 200 "$TMP/b")"
SPEC="{\"version\":1,\"kind\":\"dashboard\",\"layout\":{\"type\":\"grid\",\"row_height\":96},\"widgets\":[{\"id\":\"log\",\"type\":\"period_table\",\"geometry\":{\"x\":0,\"y\":0,\"w\":12,\"h\":5},\"bindings\":{\"datastreams\":[\"$PIN\"]},\"params\":{\"title\":\"Passages\",\"interval\":\"day\",\"values\":\"sum\",\"period\":\"this_month\",\"total\":true}}]}"
req POST /api/v1/dashboards "{\"name\":\"Counts $SUF\",\"spec\":$SPEC,\"reason\":\"e2e\"}"; check "dashboard with a period table" "$(st)" 201; SLUG=$(jx j.slug)
req POST /api/v1/notification-channels "{\"name\":\"Managers $SUF\",\"kind\":\"email\",\"settings\":{\"recipients\":[\"boss-$SUF@example.com\",\"$TO\"]},\"reason\":\"e2e\"}"; CH=$(jx j.id)
[[ -n "$CH" && "$CH" != null ]] && ok "e-mail channel" || bad "channel: $(cat "$TMP/b")"

echo "== form choices, cron"
req GET /api/v1/scheduled-reports/options
check "areas, period tables, channels, presets" "$(jx "[j.areas.some(a => a.id === '$AREA'), j.period_tables.some(t => t.dashboard === '$SLUG' && t.widget === 'log'), j.channels.some(c => c.id === '$CH' && c.recipients === 2), j.presets.monday_0700].join()")" "true,true,true,0 7 * * 1"
SMTP=$(jx j.smtp); PDF=$(jx j.pdf); TZ=$(jx j.tz)
req POST /api/v1/scheduled-reports/preview-cron '{"cron":"monday_0700","count":3}'
check "next runs of a preset: Mondays 07:00 local" "$(st) $(jx 'j.cron + " " + j.next.length + " " + j.next.every(t => new Date(t).toLocaleString("en-GB", {timeZone: j.tz, weekday: "short", hour: "2-digit", minute: "2-digit", hour12: false}).startsWith("Mon") && t.includes("T07:00:00"))')" "200 0 7 * * 1 3 true"
req POST /api/v1/scheduled-reports/preview-cron '{"cron":"0 7 * *"}'; check "four fields refused" "$(st)" 400
req POST /api/v1/scheduled-reports/preview-cron '{"cron":"0 25 * * *"}'; check "hour 25 refused" "$(st)" 400
req POST /api/v1/scheduled-reports/preview-cron '{"cron":"*/30 6-7 * * MON-FRI","count":4}'; check "ranges, steps and day names" "$(jx 'j.next.map(t => t.slice(11, 16)).join()')" "06:00,06:30,07:00,07:30"

echo "== validation"
BODY="\"content\":{\"type\":\"counting\",\"area_id\":\"$AREA\"},\"period\":\"previous_day\",\"cron\":\"daily_0700\""
req POST /api/v1/scheduled-reports "{\"name\":\"x\",$BODY,\"recipients\":{\"addresses\":[]},\"reason\":\"e2e\"}"; check "no recipients refused" "$(st)" 400
req POST /api/v1/scheduled-reports "{\"name\":\"x\",$BODY,\"recipients\":{\"addresses\":[\"not-an-address\"]},\"reason\":\"e2e\"}"; check "bad address refused" "$(st)" 400
req POST /api/v1/scheduled-reports "{\"name\":\"x\",\"content\":{\"type\":\"counting\",\"area_id\":\"$AREA\"},\"period\":\"yesterday\",\"cron\":\"daily_0700\",\"recipients\":{\"addresses\":[\"$TO\"]},\"reason\":\"e2e\"}"; check "unknown period refused" "$(st)" 400
req POST /api/v1/scheduled-reports "{\"name\":\"x\",\"content\":{\"type\":\"counting\",\"area_id\":\"01a0e90b-0000-7000-8000-000000000000\"},\"period\":\"previous_day\",\"cron\":\"daily_0700\",\"recipients\":{\"addresses\":[\"$TO\"]},\"reason\":\"e2e\"}"; check "unknown area refused" "$(st)" 400
req POST /api/v1/scheduled-reports "{\"name\":\"x\",\"content\":{\"type\":\"period_table\",\"dashboard\":\"$SLUG\",\"widget\":\"nope\"},\"period\":\"previous_day\",\"cron\":\"daily_0700\",\"recipients\":{\"addresses\":[\"$TO\"]},\"reason\":\"e2e\"}"; check "unknown widget refused" "$(st)" 400
req POST /api/v1/scheduled-reports "{\"name\":\"x\",$BODY,\"recipients\":{\"addresses\":[\"$TO\"]},\"attach_pdf\":false,\"attach_xlsx\":false,\"reason\":\"e2e\"}"; check "no attachment refused" "$(st)" 400
req POST /api/v1/scheduled-reports "{\"name\":\"x\",$BODY,\"recipients\":{\"addresses\":[\"$TO\"]}}"; check "reason required" "$(st)" 400

echo "== a daily summary of object counting"
req POST /api/v1/scheduled-reports "{\"name\":\"Office daily $SUF\",$BODY,\"recipients\":{\"channels\":[\"$CH\"],\"addresses\":[\"$TO\",\"extra-$SUF@example.com\"]},\"attach_pdf\":$([[ $PDF == true ]] && echo true || echo false),\"attach_xlsx\":true,\"reason\":\"e2e daily report for the managers\"}"
check "created" "$(st)" 201; R1=$(jx j.id)
check "next run tomorrow or today at 07:00 local" "$(jx "new Date(j.next_run_at).toLocaleString('en-GB', {timeZone: j.tz, hour: '2-digit', minute: '2-digit', hour12: false})")" "07:00"
req GET /api/v1/scheduled-reports; check "listed with cron and time zone" "$(jx "(r => r.cron + ' ' + r.tz + ' ' + r.period)(j.reports.find(r => r.id === '$R1'))")" "0 7 * * * $TZ previous_day"
BEFORE=$(ls "$MAIL_DIR" 2>/dev/null | wc -l)
req POST "/api/v1/scheduled-reports/$R1/send"
if [[ "$SMTP" == true ]]; then
  check "sent now to 3 different addresses" "$(st) $(jx 'j.status + " " + j.recipients')" "200 sent 3"
else
  check "without SMTP the run fails with a reason" "$(st) $(jx 'j.status + " " + j.error.includes("SMTP")')" "200 failed true"
fi
check "attachments: XLSX$([[ $PDF == true ]] && echo ' and PDF')" "$(jx 'j.attachments.map(a => a.name.split(".").pop()).sort().join()')" "$([[ $PDF == true ]] && echo 'pdf,xlsx' || echo xlsx)"
check "attachments with size and SHA-256" "$(jx 'j.attachments.every(a => a.bytes > 500 && a.sha256.length === 64)')" true
check "the period is the previous local day" "$(jx "(new Date(j.period_to) - new Date(j.period_from)) / 3600000 >= 23 && new Date(j.period_to).toLocaleString('en-GB', {timeZone: '$TZ', hour: '2-digit', minute: '2-digit', hour12: false}) === '00:00'")" true
req GET "/api/v1/scheduled-reports/$R1/runs"; check "delivery log: a manual run" "$(jx 'j.length + " " + j[0].trigger + " " + j[0].actor')" "1 manual $USER"
if [[ -n "$MAIL_DIR" && "$SMTP" == true ]]; then
  waitfor "[[ \$(ls '$MAIL_DIR' | wc -l) -ge $((BEFORE + 3)) ]]" 40 && ok "three e-mails in the sink" || bad "mails: $(ls "$MAIL_DIR" | wc -l) (before $BEFORE)"
  M=$(grep -l "extra-$SUF@example.com" "$MAIL_DIR"/*.eml | head -1)
  grep -q "Subject: .*Office.daily.$SUF" "$M" && ok "subject with the report name" || bad "subject: $(grep -i subject "$M")"
  grep -q 'filename=".*\.xlsx"' "$M" && ok "XLSX attached" || bad "no xlsx in $M"
  grep -qi "multipart/mixed" "$M" && ok "a multipart e-mail" || bad "not multipart"
fi

echo "== a period table of a dashboard and a report template"
req POST /api/v1/scheduled-reports "{\"name\":\"Passages weekly $SUF\",\"content\":{\"type\":\"period_table\",\"dashboard\":\"$SLUG\",\"widget\":\"log\"},\"period\":\"previous_week\",\"cron\":\"monday_0700\",\"recipients\":{\"addresses\":[\"$TO\"]},\"attach_pdf\":false,\"reason\":\"e2e\"}"; R2=$(jx j.id); check "period table report" "$(st)" 201
req POST "/api/v1/scheduled-reports/$R2/send"; check "made with the period table export" "$(jx 'j.attachments.length + " " + j.attachments[0].name.startsWith("Passages") + " " + j.attachments[0].name.endsWith(".xlsx")')" "1 true true"
req POST /api/v1/scheduled-reports "{\"name\":\"Raw monthly $SUF\",\"content\":{\"type\":\"template\",\"kind\":\"observations\",\"datastream_id\":\"$PIN\"},\"period\":\"previous_month\",\"cron\":\"monthly_0700\",\"recipients\":{\"addresses\":[\"$TO\"]},\"attach_pdf\":false,\"reason\":\"e2e\"}"; R3=$(jx j.id); check "template report" "$(st)" 201
req POST "/api/v1/scheduled-reports/$R3/send"; check "observations as CSV" "$(jx 'j.attachments.length + " " + j.attachments[0].name.endsWith(".csv")')" "1 true"
check "the previous month" "$(jx "new Date(j.period_from).toLocaleString('en-GB', {timeZone: '$TZ', day: '2-digit'})")" "01"

echo "== change, schedule, remove"
req PUT "/api/v1/scheduled-reports/$R1" "{\"name\":\"Office daily $SUF\",$BODY,\"recipients\":{\"addresses\":[\"$TO\"]},\"enabled\":false,\"attach_pdf\":false,\"reason\":\"e2e pause\"}"; check "disabled" "$(st) $(jx 'j.next_run_at')" "200 null"
if [[ "${WAIT_SCHEDULE:-}" == 1 ]]; then
  req PUT "/api/v1/scheduled-reports/$R2" "{\"name\":\"Passages weekly $SUF\",\"content\":{\"type\":\"period_table\",\"dashboard\":\"$SLUG\",\"widget\":\"log\"},\"period\":\"previous_day\",\"cron\":\"* * * * *\",\"recipients\":{\"addresses\":[\"$TO\"]},\"attach_pdf\":false,\"reason\":\"e2e every minute\"}"; check "every minute" "$(st)" 200
  waitfor "req GET '/api/v1/scheduled-reports/$R2/runs'; [[ \"\$(jx 'j.some(r => r.trigger === \"schedule\")')\" == true ]]" 600 && ok "run on the schedule by the scheduler" || bad "no scheduled run: $(head -c 300 "$TMP/b")"
  req PUT "/api/v1/scheduled-reports/$R2" "{\"name\":\"Passages weekly $SUF\",\"content\":{\"type\":\"period_table\",\"dashboard\":\"$SLUG\",\"widget\":\"log\"},\"period\":\"previous_week\",\"cron\":\"monday_0700\",\"recipients\":{\"addresses\":[\"$TO\"]},\"attach_pdf\":false,\"reason\":\"e2e back to weekly\"}"
else
  echo "  skip waiting for a scheduled run (WAIT_SCHEDULE=1)"
fi
req POST /api/v1/tokens '{"name":"reader","scopes":["data.read"]}'; RT=$(jx j.token)
tok GET "$RT" /api/v1/scheduled-reports; check "a reader cannot see scheduled reports" "$(st)" 403
tok POST "$RT" "/api/v1/scheduled-reports/$R1/send"; check "a reader cannot send one" "$(st)" 403
for r in $R1 $R2 $R3; do req DELETE "/api/v1/scheduled-reports/$r?reason=e2e"; done; check "removed" "$(st)" 200
req GET /api/v1/scheduled-reports; check "not listed any more" "$(jx "j.reports.some(r => r.id === '$R1')")" false
req GET "/api/v1/export/audit?format=csv"
for a in scheduled_report.create scheduled_report.send scheduled_report.update scheduled_report.delete; do grep -q "$a" "$TMP/b" && ok "audited: $a" || bad "not audited: $a"; done
req DELETE "/api/v1/notification-channels/$CH?reason=e2e"; req DELETE "/api/v1/dashboards/$SLUG?reason=e2e"

echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

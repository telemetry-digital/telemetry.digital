#!/usr/bin/env bash
# A scheduled report whose run was interrupted is run once more (0.98.1, audit). The interruption is staged in the
# database (a claim older than half an hour that was never recorded), so the test needs psql and is not part of
# tests/run-all.sh. The scheduler looks every 30 s. Without SMTP the repeated run is recorded as failed — it is the
# record that is checked.
#   BASE=http://127.0.0.1:8080 DB_ADMIN=postgres://… bash tests/restart/report_recovery.sh
set -u
cd "$(dirname "$0")/../.."
BASE=${BASE:-http://127.0.0.1:8080}
USER=${USER_NAME:-admin}
PASS=${PASS:-admin1234}
DB=${DB_ADMIN:-postgres://ctrl32:ctrl32-dev-only@127.0.0.1:5433/ctrl32_dev}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
JAR="$TMP/jar"; CSRF=""
pass=0; fail=0
st()   { cat "$TMP/s"; }
ok()   { pass=$((pass+1)); echo "  ok   $1"; }
bad()  { fail=$((fail+1)); echo "  FAIL $1"; }
check(){ if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got: $2, want: $3)"; fi; }
jx()   { node -e 'const j=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const r=eval(process.argv[2]); console.log(typeof r==="string"?r:JSON.stringify(r===undefined?null:r))' "$TMP/b" "$1"; }
req()  { local m=$1 p=$2 d=${3:-}
  if [[ -n "$d" ]]; then printf '%s' "$d" > "$TMP/req"; curl -s -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' --data-binary @"$TMP/req" "$BASE$p" > "$TMP/s"
  else curl -s -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" -o "$TMP/b" -w '%{http_code}' "$BASE$p" > "$TMP/s"; fi; }
sql()  { psql -v ON_ERROR_STOP=1 -Atq "$DB" -c "$1" 2>&1; }
SUF=$RANDOM

echo "== setup"
req POST /api/v1/auth/login "{\"username\":\"$USER\",\"password\":\"$PASS\"}"; check "login" "$(st)" 200
req GET /api/v1/auth/me; CSRF=$(jx j.csrf_token)
req POST /api/v1/sites "{\"name\":\"Report site $SUF\",\"reason\":\"e2e\"}"; SITE=$(jx j.id)
req POST /api/v1/assets "{\"site_id\":\"$SITE\",\"type\":\"room\",\"name\":\"Report room $SUF\",\"reason\":\"e2e\"}"; ASSET=$(jx j.id)
req POST /api/v1/datastreams "{\"asset_id\":\"$ASSET\",\"key\":\"t\",\"quantity\":\"temperature\",\"unit\":\"C\",\"kind\":\"gauge\",\"reason\":\"e2e\"}"; DS=$(jx j.id)
mk() { req POST /api/v1/scheduled-reports "{\"name\":\"$1 $SUF\",\"content\":{\"type\":\"template\",\"kind\":\"observations\",\"datastream_id\":\"$DS\"},\"period\":\"previous_day\",\"cron\":\"daily_0700\",\"recipients\":{\"addresses\":[\"r-$SUF@example.com\"]},\"attach_pdf\":false,\"reason\":\"e2e\"}"; jx j.id; }
ONCE=$(mk "Interrupted once"); TWICE=$(mk "Interrupted twice"); DUE=$(mk "Due now")
[[ -n "$ONCE" && -n "$TWICE" && -n "$DUE" ]] && ok "three reports" || bad "reports: $(cat "$TMP/b")"

echo "== staged: two interrupted runs and one report that is due"
sql "UPDATE core.scheduled_report SET running_at = now() - interval '40 minutes', running_for = now() - interval '40 minutes', running_try = 0 WHERE id = '$ONCE'" > /dev/null
sql "UPDATE core.scheduled_report SET running_at = now() - interval '40 minutes', running_for = now() - interval '80 minutes', running_try = 1 WHERE id = '$TWICE'" > /dev/null
sql "UPDATE core.scheduled_report SET next_run_at = now() - interval '1 minute' WHERE id = '$DUE'" > /dev/null
for _ in $(seq 45); do [[ "$(sql "SELECT count(*) FROM core.scheduled_report_run WHERE report_id IN ('$ONCE', '$TWICE', '$DUE')")" == 3 ]] && break; sleep 1; done

check "the run interrupted once is repeated and recorded" "$(sql "SELECT count(*) || ' ' || bool_and(actor LIKE '%repeated%') FROM core.scheduled_report_run WHERE report_id = '$ONCE'")" "1 true"
check "for the time it was scheduled for" "$(sql "SELECT (r.period_to < now() - interval '30 minutes') FROM core.scheduled_report_run r WHERE report_id = '$ONCE'")" "t"
check "the run interrupted twice is recorded as failed, not tried again" "$(sql "SELECT status || ' ' || (error LIKE '%interrupted twice%') FROM core.scheduled_report_run WHERE report_id = '$TWICE'")" "failed true"
check "the due report ran once" "$(sql "SELECT count(*) || ' ' || bool_and(actor = 'scheduler') FROM core.scheduled_report_run WHERE report_id = '$DUE'")" "1 true"
check "no run is left marked as in progress" "$(sql "SELECT count(*) FROM core.scheduled_report WHERE id IN ('$ONCE', '$TWICE', '$DUE') AND (running_at IS NOT NULL OR running_try <> 0)")" "0"
check "the due report has its next run" "$(sql "SELECT next_run_at > now() FROM core.scheduled_report WHERE id = '$DUE'")" "t"
sleep 32
check "nothing runs a second time" "$(sql "SELECT count(*) FROM core.scheduled_report_run WHERE report_id IN ('$ONCE', '$TWICE', '$DUE')")" "3"
for r in $ONCE $TWICE $DUE; do req DELETE "/api/v1/scheduled-reports/$r?reason=e2e"; done

echo; echo "passed $pass, failed $fail"; [[ $fail -eq 0 ]]

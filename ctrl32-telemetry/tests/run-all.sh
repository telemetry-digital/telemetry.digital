#!/usr/bin/env bash
# Runs every automated check against a fresh database and a freshly built server, in the order the suites need:
#   unit      go vet, go test -race
#   c         host tests of ctrl32-telemetry-c (C99, AddressSanitizer + UBSan)
#   e2e       the suites in tests/e2e, iam.sh last (it needs a one-time password and enforced MFA)
#   headless  the browser probes in tests/headless (skipped when no Chrome/Chromium is found)
#
#   bash tests/run-all.sh                 # everything
#   bash tests/run-all.sh e2e headless    # only these phases
#
# Needs Go, Node 22+, curl, psql and a PostgreSQL 18 server (dev: docker compose -f deploy/dev/docker-compose.yml up -d).
# The database TEST_DB is DROPPED and recreated; never point PG_ADMIN_URL at production.
# Environment:
#   PG_ADMIN_URL  superuser URL of the server (default: the dev compose on port 5433)
#   TEST_DB       database to recreate (default ctrl32_e2e)
#   SKIP          space-separated suite names to skip, e.g. SKIP="ops" where NTP (udp/123) is blocked
#   CHROME        browser for the headless probes (default: first Chrome/Chromium found)
#   KEEP=1        leave the server running afterwards
# Ports 8080 (HTTP), 1883 and 1884 (MQTT) must be free: the suites assume them.
set -u
cd "$(dirname "$0")/.."
ROOT=$PWD
PG_ADMIN_URL=${PG_ADMIN_URL:-postgres://ctrl32:ctrl32-dev-only@127.0.0.1:5433/postgres?sslmode=disable}
TEST_DB=${TEST_DB:-ctrl32_e2e}
SKIP=" ${SKIP:-} "
PHASES=${*:-unit c e2e headless}
BASE=http://127.0.0.1:8080
WORK=$(mktemp -d)
LOGS=$ROOT/test-results
mkdir -p "$LOGS"
SERVER_PID=""
declare -a RESULTS=()
failed=0

say()  { printf '\n\033[1m== %s\033[0m\n' "$*"; }
res()  { RESULTS+=("$(printf '%-24s %s' "$1" "$2")"); [[ $2 == ok* || $2 == skipped* ]] || failed=1; }
has()  { [[ " $PHASES " == *" $1 "* ]]; }
skip() { [[ "$SKIP" == *" $1 "* ]]; }
cleanup() {
  if [[ -n "$SERVER_PID" && "${KEEP:-}" != 1 ]]; then kill "$SERVER_PID" 2>/dev/null; wait "$SERVER_PID" 2>/dev/null; fi
  rm -rf "$WORK"
}
trap cleanup EXIT

# The database URL of TEST_DB with the given credentials (swaps user, password and database of PG_ADMIN_URL).
db_url() { # user password database
  local rest=${PG_ADMIN_URL#*://}; rest=${rest#*@}
  local hostport=${rest%%/*} query=""
  [[ "$rest" == *\?* ]] && query="?${rest#*\?}"
  printf 'postgres://%s:%s@%s/%s%s' "$1" "$2" "$hostport" "$3" "$query"
}
admin_user() { local r=${PG_ADMIN_URL#*://}; r=${r%%@*}; printf '%s' "${r%%:*}"; }
admin_pass() { local r=${PG_ADMIN_URL#*://}; r=${r%%@*}; [[ "$r" == *:* ]] && printf '%s' "${r#*:}"; }

# ------------------------------------------------------------------------------------------------ unit
if has unit; then
  say "unit: go vet, go test -race"
  if go vet ./... > "$LOGS/go-vet.log" 2>&1; then res "go vet" ok; else res "go vet" "FAIL (test-results/go-vet.log)"; fi
  if go test -race -count=1 ./... > "$LOGS/go-test.log" 2>&1; then res "go test -race" ok
  else res "go test -race" "FAIL (test-results/go-test.log)"; grep -E '^(--- FAIL|FAIL)' "$LOGS/go-test.log" | head -20; fi
fi

# ------------------------------------------------------------------------------------------------ c
if has c; then
  say "c: ctrl32-telemetry-c host tests"
  C=../ctrl32-telemetry-c
  if [[ ! -d $C ]]; then res "c core tests" "skipped (no ../ctrl32-telemetry-c)"
  elif ${CC:-cc} -std=c99 -Wall -Wextra -Werror -fsanitize=address,undefined -fno-sanitize-recover=all \
        -I"$C/src" "$C"/src/*.c "$C/tests/core_test.c" -o "$WORK/core_test" -lm > "$LOGS/c-tests.log" 2>&1 \
       && "$WORK/core_test" >> "$LOGS/c-tests.log" 2>&1; then res "c core tests" "ok ($(tail -1 "$LOGS/c-tests.log"))"
  else res "c core tests" "FAIL (test-results/c-tests.log)"; tail -5 "$LOGS/c-tests.log"; fi
fi

if ! has e2e && ! has headless; then
  say "summary"; printf '%s\n' "${RESULTS[@]}"; exit $failed
fi

# ------------------------------------------------------------------------------------------------ server
say "server: fresh database $TEST_DB, build, migrate, seed"
if curl -s -o /dev/null --max-time 2 "$BASE/healthz"; then echo "something already listens on $BASE; stop it first" >&2; exit 2; fi
PGPASSWORD=$(admin_pass) psql -q -v ON_ERROR_STOP=1 "$PG_ADMIN_URL" \
  -c "DROP DATABASE IF EXISTS $TEST_DB WITH (FORCE)" -c "CREATE DATABASE $TEST_DB OWNER $(admin_user)" || exit 2
DB_ADMIN=$(db_url "$(admin_user)" "$(admin_pass)" "$TEST_DB")
if [[ $(psql -Atq "$DB_ADMIN" -c "SHOW server_version_num") -lt 180000 ]]; then
  # PostgreSQL < 18 has no uuidv7(): a stand-in keeps the suites usable, but the result does not prove PostgreSQL 18.
  echo "WARNING: PostgreSQL < 18, using a uuidv7() stand-in"
  psql -q "$DB_ADMIN" <<'SQL'
CREATE FUNCTION public.uuidv7() RETURNS uuid LANGUAGE sql VOLATILE AS $$
  SELECT (lpad(to_hex(floor(extract(epoch FROM clock_timestamp()) * 1000)::bigint), 12, '0')
          || '7' || substr(h, 1, 3) || substr('89ab', 1 + (get_byte(decode(substr(h, 4, 2), 'hex'), 0) & 3), 1)
          || substr(h, 6, 15))::uuid
  FROM (SELECT md5(random()::text || clock_timestamp()::text) h) r
$$;
SQL
fi

mkdir -p "$WORK/bin" "$WORK/reports" "$WORK/firmware"
BIN=$WORK/bin/ctrl32-telemetry
# test builds trust the test license key (tests/keys/README.md); release builds do not
LICKEY=$(grep -v '^#' tests/keys/license-test.pub | tr -d '[:space:]')
go build -ldflags "-X github.com/telemetry-digital/ctrl32-telemetry/internal/license.extraKey=$LICKEY" -o "$BIN" ./cmd/ctrl32-telemetry || exit 2
go run ./tools/pmtilesgen -o "$WORK/bin/sample.pmtiles" -maxzoom 4 > /dev/null || exit 2
CONFIG=$WORK/config.toml
APP_PW=e2e-app-$RANDOM$RANDOM
sed -e "s|^url = .*|url = \"$(db_url ctrl32_app "$APP_PW" "$TEST_DB")\"|" \
    -e "s|^admin_url = .*|admin_url = \"$DB_ADMIN\"|" \
    -e "s|^app_password = .*|app_password = \"$APP_PW\"|" \
    -e "s|REPLACE-WITH-RANDOM-32-CHARS|e2e-$RANDOM$RANDOM-not-a-secret-0123456789|" \
    -e "s|^\[report\]$|[report]\nstorage_dir = \"$WORK/reports\"|" \
    -e "s|^\[fota\]$|[fota]\nstorage_dir = \"$WORK/firmware\"|" \
    -e "s|^\[map\]$|[map]\npmtiles = \"$WORK/bin/sample.pmtiles\"\nattribution = \"sample tiles (e2e)\"|" \
    config.example.toml > "$CONFIG"

"$BIN" migrate -config "$CONFIG" > "$LOGS/migrate.log" 2>&1 || { tail "$LOGS/migrate.log"; exit 2; }
"$BIN" dev seed -config "$CONFIG" > "$LOGS/seed.log" 2>&1 || { tail "$LOGS/seed.log"; exit 2; }
"$BIN" admin bootstrap -config "$CONFIG" > "$LOGS/bootstrap.log" 2>&1 || { tail "$LOGS/bootstrap.log"; exit 2; }
"$BIN" admin reset-password -username admin -password admin1234 -permanent -allow-weak -config "$CONFIG" > /dev/null || exit 2
ORG=$(psql -Atq "$DB_ADMIN" -c "SELECT slug FROM core.organization ORDER BY created_at LIMIT 1")
set_mfa_optional() { # true|false — the dev organization does not force MFA enrolment while the suites run
  psql -q "$DB_ADMIN" -c "UPDATE core.organization SET settings = jsonb_set(coalesce(settings, '{}'), '{security}',
    coalesce(settings->'security', '{}') || '{\"mfa_optional\": $1}') WHERE slug = '$ORG'"
}
set_mfa_optional true
"$BIN" admin seed-dashboard -org-slug "$ORG" -kind showcase -default -config "$CONFIG" > /dev/null || exit 2
SHOWCASE=$(psql -Atq "$DB_ADMIN" -c "SELECT slug FROM core.dashboard WHERE slug LIKE 'showcase%' ORDER BY slug LIMIT 1")

# The login limiter allows 10 attempts a minute per address, so the server restarts before each suite.
start_server() {
  [[ -n "$SERVER_PID" ]] && { kill "$SERVER_PID" 2>/dev/null; wait "$SERVER_PID" 2>/dev/null; }
  "$BIN" serve -config "$CONFIG" >> "$LOGS/server.log" 2>&1 &
  SERVER_PID=$!
  for _ in $(seq 60); do curl -s -o /dev/null --max-time 1 "$BASE/healthz" && return 0; sleep 0.25; done
  echo "server did not start, see test-results/server.log" >&2; exit 2
}
: > "$LOGS/server.log"
export BASE USER=admin PASS=admin1234 MQTT=127.0.0.1:1883 BIN CONFIG REPORTS_DIR=$WORK/reports

# ------------------------------------------------------------------------------------------------ e2e
if has e2e; then
  say "e2e"
  for f in tests/e2e/*.sh; do
    n=$(basename "$f" .sh)
    [[ $n == iam ]] && continue
    if skip "$n"; then res "e2e $n" skipped; continue; fi
    start_server
    timeout 600 bash "$f" > "$LOGS/e2e-$n.log" 2>&1; rc=$?
    okc=$(grep -c '^  ok' "$LOGS/e2e-$n.log"); bad=$(grep -c 'FAIL' "$LOGS/e2e-$n.log")
    if [[ $rc == 0 && $bad == 0 ]]; then res "e2e $n" "ok ($okc)"; else res "e2e $n" "FAIL ($bad of $((okc + bad)), test-results/e2e-$n.log)"; fi
    printf '%-18s %s\n' "$n" "${RESULTS[-1]#* }"
  done
fi

# ------------------------------------------------------------------------------------------------ headless
if has headless; then
  say "headless"
  CH=${CHROME:-}
  if [[ -z "$CH" ]]; then
    for c in google-chrome google-chrome-stable chromium chromium-browser /opt/pw-browsers/chromium-*/chrome-linux/chrome; do
      CH=$(command -v "$c" 2>/dev/null) && [[ -n "$CH" ]] && break; CH=""
    done
  fi
  if [[ -z "$CH" ]]; then res "headless" "skipped (no Chrome/Chromium; set CHROME)"
  else
    # No sandbox: containers and CI runners (root, user namespaces off) cannot start the Chrome sandbox.
    printf '#!/bin/sh\nexec "%s" --no-sandbox "$@"\n' "$CH" > "$WORK/chrome"; chmod +x "$WORK/chrome"
    mkdir -p "$LOGS/screenshots"
    for f in tests/headless/*.js; do
      n=$(basename "$f" .js)
      if skip "$n"; then res "headless $n" skipped; continue; fi
      start_server
      CHROME=$WORK/chrome TEMP=$WORK OUT=$LOGS/screenshots LOGIN=1 SLUG=$SHOWCASE SLUGS=$SHOWCASE \
        timeout 180 node "$f" > "$LOGS/headless-$n.log" 2>&1; rc=$?
      probs=$(grep '^problems:' "$LOGS/headless-$n.log" | grep -v 'problems: none' | head -1)
      [[ $n == dash ]] && ! grep -q 'widgets=[1-9]' "$LOGS/headless-$n.log" && probs="no widgets rendered"
      if [[ $rc == 0 && -z "$probs" ]] && grep -q 'problems' "$LOGS/headless-$n.log"; then res "headless $n" ok
      else res "headless $n" "FAIL (${probs:-exit $rc}, test-results/headless-$n.log)"; fi
      printf '%-18s %s\n' "$n" "${RESULTS[-1]#* }"
    done
  fi
fi

# ------------------------------------------------------------------------------------------------ iam (last)
if has e2e && ! skip iam; then
  say "e2e iam (one-time password, enforced MFA)"
  set_mfa_optional false
  OTP=$("$BIN" admin reset-password -username admin -config "$CONFIG" 2>&1 | grep -o '[^ ]*$' | tail -1)
  start_server
  if timeout 600 bash tests/e2e/iam.sh "$BASE" admin "$OTP" > "$LOGS/e2e-iam.log" 2>&1; then res "e2e iam" "ok ($(grep -c PASS "$LOGS/e2e-iam.log"))"
  else res "e2e iam" "FAIL (test-results/e2e-iam.log)"; grep FAIL "$LOGS/e2e-iam.log" | head -5; fi
fi

say "summary"
printf '%s\n' "${RESULTS[@]}"
[[ $failed == 0 ]] && echo "ALL CHECKS PASSED" || echo "SOME CHECKS FAILED (logs in test-results/)"
exit $failed

#!/usr/bin/env bash
# Development environment of ctrl32 telemetry on a PC (Linux, macOS, Windows with Git Bash). See deploy/dev/README.md.
#
#   bash deploy/dev/dev.sh up        # PostgreSQL 18 in Docker, a fresh ctrl32_dev database, demo seed, server on :8080
#   bash deploy/dev/dev.sh restart   # rebuild and restart the server, keep the data
#   bash deploy/dev/dev.sh camsim    # simulated cameras (RTSP :8554, HTTP/ONVIF :8555)
#   bash deploy/dev/dev.sh demo      # demo cameras, video walls, production screens, floor plan (needs camsim)
#   bash deploy/dev/dev.sh stop      # stop server, camera simulator and the production feeder
#
# Without Docker (NO_DOCKER=1, or when the daemon is not running) a PostgreSQL 16+ at PG_ADMIN_URL is used.
# Sign in at http://127.0.0.1:8080 as admin / admin1234. Everything local lives in ctrl32-telemetry/.dev (git-ignored).
set -euo pipefail
cd "$(dirname "$0")/../.."
R=$(pwd)
D=$R/.dev
mkdir -p "$D"
PG_ADMIN_URL=${PG_ADMIN_URL:-postgres://ctrl32:ctrl32-dev-only@127.0.0.1:5433/postgres?sslmode=disable}
BIN=$D/c32
[[ "$(uname -s)" == MINGW* || "$(uname -s)" == MSYS* ]] && BIN=$D/c32.exe

log() { printf '\033[1;34m[dev]\033[0m %s\n' "$*"; }
psqlq() { # psql locally, or inside the development container
  if command -v psql >/dev/null 2>&1 || [[ -n "${NO_DOCKER:-}" ]]; then psql -q "${1}" -c "$2"
  else docker exec -i ctrl32-pg-dev psql -q -U ctrl32 -d "$(basename "${1%%\?*}")" -c "$2"; fi
}
pidfile() { echo "$D/$1.pid"; }
start_bg() { # name, command...
  local n=$1; shift
  stop_one "$n"
  nohup "$@" > "$D/$n.log" 2>&1 &
  echo $! > "$(pidfile "$n")"
}
stop_one() { local f; f=$(pidfile "$1"); if [[ -f $f ]]; then kill "$(cat "$f")" 2>/dev/null || true; rm -f "$f"; fi; }
wait_http() { for _ in $(seq 60); do curl -s -o /dev/null "$1" && return 0; sleep 0.5; done; echo "no answer from $1 (see $D/server.log)"; return 1; }

build() { log "building"; go build -o "$BIN" ./cmd/ctrl32-telemetry; }
serve() { start_bg server "$BIN" serve -config "$D/config.toml"; wait_http http://127.0.0.1:8080/healthz; log "server on http://127.0.0.1:8080 (log: $D/server.log)"; }

case "${1:-}" in
  up)
    if [[ -z "${NO_DOCKER:-}" ]] && command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
      docker compose -f deploy/dev/docker-compose.yml up -d
      for _ in $(seq 60); do docker exec ctrl32-pg-dev pg_isready -U ctrl32 -q 2>/dev/null && break; sleep 1; done
    fi
    stop_one server
    log "fresh database ctrl32_dev"
    psqlq "$PG_ADMIN_URL" "DROP DATABASE IF EXISTS ctrl32_dev WITH (FORCE)"
    psqlq "$PG_ADMIN_URL" "CREATE DATABASE ctrl32_dev OWNER ctrl32"
    DEVURL=${PG_ADMIN_URL/\/postgres\?/\/ctrl32_dev?}
    # PostgreSQL before 18 has no uuidv7(): a stand-in for development only
    psqlq "$DEVURL" "DO \$do\$ BEGIN IF current_setting('server_version_num')::int < 180000 THEN
      EXECUTE \$f\$CREATE FUNCTION public.uuidv7() RETURNS uuid LANGUAGE sql VOLATILE AS \$b\$
        SELECT (lpad(to_hex(floor(extract(epoch FROM clock_timestamp())*1000)::bigint),12,'0')||'7'||substr(h,1,3)||substr('89ab',1+(get_byte(decode(substr(h,4,2),'hex'),0)&3),1)||substr(h,6,15))::uuid
        FROM (SELECT md5(random()::text||clock_timestamp()::text) h) r \$b\$\$f\$; END IF; END \$do\$"
    mkdir -p "$D/reports" "$D/firmware" "$D/video"
    sed -e "s|^url = .*|url = \"postgres://ctrl32_app:devpw@127.0.0.1:5433/ctrl32_dev?sslmode=disable\"|" \
        -e "s|^admin_url = .*|admin_url = \"$DEVURL\"|" \
        -e 's|^app_password = .*|app_password = "devpw"|' -e 's|REPLACE-WITH-RANDOM-32-CHARS|dev-not-a-secret-0123456789abcdef|' \
        -e "s|^\[report\]$|[report]\nstorage_dir = \"$D/reports\"|" -e "s|^\[fota\]$|[fota]\nstorage_dir = \"$D/firmware\"|" \
        -e "s|^# push_listen = \":8322\".*|push_listen = \"127.0.0.1:8322\"|" \
        config.example.toml > "$D/config.toml"
    sed -i.bak "s|^\[video\]$|[video]\nstorage_dir = \"$D/video\"|" "$D/config.toml" && rm -f "$D/config.toml.bak"
    build
    "$BIN" migrate -config "$D/config.toml" >/dev/null
    "$BIN" dev seed -config "$D/config.toml" > "$D/seed.txt"
    "$BIN" admin bootstrap -config "$D/config.toml" >/dev/null
    "$BIN" admin reset-password -username admin -password admin1234 -permanent -allow-weak -config "$D/config.toml" >/dev/null
    psqlq "$DEVURL" "UPDATE core.organization SET settings = jsonb_set(coalesce(settings,'{}'),'{security}',coalesce(settings->'security','{}')||'{\"mfa_optional\":true}')"
    serve
    J=$D/cookies.txt
    curl -s -c "$J" -b "$J" -H 'Content-Type: application/json' -d '{"username":"admin","password":"admin1234"}' http://127.0.0.1:8080/api/v1/auth/login >/dev/null
    CSRF=$(curl -s -b "$J" http://127.0.0.1:8080/api/v1/auth/me | node -pe 'JSON.parse(require("fs").readFileSync(0)).csrf_token')
    curl -s -b "$J" -X PUT -H "X-CSRF-Token: $CSRF" -H 'Content-Type: application/json' \
      -d '{"enabled":["sk","cs","en"],"default":"sk","reason":"dev"}' http://127.0.0.1:8080/api/v1/i18n/languages >/dev/null
    log "admin / admin1234 — seed details in $D/seed.txt"
    ;;
  restart) build; serve ;;
  camsim)
    go build -o "$D/camsim" ./tools/camsim
    start_bg camsim "$D/camsim" -rtsp :8554 -http :8555 -fps 8 -onvif BRANA,VRATNICA -motion-every 45s
    log "camera simulator: rtsp://127.0.0.1:8554/<label>/<h264|mjpeg>/<W>x<H>[/opus|/g711] (log: $D/camsim.log)"
    ;;
  demo)
    cd deploy/dev/demo
    python3 cameras.py
    python3 production.py setup
    python3 floorplan.py
    start_bg production python3 production.py live
    log "demo ready; the production feeder sends data every 10 s (log: $D/production.log)"
    ;;
  stop) for n in production camsim server; do stop_one "$n"; done; log "stopped" ;;
  *) sed -n '2,12p' "$0"; exit 2 ;;
esac

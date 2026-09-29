#!/usr/bin/env bash
# ctrl32 telemetry — one-command installer for Linux (Debian/Ubuntu tested; systemd required).
#
#   curl -fsSL https://telemetry.digital/install.sh | sudo bash -s -- --domain telemetry.digital --demo
#
# What it does (idempotent, re-run safe):
#   1. creates the system user and directories
#   2. installs the single binary (downloaded, or --binary PATH for a local build)
#   3. starts PostgreSQL 18 (Docker by default, --db apt for a native PGDG install)
#   4. writes /etc/ctrl32-telemetry/config.toml with random passwords (kept on re-run)
#   5. applies schema migrations, installs and starts the systemd service
#   6. with --domain: runs Caddy (Docker, host network) for automatic HTTPS and syncs the certificate for MQTT/TLS 8883
#   7. with --demo: seeds a demo organization and prints the device token
#   8. with --cloudflare-token: installs cloudflared (systemd service) connected to your Cloudflare Tunnel instead of Caddy;
#      the server listens on 127.0.0.1 only, no port is opened (docs/cloudflare-tunnel.md)
#
# Options:
#   --domain NAME        public hostname (enables profile "internet", Caddy, MQTT TLS on 8883)
#   --binary PATH        use a local binary instead of downloading
#   --url URL            download URL for the binary (default: https://telemetry.digital/dl/ctrl32-telemetry-linux-ARCH)
#   --db docker|apt      database installation method (default: docker if available, else apt)
#   --db-port N          host port for the Docker PostgreSQL (default 5432, or 5433 if 5432 is taken)
#   --public-read        allow unauthenticated read-only API/UI (demo instances only)
#   --demo               seed demo data and print the device token
#   --cloudflare-token T reach the server through a Cloudflare Tunnel (needs --domain = the tunnel's public hostname)
#   --no-start           install everything but do not start the service
set -euo pipefail

APP=ctrl32-telemetry
BIN=/usr/local/bin/$APP
ETC=/etc/$APP
LIB=/var/lib/$APP
LOG=/var/log/$APP
OPT=/opt/$APP
USER_NAME=ctrl32
DOMAIN=""
BINARY=""
URL=""
DB_MODE=""
DB_PORT=""
PUBLIC_READ=false
DEMO=false
START=true
CF_TOKEN=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain) DOMAIN="$2"; shift 2;;
    --binary) BINARY="$2"; shift 2;;
    --url) URL="$2"; shift 2;;
    --db) DB_MODE="$2"; shift 2;;
    --db-port) DB_PORT="$2"; shift 2;;
    --public-read) PUBLIC_READ=true; shift;;
    --demo) DEMO=true; shift;;
    --no-start) START=false; shift;;
    --cloudflare-token) CF_TOKEN="$2"; shift 2;;
    -h|--help) sed -n '2,25p' "$0"; exit 0;;
    *) echo "unknown option: $1" >&2; exit 2;;
  esac
done

log()  { printf '\033[1;34m[ctrl32]\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m[ctrl32] error:\033[0m %s\n' "$*" >&2; exit 1; }
rand() { head -c 32 /dev/urandom | base64 | tr -d '/+=\n' | head -c 32; }

[[ $EUID -eq 0 ]] || die "run as root (sudo)"
command -v systemctl >/dev/null || die "systemd is required"
ARCH=$(uname -m); case "$ARCH" in x86_64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; *) die "unsupported architecture $ARCH";; esac
[[ -z "$CF_TOKEN" || -n "$DOMAIN" ]] || die "--cloudflare-token needs --domain: the public hostname of the tunnel"

# 1. user and directories --------------------------------------------------------------------------
if ! id -u "$USER_NAME" >/dev/null 2>&1; then
  useradd --system --home-dir "$LIB" --shell /usr/sbin/nologin "$USER_NAME"
  log "created system user $USER_NAME"
fi
install -d -m 0750 -o root -g "$USER_NAME" "$ETC" "$ETC/tls"
install -d -m 0750 -o "$USER_NAME" -g "$USER_NAME" "$LIB" "$LOG"
install -d -m 0755 "$OPT"

# 2. binary ----------------------------------------------------------------------------------------
if [[ -n "$BINARY" ]]; then
  install -m 0755 "$BINARY" "$BIN"
  log "installed binary from $BINARY"
else
  URL=${URL:-"https://telemetry.digital/dl/$APP-linux-$ARCH"}
  TMP=$(mktemp)
  curl -fsSL "$URL" -o "$TMP" || die "download failed: $URL"
  install -m 0755 "$TMP" "$BIN"; rm -f "$TMP"
  log "installed binary from $URL"
fi
"$BIN" version

# 2b. Typst (PDF reports; Apache-2.0, pinned release) --------------------------------------------
TYPST_VERSION=${TYPST_VERSION:-v0.15.1}
TYPST_BIN=/usr/local/bin/typst
if ! [[ -x "$TYPST_BIN" ]] || ! "$TYPST_BIN" --version 2>/dev/null | grep -q "${TYPST_VERSION#v}"; then
  case "$ARCH" in amd64) TARCH=x86_64;; arm64) TARCH=aarch64;; esac
  TURL="https://github.com/typst/typst/releases/download/$TYPST_VERSION/typst-$TARCH-unknown-linux-musl.tar.xz"
  TMPD=$(mktemp -d)
  if curl -fsSL "$TURL" -o "$TMPD/typst.tar.xz" && tar -xJf "$TMPD/typst.tar.xz" -C "$TMPD"; then
    install -m 0755 "$(find "$TMPD" -type f -name typst | head -1)" "$TYPST_BIN"
    log "installed typst $("$TYPST_BIN" --version) for PDF reports"
  else
    log "WARNING: could not download typst ($TURL); PDF reports will be unavailable until it is installed"
  fi
  rm -rf "$TMPD"
fi
install -d -m 0750 -o "$USER_NAME" -g "$USER_NAME" "$LIB/reports"
install -d -m 0750 -o root -g "$USER_NAME" "$LIB/backups"

# 3. database --------------------------------------------------------------------------------------
if [[ -z "$DB_MODE" ]]; then
  if command -v docker >/dev/null 2>&1; then DB_MODE=docker; else DB_MODE=apt; fi
fi
DB_NAME=ctrl32; DB_ADMIN=ctrl32; DB_HOST=127.0.0.1
ENVFILE="$ETC/db.env"
if [[ -f "$ENVFILE" ]]; then
  # shellcheck disable=SC1090
  source "$ENVFILE"
else
  DB_ADMIN_PASSWORD=$(rand); DB_APP_PASSWORD=$(rand)
  if [[ -z "$DB_PORT" ]]; then
    if ss -ltn 2>/dev/null | grep -q ':5432 '; then DB_PORT=5433; else DB_PORT=5432; fi
  fi
  umask 077
  cat > "$ENVFILE" <<EOF
DB_PORT=$DB_PORT
DB_ADMIN_PASSWORD=$DB_ADMIN_PASSWORD
DB_APP_PASSWORD=$DB_APP_PASSWORD
EOF
  umask 022
  chown root:"$USER_NAME" "$ENVFILE"; chmod 0640 "$ENVFILE"
fi

case "$DB_MODE" in
  docker)
    cat > "$OPT/docker-compose.yml" <<EOF
name: ${APP}-db
services:
  postgres:
    image: postgres:18
    container_name: ${APP}-postgres
    restart: unless-stopped
    environment:
      POSTGRES_USER: $DB_ADMIN
      POSTGRES_PASSWORD: $DB_ADMIN_PASSWORD
      POSTGRES_DB: $DB_NAME
    ports:
      - "127.0.0.1:${DB_PORT}:5432"
    volumes:
      - ${APP}-postgres:/var/lib/postgresql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U $DB_ADMIN -d $DB_NAME"]
      interval: 5s
      timeout: 3s
      retries: 30
volumes:
  ${APP}-postgres:
    name: ${APP}-postgres
EOF
    docker compose -f "$OPT/docker-compose.yml" up -d postgres
    log "waiting for PostgreSQL"
    for i in $(seq 1 60); do
      if docker exec "${APP}-postgres" psql -U "$DB_ADMIN" -d "$DB_NAME" -tAc "select 1" >/dev/null 2>&1; then break; fi
      sleep 1
    done
    ;;
  apt)
    if ! command -v psql >/dev/null 2>&1 || ! psql --version | grep -q ' 18'; then
      log "installing PostgreSQL 18 from PGDG"
      apt-get update -qq && apt-get install -y -qq curl ca-certificates gnupg lsb-release >/dev/null
      install -d /usr/share/postgresql-common/pgdg
      curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc
      echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list
      apt-get update -qq && apt-get install -y -qq postgresql-18 >/dev/null
    fi
    DB_PORT=${DB_PORT:-5432}
    sudo -u postgres psql -v ON_ERROR_STOP=1 -tAc "DO \$\$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='$DB_ADMIN') THEN CREATE ROLE $DB_ADMIN LOGIN SUPERUSER PASSWORD '$DB_ADMIN_PASSWORD'; END IF; END \$\$;" >/dev/null
    sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='$DB_NAME'" | grep -q 1 || sudo -u postgres createdb -O "$DB_ADMIN" "$DB_NAME"
    ;;
  *) die "--db must be docker or apt";;
esac
log "PostgreSQL ready on $DB_HOST:$DB_PORT ($DB_MODE)"

# 4. config ----------------------------------------------------------------------------------------
CONFIG="$ETC/config.toml"
if [[ -n "$DOMAIN" ]]; then PROFILE=internet; HTTP_LISTEN="127.0.0.1:8080"; BASE_URL="https://$DOMAIN"; else PROFILE=intranet; HTTP_LISTEN=":8080"; BASE_URL="http://$(hostname -f 2>/dev/null || hostname):8080"; fi
if [[ ! -f "$CONFIG" ]]; then
  umask 027
  cat > "$CONFIG" <<EOF
# ctrl32 telemetry configuration — generated by install.sh on $(date -Is). Edit and restart the service.
profile = "$PROFILE"

[http]
listen = "$HTTP_LISTEN"
base_url = "$BASE_URL"
public_read = $PUBLIC_READ

[mqtt]
listen = "127.0.0.1:1883"
ws_listen = "127.0.0.1:1884"
$( [[ -n "$DOMAIN" && -z "$CF_TOKEN" ]] && printf 'tls_listen = ":8883"\ntls_cert = "%s/tls/fullchain.pem"\ntls_key = "%s/tls/privkey.pem"\n' "$ETC" "$ETC" )

[db]
url = "postgres://ctrl32_app:${DB_APP_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"
admin_url = "postgres://${DB_ADMIN}:${DB_ADMIN_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"
app_password = "${DB_APP_PASSWORD}"

[smtp]
port = 587

[log]
level = "info"
file = "$LOG/server.log"

[gap]
check_interval = "1m"

[security]
pepper = "$(rand)"
secret_key = "$(rand)"

[report]
typst_path = "$TYPST_BIN"
storage_dir = "$LIB/reports"

[agent]
socket = "/run/$APP/agent.sock"

# Self-hosted map tiles for the map widgets (no Internet needed): a raster PMTiles archive.
# [map]
# pmtiles = "$LIB/map.pmtiles"
# attribution = "© OpenStreetMap contributors"
EOF
  umask 022
  chown root:"$USER_NAME" "$CONFIG"; chmod 0640 "$CONFIG"
  log "wrote $CONFIG"
else
  log "keeping existing $CONFIG"
  if ! grep -q '^\[security\]' "$CONFIG"; then
    printf '\n[security]\npepper = "%s"\nsecret_key = "%s"\n' "$(rand)" "$(rand)" >> "$CONFIG"
    log "added [security] secrets to existing config"
  fi
  if ! grep -q '^\[report\]' "$CONFIG"; then
    printf '\n[report]\ntypst_path = "%s"\nstorage_dir = "%s/reports"\n' "$TYPST_BIN" "$LIB" >> "$CONFIG"
    log "added [report] section to existing config"
  fi
  if ! grep -q '^\[agent\]' "$CONFIG"; then
    printf '\n[agent]\nsocket = "/run/%s/agent.sock"\n' "$APP" >> "$CONFIG"
    log "added [agent] section to existing config"
  fi
fi

# 5. migrations + service --------------------------------------------------------------------------
"$BIN" migrate -config "$CONFIG"

cat > /etc/systemd/system/$APP.service <<EOF
[Unit]
Description=ctrl32 telemetry
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
User=$USER_NAME
Group=$USER_NAME
ExecStart=$BIN serve -config $CONFIG
Restart=always
RestartSec=3
# hardening
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=$LIB $LOG /run/
ProtectKernelTunables=yes
ProtectControlGroups=yes
RestrictSUIDSGID=yes
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
# 5b. privileged agent (ADR-0012): host operations for the UI — restart, domain/TLS, WireGuard, updates
cat > /etc/systemd/system/$APP-agent.service <<EOF
[Unit]
Description=ctrl32 telemetry server agent (privileged operations for the UI)
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=$BIN agent -config $CONFIG
Restart=always
RestartSec=3
RuntimeDirectory=$APP
RuntimeDirectoryMode=0755
RuntimeDirectoryPreserve=yes

[Install]
WantedBy=multi-user.target
EOF
if command -v apt-get >/dev/null 2>&1 && ! command -v wg >/dev/null 2>&1; then
  apt-get install -y -qq wireguard-tools >/dev/null 2>&1 && log "installed wireguard-tools (VPN for devices, configured in the UI)" || log "WARNING: wireguard-tools not installed; the UI installs it when WireGuard is enabled"
fi
systemctl daemon-reload
systemctl enable "$APP" >/dev/null
if $START; then systemctl restart "$APP"; sleep 2; systemctl --no-pager --lines=5 status "$APP" || true; fi
systemctl enable "$APP-agent" >/dev/null
if $START; then systemctl restart "$APP-agent"; fi

# 6. reverse proxy with automatic HTTPS + MQTT certificate sync --------------------------------------
if [[ -n "$CF_TOKEN" ]]; then
  # Cloudflare Tunnel instead of Caddy: cloudflared connects out to Cloudflare and forwards to 127.0.0.1:8080
  log "installing cloudflared (Cloudflare Tunnel)"
  curl -fsSL -o /usr/local/bin/cloudflared.new "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-$ARCH" || die "cannot download cloudflared"
  chmod 0755 /usr/local/bin/cloudflared.new
  if systemctl list-unit-files cloudflared.service >/dev/null 2>&1 && systemctl cat cloudflared >/dev/null 2>&1; then cloudflared service uninstall >/dev/null 2>&1 || true; fi
  mv /usr/local/bin/cloudflared.new /usr/local/bin/cloudflared
  cloudflared service install "$CF_TOKEN" >/dev/null 2>&1 || die "cloudflared service install failed (check the token)"
  systemctl enable --now cloudflared >/dev/null 2>&1 || true
  log "Cloudflare Tunnel connected; in Cloudflare the public hostname $DOMAIN must point to http://localhost:8080"
elif [[ -n "$DOMAIN" ]]; then
  command -v docker >/dev/null 2>&1 || die "--domain requires Docker for Caddy (native TLS/ACME in the binary comes in a later version)"
  SSLIP="$(curl -fsS4 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}').sslip.io"
  if [[ -f "$OPT/caddy.json" ]]; then log "Caddy is managed from the UI ($OPT/caddy.json) — keeping its configuration"; else
  cat > "$OPT/Caddyfile" <<EOF
# ctrl32 telemetry — generated by install.sh
$DOMAIN, $SSLIP {
	encode gzip
	header {
		Strict-Transport-Security "max-age=31536000"
		X-Content-Type-Options nosniff
		Referrer-Policy strict-origin-when-cross-origin
		-Server
	}
	# published installer and binaries (deploy/publish.sh copies them to /var/www/ctrl32); 404 until published
	handle /install.sh {
		root * /srv/ctrl32
		file_server
	}
	handle /install.ps1 {
		root * /srv/ctrl32
		file_server
	}
	handle_path /dl/* {
		root * /srv/ctrl32/dl
		file_server browse
	}
	reverse_proxy 127.0.0.1:8080
}

# additional sites: $OPT/caddy-sites/*.caddy
import sites/*.caddy
EOF
  install -d -m 0755 "$OPT/caddy-sites"
  cat > "$OPT/docker-compose.caddy.yml" <<EOF
name: ${APP}-caddy
services:
  caddy:
    image: caddy:2-alpine
    container_name: ${APP}-caddy
    restart: unless-stopped
    network_mode: host
    volumes:
      - $OPT/Caddyfile:/etc/caddy/Caddyfile:ro
      - $OPT/caddy-sites:/etc/caddy/sites:ro
      - /var/www/ctrl32:/srv/ctrl32:ro
      - ${APP}-caddy-data:/data
      - ${APP}-caddy-config:/config
volumes:
  ${APP}-caddy-data:
    name: ${APP}-caddy-data
  ${APP}-caddy-config:
    name: ${APP}-caddy-config
EOF
  fi
  install -d -m 0755 /var/www/ctrl32/dl
  docker compose -f "$OPT/docker-compose.caddy.yml" up -d caddy
  # copy the Let's Encrypt certificate for the MQTT TLS listener (Caddy cannot proxy raw MQTT)
  cat > "$OPT/mqtt-certs-sync.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
SRC=\$(docker volume inspect ${APP}-caddy-data -f '{{.Mountpoint}}')/caddy/certificates
# prefer the real domain; fall back to the sslip.io certificate until DNS points at this host
for NAME in "$DOMAIN" "$SSLIP"; do
  CRT=\$(find "\$SRC" -path "*/\$NAME/\$NAME.crt" 2>/dev/null | head -1 || true)
  KEY=\$(find "\$SRC" -path "*/\$NAME/\$NAME.key" 2>/dev/null | head -1 || true)
  [[ -n "\$CRT" && -n "\$KEY" ]] && break
done
[[ -n "\$CRT" && -n "\$KEY" ]] || exit 0
if ! cmp -s "\$CRT" $ETC/tls/fullchain.pem 2>/dev/null; then
  install -m 0640 -o root -g $USER_NAME "\$CRT" $ETC/tls/fullchain.pem
  install -m 0640 -o root -g $USER_NAME "\$KEY" $ETC/tls/privkey.pem
  systemctl restart $APP
  logger -t $APP "MQTT TLS certificate updated from Caddy"
fi
EOF
  chmod 0755 "$OPT/mqtt-certs-sync.sh"
  echo "*/10 * * * * root $OPT/mqtt-certs-sync.sh" > /etc/cron.d/$APP-certs
  log "Caddy started for https://$DOMAIN (fallback https://$SSLIP); MQTT certificate sync every 10 minutes"
  if command -v ufw >/dev/null 2>&1 && ufw status | grep -q 'Status: active'; then
    ufw allow 80/tcp >/dev/null; ufw allow 443/tcp >/dev/null; ufw allow 8883/tcp comment 'ctrl32 MQTT TLS' >/dev/null
    log "ufw: opened 80, 443, 8883"
  fi
fi

# 7. first administrator and demo ------------------------------------------------------------------
if ! "$BIN" admin bootstrap -config "$CONFIG" -org-name "${ORG_NAME:-My organization}" -username admin -name "Administrator" > "$ETC/admin-bootstrap.txt" 2>&1; then
  if grep -q "duplicate key" "$ETC/admin-bootstrap.txt"; then log "admin user already exists"; else cat "$ETC/admin-bootstrap.txt"; fi
  rm -f "$ETC/admin-bootstrap.txt"
else
  chmod 0600 "$ETC/admin-bootstrap.txt"
  log "first administrator created — one-time password saved in $ETC/admin-bootstrap.txt (delete after first login)"
fi
if $DEMO; then
  log "seeding demo data"
  "$BIN" dev seed -config "$CONFIG"
fi

log "done. Service: systemctl status $APP · logs: journalctl -u $APP -f · config: $CONFIG"
if [[ -n "$CF_TOKEN" ]]; then log "UI: https://$DOMAIN/ through the Cloudflare Tunnel (no open ports)  ·  MQTT: localhost:1883"
elif [[ -n "$DOMAIN" ]]; then log "UI: https://$DOMAIN/  ·  MQTT: $DOMAIN:8883 (TLS)  ·  after the first certificate: $OPT/mqtt-certs-sync.sh"
else log "UI: $BASE_URL/  ·  MQTT: localhost:1883"; fi

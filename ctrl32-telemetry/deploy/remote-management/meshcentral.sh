#!/usr/bin/env bash
# MeshCentral for remote management of PCs at customers' sites (remote desktop, terminal, files, Windows/Linux
# updates) on a ctrl32 telemetry host, behind its Caddy (docs/remote-management.md). MeshCentral is a separate
# open-source product (Apache-2.0); ctrl32 telemetry itself never controls the operating system (ADR-0012).
#
#   sudo bash meshcentral.sh mesh.example.com [admin-e-mail]
#
# What it does (idempotent):
#   1. /opt/meshcentral/{data,files,backups} and config.json: listens on 127.0.0.1:4430 only (TLS by Caddy,
#      certificate announced to agents via certUrl), no self-registration, two-factor sign-in required,
#      no Intel AMT port, no self-update (the image is updated with docker compose pull)
#   2. runs ghcr.io/ylianst/meshcentral in Docker (host network, restart unless-stopped)
#   3. adds the site to Caddy (/opt/ctrl32-telemetry/caddy-sites/meshcentral.caddy, imported by the Caddyfile)
#   4. creates the administrator "admin" once; the password is in /root/meshcentral-admin.txt (0600)
# The DNS name must point to this host (A record; on Cloudflare "DNS only", not proxied).
set -euo pipefail
HOST=${1:?usage: meshcentral.sh mesh.example.com [admin-e-mail]}
EMAIL=${2:-admin@$HOST}
IMAGE=ghcr.io/ylianst/meshcentral:latest
DIR=/opt/meshcentral
SITES=/opt/ctrl32-telemetry/caddy-sites
log() { printf '\033[1;34m[meshcentral]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[meshcentral] error:\033[0m %s\n' "$*" >&2; exit 1; }
[[ $EUID -eq 0 ]] || die "run as root"
command -v docker >/dev/null || die "Docker is required"
[[ "$HOST" =~ ^[a-z0-9.-]+$ ]] || die "hostname: letters, digits, dots and dashes"
[[ -d /opt/ctrl32-telemetry ]] || die "no ctrl32 telemetry host (Caddy managed from /opt/ctrl32-telemetry)"
grep -q 'import sites/\*.caddy' /opt/ctrl32-telemetry/Caddyfile || die "the Caddyfile does not import $SITES yet: update ctrl32 telemetry (0.63.1+) and save System → Domain and TLS once"

install -d -m 0750 "$DIR" "$DIR/data" "$DIR/files" "$DIR/backups"
if [[ ! -f "$DIR/data/config.json" ]]; then
  cat > "$DIR/data/config.json" <<EOF
{
  "\$schema": "https://raw.githubusercontent.com/Ylianst/MeshCentral/master/meshcentral-config-schema.json",
  "settings": {
    "cert": "$HOST",
    "WANonly": true,
    "port": 4430,
    "portBind": "127.0.0.1",
    "aliasPort": 443,
    "redirPort": 0,
    "mpsPort": 0,
    "tlsOffload": "127.0.0.1",
    "trustedProxy": "127.0.0.1",
    "selfUpdate": false,
    "allowFraming": false,
    "cookieIpCheck": false,
    "agentPing": 60,
    "browserPing": 60,
    "maxInvalidLogin": { "time": 10, "count": 10, "coolofftime": 30 }
  },
  "domains": {
    "": {
      "title": "ctrl32 remote",
      "title2": "remote management",
      "newAccounts": false,
      "certUrl": "https://$HOST:443/",
      "passwordRequirements": { "min": 12, "force2factor": true }
    }
  }
}
EOF
  log "wrote $DIR/data/config.json"
else
  log "keeping $DIR/data/config.json"
fi

cat > "$DIR/docker-compose.yml" <<EOF
name: meshcentral
services:
  meshcentral:
    image: $IMAGE
    container_name: meshcentral
    restart: unless-stopped
    network_mode: host
    volumes:
      - $DIR/data:/opt/meshcentral/meshcentral-data
      - $DIR/files:/opt/meshcentral/meshcentral-files
      - $DIR/backups:/opt/meshcentral/meshcentral-backups
EOF

# the administrator, once (MeshCentral's database must not be open by the server while the account is written)
if [[ ! -f /root/meshcentral-admin.txt ]]; then
  docker compose -f "$DIR/docker-compose.yml" stop meshcentral >/dev/null 2>&1 || true
  PASS=$(head -c 48 /dev/urandom | base64 | tr -d '/+=\n' | head -c 24)
  run() { docker run --rm -v "$DIR/data:/opt/meshcentral/meshcentral-data" -v "$DIR/files:/opt/meshcentral/meshcentral-files" --entrypoint node "$IMAGE" /opt/meshcentral/meshcentral/meshcentral --configfile /opt/meshcentral/meshcentral-data/config.json "$@"; }
  run --createaccount admin --pass "$PASS" --email "$EMAIL" >/dev/null
  run --adminaccount admin >/dev/null
  umask 077
  printf 'MeshCentral https://%s/\nuser: admin\npassword: %s\n(two-factor sign-in is set up at the first sign-in)\n' "$HOST" "$PASS" > /root/meshcentral-admin.txt
  log "administrator created — /root/meshcentral-admin.txt"
fi
docker compose -f "$DIR/docker-compose.yml" up -d meshcentral >/dev/null
log "MeshCentral running on 127.0.0.1:4430"

install -d -m 0755 "$SITES"
cat > "$SITES/meshcentral.caddy" <<EOF
# MeshCentral (remote management) — written by deploy/remote-management/meshcentral.sh
$HOST {
	encode gzip
	header -Server
	reverse_proxy 127.0.0.1:4430
}
EOF
docker exec ctrl32-telemetry-caddy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null
log "Caddy serves https://$HOST/ (the certificate comes on the first request)"

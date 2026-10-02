#!/usr/bin/env bash
# ctrl32 camera relay — installer for a remote site (Linux with systemd: Debian, Ubuntu, Raspberry Pi OS).
#
#   curl -fsSL https://portal.telemetry.digital/install-relay.sh -o install-relay.sh
#   sudo bash install-relay.sh --config relay.toml
#
# relay.toml is the file shown once when the remote camera was created (Cameras -> Add camera -> "The camera sends its
# video to the server"), with the local address of the camera filled in. See docs/video.md, "Remote cameras".
#
# What it does (idempotent, re-run to change the configuration or update the program):
#   1. creates the system user ctrl32relay (no shell, no home)
#   2. installs the program as /usr/local/bin/ctrl32-camera-relay (downloaded and checked against SHA256SUMS, or
#      --binary PATH); a ctrl32 server on the same computer keeps its own binary
#   3. copies relay.toml to /etc/ctrl32-camera-relay/relay.toml (root:ctrl32relay 0640 — it holds camera secrets)
#   4. tests it: the server is reachable and its certificate trusted, every camera answers, the server accepts each
#      secret (stops here on failure unless --force)
#   5. installs and starts the systemd service ctrl32-camera-relay (sandboxed, restarts on failure)
#
# Options:
#   --config FILE   relay configuration to install (required on the first run)
#   --binary PATH   use a local binary instead of downloading
#   --url URL       download URL (default: https://portal.telemetry.digital/dl/ctrl32-telemetry-linux-ARCH)
#   --update        download the current program again (otherwise an installed one is kept)
#   --force         install even when the test fails (e.g. a camera is switched off for now)
#   --check         only test the installed configuration
#   --uninstall     stop and remove the service (the configuration stays unless --purge)
#   --purge         with --uninstall: also remove /etc/ctrl32-camera-relay and the user
set -euo pipefail

NAME=ctrl32-camera-relay
BIN=/usr/local/bin/$NAME
ETC=/etc/$NAME
CFG=$ETC/relay.toml
USER_NAME=ctrl32relay
UNIT=/etc/systemd/system/$NAME.service
CONFIG=""; BINARY=""; URL=""; UPDATE=false; FORCE=false; CHECK=false; UNINSTALL=false; PURGE=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --config) CONFIG="$2"; shift 2;;
    --binary) BINARY="$2"; shift 2;;
    --url) URL="$2"; shift 2;;
    --update) UPDATE=true; shift;;
    --force) FORCE=true; shift;;
    --check) CHECK=true; shift;;
    --uninstall) UNINSTALL=true; shift;;
    --purge) PURGE=true; shift;;
    -h|--help) sed -n '2,30p' "$0"; exit 0;;
    *) echo "unknown option: $1" >&2; exit 2;;
  esac
done

log() { printf '\033[1;34m[relay]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[relay] error:\033[0m %s\n' "$*" >&2; exit 1; }
[[ $EUID -eq 0 ]] || die "run as root (sudo)"
command -v systemctl >/dev/null || die "systemd is required"
check() { runuser -u "$USER_NAME" -- "$BIN" camera-relay -check -config "$CFG"; }

if $UNINSTALL; then
  systemctl disable --now "$NAME" 2>/dev/null || true
  rm -f "$UNIT"; systemctl daemon-reload
  log "service removed"
  if $PURGE; then
    rm -rf "$ETC" "$BIN"; userdel "$USER_NAME" 2>/dev/null || true
    log "configuration, program and user removed"
  else
    log "configuration kept in $ETC (use --purge to remove it)"
  fi
  exit 0
fi
if $CHECK; then
  [[ -f "$CFG" ]] || die "not installed ($CFG missing)"
  check; exit $?
fi

[[ -n "$CONFIG" || -f "$CFG" ]] || die "--config relay.toml is required on the first run"
[[ -z "$CONFIG" || -f "$CONFIG" ]] || die "$CONFIG not found"
ARCH=$(uname -m); case "$ARCH" in x86_64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; *) die "unsupported architecture $ARCH (64-bit Linux needed; Raspberry Pi: 64-bit OS)";; esac

# 1. user -------------------------------------------------------------------------------------------
if ! id -u "$USER_NAME" >/dev/null 2>&1; then
  useradd --system --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin "$USER_NAME"
  log "created system user $USER_NAME"
fi

# 2. program ----------------------------------------------------------------------------------------
if [[ -n "$BINARY" ]]; then
  install -m 0755 "$BINARY" "$BIN"
  log "installed program from $BINARY"
elif [[ ! -x "$BIN" || -n "$URL" ]] || $UPDATE; then
  URL=${URL:-"https://portal.telemetry.digital/dl/ctrl32-telemetry-linux-$ARCH"}
  TMPD=$(mktemp -d); trap 'rm -rf "$TMPD"' EXIT
  curl -fsSL "$URL" -o "$TMPD/bin" || die "download failed: $URL"
  if curl -fsSL "${URL%/*}/SHA256SUMS" -o "$TMPD/SUMS" 2>/dev/null; then
    WANT=$(awk -v f="${URL##*/}" '$2 == f || $2 == "*"f {print $1}' "$TMPD/SUMS")
    [[ -n "$WANT" ]] || die "${URL##*/} is not listed in SHA256SUMS"
    [[ "$(sha256sum "$TMPD/bin" | cut -d' ' -f1)" == "$WANT" ]] || die "checksum mismatch for $URL — not installed"
    log "checksum verified"
  else
    log "WARNING: no SHA256SUMS next to $URL; checksum not verified"
  fi
  install -m 0755 "$TMPD/bin" "$BIN"
  log "installed program from $URL"
fi
"$BIN" version

# 3. configuration ----------------------------------------------------------------------------------
install -d -m 0750 -o root -g "$USER_NAME" "$ETC"
if [[ -n "$CONFIG" ]]; then
  install -m 0640 -o root -g "$USER_NAME" "$CONFIG" "$CFG.new"
  mv -f "$CFG.new" "$CFG"
  log "configuration installed as $CFG (readable only by root and $USER_NAME)"
  log "the file you passed still holds the camera secrets: delete it (shred -u $CONFIG) when done"
fi
if CAF=$(grep -E '^\s*ca_file\s*=' "$CFG" | sed -E 's/^[^=]*=\s*"?([^"#]*)"?.*/\1/' | xargs) && [[ -n "$CAF" ]]; then
  [[ -r "$CAF" ]] || die "ca_file $CAF not found"
fi

# 4. test -------------------------------------------------------------------------------------------
log "testing the configuration"
if ! check; then
  $FORCE || die "the test failed (see above); fix relay.toml and run again, or --force to install anyway"
  log "WARNING: installing despite the failed test (--force)"
fi

# 5. service ----------------------------------------------------------------------------------------
cat > "$UNIT" <<EOF
[Unit]
Description=ctrl32 camera relay (forwards local cameras to the ctrl32 telemetry server)
Documentation=https://telemetry.digital/docs/
After=network-online.target
Wants=network-online.target

[Service]
User=$USER_NAME
Group=$USER_NAME
ExecStart=$BIN camera-relay -config $CFG
Restart=always
RestartSec=5
# sandbox: the relay only reads its configuration and talks to the cameras and the server
NoNewPrivileges=yes
CapabilityBoundingSet=
AmbientCapabilities=
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
PrivateDevices=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectKernelLogs=yes
ProtectControlGroups=yes
ProtectClock=yes
ProtectHostname=yes
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
RestrictNamespaces=yes
RestrictRealtime=yes
RestrictSUIDSGID=yes
LockPersonality=yes
MemoryDenyWriteExecute=yes
SystemCallArchitectures=native
SystemCallFilter=@system-service
SystemCallFilter=~@privileged @resources
UMask=0077

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable "$NAME" >/dev/null
systemctl restart "$NAME"
sleep 2
if systemctl is-active --quiet "$NAME"; then
  log "service $NAME is running — the cameras appear online on the server within seconds"
  log "logs: journalctl -u $NAME -f    test again: sudo bash $0 --check"
else
  journalctl -u "$NAME" -n 20 --no-pager || true
  die "the service did not start (log above)"
fi

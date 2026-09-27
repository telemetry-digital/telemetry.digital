#!/usr/bin/env bash
# Publish the Home Assistant add-on repository as a read-only git repository served over plain HTTPS
# ("dumb" git protocol): https://<host>/dl/ha-addons.git — add that URL in Home Assistant under
# Settings → Add-ons → Add-on store → ⋮ → Repositories.
# Usage (from ctrl32-telemetry/): deploy/homeassistant/publish-addon.sh VERSION user@host [ssh options]
set -euo pipefail
VERSION=${1:?version}
TARGET=${2:?user@host}
shift 2
SSH_OPTS=("$@")
W=$(mktemp -d)
trap 'rm -rf "$W"' EXIT
cp -r deploy/homeassistant/repository.yaml deploy/homeassistant/ctrl32_camera_relay "$W/"
sed -i.bak "s/^version: .*/version: \"$VERSION\"/" "$W/ctrl32_camera_relay/config.yaml" && rm -f "$W/ctrl32_camera_relay/config.yaml.bak"
(
  cd "$W"
  git init -q -b main
  git -c core.autocrlf=false add -A
  git update-index --chmod=+x ctrl32_camera_relay/run.sh
  git -c user.name="ctrl32 telemetry" -c user.email="bugs@ctrl32.com" commit -q -m "ctrl32 camera relay $VERSION"
  git clone -q --bare . ha-addons.git
  git -C ha-addons.git update-server-info
  tar czf ha-addons.tgz ha-addons.git
)
scp "${SSH_OPTS[@]}" "$W/ha-addons.tgz" "$TARGET:/tmp/ha-addons.tgz"
ssh "${SSH_OPTS[@]}" "$TARGET" 'set -e; cd /var/www/ctrl32/dl; rm -rf ha-addons.git.new; mkdir ha-addons.git.new
  tar xzf /tmp/ha-addons.tgz -C ha-addons.git.new --strip-components=1; rm /tmp/ha-addons.tgz
  chmod -R u=rwX,go=rX ha-addons.git.new; rm -rf ha-addons.git; mv ha-addons.git.new ha-addons.git'
echo "published the Home Assistant add-on $VERSION: https://${TARGET#*@}/dl/ha-addons.git (use the public host name)"

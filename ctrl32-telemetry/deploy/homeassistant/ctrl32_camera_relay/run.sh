#!/bin/sh
# Builds relay.toml from the add-on options, tests it, then runs the relay (it reconnects on its own).
set -eu
OPTS=/data/options.json
CONF=/data/relay.toml
umask 077

n=$(jq '.cameras | length' "$OPTS")
if [ "$n" -eq 0 ]; then
  echo "No camera configured. In ctrl32 telemetry: Cameras -> Camera management -> Add camera ->"
  echo "Connection: The camera sends its video to the server. Copy id and secret into this add-on's"
  echo "configuration, with source = rtsp://USER:PASSWORD@CAMERA-IP:554/stream1, and restart the add-on."
  exec sleep infinity
fi

jq -r '
  "server = \(.server | tojson)",
  (if (.ca_file // "") != "" then "ca_file = \(.ca_file | tojson)" else empty end),
  (.cameras[] |
    "",
    "[[camera]]",
    "id = \(.id | tojson)",
    "secret = \(.secret | tojson)",
    "source = \(.source | tojson)",
    (if (.sub_source // "") != "" then "sub_source = \(.sub_source | tojson)" else empty end),
    (if (.transport // "") != "" then "transport = \(.transport | tojson)" else empty end))
' "$OPTS" > "$CONF"

echo "ctrl32 camera relay: $n camera(s) -> $(jq -r .server "$OPTS")"
ctrl32-telemetry camera-relay -check -config "$CONF" || echo "check failed; the relay starts anyway and keeps retrying"
exec ctrl32-telemetry camera-relay -config "$CONF"

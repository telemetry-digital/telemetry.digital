#!/usr/bin/env bash
# Build release binaries and publish them with the installer to a host running ctrl32 telemetry behind Caddy:
#   https://<host>/install.sh, /install-relay.sh (+ .ps1) and  https://<host>/dl/ctrl32-telemetry-linux-{amd64,arm64} (+ SHA256SUMS)
# Usage (from the repository root, Go on PATH):
#   deploy/publish.sh 0.4.0 root@2.29.5.17 [-i ~/.ssh/key]
# The host directory /var/www/ctrl32 is served read-only by the Caddy container that install.sh configures.
set -euo pipefail
GOBIN_DIR="$(go env GOPATH)/bin"; command -v cygpath >/dev/null 2>&1 && GOBIN_DIR="$(cygpath -u "$GOBIN_DIR")"   # Git Bash: C:\… → /c/…
export PATH="$GOBIN_DIR:$PATH"
VERSION=${1:?version}
TARGET=${2:?user@host}
shift 2
SSH_OPTS=("$@")
OUT=dist/$VERSION
mkdir -p "$OUT"
for arch in amd64 arm64 arm; do   # arm = ARMv7 (32-bit Raspberry Pi OS / Home Assistant armv7), for the camera relay
  GOOS=linux GOARCH=$arch GOARM=7 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$OUT/ctrl32-telemetry-linux-$arch" ./cmd/ctrl32-telemetry
done
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$OUT/ctrl32-telemetry-windows-amd64.exe" ./cmd/ctrl32-telemetry
# Authenticode signature when a code-signing certificate is available (CTRL32_SIGN_PFX + CTRL32_SIGN_PASS, Windows SDK signtool).
if [[ -n "${CTRL32_SIGN_PFX:-}" ]]; then
  SIGNTOOL=${SIGNTOOL:-$(ls "/c/Program Files (x86)/Windows Kits/10/bin/"*/x64/signtool.exe 2>/dev/null | sort -V | tail -1)}
  [[ -x "$SIGNTOOL" ]] || { echo "signtool not found; set SIGNTOOL"; exit 1; }
  "$SIGNTOOL" sign /f "$CTRL32_SIGN_PFX" /p "${CTRL32_SIGN_PASS:-}" /fd sha256 /tr http://timestamp.digicert.com /td sha256 /d "ctrl32 telemetry" "$OUT/ctrl32-telemetry-windows-amd64.exe"
  echo "signed windows binary"
else
  echo "note: CTRL32_SIGN_PFX not set — windows binary is unsigned"
fi
# Supply-chain evidence (URS H-16): govulncheck result and a CycloneDX SBOM are published next to the binaries.
if command -v govulncheck >/dev/null 2>&1; then
  if govulncheck ./... > "$OUT/govulncheck-$VERSION.txt" 2>&1; then echo "govulncheck: no reachable vulnerabilities"
  else echo "govulncheck found problems (see $OUT/govulncheck-$VERSION.txt)"; [[ -n "${CTRL32_ALLOW_VULN:-}" ]] || exit 1; fi
else echo "note: govulncheck not installed (go install golang.org/x/vuln/cmd/govulncheck@latest)"; fi
if command -v cyclonedx-gomod >/dev/null 2>&1; then
  cyclonedx-gomod app -json -licenses -main cmd/ctrl32-telemetry -output "$OUT/sbom-$VERSION.cdx.json" . && echo "sbom: $OUT/sbom-$VERSION.cdx.json"
else echo "note: cyclonedx-gomod not installed (go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest)"; fi
( cd "$OUT" && sha256sum ctrl32-telemetry-* $(ls sbom-* govulncheck-* 2>/dev/null) > SHA256SUMS )
cp deploy/install.sh "$OUT/install.sh"
cp deploy/install.ps1 "$OUT/install.ps1"
cp deploy/install-relay.sh deploy/install-relay.ps1 "$OUT/"
echo "built $(ls "$OUT")"
ssh "${SSH_OPTS[@]}" "$TARGET" 'install -d -m 0755 /var/www/ctrl32/dl'
scp "${SSH_OPTS[@]}" "$OUT"/ctrl32-telemetry-* "$OUT/SHA256SUMS" "$TARGET:/var/www/ctrl32/dl/"
for f in "$OUT"/sbom-* "$OUT"/govulncheck-*; do [[ -e "$f" ]] && scp "${SSH_OPTS[@]}" "$f" "$TARGET:/var/www/ctrl32/dl/"; done
scp "${SSH_OPTS[@]}" "$OUT/install.sh" "$TARGET:/var/www/ctrl32/install.sh"
scp "${SSH_OPTS[@]}" "$OUT/install.ps1" "$TARGET:/var/www/ctrl32/install.ps1"
scp "${SSH_OPTS[@]}" "$OUT/install-relay.sh" "$OUT/install-relay.ps1" "$TARGET:/var/www/ctrl32/"
ssh "${SSH_OPTS[@]}" "$TARGET" "chmod 0644 /var/www/ctrl32/install.sh /var/www/ctrl32/install.ps1 /var/www/ctrl32/install-relay.sh /var/www/ctrl32/install-relay.ps1 /var/www/ctrl32/dl/*; printf '%s\n' '$VERSION' > /var/www/ctrl32/dl/VERSION"
bash deploy/homeassistant/publish-addon.sh "$VERSION" "$TARGET" "${SSH_OPTS[@]}"
echo "published $VERSION to $TARGET (/install.sh, /dl/, /dl/ha-addons.git)"

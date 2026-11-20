#!/usr/bin/env bash
# Build a signed release and publish it with the installer to a host running ctrl32 telemetry behind Caddy:
#   https://<host>/install.sh, /install-relay.sh (+ .ps1), /install-wg-modem.sh and https://<host>/dl/ with
#   ctrl32-telemetry-linux-{amd64,arm64,arm}, ctrl32-telemetry-windows-amd64.exe, SHA256SUMS, VERSION,
#   release.json + release.json.sig (the signed manifest, ADR-0024), analytics/ (the pinned parts of the object
#   counting detector) and ctrl32-telemetry-<version>-offline.zip (everything for a server without the Internet)
# Usage (from the repository root, Go on PATH):
#   deploy/publish.sh 0.76.0 root@2.29.5.17 [-i ~/.ssh/key]
# The release signing key never enters the repository: it is read from CTRL32_RELEASE_KEY (default
# ~/.ctrl32-release/release-signing.key, created with `go run ./tools/relsign keygen`); without it nothing is
# published. NO_DETECTOR=1 leaves the detector parts out (smaller offline package); DETECTOR_FROM=dist/<older>/analytics
# reuses the parts of an earlier release (still checked against the pins).
# SHA256SUMS stays published for servers before 0.76.0, whose agent checks only it; from 0.76.0 on the agent installs
# only releases whose manifest carries a valid signature of a trusted key.
# The host directory /var/www/ctrl32 is served read-only by the Caddy container that install.sh configures.
set -euo pipefail
GOBIN_DIR="$(go env GOPATH)/bin"; command -v cygpath >/dev/null 2>&1 && GOBIN_DIR="$(cygpath -u "$GOBIN_DIR")"   # Git Bash: C:\… → /c/…
export PATH="$GOBIN_DIR:$PATH"
VERSION=${1:?version}
TARGET=${2:?user@host}
shift 2
SSH_OPTS=("$@")
KEY=${CTRL32_RELEASE_KEY:-$HOME/.ctrl32-release/release-signing.key}
[[ -f "$KEY" ]] || { echo "release signing key not found: $KEY (go run ./tools/relsign keygen; it lives outside the repository)" >&2; exit 1; }
OUT=dist/$VERSION
rm -rf "$OUT"
mkdir -p "$OUT"
# release date in the binary: the licence includes updates released until its updates_until date (internal/release)
RELEASED=${RELEASED:-$(date -u +%Y-%m-%d)}
LDF="-s -w -X main.version=$VERSION -X github.com/telemetry-digital/ctrl32-telemetry/internal/release.date=$RELEASED"
for arch in amd64 arm64 arm; do   # arm = ARMv7 (32-bit Raspberry Pi OS / Home Assistant armv7), for the camera relay
  GOOS=linux GOARCH=$arch GOARM=7 CGO_ENABLED=0 go build -trimpath -ldflags "$LDF" -o "$OUT/ctrl32-telemetry-linux-$arch" ./cmd/ctrl32-telemetry
done
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$LDF" -o "$OUT/ctrl32-telemetry-windows-amd64.exe" ./cmd/ctrl32-telemetry
# Authenticode signature when a code-signing certificate is available (CTRL32_SIGN_PFX + CTRL32_SIGN_PASS, Windows SDK signtool).
if [[ -n "${CTRL32_SIGN_PFX:-}" ]]; then
  SIGNTOOL=${SIGNTOOL:-$(ls "/c/Program Files (x86)/Windows Kits/10/bin/"*/x64/signtool.exe 2>/dev/null | sort -V | tail -1)}
  [[ -x "$SIGNTOOL" ]] || { echo "signtool not found; set SIGNTOOL"; exit 1; }
  "$SIGNTOOL" sign /f "$CTRL32_SIGN_PFX" /p "${CTRL32_SIGN_PASS:-}" /fd sha256 /tr http://timestamp.digicert.com /td sha256 /d "ctrl32 telemetry" "$OUT/ctrl32-telemetry-windows-amd64.exe"
  echo "signed windows binary"
else
  echo "note: CTRL32_SIGN_PFX not set — windows binary is unsigned (Authenticode; the release manifest is signed regardless)"
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
cp deploy/install-relay.sh deploy/install-relay.ps1 deploy/install-wg-modem.sh "$OUT/"
printf '%s\n' "$VERSION" > "$OUT/VERSION"
# the detector parts for servers without the Internet (pinned, unmodified, with their licences)
# DETECTOR_FROM=dist/<older>/analytics seeds them from an earlier release when an upstream download is gone; relsign
# keeps a file only when its SHA-256 matches the pin and downloads the rest
if [[ "${NO_DETECTOR:-}" != 1 ]]; then
  if [[ -n "${DETECTOR_FROM:-}" ]]; then mkdir -p "$OUT/analytics"; cp "$DETECTOR_FROM"/* "$OUT/analytics/"; fi
  go run ./tools/relsign fetch-detector -out "$OUT/analytics"
fi
# the signed manifest over every file, then the offline package, then a check of both with the keys of this source
go run ./tools/relsign sign -dir "$OUT" -version "$VERSION" -released "$RELEASED" -key "$KEY"
PKG=ctrl32-telemetry-$VERSION-offline.zip
go run ./tools/relsign package -dir "$OUT" -out "$OUT/$PKG"
go run ./tools/relsign verify -dir "$OUT"
go run ./tools/relsign verify -file "$OUT/$PKG"
echo "built $(ls "$OUT")"
ssh "${SSH_OPTS[@]}" "$TARGET" 'install -d -m 0755 /var/www/ctrl32/dl /var/www/ctrl32/dl/analytics'
# the files first, the manifest, its signature and VERSION last: a server never sees a release whose files are not there
scp "${SSH_OPTS[@]}" "$OUT"/ctrl32-telemetry-* "$OUT/SHA256SUMS" "$TARGET:/var/www/ctrl32/dl/"
for f in "$OUT"/sbom-* "$OUT"/govulncheck-*; do [[ -e "$f" ]] && scp "${SSH_OPTS[@]}" "$f" "$TARGET:/var/www/ctrl32/dl/"; done
if [[ -d "$OUT/analytics" ]]; then scp "${SSH_OPTS[@]}" "$OUT"/analytics/* "$TARGET:/var/www/ctrl32/dl/analytics/"; fi
scp "${SSH_OPTS[@]}" "$OUT/install.sh" "$TARGET:/var/www/ctrl32/install.sh"
scp "${SSH_OPTS[@]}" "$OUT/install.ps1" "$TARGET:/var/www/ctrl32/install.ps1"
scp "${SSH_OPTS[@]}" "$OUT/install-relay.sh" "$OUT/install-relay.ps1" "$OUT/install-wg-modem.sh" "$TARGET:/var/www/ctrl32/"
# the manifest lists install.sh & co. too: the copies under /dl/ keep the mirror complete
scp "${SSH_OPTS[@]}" "$OUT/install.sh" "$OUT/install.ps1" "$OUT/install-relay.sh" "$OUT/install-relay.ps1" "$OUT/install-wg-modem.sh" "$TARGET:/var/www/ctrl32/dl/"
scp "${SSH_OPTS[@]}" "$OUT/release.json" "$OUT/release.json.sig" "$OUT/VERSION" "$TARGET:/var/www/ctrl32/dl/"
ssh "${SSH_OPTS[@]}" "$TARGET" "chmod 0644 /var/www/ctrl32/install.sh /var/www/ctrl32/install.ps1 /var/www/ctrl32/install-relay.sh /var/www/ctrl32/install-relay.ps1 /var/www/ctrl32/install-wg-modem.sh; find /var/www/ctrl32/dl -type f -exec chmod 0644 {} +; find /var/www/ctrl32/dl -maxdepth 1 -name 'ctrl32-telemetry-*-offline.zip' ! -name '$PKG' -delete"
bash deploy/homeassistant/publish-addon.sh "$VERSION" "$TARGET" "${SSH_OPTS[@]}"
echo "published $VERSION to $TARGET (/install.sh, /dl/ with the signed manifest and $PKG, /dl/ha-addons.git)"

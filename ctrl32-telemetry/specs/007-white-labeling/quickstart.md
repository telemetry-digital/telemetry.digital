# Quickstart: validate spec 007

1. `go run ./tools/licgen keygen -out /tmp/k -name demo` → copy `demo.pub` to `internal/license/trusted/` (or build
   with the test key: `-ldflags "-X github.com/telemetry-digital/ctrl32-telemetry/internal/license.extraKey=…"`).
2. `go run ./tools/licgen issue -key /tmp/k/demo.key -licensee "Acme" -days 365` → key.
3. System → License: paste the key → valid, feature white_label.
4. Settings → Branding and theme → White label: product name, footer, links, icon → save; every page shows the brand.
5. Remove the license → ctrl32 footer returns, settings stay.

Automated: `go test ./internal/license`, `tests/e2e/whitelabel.sh`.

# Test license key

`license-test.key` / `license-test.pub` sign licenses for the automated tests only. `tests/run-all.sh` builds the test
server with `-ldflags "-X …/internal/license.extraKey=<license-test.pub>"`; release builds (`deploy/publish.sh`) do not
trust this key, so licenses signed with it are rejected in production. The private key is public on purpose.

`license-online-test.key` / `license-online-test.pub` stand for the ONLINE key of the licence worker: test builds
trust them with `-X …/internal/license.extraOnlineKey=<license-online-test.pub>` as key `build-online` with the
31-day limit, and `tests/e2e/licfake.mjs` (the stand-in activation service of `whitelabel.sh`) signs its 30-day
licenses with it, its perpetual ones with `license-test.key`.

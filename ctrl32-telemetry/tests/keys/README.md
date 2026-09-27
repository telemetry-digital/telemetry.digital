# Test license key

`license-test.key` / `license-test.pub` sign licenses for the automated tests only. `tests/run-all.sh` builds the test
server with `-ldflags "-X …/internal/license.extraKey=<license-test.pub>"`; release builds (`deploy/publish.sh`) do not
trust this key, so licenses signed with it are rejected in production. The private key is public on purpose.

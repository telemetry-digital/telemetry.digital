# Trusted release keys

Each `<key id>.pub` file here is an Ed25519 public key the server trusts for release signatures (ADR-0024). The
binary embeds this directory; the agent installs a release only when `release.json.sig` carries a valid signature of
one of these keys.

- `release-2026.pub` — created 4 Oct 2026 on the release PC with `go run ./tools/relsign keygen -id release-2026`.

The private keys never enter the repository. The release PC keeps the private key in
`%USERPROFILE%\.ctrl32-release\release-signing.key` (readable by its user only); `deploy/publish.sh` refuses to publish
without it. It must be backed up offline (two copies, e.g. an encrypted USB key in a safe): without it no release can
be signed for servers that trust only this key.

**Rotation:** create the new key, add its `.pub` here and publish at least one release that still verifies with the
old key (sign with both: `relsign sign -key old.key,new.key`). Servers updated to that release trust both keys; later
releases may be signed with the new key only, and the old `.pub` is removed. A compromised key is removed at once and
the release that removes it is signed with the new key and the old one, so existing servers can still install it.

# Trusted license keys

Each `*.pub` file holds one base64 Ed25519 public key (lines starting with `#` are comments). Licenses signed with the
matching private key are accepted by the binary; every build, release builds included, embeds the files of this
directory.

| File | Key | Created |
|---|---|---|
| `telemetry-digital-2026.pub` | production licensor (MASTER) key of telemetry.digital: perpetual licenses (white labeling, ADR-0016) | 27 Sep 2026 |
| `telemetry-online-2026.pub` | ONLINE key of the licence worker lic.ctrl32.com: 30-day licenses of the online activation, `# max-days: 31` | 27 Sep 2026 |

A line `# max-days: N` marks an **online** key: tokens it verifies are accepted only with an issue date and an expiry at
most N days later (a perpetual token of such a key is rejected), so a key held by an online service can never issue a
perpetual license. The master key has no such line.

The private keys never enter this repository (`*.key` is ignored here); the online key's seed lives only in the
worker secret `TELEMETRY_KEY_SEED` and offline with the licensor. It is kept offline by the licensor and backed
up; whoever has it can issue licenses. To rotate, create a new pair with
`go run ./tools/licgen keygen -out <offline dir> -name <name>`, add the new `.pub` next to the old one (licenses signed
with the old key keep working while its `.pub` stays here) and issue new licenses with the new key.

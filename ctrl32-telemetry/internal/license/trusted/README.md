# Trusted license keys

Each `*.pub` file holds one base64 Ed25519 public key (lines starting with `#` are comments). Licenses signed with the
matching private key are accepted by the binary; every build, release builds included, embeds the files of this
directory.

| File | Key | Created |
|---|---|---|
| `telemetry-digital-2026.pub` | production licensor key of telemetry.digital (white labeling, ADR-0016) | 27 Sep 2026 |

The private key never enters this repository (`*.key` is ignored here). It is kept offline by the licensor and backed
up; whoever has it can issue licenses. To rotate, create a new pair with
`go run ./tools/licgen keygen -out <offline dir> -name <name>`, add the new `.pub` next to the old one (licenses signed
with the old key keep working while its `.pub` stays here) and issue new licenses with the new key.

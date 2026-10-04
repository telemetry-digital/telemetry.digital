# ADR-0024: Signed releases, update modes and offline updates

**Status:** accepted 4 Oct 2026 (owner: "signed releases — Ed25519 signature of the release manifest, the public keys
embedded in the binary with key rotation, the agent refuses unsigned or badly signed releases … update modes Manual /
Notify (default) / Automatic in a maintenance window … roll back automatically … offline updates: upload a package,
the command line, an internal mirror … SMTP in the UI"). Amends ADR-0012 (server agent), ADR-0016 (licence:
`updates_until` now checked against the *signed* release date) and ADR-0022 (the detector parts may travel in the
offline package and on a mirror, unmodified and pinned).

## Context

Until 0.75.0 the agent downloaded `SHA256SUMS` and the binary from the distribution point over HTTPS and compared the
two. Whoever controls the distribution point (or the TLS path to it, or a customer's mirror) controls both, so the
checksum proves only that the download is complete, not who built it. The release date the licence check relies on
came from the downloaded binary itself. Updates were manual only, and servers without the Internet could not update
from the user interface at all.

## Decision

1. **Signed manifest.** Every release publishes `release.json` (format 1: product, version, release date, and for
   every published file its name, size and SHA-256 — the binaries, `SHA256SUMS`, `VERSION`, the SBOM, the installers
   and the detector parts under `analytics/`) and `release.json.sig`: one line per signature,
   `ed25519 <key id> <base64 signature over the exact bytes of release.json>`.
2. **Trusted keys in the binary, with rotation.** `internal/release/trusted/<key id>.pub` are embedded; a release is
   accepted when one line verifies with a trusted key, lines of unknown keys are skipped. Rotation: add the new key
   one release before it signs, sign the transition releases with both keys (`relsign sign -key old,new`), remove the
   old `.pub` later. A compromised key is removed from `trusted/` at once. Test builds may trust one extra key given at
   build time (`-X …/internal/release.extraKey`, key id `build-test`); release builds never set it.
3. **The private key never enters the repository.** `go run ./tools/relsign keygen` writes it to
   `%USERPROFILE%\.ctrl32-release\release-signing.key` (readable by its user only; the tool refuses a directory inside
   a git repository). `deploy/publish.sh` signs with it and stops when it is missing. The key must be backed up
   offline; losing it means servers that trust only this key cannot verify new releases until a release signed by a
   still-trusted key adds a new one.
4. **The agent refuses** a release without a manifest, with a bad or untrusted signature, another version than asked
   for, a file whose size or SHA-256 differs from the manifest, or a binary that reports another version or release
   date — before anything on the system changes. The licence check (`updates_until`) uses the signed release date.
   The only exception: a *development* build (no release date compiled in) with `[agent] allow_unsigned_updates =
   true`; release builds ignore the key.
5. **Compatibility.** `SHA256SUMS` stays published as before, so a 0.75 server (which checks only it) updates to the
   first signed release; from 0.76.0 on the signature is required.
6. **Update modes** (System → Updates, stored in `core.server_setting`): *manual*; *notify* (the default for new and
   upgraded servers): one check a day with up to an hour of jitter, a banner for system administrators and one e-mail
   per version to the server administrators when SMTP works; *automatic*: a weekly one-hour window (weekday and time in
   a chosen IANA time zone, default the server's), patch versions only — minor versions when allowed, major versions
   never — always with a backup first; a version that failed is not retried automatically. The licence is respected:
   a release dated after `updates_until` is neither installed nor attempted.
7. **Installation and rollback.** Backup (database dump, configuration, agent files; the existing backup mechanism) →
   the running binary is kept as `<binary>.prev` → the new binary is put in place → migrations. **When the migrations
   fail** the previous binary returns at once and the database is restored from the backup of this installation
   (restored into a new database with `pg_restore --single-transaction`, then swapped in by renaming while the service
   is stopped — `--clean` on the live database fails on partitioned tables, and a failed restore must never leave the
   server without its database; the swap needs the disk space of a second copy for a moment). The restore runs in the
   background because it stops the web service that is waiting for the answer. **When they succeed** the services
   restart and a guard — a process of the *previous* binary (`ctrl32-telemetry update guard`, a systemd transient unit
   on Linux, a detached process on Windows), so it survives the restarts and works even when the new binary cannot
   start — waits until `/healthz` answers `status ok` with the new version three times in a row. When that does not
   happen within the rollback time (default 10 minutes, 2–60), the guard stops the service, puts the previous binary
   back and restarts the services. **The database is not restored then**: the migrations are forward-only and
   additive by convention, the old binary runs on the newer schema, and a restore would lose the data written since.
   If an old binary cannot work with the new schema, the administrator restores the backup named in the history.
8. **Offline updates.** (a) `ctrl32-telemetry-<version>-offline.zip` (signed manifest, binaries of every platform,
   `SHA256SUMS`, `VERSION`, SBOM, installers, detector parts with their licences and `SOURCES.txt`) is uploaded in
   System → Updates: the server streams it to the agent, which verifies the signature and every member (no member
   outside the manifest, no path outside the package, sizes and hashes) before keeping anything, stages it and
   installs it exactly like an online release. Size limit `[agent] max_package_mb` (default 2048), password and
   authenticator code again, a reason, browser sessions only (not API tokens, not AI assistants), audited including
   refusals. (b) `ctrl32-telemetry update --file <package> [--yes]` does the same through the agent, or in its own
   process (as root) when the agent does not run. (c) `[agent] update_url` may point at any HTTP(S) server in the
   customer network; `ctrl32-telemetry update mirror --to <dir>` fills it with a verified copy (manifest last). The
   detector installation takes its parts from the staged package, then from the mirror, then from the official
   releases — the pins decide in every case.
9. **Audit.** Mode changes (`server.update_settings`), installations (`server.update`, `server.update_package`) and
   their results including rollbacks (`server.update_result`, written by the server from the agent's history — also
   for installations of the command line and results the guard recorded after a restart).
10. **Outgoing e-mail in the UI** (same release, spec 031): System → Server → E-mail (SMTP) for system administrators;
    the settings saved there take precedence over `[smtp]` of config.toml unless `[smtp] override = true`; the
    password is write-only and stored with the existing secret box (AES-256-GCM, key derived from
    `security.secret_key`); an unencrypted connection only to this host or the local network; a test e-mail returns
    the SMTP conversation.

## Consequences

- A compromised distribution point or mirror can withhold releases but cannot make a server install a binary we did
  not sign. The signing key is the new crown jewel; its backup and custody are the owner's responsibility.
- Each release needs the signing key on the release PC; CI cannot publish on its own.
- The offline package is large (binaries of four platforms, ~500 MB with the detector parts); `NO_DETECTOR=1` builds a
  small one.
- A rollback after successful migrations keeps the newer schema; this is stated in the history and in the docs.
- Redistributing the detector parts (LGPL ffmpeg build, MIT ONNX Runtime, Apache-2.0 YOLOX models) unmodified with
  their licence texts and a source note is allowed by their licences; they are still never linked into our binary.

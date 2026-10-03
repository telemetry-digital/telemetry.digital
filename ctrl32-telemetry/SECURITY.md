# Security policy

ctrl32 telemetry stores records that organizations rely on (measurements, audit trails, reports). Security
problems are treated as data-integrity problems and fixed with priority.

## Supported versions

Only the latest released version is supported. Releases are frequent and upgrades are in place (the Server page
or a re-run of the installer); every release lists its changes in `JOURNAL.md`.

## Reporting a vulnerability

- **The product:** e-mail `info@telemetry.digital` (subject starting with "security"). Every installation lists it
  in `/.well-known/security.txt` and serves this policy at `/security-policy`.
- **One installation:** the operator's own contact, listed first in its `/.well-known/security.txt` (set with
  `[security] contact` in `config.toml`), for problems of that installation rather than of the product.
- Please include the version (`/healthz` reports it), the steps to reproduce, and the impact you see. Do not include
  data from a production installation that is not yours.

We acknowledge reports within three working days, keep the reporter informed, and aim to release a fix within
30 days for high-impact issues. Reporters are credited in the release entry unless they prefer not to be.

## Scope

In scope: the server binary (HTTP API, UI, MQTT broker, connectors, agent), the installers, the device client
libraries, and the published release artifacts. Out of scope: third-party services an operator connects (SMTP,
SMS or voice gateways, tile providers), and installations that disabled the secure defaults (profile `intranet`
without TLS on an untrusted network, `public_read` outside demos).

## What each release ships for verification

- `SHA256SUMS` for every binary at `https://portal.telemetry.digital/dl/`, checked by the updater before installing.
- A CycloneDX SBOM (`sbom-<version>.cdx.json`) and the `govulncheck` result (`govulncheck-<version>.txt`) next to
  the binaries; a release with an unresolved reachable vulnerability is not published.
- The threat model (`docs/threat-model.md`) and the security baseline (ADR-0007) in the repository.

## Hardening pointers for operators

Use profile `internet` for anything reachable from the Internet (TLS everywhere, MFA for everyone, strict headers),
keep the agent socket and the PostgreSQL port local, enable syslog forwarding (`[log] syslog`) and scrape `/metrics`
so failures are noticed, keep backups encrypted and off the server, and check `/.well-known/security.txt` and this
policy after each upgrade.

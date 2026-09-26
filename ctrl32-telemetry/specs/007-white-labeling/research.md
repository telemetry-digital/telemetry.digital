# Research: Paid white labeling

## R1 License verification
- **Decision**: Ed25519 signatures (Go standard library), token `c32l1.` + base64url(JSON) + `.` + base64url(sig).
- **Rationale**: small keys and tokens (paste-able), fast, no dependency; offline works on isolated plant networks.
- **Alternatives**: JWT libraries (dependency, algorithm confusion risks); online activation (needs a service and
  Internet; many installations are offline).

## R2 Where the public key lives
- **Decision**: `internal/license/trusted/*.pub` embedded; several keys allowed (rotation); test key only through
  `-ldflags -X` in test builds.
- **Rationale**: the licensor rotates keys by adding a file; release builds cannot be tricked by configuration.

## R3 Binding
- **Decision**: per server, optional host names matched against `http.base_url` (not the request Host header, which a
  client controls).

## R4 Applying the brand in the UI
- **Decision**: the server returns the product name and footer; `app.js` renders the footer and replaces the phrases
  "ctrl32 telemetry" / "ctrl32 · telemetry" in visible texts, attributes and titles (also in content rendered later),
  except inside `.no-rebrand` (texts about the ctrl32 license).
- **Alternatives**: editing every page (100+ mentions, easy to miss).

## R5 What stays
- Code-level names (`ctrl32.api`, CLI `ctrl32-telemetry`), the `/license` page and service names on the host are not
  customer branding and stay.

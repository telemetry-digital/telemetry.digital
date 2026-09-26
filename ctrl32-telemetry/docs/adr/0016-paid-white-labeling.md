# ADR-0016: Paid white labeling with offline license keys

**Status:** accepted 26 Sep 2026 (user: "Predpríprava white labeling, bude platený" — prepare white labeling, it will be
paid) — amends ADR-0002 (everything free) and ADR-0011 decision 5 (fixed product footer). The change of the `LICENSE`
text (condition 2) is a draft for legal review.
**Context:** Integrators want to deliver the system under their own brand. ADR-0011 made the product footer fixed and
ADR-0002 made everything free; the `LICENSE` required the footer to stay unchanged.

## Decision

1. **The core stays free** (ADR-0002 unchanged for everything else). White labeling is the first paid feature.
2. **White label** (per organization, spec 007): product name, short app name, footer text and up to four footer links,
   version on/off, browser icon, start-page introduction. Applied to every page (titles and visible texts naming the
   product), the PWA manifest, e-mails sent by the server, the authenticator-app issuer and PDF report footers. The
   logo and theme remain free branding (ADR-0011).
3. **License keys, verified offline**: Ed25519-signed tokens (`c32l1.<payload>.<signature>`) with id, licensee, issue
   and expiry dates, features and optional server names. The binary embeds only public keys
   (`internal/license/trusted/*.pub`); the licensor issues keys with `tools/licgen` and keeps the private key offline.
   No online activation, no phone-home (works on isolated networks, constitution II/III).
4. **Bound to a server**: one license per installation (all its organizations), optionally restricted to the host name
   of `http.base_url`. Installed by a system administrator under System → License with a reason (audit trail).
5. **Graceful expiry**: when the license expires or is removed, the settings stay but the fixed ctrl32 footer and name
   return; nothing else stops working.
6. **Licence text**: condition 2 of `LICENSE` allows replacing the footer only in a deployment covered by a valid
   white-label license. Technical enforcement can be bypassed once the source is public (after 1.0); the licence
   condition is what binds.
7. **Test key**: `tests/keys/license-test.*` is public and trusted only by test builds (`-ldflags -X …extraKey=`);
   release builds (`deploy/publish.sh`) do not trust it.

## Consequences

- Until the licensor commits a production public key to `internal/license/trusted/`, release builds accept no license.
- Pricing, ordering and invoicing are outside the software.
- ADR-0002 and ADR-0011 carry a note pointing here.

# ADR-0016: Paid white labeling — 99 EUR one-time, perpetual license key verified offline

**Status:** accepted 27 Sep 2026 (user: white labeling is paid, **99 EUR, one-time, forever**). History: first accepted
26 Sep 2026 (user: "Predpríprava white labeling, bude platený" — prepare white labeling, it will be paid), withdrawn the
same day ("zostaneme pri free verzii, nič platené" — the implementation was reverted), reinstated on 27 Sep 2026 with
the price and a perpetual license. Amends ADR-0002 (everything free) and ADR-0011 decision 5 (fixed product footer).
The change of the `LICENSE` text (condition 2) is a draft for legal review.
**Context:** Integrators want to deliver the system under their own brand. ADR-0011 made the product footer fixed and
ADR-0002 made everything free; the `LICENSE` required the footer to stay unchanged.

## Decision

1. **Everything else stays free** (ADR-0002 unchanged for all other functions). White labeling is the only paid
   feature. Without a license every deployment shows the fixed ctrl32 footer (ADR-0011).
2. **Price: 99 EUR, one-time, per server (installation), forever.** One license covers one installation with all its
   organizations; there is no subscription, no renewal and no expiry. Updates to newer versions keep the license (every
   later binary verifies the key as long as the licensor's public key stays in `internal/license/trusted/`).
3. **White label** (per organization, spec 007): product name, short app name, footer text and up to four footer links,
   version on/off, browser icon, start-page introduction. Applied to every page (titles and visible texts naming the
   product), the PWA manifest, e-mails sent by the server, the authenticator-app issuer and PDF report footers. The
   logo and theme remain free branding (ADR-0011).
4. **License keys, verified offline**: Ed25519-signed tokens (`c32l1.<payload>.<signature>`) with id, licensee, issue
   date, features, optional server names and an optional expiry date. A token without an expiry date is **perpetual**;
   `tools/licgen issue` creates perpetual `white_label` licenses by default (`-days` / `-expires` remain for special
   cases such as a time-limited trial). The binary embeds only public keys (`internal/license/trusted/*.pub`); the
   licensor keeps the private key offline. No online activation, no phone-home (works on isolated networks,
   constitution II/III).
5. **Bound to a server**: one license per installation, optionally restricted to the host name of `http.base_url`
   (issued with `-domains`). Installed by a system administrator under System → License with a reason (audit trail);
   the page shows "perpetual (never expires)" for a license without expiry.
6. **Removal**: when the license is removed (or a limited one expires), the settings stay but the fixed ctrl32 footer
   and name return; nothing else stops working.
7. **Licence text**: condition 2 of `LICENSE` allows replacing the footer only in a deployment covered by a valid
   white-label license. Technical enforcement can be bypassed once the source is public (after 1.0); the licence
   condition is what binds.
8. **Keys**: the production licensor key `telemetry-digital-2026` was created on 27 Sep 2026; its public half is in
   `internal/license/trusted/`, so release builds accept licenses from 0.57.0 on. The test key
   `tests/keys/license-test.*` is public and trusted only by test builds (`-ldflags -X …extraKey=`); release builds
   (`deploy/publish.sh`) do not trust it.

## Consequences

- Ordering and invoicing (99 EUR) happen outside the software; the licensor issues the key with `licgen` and sends it
  to the customer (`docs/whitelabel.md`).
- Losing the private key means no new licenses can be issued with it (existing ones keep working); leaking it lets
  anyone issue licenses. It is kept offline and backed up; a compromised key is rotated by adding a new `.pub` and
  removing the old one in the next release (licenses signed with the old key must then be reissued free of charge).
- ADR-0002 and ADR-0011 carry a note pointing here.

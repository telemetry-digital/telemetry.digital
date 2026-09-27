# ADR-0016: Paid white labeling — 39 EUR one-time, perpetual license key verified offline

**Status:** accepted 27 Sep 2026 (user: white labeling is paid, **39 EUR, one-time, forever**). History: first accepted
26 Sep 2026 (user: "Predpríprava white labeling, bude platený" — prepare white labeling, it will be paid), withdrawn the
same day ("zostaneme pri free verzii, nič platené" — the implementation was reverted), reinstated on 27 Sep 2026 with
the price and a perpetual license. Amends ADR-0002 (everything free) and ADR-0011 decision 5 (fixed product footer).
The change of the `LICENSE` text (condition 2) is a draft for legal review.
**Context:** Integrators want to deliver the system under their own brand. ADR-0011 made the product footer fixed and
ADR-0002 made everything free; the `LICENSE` required the footer to stay unchanged.

## Decision

1. **Everything else stays free** (ADR-0002 unchanged for all other functions). White labeling is the only paid
   feature. Without a license every deployment shows the fixed ctrl32 footer (ADR-0011).
2. **Price: 39 EUR, one-time, per server (installation), forever.** One license covers one installation with all its
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

## Amendment 27 Sep 2026 (evening): online activation through lic.ctrl32.com (0.59.0)

User decisions: white labeling is sold on ctrl32.com through Paddle exactly like the ctrl32 editor licence, and it is
**the same product**: one ctrl32 licence (39 EUR, once, perpetual) unlocks **either** the ctrl32 editor **or** white
labeling on one telemetry server — the product it is first activated for claims the order.

9. **Activation with the order number.** System → License sends the order number and the host name of `http.base_url`
   to the licence worker (`license.activation_url`, default `https://lic.ctrl32.com/telemetry/activate`) and
   installs the returned token after verifying it like a pasted one. This is the one exception to "no phone-home" in
   decision 4, and it is initiated by the administrator; the offline path (a pasted key) stays for isolated networks.
10. **Two keys, the editor's safety model.** Inside the 14-day refund window the worker signs a **30-day** license with
    the **online key** `telemetry-online-2026` (worker secret). Its trusted key file declares `# max-days: 31`: the
    binary rejects any token of that key without an expiry or valid longer than 31 days after issue, so a compromised
    worker or Cloudflare account can issue only 30-day licenses until the key is rotated. **Perpetual** licenses are
    minted offline with the **master key** `telemetry-digital-2026` (`licgen batch` on the licensor's PC, from the
    worker's queue) and returned by the next activation. Refunded orders get nothing; queued mints of a refunded order
    are dropped before they reach the master key.
11. **Automatic renewal.** The server stores the order number and asks the worker once a day while its license expires
    within 20 days (never replacing a perpetual license, never shortening one); a refusal (refunded, claimed by the
    editor, host limit) stops it and is shown on the License page. Removing the license forgets the order number.
12. **Limits in the worker**: two host names per order (a known one is always allowed again), up to six re-issued
    30-day licenses while the perpetual one is pending, one product per order (`claim:<order>`, older editor
    activations count as editor claims).

## Consequences

- Ordering and invoicing (39 EUR) happen outside the software; the licensor issues the key with `licgen` and sends it
  to the customer (`docs/whitelabel.md`).
- Losing the private key means no new licenses can be issued with it (existing ones keep working); leaking it lets
  anyone issue licenses. It is kept offline and backed up; a compromised key is rotated by adding a new `.pub` and
  removing the old one in the next release (licenses signed with the old key must then be reissued free of charge).
- ADR-0002 and ADR-0011 carry a note pointing here.

# ADR-0016: The licence of a server (white labeling and more) — license key verified offline

**Status:** accepted 27 Sep 2026 (user: white labeling is paid, **99,- EUR, one-time, forever**). History: first accepted
26 Sep 2026 (user: "Predpríprava white labeling, bude platený" — prepare white labeling, it will be paid), withdrawn the
same day ("zostaneme pri free verzii, nič platené" — the implementation was reverted), reinstated on 27 Sep 2026 with
the price and a perpetual license. Amends ADR-0002 (everything free) and ADR-0011 decision 5 (fixed product footer).
The change of the `LICENSE` text (condition 2) is a draft for legal review. **Amended 28 Sep 2026 by ADR-0017**: the
same license also unlocks changes through AI assistants (MCP); reading through assistants stays free. **Amended 2 Oct 2026 by ADR-0018**: the same license also unlocks VPN sites (routed subnets), time-limited
access and more than five VPN peers. **Amended 2 Oct 2026 (licence model, 0.66.0)**: the system is free including up
to 4 cameras; the licence of a server covers more cameras, white labeling, AI changes and the VPN extras and includes
updates for a period; the earlier "one-time, perpetual" wording below is superseded — see the last amendment.
**Context:** Integrators want to deliver the system under their own brand. ADR-0011 made the product footer fixed and
ADR-0002 made everything free; the `LICENSE` required the footer to stay unchanged.

## Decision

1. **Everything else stays free** (ADR-0002 unchanged for all other functions). White labeling is the only paid
   feature. Without a license every deployment shows the fixed ctrl32 footer (ADR-0011).
2. **Price: 99,- EUR, one-time, per server (installation), forever.** One license covers one installation with all its
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
**the same product**: one ctrl32 licence (99,- EUR, once, perpetual) unlocks **either** the ctrl32 editor **or** white
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

- Ordering and invoicing happen outside the software; the licensor issues the key with `licgen` and sends it
  to the customer (`docs/whitelabel.md`).
- Losing the private key means no new licenses can be issued with it (existing ones keep working); leaking it lets
  anyone issue licenses. It is kept offline and backed up; a compromised key is rotated by adding a new `.pub` and
  removing the old one in the next release (licenses signed with the old key must then be reissued free of charge).
- ADR-0002 and ADR-0011 carry a note pointing here.

## Amendment 2 Oct 2026: the licence model (0.66.0, spec 026)

Owner's plan of 2 Oct 2026: "The system is free including up to 4 cameras. A licence (one per server) covers more than
4 cameras, white labeling, changes through AI assistants (MCP writes) and the VPN extras (more than 5 peers, sites,
time-limited and approved technician access). The licence includes one year of updates; after the year the system keeps
running with everything it had — only updates stop until the licence is renewed." This replaces "bought once, valid
forever" in decision 2, the Status line and the 27 Sep amendment; prices are kept out of the product and its
documentation except the existing buy button.

13. **Free tier.** Without a valid licence a server runs up to **4 enabled cameras** (all organizations together),
    5 VPN peers without sites, expiries or grants, no white labeling and read-only AI assistants. Every licensed
    capability is unlocked by the same licence feature (`white_label`), one licence per server.
14. **Never switch anything off.** Limits apply only to adding or enabling: a fifth enabled camera (create, or enabling a
    disabled one) is refused with HTTP 402 `license_required` and a clear message; cameras beyond 4 on a server without
    a licence (e.g. after the licence was removed) keep running and can be changed. The check runs in the same
    transaction as the insert/update under a PostgreSQL advisory lock, so two parallel requests cannot both take the
    fourth slot. The License page and Camera management show "Cameras: N of 4 without a licence".
15. **Updates included for a period.** The licence key carries an optional claim `updates_until` (YYYY-MM-DD, the last
    release date the licence includes updates for). `licgen issue` sets it to one year after the issue date by default
    (`-updates-until DATE|none`); `licgen batch` to one year after the payment (`paid_at` of the worker's queue) unless
    the worker sends `updates_until`. Keys without the claim (all keys issued before 0.66.0) include updates without a
    limit. Tokens of an online key may carry `updates_until` at most 400 days after their issue date. The claim is read
    only from the signed token: an unsigned field in the activation reply is ignored.
16. **Release date.** Every release binary carries its release date (`internal/release`, set by `deploy/publish.sh`
    with `-X …/internal/release.date=`; development builds fall back to the commit time Go stamps into the binary).
    `ctrl32-telemetry version` prints it ("released YYYY-MM-DD"). The update channel publishes no separately signed
    release metadata, so this date is as trustworthy as the downloaded binary (HTTPS plus the published SHA-256) —
    an accepted limitation; a signed release manifest would be a later improvement.
17. **Installing updates.** System → Updates passes the licence's `updates_until` to the server agent; the agent
    downloads and verifies the binary, reads its release date and refuses it with 402 `updates_ended` when it was
    released after that date — before anything is replaced. A binary without a release date (before 0.66.0) is
    accepted only while the updates have not ended. Servers without a licence (or with a licence that is not valid on
    them) always update. **Updates ending never stops or degrades a running system**: the licence stays valid, every
    licensed feature keeps working; the License page shows "Updates ended on <date> — the system keeps running; renew
    the licence to install newer versions".
18. **No licence on a newer version than it covers.** Installing a licence key (pasted or by activation) whose
    `updates_until` lies before the release date of the running version is refused with 409 `updates_ended`;
    otherwise removing the licence, updating as a free server and installing it again would bypass decision 17. A
    licence keeps working on every version it covers, also after it was removed and installed again.
19. **Wording.** The product and its documentation no longer say "bought once", "forever", "perpetual" or "one-time"
    about the licence; a licence without an expiry date is described as "no expiry date". The API keeps the field
    `perpetual` as an older name of `no_expiry`.

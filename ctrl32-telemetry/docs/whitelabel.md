# The licence and white labeling

ctrl32 telemetry is free, **including up to 4 cameras**. A **licence** (one per server) covers:

- **more than 4 cameras** (enabled cameras of all organizations of the server together);
- **white labeling** — delivering the system under your own brand (below);
- **changes through AI assistants over MCP** (reading through assistants is free; `docs/mcp.md`, ADR-0017);
- the **VPN extras**: more than 5 peers, sites with routed subnets, time-limited and approved access for technicians
  (`docs/vpn.md`, ADR-0018/0020).

The licence **includes updates for a period** (`updates_until` in the licence key; one year from the payment for new
licences). After that date the system **keeps running with everything it has** — nothing is switched off — only newer
versions are not installed until the licence is renewed. Without a licence a server is never limited in updates.

The licence is the **ctrl32 licence** sold on [ctrl32.com](https://ctrl32.com/pricing/). One ctrl32 licence unlocks
**either** the ctrl32 editor **or** the licensed features of one telemetry server (installation) — not both; whichever
it is activated for first keeps it. Without a licence every deployment shows the fixed ctrl32 footer, and nothing that
already runs is ever switched off: cameras or VPN peers beyond the free limits keep working, only adding more is
refused (HTTP 402 `license_required`). Specification: `specs/007-white-labeling/`, `specs/026-licence-model/`;
ADR-0016 (amendment of 2 Oct 2026); URS J-16a.

## Cameras without a licence

Without a valid licence a server runs up to **4 enabled cameras** (all organizations together). Adding an enabled
camera, or enabling a disabled one, when 4 are enabled is refused with 402 `license_required`; a disabled camera can
still be added and every existing camera can be changed. A server that already has more (for example after its licence
was removed) keeps them all running. *System → License* and *Cameras → Camera management* show "Cameras: N of 4
without a licence".

## Updates included in the licence

| Licence | Updates |
|---|---|
| none | every version can be installed |
| without `updates_until` (keys issued before 0.66.0) | every version |
| with `updates_until` | versions **released** until that day (included); later ones are refused, the running system stays as it is |

*System → License* shows "Updates included until <date>", or after it "Updates ended on <date> — the system keeps
running; renew the licence to install newer versions". *System → Updates* shows the release date of the installed
version and the same line.

How it is checked: every release binary carries its release date (`ctrl32-telemetry version` prints
`ctrl32-telemetry 0.66.0 released 2026-10-02`; set at build time by `deploy/publish.sh`, package `internal/release`).
On *Install update* the server passes the licence's `updates_until` to the server agent; the agent downloads the binary,
verifies its SHA-256, asks it for its release date and refuses it (402 `updates_ended`) when it was released later —
before anything is replaced. A binary that reports no release date (before 0.66.0) is accepted only while the updates
have not ended.

**Limitation:** the update channel (`VERSION`, `SHA256SUMS` and the binaries at the distribution point) carries no
separately signed release metadata, so the release date is only as trustworthy as the downloaded binary itself (HTTPS
from the distribution point plus the published checksum). A licence key cannot be installed on a version released after
the updates it includes ended (409 `updates_ended`); it can on the versions it covers.

## What changes

With an active white label an organization's users see, instead of *ctrl32 telemetry*:

| Where | What |
|---|---|
| Every page | the product name in titles and texts, your footer text and up to four links (support, terms, imprint …), the version only if you want it |
| Browser and installed app | your icon, your app name (PWA manifest) |
| Start page | your introduction text |
| E-mails | password reset, account requests, test notifications name your product |
| Authenticator apps | new 2FA registrations show your product name |
| PDF reports | the footer names your product |

Logo, colours, font and map tiles are free branding (Settings → Branding and theme) and do not need a license. The
`/license` page keeps showing the software license. One license covers all organizations of the server.

## For the customer: buying and activating (online)

1. **Buy** the ctrl32 licence on ctrl32.com (System → License → *Buy the license* opens the shop; the link is
   `license.buy_url`). Payment goes through Paddle (merchant of record: invoice and VAT). Keep the **order number**
   from the confirmation: the Paddle transaction id (`txn_…`) or the ctrl32.com order id — either works.
2. Check that `http.base_url` holds the name the server is reached at (e.g. `https://scada.acme.sk`): the license is
   issued for that host name.
3. **System → License → Activate with the order number**: enter the order number and a reason, *Activate*. The server
   sends the order number and its host name to the licence service `lic.ctrl32.com` (`license.activation_url`), checks
   the license it gets (signature, server name) and installs it; the page shows the licensee, validity and the stored
   order number. Activation is in the audit trail (`license.activate`).
4. **Settings → Branding and theme → White label**: product name, app name, footer text and links, icon,
   introduction, *Use the white label*, reason, save.

What happens next, automatically:

- **During the 14-day refund period** the service gives a license valid **30 days**. The server asks the service again
  once a day while its license expires within 20 days (log only; the audit trail gets an entry when the license without
  expiry arrives). *Renew now* on the License page asks immediately (with a reason, audited as `license.renew`).
- **After the refund period** the licensor mints the license **without an expiry date** for your server, with the updates
  included for one year from the payment; the next automatic renewal installs it and renewing stops. From then on the
  server never needs to contact the service again.
- **A refused order** (refunded, already used for the ctrl32 editor, already active on two other servers, unknown or
  unpaid) is shown on the License page with the reason; automatic renewal stops until you activate again. A temporary
  problem (no Internet, service busy) is retried the next day.
- One order serves at most **two host names** (e.g. a server and its replacement); asking again for a known host name
  always works (reinstallation).

Removing the license (System → License → *Remove license*) also forgets the order number; the ctrl32 footer and name
return, the white-label settings stay and apply again once a license is installed.

## For the customer: offline installation

A server without Internet access cannot reach `lic.ctrl32.com`. Ask the licensor for a license key instead (give the
order number and the host name of `http.base_url`): one line starting with `c32l1.`. **System → License → Install a
license key (offline)**: paste the key, give a reason, *Install license*. Everything is verified offline.

## For the licensor: the licence service and the batch

The activation runs on the licence worker `lic.ctrl32.com` (Cloudflare Worker in the ctrl32 repository,
`server/lic-worker`, routes `/telemetry/activate`, `/telemetry/queue`, `/telemetry/minted`), next to the ctrl32 editor
activation. It verifies every order against Paddle (signed webhooks), records which product claimed an order, caps two
host names per order and signs the 30-day licenses with the **online key** `telemetry-online-2026` (worker secret
`TELEMETRY_KEY_SEED`). The binary accepts licenses of this key only when they expire at most **31 days** after issue
(`# max-days: 31` in `internal/license/trusted/telemetry-online-2026.pub`) and with `updates_until` at most 400 days
after issue, so the online key can never produce a license without expiry — a leaked online key buys 30-day licenses
until it is rotated.

Licenses without expiry are signed with the **master key** `telemetry-digital-2026`, which never goes online. Orders past
the refund period are queued by the worker; mint them on the licensor's PC:

```
go run ./tools/licgen batch                       # dry run: lists the queue
go run ./tools/licgen batch -apply                # sign with the master key, verify, archive, push to /telemetry/minted
```

Defaults: `-url https://lic.ctrl32.com`, `-token-file %CTRL32_KEYS%\worker-token.txt` (the worker's `ADMIN_TOKEN`, the
same file `visu_lic batch` uses), `-key ~/.ctrl32-license/telemetry-digital-2026.key`. Every token is archived with a
CSV ledger in `<key dir>/issued-telemetry/` before it is pushed. Offline: `-queue queue.json -o <dir>` reads a saved
queue and writes the tokens and `minted.json` without network access. Timing is loose: the customer always has a
30-day license while the one without expiry is pending (the worker re-issues up to six 30-day licenses). The minted
license includes updates until one year after the queue entry's `paid_at` (RFC 3339 or a date; without it one year
from minting), or until the entry's `updates_until` when the worker sends one. The worker may also put
`updates_until` into its 30-day licenses.

## For the licensor: issuing licenses by hand

The production key pair `telemetry-digital-2026` exists (27 Sep 2026): the public half is
`internal/license/trusted/telemetry-digital-2026.pub` (built into every binary from 0.57.0), the private half is kept
offline by the licensor, outside this repository, and must be backed up (whoever has it can issue licenses; losing it
means no new licenses with this key).

```
go run ./tools/licgen issue -key <offline dir>/telemetry-digital-2026.key -licensee "Acme s.r.o." \
    -email it@acme.sk -domains scada.acme.sk -notes "order txn_01…"
```

prints the license key (`c32l1.…`) to send to the customer, and on stderr a summary such as
`issued L-20261002-a1b2c3 for Acme s.r.o.: white_label, no expiry, updates included until 2027-10-02`. Options:

| Option | Meaning |
|---|---|
| `-features` | default `white_label` |
| `-domains scada.acme.sk` | bind to the host name of the server's `http.base_url` (`*.acme.sk` allowed); empty = any server |
| `-days N`, `-expires YYYY-MM-DD` | only for special cases (e.g. a trial); default: no expiry |
| `-updates-until YYYY-MM-DD` | last release date the licence includes updates for; default: one year after today; `none` = no limit |
| `-id` | own license id (default `L-<date>-<random>`) |

`go run ./tools/licgen show -token c32l1.… [-pub file.pub]` prints and verifies a key (with its validity, e.g.
*no expiry, updates included until …*; a `# max-days` line in the `.pub` file applies). A hand-issued license for an order bypasses the worker's
claim: check first that the order was not used for the ctrl32 editor.

Key rotation: `go run ./tools/licgen keygen -out <offline dir> -name <new name>` (it refuses to overwrite an existing
private key), commit the new `.pub` next to the old one and release. Keep the old `.pub` as long as licenses signed
with it should keep working. For a new online key, add the `# max-days: 31` line to its `.pub` and put the seed into
the worker secret `TELEMETRY_KEY_SEED` (`server/lic-worker/README.md`).

## Configuration

```toml
[license]
# activation_url = "https://lic.ctrl32.com/telemetry/activate"   # https only (http just to 127.0.0.1 for tests)
# buy_url = "https://ctrl32.com/pricing/"
```

## License terms

Condition 2 of the software license requires the ctrl32 product footer in every deployment, except a deployment covered
by a valid white-label license. Removing the footer in any other way — including by changing the software — is not
permitted. *(The wording is a draft pending legal review.)*

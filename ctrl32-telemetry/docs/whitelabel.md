# White labeling (paid: 39 EUR one-time, forever)

ctrl32 telemetry is free, and everything except white labeling and changes through AI assistants stays free.
**White labeling** — delivering the system under your own brand — is a paid feature; the same license also unlocks
**changes through AI assistants over MCP** (reading through assistants is free; `docs/mcp.md`, ADR-0017). It is unlocked by the **ctrl32 licence** sold on
[ctrl32.com](https://ctrl32.com/pricing/): **39 EUR, paid once, valid forever** (no subscription). One ctrl32 licence
unlocks **either** the ctrl32 editor **or** white labeling on one telemetry server (installation) — not both; whichever
it is activated for first keeps it. Without a license every deployment shows the fixed ctrl32 footer. Specification:
`specs/007-white-labeling/`; ADR-0016; URS J-16a.

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

1. **Buy** the ctrl32 licence on ctrl32.com (System → License → *Buy white labeling (39 EUR)* opens the shop; the link
   is `license.buy_url`). Payment goes through Paddle (merchant of record: invoice and VAT). Keep the **order number**
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
  once a day while its license expires within 20 days (log only; the audit trail gets an entry when the perpetual license
  arrives). *Renew now* on the License page asks immediately (with a reason, audited as `license.renew`).
- **After the refund period** the licensor mints the **perpetual** license for your server; the next automatic renewal
  installs it and renewing stops. From then on the server never needs to contact the service again.
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
(`# max-days: 31` in `internal/license/trusted/telemetry-online-2026.pub`), so the online key can never produce a
perpetual license — a leaked online key buys 30-day licenses until it is rotated.

Perpetual licenses are signed with the **master key** `telemetry-digital-2026`, which never goes online. Orders past
the refund period are queued by the worker; mint them on the licensor's PC:

```
go run ./tools/licgen batch                       # dry run: lists the queue
go run ./tools/licgen batch -apply                # sign with the master key, verify, archive, push to /telemetry/minted
```

Defaults: `-url https://lic.ctrl32.com`, `-token-file %CTRL32_KEYS%\worker-token.txt` (the worker's `ADMIN_TOKEN`, the
same file `visu_lic batch` uses), `-key ~/.ctrl32-license/telemetry-digital-2026.key`. Every token is archived with a
CSV ledger in `<key dir>/issued-telemetry/` before it is pushed. Offline: `-queue queue.json -o <dir>` reads a saved
queue and writes the tokens and `minted.json` without network access. Timing is loose: the customer always has a
30-day license while the perpetual one is pending (the worker re-issues up to six 30-day licenses).

## For the licensor: issuing licenses by hand

The production key pair `telemetry-digital-2026` exists (27 Sep 2026): the public half is
`internal/license/trusted/telemetry-digital-2026.pub` (built into every binary from 0.57.0), the private half is kept
offline by the licensor, outside this repository, and must be backed up (whoever has it can issue licenses; losing it
means no new licenses with this key).

```
go run ./tools/licgen issue -key <offline dir>/telemetry-digital-2026.key -licensee "Acme s.r.o." \
    -email it@acme.sk -domains scada.acme.sk -notes "order txn_01…, 39 EUR"
```

prints the license key (`c32l1.…`) to send to the customer, and on stderr a summary such as
`issued L-20260927-a1b2c3 for Acme s.r.o.: white_label, perpetual (never expires)`. Options:

| Option | Meaning |
|---|---|
| `-features` | default `white_label` |
| `-domains scada.acme.sk` | bind to the host name of the server's `http.base_url` (`*.acme.sk` allowed); empty = any server |
| `-days N`, `-expires YYYY-MM-DD` | only for special cases (e.g. a trial); default: perpetual |
| `-id` | own license id (default `L-<date>-<random>`) |

`go run ./tools/licgen show -token c32l1.… [-pub file.pub]` prints and verifies a key (with its validity, e.g.
*perpetual*; a `# max-days` line in the `.pub` file applies). A hand-issued license for an order bypasses the worker's
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

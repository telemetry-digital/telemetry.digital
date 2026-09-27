# White labeling (paid: 99 EUR one-time, forever)

ctrl32 telemetry is free, and everything except white labeling stays free. **White labeling** — delivering the system
under your own brand — is a paid feature: **99 EUR, paid once, per server (installation), valid forever** (a perpetual
license key, no subscription, no renewal; updates keep it). Without a license every deployment shows the fixed ctrl32
footer. Specification: `specs/007-white-labeling/`; ADR-0016; URS J-16a.

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

## For the customer: buying

1. Order white labeling from telemetry.digital (99 EUR, one-time). Give the licensee name (company) and a contact
   e-mail; optionally the host name the server is reached at (`http.base_url`, e.g. `scada.acme.sk`) if the license
   should be bound to it.
2. After payment you receive a license key: one line starting with `c32l1.`.

## For the customer: installing

1. **System → License**: paste the key, give a reason, *Install license*. The page shows the licensee, features,
   validity (*perpetual (never expires)*) and whether the key is valid on this server.
2. **Settings → Branding and theme → White label**: product name, app name, footer text and links, icon,
   introduction, *Use the white label*, reason, save.

Everything is verified offline; the server never contacts the licensor. Installation and removal are in the audit
trail. If the license is removed, the ctrl32 footer and name return; the settings stay and apply again when a license
is installed. Moving to another server: install the same key there (a key bound to a host name works only on that
name — ask for a new key when the name changes).

## For the licensor: issuing licenses

The production key pair `telemetry-digital-2026` exists (27 Sep 2026): the public half is
`internal/license/trusted/telemetry-digital-2026.pub` (built into every binary from 0.57.0), the private half is kept
offline by the licensor, outside this repository, and must be backed up (whoever has it can issue licenses; losing it
means no new licenses with this key).

```
go run ./tools/licgen issue -key <offline dir>/telemetry-digital-2026.key -licensee "Acme s.r.o." \
    -email it@acme.sk -notes "invoice 2026-042, 99 EUR"
```

prints the license key (`c32l1.…`) to send to the customer, and on stderr a summary such as
`issued L-20260927-a1b2c3 for Acme s.r.o.: white_label, perpetual (never expires)`. Options:

| Option | Meaning |
|---|---|
| `-features` | default `white_label` |
| `-domains scada.acme.sk` | bind to the host name of the server's `http.base_url` (`*.acme.sk` allowed); empty = any server |
| `-days N`, `-expires YYYY-MM-DD` | only for special cases (e.g. a trial); default: perpetual |
| `-id` | own license id (default `L-<date>-<random>`) |

`go run ./tools/licgen show -token c32l1.… -pub internal/license/trusted/telemetry-digital-2026.pub` prints and
verifies a key (with its validity, e.g. *perpetual*). Keep a list of issued licenses (id, licensee, invoice).

Key rotation: `go run ./tools/licgen keygen -out <offline dir> -name <new name>` (it refuses to overwrite an existing
private key), commit the new `.pub` next to the old one and release. Keep the old `.pub` as long as licenses signed
with it should keep working.

## License terms

Condition 2 of the software license requires the ctrl32 product footer in every deployment, except a deployment covered
by a valid white-label license. Removing the footer in any other way — including by changing the software — is not
permitted. *(The wording is a draft pending legal review.)*

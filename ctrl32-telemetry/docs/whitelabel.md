# White labeling (paid)

ctrl32 telemetry is free. **White labeling** — delivering the system under your own brand — is a paid feature that
needs a license key from the licensor (telemetry.digital). Specification: `specs/007-white-labeling/`; ADR-0016;
URS J-16a.

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
`/license` page keeps showing the software license.

## For the licensor: issuing licenses

```
go run ./tools/licgen keygen -out ~/ctrl32-keys -name ctrl32-2026
```

writes `ctrl32-2026.key` (private — keep it offline and backed up; whoever has it can issue licenses) and
`ctrl32-2026.pub`. Commit the `.pub` file to `internal/license/trusted/` and release; from that version on the binaries
accept licenses signed with the key. Several `.pub` files may be present (key rotation).

```
go run ./tools/licgen issue -key ~/ctrl32-keys/ctrl32-2026.key -licensee "Acme s.r.o." -email it@acme.sk \
    -features white_label -days 365 -domains scada.acme.sk -notes "order 2026-042"
```

prints the license key (`c32l1.…`) to send to the customer. `-days 0` = no expiry; `-domains` is optional (the host
name of the server's `http.base_url`; `*.acme.sk` allowed). `licgen show -token … -pub ctrl32-2026.pub` prints and
verifies a key.

## For the customer: installing

1. **System → License**: paste the key, give a reason, *Install license*. The page shows the licensee, features,
   validity and whether the key is valid on this server.
2. **Settings → Branding and theme → White label**: product name, app name, footer text and links, icon,
   introduction, *Use the white label*, reason, save.

Everything is verified offline; the server never contacts the licensor. Installation and removal are in the audit
trail. When the license expires or is removed, the ctrl32 footer and name return; the settings stay for renewal.

## License terms

Condition 2 of the software license requires the ctrl32 product footer in every deployment, except a deployment covered
by a valid white-label license. Removing the footer in any other way — including by changing the software — is not
permitted. *(The wording is a draft pending legal review.)*

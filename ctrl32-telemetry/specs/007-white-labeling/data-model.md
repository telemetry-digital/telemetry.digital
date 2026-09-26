# Data model: Paid white labeling

`core.product_license` (migration 0026): `id` = 1 (single row), `token` (≤ 8192), `installed_at`, `installed_by`.

`core.organization.branding.white_label` (JSONB):

| Key | Type | Rule |
|---|---|---|
| enabled | bool | true only with a valid license (402 otherwise) |
| product_name | text | required when enabled, ≤ 64, plain text |
| short_name | text | ≤ 24 (PWA) |
| footer_text | text | ≤ 120 |
| intro | text | ≤ 400, start page |
| footer_links | [{label ≤ 40, url}] | ≤ 4; url http(s)://, mailto: or /path |
| show_version | bool | |
| icon | {mime, data, updated_at} | PNG or SVG ≤ 64 KiB, no scripts |

License payload: `id`, `licensee`, `email`, `issued`, `expires` (optional), `features` (`white_label`), `domains`
(optional), `notes`.

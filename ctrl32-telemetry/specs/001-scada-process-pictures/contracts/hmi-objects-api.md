# Contract: HMI object library API

All routes are organization-scoped, JSON, session (cookie + CSRF) or API token; mutations need a `reason`.

| Route | Permission | Request | Response |
|---|---|---|---|
| `GET /api/v1/hmi/objects` | data.read | `?kind=svg\|composite` | `[{slug, kind, name, category, version, width, height, created_at}]` (current versions) |
| `GET /api/v1/hmi/objects/{slug}` | data.read | — | `{slug, kind, name, category, version, body, checksum, created_by, created_at, reason}` |
| `POST /api/v1/hmi/objects` | content.write | `{name, slug?, kind, category?, body, reason}` | 201 current object; 400 with field message; 409 slug exists |
| `PUT /api/v1/hmi/objects/{slug}` | content.write | `{name?, category?, body, reason}` | 200 new version |
| `DELETE /api/v1/hmi/objects/{slug}?reason=` | content.write | — | 204, current version retired |
| `POST /api/v1/hmi/objects/import-svg` | content.write | `{name, slug?, category?, svg (text ≤ 512 KiB), reason}` | 201 object with sanitised `body.svg`, `removed: [what was stripped]` |
| `GET /api/v1/hmi/objects/{slug}/svg` | data.read | — | `image/svg+xml` sanitised SVG, headers `CSP: default-src 'none'; style-src 'unsafe-inline'; sandbox`, `nosniff`, `Cache-Control: private, max-age=60` |
| `GET /api/v1/public/{token}/hmi/objects/{slug}/svg` | public link | — | same, only if the linked picture references the slug |

Validation (400): kind svg ⇒ body `{svg, width, height}` where svg passes the sanitiser and parses as one `<svg>`
root; composite ⇒ body `{elements, tags, width, height}` where `elements` passes the drawing-layer schema (see
`screen-elements.md`) and every `tag` placeholder is a name `^[a-z][a-z0-9_]{0,31}$` used as `tag: "$name"` inside
the elements (placeholders are replaced by datastream ids when placed).

Audit: `hmi_object.create|update|retire` with slug, kind, version, checksum, reason.

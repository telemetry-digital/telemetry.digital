# Implementation Plan: Installable apps (PWA profiles)

**Spec**: `specs/021-pwa-apps/spec.md` · **Version**: 0.60.0

## Technical context

- Go server (`internal/httpapi`), PostgreSQL 18, plain-JS pages (ADR-0013). No new dependency: the QR code uses the
  vendored `static/vendor/qrcode.js` (MIT), the install page is rendered with `html/template`.
- Existing pieces reused: `wlText`, `hexColor`, `reasonOf`, `mustOrg`, `audit`, `loadBranding`,
  `activeWhiteLabel`, `PrincipalFrom(...).SeesCameraGroup`, the service worker (only listed shell files are cached).

## Design

| Part | Where | Notes |
|---|---|---|
| Table `core.pwa_app` | `db/migrations/0041_pwa_apps.sql` | content row per version; unique current slug per organization; own GRANT for `ctrl32_app`. Number 0041 because 0040 is taken by license activation on a parallel branch (migrations are applied by name, gaps are fine). |
| API and validation | `internal/httpapi/pwa_apps.go` | `validatePWAApp`, `validPWAStart`; CRUD like dashboards (supersede + insert in one transaction, audit after commit). |
| Public lookup | `publicPWAApp` | signed-in user: own organization; anonymous (manifest fetches carry no cookies): the oldest organization that has the slug. Only enabled, current apps. |
| Manifest and page | `pwaIdentityOf`, `pwaManifest`, `pwaPage` | white-label rule in one function (unit-tested); `id` and `start_url` `/app/{slug}`, `scope` `/`. |
| Install page script | `static/app-install.js` | standalone or `?launch=1` → `ctrl32.enterApp(slug, start)`; otherwise install hints, `beforeinstallprompt` (captured by an inline script before `app.js`), QR code. |
| App mode | `static/app.js` | `sessionStorage['ctrl32.app']`, `loadApp()` in `ready()`, `sec:` keys on the top-level menu items, `home()`, `enterApp()`, `?full=1` exit. |
| Settings UI | `static/settings.html` (section, dialog), `static/settings-apps.js` | presets fill the form; start page chooser grouped (pages, dashboards, walls, cameras) plus a custom path. |
| Login | `static/login.html` | after sign-in go to `ctrl32.home()` (the app's start page in app mode). |
| OpenAPI | `openapi.go` | tag "Apps (PWA)", 7 operations, schemas `PWAApp`, `PWAAppInput`; `go run ./tools/apigen`. |

## Security notes

- Start page: server-side allow-list regex plus refused prefixes; the client never builds a URL from the profile
  other than `location.replace(start_url)` of a validated path (same origin by construction).
- Public endpoints expose only what an installed icon needs; no dashboards, cameras or names of objects.
- The app mode is cosmetic: it never grants anything; permissions filter the menu before the app does.
- Page and manifest are never cached stale: pages `no-store` (secure headers), manifest `no-cache`, the service worker
  caches only its fixed shell list.

## Testing

Unit (`pwa_apps_test.go`), e2e `tests/e2e/pwa_apps.sh`, headless `tests/headless/pwa_apps.js` (screenshots
`pwa-*.png`). Chrome ignores an emulated `display-mode`, so the probe plays an iPhone home-screen launch
(`navigator.standalone`) and keeps `?launch=1` as a fallback.

# Feature Specification: Installable apps (PWA profiles)

**Feature Branch**: `feature/pwa-apps`

**Created**: 2026-09-27

**Status**: Implemented (0.60.0)

**Input**: The user (27. 9. 2026): "Chcelo by to spraviť nastavenie, kde by sa dali robiť rôzne PWA — na kamery
napríklad atď." — a settings page where different PWAs can be made, for example one for the cameras.

**Related**: ADR-0010 (content framework), ADR-0011 (PWA, branding), ADR-0016 and spec 007 (white labeling),
specs 010/011 (cameras, video walls, phone use); URS J-15, J-15a; constitution I (security), II (API first).

## Clarifications (defaults)

- An app is a **view** of the same web application, not a permission boundary: the menu is first filtered by the
  user's permissions, then by the app's sections. A user who types another address still gets only what the role
  allows; the server enforces permissions as always.
- Install page, manifest and launch info are public (the app must install before sign-in) and contain no data of
  the organization: name, short name, start page path, section keys, colour.
- Without an active white-label license the full app name carries the product ("Cameras · ctrl32 telemetry") and the
  product icon is used; the home-screen label is the app's own short name. With white labeling the app's own name
  and the organization's icon are used (paid naming stays paid, ADR-0016).
- `scope` is `/` so that sign-in, sub-pages and links stay inside the app window; each app has its own manifest
  `id` (`/app/{slug}`), so several apps and the main app install side by side.
- The app mode lives in the window (sessionStorage), because every app shares the origin and the pages.

## User Scenarios

1. An administrator opens *Settings → Apps*, clicks *New app*, keeps the **Cameras** preset (start page: live view,
   menu: Cameras), writes a reason and saves. The list shows the address `/app/cameras`.
2. A guard opens `https://server/app/cameras` on an Android phone (or scans the QR code from a PC), taps *Install*
   and gets a "Cameras" icon. Launching it opens the live view; the menu shows only Cameras and My account.
3. On an iPhone the same page explains *Share → Add to Home Screen*; the icon launches the app in its own window.
4. A kiosk tablet at the gate gets a **Video wall** app whose start page is one video wall.
5. Leaving the app mode: *My account → Open the full application*.

## Requirements

- FR-001 Table `core.pwa_app` (migration 0041): slug (unique per organization among current versions), name,
  short name, start page, sections, theme colour, enabled; versioned (new row per change, `valid_to`, `checksum`,
  `reason`, `created_by`).
- FR-002 API `/api/v1/pwa-apps` (list, options, create, get, update, delete) with `content.write`; every change
  needs a reason and is audited (`pwa_app.create`, `pwa_app.update`, `pwa_app.retire`).
- FR-003 Validation on the server: slug `^[a-z0-9][a-z0-9-]{0,39}$` (not `options`); name 1–64 and short name
  ≤ 24 characters of plain text (warning above 12); start page a path of this application — starts with `/`, no
  `//`, no `..`, no scheme, only URL-safe characters, not `/api/v1`, `/app/`, `/tiles/`, `/sw.js`, manifests;
  sections from the known list (at least one, sorted in menu order, duplicates dropped); colour `#rrggbb` or empty.
- FR-004 `GET /api/v1/pwa-apps/options` offers the sections and start pages: main pages plus the organization's
  dashboards, video walls and cameras (cameras only with `video.view`, limited to the user's camera groups).
- FR-005 `GET /app/{slug}`: install page with the app name, install button (`beforeinstallprompt`), iPhone/iPad
  instructions, a QR code of its address (vendored qrcode.js), *Open* and *Full application*; links its own manifest
  and sets `apple-mobile-web-app-title` and `apple-touch-icon`. Launched standalone (`display-mode` standalone,
  fullscreen, minimal-ui or `navigator.standalone`) or with `?launch=1` it enters the app mode and opens the start
  page at once. Unknown or disabled app: 404 page.
- FR-006 `GET /app/{slug}/manifest.webmanifest`: `id` and `start_url` `/app/{slug}`, `scope` `/`, names per the
  white-label rule, icons from branding, theme colour (the app's, else the organization's), up to four shortcuts to
  the chosen sections. `Cache-Control: no-cache`; the service worker never caches app pages or manifests.
- FR-007 `GET /api/v1/pwa-apps/{slug}/launch` (public): name, short name, start page, sections, colour.
- FR-008 App mode in `app.js`: remembered per window; menu restricted to the app's sections plus My account (on the
  desktop sidebar and the phone drawer); the header shows the app name; the logo leads to the start page; after
  sign-in the start page opens; sign-out stays in the app (`/login`); *Open the full application* (`/?full=1`)
  leaves the mode; a deleted or disabled app ends the mode on the next page.
- FR-009 Settings → Apps: list (name, address, start page, sections, state, install page, edit, delete) and a dialog
  with presets **Cameras**, **Video wall** (first wall), **Dashboards**, **Smart home** (Home, Energy); the presets
  only fill the form. Translations sk/cs/en.
- FR-010 The main manifest `/manifest.webmanifest` is unchanged.

## Success Criteria

- SC-001 Unit tests: start-page validation (external addresses, `//`, `javascript:`, `..`, API paths refused),
  profile validation, manifest JSON with and without white label, service worker does not cache `/app/`, menu keys
  of `app.js` equal the server's sections.
- SC-002 e2e `pwa_apps`: create, conflict, list, get, update (new version, warnings), disable, delete, reuse of the
  address; validation; public page, manifest, launch info, 404s, main manifest unchanged, caching headers;
  permissions (operator refused, anonymous refused); white-label naming with the test license; audit with reasons.
- SC-003 Headless `pwa_apps`: Settings → Apps with presets, install page (QR, Android and iPhone hints), launch as
  from the home screen, restricted menu on desktop and phone, the mode kept across pages, the way out, a deleted app.

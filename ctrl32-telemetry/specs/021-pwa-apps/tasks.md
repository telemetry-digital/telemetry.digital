# Tasks: Installable apps (PWA profiles)

- [x] T001 Migration 0041 `core.pwa_app` (versioned content, GRANT for ctrl32_app)
- [x] T002 `internal/httpapi/pwa_apps.go`: validation (slug, name, short name, start page, sections, colour), CRUD with
      reason and audit, options (sections, start pages from dashboards, walls, cameras by camera groups)
- [x] T003 Public: `/app/{slug}` install page, `/app/{slug}/manifest.webmanifest`, `/api/v1/pwa-apps/{slug}/launch`;
      white-label naming; 404 for unknown or disabled apps
- [x] T004 `app.js`: app mode per window, menu restricted to sections + My account, app name, logo to the start page,
      way out (`?full=1`), `home()` after sign-in and `enterApp()`; `app-install.js` (standalone launch, install
      button, iPhone hints, QR code)
- [x] T005 Settings → Apps: list, dialog, presets Cameras / Video wall / Dashboards / Smart home; login redirect
- [x] T006 OpenAPI rows and schemas, `go run ./tools/apigen`; service worker check (no `/app/` caching)
- [x] T007 Translations sk/cs/en (38 new strings)
- [x] T008 Tests: unit, e2e `pwa_apps`, headless `pwa_apps` with screenshots
- [x] T009 Docs: `docs/pwa.md`, `docs/video.md` (Phone and tablet), URS J-15a, README, JOURNAL, STAV
- [x] T010 0.61.0: migration 0042 (`locked`, `page_zoom`, `camera_layout`), API and OpenAPI, Settings → Apps options,
      locked app mode and page zoom in `app.js`, grid/list layout of the live view and walls, player touch handling
      and playback speed, PTZ press-and-hold; translations in all 19 catalogs (13 strings); tests and docs

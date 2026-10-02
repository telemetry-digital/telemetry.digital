# Tasks: Camera inventory — export and import of all camera settings

- [x] T001 `internal/xlsx`: workbooks with several sheets, text-formatted columns, a reader (shared and inline strings, rich text, booleans, references); unit tests
- [x] T002 `internal/camerainv`: columns, Excel/CSV/template writers, parser (header detection, `;` `,` tab, UTF-8/UTF-16/Windows-1250, yes/no words, Excel numbers, `-` clears, `#` rows ignored, limits), makes and ip moves; unit tests with round trip
- [x] T003 Migration 0050: camera `note`, permission `video.credentials_export` for `org_admin`
- [x] T004 `internal/httpapi/camera_inventory.go`: export (Excel, CSV, PDF stored as a report), confidential export (step-up, reason, rate limit, no tokens/MCP, audit, PDF not stored), template, import plan and apply (matching, kept ids, restore, ip move, passwords, relay secrets, camera groups, licence limit, one transaction, audit, reload); unit tests
- [x] T005 Report engine: `RenderBuiltin` (not stored, built-in template), template `cameras.typ` (cards, CONFIDENTIAL marking), sample data
- [x] T006 Video manager: reload cameras changed in the database (command line import while the server runs)
- [x] T007 Command line `cameras export|import`
- [x] T008 OpenAPI (4 operations, camera `note`), MCP exclusions with reasons, clients regenerated
- [x] T009 UI: buttons, confidential dialog, import dialog with the preview; camera form `note`; 19 catalogs
- [x] T010 e2e `camera_inventory.sh`; headless `camera_inventory.js`; docshots
- [x] T011 Docs: spec, threat model, `docs/video.md`, README, JOURNAL, STAV; public documentation `video-nvr/export-and-import.md`, cameras, reference, permissions reference
- [ ] T012 On a real installation: a crash-and-restore drill with a confidential export from the handover

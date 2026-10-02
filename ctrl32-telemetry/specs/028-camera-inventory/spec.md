# Feature Specification: Camera inventory — export and import of all camera settings

**Feature Branch**: `feature/0.70-cameras`

**Created**: 2026-10-02

**Status**: Implemented (0.70.0), not yet deployed

**Input**: The owner (2 Oct 2026): "One button to export camera parameters to Excel/PDF — customers often don't know,
after the installation company leaves, what IP addresses, names, passwords and parameters their cameras have. They need
an output to keep, e.g. when changing the system. And import of camera settings: the system crashes, load one CSV with
the camera parameters and continue in 5 minutes instead of entering and searching manually."

**Related**: specs 010 (cameras), 011 (remote cameras), 014 (camera groups), 016 (privacy masks), 017 (pins), 019
(motion), 020 (sound), 026 (licence model); ADR-0007 (security baseline), ADR-0017 (AI assistants), ADR-0018 (the
password step-up of the VPN); threat model; constitution "SAFETY FIRST".

## Clarifications (owner's decisions and defaults)

- **Where**: Cameras → Camera management, a row of buttons above the camera table: *Camera list: Excel · CSV · PDF ·
  With passwords… · Import…*. The same in the API and on the command line.
- **Which cameras**: the cameras of the user's organization that the user may see (camera groups narrow it, spec 014).
  Removed cameras are not exported.
- **Columns** (`internal/camerainv`, the keys are a file format and are never renamed): id, name, location, group,
  enabled, connection (pull/push), make, ip, main_stream_url, sub_stream_url, transport, username, *password*,
  *relay_secret*, onvif_url, ptz_profile, snapshot_url, recording, pre_event_s, post_event_s, event_kinds,
  retention_days, motion_detection, motion_sensitivity, motion_zones (JSON), privacy_masks (JSON), audio, audio_record,
  sort, note, and for information only floor_plans and state. Stream addresses never carry credentials.
- **New camera field `note`** (≤ 500 characters: mounting, cabling, recorder channel, warranty) — in the camera form,
  the API and the inventory (migration 0050).
- **Make**: not stored; recognised from the addresses on export (the presets of the camera form), and on import `make`
  with `ip` fills in the addresses of a camera without a main stream.
- **ip**: the host of the main stream (of the ONVIF address for push cameras). A different `ip` on import moves every
  address of the camera that uses the old host (the port and path stay) — the usual change after a network change.
- **Formats**: Excel (sheet *Cameras* with a header block — organization, server, time of export, user, version — and
  the sheet *Read me* explaining every column), CSV (UTF-8 with a byte-order mark, **semicolons**: Excel with a
  Slovak, Czech or German list separator opens it in columns; LibreOffice and Google Sheets recognise it; header lines
  start with `#`), PDF (Typst, A4 landscape, a card per camera, stored as a report like other PDF exports).
- **Passwords — SAFETY FIRST**: never in a normal export. *With passwords…* needs `video.manage` and the new permission
  **`video.credentials_export`** (built-in `org_admin` only, like `video.unmask` and `video.audio`; the name has one dot
  because permission names do), the user's password and authenticator code again (the VPN step-up), a reason; at most 3
  at once and then one a minute per user; refused for API tokens and AI assistants (MCP); audited as
  `video.credentials_export` (who, when, how many cameras, format, file name, reason) and logged as a security event.
  The file is marked *CONFIDENTIAL — contains passwords* in its name, the sheet name, the first line, and the PDF's
  header, footer and a box above the cards. A PDF with passwords is compiled from the built-in template (organization
  overrides cannot remove the marking) and **not stored** on the server.
- **Import**: Excel or CSV (detected by content; CSV with `;`, `,` or tab; UTF-8, UTF-16 or Windows-1250), at most
  2 MB and 1000 rows. Upload → server-side parsing and the same validation as the camera form → a preview per row
  (*new, update, restore, unchanged, skipped, error* with the changed settings and messages) → *Import* with a reason, in
  one transaction: all rows or none. Options *update existing cameras* (default on) and *add missing cameras* (default
  on). An import never deletes a camera.
- **Matching**: by `id`; without an id by `name` (case-insensitive), narrowed by `ip` when several cameras have the
  name; ambiguous rows are errors. A file id that is free on this server is **kept** (dashboards, walls and pins of a
  restored organization keep working); an id of another organization gets a new id with a warning; the id of a removed
  camera of the organization **restores** it (recordings that still exist come back with it).
- **Cells**: an empty cell keeps the stored value (a new camera gets the default); `-` clears a text (location, group,
  sub_stream_url, username, onvif_url, ptz_profile, snapshot_url, note); `none` is no event kinds; yes/no accepts the
  usual words (yes/no, true/false, 1/0, áno/nie, ja/nein …). An empty password keeps the stored one; the same password
  is no change. Rows whose id starts with `#` are ignored (the examples of the template).
- **Push cameras**: the relay secret from a confidential export keeps the relay at the site working; without it the
  camera gets a new secret and a warning tells to choose *New secret* to get the relay configuration.
- **Licence limit (spec 026)**: following the existing rule (adding a disabled camera is allowed, enabling a fifth is
  refused), cameras enabled before stay enabled, newly enabled rows take the free places in file order and the rest are
  **imported disabled** with a warning per row and a count in the preview. Checked again inside the transaction under
  the camera-limit lock.
- **After an import** the server reloads the changed cameras: they connect and record at once.
- **Command line** (when the web interface is not available): `ctrl32-telemetry cameras export -org SLUG -file
  cameras.xlsx [-format xlsx|csv|pdf] [-passwords -reason "…"]` and `ctrl32-telemetry cameras import -org SLUG -file
  cameras.xlsx -reason "…" [-dry-run] [-no-update] [-no-create]` — the same code, audited with the actor
  `cli cameras … (OS user)`. A running server picks the changes up within a minute (the video manager compares the
  stored cameras with the running ones every minute); a stopped server when it starts.
- **Floor-plan pins** are part of dashboards: the export lists them (`floor_plans`), the import ignores the column.

## User Scenarios

1. **Handover.** The installer finishes 12 cameras, chooses *With passwords…*, enters the password and the reason
   "handover to the customer", and gives the customer the PDF and the Excel file. The audit trail records it.
2. **Crash.** The server's disk dies. A new server is installed from scratch, the administrator imports the Excel file
   from the handover (with passwords): the preview shows 12 *new* rows, *Import* with a reason — all cameras connect.
   Without a licence 4 come enabled and 8 disabled, with a warning on each.
3. **Network change.** The customer moves the cameras to 10.20.0.0/24: export, change the `ip` column, import — every
   stream, ONVIF and snapshot address follows.
4. **Many cameras at once.** A new site: the template, one row per camera with name, make, ip, user and password;
   import fills in the addresses of the makes.
5. **No web interface.** `ctrl32-telemetry cameras import -dry-run` shows the plan, the same without `-dry-run` with a
   reason applies it.

## Requirements

- FR-001 `GET /api/v1/video/inventory?format=xlsx|csv|pdf` (video.manage), without passwords; audited as `export.read`
  (PDF: stored as a report).
- FR-002 `POST /api/v1/video/inventory/credentials` {format, reason, password, code} (video.credentials_export +
  video.manage), step-up, rate limit, not for tokens or MCP, audited `video.credentials_export`, PDF not stored.
- FR-003 `GET /api/v1/video/inventory/template?format=xlsx|csv` (video.manage): columns with two example rows.
- FR-004 `POST /api/v1/video/inventory/import` multipart file, update, create, apply, reason (video.manage): the plan,
  or applied in one transaction (422 `rows_invalid` with the plan when a row has an error); audited `video.import`
  (counts, options, the cameras) and `camera.create|update|restore` per camera with the reason "import: …".
- FR-005 Validation is the camera form's (`applyCamera`), camera groups as in the API, the licence limit as in FR-001 of
  spec 026.
- FR-006 None of the four operations is offered to AI assistants (mcpExclusions, with reasons).
- FR-007 Command line export and import with the same rules; the video manager reloads cameras changed in the database.
- FR-008 UI: the buttons, the confidential export dialog (password, authenticator code with two-factor sign-in,
  reason), the import dialog (template links, file, options, preview table with counts, reason, *Import (N)*); 19
  languages.

## Success Criteria

- SC-001 Unit: round trip export → import equals the original (Excel and CSV, with and without passwords, a push camera
  with its relay secret); parsing (separators, encodings, yes/no words, numbers as Excel writes them, errors per cell,
  limits); matching, ip move, password handling, restore and kept ids, camera groups, the licence limit.
- SC-002 e2e `camera_inventory` (all of the above against a running server, the command line included).
- SC-003 Headless: the buttons, the confidential dialog, the import preview.
- SC-004 Public documentation `video-nvr/export-and-import.md` with the column reference and the recovery procedure;
  screenshots of the buttons, the Excel file, the PDF and the import preview.

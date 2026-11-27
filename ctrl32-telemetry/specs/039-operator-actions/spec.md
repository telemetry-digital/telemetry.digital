# 039 — Operator action log (0.93.0)

First step of ADR-0026 (operator action log, four eyes everywhere, SCADA users).

## Why

"A button *start the motor*: with logging on, it records which user pressed it." Commands were already recorded, but
not everything a person does at a station, not in one place, and camera movement only once in five minutes. A control
room wants the diary of the shift: who did what, where and when — as it has the list of alarms.

## What

- **Off by default.** *Settings → Security policy → Operator log* (policy `log_operator_actions`, permission
  `user.admin`, a reason, audited). While it is off nothing is written.
- When on, `core.operator_action` (append-only) gets a row for:

  | Kind | When |
  |---|---|
  | `command`, `command_approve`, `command_reject` | a command from a control widget, a picture element, the device page, a scene, the API |
  | `recipe_load`, `recipe_approve`, `recipe_reject` | a recipe record loaded into a unit |
  | `attributes` | shared attributes of a device changed |
  | `alarm_ack`, `alarm_shelve`, `alarm_unshelve` | alarms |
  | `ptz` | every PTZ move, stop and preset (the audit trail keeps one move per five minutes) |
  | `video_playback`, `video_export`, `video_hold`, `video_hold_release`, `video_evidence`, `video_unmask`, `camera_mark` | recordings and evidence |
  | `display`, `kiosk` | commands to displays and kiosk PCs |
  | `scene`, `flow_inject`, `counting_correction` | a scene activated, a test message of a flow, occupancy corrected |
  | `login`, `logout` | sign-in and sign-out |
  | `navigate`, `popup`, `cancel`, `press` | reported by the pages: a page or picture opened, a faceplate or window opened, a press cancelled in the confirmation |

- A row: time, user, kind, target (the name of the device, camera, picture …), the page it came from
  (`X-Ctrl32-Screen`, sent by the pages with every change), detail, reason, the address of the computer, and a column
  for the SCADA user of step 3.
- Actions that write the audit trail are copied in `audit()`; PTZ, scenes, sessions and the page reports are logged
  where they happen. A failed insert never fails the action.
- `GET /api/v1/operator-actions` (`audit.read`): newest first, filters `from`, `to`, `user`, `kind`, `q`, paging with
  `before`; `enabled` says whether the log is on. `POST /api/v1/operator-actions` (a browser session): the page
  reports; only `navigate`, `popup`, `press`, `cancel`. `GET /api/v1/export/operator-actions` (`audit.read`): CSV, JSON
  Lines.
- `GET /api/v1/auth/me` carries `log_actions`, so pages report only when the log is on.
- Page **Operator log** (`/oplog`, menu under Alarms): range, user, kind (all, actions only, pictures and windows
  only, one kind), search, CSV, a warning when the log is off.

## Not in scope

- Four eyes for the areas that do not have it and approval on the spot (step 2), SCADA users (step 3).
- Retention of the log (rows are kept); a log of what a public link or a paired display shows (they have no user).
- Logging each press of a control that sends nothing and opens nothing.

## Tests

- E2E `tests/e2e/operator_actions.sh` (42): off by default, switching on, commands with user, target, page and
  reason, page reports, sign-in and sign-out, filters, search, paging, export, permissions, switching off.
- Headless `tests/headless/operator_log.js`: the switch in the policy, pages reporting themselves, the command with
  its page, the Operator log page with its filters, phone width.

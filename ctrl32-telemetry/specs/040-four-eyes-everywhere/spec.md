# 040 — Four eyes everywhere (0.94.0)

Second step of ADR-0026 (operator action log, four eyes everywhere, SCADA users).

## Why

"Four-eyes approval is explicitly off; there must be the option to switch it on — everywhere: cameras, SCADA,
everything, every single widget." Until 0.93.0 a second person could be required for commands (with recipes, scenes,
rules and flows), alarm limits, assignments and VPN access, and could approve only later, from their own session. A
control room wants it for the rest too, and wants the shift leader to approve at the operator's desk.

## What

Everything is **off by default**; an organization switches it on in *Settings → Security policy* (`user.admin`, a
reason, audited).

### Areas

| Policy | Covers | How it is held |
|---|---|---|
| `four_eyes_commands` (exists) | commands, recipe loads, scenes, rules and flows that send commands | the action waits (202, `pending`) |
| `four_eyes_alarms` | acknowledge (one, several), shelve, end a shelf | change request (202) |
| `four_eyes_attributes` | shared attributes of a device | change request (202) |
| `four_eyes_limits`, `four_eyes_assignments`, `four_eyes_vpn` (exist) | alarm rules, datastream assignments, VPN access of technicians | change request (202) |
| `four_eyes_cameras` | PTZ, playback, export, evidence holds and packages, a live picture without masks | approval for a limited time |
| `four_eyes_displays` | commands to displays and kiosk PCs, the emergency remote desktop | approval for a limited time |
| `four_eyes_minutes` | length of an approval for cameras and displays: 1–480, default 15 | |

- **A single control.** The six control widgets (setpoint, switch, button, knob, slider, round switch) and every element
  of a process picture with a command have the property *Needs a second person*. Its command carries
  `second_person: true` and is held even when `four_eyes_commands` is off. The mark can only add the approval. It is
  applied by the page: a caller of the API with `device.command` who leaves it out is bound by the area switch only.
- **Approval for a limited time** (cameras, displays). Without it the calls answer 403 with the code
  `second_person_required`. `POST /api/v1/second-person/grants {area, reason}` asks; a different user with a permission
  of the area approves; then the work is allowed for `four_eyes_minutes` on that **station** — the browser session or
  the API token that asked. The same user at another computer needs an approval of their own. It can be ended early.
  Every press is still written to the operator log. Watching live pictures is not held back; paired displays and public
  links have no user and are not affected.

### Two ways to approve

- **From the list.** *Approvals* (`/approvals`, menu under Operator log) shows everything that waits — held commands,
  recipe loads, change requests, requests for a time-limited approval — with who asked, what and why, the running
  approvals and the earlier ones. `GET /api/v1/second-person/waiting`.
- **On the spot.** When the server holds an action or asks for an approval, the page opens the dialog of the second
  person: name, password, the code of the authenticator app when that account uses one, a note.
  `POST /api/v1/second-person/approve {kind, id, device_id, username, password, code, reason}`: the credentials are
  checked like a sign-in (same lockout, written to the access log), **no session is created**, and the server runs the
  same approval the list would run, as that person — so the permission needed is the same and the record carries both
  names; the audit reason ends with "second person on the spot, at the station of …". Only the approvals named by
  `kind` (command, recipe, change, grant) can be run this way, never a path chosen by the caller. Failed attempts are
  limited per station user (5 a minute).
- The person who asked can **never** approve — neither from the list nor with their own credentials in the dialog.
- A PIN instead of the password comes with the SCADA users of step 3.

### Operator log

New kinds: `second_request`, `second_approve`, `second_reject`, `second_end`, `change_request`, `change_approve`,
`change_reject`.

## Not in scope

- SCADA users, groups per control and the PIN (step 3).
- A scene that holds several commands opens no dialog (its commands wait in Approvals); rules waiting for approval
  stay on the automation page.
- Retention of the operator log (ADR-0026 planned it for this step at the latest): the table is append-only for
  everyone, so deleting old rows needs the same authorised path as `admin purge`; it moves to step 3.
- A server-side check of the mark of a single control (see above).

## Tests

- E2E `tests/e2e/four_eyes_everywhere.sh` (77): defaults, a marked command, alarms, attributes, cameras and displays
  with approvals from the list and on the spot, wrong credentials, self-approval, another station, ending early.
- Headless `tests/headless/second_person.js` (en, sk): the switches in the policy, a marked switch on a dashboard with
  the dialog, Approvals, a camera move that asks and is sent again, phone width.

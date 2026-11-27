# 041 — SCADA users (0.95.0)

Third step of ADR-0026 (operator action log, four eyes everywhere, SCADA users).

## Why

"Users as WinCC has them — operator and so on; that is a second level and works only in SCADA; who signs in can do
things separately from the system." On a shared computer of a control room the session belongs to whoever signed in at
the start of the shift, or to a station account, so until 0.94.0 the records named the station, not the person. The
owner chose: sign-in in the picture by **PIN or password, set per user**, and **groups as in WinCC**.

## What

Nothing changes for an organization until it creates SCADA users or switches the requirement on.

### SCADA users and groups

- *Users → SCADA users* (`/users/scada`, `user.admin`, every change with a reason, audited). A SCADA user has a name
  (what the person types), a full name, a **PIN** (4–12 digits) or a **password** (at least 8 characters) — the
  administrator chooses per person — and memberships in groups. It is not an account: no session of its own, no
  permissions, no API. The secret is stored hashed like account passwords and is never returned.
- **Groups**: Operator, Shift leader, Technologist, Maintenance by default (created for every organization); the
  administrator renames, adds and removes them. A group has a `key` that never changes — controls name groups by key —
  and the flag *may approve as the second person* (on for Shift leader).
- Wrong secrets count towards the lockout of that SCADA user with the thresholds of the security policy and are
  limited per station; every attempt is in the access log. The administrator unlocks, disables, replaces the secret
  (which signs the person out everywhere) and removes.

### Signing in at the controls

- `POST /api/v1/scada/session {name, secret}` signs a person in **on top of the browser session of the station**. The
  session of the station is the station: another browser session of the same account is another station. An API token
  cannot carry a SCADA user (403).
- The person is signed out after `scada_idle_minutes` without activity (policy, 1–480, default 10; the pages report
  activity with `POST /api/v1/scada/session/touch`), with `DELETE /api/v1/scada/session`, when somebody else signs in
  at that station, or with the session of the station.
- Pages: a chip says who is at the controls (the strip above the page, the header on a phone, the frame of the operator
  view) and signs in and out. When the server answers `scada_user_required` or `scada_group_required`, the page opens
  the sign-in — name, PIN or password, a PIN pad for touch screens, the names used at this computer before — and sends
  the call again.

### Groups per control, checked by the server

- The six control widgets and every element of a process picture have **SCADA groups that may use it**
  (`scada_groups`, keys). A control with groups shows them next to its title.
- A command names the control it came from: `control: {dashboard, id}`. The server finds the widget or the element in
  the stored dashboard, checks that it is a control and that the key is the one it sends (for a faceplate type, the
  part of the key after the prefix), and reads from it:
  - the groups — the caller needs a SCADA user who is a member of one of them (403 `scada_user_required` /
    `scada_group_required`, with the names of the groups);
  - the mark *Needs a second person* — the command is held even when the body does not say so. This closes what 0.94.0
    left to the page.
- **The effective right is the intersection**: the request is authorised as the account of the station first
  (`device.command`), then the group is checked. A SCADA user never gets more than the station.
- Policy `scada_users_required` (off by default): commands, recipe loads and scenes sent from a browser session need a
  SCADA user. An account with `device.manage` that names no control (the device page) works under its own name;
  everybody else must then send commands from a control (403 `scada_control_required`), so that the groups of the
  controls cannot be walked around by leaving the control out.
- API tokens and AI assistants are the system, not a person at the controls: they are not asked for a SCADA user, and
  cannot use a control that names groups.

### Records

- Commands (`operator`), recipe loads, change requests and time-limited approvals store the SCADA user next to the
  account; the command log and *Approvals* show "station / person".
- The audit trail names the actor "station / person"; the operator log fills its `operator` column and has the kinds
  `scada_login`, `scada_logout`, `scada_failed`, `scada_denied`, `scada_admin`.

### The second person with a PIN

`POST /api/v1/second-person/approve` with `scada: true`: the second person is a SCADA user (name, PIN or password).
The person must be in a group that may approve; the request must have been made by a SCADA user, and by a different
one. The approval runs with the rights of the station and is recorded as "station / person". A request made with
nobody signed in at the controls can be approved only by an account.

### Retention of the operator log

Policy `operator_log_days` (0 = keep for good, the default; otherwise 30–3650). The table stays append-only for
everyone; `core.purge_operator_actions(org, days)` is the one way to drop old rows and refuses anything younger than 30
days. The server runs it once a day.

### Fixed on the way

An element of a process picture lost its mark *Needs a second person* when the picture was saved (0.94.0: the
validation did not keep the property). It is kept now.

## Limits, said plainly

- Without `scada_users_required`, a person with the account of the station and `device.command` can send a command
  that names no control (from the device page, or by calling the API); then no group is checked. Switch the requirement
  on to make the groups binding.
- The server checks that the key of the command belongs to the control named, not that the device does: faceplate
  types and aliases bind devices when the picture is drawn. A person who is signed in could name another control of
  the same key. Every such command is in the log with their name.
- Alarm acknowledgement and shelving record the SCADA user when one is signed in but never require one.
- The default groups have English names in the database; the pages show the four default names in the language of the
  viewer until the administrator renames them.

## Not in scope

Trends and the batch record (next), e-signature of records with a SCADA user, sign-in with a card or a badge reader.

## Tests

- E2E `tests/e2e/scada_users.sh` (99 with database access, 95 without): defaults, administration, controls and
  groups, sign-in, tokens, the mark checked by the server, the second person with a PIN, the requirement, sign-out
  after a minute without activity, lockout, disabling, removing, retention.
- Headless `tests/headless/scada_users.js` (en, sk): the administration page, the policy, the groups in the editor,
  the sign-in with the PIN pad, a refusal by group, a shift leader approving with a PIN, the operator view, the
  operator log, phone width.
- Unit: `scada_users_test.go`.

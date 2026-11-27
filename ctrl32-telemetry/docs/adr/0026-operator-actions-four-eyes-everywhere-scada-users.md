# ADR-0026: Operator action log, four eyes everywhere, and SCADA users as a second level

**Status:** accepted 27 Nov 2026 (owner, 26–27 Nov: "four-eyes approval is explicitly off; there must be the option
to switch it on — everywhere, cameras, SCADA, everything, every single widget. When it is not on, the option to log
everything, every single press, as with the alarms: a button *start the motor* — with logging on it records which
user. And users as WinCC has them — operator and so on; that is a second level and works only in SCADA; who signs in
can do things separately from the system." Of the options offered the owner chose: approval **both** on the spot and
from the list; sign-in in the picture by **PIN or password**, set per user; **groups as in WinCC**; the log keeps
**actions and movement**). Extends ADR-0008 (SCADA step by step: a control write is always a command with identity,
time, acknowledgement and reason) and URS H-08 (four eyes, optional per organization).

## Context

Until 0.92.0:

- **Four eyes** could be switched on for device commands (with recipes, scenes, rules and flows that send commands),
  alarm limits, datastream assignments and VPN access. Cameras (PTZ, playback, export, evidence), displays, alarm
  acknowledgement and shelving, and shared attributes were outside it. A waiting action could be approved only by
  another user from their own session.
- **What a person did** was known for commands (the command log, the logbook widget) and for everything that writes
  the audit trail. PTZ moves were thinned to one audit entry per five minutes, and nothing recorded which pictures
  and faceplates a person opened. There was no page for it.
- **Who acted** was the user of the browser session. On a shared control-room computer that is whoever signed in at
  the start of the shift — or a station account — so the log names the station, not the person.

A control room needs all three, and a plant under GxP needs them provable. They are also what WinCC offers as
operator input messages, electronic signatures and user administration.

## Decision

Three steps, each off by default and switched on per organization (SAFETY FIRST: a policy change needs a reason and is
audited; nothing changes for an organization that does not switch it on).

### 1. Operator action log (0.93.0, spec 039)

- Policy `log_operator_actions`. When on, every action of a person at the controls is kept in `core.operator_action`
  (append-only): commands and presses of controls, recipe loads, **every** PTZ move, display and kiosk commands, alarm
  acknowledgement and shelving, recordings viewed and exported, evidence, sign-in and sign-out, and — reported by the
  pages — the pictures, pages and windows opened and presses cancelled in the confirmation.
- Each row: time, user (and later the SCADA user), kind, target, the page it came from, detail, reason, the address
  of the computer.
- Actions that write the audit trail are copied from there in one place (`audit()`), so a new handler is logged
  without remembering to; the rest is logged where it happens. The audit trail stays the hash-chained legal record of
  changes; the operator log is the readable, complete diary of the shift. Reading it needs `audit.read`: it is a
  record of people's work.
- What a page reports (`navigate`, `popup`, `press`, `cancel`) changes nothing in the plant and is trusted as much as
  the browser; everything that changes something is logged by the server.

### 2. Four eyes everywhere (planned 0.94.0)

- A switch per area in the organization policy, all off by default: commands (exists), alarms (acknowledge, shelve),
  cameras (PTZ, playback, export, evidence, pictures without masks), displays and kiosk PCs, device attributes,
  configuration (the existing limits and assignments). Plus a mark on a single widget or picture element — *needs a
  second person* — that holds even when the area switch is off.
- **Approval on the spot and from the list.** The second person confirms in the same dialog with their own name and
  password or PIN (their identity is checked by the server, the action is recorded with both names), or approves later
  from their own session in the list of waiting actions. The person who asked can never be the one who approves.
- Continuous actions (PTZ, live control of a display) are approved once for a limited time on that station, not per
  press; every press is still logged.

### 3. SCADA users (planned 0.95.0)

- A second level of identity that exists **only in the pictures** (operator view, process pictures, dashboards with
  controls): a list of SCADA users per organization, each with a PIN or a password (the administrator chooses per
  user) and membership in **groups** (Operator, Shift leader, Technologist, Maintenance by default; the administrator
  changes them). A SCADA user cannot sign in to the application and has no access to its API.
- A person signs in **in the picture** on top of the session of the station and is signed out after a time without
  activity. Without a SCADA user the picture is read-only when the organization requires one.
- Every control element and widget can name the group that may use it. The server checks it: the command carries the
  SCADA user, and the effective right is the intersection of what the station's account may do and what the group
  allows — a SCADA user never gets more than the station.
- Commands, the operator log and four-eyes approvals record the SCADA user next to the station account.

## Consequences

- One more table that grows with every press and page; a retention setting comes with step 2 at the latest.
- `audit()` reads the policy for each entry; it is cached for five seconds per organization.
- Step 3 adds credentials that are weaker than account passwords (a PIN). They are confined to pictures, rate
  limited, never valid for the API, and every use is in the operator log. Organizations that need more choose
  passwords per user.
- Pages send `X-Ctrl32-Screen` with every change so that the log can name the page.

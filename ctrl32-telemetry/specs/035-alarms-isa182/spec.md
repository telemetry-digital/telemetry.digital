# 035 — Alarm management after ISA-18.2 (0.89.0)

## Why

Operators judge a SCADA by its alarm system. Until 0.88 an alarm was a row with the states active / acknowledged /
cleared, shown only in dashboard widgets: an alarm that came and went while nobody looked left no trace on the screen,
every alarm looked the same, a nuisance alarm could not be put aside, and nothing told which alarms fire most.
ANSI/ISA-18.2 (IEC 62682) names what a professional alarm system has; this spec brings the operator-facing part.

## What

### 1. Alarm states (ISA-18.2 state model)

The condition (active / returned to normal) and the acknowledgement are two independent facts:

| `isa_state` | Condition | Acknowledged | Shown |
|---|---|---|---|
| `unack_active` | active | no | blinking, in the class colour |
| `ack_active` | active | yes | steady, in the class colour |
| `unack_rtn` | returned to normal | no | blinking, pale: it happened and nobody confirmed it |
| `normal` | returned to normal | yes | history only |
| `shelved` | active | — | separate list, not annunciated |

- `core.alarm` gains `acked_at`, `acked_by`, `class_id`, `shelved_until`. The existing `state` stays
  (`active`, `acknowledged`, `suppressed` = shelved, `cleared`) so earlier clients keep working.
- An alarm that returned to normal can be acknowledged (before: 404).
- Alarms of a class without acknowledgement are acknowledged by the system when raised.
- Migration: alarms cleared before the upgrade count as acknowledged (at their acknowledgement or clearing).

### 2. Alarm classes

Per organization, created on first use: **Alarm** (priority 1, red, acknowledgement, siren), **Warning** (priority 2,
amber, acknowledgement, beep), **Message** (priority 4, blue, no acknowledgement, silent). Each class: code, name,
priority 1–4, colour, acknowledgement required, sound (`none`, `beep`, `siren`). Changes need `config.write` and a
reason and are audited. A rule may name its class; without one the severity decides (`action`/`critical` → Alarm,
`warning` → Warning). A class in use cannot be deleted; the three built-in ones never.

### 3. Shelving

An operator (`alarm.ack`) shelves an alarm for 15 minutes to 24 hours with a reason: the alarm and every new alarm
of the same rule until then is recorded but not annunciated (no banner, no sound, no notification, no escalation).
The shelf ends by itself, or earlier by hand with a reason; alarms still active then return to the list and, when not
acknowledged, are annunciated. Shelving and its end are alarm events (`suppressed`, `unsuppressed`) and audit entries.
A new version of the rule ends its shelf.

### 4. Alarms page (`/alarms`, menu Monitoring)

- **Current**: everything that needs attention (`unack_active`, `ack_active`, `unack_rtn`), ordered by priority,
  unacknowledged first, newest first; class colour, time, source (asset / device / datastream), value and limit,
  state, duration. Acknowledge one, the selected or all shown with one reason; shelve.
- **Shelved**: who, why, until when; unshelve.
- **History**: any range, filter by class and text, export through the existing alarm export.
- **Statistics** (ISA-18.2 performance): alarms per day and per hour (target: about 6 an hour per operator), the ten
  most frequent alarms with their share, standing alarms (active longer than 24 h), chattering alarms (3 or more in a
  minute), mean time to acknowledge.
- **Classes**: the table of classes (with `config.write`).

### 5. Alarm bar on every page

Under the top bar of every page for people with `data.read`: the most important unacknowledged alarm (class colour,
blinking), the numbers of unacknowledged and active alarms, a link to the page; the class sound repeats until the
alarm is acknowledged. Sound is per browser, off until the user switches it on (browsers refuse sound before a
gesture); the bar can be hidden per user for the session. Not on public links and paired displays in this version.

## API

- `GET /api/v1/alarms` — as before; additionally `view=current|shelved|history`, `from`, `to`, `class`, `q`, and the
  fields `isa_state`, `acked_at`, `acked_by`, `shelved_until`, `class`, `source`, `limit`.
- `POST /api/v1/alarms/{id}/ack` — also for alarms returned to normal; `POST /api/v1/alarms/ack` `{ids, reason}`.
- `POST /api/v1/alarms/{id}/shelve` `{minutes, reason}`, `POST /api/v1/alarms/{id}/unshelve` `{reason}`.
- `GET /api/v1/alarms/summary` — counts and the top alarm for the bar.
- `GET /api/v1/alarms/stats?from&to`.
- `GET|POST /api/v1/alarm-classes`, `PUT|DELETE /api/v1/alarm-classes/{id}`; `class_id` in `POST /api/v1/alarm-rules`.

## Out of scope (later)

Suppression by design (alarm hidden by a plant state), out-of-service with a permit, alarm groups with a collective
indicator in process pictures, the bar on paired displays, first-out, per-operator consoles.

## Tests

Unit: state derivation, class resolution, shelf expiry. e2e `alarms.sh`: return to normal unacknowledged →
acknowledge; bulk acknowledge; class without acknowledgement; shelve → no notification, new alarm shelved, unshelve →
annunciated; classes CRUD and permissions; summary and statistics. Headless: page, bar, blinking, shelving dialog.

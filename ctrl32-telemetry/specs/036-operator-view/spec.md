# 036 — Operator view: screen frame, picture tree, group alarms, faceplate types (0.90.0)

## Why

A process picture is today a page of the web application: sidebar, page head, the picture. Going to another picture
reloads the page, pictures form a flat list, nothing tells in which part of the plant an alarm waits, and a motor
window (0.87.0 `popup`) is a separate picture per motor with fixed tags. A control-room station of WinCC has a fixed
frame (alarm line on top, picture in the middle, navigation buttons below), a picture tree whose buttons blink when an
alarm waits anywhere below them, and faceplate types drawn once and opened for any motor by a tag prefix. This spec
brings these four things; the pictures, the editor and their versioning stay as they are.

## What

### 1. Picture tree

A screen (`core.dashboard`, any kind) may name its place in the tree in its spec:

```json
"nav": {"parent": "<slug>", "order": 10, "label": "Aeration"}
```

- `parent` — slug of another current screen of the organization; absent = top level. Cycles and a depth over 5 are
  refused. A parent that is later deleted leaves its children at the top level (resolved at read time, never an error).
- `order` — 0–9999, siblings sort by it, then by name. `label` — up to 24 characters for the button; default the name.
- Only screens with `nav` are in the tree; a screen without it stays reachable by actions and the list as before.
- It is part of the spec, so a change is a new version with a reason, like any change of a screen.
- The editor's *Screen properties* gain *Place in the operator view*: parent, order, button label.

### 2. Group alarms

The alarms of a picture are the alarms that need attention (`unack_active`, `ack_active`, `unack_rtn`; not shelved)
on a datastream the picture uses: element tags, dynamics' tags and widget datastream bindings of the current version.
The group state of a tree node is that of its own picture and of every picture below it:

| Field | Meaning |
|---|---|
| `unack` | alarms waiting for acknowledgement |
| `active` | alarms whose condition is active |
| `color`, `priority` | class of the most important one (priority, unacknowledged first) |

No configuration: drawing a tag into a picture puts its alarms on the picture's button. Alarms of automation rules
without a datastream belong to no picture; they show in the alarm line only.

### 3. Operator view (`/operator`, `/operator/{slug}`)

A page without the application menu, for a control-room monitor. Permission `data.read`; menu *Monitoring → Operator
view*. Fixed frame, top to bottom:

1. **Header** — logo, name of the current picture and its path in the tree, date and time, signed-in user, a button
   back to the application, full screen.
2. **Alarm line** — the most important unacknowledged alarm in its class colour (blinking), the counts, the sound
   toggle, **Acknowledge** for the shown alarm (`alarm.ack`, reason dialog) and a button to the Alarms page. The
   same data as the alarm bar of 035; the application's own bar is not shown here.
3. **Work area** — the picture, scaled to fit both width and height, live.
4. **Button bar** — *Home* (first top-level picture), *Back* (history of this tab, up to 20), *Up* (parent), then one
   button per child of the current picture, or per sibling when it has none. A button carries the group state: class
   colour, blinking while an alarm below it is unacknowledged, steady while active and acknowledged, plain otherwise.

- Changing the picture does not reload the page: the `screen` action and the buttons swap the work area and push the
  browser history (`/operator/{slug}`), so the browser's Back works too.
- `/operator` opens the organization's default screen when it is in the tree, else the first top-level picture; with
  an empty tree it lists the screens and says how to place them.
- The tree with group states is polled every 5 s together with the alarm summary.
- Commands, faceplates and popups work as on the dashboard page (same permissions, reasons, four eyes, audit).
- Paired displays and public links are unchanged (no user, no commands) — out of scope.

### 4. Faceplate types

Datastreams belong to an asset and have a key unique in it (`speed`, `current`, `m1_run`). A picture becomes a
**faceplate type** by naming the asset it was drawn for:

```json
"faceplate": {"asset": "<uuid>", "prefix": "m1_"}
```

It is drawn against one real example (WYSIWYG, live values in the editor). Opened for another instance, every
reference to the example is re-bound **by datastream key**:

- a tag of the example asset whose key starts with the type's prefix → the datastream of the instance asset with the
  key `instance prefix + rest` (element tags, dynamics' tags, widget datastream bindings);
- the `device` of an element (the target of its commands) → the device assigned to the re-bound datastream; a command
  key that starts with the type's prefix gets the instance prefix;
- a tag that has no counterpart is unbound (shows "—", its dynamics are dropped); tags of other assets or outside the
  prefix stay as drawn (shared values such as a supply voltage).

Both common layouts are covered: one asset per motor (prefix empty, the asset changes) and one asset with all tags of
a PLC (`m1_run`, `m2_run`: the asset stays, the prefix changes).

The `popup` action gains optional `asset` and `prefix`:

```json
{"act": "popup", "target": "motor", "prefix": "m2_"}
```

Without `asset` a popup of a faceplate type takes the asset of the clicked element's own tag; without `prefix` the
type's prefix. So a motor symbol bound to any tag of its motor needs only "popup: motor". A popup of an ordinary
picture behaves as before. The window title is the type's name and the instance (`Motor — Pump P2`). Windows can be
dragged; up to four stay open at once, the fifth closes the oldest.

The server does the re-binding: `GET /api/v1/dashboards/{slug}/instance?asset=&tag=&prefix=` returns the spec bound to
the instance (permission `data.read`; asset and tag must belong to the organization), so the browser never guesses
UUIDs.

## API

| Method and path | Permission | |
|---|---|---|
| `GET /api/v1/screens/tree` | `data.read` | nodes `{slug, name, label, kind, parent, order, unack, active, color, priority}` in tree order, plus `home` |
| `GET /api/v1/dashboards/{slug}/instance?asset=&tag=&prefix=` | `data.read` | faceplate type bound to an instance; 400 when the screen is no faceplate type |
| `GET /operator`, `GET /operator/{slug}` | session | the page |

`PUT /api/v1/dashboards/{slug}` validates `nav` and `faceplate`.

## Out of scope (later)

- The operator view on paired displays and public links; several monitors of one station.
- Alarm state as a dynamic of a drawn element (frame of the motor blinking in the picture).
- Per-user start picture and per-role picture tree; picture-level permissions.
- Live link of library composites to their type (composites stay copies).

## Tests

- Unit: tree building (order, cycle, missing parent, depth), group state aggregation, re-binding by key (asset
  change, prefix change, missing counterpart, foreign tags untouched, command actions).
- e2e `tests/e2e/operator.sh`: nav validation, tree with group states following raise / acknowledge / clear / shelve,
  instance endpoint, permissions.
- Headless `tests/headless/operator.js`: frame, navigation without reload, Back/Up/Home, blinking button, alarm line
  acknowledge, faceplate popup for two instances, screenshots.

# 038 — Recipes (0.92.0)

## Why

A plant makes several products on the same machines. Each product is a set of setpoints — speed, temperature, times,
quantities — and changing the product means writing all of them to the controller, the same every time. Typing them
one by one into setpoint widgets is slow and a single mistyped value spoils a batch. WinCC has recipes for this:
named data records that are loaded into the PLC with one action. Fourth step of "SCADA as professional as WinCC"
(035 alarms, 036 operator view, 037 tag import).

## What

### Model

- A **recipe** has a name, **elements** and **units**.
- An **element** is one writable key of a device: key, shown name, unit of measure, type (`number`, `bool`, `text`)
  and, for numbers, limits and decimals. The order of the elements is the order of writing.
- A **unit** is where the recipe can be loaded: a device of an S7, OPC UA or Modbus connector and an optional **key
  prefix**. Ten mixers behind one PLC are ten units with the prefixes `m1_` … `m10_` and one recipe (the tag import of
  037 writes the same prefixes).
- A **data record** is a numbered, named set of values, one for every element. Every change raises its version and
  goes to the audit trail with the old and the new values.

### Loading a record into a unit

1. **Check before the first write.** Every value is validated by the connector exactly as a single command would be:
   the key exists, is writable, the type fits, the connector's own limits hold. One refusal and **nothing is
   written** (409 with the list of problems). `dry_run` runs only this check; the page shows it before the user
   confirms, next to the values the unit holds now.
2. **Writing in order.** Each value is a command in the command log (reason `recipe …, record …: <reason>`), written
   and read back. The load **stops at the first value the controller refuses**; the rest is reported as not written.
   The controller should take the values over only on a bit that is the last element of the recipe.
3. **The load record.** Who asked, which record (name, number, version), the values as written, the unit, the answer
   for every value, and who approved it.

Safety:

- Permission `device.command` and a reason, as for any command. The limits of an element are checked when the record
  is saved; the limits of the connector when it is loaded. Neither can be bypassed by the other.
- **Four eyes.** With the organization policy `four_eyes_commands` a load waits as `pending` until a different user
  with `device.command` approves it (or anyone with the permission rejects it). The values are copied when the load is
  asked for: what is approved is what is written, whatever happens to the record meanwhile. Everything is checked
  against the connector again at approval. A load nobody decides within 24 hours expires.
- One load at a time into a device.
- A load is finished even when the browser goes away in the middle.
- A record that no longer fits the recipe (an element was added, a limit was narrowed) is marked and cannot be loaded
  until it is saved again.

### The values in the unit

`GET /api/v1/recipes/{id}/compare` returns the value of every element as the connector last read it (its poll, not a
read on demand) and whether it equals a record (numbers within half of the last decimal of the element). The page
shows the column *In the unit now*, marks differences, and **Take the values from the unit** fills the form — a
record can be made from a unit that was tuned by hand.

### Permissions

| Action | Permission |
|---|---|
| see recipes, records, loads, the values in a unit | `data.read` |
| elements and units of a recipe | `config.write` |
| create, change, delete, import data records | `recipe.edit` (new; roles engineer and org_admin) |
| load a record, approve or reject a load | `device.command` |
| export records as CSV | `data.export` |

An operator loads records; a technologist changes them.

### CSV

Export: `number, name, note` and one column per element (its key), UTF-8 with a byte order mark. Import of the same
table (comma or semicolon; with semicolons a decimal comma is accepted): an existing number is changed, a new one is
added; every line is checked like a saved record; a column that is not an element, a missing column or one bad line
refuses the whole import.

### API

`/api/v1/recipes` (list, create), `/api/v1/recipes/{id}` (read, change, delete), `…/records` and `…/records/{rid}`,
`…/records/{rid}/download`, `…/compare`, `…/export`, `…/import`, `/api/v1/recipe-downloads` and
`/api/v1/recipe-downloads/{id}/approve|reject`. Tables `core.recipe`, `core.recipe_record`, `core.recipe_download`
(migration 0062); deleted recipes and records stay in the database for the loads that refer to them.

### UI

Menu → **Recipes**: the recipe, its records on the left, the selected record on the right with the value of every
element, its limits and the value in the chosen unit; *Load into the unit*, *Save*, *Save as a new record*, *Take the
values from the unit*, CSV. **Loads**: the log with the state of every load, its detail and, for four eyes, *Approve*
and *Reject*. Limits: 200 elements, 50 units, 1000 records per recipe.

## Not in scope

- A batch record (start and end of a batch, the trends of its values, consumption) — it builds on the load record
  and comes with the trends of step 5.
- Loading a recipe from a process picture or from a flow; recipes for devices that are not behind a connector (MQTT
  devices of the own protocol).
- Reading the values from the controller on demand (the comparison uses the connector's last poll) and an automatic
  handshake with the PLC (status word, "recipe accepted") — the take-over bit as the last element covers the common
  case.
- Electronic signature of a load (planned with the e-signature step).
- Server messages with values in them ("… is above the maximum …") are shown in English.

## Tests

- Unit `internal/httpapi/recipes_test.go`: structure of a recipe, values against types and limits (rounding before
  the limit), comparison with values of the connector, CSV both ways and its refusals.
- E2E `tests/e2e/recipes.sh` (82) against the S7 simulator: two units behind one controller, the check, the load with
  read back, the other unit untouched, a value the connector refuses (nothing written), a value the controller
  refuses in the middle (the rest not written), a record that no longer fits, four eyes (the values asked for are
  written, not the record changed meanwhile), permissions of an operator, CSV, deleting, the audit trail.
- Headless `tests/headless/recipes.js` (42): the dialogs and the page in the browser, also at phone width.

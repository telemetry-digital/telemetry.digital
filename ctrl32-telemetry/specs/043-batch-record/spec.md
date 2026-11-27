# 043 — Batch record, trend and batch widgets (0.97.0)

Closes step 5 of "SCADA as professional as WinCC" (trends with the batch record).

## Why

A plant makes its product in batches. After a complaint, or before releasing a lot, somebody asks: when did batch 44
run, what was loaded for it, who started it, how did the temperature go, and how does it compare with the batch
before? Until 0.96.1 the answer was put together by hand from the load log of recipes and a trend with typed times.

The owner chose how a batch begins: **both** ways, per unit — the load of a recipe record, or a tag of the controller.
And: "widgets of all of it too".

## What

### Units

*Batches → Units* (`device.manage`, a reason, audited). A unit is a place where batches run. It says:

- **Batches begin with**
  - *the load of a recipe record*: the device and the key prefix of the unit, as in its recipes. A load that finished
    begins a batch and ends the one that ran; a check, a refused or a failed load begins nothing. The last batch is
    ended by a person.
  - *a tag of the controller*: a datastream, read as
    - *a run bit* — not zero while a batch runs: the change from zero begins a batch, the change to zero ends it;
    - *the batch number* — a new number ends the batch that ran and begins the next one, which carries the number as
      its code; zero means no batch.

    The batch begins and ends **at the time of the sample**, not when the server noticed. The tags are looked at every
    five seconds; samples of doubtful quality are skipped. A unit that was just set up starts watching from now — what
    the tag did before is not a batch.
- **The values drawn for each batch** (at most 16 datastreams).
- *Batches begin and end by themselves* can be switched off; then only a person begins and ends them.

At most 200 units. Removing a unit ends its running batch and takes it from the lists; its batches stay.

### Batches

- One batch runs on a unit at a time: a batch that begins ends the one that ran, in one transaction.
- A batch has a number counted per unit, a code (the number from the controller, the recipe record, or typed), when it
  began and ended, what began and ended it (recipe, tag, a person), the recipe, record and load, the account and the
  SCADA user at the controls, and the reasons.
- **By hand** on any unit: `POST /api/v1/batches` and `POST /api/v1/batches/{id}/end` — `device.command`, a reason, the
  audit trail (`batch.start`, `batch.end`), the operator log, and the person at the controls when the organization
  requires one.
- **A batch is a record.** It is never deleted. When it ends it keeps the curves of its unit and their statistics
  (samples, minimum, maximum, average, the first and the last value), so the record says the same after the unit is
  changed. A running batch shows the statistics so far.

### The pages

Menu → **Batches** (`/batches`, `data.read`).

- The list: unit, state, time, search by number, code and recipe.
- The record of a batch: its head, the trend of its curves with ruler A at its beginning and B at its end and the
  alarm limits, the statistics, the alarms of its values raised during it, *Open in Trends*, *Print*, the batch before
  and after it, *End the batch*.
- **Compare with another batch of this unit**: the second batch under the first, both over the same length of time
  from their beginning, with the statistics of the second and the difference of the averages.

### Widgets

- **Trend** (charts): the trend control of the Trends page with the bound datastreams as curves on axes by unit — a
  live window, dragging, the wheel, a ruler; optionally the buttons for the time, a table (value at the ruler,
  minimum, maximum, average) and the alarm limits. It keeps the time a person moved to when the dashboard refreshes.
  Works on dashboards, in process pictures, in the operator view and on displays; on a public link without alarm
  limits.
- **Batches** (lists): the batch that runs on a unit (or on every unit) and the last batches, each a link to its
  record. For signed-in users.

### Fixed on the way

The trend control took every gap longer than three buckets for a hole in the data. A value sampled once a minute,
looked at in a window of six-second buckets, was then not drawn at all (0.96.0). The usual step of the data counts
now.

## Not in scope

- A batch report as a signed PDF and the e-signature of a batch (with the e-signature step).
- Phases and steps inside a batch (ISA-88), consumption of materials, genealogy of lots.
- Laying the curves of two batches over each other in one chart (they are drawn under each other on the same scale
  of time).
- Beginning a batch from a flow or a rule.

## Tests

- E2E `tests/e2e/batches.sh` (49): units, a run bit, a batch number, by hand, one batch per unit, the statistics kept
  with the record, permissions, reasons, the audit trail. `tests/e2e/recipes.sh` (+6): a load begins a batch, the next
  load ends it, a failed load begins nothing.
- Headless `tests/headless/batches.js` (en, sk): units, the list, the record, the trend, statistics, comparison,
  ending and beginning by hand, both widgets, phone width; with `tests/headless/batches_demo.sh` hour-long batches.
- Unit: `batches_test.go` (what a sample of the tag means).

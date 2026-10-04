# Period tables: daily logs and consumption with export

The dashboard widget **Period table** (0.63.0, spec 023) shows values of datastreams per local period — hour, day,
week, month or year in the organization's time zone — and exports exactly that table to **Excel (XLSX)**, **CSV** and
a **PDF report** (stored under *Reports → Stored reports* with its hashes).

## Typical set-ups

| Need | Datastreams | Row per | Values | Period | Rows | Other |
|---|---|---|---|---|---|---|
| Daily temperature log of a medicine fridge | the fridge sensor | day | min and max | this month | a row per period | lower limit 2, upper limit 8, total row |
| Monthly consumption of electricity meters | all meters (kWh counters) | month | consumption | last 12 months (or this year) | a row per datastream | total row |
| Meter readings for accounting | the meters | month | start, end and consumption | last month | a row per datastream | |
| Hourly load of one day | a meter | hour | consumption | today | a row per period | |

Names of the datastreams in the table come from the widget's *Series* tab (e.g. "Main meter", "Heating").

## What the values mean

| Value | Meaning |
|---|---|
| min, max, average | of the samples in the period |
| start, end | the first and the last reading in the period |
| consumption | the sum of increases of a counter; a meter reset counts the new reading; the last reading before the period is the baseline, so consecutive periods add up |
| samples | number of readings |
| out of limits | readings below the lower or above the upper limit |

Periods start at local midnight and on the local 1st of the month, also on the days the clocks change (23 or 25 hours).
Days without data stay empty. Values outside the limits are red; a `*` marks a period with readings of doubtful quality
(not *ok* and not *backfilled*). ◀ ▶ move to the previous or next period, *Current* returns; the export takes the
period shown.

## Exports

Excel, CSV and PDF buttons appear for users with the permission `data.export` (not on public links). The file carries
the title, the period, the time zone, the organization, the author and the time; headers are in the user's language.
Every export is in the audit trail (`export.read`).

API: `GET /api/v1/period-table` and `GET /api/v1/period-table/export?format=xlsx|csv|pdf` (see `/api/docs`); both are
also tools of AI assistants over MCP.

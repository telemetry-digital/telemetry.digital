# Feature Specification: Period tables with export

**Feature Branch**: `feature/period-tables`

**Created**: 2026-09-28

**Status**: Implemented (0.63.0)

**Input**: The user (28. 9. 2026), before deploying to the first customers: both need overviews — one of medicine
temperatures, one of energy consumption on electricity meters in rows — "normally as a table and then an export"; then
"medicines one sensor per day, energy per month by meters … we need this globally for the future". Chosen: a daily log
(a row per day), exports as Excel (XLSX), CSV and a PDF report.

**Related**: spec 002 (widgets), spec 008 (energy: consumption of counters), ADR-0010 (API first, content), ADR-0016/7
(nothing paid here); URS J-34.

## Clarifications (defaults)

- One general widget instead of two special ones: rows are local periods (hour, day, week, month, year) or datastreams,
  cells are chosen values (min, max, average, first and last reading, consumption, samples, samples outside limits).
- Periods are local to the organization's time zone (days start at local midnight, months at the local 1st), so a daily
  log matches the calendar and the paper record, also on daylight-saving days.
- Consumption uses the rule of the energy overview (spec 008): the sum of increases between readings, a meter reset
  counts the new reading, the last reading before the period is the baseline.
- The export is the table the server computes — the same numbers as on the screen, in the user's language.
- Exports need `data.export` and are audited; a PDF is stored under Reports with its hashes like every PDF export.
- A public dashboard link shows the table (bound datastreams only) without export.

## User Scenarios

1. A pharmacy: a dashboard with *Period table* for the fridge sensor — row per day, values *min and max*, period
   *this month*, limits 2 and 8 °C, total row. Days outside the limits are red. At the end of the month the pharmacist
   presses *PDF* and files the report; *Excel* for the quality system.
2. An operator of a building: *Period table* for all electricity meters — rows *a row per datastream*, a row per month,
   values *consumption*, period *last 12 months*, total. *Excel* goes to accounting.
3. ◀ ▶ move to the previous and next period; the export follows the shown period.

## Requirements

- FR-001 `GET /api/v1/period-table` (`data.read`): datastreams (1–50), interval hour|day|week|month|year, values
  (min, max, avg, first, last, delta, count, out), period (today … last_12m, custom from/to days), shift (whole
  periods), low/high limits, orientation, total, decimals, labels. Answer: time zone, columns, every period of the range
  (empty cells without data), cells and totals per datastream; at most 1000 periods.
- FR-002 Aggregation in the database per datastream and local period (`date_trunc` in the organization's time zone);
  totals: min of minima, max of maxima, average of all samples, first and last reading, sum of consumption, samples,
  samples outside the limits; a period is flagged when a sample has a doubtful quality (not ok, not backfilled).
- FR-003 `GET /api/v1/period-table/export?format=xlsx|csv|pdf` (`data.export`): the same table with title, period,
  time zone, organization, author and time above it (XLSX: frozen header, numbers as numbers with decimals, bold total;
  CSV: UTF-8 with BOM; PDF: generic report with the organization's logo, stored under Reports); headers in the user's
  language (or `locale`); file name from the title and period; audited as `export.read`.
- FR-004 `GET /api/v1/public/{token}/period-table`: only datastreams bound in the shared dashboard.
- FR-005 Widget *Period table* in the catalog: row per, values (readable presets), period, rows (periods or
  datastreams), total row, lower and upper limit, decimals, export buttons; per-datastream labels from the series
  settings; ◀ ▶ and *Current*; values outside the limits in red; export buttons only for users with `data.export` and
  not on public links; readable option names in the editor; the showcase dashboard includes it.
- FR-006 XLSX without a new dependency (`internal/xlsx`): a single sheet, inline strings, number formats 0–6 decimals,
  bold, column widths, frozen rows.
- FR-007 Translations of the new texts in all 19 catalogs; the operation is offered to AI assistants over MCP
  (data area) automatically.

## Success Criteria

- SC-001 Unit: local periods across daylight-saving changes (23 h and 25 h days, 23 hours on the spring day), named
  periods and shifts, validation, totals, the export grid in both orientations, file names; XLSX parts well-formed.
- SC-002 e2e `period_table` (40): fridge daily log (days, totals, samples outside limits, local midnight), meters
  (consumption, start and end readings, days add up, hours, previous month, custom range), validation, exports (XLSX,
  CSV with BOM and header, PDF, audit), widget in a dashboard with validated options, public link (bound only, no
  export).
- SC-003 Headless `period_table`: the widget renders rows, ◀ ▶ and *Current*, export links follow the period, the
  Excel download is a workbook, readable options in the editor; screenshots on desktop and phone.

# 042 — Trends (0.96.0)

Step 5 of "SCADA as professional as WinCC" (after alarms, the operator view, tag import, recipes and ADR-0026).

## Why

A chart widget shows the curves its author chose, for the range the dashboard has. An operator who looks for the cause
of an alarm needs the opposite: pick any measured values **now**, put them on one time axis, go back to last night,
zoom into ten minutes, read the values at two moments and the difference between them. That is the trend control of
WinCC; until 0.95.0 the closest thing was the small trend of a faceplate.

## What

### The trend page

Menu → **Trends** (`/trends`, `data.read`).

- **Curves**: *Add curves* lists the datastreams of the organization with a search; at most 16 in a view. Each curve
  has a colour, a label, the way it is drawn (line, steps, points; automatic = steps for states), and its axis: curves
  of the same unit share an axis, a curve can have an axis of its own, and an axis can have a fixed range. Up to six
  axes are labelled (two on a phone). A click on the colour hides a curve.
- **Time**: live windows of 5 min to 30 days that follow the clock, or a fixed window (*From*, *To*). Dragging moves
  in time, the wheel zooms around the pointer, *Zoom to selection* zooms into a dragged box, the arrows move by half a
  window, `+` `−` zoom; the keyboard does the same (arrows, `+`, `−`). Anything that leaves the present ends the live
  mode; *Live* returns.
- **Rulers**: the pointer shows the time and the value of every curve under it; a click places ruler **A**, with
  *Ruler B* on (or Ctrl+click) ruler **B**. The table shows the values at the pointer, at A, at B, and B − A; the line
  above it the two times and the time between them.
- **Statistics** of the window shown: minimum, maximum and average of every curve.
- **Alarm limits** of the curves as dashed lines (warning, action) and **Alarms** as bars along the top from the
  moment an alarm was raised until it cleared.
- **Export**: *CSV* (the points of the window, one row per time; needs `data.export`) and *PNG* (a picture of the chart).
- The address carries the view, or the datastreams and the time, so a trend can be sent as a link; the window of a
  process-picture object has *Open in Trends*.

### How the data are read

`GET /api/v1/observations` per curve. A window is loaded with half a window on each side; the server buckets it so that
a window is about 1500 points whatever its length (raw samples below 5 s per point, at most 10 000). With buckets a
point is the average, and a band shows the lowest and the highest value of each bucket; the page says so under the
chart. A hole longer than a few usual steps breaks the line. Samples of doubtful quality are circled when few enough
are shown.

### Saved views

`core.trend_view` (migration 0066), `GET/POST /api/v1/trends`, `GET/PUT/DELETE /api/v1/trends/{id}`.

- A **personal** view is a person's own selection, like the remembered filters of a dashboard: only its owner reads
  and changes it; no reason, not audited.
- A view **shared with the organization** is configuration: `content.write`, a reason, the audit trail
  (`trend_view.create/update/delete`). Everybody with `data.read` sees it. Taking it back needs a reason too and makes
  it the caller's own.
- A view remembers its curves, the live range or the fixed window, and whether limits and alarms are drawn.

## Not in scope

- The **batch record** (start and end of a batch, its trends, comparing batches) — the next release; it builds on
  this control and on the load record of recipes.
- A trend widget for dashboards, process pictures and displays (the chart widget stays what it is).
- Comparing a curve with itself at another time, calculated curves, curves of text values.
- Server-side export of a trend (the CSV holds what the page has loaded: averages for long windows; raw data are
  exported with *Reports → Export*).

## Tests

- E2E `tests/e2e/trends.sh` (40): validation of a view, personal and shared views, permissions, reasons, the audit
  trail.
- Headless `tests/headless/trends.js` (en, sk) with real mouse input: adding curves, axes, statistics, the pointer,
  rulers A and B, ranges, the wheel, dragging, the zoom box, live, curve settings, limits, saving a personal and a
  shared view, links, the export table, the picture, another user, phone width. Needs datastreams with history
  (`deploy/dev/demo/production.py setup`).
- Unit: `trends_test.go`.

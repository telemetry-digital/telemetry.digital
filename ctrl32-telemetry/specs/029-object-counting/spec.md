# Feature Specification: Object counting (people and vehicles) and scheduled reports by e-mail

**Feature Branch**: `feature/0.71-counting`

**Created**: 2026-10-02

**Status**: Implemented (0.71.0)

**Input**: The owner (2 Oct 2026): counting people coming in and leaving through the entrances of a building, the
share of each entrance, the number of people inside, outputs and statistics mailed on a schedule. Then: "the system
itself does the counting and writes it into telemetry; there will also be sensors and it will be aggregated in various
ways — we are building a secure system without random cameras' analytics": an own detector working with any RTSP
camera, sensors through the device protocol, vehicles as well as people, and the speed of vehicles between two lines.
Counting from third-party software is not part of the product; whoever has counts elsewhere sends them through the
generic device protocol (MQTT/HTTP).

**Related**: spec 010–013 (cameras, ONVIF, relay), spec 019 (motion detection on the server), spec 023 (period
tables), spec 008 (energy), ADR-0009 (licences; amended by ADR-0022), ADR-0010 (API first), ADR-0022 (analytics
worker); URS J-35, J-36.

## Clarifications (defaults)

- **Model on assets and datastreams, not a parallel store.** An *area* (building, car park, zone) is an asset of type
  `counting_area` on a site, an *entrance* is an asset of type `entrance` under it, a *speed section* an asset of type
  `speed_section` under its area. Counts are observations: `<group>_in`, `<group>_out` (kind `event`, the value is the
  number of passages) on the entrance and the area, `<group>_occupancy` (gauge) on the area, `vehicle_speed` (km/h,
  with class and direction as `value_json`) and `over_limit` on the section. Groups: `people`, `vehicles` (car,
  motorcycle, bus, truck together), `cars`, `trucks`, `buses`, `motorcycles`, `bicycles`.
- **Occupancy** = arrivals − departures per group since the last reset, kept in `core.counting_occupancy`; never below
  zero (a departure that would make it negative clamps it at zero, the stored value has quality `out_of_range`, the
  clamp is logged); a daily reset at a local time (e.g. 03:00, also when the server was down then, DST-safe); a
  correction by hand with a reason (counting log + audit trail). Alarms on the occupancy are the ordinary alarm rules
  on `<group>_occupancy` (the page offers "Capacity alarm").
- **Sources**: (1) the server's own detector on camera pictures — lines drawn on the picture, each counting for an
  entrance, the side of the area chooses in and out, classes per line; (2) sensors — any datastream of a device
  (light barrier, turnstile, door counter) mapped to an entrance and direction, values either "passages since the
  last message" or "running total" (a restart from zero counts the new value). No webhook endpoint of our own: sensors
  and other systems use the device protocol (token per device).
- **The detector** (ADR-0022): a separate worker process per camera (`ctrl32-telemetry analytics-worker`); the server
  feeds it the camera's sub-stream from its own subscription (direct and relayed cameras alike, the camera is
  connected once), an unmodified ffmpeg decodes it, YOLOX nano (default) or tiny runs through ONNX Runtime on the CPU,
  a tracker of our own follows objects, crossings and speeds go back as JSON lines. 1–15 analysed pictures per second
  (default 5). Off by default per camera, `config.write` to configure, every change audited with a reason. No picture
  is stored or sent anywhere; the line editor shows a picture taken in the browser from the live view (privacy masks
  burnt in) and boxes of the last analysed frame.
- **Counting rules**: a crossing counts when a confirmed track (3 detections) moves from one clear side of the line to
  the other through the drawn segment (dead band ±2 % of the picture), once per track and line; boxes cut by the
  picture border are ignored (objects coming into view); the moment of the crossing is interpolated between frames.
- **Speed**: two lines and the real distance between them on the ground; speed = distance ÷ time between crossing line
  A and line B of the same track (with direction A→B or B→A); plausible range 1–250 km/h (configurable); vehicles
  only by default; an over-limit passage is stored on `over_limit`. Indicative, not certified enforcement; no number
  plates, no personal data. Error: ±1 frame period without interpolation (10 fps, 10 m, 50 km/h: 0.1 s of 0.72 s ≈
  ±14 %); with interpolation of the crossing moments steady motion is within a few per cent; perspective and the
  ground-point of the box add to it.
- **Outputs**: page *Object counting* (occupancy now, today in/out, peak, by hour with the occupancy, entrances with
  shares, comparison of days, speed sections, resets/corrections, export), widgets (occupancy, in/out per entrance,
  share per entrance, vehicle speeds), period tables gain the value `sum`, exports through the period table export.
- **Aggregation**: per entrance, per area, per site (the overview sums areas of a site), per period (period tables:
  hour/day/week/month/year); further formulas (share, peak, combinations of areas) with the existing automation rules
  ("derived datastream" with the expression language) or flows ("Store value").
- **Scheduled reports**: Settings → Scheduled reports: content (counting summary of an area, a period table widget of
  a dashboard, a report template: observations of a datastream or alarms), period (previous day/week/month in the
  organization's time zone), cron (five fields, presets daily 07:00, Monday 07:00, 1st of the month 07:00; DST:
  a skipped time runs at the first moment after the jump, a repeated time once), recipients (e-mail channels and
  addresses), PDF and/or XLSX (CSV for templates), enable/disable, *Send now*, delivery log (append-only); runs as the
  author with read/export permissions; audited.
- **Privacy (GDPR)**: counts and speeds are aggregate numbers; no image, track, face, number plate or identifier is
  stored or leaves the worker. A camera that counts still films people — its recording, masks and viewing follow the
  camera's own rules (specs 010–016); counting itself does not need recording.

## User Scenarios

1. A plant: the main building with three entrances counted by light barriers (sensors) and one camera line; the page
   shows 152 people inside, the shares of the entrances and the shift peaks; a capacity alarm at 300 people.
2. A car park: cars and trucks at the gate, free spaces = capacity − occupancy; the plant road camera measures speed
   between two lines 18 m apart, 34 of 207 vehicles above 30 km/h yesterday.
3. The facility manager gets a PDF and an Excel file with yesterday's visitors every morning at 07:00.
4. An integrator draws a counting line on the entrance camera, chooses "person", sees the boxes of the detected people
   and the counts rising in the editor.

## Requirements

- FR-001 Areas and entrances: `GET/POST/PUT/DELETE /api/v1/counting/areas…` with assets and datastreams created
  automatically; `config.write`, reason, audit; archive keeps the data.
- FR-002 Sensor inputs: `GET/POST/PUT/DELETE /api/v1/counting/inputs…`; increment and total mode; a counting result
  cannot be an input (no loops).
- FR-003 Camera counting: `GET/PUT /api/v1/counting/cameras/{id}`; validation of lines (inside the picture, side
  left/right, known classes, entrance of the organization), sections (two different lines, 1–500 m, limit 0–300 km/h);
  worker state, analysed pictures per second, inference time, CPU of worker and decoder, boxes of the last frame.
- FR-004 Occupancy: never negative (clamp flagged and logged), daily reset, correction with reason
  (`POST /api/v1/counting/areas/{id}/occupancy`), counting log (`GET /api/v1/counting/log`).
- FR-005 Summary: `GET /api/v1/counting/summary?area=|datastream=&date=&days=` — totals, peak, entrances with shares,
  hours with occupancy, days, speed sections.
- FR-006 Widgets `counting_occupancy`, `counting_entrances`, `counting_share`, `vehicle_speeds`; period table value
  `sum`.
- FR-007 Scheduled reports: `GET/POST/PUT/DELETE /api/v1/scheduled-reports…`, `…/{id}/send`, `…/{id}/runs`,
  `…/options`, `…/preview-cron`; `config.write`.
- FR-008 Worker command `ctrl32-telemetry analytics-worker` (also `-input <file>` to evaluate a recording).
- FR-009 Configuration `[analytics]` (ffmpeg, onnxruntime, model_nano/model_tiny with optional SHA-256 pins, threads,
  max_cameras); without it sensors work and cameras show "not installed".

## Success Criteria

- Unit tests: direction mapping, crossing interpolation, jitter, confirmation, truncated boxes, tracker identities,
  speed math (times, direction, minimum track, outliers), occupancy clamp and reset (DST), cron parsing and next runs
  (DST in two time zones), periods.
- e2e `counting` and `scheduled_reports`; headless `counting`.
- Live pipeline on a camera in the development environment: a CC BY 3.0 recording played by the camera simulator,
  counted end to end into datastreams; CPU measured per camera.

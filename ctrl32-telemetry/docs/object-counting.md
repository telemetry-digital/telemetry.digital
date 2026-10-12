# Object counting (people and vehicles)

Spec 029, ADR-0022, since 0.71.0. People and vehicles are counted at the entrances of an *area* — a building, a car
park or a zone. The occupancy of the area is arrivals minus departures. Vehicles can also be timed between two lines
for their speed. Every number is an ordinary datastream, so dashboards, period tables, alarm rules, flows and exports
work with it.

## Model

| Object | Asset | Datastreams |
|---|---|---|
| Area (building, car park, zone) | type `counting_area` on a site | `<group>_in`, `<group>_out` (passages, kind `event`), `<group>_occupancy` (gauge) |
| Entrance | type `entrance`, under the area | `<group>_in`, `<group>_out` |
| Speed section | type `speed_section`, under the area | `vehicle_speed` (km/h, `value_json` = class and direction), `over_limit` (1 per vehicle above the limit) |

Groups: `people`; `vehicles` (car, motorcycle, bus, truck together); `cars`, `trucks`, `buses`, `motorcycles`,
`bicycles`. A detected car counts for `vehicles` and `cars` when the area counts both.

**Occupancy** per group is kept by the server: it never goes below zero (a departure that would make it negative keeps
it at zero, the stored value has quality `out_of_range` and the clamp is logged), it is reset daily at a local time
(e.g. 03:00; also when the server was down at that time; on a day the clocks jump the reset runs at the first moment
after the jump) and it can be corrected by hand with a reason (counting log + audit trail). A capacity alarm is an
ordinary alarm rule on `<group>_occupancy` (the page offers it as *Capacity alarm*).

## Sources

### Cameras: the server's own detector

Off until switched on per camera (*Object counting → Cameras → Configure*, `config.write`, with a reason).

- Draw a **counting line** on the camera picture (two clicks: A and B); the arrow points to the side where the area
  lies — crossing towards it counts *in*, away from it *out* (*⇅ in/out* swaps it). Choose the entrance and the
  classes the line counts (person, bicycle, car, motorcycle, bus, truck).
- A **speed section** is two lines across a road and the real distance between them on the ground (measure it, 1–500
  m), a speed limit (0 = none) and the area whose section asset stores the values.
- Settings: analysed pictures per second (1–15, default 5; speed sections need 10 or more), the detector (nano: fast;
  tiny: more accurate, about 3× the CPU), the detection threshold (0.35), the point of an object that crosses (where it
  touches the ground; or its centre for a camera above the door looking down).

How it works: the server reads the camera's sub-stream once (the same connection serves recording and viewers; relayed
cameras work too) and feeds it to a worker process per camera (`ctrl32-telemetry analytics-worker`). An unmodified
ffmpeg decodes it to small pictures, the YOLOX model finds people and vehicles through ONNX Runtime on the CPU, a
tracker follows them and a crossing counts when a confirmed object (seen 3 times) moves from one clear side of the line
to the other through the drawn segment, once per object and line. Objects coming into view at the picture border are
ignored until they are fully visible. The moment of a crossing is interpolated between pictures.

**No picture is stored or leaves the worker** — only the numbers. The editor shows the live sub-stream in the browser
(privacy masks like in the player; `video.view`), or on request a still picture taken from it, with the lines on top,
the boxes of the last analysed picture and a flash of each line crossed since the editor was opened (from `recent`).

### Sensors: the device protocol

Light barriers, turnstiles, door counters, induction loops and any other system send their counts as telemetry of a
device (MQTT or HTTP, docs/integration). Under *Object counting → Sensors* map a datastream of passages in and/or out to
an entrance and a group. Values are either the number of passages since the last message, or a running total (the
difference counts; a restart of the counter from zero counts the new value).

```
POST /api/v1/d/telemetry        Authorization: Bearer <device token>
{"ts": 1759392000, "v": {"door_in": 3, "door_out": 1}}
```

## Installation of the detector

The installers do it (0.72.0, ADR-0022 addendum): off by default, on request they download an unmodified LGPL build
of ffmpeg, ONNX Runtime and the YOLOX nano and tiny models from their official releases (pinned versions, SHA-256
verified, HTTPS only; none of it is part of our release files), store the licences next to them, write `[analytics]`
and run the self-check. On an existing installation the option adds only these parts and restarts the service.

```sh
curl -fsSL https://portal.telemetry.digital/install.sh | sudo bash -s -- --analytics      # Linux x86-64 or ARM64
.\install.ps1 -Analytics                                                                # Windows x64, elevated
```

| Part | Version | Licence | Linux | Windows |
|---|---|---|---|---|
| ffmpeg (LGPL build) | 8.1.2 (BtbN `autobuild-2026-08-31-13-27`) | LGPL-3.0 | `/opt/ctrl32-telemetry/analytics/` | `%ProgramFiles%\ctrl32-telemetry\analytics\` |
| ONNX Runtime (CPU) | 1.30.0 | MIT | `/opt/ctrl32-telemetry/analytics/` | `%ProgramFiles%\ctrl32-telemetry\analytics\` |
| YOLOX nano, tiny | release 0.1.1rc0 | Apache-2.0 | `/var/lib/ctrl32-telemetry/models/` | `%ProgramData%\ctrl32-telemetry\models\` |

Download size about 90 MB on Linux and 180 MB on Windows; on disk about 200 MB. Distribution packages of ffmpeg
(Debian, Ubuntu) are GPL builds; `install.sh --analytics --ffmpeg apt` uses one anyway on your explicit choice and the
self-check marks it as a GPL build.

**Self-check:** `ctrl32-telemetry analytics-worker -selfcheck [-config /etc/ctrl32-telemetry/config.toml]` prints
ffmpeg (version, LGPL or GPL build), ONNX Runtime (version), each model (SHA-256 against the pin, one test inference)
and whether cameras can count (exit code 0) or not (1). The same check is shown on *Object counting → Cameras* and
*System → Server* (*Check again* repeats it; the server caches it for 5 minutes), with advice for a missing part.

**From the user interface (0.75.0):** a server administrator (`system.admin`) presses *Install the detector* on the
Detector installation card (*Object counting → Cameras*, *System → Server*), gives a reason, the password and, with
two-factor sign-in, the authenticator code. The server asks the privileged agent (`POST /analytics/install` on its
socket) to install exactly the parts above: the pins are in one place, `internal/analytics/pins.go`, and the installers
carry the same values (`TestInstallerPinsMatch`). The agent downloads over HTTPS from the official hosts only, keeps a
file only when its SHA-256 matches the pin, refuses an ffmpeg configured with `--enable-gpl`/`--enable-nonfree`,
unpacks into a staging directory next to the target and swaps the parts in by renaming them only when everything was
downloaded and checked (`internal/analytics/setup`; a failed swap is rolled back), then writes the paths into
`[analytics]` (threads, max_cameras and everything else stay) and records the installation in `analytics.json` next to
`config.toml`. The server applies the new paths without a restart (camera workers pause meanwhile) and runs the
self-check again; the card polls the progress (download x of y with megabytes, unpack, verify). A second run downloads
only what is missing or broken; *Repair or update* with *force* downloads every part again; *Remove* deletes the parts
and their paths. Request and result are audited (`analytics.install`, `analytics.remove`); API tokens and AI assistants
are refused (MCP exclusion). Linux amd64/arm64 and Windows amd64; on Linux the agent installs `xz-utils` when `xz` is
missing (the ffmpeg build is a `.tar.xz`). API: `GET|POST|DELETE /api/v1/counting/install`. Threat model:
`docs/threat-model.md`.

**Then switch counting on per camera:** *Object counting → Areas and entrances* (an area with its entrances) →
*Cameras* → *Configure*: draw a line, choose the entrance and the classes, enable, save with a reason.

By hand, the server needs three things, named in `config.toml` (without them sensors work and cameras show *not
installed*):

```toml
[analytics]
ffmpeg = "/opt/ctrl32-telemetry/analytics/ffmpeg-n8.1.2-50-g1a748fe2cd-lgpl/bin/ffmpeg"   # an unmodified LGPL build
onnxruntime = "/opt/ctrl32-telemetry/analytics/onnxruntime-1.30.0/lib/libonnxruntime.so"  # 1.17 or newer (MIT), CPU
model_nano = "/var/lib/ctrl32-telemetry/models/yolox_nano.onnx"   # YOLOX release 0.1.1rc0 (Apache-2.0)
model_tiny = "/var/lib/ctrl32-telemetry/models/yolox_tiny.onnx"
nano_sha256 = "c789161ed43c8269fcd4e67c67eeeb4e80c622da2eb296a20bc6007bd18a0b7d"
tiny_sha256 = "427cc366d34e27ff7a03e2899b5e3671425c262ea2291f88bb942bc1cc70b0f7"
threads = 1          # inference threads per camera
max_cameras = 4      # cameras analysed at the same time
```

- ONNX Runtime: `onnxruntime-linux-x64-<version>.tgz` / `onnxruntime-win-x64-<version>.zip` from the project's
  releases (64-bit Linux, Windows, macOS; not on 32-bit ARM).
- Models: `yolox_nano.onnx` and `yolox_tiny.onnx` from the YOLOX repository (Megvii), release 0.1.1rc0. With the SHA-256
  pins the worker refuses any other file.
- ffmpeg must be an LGPL build (no `--enable-gpl`); it runs as a separate program and is not part of the product.
  Debian's and Ubuntu's `ffmpeg` packages are GPL builds; the "lgpl" builds of BtbN/FFmpeg-Builds are LGPL.

## Accuracy and CPU

Measured on the development PC (Intel Xeon E3-1240 v3, 4 cores / 8 threads, 3.4 GHz, 2013; one inference thread per
camera), sub-stream 640 × 352 H.264 from the camera simulator playing a recording:

| Detector | Pictures/s | Inference | CPU of one core (detector + decoder) |
|---|---|---|---|
| nano | 5 | 65 ms | 32 % (31 + 1) |
| nano | 10 | 43 ms | 44 % (43 + 1) |
| tiny | 5 | 162 ms | 81 % (77 + 4) |
| tiny | 10 | 141 ms | 99 % — reaches only 6.9 pictures/s |

Evaluation on a recorded street scene (69 s, CC BY 3.0, a static camera at about 1.5 m, people walking towards and away
from it, a horizontal counting line): 7 real crossings. The nano model counted 6 at 5, 8 and 10 analysed pictures per
second (3 of 3 arrivals, 3 of 4 departures — two people walking side by side were seen as one), the tiny model at 5
pictures per second all 7; no false count with either. Accuracy depends on the view (a camera above the entrance looking down at an angle is best; a strictly vertical
view is harder for models trained on side views), light, crowding and the frame rate. Check the counts against a
manual count when you install a camera, and correct the occupancy when needed.

**Speed** = distance between the two lines ÷ time between crossing them. Without interpolation the time is known to
±1 picture: at 10 pictures per second over 10 m at 50 km/h that is 0.1 s of 0.72 s ≈ ±14 %; the crossing moments are
interpolated between pictures, so steady traffic is usually within a few per cent, but perspective, the ground point
of the box and braking add to the error. A higher frame rate and a longer section help. The speed is an indicative
measurement, **not certified speed enforcement**; no number plates and no persons are recorded.

## Privacy (GDPR)

Counts and speeds are aggregate numbers without personal data: no image, track, face or number plate is stored or sent
anywhere. A camera used for counting still films people — its recording (which counting does not need), masks and
viewing follow the camera's own rules (docs/video.md). Inform people about the camera as usual.

## Outputs

- *Object counting*: per area occupancy now (with capacity), arrivals, departures and the peak of the day, by hour with
  the occupancy, entrances with their shares, the days before, speed sections (count, average, maximum, over the limit,
  last vehicles, histogram), resets and corrections; sums per site when a site has several areas; export (period table
  of the area: Excel, CSV, PDF).
- *Live check of the counting* (today, `video.view`, switched on per browser): the live sub-stream of every camera that
  counts for the area with its lines; each crossing flashes its line (green in, red out) and is listed with its time,
  line, direction and class, with counts per line since the page was opened. The crossings come from the camera's
  counting status (`recent`: the last 30, kept in memory), so a person can check the counts by eye.
- Widgets: *Occupancy*, *In and out per entrance*, *Share per entrance*, *Vehicle speeds* (bind a datastream of the area
  or the section); the occupancy also works with the value, gauge and chart widgets.
- Period tables: the value `sum` adds passages per hour, day, week, month or year.
- Further aggregation with automation rules (derived datastream, expression language) or flows (*Store value*).

## Scheduled reports by e-mail

*Reports → Scheduled reports* (`config.write`; before 0.75.0 under *Settings*): content (counting summary of an area, a period table of a dashboard,
or a report template — observations of a datastream or alarms), period (previous day, week Monday–Sunday, month in the
organization's time zone), a cron schedule (`minute hour day month weekday`, presets daily 07:00, Monday 07:00, 1st of
the month 07:00; a time skipped by the change to summer time runs at the first moment after it, a repeated time once),
recipients (e-mail notification channels and addresses), PDF and/or Excel attachments (CSV for templates), enable,
*Send now*, the delivery log. Files are made like the exports of the user interface, as the report's author with read
and export permissions; every change and every sending is audited. Needs `[smtp]` in `config.toml`; for development
`go run ./tools/smtpsink` collects the e-mails as files.

## API

`/api/v1/counting/…` (areas, entrances, inputs, cameras, summary, log, occupancy correction) and
`/api/v1/scheduled-reports/…` — see `/api/docs`.

# Development on a PC

Everything needed to build, run, demo and test ctrl32 telemetry on your own computer (Linux, macOS, or Windows with
Git Bash).

## Tools

| Tool | Why | Windows | Linux |
|---|---|---|---|
| Go (the version in `go.mod`) | build and unit tests | go.dev/dl | go.dev/dl |
| Node.js 22+ | e2e test helpers | nodejs.org | distribution or nodejs.org |
| Python 3 | demo scripts | python.org | usually installed |
| Docker Desktop / Docker Engine | PostgreSQL 18 for development | docker.com | docker.com |
| Git Bash | runs the `.sh` scripts | comes with Git for Windows | — |
| Chrome or Chromium | headless browser tests (optional) | installed | installed |

Without Docker, any PostgreSQL 16+ works: set `PG_ADMIN_URL` to a superuser URL (default
`postgres://ctrl32:ctrl32-dev-only@127.0.0.1:5433/postgres`) and run with `NO_DOCKER=1`.

## Run it

From `ctrl32-telemetry/`:

```sh
bash deploy/dev/dev.sh up        # PostgreSQL 18 in Docker, fresh database, demo seed, server on http://127.0.0.1:8080
bash deploy/dev/dev.sh camsim    # simulated cameras (RTSP :8554, HTTP/ONVIF :8555)
bash deploy/dev/dev.sh demo      # 40 cameras, 2 video walls, production dashboard and SCADA screen, floor plan,
                                 # plus the documentation demo (deploy/dev/demo/docs.py, below)
```

Without a licence a server runs up to 4 cameras. The development build of `dev.sh` trusts the test licence key of
`tests/keys` (release builds never do), and `dev.sh demo` installs a development licence (licensee *ctrl32
development*) so that all 40 cameras run; remove it under *System → License* to see the limits of a server without a
licence.

The documentation demo (`python3 deploy/dev/demo/docs.py setup`, run by `dev.sh demo`; it can be run again and skips
what exists, state in `.dev/docs.json`) fills the screens the other scripts leave empty: a cold chain of medicine fridges
with 30 days of history, alarm rules, alarms and incidents, five electricity meters with 13 months of history for the
Energy page, dashboards with period tables, the *overview* and *showcase* starter dashboards, automation rules and
flows, smart-home MQTT accounts with simulated zigbee2mqtt, Tasmota and Shelly devices, a display, firmware with a
campaign, users with a custom role, an API token, a report template with stored PDF reports and two PWA apps. The
server creates the month partitions of the history itself (0.66.0+; for an older server set `CTRL32_DB_ADMIN_URL`, or
`CTRL32_DB_NAME` for a database in the `ctrl32-pg-dev` container); PDF reports need `typst` on PATH or next to the
server binary in `.dev/`. Against another server set `CTRL32_URL`, `CTRL32_MQTT` (its MQTT listener, for the
smart-home simulators) and `CTRL32_DEV_DIR` (state files). Screenshots of every screen for the documentation:
`BASE=http://127.0.0.1:8080 OUT=<folder> node tests/headless/docshots.js`.

Sign in as **admin / admin1234**. After a code change: `bash deploy/dev/dev.sh restart`; to stop everything:
`bash deploy/dev/dev.sh stop`. Local data, logs, recordings and binaries live in `ctrl32-telemetry/.dev/`
(git-ignored); `dev.sh up` recreates the database `ctrl32_dev`.

Real cameras on your network can be added next to the simulated ones (*Cameras → Camera management → Add camera*,
camera make *TP-Link Tapo* and others — see `docs/video.md`, "Camera makes").

## Tests

```sh
bash tests/run-all.sh                    # unit, C, e2e and headless suites (needs ports 8080, 1883, 1884 free)
bash tests/run-all.sh e2e                # one phase
BASE=http://127.0.0.1:8080 bash tests/e2e/video.sh   # one suite against the running dev server
```

Stop the dev server first for `run-all.sh` (it starts its own on port 8080). The `ops` suite needs NTP (UDP 123) to
the Internet; skip it where that is blocked: `SKIP="ops" bash tests/run-all.sh`.

## Object counting and scheduled reports (0.71.0)

- The demo of object counting: `python deploy/dev/demo/docs.py counting` (part of `docs.py setup`) — the main building
  of the Nitra plant with three entrances and 30 days of counts, a car park with a speed section, a dashboard and three
  scheduled reports. `CTRL32_CAMSIM=rtsp://127.0.0.1:8554` names the camera simulator, `CTRL32_DEMO_DETECTOR=1`
  switches the detector on for the entrance camera.
- The detector needs `[analytics]` in `.dev/config.toml` (ffmpeg LGPL build, ONNX Runtime, the YOLOX models; see
  `docs/object-counting.md`). To see it count, play a recording of people walking in the camera simulator:
  extract pictures (`ffmpeg -i clip.webm -vf fps=10,scale=640:360 frames/f%04d.jpg`) and start
  `go run ./tools/camsim -clip frames`; cameras labelled `clip…` (e.g. `rtsp://127.0.0.1:8554/clip/h264/640x360`) play it.
- Evaluate a recording without the server: `ctrl32-telemetry analytics-worker -config @cfg.json -input clip.webm`
  (crossings and statistics as JSON lines; `CTRL32_ANALYTICS_TRACE=1` prints the tracks).
- E-mails of scheduled reports: `go run ./tools/smtpsink -listen 127.0.0.1:2525 -dir .dev/mail` and `[smtp] host =
  "127.0.0.1", port = 2525, from = "reports@example.com"`; `tests/e2e/scheduled_reports.sh` checks them with
  `MAIL_DIR=.dev/mail`.

## Before a commit

`gofmt -l .` empty, `go vet ./...`, the unit tests of the changed packages, `go run ./tools/apigen` after any change of
`internal/httpapi/openapi.go` (the generated clients in `clients/` are checked by a test), and the e2e suite of the
feature. CI (`.github/workflows/ctrl32-telemetry.yml`) runs `tests/run-all.sh` on every pull request.

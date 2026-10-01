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

The documentation demo (`python3 deploy/dev/demo/docs.py setup`, run by `dev.sh demo`; it can be run again and skips
what exists, state in `.dev/docs.json`) fills the screens the other scripts leave empty: a cold chain of medicine fridges
with 30 days of history, alarm rules, alarms and incidents, five electricity meters with 13 months of history for the
Energy page, dashboards with period tables, the *overview* and *showcase* starter dashboards, automation rules and
flows, smart-home MQTT accounts with simulated zigbee2mqtt, Tasmota and Shelly devices, a display, firmware with a
campaign, users with a custom role, an API token, a report template with stored PDF reports and two PWA apps. It needs
Docker's `ctrl32-pg-dev` container (or `CTRL32_DB_ADMIN_URL`) to add month partitions for the history; PDF reports need
`typst` on PATH or next to the server binary in `.dev/`. Screenshots of every screen for the documentation:
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

## Before a commit

`gofmt -l .` empty, `go vet ./...`, the unit tests of the changed packages, `go run ./tools/apigen` after any change of
`internal/httpapi/openapi.go` (the generated clients in `clients/` are checked by a test), and the e2e suite of the
feature. CI (`.github/workflows/ctrl32-telemetry.yml`) runs `tests/run-all.sh` on every pull request.

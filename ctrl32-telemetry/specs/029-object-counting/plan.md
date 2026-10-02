# Implementation Plan: Object counting and scheduled reports (spec 029)

## Architecture

```
camera (RTSP, relay, MJPEG) ──► video.Source (one connection) ──► internal subscription (sub-stream)
                                                                    │ framed samples on stdin (time + Annex B / JPEG)
                                                                    ▼
                                             analytics-worker process (same binary, one per camera)
                                               ffmpeg (unmodified, LGPL build) → 416×416 BGR pictures
                                               ONNX Runtime (MIT, loaded at run time, no cgo) + YOLOX (Apache-2.0)
                                               tracker + line crossing + speed (Go, ours)
                                                                    │ JSON lines on stdout (cross, speed, stats)
                                                                    ▼
sensors (device protocol) ──► ingest ──► counting.Engine (sink) ──► Record(entrance, groups, dir, n, at)
                                                                    │ occupancy (row lock, clamp), counting log
                                                                    ▼
                                         ingest.StoreCounted → core.observation → alarm rules, flows, live, period tables
```

- `internal/analytics`: detector pre/post-processing (`detect.go`), tracker (`track.go`), lines and speed
  (`count.go`), the worker (`worker.go`), CPU times; `internal/analytics/ort`: ONNX Runtime C API through the shared
  library (`syscall` on Windows, `purego` on Linux/macOS; a stub on 32-bit ARM).
- `internal/counting`: groups and keys, occupancy rules, DST-safe reset (`groups.go`), the engine (`engine.go`:
  Record, Correct, resets, sensors as an ingest sink, speeds), camera supervision (`cameras.go`).
- `internal/schedule`: cron parser, next run in a time zone, reporting periods.
- `internal/httpapi/counting.go`, `scheduled_reports.go`, `counting_widgets.go`; pages `counting.html` (+
  `counting.js`, `counting-editor.js`), `scheduled-reports.html`, `widgets-counting.js`.
- Migrations 0047 (counting) and 0048 (scheduled reports); `tools/smtpsink` for development and tests; camsim plays a
  recorded clip (`-clip`) for the detector.

## Decisions

See ADR-0021 (worker process, ffmpeg as an external unmodified program, ONNX Runtime and YOLOX, no images).

# Tasks: Object counting and scheduled reports (spec 029)

- [x] T001 Detector pre/post-processing (letterbox, YOLOX decoding, NMS) with unit tests
- [x] T002 ONNX Runtime bindings without cgo (Windows syscall, Linux/macOS purego, stub elsewhere); test against the model
- [x] T003 Tracker (constant velocity, overlap and per-axis distance gates, size ratio gate, class votes)
- [x] T004 Line crossing (sides, dead band, segment check, once per track and line, truncated boxes, interpolated time)
- [x] T005 Speed between two lines (interpolated times, direction, plausible range, vehicles by default)
- [x] T006 Worker command with ffmpeg, statistics (fps, inference, CPU of worker and decoder), bench mode on a file
- [x] T007 Migrations 0051, 0052; configuration `[analytics]`
- [x] T008 Engine: Record with occupancy (clamp, log), corrections, daily reset (DST), sensors as an ingest sink, speeds
- [x] T009 Camera supervision: feed from the server's subscription, restart on stream changes, limit of cameras
- [x] T010 API: areas, entrances, inputs, cameras, summary, log, correction; OpenAPI, MCP area, clients
- [x] T011 Period table value `sum`; widgets (occupancy, entrances, share, vehicle speeds)
- [x] T012 Page Object counting (overview, areas, cameras with the line editor, sensors)
- [x] T013 Scheduled reports: cron, periods, content (counting, period table, template), attachments, e-mail, log
- [x] T014 Settings → Scheduled reports page
- [x] T015 e2e `counting`, `scheduled_reports`; headless `counting`; unit tests
- [x] T016 Demo (`docs.py counting`), documentation, screenshots, translations in 19 languages
- [ ] T017 Later: perspective calibration (homography) for continuous speed along a track; GPU execution provider;
      the installer downloading ffmpeg, ONNX Runtime and the models; an occupancy history for imported data

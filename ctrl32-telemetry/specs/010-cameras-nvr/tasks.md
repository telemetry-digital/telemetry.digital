---
description: "Tasks for cameras, recording and video walls"
---

# Tasks: Cameras — recording and video walls

**Input**: `specs/010-cameras-nvr/` · **Tests**: required · **Paths**: relative to `ctrl32-telemetry/`

## Phase 1: Video core
- [x] T001 I_PCM H.264 test encoder in internal/video/testsrc; camera simulator internal/video/camsim + tools/camsim
- [x] T002 Source with RTSP (H.264/H.265/MJPEG) and HTTP MJPEG, reconnect, fan-out, GOP cache in internal/video
- [x] T003 fMP4 muxer; unit tests source_test.go
- [x] T004 Recorder, playback reader, manager with retention and index repair; config [video]

## Phase 2: Server
- [x] T005 Migration 0027 (camera, video_segment, video_wall, permissions)
- [x] T006 internal/httpapi/video.go: cameras, test, live/playback WebSocket, recordings, export, storage, walls; OpenAPI

## Phase 3: UI
- [x] T007 videoplayer.js (MSE + MJPEG canvas), video.html, camera.html (timeline day + hour, export), wall.html (multi-monitor)
- [x] T008 Menu, sk/cs

## Phase 4: Polish
- [x] T009 e2e tests/e2e/video.sh; docs/video.md; URS P-01…P-06; JOURNAL, README, STAV; run-all

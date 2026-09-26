# Tasks: Cameras — widget, player controls, timeshift, remote sites, PWA

**Input**: `specs/011-cameras-remote/` · **Tests**: required · **Paths**: relative to `ctrl32-telemetry/`

- [x] T001 Camera widget (widget_catalog_more.go, widgets-more.js, parameter kind `camera`, editor picker)
- [x] T002 Player: open()/pause()/zoom, leak guard for detached players (videoplayer.js); camera.html control bar
- [x] T003 Timeshift: offset-based reading of growing files, follow mode (internal/video/playback.go)
- [x] T004 Push listener with TLS/digest, secrets, blocking (internal/video/push.go); migration 0028; config
- [x] T005 camera-relay (internal/video/relay.go, cmd); API source/push/push-secret; UI connection mode + secret dialog
- [x] T006 PWA: ManagedMediaSource, manifest shortcuts; mobile check
- [x] T007 Tests: push_test.go (TLS, wrong secret, blocking, config validation), e2e video.sh (61)
- [x] T008 Load test 128 cameras; docs/video.md, URS P-07…P-10, JOURNAL, STAV; run-all

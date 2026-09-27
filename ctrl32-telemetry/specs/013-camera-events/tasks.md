# Tasks: Camera events, recording on events, PTZ, discovery

- [x] T001 Migration 0030 (camera ONVIF fields, record mode `events`, core.camera_event, permission video.ptz)
- [x] T002 internal/video/onvif: client, events, PTZ, discovery; unit tests against the simulator
- [x] T003 internal/video/camsim/onvif.go: ONVIF simulator (auth, events, PTZ moving the picture, discovery)
- [x] T004 internal/video/events.go and recorder pre-roll/hold/extend; events closed on reload and restart
- [x] T005 internal/httpapi/video_events.go: discover, profiles, events, marks, PTZ; OpenAPI
- [x] T006 Flows: camera_event trigger, camera_mark action; unit test
- [x] T007 UI: camera form (ONVIF, find, fill in, events mode), camera page (PTZ, marks, events), wall highlight; sk/cs
- [x] T008 e2e tests/e2e/video.sh; docs/video.md; URS P-11…P-14; JOURNAL, STAV; run-all

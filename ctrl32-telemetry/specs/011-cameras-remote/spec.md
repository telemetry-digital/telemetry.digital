# Feature Specification: Cameras — widget, player controls, timeshift, remote sites, PWA

**Feature Branch**: `assistant/jolly-sagan-nzolwc`

**Created**: 2026-09-26

**Status**: Implemented (0.46.0)

**Input**: User description: "Kamery dať aj ako widgety, aby sa dali kdekoľvek pridávať. Potom prehrávanie videa
nezávislé od nahrávania — keď prehrávam, aby mi to nahrávalo ďalej. Pekné dizajnové tlačidlá dopredu, dozadu,
priblíženie. 128 kamier a vymyslieť nejakú vzdialenú správu, alebo aby kamery z diaľky nahrávali na server, a PWA,
aby sa dali kamery zobraziť a aj prehrávať, ale všetko zabezpečené ako vždy, SAFETY FIRST!"

**Related**: spec 010; URS P-07…P-10; constitution I (audit), II, VII.

## Clarifications (defaults, the user asked to continue)
- Remote sites → a relay that pushes over RTSPS (outgoing connection, no port or VPN at the site); WireGuard stays
  the alternative. Secrets per camera, shown once, sealed at rest.
- Timeshift → playback without an end follows the file being written; pause on live continues from the recording.
- Zoom → digital (the picture is enlarged in the browser); PTZ control of cameras is a later step.

## User Scenarios
1. **Camera on a dashboard** (P1): an operator adds the camera "Gate" next to the gate's sensors.
2. **Controls** (P1): back 10 s, pause, zoom into a number plate, play on at 4×, back to LIVE — recording never stops.
3. **Remote site** (P1): a branch without a VPN sends its cameras through a Raspberry Pi relay; the headquarters
   records and watches them; a stolen relay is cut off by issuing a new secret.
4. **Phone** (P2): a manager opens the app from the home screen, watches a wall and plays back the night.
5. **Scale** (P1): 128 cameras record on one server.

## Requirements
- FR-001 Widget type `camera` (parameter kind `camera`), never rendered on public dashboards.
- FR-002 Player controls, digital zoom, keyboard; one player switches between live and recordings.
- FR-003 Playback follows the recording (FollowLimit 20 s without growth ends it); recording is independent.
- FR-004 Push listener (`video.push_listen`), TLS or digest, per-camera secret, rotation, blocking, audit.
- FR-005 `camera-relay` command with relay.toml validation, reconnect, no secrets in logs.
- FR-006 ManagedMediaSource for iPhone; PWA shortcuts; nothing cached.
- FR-007 Load test 128 cameras.

## Success Criteria
- SC-001 e2e `video` passes (timeshift, widget, relay, wrong secret, rotation).
- SC-002 unit tests: relay over TLS with a private CA, wrong secret refused and address blocked.
- SC-003 128 cameras recording at ≤ 2 cores on a 4-core VM.

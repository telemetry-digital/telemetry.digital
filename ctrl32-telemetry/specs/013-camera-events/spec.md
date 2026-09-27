# Feature Specification: Camera events, recording on events, PTZ, discovery

**Feature Branch**: `assistant/jolly-sagan-nzolwc`

**Created**: 2026-09-26

**Status**: Implemented (0.48.0)

**Input**: User agreed ("Áno") to close the gaps against ZoneMinder: motion detection from camera metadata (ONVIF
events, no decoding), recording on events with marks on the timeline, PTZ and ONVIF discovery, camera events as input
to flows and alarms.

**Related**: specs 010, 011; URS P-11…P-14; constitution I (audit), II, VII.

## Clarifications (defaults)
- Motion comes from the camera's own analytics (ONVIF events); the server never decodes video, so 128 cameras stay cheap.
- Event recording keeps a pre-roll in memory from the last key frame; no second recorder.
- PTZ needs its own permission `video.ptz` (operator, engineer, org_admin); moves are audited once per 5 minutes per
  user and camera, presets always.
- Discovery only answers IP addresses (no host names) and at most 16 extra unicast targets.

## User Scenarios
1. (P1) A gate camera reports motion; the camera records 5 s before and 10 s after, the timeline shows the event.
2. (P1) An operator turns a PTZ camera by holding the arrows and jumps to a preset.
3. (P1) An engineer finds cameras in the network and fills stream addresses from the ONVIF profiles.
4. (P2) A flow marks the camera when a door opens; the camera records around the mark.

## Requirements
- FR-001 ONVIF client: SOAP 1.2, WS-Security digest, clock offset, capabilities, profiles, stream URIs, PullPoint
  subscription with renew and unsubscribe; events classified motion/tamper/line/intrusion/input/other.
- FR-002 Events stored (`core.camera_event`), merged within 5 s, closed on disconnect, reload and restart; event
  times never in the future.
- FR-003 Record mode `events` with pre-roll 0–60 s, post-roll 1–600 s, trigger kinds.
- FR-004 Marks by users (`video.view`, audited) and flows.
- FR-005 PTZ continuous move, stop, presets; permission `video.ptz`; audit.
- FR-006 WS-Discovery multicast and unicast; profiles endpoint for the camera form.
- FR-007 Flows: trigger `camera_event`, action `camera_mark`.
- FR-008 Everything adjustable in the UI: server-wide video settings (disk limit, file length, event merging,
  discovery wait, PTZ audit interval, linger, timeshift end, wall sub-stream threshold, relay blocking) with bounds,
  system.admin only, audited; stream per wall screen; PTZ speed per viewer.

## Success Criteria
- SC-001 e2e `video` (89 checks) and unit tests of the ONVIF client pass; a restart leaves no event open.

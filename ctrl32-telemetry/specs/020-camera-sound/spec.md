# Feature Specification: Camera sound

**Feature Branch**: `assistant/jolly-sagan-nzolwc`

**Created**: 2026-09-27

**Status**: Implemented (0.54.0)

**Input**: From the list of what a professional system lacks: sound; the user: "áno, poupravuj všetko".

**Related**: specs 010, 011 (relay), 015 (evidence); URS P-25; constitution I (security, privacy), II.

## Clarifications (defaults)
- Off by default per camera; recording sound is more strictly regulated than pictures (safety first).
- No transcoding: AAC and Opus pass through (both play in browsers via Media Source Extensions); G.711/G.726 are
  reported with the advice to switch the camera to AAC.
- In the browser the sound is a separate stream and element, so a gap in the sound never stalls the picture.

## Requirements
- FR-001 Camera setting `audio` (default off); migration 0037; permission `video.audio` (org_admin).
- FR-002 The main stream takes AAC (RFC 3640) or Opus; the status names the sound or why there is none.
- FR-003 Recordings hold the sound as the second MP4 track on the picture's time line; playback reads it back.
- FR-004 Live and playback send the sound only with `audio=1`, `video.audio` and speed 1: a `{type:"audio"}` message
  and binary messages prefixed with `A` (an audio-only fMP4 stream starting at the first picture); audited as
  `video.listen`.
- FR-005 Exports and evidence packages include the sound only for people with `video.audio`.
- FR-006 The relay forwards the sound only with `audio = true`.
- FR-007 Player: a hidden `<audio>` element fed by its own MediaSource, synchronised to the picture every second,
  muted until the user switches the sound on; camera page speaker button (S); no sound on walls, widgets, displays.

## Success Criteria
- SC-001 Unit test: two-track recording, sound before the first picture dropped, separate picture/sound streams.
- SC-002 e2e `video`: status, no sound unless asked, live and recorded sound, no sound at 4×, export with and without
  the permission, G.711 explained, listening audited.
- SC-003 Browser: the sound plays next to an MJPEG picture (headless Chromium without H.264/AAC codecs).

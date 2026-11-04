# Feature Specification: Cameras — recording and video walls

**Feature Branch**: `main`

**Created**: 2026-09-26

**Status**: Implemented (0.45.0)

**Input**: User description: "ďalšia vec by mohli byť nahrávanie, zaznamenávanie obrazu a na dlaždice, aby sa dali
kamery zobrazovať na obrazovke, napríklad na 2 monitoroch, na 1 20, na druhom 20" (record camera video and show
cameras as tiles, e.g. 20 on each of two monitors).

**Related**: URS P-01…P-06; constitution I (audit), II (single binary, no external services), VII.

## Clarifications

### Session 2026-09-26
Defaults chosen (the user asked to continue without questions):
- Q: Transcoding? → A: None. The server repackages what the camera sends (RTSP → fragmented MP4) and never decodes;
  tiles use the camera's sub-stream. No ffmpeg, the single binary stays.
- Q: Codecs? → A: H.264 and H.265 over RTSP (browsers play H.265 only where the platform supports it), MJPEG over RTSP
  or HTTP (many cheap cameras; also what the headless test browser can show).
- Q: Recording modes? → A: continuous or off in this version; motion or event recording later.
- Q: Who may watch recordings? → A: a separate permission (`video.playback`); every playback and export is audited
  (recordings are personal data).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Video wall on two monitors (Priority: P1)
A guard opens the wall "Control room" (two screens of 5 × 4) with one click on both monitors; each shows 20 live
cameras; a click enlarges one camera to the full monitor.

**Independent Test**: a wall with two 5 × 4 screens of simulated cameras shows 20 live tiles per screen.

### User Story 2 - Continuous recording (Priority: P1)
Every camera set to record writes one-minute files; the operator keeps 7 days per camera and at most 50 GB overall.

**Independent Test**: after a minute the camera's recordings list a span; the files exist; retention deletes old ones.

### User Story 3 - Find and export an incident (Priority: P1)
An engineer opens the camera, switches to Recordings, picks the day and the hour, plays from the time of an incident
at 4× speed and downloads the relevant five minutes as MP4 for the police. The audit shows who watched and exported.

### User Story 4 - Adding a camera (Priority: P2)
The administrator pastes `rtsp://admin:pw@192.168.1.64/Streaming/Channels/101`, tests the connection (codec and size
are shown), adds the sub-stream `…/102` for tiles and saves; the password is stored encrypted.

### Edge Cases
- Camera offline: the tile shows "Camera offline", the server reconnects with back-off (1 s … 30 s).
- Slow viewer: frames are skipped to the next key frame; other viewers and the recorder are not slowed.
- Resolution change on the camera: live players re-initialize, recording starts a new file, export ends early.
- Server crash while writing: the index is repaired from the files on the next start.
- Browser without H.265: the tile says so; use an H.264 sub-stream.

## Requirements *(mandatory)*
- **FR-001** RTSP (H.264/H.265/MJPEG) and HTTP MJPEG; main and sub-stream; credentials separate, password encrypted.
- **FR-002** Continuous recording into fragmented MP4 files of about 60 s starting with key frames, indexed in the database.
- **FR-003** Retention per camera (days) and a disk limit for all recordings.
- **FR-004** Live and recorded video over WebSocket (fMP4 for MSE, JPEG for MJPEG); playback speed 0.25–16.
- **FR-005** Export of at most 24 hours as MP4 (one hour until 0.86.10).
- **FR-006** Video walls: 1–8 screens of 1×1…8×8 tiles, opened on all monitors at once.
- **FR-007** Permissions video.view / video.playback / video.manage; audit of changes, playback and export.

## Success Criteria
- **SC-001** 40 simulated cameras on a two-screen wall play live in the browser (screenshot).
- **SC-002** e2e `video` passes: stream test, live, recording, playback, export, audit, walls, validation.

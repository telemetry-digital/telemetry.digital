# Feature Specification: Privacy masks (GDPR)

**Feature Branch**: `assistant/jolly-sagan-nzolwc`

**Created**: 2026-09-27

**Status**: Implemented (0.50.0)

**Input**: From the list of what a professional system lacks: privacy masks; the user: "áno, poupravuj všetko".

**Related**: specs 010, 014, 015; URS P-21; constitution I, II.

## Clarifications (defaults)
- The server never decodes video; masks cannot be burnt into the recording. They travel with every stream and every
  player covers them; snapshots are masked on the server; exports of a masked camera need `video.unmask`.
- True exclusion from the recording is configured in the camera (documented).

## Requirements
- FR-001 Masks per camera: up to 16 polygons of 3–16 points in 0..1 picture coordinates; validated; audited.
- FR-002 A `masks` message before the first frame of live and recorded video; the player covers the picture area
  (object-fit contain, zoom).
- FR-003 JPEG snapshots masked on the server.
- FR-004 Unmasked view (`?unmask=1`) and exports of masked cameras only with `video.unmask`; audited.
- FR-005 Editor: rectangles dragged over the live picture in the camera form.

## Success Criteria
- SC-001 e2e `masks` passes; unit tests of JPEG masking and validation.

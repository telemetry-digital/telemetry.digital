# Feature Specification: Motion detection on the server

**Feature Branch**: `main`

**Created**: 2026-09-27

**Status**: Implemented (0.53.0)

**Input**: From the list of what a professional system lacks: motion for cameras without ONVIF events; the user:
"áno, poupravuj všetko".

**Related**: specs 010, 013 (events, event recording), 016 (editor), 017; URS P-24; constitution I, II.

## Clarifications (defaults)
- The server keeps its rule of never decoding H.264/H.265: detection uses MJPEG frames or JPEG snapshots.
- Grid-based background subtraction with frame difference (not a neural network): cheap, explainable, tunable with one
  sensitivity value and zones; object classification is out of scope.

## Requirements
- FR-001 Per camera: `motion_detect`, `motion_sensitivity` (1–100), `motion_zones` (up to 16 polygons 0..1),
  `snapshot_url` (http/https, no credentials, not for relayed cameras); validated and audited.
- FR-002 MJPEG frames (sub-stream first, main if the sub is not MJPEG) at most 2.5/s; otherwise the snapshot address
  once a second with basic or digest authentication; otherwise the status explains what is missing.
- FR-003 Motion needs three changed pictures in a row, ends 4 s after the last; stopped objects become background;
  global changes (> 70 % of cells) reset the background.
- FR-004 Events kind `motion`, source `server`: event recording, walls, plans, displays, flows as for ONVIF events.
- FR-005 The detector is not counted as a viewer; the camera status shows the detection state.
- FR-006 Editor: zones drawn next to the privacy masks (switch between the two).

## Success Criteria
- SC-001 Unit tests: still, moving, single odd picture, zones, lighting change, end of motion, digest snapshot.
- SC-002 e2e `video`: validation, MJPEG and snapshot detection, zones, event recording, H.264 without snapshot.

# Feature Specification: Cameras on a floor plan

**Feature Branch**: `main`

**Created**: 2026-09-27

**Status**: Implemented (0.51.0)

**Input**: From the list of what a professional system lacks: cameras on a floor plan or map; the user: "áno, poupravuj
všetko".

**Related**: specs 001 (process pictures), 010, 014, 016; URS P-22; constitution I, II.

## Clarifications (defaults)
- A floor plan is a process picture (kind `mimic`) with an image from the organization library (imported SVG) and
  one `camera_pin` widget per camera; no new screen kind and no new storage.
- A pin shows the camera, its view cone (direction, field of view, reach), its name and its state: online, offline,
  event (motion or an ONVIF event) in progress.
- Click opens the live picture in a window; the camera page is one more click away.
- A camera the viewer may not see (camera groups) is not drawn at all.

## Requirements
- FR-001 Widget `camera_pin` with parameters camera, direction (0 = up, clockwise), fov (10–180°), reach (0–100 %),
  show_name, title; frameless on process pictures.
- FR-002 State from `GET /api/v1/video/cameras/{id}` (signed in, cached 10 s) or from the display state (kiosk:
  names and active events of every camera on its content).
- FR-003 An event in progress colours the pin and its cone and pulses (no animation with reduced motion).
- FR-004 Click: a window with the live picture (masks respected), "Open camera", Esc/close; on a kiosk it closes by
  itself after 60 s. Public shared dashboards show pins without state and without a click.
- FR-005 Cameras on pins count as the camera content of a dashboard: a limited user cannot save a pin of a camera
  outside their groups; a display may play the camera of a pin on its dashboard.
- FR-006 Any widget can be shown without a frame on a process picture (common parameter `frame`).

## Success Criteria
- SC-001 e2e `displays` covers pins (kiosk may play the pin camera; limited user refused).
- SC-002 Screenshots: plan, event, window with live picture, kiosk.

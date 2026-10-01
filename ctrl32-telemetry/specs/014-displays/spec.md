# Feature Specification: Displays (kiosks), camera groups, health incidents

**Feature Branch**: `main`

**Created**: 2026-09-27

**Status**: Implemented (0.49.0)

**Input**: After the list of what a professional system still lacks, the user: "Pokračuj a tie kiosky to sme mali mať
aj s dashboardmi, nie? Aby to bolo ako SCADA extra obrazovky a aj public dashboards." (continue; kiosks for dashboards
and SCADA screens too, like public dashboards). Agreed priorities: kiosk mode for walls, control of walls from the
control room with alarm pop-ups, per-camera permissions, health monitoring.

**Related**: specs 010, 011, 013; URS P-16…P-19; constitution I (audit), II, VII.

## Clarifications (defaults)
- One mechanism for all wall content: a paired display shows dashboards, process screens and video wall screens.
- A display never gets a user session; its credential opens only its own content. Public links stay without cameras.
- Pairing by a short code shown on the kiosk; the credential is collected by the kiosk itself (secret cookie), so a
  code seen by someone else cannot be used to impersonate the display.
- Operators (`display.control`) send cameras and commands; engineers and administrators (`display.manage`) pair.
- Camera groups are per camera; a user is limited to groups or sees all cameras (default).
- Health incidents use the incident log; thresholds are video settings.

## User Scenarios
1. (P1) An engineer pairs the PC of the left monitor in the control room with the code it shows; it alternates the
   video wall and the process screen and survives restarts.
2. (P1) Motion at the gate: the gate camera fills the wall display for 30 s.
3. (P1) An operator sends the car park camera to the wall display.
4. (P1) The gatekeeper sees only the gate cameras.
5. (P1) A recording camera goes offline at night: an incident opens and administrators are notified.

## Requirements
- FR-001 Pairing, display credential (HttpOnly, hashed), content validation, revoke, audit.
- FR-002 Display reads its dashboards (public API with `~slug`), its walls and cameras on its content only.
- FR-003 Kiosk page: items in turn, alarm pop-ups, commands, wake lock, reconnection, language.
- FR-004 Commands: camera, identify, clear, reload.
- FR-005 Camera groups enforced in every camera endpoint; user access editor.
- FR-006 Health incidents: camera offline / not recording, display dark, video disk nearly full.

## Success Criteria
- SC-001 e2e `displays` (61 checks) and unit tests pass; `video` still passes.

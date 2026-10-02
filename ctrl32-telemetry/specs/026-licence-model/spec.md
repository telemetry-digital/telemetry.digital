# Feature Specification: Licence model — free up to 4 cameras, updates included for a period

**Feature Branch**: `feature/0.66`

**Created**: 2026-10-02

**Status**: Implemented (0.66.0), not yet deployed

**Input**: The owner's plan (2 Oct 2026): "The system is free including up to 4 cameras. A licence (one per server)
covers more than 4 cameras, white labeling, changes through AI assistants (MCP writes) and the VPN extras (already:
>5 peers, sites, time-limited/approved technician access). The licence includes one year of updates; after the year the
system keeps running with everything it had — only updates stop until the licence is renewed." No prices anywhere new
(the existing buy button keeps its text); remove "bought once", "forever", "perpetual", "one-time" about the licence.

**Related**: ADR-0016 (amendment of 2 Oct 2026), ADR-0002, ADR-0017, ADR-0018, URS J-16a, J-16b, `docs/whitelabel.md`,
threat model.

## Clarifications (owner's decisions and defaults)

- The camera limit counts **enabled** cameras of **all organizations** of the server; removed cameras do not count.
- Nothing is switched off automatically: cameras over the limit keep running and can be edited; only adding an enabled
  camera or enabling a disabled one is refused (402 `license_required`). A disabled camera can be added.
- `updates_until` is a date claim in the licence key (YYYY-MM-DD, the whole day included). New keys get one year from
  the issue date (`licgen issue`) or from the payment (`licgen batch`, queue field `paid_at`); keys without it (issued
  before 0.66.0) include updates without a limit. Online-key tokens may carry it at most 400 days after issue.
- The release date of a version comes from the binary (`internal/release`, set at build time; development builds fall
  back to the VCS commit time). The update channel has no signed release metadata — a documented limitation.
- Servers without a valid licence always update; a licensed server refuses a version released after `updates_until`.
- A licence key whose updates ended before the running version was released cannot be installed (409 `updates_ended`).

## User Scenarios

1. **Small warehouse, no licence.** The administrator adds four cameras; the fifth is refused with "Without a licence
   this server runs up to 4 cameras (4 enabled now). The existing cameras keep working; more cameras need a licence
   (System → License)." Camera management shows "Cameras: 4 of 4 without a licence".
2. **Licence removed.** A server with 12 cameras loses its licence (removed with a reason): all 12 keep recording;
   enabling a thirteenth is refused; renaming any of them works.
3. **Updates ended.** A licence with updates until 2027-10-02 is installed. In November 2027 the administrator tries to
   install a version released 2027-11-01: the agent downloads and verifies it, reads its release date and refuses with
   "this version was released on 2027-11-01, after the updates included in the licence ended (2027-10-02); the system
   keeps running — renew the licence to install newer versions". The running system is unchanged. The License page shows
   "Updates ended on 2027-10-02 — the system keeps running; renew the licence to install newer versions".
4. **Free server.** A server without a licence installs every new version.

## Requirements

- FR-001 Camera limit in `POST /api/v1/video/cameras` and `PUT /api/v1/video/cameras/{id}` (enabled / switched on),
  in the same transaction under an advisory lock; 402 `license_required` with a clear message; MCP goes through the same
  handlers.
- FR-002 `GET /api/v1/server/license`: `cameras {enabled, free_limit, licensed}`, `updates_until`, `updates_ended`,
  `release_date`, `no_expiry` (`perpetual` kept as its older name); `GET /api/v1/video/storage`: `camera_limit`;
  `GET /api/v1/server`: `update.release_date`, `update.updates_until`, `update.updates_ended`.
- FR-003 Licence key claim `updates_until` (optional, validated as a date); `licgen issue -updates-until DATE|none`
  (default one year), `licgen batch` (`paid_at` / `updates_until` of the queue); online-key limit 400 days.
- FR-004 `ctrl32-telemetry version` prints "released YYYY-MM-DD"; `deploy/publish.sh` sets the date.
- FR-005 `POST /api/v1/server/update` passes `updates_until` to the agent; the agent refuses a later release (402
  `updates_ended`) before replacing anything; a binary without a date only while the updates run.
- FR-006 `PUT /api/v1/server/license` and the activation refuse a key whose updates ended before this release (409
  `updates_ended`).
- FR-007 UI: License page (intro, Updates and Cameras rows, no "perpetual"), Camera management (count badge without a
  licence), Updates page (release date, included updates, note), Settings → AI (no price); 19 catalogs.
- FR-008 Docs: `docs/whitelabel.md`, README, ADR-0016 amendment, ADR-0002/0017 notes, URS, threat model, constitution
  wording (1.0.1), public documentation.

## Success Criteria

- SC-001 Unit: `license` (claim, end of day, online limit), `release`, `agent` (`updateEntitled`), `licgen` (defaults,
  queue dates).
- SC-002 e2e `video` (camera limit), `whitelabel` (updates_until, refusals, activation with the claim), `server`,
  `displays`, `evidence`, `masks`, `mcp` stay green apart from failures that also occur on 0.65.0 on Windows.
- SC-003 Headless `license` with and without a licence (English and Slovak).
- SC-004 Not verifiable on the development PC: a real update through the agent refused by the release date (it would
  replace the running binary); the licence worker sending `paid_at`.

# Feature Specification: Redundant database — attach, copy, stand by, detach safely

**Feature Branch**: `assistant/jolly-sagan-nzolwc`

**Created**: 2026-09-25

**Status**: Implemented (0.39.0; phase 2 open: read-only sign-in on replicas, bandwidth limit, Windows host test)

**Input**: User description: "Redundant database. Example: one database is running, I connect a second one on another
PC, the data from the first one is poured into it, and the second one disconnects safely." Follow-up answer: all of
these must be possible and selectable — a copy to another PC followed by a safe disconnect, a hot standby with
failover, and a continuous offsite backup.

**Related**: ADR-0003 (sizing: "higher volumes = PostgreSQL replica, not another architecture"), ADR-0005 (record
integrity), ADR-0008 S5, ADR-0012 (server agent), URS K-01…K-05 (backup/restore), new URS K-15…K-20.

## Clarifications

### Session 2026-09-25

- Q: Promotion of the hot standby — manual one-click or automatic? → A: Manual one-click by an administrator (with reason); no automatic failover in this feature.
- Q: "Resume without starting over" during the initial copy? → A (planning, research R5): the catch-up after the base
  copy resumes from the replication slot without gaps; an interruption during the base copy itself restarts the base
  copy (PostgreSQL cannot resume a partial base backup). No change is lost in either case.
- Q: Signing in on a replica for viewing (US2 AS3)? → A (planning, research R7): phase 2 of this feature; phase 1
  shows the replica's state on its status page and in the CLI, and the replica refuses every write.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Copy the live database to another PC and disconnect it safely (Priority: P1)

An administrator installs ctrl32 telemetry on a second PC in "replica" mode, pairs it with the running server using
a one-time pairing code shown on the Server page, and chooses the mode "one-time copy". The second PC receives all
data from the first while the first keeps running and collecting. The Server page shows progress (copied size,
remaining, estimated time) and then "in sync". The administrator presses "Detach safely": the system waits until
the copy holds everything written so far, verifies it (record counts and the audit-trail hash chain up to the same
last entry), records the detachment in the audit trail of both databases with the last entry's number and hash, and
stops the connection. The detached copy is a complete, consistent, verifiable database that can be archived, moved
or started as its own server.

**Why this priority**: This is the user's own example and the basis for the other modes.

**Independent Test**: With the primary receiving telemetry, attach a second PC, wait for "in sync", detach, then on
the second PC run the integrity verification and compare counts and the last audit entry with the primary at the
detachment time.

**Acceptance Scenarios**:

1. **Given** a running server with data, **When** a replica is paired with a valid code, **Then** the copy starts
   without stopping data collection on the primary and progress is visible.
2. **Given** a replica "in sync", **When** "Detach safely" is confirmed with a reason, **Then** detachment completes
   only after the replica holds every record up to a named audit entry, and both audit trails show the detachment
   with user, time, reason, last entry number and hash.
3. **Given** a detached copy, **When** its integrity verification runs, **Then** it reports the same last audit
   entry and hash as recorded at detachment and no broken link in the chain.
4. **Given** the network drops during the copy, **When** it returns, **Then** the copy resumes without starting over
   and without duplicate or missing records.
5. **Given** a wrong or expired pairing code, **Then** pairing is refused and the attempt is logged as a security event.

---

### User Story 2 - Continuous offsite replica as a live backup (Priority: P1)

The administrator chooses the mode "continuous replica" for a second PC at another site. The replica stays
connected and receives every change within seconds. The Server page shows its state (connected, lag in seconds
and in data size, last received time) and raises an alarm/notification when the replica is disconnected or its lag
exceeds a limit. The replica is read-only; it can be opened for viewing (dashboards, reports, exports) without
being able to change anything.

**Why this priority**: Protects against loss of the primary machine (fire, disk failure, ransomware on one site)
with minutes of work, which regulated users expect (Annex 11 data availability).

**Independent Test**: Run a continuous replica, generate data, measure lag, cut the link to trigger the alarm,
restore it and verify catch-up; attempt a write on the replica and verify it is refused.

**Acceptance Scenarios**:

1. **Given** a continuous replica, **When** a measurement is stored on the primary, **Then** it is present on the
   replica within the configured lag limit (default 10 s) under normal load.
2. **Given** a lag limit of 60 s, **When** the replica is unreachable for 2 minutes, **Then** an alarm is raised
   and notifications go to the configured recipients; **When** it catches up, **Then** the alarm clears.
3. **Given** a user signed in on the replica, **When** they try to change anything, **Then** the system refuses
   and explains that this is a read-only replica.

---

### User Story 3 - Hot standby and failover (Priority: P2)

The administrator chooses the mode "hot standby" for the second PC. It runs a complete ctrl32 server in standby: it
receives all data like a continuous replica and is ready to take over. When the primary fails, the standby is
promoted to primary manually, by an administrator with one click, a reason and a confirmation (no automatic promotion: it would
need a third witness machine to avoid two primaries). After promotion, devices and users reach the new primary (same address through a shared name or
virtual address configured by the administrator), and the former primary, when it returns, never accepts writes
again until it is re-attached as a replica of the new primary.

**Why this priority**: The highest availability level; more setup (network name/address) and more risk than US1–US2.

**Independent Test**: Stop the primary, promote the standby, publish telemetry from a device to the shared address,
verify it is stored; start the old primary and verify it refuses writes and offers "re-attach as replica".

**Acceptance Scenarios**:

1. **Given** a hot standby in sync, **When** promotion is triggered, **Then** the standby accepts writes within 60 s
   and the audit trail continues without a gap from the last replicated entry.
2. **Given** a promoted standby, **When** the old primary starts, **Then** it detects that it was demoted and does not
   accept writes (no two primaries).

---

### User Story 4 - Choose and change the redundancy function in settings (Priority: P2)

On the Server page the administrator sees the redundancy function: none, one-time copy, continuous replica, hot
standby; lists attached replicas with their mode and state; changes a replica's mode (for example continuous →
detach safely), sets lag limits and notification recipients. Every change needs a reason and is audited.

**Why this priority**: The user asked for "možnosť nastaviť funkciu" — the choice itself is a requirement.

**Independent Test**: Switch a replica through the modes and check the audit trail and the Server page.

**Acceptance Scenarios**:

1. **Given** a continuous replica, **When** the administrator selects "Detach safely", **Then** US1's safe detach runs.
2. **Given** any change of mode or limits, **Then** the audit trail records old value, new value, user and reason.

---

### Edge Cases

- Different PostgreSQL major versions or ctrl32 versions on the two PCs: pairing is refused with a clear message.
- Not enough disk space on the replica: the copy stops before filling the disk and reports the required size.
- Primary restarted during the copy: the copy resumes.
- Slow link (for example 10 Mbit/s VPN): the copy continues; the estimate reflects the real speed; the primary is
  not slowed down beyond a configured bandwidth limit.
- Primary disk filling because a disconnected replica holds back cleanup: after a configured maximum retained size
  the replica is dropped (and must be re-attached), with an alarm — the primary never stops collecting data.
- Linux primary with Windows replica and vice versa: supported (same database major version).
- Detach while not in sync: refused unless the administrator explicitly accepts a partial copy, which is then
  marked as partial with its last complete audit entry.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST let an administrator (permission `system.admin`) attach a second database on another
  machine by a one-time pairing code valid for at most 15 minutes.
- **FR-002**: The system MUST support three selectable modes per replica: one-time copy, continuous replica,
  hot standby.
- **FR-003**: The initial copy MUST run while the primary keeps collecting data, resume after interruptions and
  show progress and estimated time.
- **FR-004**: Replication traffic MUST be encrypted and authenticated with credentials dedicated to replication; no
  database superuser password leaves the primary.
- **FR-005**: A replica MUST be read-only for users and devices; attempts to change data are refused.
- **FR-006**: "Detach safely" MUST: wait for full catch-up to a named last audit entry, verify record counts per
  record table and the audit hash chain on the replica, record the detachment in both audit trails (user, time,
  reason, last entry number and hash), then disconnect. A failed verification MUST stop the detachment and report
  what differs.
- **FR-007**: A detached copy MUST be startable as an independent server and verifiable with the existing
  integrity verification.
- **FR-008**: The Server page MUST show each replica's mode, state, lag (time and size), last contact and history.
- **FR-009**: The system MUST raise an alarm and notify when a replica is disconnected or exceeds its lag limit, and
  MUST protect the primary's disk with a maximum retained size per replica.
- **FR-010**: Hot standby MUST support promotion to primary and MUST prevent two primaries (fencing of the old
  primary).
- **FR-011**: Every attach, mode change, detach, promotion and failed pairing MUST be audited (security events
  flagged).
- **FR-012**: The feature MUST work on Linux and Windows servers and be available through the API
  (constitution IV), with host operations performed by the server agent's closed operation list.
- **FR-013**: The documentation MUST include the procedure for moving a detached copy to another site and for
  re-attaching a demoted primary.

### Key Entities

- **Replica**: identity, name, machine, mode, state (pairing, copying, in sync, lagging, disconnected, detaching,
  detached, promoted), lag, limits, created by/at.
- **Pairing code**: one-time secret with expiry, bound to the administrator who created it.
- **Detachment record**: replica, time, user, reason, last audit entry number and hash, verification result.
- **Replication event**: state changes and alarms of a replica, append-only.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A 10 GB database is copied to a replica on a 1 Gbit/s LAN in under 30 minutes while the primary keeps
  storing 1 000 measurements per second without loss.
- **SC-002**: A continuous replica lags less than 10 seconds behind the primary under normal load (95th percentile).
- **SC-003**: Safe detachment of an in-sync replica, including verification, completes in under 5 minutes for 10 GB.
- **SC-004**: After detachment, 100 % of records and the audit chain head match the values recorded at detachment.
- **SC-005**: A hot standby accepts writes within 60 seconds of promotion; 0 records acknowledged by the primary
  before its failure are missing on the promoted standby, except records written in the last lag window.
- **SC-006**: An administrator attaches a replica in under 10 minutes following the documentation.

## Assumptions

- Both machines run the same ctrl32 release and the same PostgreSQL major version (18).
- The network path between the machines (LAN, VPN, or the WireGuard tunnel the agent already manages) is provided
  by the deployer; the product secures the replication channel itself.
- Shared addressing for failover (DNS name, virtual IP or reverse proxy) is configured by the deployer; the product
  documents the options and checks them.
- One primary with up to 3 replicas is enough for the target market (single-company deployments, ADR-0003).
- Backups (URS K-01…K-05) remain required; a replica is not a backup of deleted or corrupted data and the
  documentation says so.

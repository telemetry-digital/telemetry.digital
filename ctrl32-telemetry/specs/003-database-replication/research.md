# Research: Redundant database

## R1 — Replication mechanism

- **Decision**: PostgreSQL physical streaming replication of the whole cluster, one physical replication slot per
  replica, orchestrated by ctrl32 (pairing, transport, monitoring, detach, promotion).
- **Rationale**: ADR-0003 already names "PostgreSQL replica" as the scale-up path; a physical replica is a
  byte-identical copy (partitions, triggers, audit chain, functions, roles) with no application-level merge logic;
  `pg_basebackup` copies online while the primary keeps writing; hot standby gives read-only queries; `pg_promote()`
  turns a replica into an independent primary without restart; `max_slot_wal_keep_size` caps the WAL a disconnected
  replica can hold back (FR-009 disk protection — PostgreSQL invalidates the slot instead of filling the disk).
- **Alternatives**: logical replication (per-table publications; sequences, DDL and new month partitions are not
  replicated — breaks with every migration and every new partition); application-level copy in Go (re-implements
  consistency, conflicts and ordering; many mutable tables); file-level `rsync` of the data directory (needs a stopped
  primary or backup mode and root on both hosts).

## R2 — Transport between the PCs

- **Decision**: the replica's ctrl32 opens an HTTPS connection to the primary's public address and upgrades it
  (`Upgrade: ctrl32-pgrepl`) at `GET /api/v1/replication/tunnel`, authenticated with the replica's bearer token; the
  primary's ctrl32 connects the upgraded stream to its local PostgreSQL. On the replica, ctrl32 listens on
  `127.0.0.1:<tunnel port>` and each local connection (pg_basebackup, the standby's WAL receiver) becomes one tunnel.
- **Rationale**: PostgreSQL stays bound to localhost on both machines (install.sh default); only outbound 443 is
  needed from the replica (works through NAT and firewalls, like the devices); TLS comes from the existing Caddy
  certificate; the token is revocable per replica; a bandwidth limit can be applied in the copier (edge case "slow
  link").
- **Alternatives**: exposing 5432 with PostgreSQL TLS (needs certificates for the DB, firewall rules, pg_hba per
  address); the agent's WireGuard tunnel (works, remains an option for deployers — the replica can point the tunnel
  at a VPN address instead of the public name).

## R3 — Credentials (FR-004)

- **Decision**: at pairing the primary creates a role `ctrl32_repl_<id>` with `LOGIN REPLICATION` and a random
  32-byte password, a `pg_hba` rule allowing it only for `replication` connections from localhost, and the slot
  `ctrl32_replica_<id>`. The password and a random replica token (stored as SHA-256) are returned once to the
  pairing replica over TLS. Superuser credentials never leave the primary. Detach/drop removes role, rule and slot.
- The `pg_hba` edit and reload are host operations → a new closed agent operation `replication-hba` (docker: edit
  the file inside the container volume via `docker exec`; native: the cluster's `hba_file`). The SQL parts (role,
  slot, `ALTER SYSTEM SET max_slot_wal_keep_size`, reload) use `admin_url`.

## R4 — Pairing code

- **Decision**: 12 characters from an unambiguous alphabet (`ABCDEFGHJKMNPQRSTUVWXYZ23456789`, ~59 bits) shown as
  `XXXX-XXXX-XXXX`, valid 15 minutes, single use, stored as SHA-256, bound to mode, name and creating admin.
  `POST /api/v1/replication/pair` is rate-limited per IP (5/min); a wrong or expired code is a security event.

## R5 — Initial copy and resume (FR-003, US1 AS4)

- **Decision**: `pg_basebackup -X stream -S <slot> -R --checkpoint=fast --progress` through the tunnel; progress
  comes from `pg_stat_progress_basebackup` on the primary (bytes total/streamed → percentage and ETA on the Server
  page). After the base copy the WAL receiver streams from the slot and resumes after any interruption without gaps
  or duplicates (the slot keeps the WAL). An interruption **during** the base copy restarts the base copy (pg_basebackup
  cannot resume a partial copy); the slot guarantees that no change is lost in either case.
- Recorded as a clarification in the spec (the "without starting over" wording applies to the catch-up phase).

## R6 — Safe detach and verification (FR-006, SC-004)

- **Decision**:
  1. The primary opens a `REPEATABLE READ` transaction D: `pg_current_snapshot()` → S; counts of every append-only
     record table (the tables protected by `core.make_append_only`) in S; appends the audit entry
     `replica.detach` (seq N, hash H, with S and the counts); commits; reads the WAL position L after the commit.
     No locks — the primary keeps collecting.
  2. The replica is told (command channel) to detach at L: it waits until `pg_last_wal_replay_lsn() ≥ L`, then
     calls `pg_promote()` (now an independent database), and verifies: `core.verify_audit_chain()` is OK, the entry
     N exists with hash H, and for every record table the count of rows **visible in snapshot S**
     (`pg_visible_in_snapshot(xid8 of xmin, S)`) equals the primary's count. Rows written after S are part of the
     copy but outside the comparison.
  3. The replica appends `replica.detached` (N, H, verification) to its own audit trail and reports the result; the
     primary appends `replica.detach_done` (or `replica.detach_failed` with the differences), then drops slot, role,
     hba rule and token.
- Purge (the only deleter of old observations) takes the advisory lock `ctrl32.purge`; detach holds it shared from
  step 1 until the result, so no row visible in S disappears on the primary side in between.
- **Partial detach** (not in sync, admin accepts): the replica promotes at its current position and records the last
  complete audit entry it holds; marked `partial`.

## R7 — Replica mode of the ctrl32 server

- **Decision**: at start, `pg_is_in_recovery()` decides the mode. In replica mode ctrl32 starts only: HTTP (read-only
  replica page and read-only API role/status), the tunnel listener, the command poller and the status reporter —
  no MQTT broker, ingest, rules, gap detector, connectors, reports or escalations. When the database is promoted
  (detach or failover) the process exits with a distinct status and the service manager restarts it as a normal
  server.
- Signing in on a replica needs sessions that cannot be written to a read-only database → phase 2 (in-memory
  sessions for replicas, read-only UI); phase 1 exposes the replica's state on a status page and via the CLI.

## R8 — Hot standby promotion and fencing (FR-010)

- **Decision**: promotion is manual (clarified): `ctrl32-telemetry replica promote -reason …` on the standby (and the
  button on its page in phase 2). It calls `pg_promote()`, records `replica.promoted` with the new epoch and the former
  primary's node id, stops the tunnel and restarts as a primary.
- Every ctrl32 node has a node id (random, stored in `core.node` of its own database and in the state file). A hot
  standby registers its public URL at pairing. A primary checks each registered standby at start and every minute
  (`GET /api/v1/replication/role`); when a standby reports `role=primary, promoted_from=<this node>`, the primary
  **fences itself**: writes a fence file next to its config, stops ingest and MQTT, refuses writes (HTTP 503 with the
  explanation) and shows "demoted — re-attach as replica" until it is re-attached (a fresh base copy from the new
  primary). The fence file is checked before anything else at start, so a restart does not un-fence.

## R9 — Monitoring and alarms (FR-008, FR-009)

- The primary's monitor (10 s) reads `pg_stat_replication` (state, write/flush/replay LSN and lag) and
  `pg_replication_slots` (active, `wal_status`, `safe_wal_size`), stores the current state on `core.replica` and appends
  `core.replica_event` on every state change. Disconnected longer than the lag limit, lag above the limit, or slot
  `lost` → an **incident** (existing incident module: notifications, acknowledgement) that closes itself on recovery.

## R10 — Platforms

- Host operations on the replica (stop PostgreSQL, replace its data directory with the base copy, start it) are done by
  the CLI run as root/Administrator on the replica: docker (`docker compose` + a one-off `postgres:18` container on the
  volume), native Linux (`systemctl` + `pg_basebackup` of the installed major version), native Windows (`sc` +
  `pg_basebackup.exe`). A `-pgdata`/`-pgctl` pair supports clusters managed by hand (and the tests).

---
description: "Tasks for the redundant database"
---

# Tasks: Redundant database

**Input**: `specs/003-database-replication/` · **Tests**: required · **Paths**: relative to `ctrl32-telemetry/`

## Phase 1: Foundation
- [x] T001 Migration 0022: core.node, core.replica, core.replica_pairing, core.replica_event (append-only), grants, count-in-snapshot function in db/migrations/0022_replication.sql
- [x] T002 [P] Pairing codes (generate, normalise, hash) with unit tests in internal/replication/code.go
- [x] T003 [P] Upgrade tunnel: server side (hijack → dial PG) and client side (local listener → upgraded HTTPS) with a unit test over httptest in internal/replication/tunnel.go

## Phase 2: US1 attach and copy (P1) 🎯 MVP
- [x] T004 [US1] Primary: create pairing, pair (versions check, role, slot, hba via agent or admin SQL, token), replica token auth, progress from pg_stat_progress_basebackup in internal/replication/primary.go
- [x] T005 [US1] HTTP API + OpenAPI entries in internal/httpapi/replication.go
- [x] T006 [US1] CLI `replica attach|status|forget` with host operations (docker, native Linux, native Windows, -pgdata/-pgctl) in cmd/ctrl32-telemetry/replica.go and internal/replication/hostops.go
- [x] T007 [US1] Replica mode of the server (recovery detection, tunnel listener, command poller, reporter, status page, restart on promotion) in cmd/ctrl32-telemetry/main.go and internal/replication/replica.go

## Phase 3: US2 continuous replica (P1)
- [x] T008 [US2] Monitor loop (pg_stat_replication, slots, states, events, max_slot_wal_keep_size) and incidents in internal/replication/monitor.go
- [x] T009 [US2] Server page: Redundancy section (node role, replicas, progress, lag, events, attach dialog with code) in internal/httpapi/static/server.html

## Phase 4: US1 safe detach
- [x] T010 [US1] Detach: snapshot counts + audit entry on the primary, command to the replica, promote + verify + report, cleanup, partial detach, purge advisory lock in internal/replication/detach.go
- [x] T011 [US1] e2e: pair (wrong code refused and logged), copy, telemetry reaches the replica, detach, verification, detached server verifies its audit chain in tests/e2e/replication.sh

## Phase 5: US3 hot standby (P2)
- [x] T012 [US3] `replica promote`, `GET /api/v1/replication/role`, node epoch, fencing check and fence file, read-only fenced mode
- [x] T013 [US3] e2e: promote the standby, write to it, start the old primary → fenced in tests/e2e/replication.sh

## Phase 6: US4 settings (P2)
- [x] T014 [US4] Change mode/limits/standby URL with reason (audit old/new), drop replica; UI controls
- [x] T015 [P] [US4] Headless: Redundancy section renders, attach dialog shows a code, detach needs a reason in tests/headless/replication.js

## Phase 7: Polish
- [x] T016 [P] Runbook docs/runbook-replication.md (attach, detach, move a copy, failover, re-attach a demoted primary, "a replica is not a backup")
- [x] T017 [P] URS K-15…K-20, JOURNAL, README; sk/cs translations
- [ ] T018 run-all green, commit, push

## Later (phase 2 of the feature)
- Read-only sign-in on replicas (in-memory sessions), bandwidth limit on the tunnel, Windows host operations tested on a Windows machine.

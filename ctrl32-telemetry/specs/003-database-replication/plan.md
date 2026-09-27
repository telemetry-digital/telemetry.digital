# Implementation Plan: Redundant database

**Branch**: `assistant/jolly-sagan-nzolwc` | **Spec**: [spec.md](spec.md)

## Summary
PostgreSQL physical streaming replication orchestrated by ctrl32: one-time pairing code, replication carried inside
the existing HTTPS endpoint (upgrade tunnel), `pg_basebackup` driven by a CLI on the second PC, a replica mode of the
server, monitoring with incidents, verified safe detach, manual promotion with fencing. See research.md R1–R10.

## Technical Context
Go 1.27 server + agent; PostgreSQL 18 (docker or native; tests on 16); pgx; plain JS UI (ADR-0013); new package
`internal/replication`; migration 0022; server page section; CLI `replica`.

## Constitution Check
- I Data integrity: detach verification (audit chain + snapshot-exact counts) and append-only events — ✅
- II Security: dedicated replication role, token, TLS, rate-limited pairing, security events, no superuser secrets
  leave the primary — ✅
- III No user code: none — ✅
- IV API first: every function in the API; CLI only for host operations on the second PC — ✅
- V Tests: unit (pairing code, tunnel, state machine, count SQL), e2e with two clusters, headless Server page — ✅
- VI Plain JS: ✅  VII Docs: runbook in docs + URS K-15…K-20 — ✅

## Project structure
```
db/migrations/0022_replication.sql
internal/replication/{code.go, tunnel.go, primary.go, monitor.go, detach.go, replica.go, hostops.go, *_test.go}
internal/httpapi/replication.go (+ openapi entries), static/server.html (Redundancy section)
cmd/ctrl32-telemetry/replica.go (CLI), main.go (replica mode / fence check)
internal/agent: operation replication-hba
tests/e2e/replication.sh, tests/headless/replication.js
docs/runbook-replication.md
```

## Phases
1. Foundation: migration, node id, pairing codes, tunnel (both ends), replica token auth.
2. US1 attach + copy: pair endpoint (role, slot, hba), CLI attach (host ops), replica mode, progress.
3. US2 monitoring: monitor loop, events, incidents, Server page list.
4. US1 detach: command channel, snapshot counts, promote + verify, cleanup.
5. US3 standby: promote CLI, role endpoint, fencing.
6. US4 settings: mode/limits changes, drop, audit.
7. Phase 2 (later): read-only sign-in on replicas (in-memory sessions), bandwidth limit, Windows host ops test.

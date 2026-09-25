# Database

PostgreSQL 18 is the only external dependency of ctrl32 telemetry (ADR-0001). The schema lives in `migrations/`, applied in order by the application's migration runner (phase 1) or manually with `psql` during phase 0.

## Layout

- `migrations/0001_core.sql` — core schema v1: organization hierarchy, devices/decoders/sensors/datastreams, append-only `raw_message` and `observation` (month partitions), gaps, identity and access, hash-chained `audit_trail` with signed checkpoints, alarms, configuration baseline, system log, and least-privilege grants for the `ctrl32_app` role.
- `tests/0001_smoke.sql` — executable acceptance test for the integrity guarantees (URS B-03, B-04, I-01, I-02, I-03, C-03, H-11). Ends with `SMOKE OK`.

## Guarantees enforced in the database (not only in application code)

| Guarantee | Mechanism |
|---|---|
| Record tables are append-only | `core.make_append_only()` attaches BEFORE UPDATE/DELETE row triggers and a BEFORE TRUNCATE statement trigger; applies to every month partition automatically |
| `received_at` is database time | `DEFAULT now()`; the application never supplies it |
| Audit trail is a gapless hash chain | `audit_chain_before_insert()` locks the single `audit_chain_state` row, assigns `seq`, `prev_hash`, `hash = sha256(prev_hash | seq | canonical row)` |
| Reason is mandatory for changes/deletions/acknowledgements | CHECK constraint on `audit_trail.action` verb and on `alarm_event.event` |
| Chain can be verified by anyone with SELECT | `core.verify_audit_chain(from_seq, to_seq)` returns ok/checked/first_bad_seq/head |
| Application cannot alter records or schema | `ctrl32_app` has SELECT/INSERT on all tables, UPDATE only on operational tables, DELETE only on `session` and `system_log`, no DDL |

## Local development

```
docker compose -f deploy/dev/docker-compose.yml up -d
docker exec -i ctrl32-pg-dev psql -U ctrl32 -d ctrl32 -v ON_ERROR_STOP=1 < db/migrations/0001_core.sql
docker exec -i ctrl32-pg-dev psql -U ctrl32 -d ctrl32 -v ON_ERROR_STOP=1 < db/tests/0001_smoke.sql
```

Month partitions: `core.ensure_month_partitions('core.observation', current_date, 3)` creates the current and the next two months; the scheduler calls it daily (phase 1). Partitions are never dropped automatically; retention detaches them only after a verified archive (URS K-06).

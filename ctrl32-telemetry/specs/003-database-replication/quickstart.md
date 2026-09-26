# Quickstart: validate the redundant database

Local (tests/e2e/replication.sh does this): the primary is the e2e PostgreSQL cluster, the replica a second
cluster created by `pg_basebackup` into a temporary directory and run with `pg_ctl`; both ctrl32 servers on one
machine (ports 8080/8090).

1. Primary Server page → Redundancy → "Attach a replica" → mode, name, reason → code.
2. Second PC: install ctrl32 (any mode), then `sudo ctrl32-telemetry replica attach -primary https://<primary>
   -code <code>`. Expected: progress on both sides, then state `in_sync`.
3. Publish telemetry on the primary → visible on the replica within the lag limit (`replica status`).
4. Stop the replica's network → after the lag limit an incident "replica disconnected"; restore → catch-up, incident
   closed.
5. Primary → "Detach safely" with a reason → state `detached`, detachment record with seq/hash and "verification ok";
   the second PC restarts as an independent server; `ctrl32-telemetry audit verify` there reports the same entry.
6. Standby: attach with `-standby-url`, stop the primary, `ctrl32-telemetry replica promote -reason …` on the standby
   → it accepts telemetry; start the old primary → it reports "fenced" and refuses writes.

# Contract: replication API (all JSON; admin endpoints need `system.admin` and a reason for changes)

## Primary — administration
- `GET /api/v1/replication` → `{node:{id, role: primary|replica|standby|fenced, epoch}, settings:{max_retained_mb},
  replicas:[Replica]}`
- `POST /api/v1/replication/pairings` `{mode, name, lag_limit_s?, reason}` → 201 `{code:"ABCD-EFGH-JKLM",
  expires_at}` (shown once)
- `PATCH /api/v1/replication/replicas/{id}` `{mode?, lag_limit_s?, max_retained_mb?, standby_url?, reason}` → Replica
- `POST /api/v1/replication/replicas/{id}/detach` `{reason, accept_partial?}` → 202 Replica (state `detaching`);
  refused 409 when not in sync unless `accept_partial`
- `DELETE /api/v1/replication/replicas/{id}?reason=` → drop (slot, role, rule, token) → 200
- `GET /api/v1/replication/replicas/{id}/events` → `[Event]`

## Primary — used by the replica (bearer replica token; pairing by code)
- `POST /api/v1/replication/pair` `{code, name, hostname, os, pg_version, ctrl32_version, standby_url?}` →
  201 `{replica_id, token, role, password, slot, mode, primary_node, pg_port_hint}`; 400 version mismatch; 403
  wrong/expired/used code (security event); 429 rate limit
- `GET /api/v1/replication/tunnel` with `Connection: Upgrade`, `Upgrade: ctrl32-pgrepl` → 101 and a raw byte stream to
  the primary's PostgreSQL
- `GET /api/v1/replication/agent` → `{command|null}` (long poll ≤ 25 s)
- `POST /api/v1/replication/agent` `{state?, copy?:{total,done}, replay_lsn?, result?:{…}}` → 200

## Any node
- `GET /api/v1/replication/role` (no auth) → `{node_id, role, epoch, promoted_from}`

## Replica = Replica object
`{id, name, mode, state, lag_s, lag_bytes, last_contact, copy:{total_bytes, done_bytes, percent, eta_s},
  lag_limit_s, max_retained_mb, standby_url, host_info, detach, created_at, created_by}`

## CLI (on the second PC, as root/Administrator)
- `ctrl32-telemetry replica attach -primary https://host -code XXXX-XXXX-XXXX [-name N] [-standby-url URL]
   [-pgdata DIR -pgctl PATH] [-config path]`
- `ctrl32-telemetry replica status | promote -reason R | forget`

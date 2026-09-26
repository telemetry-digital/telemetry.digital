# Runbook: redundant database

How to attach a second PC to a running ctrl32 telemetry server, keep it as a live copy or a hot standby, detach it
safely, fail over, and bring a former primary back. Specification: `specs/003-database-replication/`; URS K-15…K-20.

**A replica is not a backup.** A deletion or a damaged record on the primary is copied to the replica within seconds.
Keep the daily backups (Server → Backups, URS K-14) in any case.

## How it works (in one paragraph)

The copy is PostgreSQL's own physical streaming replication: a byte-identical copy of the whole database cluster
(records, audit trail and its hash chain, partitions, roles). It travels inside the primary's normal HTTPS address:
the second PC opens `https://<primary>/api/v1/replication/tunnel`, upgraded to a raw stream, with a token of its own.
PostgreSQL stays bound to localhost on both machines; only outbound HTTPS is needed from the second PC. Each replica has
its own replication role, slot and `pg_hba` rule on the primary, all removed when it is detached or dropped. The primary
keeps at most "Maximum kept" of change log for a replica that is away; beyond that the replica is dropped ("lost") and
must be attached again — the primary never stops collecting and never fills its disk.

## Requirements

- Both machines run the **same ctrl32 telemetry release** and the **same PostgreSQL major version** (18).
- The second PC reaches the primary's public URL over HTTPS (LAN, VPN, the WireGuard tunnel of Server → WireGuard,
  or the internet).
- The second PC has ctrl32 telemetry installed (`install.sh` / `install.ps1`, any profile). **Its local database is
  replaced by the copy**; the old data directory is moved aside (`<data dir>.before-replica-<time>`), never deleted.
- Disk on the second PC: at least the size of the primary's database, plus the old data directory.

## Attach (one-time copy, continuous replica or hot standby)

1. Primary: Server → **Redundant database** → **Attach a replica**. Choose the function, a name (e.g. "Site B") and
   the lag limit; enter a reason. A code `XXXX-XXXX-XXXX` appears, valid 15 minutes, shown once.
2. Second PC, as root (Administrator):

       sudo ctrl32-telemetry replica attach -primary https://telemetry.example.com -code XXXX-XXXX-XXXX

   Hot standby: add `-standby-url https://<this machine's public URL>` — a returning primary asks this address who is
   primary (fencing). The command stops the local service and PostgreSQL, copies the primary (progress is printed and
   shown on the primary's page), starts PostgreSQL as a standby and the service in replica mode.
3. Primary: the replica goes `waiting for the replica → copying → in sync`. Continuous replica and hot standby stay
   in sync; lag and last contact are on the page. Disconnection longer than the lag limit, lag above it, or "lost"
   opens an **incident** (Incidents page, e-mail/push to administrators) that resolves itself when the replica recovers.

The second PC's own page (`http://<second PC>/`) shows the replica's state; every change there is refused (it is
read-only). `ctrl32-telemetry replica status` prints the same on the console.

## Detach safely (the user's scenario: copy, then disconnect)

Primary → the replica → **Detach safely**, with a reason. Then, without stopping data collection:

1. The primary counts every record table in one snapshot and writes the audit entry `replica.detach` (number and hash).
2. The replica catches up to that entry, becomes an independent database (new node identity, epoch 2), and verifies:
   the audit hash chain, the `replica.detach` entry with the same hash, and the record counts as of the snapshot.
3. Both audit trails record the result (`replica.detach_done` on the primary, `replica.detached` on the copy); the
   primary removes the replica's access. The page shows "verified" or the exact differences ("failed").
4. The second PC restarts as a normal, independent ctrl32 telemetry server with the copy.

A replica that is not in sync can be detached only with **accept a partial copy**; it is marked partial.

## Move a detached copy to another site

The detached copy is a normal installation: take a backup there (Server → Backups) and restore it anywhere, or move
the machine. `ctrl32-telemetry audit verify` on it reports the chain intact including the detachment entry. It does
not receive data (devices still send to the primary) unless you point devices at it.

## Failover to a hot standby

Manual by design (no automatic promotion: that needs a third machine to avoid two primaries).

1. Make sure the primary is really down or isolated.
2. On the standby, as root:

       sudo ctrl32-telemetry replica promote -reason "primary hardware failure, ticket 123"

   It promotes the database, records `replica.promoted` with the reason, and restarts the service as the primary.
3. Point devices and users at the standby: switch the shared DNS name (or virtual IP / reverse proxy) to it. Devices
   reconnect by themselves; their backfill closes the gap of the minutes in between.
4. The changes of the last lag window before the failure (normally seconds) may be missing — the incident history
   and the replica page showed the lag.

## When the former primary returns (fencing)

At start and every minute a primary asks its registered hot standbys who is primary. If a standby reports that it was
promoted from this server, the server **fences itself**: it writes `fenced.json` next to its configuration, stops
MQTT, ingest and every writer, answers every request with 503 and an explanation, and reports `role: fenced` at
`/api/v1/replication/role`. The fence survives restarts.

To use the machine again, re-attach it as a replica of the new primary: on the new primary create a code, on the old
machine remove `fenced.json` and run `ctrl32-telemetry replica attach …` (its database is replaced by a fresh copy;
the old data directory is kept aside for investigation).

## Change or remove

- **Settings**: function, lag limit, maximum kept on the primary, standby URL — with a reason; the audit trail keeps
  old and new values.
- **Drop**: removes the replica's token, role, slot and rule immediately; the copy is left as it is (not verified).

## Troubleshooting

| Symptom | Cause and remedy |
|---|---|
| `pairing refused (403)` | wrong, used or expired code — create a new one (refused attempts are security events) |
| `pairing refused (400): … PostgreSQL 18, this machine 17` | install the same PostgreSQL major version on both |
| stuck at "waiting for the replica" | the second PC did not complete the base copy — see the command's output; the primary's `/api/v1/replication/tunnel` must be reachable over HTTPS |
| "lost" | the replica was away longer than "maximum kept" allowed — drop it and attach again |
| detach "failed" | read the differences on the page; the copy is independent anyway but is not a verified complete copy |

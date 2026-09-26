-- Redundant database (spec 003): identity of this database, replicas attached to it, one-time pairing codes and the
-- append-only replica history. Replication itself is PostgreSQL physical streaming replication (research R1); these
-- tables hold what ctrl32 needs to pair, monitor, detach and promote.

-- This database's identity. A physical replica carries its primary's row; promotion replaces id/epoch in one
-- guarded update (the only change ever allowed), so "who was promoted from whom" is part of the data.
CREATE TABLE core.node (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  singleton      boolean NOT NULL DEFAULT true UNIQUE CHECK (singleton),
  epoch          integer NOT NULL DEFAULT 1,
  promoted_from  uuid,
  created_at     timestamptz NOT NULL DEFAULT now()
);
INSERT INTO core.node DEFAULT VALUES;
CREATE OR REPLACE FUNCTION core.guard_node() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'UPDATE' AND current_setting('ctrl32.promote', true) = 'on' AND NEW.epoch = OLD.epoch + 1 AND NEW.promoted_from = OLD.id THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'ctrl32: core.node changes only on promotion' USING ERRCODE = 'integrity_constraint_violation';
END $$;
CREATE TRIGGER guard_node BEFORE UPDATE OR DELETE ON core.node FOR EACH ROW EXECUTE FUNCTION core.guard_node();
CREATE TRIGGER guard_node_truncate BEFORE TRUNCATE ON core.node FOR EACH STATEMENT EXECUTE FUNCTION core.deny_truncate();

-- core.promote_node(): called on a freshly promoted replica; returns the new node id.
CREATE OR REPLACE FUNCTION core.promote_node() RETURNS uuid
LANGUAGE plpgsql AS $$
DECLARE nid uuid := gen_random_uuid();
BEGIN
  PERFORM set_config('ctrl32.promote', 'on', true);
  UPDATE core.node SET promoted_from = id, id = nid, epoch = epoch + 1;
  RETURN nid;
END $$;

CREATE TABLE core.replica (
  id               uuid PRIMARY KEY DEFAULT uuidv7(),
  name             text NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
  mode             text NOT NULL CHECK (mode IN ('copy', 'continuous', 'standby')),
  state            text NOT NULL DEFAULT 'pairing' CHECK (state IN ('pairing', 'copying', 'in_sync', 'lagging', 'disconnected', 'detaching', 'detached', 'failed', 'lost', 'dropped')),
  token_hash       bytea NOT NULL UNIQUE,
  role_name        text NOT NULL UNIQUE CHECK (role_name ~ '^ctrl32_repl_[a-z0-9]{8,16}$'),
  slot_name        text NOT NULL UNIQUE CHECK (slot_name ~ '^ctrl32_replica_[a-z0-9]{8,16}$'),
  standby_url      text CHECK (standby_url IS NULL OR standby_url ~ '^https?://[^\s]{3,200}$'),
  host_info        jsonb NOT NULL DEFAULT '{}'::jsonb,
  lag_limit_s      integer NOT NULL DEFAULT 10 CHECK (lag_limit_s BETWEEN 1 AND 86400),
  max_retained_mb  integer NOT NULL DEFAULT 10240 CHECK (max_retained_mb BETWEEN 64 AND 10485760),
  copy_total_bytes bigint,
  copy_done_bytes  bigint,
  lag_s            numeric,
  lag_bytes        bigint,
  replay_lsn       text,
  last_contact     timestamptz,
  command          jsonb,
  detach           jsonb,
  created_by       uuid,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE core.replica_pairing (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  code_hash   bytea NOT NULL UNIQUE,
  mode        text NOT NULL CHECK (mode IN ('copy', 'continuous', 'standby')),
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
  lag_limit_s integer NOT NULL DEFAULT 10,
  created_by  uuid NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  expires_at  timestamptz NOT NULL,
  used_at     timestamptz,
  replica_id  uuid REFERENCES core.replica(id)
);

CREATE TABLE core.replica_event (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  replica_id  uuid NOT NULL REFERENCES core.replica(id),
  ts          timestamptz NOT NULL DEFAULT now(),
  kind        text NOT NULL CHECK (kind IN ('state', 'alarm', 'command', 'detach', 'promote', 'security', 'config')),
  detail      jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX ON core.replica_event (replica_id, ts DESC);
SELECT core.make_append_only('core.replica_event');

-- Row count of a table as seen by a snapshot taken on the primary (safe detach, research R6): the replica holds the
-- same row versions with the same xmin, so counting the rows visible in the primary's snapshot gives the primary's
-- count at that instant, even though the replica replayed further. Rows older than ~10⁹ transactions (possibly
-- frozen) are always visible.
CREATE OR REPLACE FUNCTION core.count_in_snapshot(tbl regclass, snap pg_snapshot) RETURNS bigint
LANGUAGE plpgsql STABLE AS $$
DECLARE
  n bigint;
  base bigint := pg_snapshot_xmax(pg_current_snapshot())::text::bigint;
  hi bigint := (pg_snapshot_xmax(pg_current_snapshot())::text::bigint >> 32) << 32;
BEGIN
  EXECUTE format($q$SELECT count(*) FROM %1$s t WHERE age(t.xmin) > 1000000000 OR pg_visible_in_snapshot(
      (CASE WHEN %2$s + t.xmin::text::bigint > %3$s THEN %2$s + t.xmin::text::bigint - 4294967296 ELSE %2$s + t.xmin::text::bigint END)::text::xid8,
      %4$L::pg_snapshot)$q$, tbl, hi, base, snap::text) INTO n;
  RETURN n;
END $$;

-- The append-only record tables (partition parents) whose counts are compared at a safe detach.
CREATE OR REPLACE VIEW core.record_tables AS
  SELECT DISTINCT c.oid::regclass AS tbl, c.relname
  FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid JOIN pg_namespace n ON n.oid = c.relnamespace
  WHERE n.nspname = 'core' AND t.tgname LIKE 'ao_row_%' AND NOT c.relispartition AND c.relname <> 'replica_event';

GRANT SELECT ON core.node, core.replica, core.replica_pairing, core.replica_event, core.record_tables TO ctrl32_app;
GRANT INSERT, UPDATE ON core.replica, core.replica_pairing TO ctrl32_app;
GRANT INSERT ON core.replica_event TO ctrl32_app;
GRANT EXECUTE ON FUNCTION core.count_in_snapshot(regclass, pg_snapshot) TO ctrl32_app;

-- Replica problems (disconnected, lagging, slot lost, failed detach) are incidents like undelivered alarms.
ALTER TABLE core.incident DROP CONSTRAINT IF EXISTS incident_kind_check;
ALTER TABLE core.incident ADD CONSTRAINT incident_kind_check CHECK (kind IN ('alarm_undelivered', 'audit_chain', 'backup_failed', 'manual', 'replica'));

-- ctrl32 telemetry — core schema v1 (phase 0 / PoC)
-- Requires PostgreSQL 18 (uuidv7()) and the pgcrypto extension (digest()).
-- Run as the schema owner (migration role). The application connects as ctrl32_app.
--
-- Principles (ADR-0005, ADR-0007):
--   * append-only for every record table: no UPDATE/DELETE/TRUNCATE for anyone, enforced by triggers
--   * two timestamps on every observation: measured_at (source) and received_at (database)
--   * audit_trail is a hash chain with a gapless sequence and a single-row chain state
--   * app role has no DDL and no UPDATE/DELETE on record tables

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE SCHEMA IF NOT EXISTS core;
SET search_path = core, public;

-- ---------------------------------------------------------------------------
-- Generic guard functions
-- ---------------------------------------------------------------------------

CREATE OR REPLACE FUNCTION core.deny_row_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'ctrl32: % on append-only table %.% is not allowed', TG_OP, TG_TABLE_SCHEMA, TG_TABLE_NAME
    USING ERRCODE = 'integrity_constraint_violation';
END $$;

CREATE OR REPLACE FUNCTION core.deny_truncate() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'ctrl32: TRUNCATE on append-only table %.% is not allowed', TG_TABLE_SCHEMA, TG_TABLE_NAME
    USING ERRCODE = 'integrity_constraint_violation';
END $$;

-- Attach append-only guards to a table (row triggers for UPDATE/DELETE, statement trigger for TRUNCATE).
CREATE OR REPLACE FUNCTION core.make_append_only(tbl regclass) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
  n text := (SELECT relname FROM pg_class WHERE oid = tbl);   -- stable name regardless of search_path
BEGIN
  EXECUTE format('CREATE OR REPLACE TRIGGER %I BEFORE UPDATE OR DELETE ON %s FOR EACH ROW EXECUTE FUNCTION core.deny_row_change()',
                 'ao_row_' || n, tbl);
  EXECUTE format('CREATE OR REPLACE TRIGGER %I BEFORE TRUNCATE ON %s FOR EACH STATEMENT EXECUTE FUNCTION core.deny_truncate()',
                 'ao_stmt_' || n, tbl);
END $$;

-- ---------------------------------------------------------------------------
-- Organization hierarchy (ADR-0004)
-- ---------------------------------------------------------------------------

CREATE TABLE core.organization (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  slug        text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,62}$'),
  name        text NOT NULL,
  locale      text NOT NULL DEFAULT 'en',
  tz          text NOT NULL DEFAULT 'UTC',
  profile     text NOT NULL DEFAULT 'intranet' CHECK (profile IN ('intranet', 'internet')),
  settings    jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE core.customer (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  name        text NOT NULL,
  settings    jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, name)
);

CREATE TABLE core.site (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  customer_id uuid REFERENCES core.customer(id),
  name        text NOT NULL,
  tz          text NOT NULL DEFAULT 'UTC',
  address     text,
  attributes  jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, name)
);

CREATE TABLE core.retention_policy (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id       uuid REFERENCES core.organization(id),          -- NULL = built-in
  name         text NOT NULL,
  record_class text NOT NULL CHECK (record_class IN ('observation', 'raw_message', 'audit', 'alarm', 'report', 'console', 'system_log')),
  years        numeric(5,2) NOT NULL CHECK (years > 0),
  legal_basis  text,
  UNIQUE (org_id, name)
);

CREATE TABLE core.asset (
  id                  uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id              uuid NOT NULL REFERENCES core.organization(id),
  site_id             uuid NOT NULL REFERENCES core.site(id),
  parent_id           uuid REFERENCES core.asset(id),
  type                text NOT NULL,                             -- room, fridge, freezer, machine, meter, building, vehicle, ...
  name                text NOT NULL,
  attributes          jsonb NOT NULL DEFAULT '{}'::jsonb,        -- application-pack attributes (storage class, vaccines flag, ...)
  retention_policy_id uuid REFERENCES core.retention_policy(id),
  created_at          timestamptz NOT NULL DEFAULT now(),
  UNIQUE (site_id, name)
);

-- ---------------------------------------------------------------------------
-- Devices, decoders, sensors, datastreams
-- ---------------------------------------------------------------------------

CREATE TABLE core.decoder (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  name        text NOT NULL,
  version     text NOT NULL,
  kind        text NOT NULL CHECK (kind IN ('builtin', 'js', 'register_map')),
  source      text,                                                -- JS source or YAML register map; NULL for builtin
  hash        text NOT NULL,                                       -- sha256 hex of source (or of builtin identifier@version)
  valid_from  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (name, version)
);
SELECT core.make_append_only('core.decoder');                    -- decoders are versioned, never edited in place

CREATE TABLE core.device_profile (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid REFERENCES core.organization(id),               -- NULL = built-in catalog profile
  name        text NOT NULL,
  transport   text NOT NULL CHECK (transport IN ('mqtt', 'http', 'lorawan', 'opcua', 'modbus')),
  decoder_id  uuid REFERENCES core.decoder(id),
  version     integer NOT NULL DEFAULT 1,
  config      jsonb NOT NULL DEFAULT '{}'::jsonb,                  -- expected keys, units, kinds, intervals, allowed console commands, ...
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, name, version)
);

CREATE TABLE core.device (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  profile_id  uuid REFERENCES core.device_profile(id),
  external_id text NOT NULL,                                       -- DevEUI, MQTT clientId, OPC UA endpoint, Modbus unit address
  name        text NOT NULL,
  transport   text NOT NULL CHECK (transport IN ('mqtt', 'http', 'lorawan', 'opcua', 'modbus')),
  status      jsonb NOT NULL DEFAULT '{}'::jsonb,                  -- last heartbeat, battery, rssi, fw (mutable operational state)
  created_at  timestamptz NOT NULL DEFAULT now(),
  revoked_at  timestamptz,
  UNIQUE (org_id, external_id)
);

CREATE TABLE core.device_credential (
  id               uuid PRIMARY KEY DEFAULT uuidv7(),
  device_id        uuid NOT NULL REFERENCES core.device(id),
  kind             text NOT NULL CHECK (kind IN ('token', 'cert', 'claim')),
  secret_hash      text,                                           -- argon2id/sha256 of token or claim code; never the secret itself
  cert_fingerprint text,
  valid_from       timestamptz NOT NULL DEFAULT now(),
  valid_to         timestamptz,
  revoked_at       timestamptz,
  CHECK ((kind = 'cert' AND cert_fingerprint IS NOT NULL) OR (kind <> 'cert' AND secret_hash IS NOT NULL))
);
CREATE INDEX ON core.device_credential (device_id) WHERE revoked_at IS NULL;

CREATE TABLE core.sensor (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  serial      text NOT NULL,
  type        text NOT NULL,                                       -- pt1000, ntc, ds18b20, sht41, ct_clamp, ...
  model       text,
  notes       text,
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, serial)
);

CREATE TABLE core.datastream (
  id                  uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id              uuid NOT NULL REFERENCES core.organization(id),
  asset_id            uuid NOT NULL REFERENCES core.asset(id),
  key                 text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,31}$'),
  quantity            text NOT NULL,                               -- temperature, humidity, energy_active_import, power_active, state, ...
  unit                text NOT NULL DEFAULT '',
  kind                text NOT NULL CHECK (kind IN ('gauge', 'counter', 'state', 'event')),
  expected_interval   interval,                                     -- NULL = no gap detection
  accuracy            numeric,                                      -- required accuracy (same unit), NULL = not specified
  retention_policy_id uuid REFERENCES core.retention_policy(id),
  attributes          jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at          timestamptz NOT NULL DEFAULT now(),
  UNIQUE (asset_id, key)
);

-- Assignments are history rows: a new row supersedes the previous one (valid_to set by a new INSERT via the app, never by UPDATE).
CREATE TABLE core.assignment (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  kind        text NOT NULL CHECK (kind IN ('sensor_datastream', 'device_sensor', 'device_datastream')),
  from_id     uuid NOT NULL,
  to_id       uuid NOT NULL,
  valid_from  timestamptz NOT NULL DEFAULT now(),
  valid_to    timestamptz,
  changed_by  uuid,
  reason      text NOT NULL,
  CHECK (valid_to IS NULL OR valid_to > valid_from)
);
SELECT core.make_append_only('core.assignment');

-- ---------------------------------------------------------------------------
-- Records: raw messages and observations (partitioned by month, append-only)
-- ---------------------------------------------------------------------------

CREATE TABLE core.raw_message (
  id            uuid NOT NULL DEFAULT uuidv7(),
  org_id        uuid NOT NULL,
  device_id     uuid,
  connector     text NOT NULL,                                     -- mqtt, http, lorawan.chirpstack, opcua, modbus, ...
  direction     text NOT NULL CHECK (direction IN ('in', 'out')),
  received_at   timestamptz NOT NULL DEFAULT now(),                 -- set by the database, never by the client
  topic         text,
  payload       bytea,
  payload_json  jsonb,
  meta          jsonb NOT NULL DEFAULT '{}'::jsonb,                 -- rssi, snr, fcnt, opcua status, modbus exception, ...
  seq           bigint,
  decode_status text NOT NULL DEFAULT 'pending'
                CHECK (decode_status IN ('pending', 'ok', 'error', 'duplicate', 'unauthenticated', 'simulated', 'unknown_key')),
  decode_error  text,
  PRIMARY KEY (received_at, id)
) PARTITION BY RANGE (received_at);
CREATE INDEX ON core.raw_message (device_id, received_at DESC);
CREATE INDEX ON core.raw_message (org_id, received_at DESC);

CREATE TABLE core.observation (
  id                  uuid NOT NULL DEFAULT uuidv7(),
  org_id              uuid NOT NULL,
  datastream_id       uuid NOT NULL,
  measured_at         timestamptz NOT NULL,                         -- source time (device / OPC UA SourceTimestamp / poll time)
  received_at         timestamptz NOT NULL DEFAULT now(),           -- database time
  value               numeric,
  value_json          jsonb,
  quality             text NOT NULL DEFAULT 'ok'
                      CHECK (quality IN ('ok', 'backfilled', 'sensor_fault', 'out_of_range', 'clock_suspect', 'stale', 'simulated')),
  time_source         text NOT NULL DEFAULT 'server' CHECK (time_source IN ('device', 'device_relative', 'server')),
  decoder_version     text,
  calibration_version integer NOT NULL DEFAULT 0,
  series_version      integer NOT NULL DEFAULT 1,
  raw_id              uuid,
  PRIMARY KEY (measured_at, id),
  CHECK (value IS NOT NULL OR value_json IS NOT NULL)
) PARTITION BY RANGE (measured_at);
CREATE INDEX ON core.observation (datastream_id, measured_at DESC);
CREATE INDEX ON core.observation USING brin (measured_at);

-- Month partitions for a range-partitioned table: from the first day of `from_month` for `months` months.
CREATE OR REPLACE FUNCTION core.ensure_month_partitions(tbl regclass, from_month date, months integer)
RETURNS integer LANGUAGE plpgsql AS $$
DECLARE
  i integer; created integer := 0;
  m_start date; m_end date; pname text;
BEGIN
  FOR i IN 0 .. months - 1 LOOP
    m_start := date_trunc('month', from_month)::date + (i || ' months')::interval;
    m_end   := m_start + interval '1 month';
    pname   := replace(tbl::text, '.', '_') || '_' || to_char(m_start, 'YYYYMM');
    IF to_regclass('core.' || pname) IS NULL THEN
      EXECUTE format('CREATE TABLE core.%I PARTITION OF %s FOR VALUES FROM (%L) TO (%L)', pname, tbl, m_start, m_end);
      PERFORM core.make_append_only(('core.' || pname)::regclass);
      created := created + 1;
    END IF;
  END LOOP;
  RETURN created;
END $$;

-- Guard the parent tables too (UPDATE/DELETE on the parent routes to children, but be explicit).
SELECT core.make_append_only('core.raw_message');
SELECT core.make_append_only('core.observation');
SELECT core.ensure_month_partitions('core.raw_message', current_date, 3);
SELECT core.ensure_month_partitions('core.observation', current_date, 3);

CREATE TABLE core.gap (
  id            uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id        uuid NOT NULL,
  datastream_id uuid NOT NULL REFERENCES core.datastream(id),
  gap_from      timestamptz NOT NULL,
  gap_to        timestamptz,                                       -- NULL while open
  detected_at   timestamptz NOT NULL DEFAULT now(),
  closed_at     timestamptz,
  closed_by     text CHECK (closed_by IN ('backfill', 'annotation', 'resumed'))
);
CREATE INDEX ON core.gap (datastream_id, gap_from DESC);

-- ---------------------------------------------------------------------------
-- Identity and access (ADR-0007)
-- ---------------------------------------------------------------------------

CREATE TABLE core.user_account (
  id                   uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id               uuid NOT NULL REFERENCES core.organization(id),
  customer_id          uuid REFERENCES core.customer(id),
  username             text NOT NULL,
  email                text,
  full_name            text NOT NULL,
  password_hash        text NOT NULL,                              -- argon2id encoded string
  totp_secret_enc      bytea,                                       -- AES-GCM, key outside the database
  mfa_enabled          boolean NOT NULL DEFAULT false,
  must_change_password boolean NOT NULL DEFAULT true,
  failed_logins        integer NOT NULL DEFAULT 0,
  locked_until         timestamptz,
  disabled_at          timestamptz,
  created_at           timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, username)
);

CREATE TABLE core.role (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid REFERENCES core.organization(id),               -- NULL = built-in role
  name        text NOT NULL,
  permissions text[] NOT NULL DEFAULT '{}',
  is_admin    boolean NOT NULL DEFAULT false,
  is_qa       boolean NOT NULL DEFAULT false,
  UNIQUE (org_id, name)
);

-- Role grants are an append-only history; the current set = grants without a later revoke.
CREATE TABLE core.user_role_history (
  id            uuid PRIMARY KEY DEFAULT uuidv7(),
  ts            timestamptz NOT NULL DEFAULT now(),
  user_id       uuid NOT NULL REFERENCES core.user_account(id),
  role_id       uuid NOT NULL REFERENCES core.role(id),
  scope_site_id uuid REFERENCES core.site(id),
  event         text NOT NULL CHECK (event IN ('granted', 'revoked')),
  actor_id      uuid,
  reason        text NOT NULL
);
SELECT core.make_append_only('core.user_role_history');

CREATE TABLE core.session (
  id           text PRIMARY KEY,                                   -- opaque random id (hashed by the app before storing)
  user_id      uuid NOT NULL REFERENCES core.user_account(id),
  created_at   timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  expires_at   timestamptz NOT NULL,
  ip           inet,
  user_agent   text,
  mfa_at       timestamptz,
  revoked_at   timestamptz
);

CREATE TABLE core.access_log (
  id        uuid PRIMARY KEY DEFAULT uuidv7(),
  ts        timestamptz NOT NULL DEFAULT now(),
  org_id    uuid,
  user_id   uuid,
  username  text,
  event     text NOT NULL,                                          -- login_ok, login_failed, logout, locked, mfa_failed, token_used, ...
  success   boolean NOT NULL,
  ip        inet,
  user_agent text,
  detail    jsonb NOT NULL DEFAULT '{}'::jsonb
);
SELECT core.make_append_only('core.access_log');
CREATE INDEX ON core.access_log (ts DESC);
CREATE INDEX ON core.access_log (user_id, ts DESC);

CREATE TABLE core.api_token (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id       uuid NOT NULL REFERENCES core.organization(id),
  user_id      uuid NOT NULL REFERENCES core.user_account(id),
  name         text NOT NULL,
  token_hash   text NOT NULL UNIQUE,
  scopes       text[] NOT NULL,
  expires_at   timestamptz,
  last_used_at timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now(),
  revoked_at   timestamptz
);

-- ---------------------------------------------------------------------------
-- Audit trail: gapless sequence + SHA-256 hash chain (ADR-0005)
-- ---------------------------------------------------------------------------

CREATE TABLE core.audit_chain_state (
  singleton  boolean PRIMARY KEY DEFAULT true CHECK (singleton),
  last_seq   bigint NOT NULL,
  last_hash  text NOT NULL
);
INSERT INTO core.audit_chain_state (last_seq, last_hash)
VALUES (0, encode(digest('ctrl32-telemetry audit chain genesis', 'sha256'), 'hex'));

CREATE TABLE core.audit_trail (
  seq         bigint PRIMARY KEY,
  ts          timestamptz NOT NULL DEFAULT now(),
  org_id      uuid,
  actor_type  text NOT NULL CHECK (actor_type IN ('user', 'api_token', 'device', 'system')),
  actor_id    uuid,
  actor_name  text NOT NULL,
  action      text NOT NULL CHECK (action ~ '^[a-z_]+\.[a-z_]+$'),  -- entity.verb, e.g. alarm_rule.update, alarm.acknowledge
  entity_type text NOT NULL,
  entity_id   uuid,
  old_value   jsonb,
  new_value   jsonb,
  reason      text,
  request_id  text,
  prev_hash   text NOT NULL,
  hash        text NOT NULL,
  -- reason is mandatory for GMP-relevant changes/deletions and for alarm handling (Annex 11 cl. 9, draft ch. 8)
  CHECK (split_part(action, '.', 2) NOT IN ('update', 'delete', 'revoke', 'acknowledge', 'suppress', 'deactivate', 'annotate', 'redecode')
         OR (reason IS NOT NULL AND length(btrim(reason)) >= 3))
);
SELECT core.make_append_only('core.audit_trail');

-- The chain state row may be updated only from inside the chain trigger, which sets a transaction-local flag.
-- DELETE/TRUNCATE are never allowed. (Defence against owner mistakes; tampering is detected by verify_audit_chain.)
CREATE OR REPLACE FUNCTION core.guard_chain_state() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'UPDATE' AND current_setting('ctrl32.chain_update', true) = 'on' THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'ctrl32: % on core.audit_chain_state is only allowed through the audit chain trigger', TG_OP
    USING ERRCODE = 'integrity_constraint_violation';
END $$;
CREATE TRIGGER guard_row_audit_chain_state BEFORE UPDATE OR DELETE ON core.audit_chain_state
  FOR EACH ROW EXECUTE FUNCTION core.guard_chain_state();
CREATE TRIGGER guard_stmt_audit_chain_state BEFORE TRUNCATE ON core.audit_chain_state
  FOR EACH STATEMENT EXECUTE FUNCTION core.deny_truncate();
CREATE INDEX ON core.audit_trail (ts DESC);
CREATE INDEX ON core.audit_trail (entity_type, entity_id, ts DESC);
CREATE INDEX ON core.audit_trail (actor_id, ts DESC);

-- Canonical content for hashing: every column except prev_hash/hash/seq, as jsonb text (jsonb key order is canonical).
CREATE OR REPLACE FUNCTION core.audit_canonical(r core.audit_trail) RETURNS text
LANGUAGE sql IMMUTABLE AS $$
  SELECT (to_jsonb(r) - 'prev_hash' - 'hash' - 'seq')::text;
$$;

CREATE OR REPLACE FUNCTION core.audit_chain_before_insert() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = core, public AS $$
DECLARE
  st core.audit_chain_state%ROWTYPE;
BEGIN
  -- Serialize chain appends: lock the single state row.
  SELECT * INTO st FROM core.audit_chain_state WHERE singleton FOR UPDATE;
  NEW.seq       := st.last_seq + 1;
  NEW.prev_hash := st.last_hash;
  IF NEW.ts IS NULL THEN NEW.ts := now(); END IF;
  NEW.hash := encode(digest(NEW.prev_hash || '|' || NEW.seq::text || '|' || core.audit_canonical(NEW), 'sha256'), 'hex');
  -- Advance the chain head; the guard trigger accepts the UPDATE only while this transaction-local flag is on.
  PERFORM set_config('ctrl32.chain_update', 'on', true);
  UPDATE core.audit_chain_state SET last_seq = NEW.seq, last_hash = NEW.hash WHERE singleton;
  PERFORM set_config('ctrl32.chain_update', 'off', true);
  RETURN NEW;
END $$;

CREATE TRIGGER audit_chain BEFORE INSERT ON core.audit_trail
  FOR EACH ROW EXECUTE FUNCTION core.audit_chain_before_insert();

-- Re-walk the chain and report the first broken link. Used by `ctrl32-telemetry audit verify` and by IQ/OQ tests.
CREATE OR REPLACE FUNCTION core.verify_audit_chain(from_seq bigint DEFAULT 1, to_seq bigint DEFAULT NULL)
RETURNS TABLE (ok boolean, checked bigint, first_bad_seq bigint, head_seq bigint, head_hash text)
LANGUAGE plpgsql STABLE AS $$
DECLARE
  r core.audit_trail%ROWTYPE;
  expected_prev text;
  n bigint := 0;
BEGIN
  IF from_seq = 1 THEN
    expected_prev := encode(digest('ctrl32-telemetry audit chain genesis', 'sha256'), 'hex');
  ELSE
    SELECT a.hash INTO expected_prev FROM core.audit_trail a WHERE a.seq = from_seq - 1;
    IF expected_prev IS NULL THEN
      RETURN QUERY SELECT false, 0::bigint, from_seq, NULL::bigint, NULL::text; RETURN;
    END IF;
  END IF;
  FOR r IN SELECT * FROM core.audit_trail a WHERE a.seq >= from_seq AND (to_seq IS NULL OR a.seq <= to_seq) ORDER BY a.seq LOOP
    IF r.prev_hash <> expected_prev
       OR r.hash <> encode(digest(r.prev_hash || '|' || r.seq::text || '|' || core.audit_canonical(r), 'sha256'), 'hex') THEN
      RETURN QUERY SELECT false, n, r.seq, r.seq, r.hash; RETURN;
    END IF;
    expected_prev := r.hash; n := n + 1;
  END LOOP;
  RETURN QUERY SELECT true, n, NULL::bigint, s.last_seq, s.last_hash FROM core.audit_chain_state s;
END $$;

-- Signed checkpoints (signature produced by the application with an Ed25519 key kept outside the database).
CREATE TABLE core.audit_checkpoint (
  id         uuid PRIMARY KEY DEFAULT uuidv7(),
  ts         timestamptz NOT NULL DEFAULT now(),
  last_seq   bigint NOT NULL,
  head_hash  text NOT NULL,
  key_id     text,
  signature  bytea,
  anchored   jsonb NOT NULL DEFAULT '{}'::jsonb                    -- where the hash was published: report id, email, TSA token, ...
);
SELECT core.make_append_only('core.audit_checkpoint');

-- ---------------------------------------------------------------------------
-- Alarms (P-17, draft Annex 11 ch. 8)
-- ---------------------------------------------------------------------------

CREATE TABLE core.escalation_policy (
  id      uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id  uuid NOT NULL REFERENCES core.organization(id),
  name    text NOT NULL,
  steps   jsonb NOT NULL,                                           -- [{after_s, channel, recipients}], validated by the app
  version integer NOT NULL DEFAULT 1,
  UNIQUE (org_id, name, version)
);

-- Alarm rules are versioned rows (new version = new row with valid_from; old row gets valid_to via app-managed supersede INSERT).
CREATE TABLE core.alarm_rule (
  id                   uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id               uuid NOT NULL REFERENCES core.organization(id),
  datastream_id        uuid REFERENCES core.datastream(id),
  asset_id             uuid REFERENCES core.asset(id),
  device_id            uuid REFERENCES core.device(id),
  type                 text NOT NULL CHECK (type IN ('high', 'low', 'rate', 'sensor_fault', 'comm_loss', 'low_battery', 'power_loss', 'door_open', 'state', 'calibration_due', 'custom')),
  warning              numeric,
  action               numeric,
  delay                interval NOT NULL DEFAULT '0',
  hysteresis           numeric NOT NULL DEFAULT 0,
  escalation_policy_id uuid REFERENCES core.escalation_policy(id),
  enabled              boolean NOT NULL DEFAULT true,
  version              integer NOT NULL DEFAULT 1,
  valid_from           timestamptz NOT NULL DEFAULT now(),
  valid_to             timestamptz,
  changed_by           uuid,
  reason               text NOT NULL,
  CHECK (datastream_id IS NOT NULL OR asset_id IS NOT NULL OR device_id IS NOT NULL)
);
SELECT core.make_append_only('core.alarm_rule');

CREATE TABLE core.alarm (
  id            uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id        uuid NOT NULL,
  rule_id       uuid NOT NULL REFERENCES core.alarm_rule(id),
  datastream_id uuid,
  device_id     uuid,
  raised_at     timestamptz NOT NULL DEFAULT now(),
  cleared_at    timestamptz,
  state         text NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'acknowledged', 'suppressed', 'cleared')),
  severity      text NOT NULL CHECK (severity IN ('warning', 'action', 'critical')),
  retrospective boolean NOT NULL DEFAULT false,
  value         numeric,
  detail        jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX ON core.alarm (org_id, state, raised_at DESC);

CREATE TABLE core.alarm_event (
  id        uuid PRIMARY KEY DEFAULT uuidv7(),
  ts        timestamptz NOT NULL DEFAULT now(),
  alarm_id  uuid NOT NULL REFERENCES core.alarm(id),
  event     text NOT NULL CHECK (event IN ('raised', 'notified', 'delivered', 'delivery_failed', 'acknowledged', 'escalated', 'suppressed', 'unsuppressed', 'cleared', 'retrospective')),
  actor_id  uuid,
  actor_name text,
  reason    text,
  detail    jsonb NOT NULL DEFAULT '{}'::jsonb,
  CHECK (event NOT IN ('acknowledged', 'suppressed') OR (reason IS NOT NULL AND actor_id IS NOT NULL))
);
SELECT core.make_append_only('core.alarm_event');
CREATE INDEX ON core.alarm_event (alarm_id, ts);

-- ---------------------------------------------------------------------------
-- Configuration baseline and system log
-- ---------------------------------------------------------------------------

CREATE TABLE core.config_change (
  id        uuid PRIMARY KEY DEFAULT uuidv7(),
  ts        timestamptz NOT NULL DEFAULT now(),
  actor_id  uuid,
  actor_name text NOT NULL,
  key       text NOT NULL,
  old_value jsonb,
  new_value jsonb,
  reason    text NOT NULL
);
SELECT core.make_append_only('core.config_change');

CREATE TABLE core.system_log (
  id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  ts       timestamptz NOT NULL DEFAULT now(),
  level    text NOT NULL CHECK (level IN ('debug', 'info', 'warn', 'error')),
  logger   text NOT NULL,
  msg      text NOT NULL,
  fields   jsonb NOT NULL DEFAULT '{}'::jsonb,
  security boolean NOT NULL DEFAULT false,
  request_id text
);
CREATE INDEX ON core.system_log (ts DESC);
CREATE INDEX ON core.system_log (ts DESC) WHERE security;

-- ---------------------------------------------------------------------------
-- Application role privileges (least privilege; ADR-0007)
-- ---------------------------------------------------------------------------

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'ctrl32_app') THEN
    CREATE ROLE ctrl32_app LOGIN;
  END IF;
END $$;

GRANT USAGE ON SCHEMA core TO ctrl32_app;
GRANT SELECT, INSERT ON ALL TABLES IN SCHEMA core TO ctrl32_app;
-- Mutable operational tables only:
GRANT UPDATE ON core.organization, core.customer, core.site, core.asset, core.datastream, core.device, core.device_credential,
                core.sensor, core.device_profile, core.retention_policy, core.gap, core.user_account, core.role, core.session,
                core.api_token, core.alarm, core.escalation_policy TO ctrl32_app;
GRANT DELETE ON core.session, core.system_log TO ctrl32_app;        -- session cleanup and log rotation only
GRANT USAGE ON ALL SEQUENCES IN SCHEMA core TO ctrl32_app;
GRANT EXECUTE ON FUNCTION core.verify_audit_chain(bigint, bigint) TO ctrl32_app;
GRANT EXECUTE ON FUNCTION core.ensure_month_partitions(regclass, date, integer) TO ctrl32_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA core GRANT SELECT, INSERT ON TABLES TO ctrl32_app;   -- new month partitions

COMMIT;

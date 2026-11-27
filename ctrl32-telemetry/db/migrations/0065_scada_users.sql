-- SCADA users (spec 041, ADR-0026 step 3): a second level of identity that exists only at the controls. A person signs
-- in inside a picture or a dashboard, on top of the session of the station, with a PIN or a password. A SCADA user is
-- not an account: it cannot sign in to the application and has no access to the API.

-- Groups as in WinCC. The key is what controls name (it never changes); the name is what people read.
CREATE TABLE core.scada_group (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id) ON DELETE CASCADE,
  key         text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,31}$'),
  name        text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 60),
  description text NOT NULL DEFAULT '',
  approves    boolean NOT NULL DEFAULT false,   -- its members may approve as the second person (four eyes)
  position    int NOT NULL DEFAULT 0,
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, key)
);

CREATE TABLE core.scada_user (
  id            uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id        uuid NOT NULL REFERENCES core.organization(id) ON DELETE CASCADE,
  name          text NOT NULL CHECK (name ~ '^[a-z0-9][a-z0-9._-]{0,31}$'),   -- what the person types
  full_name     text NOT NULL DEFAULT '',
  secret_kind   text NOT NULL CHECK (secret_kind IN ('pin', 'password')),
  secret_hash   text NOT NULL,
  enabled       boolean NOT NULL DEFAULT true,
  failed_count  int NOT NULL DEFAULT 0,
  locked_until  timestamptz,
  last_used_at  timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, name)
);

CREATE TABLE core.scada_user_group (
  user_id  uuid NOT NULL REFERENCES core.scada_user(id) ON DELETE CASCADE,
  group_id uuid NOT NULL REFERENCES core.scada_group(id) ON DELETE CASCADE,
  PRIMARY KEY (user_id, group_id)
);

-- Who is signed in at which station. The station is the browser session of the account (hashed id); a SCADA sign-in
-- ends with it, after the time without activity of the policy, or when the person signs out.
CREATE TABLE core.scada_session (
  id            uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id        uuid NOT NULL REFERENCES core.organization(id) ON DELETE CASCADE,
  scada_user_id uuid NOT NULL REFERENCES core.scada_user(id) ON DELETE CASCADE,
  name          text NOT NULL,
  station       text NOT NULL,
  station_user  text NOT NULL DEFAULT '',
  station_ip    text NOT NULL DEFAULT '',
  started_at    timestamptz NOT NULL DEFAULT now(),
  last_seen_at  timestamptz NOT NULL DEFAULT now(),
  ended_at      timestamptz,
  ended_why     text NOT NULL DEFAULT ''          -- signed_out, idle, replaced, removed
);
CREATE INDEX scada_session_station_idx ON core.scada_session (station) WHERE ended_at IS NULL;

GRANT SELECT, INSERT, UPDATE, DELETE ON core.scada_group, core.scada_user, core.scada_user_group TO ctrl32_app;
GRANT SELECT, INSERT, UPDATE ON core.scada_session TO ctrl32_app;

-- The default groups of an organization. The administrator renames, adds and removes them.
CREATE OR REPLACE FUNCTION core.seed_scada_groups(org uuid) RETURNS void
LANGUAGE sql AS $$
  INSERT INTO core.scada_group (org_id, key, name, description, approves, position) VALUES
    (org, 'operator',     'Operator',     'Runs the plant from the pictures', false, 1),
    (org, 'shift_leader', 'Shift leader', 'Leads the shift; approves as the second person', true, 2),
    (org, 'technologist', 'Technologist', 'Changes setpoints and recipes', false, 3),
    (org, 'maintenance',  'Maintenance',  'Service and manual operation', false, 4)
  ON CONFLICT (org_id, key) DO NOTHING;
$$;
SELECT core.seed_scada_groups(id) FROM core.organization;

CREATE OR REPLACE FUNCTION core.seed_scada_groups_trg() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM core.seed_scada_groups(NEW.id);
  RETURN NEW;
END $$;
CREATE OR REPLACE TRIGGER organization_scada_groups AFTER INSERT ON core.organization
  FOR EACH ROW EXECUTE FUNCTION core.seed_scada_groups_trg();

-- The SCADA user is recorded next to the account of the station: in the commands, the recipe loads, the change
-- requests and the time-limited approvals. Empty when nobody was signed in at the controls.
ALTER TABLE core.device_command      ADD COLUMN IF NOT EXISTS operator text NOT NULL DEFAULT '';
ALTER TABLE core.recipe_download     ADD COLUMN IF NOT EXISTS operator text NOT NULL DEFAULT '';
ALTER TABLE core.change_request      ADD COLUMN IF NOT EXISTS operator text NOT NULL DEFAULT '';
ALTER TABLE core.second_person_grant ADD COLUMN IF NOT EXISTS operator text NOT NULL DEFAULT '';

-- Retention of the operator action log (ADR-0026). The table is append-only for everyone; this function is the one
-- sanctioned way to drop old rows: it never deletes anything younger than 30 days, whatever it is asked for.
CREATE OR REPLACE FUNCTION core.deny_operator_action_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' AND current_setting('ctrl32.oplog_retention', true) = 'on' AND OLD.ts < now() - interval '30 days' THEN
    RETURN OLD;
  END IF;
  RAISE EXCEPTION 'ctrl32: % on append-only table %.% is not allowed', TG_OP, TG_TABLE_SCHEMA, TG_TABLE_NAME
    USING ERRCODE = 'integrity_constraint_violation';
END $$;
CREATE OR REPLACE TRIGGER ao_row_operator_action BEFORE UPDATE OR DELETE ON core.operator_action
  FOR EACH ROW EXECUTE FUNCTION core.deny_operator_action_change();

CREATE OR REPLACE FUNCTION core.purge_operator_actions(org uuid, keep_days int) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path = core, pg_temp AS $$
DECLARE
  n bigint;
BEGIN
  IF keep_days IS NULL OR keep_days < 30 THEN
    RAISE EXCEPTION 'ctrl32: the operator log is kept for at least 30 days';
  END IF;
  PERFORM set_config('ctrl32.oplog_retention', 'on', true);
  DELETE FROM core.operator_action WHERE org_id = org AND ts < now() - make_interval(days => keep_days);
  GET DIAGNOSTICS n = ROW_COUNT;
  PERFORM set_config('ctrl32.oplog_retention', 'off', true);
  RETURN n;
END $$;
REVOKE ALL ON FUNCTION core.purge_operator_actions(uuid, int) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION core.purge_operator_actions(uuid, int) TO ctrl32_app;

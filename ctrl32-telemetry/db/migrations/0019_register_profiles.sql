-- Versioned register profiles (URS A-11): a register map shared by Modbus connectors of the same device type.
-- A change is a new version (new row, previous row closed with valid_to); connectors keep the version they were
-- given (settings.profile_id) until a new version is applied to them, which is an audited connector update.
CREATE TABLE core.register_profile (
  id              uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id          uuid NOT NULL REFERENCES core.organization(id),
  lineage         uuid NOT NULL DEFAULT uuidv7(),                 -- same across the versions of one profile
  name            text NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
  description     text NOT NULL DEFAULT '',
  kind            text NOT NULL DEFAULT 'modbus' CHECK (kind IN ('modbus')),
  version         integer NOT NULL DEFAULT 1,
  spec            jsonb NOT NULL,                                  -- {byte_order, registers:[{key, table, address, type, scale, offset, writable, min, max}]}
  changelog       text NOT NULL DEFAULT '',
  created_by_name text NOT NULL DEFAULT '',
  reason          text NOT NULL,
  valid_from      timestamptz NOT NULL DEFAULT now(),
  valid_to        timestamptz,
  retired_at      timestamptz
);
CREATE UNIQUE INDEX register_profile_current ON core.register_profile (org_id, name) WHERE valid_to IS NULL AND retired_at IS NULL;
CREATE INDEX ON core.register_profile (lineage, version);
GRANT SELECT, INSERT, UPDATE ON core.register_profile TO ctrl32_app;

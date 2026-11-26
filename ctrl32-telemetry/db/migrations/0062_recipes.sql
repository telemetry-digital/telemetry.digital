-- Recipes (spec 038, 0.92.0). A recipe is a list of elements (one writable key of a device each, with its limits), the
-- units it can be loaded into (a device, and a key prefix for several units behind one PLC) and data records: named
-- sets of values. Loading a record into a unit writes its values through the same path as any command (connector
-- limits, permission device.command, four eyes, audit trail) and leaves a load record: who loaded which values where,
-- and what the controller answered for every value.

CREATE TABLE core.recipe (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  description text NOT NULL DEFAULT '' CHECK (length(description) <= 500),
  elements    jsonb NOT NULL DEFAULT '[]'::jsonb,   -- [{key, label, unit, type, min, max, decimals}] in the order of writing
  targets     jsonb NOT NULL DEFAULT '[]'::jsonb,   -- [{id, name, device_id, prefix}]
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  updated_by  text NOT NULL DEFAULT '',
  deleted_at  timestamptz
);
CREATE UNIQUE INDEX recipe_name_idx ON core.recipe (org_id, lower(name)) WHERE deleted_at IS NULL;

CREATE TABLE core.recipe_record (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  recipe_id   uuid NOT NULL REFERENCES core.recipe(id),
  number      int NOT NULL CHECK (number BETWEEN 1 AND 99999),
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  "values"    jsonb NOT NULL DEFAULT '{}'::jsonb,   -- {element key: value}
  note        text NOT NULL DEFAULT '' CHECK (length(note) <= 500),
  version     int NOT NULL DEFAULT 1,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  updated_by  text NOT NULL DEFAULT '',
  deleted_at  timestamptz
);
CREATE UNIQUE INDEX recipe_record_number_idx ON core.recipe_record (recipe_id, number) WHERE deleted_at IS NULL;

-- A load of a record into a unit. The values are a copy taken when the load was asked for: what a second user approves
-- is exactly what is written, whatever happens to the record in between.
CREATE TABLE core.recipe_download (
  id                uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id            uuid NOT NULL REFERENCES core.organization(id),
  recipe_id         uuid NOT NULL REFERENCES core.recipe(id),
  record_id         uuid NOT NULL REFERENCES core.recipe_record(id),
  recipe_name       text NOT NULL,
  record_name       text NOT NULL,
  record_number     int NOT NULL,
  record_version    int NOT NULL,
  device_id         uuid NOT NULL REFERENCES core.device(id),
  target_name       text NOT NULL,
  prefix            text NOT NULL DEFAULT '',
  items             jsonb NOT NULL,                  -- [{key, label, unit, value}] in the order of writing
  results           jsonb NOT NULL DEFAULT '[]'::jsonb, -- [{key, status, readback, error, command_id}]
  status            text NOT NULL CHECK (status IN ('pending', 'running', 'done', 'failed', 'rejected', 'expired')),
  error             text NOT NULL DEFAULT '',
  requested_by      uuid,
  requested_by_name text NOT NULL,
  requested_at      timestamptz NOT NULL DEFAULT now(),
  reason            text NOT NULL CHECK (length(btrim(reason)) >= 1),
  decided_by_name   text,
  decided_at        timestamptz,
  decide_reason     text,
  finished_at       timestamptz
);
CREATE INDEX recipe_download_org_idx ON core.recipe_download (org_id, requested_at DESC);
CREATE INDEX recipe_download_recipe_idx ON core.recipe_download (recipe_id, requested_at DESC);

GRANT SELECT, INSERT, UPDATE ON core.recipe, core.recipe_record, core.recipe_download TO ctrl32_app;

-- Changing recipe data is its own permission: a technologist changes records, an operator only loads them
-- (device.command), and the structure of a recipe is configuration (config.write).
INSERT INTO core.permission (name, description) VALUES
  ('recipe.edit', 'Create, change and delete recipe data records')
ON CONFLICT (name) DO NOTHING;
UPDATE core.role SET permissions = array_append(permissions, 'recipe.edit')
  WHERE org_id IS NULL AND name IN ('engineer', 'org_admin') AND NOT ('recipe.edit' = ANY (permissions));

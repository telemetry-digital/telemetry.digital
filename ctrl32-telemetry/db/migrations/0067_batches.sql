-- Batch record (spec 043, 0.97.0): when a batch ran on a production unit, what was loaded for it and how its values
-- went. A unit says how its batches begin and end: with the load of a recipe record, or with a tag of the controller.

CREATE TABLE core.batch_unit (
  id             uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id         uuid NOT NULL REFERENCES core.organization(id) ON DELETE CASCADE,
  name           text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
  mode           text NOT NULL CHECK (mode IN ('recipe', 'tag')),
  -- mode recipe: the unit of a recipe — a device and the key prefix of the unit behind it
  device_id      uuid REFERENCES core.device(id),
  prefix         text NOT NULL DEFAULT '',
  -- mode tag: a datastream of the controller. run: not zero while a batch runs. number: the number of the batch;
  -- a new number begins a batch, zero means none.
  tag_datastream uuid REFERENCES core.datastream(id),
  tag_kind       text NOT NULL DEFAULT 'run' CHECK (tag_kind IN ('run', 'number')),
  curves         jsonb NOT NULL DEFAULT '[]'::jsonb,   -- datastream ids drawn in the trend of a batch
  enabled        boolean NOT NULL DEFAULT true,
  tag_seen_at    timestamptz,                          -- the last sample of the tag that was looked at
  tag_last       double precision,
  next_number    int NOT NULL DEFAULT 1,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  removed_at     timestamptz,
  CHECK ((mode = 'recipe' AND device_id IS NOT NULL) OR (mode = 'tag' AND tag_datastream IS NOT NULL))
);
CREATE UNIQUE INDEX batch_unit_name_idx ON core.batch_unit (org_id, lower(name)) WHERE removed_at IS NULL;

-- A batch is a record: it is never deleted. One batch runs on a unit at a time.
CREATE TABLE core.batch (
  id                 uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id             uuid NOT NULL REFERENCES core.organization(id),
  unit_id            uuid NOT NULL REFERENCES core.batch_unit(id),
  unit_name          text NOT NULL,
  number             int NOT NULL,                     -- counted per unit
  code               text NOT NULL DEFAULT '',         -- the number from the controller, the recipe record, or typed
  status             text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'finished')),
  started_at         timestamptz NOT NULL,
  ended_at           timestamptz,
  start_source       text NOT NULL CHECK (start_source IN ('recipe', 'tag', 'manual')),
  end_source         text NOT NULL DEFAULT '' CHECK (end_source IN ('', 'recipe', 'tag', 'manual')),
  recipe_download_id uuid,
  recipe_name        text NOT NULL DEFAULT '',
  record_name        text NOT NULL DEFAULT '',
  record_number      int,
  started_by         text NOT NULL DEFAULT '',         -- the account, empty when the controller began it
  operator           text NOT NULL DEFAULT '',         -- the SCADA user at the controls
  ended_by           text NOT NULL DEFAULT '',
  reason             text NOT NULL DEFAULT '',         -- of a batch begun by hand
  end_reason         text NOT NULL DEFAULT '',
  curves             jsonb NOT NULL DEFAULT '[]'::jsonb,   -- the curves of the unit when the batch ended
  stats              jsonb,                            -- per curve: n, min, max, avg, first, last (kept when it ended)
  CHECK (ended_at IS NULL OR ended_at >= started_at),
  UNIQUE (unit_id, number)
);
CREATE UNIQUE INDEX batch_running_idx ON core.batch (unit_id) WHERE status = 'running';
CREATE INDEX batch_org_started_idx ON core.batch (org_id, started_at DESC);

GRANT SELECT, INSERT, UPDATE ON core.batch_unit, core.batch TO ctrl32_app;

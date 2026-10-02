-- Object counting (spec 029, 0.71.0): people and vehicles counted at entrances of an area (a building, a car park, a
-- zone), from the server's own detector on camera pictures or from sensors (light barriers, turnstiles, door counters)
-- through the device protocol. The counts themselves are ordinary observations of datastreams on assets (an asset per
-- area, entrance and speed section), so dashboards, period tables, alarm rules, flows and exports work on them; these
-- tables hold only the configuration and the running occupancy. No pictures are stored, only numbers.
CREATE TABLE core.counting_area (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  site_id     uuid NOT NULL REFERENCES core.site(id),
  asset_id    uuid NOT NULL UNIQUE REFERENCES core.asset(id),
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  kind        text NOT NULL DEFAULT 'building' CHECK (kind IN ('building', 'car_park', 'zone')),
  -- what is counted: people, vehicles (car + motorcycle + bus + truck), and single classes car, truck, bus,
  -- motorcycle, bicycle; each has datastreams <group>_in, <group>_out and <group>_occupancy
  groups      text[] NOT NULL DEFAULT '{people}',
  reset_time  text CHECK (reset_time ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'), -- local daily reset of the occupancy, NULL = never
  limits      jsonb NOT NULL DEFAULT '{}'::jsonb,                          -- group -> capacity (shown; alarms are alarm rules)
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz
);
CREATE UNIQUE INDEX counting_area_name ON core.counting_area (org_id, lower(name)) WHERE archived_at IS NULL;

CREATE TABLE core.counting_entrance (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  area_id     uuid NOT NULL REFERENCES core.counting_area(id),
  asset_id    uuid NOT NULL UNIQUE REFERENCES core.asset(id),
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  sort        integer NOT NULL DEFAULT 0,
  created_at  timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz
);
CREATE UNIQUE INDEX counting_entrance_name ON core.counting_entrance (area_id, lower(name)) WHERE archived_at IS NULL;

-- the running occupancy of an area per group (in − out since the last reset, never below zero)
CREATE TABLE core.counting_occupancy (
  area_id       uuid NOT NULL REFERENCES core.counting_area(id),
  grp           text NOT NULL,
  occupancy     bigint NOT NULL DEFAULT 0 CHECK (occupancy >= 0),
  clamped       bigint NOT NULL DEFAULT 0,          -- departures counted while the area seemed empty, since the reset
  last_reset_at timestamptz,
  updated_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (area_id, grp)
);

-- object counting on a camera: lines (each counts for an entrance), speed sections, analysis settings; off by default
CREATE TABLE core.counting_camera (
  camera_id  uuid PRIMARY KEY REFERENCES core.camera(id),
  org_id     uuid NOT NULL REFERENCES core.organization(id),
  enabled    boolean NOT NULL DEFAULT false,
  fps        numeric(4,1) NOT NULL DEFAULT 5 CHECK (fps BETWEEN 1 AND 15),
  model      text NOT NULL DEFAULT 'nano' CHECK (model IN ('nano', 'tiny')),
  min_score  numeric(3,2) NOT NULL DEFAULT 0.35 CHECK (min_score BETWEEN 0.1 AND 0.95),
  config     jsonb NOT NULL DEFAULT '{"lines": []}'::jsonb, -- lines (with entrance_id), sections (with asset_id, limit_kmh), anchor
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- sensors: datastreams of devices (light barrier, turnstile, door counter …) that report people or vehicles passing
CREATE TABLE core.counting_input (
  id                uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id            uuid NOT NULL REFERENCES core.organization(id),
  entrance_id       uuid NOT NULL REFERENCES core.counting_entrance(id),
  name              text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  grp               text NOT NULL DEFAULT 'people',
  in_datastream_id  uuid REFERENCES core.datastream(id),
  out_datastream_id uuid REFERENCES core.datastream(id),
  mode              text NOT NULL DEFAULT 'increment' CHECK (mode IN ('increment', 'total')), -- a value per passage, or a running total
  enabled           boolean NOT NULL DEFAULT true,
  last_values       jsonb NOT NULL DEFAULT '{}'::jsonb,  -- total mode: the previous reading per datastream
  created_at        timestamptz NOT NULL DEFAULT now(),
  archived_at       timestamptz,
  CHECK (in_datastream_id IS NOT NULL OR out_datastream_id IS NOT NULL)
);

-- resets, corrections and clamps of the occupancy (append-only, with the reason of a correction)
CREATE TABLE core.counting_log (
  id       uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id   uuid NOT NULL,
  area_id  uuid NOT NULL REFERENCES core.counting_area(id),
  grp      text NOT NULL,
  at       timestamptz NOT NULL DEFAULT now(),
  kind     text NOT NULL CHECK (kind IN ('reset', 'correction', 'clamp')),
  before   bigint NOT NULL,
  after    bigint NOT NULL,
  reason   text,
  actor    text NOT NULL
);
SELECT core.make_append_only('core.counting_log');
CREATE INDEX ON core.counting_log (area_id, at DESC);

GRANT SELECT, INSERT, UPDATE ON core.counting_area, core.counting_entrance, core.counting_occupancy, core.counting_camera, core.counting_input TO ctrl32_app;
GRANT SELECT, INSERT ON core.counting_log TO ctrl32_app;

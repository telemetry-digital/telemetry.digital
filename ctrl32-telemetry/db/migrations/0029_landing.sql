-- The public front page (spec 012): what visitors see before signing in, built from blocks in a graphical editor.
-- Server-wide (one page per server). Every save is a new version; one version is published at a time (or none: the
-- built-in page). Images are stored in the database; SVG only after sanitising.
CREATE TABLE core.landing_version (
  version     serial PRIMARY KEY,
  spec        jsonb NOT NULL,
  reason      text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  created_by  text NOT NULL
);
CREATE TABLE core.landing_state (
  id                 integer PRIMARY KEY CHECK (id = 1),
  published_version  integer REFERENCES core.landing_version(version),
  published_at       timestamptz,
  published_by       text
);
INSERT INTO core.landing_state (id) VALUES (1);
CREATE TABLE core.landing_asset (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  name        text NOT NULL CHECK (length(name) <= 120),
  mime        text NOT NULL CHECK (mime IN ('image/png', 'image/jpeg', 'image/webp', 'image/svg+xml')),
  data        bytea NOT NULL,
  bytes       integer NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  created_by  text NOT NULL
);
GRANT SELECT, INSERT ON core.landing_version, core.landing_asset TO ctrl32_app;
GRANT SELECT, UPDATE ON core.landing_state TO ctrl32_app;
GRANT USAGE ON SEQUENCE core.landing_version_version_seq TO ctrl32_app;

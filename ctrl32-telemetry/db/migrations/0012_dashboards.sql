-- Dashboards (URS J-02): versioned content items of the organization (ADR-0010). Every save is a new row that
-- supersedes the current one (valid_to); slug is the stable identity across versions.
CREATE TABLE core.dashboard (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  slug        text NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
  name        text NOT NULL,
  version     integer NOT NULL DEFAULT 1,
  spec        jsonb NOT NULL,
  checksum    text NOT NULL,
  is_default  boolean NOT NULL DEFAULT false,
  created_by  uuid,
  created_at  timestamptz NOT NULL DEFAULT now(),
  valid_to    timestamptz,
  reason      text NOT NULL DEFAULT 'initial'
);
CREATE INDEX dashboard_current ON core.dashboard (org_id, slug) WHERE valid_to IS NULL;
GRANT SELECT, INSERT, UPDATE ON core.dashboard TO ctrl32_app;

-- Device position (URS J-19): {lat, lon, source: fixed|reported, updated_at}; fixed positions are set in the UI, reported ones
-- arrive with telemetry (lat/lon) or the status heartbeat and never overwrite a fixed one.
ALTER TABLE core.device ADD COLUMN location jsonb;

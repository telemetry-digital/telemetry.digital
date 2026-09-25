-- Public read-only links for dashboards (URS J-02): one active token per dashboard slug. The token exposes only the
-- data the dashboard's widgets bind (datastreams, devices, their alarms). Revoked rows stay for the audit trail.
CREATE TABLE core.dashboard_share (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  slug        text NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
  token       text NOT NULL UNIQUE CHECK (length(token) >= 32),
  created_by  uuid,
  created_at  timestamptz NOT NULL DEFAULT now(),
  revoked_at  timestamptz,
  reason      text NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX dashboard_share_active ON core.dashboard_share (org_id, slug) WHERE revoked_at IS NULL;
GRANT SELECT, INSERT, UPDATE ON core.dashboard_share TO ctrl32_app;

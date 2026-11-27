-- Trends (spec 042, 0.96.0): saved trend views — which curves a person looks at together, their colours, axes and time
-- window. A view is shared with the organization (changed with content.write and a reason, audited) or personal (only
-- its owner sees and changes it).
CREATE TABLE core.trend_view (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id) ON DELETE CASCADE,
  name        text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
  shared      boolean NOT NULL DEFAULT false,
  owner_id    uuid,
  owner_name  text NOT NULL DEFAULT '',
  spec        jsonb NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  updated_by  text NOT NULL DEFAULT ''
);
CREATE INDEX trend_view_org_idx ON core.trend_view (org_id, name);
GRANT SELECT, INSERT, UPDATE, DELETE ON core.trend_view TO ctrl32_app;

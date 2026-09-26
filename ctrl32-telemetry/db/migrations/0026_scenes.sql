-- Energy and scenes (spec 008).
-- A scene sets several entities at once ("Evening": lights at 30 %, blinds down). Activating it issues one device
-- command per action through the normal command path (permissions, four eyes, audit). Energy settings (price, sources)
-- live in organization.settings.energy.
CREATE TABLE core.scene (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  icon        text NOT NULL DEFAULT '' CHECK (length(icon) <= 40),
  room_id     uuid REFERENCES core.asset(id),
  actions     jsonb NOT NULL DEFAULT '[]'::jsonb,    -- [{entity_id, service, params}]
  sort        integer NOT NULL DEFAULT 0,
  created_at  timestamptz NOT NULL DEFAULT now(),
  created_by  text NOT NULL,
  updated_at  timestamptz NOT NULL DEFAULT now(),
  removed_at  timestamptz
);
CREATE INDEX ON core.scene (org_id) WHERE removed_at IS NULL;
GRANT SELECT, INSERT, UPDATE ON core.scene TO ctrl32_app;

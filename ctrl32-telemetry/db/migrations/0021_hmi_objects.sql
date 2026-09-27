-- HMI object library (spec 001, US6): reusable content for process pictures — customer SVG graphics (sanitised on
-- import, shown through <image> so they can never run code) and composites (groups of drawing elements with tag
-- placeholders). Versioned content (ADR-0010): a save is a new row that supersedes the current one (valid_to);
-- slug is the identity across versions; every change has a user and a reason and an audit trail entry.
CREATE TABLE core.hmi_object (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  slug        text NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
  kind        text NOT NULL CHECK (kind IN ('svg', 'composite')),
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
  category    text NOT NULL DEFAULT '' CHECK (length(category) <= 64),
  version     integer NOT NULL DEFAULT 1,
  body        jsonb NOT NULL,
  checksum    text NOT NULL,
  created_by  uuid,
  created_at  timestamptz NOT NULL DEFAULT now(),
  valid_to    timestamptz,
  reason      text NOT NULL
);
CREATE UNIQUE INDEX hmi_object_current ON core.hmi_object (org_id, slug) WHERE valid_to IS NULL;
GRANT SELECT, INSERT, UPDATE ON core.hmi_object TO ctrl32_app;

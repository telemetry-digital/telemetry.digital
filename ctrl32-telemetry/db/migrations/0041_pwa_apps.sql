-- Installable app profiles (spec 021, 0.60.0): an organization defines several PWAs over the same web application,
-- e.g. a phone app that shows only the cameras. Each profile has its own install page and manifest under /app/{slug};
-- the app restricts the menu to the chosen sections (permissions still apply). Content framework (ADR-0010): a change
-- is a new row, the previous one gets valid_to, every change carries a reason. (0040 is left to license activation.)
CREATE TABLE core.pwa_app (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  slug        text NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,39}$'),
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
  short_name  text NOT NULL CHECK (length(short_name) BETWEEN 1 AND 24),
  start_url   text NOT NULL CHECK (start_url ~ '^/' AND start_url !~ '^//' AND length(start_url) <= 300),
  sections    text[] NOT NULL DEFAULT '{}',
  theme_color text NOT NULL DEFAULT '' CHECK (theme_color = '' OR theme_color ~ '^#[0-9a-fA-F]{6}$'),
  enabled     boolean NOT NULL DEFAULT true,
  version     integer NOT NULL DEFAULT 1,
  checksum    text NOT NULL,
  created_by  uuid,
  created_at  timestamptz NOT NULL DEFAULT now(),
  valid_to    timestamptz,
  reason      text NOT NULL
);
CREATE UNIQUE INDEX pwa_app_current ON core.pwa_app (org_id, slug) WHERE valid_to IS NULL;
GRANT SELECT, INSERT, UPDATE ON core.pwa_app TO ctrl32_app;

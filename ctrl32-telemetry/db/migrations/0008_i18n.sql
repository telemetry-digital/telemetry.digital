-- ctrl32 telemetry — UI languages and organization translations (ADR-0009 J-10, content framework ADR-0010).
BEGIN;
SET search_path = core, public;

-- Organization overrides/additions to the built-in catalogs, keyed by the English string.
CREATE TABLE core.translation (
  org_id          uuid NOT NULL REFERENCES core.organization(id),
  locale          text NOT NULL CHECK (locale ~ '^[a-z]{2}(-[A-Za-z]{2})?$'),
  key             text NOT NULL,
  text            text NOT NULL,
  updated_by_name text NOT NULL,
  updated_at      timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (org_id, locale, key)
);
GRANT SELECT, INSERT, UPDATE, DELETE ON core.translation TO ctrl32_app;

-- Enabled languages live in organization.settings->'languages': {"enabled": ["en","sk"], "default": "en"}.
-- The user's own choice is user_account.locale (already present).
COMMIT;

-- ctrl32 telemetry — report templates (content type, ADR-0010) and generated reports (records, ADR-0005).
BEGIN;
SET search_path = core, public;

-- Versioned content: a new version is a new row; current = highest version without valid_to.
CREATE TABLE core.report_template (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid REFERENCES core.organization(id),              -- NULL = built-in default
  kind        text NOT NULL CHECK (kind ~ '^[a-z][a-z0-9_]{1,63}$'), -- observations, alarms, audit, messages, monthly_gxp, ...
  name        text NOT NULL,
  version     integer NOT NULL DEFAULT 1,
  locale      text NOT NULL DEFAULT 'en',
  engine      text NOT NULL DEFAULT 'typst' CHECK (engine IN ('typst')),
  body        text NOT NULL,                                       -- Typst source; receives data.json (documented data contract)
  spec        jsonb,                                               -- optional block-designer representation (phase 2)
  checksum    text NOT NULL,                                       -- sha256 of body
  created_by  uuid,
  created_at  timestamptz NOT NULL DEFAULT now(),
  valid_to    timestamptz,
  reason      text NOT NULL DEFAULT 'initial',
  UNIQUE (org_id, kind, locale, version)
);
CREATE INDEX ON core.report_template (kind, locale) WHERE valid_to IS NULL;

-- Generated reports are records: never modified, file hash verifiable.
CREATE TABLE core.report (
  id            uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id        uuid,
  kind          text NOT NULL,
  template_id   uuid REFERENCES core.report_template(id),
  template_ver  integer,
  scope         jsonb NOT NULL DEFAULT '{}'::jsonb,                -- datastream/device/period parameters
  data_hash     text NOT NULL,                                     -- sha256 of data.json
  file_hash     text NOT NULL,                                     -- sha256 of the PDF
  file_path     text NOT NULL,
  bytes         bigint NOT NULL,
  generated_at  timestamptz NOT NULL DEFAULT now(),
  generated_by  text NOT NULL,
  audit_seq     bigint                                             -- audit checkpoint head printed in the report, if any
);
SELECT core.make_append_only('core.report');
CREATE INDEX ON core.report (org_id, generated_at DESC);

GRANT SELECT, INSERT ON core.report_template, core.report TO ctrl32_app;
GRANT UPDATE (valid_to) ON core.report_template TO ctrl32_app;    -- superseding a version sets valid_to only

-- ---------------------------------------------------------------------------
-- Content permission (ADR-0010): editing templates and other organization content.
-- ---------------------------------------------------------------------------
INSERT INTO core.permission (name, description) VALUES
  ('content.write', 'Edit report templates, branding and other organization content');
UPDATE core.role SET permissions = array_append(permissions, 'content.write')
  WHERE org_id IS NULL AND name IN ('engineer', 'org_admin') AND NOT ('content.write' = ANY (permissions));

-- ---------------------------------------------------------------------------
-- Branding per organization (URS J-16): custom logo, theme colours and font. The product footer is fixed in the binary.
-- {"logo": {"mime": "image/png", "data": "<base64>", "updated_at": ...}, "theme": {"primary": "#0b5fff", "font": "Inter", "mode": "auto"}}
-- ---------------------------------------------------------------------------
ALTER TABLE core.organization ADD COLUMN branding jsonb NOT NULL DEFAULT '{}'::jsonb;

COMMIT;

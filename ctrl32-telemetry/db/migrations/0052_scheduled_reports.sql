-- Scheduled reports by e-mail (spec 029, 0.71.0): a summary of object counting, a period table of a dashboard or a
-- report template is rendered for the previous day, week or month on a cron schedule in the organization's time zone
-- and mailed with PDF and/or XLSX attachments. Every delivery is recorded (append-only).
CREATE TABLE core.scheduled_report (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  content     jsonb NOT NULL,                       -- {"type": "counting" | "period_table" | "template", …}
  period      text NOT NULL CHECK (period IN ('previous_day', 'previous_week', 'previous_month')),
  cron        text NOT NULL CHECK (length(cron) BETWEEN 9 AND 120),
  tz          text NOT NULL,
  recipients  jsonb NOT NULL DEFAULT '{}'::jsonb,   -- {"channels": [notification channel ids], "addresses": [e-mail]}
  attach_pdf  boolean NOT NULL DEFAULT true,
  attach_xlsx boolean NOT NULL DEFAULT true,
  enabled     boolean NOT NULL DEFAULT true,
  next_run_at timestamptz,
  last_run_at timestamptz,
  last_status text,
  created_by  uuid,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz
);
CREATE INDEX ON core.scheduled_report (next_run_at) WHERE enabled AND archived_at IS NULL;

CREATE TABLE core.scheduled_report_run (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL,
  report_id   uuid NOT NULL REFERENCES core.scheduled_report(id),
  started_at  timestamptz NOT NULL,
  finished_at timestamptz NOT NULL DEFAULT now(),
  trigger     text NOT NULL CHECK (trigger IN ('schedule', 'manual')),
  status      text NOT NULL CHECK (status IN ('sent', 'partial', 'failed')),
  period_from timestamptz,
  period_to   timestamptz,
  recipients  integer NOT NULL DEFAULT 0,
  attachments jsonb NOT NULL DEFAULT '[]'::jsonb,   -- [{name, bytes, sha256}]
  error       text,
  actor       text NOT NULL
);
SELECT core.make_append_only('core.scheduled_report_run');
CREATE INDEX ON core.scheduled_report_run (report_id, started_at DESC);

GRANT SELECT, INSERT, UPDATE ON core.scheduled_report TO ctrl32_app;
GRANT SELECT, INSERT ON core.scheduled_report_run TO ctrl32_app;

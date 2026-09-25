-- Four eyes for configuration changes (URS H-08): with the organization policies four_eyes_limits (alarm rules)
-- and four_eyes_assignments (device ↔ datastream assignments) the change is not executed at once but stored as a
-- change request — the original call (method, path, body) — and executed exactly as requested when a different
-- user with the permission of that call approves it. Every step is in the audit trail.
CREATE TABLE core.change_request (
  id                uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id            uuid NOT NULL REFERENCES core.organization(id),
  kind              text NOT NULL CHECK (kind IN ('limits', 'assignments', 'calibration')),
  method            text NOT NULL,
  path              text NOT NULL,
  body              jsonb NOT NULL DEFAULT '{}'::jsonb,
  summary           text NOT NULL DEFAULT '',
  reason            text NOT NULL,
  requested_by      uuid,
  requested_by_name text NOT NULL,
  requested_at      timestamptz NOT NULL DEFAULT now(),
  expires_at        timestamptz NOT NULL DEFAULT now() + interval '7 days',
  status            text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'expired', 'failed')),
  decided_by        uuid,
  decided_by_name   text,
  decided_at        timestamptz,
  decision_reason   text,
  result            jsonb
);
CREATE INDEX ON core.change_request (org_id, status, requested_at DESC);
GRANT SELECT, INSERT, UPDATE ON core.change_request TO ctrl32_app;

-- Server-wide settings of the system administrator (0.76.0): the update mode (System → Updates, ADR-0024) and the
-- outgoing e-mail (System → Server → E-mail (SMTP)). One row per key; the SMTP password is stored encrypted with the
-- server's secret box (security.secret_key never enters the database) and is never returned by the API.
CREATE TABLE core.server_setting (
  key         text PRIMARY KEY CHECK (key IN ('updates', 'smtp')),
  data        jsonb NOT NULL DEFAULT '{}'::jsonb,
  updated_at  timestamptz NOT NULL DEFAULT now(),
  updated_by  uuid
);
GRANT SELECT, INSERT, UPDATE, DELETE ON core.server_setting TO ctrl32_app;

-- notices of the server itself in the delivery log (update available, update results, test e-mail of the SMTP page)
ALTER TABLE core.notification DROP CONSTRAINT IF EXISTS notification_event_check;
ALTER TABLE core.notification ADD CONSTRAINT notification_event_check CHECK (event IN ('raised', 'escalated', 'cleared', 'test', 'rule', 'flow', 'system'));

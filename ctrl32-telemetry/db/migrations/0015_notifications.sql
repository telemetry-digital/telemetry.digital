-- Notification channels and delivery log (URS C-06) with escalation policies (URS C-07/C-08 style steps).
-- A channel is an e-mail recipient list, an HTTP webhook (HMAC-signed) or a generic HTTP SMS gateway. An alarm
-- rule may reference an escalation policy: steps [{after_s, channel_id}] — step 0 immediately, later steps only
-- while the alarm is still active and unacknowledged. Every delivery attempt is a row in core.notification.
CREATE TABLE core.notification_channel (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
  kind        text NOT NULL CHECK (kind IN ('email', 'webhook', 'sms_http')),
  settings    jsonb NOT NULL DEFAULT '{}'::jsonb,
  enabled     boolean NOT NULL DEFAULT true,
  created_at  timestamptz NOT NULL DEFAULT now(),
  revoked_at  timestamptz
);
CREATE UNIQUE INDEX notification_channel_name ON core.notification_channel (org_id, name) WHERE revoked_at IS NULL;

ALTER TABLE core.escalation_policy
  ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS revoked_at timestamptz,
  ADD COLUMN IF NOT EXISTS reason     text NOT NULL DEFAULT '';

CREATE TABLE core.notification (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  alarm_id    uuid REFERENCES core.alarm(id),
  channel_id  uuid REFERENCES core.notification_channel(id),
  policy_id   uuid REFERENCES core.escalation_policy(id),
  step        integer NOT NULL DEFAULT 0,
  event       text NOT NULL CHECK (event IN ('raised', 'escalated', 'cleared', 'test')),
  due_at      timestamptz NOT NULL DEFAULT now(),
  sent_at     timestamptz,
  status      text NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'sent', 'failed', 'cancelled')),
  attempts    integer NOT NULL DEFAULT 0,
  error       text,
  payload     jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX ON core.notification (status, due_at);
CREATE INDEX ON core.notification (org_id, due_at DESC);
GRANT SELECT, INSERT, UPDATE ON core.notification_channel, core.notification, core.escalation_policy TO ctrl32_app;

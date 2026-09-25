-- Web Push (URS C-06): one VAPID key pair per server, push subscriptions per user and browser, and two more
-- channel kinds: web_push (browser notifications for named users or all users of the organization) and
-- voice_http (a generic HTTP voice/call provider with {to} and {text} placeholders, like sms_http).
CREATE TABLE core.web_push_key (
  id          integer PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  public_key  text NOT NULL,
  private_key text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE core.push_subscription (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id       uuid NOT NULL REFERENCES core.organization(id),
  user_id      uuid NOT NULL REFERENCES core.user_account(id),
  endpoint     text NOT NULL,
  p256dh       text NOT NULL,
  auth         text NOT NULL,
  user_agent   text NOT NULL DEFAULT '',
  created_at   timestamptz NOT NULL DEFAULT now(),
  last_used_at timestamptz,
  failed_at    timestamptz,
  revoked_at   timestamptz
);
CREATE UNIQUE INDEX push_subscription_endpoint ON core.push_subscription (endpoint) WHERE revoked_at IS NULL;
CREATE INDEX ON core.push_subscription (org_id, user_id) WHERE revoked_at IS NULL;

ALTER TABLE core.notification_channel DROP CONSTRAINT IF EXISTS notification_channel_kind_check;
ALTER TABLE core.notification_channel ADD CONSTRAINT notification_channel_kind_check CHECK (kind IN ('email', 'webhook', 'sms_http', 'web_push', 'voice_http'));

GRANT SELECT, INSERT, UPDATE ON core.web_push_key, core.push_subscription TO ctrl32_app;

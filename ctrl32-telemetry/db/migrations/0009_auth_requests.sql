-- ctrl32 telemetry — self-service authentication flows: password reset links and account requests (URS H-*).
BEGIN;
SET search_path = core, public;

-- A reset link is a one-time token (hash only), valid one hour, bound to the user; used_at records consumption.
CREATE TABLE core.password_reset (
  id         uuid PRIMARY KEY DEFAULT uuidv7(),
  user_id    uuid NOT NULL REFERENCES core.user_account(id),
  token_hash text NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  used_at    timestamptz,
  ip         inet
);
CREATE INDEX ON core.password_reset (user_id, created_at DESC);

-- Someone without an account asks for one; an administrator creates the account (or dismisses the request).
CREATE TABLE core.account_request (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id       uuid REFERENCES core.organization(id),
  full_name    text NOT NULL,
  email        text NOT NULL,
  message      text,
  ip           inet,
  created_at   timestamptz NOT NULL DEFAULT now(),
  handled_at   timestamptz,
  handled_by   text,
  outcome      text CHECK (outcome IN ('created', 'dismissed'))
);
CREATE INDEX ON core.account_request (handled_at NULLS FIRST, created_at DESC);

GRANT SELECT, INSERT, UPDATE ON core.password_reset, core.account_request TO ctrl32_app;
COMMIT;

-- Organizations as tenants (URS K-01..K-05): a server hosts several organizations; every user, device, site,
-- role, template, translation and report belongs to exactly one of them. A system administrator creates
-- organizations and can disable them; a disabled organization cannot sign in and its devices cannot connect.
ALTER TABLE core.organization
  ADD COLUMN disabled_at     timestamptz,
  ADD COLUMN disabled_reason text;

-- Sign-in is by username or e-mail without naming the organization, so both must be unique on the whole server.
CREATE UNIQUE INDEX user_account_username_server ON core.user_account (username);
CREATE UNIQUE INDEX user_account_email_server ON core.user_account (lower(email)) WHERE email IS NOT NULL AND email <> '';

GRANT SELECT, INSERT, UPDATE ON core.organization TO ctrl32_app;

-- ctrl32 telemetry — IAM v1: permissions catalog, built-in roles, TOTP state, recovery codes,
-- session hardening, pre-auth (MFA) tokens, organization security policy defaults.
-- Requires 0001_core.sql.

BEGIN;
SET search_path = core, public;

-- ---------------------------------------------------------------------------
-- Permissions and built-in roles (URS H-06)
-- ---------------------------------------------------------------------------

CREATE TABLE core.permission (
  name        text PRIMARY KEY CHECK (name ~ '^[a-z_]+\.[a-z_]+$'),
  description text NOT NULL
);
INSERT INTO core.permission (name, description) VALUES
  ('data.read',        'Read assets, datastreams, observations, devices, alarms'),
  ('data.export',      'Export data (CSV/JSON)'),
  ('alarm.ack',        'Acknowledge and suppress alarms'),
  ('data.annotate',    'Annotate observations and gaps'),
  ('device.manage',    'Create, edit, revoke devices; issue and rotate device tokens'),
  ('device.console',   'Open a remote console session on a device'),
  ('device.command',   'Send commands to devices'),
  ('fota.manage',      'Upload firmware and run FOTA campaigns'),
  ('config.write',     'Change assets, datastreams, alarm rules, connectors, retention'),
  ('user.admin',       'Manage users and roles'),
  ('token.manage',     'Create and revoke own API tokens'),
  ('audit.read',       'Read audit trail, access log and alarm log'),
  ('audit.review',     'Record audit trail reviews and sign records'),
  ('report.sign',      'Electronically sign reports'),
  ('system.admin',     'Backups, updates, system configuration');

-- Built-in roles have org_id NULL and cannot be edited by tenants (the app enforces it).
INSERT INTO core.role (org_id, name, permissions, is_admin, is_qa) VALUES
  (NULL, 'viewer',     ARRAY['data.read'], false, false),
  (NULL, 'operator',   ARRAY['data.read','data.export','alarm.ack','data.annotate','device.command'], false, false),
  (NULL, 'qa',         ARRAY['data.read','data.export','audit.read','audit.review','report.sign'], false, true),
  (NULL, 'engineer',   ARRAY['data.read','data.export','alarm.ack','data.annotate','device.manage','device.console','device.command','fota.manage','config.write','token.manage'], false, false),
  (NULL, 'org_admin',  ARRAY['data.read','data.export','alarm.ack','data.annotate','device.manage','device.console','device.command','fota.manage','config.write','user.admin','token.manage','audit.read','system.admin'], true, false);

-- ---------------------------------------------------------------------------
-- User account additions (URS H-02, H-03)
-- ---------------------------------------------------------------------------

ALTER TABLE core.user_account
  ADD COLUMN password_changed_at timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN totp_confirmed_at   timestamptz,
  ADD COLUMN totp_last_step      bigint NOT NULL DEFAULT 0,        -- last accepted TOTP time step (replay protection)
  ADD COLUMN locale              text NOT NULL DEFAULT 'en',
  ADD COLUMN last_login_at       timestamptz;

CREATE TABLE core.user_recovery_code (
  id        uuid PRIMARY KEY DEFAULT uuidv7(),
  user_id   uuid NOT NULL REFERENCES core.user_account(id),
  code_hash text NOT NULL,
  used_at   timestamptz
);
CREATE INDEX ON core.user_recovery_code (user_id) WHERE used_at IS NULL;

-- ---------------------------------------------------------------------------
-- Sessions (URS H-05): idle + absolute expiry, CSRF secret, MFA state
-- ---------------------------------------------------------------------------

ALTER TABLE core.session
  ADD COLUMN absolute_expires_at timestamptz,
  ADD COLUMN csrf_secret         text,
  ADD COLUMN mfa_pending         boolean NOT NULL DEFAULT false;   -- true between password and TOTP step
CREATE INDEX ON core.session (user_id) WHERE revoked_at IS NULL;
CREATE INDEX ON core.session (expires_at);

-- ---------------------------------------------------------------------------
-- Organization security policy defaults (stored in organization.settings->'security')
-- ---------------------------------------------------------------------------

UPDATE core.organization
SET settings = settings || jsonb_build_object('security', jsonb_build_object(
  'session_idle_minutes', 30,
  'session_absolute_hours', 8,
  'lockout_threshold', 5,
  'lockout_minutes', 15,
  'password_min_length', 12,
  'mfa_required_roles', jsonb_build_array('org_admin', 'qa', 'engineer'),
  'mfa_required_all', false
))
WHERE NOT (settings ? 'security');

-- ---------------------------------------------------------------------------
-- Device claim codes (protocol §3.7) — one-time, short-lived
-- ---------------------------------------------------------------------------

ALTER TABLE core.device_credential ADD COLUMN used_at timestamptz;

COMMIT;

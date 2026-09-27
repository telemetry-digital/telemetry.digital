-- Displays (spec 014): kiosk devices (a PC or TV at a wall) paired once with the organization. A display shows only
-- what it was given (dashboards, process screens, video wall screens, in turn), cannot change anything and is revoked
-- at any time. Its credential is a random token kept in an HttpOnly cookie; only its hash is stored.
CREATE TABLE core.display (
  id            uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id        uuid NOT NULL REFERENCES core.organization(id),
  name          text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  token_hash    text UNIQUE,
  content       jsonb NOT NULL DEFAULT '{"items": []}'::jsonb,
  command       jsonb,                      -- the latest operator command (show a camera, identify, reload)
  created_at    timestamptz NOT NULL DEFAULT now(),
  created_by    uuid,
  updated_at    timestamptz NOT NULL DEFAULT now(),
  last_seen_at  timestamptz,
  last_ip       text,
  last_agent    text,
  offline_alert boolean NOT NULL DEFAULT true,
  revoked_at    timestamptz
);
CREATE INDEX ON core.display (org_id) WHERE revoked_at IS NULL;
GRANT SELECT, INSERT, UPDATE ON core.display TO ctrl32_app;

-- A kiosk waiting to be paired: it shows the code; the secret stays in its HttpOnly cookie, so knowing the code
-- alone never yields the display's credential.
CREATE TABLE core.display_pairing (
  code_hash    text PRIMARY KEY,
  secret_hash  text NOT NULL UNIQUE,
  ip           text NOT NULL,
  agent        text NOT NULL DEFAULT '',
  created_at   timestamptz NOT NULL DEFAULT now(),
  expires_at   timestamptz NOT NULL,
  display_id   uuid REFERENCES core.display(id),
  token        text                          -- handed to the kiosk once, then the row is deleted
);
GRANT SELECT, INSERT, UPDATE, DELETE ON core.display_pairing TO ctrl32_app;

-- Camera groups: a user limited to some groups sees only those cameras (NULL = all cameras of the organization).
ALTER TABLE core.camera ADD COLUMN camera_group text NOT NULL DEFAULT '' CHECK (length(camera_group) <= 60);
ALTER TABLE core.user_account ADD COLUMN camera_groups text[];

INSERT INTO core.permission (name, description) VALUES
  ('display.manage', 'Pair and control displays (kiosks)')
ON CONFLICT (name) DO NOTHING;
UPDATE core.role SET permissions = array_append(permissions, 'display.manage')
  WHERE org_id IS NULL AND name IN ('engineer', 'org_admin') AND NOT ('display.manage' = ANY (permissions));
INSERT INTO core.permission (name, description) VALUES
  ('display.control', 'Send cameras and commands to displays')
ON CONFLICT (name) DO NOTHING;
UPDATE core.role SET permissions = array_append(permissions, 'display.control')
  WHERE org_id IS NULL AND name IN ('operator', 'engineer', 'org_admin') AND NOT ('display.control' = ANY (permissions));

-- health incidents of cameras, displays and the video storage
ALTER TABLE core.incident DROP CONSTRAINT IF EXISTS incident_kind_check;
ALTER TABLE core.incident ADD CONSTRAINT incident_kind_check CHECK (kind IN ('alarm_undelivered', 'audit_chain', 'backup_failed', 'manual', 'replica', 'flow', 'mqtt_source', 'camera', 'display', 'storage'));

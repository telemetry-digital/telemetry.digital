-- ctrl32 telemetry — remote device console (protocol §3.5, URS D-*) and FOTA (protocol §3.6, URS C-*).
BEGIN;
SET search_path = core, public;

-- A console session is opened by an operator; the device only accepts console input while the shared attribute
-- console.enabled is true (set on open, cleared on close or idle timeout). Every line in both directions is a record.
CREATE TABLE core.console_session (
  id               uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id           uuid NOT NULL REFERENCES core.organization(id),
  device_id        uuid NOT NULL REFERENCES core.device(id),
  opened_by        uuid,
  opened_by_name   text NOT NULL,
  opened_at        timestamptz NOT NULL DEFAULT now(),
  last_activity_at timestamptz NOT NULL DEFAULT now(),
  closed_at        timestamptz,
  close_reason     text
);
CREATE INDEX ON core.console_session (device_id, opened_at DESC);

CREATE TABLE core.console_log (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  session_id  uuid NOT NULL REFERENCES core.console_session(id),
  org_id      uuid NOT NULL,
  device_id   uuid NOT NULL,
  ts          timestamptz NOT NULL DEFAULT now(),
  device_ts   timestamptz,
  direction   text NOT NULL CHECK (direction IN ('in', 'out')),
  line        text NOT NULL,
  actor_name  text NOT NULL
);
SELECT core.make_append_only('core.console_log');
CREATE INDEX ON core.console_log (session_id, id);

-- Firmware images are records (never modified); the blob lives on disk under fota.storage_dir, hashed.
CREATE TABLE core.firmware (
  id               uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id           uuid NOT NULL REFERENCES core.organization(id),
  name             text NOT NULL,
  version          text NOT NULL,
  hw               text NOT NULL,                                   -- hardware id the image is built for (esp32-s3, stm32f4, ...)
  size             bigint NOT NULL,
  sha256           text NOT NULL,
  signature        text,                                            -- author signature (Ed25519, hex) when provided
  min_version      text,
  notes            text,
  file_path        text NOT NULL,
  uploaded_by      uuid,
  uploaded_by_name text NOT NULL,
  uploaded_at      timestamptz NOT NULL DEFAULT now(),
  reason           text NOT NULL,
  UNIQUE (org_id, name, version, hw)
);
SELECT core.make_append_only('core.firmware');

CREATE TABLE core.fota_campaign (
  id              uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id          uuid NOT NULL REFERENCES core.organization(id),
  firmware_id     uuid NOT NULL REFERENCES core.firmware(id),
  name            text NOT NULL,
  created_by      uuid,
  created_by_name text NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  closed_at       timestamptz,
  close_reason    text,
  reason          text NOT NULL
);

CREATE TABLE core.fota_target (
  campaign_id uuid NOT NULL REFERENCES core.fota_campaign(id),
  device_id   uuid NOT NULL REFERENCES core.device(id),
  status      text NOT NULL DEFAULT 'pending'
              CHECK (status IN ('pending', 'downloading', 'verifying', 'applying', 'updated', 'failed', 'rolled_back', 'cancelled')),
  pct         integer,
  version     text,
  error       text,
  updated_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (campaign_id, device_id)
);
CREATE INDEX ON core.fota_target (device_id);

INSERT INTO core.permission (name, description) VALUES
  ('fota.read', 'View firmware images and update campaigns')
ON CONFLICT (name) DO NOTHING;
UPDATE core.role SET permissions = array_append(permissions, 'fota.read')
  WHERE org_id IS NULL AND name IN ('operator', 'engineer', 'org_admin', 'qa') AND NOT ('fota.read' = ANY (permissions));

GRANT SELECT, INSERT, UPDATE ON core.console_session, core.fota_campaign, core.fota_target TO ctrl32_app;
GRANT SELECT, INSERT ON core.console_log, core.firmware TO ctrl32_app;

COMMIT;

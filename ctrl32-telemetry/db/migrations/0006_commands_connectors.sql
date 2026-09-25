-- ctrl32 telemetry — device commands (RPC with ack), shared attributes, connectors (protocol §3.3, §3.4; URS D-*, E-*).
BEGIN;
SET search_path = core, public;

-- Commands are operational state with a small lifecycle (queued -> sent -> acked|failed|expired); issuing one is audited.
CREATE TABLE core.device_command (
  id             uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id         uuid NOT NULL REFERENCES core.organization(id),
  device_id      uuid NOT NULL REFERENCES core.device(id),
  method         text NOT NULL CHECK (method ~ '^[a-z][a-z0-9_]{0,31}$'),
  args           jsonb NOT NULL DEFAULT '{}'::jsonb,
  expires_at     timestamptz,
  status         text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sent', 'acked', 'failed', 'expired')),
  issued_by      uuid,
  issued_by_name text NOT NULL,
  issued_at      timestamptz NOT NULL DEFAULT now(),
  sent_at        timestamptz,
  acked_at       timestamptz,
  result         jsonb,
  error          text,
  reason         text NOT NULL
);
CREATE INDEX ON core.device_command (device_id, issued_at DESC);

-- Shared attributes: configuration the server pushes to the device (retained on d/{id}/attributes/shared).
ALTER TABLE core.device ADD COLUMN shared_attributes jsonb NOT NULL DEFAULT '{}'::jsonb;

-- Connectors: inbound integrations authenticated by a per-connector secret (hash only). Phase 1: ChirpStack HTTP.
CREATE TABLE core.connector (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id       uuid NOT NULL REFERENCES core.organization(id),
  kind         text NOT NULL CHECK (kind IN ('chirpstack_http')),
  name         text NOT NULL,
  secret_hash  text NOT NULL,
  settings     jsonb NOT NULL DEFAULT '{}'::jsonb,               -- {"auto_create_devices": true}
  created_at   timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz,
  messages     bigint NOT NULL DEFAULT 0,
  revoked_at   timestamptz,
  UNIQUE (org_id, name)
);

GRANT SELECT, INSERT, UPDATE ON core.device_command, core.connector TO ctrl32_app;

COMMIT;

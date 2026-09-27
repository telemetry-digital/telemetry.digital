-- MQTT provisioning and smart-home devices (spec 005).
-- Provisioning profiles let devices register themselves (ThingsBoard-compatible provisioning over MQTT and HTTP).
-- MQTT sources are accounts on the embedded broker (own topic namespace) or bridges to a foreign broker; devices that
-- announce themselves with Home Assistant discovery become devices of transport mqtt_discovery with entities. The
-- current state of an entity is a mutable row (like device.status); its history is the append-only observations of
-- the entity's datastream.

CREATE TABLE core.provision_profile (
  id               uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id           uuid NOT NULL REFERENCES core.organization(id),
  name             text NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
  key              text NOT NULL UNIQUE CHECK (key ~ '^pk_[0-9a-f]{20}$'),
  secret_hash      text NOT NULL,
  strategy         text NOT NULL DEFAULT 'create' CHECK (strategy IN ('create', 'preprovisioned')),
  approval         boolean NOT NULL DEFAULT false,
  site_id          uuid REFERENCES core.site(id),
  room_id          uuid REFERENCES core.asset(id),
  auto_datastreams boolean NOT NULL DEFAULT true,
  name_pattern     text NOT NULL DEFAULT '',
  transport        text NOT NULL DEFAULT 'mqtt' CHECK (transport IN ('mqtt', 'http')),
  max_devices      integer NOT NULL DEFAULT 0 CHECK (max_devices >= 0),
  created_at       timestamptz NOT NULL DEFAULT now(),
  revoked_at       timestamptz
);
CREATE UNIQUE INDEX provision_profile_name ON core.provision_profile (org_id, name) WHERE revoked_at IS NULL;

ALTER TABLE core.device DROP CONSTRAINT IF EXISTS device_transport_check;
ALTER TABLE core.device ADD CONSTRAINT device_transport_check CHECK (transport IN ('mqtt', 'http', 'lorawan', 'opcua', 'modbus', 'mqtt_discovery'));
ALTER TABLE core.device ADD COLUMN auto_datastreams boolean NOT NULL DEFAULT false;
ALTER TABLE core.device ADD COLUMN approval text CHECK (approval IN ('pending', 'approved', 'rejected'));
ALTER TABLE core.device ADD COLUMN provision_profile_id uuid REFERENCES core.provision_profile(id);
CREATE INDEX device_credential_token ON core.device_credential (secret_hash) WHERE kind = 'token' AND revoked_at IS NULL;

ALTER TABLE core.connector DROP CONSTRAINT IF EXISTS connector_kind_check;
ALTER TABLE core.connector ADD CONSTRAINT connector_kind_check CHECK (kind IN ('chirpstack_http', 'opcua', 'modbus_tcp', 'mqtt_account', 'mqtt_bridge'));
ALTER TABLE core.connector ADD COLUMN status jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE core.entity (
  id            uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id        uuid NOT NULL REFERENCES core.organization(id),
  connector_id  uuid NOT NULL REFERENCES core.connector(id),
  device_id     uuid NOT NULL REFERENCES core.device(id),
  unique_id     text NOT NULL CHECK (length(unique_id) BETWEEN 1 AND 255),
  component     text NOT NULL,
  name          text NOT NULL DEFAULT '',
  config        jsonb NOT NULL,
  config_topic  text NOT NULL DEFAULT '',
  datastream_id uuid REFERENCES core.datastream(id),
  room_id       uuid REFERENCES core.asset(id),
  state         text,
  attributes    jsonb NOT NULL DEFAULT '{}'::jsonb,
  available     boolean NOT NULL DEFAULT true,
  last_changed  timestamptz,
  last_updated  timestamptz,
  status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'unsupported', 'removed')),
  error         text NOT NULL DEFAULT '',
  hidden        boolean NOT NULL DEFAULT false,
  sort          integer NOT NULL DEFAULT 0,
  created_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (connector_id, unique_id)
);
CREATE INDEX ON core.entity (org_id, room_id);
CREATE INDEX ON core.entity (device_id);

CREATE TABLE core.discovery (
  id            uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id        uuid NOT NULL REFERENCES core.organization(id),
  connector_id  uuid NOT NULL REFERENCES core.connector(id),
  device_key    text NOT NULL,
  name          text NOT NULL DEFAULT '',
  manufacturer  text NOT NULL DEFAULT '',
  model         text NOT NULL DEFAULT '',
  configs       jsonb NOT NULL DEFAULT '{}'::jsonb,
  status        text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'adopted', 'ignored')),
  device_id     uuid REFERENCES core.device(id),
  first_seen    timestamptz NOT NULL DEFAULT now(),
  last_seen     timestamptz NOT NULL DEFAULT now(),
  UNIQUE (connector_id, device_key)
);

GRANT SELECT, INSERT, UPDATE ON core.provision_profile, core.entity, core.discovery TO ctrl32_app;

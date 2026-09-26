-- Visual automation flows (spec 004). A flow is versioned content like HMI objects (ADR-0010): every save is a new
-- row that supersedes the current one (valid_to); slug is the identity across versions; every change has a user and a
-- reason. Node memory (latches, counters, flow variables) survives restarts in core.flow_state; incoming webhook
-- tokens are derived from the server's secret key (no table). Notifications and commands sent by a flow name the flow version as their origin.
CREATE TABLE core.flow (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  slug        text NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
  version     integer NOT NULL DEFAULT 1,
  state       text NOT NULL DEFAULT 'draft' CHECK (state IN ('draft', 'deployed', 'disabled')),
  spec        jsonb NOT NULL,
  checksum    text NOT NULL,
  created_by  uuid,
  created_at  timestamptz NOT NULL DEFAULT now(),
  valid_to    timestamptz,
  reason      text NOT NULL
);
CREATE UNIQUE INDEX flow_current ON core.flow (org_id, slug) WHERE valid_to IS NULL;
CREATE INDEX ON core.flow (org_id, slug, version DESC);

CREATE TABLE core.flow_state (
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  flow_slug   text NOT NULL,
  node_id     text NOT NULL,
  state       jsonb NOT NULL,
  updated_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (org_id, flow_slug, node_id)
);

ALTER TABLE core.notification DROP CONSTRAINT IF EXISTS notification_event_check;
ALTER TABLE core.notification ADD CONSTRAINT notification_event_check CHECK (event IN ('raised', 'escalated', 'cleared', 'test', 'rule', 'flow'));
ALTER TABLE core.notification ADD COLUMN flow_id uuid REFERENCES core.flow(id);
ALTER TABLE core.device_command ADD COLUMN flow_id uuid REFERENCES core.flow(id);

GRANT SELECT, INSERT, UPDATE ON core.flow TO ctrl32_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON core.flow_state TO ctrl32_app;

-- Deploying a flow that sends commands is a four-eyes change when the organization requires four eyes for commands.
ALTER TABLE core.change_request DROP CONSTRAINT IF EXISTS change_request_kind_check;
ALTER TABLE core.change_request ADD CONSTRAINT change_request_kind_check CHECK (kind IN ('limits', 'assignments', 'calibration', 'flows'));
-- Flows can open and resolve incidents.
ALTER TABLE core.incident DROP CONSTRAINT IF EXISTS incident_kind_check;
ALTER TABLE core.incident ADD CONSTRAINT incident_kind_check CHECK (kind IN ('alarm_undelivered', 'audit_chain', 'backup_failed', 'manual', 'replica', 'flow'));

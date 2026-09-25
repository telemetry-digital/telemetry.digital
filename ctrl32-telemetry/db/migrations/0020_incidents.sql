-- Incident log (URS C-07, I-09): an incident has a type, impact, cause and CAPA (corrective and preventive action)
-- and a small workflow (open → investigating → resolved → closed). The system opens incidents itself when an alarm
-- could not be delivered (every channel of an escalation step failed) and when the audit hash chain is broken;
-- operators open the rest by hand. Every change is an incident_event row (append-only) and an audit trail entry.
CREATE TABLE core.incident (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id       uuid REFERENCES core.organization(id),           -- NULL = instance level (audit chain)
  kind         text NOT NULL CHECK (kind IN ('alarm_undelivered', 'audit_chain', 'backup_failed', 'manual')),
  severity     text NOT NULL DEFAULT 'major' CHECK (severity IN ('minor', 'major', 'critical')),
  status       text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'investigating', 'resolved', 'closed')),
  title        text NOT NULL,
  detail       jsonb NOT NULL DEFAULT '{}',                      -- alarm id, step, channel, error, seq …
  impact       text NOT NULL DEFAULT '',
  cause        text NOT NULL DEFAULT '',
  capa         text NOT NULL DEFAULT '',
  dedup_key    text,                                             -- one open incident per source event
  opened_at    timestamptz NOT NULL DEFAULT now(),
  opened_by    text NOT NULL DEFAULT 'system',
  updated_at   timestamptz NOT NULL DEFAULT now(),
  resolved_at  timestamptz,
  closed_at    timestamptz,
  closed_by    text
);
CREATE INDEX ON core.incident (org_id, status, opened_at DESC);
CREATE UNIQUE INDEX incident_open_dedup ON core.incident (dedup_key) WHERE dedup_key IS NOT NULL AND status IN ('open', 'investigating');

CREATE TABLE core.incident_event (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  incident_id  uuid NOT NULL REFERENCES core.incident(id),
  at           timestamptz NOT NULL DEFAULT now(),
  actor        text NOT NULL,
  event        text NOT NULL CHECK (event IN ('opened', 'status', 'fields', 'note', 'notified')),
  detail       jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX ON core.incident_event (incident_id, at);

GRANT SELECT, INSERT, UPDATE ON core.incident TO ctrl32_app;
GRANT SELECT, INSERT ON core.incident_event TO ctrl32_app;

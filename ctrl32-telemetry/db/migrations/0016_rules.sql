-- Automation rules "condition → action" (URS C-09): a rule declares named inputs (datastreams), a condition over
-- them (all/any of comparisons, "stale" = no data for N seconds, optional hold time and cooldown) and actions on
-- enter/exit: notification channel, inline webhook, device command (write), alarm, derived datastream (expression).
-- Rules are versioned in place (version + audit trail) and every transition is a row in core.rule_run. With the
-- organization policy four_eyes_commands a rule that sends commands stays 'pending' until a second user approves it.
CREATE TABLE core.rule (
  id               uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id           uuid NOT NULL REFERENCES core.organization(id),
  name             text NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
  description      text NOT NULL DEFAULT '',
  spec             jsonb NOT NULL DEFAULT '{}'::jsonb,
  enabled          boolean NOT NULL DEFAULT true,
  status           text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'pending', 'rejected')),
  version          integer NOT NULL DEFAULT 1,
  created_by       uuid,
  created_by_name  text NOT NULL DEFAULT '',
  updated_by       uuid,
  updated_by_name  text NOT NULL DEFAULT '',
  approved_by      uuid,
  approved_by_name text,
  approved_at      timestamptz,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  revoked_at       timestamptz
);
CREATE UNIQUE INDEX rule_name ON core.rule (org_id, name) WHERE revoked_at IS NULL;

CREATE TABLE core.rule_run (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id       uuid NOT NULL REFERENCES core.organization(id),
  rule_id      uuid NOT NULL REFERENCES core.rule(id),
  rule_version integer NOT NULL DEFAULT 1,
  fired_at     timestamptz NOT NULL DEFAULT now(),
  event        text NOT NULL CHECK (event IN ('enter', 'exit')),
  inputs       jsonb NOT NULL DEFAULT '{}'::jsonb,   -- {name: value} at evaluation time
  actions      jsonb NOT NULL DEFAULT '[]'::jsonb,   -- [{type, status: done|failed, detail}]
  ok           boolean NOT NULL DEFAULT true
);
CREATE INDEX ON core.rule_run (org_id, fired_at DESC);
CREATE INDEX ON core.rule_run (rule_id, fired_at DESC);

-- Alarms raised by automation rules: the alarm-rule reference becomes optional, the automation rule is recorded.
ALTER TABLE core.alarm ALTER COLUMN rule_id DROP NOT NULL;
ALTER TABLE core.alarm ADD COLUMN automation_rule_id uuid REFERENCES core.rule(id);
ALTER TABLE core.alarm ADD CONSTRAINT alarm_source CHECK (rule_id IS NOT NULL OR automation_rule_id IS NOT NULL);
CREATE INDEX ON core.alarm (automation_rule_id) WHERE automation_rule_id IS NOT NULL;

-- Notifications sent by rules and commands issued by rules are traceable to the rule.
ALTER TABLE core.notification DROP CONSTRAINT IF EXISTS notification_event_check;
ALTER TABLE core.notification ADD CONSTRAINT notification_event_check CHECK (event IN ('raised', 'escalated', 'cleared', 'test', 'rule'));
ALTER TABLE core.notification ADD COLUMN rule_id uuid REFERENCES core.rule(id);
ALTER TABLE core.device_command ADD COLUMN rule_id uuid REFERENCES core.rule(id);

GRANT SELECT, INSERT, UPDATE ON core.rule, core.rule_run TO ctrl32_app;

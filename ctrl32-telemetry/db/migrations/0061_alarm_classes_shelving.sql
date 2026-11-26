-- Alarm management after ISA-18.2 (spec 035, 0.89.0): alarm classes, the acknowledgement as a fact of its own (an
-- alarm that returned to normal stays on the screen until someone acknowledges it) and shelving. The class of an alarm
-- is the class of its rule (rule versions never change), else the built-in class of its severity.

CREATE TABLE core.alarm_class (
  id           uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id       uuid NOT NULL REFERENCES core.organization(id),
  code         text NOT NULL CHECK (code ~ '^[a-z][a-z0-9_]{1,30}$'),
  name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 60),
  priority     smallint NOT NULL CHECK (priority BETWEEN 1 AND 4),
  color        text NOT NULL CHECK (color ~ '^#[0-9a-fA-F]{6}$'),
  ack_required boolean NOT NULL DEFAULT true,
  sound        text NOT NULL DEFAULT 'none' CHECK (sound IN ('none', 'beep', 'siren')),
  builtin      boolean NOT NULL DEFAULT false,
  created_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, code)
);
GRANT SELECT, INSERT, UPDATE, DELETE ON core.alarm_class TO ctrl32_app;
-- the three built-in classes of every organization (new organizations get them on first use)
INSERT INTO core.alarm_class (org_id, code, name, priority, color, ack_required, sound, builtin)
  SELECT o.id, c.code, c.name, c.priority, c.color, c.ack, c.sound, true
  FROM core.organization o, (VALUES ('alarm', 'Alarm', 1, '#d63939', true, 'siren'), ('warning', 'Warning', 2, '#f59f00', true, 'beep'),
    ('message', 'Message', 4, '#4299e1', false, 'none')) AS c(code, name, priority, color, ack, sound)
  ON CONFLICT DO NOTHING;

ALTER TABLE core.alarm_rule ADD COLUMN IF NOT EXISTS class_id uuid REFERENCES core.alarm_class(id);

ALTER TABLE core.alarm
  ADD COLUMN IF NOT EXISTS acked_at      timestamptz,
  ADD COLUMN IF NOT EXISTS acked_by      text,
  ADD COLUMN IF NOT EXISTS shelved_until timestamptz;

-- alarms from before: the acknowledgement time from the alarm log; alarms that had cleared count as acknowledged
-- (they could not be acknowledged afterwards, so they must not all appear as "returned to normal, unacknowledged")
UPDATE core.alarm a SET acked_at = e.ts, acked_by = e.actor_name
  FROM (SELECT DISTINCT ON (alarm_id) alarm_id, ts, actor_name FROM core.alarm_event WHERE event = 'acknowledged' ORDER BY alarm_id, ts) e
  WHERE e.alarm_id = a.id AND a.acked_at IS NULL;
UPDATE core.alarm SET acked_at = cleared_at WHERE acked_at IS NULL AND cleared_at IS NOT NULL;
UPDATE core.alarm SET acked_at = raised_at WHERE acked_at IS NULL AND state = 'acknowledged';

-- what needs attention: not returned to normal, or not acknowledged
CREATE INDEX IF NOT EXISTS alarm_attention_idx ON core.alarm (org_id, raised_at DESC) WHERE cleared_at IS NULL OR acked_at IS NULL;
CREATE INDEX IF NOT EXISTS alarm_rule_raised_idx ON core.alarm (org_id, rule_id, raised_at);

CREATE TABLE core.alarm_shelf (
  id             uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id         uuid NOT NULL REFERENCES core.organization(id),
  rule_id            uuid REFERENCES core.alarm_rule(id),   -- a limit alarm …
  automation_rule_id uuid REFERENCES core.rule(id),         -- … or the alarm of an automation rule
  until          timestamptz NOT NULL,
  by_id          uuid NOT NULL,
  by_name        text NOT NULL,
  reason         text NOT NULL CHECK (length(btrim(reason)) >= 3),
  created_at     timestamptz NOT NULL DEFAULT now(),
  released_at    timestamptz,
  released_by    text,
  release_reason text,
  CHECK ((rule_id IS NULL) <> (automation_rule_id IS NULL))
);
CREATE UNIQUE INDEX alarm_shelf_open_idx ON core.alarm_shelf (coalesce(rule_id, automation_rule_id)) WHERE released_at IS NULL;
GRANT SELECT, INSERT, UPDATE ON core.alarm_shelf TO ctrl32_app;

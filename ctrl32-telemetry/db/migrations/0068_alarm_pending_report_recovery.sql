-- 0.98.1 (audit, alarms and scheduled reports).
--
-- 1. The delay of an alarm survives a restart. A condition that waits for the delay of its rule was kept in memory
--    only, so a restart started the delay again and the alarm came late. The start of the delay is stored here.
CREATE TABLE core.alarm_pending (
  rule_id       uuid PRIMARY KEY REFERENCES core.alarm_rule(id),
  org_id        uuid NOT NULL,
  datastream_id uuid NOT NULL,
  key           text NOT NULL,
  since         timestamptz NOT NULL,          -- the measurement time the condition began
  severity      text NOT NULL CHECK (severity IN ('warning', 'action')),
  value         double precision NOT NULL,
  measured_at   timestamptz NOT NULL,          -- the observation the row was written for
  created_at    timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT, INSERT, UPDATE, DELETE ON core.alarm_pending TO ctrl32_app;

-- 2. At most one open alarm per rule, guaranteed by the database (the engine only locked inside one process). Open
--    duplicates left by older versions are closed first: the newest stays.
WITH d AS (
  SELECT id, row_number() OVER (PARTITION BY rule_id ORDER BY raised_at DESC, id DESC) AS n
  FROM core.alarm WHERE cleared_at IS NULL AND rule_id IS NOT NULL
), c AS (
  UPDATE core.alarm a SET state = 'cleared', cleared_at = now() FROM d WHERE a.id = d.id AND d.n > 1 RETURNING a.id
)
INSERT INTO core.alarm_event (alarm_id, event, detail)
SELECT id, 'cleared', '{"reason":"duplicate of a newer open alarm of the same rule (migration 0068)"}'::jsonb FROM c;
CREATE UNIQUE INDEX alarm_one_open_per_rule_idx ON core.alarm (rule_id) WHERE cleared_at IS NULL;

-- 3. A scheduled report that was interrupted (the server stopped between claiming the run and recording it) is run
--    once more. running_at marks a run in progress, running_for the scheduled time it belongs to.
ALTER TABLE core.scheduled_report
  ADD COLUMN running_at  timestamptz,
  ADD COLUMN running_for timestamptz,
  ADD COLUMN running_try integer NOT NULL DEFAULT 0;

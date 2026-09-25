-- ctrl32 telemetry — history tables may be closed but never edited (ADR-0005).
-- assignment and alarm_rule are versioned history: a new version is a new row and the previous row is closed by
-- setting valid_to once. Until now the append-only guard denied even that, so configuration could not change
-- through the application. The close-only guard allows exactly one transition: valid_to NULL -> timestamp, with
-- every other column unchanged.
BEGIN;
SET search_path = core, public;

CREATE OR REPLACE FUNCTION core.close_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'ctrl32: DELETE on history table %.% is not allowed', TG_TABLE_SCHEMA, TG_TABLE_NAME
      USING ERRCODE = 'integrity_constraint_violation';
  END IF;
  IF OLD.valid_to IS NOT NULL OR NEW.valid_to IS NULL THEN
    RAISE EXCEPTION 'ctrl32: rows of history table %.% can only be closed once (valid_to NULL -> timestamp)', TG_TABLE_SCHEMA, TG_TABLE_NAME
      USING ERRCODE = 'integrity_constraint_violation';
  END IF;
  IF (to_jsonb(OLD) - 'valid_to') <> (to_jsonb(NEW) - 'valid_to') THEN
    RAISE EXCEPTION 'ctrl32: only valid_to may change on history table %.%', TG_TABLE_SCHEMA, TG_TABLE_NAME
      USING ERRCODE = 'integrity_constraint_violation';
  END IF;
  RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS ao_row_assignment ON core.assignment;
CREATE TRIGGER ao_row_assignment BEFORE UPDATE OR DELETE ON core.assignment FOR EACH ROW EXECUTE FUNCTION core.close_only();
DROP TRIGGER IF EXISTS ao_row_alarm_rule ON core.alarm_rule;
CREATE TRIGGER ao_row_alarm_rule BEFORE UPDATE OR DELETE ON core.alarm_rule FOR EACH ROW EXECUTE FUNCTION core.close_only();

GRANT UPDATE (valid_to) ON core.assignment, core.alarm_rule TO ctrl32_app;

COMMIT;

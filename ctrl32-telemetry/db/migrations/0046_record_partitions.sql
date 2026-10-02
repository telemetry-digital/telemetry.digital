-- 0.66.0: month partitions of the record tables are kept up by the server.
--
-- core.raw_message and core.observation are partitioned by month. Until now partitions were created only by migration
-- 0001 (the month of the installation and the two after it), so (1) a reading backdated before that month - a device
-- emptying its buffer, a meter read out for the last year - was refused with "no partition of relation observation
-- found for row", and (2) nothing created the partitions of later months: a server that ran longer than that would
-- have stopped storing telemetry. The application role may not create tables, so it calls these two functions; they
-- run with the rights of the migration owner and only ever add a month partition of these two tables.

-- The month partition of tbl that holds the time at. Only core.raw_message and core.observation, and only months
-- from 20 years back to one year ahead (a device clock at 1970 must not create partitions; such a reading stays
-- refused). Returns whether the partition exists now.
CREATE OR REPLACE FUNCTION core.ensure_record_month(tbl text, at timestamptz) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = core, pg_catalog AS $$
BEGIN
  IF tbl NOT IN ('raw_message', 'observation') THEN
    RAISE EXCEPTION 'ctrl32: % is not a record table', tbl;
  END IF;
  IF at IS NULL OR at < now() - interval '20 years' OR at > now() + interval '1 year' THEN
    RETURN false;
  END IF;
  PERFORM core.ensure_month_partitions(('core.' || tbl)::regclass, (at AT TIME ZONE 'UTC')::date, 1);
  RETURN true;
END $$;

-- The partitions of both record tables from months_back months before the current month to months_ahead after it
-- (the server calls it at start and daily with 0 and 3).
CREATE OR REPLACE FUNCTION core.ensure_record_partitions(months_back integer, months_ahead integer) RETURNS integer
LANGUAGE plpgsql SECURITY DEFINER SET search_path = core, pg_catalog AS $$
DECLARE
  first date := (date_trunc('month', now() AT TIME ZONE 'UTC') - make_interval(months => least(greatest(months_back, 0), 240)))::date;
  n integer := least(greatest(months_back, 0), 240) + least(greatest(months_ahead, 0), 12) + 1;
BEGIN
  RETURN core.ensure_month_partitions('core.raw_message'::regclass, first, n)
       + core.ensure_month_partitions('core.observation'::regclass, first, n);
END $$;

REVOKE ALL ON FUNCTION core.ensure_record_month(text, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION core.ensure_record_partitions(integer, integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION core.ensure_record_month(text, timestamptz) TO ctrl32_app;
GRANT EXECUTE ON FUNCTION core.ensure_record_partitions(integer, integer) TO ctrl32_app;

-- catch up now: the current month and three ahead (an installation older than three months has none of them)
SELECT core.ensure_record_partitions(0, 3);

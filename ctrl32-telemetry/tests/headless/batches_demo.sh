#!/usr/bin/env bash
# Demo batches for tests/headless/batches.js and the documentation pictures: a unit "Press line docs" with three
# finished batches of about an hour each in the last six hours, over the demo datastreams of Line 1
# (deploy/dev/demo/production.py setup). A batch cannot be backdated through the API — it is a record — so the rows are
# written with psql; development databases only.
#   DB_ADMIN=postgres://ctrl32:ctrl32-dev-only@127.0.0.1:5433/ctrl32_dev bash tests/headless/batches_demo.sh
set -eu
DB=${DB_ADMIN:-postgres://ctrl32:ctrl32-dev-only@127.0.0.1:5433/ctrl32_dev}
psql -v ON_ERROR_STOP=1 -Atq "$DB" <<'SQL'
DO $$
DECLARE
  o uuid; dev uuid; u uuid; cur jsonb;
BEGIN
  SELECT d.org_id, jsonb_agg(d.id::text ORDER BY array_position(ARRAY['temperature','pressure','level'], d.key)) INTO o, cur
    FROM core.datastream d JOIN core.asset a ON a.id = d.asset_id
    WHERE a.name LIKE 'Line 1%' AND d.key IN ('temperature', 'pressure', 'level') GROUP BY d.org_id LIMIT 1;
  IF o IS NULL THEN RAISE EXCEPTION 'no Line 1 datastreams: run deploy/dev/demo/production.py setup'; END IF;
  SELECT id INTO dev FROM core.device WHERE org_id = o AND revoked_at IS NULL ORDER BY created_at LIMIT 1;
  SELECT id INTO u FROM core.batch_unit WHERE org_id = o AND name = 'Press line docs' AND removed_at IS NULL;
  IF u IS NOT NULL THEN RETURN; END IF;
  INSERT INTO core.batch_unit (org_id, name, mode, device_id, prefix, curves, next_number) VALUES (o, 'Press line docs', 'recipe', dev, 'm1_', cur, 45) RETURNING id INTO u;
  INSERT INTO core.batch (org_id, unit_id, unit_name, number, code, status, started_at, ended_at, start_source, end_source, recipe_name, record_name, record_number, started_by, operator, ended_by, end_reason, curves) VALUES
    (o, u, 'Press line docs', 42, '12 Bracket A', 'finished', now() - interval '5 hours 40 minutes', now() - interval '4 hours 35 minutes', 'recipe', 'recipe', 'Pressing', 'Bracket A', 12, 'station1', 'jana', 'station1', 'the next batch began', cur),
    (o, u, 'Press line docs', 43, '14 Bracket B', 'finished', now() - interval '4 hours 35 minutes', now() - interval '3 hours 20 minutes', 'recipe', 'recipe', 'Pressing', 'Bracket B', 14, 'station1', 'jana', 'station1', 'the next batch began', cur),
    (o, u, 'Press line docs', 44, '12 Bracket A', 'finished', now() - interval '3 hours 20 minutes', now() - interval '2 hours 10 minutes', 'recipe', 'manual', 'Pressing', 'Bracket A', 12, 'station1', 'peter', 'station1 / veduci', 'end of the order', cur);
END $$;
SQL
echo "demo batches ready: unit Press line docs"

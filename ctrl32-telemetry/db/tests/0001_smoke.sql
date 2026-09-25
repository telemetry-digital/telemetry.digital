-- Smoke test for core schema v1. Run as the schema owner after 0001_core.sql; must finish with "SMOKE OK".
-- Checks: append-only guards (UPDATE/DELETE/TRUNCATE rejected), audit chain hashing and verification,
-- reason CHECK, two timestamps on observations, partition routing, app-role privileges.
\set ON_ERROR_STOP on
SET search_path = core, public;

-- 1. Fixture
INSERT INTO organization (slug, name, tz) VALUES ('acme', 'ACME s.r.o.', 'Europe/Bratislava');
INSERT INTO site (org_id, name, tz) SELECT id, 'Main plant', 'Europe/Bratislava' FROM organization WHERE slug = 'acme';
INSERT INTO asset (org_id, site_id, type, name, attributes)
  SELECT o.id, s.id, 'fridge', 'Fridge 1', '{"storage_class":"2-8C"}' FROM organization o JOIN site s ON s.org_id = o.id;
INSERT INTO datastream (org_id, asset_id, key, quantity, unit, kind, expected_interval, accuracy)
  SELECT a.org_id, a.id, 'temp', 'temperature', '°C', 'gauge', interval '10 minutes', 0.5 FROM asset a;

-- 2. Observations: two timestamps, partition routing
INSERT INTO observation (org_id, datastream_id, measured_at, value, time_source, decoder_version)
  SELECT org_id, id, now() - interval '5 minutes', 4.2, 'device', 'milesight-em320@1' FROM datastream;
DO $$
DECLARE r record;
BEGIN
  SELECT measured_at, received_at, tableoid::regclass AS part INTO r FROM observation LIMIT 1;
  IF r.received_at < r.measured_at THEN RAISE EXCEPTION 'received_at must not precede measured_at'; END IF;
  IF r.part::text NOT LIKE 'observation_%' THEN RAISE EXCEPTION 'observation not routed to a month partition: %', r.part; END IF;
  RAISE NOTICE 'observation ok: measured_at=% received_at=% partition=%', r.measured_at, r.received_at, r.part;
END $$;

-- 3. Append-only guards
DO $$
BEGIN
  BEGIN
    UPDATE observation SET value = 99; RAISE EXCEPTION 'UPDATE on observation was allowed';
  EXCEPTION WHEN integrity_constraint_violation THEN RAISE NOTICE 'update on observation rejected: ok'; END;
  BEGIN
    DELETE FROM observation; RAISE EXCEPTION 'DELETE on observation was allowed';
  EXCEPTION WHEN integrity_constraint_violation THEN RAISE NOTICE 'delete on observation rejected: ok'; END;
  BEGIN
    TRUNCATE observation; RAISE EXCEPTION 'TRUNCATE on observation was allowed';
  EXCEPTION WHEN integrity_constraint_violation THEN RAISE NOTICE 'truncate on observation rejected: ok'; END;
END $$;

-- 4. Audit chain: three entries, verify, tamper detection via canonical recompute
INSERT INTO audit_trail (org_id, actor_type, actor_name, action, entity_type, entity_id, new_value)
  SELECT id, 'user', 'admin', 'organization.create', 'organization', id, to_jsonb(o) FROM organization o;
INSERT INTO audit_trail (org_id, actor_type, actor_name, action, entity_type, entity_id, old_value, new_value, reason)
  SELECT org_id, 'user', 'admin', 'alarm_rule.update', 'alarm_rule', id, '{"action":8}', '{"action":7.5}', 'Alarm inside the 2–8 °C band per NHS SPS guidance' FROM datastream;
INSERT INTO audit_trail (org_id, actor_type, actor_name, action, entity_type, new_value)
  SELECT id, 'system', 'scheduler', 'backup.run', 'backup', '{"ok":true}' FROM organization;

DO $$
DECLARE v record; s record;
BEGIN
  SELECT * INTO v FROM verify_audit_chain();
  IF NOT v.ok OR v.checked <> 3 THEN RAISE EXCEPTION 'audit chain verification failed: %', v; END IF;
  SELECT * INTO s FROM audit_chain_state;
  IF s.last_seq <> 3 OR s.last_hash <> v.head_hash THEN RAISE EXCEPTION 'chain state mismatch: % vs %', s, v; END IF;
  RAISE NOTICE 'audit chain ok: % entries, head %', v.checked, left(v.head_hash, 16);
  -- seq must be gapless 1..3
  IF (SELECT array_agg(seq ORDER BY seq) FROM audit_trail) <> ARRAY[1,2,3]::bigint[] THEN RAISE EXCEPTION 'audit seq not gapless'; END IF;
  -- reason is mandatory for *.update
  BEGIN
    INSERT INTO audit_trail (actor_type, actor_name, action, entity_type) VALUES ('user', 'admin', 'alarm_rule.update', 'alarm_rule');
    RAISE EXCEPTION 'audit update without reason was allowed';
  EXCEPTION WHEN check_violation THEN RAISE NOTICE 'audit update without reason rejected: ok'; END;
  -- audit rows are immutable
  BEGIN
    UPDATE audit_trail SET reason = 'x' WHERE seq = 2; RAISE EXCEPTION 'UPDATE on audit_trail was allowed';
  EXCEPTION WHEN integrity_constraint_violation THEN RAISE NOTICE 'update on audit_trail rejected: ok'; END;
  BEGIN
    UPDATE audit_chain_state SET last_seq = 0; RAISE EXCEPTION 'UPDATE on audit_chain_state was allowed';
  EXCEPTION WHEN integrity_constraint_violation THEN RAISE NOTICE 'update on audit_chain_state rejected: ok'; END;
END $$;

-- 5. Tamper simulation: recompute hash of row 2 with altered content must differ from stored hash
DO $$
DECLARE r audit_trail%ROWTYPE; h text;
BEGIN
  SELECT * INTO r FROM audit_trail WHERE seq = 2;
  r.reason := 'tampered';
  h := encode(digest(r.prev_hash || '|' || r.seq::text || '|' || audit_canonical(r), 'sha256'), 'hex');
  IF h = (SELECT hash FROM audit_trail WHERE seq = 2) THEN RAISE EXCEPTION 'hash did not change on tamper'; END IF;
  RAISE NOTICE 'tamper detection ok';
END $$;

-- 6. Alarm event reason check and role history append-only
INSERT INTO escalation_policy (org_id, name, steps) SELECT id, 'default', '[{"after_s":0,"channel":"email","recipients":["ops"]}]' FROM organization;
INSERT INTO alarm_rule (org_id, datastream_id, type, warning, action, reason)
  SELECT org_id, id, 'high', 7.5, 8.0, 'initial limits per storage class 2-8C' FROM datastream;
INSERT INTO alarm (org_id, rule_id, datastream_id, severity, value)
  SELECT org_id, id, datastream_id, 'warning', 7.8 FROM alarm_rule;
DO $$
BEGIN
  BEGIN
    INSERT INTO alarm_event (alarm_id, event) SELECT id, 'acknowledged' FROM alarm;
    RAISE EXCEPTION 'acknowledge without actor/reason was allowed';
  EXCEPTION WHEN check_violation THEN RAISE NOTICE 'acknowledge without reason rejected: ok'; END;
END $$;
INSERT INTO alarm_event (alarm_id, event, actor_id, actor_name, reason) SELECT id, 'acknowledged', uuidv7(), 'operator', 'door was open for restocking' FROM alarm;

-- 7. App role: cannot UPDATE observations, can INSERT; cannot DDL
SET ROLE ctrl32_app;
INSERT INTO observation (org_id, datastream_id, measured_at, value) SELECT org_id, id, now(), 4.3 FROM datastream;
DO $$
BEGIN
  BEGIN
    UPDATE observation SET value = 0; RAISE EXCEPTION 'app role could UPDATE observation';
  EXCEPTION WHEN insufficient_privilege OR integrity_constraint_violation THEN RAISE NOTICE 'app role update on observation rejected: ok'; END;
  BEGIN
    EXECUTE 'CREATE TABLE core.evil (x int)'; RAISE EXCEPTION 'app role could CREATE TABLE';
  EXCEPTION WHEN insufficient_privilege THEN RAISE NOTICE 'app role DDL rejected: ok'; END;
END $$;
RESET ROLE;

SELECT 'SMOKE OK' AS result, (SELECT count(*) FROM observation) AS observations, (SELECT last_seq FROM audit_chain_state) AS audit_seq;

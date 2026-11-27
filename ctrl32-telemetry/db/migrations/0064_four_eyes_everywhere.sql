-- Four eyes everywhere (spec 040, ADR-0026 step 2). All of it is off until an organization switches it on.
--
-- Single actions (alarm acknowledgement and shelving, device attributes) are held as change requests like alarm limits;
-- a control marked "needs a second person" holds its command like the policy four_eyes_commands does.
ALTER TABLE core.change_request DROP CONSTRAINT IF EXISTS change_request_kind_check;
ALTER TABLE core.change_request ADD CONSTRAINT change_request_kind_check CHECK (kind IN ('limits', 'assignments', 'calibration', 'flows', 'vpn', 'alarms', 'attributes'));

-- Continuous work (moving a camera, viewing and exporting recordings, controlling displays) is approved by a second
-- person once for a limited time on one station: the session (or API token) that asked.
CREATE TABLE core.second_person_grant (
  id              uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id          uuid NOT NULL REFERENCES core.organization(id),
  area            text NOT NULL CHECK (area IN ('cameras', 'displays')),
  user_id         uuid,
  user_name       text NOT NULL,
  station         text NOT NULL DEFAULT '',          -- hashed session id, or "token:<id>"
  station_ip      text NOT NULL DEFAULT '',
  reason          text NOT NULL CHECK (length(btrim(reason)) >= 1),
  minutes         int NOT NULL CHECK (minutes BETWEEN 1 AND 480),
  requested_at    timestamptz NOT NULL DEFAULT now(),
  expires_at      timestamptz NOT NULL DEFAULT now() + interval '1 hour',  -- an unanswered request lapses
  status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'rejected', 'expired', 'ended')),
  on_spot         boolean NOT NULL DEFAULT false,    -- approved at the requester's station with the second person's credentials
  decided_by      uuid,
  decided_by_name text,
  decided_at      timestamptz,
  decision_reason text,
  valid_until     timestamptz,
  ended_at        timestamptz,
  ended_by_name   text
);
CREATE INDEX ON core.second_person_grant (org_id, status, requested_at DESC);
CREATE INDEX ON core.second_person_grant (org_id, user_id, station, area, status);
GRANT SELECT, INSERT, UPDATE ON core.second_person_grant TO ctrl32_app;

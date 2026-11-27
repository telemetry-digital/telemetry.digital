-- Operator action log (spec 039, 0.93.0, ADR-0026). When an organization turns it on (policy log_operator_actions, off
-- by default), every action of a person at the controls is kept here: each press of a control, each command, PTZ
-- move, display command, alarm acknowledgement, sign-in and sign-out, and also the pictures and faceplates they
-- opened. It is a record: rows are never changed or deleted by the application.
CREATE TABLE core.operator_action (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  ts          timestamptz NOT NULL DEFAULT now(),
  user_id     uuid,
  user_name   text NOT NULL,
  actor_type  text NOT NULL DEFAULT 'user',          -- user, api_token, system
  operator    text NOT NULL DEFAULT '',              -- the person signed in at the picture (SCADA user), when there is one
  kind        text NOT NULL CHECK (kind ~ '^[a-z][a-z_]{1,31}$'),
  target_type text NOT NULL DEFAULT '',
  target_id   uuid,
  target      text NOT NULL DEFAULT '',              -- the name of the device, camera, display, picture …
  screen      text NOT NULL DEFAULT '',              -- the page the action came from
  detail      jsonb NOT NULL DEFAULT '{}'::jsonb,
  reason      text NOT NULL DEFAULT '',
  station     text NOT NULL DEFAULT ''               -- the address of the computer
);
CREATE INDEX operator_action_org_ts_idx ON core.operator_action (org_id, ts DESC);
CREATE INDEX operator_action_user_idx ON core.operator_action (org_id, user_name, ts DESC);
SELECT core.make_append_only('core.operator_action');
GRANT SELECT, INSERT ON core.operator_action TO ctrl32_app;

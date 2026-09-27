-- Camera events and PTZ (spec 013). Events come from the camera itself over ONVIF (motion, tampering, line
-- crossing, intrusion, digital inputs — the server never decodes video), from flows (a door sensor marks the cameras)
-- or from people. A camera can record only around events ("events": pre-roll before the event, post-roll after it).
ALTER TABLE core.camera
  ADD COLUMN onvif_url     text NOT NULL DEFAULT '' CHECK (onvif_url = '' OR onvif_url ~ '^https?://'),
  ADD COLUMN ptz_profile   text NOT NULL DEFAULT '' CHECK (length(ptz_profile) <= 64),
  ADD COLUMN event_pre_s   integer NOT NULL DEFAULT 10 CHECK (event_pre_s BETWEEN 0 AND 60),
  ADD COLUMN event_post_s  integer NOT NULL DEFAULT 20 CHECK (event_post_s BETWEEN 1 AND 600),
  ADD COLUMN event_kinds   text[] NOT NULL DEFAULT '{motion,tamper,line,intrusion,input,mark}';
ALTER TABLE core.camera DROP CONSTRAINT camera_record_check;
ALTER TABLE core.camera ADD CONSTRAINT camera_record_check CHECK (record IN ('off', 'continuous', 'events'));

CREATE TABLE core.camera_event (
  id          bigserial PRIMARY KEY,
  camera_id   uuid NOT NULL REFERENCES core.camera(id),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  kind        text NOT NULL CHECK (kind IN ('motion', 'tamper', 'line', 'intrusion', 'input', 'mark', 'other')),
  source      text NOT NULL CHECK (source IN ('camera', 'flow', 'user')),
  label       text NOT NULL DEFAULT '' CHECK (length(label) <= 200),
  started_at  timestamptz NOT NULL,
  ended_at    timestamptz,
  data        jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX ON core.camera_event (camera_id, started_at);
CREATE INDEX ON core.camera_event (started_at);
GRANT SELECT, INSERT, UPDATE, DELETE ON core.camera_event TO ctrl32_app;
GRANT USAGE ON SEQUENCE core.camera_event_id_seq TO ctrl32_app;

INSERT INTO core.permission (name, description) VALUES
  ('video.ptz', 'Turn and zoom PTZ cameras')
ON CONFLICT (name) DO NOTHING;
UPDATE core.role SET permissions = array_append(permissions, 'video.ptz')
  WHERE org_id IS NULL AND name IN ('operator', 'engineer', 'org_admin') AND NOT ('video.ptz' = ANY (permissions));

-- Cameras and recordings (spec 010). The server pulls RTSP (H.264, H.265, MJPEG) or HTTP MJPEG from IP cameras,
-- records continuously into one-minute fragmented MP4 files (video.storage_dir) and streams live and recorded video to
-- browsers. Video walls are saved tile layouts over one or more screens. Recordings are personal data (GDPR): viewing
-- them needs video.playback and every playback or export is written to the audit trail.
CREATE TABLE core.camera (
  id               uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id           uuid NOT NULL REFERENCES core.organization(id),
  name             text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  location         text NOT NULL DEFAULT '' CHECK (length(location) <= 120),
  url              text NOT NULL CHECK (url ~ '^(rtsps?|https?)://'),        -- main stream, without credentials
  sub_url          text NOT NULL DEFAULT '' CHECK (sub_url = '' OR sub_url ~ '^(rtsps?|https?)://'), -- smaller stream for tiles
  username         text NOT NULL DEFAULT '',
  password_sealed  text NOT NULL DEFAULT '',                               -- AES-GCM, key outside the database
  transport        text NOT NULL DEFAULT 'auto' CHECK (transport IN ('auto', 'tcp', 'udp')),
  record           text NOT NULL DEFAULT 'continuous' CHECK (record IN ('off', 'continuous')),
  retention_days   integer NOT NULL DEFAULT 7 CHECK (retention_days BETWEEN 1 AND 3650),
  enabled          boolean NOT NULL DEFAULT true,
  sort             integer NOT NULL DEFAULT 0,
  created_at       timestamptz NOT NULL DEFAULT now(),
  created_by       text NOT NULL,
  updated_at       timestamptz NOT NULL DEFAULT now(),
  removed_at       timestamptz
);
CREATE INDEX ON core.camera (org_id) WHERE removed_at IS NULL;

-- One recorded file: a fragmented MP4 that starts with a key frame.
CREATE TABLE core.video_segment (
  id          bigserial PRIMARY KEY,
  camera_id   uuid NOT NULL REFERENCES core.camera(id),
  started_at  timestamptz NOT NULL,
  ended_at    timestamptz NOT NULL,
  path        text NOT NULL,                  -- relative to video.storage_dir
  bytes       bigint NOT NULL DEFAULT 0,
  codec       text NOT NULL,
  width       integer NOT NULL DEFAULT 0,
  height      integer NOT NULL DEFAULT 0
);
CREATE INDEX ON core.video_segment (camera_id, started_at);
CREATE INDEX ON core.video_segment (started_at);

CREATE TABLE core.video_wall (
  id          uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id      uuid NOT NULL REFERENCES core.organization(id),
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  screens     jsonb NOT NULL DEFAULT '[]'::jsonb,   -- [{name, cols, rows, tiles: [camera_id | null]}]
  created_at  timestamptz NOT NULL DEFAULT now(),
  created_by  text NOT NULL,
  updated_at  timestamptz NOT NULL DEFAULT now(),
  removed_at  timestamptz
);
CREATE INDEX ON core.video_wall (org_id) WHERE removed_at IS NULL;

INSERT INTO core.permission (name, description) VALUES
  ('video.view',     'Watch live camera video and video walls'),
  ('video.playback', 'Play back and export camera recordings'),
  ('video.manage',   'Add and configure cameras and video walls')
ON CONFLICT (name) DO NOTHING;
UPDATE core.role SET permissions = array_append(permissions, 'video.view')
  WHERE org_id IS NULL AND name IN ('operator', 'engineer', 'org_admin') AND NOT ('video.view' = ANY (permissions));
UPDATE core.role SET permissions = array_append(permissions, 'video.playback')
  WHERE org_id IS NULL AND name IN ('engineer', 'org_admin') AND NOT ('video.playback' = ANY (permissions));
UPDATE core.role SET permissions = array_append(permissions, 'video.manage')
  WHERE org_id IS NULL AND name IN ('engineer', 'org_admin') AND NOT ('video.manage' = ANY (permissions));

GRANT SELECT, INSERT, UPDATE ON core.camera, core.video_wall TO ctrl32_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON core.video_segment TO ctrl32_app;
GRANT USAGE ON SEQUENCE core.video_segment_id_seq TO ctrl32_app;

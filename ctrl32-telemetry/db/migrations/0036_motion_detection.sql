-- Motion detection on the server (spec 019) for cameras without ONVIF events: the server compares MJPEG pictures (or
-- JPEG snapshots of H.264 cameras) with the background, inside optional zones; events have source 'server'.
ALTER TABLE core.camera
  ADD COLUMN motion_detect boolean NOT NULL DEFAULT false,
  ADD COLUMN motion_sensitivity int NOT NULL DEFAULT 50 CHECK (motion_sensitivity BETWEEN 1 AND 100),
  ADD COLUMN motion_zones jsonb NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN snapshot_url text NOT NULL DEFAULT '';

ALTER TABLE core.camera_event DROP CONSTRAINT IF EXISTS camera_event_source_check;
ALTER TABLE core.camera_event ADD CONSTRAINT camera_event_source_check CHECK (source IN ('camera', 'flow', 'user', 'server'));

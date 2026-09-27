-- The sound of cameras (spec 020): off by default, because recording sound (conversations) is regulated more strictly
-- than pictures. When a camera's sound is on it is recorded with the picture; hearing it live or recorded, and
-- exporting it, needs video.audio.
ALTER TABLE core.camera ADD COLUMN audio boolean NOT NULL DEFAULT false;

INSERT INTO core.permission (name, description) VALUES
  ('video.audio', 'Hear camera sound (live and recorded) and export it')
ON CONFLICT (name) DO NOTHING;
UPDATE core.role SET permissions = array_append(permissions, 'video.audio')
  WHERE org_id IS NULL AND name IN ('org_admin') AND NOT ('video.audio' = ANY (permissions));

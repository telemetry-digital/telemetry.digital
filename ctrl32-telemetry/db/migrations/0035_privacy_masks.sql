-- Privacy masks (spec 016, GDPR): areas of a camera's picture that are covered wherever the picture is shown
-- (polygons in 0..1 picture coordinates). The server does not decode video, so masks cover the picture in every
-- player and in snapshots; the recording itself stays complete. Seeing and exporting unmasked pictures needs
-- video.unmask and is audited.
ALTER TABLE core.camera ADD COLUMN privacy_masks jsonb NOT NULL DEFAULT '[]'::jsonb;

INSERT INTO core.permission (name, description) VALUES
  ('video.unmask', 'See and export camera pictures without privacy masks')
ON CONFLICT (name) DO NOTHING;
UPDATE core.role SET permissions = array_append(permissions, 'video.unmask')
  WHERE org_id IS NULL AND name IN ('org_admin') AND NOT ('video.unmask' = ANY (permissions));

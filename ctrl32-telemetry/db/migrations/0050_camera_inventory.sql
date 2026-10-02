-- Camera inventory (spec 028): every setting of the cameras of an organization exported to Excel, CSV or PDF and
-- imported back after a crash or on a new server. A free note per camera (mounting, cabling, NVR channel) goes with it.
-- Camera passwords and relay secrets are in an export only with video.credentials_export (organization administrators
-- by default), after the user confirms their password, with a reason; every such export is audited under the same
-- name, as video.unmask is (permission names have one dot).
ALTER TABLE core.camera ADD COLUMN note text NOT NULL DEFAULT '' CHECK (length(note) <= 500);

INSERT INTO core.permission (name, description) VALUES
  ('video.credentials_export', 'Export camera passwords and relay secrets with the camera inventory')
ON CONFLICT (name) DO NOTHING;
UPDATE core.role SET permissions = array_append(permissions, 'video.credentials_export')
  WHERE org_id IS NULL AND name IN ('org_admin') AND NOT ('video.credentials_export' = ANY (permissions));

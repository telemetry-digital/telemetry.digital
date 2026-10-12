-- App option "View only" (0.80.0): a phone app for watching, controlling and playing back cameras leaves out the pages
-- that configure (camera management, camera settings, the setup of object counting). Like locked, it confines the
-- user interface only; permissions remain the security boundary. Versioned with the rest of the profile.
ALTER TABLE core.pwa_app ADD COLUMN IF NOT EXISTS view_only boolean NOT NULL DEFAULT false;

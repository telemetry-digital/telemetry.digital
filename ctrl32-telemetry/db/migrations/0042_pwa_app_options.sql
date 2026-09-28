-- App profile options (spec 021, 0.61.0): a phone app for the cameras should not lead into the whole system and should
-- not zoom the page with two fingers. Versioned with the rest of the profile (a change is a new row).
--   locked         the app window stays on the app's sections: no way into the full application (UI confinement only;
--                  permissions remain the security boundary)
--   page_zoom      false: pinch and double-tap do not zoom the page in the app window (the video player still zooms
--                  its picture)
--   camera_layout  live view and video walls: auto (one camera per row on narrow screens), single (always one per
--                  row), grid (always the grid)
-- Existing profiles keep today's behaviour (not locked, page zoom allowed, automatic layout).
ALTER TABLE core.pwa_app
  ADD COLUMN locked        boolean NOT NULL DEFAULT false,
  ADD COLUMN page_zoom     boolean NOT NULL DEFAULT true,
  ADD COLUMN camera_layout text    NOT NULL DEFAULT 'auto' CHECK (camera_layout IN ('auto', 'single', 'grid'));

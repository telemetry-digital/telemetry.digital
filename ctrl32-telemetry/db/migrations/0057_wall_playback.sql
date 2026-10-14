-- Video wall option "A click on a camera opens playback" (0.82.0): on such a wall a click on a tile opens the camera's
-- player with its recordings over the wall (only for users with video.playback; playing back is audited as on the
-- camera page). Off by default (SAFETY FIRST): a wall on a shared monitor shows live pictures only.
ALTER TABLE core.video_wall ADD COLUMN IF NOT EXISTS playback_on_click boolean NOT NULL DEFAULT false;

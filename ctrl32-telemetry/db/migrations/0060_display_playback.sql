-- A paired display may play the recordings of the cameras of its content (0.86.11): a display has no user, so anyone
-- standing at it can watch them. Off by default (SAFETY FIRST), switched on per display with a reason; it never downloads.
ALTER TABLE core.display ADD COLUMN IF NOT EXISTS allow_playback boolean NOT NULL DEFAULT false;

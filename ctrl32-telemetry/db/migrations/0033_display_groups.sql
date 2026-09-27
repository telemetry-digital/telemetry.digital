-- Displays get a location (group), e.g. "Hall A", to filter them and to send messages to all displays of a hall.
ALTER TABLE core.display ADD COLUMN location text NOT NULL DEFAULT '' CHECK (length(location) <= 60);
-- A message shown on displays (announcement, alarm) apart from the camera command, so both can be shown at once.
ALTER TABLE core.display ADD COLUMN message jsonb;

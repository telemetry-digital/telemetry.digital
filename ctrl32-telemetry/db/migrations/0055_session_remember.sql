-- "Stay signed in on this device" (0.79.0): a session opened with it lasts the organization's remember_device_days
-- (security policy, off by default) instead of the idle and absolute limits, so an installed app (PWA) on a phone
-- does not ask for the password and code every time it is opened. Logout and policy changes still end it.
ALTER TABLE core.session ADD COLUMN IF NOT EXISTS remember boolean NOT NULL DEFAULT false;

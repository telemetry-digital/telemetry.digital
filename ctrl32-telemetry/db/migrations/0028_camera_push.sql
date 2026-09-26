-- Remote cameras (spec 011): a relay at a remote site sends the video to the server over RTSPS instead of the server
-- pulling it. Such a camera has no URL on the server, only a secret the relay authenticates with (AES-GCM sealed; the
-- plaintext is shown once when it is created or rotated).
ALTER TABLE core.camera
  ADD COLUMN source text NOT NULL DEFAULT 'pull' CHECK (source IN ('pull', 'push')),
  ADD COLUMN push_secret_sealed text NOT NULL DEFAULT '';
ALTER TABLE core.camera DROP CONSTRAINT camera_url_check;
ALTER TABLE core.camera ADD CONSTRAINT camera_url_check CHECK (url ~ '^(rtsps?|https?)://' OR (source = 'push' AND url = ''));

-- Server-wide video settings changed in the UI (disk limit, file length, event merging, relay blocking, PTZ audit,
-- discovery, wall streams). One row; missing fields keep their defaults. Changes are audited with a reason.
CREATE TABLE core.video_settings (
  id          boolean PRIMARY KEY DEFAULT true CHECK (id),
  data        jsonb NOT NULL,
  updated_at  timestamptz NOT NULL DEFAULT now(),
  updated_by  uuid
);
GRANT SELECT, INSERT, UPDATE ON core.video_settings TO ctrl32_app;

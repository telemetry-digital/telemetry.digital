-- Evidence (spec 015): recordings held as evidence are never deleted by retention or the disk limit until released;
-- every recorded file keeps the SHA-256 computed while it was written; evidence packages are signed with the server's
-- Ed25519 key (the private key sealed with the configuration secret).
ALTER TABLE core.video_segment ADD COLUMN sha256 text;

CREATE TABLE core.video_hold (
  id              uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id          uuid NOT NULL REFERENCES core.organization(id),
  camera_id       uuid NOT NULL REFERENCES core.camera(id),
  from_ts         timestamptz NOT NULL,
  to_ts           timestamptz NOT NULL CHECK (to_ts > from_ts),
  title           text NOT NULL CHECK (length(title) BETWEEN 1 AND 120),
  case_ref        text NOT NULL DEFAULT '' CHECK (length(case_ref) <= 80),
  reason          text NOT NULL,
  created_by      text NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  released_at     timestamptz,
  released_by     text,
  release_reason  text
);
CREATE INDEX ON core.video_hold (camera_id, from_ts) WHERE released_at IS NULL;
GRANT SELECT, INSERT, UPDATE ON core.video_hold TO ctrl32_app;

CREATE TABLE core.server_key (
  name            text PRIMARY KEY,
  public_key      text NOT NULL,
  private_sealed  text NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT, INSERT ON core.server_key TO ctrl32_app;

INSERT INTO core.permission (name, description) VALUES
  ('video.evidence', 'Hold recordings as evidence and export signed evidence packages')
ON CONFLICT (name) DO NOTHING;
UPDATE core.role SET permissions = array_append(permissions, 'video.evidence')
  WHERE org_id IS NULL AND name IN ('engineer', 'org_admin') AND NOT ('video.evidence' = ANY (permissions));

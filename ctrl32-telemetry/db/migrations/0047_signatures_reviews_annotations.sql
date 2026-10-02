-- 0.67.0: electronic signatures (report.sign, URS H-13), audit trail reviews (audit.review, URS I-05) and annotations
-- of observations (data.annotate, URS C-11, P-03). All three are records: append-only, nothing is ever changed in place.
--
--   core.esignature    a signature of one version of a record (a stored PDF report or an audit review): who signed,
--                      the meaning, the signed hash of the record version, the re-authentication used, the manifest
--                      that was signed and the server's Ed25519 signature over its hash. A changed record no longer
--                      matches record_hash, which invalidates the signature (checked when it is shown).
--   core.audit_review  a review of the audit trail of an organization: a period (or a single entry), the range of
--                      entries it covered with the hash of the last one, the outcome and a comment; signed with the
--                      meaning "reviewed".
--   core.annotation    a comment on observations of a datastream: one observation (ts_from = ts_to) or a period.
--                      Data is never changed by it. A retraction is a new row pointing at the retracted one.

BEGIN;
SET search_path = core, public;

CREATE TABLE core.esignature (
  id               uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id           uuid NOT NULL REFERENCES core.organization(id),
  record_type      text NOT NULL CHECK (record_type IN ('report', 'audit_review')),
  record_id        uuid NOT NULL,
  record_hash      text NOT NULL CHECK (record_hash ~ '^[0-9a-f]{64}$'),   -- sha256 of the signed record version
  meaning          text NOT NULL CHECK (meaning IN ('authored', 'reviewed', 'approved', 'verified', 'responsible')),
  comment          text CHECK (comment IS NULL OR length(comment) <= 500),
  signer_id        uuid NOT NULL REFERENCES core.user_account(id),
  signer_username  text NOT NULL,
  signer_name      text NOT NULL,
  signer_roles     text[] NOT NULL,
  signed_at        timestamptz NOT NULL,
  tz               text NOT NULL,                                          -- time zone of the manifestation
  method           text NOT NULL CHECK (method IN ('password', 'password_totp')),
  manifest         jsonb NOT NULL,                                         -- exactly what was signed
  manifest_hash    text NOT NULL CHECK (manifest_hash ~ '^[0-9a-f]{64}$'), -- sha256 of the canonical manifest
  key_id           text NOT NULL,                                          -- fingerprint of the server key
  server_signature text NOT NULL                                           -- Ed25519 over manifest_hash, base64
);
SELECT core.make_append_only('core.esignature');
CREATE INDEX ON core.esignature (record_type, record_id, signed_at);
CREATE INDEX ON core.esignature (org_id, signed_at DESC);

CREATE TABLE core.audit_review (
  id                uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id            uuid NOT NULL REFERENCES core.organization(id),
  scope             text NOT NULL CHECK (scope IN ('period', 'entry')),
  period_from       timestamptz NOT NULL,
  period_to         timestamptz NOT NULL,
  from_seq          bigint,                                                -- NULL when the period had no entries
  to_seq            bigint,
  entries           bigint NOT NULL CHECK (entries >= 0),
  last_hash         text,                                                  -- hash of the entry to_seq (binds the review to the chain)
  outcome           text NOT NULL CHECK (outcome IN ('no_findings', 'findings')),
  comment           text CHECK (comment IS NULL OR length(comment) <= 2000),
  reviewer_id       uuid NOT NULL REFERENCES core.user_account(id),
  reviewer_username text NOT NULL,
  reviewer_name     text NOT NULL,
  reviewed_at       timestamptz NOT NULL DEFAULT now(),
  CHECK (period_to >= period_from),
  CHECK (outcome = 'no_findings' OR (comment IS NOT NULL AND length(btrim(comment)) >= 3))
);
SELECT core.make_append_only('core.audit_review');
CREATE INDEX ON core.audit_review (org_id, reviewed_at DESC);
CREATE INDEX ON core.audit_review (org_id, to_seq);

CREATE TABLE core.annotation (
  id              uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id          uuid NOT NULL REFERENCES core.organization(id),
  datastream_id   uuid NOT NULL REFERENCES core.datastream(id),
  ts_from         timestamptz NOT NULL,
  ts_to           timestamptz NOT NULL,
  category        text NOT NULL DEFAULT 'note'
                  CHECK (category IN ('note', 'expected_event', 'maintenance', 'calibration', 'sensor_fault', 'data_issue')),
  text            text NOT NULL CHECK (length(btrim(text)) BETWEEN 1 AND 1000),
  retracts        uuid REFERENCES core.annotation(id),                   -- set on a retraction; text is then its reason
  created_by      uuid REFERENCES core.user_account(id),
  created_by_name text NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  CHECK (ts_to >= ts_from)
);
SELECT core.make_append_only('core.annotation');
CREATE INDEX ON core.annotation (datastream_id, ts_from);
CREATE UNIQUE INDEX annotation_one_retraction ON core.annotation (retracts) WHERE retracts IS NOT NULL;

GRANT SELECT, INSERT ON core.esignature, core.audit_review, core.annotation TO ctrl32_app;

UPDATE core.permission SET description = 'Record reviews of the audit trail (signed)' WHERE name = 'audit.review';
UPDATE core.permission SET description = 'Electronically sign stored reports' WHERE name = 'report.sign';
UPDATE core.permission SET description = 'Annotate observations (comments shown on charts and in exports)' WHERE name = 'data.annotate';

COMMIT;

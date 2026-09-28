-- AI assistants over MCP (spec 022, ADR-0017, 0.62.0): OAuth 2.1 authorization for the endpoint /mcp.
--
-- An AI client (assistant, …) registers itself (RFC 7591), the user signs in and approves it (consent), and the client
-- gets short-lived access tokens and a rotating refresh token. The consent is a grant: user, client, the user's
-- permissions at that time (scopes) and whether it is read-only. Access tokens are rows of core.api_token linked to the
-- grant (grant_id), so they are checked exactly like personal API tokens (scopes ∩ the user's current permissions) and
-- they end with the grant. Tokens are stored as SHA-256 hashes only. Whether the organization allows AI access at all
-- is the setting settings->'mcp'->'enabled' of core.organization (off by default).

CREATE TABLE core.oauth_client (
  id            uuid PRIMARY KEY DEFAULT uuidv7(),
  client_id     text NOT NULL UNIQUE CHECK (client_id ~ '^c32c_[A-Za-z0-9_-]{16,64}$'),
  name          text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
  client_uri    text NOT NULL DEFAULT '' CHECK (length(client_uri) <= 300),
  redirect_uris text[] NOT NULL CHECK (cardinality(redirect_uris) BETWEEN 1 AND 10),
  created_ip    inet,
  created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE core.oauth_grant (
  id                 uuid PRIMARY KEY DEFAULT uuidv7(),
  org_id             uuid NOT NULL REFERENCES core.organization(id),
  user_id            uuid NOT NULL REFERENCES core.user_account(id),
  client_id          uuid NOT NULL REFERENCES core.oauth_client(id),
  scopes             text[] NOT NULL,
  read_only          boolean NOT NULL DEFAULT false,
  refresh_hash       text UNIQUE,
  refresh_expires_at timestamptz,
  created_at         timestamptz NOT NULL DEFAULT now(),
  last_used_at       timestamptz,
  revoked_at         timestamptz,
  revoked_by         uuid REFERENCES core.user_account(id),
  revoked_reason     text CHECK (revoked_reason IS NULL OR length(revoked_reason) <= 200)
);
CREATE INDEX ON core.oauth_grant (org_id, created_at);
CREATE INDEX ON core.oauth_grant (user_id, created_at);

-- authorization codes: single use, ten minutes, bound to the redirect address and the PKCE challenge (S256)
CREATE TABLE core.oauth_code (
  code_hash    text PRIMARY KEY,
  grant_id     uuid NOT NULL REFERENCES core.oauth_grant(id),
  redirect_uri text NOT NULL,
  challenge    text NOT NULL,
  resource     text NOT NULL DEFAULT '',
  expires_at   timestamptz NOT NULL,
  used_at      timestamptz
);

ALTER TABLE core.api_token ADD COLUMN grant_id uuid REFERENCES core.oauth_grant(id);
CREATE INDEX ON core.api_token (grant_id) WHERE grant_id IS NOT NULL;

GRANT SELECT, INSERT ON core.oauth_client TO ctrl32_app;
GRANT SELECT, INSERT, UPDATE ON core.oauth_grant, core.oauth_code TO ctrl32_app;
-- expired access tokens of grants are removed (personal API tokens are never deleted, only revoked)
GRANT DELETE ON core.api_token, core.oauth_code TO ctrl32_app;

# Implementation Plan: AI assistants over MCP

**Spec**: `specs/022-mcp/spec.md` · **Version**: 0.62.0 · **ADR**: ADR-0017

## Technical context

- Go server (`internal/httpapi`), PostgreSQL 18, plain-JS pages (ADR-0013). No new dependency: the protocol is a small
  JSON-RPC handler (`internal/mcp`), OAuth uses the existing token helpers (`iam.RandomToken`, `iam.HashToken`).
- Reused: the OpenAPI table `apiOps` and schemas (tools, `api_reference`), the mux and `require` (dispatch), the audit
  function, `api_token` (access tokens), organization `settings` jsonb, `limiter`, `productName`, `page()`.

## Design

| Part | Where | Notes |
|---|---|---|
| Protocol | `internal/mcp/mcp.go` | `Server.ServeHTTP(w, r, Backend)`; version negotiation; batches; errors; no state |
| Tools and dispatch | `internal/httpapi/mcp.go` | `mcpAreas` (tag → area), `mcpExcluded` (reasons), `mcpCatalog` (built once), `mcpBackend` (per caller: filtered tools, `Call`, resources), `mcpRun` (internal request with the principal and `viaMCPKey` → `s.mux`, `captureWriter`), `mcpAnswer` |
| Endpoint | `mcpEndpoint` | bearer only (session → 401 with resource metadata), organization switch, rate limit, grant last use |
| OAuth | `internal/httpapi/oauth.go` | metadata, `oauthRegister`, `authorizeInfo`/`authorizeDecide` (consent API), `oauthToken` (code with PKCE, refresh rotation), `oauthRevoke`, `validRedirect`, `redirectMatches`, `validResource` |
| Storage | `db/migrations/0043_mcp_oauth.sql` | `oauth_client`, `oauth_grant` (scopes, read-only, refresh hash), `oauth_code`; `api_token.grant_id` |
| Tokens | `internal/iam/service.go` | `AuthenticateToken` joins the grant (revoked grant = invalid token), `Principal.GrantID/ClientName/ReadOnly`; personal token list and revoke exclude grant tokens |
| Middleware | `internal/httpapi/middleware.go` | grant tokens only at `/mcp`; read-only principals refused for changes; audit actor "… via MCP" |
| UI | `static/oauth.html`, `settings.html` (section ai), `account.html` (section ai), `app.js` (menu), `login.html` (`?next=`) | |
| OpenAPI | `openapi.go` | tag "AI assistants (MCP)", 6 operations, schemas `MCPSettings`, `MCPGrant`, `MCPConsent`; `go run ./tools/apigen` |

## Security notes

- The consent is taken only in a signed-in browser session with CSRF (the API's POST); the page shows the client's
  name, the user, the organization and the host the code goes to.
- Codes and tokens are random 32 bytes, stored hashed; PKCE verifier compared in constant time; the code is marked used
  atomically; a second presentation revokes the grant.
- Grant tokens are refused outside `/mcp`, so an assistant's token cannot be used against the REST API directly.
- The dispatch skips only the outer middleware (authentication is already done); every handler still runs `require`.

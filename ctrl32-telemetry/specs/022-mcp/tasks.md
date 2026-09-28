# Tasks: AI assistants over MCP

- [x] T001 `internal/mcp`: JSON-RPC over Streamable HTTP (initialize, ping, tools, resources, prompts; batches;
      notifications 202; GET 405) with unit tests
- [x] T002 Migration 0043: `oauth_client`, `oauth_grant`, `oauth_code`, `api_token.grant_id`, grants for ctrl32_app
- [x] T003 iam: grant-aware token authentication, principal fields, personal tokens exclude grant tokens
- [x] T004 Tools from the OpenAPI table (areas, exclusions with reasons, read/change pairs, `api_reference`), dispatch
      through the mux, answer conversion, resources, rate limit, audit "via MCP"
- [x] T005 OAuth 2.1: metadata, registration, consent API and page, token (PKCE, rotation, reuse detection), revoke;
      login `?next=`; a new approval replaces the earlier one
- [x] T006 Organization switch and approvals API (settings, grants, disconnect), OpenAPI tag and schemas, apigen
- [x] T007 UI: Settings → AI assistants, My account → AI assistants, menu entry; translations in 19 catalogs (47 strings)
- [x] T008 Tests: unit (httpapi, mcp), e2e `mcp` (87), headless `mcp` with screenshots, a real client (AI assistant)
- [x] T009 Docs: `docs/mcp.md`, ADR-0017, threat model, URS G-11/G-12, README, JOURNAL, STAV

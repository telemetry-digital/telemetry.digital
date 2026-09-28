# Feature Specification: AI assistants over MCP

**Feature Branch**: `feature/mcp`

**Created**: 2026-09-28

**Status**: Implemented (0.62.0)

**Input**: The user (28. 9. 2026): "ďalšia vec čo ideme dopĺňať kompletne stopercentne všade mcp server a tým pádom by to
malo byť uzavreté" — an MCP server covering the whole system; after it the product is to be only polished. Chosen:
MCP in the server with OAuth (works as a assistant.example connector), tools per area, everything the user's permissions allow.

**Related**: ADR-0017, ADR-0010 (API first, tokens), ADR-0007 (security baseline); URS G-11, G-12; constitution I
(security), II (API first).

## Clarifications (defaults)

- "Everywhere" = every operation of the REST API that a user performs, now and in the future (generated from the
  OpenAPI table). Machine endpoints (devices, connectors, relays, displays, replicas), sign-in and streams are excluded
  with a reason.
- The assistant is the user: same permissions, camera groups, validation, reasons and audit. MCP adds no permission.
- Off by default per organization (SAFETY FIRST); switching needs `user.admin` and a reason.
- Reading through assistants is free; changes through assistants are paid (user, 28. 9. 2026): the same license as
  white labeling unlocks them. Only MCP changes are paid — the web UI and the REST API stay free.
- assistant.example custom connectors need OAuth 2.1 with dynamic client registration and PKCE; AI assistant and scripts may use
  a personal API token.

## User Scenarios

1. An administrator opens *Settings → AI assistants*, switches AI access on with a reason and copies the address.
2. A user adds the address as a custom connector in assistant, is sent to the sign-in page, signs in, chooses *Reading
   only* and presses *Allow*. assistant answers "which devices are offline and since when" from live data.
3. The same user connects AI assistant with an API token and asks it to create a dashboard; AI assistant asks before the
   change; the audit trail shows "user (token …) via MCP" with the reason.
4. The user disconnects the assistant under *My account → AI assistants*; its next call is refused.
5. The administrator switches AI access off; every connected assistant is refused at once.

## Requirements

- FR-001 `POST /mcp`: Model Context Protocol over Streamable HTTP (JSON answers; protocol versions 2025-11-25,
  2025-06-18, 2025-03-26, 2024-11-05): initialize (with instructions), ping, tools/list, tools/call, resources/list,
  resources/read, resources/templates/list, prompts/list; notifications answered 202; GET answered 405.
- FR-002 Tools generated from the OpenAPI table: per tag `<area>_read` (GET, readOnlyHint) and `<area>_change`
  (destructiveHint), plus `api_reference` (parameters, resolved body schema, answer; by ids or search words). A caller
  sees only operations their permissions allow; a read-only approval sees no change tools.
- FR-003 Every operation is offered or excluded with a reason (unit test); exclusions: sign-in, own password and
  two-factor, API tokens, AI access management, device/connector/relay/display/replica endpoints, WebSocket and SSE
  streams, Web Push subscriptions, UI catalog/icon/app launch, flow webhooks, replica pairing, the OpenAPI document
  (offered as a resource).
- FR-004 A tool call runs the API's handler with the caller's principal; path, query, JSON body or multipart upload
  (form fields and a base64 file); answers: JSON text, images as image content, text cut at 200 kB with a note, binary
  files as a description with the download address; failures as tool errors with the API's message.
- FR-005 Audit: changes through MCP carry "via MCP" in the actor (with the client's name for OAuth approvals).
- FR-006 OAuth 2.1: metadata (RFC 9728 for the resource, RFC 8414 for the server, also under `/mcp` suffixes), dynamic
  registration (public clients; redirects https, loopback http with any port, app schemes), consent page in the browser
  session (sign-in with `?next=` back to it), PKCE S256 required, single-use codes (10 min, reuse ends the grant),
  access tokens one hour and valid only at `/mcp`, rotating refresh tokens (90 days since last use), revocation
  (RFC 7009), `iss` in the authorization response. A new approval of the same client by the same user replaces the
  earlier one.
- FR-007 Organization setting `mcp.enabled` (default off): `GET/PUT /api/v1/mcp/settings` (`user.admin` to change, with
  reason, audited). Off: `/mcp` answers 403, consent and token requests are refused.
- FR-008 Approvals: `GET /api/v1/mcp/grants` (own; `all=1` the organization's for `user.admin`), `DELETE
  /api/v1/mcp/grants/{id}?reason=` (owner or `user.admin`) ends the approval and its tokens; audited.
- FR-009 UI: *Settings → AI assistants* (switch with reason, address, how to connect for assistant, AI assistant and other
  clients, connected assistants of the organization with disconnect), *My account → AI assistants* (own assistants,
  disconnect), consent page `/oauth/authorize`; translations in all 19 catalogs.
- FR-011 Without the server's license for changes (`white_label`) every assistant is read-only: no change tools,
  a change is refused with an explanation (the license, reading free), `ctrl32://me` says `changes_licensed: false`,
  the consent page offers only *Reading only*; settings and My account show the note and the Buy link.
- FR-010 `/mcp` refuses browser sessions (401 with `WWW-Authenticate: Bearer resource_metadata=…`), 300 calls a minute
  per user, CORS without credentials.

## Success Criteria

- SC-001 Unit: protocol (negotiation, notifications, errors, batches, GET 405); every operation offered or excluded;
  tool names, count (< 100), directions, schemas; read-only and permission filtering; `api_reference` resolves schemas;
  answer conversion (JSON, error, image, binary, cut text); redirect validation and loopback matching.
- SC-002 e2e `mcp`: discovery, registration validation, consent while off/on, request validation, deny, approve, PKCE
  and client checks, token exchange, MCP initialize/list/call/resources, changes with and without reason, audit "via
  MCP", token only at /mcp, browser session refused, refresh rotation, code reuse ends the grant, reading only,
  operator's permissions, disconnect, revocation, personal API token, switched off.
- SC-003 Headless `mcp`: settings (switch on, address), consent reached signed out (sign-in with next), consent page on
  desktop and phone, approval with reading only, redirect with code/state/iss, code exchange, My account list and
  disconnect.
- SC-004 A real client: AI assistant (`assistant -p` with `--mcp-config`) lists sites and devices and creates a site with a
  reason through the dev server.

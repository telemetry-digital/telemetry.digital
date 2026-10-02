# ADR-0017: AI assistants over MCP — the whole API as tools, OAuth 2.1, off by default

**Status:** accepted 28 Sep 2026 (user: "MCP server, kompletne stopercentne všade" — the whole system for AI assistants;
chosen options: MCP in the server with OAuth so it works as a custom connector of AI assistants, tools per area, everything the user's
permissions allow; then "na čítanie zdarma a aj na zápis platené" — reading free, writing paid, together with the
white-labeling license, for every change through MCP). Builds on ADR-0010 (API first, tokens) and ADR-0007 (security
baseline); amends ADR-0016 (a second paid capability).
**Context:** Users want to work with the system through AI assistants (MCP clients on the web, desktop and phone, in a
terminal or a code editor). The Model Context Protocol is the common way assistants reach other systems; custom
connectors of web assistants require Streamable HTTP and OAuth 2.1 with dynamic client registration.

## Decision

1. **MCP is part of the server** at `/mcp` (Streamable HTTP, JSON answers, no server stream and no session — every POST
   stands alone). No separate process, no new dependency; `internal/mcp` speaks the protocol.
2. **The whole API, generated**: tools are generated from the OpenAPI table (`apiOps`), so every operation a user can
   perform is offered and every future one is offered from the start. An operation that is not a user's request is
   excluded with a reason in code (`mcpExcluded`); a unit test fails when an operation is neither offered nor excluded.
3. **Tools per area and direction**: one `<area>_read` (GET, read-only hint) and one `<area>_change` tool per OpenAPI
   tag, plus `api_reference` for parameters and schemas. ~55 tools instead of ~290, within the limits of clients, and
   a client can let reading run without asking while it asks before changes. (Rejected: one tool per operation — too
   many for clients and context; only "search + call" — the assistant must always search first.)
4. **Same path as the API**: a tool call is dispatched to the API's own handlers with the caller's principal, so
   permissions, camera groups, validation, the mandatory reason and the audit trail are exactly those of the API. The
   audit names the assistant ("… via MCP").
5. **OAuth 2.1 authorization server in the product**: dynamic client registration (public clients only), consent page
   in the signed-in browser session, PKCE S256 only, single-use codes, one-hour access tokens valid only at `/mcp`,
   rotating refresh tokens, revocation, resource and authorization-server metadata. Personal API tokens also work at
   `/mcp` (command-line assistants, scripts). Browser sessions are not accepted at `/mcp`.
6. **SAFETY FIRST**: AI access is **off** per organization until a `user.admin` switches it on with a reason. The user
   chooses *all my permissions* or *reading only* at consent; reading only is enforced by the API too. Switching off
   refuses every connected assistant at once; disconnecting ends an approval and its tokens. Assistants do not manage
   API tokens or AI access and do not get sign-in or two-factor operations.
7. **Reading is free, changes are paid**: changes through an assistant (every `<area>_change` tool) need the paid
   license of the server — the same license as white labeling (feature `white_label`; licence model: ADR-0016 amendment of 2 Oct 2026; one
   purchase unlocks both). Without it every assistant is read-only (change tools not offered, changes refused with an
   explanation, consent offers only reading). Only MCP is affected: the web UI and the REST API with personal tokens
   stay free (ADR-0002, ADR-0010).

## Consequences

- Every new API operation must be classified for MCP (offered, or excluded with a reason) — enforced by the unit test.
- The server is an OAuth authorization server; its security (token storage as hashes, PKCE, redirect rules, rate
  limits) is part of the threat model.
- Binary answers (PDF, MP4, backups) are not streamed to assistants; they get the size and the download address.
- Live video and SSE streams stay outside MCP (a tool call is one request).

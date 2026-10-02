# AI assistants over MCP

Since 0.62.0 an AI assistant — on the web, on the desktop, on a phone, in a terminal or a code editor: any client of the
[Model Context Protocol](https://modelcontextprotocol.io/) — can work in ctrl32 telemetry on behalf of a user: read
data, find out why an alarm fired, create a dashboard, change a flow, send a command to a device or turn a camera. It
does so through the same API as the web UI, with the permissions of the user who approved it, and every change it makes
has a reason in the audit trail with the note "via MCP" (spec 022, ADR-0017).

**AI access is off in every organization** until an administrator (`user.admin`) switches it on under
*Settings → AI assistants* with a reason.

**Reading through an assistant is free. Changes through an assistant are paid**: they need the license of the server —
the same ctrl32 licence that unlocks white labeling (99,- EUR, once, forever; one purchase unlocks both; *System →
License*, `docs/whitelabel.md`). Without it every assistant only reads: the change tools are not offered, a change is
refused with an explanation, and the consent page offers only *Reading only*. The web UI and the REST API with your own
API token stay free as before.

## Connecting

The MCP address is `https://<your server>/mcp` (shown under *Settings → AI assistants* and *My account → AI
assistants*).

| Client | How |
|---|---|
| Web, desktop and phone assistants with custom connectors | In your AI assistant add a custom connector (MCP server) with the address. The assistant opens the sign-in page of this server; sign in, choose *Everything my permissions allow* or *Reading only* and press *Allow*. |
| Command-line and code assistants | Add an HTTP MCP server with the address. If the client supports OAuth, the sign-in opens in the browser; otherwise give it a personal API token (*My account → API tokens*) as the header `Authorization: Bearer c32_…`. |
| Other clients | Streamable HTTP at the address; OAuth 2.1 is discovered from the 401 answer (RFC 9728 → RFC 8414), clients register themselves (RFC 7591). A personal API token (*My account → API tokens*) works as `Authorization: Bearer`. |

Every user sees and disconnects their own assistants under *My account → AI assistants*; an administrator sees and
disconnects all of the organization's under *Settings → AI assistants*.

## What the assistant can do

Every operation of the REST API that a user performs is offered — 286 of 336 operations in 0.62.0, in 55 tools. The
unit test `TestMCPEveryOperationOfferedOrExcluded` makes sure every operation is either offered or excluded with a
reason, so a new API operation is offered to assistants from the start.

Tools come in pairs per area of the API: `<area>_read` for reading (GET, marked read-only so a client may run it without
asking) and `<area>_change` for everything else (marked as changing, so a client asks the user first). Areas:
`account`, `alarms`, `apps`, `branding`, `cameras`, `commands`, `connectors`, `dashboards`, `data`, `device_console`,
`devices`, `displays`, `exports`, `firmware`, `flows`, `front_page`, `hmi_library`, `notifications`, `organizations`,
`provisioning`, `public_dashboards`, `replication`, `reports`, `rules`, `server`, `sites`, `smart_home`, `users`.
`api_reference` returns the parameters, body schema and answer of operations (by id or by search words).

A tool call names the operation and its parameters:

```json
{"operation": "patchSitesId", "path": {"id": "…"}, "body": {"name": "Hall B", "reason": "renamed at the user's request"}}
```

Answers: JSON as text; images (camera snapshots) as images; CSV and other text as text (cut at 200 kB with a note);
binary files (PDF reports, MP4 exports, backups) are not passed to the assistant — it gets their size and the address to
download them. Resources: `ctrl32://me` (who the assistant acts as), `ctrl32://guide`, `ctrl32://openapi.json`.

### Not offered, and why

| Operations | Reason |
|---|---|
| Sign-in, sign-out, password recovery, own password, two-factor enrolment | done by the user |
| API tokens, AI access settings and approvals | an assistant does not create lasting credentials or manage AI access |
| Device, connector, relay, display and replica endpoints | authenticated by those machines, not by a user |
| Live video and recordings (WebSocket), live events (SSE) | streams; a tool call is one request — snapshots, recordings lists and MP4 exports are offered |
| Web Push subscriptions, the translation catalog, browser icon, app launch info, flow webhooks, replica pairing | belong to a browser, a page or another system |

## Security

- **Permissions**: the assistant acts as the user — the menu of operations and every call are limited by the user's
  permissions and camera groups. *Reading only* refuses every change (also directly at the API). Camera sound, unmasked
  pictures and evidence need their own permissions (`video.audio`, `video.unmask`, `video.evidence`) as in the web UI.
- **Audit**: changes are audited by the API as always, with the actor "user (AI assistant *client* via MCP)" (or
  "user (token …) via MCP" for an API token); approving, disconnecting and switching AI access are audited too.
- **Tokens**: OAuth access tokens are valid for one hour and **only at `/mcp`** (the REST API refuses them); refresh
  tokens rotate on every use and end after 90 days without use; all tokens are stored as SHA-256 hashes. PKCE (S256) is
  required, codes are single-use and valid ten minutes; a code presented twice ends its approval. Only public clients
  (no client secrets); redirect addresses must be https, http on the loopback interface (any port, RFC 8252) or an app
  scheme. A new approval of the same assistant by the same user replaces the earlier one.
- **License**: without the license of the server (white labeling and AI changes) assistants are read-only; checked on
  every call, so installing or removing the license takes effect at once.
- **Switching off** refuses every connected assistant at once (and every refresh); switching on lets them continue.
  Disconnecting an assistant ends its approval and every token issued under it at once.
- `/mcp` does not accept browser sessions (no CSRF for a JSON-RPC endpoint); it answers POST only (no server stream), is
  limited to 300 calls a minute per user and allows cross-origin requests without cookies (browser-based MCP clients).

## Protocol

Streamable HTTP, one POST per JSON-RPC message (batches of protocol 2025-03-26 are answered too), `application/json`
answers, no session id. Protocol versions 2025-11-25, 2025-06-18, 2025-03-26 and 2024-11-05. Methods: `initialize`,
`ping`, `tools/list`, `tools/call`, `resources/list`, `resources/read`, `resources/templates/list`, `prompts/list`.

| Endpoint | |
|---|---|
| `POST /mcp` | MCP (Bearer: OAuth access token or personal API token) |
| `GET /.well-known/oauth-protected-resource[/mcp]` | resource metadata (RFC 9728) |
| `GET /.well-known/oauth-authorization-server[/mcp]` | authorization server metadata (RFC 8414) |
| `POST /oauth/register` | dynamic client registration (RFC 7591), rate limited |
| `GET /oauth/authorize` | consent page (sign-in first) |
| `POST /oauth/token` | `authorization_code` with PKCE, `refresh_token` |
| `POST /oauth/revoke` | revocation (RFC 7009) |
| `/api/v1/mcp/…` | settings, approvals, consent (OpenAPI tag *AI assistants (MCP)*) |

The public address in the metadata is `http.base_url`; set it to the address users reach (for example
`https://telemetry.example.com`). The OAuth `resource` may also name the address the client reached the server with.

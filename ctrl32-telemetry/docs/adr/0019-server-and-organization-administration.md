# ADR-0019: Server administration separated from organization administration

**Status:** accepted 2 Oct 2026 (owner: "separate server administration from organization administration (security
fix) … migration must not lock anyone out … SAFETY FIRST"). Amends ADR-0012 (who may use the server operations of the
agent), ADR-0007 (roles of the security baseline) and ADR-0018 decision 9 (VPN management per organization becomes
possible).
**Context:** One server hosts several organizations (tenants, URS O-01..O-05). The built-in role `org_admin`, which the
first administrator of every organization gets, contained `system.admin` since migration 0002. `system.admin` gates
everything that belongs to the server, not to a tenant: the list of organizations and their administrators' passwords,
server settings, domain and TLS, the VPN of the whole server (peers into every customer's site networks), backups of the
whole database, updates of the binary, the licence, the front page before sign-in, the video storage settings. The
administrator of any tenant therefore controlled the whole server and every other tenant. Two further gaps made the
role boundary weaker than it looked: an administrator with `user.admin` could create an organization role holding any
permission (also `system.admin`) and could reset the password, the two-factor sign-in or the e-mail of any account of
the own organization — also of an account that administers the server.

## Decision

1. **New built-in role `server_admin`** (organization-independent, like every built-in role) with the permission
   `system.admin`, which is now described as server administration (organizations, server settings, the VPN of the whole
   server, backups, updates, licence). `system.admin` is removed from `org_admin`; `org_admin` gets `vpn.manage`
   (the VPN of the own organization, ADR-0018 amendment). Everything that was gated by `system.admin` stays server
   administration, with one exception: the full export of the caller's own organization (URS J-09) is an organization
   task and now needs `user.admin`.
2. **Nobody is locked out (migration 0045).** Every user who holds `org_admin` when the migration runs — disabled
   accounts included — is granted `server_admin` in the append-only role history (actor *system*, reason
   *migration 0045 (0.65.0): … granted to keep existing access — review*), so effective permissions do not change on the
   upgrade. New organization administrators (created with a new organization, or given `org_admin` later) do not get it.
   The administrator created from the host shell (`admin bootstrap`) gets both roles. An organization role that was
   called `server_admin` is renamed so it cannot shadow the built-in role. Where an organization's policy requires
   two-factor sign-in for `org_admin`, it now requires it for `server_admin` too (also in the defaults). **The operator
   reviews after the upgrade who holds `server_admin`** (Users → Roles shows the count; the role history names the
   migration) and revokes it where it is not needed — on a server with several customers, every customer's first
   administrator holds it until then.
3. **Only server administrators touch server administration.** Granting or removing a role that holds `system.admin`
   (`server_admin` or an organization role with it), creating or changing an organization role that holds
   `system.admin`, and any change to an account that holds `system.admin` (profile and e-mail, roles, disabling,
   password reset, two-factor reset, unlock, sign-out) need `system.admin` (403 `server_admin_required`). Every such
   change carries a reason and is in the audit trail and the role history.
4. **Server-side self-protection (not only in the dialog).** Nobody disables their own account, nobody removes their own
   administration (`user.admin` or `system.admin` through any role), and the last enabled server administrator (in an
   enabled organization) keeps `server_admin` (409 `last_server_admin`) — also when the permission is removed from an
   organization role. The checks run in the API, so they hold for the browser, API tokens and AI assistants alike.
5. **Routes with an "any of" permission.** A route may require `system.admin|vpn.manage`: the server administrator or
   the organization's VPN manager; the handler limits the latter to the own organization (ADR-0018 amendment). OpenAPI
   lists both permissions; the MCP bridge applies the same rule.

## Consequences

- A tenant's administrator manages users, roles, settings, approvals, the organization export and the VPN of the own
  organization — nothing of the server or of other tenants. The System menu appears only for server administrators.
- After the upgrade the security gain needs the operator's review (decision 2); the documentation (Users → Roles,
  permissions reference) and the release notes say so.
- The incident notifications still reach holders of `system.admin` for instance-level incidents; that list is now the
  server administrators instead of every tenant's administrator.
- Considered and rejected: moving `system.admin` holders by a heuristic (e.g. only the first organization's
  administrators) — it would silently remove access somebody relies on; a separate "super user" flag on the account —
  it would bypass the role history, the audit trail and the existing permission checks.

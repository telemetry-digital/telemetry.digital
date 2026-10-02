# Feature Specification: Server administration split, VPN phase 2

**Feature Branch**: `feature/0.65`

**Created**: 2026-10-02

**Status**: Implemented (0.65.0), not yet deployed

**Input**: The owner (2 Oct 2026). Part A, a security fix: "the built-in role org_admin of every organization contains
system.admin, so the admin of any tenant controls the whole server … new built-in role server_admin … migration must not
lock anyone out … server-side protection against disabling yourself / removing your own last admin role." Part B, VPN
phase 2: Windows server support, technician access with approval (four eyes) and time-limited grants, management per
organization, more router templates. Licence split unchanged (free up to 5 peers; unlimited, sites, time-limited and
approved technician access with the licence; no prices). SAFETY FIRST: off by default, permission-gated, reason and
audit on every change.

**Related**: ADR-0019 (server/organization administration), ADR-0018 amendment (phase 2), ADR-0020 (Windows hub),
ADR-0012, spec 024, `docs/vpn.md`, threat model.

## Clarifications (owner's decisions and defaults)

- `server_admin` holds `system.admin` only; `org_admin` loses `system.admin` and gets `vpn.manage`. Every current
  holder of `org_admin` gets `server_admin` in the migration (effective permissions unchanged); the operator reviews.
- The organization export is an organization task (`user.admin`); everything else that needed `system.admin` stays
  server administration.
- Four eyes for VPN access reuses the change requests of Settings → Approvals; the policy is per organization
  (`four_eyes_vpn`), off by default, covers technicians (user peers) only.
- Grants: 15 minutes to 30 days; presets 2 h, 8 h, 1 day, custom; licence needed.
- Windows: userspace WireGuard in the agent (no wintun, no DLL shipped); limits documented (ADR-0020).

## User Scenarios

1. **Tenant administrator.** The administrator of customer B signs in after the upgrade with `org_admin` only (a new
   organization): no System menu; *Settings → VPN* shows B's peers and rules; a rule *B's technician → B's site PLC :
   102* is accepted, *all peers → …* or SSH of the host is refused; the server's peers are invisible.
2. **Server administrator review.** After the upgrade the operator opens *Users → Roles and permissions*, sees
   *server_admin: 4 users*, opens each account and removes `server_admin` with a reason where it is not needed; removing
   it from himself or from the last server administrator is refused.
3. **Night shift with four eyes.** The policy is on. The administrator adds technician *novak* (disabled, configuration
   installed on the laptop) and requests 8 hours of access with the reason *service call 117*. The shift lead approves
   it under *Settings → Approvals*; novak reaches the PLC for 8 hours from the approval; afterwards he is disabled and
   the audit trail shows who asked, who approved and when it ended.
4. **Windows server.** On a Windows install the VPN page works as on Linux; *Runs: Windows: inside the server agent*;
   a device reaches MQTT through the tunnel; site-to-site forwarding between peers works; the server's own connectors
   cannot reach site PLCs (documented).
5. **Teltonika router.** A site peer shows a RutOS text next to OpenWrt, MikroTik, EdgeRouter, pfSense and OPNsense.

## Requirements

- FR-001 Migration 0045 (applied migrations untouched): permission `vpn.manage`; built-in role `server_admin`
  (`system.admin`); grant to current `org_admin` holders with a reason; `org_admin` without `system.admin`, with
  `vpn.manage`; MFA-required roles extended where they contain `org_admin`; `vpn_rule.org_id`; `core.vpn_grant`;
  change request kind `vpn`.
- FR-002 Server-side checks (ADR-0019): roles with `system.admin` granted/removed, created or changed only with
  `system.admin`; accounts holding `system.admin` changed only by `system.admin` (profile, roles, disable, password,
  MFA, unlock, sessions); no self-disable, no removal of own administration (`user.admin`, `system.admin`); the last
  server administrator keeps it (409). Codes `server_admin_required`, `last_server_admin`.
- FR-003 Users UI: the role shown with a count and a review note; boxes of server roles disabled without
  `system.admin`; own administration roles disabled; organization export under `user.admin`.
- FR-004 VPN per organization: routes `system.admin|vpn.manage` for peers, rules and grants (server settings and
  *Apply again* `system.admin`); scope rules of ADR-0018 decision 12, enforced in the API; `GET /api/v1/vpn` filtered
  (`scope`, no ruleset, no SSH service); *Settings → VPN* for organization managers.
- FR-005 Grants: `POST /api/v1/vpn/peers/{id}/grant {minutes, reason}`; window 15 min–30 days; replaces the open grant;
  disabling or clearing the expiry ends it; a new expiry moves it; the expiry job closes it (audited); licence (402).
- FR-006 Four eyes: policy `four_eyes_vpn` (Security policy, `PUT /api/v1/settings/policy`); technician grants held as
  change requests (202) with a summary; approval by a different user with `vpn.manage` or `system.admin` replays the
  grant (window from the approval, approver recorded); new technicians disabled; direct enabling or expiry changes 409
  `approval_required`; `GET /api/v1/vpn` shows `grant` and `pending_grant`; grants excluded from MCP.
- FR-007 Windows: agent hub (ADR-0020) with `mode` `userspace`; the UI shows the mode and the enforced rules text.
- FR-008 Templates: RutOS, EdgeRouter, pfSense, OPNsense for sites; Windows `.conf` and phone steps for every peer;
  `template_keys`, `client_templates`, `client_template_keys` in the configuration response.
- FR-009 Translations of every new string in 19 catalogs; OpenAPI and clients regenerated.

## Success Criteria

- SC-001 Unit: `iam` builds; `httpapi` (any-of permissions, VPN scope, organization rule limits, grant durations,
  route permissions, MCP exclusions); `vpn` templates; `wghub` filter and an end-to-end hub test with real WireGuard
  clients over loopback; `agent` userspace mode with a fake hub.
- SC-002 e2e `users` (server administration cases), `vpn2` (organizations, grants, four eyes, expiry, templates),
  existing `vpn`, `fouryes`, `fouryes2`, `organizations`, `server`, `mcp`, `openapi` stay green.
- SC-003 Headless `vpn` and `users` with screenshots of the changed screens.
- SC-004 Not verifiable on the development PC: Windows hub with remote peers and Windows Firewall on a real install;
  the router templates on real devices.

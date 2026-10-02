# Tasks: Server administration split, VPN phase 2

- [x] T001 Migration 0045: `server_admin`, `vpn.manage`, grants to current `org_admin` holders, `vpn_rule.org_id`, `core.vpn_grant`, change request kind `vpn`
- [x] T002 IAM and API: server-side checks of ADR-0019 (server roles, server administrators' accounts, self-protection, last server administrator); organization export under `user.admin`; bootstrap grants both roles
- [x] T003 Users UI: server_admin shown and locked without `system.admin`, review note with the count
- [x] T004 VPN per organization: any-of permission routes, scope checks for peers, rules, rotation, removal; filtered state; *Settings → VPN*
- [x] T005 Grants and four eyes: grant endpoint, `core.vpn_grant`, change requests of kind `vpn`, policy switch, disabled new technicians, approval-required errors, expiry closes grants; MCP exclusion
- [x] T006 Windows: userspace hub in the agent (`internal/wghub`, ADR-0020)
- [x] T007 Templates: RutOS, EdgeRouter, pfSense, OPNsense, Windows, phones
- [x] T008 OpenAPI, clients, translations (19 catalogs)
- [x] T009 Tests: unit, e2e `users`, `vpn2`, headless with screenshots
- [x] T010 Docs: ADR-0019, ADR-0018 amendment, ADR-0020, threat model, `docs/vpn.md`, public documentation
- [ ] T011 On a Windows server with remote peers: tunnel, MQTT through the hub, forwarding to a site, Windows Firewall rule
- [ ] T012 On real routers: RutOS, EdgeRouter, pfSense, OPNsense templates

# Tasks: Own WireGuard VPN (phase 1)

- [x] T001 Phase 0: review of listeners and bindings of a production-like install; findings and fix (firewall table) in `docs/vpn.md`
- [x] T002 `internal/vpn`: model, validation, license split, compilation, nftables and configuration rendering, router templates; unit tests
- [x] T003 Migration 0044 (`core.vpn_peer`, `core.vpn_rule`)
- [x] T004 Agent: `PUT /vpn` with atomic apply and rollback, firewall before the interface, routes, forwarding, ufw, keys in Go, simulation; unit tests with a fake runner
- [x] T005 API: peers (create, edit, disable/enable, rotate, remove), rules, apply; step-up; expiry job; import of earlier peers; OpenAPI, MCP classification, clients
- [x] T006 UI *System → VPN* with dialogs, QR, router templates, rules editor, firewall view; translations in 19 catalogs
- [x] T007 Tests: e2e `vpn`, headless `vpn`, run-all with the agent simulation for the VPN suites
- [x] T008 Docs: ADR-0018, `docs/vpn.md`, threat model, README, JOURNAL, STAV
- [ ] T009 On a Linux server: real `nft`, `wg syncconf`, routes and forwarding with two peers (technician → site host allowed, other port dropped) before production
- [ ] T010 Phase 2: Windows server (userspace WireGuard), approval of technician access by a second person, org-scoped management (needs the split of server and organization administration, ADR-0012 addendum)
- [ ] T011 Phase 3: cloud hub for servers behind NAT

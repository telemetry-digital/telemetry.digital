# Tasks: VPN phase 3 — uplink to a hub

- [x] T001 Migration 0048: `vpn_peer.uplink`, `tunnel_key_hash`, `core.vpn_uplink_rule`
- [x] T002 Model: token, validation, overlap checks, uplink rules, compilation, nftables table, `wg1.conf`; unit tests
- [x] T003 HTTPS transport (`internal/uptunnel`): hub handler and customer client; in-process test with real WireGuard
- [x] T004 Windows: uplink mode of the userspace device (filter, relay to the host and the LAN); in-process test with a hub and technicians
- [x] T005 Agent: connect, rules, transport, disconnect (Linux, Windows, simulation), rollback, forwarding with the LAN guard, endpoint watch; unit tests with a fake host and a fake device
- [x] T006 Web: uplink peer and token, rotation, tunnel endpoint, customer endpoints with step-up and audit, rules, view, synchronisation; OpenAPI, clients, MCP exclusions
- [x] T007 UI: fourth kind, token result, uplink card and dialogs; 19 catalogs
- [x] T008 Tests: e2e `vpn3` (run-all), `vpn`, `vpn2` green; Linux containers with kernel WireGuard and nftables
- [x] T009 Docs: ADR-0021, ADR-0018 note, `docs/vpn.md`, threat model, public documentation with pictures, JOURNAL, STAV, README
- [ ] T010 A real cloud hub behind Caddy (portal.telemetry.digital) with a customer server over the internet, UDP and HTTPS
- [ ] T011 The Windows uplink on a real Windows server with PLCs in its LAN
